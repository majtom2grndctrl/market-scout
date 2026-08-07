---
name: batch-enrich
description: >
  Classifies unenriched job postings via parallel Haiku subagents — the
  subscription-covered bulk-enrichment path now that `claude -p` bills API
  dollars outside the Max plan. Each subagent is self-contained: it fetches its
  own posting descriptions, loads the taxonomy and cross-table slug-collision
  list, classifies into canonical roles (with dimension mappings),
  specializations, skills, and a summary, then writes back by calling
  `save_enrichment` directly. Orchestrator selects posting IDs only and
  dispatches in waves of 10 agents, each processing a small sequential chunk.
  Use to enrich job postings in bulk during active data collection.
allowed-tools: Read, Bash, Agent, Write
argument-hint: "<count> [focus description] [--force] [--recent]"
---

> **Status:** previously deprecated in favor of `cmd/batch-enrich` — back in play as of 2026-07. `claude -p` is no longer covered by the Max subscription, so the Go binary bills API dollars per posting while this skill runs on session tokens. Path forward (skill revival, hook adaptation) is undecided; until then, keep this skill and the binary's classification contract in sync — `classification-pins` below is the shared anchor.

```classification-pins
PROMPT_VERSION=batch-enrich-v4
MODEL=claude-haiku-4-5-20251001
```

# Batch Enrich

Enrich job postings into canonical roles, specializations, skills, and a structured summary. **You coordinate, you don't classify.** Select posting IDs, dispatch Haiku agents in parallel waves, aggregate their reports. Each agent is fully self-contained: it reads its own data, classifies, and writes its own results through `save_enrichment`. There is no JSON handoff back to the orchestrator.

## Architecture at a glance

| Role | Owns |
|---|---|
| Orchestrator (this skill) | Arg parsing, posting-ID selection, wave/chunk dispatch, report |
| Agent (Haiku, per chunk) | Fetch descriptions, load taxonomy + collision list, classify, `save_enrichment`, retry on `ok:false` |

- Orchestrator selects **which posting IDs** — nothing else. It never loads taxonomy or collision data. Injecting that into every prompt double-pays: read once, then re-typed as input tokens per agent, and agents re-query it anyway.
- Every agent reads taxonomy/collision state fresh, itself, every time.
- `save_enrichment` commits per posting, so a killed run (e.g. hitting a usage limit mid-wave) leaves already-processed postings safely committed. Unreached postings re-select automatically on the next run.

## Terminology

| Term | Meaning |
|---|---|
| Wave | One round of concurrently-dispatched `Agent` calls — size 10, sent in a single message. |
| Chunk | The set of posting IDs assigned to one agent, processed **sequentially inside** that agent. |

A wave's total postings = (agents per wave) × (chunk size).

## Args

`$ARGUMENTS` — first token is record count (integer); the rest is freeform focus guidance.

| Arg | Example | Effect |
|---|---|---|
| count | `25` | LIMIT on selection |
| focus | `prioritize AI/ML engineering roles` | ILIKE prefilter on title + description; passed to each agent as guidance |
| `--force` | `--force` | Re-classify postings that already have classifications. `save_enrichment` inserts a new classification row; history is never deleted. May appear anywhere in args. |
| `--recent` | `--recent` | Flip selection order to newest-first (`first_seen_at DESC`). Default is oldest-first. May appear anywhere in args. |

If focus is empty: select oldest unenriched, no agent guidance beyond the schema.

## Process

### 1. Read context (skip if already loaded this session)

- `agent-context/lib/index.md`
- `agent-context/lib/project.md` §Settled architecture, §Database as AI agent knowledge store

### 2. Parse args

Strip `--force` and `--recent` from `$ARGUMENTS` first using exact token matching (whitespace-delimited — a token like `forceful` that merely contains `force` as a substring is unaffected); record whether each was present. Then:

- `count` — first remaining token; integer; default 10 if absent, non-numeric, zero, or negative
- `focus` — remaining text, may be empty
- `force` — boolean from the strip step
- `recent` — boolean from the strip step

### 3. Select posting IDs

Selection is the orchestrator's only DB read. Pull IDs, not descriptions — agents fetch their own text.

Postgres runs in docker compose (service name `db`). Run psql inside the container:

```bash
docker compose exec -T db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1
```

`POSTGRES_USER` and `POSTGRES_DB` are in `.env.local` (loaded by the compose file). Source `.env.local` in the orchestrator shell before invoking, or pass them inline. Do not use host-side psql — version skew with the container is a known footgun.

Selection contract:
- **Always:** latest snapshot has non-null `description_text`
- **When `force=false`:** also require `NOT EXISTS (SELECT 1 FROM classifications c WHERE c.job_posting_id = job_postings.id)`
- **When `force=true`:** drop the `NOT EXISTS` filter; `description_text IS NOT NULL` still applies. No DELETE of old rows.
- If focus is non-empty: ILIKE prefilter on title or description_text
- Order by `job_postings.first_seen_at` — `ASC` by default, `DESC` when `recent=true`
- LIMIT by `count`

Pull `posting_id` and `company_id` only. `company_id` is used to spread same-company postings across chunks (see chunk risks in Step 5) — not to fetch descriptions.

Log how many postings were skipped because their latest snapshot had NULL `description_text`.

**Reference query** (latest-snapshot-per-posting via `LATERAL`; `<order>` is `ASC` or `DESC`; drop the `NOT EXISTS` block when `force=true`):

```sql
SELECT jp.id AS posting_id, jp.company_id
FROM job_postings jp
JOIN LATERAL (
    SELECT description_text
    FROM posting_snapshots
    WHERE job_posting_id = jp.id
    ORDER BY fetched_at DESC
    LIMIT 1
) s ON true
WHERE s.description_text IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM classifications c WHERE c.job_posting_id = jp.id
  )
ORDER BY jp.first_seen_at <order>
LIMIT <count>;
```

Use `LATERAL` (or `DISTINCT ON`) to keep one row per posting; a naive join against `posting_snapshots` produces duplicates.

To preview a selection before committing to it, `enrichment_preview` takes a `sort` param: `"newest_first"` matches `--recent`, `"oldest_first"` (the default) matches a plain run.

### 4. Strip per-company boilerplate (on by default)

`cmd/strip-boilerplate` is fixed and on by default. (Prior bug: it matched on `\n\n` paragraph breaks, but `internal/ats`'s HTML-to-text conversion collapses all whitespace, so real `description_text` has no paragraph breaks and nothing ever matched. Fixed via exact-substring detection with a conservative whitespace-boundary trim — see the binary's package for detail.) Cleaned text now measurably shrinks (15–48% observed) with shared blurb/footer text removed and posting-specific content intact.

```bash
echo '{"company_id": <int>, "selected_ids": [<int>, ...]}' \
  | go run ./cmd/strip-boilerplate
```

It self-fetches the company's description corpus, runs `boilerplate.Strip`, and returns `{"postings": [{"posting_id": <int>, "cleaned_text": "<string>"}, ...]}`. Companies with fewer than 3 selected postings aren't included in the response — there's no corpus to detect shared text against, so the agent uses raw `description_text` for those unchanged.

**This needs same-company postings co-located in one chunk to do anything.** The tool only strips shared text it can see within a single invocation, so if a company has ≥3 postings selected in the run but they're spread across separate chunks (per the anchoring-avoidance guidance in Step 5), none of those chunks ever has enough of that company's corpus to trigger stripping — the step would silently do nothing again, just via a different failure mode than the original bug. Step 5's chunk-assignment guidance is written to account for this: spread singles and small (<3) same-company groups to limit anchoring; keep a company's postings together when it has ≥3 selected in the run, so stripping actually has a corpus to work with. See Step 6 for exactly when and how each agent invokes this.

### 5. Dispatch waves and chunks

Run `mkdir -p agent-output/batch-enrich` once before the wave loop — failure-log writes may occur during the wave.

**Wave:** size **10**. For each wave, send 10 `Agent` tool calls in a **single message** — parallel dispatch is the point. Wait for all to return before the next wave. Final wave may be smaller.

**Chunk:** assign each agent a set of posting IDs. The chunk amortizes an agent's fixed cost — tool loading plus the taxonomy fetch (~700 rows across three tables) — across several postings instead of paying it once per posting. Trade-off, stated explicitly:
- Larger chunks: cheaper per posting, but risk (a) context drift/anchoring across many postings from the same company, and (b) less wall-clock parallelism (fewer, bigger agents run slower for the same total work).
- Smaller chunks: more parallelism, higher fixed-cost overhead.

**Chunk size is not a fixed default — it's an open experiment.** The ceiling where per-agent context drift or task-quality degradation kicks in is unknown; it needs to be found empirically rather than asserted. Start experiments at **15** and be willing to push to **50**, watching for: classification quality drift late in a chunk (spot-check postings near the end of a large chunk against ones near the start), retry/error rates climbing, and whether an agent begins to anchor on a company's earlier postings when classifying later ones from a different company in the same chunk. Whatever the operator (a human or an orchestrating agent) picks per run, based on what a given run is optimizing for — reserve small chunks for when parallelism/wall-clock matters more, larger chunks for pure token-efficiency runs over the large and growing backlog.

Spread same-company postings across different chunks where possible, to limit anchoring — this matters more, not less, as chunk size grows. **Exception:** when a company has ≥3 postings selected in the run, keep that company's postings together in one chunk instead of spreading them — this is what lets Step 4's boilerplate stripping find a corpus to work with. Spread everything else (singles and pairs) as usual.

Agent config per call:
- `subagent_type`: `general-purpose`
- `model`: `haiku`
- `description`: `Enrich chunk <first_id>…` (or list the IDs)

**Cross-wave taxonomy visibility.** Agents in one wave each query the taxonomy once, at chunk start, so parallel agents cannot see each other's newly-minted slugs — a pre-existing property of parallel dispatch, not a new risk from chunking. Taxonomy created mid-wave becomes visible starting the *next* wave, when fresh agents query. Durability is unaffected by chunk size: `save_enrichment` commits per posting, so an interrupted chunk leaves reached postings committed and only the unreached ones re-select later.

### 6. Agent contract

Each agent receives a **chunk of posting IDs** and the pinned `PROMPT_VERSION` / `MODEL` (from the `classification-pins` block, read once at invocation start). It is self-contained and tool-using.

**Setup (once per chunk):**

1. Load `mcp__market-scout-postgres__query` and `mcp__market-scout-postgres__save_enrichment` via ToolSearch.
2. Load the taxonomy:
   ```sql
   SELECT slug, name FROM canonical_roles   ORDER BY slug;
   SELECT slug, name FROM specializations   ORDER BY slug;
   SELECT slug, name FROM skills            ORDER BY slug;
   SELECT slug, name FROM role_dimensions   ORDER BY slug;
   ```
3. Load the cross-table slug-collision list:
   ```sql
   SELECT slug FROM (
     SELECT slug FROM canonical_roles
     UNION ALL SELECT slug FROM specializations
     UNION ALL SELECT slug FROM skills
   ) t GROUP BY slug HAVING count(*) > 1;
   ```
   Slugs must be unique **across all three tables**, not just within one. `save_enrichment` enforces this and rejects colliding inserts. Loading the live list lets the agent avoid a collision proactively instead of discovering it via a failed write. (Legacy collisions predate this skill and are a cleanup candidate — rely on the live query, never a hardcoded list.)
4. If the chunk contains ≥3 postings from the same company (chunk assignment should already group them this way — see Step 5), strip boilerplate for that subgroup once, up front:
   ```bash
   echo '{"company_id": <int>, "selected_ids": [<int>, ...]}' \
     | (cd apps/tools && go run ./cmd/strip-boilerplate)
   ```
   Use each returned `cleaned_text` in place of raw `description_text` for that posting in the loop below. Postings from companies with <3 in the chunk use raw text as normal — skip this step for them.

**Per posting in the chunk (loop):**

1. Fetch the posting's latest snapshot text by ID (unless boilerplate stripping already supplied `cleaned_text` for it above):
   ```sql
   SELECT s.title, s.description_text
   FROM job_postings jp
   JOIN LATERAL (
       SELECT title, description_text
       FROM posting_snapshots
       WHERE job_posting_id = jp.id
       ORDER BY fetched_at DESC
       LIMIT 1
   ) s ON true
   WHERE jp.id = <posting_id>;
   ```
2. Classify per the discipline in Step 7.
3. Write back with `save_enrichment` (see below).
4. Move to the next posting.

**Writeback — call `save_enrichment` directly.** The tool handles slug validation, string escaping, null-byte rejection, cross-table slug uniqueness, and the transaction atomically. This is the tool boundary: the agent never interpolates SQL or escapes strings for writeback — that is the MCP server's concern.

Call with:

| Param | Value |
|---|---|
| `posting_id` | the current posting ID |
| `classification` | `{seniority, notes}` |
| `canonical_roles` | `[{slug, name, dimensions}]` |
| `specializations` | `[{slug, name}]` |
| `skills` | `[{slug, name, requirement}]` |
| `summary` | the structured summary string |
| `provenance` | `{model: <MODEL>, prompt_version: <PROMPT_VERSION>}` — **required, pass explicitly** |

**Provenance is not optional in practice.** `save_enrichment` silently defaults to `model: "mcp-agent"` / `prompt_version: "mcp-save-enrichment-v1"` when provenance is omitted — losing the cohort tracking the `classification-pins` block exists for. Always pass the pinned `MODEL` and `PROMPT_VERSION`.

`summary` and `skills[].requirement` are echoed but not persisted (pgvector storage and requirement columns are deferred per `project.md` non-goals). Keep emitting them so the signal lands when storage arrives; report `summary` back for the markdown report.

**On `ok:false` (e.g. `slug_collision`):** read the error and pick a real fix — a distinct non-colliding slug, or drop the field if the concept is already covered elsewhere. Never blindly rename without checking the axis rule (Step 7). Retry the same posting. **Cap: 2 additional attempts (3 total).** If still unresolved, report that posting as failed in the chunk summary rather than looping.

**Agent's final report (back to orchestrator):** for its chunk, return per-posting outcome (classified / failed + reason), any new slugs it minted (role/specialization/skill slug + name), and each posting's `summary` text. The orchestrator aggregates these — it does not re-read taxonomy.

### 7. Classification discipline (in agent prompt)

- `canonical_roles` is an array. Blended roles are first-class; a posting may map to multiple roles (e.g. design-engineering hybrids).
- Always emit `seniority`. If it cannot be determined, emit `"seniority": "unknown"` — never omit the field. Allowed values: `intern`, `junior`, `mid`, `senior`, `staff`, `principal`, `lead`, `director`, `unknown`.
- `notes` is optional. Omit the key or emit `null` when there are none. Whitespace-only is treated as null.
- `dimensions` is a **closed set** (`role_dimensions`). Pick only slugs from that list. Never invent dimension slugs.
- Every emitted canonical_role — new or existing — must carry a non-empty `dimensions` array.

**Slug discipline:**
- Prefer an existing slug. Propose a new one only when none fits.
- Kebab-case, ASCII, stable across re-runs.
- Avoid slugs on the collision list; `save_enrichment` rejects cross-table collisions.
- Pick the table by axis, not by how domain-specific a term sounds:

  | Table | Axis | Examples |
  |---|---|---|
  | `specializations` | Domain, industry, product area | `fintech`, `retail-commerce`, `ai-agents` |
  | `skills` | Concrete technology, tool, framework, language, or competency | `spring-boot`, `react`, `people-management` |

  A framework is a skill, never a specialization — `spring-boot` is backend-only and not a stand-in for `full-stack-engineering`, which describes working across both layers, not any one framework.

**Grounding discipline (skills and specializations):**
- Tag only what the description names or clearly implies — not what the role typically needs. Before adding an item, find the phrase that supports it. No phrase, no tag.
- An empty or short list is a correct result, not a failure. A posting that says "designing and implementing a robust, scalable data platform" with no tools named gets no tool skills — not `aws, azure, gcp, snowflake, databricks, bigquery, kafka, spark, airflow, dbt, java, scala, python` inferred from what data-platform roles typically use.

**Summary contract:**
- 100–200 tokens.
- Covers: role, seniority, required skills, preferred skills, domain, role type (IC vs lead, full-time vs contract).
- No marketing fluff, no company-specific framing.
- This text is what we'll embed later — write it for semantic similarity, not human reading.

**Output shape** the agent builds for each `save_enrichment` call:

```json
{
  "posting_id": <int>,
  "classification": {
    "seniority": "intern|junior|mid|senior|staff|principal|lead|director|unknown",
    "notes": "<optional freeform or null>"
  },
  "canonical_roles": [
    {"slug": "<existing or new>", "name": "<human-readable>", "dimensions": ["design", "engineering"]}
  ],
  "specializations": [{"slug": "...", "name": "..."}],
  "skills": [{"slug": "...", "name": "...", "requirement": "required|preferred"}],
  "summary": "<100–200 tokens>"
}
```

### 8. Report

Write `agent-output/batch-enrich/<YYYY-MM-DD-HHMM>.md`. Create the directory if missing. `agent-output/` is gitignored.

Aggregate from agent reports (the orchestrator holds no classification data of its own):

- Run params (count, focus, force, recent)
- Counts: selected, dispatched, enriched, failed, skipped-no-description, re-enriched (when force)
- Failure breakdown by reason (from agent reports — e.g. `slug_collision unresolved after retries`)
- New taxonomy minted this run (slug + name for roles; slug + name for specializations and skills), deduped across agents — two agents in a wave may propose the same new slug
- **Recurring failures.** Read `agent-output/batch-enrich/failures.jsonl` (tolerate a missing or empty file — render `(none)`, do not error). Group by `posting_id`, count distinct `run_timestamp` values. Render only postings with ≥ 2 failed runs: `posting_id`, failed-run count, and the set of distinct `reason` strings across all runs. Postings that have since classified still appear — the log is append-only and records no successes.
  - TODO: add a rotation strategy for `failures.jsonl` — scanned in full every run with no size bound.
- Per-posting summaries (`posting_id`, title, summary text)

Note in the report that failed postings are not written and will be re-selected on the next non-`--force` run.

After writing, print a one-line summary to the user: counts + report path.

#### Failure logging

For each posting an agent reports as failed, append one line to `agent-output/batch-enrich/failures.jsonl`. Append-only — never truncated. Ensure the directory exists before the first append (`mkdir -p agent-output/batch-enrich`); do not assume Step 8's report run has happened.

| Field | Type | Notes |
|---|---|---|
| `run_timestamp` | string | Orchestrator invocation-start time, `YYYY-MM-DD-HHMM`. Captured once at run start, reused for every line. Matches the report filename stem. |
| `posting_id` | int | Failed posting. |
| `reason` | string | Short reason from the agent's report (e.g. `slug_collision unresolved`, `save_enrichment error`). |
| `prompt_version` | string | The pinned `PROMPT_VERSION`. |
| `model` | string | The pinned `MODEL`. |
| `attempted_at` | string | ISO-8601 timestamp when the agent report was collected. |

`save_enrichment` failures that the agent *resolves* on retry produce no line — only postings the agent ultimately gives up on are logged.

## Principles

- **You coordinate, you don't classify.** Never read descriptions in the orchestrator.
- **Agents are self-contained.** They fetch data, load taxonomy/collision state fresh, classify, and write via `save_enrichment` themselves. No orchestrator-mediated JSON handoff.
- **Selection only.** The orchestrator's DB reach is posting-ID selection — never taxonomy or collision data.
- **Wave size is 10, dispatched in one message.** Serial `Agent` calls defeat the purpose.
- **Provenance is explicit.** Every `save_enrichment` call passes the pinned `MODEL` / `PROMPT_VERSION`, or cohort tracking is silently lost.
- **New taxonomy is a signal.** Surface every new slug so the user catches taxonomy drift early.
- **Fail loudly, cap retries.** Log every ultimate failure with posting ID and reason; agents retry `ok:false` at most twice before giving up.
- **`--force` is additive, not destructive.** `save_enrichment` inserts a new classification row per posting; prior rows remain as history.
- **No schema changes.** Summary storage and `requirement` persistence are follow-ups, not this skill's job.
