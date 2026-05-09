# Classification Provenance and Archetypes

## Goal

Refactor the classification schema so postings can be classified multiple times over time (provenance), so a canonical role can blend multiple legacy archetypes (e.g. `design-engineer` → both `design` and `engineering`), and so emergent taxonomy is observable from `created_at`. Update the `/batch-enrich` agent contract and writeback flow to match. The point is to discover blended/emerging role shapes — the schema must not force them into rigid buckets, and we must be able to tell stale classifications from current ones, A/B prompt versions, and observe taxonomy drift.

## Scope

### In scope

- Destructive rewrite of migration `000001_initial_schema.up.sql` (and matching `.down.sql`).
- New tables: `legacy_archetypes`, `canonical_role_archetypes`, `classifications`.
- Drop `canonical_roles.category` CHECK; remove the `category` column.
- Add `classification_id` FK to `job_posting_roles`, `job_posting_specializations`, `job_posting_skills` and re-key their PKs.
- Add `created_at` to `canonical_roles`, `specializations`, `skills`.
- Seed `legacy_archetypes` and a `general-application` canonical_role mapped to `non-role`.
- New sqlc queries to drive the new writeback flow.
- Update `.claude/skills/batch-enrich/SKILL.md` agent JSON contract and writeback steps.
- Boundary inventory for new identifiers crossing Go ↔ JSON ↔ SQL.
- Drop the stale `category` column from the SKILL.md specializations select (current SKILL.md references a column that does not exist).

### Out of scope (non-goals)

- Embedding storage for the agent `summary` field (separate plan).
- Boilerplate detection (separate plan being drafted in parallel).
- Next.js / UI work.
- Migration of historical classification data — none exists.
- A storage column for `skills[].requirement` — keep the existing "no column to land in" note.
- A storage column for `summary` — still markdown-report-only.

## Acceptance criteria

- [ ] Dropping the docker volume and running `migrate up` applies migration 1 cleanly on an empty database; the rewritten `000001` contains no reference to `canonical_roles.category`.
- [ ] `INSERT` into `canonical_roles` with no `category` succeeds; the column does not exist.
- [ ] A canonical_role with two `canonical_role_archetypes` rows (one per archetype) is queryable as a blended role and is not rejected by any constraint.
- [ ] Inserting a `job_posting_roles` row with NULL `classification_id` fails (NOT NULL constraint).
- [ ] `job_posting_roles`, `job_posting_specializations`, `job_posting_skills` no longer have a `job_posting_id` column.
- [ ] Two `classifications` rows for the same `job_posting_id` with different `classified_at` values both persist; `job_posting_roles` rows from each set coexist (different `classification_id`).
- [ ] Deleting a `classifications` row cascades to its `job_posting_roles`/`job_posting_specializations`/`job_posting_skills` rows; deleting a `job_postings` row cascades to its classifications and through to join rows.
- [ ] Deleting a `canonical_roles` row that is referenced by any `job_posting_roles` row fails with a foreign-key violation; same for `specializations` ↔ `job_posting_specializations` and `skills` ↔ `job_posting_skills`.
- [ ] `classifications.seniority` rejects values outside `intern | junior | mid | senior | staff | principal | lead | director | unknown`.
- [ ] Agents emit `unknown` for indeterminate seniority; absent or null `seniority` in agent JSON is a parse error.
- [ ] The seeded `legacy_archetypes` table contains exactly: `design`, `engineering`, `research`, `non-role`.
- [ ] The seeded `general-application` canonical_role exists and maps via `canonical_role_archetypes` to `non-role`.
- [ ] Emergent-rows queries (`WHERE created_at > <cutoff>`) on `canonical_roles`, `specializations`, and `skills` each return rows inserted after the cutoff and exclude rows seeded by migration 1.
- [ ] A "current classification per posting" query returns exactly one row per posting that has any classifications, and it is the one with the latest `classified_at`.
- [ ] After running `/batch-enrich 5` against postings with descriptions, each enriched posting has exactly one new `classifications` row; its `model` and `prompt_version` match the values the orchestrator declared; join rows reference that classification.
- [ ] Re-running `/batch-enrich 5 --force` on the same postings inserts a *second* `classifications` row per posting and a *second* set of join rows; prior rows remain.
- [ ] Re-running `/batch-enrich 5` *without* `--force` against already-classified postings selects zero of them; no new `classifications` row is written.
- [ ] Taxonomy upserts on `slug` are idempotent: re-running enrichment with a slug that already exists does not create a duplicate `canonical_roles` / `specializations` / `skills` row.
- [ ] Re-running enrichment with the same `slug` but a different `name` does not modify the existing row's `name` or `created_at`.
- [ ] `legacy_archetypes` is a closed set: an agent emitting an archetype slug not present in the seeded table causes writeback to skip that posting and log a parse error; no row is upserted into `legacy_archetypes`.
- [ ] In a batch where one posting emits an unknown archetype slug, the other postings still produce `classifications` rows; only the offending posting is skipped.
- [ ] A blended canonical_role emitted by an agent (e.g. one role mapped to two archetype slugs) results in two `canonical_role_archetypes` rows after writeback — starting from a state where that canonical_role has no prior archetype mappings.
- [ ] Re-running enrichment with the same blended mapping does not error and does not create duplicate `canonical_role_archetypes` rows.

## Tasks

### Task 1: Rewrite migration 000001

Edit `internal/db/migrations/000001_initial_schema.up.sql` and `.down.sql` in place. Drop `canonical_roles.category` and its CHECK. Add `created_at timestamptz NOT NULL DEFAULT now()` to `canonical_roles`, `specializations`, `skills`. Create `legacy_archetypes`, `canonical_role_archetypes`, `classifications`. Re-create the three join tables with `classification_id` FK and the new PK shape. The down migration drops every object created by the rewritten up migration in reverse FK order; it does not attempt to restore the previous 000001 shape (this is a destructive rewrite, not a reversible migration pair). Remove every reference to `canonical_roles.category` and to `job_posting_id` on the three join tables from 000001 — including any indexes, constraints, or comments.

Schema columns:

**`legacy_archetypes`**
| Column | Type | Notes |
|---|---|---|
| `id` | `bigserial` PK | |
| `slug` | `text NOT NULL UNIQUE` | kebab-case |
| `name` | `text NOT NULL` | |

`legacy_archetypes` has no `created_at` — it is a closed, seed-only set; runtime never inserts.

**`canonical_roles`** (revised)
| Column | Type | Notes |
|---|---|---|
| `id` | `bigserial` PK | |
| `slug` | `text NOT NULL UNIQUE` | |
| `name` | `text NOT NULL` | |
| `created_at` | `timestamptz NOT NULL DEFAULT now()` | sole emergent-vs-seeded discriminant; query `created_at > <cutoff>` |

(`category` is gone.)

**`canonical_role_archetypes`**
| Column | Type | Notes |
|---|---|---|
| `canonical_role_id` | `bigint NOT NULL REFERENCES canonical_roles(id) ON DELETE CASCADE` | |
| `archetype_id` | `bigint NOT NULL REFERENCES legacy_archetypes(id) ON DELETE RESTRICT` | taxonomy drop is a manual choice |

Composite primary key: (canonical_role_id, archetype_id).

**`specializations`** and **`skills`**: existing columns + `created_at timestamptz NOT NULL DEFAULT now()`.

**`classifications`**
| Column | Type | Notes |
|---|---|---|
| `id` | `bigserial` PK | |
| `job_posting_id` | `bigint NOT NULL REFERENCES job_postings(id) ON DELETE CASCADE` | |
| `model` | `text NOT NULL` | e.g. `claude-haiku-4-5` |
| `prompt_version` | `text NOT NULL` | orchestrator-declared, e.g. `batch-enrich-v2` |
| `classified_at` | `timestamptz NOT NULL DEFAULT now()` | "current" = latest per posting |
| `seniority` | `text NOT NULL CHECK (seniority IN ('intern','junior','mid','senior','staff','principal','lead','director','unknown'))` | |
| `notes` | `text` | nullable, agent freeform |

Index `(job_posting_id, classified_at DESC)` to back current-classification lookups. Current classification is derived per query (`ORDER BY classified_at DESC LIMIT 1` per posting) — no pointer column on `job_postings`, no materialized view. Append-only writers don't mutate prior rows.

**`job_posting_roles`** (revised; same pattern for `job_posting_specializations`, `job_posting_skills`)
| Column | Type | Notes |
|---|---|---|
| `classification_id` | `bigint NOT NULL REFERENCES classifications(id) ON DELETE CASCADE` | |
| `role_id` | `bigint NOT NULL REFERENCES canonical_roles(id) ON DELETE RESTRICT` | RESTRICT preserves history; deleting a taxonomy row must confront its join rows |

Composite primary key: (classification_id, role_id).

Same RESTRICT applies to `specialization_id` on `job_posting_specializations` and `skill_id` on `job_posting_skills`. The `job_posting_id` column is removed from these join tables — it's reachable via `classifications.job_posting_id`.

**`job_posting_specializations`**: columns (classification_id, specialization_id) — same FK pattern; composite PK (classification_id, specialization_id).

**`job_posting_skills`**: columns (classification_id, skill_id) — same FK pattern; composite PK (classification_id, skill_id). skills[].requirement remains "no column to land in" per Out of scope.

### Task 2: Seed data

In the same `000001_initial_schema.up.sql` migration (inline `INSERT` statements): seed `legacy_archetypes` with `design`, `engineering`, `research`, `non-role`; insert one `canonical_roles` row with slug `general-application`, name `General Application`; insert one `canonical_role_archetypes` row mapping `general-application` to `non-role`.

The four archetypes mirror the dropped canonical_roles.category CHECK enum (design, engineering, research) plus non-role for general-application coverage.

These rows must exist after `migrate up` on an empty DB; no separate seed step required for these specific rows. Company seeding stays in `internal/db/seeds/companies.sql`.

Seed the mapping with `INSERT INTO canonical_role_archetypes SELECT cr.id, la.id FROM canonical_roles cr, legacy_archetypes la WHERE cr.slug='general-application' AND la.slug='non-role';` (or equivalent subselect — the join table holds ids, not slugs).

### Task 3: sqlc query updates

The current `internal/db/queries/fetcher.sql` has no enrichment queries — `/batch-enrich` runs raw psql today. This plan introduces sqlc queries for the writeback flow so the skill (or a future Go caller) has a typed contract. Add a new file `internal/db/queries/classifications.sql` with at minimum (no `sqlc.yaml` update needed — the config already globs `internal/db/queries/`):

- `GetLegacyArchetypeBySlug` (slug → id; returns no row if slug is unknown — `legacy_archetypes` is a closed set, never upserted at runtime) (`:one`)
- `UpsertCanonicalRole` (slug, name → id, `ON CONFLICT (slug) DO NOTHING` then `SELECT id` — name is not refreshed on conflict) (`:one`)
- `UpsertSpecialization` (slug, name → id, same pattern) (`:one`)
- `UpsertSkill` (slug, name → id, same pattern) (`:one`)

Upserts use `ON CONFLICT (slug) DO NOTHING` then `SELECT id`; on conflict the existing row's `created_at` is preserved (load-bearing for the emergent-vs-seeded discriminant).

- `UpsertCanonicalRoleArchetype` (canonical_role_id, archetype_id → ON CONFLICT DO NOTHING) (`:exec`)
- `InsertClassification` (job_posting_id, model, prompt_version, seniority, notes → id) — `seniority` is a plain text param; the column CHECK enforces the enum. classified_at is server-set via DEFAULT now(); two --force runs are distinct invocations and will receive distinct timestamps. (`:one`)
- `InsertJobPostingRole`, `InsertJobPostingSpecialization`, `InsertJobPostingSkill` (classification_id, taxonomy_id) (`:exec`)
- `GetCurrentClassificationForPosting` (job_posting_id → latest classification row by `classified_at DESC`) (`:one`)
- `ListCurrentClassifications` (no args; returns one row per `job_posting_id` that has any classifications, each the latest by `classified_at DESC`) (`:many`)

Before regenerating, grep for `JobPostingRole` and any `*.JobPostingID` accesses on join-table structs. Today there are no such references outside `internal/db/models.go` itself, so no test edits are expected. Run `sqlc generate` after; do not hand-edit `internal/db/models.go`, `db.go`, or generated `*.sql.go` files. Run `go build ./...` after regeneration. The new `JobPostingRole` struct has `ClassificationID` and `RoleID` (the old `JobPostingID` field is gone). After `sqlc generate`, `CanonicalRole.Category` disappears from `internal/db/models.go`; grep confirms no caller uses it today, so removal is safe.

The skill executes writeback as raw psql via Bash — Claude is the orchestrator and shells out directly. The sqlc queries introduced here are the typed contract for future Go or TypeScript callers; they are not invoked at runtime by the skill today. The inline SQL in SKILL.md is the runtime source of truth; the sqlc file mirrors it for the typed contract. Keep them in sync until a Go caller takes over.

### Task 4: Update `/batch-enrich` agent contract and writeback

Edit `.claude/skills/batch-enrich/SKILL.md`. Changes:

**Output schema (replaces step 6 schema):**

```json
{
  "posting_id": <int>,
  "classification": {
    "seniority": "intern|junior|mid|senior|staff|principal|lead|director|unknown",
    "notes": "<optional freeform>"
  },
  "canonical_roles": [
    {
      "slug": "<existing or new>",
      "name": "<human-readable>",
      "legacy_archetypes": ["design", "engineering"]  // closed set — must be slugs from the seeded legacy_archetypes table
    }
  ],
  "specializations": [{"slug": "...", "name": "..."}],
  "skills": [{"slug": "...", "name": "...", "requirement": "required|preferred"}],
  "summary": "<100–200 tokens>"
}
```

The orchestrator stamps `classification.model` and `classification.prompt_version` after the agent returns — they are not part of the agent's emitted JSON. Both are declared in a fenced code block at the top of SKILL.md with language identifier `classification-pins` (i.e. ` ```classification-pins `); format: two lines, `PROMPT_VERSION=<slug>` and `MODEL=<model-id>` (e.g. `MODEL=claude-haiku-4-5`). Hand-bump both when the agent prompt or target model changes materially. Claude reads these from its own SKILL.md instructions and substitutes them into the writeback INSERT. Pins are read once at invocation start and applied uniformly to every posting in the batch.

Agent omits the `notes` key or emits null for absent notes; empty string is treated as null. Agent prompt must instruct: emit `seniority: "unknown"` when seniority cannot be determined; never omit the field. `canonical_roles` is now an array — blended roles are first-class.

**Selection (step 3) update:** "no rows in `job_posting_roles`" no longer scopes to a posting — it scopes to a classification. The unenriched filter becomes "no rows in `classifications` for this `job_posting_id`". Combined: latest snapshot has non-null `description_text` AND (when `--force` is false) `NOT EXISTS (SELECT 1 FROM classifications c WHERE c.job_posting_id = job_postings.id)`; optional ILIKE prefilter on focus (latest-snapshot resolution itself is unchanged from current SKILL.md step 3; only the unenriched filter changes). With `--force`, the filter drops as today. The unenriched filter checks for *any* prior classification, regardless of `prompt_version`. Re-running a different prompt version against already-classified postings requires `--force`. Selection SQL stays inline in SKILL.md (raw psql); no sqlc query is added for selection.

**Existing-taxonomy load (step 4) update:** also `SELECT slug, name FROM legacy_archetypes` and pass to each agent. Render the archetype list as a bullet list under a `## Legacy archetypes (closed set)` heading in the agent prompt template, immediately after the existing canonical-roles taxonomy section — same mechanism as the existing taxonomy load. Agent prompt must state: pick archetype slugs only from this list; `legacy_archetypes` is a closed set, agents may not invent new ones. Also change the existing `SELECT slug, name, category FROM specializations` line to `SELECT slug, name FROM specializations` — the `category` column never existed on `specializations` and the current select would error against the live DB.

**Writeback (step 7) replacement:** Writeback runs as raw psql INSERTs/SELECTs in a psql transaction block (BEGIN ... COMMIT). Query names below name the sqlc contract; the skill's Bash steps implement the same logic directly.

Open one transaction per posting: Each posting runs in a single `psql` invocation wrapping one BEGIN…COMMIT block; a failure for one posting cannot poison another's transaction.
0. Validate `classification.seniority` is present and non-null; if missing or null, skip this posting, log a parse error (posting_id + field name), and continue the batch.
1. Resolve each emitted `canonical_roles[].legacy_archetypes[]` slug via `GetLegacyArchetypeBySlug`. If any slug is unknown, abort the transaction for this posting and log a parse error — `legacy_archetypes` is a closed set; agents do not extend it. Retain resolved `(canonical_role slug → [archetype_id])` mapping within the transaction (e.g. via shell variables or CTEs in the same psql session) for step 4.
2. Upsert taxonomy (`canonical_roles`, `specializations`, `skills`) by slug; collect ids. Agent-emitted `name` is used only on initial insert; on conflict, existing row's `name` and `created_at` are unchanged.
3. Upsert `canonical_role_archetypes` mappings from each emitted canonical_role to its (now-validated) archetype ids.
4. Insert one `classifications` row → get `classification_id`.
5. Insert join rows: `job_posting_roles` (classification_id, role_id) per emitted canonical_role; `job_posting_specializations`; `job_posting_skills`.

On transaction failure for a posting (including unknown-archetype-slug aborts), skip it and continue the batch; log `posting_id` and error to the markdown report. Postings skipped due to transaction failure have no `classifications` row and will be re-selected on the next non-`--force` run. Whole-posting abort (rather than per-role skip) is intentional — a hallucinated archetype usually signals a broader misclassification of the posting; revisit if abort rate climbs.

Drop the `--force` `DELETE FROM job_posting_*` block from step 7. With provenance, `--force` becomes "insert a new classification row" — older classifications stay as history. Note this change in the skill's principles section. Under provenance, `--force` re-classifies *every* selected candidate (filter drops entirely), not just previously-classified ones.

Drop fields with no column at writeback: `summary` (markdown report only) and `skills[].requirement` (no column to land in). The agent still emits `skills[].requirement` in JSON — the contract preserves it for future storage; writeback simply ignores it today. Out-of-enum `requirement` values are not a parse error today — writeback ignores the field entirely regardless of value.

## Sequencing

**Phase 1 (sequential):** Task 1 — migration shape blocks everything.
**Phase 2 (sequential):** Task 2 — seeds depend on tables existing.
**Phase 3 (concurrent):** Task 3, Task 4 — sqlc queries and skill contract are independent edits against the same agreed schema.

## Boundary inventory

| Concept | Go field (sqlc) | JSON key (agent) | SQL column |
|---|---|---|---|
| Posting id (classifications FK) | `Classification.JobPostingID` | `posting_id` | `classifications.job_posting_id` |
| Posting id (job_postings PK) | `JobPosting.ID` | `posting_id` | `job_postings.id` |
| Classification id | `JobPostingRole.ClassificationID` | (orchestrator-internal; never serialized to agent JSON) | `classifications.id` / join `classification_id` |
| Model | `Classification.Model` | `classification.model` | `classifications.model` |
| Prompt version | `Classification.PromptVersion` | `classification.prompt_version` | `classifications.prompt_version` |
| Classified at | `Classification.ClassifiedAt` | (server-set) | `classifications.classified_at` |
| Seniority | `Classification.Seniority` (`string`) | `classification.seniority` | `classifications.seniority` |
| Classification notes | `Classification.Notes` (`sql.NullString`) | `classification.notes` | `classifications.notes` |
| Skill requirement | (not stored) | `skills[].requirement` | (no column) |
| Summary | (not stored) | `summary` | (no column) |
| Canonical role slug | `CanonicalRole.Slug` | `canonical_roles[].slug` | `canonical_roles.slug` |
| Legacy archetype slug | `LegacyArchetype.Slug` | `canonical_roles[].legacy_archetypes[]` | `legacy_archetypes.slug` |
| Role created_at | `CanonicalRole.CreatedAt` | (n/a; query-derived "emergent") | `canonical_roles.created_at` |

The agent JSON `posting_id` key always refers to `job_postings.id`; the orchestrator uses the same value when populating `classifications.job_posting_id`. The overload is naming, not semantics.

All slugs are kebab-case ASCII at every boundary. Agent JSON keys are snake_case. Go field names are PascalCase as sqlc emits them. SQL columns are snake_case.

Archetype mappings on `canonical_role_archetypes` are additive across classifications: re-emitting a different archetype set for an existing canonical_role inserts new mapping rows (idempotent via PK conflict) and never deletes existing mappings.

## Resolved decisions

- **Seniority is a CHECK column on `classifications`, not a lookup table.** Vocabulary is stable (nine industry-standard slugs); only one table references it; no admin UI or per-tenant variation. CHECK saves a join and a subselect. Lookup tables earn their keep when vocabulary is fluid or shared across many tables — neither applies here.
- **Current classification is derived per query** via `ORDER BY classified_at DESC`, backed by the `(job_posting_id, classified_at DESC)` index. No pointer column on `job_postings`, no materialized view. A pointer column would force writers to mutate posting rows on every insert, contradicting the append-only architecture.
- **Taxonomy FKs on join tables are RESTRICT, not CASCADE.** Provenance is the goal; cascading deletes from `canonical_roles` (or specializations / skills) would shred the historical join rows the plan exists to preserve. RESTRICT forces any taxonomy cleanup to confront the history rather than silently destroy it.
- **`prompt_version` is a hand-bumped constant in SKILL.md.** Deliberate cadence matches the "compound observations over weeks" rhythm of the project.
- **`seniority` is a plain `string` in the Go sqlc layer, not a typed enum.** The DB CHECK is the enforcement boundary; the agent JSON schema is the second pin. Adding a Go `type Seniority string` + constants introduces a third pin with no branching logic to justify it. Plain string is idiomatic for a thin sqlc passthrough.
