# Composition Grammar

> Brief — decisions and non-goals. No task breakdown, no acceptance criteria.
> Rationale: [`research/composition-grammar.md`](../../../../research/composition-grammar.md). Roadmap: Compositional Analysis Surface → Grammar.

## Goal

The typed contract the agent composes analyses from — measure × grouping × filter × encoding, plus the fetch-run-aware cohort modifier. It validates a model-emitted composition, serializes it to a deep link and back, and hands the engine a typed value, never SQL. Keystone: `measure-engine`, `chat-transport`, `chart-primitives`, and `agent-readable-composition-state` all bind to this shape, so it ships first and stands alone.

## Target usage

```ts
// The only path from model to numbers: validate a model-emitted composition,
// then hand the typed value to the engine. The engine never sees SQL; the model
// never sees a number it didn't get back from the engine.
const parsed = parseComposition(toolArgs);      // Result<Composition, GrammarError>
if (!parsed.ok) return parsed.error;            // structurally invalid → refuse, don't compute
const { rows, denominator } = await runComposition(parsed.value);  // measure-engine, its own spec

// Deep link is the composition serialized — chat answer and page are one query.
const href = paths.view(composition);           // Route: "/view?v=1&m=delta&by=company&win=10w"
parseComposition(fromSearchParams(href));       // deep-equals composition; the page can't disagree

// Out-of-scope is a compile error, not a runtime refusal. The vocabulary has no
// compensation measure and no geography grouping, so this never type-checks:
//   { measure: "open_count", groupBy: ["location"] }   // ✗ "location" is not a Grouping

// A classified grouping carries its coverage debt in the contract. The engine
// returns the denominator; the chat narrates it. Full-corpus groupings don't.
requiresDenominator("skill");    // true  — 35% classified slice
requiresDenominator("company");  // false — full corpus
```

## Decisions

- Composition is data, never SQL — the only route to numbers is the engine consuming a *validated* value. This is the line the whole surface rests on.
- Vocabulary is closed enums; unreliable dimensions (compensation, geography) are absent from the types, so out-of-scope fails at compile, not at runtime.
- Validation via zod — one source for the runtime parse *and* the tool's JSON schema (`zod-to-json-schema`), tsc-checked. Hand-written guards would duplicate the shape and drift from the tool schema.
- Grouping over a classified dimension is marked `requiresDenominator` in the contract — the grammar flags coverage debt, the engine fills it. Coverage can't be forgotten because it's structural.
- Time grouping is weekly only — no daily bucket. 15 fetch days can't honestly support finer grain; the vocabulary refuses to imply it.
- Serialization is query-param, owned here, versioned (`v=`). `paths.view(c)` is the only `/view` href builder, extending `lib/paths.ts`. A saved link is a saved call.
- Module is pure — no `postgres` import, no React. Unit-tested under `pnpm test`, never `test:db`. A contract that needs a database to validate isn't a contract.
- New dir `apps/web/lib/composition/`. Not `lib/db/` — it holds no SQL.

## Not doing

- Computing aggregates — `measure-engine`. Tempting to wire one query to "prove" the vocabulary; that couples the contract to the engine and both rot together.
- Building `/view`, any chart, or the tool binding — frame, chart, and transport specs. A rendered composition is satisfying and it hides whether the contract stands headless. It must.
- A general query builder / arbitrary-column grouping — reintroduces text-to-SQL and unverifiable slices behind a friendlier name. The vocabulary stays closed.
- Scoring interestingness — novel × sound is `composition-quality-evals`. The grammar enforces only *structural* validity: a crossing that can't compute is rejected; a dull-but-valid one is not.
- Registering the tool with LangChain / OpenRouter — the grammar *exports* the schema; `chat-transport` binds it.
- Wiring the Inspector to real rows or a chart — it shows the composition, its deep link, and its verdict, nothing computed. A live readout is `measure-engine`'s demo, not this one.

## Build order

1. Vocabulary enums + `Composition` type. No deps.
2. Composability table + `validate` — reject impossible crossings (e.g. `lifespan` needs the closed cohort), mark `requiresDenominator`.
3. Runtime parse (`parseComposition`) over the schema, plus the JSON schema the tool will later consume.
4. `serialize`/`deserialize` to `URLSearchParams` with a `v=` version; add `paths.view(c)`.
5. Encode the seven catalog views as example compositions — proves the grammar spans them, seeds `seed-compositions`.
6. Unit tests: round-trip the seven, reject invalid crossings, assert denominator flags.
7. Composition Inspector story — pick a composition; show its deep link, valid/invalid verdict with reason, and denominator flag. List the seven seeds with their links. No chart, no query.

## Done when

- `pnpm typecheck` and `pnpm test` pass; `{ measure: "open_count", groupBy: ["location"] }` is a type error you can see.
- Round-trip holds for all seven seed compositions: `parse(serialize(c))` deep-equals `c`.
- `grep -r postgres apps/web/lib/composition` is empty — the contract is pure.
- In Storybook (`pnpm storybook`), the Inspector renders the seven seeds with their `/view?...` links; edit a field to an invalid crossing and the rejection and its reason show inline.
