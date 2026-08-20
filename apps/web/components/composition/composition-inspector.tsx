"use client";

import { useId, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import {
  allowedCohorts,
  COHORTS,
  defaultCohort,
  ENCODINGS,
  FILTER_DIMENSIONS,
  GROUPINGS,
  MEASURES,
  parseComposition,
  requiresDenominator,
  SEED_COMPOSITIONS,
  serializeComposition,
  SORTS,
  validate,
  type Composition,
  type Filter,
  type GrammarIssue,
  type SeedName,
} from "@/lib/composition";
import { cn } from "@/lib/utils";

// The first prototype of the compose interaction: load a seed, mutate a slot,
// watch the grammar re-validate and the link re-serialize. It computes nothing
// — no rows, no chart. Every rule it reports comes from `lib/composition`;
// nothing here decides what is valid.
// See: agent-context/plans/in-progress/composition-grammar/index.md

// Slots an issue can be pinned to, keyed by the first segment of its path. An
// issue with no slot — a shape error zod raises above the field level, which
// carries an empty path — falls through to the verdict panel rather than
// vanishing.
const SLOTS = [
  "measure",
  "cohort",
  "groupBy",
  "filter",
  "window",
  "sort",
  "limit",
  "encoding",
] as const satisfies readonly (keyof Composition)[];

type SlotName = (typeof SLOTS)[number];

// Pinned in both directions. `satisfies` above refuses a slot `Composition` has
// no field for; this refuses a field with no slot. Renaming a field is a
// compile error here rather than a slot that silently stops receiving refusals.
type AssertNever<T extends never> = T;
type UnroutedField = AssertNever<Exclude<keyof Composition, SlotName>>;

const CONTROL =
  "h-8 w-full rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 dark:bg-input/30";

// Window and Limit are the only controls that can hold something no
// `Composition` can: `1e` is neither a number nor a cleared field. The text is
// kept beside the draft so what was typed stays on screen while the raw value
// reaches the parse.
interface ModifierText {
  readonly window: string;
  readonly limit: string;
}

export function CompositionInspector({
  initialSeed = "demandTrend",
}: {
  initialSeed?: SeedName;
}) {
  // `initialSeed` seeds the draft at mount and is never read again — after that
  // the draft belongs to whoever is editing it, not to the caller. A caller
  // that needs to reseed remounts with a `key`; the story's render does exactly
  // that so the Controls-panel select stays live.
  const [loadedSeed, setLoadedSeed] = useState<SeedName>(initialSeed);
  const [draft, setDraft] = useState<Composition>(() => SEED_COMPOSITIONS[initialSeed]);
  const [modifierText, setModifierText] = useState<ModifierText>(() =>
    modifierTextOf(SEED_COMPOSITIONS[initialSeed]),
  );
  const ids = useSlotIds();

  const parsed = parseComposition(draft);
  // Both surfaces are asked, and their issues merged. `parseComposition` is the
  // only one that reports shape, but zod skips its refinements entirely once
  // the base shape fails — so a single blank filter row would take every
  // crossing verdict down with it. `validate` runs the crossing rules on their
  // own and normalizes internally, so it reaches the same verdict on the raw
  // draft that the parse reaches on the normalized value.
  const validation = validate(draft);
  const issues = mergeIssues(
    parsed.ok ? [] : parsed.error.issues,
    validation.ok ? [] : validation.error.issues,
  );

  // Raw while rejected, canonical once accepted. Serializing the draft is what
  // keeps the link live through an edit the grammar refuses — serialization
  // does not judge. But it canonicalizes only ordering and exact-string dedup,
  // never the trim the parse applies, so an accepted composition serializes
  // from the parsed value instead: the Link and Canonical form panels cannot
  // disagree about filter count or spelling.
  const link = serializeComposition(parsed.ok ? parsed.value : draft);

  // Coverage debt is read off `validate`, but only for a composition that also
  // parsed: a half-typed filter row still carries a dimension, and attributing
  // a denominator to it would claim a slice the composition has not asked for.
  const denominator = parsed.ok && validation.ok ? validation : null;

  function edit(patch: Partial<Composition>) {
    setDraft((previous) => ({ ...previous, ...patch }));
  }

  function loadSeed(name: SeedName) {
    setLoadedSeed(name);
    setDraft(SEED_COMPOSITIONS[name]);
    setModifierText(modifierTextOf(SEED_COMPOSITIONS[name]));
  }

  function editWindow(raw: string) {
    setModifierText((text) => ({ ...text, window: raw }));
    edit({ window: readNumber(raw, (weeks) => ({ weeks })) });
  }

  function editLimit(raw: string) {
    setModifierText((text) => ({ ...text, limit: raw }));
    edit({ limit: readNumber(raw, (limit) => limit) });
  }

  const groupBy = draft.groupBy ?? [];
  const filters = draft.filter ?? [];

  return (
    <div className="space-y-6 p-4 text-foreground sm:p-6">
      <header className="space-y-2">
        <h2 className="text-lg font-semibold tracking-tight">Composition Inspector</h2>
        <p className="max-w-[65ch] text-sm text-muted-foreground">
          Load a seed, change a slot, watch the verdict and the link move. Every
          control is the closed enum it edits, so an out-of-vocabulary value is
          not offered. Nothing here is computed — no rows, no chart.
        </p>
      </header>

      <div className="space-y-2">
        <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Seeds
        </span>
        <div className="flex flex-wrap gap-2">
          {seedNames().map((name) => (
            <Button
              key={name}
              size="sm"
              variant={name === loadedSeed ? "secondary" : "outline"}
              onClick={() => loadSeed(name)}
            >
              {humanize(name)}
            </Button>
          ))}
        </div>
        <p className="text-xs text-muted-foreground">
          Loading a seed replaces the draft. The highlighted seed is the one last
          loaded, not a claim the draft still matches it.
        </p>
      </div>

      <Separator />

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,24rem)]">
        <div className="grid gap-5 sm:grid-cols-2">
          <Slot label="Measure" labelFor={ids.measure} issues={issuesFor(issues, "measure")}>
            <EnumSelect
              id={ids.measure}
              options={MEASURES}
              value={draft.measure}
              invalid={issuesFor(issues, "measure").length > 0}
              onSelect={(measure) => {
                if (measure) edit({ measure });
              }}
            />
          </Slot>

          <Slot
            label="Cohort"
            labelFor={ids.cohort}
            // Read off the rules table rather than restated here: switching the
            // measure moves this line, which is the point of leaving an
            // out-of-set cohort selectable.
            hint={`${draft.measure}: ${allowedCohorts(draft.measure).join(" · ")} (default ${defaultCohort(draft.measure)})`}
            issues={issuesFor(issues, "cohort")}
          >
            <EnumSelect
              id={ids.cohort}
              options={COHORTS}
              value={draft.cohort}
              invalid={issuesFor(issues, "cohort").length > 0}
              onSelect={(cohort) => {
                if (cohort) edit({ cohort });
              }}
            />
          </Slot>

          <Slot
            label="Group by"
            hint="✳ marks coverage debt"
            issues={issuesFor(issues, "groupBy")}
            className="sm:col-span-2"
          >
            <div className="flex flex-wrap gap-x-4 gap-y-2">
              {GROUPINGS.map((grouping) => (
                <label
                  key={grouping}
                  className="flex items-center gap-1.5 text-sm"
                  htmlFor={`${ids.groupBy}-${grouping}`}
                >
                  <input
                    id={`${ids.groupBy}-${grouping}`}
                    type="checkbox"
                    className="size-4 accent-primary"
                    checked={groupBy.includes(grouping)}
                    // The refusals that target this slot are about the set, not
                    // one box, so every box carries the signal the group does.
                    aria-invalid={issuesFor(issues, "groupBy").length > 0 || undefined}
                    onChange={(event) =>
                      edit({
                        groupBy: event.target.checked
                          ? [...groupBy, grouping]
                          : groupBy.filter((entry) => entry !== grouping),
                      })
                    }
                  />
                  {grouping}
                  {requiresDenominator(grouping) ? (
                    <span
                      aria-label="requires a denominator"
                      className="text-muted-foreground"
                    >
                      ✳
                    </span>
                  ) : null}
                </label>
              ))}
            </div>
          </Slot>

          <Slot
            label="Filter"
            hint="scopes the corpus before the measure runs"
            issues={filterSlotIssues(issues, filters.length)}
            className="sm:col-span-2"
          >
            <div className="space-y-2">
              {filters.map((filter, index) => {
                const rowIssues = filterRowIssues(issues, index);
                return (
                  // Positional key. Removing a row renumbers every row below
                  // it, so the key is not stable across edits — safe only
                  // because both controls in a row are fully controlled: React
                  // overwrites a reused node with the correct value, and no
                  // uncontrolled or focus-bearing state rides along to be
                  // carried onto the wrong row.
                  <div key={index} className="space-y-1">
                    <div className="flex items-center gap-2">
                      <EnumSelect
                        aria-label={`Filter ${index + 1} dimension`}
                        options={FILTER_DIMENSIONS}
                        value={filter.dim}
                        invalid={rowControlInvalid(rowIssues, "dim")}
                        className="w-40"
                        onSelect={(dim) => {
                          if (dim) edit({ filter: replaceAt(filters, index, { ...filter, dim }) });
                        }}
                      />
                      <Input
                        aria-label={`Filter ${index + 1} value`}
                        placeholder="value"
                        value={filter.value}
                        aria-invalid={rowControlInvalid(rowIssues, "value") || undefined}
                        onChange={(event) =>
                          edit({
                            filter: replaceAt(filters, index, {
                              ...filter,
                              value: event.target.value,
                            }),
                          })
                        }
                      />
                      <Button
                        size="sm"
                        variant="ghost"
                        aria-label={`Remove filter ${index + 1}`}
                        onClick={() =>
                          edit({ filter: filters.filter((_, position) => position !== index) })
                        }
                      >
                        Remove
                      </Button>
                    </div>
                    <IssueList issues={rowIssues} prefix={`Filter ${index + 1}`} />
                  </div>
                );
              })}
              <Button
                size="sm"
                variant="outline"
                onClick={() =>
                  edit({
                    filter: [...filters, { dim: FILTER_DIMENSIONS[0], value: "" }],
                  })
                }
              >
                Add filter
              </Button>
            </div>
          </Slot>

          <Slot
            label="Window"
            labelFor={ids.window}
            hint="whole weeks; blank for none"
            issues={issuesFor(issues, "window")}
          >
            <Input
              id={ids.window}
              // Deliberately not `type="number"`: that control reports anything
              // it cannot parse as the empty string, which is the same signal a
              // cleared field sends — so the modifier was dropped while the text
              // stayed on screen, a control that visibly did nothing.
              type="text"
              inputMode="numeric"
              placeholder="none"
              value={modifierText.window}
              aria-invalid={issuesFor(issues, "window").length > 0 || undefined}
              onChange={(event) => editWindow(event.target.value)}
            />
          </Slot>

          <Slot
            label="Limit"
            labelFor={ids.limit}
            hint="blank for none"
            issues={issuesFor(issues, "limit")}
          >
            <Input
              id={ids.limit}
              type="text"
              inputMode="numeric"
              placeholder="none"
              value={modifierText.limit}
              aria-invalid={issuesFor(issues, "limit").length > 0 || undefined}
              onChange={(event) => editLimit(event.target.value)}
            />
          </Slot>

          <Slot label="Sort" labelFor={ids.sort} issues={issuesFor(issues, "sort")}>
            <EnumSelect
              id={ids.sort}
              options={SORTS}
              value={draft.sort}
              unsetLabel="none"
              invalid={issuesFor(issues, "sort").length > 0}
              onSelect={(sort) => edit({ sort })}
            />
          </Slot>

          <Slot label="Encoding" labelFor={ids.encoding} issues={issuesFor(issues, "encoding")}>
            <EnumSelect
              id={ids.encoding}
              options={ENCODINGS}
              value={draft.encoding}
              invalid={issuesFor(issues, "encoding").length > 0}
              onSelect={(encoding) => {
                if (encoding) edit({ encoding });
              }}
            />
          </Slot>
        </div>

        <div className="space-y-4">
          <section className="space-y-2 rounded-lg border bg-card p-4">
            <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              Link
            </h3>
            <code className="block break-all font-mono text-xs">{link}</code>
          </section>

          <section className="space-y-3 rounded-lg border bg-card p-4">
            <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              Verdict
            </h3>
            <p className={cn("text-sm font-medium", !parsed.ok && "text-destructive")}>
              {parsed.ok ? "Accepted" : "Rejected"}
            </p>

            {/* Issues already pinned to a slot are shown there; only the ones
                with nowhere to land repeat here. */}
            <IssueList issues={issues.filter((issue) => slotOf(issue) === undefined)} />

            <div className="space-y-1 text-sm">
              <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Denominator
              </p>
              {denominator === null ? (
                <p className="text-muted-foreground">
                  Unknown while rejected — coverage debt is marked ✳ per grouping above.
                </p>
              ) : (
                <p>
                  {denominator.requiresDenominator
                    ? `Required — ${denominator.denominatorGroupings.join(", ")}`
                    : "Not required"}
                </p>
              )}
            </div>

            {parsed.ok ? (
              <div className="space-y-1">
                <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                  Canonical form
                </p>
                <pre className="overflow-x-auto font-mono text-xs">
                  {JSON.stringify(parsed.value, null, 2)}
                </pre>
              </div>
            ) : null}
          </section>
        </div>
      </div>
    </div>
  );
}

function Slot({
  label,
  labelFor,
  hint,
  issues,
  className,
  children,
}: {
  label: string;
  labelFor?: string;
  hint?: string;
  issues: readonly GrammarIssue[];
  className?: string;
  children: React.ReactNode;
}) {
  const headingId = useId();
  const heading = (
    <div className="flex flex-wrap items-baseline justify-between gap-x-2">
      {labelFor === undefined ? (
        <span
          id={headingId}
          className="text-xs font-medium uppercase tracking-wider text-muted-foreground"
        >
          {label}
        </span>
      ) : (
        <label
          htmlFor={labelFor}
          className="text-xs font-medium uppercase tracking-wider text-muted-foreground"
        >
          {label}
        </label>
      )}
      {hint === undefined ? null : (
        <span className="text-xs text-muted-foreground">{hint}</span>
      )}
    </div>
  );

  return (
    <div className={cn("space-y-1.5", className)}>
      {heading}
      {labelFor === undefined ? (
        // No `aria-invalid` here: `group` is not one of the roles ARIA allows
        // it on, so the signal goes on the widgets inside instead — every
        // checkbox for `groupBy`, the two controls of the offending row for
        // `filter`.
        <div role="group" aria-labelledby={headingId}>
          {children}
        </div>
      ) : (
        children
      )}
      <IssueList issues={issues} />
    </div>
  );
}

// `prefix` names the row an issue belongs to where one heading covers several —
// without it two filter rows render byte-identical, unattributable lines.
function IssueList({
  issues,
  prefix,
}: {
  issues: readonly GrammarIssue[];
  prefix?: string;
}) {
  if (issues.length === 0) return null;

  return (
    <ul className="space-y-1">
      {issues.map((issue, index) => (
        // Position is part of the key because it is the only unique part: zod
        // raises several issues at one path, and every non-refinement issue
        // collapses to the code "shape".
        <li
          key={`${index}:${issue.code}:${issue.path.join(".")}`}
          className="text-xs text-destructive"
        >
          {prefix === undefined ? null : <span>{prefix} · </span>}
          <span className="font-mono">{issue.code}</span> — {issue.message}
        </li>
      ))}
    </ul>
  );
}

// One control shape for every closed enum, so a vocabulary change reaches the
// UI by widening the array it was handed and nothing else. `onSelect` sees
// `undefined` only where `unsetLabel` renders a blank option to select.
function EnumSelect<T extends string>({
  id,
  options,
  value,
  unsetLabel,
  invalid,
  className,
  onSelect,
  ...rest
}: {
  id?: string;
  options: readonly T[];
  value: T | undefined;
  unsetLabel?: string;
  invalid?: boolean;
  className?: string;
  onSelect: (value: T | undefined) => void;
} & Pick<React.ComponentProps<"select">, "aria-label">) {
  return (
    <select
      {...rest}
      id={id}
      className={cn(CONTROL, className)}
      value={value ?? ""}
      aria-invalid={invalid || undefined}
      onChange={(event) =>
        onSelect(event.target.value === "" ? undefined : (event.target.value as T))
      }
    >
      {unsetLabel === undefined ? null : <option value="">{unsetLabel}</option>}
      {options.map((option) => (
        <option key={option} value={option}>
          {option}
        </option>
      ))}
    </select>
  );
}

function useSlotIds(): Record<SlotName, string> {
  const prefix = useId();
  return Object.fromEntries(SLOTS.map((slot) => [slot, `${prefix}-${slot}`])) as Record<
    SlotName,
    string
  >;
}

// Shape and crossing issues arrive from two calls that overlap: whenever the
// base shape parses, the refinement inside `parseComposition` has already run
// the same rules `validate` runs. One crossing is at most one line, so the
// second copy is dropped. Shape issues are never deduped — zod legitimately
// raises several at one path, and each names a different thing.
function mergeIssues(
  shape: readonly GrammarIssue[],
  crossing: readonly GrammarIssue[],
): readonly GrammarIssue[] {
  const reported = new Set(shape.map(issueKey));
  return [...shape, ...crossing.filter((issue) => !reported.has(issueKey(issue)))];
}

function issueKey(issue: GrammarIssue): string {
  return `${issue.code}:${issue.path.join(".")}`;
}

function slotOf(issue: GrammarIssue): SlotName | undefined {
  const head = issue.path[0];
  return SLOTS.find((slot) => slot === head);
}

function issuesFor(issues: readonly GrammarIssue[], slot: SlotName): readonly GrammarIssue[] {
  return issues.filter((issue) => slotOf(issue) === slot);
}

// A filter issue carries its row in `path[1]`, so it lands under the row that
// produced it instead of joining an undifferentiated list under one heading.
function filterRowIssues(issues: readonly GrammarIssue[], row: number): readonly GrammarIssue[] {
  return issuesFor(issues, "filter").filter((issue) => issue.path[1] === row);
}

// Whatever no row claimed: an issue on the array itself, or on a row index the
// draft no longer has.
function filterSlotIssues(issues: readonly GrammarIssue[], rows: number): readonly GrammarIssue[] {
  return issuesFor(issues, "filter").filter((issue) => {
    const row = issue.path[1];
    return typeof row !== "number" || row < 0 || row >= rows;
  });
}

// `path[2]` names the control within the row; an issue on the row as a whole
// has no third segment and marks both controls.
function rowControlInvalid(
  issues: readonly GrammarIssue[],
  control: keyof Filter,
): boolean {
  return issues.some((issue) => issue.path[2] === undefined || issue.path[2] === control);
}

function seedNames(): readonly SeedName[] {
  return Object.keys(SEED_COMPOSITIONS) as SeedName[];
}

// Seeds carry no display names, so the key is the label: "roleLifecycle" reads
// as "Role lifecycle". A seed added to the module shows up here unaided.
function humanize(name: string): string {
  const spaced = name.replace(/([A-Z])/g, " $1").toLowerCase();
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

// What the two numeric controls display, seeded from the composition so a
// loaded seed shows its own values. After that the field owns the text: what
// was typed stays visible whether or not the grammar can read it.
function modifierTextOf(composition: Composition): ModifierText {
  return {
    window: composition.window === undefined ? "" : String(composition.window.weeks),
    limit: composition.limit === undefined ? "" : String(composition.limit),
  };
}

// A blank field clears the modifier. Anything else is handed on as typed, so
// `0`, `1e`, and `1e999` all reach the parse and come back as a named rejection
// on their own slot. `Number` is what makes an unreadable field visible rather
// than swallowed: it yields `NaN`, which the schema refuses by name.
function readNumber<T>(raw: string, wrap: (value: number) => T): T | undefined {
  if (raw.trim() === "") return undefined;
  return wrap(Number(raw));
}

function replaceAt(filters: readonly Filter[], index: number, filter: Filter): Filter[] {
  return filters.map((entry, position) => (position === index ? filter : entry));
}
