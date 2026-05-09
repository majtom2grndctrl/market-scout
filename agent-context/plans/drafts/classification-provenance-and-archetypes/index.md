# Classification Provenance and Archetypes

## Goal

Refactor the classification schema so postings can be classified multiple times over time (provenance), so a canonical role can blend multiple legacy archetypes (e.g. `design-engineer` → both `design` and `engineering`), and so emergent taxonomy is observable from `created_at`. Update the `/batch-enrich` agent contract and writeback flow to match. The point is to discover blended/emerging role shapes — the schema must not force them into rigid buckets, and we must be able to tell stale classifications from current ones, A/B prompt versions, and observe taxonomy drift.

## Scope

### In scope

- Destructive rewrite of migration `000001_initial_schema.up.sql` (and matching `.down.sql`).
- New tables: `legacy_archetypes`, `canonical_role_archetypes`, `classifications`, `seniorities` (lookup).
- Drop `canonical_roles.category` CHECK; remove the `category` column.
- Add `classification_id` FK to `job_posting_roles`, `job_posting_specializations`, `job_posting_skills` and re-key their PKs.
- Add `created_at` to `canonical_roles`, `specializations`, `skills`.
- Seed `legacy_archetypes`, `seniorities`, and a `general-application` canonical_role mapped to `non-role`.
- New sqlc queries to drive the new writeback flow; remove queries the new schema obsoletes.
- Update `.claude/skills/batch-enrich/SKILL.md` agent JSON contract and writeback steps.
- Boundary inventory for new identifiers crossing Go ↔ JSON ↔ SQL.

### Out of scope (non-goals)

- Embedding storage for the agent `summary` field (separate plan).
- Boilerplate detection (separate plan being drafted in parallel).
- Next.js / UI work.
- Migration of historical classification data — none exists.
- A storage column for `skills[].requirement` — keep the existing "no column to land in" note.
- A storage column for `summary` — still markdown-report-only.

## Acceptance criteria

- [ ] `make db-reset` (or equivalent: drop volume, `migrate up`) applies migration 1 cleanly on an empty database; no migration past `000001` references the dropped `category` column.
- [ ] `INSERT` into `canonical_roles` with no `category` succeeds; the column does not exist.
- [ ] A canonical_role can have N rows in `canonical_role_archetypes`; the `design-engineer` seed (if added later) would map to both `design` and `engineering` without schema change.
- [ ] Inserting a `job_posting_roles` row without a `classification_id` fails with NOT NULL violation.
- [ ] Two `classifications` rows for the same `job_posting_id` with different `classified_at` values both persist; `job_posting_roles` rows from each set coexist (different `classification_id`).
- [ ] Deleting a `classifications` row cascades to its `job_posting_roles`/`job_posting_specializations`/`job_posting_skills` rows; deleting a `job_postings` row cascades to its classifications and through to join rows.
- [ ] `seniority` (whether CHECK or FK) rejects values outside `intern | junior | mid | senior | staff | principal | lead | director | unknown`.
- [ ] The seeded `legacy_archetypes` table contains at minimum: `design`, `engineering`, `research`, `non-role`.
- [ ] The seeded `general-application` canonical_role exists and maps via `canonical_role_archetypes` to `non-role`.
- [ ] An "emergent canonical_roles" query (`WHERE created_at > <cutoff>`) returns roles inserted after the cutoff and excludes seeded ones.
- [ ] A "current classification per posting" query returns exactly one row per posting that has any classifications, and it is the one with the latest `classified_at`.
- [ ] After running `/batch-enrich 5` against postings with descriptions, each enriched posting has exactly one new `classifications` row; its `model` and `prompt_version` match the values the orchestrator declared; join rows reference that classification.
- [ ] Re-running `/batch-enrich 5 --force` on the same postings inserts a *second* `classifications` row per posting and a *second* set of join rows; prior rows remain.
- [ ] Taxonomy upserts on `slug` are idempotent: re-running enrichment with a slug that already exists does not create a duplicate `canonical_roles` / `specializations` / `skills` / `legacy_archetypes` row.
- [ ] A blended canonical_role emitted by an agent (e.g. one role mapped to two archetype slugs) results in two `canonical_role_archetypes` rows after writeback.

## Tasks

### Task 1: Rewrite migration 000001

Edit `internal/db/migrations/000001_initial_schema.up.sql` and `.down.sql` in place. Drop `canonical_roles.category` and its CHECK. Add `created_at timestamptz NOT NULL DEFAULT now()` to `canonical_roles`, `specializations`, `skills`. Create `legacy_archetypes`, `canonical_role_archetypes`, `seniorities` (or equivalent CHECK — see Open questions), `classifications`. Re-create the three join tables with `classification_id` FK and the new PK shape. The down migration drops everything in reverse FK order.

Schema columns:

**`legacy_archetypes`**
| Column | Type | Notes |
|---|---|---|
| `id` | `bigserial` PK | |
| `slug` | `text NOT NULL UNIQUE` | kebab-case |
| `name` | `text NOT NULL` | |

**`canonical_roles`** (revised)
| Column | Type | Notes |
|---|---|---|
| `id` | `bigserial` PK | |
| `slug` | `text NOT NULL UNIQUE` | |
| `name` | `text NOT NULL` | |
| `created_at` | `timestamptz NOT NULL DEFAULT now()` | replaces `is_emergent` semantics |

(`category` is gone.)

**`canonical_role_archetypes`**
| Column | Type | Notes |
|---|---|---|
| `canonical_role_id` | `bigint NOT NULL REFERENCES canonical_roles(id) ON DELETE CASCADE` | |
| `archetype_id` | `bigint NOT NULL REFERENCES legacy_archetypes(id) ON DELETE RESTRICT` | taxonomy drop is a manual choice |
| | `PRIMARY KEY (canonical_role_id, archetype_id)` | |

**`specializations`** and **`skills`**: existing columns + `created_at timestamptz NOT NULL DEFAULT now()`.

**`seniorities`** (lookup, parallels archetype approach)
| Column | Type | Notes |
|---|---|---|
| `id` | `bigserial` PK | |
| `slug` | `text NOT NULL UNIQUE` | one of: intern, junior, mid, senior, staff, principal, lead, director, unknown |

(Implementer may swap to a CHECK on `classifications.seniority` instead — see Open questions. AC names the constraint behavior, not the implementation.)

**`classifications`**
| Column | Type | Notes |
|---|---|---|
| `id` | `bigserial` PK | |
| `job_posting_id` | `bigint NOT NULL REFERENCES job_postings(id) ON DELETE CASCADE` | |
| `model` | `text NOT NULL` | e.g. `claude-haiku-4-5` |
| `prompt_version` | `text NOT NULL` | orchestrator-declared, e.g. `batch-enrich-v2` |
| `classified_at` | `timestamptz NOT NULL DEFAULT now()` | "current" = latest per posting |
| `seniority_id` | `bigint NOT NULL REFERENCES seniorities(id) ON DELETE RESTRICT` | or `seniority text NOT NULL CHECK (...)` if CHECK chosen |
| `notes` | `text` | nullable, agent freeform |

Index `(job_posting_id, classified_at DESC)` to back current-classification lookups.

**`job_posting_roles`** (revised; same pattern for `job_posting_specializations`, `job_posting_skills`)
| Column | Type | Notes |
|---|---|---|
| `classification_id` | `bigint NOT NULL REFERENCES classifications(id) ON DELETE CASCADE` | |
| `role_id` | `bigint NOT NULL REFERENCES canonical_roles(id) ON DELETE CASCADE` | |
| | `PRIMARY KEY (classification_id, role_id)` | |

The `job_posting_id` column is removed from these join tables — it's reachable via `classifications.job_posting_id`. Add an index on `classification_id` if the FK doesn't auto-cover lookup patterns the queries need.

### Task 2: Seed data

In the same migration (or a sibling `.sql` invoked from it — implementer's call): seed `legacy_archetypes` with `design`, `engineering`, `research`, `non-role`; seed `seniorities` with the nine slugs above; insert one `canonical_roles` row with slug `general-application`, name `General Application`; insert one `canonical_role_archetypes` row mapping `general-application` to `non-role`.

These rows must exist after `migrate up` on an empty DB; no separate seed step required for these specific rows. Company seeding stays in `internal/db/seeds/companies.sql`.

### Task 3: sqlc query updates

The current `internal/db/queries/fetcher.sql` has no enrichment queries — `/batch-enrich` runs raw psql today. This plan introduces sqlc queries for the writeback flow so the skill (or a future Go caller) has a typed contract. Add a new file `internal/db/queries/classifications.sql` with at minimum:

- `UpsertLegacyArchetype` (slug → id, ON CONFLICT DO NOTHING + SELECT)
- `UpsertCanonicalRole` (slug, name → id)
- `UpsertSpecialization` (slug, name → id)
- `UpsertSkill` (slug, name → id)
- `UpsertCanonicalRoleArchetype` (canonical_role_id, archetype_id → ON CONFLICT DO NOTHING)
- `InsertClassification` (job_posting_id, model, prompt_version, seniority slug or id, notes → id)
- `InsertJobPostingRole`, `InsertJobPostingSpecialization`, `InsertJobPostingSkill` (classification_id, taxonomy_id)
- `GetCurrentClassification` (latest `classified_at` per `job_posting_id`)
- `ListUnclassifiedPostingsWithDescription` (for `/batch-enrich` selection — optional, see Open questions)

Run `sqlc generate` after; do not hand-edit `internal/db/models.go`, `db.go`, or generated `*.sql.go` files. Confirm the integration tests under `internal/db/*_integration_test.go` still compile against regenerated models — `JobPostingRole.JobPostingID` is gone, replaced by `ClassificationID`.

### Task 4: Update `/batch-enrich` agent contract and writeback

Edit `.claude/skills/batch-enrich/SKILL.md`. Changes:

**Output schema (replaces step 6 schema):**

```json
{
  "posting_id": <int>,
  "classification": {
    "model": "<model id>",
    "prompt_version": "<orchestrator-declared>",
    "seniority": "intern|junior|mid|senior|staff|principal|lead|director|unknown",
    "notes": "<optional freeform>"
  },
  "canonical_roles": [
    {
      "slug": "<existing or new>",
      "name": "<human-readable>",
      "legacy_archetypes": ["design", "engineering"]
    }
  ],
  "specializations": [{"slug": "...", "name": "..."}],
  "skills": [{"slug": "...", "name": "...", "requirement": "required|preferred"}],
  "summary": "<100–200 tokens>"
}
```

The orchestrator (not the agent) supplies `classification.model` and `classification.prompt_version` — the agent receives them as inputs and echoes them back, OR the orchestrator stamps them after the fact. Either is fine; pick one and document. `canonical_roles` is now an array — blended roles are first-class.

**Selection (step 3) update:** "no rows in `job_posting_roles`" no longer scopes to a posting — it scopes to a classification. The unenriched filter becomes "no rows in `classifications` for this `job_posting_id`". With `--force`, the filter drops as today.

**Existing-taxonomy load (step 4) update:** also `SELECT slug, name FROM legacy_archetypes` and pass to each agent. Remove `category` from the specializations select (it never existed — current SKILL.md is wrong).

**Writeback (step 7) replacement:** open one transaction per posting:
1. Upsert taxonomy (`canonical_roles`, `specializations`, `skills`, `legacy_archetypes`) by slug; collect ids.
2. Upsert `canonical_role_archetypes` mappings from each emitted canonical_role to its archetype slugs.
3. Insert one `classifications` row → get `classification_id`.
4. Insert join rows: `job_posting_roles` (classification_id, role_id) per emitted canonical_role; `job_posting_specializations`; `job_posting_skills`.

Drop the `--force` `DELETE FROM job_posting_*` step. With provenance, `--force` becomes "insert a new classification row" — older classifications stay as history. Note this change in the skill's principles section.

Drop fields with no column: `skills[].requirement`, `summary` — same as today.

## Sequencing

**Phase 1 (sequential):** Task 1 — migration shape blocks everything.
**Phase 2 (sequential):** Task 2 — seeds depend on tables existing.
**Phase 3 (concurrent):** Task 3, Task 4 — sqlc queries and skill contract are independent edits against the same agreed schema.

## Boundary inventory

| Concept | Go field (sqlc) | JSON key (agent) | SQL column |
|---|---|---|---|
| Posting id | `JobPostingID` | `posting_id` | `job_postings.id` / `classifications.job_posting_id` |
| Classification id | `ClassificationID` | (n/a; orchestrator-internal) | `classifications.id` / join `classification_id` |
| Model | `Model` | `classification.model` | `classifications.model` |
| Prompt version | `PromptVersion` | `classification.prompt_version` | `classifications.prompt_version` |
| Classified at | `ClassifiedAt` | (server-set) | `classifications.classified_at` |
| Seniority | `SeniorityID` (or `Seniority string` if CHECK chosen) | `classification.seniority` | `classifications.seniority_id` (or `seniority`) |
| Classification notes | `Notes` (`sql.NullString`) | `classification.notes` | `classifications.notes` |
| Canonical role slug | `Slug` | `canonical_roles[].slug` | `canonical_roles.slug` |
| Legacy archetype slug | `Slug` | `canonical_roles[].legacy_archetypes[]` | `legacy_archetypes.slug` |
| Role created_at | `CreatedAt` | (n/a; query-derived "emergent") | `canonical_roles.created_at` |

All slugs are kebab-case ASCII at every boundary. Agent JSON keys are snake_case. Go field names are PascalCase as sqlc emits them. SQL columns are snake_case.

## Open questions

1. **Seniority: lookup table vs CHECK constraint.** Plan specs the lookup (`seniorities`) for consistency with `legacy_archetypes` and to allow adding values without a migration. CHECK is simpler. Implementer's call; AC names the behavior, not the mechanism.
2. **"Current classification" mechanism.** Three options: (a) ORDER BY in queries, (b) materialized view, (c) `current_classification_id` pointer column on `job_postings`. (a) is least clever; (c) is fastest but requires writers to update on every insert. Recommend (a) until query patterns demand otherwise. Implementer picks; document the choice in a code comment on `classifications`.
3. **Where prompt_version comes from.** Orchestrator-declared (constant in the skill) or agent-echoed (passed in, returned). Plan assumes orchestrator-declared — agents don't need to know it. Confirm before implementation.
4. **`ListUnclassifiedPostingsWithDescription` sqlc query.** `/batch-enrich` runs raw psql today and may continue to. Adding a typed query is nice-to-have, not load-bearing for the schema refactor. Drop if it bloats Task 3.
5. **`canonical_role_archetypes.archetype_id` ON DELETE behavior.** Plan specs `RESTRICT` (don't silently lose mappings if someone drops an archetype). `CASCADE` is the alternative. Confirm.
