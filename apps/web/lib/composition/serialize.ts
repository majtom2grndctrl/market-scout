import type { GrammarError, GrammarIssue, Result } from "./errors";
import { grammarError } from "./errors";
import { orderFilters, orderGroupings } from "./rules";
import { parseComposition } from "./schema";
import type { Composition, Filter } from "./vocabulary";

// The link is the call. Chat answer and page cannot disagree because they are
// the same query at two altitudes, so the version travels with it: a link
// written by an older grammar refuses rather than silently re-reading as
// something else.
const VERSION = "1";

// Every slot but `filter` is single-valued, so `f` is the one key a link may
// repeat. Each of the others maps to the composition field it fills, which is
// what a rejection about that key points at: issues from this module stay in
// the same path namespace as issues from `parseComposition`, so a consumer
// walking `path` never has to ask which namespace it is in. The version fills
// no field, so its path is empty — `grammarError` renders that as
// "composition".
const SCALAR_KEY_PATHS: Readonly<Record<string, readonly string[]>> = {
  v: [],
  m: ["measure"],
  coh: ["cohort"],
  by: ["groupBy"],
  win: ["window"],
  sort: ["sort"],
  n: ["limit"],
  enc: ["encoding"],
};

// Returns the query string alone, not a `Route`. `/view` does not exist yet,
// and a typed path belongs to the spec that builds it.
export function serializeComposition(composition: Composition): string {
  const params = new URLSearchParams();
  params.set("v", VERSION);
  params.set("m", composition.measure);
  params.set("coh", composition.cohort);

  const groupBy = orderGroupings(composition.groupBy ?? []);
  if (groupBy.length > 0) params.set("by", groupBy.join(","));

  for (const filter of orderFilters(trimValues(composition.filter ?? []))) {
    params.append("f", `${filter.dim}:${filter.value}`);
  }

  if (composition.window) params.set("win", `${composition.window.weeks}w`);
  if (composition.sort) params.set("sort", composition.sort);
  if (composition.limit !== undefined) params.set("n", String(composition.limit));
  params.set("enc", composition.encoding);

  return params.toString();
}

// The parse trims filter values, so the link has to carry the trimmed spelling
// or a composition and the composition its own link reads back as would differ
// by whitespace nobody asked about. Trimming before `orderFilters` also lets
// dedup and sort see the values the parse will — " go " and "go" are one
// filter here exactly as they are there. A value that is blank once trimmed
// still serializes, and still refuses on the way back in: serialization is
// total, and the gate is on the read.
function trimValues(filters: readonly Filter[]): readonly Filter[] {
  return filters.map((filter) => ({ dim: filter.dim, value: filter.value.trim() }));
}

// Reads the link back through `parseComposition`, so a hand-edited URL faces
// exactly the rules a model-emitted composition faces.
export function deserializeComposition(query: string): Result<Composition, GrammarError> {
  const params = new URLSearchParams(query.startsWith("?") ? query.slice(1) : query);

  // Ahead of the version gate, because a link that spells `v` twice has no one
  // version to gate on.
  const repeated = repeatedKeyIssues(params);
  if (repeated.length > 0) return { ok: false, error: grammarError(repeated) };

  const version = params.get("v");
  if (version !== VERSION) {
    return {
      ok: false,
      error: grammarError([
        {
          code: "version_unsupported",
          path: [],
          message: `link version ${version === null ? "missing" : `"${version}"`}, expected "${VERSION}"`,
        },
      ]),
    };
  }

  return parseComposition({
    measure: params.get("m"),
    cohort: params.get("coh"),
    groupBy: splitList(params.get("by")),
    filter: params.getAll("f").map(readFilter),
    window: readWindow(params.get("win")),
    sort: params.get("sort"),
    limit: params.get("n"),
    encoding: params.get("enc"),
  });
}

// `URLSearchParams.get` takes the first occurrence, which would make a doubled
// key a quietly different query: `v=1&v=2` reads as version 1 and walks
// straight through the version gate, and `by=company&by=week` drops a grouping
// with nothing said. Repetition is a spelling the grammar has no meaning for,
// so it is named rather than resolved.
function repeatedKeyIssues(params: URLSearchParams): readonly GrammarIssue[] {
  const issues: GrammarIssue[] = [];
  for (const [key, path] of Object.entries(SCALAR_KEY_PATHS)) {
    const count = params.getAll(key).length;
    if (count < 2) continue;
    issues.push({
      code: "duplicate_link_key",
      path,
      message: `link key "${key}" takes one value, got ${count}`,
    });
  }
  return issues;
}

function splitList(raw: string | null): string[] | null {
  return raw === null ? null : raw.split(",").filter((part) => part.length > 0);
}

// Malformed halves are handed to zod as-is rather than dropped, so a typo in a
// shared link produces a named rejection instead of a quietly different query.
function readFilter(entry: string): unknown {
  const separator = entry.indexOf(":");
  if (separator === -1) return entry;
  return { dim: entry.slice(0, separator), value: entry.slice(separator + 1) };
}

// Widened past digits so every `<count>w` a link can spell reaches
// `window.weeks`. Digits-only sent `0w` there for a verdict on the number
// while `-3w` and `2.5w` — the same mistake — fell through as a window that
// was not an object at all: a worse message against the wrong slot. What the
// schema says about each count is its own business; this only makes sure it is
// asked. Anything that is not `<count>w` is still handed over whole — that one
// is a malformed window, not a bad count.
function readWindow(raw: string | null): unknown {
  if (raw === null) return null;
  const weeks = /^([+-]?\d*\.?\d+)w$/.exec(raw);
  return weeks === null ? raw : { weeks: weeks[1] };
}
