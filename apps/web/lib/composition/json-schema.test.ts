import { describe, expect, it } from "vitest";

import { COMPOSITION_JSON_SCHEMA, ROLE_LIFECYCLE, parseComposition } from "./index";

// The keystone the spec names: the exported tool schema must stay looser than
// `parseComposition`. If a crossing rule ever hardened into schema structure,
// the model would lose the shape it needs to be corrected on, and the refusal
// would move from a narratable answer to a silent impossibility.
//
// No JSON-Schema validator is importable from `apps/web`: `@cfworker/json-schema`
// is in the lockfile only as a transitive dependency of `shadcn` (via
// `@modelcontextprotocol/sdk`), and pnpm's strict isolation does not link a
// package `apps/web` never declared. So this file carries a validator for
// exactly the keyword set the export uses. `supported keyword set` below fails
// the moment the export grows a keyword this validator would silently ignore, so
// "the schema accepts it" cannot quietly degrade into "the validator skipped it".

const SUPPORTED_KEYWORDS = new Set([
  "type",
  "enum",
  "anyOf",
  "properties",
  "required",
  "additionalProperties",
  "items",
  "minLength",
  "minimum",
  "maximum",
]);

// Keywords that describe rather than constrain.
const ANNOTATION_KEYWORDS = new Set(["description", "title", "$schema"]);

// Keywords that could encode a crossing rule as structure. None may appear.
const CROSSING_CAPABLE_KEYWORDS = [
  "oneOf",
  "allOf",
  "not",
  "if",
  "then",
  "else",
  "const",
  "dependentSchemas",
  "dependentRequired",
  "dependencies",
  "propertyNames",
  "patternProperties",
];

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function keywordsIn(schema: unknown, found: Set<string> = new Set()): Set<string> {
  if (!isPlainObject(schema)) return found;

  for (const [keyword, value] of Object.entries(schema)) {
    found.add(keyword);
    if (keyword === "properties" && isPlainObject(value)) {
      for (const sub of Object.values(value)) keywordsIn(sub, found);
    } else if (keyword === "items" || keyword === "additionalProperties") {
      keywordsIn(value, found);
    } else if (keyword === "anyOf" && Array.isArray(value)) {
      for (const sub of value) keywordsIn(sub, found);
    }
  }

  return found;
}

function isType(type: string, value: unknown): boolean {
  switch (type) {
    case "object":
      return isPlainObject(value);
    case "array":
      return Array.isArray(value);
    case "string":
      return typeof value === "string";
    case "number":
      return typeof value === "number" && Number.isFinite(value);
    case "integer":
      return typeof value === "number" && Number.isInteger(value);
    case "boolean":
      return typeof value === "boolean";
    case "null":
      return value === null;
    default:
      throw new Error(`validator does not handle type "${type}"`);
  }
}

function errorsFor(schema: unknown, value: unknown, path: string): string[] {
  if (!isPlainObject(schema)) return [];
  const errors: string[] = [];

  if (Array.isArray(schema.anyOf)) {
    const matched = schema.anyOf.some((branch) => errorsFor(branch, value, path).length === 0);
    if (!matched) errors.push(`${path}: matches no anyOf branch`);
  }

  if (typeof schema.type === "string" && !isType(schema.type, value)) {
    return [...errors, `${path}: expected type ${schema.type}`];
  }

  if (Array.isArray(schema.enum) && !schema.enum.includes(value)) {
    errors.push(`${path}: ${JSON.stringify(value)} is not in the enum`);
  }

  if (typeof schema.minLength === "number" && typeof value === "string") {
    if (value.length < schema.minLength) errors.push(`${path}: shorter than ${schema.minLength}`);
  }

  if (typeof schema.minimum === "number" && typeof value === "number") {
    if (value < schema.minimum) errors.push(`${path}: below minimum ${schema.minimum}`);
  }

  if (typeof schema.maximum === "number" && typeof value === "number") {
    if (value > schema.maximum) errors.push(`${path}: above maximum ${schema.maximum}`);
  }

  if (isPlainObject(value)) {
    const properties = isPlainObject(schema.properties) ? schema.properties : {};

    if (Array.isArray(schema.required)) {
      for (const key of schema.required) {
        if (typeof key === "string" && !(key in value)) {
          errors.push(`${path}: missing required property "${key}"`);
        }
      }
    }

    if (schema.additionalProperties === false) {
      for (const key of Object.keys(value)) {
        if (!(key in properties)) errors.push(`${path}: unexpected property "${key}"`);
      }
    }

    for (const [key, sub] of Object.entries(properties)) {
      if (key in value) errors.push(...errorsFor(sub, value[key], `${path}.${key}`));
    }
  }

  if (Array.isArray(value) && schema.items !== undefined) {
    value.forEach((item, index) => {
      errors.push(...errorsFor(schema.items, item, `${path}[${index}]`));
    });
  }

  return errors;
}

// The model emits JSON, so validate what survives a JSON round-trip.
function schemaErrors(value: unknown): string[] {
  return errorsFor(COMPOSITION_JSON_SCHEMA, JSON.parse(JSON.stringify(value)), "$");
}

// `target: "openAi"` reports every optional field as nullable-and-required, so
// a model emission carries all eight keys with `null` for the modifiers it does
// not use.
function emission(fields: Record<string, unknown>): Record<string, unknown> {
  return {
    cohort: null,
    groupBy: null,
    filter: null,
    window: null,
    sort: null,
    limit: null,
    ...fields,
  };
}

// Every named slot in the export, wherever it nests, paired with whether it
// carries a description. The walk follows `anyOf` and `items` because a
// nullable field's object shape lives one branch down.
function propertyDescriptions(
  schema: unknown,
  path = "$",
  found: [string, boolean][] = [],
): [string, boolean][] {
  if (!isPlainObject(schema)) return found;

  if (isPlainObject(schema.properties)) {
    for (const [key, sub] of Object.entries(schema.properties)) {
      const described =
        isPlainObject(sub) && typeof sub.description === "string" && sub.description.length > 0;
      found.push([`${path}.${key}`, described]);
      propertyDescriptions(sub, `${path}.${key}`, found);
    }
  }

  if (Array.isArray(schema.anyOf)) {
    for (const branch of schema.anyOf) propertyDescriptions(branch, path, found);
  }

  if (schema.items !== undefined) propertyDescriptions(schema.items, `${path}[]`, found);

  return found;
}

describe("the validator itself", () => {
  it("only skips keywords that describe rather than constrain", () => {
    const unhandled = [...keywordsIn(COMPOSITION_JSON_SCHEMA)].filter(
      (keyword) => !SUPPORTED_KEYWORDS.has(keyword) && !ANNOTATION_KEYWORDS.has(keyword),
    );

    expect(unhandled).toStrictEqual([]);
  });

  it.each([
    ["a measure outside the vocabulary", emission({ measure: "revenue", encoding: "table" })],
    [
      "a grouping outside the vocabulary",
      emission({ measure: "count", groupBy: ["location"], encoding: "table" }),
    ],
    ["an encoding outside the vocabulary", emission({ measure: "count", encoding: "sankey" })],
    ["a missing required key", { measure: "count", encoding: "table" }],
    [
      "a key the vocabulary does not name",
      emission({ measure: "count", encoding: "table", region: "emea" }),
    ],
    [
      "a key nested inside a filter item",
      emission({
        measure: "count",
        filter: [{ dim: "skill", value: "go", bogus: 1 }],
        encoding: "table",
      }),
    ],
    [
      "a key nested inside the window",
      emission({ measure: "delta", window: { weeks: 2, bogus: 1 }, encoding: "diverging_bars" }),
    ],
    ["a limit below the minimum", emission({ measure: "count", limit: 0, encoding: "table" })],
    ["a fractional limit", emission({ measure: "count", limit: 2.5, encoding: "table" })],
    [
      "a blank filter value",
      emission({ measure: "count", filter: [{ dim: "skill", value: "" }], encoding: "table" }),
    ],
    [
      "a filter missing its dimension",
      emission({ measure: "count", filter: [{ value: "go" }], encoding: "table" }),
    ],
    ["a limit of the wrong type", emission({ measure: "count", limit: "20", encoding: "table" })],
    ["a limit above the maximum", emission({ measure: "count", limit: 1001, encoding: "table" })],
    [
      "a window above the maximum",
      emission({ measure: "delta", window: { weeks: 521 }, encoding: "diverging_bars" }),
    ],
    ["a non-object", "composition"],
  ])("rejects %s, so an acceptance below means something", (_name, value) => {
    expect(schemaErrors(value).length).toBeGreaterThan(0);
  });

  it("accepts a well-formed emission", () => {
    expect(
      schemaErrors(
        emission({
          measure: "delta",
          cohort: "open",
          groupBy: ["company"],
          filter: [{ dim: "skill", value: "go" }],
          window: { weeks: 2 },
          sort: "desc",
          limit: 20,
          encoding: "diverging_bars",
        }),
      ),
    ).toStrictEqual([]);
  });
});

describe("exported schema stays looser than the parse", () => {
  // The assertion the spec's Done-when names.
  it("accepts lifespan + open, which parseComposition rejects", () => {
    const crossingInvalid = emission({
      measure: "lifespan",
      cohort: "open",
      encoding: "histogram",
    });

    expect(schemaErrors(crossingInvalid)).toStrictEqual([]);

    const parsed = parseComposition(crossingInvalid);
    expect(parsed.ok).toBe(false);
    if (parsed.ok) return;
    expect(parsed.error.issues.map((issue) => issue.code)).toStrictEqual(["cohort_not_allowed"]);
  });

  it.each([
    [
      "age + closed",
      emission({ measure: "age", cohort: "closed", encoding: "histogram" }),
      "cohort_not_allowed",
    ],
    [
      "rate + open",
      emission({ measure: "rate", cohort: "open", groupBy: ["week"], encoding: "line" }),
      "cohort_not_allowed",
    ],
    [
      "line without week",
      emission({ measure: "count", groupBy: ["company"], encoding: "line" }),
      "encoding_needs_week",
    ],
    [
      "histogram over count",
      emission({ measure: "count", encoding: "histogram" }),
      "encoding_needs_per_posting_measure",
    ],
    [
      "diverging_bars over count",
      emission({ measure: "count", encoding: "diverging_bars" }),
      "encoding_needs_signed_measure",
    ],
    [
      "stacked_bar with one grouping",
      emission({ measure: "count", groupBy: ["company"], encoding: "stacked_bar" }),
      "encoding_needs_two_groupings",
    ],
    [
      "share with no grouping",
      emission({ measure: "share", encoding: "ranked_bars" }),
      "measure_needs_grouping",
    ],
    [
      "delta with no window",
      emission({ measure: "delta", groupBy: ["company"], encoding: "diverging_bars" }),
      "measure_needs_window",
    ],
    [
      "histogram over two groupings",
      emission({ measure: "lifespan", groupBy: ["company", "role"], encoding: "histogram" }),
      "encoding_needs_at_most_one_grouping",
    ],
    [
      "a per-posting measure on a non-distribution encoding",
      emission({ measure: "age", encoding: "ranked_bars" }),
      "measure_needs_distribution_encoding",
    ],
  ])("accepts %s and leaves the refusal to parse time", (_name, value, code) => {
    expect(schemaErrors(value)).toStrictEqual([]);

    const parsed = parseComposition(value);
    expect(parsed.ok).toBe(false);
    if (parsed.ok) return;
    expect(parsed.error.issues.map((issue) => issue.code)).toContain(code);
  });

  // The description is the whole tool contract the model reads: a slot without
  // one is a slot it has to guess at. `.nullable().describe().optional()` is the
  // only ordering that survives the export — zod-to-json-schema's openAi target
  // unwraps the outermost optional, taking any description attached above it
  // with it. The natural `.nullable().optional().describe()` a contributor
  // reaches for on a sixth modifier fails here instead of shipping a silently
  // degraded schema.
  it("carries a description on every slot the model has to fill", () => {
    const undescribed = propertyDescriptions(COMPOSITION_JSON_SCHEMA)
      .filter(([, described]) => !described)
      .map(([path]) => path);

    expect(undescribed).toStrictEqual([]);
  });

  it("describes every slot the vocabulary names, so the walk cannot pass vacuously", () => {
    expect(propertyDescriptions(COMPOSITION_JSON_SCHEMA).map(([path]) => path)).toStrictEqual([
      "$.measure",
      "$.cohort",
      "$.groupBy",
      "$.filter",
      "$.filter[].dim",
      "$.filter[].value",
      "$.window",
      "$.window.weeks",
      "$.sort",
      "$.limit",
      "$.encoding",
    ]);
  });

  it("carries no keyword capable of encoding a crossing", () => {
    const keywords = keywordsIn(COMPOSITION_JSON_SCHEMA);

    expect(CROSSING_CAPABLE_KEYWORDS.filter((keyword) => keywords.has(keyword))).toStrictEqual([]);
  });

  it("keeps measure and cohort independent enums", () => {
    const schema = COMPOSITION_JSON_SCHEMA as {
      properties: {
        measure: { enum: string[] };
        cohort: { anyOf: { enum?: string[] }[] };
      };
    };

    expect(schema.properties.measure.enum).toContain("lifespan");
    expect(schema.properties.cohort.anyOf.flatMap((branch) => branch.enum ?? [])).toStrictEqual([
      "open",
      "closed",
      "all",
    ]);
  });
});

describe("the exported schema is the emission shape, not the canonical shape", () => {
  // `target: "openAi"` marks every property required, so the canonical
  // `Composition` — which omits the modifiers it does not use — does not
  // satisfy the export. The rejection is about missing keys, never about the
  // crossing: a perfectly valid seed is rejected exactly the same way.
  it("rejects a canonical composition for its absent modifiers", () => {
    const errors = schemaErrors(ROLE_LIFECYCLE);

    expect(parseComposition(ROLE_LIFECYCLE).ok).toBe(true);
    expect(errors).toStrictEqual([
      '$: missing required property "groupBy"',
      '$: missing required property "filter"',
      '$: missing required property "window"',
      '$: missing required property "sort"',
      '$: missing required property "limit"',
    ]);
  });

  it("rejects a crossing-invalid canonical composition the same way", () => {
    expect(schemaErrors({ measure: "lifespan", cohort: "open", encoding: "histogram" })).toStrictEqual(
      [
        '$: missing required property "groupBy"',
        '$: missing required property "filter"',
        '$: missing required property "window"',
        '$: missing required property "sort"',
        '$: missing required property "limit"',
      ],
    );
  });
});
