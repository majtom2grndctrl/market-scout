# Enrichment Tool: Design Inputs

> **Read this when:** speccing the replacement enrichment tool, a new Go binary that classifies postings with a non-LLM model.
> **What it is:** constraints and findings gathered 2026-09-21/22 while cleaning up the database. Not a spec. Numbers are dated snapshots; re-measure before relying on one.
> **Related:** `agent-context/lib/project.md` (Settled architecture, Evidence trust tiers), `agent-context/lib/developer-guide.md` §6.2 (prompt-version pin register), `agent-context/plans/drafts/work-unit-dedup-edges/index.md` (parked; its findings are summarized below).

---

## State at handoff

- Database at migration 43. Every migration from 36 on was round-tripped by running its own `.down.sql`/`.up.sql` directly.
- LLM classification contract is `batch-enrich` v9, in both `.claude/` and `.agents/` skills. It is the fallback path, not the priority.
- `skills` and `specializations` deduplicated and name-repaired (000037). `canonical_roles` name collisions merged (000040).
- Corpus: 202 companies, 119 ever fetched successfully; about 83 get their first fetch in the next run. OpenAI, Stripe and Anthropic were about 43% of open postings.
- Classified history is mixed: several prompt versions, an oldest-first backlog drain until 2026-09-21, and 66 rows labeled `batch-enrich-v6` / `claude-sonnet-5` whose origin nobody can trace. Treat pre-v9 classifications as a different instrument, not as a baseline.

## Write path

**The tool writes through `mcp.save_enrichment`.** That boundary is where every protection lives:

| Enforced there | Why it matters |
|---|---|
| Mint-only `name_collision` gate on roles, specializations and skills | Stops duplicate-name terms. Key is case- and whitespace-insensitive; punctuation counts, so C, C++ and C# stay distinct. |
| `retired_slug` rejection | 71+ retired slugs, most naming their survivor as a redirect. |
| Required provenance (`model`, `prompt_version`) | A defaulted provenance value is a fabricated agent-asserted signal. |
| Similarity advisories persisted (000039) | The only measure of whether writers heed near-match warnings. |
| Transaction-scoped advisory lock | Serializes concurrent writers so cross-table taxonomy ownership stays consistent. |

- **The old runner, `cmd/batch-enrich`, bypasses all of it.** It upserts taxonomy and classification rows directly under owner credentials. Do not reuse its write-back. It stays unused until rewritten.
- **Calling the SQL function directly skips the Go-side validation in the MCP server.** The function's own checks are then the only guard. Two gaps remain for direct callers:
  - A non-numeric `posting_id` fails as a raw cast error.
  - A null `p_model` or `p_prompt_version` reaches a NOT NULL constraint unchecked.
  Close both, or call through MCP.
- Writes are append-only. A new classification supersedes, never updates. `latest_classifications` picks the newest per posting.

## Taxonomy interaction

- **LLM workers mint new terms; a fixed-label classifier picks from existing ones.** That is a real design choice. Minting keeps the taxonomy open; picking keeps it stable and makes scores comparable. Decide which, per dimension.
- **The dimension ratchet is unfixed.** `canonical_role_dimensions` only accumulates (`ON CONFLICT DO NOTHING`), so it is a union over every writer. `Software Engineer` carries 7 of 8 dimensions. Role merges take the union. The open decision is whether dimensions become per-classification or a curated, human-owned mapping. Don't build on the table as it stands.
- **Near-synonyms get past the name gate.** About 20 narrow "analytics" skills, minted Sept 11–21 with 1–4 links each, differ in name. Embeddings are the natural tool here.
- **Role near-duplicates by slug similarity** (for example `business-operations` / `business-operations-manager` before 000040 merged them) are not name collisions. Nothing merges them automatically.

## Provenance and lineage

- Each writer has its own prompt-version lineage, pinned in one place and never restated as a literal. See the §6.2 register. A non-LLM tool needs its own lineage. There, "prompt version" means model version plus configuration.
- **The schema has no scores.** `job_posting_roles`, `_skills` and `_specializations` are bare join tables. Classifiers and embedding similarity emit per-label scores, and thresholds, calibration and model comparison all need them stored.
- A run record, like `fetch_runs` is for the fetcher, would let every output link back to the model run that produced it.
- **Seniority source.** v9 tags each non-`unknown` seniority with the step that produced it, inside `classifications.notes` as `seniority[<tag>]: "<verbatim phrase>"`. A real source column was deferred on purpose. If this tool produces seniority, its output schema is where that column belongs, and the notes tags can migrate into it.

## Seniority semantics

- v9 abstains rather than guesses. `mid` and `junior` come only from a stated level word. Inference yields only `senior` or `director`, from closed phrase lists. Expect `unknown` on most postings with no level word: 34 of 40 in a stress sample.
- Two pilots shaped it. The prose version of "aligns with a named team" fired on 25 of 42 postings at about 52% precision. The closed list fired once, correctly, on a fresh sample. Haiku matched the Sonnet reference on 38 of 40, and both differences favoured the current rules.
- `open_posting_titles.title_seniority` is what the title states, derived deterministically in the read model. It counts `manager` as a rank; v9 does not. They disagree on Manager titles by design.

## What counts as the same job

From the parked dedup draft, re-measure before use:

- **Requisition keys by platform:** Greenhouse 5,800 of 5,820; Workday 623 of 623; Workable 5 of 61; Ashby, Lever and Gem 0. On Greenhouse the key names a job family, not a posting, so it merges only with a matching normalized title.
- **Whitespace-only twins.** Some postings differ by one byte, like `Compensation$160,000` vs `Compensation $160,000`. Hashing descriptions with all whitespace removed catches them. That hash is a natural embedding cache key: embed each distinct text once.
- **Regional suffixes are a trap.** Stripping `- Italy` or `(Munich)` would wrongly merge 471 of 498 candidate groups. Territory in the title marks a distinct posting.
- **Fan-out.** One requisition listed once per location inflates posting counts. Snap! Raise's 122-posting "Independent Sales Representative" clique is the extreme case. Report postings and requisitions both.
- A deterministic model gives identical output for identical input, so sharing one answer across a work unit buys nothing. That is why the draft is parked.

## Coverage

- Workday and Workable postings carry no description, so they cannot be classified from text. Workday is the bigger hole: 14 companies, 11 of them never fetched yet.
- Selection defaults to newest-first with a per-company cap of 5 per wave. A newly onboarded company's whole board is "first seen" on onboarding day, so for that cohort "recent" means "recently onboarded".

## Testing

- **Isolate the tool's integration tests from the start.** Go integration tests currently write to the development database through `DATABASE_URL`, and skip silently unless `.env.local` is sourced. Moving the existing suites is sized at 17 files (about 4,550 lines) and mostly mechanical:
  - One shared helper modelled on `apps/web/lib/db/test-dsn.ts`.
  - One new variable, `DATABASE_URL_TEST_ACTIONS`.
  - One test (`TestTaxonomySearch_LiveSlugSimilarPairLandsInOneCluster`) needs live skill slugs, so it needs a fixture.
- After any migration that touches a view, run `pnpm test:db` from `apps/web/`.
- Never use `migrate down` to test reversibility. It is a full teardown. Run the migration's own down and up files directly.

## Opportunities

- **Title fit.** Embed titles and measure each one's distance from its canonical role. That measures how far a title strays from its role, which is the core of the research question (traditional vs. repackaged titles), without an LLM.
- **Taxonomy hygiene.** Use embedding neighbours to find near-synonyms the name gate can't see.
- pgvector has been enabled since the first migration and is unused.
- The prototype at `apps/web/app/prototypes/title-vs-role-divergence/` measures modal title-head share per role. It still runs hand-rolled title SQL. Rebuild it on `open_posting_titles` once a fresh classified cohort exists.

## Non-goals of this note

- No schema design, model choice or task breakdown. Those belong in the spec.
- The LLM skills' internals, beyond the constraints above.
