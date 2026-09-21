---
name: batch-enrich
description: >
  Classifies unenriched job postings via parallel Haiku subagents — the
  subscription-covered bulk-enrichment path now that `claude -p` bills API
  dollars outside the Max plan. Each subagent is self-contained: it fetches its
  own posting descriptions, loads the taxonomy and cross-table slug-collision
  list, classifies into canonical roles (with dimension mappings),
  specializations, skills, and a summary, then writes back by calling
  `save_enrichment` directly. Orchestrator selects deduplicated work units
  and dispatches in waves of 10 agents, each processing a small sequential chunk.
  Use to enrich job postings in bulk during active data collection.
allowed-tools: Read, Bash, Agent, Write
argument-hint: "<count> [focus description] [--force] [--backlog] [--per-company N]"
---

> **Status:** previously deprecated in favor of `cmd/batch-enrich` — back in play as of 2026-07. `claude -p` is no longer covered by the Max subscription, so the Go binary bills API dollars per posting while this skill runs on session tokens. Path forward (skill revival, hook adaptation) is undecided; until then, keep this skill and the binary's classification contract in sync — `classification-pins` below is the shared anchor.

```classification-pins
PROMPT_VERSION=batch-enrich-v8
MODEL=claude-haiku-4-5-20251001
```

This block is the only place these values are written. Read it once at
invocation start and substitute it wherever this file shows `<PROMPT_VERSION>`
or `<MODEL>`. Never restate a version string as a literal — the Codex skill did,
and its workers wrote `batch-enrich-v6` for weeks under a `batch-enrich-v7` pin.

`PROMPT_VERSION` names the **classifier contract**, not the model and not the
harness. Model is its own column, and one contract runs under Haiku here and
under GPT-5.6 Luna/Terra in `.agents/skills/batch-enrich/SKILL.md` — so the two
skills share one pin value and bump together. `apps/tools/cmd/batch-enrich` is a
different contract on a different transport and carries its own `batch-enrich-go-v<n>`
lineage; do not reuse this one there.

**Bump `PROMPT_VERSION` in the same commit as any change to classification
discipline, grounding rules, the agent contract, or selection semantics.**
This pin sat at `batch-enrich-v4` from 2026-05-15 to 2026-09-17 while five
materially different prompts shipped under it (2026-07-27, 2026-08-07 ×2,
2026-09-17 ×3). Every `provenance.prompt_version` written in that window
collapses onto one string: no cross-run query can separate the cohorts,
and no backfill can tell which prompt classified which posting. A stale
pin isn't a paperwork gap — it destroys the one thing that lets later work
tell these runs apart.

# Batch Enrich

Enrich job postings into canonical roles, specializations, skills, and a structured summary. **You coordinate, you don't classify.** Select deduplicated work units, dispatch Haiku agents in parallel waves, aggregate their reports. Each agent is fully self-contained: it reads its own data, classifies, and writes its own results through `save_enrichment`. There is no JSON handoff back to the orchestrator.

## Architecture at a glance

| Role | Owns |
|---|---|
| Orchestrator (this skill) | Arg parsing, **deduplicated work-unit selection via `enrichment_preview`**, wave/chunk dispatch, report |
| Agent (Haiku, per chunk) | Fetch the representative's description, load taxonomy + collision list, classify once, `save_enrichment` per sibling, retry on `ok:false` |

- Orchestrator selects **which work units** — nothing else, and it selects by calling `enrichment_preview`, not by writing SQL. It never loads taxonomy or collision data. Injecting that into every prompt double-pays: read once, then re-typed as input tokens per agent, and agents re-query it anyway.
- Every agent reads taxonomy/collision state fresh, itself, every time.
- `save_enrichment` commits per posting, so a killed run (e.g. hitting a usage limit mid-wave) leaves already-processed postings safely committed. Unreached postings re-select automatically on the next run.
- Duplicate postings are collapsed **before** dispatch, not reconciled after. One job is read once, judged once, and written to each of its postings.

## Terminology

| Term | Meaning |
|---|---|
| Wave | One round of concurrently-dispatched `Agent` calls — size 10, sent in a single message. |
| Chunk | The set of work units assigned to one agent, processed **sequentially inside** that agent. |
| Work unit | One representative posting plus its sibling postings — the same job listed more than once. Read once, classified once, saved to every posting in the unit. Atomic: never split across chunks. |
| Representative | The posting whose text the agent actually reads. Lowest `posting_id` in the unit. |

A wave's total work units = (agents per wave) × (chunk size). Postings touched is higher, by the number of siblings.

## Args

`$ARGUMENTS` — first token is record count (integer); the rest is freeform focus guidance.

| Arg | Example | Effect |
|---|---|---|
| count | `25` | Window size on selection, passed to `enrichment_preview`'s `count` param. Bounds **work units**; complete units are never split, so postings returned is ≥ `count`. Report both numbers. Hard-bounded 1..500 by the tool (Step 3); a run wanting more must be split across multiple invocations. |
| focus | `prioritize AI/ML engineering roles` | ILIKE prefilter on title + description; passed to each agent as guidance |
| `--force` | `--force` | Re-classify postings that already have classifications. `save_enrichment` inserts a new classification row; history is never deleted. May appear anywhere in args. |
| `--backlog` | `--backlog` | Flip selection order to oldest-first (`first_seen_at ASC`). **Default is newest-first.** Use only for a deliberate backlog drain. May appear anywhere in args. |
| `--per-company` | `--per-company 3` | Max work units one company may contribute to this wave. Defaults to 5. Takes the next token as its integer value; may appear anywhere in args. |

If focus is empty: select the newest unenriched work, no agent guidance beyond the schema.

**Why newest-first is the default.** Oldest-first drained the queue in arrival order, which structurally guaranteed the classified set lagged the market: on 2026-09-21 only 4 of 1,946 classified postings had been first seen in the preceding three weeks, against 1,108 of 2,712 unclassified ones. The project's question is which job titles are *emerging*; a classified set months behind the market cannot answer it.

**Why the per-company cap.** The corpus is ~43% OpenAI/Stripe/Anthropic. Before the cap, a 50-unit wave drew 58 postings from OpenAI alone. The tool also cycles across companies before recency, so a wave takes every company's newest unit, then every company's second-newest. This is deliberate under-sampling of the giants, not proportional sampling: title analysis has to reflect more than three companies' naming conventions.

## Process

### 1. Read context (skip if already loaded this session)

- `agent-context/lib/index.md`
- `agent-context/lib/project.md` §Settled architecture, §Database as AI agent knowledge store

### 2. Parse args

Strip `--force`, `--backlog`, and `--per-company <N>` from `$ARGUMENTS` first using exact token matching (whitespace-delimited — a token like `forceful` that merely contains `force` as a substring is unaffected); record whether each was present, and for `--per-company` take the following token as its value. Then:

- `count` — first remaining token; integer; default 10 if absent, non-numeric, zero, or negative
- `focus` — remaining text, may be empty
- `force` — boolean from the strip step
- `backlog` — boolean from the strip step
- `per_company` — integer from the strip step; omit the param entirely when the flag was absent, non-numeric, or out of 1..500

### 3. Select the work-unit cohort via `enrichment_preview`

Selection is a single MCP tool call, not SQL the orchestrator writes. Pull IDs, not descriptions — agents fetch their own text.

**Preflight — the tool must be reachable before a run starts.** Load `mcp__market-scout-postgres__enrichment_preview` via ToolSearch before dispatch. If ToolSearch can't resolve it, or the call returns a transport error, **stop the run and report the failure.** Do not fall back to hand-written selection SQL. A silent fallback is exactly the drift this section exists to close — see below.

**Why delegate instead of selecting locally.** Correct dedup is transitive connected components over two edges (identical description text; identical requisition key *and* identical normalized title), not a scalar key — an algorithm that doesn't belong hand-copied into a markdown file. It already drifted once: this skill's own prior version of this step implemented text-equality only, the Go core it was meant to mirror implemented the fuller two-edge closure, and the two disagreed on real data within a day of each other (Boulder Care: 41 postings under one requisition key and one normalized title, description revised three times — the Go core correctly resolves that to one work unit; the old text-only SQL here split it into three). One implementation, living in Go, is the only version of this rule that can't drift from itself.

**Call**, mapping the skill's parsed args (Step 2) onto the tool's parameters:

| Skill arg | Tool param | Mapping |
|---|---|---|
| `count` | `count` | Passed through. Hard-bounded **1..500** — see below. |
| `focus` | `focus` | Passed through (empty string when no focus given). ILIKE prefilter on title or description. |
| `force` | `force` | Passed through. `true` drops the already-classified filter. |
| `backlog` | `sort` | `backlog=true` → `"oldest_first"`; otherwise omit the param, which the tool resolves to `"newest_first"`. |
| `per_company` | `max_per_company` | Passed through when given; omit it otherwise and the tool applies its default of 5. Bounded **1..500**; `0` and negatives are rejected with `invalid_max_per_company`, not read as "no cap". |

Dedup is always on inside the tool — there's no parameter to disable it; that's the coordinator's contract, not a per-run choice.

**The 1..500 cap is hard.** A `count` outside that range gets no partial result: the tool returns `ok:false` with an `errors[]` entry (`path: "count", code: "invalid_count"`) and empty `sample`/`postings`. **A desired run over 500 work units cannot be requested in one call — split it across multiple `enrichment_preview` calls and dispatch passes, each ≤ 500.** Do not try to raise the cap or page around it another way.

**Response fields the orchestrator needs:**

| Field | Type | Use |
|---|---|---|
| `ok` | bool | `false` means the call was rejected (`errors[].code`: `invalid_count`, `invalid_sort`, `invalid_max_per_company`, or `db_error`). Stop and report — do not retry with local SQL. |
| `selected_count` | int | Total postings selected, siblings included. ≥ `work_unit_count`. |
| `work_unit_count` | int | Distinct work units selected — what `count` actually bounds. |
| `already_classified_count` | int | Already-classified postings included (nonzero only under `force`). |
| `postings[]` | array | The dispatch cohort — hand this whole list to agents. Distinct from `sample[]`, a lightweight count-capped preview without dedup fields; don't dispatch from `sample`. |
| `postings[].posting_id` | int | Posting to fetch/classify/save. |
| `postings[].company_id` | int | Drives chunk assignment (Step 5). |
| `postings[].company_name` | string | For chunk descriptions and reporting. |
| `postings[].title` | string | For reporting. |
| `postings[].dedup_key` | string | Groups siblings into one work unit. |
| `postings[].is_representative` | bool | The one posting per unit an agent actually reads and classifies; the rest are siblings written with the same result. |

Report both `work_unit_count` and `selected_count` per Step 8 — they differ whenever a unit has siblings, and collapsing them into one number is the failure this step exists to prevent. Log `already_classified_count`, and siblings riding along beyond `count` (`selected_count - work_unit_count`).

**What the tool does — rationale for trusting its output, not a spec to re-derive here.** `enrichment_preview` calls the same shared selection core (`internal/enrich/selection`) that the `batch-enrich` Go binary uses, backed by `ListUnclassifiedPostings` / `ListUnclassifiedPostingsForced` in `apps/tools/internal/db/queries/enrich.sql` — that file is the implementation of record; read its header comment for the exact algorithm.

- **Selection contract**, unchanged from before: latest snapshot must have non-null `description_text`. `force=false` additionally requires no existing `classifications` row for the posting; `force=true` drops that filter and never deletes prior rows. An empty `focus` applies no filter; a non-empty one ILIKE-matches title or description. Ordering is by `first_seen_at`, descending by default, ascending when `sort` is `oldest_first`, and the wave cycles across companies before recency under the `max_per_company` cap.

- **Postings with no description are invisible to selection**, not skipped by it: the shared core filters `description_text IS NOT NULL` before any edge is built. Workday (~120 postings) and Workable (~30) currently store none. That is a fetcher gap, out of this skill's scope — don't report it as a classification failure.
- **A work unit is never split across the `count` boundary.** `count` bounds work units, not raw posting rows; every sibling of a selected unit rides along even past `count`, so postings returned is ≥ `count` (fewer only if the remaining pool has fewer than `count` units left).
- **Why duplicates must collapse before dispatch.** A board can list one job many times — once per location — so several postings describe identical work. Classifying each independently is wasted model time and a correctness problem: the 2026-09-16 run classified Harvey's five identical "GTM Technology Product Owner" postings in different chunks and got three different seniorities for one job.
- **Why text equality is the strong signal.** Two byte-identical descriptions, within one company, are self-evidencing — no judgement required. That catches the shape that matters most: 442 byte-identical text groups covering 1,253 postings, 149 of them spanning multiple requisition keys (the one-posting-per-location fan-out).
- **The requisition-key job-family problem, and why it's gated.** An earlier version of this skill treated a shared ATS requisition key alone as a merge signal — a correctness bug that shipped and mis-merged real jobs on 2026-09-17. At Greenhouse and Ashby the requisition key is a job-family / opening-group identifier, not a per-posting identity: 159 of 271 multi-posting requisition-key groups (59%, 481 postings) span more than one title. Merging on it blindly wrote one classification across four different Stripe Staff SWE roles and three different PMM roles — worse than the inconsistency dedup exists to prevent, because it's silent: three jobs get one answer instead of one job getting three visibly different ones. The Go core's fix is narrower than "ignore requisition key": it merges on requisition key *only when the normalized title also matches*, as an additional edge alongside text equality, closed transitively (if A and B share text, and B and C share a gated requisition key, A, B, and C are one unit).
- **Title alone is never a signal.** Same title with different text is usually a genuinely different job — this holds in the Go core exactly as it held here before.

To preview a selection before committing to it, call `enrichment_preview` directly ahead of the run — it's the same tool the orchestrator uses for real, so there's no separate preview path to keep in sync.

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

**Chunk:** assign each agent a set of **work units** (Step 3), never a bare list of posting IDs. A work unit is atomic — its representative and siblings always travel together, because splitting one across chunks is precisely how the same job gets two different answers.

The chunk amortizes an agent's fixed cost — tool loading — across several work units instead of paying it once each. **The taxonomy fetch is not part of that fixed cost, because it never reliably completed as written.** The MCP query tool caps every query at 1000 rows (`rowCap`, `apps/tools/cmd/mcp/main.go:34`) and sets `truncated: true` on any response it cuts off — a flag nothing in this skill ever told an agent to check. `skills` alone passed 1000 rows on 2026-09-13 and is now ~2,260: a plain `SELECT slug, name FROM skills` silently returns under half the table. **Never run that query wholesale.** Step 6 pages it and checks `truncated` on every page.

Because the fetch was already incomplete, any framing that treats "amortize the taxonomy fetch" as the reason to prefer one chunk size over another rests on an operation that wasn't returning complete results in the first place. **Do not hardcode the taxonomy's size either.** It grows every run and any number written down goes stale: this section once claimed "~700 rows" while the live count was near 2,800, which made small chunks look roughly four times cheaper than they were. Check it when sizing a run via `mcp__market-scout-postgres__query` (loaded through ToolSearch same as any other run), which returns one row and isn't subject to the cap:

```sql
SELECT (SELECT count(*) FROM canonical_roles)
     + (SELECT count(*) FROM specializations)
     + (SELECT count(*) FROM skills) AS taxonomy_rows;
```

Trade-off, stated explicitly:
- Larger chunks: fewer, bigger agents — less wall-clock parallelism for the same total work — and more risk of context drift/anchoring across many units from the same company.
- Smaller chunks: more parallelism, but each agent still pays the same per-agent taxonomy-load overhead (now paginated rather than silently incomplete), and that overhead scales with the taxonomy's size regardless of chunk size.

**Chunk size does not drive taxonomy minting — measured, not assumed.** A controlled A/B ran chunk sizes that were near-identical in practice (mean 11.8 postings/agent vs. 12.2), while a separate, unrelated change dropped new-slug minting 18x between the two runs. Near-identical chunk sizes cannot produce an 18x swing if chunk size were the driver. Do not size chunks to control minting — that lever doesn't exist. Chunk size stays open on the question it actually bears on: context drift and wall-clock time.

**Chunk size is not a fixed default — it's an open experiment.** The ceiling where per-agent context drift or task-quality degradation kicks in is unknown; it needs to be found empirically rather than asserted. Start experiments at **15** and be willing to push to **50**, watching for: classification quality drift late in a chunk (spot-check postings near the end of a large chunk against ones near the start), retry/error rates climbing, and whether an agent begins to anchor on a company's earlier postings when classifying later ones from a different company in the same chunk. Whatever the operator (a human or an orchestrating agent) picks per run, based on what a given run is optimizing for — reserve small chunks for when parallelism/wall-clock matters more, larger chunks for pure token-efficiency runs over the large and growing backlog.

Spread same-company **work units** across different chunks where possible, to limit anchoring — this matters more, not less, as chunk size grows. Two exceptions, in priority order:

1. **A work unit is never split.** Its postings are one job and one classification; this is not negotiable against anchoring.
2. **When a company has ≥3 postings selected in the run, keep them together** in one chunk — this is what lets Step 4's boilerplate stripping find a corpus to work with.

Spread everything else (singles and pairs) as usual.

Agent config per call:
- `subagent_type`: `general-purpose`
- `model`: `haiku`
- `description`: `Enrich chunk <first_id>…` (or list the IDs)

**Cross-wave taxonomy visibility.** Agents in one wave each query the taxonomy once, at chunk start, so parallel agents cannot see each other's newly-minted slugs — a pre-existing property of parallel dispatch, not a new risk from chunking. Taxonomy created mid-wave becomes visible starting the *next* wave, when fresh agents query. Durability is unaffected by chunk size: `save_enrichment` commits per posting, so an interrupted chunk leaves reached postings committed and only the unreached ones re-select later.

**One edge case worth knowing:** because commits are per posting, an interruption *mid-work-unit* can leave some siblings written and others not. The unwritten ones re-select on the next run as their own unit and get judged independently — the split-group problem, arriving by a different route. It is rare and self-limiting (a unit is usually 2–5 postings saved back to back), and the fix if it bites is a `--force` re-run over the affected unit, which appends a fresh consistent row to every sibling. Do not try to prevent it by batching saves; per-posting commits are what make an interrupted run safe in the first place.

**Known gotcha — concurrent near-duplicate minting.** Migration `000026` wrapped `mcp.save_enrichment` in a transaction-scoped advisory lock, so every enrichment save across every agent now serializes globally. That lock closes one specific race: two agents can no longer insert the *same* slug into *different* taxonomy tables. It does **not** stop two agents independently minting *different* slugs for one concept, because each still loads the taxonomy once at chunk start and cannot see a sibling's work. That is the failure mode actually observed on 2026-09-16 — `campaign-execution` and `marketing-campaign-execution` minted for the same idea in a single chunk, and `soc2-compliance` alongside a pre-existing `soc-2`.

Two consequences worth holding:

- **Parallelism buys less than it looks like.** Reads and classification parallelize; writes do not. Wave size is still worth keeping at 10 for wall-clock on the read side, but do not expect write throughput to scale with it.
- **Prompt rules are a weak lever here.** A 2026-09-16 run gave ten agents identical hardened instructions and identical chunk sizes; new-slug counts ranged from 0 to 49. Domain density drove minting far more than wording did. Prefer enforcement at the tool boundary over another paragraph of discipline — and when adding a rule, plan to measure whether it changed anything.

A **midpoint taxonomy refresh** is the one mitigation that demonstrably works: have each agent re-run the taxonomy and collision queries once, halfway through its chunk, so the back half can see slugs minted by siblings earlier in the same wave. Agents confirmed observing sibling slugs appear in that refresh. It costs one extra taxonomy load per agent (paginated, per Step 6), which is real and rising — another reason not to over-shrink chunks.

### 6. Agent contract

Each agent receives a **chunk of work units** and the pinned `PROMPT_VERSION` / `MODEL` (from the `classification-pins` block, read once at invocation start). It is self-contained and tool-using.

A **work unit** is one representative posting ID plus zero or more sibling posting IDs that describe the same job (Step 3). The agent reads and classifies the representative **once**, then writes that single classification to every posting in the unit. It never re-reads or re-judges a sibling — that is the whole point of the grouping, and re-judging is what produced three different seniorities for one Harvey job on 2026-09-16.

**Setup (once per chunk):**

1. Load `mcp__market-scout-postgres__query` and `mcp__market-scout-postgres__save_enrichment` via ToolSearch.
2. Load the taxonomy. **The MCP query tool caps every query at 1000 rows (`rowCap`, `apps/tools/cmd/mcp/main.go:34`) and sets `truncated: true` on any response it cuts off.** `skills` passed that cap on 2026-09-13 and is now ~2,260 rows — never run `SELECT slug, name FROM skills` wholesale; it silently returns under half the table. Page it, and check `truncated` on every response (`skills` included — the cap applies to any table that grows past it):
   ```sql
   SELECT slug, name FROM canonical_roles   ORDER BY slug;
   SELECT slug, name FROM specializations   ORDER BY slug;
   SELECT slug, name FROM role_dimensions   ORDER BY slug;
   SELECT slug, name FROM skills ORDER BY slug LIMIT 1000;
   SELECT slug, name FROM skills ORDER BY slug LIMIT 1000 OFFSET 1000;
   -- step OFFSET by 1000 and keep going until a page comes back truncated: false
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

**Per work unit in the chunk (loop):**

1. Fetch the **representative** posting's latest snapshot text by ID (unless boilerplate stripping already supplied `cleaned_text` for it above). Siblings are never fetched:
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
2. Classify per the discipline in Step 7 — **once**, from the representative's text.
3. Write back with `save_enrichment`, **once per posting in the work unit** (representative first, then each sibling), passing the identical classification payload each time and changing only `posting_id`. Saving is cheap; reading and judging is the cost, and that happens once.
4. Move to the next work unit.

**Writeback — call `save_enrichment` directly.** The tool handles slug validation, string escaping, null-byte rejection, cross-table slug uniqueness, and the transaction atomically. This is the tool boundary: the agent never interpolates SQL or escapes strings for writeback — that is the MCP server's concern.

Call with:

| Param | Value |
|---|---|
| `posting_id` | the posting being written — the representative, then each sibling in turn |
| `classification` | `{seniority, notes}` |
| `canonical_roles` | `[{slug, name, dimensions}]` |
| `specializations` | `[{slug, name}]` |
| `skills` | `[{slug, name, requirement}]` |
| `summary` | the structured summary string |
| `provenance` | `{model: <MODEL>, prompt_version: <PROMPT_VERSION>}` — **required, pass explicitly** |

**Provenance is required.** `save_enrichment` rejects a call that omits `provenance.model` or `provenance.prompt_version` with `invalid_provenance`. It used to substitute `model: "mcp-agent"` / `prompt_version: "mcp-save-enrichment-v1"` instead, which quietly fabricated the cohort key the `classification-pins` block exists to carry — 231 rows still hold that pair and nothing can recover what actually wrote them. Pass the pinned `MODEL` and `PROMPT_VERSION` on every call.

`summary` and `skills[].requirement` are echoed but not persisted (pgvector storage and requirement columns are deferred per `project.md` non-goals). Keep emitting them so the signal lands when storage arrives; report `summary` back for the markdown report.

**On `ok:false` (e.g. `slug_collision`):** read the error and pick a real fix — a distinct non-colliding slug, or drop the field if the concept is already covered elsewhere. Never blindly rename without checking the axis rule (Step 7). Retry the same posting. **Cap: 2 additional attempts (3 total).** If still unresolved, report that posting as failed in the chunk summary rather than looping.

**`name_collision` — a mint whose name is already taken.** Migration `000038` blocks minting a **new** `specializations` or `skills` slug whose name, normalized for case and whitespace, already belongs to a row in that table. Reusing an existing slug is never blocked, and `canonical_roles` is not checked at all. The error names the collider so you can act without searching:

```json
{"path": "skills[0].name", "code": "name_collision", "table": "skills",
 "proposed_slug": "llm-zzz-novel", "proposed_name": "Large Language Models",
 "existing_slug": "llm", "existing_name": "Large Language Models",
 "slug_similarity": 0.286, "name_similarity": 1.000}
```

Exactly two responses are correct. Pick on the concept, not on the scores:

1. **Reuse `existing_slug`** when it is the same concept. Replace your proposed slug and name with the existing pair and re-save. This is the common case — a low `slug_similarity` means the vocabulary already spells the concept differently, not that it is a different concept.
2. **Rename** when it is genuinely a different concept. Give the new term a name that states the distinction in the name itself, and keep your slug. A name that only differs by punctuation or a trailing word you added to get past the gate is not a distinction — it recreates the duplicate the check exists to stop.

Never mint under a near-miss name to slip the check. The gate blocks rather than substitutes precisely so nothing is guessed: if neither response is defensible, report the posting as failed and say which collider you hit.


If a fix changes the payload, **re-save the whole work unit with the corrected payload**, including siblings already written. A work unit whose siblings disagree is the defect this design exists to prevent; an extra append-only row per sibling is the cheap price of avoiding it.

**Agent's final report (back to orchestrator):** for its chunk, return per-work-unit outcome (classified / failed + reason) naming the representative and every sibling written, any new slugs it minted (role/specialization/skill slug + name), and each work unit's `summary` text. The orchestrator aggregates these — it does not re-read taxonomy.

Summaries are **not** persisted (storage deferred per `project.md` non-goals), so a summary omitted from this report is lost. Three agents dropped summaries on 2026-09-16; the report is the only record.

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
- Canonical roles name the job function only. Strip these modifier types from the role slug — they belong in `seniority` or `notes`:
  - Seniority titles: Director, Head, Lead, VP, Senior, Junior, Staff, Principal
  - Relationship/deployment context: Partner, Deployed, Embedded, Federated
  - Scope qualifiers: Global, Regional, Federal, APAC, LATAM
  - Specialization-as-suffix: Enablement, Strategy, Operations (when modifying a role, not naming one)

  Examples: "Staff Software Engineer" → `software-engineer`, seniority: `staff`. "Director of Enterprise Sales" → `account-executive`, seniority: `director`. "Partner Deployed Engineer" → `software-engineer`. "GTM Enablement" → `account-executive` + specialization, not a new role slug.

  A modifier in the slug splits one population across two dimensions — role and seniority then disagree about the same postings. Migration `000025` folded `staff-engineer` back into `software-engineer` for exactly this reason.
- Pick the table by axis, not by how domain-specific a term sounds:

  | Table | Axis | Examples |
  |---|---|---|
  | `specializations` | Domain, industry, product area | `fintech`, `retail-commerce`, `ai-agents` |
  | `skills` | Concrete technology, tool, framework, language, or competency | `spring-boot`, `react`, `people-management` |

  A framework is a skill, never a specialization — `spring-boot` is backend-only and not a stand-in for `full-stack-engineering`, which describes working across both layers, not any one framework.

**Grounding discipline (skills, specializations, and seniority):**
- Tag only what the description names or clearly implies — not what the role typically needs. Before adding an item, find the phrase that supports it. No phrase, no tag.
- An empty or short list is a correct result, not a failure. A posting that says "designing and implementing a robust, scalable data platform" with no tools named gets no tool skills — not `aws, azure, gcp, snowflake, databricks, bigquery, kafka, spark, airflow, dbt, java, scala, python` inferred from what data-platform roles typically use.
- **Ground seniority in scope of influence, not in years.** Work the ladder below in order and stop at the first step that resolves. Years of experience is **corroboration, never a basis** — see the years rule below.

  **Step 1 — a level word acting as a level in the title.** Authoritative. Intern, Junior, Senior, Staff, Principal, Lead, Director, Head, VP modifying the role is the employer's own designation. It is the same word slug discipline strips from the role slug, and it has to land somewhere; `seniority` is where.
- **Read the phrase, not the keyword.** The word only counts when it functions as a level. "Chief of Staff", "Staff Accountant", and "Member of Technical Staff" carry no staff level — a Staff Accountant is an entry-to-mid accounting role.
- **A band needs a separator; adjacent level words do not form one.** With an explicit `/`, `+`, `or`, or `to` — "Senior/Staff Engineer", "Staff + Sr. Engineer", "Mid-Senior" — the employer means either level, so take the **lower** bound. Two level words sitting adjacent with no separator are a single compound level, coined by the employer as its own rung — not "senior or staff." Abstract rules for resolving compounds have failed twice in one day; resolve by this table instead:

  | Compound title | Resolved level |
  |---|---|
  | senior staff | staff |
  | senior principal | principal |
  | senior director | director |
  | senior manager | senior |
  | staff principal | principal |
  | lead staff | staff |
  | principal architect | principal |
  | senior lead | lead |
  | associate director | director |
  | deputy director | director |
  | head of \<function\> | director |
  | vp of \<function\> | director |
  | general manager | director |

  Match on the words regardless of casing, spacing, or hyphenation — "Senior Staff", "Sr. Staff", and "senior-staff" are the same form. A compound not on this table is genuinely ambiguous: use judgment and record the call in `notes` rather than silently picking a side. On 2026-09-17 an agent read "Senior Staff Software Engineer" as a *band* and filed it `senior` — the only posting in a 62-posting corrective pass that stayed wrong. A follow-up pass applying this table fixed 59 of 60 genuine candidates with zero regressions: enumeration works, description does not.

  **Step 2 — the scope the description asks the person to hold.** When the title carries no level word, read what the job is accountable for. This is the axis that actually separates rungs: junior and mid postings are written about *craft* — doing the work well, to a defined bar, inside a scope someone else set. Senior and above are written about *alignment* — setting the direction, and bringing other people to it.

  | What the description asks the person to do | Rung |
  |---|---|
  | Learn the craft; work is assigned, scoped, and reviewed by others | `junior` |
  | Own well-scoped features or accounts end to end, where **someone else set the scope**; execute a roadmap handed to you; craft quality is the stated bar | `mid` |
  | **Define the scope yourself** — an ambiguous area, a function, a territory, a 0-to-1 build — and **align other teams or functions** to it; set direction others follow; mentor or set standards | `senior` |

  **"End-to-end ownership" appears on both rows and therefore decides nothing.** Almost every posting above `junior` claims it. Two questions separate the rungs, and both must be asked:
  1. **Who set the scope?** Handed a defined problem → `mid`. Expected to define the problem → `senior`.
  2. **Who must be aligned?** Delivery inside one team → `mid`. **Named other functions or teams *at the employer*** that must be brought along — Finance, Legal, Product, leadership, another engineering org — → `senior`. **Customer, partner, and other external contact is neutral: it describes the role, not the rung.** A solutions engineer, forward-deployed engineer, account executive, or implementation consultant talks to customer engineers and executives at every level including the most junior, so "collaborate daily with our customer's engineers and executives" answers nothing. Two agents in the 2026-09-21 correction pass read external-facing language in opposite directions — one counted it as cross-org alignment and filed `senior`, one counted it as no answer and filed `mid` — because this line did not say which. It says now.

  **If neither question can be answered from a verbatim phrase, fall through to Step 3 and write `unknown`.** Do not default to `mid` because it sits in the middle. A rung asserted from weak evidence is worse than an honest gap: the gap is visible to every later query and recoverable by a re-run, while a wrong rung is indistinguishable from a right one and silently corrupts any distribution built on it. `mid` is an answer, not a shrug.

  On 2026-09-21 two agents in the same correction pass read near-identical "own X end to end" phrasing and split — one filed `mid`, one filed `senior` — because the earlier version of this table let that phrase match both rows. Ask the two questions; do not resolve on the phrase.
  | Set technical direction across several teams or the org, as an IC | `staff` / `principal` — only when the title or description names that rung |
  | Own a function, an org, or a P&L; "Head of X"; reports to an exec | `director` |

  **Step 3 — nothing in Step 1 or Step 2 resolves → `unknown`.** This is a correct, common answer, not a cop-out. Most postings that carry no level word and no scope language genuinely do not state a level.

- **People management is a second axis, not a row on the first.** Read it alongside Step 2, not instead of it. Managing individual contributors puts the floor at `senior`. Managing managers, or owning a whole function or geography — "Head of X", "VP of X", General Manager — is `director` regardless of what the IC ladder would say. A posting with moderate scope language *and* people management takes the management answer: the gap that produced contradictory calls on two General Manager postings in the 2026-09-21 run was exactly this case going unstated.

- **"Head of X" is `director`.** Not `senior`. Four postings in the 2026-09-21 run — Head of Growth Marketing, Head of Field & Executive Marketing, Head of Developer Growth Marketing, General Manager (Italy) — were each filed `senior` because the agent read the function as mid-scope and ignored the title. Head of a function is the function's owner.

- **The years rule.** A years figure may *confirm* a rung Step 1 or Step 2 already produced. It may never *produce* one. "5+ years of enterprise sales experience" on a title with no level word is `unknown`, not `senior` — plenty of employers ask five years for a mid role, and the number varies by function, geography, and company stage. Six postings in the 2026-09-21 run were filed `senior` or `mid` on a years figure alone (Growth Marketer CEE, Growth Marketer DACH, Deal Desk and Pricing Expert, ML Framework Engineer, SWE Observability, SWE Collective Communication); every one should have been `unknown`. If your grounding phrase contains the words "without explicit years requirement", or any other statement that the posting *lacks* a signal, you have just written down that you have no basis — the answer is `unknown`.

- **Never read seniority off the subject matter, the company, or the team.** Not how advanced the domain sounds — "1-3 years of building with LLMs in a production environment" is `junior`, production LLM work notwithstanding. Not the company's stage or size. Not team adjectives: "a small, gritty team" is not a level signal, and neither is "you'll wear many hats". Not compensation: a band is set by market and geography, and reading a rung off a salary number is inventing a signal the posting did not give.

- **Do not read a level out of an inclusive-sourcing blurb.** "Experience can come from student clubs or side projects", "we encourage applicants who don't meet every qualification", and similar phrasing are hiring-funnel language, not statements about the rung. This exact ElevenLabs phrase produced two wrong `junior` calls in the 2026-09-21 run.

- **These are one rule, not two.** "Not the subject matter" never means discard an explicit level word. An agent on 2026-09-16 read it the strict way, dropped the only unambiguous signal its posting had, and filed a Staff Engineer as `mid`.

- **Write the grounding phrase into `classification.notes`,** prefixed `seniority: `, so the evidence is persisted next to the value rather than living only in a chunk report that may never be saved. Eight of 72 agents in the 2026-09-21 run dropped their reports entirely; the reasoning behind those calls is unrecoverable.

- **Every non-`unknown` seniority needs a `grounding_phrase`: text copied verbatim from the title or the description.** Not your paraphrase of it, not your inference from it. "Senior Staff Product Designer" is a grounding phrase; "small, gritty team indicates mid-level responsibility" is not — that is your gloss, and a gloss cannot be checked. If you cannot find a phrase in the posting that carries the level, the level is `unknown`.

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

- Run params (count, focus, force, backlog, per_company)
- Counts: work units selected, postings selected (incl. siblings), siblings riding along beyond `count`, dispatched, enriched, failed, skipped-no-description, re-enriched (when force)
- New-slug counts taken from `created_at >= <run start>` on the three taxonomy tables, not from agent reports
- Failure breakdown by reason (from agent reports — e.g. `slug_collision unresolved after retries`)
- New taxonomy minted this run (slug + name for roles; slug + name for specializations and skills), deduped across agents — two agents in a wave may propose the same new slug
- **Recurring failures.** Read `agent-output/batch-enrich/failures.jsonl` (tolerate a missing or empty file — render `(none)`, do not error). Group by `posting_id`, count distinct `run_timestamp` values. Render only postings with ≥ 2 failed runs: `posting_id`, failed-run count, and the set of distinct `reason` strings across all runs. Postings that have since classified still appear — the log is append-only and records no successes.
  - TODO: add a rotation strategy for `failures.jsonl` — scanned in full every run with no size bound.
- Per-posting summaries (`posting_id`, title, summary text)

Note in the report that failed postings are not written and will be re-selected on the next non-`--force` run.

After writing, print a one-line summary to the user: counts + report path.

#### Vocabulary health

The taxonomy went 18x on mint rate in one week with no alarm, because nothing measured it. These three measures close that gap. Compute all three from the database via `mcp__market-scout-postgres__query`, not from agent self-reports — the same bias the "New taxonomy minted this run" bullet above already names (agents claim a slug as new that they in fact reused) applies here too.

**1. Mint rate** — new taxonomy rows created during the run, divided by postings classified during the run. Single-row aggregate; not subject to the 1000-row cap (Step 5), because it returns exactly one row. Bind `<run start>` to the same invocation-start timestamp `failures.jsonl` uses:

```sql
SELECT
  (SELECT count(*) FROM canonical_roles WHERE created_at >= '<run start>')
  + (SELECT count(*) FROM specializations WHERE created_at >= '<run start>')
  + (SELECT count(*) FROM skills WHERE created_at >= '<run start>') AS new_taxonomy_rows,
  (SELECT count(*) FROM classifications WHERE classified_at >= '<run start>') AS postings_classified,
  round(
    ( (SELECT count(*) FROM canonical_roles WHERE created_at >= '<run start>')
    + (SELECT count(*) FROM specializations WHERE created_at >= '<run start>')
    + (SELECT count(*) FROM skills WHERE created_at >= '<run start>') )::numeric
    / nullif((SELECT count(*) FROM classifications WHERE classified_at >= '<run start>'), 0)
  , 3) AS mint_rate;
```

Report the result against a **0.30 reporting line**. Above it: the run is growing vocabulary about as fast as it is classifying against it — a taxonomy that cannot see itself. Below it: most postings are landing on vocabulary the run already knows. 0.30 sits in a band with nothing observed inside it — steady-state runs measured 0.00–0.22, the current regime measures 0.37–1.38 — so a value on either side of the line places the run in one regime or the other, not on a boundary.

**2. Single-use rate of this run's mints** — of the slugs *this run* created, the share attached to exactly one posting. Also a single-row aggregate:

```sql
WITH new_slugs AS (
  SELECT 'canonical_roles' AS tbl, id, slug FROM canonical_roles WHERE created_at >= '<run start>'
  UNION ALL
  SELECT 'specializations', id, slug FROM specializations WHERE created_at >= '<run start>'
  UNION ALL
  SELECT 'skills', id, slug FROM skills WHERE created_at >= '<run start>'
),
usage AS (
  SELECT ns.tbl, ns.slug, count(DISTINCT c.job_posting_id) AS posting_count
  FROM new_slugs ns
  LEFT JOIN job_posting_roles jpr ON ns.tbl = 'canonical_roles' AND jpr.role_id = ns.id
  LEFT JOIN job_posting_specializations jps ON ns.tbl = 'specializations' AND jps.specialization_id = ns.id
  LEFT JOIN job_posting_skills jsk ON ns.tbl = 'skills' AND jsk.skill_id = ns.id
  LEFT JOIN classifications c ON c.id = coalesce(jpr.classification_id, jps.classification_id, jsk.classification_id)
  GROUP BY ns.tbl, ns.slug
)
SELECT
  count(*) AS mints_this_run,
  count(*) FILTER (WHERE posting_count = 1) AS single_use_mints,
  round(count(*) FILTER (WHERE posting_count = 1)::numeric / nullif(count(*), 0), 3) AS single_use_rate
FROM usage;
```

This is only meaningful once the run has finished — a slug minted late in the run hasn't had the same chance to be reused as one minted early, so mid-run it reads more single-use than it will end up being. A high value means the run invented vocabulary nobody else will ever match: 43% of all skills are used exactly once, and among skills minted on 2026-09-16/17 it's roughly 68%. A skill used once is not vocabulary.

**3. Advisory-ignored count** — how many mints happened despite `save_enrichment` returning a `similarity_candidates` entry for that slug: the agent minted anyway, after the tool told it a near match already existed. Migration `000034` computes `similarity_candidates`; the writeback envelope (Step 6) returns it per call, per payload. **It is returned but not persisted — there is no audit table for it**, so this count cannot be queried and must be aggregated from agent reports this run instead (an agent that minted a slug also named in its own `similarity_candidates` response for that call reports the collision in its chunk summary). It needs no threshold: an ignored advisory is either justified — the near match really is a different concept — or it isn't, and a human can read forty of them. Persisting `similarity_candidates` to a table is a follow-up, not this skill's job.

**Report only.** All three measures report; none of them gate, fail, or reject a run, and this skill adds no threshold that does. Do not "improve" this into an enforcement mechanism — a rejected run has no rollback path once written, for reasons load-bearing enough to state plainly:
- `save_enrichment` writes per payload, but mint rate is a run-level aggregate. By the time the rate is computable, the rows it's counting already exist committed — there is nothing left to roll back.
- At payload grain the numerator is 0–5 new rows. That's noise, not a metric; a threshold needs the run-level aggregate to mean anything.
- The 2026-05-15 cold-start run (first run into an empty table) measured 1.08 — it would have failed any fixed threshold that also passes steady-state runs. A gate would need a second knob sized to corpus emptiness just to let that run through, and a second knob is how a gate meant to close one problem breeds another.

**Baseline, pre-fix** (2026-09-17, before any mint-rate fix has shipped — this instrumentation only measures, it doesn't fix) — for judging whether a future run's numbers are improving:

| Run hour | Classifications | New skills | Mint rate | Skills/posting |
|---|---|---|---|---|
| 17:00 | 161 | 120 | 0.745 | 5.39 |
| 18:00 | 102 | 8 | 0.078 | 7.13 |

(Skills-only counts, matching the incident record; the mint-rate query above sums all three taxonomy tables per the existing `<run start>` convention, so a live run's number will differ slightly from this table's — 0.789 and 0.088 summing all three tables over the same two hours, confirmed against the live database.)

Minting fell between these two hours while tagging (skills/posting) rose — the signature of reuse (pulling existing skills onto more postings) rather than restraint (tagging fewer skills overall). Flag it, don't conclude from it: it's one hour of data, and cohort homogeneity — a run that happens to land on a narrow, already-well-covered slice of postings — produces the same shape with no change in behavior at all.

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
- **Selection only, via `enrichment_preview`.** The orchestrator never loads taxonomy or collision data, and never writes its own selection SQL — that algorithm lives once, in the Go core, and the orchestrator stops rather than falls back to local SQL if the tool is unreachable.
- **Dedup before dispatch.** One job is read once, judged once, written to each of its postings. Consistency comes from not asking twice, not from asking more carefully.
- **Wave size is 10, dispatched in one message.** Serial `Agent` calls defeat the purpose.
- **Provenance is explicit.** Every `save_enrichment` call passes the pinned `MODEL` / `PROMPT_VERSION`, or cohort tracking is silently lost.
- **New taxonomy is a signal.** Surface every new slug so the user catches taxonomy drift early. Report it from `created_at`, not from agent self-reports — agents routinely claim slugs as new that they in fact reused.
- **Prefer structure over prose.** When a defect can be closed at the tool or schema boundary, close it there. Instruction text is the weakest available lever and its effect must be measured, not assumed.
- **Fail loudly, cap retries.** Log every ultimate failure with posting ID and reason; agents retry `ok:false` at most twice before giving up.
- **`--force` is additive, not destructive.** `save_enrichment` inserts a new classification row per posting; prior rows remain as history.
- **No schema changes.** Summary storage and `requirement` persistence are follow-ups, not this skill's job.
