"use client";

import * as React from "react";

import {
  Combobox,
  ComboboxContent,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxStatus,
} from "@/components/ui/combobox";
import type { TaxonomyMatch } from "@/lib/db/taxonomy-search";

import type { TaxonomySearch } from "./action-state";

// Search-as-you-type over one taxonomy table, after Base UI's async
// single-select combobox example. The selected term stays in the item list so
// it remains renderable while new results arrive.
//
// Searches are Server Actions, and Next runs a client's actions one at a time,
// so a request per keystroke would queue -- and a save would wait behind the
// queue. Typing is debounced, and only the latest search's result is applied.

const SEARCH_DELAY_MS = 200;

// Resets Base UI makes on its own when the popup closes with nothing selected,
// as opposed to the person deleting the text ("input-change") or pressing the
// clear button ("clear-press").
const CLOSE_RESETS = new Set<string>(["focus-out", "outside-press", "escape-key", "input-clear"]);

export function TaxonomyPicker({
  id,
  label,
  placeholder,
  search,
  value,
  onValueChange,
  onInputValueChange,
  disabled = false,
  noun,
  keepTypedText = false,
}: {
  id: string;
  label: string;
  placeholder?: string;
  search: TaxonomySearch;
  value: TaxonomyMatch | null;
  onValueChange: (value: TaxonomyMatch | null) => void;
  // The text as typed, for a caller that keeps unmatched input.
  onInputValueChange?: (text: string) => void;
  disabled?: boolean;
  // "role" or "skill", for the status lines.
  noun: string;
  // Keep unmatched text when the popup closes. Without it, typed text with no
  // pick is cleared on close -- which would make free-text entry impossible.
  keepTypedText?: boolean;
}) {
  const [results, setResults] = React.useState<TaxonomyMatch[]>([]);
  const [term, setTerm] = React.useState("");
  const [error, setError] = React.useState<string | null>(null);
  const [isSearching, setIsSearching] = React.useState(false);
  // Identifies the latest search; a result tagged with an older one is dropped.
  const latest = React.useRef(0);
  const timer = React.useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  React.useEffect(() => () => clearTimeout(timer.current), []);

  const items = React.useMemo(
    () => (value && !results.some((r) => r.id === value.id) ? [...results, value] : results),
    [results, value],
  );

  function cancelSearch() {
    latest.current += 1;
    clearTimeout(timer.current);
    setIsSearching(false);
  }

  function scheduleSearch(text: string) {
    const request = ++latest.current;
    clearTimeout(timer.current);
    setIsSearching(true);
    timer.current = setTimeout(async () => {
      let next: { matches: TaxonomyMatch[]; error: string | null };
      try {
        const result = await search(text);
        next = result.ok ? { matches: result.matches, error: null } : { matches: [], error: result.error };
      } catch {
        // A rejected action -- the server unreachable, say -- must not escape
        // into the route's error boundary and take the whole page with it.
        next = { matches: [], error: "Search is unavailable right now." };
      }
      if (request !== latest.current) {
        return;
      }
      setResults(next.matches);
      setError(next.error);
      setIsSearching(false);
    }, SEARCH_DELAY_MS);
  }

  const trimmed = term.trim();
  const status = isSearching
    ? "Searching…"
    : error
      ? error
      : trimmed !== "" && results.length === 0 && value === null
        ? `No ${noun} matches “${trimmed}”.`
        : null;

  return (
    <div className="grid gap-1.5">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      <Combobox<TaxonomyMatch>
        items={items}
        value={value}
        filter={null}
        disabled={disabled}
        itemToStringLabel={(item) => item.name}
        isItemEqualToValue={(item, selected) => item.id === selected.id}
        onValueChange={(next) => {
          // A pick settles the question; a search still in flight is moot.
          cancelSearch();
          onValueChange(next);
          setError(null);
        }}
        inputValue={term}
        onInputValueChange={(next, { reason }) => {
          if (keepTypedText && next === "" && CLOSE_RESETS.has(reason)) {
            return;
          }
          setTerm(next);
          onInputValueChange?.(next);
          // Choosing an item writes its label into the input; that is not a search.
          if (reason === "item-press") {
            return;
          }
          // Typing away from a pick un-picks it, so the form never submits a
          // match the text no longer shows.
          if (reason === "input-change" && value !== null && next !== value.name) {
            onValueChange(null);
          }
          if (next.trim() === "") {
            cancelSearch();
            setResults([]);
            setError(null);
            return;
          }
          if (reason === "input-change") {
            scheduleSearch(next);
          }
        }}
      >
        <ComboboxInput id={id} placeholder={placeholder} showClear={value !== null} disabled={disabled} />
        <ComboboxContent aria-busy={isSearching || undefined}>
          {/* The status line carries every empty and no-match message, so
              Combobox.Empty is not used. */}
          <ComboboxStatus>{status}</ComboboxStatus>
          <ComboboxList>
            {(item: TaxonomyMatch) => (
              <ComboboxItem key={item.id} value={item}>
                <span className="flex min-w-0 flex-col">
                  <span className="truncate">{item.name}</span>
                  <span className="truncate text-xs text-content-muted">
                    {/* Classification links, not postings: one posting can be
                        classified more than once. */}
                    {item.usageCount === 0
                      ? "Not yet tagged on a posting"
                      : item.usageCount === 1
                        ? "Tagged once"
                        : `Tagged ${item.usageCount.toLocaleString()} times`}
                  </span>
                </span>
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
    </div>
  );
}
