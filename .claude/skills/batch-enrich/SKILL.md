---
name: batch-enrich
description: >
  Classifies unenriched job postings via parallel Haiku subagents. Each subagent
  reads one description and emits canonical roles (with role dimension mappings),
  specializations, skills, and a summary. Orchestrator dispatches in waves of 10,
  writes a classifications row per posting plus join rows keyed to that classification
  (provenance preserved across re-runs). Use to populate role/skill data during the
  enrichment design phase.
disable-model-invocation: true
allowed-tools: Read, Bash, Agent, Write
argument-hint: "<count> [focus description] [--force]"
---

```classification-pins
PROMPT_VERSION=batch-enrich-v2
MODEL=claude-haiku-4-5-20251001
```

# Batch Enrich

Enrich job postings into canonical roles, specializations, skills, and a structured summary. Coordinate — don't classify. Dispatch Haiku agents in parallel waves, gather JSON, write to join tables.

## Args

`$ARGUMENTS` — first token is record count (integer); the rest is freeform focus guidance.

| Arg | Example | Effect |
|---|---|---|
| count | `25` | LIMIT on selection |
| focus | `prioritize AI/ML engineering roles` | ILIKE prefilter on title + description; included in each agent's guidance |
| `--force` | `--force` | Re-classify postings that already have classifications. Inserts a new `classifications` row; does not delete history. May appear anywhere in args. |

If focus is empty: select oldest unenriched, no agent guidance beyond the schema.

## Process

### 1. Read context (skip if already loaded this session)

- `agent-context/lib/index.md`
- `agent-context/lib/project.md` §Settled architecture, §Database as AI agent knowledge store

### 2. Parse args

Strip `--force` from `$ARGUMENTS` first using exact token matching (whitespace-delimited — a token like `forceful` that merely contains `force` as a substring is unaffected); record whether it was present. Then:

- `count` — first remaining token; integer; default 10 if absent or non-numeric
- `focus` — remaining text, may be empty
- `force` — boolean from the strip step

### 3. Select postings

Postgres runs in docker compose (service name `db`). Run psql inside the container:

```bash
docker compose exec -T db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1
```

`POSTGRES_USER` and `POSTGRES_DB` are in `.env.local` (loaded by the compose file). Source `.env.local` in the orchestrator shell before invoking, or pass them inline. Do not use host-side psql — version skew with the container is a known footgun.

Selection contract:
- **Always:** latest snapshot has non-null `description_text`
- **Additionally when `force=false`:** `NOT EXISTS (SELECT 1 FROM classifications c WHERE c.job_posting_id = job_postings.id)`
- **When `force=true`:** drop only the `NOT EXISTS` filter; `description_text IS NOT NULL` still applies. No DELETE of old rows; a new `classifications` row is inserted alongside existing history.
- If focus is non-empty: ILIKE prefilter on title or description_text
- Order by `job_postings.first_seen_at` ascending
- LIMIT by `count`

Pull: `posting_id`, `company_id`, latest snapshot's `title`, latest snapshot's `description_text`.

Log how many postings were skipped because their latest snapshot had NULL `description_text`.

**Reference query** (latest-snapshot-per-posting via `LATERAL`):

Base query (always used):
```sql
SELECT jp.id AS posting_id,
       jp.company_id,
       s.title,
       s.description_text
FROM job_postings jp
JOIN LATERAL (
    SELECT title, description_text
    FROM posting_snapshots
    WHERE job_posting_id = jp.id
    ORDER BY fetched_at DESC
    LIMIT 1
) s ON true
WHERE s.description_text IS NOT NULL
ORDER BY jp.first_seen_at ASC
LIMIT <count>;
```

When `force=false`, also add to the `WHERE` clause:
```sql
  AND NOT EXISTS (
      SELECT 1 FROM classifications c WHERE c.job_posting_id = jp.id
  )
```

Use `DISTINCT ON` or `LATERAL` to keep one row per posting; a naive join against `posting_snapshots` produces duplicates.

### 4. Load existing taxonomy

```sql
SELECT slug, name FROM canonical_roles;
SELECT slug, name FROM specializations;
SELECT slug, name FROM skills;
SELECT slug, name FROM role_dimensions;
```

Hold the four lists in orchestrator state. Every agent receives them verbatim.

In the agent prompt template, render `role_dimensions` as a bullet list under a `## Role dimensions (closed set)` heading, immediately after the canonical-roles taxonomy section.

### 5. Strip per-company boilerplate

Group selected postings by `company_id`. For each company with ≥3 selected postings, strip boilerplate:

```bash
echo '{"company_id": <int>, "selected_ids": [<int>, ...]}' \
  | go run ./cmd/strip-boilerplate
```

The binary self-fetches the company's full description corpus from the DB, runs `boilerplate.Strip`, and writes to stdout:

```json
{"postings": [{"posting_id": <int>, "cleaned_text": "<string>"}, ...]}
```

Substitute `cleaned_text` for `description_text` in the agent prompt for postings returned by the binary. Companies with <3 selected postings skip this step entirely and pass through unchanged.

If the binary exits non-zero, abort the skill.

### 6. Dispatch waves


Wave size: **10**. For each wave, send 10 Agent tool calls in a **single message** — true parallel dispatch is the point of this skill. Wait for all 10 to return before starting the next wave. Final wave may be smaller than 10.

Agent config per call:
- `subagent_type`: `general-purpose`
- `model`: `haiku`
- `description`: `Enrich posting <posting_id>`

### 7. Agent contract

Each Haiku agent receives one posting and returns one JSON object.

**Input to agent:**
- `posting_id`, title, `description_text`
- Full existing taxonomy lists (canonical_roles, specializations, skills, role_dimensions)
- Focus guidance (or omitted if empty)
- The output schema below
- Instruction: respond with JSON only, no prose, no code fences

**Output schema:**

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
      "dimensions": ["design", "engineering"]
    }
  ],
  "specializations": [{"slug": "...", "name": "..."}],
  "skills": [{"slug": "...", "name": "...", "requirement": "required|preferred"}],
  "summary": "<100–200 tokens>"
}
```

Note: writeback ignores `skills[].requirement` today — there is no storage column to land it in yet. The contract preserves the field for future storage.

**Classification discipline (in agent prompt):**
- `canonical_roles` is an array. Blended roles are first-class; a posting may map to multiple roles (e.g. design-engineering hybrids).
- Always emit `seniority`. If seniority cannot be determined from the posting, emit `"seniority": "unknown"` — never omit the field.
- `notes` is optional. Either omit the key entirely or emit `null` when there are no notes. An empty string is treated as null.
- `dimensions` is a **closed set**. Pick slugs only from the list provided in the prompt. Do not invent new dimension slugs.
- Every emitted canonical_role — new or existing — must include a non-empty `dimensions` array. An empty array or missing key is a parse error; the orchestrator skips the posting.

**Slug discipline (in agent prompt):**
- Prefer an existing slug. Propose a new one only when no existing slug fits.
- Kebab-case, ASCII, stable across re-runs.
- New canonical_roles must include at least one `dimensions` slug from the closed set.

**Summary contract (in agent prompt):**
- 100–200 tokens
- Covers: role, seniority, required skills, preferred skills, domain, role type (IC vs lead, full-time vs contract)
- No marketing fluff, no company-specific framing
- This text is what we'll embed later — write it for semantic similarity, not human reading

### 8. Writeback

Read `PROMPT_VERSION` and `MODEL` from the `classification-pins` block at the top of this file once at invocation start. Apply both uniformly to every posting in the batch. Parse the block as exact `KEY=VALUE` lines, no spaces around `=`; orchestrator greps `^PROMPT_VERSION=` and `^MODEL=` from the block.

This writeback path is transitional — raw psql inside a Bash heredoc, with the orchestrator interpolating values into SQL string literals. The typed Go contract lives in `internal/db/queries/classifications.sql` (generated: `internal/db/classifications.sql.go`); a Go caller will replace this Bash path in a future plan. Until then, the validation and escaping rules in 8a are mandatory: agent-emitted strings are untrusted input.

#### 8a. Pre-interpolation validation and escaping

Run before any transaction opens. If any check fails, skip the posting, log the failure (`posting_id` + reason + offending value), continue the batch.

**Rule A — Slug validation.** Every slug emitted by the agent (canonical_role slugs, specialization slugs, skill slugs, dimension slugs) must match `^[a-z0-9]+(-[a-z0-9]+)*$` and be at most 64 characters. This rejects leading/trailing dashes and consecutive dashes. A slug that fails either check is a parse error.

**Rule B — String escaping.** Before interpolating any `name` or `notes` value into a SQL string literal, replace every `'` with `''` (SQL single-quote doubling). Apply to canonical_role names, specialization names, skill names, and classification notes. Slugs already passed Rule A and need no escaping. Relies on `standard_conforming_strings = on` (Postgres default since 9.1 — the Docker image used here sets this). If `\` appears in a name or notes value, it is treated as a literal backslash, not an escape character.

**Rule C — Null byte rejection.** Before interpolating any `name` or `notes` value, reject (skip posting, log parse error) any value containing the null byte (`\x00`). Postgres rejects it at the protocol level; a mid-transaction abort is not a clean skip.

These rules apply to every interpolation in both phases below.

#### 8b. Phase A — Pre-flight (no transaction)

For each posting, in order:

0. **Validate seniority.** If `classification.seniority` is missing or null, skip the posting, log a parse error (`posting_id` + `seniority missing`), continue the batch.

1. **Validate and escape.** Apply Rule A to every slug and Rule B to every name/notes value per 8a. Any failure: skip, log, continue.

2. **Verify every canonical_role has a non-empty dimensions array.** Before any resolution, check that every emitted canonical_role has a `dimensions` array with at least one entry. If any role has an empty or missing array: skip the posting, log a parse error (`posting_id` + role slug + `empty dimensions`), continue the batch. This is checked explicitly so a future refactor that changes resolution loop semantics cannot silently drop the guard.

3. **Resolve dimension slugs.** For each posting, build a per-posting mapping of canonical_role slug → [dimension_id] by looking up each emitted `canonical_roles[].dimensions[]` slug in the in-memory `role_dimensions` map already loaded in §4. No additional psql calls are needed. If any slug is not present in the in-memory map: skip the posting, log a parse error (`posting_id` + unknown slug), continue the batch. Do not open the transaction.

Pre-flight is the only place validation can fail. If a posting clears Phase A, Phase B can only fail on database errors.

#### 8c. Phase B — Transactional writeback

Open one psql transaction per posting. Each posting runs in a single `psql` invocation wrapping one BEGIN…COMMIT block:

```bash
# single-quoted delimiter prevents shell from expanding $, $1, $$, etc. inside SQL
docker compose exec -T db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;
SET LOCAL standard_conforming_strings = on;
-- writeback steps here
COMMIT;
SQL
```

The `SET LOCAL standard_conforming_strings = on` is the first statement inside the transaction. The Rule B escaping in §8a (single-quote doubling, treating `\` as literal) is only safe when this setting is on. Postgres has defaulted it to `on` since 9.1, but pinning it per-transaction makes the assumption explicit and survives any future server-side override.

The whole transaction — taxonomy upserts, classification insert, join inserts — runs as one psql script delivered via stdin heredoc. psql processes `\gset` (used in step 3) only when reading a script, not with `-c`, so the heredoc delivery mode is load-bearing.

Steps within each transaction:

1. **Upsert taxonomy by slug; collect ids.**
   - `canonical_roles`:
     ```sql
     INSERT INTO canonical_roles (slug, name) VALUES ('<slug>', '<name>') ON CONFLICT (slug) DO NOTHING;
     SELECT id FROM canonical_roles WHERE slug = '<slug>';
     ```
   - Same pattern for `specializations` and `skills`.
   - Agent-emitted `name` is used only on initial insert; on conflict, the existing row's `name` and `created_at` are unchanged.
   - Note: the sqlc typed contract (`GetOrCreateCanonicalRole`, etc.) uses `ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug` — a no-op write that acquires a row lock, preventing races under READ COMMITTED. This runtime `DO NOTHING` + `SELECT` path is equivalent under single-orchestrator operation but does not acquire the lock; concurrent orchestrators would need the sqlc path.

2. **Upsert canonical_role_dimensions.** For each `(canonical_role_id, dimension_id)` pair from the Phase A mapping:
   ```sql
   INSERT INTO canonical_role_dimensions (canonical_role_id, dimension_id)
   VALUES (<canonical_role_id>, <dimension_id>)
   ON CONFLICT DO NOTHING;
   ```

3. **Insert classifications row and capture id.** One row per posting per run. Use psql's `\gset` to bind the returned id to a script variable for the join inserts in step 4:
   ```sql
   INSERT INTO classifications (job_posting_id, model, prompt_version, seniority, notes)
   VALUES (<posting_id>, '<MODEL>', '<PROMPT_VERSION>', '<seniority>', <notes_or_null>)
   RETURNING id AS classification_id;
   \gset
   ```
   After `\gset`, psql binds the `classification_id` column from the last result row to the variable `:classification_id`, referenced by step 4. `\gset` runs client-side inside the same psql session, so the bind sits within the open transaction — no second connection, no atomicity break.

   `classified_at` is deliberately omitted from the INSERT — the column's `DEFAULT now()` sets it server-side, ensuring concurrent `--force` runs against the same posting receive distinct timestamps.

   For `<notes_or_null>`: if notes is null, missing, or empty after stripping ASCII whitespace (leading and trailing, equivalent to Go `strings.TrimSpace`), interpolate the bare keyword `NULL` (no quotes). Otherwise interpolate `'<escaped-notes>'` (with Rule B escaping applied).

4. **Insert join rows using `:classification_id`.**
   - `INSERT INTO job_posting_roles (classification_id, role_id) VALUES (:classification_id, <role_id>) ON CONFLICT DO NOTHING`
   - `INSERT INTO job_posting_specializations (classification_id, specialization_id) VALUES (:classification_id, <specialization_id>) ON CONFLICT DO NOTHING`
   - `INSERT INTO job_posting_skills (classification_id, skill_id) VALUES (:classification_id, <skill_id>) ON CONFLICT DO NOTHING`

   `:classification_id` is the psql variable bound in step 3; psql substitutes the integer before sending each statement to the server. All three inserts run in the same psql session and the same open transaction.

Drop fields with no storage column:
- `summary` — report only.

On transaction failure for a posting (DB error only — validation already passed): skip it, continue the batch, log `posting_id` and error to the markdown report. That posting will be re-selected on the next non-`--force` run.

### 9. Report

Write `agent-output/batch-enrich/<YYYY-MM-DD-HHMM>.md`. Create the directory if missing. `agent-output/` is gitignored.

Report contents:
- Run params (count, focus, force)
- Counts: selected, dispatched, enriched, JSON-failed, validation-failed, skipped-no-description, re-enriched (when force)
- Validation-failed breakdown by reason: seniority missing, unknown dimension slug, empty dimensions array, invalid slug format
- New taxonomy added (slug, name for roles; slug, name for specializations and skills; new canonical_role_dimensions pairs)
- Per-posting summaries (`posting_id`, title, summary text)

Note in the report that postings skipped on validation are not written and will be re-selected on the next non-`--force` run.

After writing, print a one-line verbal summary to the user: counts + report path.

## Principles

- **You coordinate, you don't classify.** Don't read descriptions in the orchestrator.
- **Wave size is 10, dispatched in one message.** Serial Agent calls defeat the purpose.
- **New taxonomy is a signal.** Surface every new slug so the user catches taxonomy drift early.
- **Fail loudly.** Log every parse error and skipped posting with its id.
- **`--force` is additive, not destructive.** Inserts a new `classifications` row per posting; prior rows remain as history. Drops only the `NOT EXISTS` unenriched filter — `description_text IS NOT NULL` still applies. Every posting with a description is a candidate, including those already classified.
- **No schema changes.** Summary storage and required/preferred preservation are follow-ups, not this skill's job.
