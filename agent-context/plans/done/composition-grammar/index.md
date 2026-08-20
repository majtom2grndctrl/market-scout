# Composition Grammar

> Brief — decisions and non-goals. No task breakdown, no acceptance criteria.
> Rationale: [`research/composition-grammar.md`](../../../../research/composition-grammar.md). Roadmap: Compositional Analysis Surface → Grammar.

## Goal

The typed contract the agent composes analyses from — one measure over a grouping and filter, rendered by an encoding, with cohort/sort/limit modifiers. It validates a model-emitted composition, serializes it to a deep link and back, and hands the engine a typed value, never SQL. Keystone: `measure-engine`, `chat-transport`, `chart-primitives`, and `agent-readable-composition-state` all bind to this shape, so it ships first and stands alone.

## Target usage

```ts
// A composition is ONE measure, one grouping, one encoding — plus modifiers.
// "Top 20 movers over two weeks" is data, and all of it serializes into the link.
const movers: Composition = {
  measure: "delta", cohort: "open", groupBy: ["company"],
  window: { weeks: 2 }, sort: "desc", limit: 20, encoding: "diverging_bars",
};

// The only path from model to numbers: validate, then hand the typed value to
// the engine. The engine never sees SQL; the model never sees a number it
// didn't get back from the engine.
const parsed = parseComposition(toolArgs);        // Result<Composition, GrammarError>
if (!parsed.ok) return parsed.error;              // invalid crossing → refuse, don't compute
const { rows, denominator } = await runComposition(parsed.value);  // measure-engine, its own spec

// Deep link is the composition serialized — chat answer and page are one query.
const link = serializeComposition(movers);        // "v=1&m=delta&coh=open&by=company&win=2w&sort=desc&n=20&enc=diverging_bars"
deserializeComposition(link);                      // deep-equals movers, in canonical form

// Two kinds of "no". Absent vocabulary is a compile error for us, and never
// reaches the model's schema:
//   { measure: "count", groupBy: ["location"] }   // ✗ "location" is not a Grouping
// An invalid *crossing* is a runtime parse rejection — lifespan needs the closed cohort:
//   parseComposition({ measure: "lifespan", cohort: "open", ... })   // → { ok: false, error }

// A classified grouping carries its coverage debt in the contract; the engine
// fills the denominator, the chat narrates it.
requiresDenominator("skill");    // true  — 35% classified slice
requiresDenominator("company");  // false — full corpus
```

## Decisions

- Composition is data, never SQL — the only route to numbers is the engine consuming a *validated* value. The line the whole surface rests on.
- One measure per composition — one `groupBy` set, one encoding, plus modifiers. Composites and co-occurrence are out (see Not doing), so `scorecard` and `set-intersection` are absent from the encoding enum: it can't promise a shape one measure can't feed.
- Ordering is part of the question — `sort` and `limit` live in the composition and serialize into the link. "Top 20 movers" is not a rendering detail.
- Cohort is the only absence modifier here: `open | closed | all`. Failed-run *gap* rendering is chart + engine (they hold per-bucket run status); the grammar just names the cohort.
- Vocabulary is closed enums. Compensation and geography are absent, so out-of-scope is a compile error for TS authors and never appears in the model's schema. Cross-field crossing rules (e.g. `lifespan` needs `closed`) are runtime parse rejections, not schema-level.
- Validation via zod — added as a direct dep (v3) with `zod-to-json-schema`; both are transitive-only today. One source for the runtime parse and the exported tool schema. The export targets the function-calling subset: refinements validate at parse time and don't leak as required structure.
- Grouping over a classified dimension is marked `requiresDenominator` (see Composability). The grammar flags coverage debt; the engine fills it. Structural, so it can't be forgotten.
- Time grouping is weekly only — no daily bucket. Only ~15 fetch days across the window (live, 2026-08-15) can't honestly support finer grain.
- Serialization is a canonical query string, owned here, versioned (`v=`). `parseComposition` normalizes first — defaults filled, `groupBy` in declared order, `filter` sorted by (dim, value), `limit` a number — so `serialize` emits one canonical form and the round-trip is deep-equal. `serializeComposition`/`deserializeComposition` return a string, not a typed `Route` — `/view` doesn't exist yet, so a typed `paths.view` waits for that spec.
- Module logic is pure — no `postgres` import. Unit-tested under `pnpm test`, never `test:db`. The Inspector *story* is React; the contract it drives is not.
- New dir `apps/web/lib/composition/`. Not `lib/db/` — it holds no SQL.
- Inspector is an editable slot builder seeded from the five presets, not a static gallery — load a seed, mutate a slot, watch it re-validate and re-serialize. Controls fall out of the closed enums. First prototype of the core compose interaction; still computes nothing.

## Composability

The validation core. `validate` rejects any crossing not sanctioned here and marks coverage debt.

| Measure | Cohort (allowed · default) | Natural encoding | Shape |
|---|---|---|---|
| `count` | {open, closed, all} · open | ranked_bars, line, table | count per group |
| `delta` | {open, closed, all} · open | diverging_bars | signed change over `window` |
| `age` | {open} fixed | histogram | per-posting distribution |
| `lifespan` | {closed} fixed | histogram | per-posting distribution |
| `rate` | {all} fixed | line | new postings per week |
| `share` | {open} · open | stacked_bar, ranked_bars | normalized; needs ≥1 grouping |

Cohort is a free modifier on the count measures (`count`, `delta`) — the crown-jewel open/closed/all choice. On the time measures it is intrinsic: `age` is the open cohort, `lifespan` the closed one; they *are* the split, so it is fixed, not chosen.

Encoding must match the result shape: `line`/`area` require `week` in `groupBy`; `histogram` renders a per-posting measure (`age`, `lifespan`) as a distribution, and with one grouping becomes small-multiples — one per group, so "lifespan by role" stays composable; `diverging_bars` requires a signed measure; `stacked_bar` requires exactly two groupings. Richer grouped-distribution forms (box, violin) are chart-primitives' call.

`requiresDenominator` by grouping: `company`, `week` → false; `role`, `specialization`, `skill`, `seniority`, `function` → true. Classified subset is ~35% of the corpus (live, 2026-08-15) — `seniority` too: non-null within a classification, but still a third of all postings.

## Not doing

- Computing aggregates — `measure-engine`. Wiring one query to "prove" the vocabulary couples the contract to the engine; both rot together.
- Composite and co-occurrence views — Company Profile's scorecard is several compositions in one layout (a later frame concern); Skill Overlap needs a co-occurrence primitive the vocabulary doesn't have. Neither is a v1 seed. Company Profile is a catalogued view, which makes forcing it in tempting; it bloats the keystone.
- Building `/view`, any chart, the tool binding, or a typed `paths.view` — frame, chart, transport, and route specs. A rendered composition hides whether the contract stands headless. It must.
- A general query builder / arbitrary-column grouping — reintroduces text-to-SQL and unverifiable slices behind a friendlier name.
- Scoring interestingness — novel × sound is `composition-quality-evals`. The grammar enforces only *structural* validity: a crossing that can't compute is rejected; a dull-but-valid one is not.
- Registering the tool with LangChain / OpenRouter — the grammar *exports* the schema; `chat-transport` binds it.
- Wiring the Inspector to real rows or a chart — it shows the composition, its link, and its verdict, nothing computed.

## Build order

1. Add `zod` (v3) and `zod-to-json-schema` as direct deps in `apps/web/package.json`.
2. Vocabulary enums + `Composition` type — measure, groupBy, filter, cohort, sort, limit, window, encoding.
3. Composability rules + `validate` — cohort set per measure, encoding-shape fit, invalid crossings rejected, `requiresDenominator` marked.
4. zod schema → `parseComposition`; export the function-calling-subset JSON schema.
5. `serializeComposition`/`deserializeComposition` — canonical query string, `v=` version. No typed `paths.view`.
6. Encode the five seed compositions: Demand Trend, Movers, Role Lifecycle, Seniority Mix, Demand by Function.
7. Unit tests: round-trip the five, reject invalid crossings (incl. cohort mismatch), assert `requiresDenominator` per grouping, and assert the exported schema *accepts* a crossing-invalid composition in emission shape (`lifespan`+`open`, modifiers present as `null`) that `parseComposition` *rejects* — proving refinements stay at parse time. Validate against the schema with a real JSON-Schema check, guarded so it can't pass by skipping a keyword.
8. Inspector story — editable slot builder seeded from a preset; live re-validate/re-serialize; controls from the enums; UI in the story/component, logic from `lib/composition`.

## Done when

- `pnpm typecheck` and `pnpm test` pass; `{ measure: "count", groupBy: ["location"] }` is a type error you can see.
- Round-trip holds for all five seeds: `deserializeComposition(serializeComposition(c))` deep-equals `c` in canonical form.
- `requiresDenominator` is asserted for every grouping. The exported JSON schema targets the function-calling subset (`openAi`), which marks every property required-and-nullable — so a crossing-invalid composition is validated against the schema in *emission shape*, with the unused modifiers present as `null`. In that shape the schema *accepts* the `lifespan`+`open` crossing that `parseComposition` *rejects* — proving the crossing rules live in parse-time refinements, not in schema structure. (The schema does reject a bare `{ measure, cohort, encoding }` object, but for missing required keys, not for the crossing — the same rejection a *valid* seed gets, which is why the test asserts on the emission shape.)
- `grep -r postgres apps/web/lib/composition` is empty — the contract is pure.
- In Storybook (`pnpm storybook`), the Inspector loads a seed; swapping a slot to an invalid crossing shows the rejection and its reason inline while the serialized link updates live.
