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

// Search-as-you-type over one taxonomy table. Results come from the server on
// every input change; the latest request wins and earlier ones are dropped.
// The selected term stays in the item list so it remains renderable while new
// results arrive. Base UI's pattern for async search (single).

// Below this the server returns nothing, so the picker does not ask.
const MIN_TERM_LENGTH = 2;

// Input resets Base UI makes on its own when the popup closes, as opposed to
// the person deleting the text or pressing clear.
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
  // Keep unmatched text when the popup closes. Base UI otherwise resets an
  // input with no selection on focus-out, outside press, or Escape -- which
  // would make free-text entry impossible to submit.
  keepTypedText?: boolean;
}) {
  const [results, setResults] = React.useState<TaxonomyMatch[]>([]);
  const [term, setTerm] = React.useState("");
  const [error, setError] = React.useState<string | null>(null);
  const [isSearching, startSearch] = React.useTransition();
  const latest = React.useRef(0);

  const items = React.useMemo(
    () => (value && !results.some((r) => r.id === value.id) ? [...results, value] : results),
    [results, value],
  );

  const trimmed = term.trim();
  const status = isSearching
    ? "Searching…"
    : error
      ? error
      : trimmed.length > 0 && trimmed.length < MIN_TERM_LENGTH
        ? "Keep typing to search."
        : trimmed.length >= MIN_TERM_LENGTH && results.length === 0
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
          if (next.trim().length < MIN_TERM_LENGTH) {
            latest.current += 1;
            setResults([]);
            setError(null);
            return;
          }

          const request = ++latest.current;
          startSearch(async () => {
            const result = await search(next);
            if (request !== latest.current) {
              return;
            }
            startSearch(() => {
              if (result.ok) {
                setResults(result.matches);
                setError(null);
              } else {
                setResults([]);
                setError(result.error);
              }
            });
          });
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
