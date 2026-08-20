import { describe, expect, it } from "vitest";

import {
  COHORTS,
  GROUPINGS,
  MEASURES,
  SEED_COMPOSITIONS,
  allowedCohorts,
  defaultCohort,
  deserializeComposition,
  parseComposition,
  requiresDenominator,
  serializeComposition,
  validate,
} from "./index";
import type { Cohort, Composition, GrammarIssueCode, Grouping, Measure } from "./index";

// A parse that refused for a reason nobody named is a test passing for the
// wrong reason, so every rejection assertion names the code it expects.
function codesOf(input: unknown): readonly GrammarIssueCode[] {
  const parsed = parseComposition(input);
  return parsed.ok ? [] : parsed.error.issues.map((issue) => issue.code);
}

function valueOf(input: unknown): Composition {
  const parsed = parseComposition(input);
  if (!parsed.ok) throw new Error(`expected a valid composition, got: ${parsed.error.message}`);
  return parsed.value;
}

describe("seed round-trip", () => {
  const seeds = Object.entries(SEED_COMPOSITIONS);

  it.each(seeds)("%s survives serialize → deserialize deep-equal", (_name, seed) => {
    const back = deserializeComposition(serializeComposition(seed));

    expect(back.ok).toBe(true);
    if (!back.ok) return;
    expect(back.value).toStrictEqual(seed);
  });

  it.each(seeds)("%s is already canonical — parsing it changes nothing", (_name, seed) => {
    expect(valueOf(seed)).toStrictEqual(seed);
  });

  it.each(seeds)("%s is a crossing the rules sanction", (_name, seed) => {
    expect(validate(seed).ok).toBe(true);
  });

  // The link in the spec's target usage, character for character.
  it("emits the documented link for Movers", () => {
    expect(serializeComposition(SEED_COMPOSITIONS.movers)).toBe(
      "v=1&m=delta&coh=open&by=company&win=2w&sort=desc&n=20&enc=diverging_bars",
    );
  });
});

describe("cohort per measure", () => {
  // The Composability table, transcribed. Cohort is free on the count measures
  // and intrinsic on the time measures.
  const EXPECTED: Record<Measure, { allowed: readonly Cohort[]; fallback: Cohort }> = {
    count: { allowed: ["open", "closed", "all"], fallback: "open" },
    delta: { allowed: ["open", "closed", "all"], fallback: "open" },
    age: { allowed: ["open"], fallback: "open" },
    lifespan: { allowed: ["closed"], fallback: "closed" },
    rate: { allowed: ["all"], fallback: "all" },
    share: { allowed: ["open"], fallback: "open" },
  };

  // An otherwise-valid composition per measure, so varying only the cohort
  // isolates the cohort rule from the encoding rules.
  const BASE: Record<Measure, Omit<Composition, "measure" | "cohort">> = {
    count: { groupBy: ["company"], encoding: "ranked_bars" },
    delta: { groupBy: ["company"], window: { weeks: 2 }, encoding: "diverging_bars" },
    age: { encoding: "histogram" },
    lifespan: { encoding: "histogram" },
    rate: { groupBy: ["week"], encoding: "line" },
    share: { groupBy: ["company"], encoding: "ranked_bars" },
  };

  it.each(MEASURES.map((measure) => [measure] as [Measure]))(
    "%s reports the allowed set and default from the table",
    (measure) => {
      expect(allowedCohorts(measure)).toStrictEqual(EXPECTED[measure].allowed);
      expect(defaultCohort(measure)).toBe(EXPECTED[measure].fallback);
    },
  );

  const matrix = MEASURES.flatMap((measure) =>
    COHORTS.map((cohort) => [measure, cohort] as [Measure, Cohort]),
  );

  it.each(matrix)("%s + %s parses only when the table allows it", (measure, cohort) => {
    const allowed = EXPECTED[measure].allowed.includes(cohort);
    const codes = codesOf({ ...BASE[measure], measure, cohort });

    expect(codes).toStrictEqual(allowed ? [] : ["cohort_not_allowed"]);
  });

  it.each(MEASURES.map((measure) => [measure] as [Measure]))(
    "%s fills its default cohort when none is given",
    (measure) => {
      expect(valueOf({ ...BASE[measure], measure }).cohort).toBe(EXPECTED[measure].fallback);
    },
  );
});

describe("encoding must fit the result shape", () => {
  const rejected: [string, unknown, GrammarIssueCode][] = [
    [
      "line without week in groupBy",
      { measure: "count", groupBy: ["company"], encoding: "line" },
      "encoding_needs_week",
    ],
    ["line with no groupBy at all", { measure: "count", encoding: "line" }, "encoding_needs_week"],
    ["area without week in groupBy", { measure: "count", encoding: "area" }, "encoding_needs_week"],
    [
      "histogram over an aggregate measure",
      { measure: "count", encoding: "histogram" },
      "encoding_needs_per_posting_measure",
    ],
    [
      "diverging_bars over an unsigned measure",
      { measure: "count", encoding: "diverging_bars" },
      "encoding_needs_signed_measure",
    ],
    [
      "stacked_bar with one grouping",
      { measure: "count", groupBy: ["company"], encoding: "stacked_bar" },
      "encoding_needs_two_groupings",
    ],
    [
      "stacked_bar with three groupings",
      { measure: "count", groupBy: ["company", "role", "week"], encoding: "stacked_bar" },
      "encoding_needs_two_groupings",
    ],
    [
      "stacked_bar with no grouping",
      { measure: "count", encoding: "stacked_bar" },
      "encoding_needs_two_groupings",
    ],
    [
      "share with no grouping to normalize within",
      { measure: "share", encoding: "ranked_bars" },
      "measure_needs_grouping",
    ],
    [
      "histogram with two groupings",
      { measure: "lifespan", groupBy: ["company", "role"], encoding: "histogram" },
      "encoding_needs_at_most_one_grouping",
    ],
    [
      "histogram with three groupings",
      { measure: "lifespan", groupBy: ["company", "role", "skill"], encoding: "histogram" },
      "encoding_needs_at_most_one_grouping",
    ],
    [
      "delta with no window to difference over",
      { measure: "delta", groupBy: ["company"], encoding: "diverging_bars" },
      "measure_needs_window",
    ],
    [
      "delta with an explicitly null window",
      { measure: "delta", groupBy: ["company"], window: null, encoding: "diverging_bars" },
      "measure_needs_window",
    ],
    [
      "age rendered by a non-distribution encoding",
      { measure: "age", encoding: "ranked_bars" },
      "measure_needs_distribution_encoding",
    ],
    [
      "lifespan rendered by a non-distribution encoding",
      { measure: "lifespan", encoding: "ranked_bars" },
      "measure_needs_distribution_encoding",
    ],
  ];

  it.each(rejected)("rejects %s", (_name, input, code) => {
    expect(codesOf(input)).toStrictEqual([code]);
  });

  const accepted: [string, unknown][] = [
    ["line with week", { measure: "count", groupBy: ["company", "week"], encoding: "line" }],
    ["area with week", { measure: "rate", groupBy: ["week"], encoding: "area" }],
    ["histogram over a per-posting measure", { measure: "age", encoding: "histogram" }],
    [
      "histogram grouped into small multiples",
      { measure: "lifespan", groupBy: ["role"], encoding: "histogram" },
    ],
    [
      "diverging_bars over the signed measure",
      { measure: "delta", groupBy: ["company"], window: { weeks: 2 }, encoding: "diverging_bars" },
    ],
    [
      "stacked_bar with exactly two groupings",
      { measure: "share", groupBy: ["company", "seniority"], encoding: "stacked_bar" },
    ],
    ["table with no grouping", { measure: "count", encoding: "table" }],
    ["table renders a per-posting measure as its rows", { measure: "age", encoding: "table" }],
    [
      "sort and limit with no grouping resolve one ordered figure",
      { measure: "count", sort: "desc", limit: 5, encoding: "table" },
    ],
    [
      "sort and limit over a grouping they can act on",
      { measure: "count", groupBy: ["company"], sort: "desc", limit: 5, encoding: "ranked_bars" },
    ],
  ];

  it.each(accepted)("accepts %s", (_name, input) => {
    expect(codesOf(input)).toStrictEqual([]);
  });

  it("marks the encoding slot, not the measure, so the Inspector points at the cause", () => {
    const parsed = parseComposition({ measure: "age", limit: 5, encoding: "ranked_bars" });

    expect(parsed.ok).toBe(false);
    if (parsed.ok) return;
    expect(parsed.error.issues[0].path).toStrictEqual(["encoding"]);
  });

  it("reports every broken rule, not just the first", () => {
    expect(codesOf({ measure: "lifespan", cohort: "open", encoding: "diverging_bars" })).toStrictEqual([
      "cohort_not_allowed",
      "measure_needs_distribution_encoding",
      "encoding_needs_signed_measure",
    ]);
  });

  it("names the offending slot so the Inspector can mark it", () => {
    const parsed = parseComposition({ measure: "lifespan", cohort: "open", encoding: "histogram" });

    expect(parsed.ok).toBe(false);
    if (parsed.ok) return;
    expect(parsed.error.issues[0].path).toStrictEqual(["cohort"]);
    expect(parsed.error.message).toContain("cohort");
  });
});

describe("requiresDenominator", () => {
  // Keyed by Grouping, so a grouping added to the vocabulary is a type error
  // here before it can slip through unasserted.
  const EXPECTED: Record<Grouping, boolean> = {
    company: false,
    week: false,
    role: true,
    specialization: true,
    skill: true,
    seniority: true,
    function: true,
  };

  it("covers exactly the grouping vocabulary", () => {
    expect(Object.keys(EXPECTED).sort()).toStrictEqual([...GROUPINGS].sort());
  });

  it.each(GROUPINGS.map((grouping) => [grouping] as [Grouping]))(
    "marks %s coverage debt per the table",
    (grouping) => {
      expect(requiresDenominator(grouping)).toBe(EXPECTED[grouping]);
    },
  );

  it("carries the debt to the composition and names which groupings owe it", () => {
    expect(
      validate({
        measure: "share",
        cohort: "open",
        groupBy: ["company", "seniority"],
        encoding: "stacked_bar",
      }),
    ).toStrictEqual({
      ok: true,
      requiresDenominator: true,
      denominatorGroupings: ["seniority"],
    });
  });

  it("marks no debt for a full-corpus grouping", () => {
    expect(
      validate({ measure: "count", cohort: "open", groupBy: ["company"], encoding: "ranked_bars" }),
    ).toStrictEqual({ ok: true, requiresDenominator: false, denominatorGroupings: [] });
  });

  // Filtering to `skill: "go"` buys the same ~35% slice grouping by `skill`
  // does, so it owes the same denominator.
  it("carries the debt a filter on a classified dimension incurs", () => {
    expect(
      validate({
        measure: "count",
        cohort: "open",
        groupBy: ["company"],
        filter: [{ dim: "skill", value: "go" }],
        encoding: "ranked_bars",
      }),
    ).toStrictEqual({ ok: true, requiresDenominator: true, denominatorGroupings: ["skill"] });
  });

  it("marks no debt for a filter on a full-corpus dimension", () => {
    expect(
      validate({
        measure: "count",
        cohort: "open",
        groupBy: ["company"],
        filter: [{ dim: "company", value: "Anthropic" }],
        encoding: "ranked_bars",
      }),
    ).toStrictEqual({ ok: true, requiresDenominator: false, denominatorGroupings: [] });
  });

  it("names a dimension once when it is both grouped and filtered", () => {
    expect(
      validate({
        measure: "count",
        cohort: "open",
        groupBy: ["skill"],
        filter: [{ dim: "skill", value: "go" }],
        encoding: "ranked_bars",
      }),
    ).toStrictEqual({ ok: true, requiresDenominator: true, denominatorGroupings: ["skill"] });
  });

  it("names the debt in declared vocabulary order", () => {
    expect(
      validate({
        measure: "count",
        cohort: "open",
        groupBy: ["seniority", "role"],
        filter: [{ dim: "function", value: "engineering" }],
        encoding: "ranked_bars",
      }),
    ).toStrictEqual({
      ok: true,
      requiresDenominator: true,
      denominatorGroupings: ["role", "seniority", "function"],
    });
  });
});

// `parseComposition` normalizes before the rules run. A typed caller reaching
// `validate` directly has no way to know it must, so `validate` normalizes too —
// otherwise the same composition gets opposite verdicts depending on which door
// it came through.
describe("validate agrees with the parse on the same value", () => {
  it("counts a repeated grouping once, exactly as the parse does", () => {
    const repeated: Composition = {
      measure: "count",
      cohort: "open",
      groupBy: ["company", "company"],
      encoding: "stacked_bar",
    };

    expect(validate(repeated).ok).toBe(false);
    expect(codesOf(repeated)).toStrictEqual(["encoding_needs_two_groupings"]);
  });

  it("does not hand the engine a duplicated denominator list", () => {
    expect(
      validate({
        measure: "count",
        cohort: "open",
        groupBy: ["role", "role", "company"],
        encoding: "ranked_bars",
      }),
    ).toStrictEqual({ ok: true, requiresDenominator: true, denominatorGroupings: ["role"] });
  });

  it("reads an unordered grouping the same way the link would", () => {
    expect(
      validate({
        measure: "count",
        cohort: "open",
        groupBy: ["week", "company"],
        encoding: "stacked_bar",
      }).ok,
    ).toBe(true);
  });

  // Every other public failure carries a GrammarError, so a caller narrating a
  // refusal reaches for one shape whatever produced it.
  it("refuses with the same GrammarError shape the parse returns", () => {
    const result = validate({ measure: "share", cohort: "open", encoding: "ranked_bars" });

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.issues.map((issue) => issue.code)).toStrictEqual(["measure_needs_grouping"]);
    expect(result.error.message).toBe(
      'groupBy: measure "share" normalizes within a grouping, so it needs at least one',
    );
  });
});

describe("canonical form", () => {
  it("reorders groupBy into the declared vocabulary order", () => {
    expect(
      valueOf({ measure: "count", groupBy: ["week", "company"], encoding: "line" }).groupBy,
    ).toStrictEqual(["company", "week"]);
  });

  it("drops repeated groupings so one set has one spelling", () => {
    expect(
      valueOf({ measure: "count", groupBy: ["week", "company", "company"], encoding: "line" })
        .groupBy,
    ).toStrictEqual(["company", "week"]);
  });

  it("omits an empty groupBy rather than carrying an empty array", () => {
    const parsed = valueOf({ measure: "count", groupBy: [], encoding: "table" });

    expect(parsed).toStrictEqual({ measure: "count", cohort: "open", encoding: "table" });
    expect("groupBy" in parsed).toBe(false);
  });

  it("sorts filters by dimension order then value", () => {
    expect(
      valueOf({
        measure: "count",
        filter: [
          { dim: "skill", value: "go" },
          { dim: "company", value: "Vercel" },
          { dim: "company", value: "Anthropic" },
        ],
        encoding: "table",
      }).filter,
    ).toStrictEqual([
      { dim: "company", value: "Anthropic" },
      { dim: "company", value: "Vercel" },
      { dim: "skill", value: "go" },
    ]);
  });

  it("trims filter values and drops the duplicates trimming creates", () => {
    expect(
      valueOf({
        measure: "count",
        filter: [
          { dim: "skill", value: " go " },
          { dim: "skill", value: "go" },
        ],
        encoding: "table",
      }).filter,
    ).toStrictEqual([{ dim: "skill", value: "go" }]);
  });

  it("rejects a filter value that is blank once trimmed", () => {
    expect(
      codesOf({ measure: "count", filter: [{ dim: "skill", value: "   " }], encoding: "table" }),
    ).toStrictEqual(["shape"]);
  });

  it("reads an explicit null as not set", () => {
    expect(
      valueOf({
        measure: "count",
        cohort: null,
        groupBy: null,
        filter: null,
        window: null,
        sort: null,
        limit: null,
        encoding: "table",
      }),
    ).toStrictEqual({ measure: "count", cohort: "open", encoding: "table" });
  });

  // The link hands every value back as a string, so a digit-string is a
  // legitimate spelling of a count — and the only one.
  it("reads a digit-string limit as the number the link meant", () => {
    expect(
      valueOf({ measure: "count", groupBy: ["company"], limit: "20", encoding: "table" }).limit,
    ).toBe(20);
  });

  it("reads a digit-string window as the number the link meant", () => {
    expect(
      valueOf({ measure: "delta", groupBy: ["company"], window: { weeks: "2" }, encoding: "diverging_bars" })
        .window,
    ).toStrictEqual({ weeks: 2 });
  });

  it.each([
    ["a fractional limit", { measure: "count", groupBy: ["company"], limit: 2.5, encoding: "table" }],
    ["a zero limit", { measure: "count", groupBy: ["company"], limit: 0, encoding: "table" }],
    [
      "a zero window",
      { measure: "delta", groupBy: ["company"], window: { weeks: 0 }, encoding: "diverging_bars" },
    ],
  ])("rejects %s", (_name, input) => {
    expect(codesOf(input)).toStrictEqual(["shape"]);
  });

  // `z.coerce.number()` would run `Number()` first and turn every one of these
  // into a fabricated count — `true` into a "top 1" the model never asked for.
  // A wrong type is a refusal the model can be corrected on.
  it.each([
    ["a boolean limit", { measure: "count", groupBy: ["company"], limit: true, encoding: "table" }],
    [
      "a single-element array limit",
      { measure: "count", groupBy: ["company"], limit: ["5"], encoding: "table" },
    ],
    ["a hex-string limit", { measure: "count", groupBy: ["company"], limit: "0x14", encoding: "table" }],
    [
      "a whitespace-padded limit",
      { measure: "count", groupBy: ["company"], limit: " 5 ", encoding: "table" },
    ],
    [
      "a boolean window",
      { measure: "delta", groupBy: ["company"], window: { weeks: true }, encoding: "diverging_bars" },
    ],
  ])("refuses %s instead of inventing a number for it", (_name, input) => {
    expect(codesOf(input)).toStrictEqual(["shape"]);
  });

  // Above 1e21 a number stringifies in exponential notation, which the link
  // reader's digit-only pattern cannot read back. The ceilings keep every value
  // the parse admits inside what the link can spell.
  it.each([
    [
      "a window past the ten-year ceiling",
      { measure: "delta", groupBy: ["company"], window: { weeks: 521 }, encoding: "diverging_bars" },
    ],
    [
      "a window large enough to serialize in exponential notation",
      { measure: "delta", groupBy: ["company"], window: { weeks: 1e21 }, encoding: "diverging_bars" },
    ],
    ["a limit past the ceiling", { measure: "count", groupBy: ["company"], limit: 1001, encoding: "table" }],
  ])("rejects %s", (_name, input) => {
    expect(codesOf(input)).toStrictEqual(["shape"]);
  });

  it("accepts the ceilings themselves", () => {
    expect(
      valueOf({ measure: "count", groupBy: ["company"], limit: 1000, encoding: "table" }).limit,
    ).toBe(1000);
    expect(
      valueOf({ measure: "delta", groupBy: ["company"], window: { weeks: 520 }, encoding: "diverging_bars" })
        .window,
    ).toStrictEqual({ weeks: 520 });
  });

  // The exported schema declares `additionalProperties: false`. The parse now
  // says the same thing, so a camelCase/snake_case slip is a refusal rather than
  // a valid, silently different question.
  it("refuses a key the vocabulary does not name", () => {
    expect(codesOf({ measure: "count", encoding: "table", location: "remote" })).toStrictEqual([
      "shape",
    ]);
  });

  it("refuses a snake_case spelling of a key it does name", () => {
    expect(codesOf({ measure: "count", group_by: ["company"], encoding: "table" })).toStrictEqual([
      "shape",
    ]);
  });

  // The export declares `additionalProperties: false` on the nested filter and
  // window objects too, so an unnamed key one level down is a refusal there as
  // well — not a key the parse silently strips while the schema rejects it.
  it("refuses a key nested inside a filter item that the vocabulary does not name", () => {
    expect(
      codesOf({
        measure: "count",
        filter: [{ dim: "skill", value: "go", bogus: 1 }],
        encoding: "table",
      }),
    ).toStrictEqual(["shape"]);
  });

  it("refuses a key nested inside the window that the vocabulary does not name", () => {
    expect(
      codesOf({
        measure: "delta",
        groupBy: ["company"],
        window: { weeks: 2, bogus: 1 },
        encoding: "diverging_bars",
      }),
    ).toStrictEqual(["shape"]);
  });

  // A repeated `f` appends ahead of the trailing `win`/`sort`/`n` block, so a
  // link that carries both pins the key order across that seam — reordering the
  // filter append relative to the scalars would fail here rather than ship green.
  it("emits filter, window, sort, and limit in canonical wire order", () => {
    expect(
      serializeComposition({
        measure: "delta",
        cohort: "open",
        groupBy: ["company"],
        filter: [{ dim: "skill", value: "go" }],
        window: { weeks: 2 },
        sort: "desc",
        limit: 20,
        encoding: "diverging_bars",
      }),
    ).toBe("v=1&m=delta&coh=open&by=company&f=skill%3Ago&win=2w&sort=desc&n=20&enc=diverging_bars");
  });

  it("serializes a non-canonical composition into the canonical link", () => {
    expect(
      serializeComposition({
        measure: "count",
        cohort: "open",
        groupBy: ["week", "company"],
        filter: [
          { dim: "skill", value: "go" },
          { dim: "company", value: "b" },
          { dim: "company", value: "a" },
        ],
        encoding: "line",
      }),
    ).toBe("v=1&m=count&coh=open&by=company%2Cweek&f=company%3Aa&f=company%3Ab&f=skill%3Ago&enc=line");
  });

  // The parse trims, so the link has to carry the trimmed spelling — otherwise
  // a composition and the composition its own link reads back as differ by
  // whitespace nobody asked about.
  it("trims filter values on the way out so the link reads back deep-equal", () => {
    const link = serializeComposition({
      measure: "count",
      cohort: "open",
      filter: [{ dim: "company", value: " Acme " }],
      encoding: "table",
    });

    expect(link).toBe("v=1&m=count&coh=open&f=company%3AAcme&enc=table");
    const back = deserializeComposition(link);
    expect(back.ok).toBe(true);
    if (!back.ok) return;
    expect(back.value.filter).toStrictEqual([{ dim: "company", value: "Acme" }]);
  });

  // Trimming ahead of the ordering pass is what lets dedup see the values the
  // parse will, so the link repeats `f` exactly as often as the parse would.
  it("drops the duplicate trimming creates rather than repeating f", () => {
    expect(
      serializeComposition({
        measure: "count",
        cohort: "open",
        filter: [
          { dim: "skill", value: " go " },
          { dim: "skill", value: "go" },
        ],
        encoding: "table",
      }),
    ).toBe("v=1&m=count&coh=open&f=skill%3Ago&enc=table");
  });

  // Serialization stays total: a value blank once trimmed still writes a link,
  // and that link still refuses on the way back in.
  it("writes a link for a blank filter value and refuses it on the read", () => {
    const link = serializeComposition({
      measure: "count",
      cohort: "open",
      filter: [{ dim: "company", value: "   " }],
      encoding: "table",
    });

    expect(link).toBe("v=1&m=count&coh=open&f=company%3A&enc=table");
    const back = deserializeComposition(link);
    expect(back.ok).toBe(false);
    if (back.ok) return;
    expect(back.error.issues.map((issue) => issue.code)).toStrictEqual(["shape"]);
    expect(back.error.issues[0].path).toStrictEqual(["filter", 0, "value"]);
  });

  it("round-trips a filter value carrying separators and percent signs", () => {
    const composition: Composition = {
      measure: "count",
      cohort: "open",
      filter: [{ dim: "company", value: "A & B, Inc: 100%" }],
      encoding: "table",
    };
    const back = deserializeComposition(serializeComposition(composition));

    expect(back.ok).toBe(true);
    if (!back.ok) return;
    expect(back.value).toStrictEqual(composition);
  });
});

describe("link version", () => {
  const valid = "v=1&m=count&coh=open&enc=table";

  it("stamps the version first in the link", () => {
    expect(serializeComposition(SEED_COMPOSITIONS.roleLifecycle).startsWith("v=1&")).toBe(true);
  });

  it.each([
    ["a missing version", "m=count&coh=open&enc=table"],
    ["an empty query", ""],
    ["a future version", `v=2&${valid.slice("v=1&".length)}`],
    ["a non-numeric version", `v=one&${valid.slice("v=1&".length)}`],
  ])("refuses %s rather than re-reading it", (_name, query) => {
    const result = deserializeComposition(query);

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.issues.map((issue) => issue.code)).toStrictEqual(["version_unsupported"]);
    // Empty, not `["v"]`. Every other issue paths a composition field, and the
    // version is not one — a consumer walking `path` should never have to ask
    // which namespace it landed in. `grammarError` renders empty as
    // "composition", which is what the version is about.
    expect(result.error.issues[0].path).toStrictEqual([]);
    expect(result.error.message).toContain("composition:");
  });

  it("accepts a query string that still carries its leading ?", () => {
    const result = deserializeComposition(`?${valid}`);

    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.value).toStrictEqual({ measure: "count", cohort: "open", encoding: "table" });
  });
});

describe("hand-edited links", () => {
  const base = "v=1&m=count&coh=open&enc=table";

  it("re-reads a window as a typed number of weeks", () => {
    const result = deserializeComposition(`v=1&m=delta&coh=open&win=2w&enc=diverging_bars`);

    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.value.window).toStrictEqual({ weeks: 2 });
  });

  it("treats an empty by= as no grouping", () => {
    const result = deserializeComposition(`${base}&by=`);

    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.value).toStrictEqual({ measure: "count", cohort: "open", encoding: "table" });
  });

  // The rules do not care which door the composition came through, so a link
  // the grammar would not have written refuses on the same codes.
  it.each([
    ["a delta link with no window", "v=1&m=delta&coh=open&by=company&enc=diverging_bars", "measure_needs_window"],
    ["a per-posting measure on a non-distribution encoding", "v=1&m=age&coh=open&enc=ranked_bars", "measure_needs_distribution_encoding"],
    [
      "a histogram over two groupings",
      "v=1&m=lifespan&coh=closed&by=company,role&enc=histogram",
      "encoding_needs_at_most_one_grouping",
    ],
  ])("refuses %s", (_name, query, code) => {
    const result = deserializeComposition(query);

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.issues.map((issue) => issue.code)).toStrictEqual([code]);
  });

  // The same mistake gets the same slot. A bad week count is a verdict about
  // `window.weeks` whether or not it happens to be spelled in digits — before,
  // only the all-digit spelling reached the count, and the rest surfaced as a
  // window that was not an object.
  it.each([
    ["a zero window", `${base}&win=0w`],
    ["a negative window", `${base}&win=-3w`],
    ["a fractional window", `${base}&win=2.5w`],
    ["a window past the ceiling", `${base}&win=521w`],
  ])("points %s at window.weeks", (_name, query) => {
    const result = deserializeComposition(query);

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.issues.map((issue) => issue.code)).toStrictEqual(["shape"]);
    expect(result.error.issues[0].path).toStrictEqual(["window", "weeks"]);
  });

  // Not a bad count — a window that is not a window. That one belongs to the
  // slot above it.
  it("points a window that is not a count at all at window", () => {
    const result = deserializeComposition(`${base}&win=abc`);

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.issues[0].path).toStrictEqual(["window"]);
  });

  it("keeps a colon inside a filter value", () => {
    const result = deserializeComposition(`${base}&f=company%3AAcme%3AInc`);

    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.value.filter).toStrictEqual([{ dim: "company", value: "Acme:Inc" }]);
  });

  it.each([
    ["a filter with no dimension separator", `${base}&f=broken`],
    ["an unparseable window", `${base}&win=abc`],
    ["a fractional window", `${base}&win=2.5w`],
    ["a non-numeric limit", `${base}&n=abc`],
    ["an unknown measure", "v=1&m=revenue&coh=open&enc=table"],
    ["an unknown grouping", `${base}&by=location`],
    ["an unknown encoding", "v=1&m=count&coh=open&enc=sankey"],
    ["a link missing measure and encoding", "v=1"],
  ])("names %s as a shape rejection instead of guessing", (_name, query) => {
    const result = deserializeComposition(query);

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.issues.every((issue) => issue.code === "shape")).toBe(true);
  });

  // Serialization is total; the gate is on the way back in. A link written from
  // an unvalidated value still faces the rules when it is read.
  it("refuses a crossing-invalid link even though serialize wrote it", () => {
    const link = serializeComposition({
      measure: "lifespan",
      cohort: "open",
      encoding: "histogram",
    });

    expect(link).toBe("v=1&m=lifespan&coh=open&enc=histogram");
    const result = deserializeComposition(link);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.issues.map((issue) => issue.code)).toStrictEqual(["cohort_not_allowed"]);
  });
});

// `URLSearchParams.get` takes the first occurrence, so first-wins would make a
// doubled key a quietly different query than the one the link spells — the
// outcome this module exists to prevent.
describe("repeated link keys", () => {
  const base = "v=1&m=count&coh=open&enc=table";

  const doubled: [string, string, readonly string[]][] = [
    ["a doubled measure", "v=1&m=count&m=delta&coh=open&enc=table", ["measure"]],
    ["a doubled cohort", "v=1&m=count&coh=open&coh=all&enc=table", ["cohort"]],
    ["a doubled grouping key", `${base}&by=company&by=week`, ["groupBy"]],
    [
      "a doubled window",
      "v=1&m=delta&coh=open&by=company&win=2w&win=8w&enc=diverging_bars",
      ["window"],
    ],
    ["a doubled sort", `${base}&by=company&sort=asc&sort=desc`, ["sort"]],
    ["a doubled limit", `${base}&by=company&n=5&n=20`, ["limit"]],
    ["a doubled encoding", "v=1&m=count&coh=open&enc=table&enc=line", ["encoding"]],
  ];

  it.each(doubled)("refuses %s at the field it fills", (_name, query, path) => {
    const result = deserializeComposition(query);

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.issues.map((issue) => issue.code)).toStrictEqual(["duplicate_link_key"]);
    expect(result.error.issues[0].path).toStrictEqual(path);
  });

  // First-wins here defeated the version gate outright: `v=1&v=2` read as
  // version 1 and walked through.
  it("refuses a doubled version instead of reading the first one", () => {
    const result = deserializeComposition("v=1&v=2&m=count&coh=open&enc=table");

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.issues.map((issue) => issue.code)).toStrictEqual(["duplicate_link_key"]);
    expect(result.error.issues[0].path).toStrictEqual([]);
  });

  // The rule is about the spelling, not the values: nothing in a URL says which
  // occurrence the reader took, so agreeing occurrences are refused too.
  it("refuses a repeat whose two values agree", () => {
    const result = deserializeComposition("v=1&v=1&m=count&coh=open&enc=table");

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.issues.map((issue) => issue.code)).toStrictEqual(["duplicate_link_key"]);
  });

  it("names every repeated key rather than stopping at the first", () => {
    const result = deserializeComposition("v=1&v=1&m=count&m=delta&coh=open&enc=table");

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.issues.map((issue) => issue.path)).toStrictEqual([[], ["measure"]]);
  });

  // `f` is the one repeatable key. Repetition is what the filter list means
  // there, so it reads as a list rather than a refusal.
  it("still reads a repeated f as the filter list", () => {
    const result = deserializeComposition(`${base}&f=company%3Aa&f=company%3Ab`);

    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.value.filter).toStrictEqual([
      { dim: "company", value: "a" },
      { dim: "company", value: "b" },
    ]);
  });
});

describe("never throws", () => {
  const circular: Record<string, unknown> = { measure: "count", encoding: "table" };
  circular.self = circular;

  const garbage: [string, unknown][] = [
    ["null", null],
    ["undefined", undefined],
    ["a number", 42],
    ["NaN", Number.NaN],
    ["a string", "measure=count"],
    ["an empty string", ""],
    ["a boolean", true],
    ["an empty object", {}],
    ["an array", [1, 2]],
    ["a function", () => 1],
    ["a symbol", Symbol("composition")],
    ["a Date", new Date()],
    ["a Map", new Map()],
    ["an Error", new Error("boom")],
    ["a null-prototype object", Object.create(null)],
  ];

  it.each(garbage)("parseComposition returns a Result for %s", (_name, input) => {
    expect(() => parseComposition(input)).not.toThrow();
    expect(parseComposition(input).ok).toBe(false);
  });

  // The refusal is about the unnamed `self` key, not the cycle: walking one
  // would be the way this throws, and it does not.
  it("parseComposition survives a self-referencing object", () => {
    expect(() => parseComposition(circular)).not.toThrow();
    expect(codesOf(circular)).toStrictEqual(["shape"]);
  });

  it.each([
    ["percent-decoding garbage", "%%%"],
    ["a bare fragment", "#composition"],
    ["a very long value", `v=1&m=count&coh=open&enc=table&f=company%3A${"x".repeat(5000)}`],
  ])("deserializeComposition returns a Result for %s", (_name, query) => {
    expect(() => deserializeComposition(query)).not.toThrow();
    expect(typeof deserializeComposition(query).ok).toBe("boolean");
  });
});
