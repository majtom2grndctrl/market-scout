---
name: batch-enrich
description: Classify a bounded cohort of Market Scout job postings through the project MCP server. Use GPT-5.6 Luna workers for normal chunks and GPT-5.6 Terra only for unresolved saves or a read-only quality sample.
---

# Batch Enrich

Enrich job postings into canonical roles, specializations, skills, and a structured summary. This is the interactive direct-MCP workflow. Coordinate selection, packing, dispatch, and reporting. Workers read their assigned postings and live taxonomy, then save through the constrained MCP action.

Do not use `cmd/batch-enrich`, direct database access, shell SQL, or another write path.

```classification-pins
PROMPT_VERSION=batch-enrich-v8
LUNA_MODEL=gpt-5.6-luna
TERRA_MODEL=gpt-5.6-terra
```

This block is the only place these values are written. Read it once at
invocation start and substitute it wherever this file shows `<PROMPT_VERSION>`,
`<LUNA_MODEL>`, or `<TERRA_MODEL>`. Never copy a version string out of an
example below — a restated literal is how the pin and the rows drifted apart
before.

`PROMPT_VERSION` names the **classifier contract**, not the model and not the
harness: `model` is its own column, and the same contract runs under Luna and
Terra here and under Haiku in `.claude/skills/batch-enrich/SKILL.md`. Bump it in
the same commit as any change to classification discipline, grounding rules, the
worker contract, or selection semantics — and bump it in both skills together,
since both implement the one contract. An unbumped pin is not a paperwork gap:
it merges two cohorts into one string, and nothing later can separate them.

## Roles

| Role | Owns |
|---|---|
| Coordinator | Argument parsing, MCP preflight, cohort selection, company-aware packing, waves, Terra escalation, report, failure history, read-only audit dispatch. |
| Luna worker | One bounded chunk. Reads its assigned postings and fresh taxonomy, strips eligible company groups, classifies sequentially, saves, and reports structured outcomes. |
| Terra worker | One still-unsaved posting after Luna exhausts its retries, or a post-run read-only quality sample. |

The coordinator does not read posting descriptions, taxonomy, or collision state. It may use preview titles and company IDs to pack work. Workers never write the report or failure history.

## Arguments

Accept `<count> [focus] [--force] [--backlog] [--per-company N]`.

- Strip exact, whitespace-delimited `--force`, `--backlog`, and `--per-company <N>` tokens first. A word such as `forceful` remains focus text. `--per-company` consumes the following token as its integer value.
- The first remaining token is `count`. If it is absent, non-numeric, zero, or negative, use `10`.
- Reject a count above `500`; do not silently cap it.
- Join remaining words as `focus`. It may be empty.
- `--force` reclassifies selected postings. Saves remain append-only; never delete or update old classifications.
- Selection is newest-first. `--backlog` flips it to oldest-first, for a deliberate backlog drain only.
- `--per-company N` caps how many work units one company contributes to the wave. Omit it and the server applies its default of 5. Values outside 1..500 are rejected by the server, not silently capped — drop a malformed value and omit the param instead of passing it.

Oldest-first drained the queue in arrival order, which structurally guaranteed the classified set lagged the market: on 2026-09-21 only 4 of 1,946 classified postings had been first seen in the preceding three weeks, against 1,108 of 2,712 unclassified ones. The per-company cap exists for the matching reason on the other axis: the corpus is ~43% OpenAI/Stripe/Anthropic, and before the cap a 50-unit wave drew 58 postings from OpenAI alone. Both defaults are deliberate under-sampling of the giants and the backlog, so title analysis reflects the current market across the watchlist.

## Preflight and cohort

1. Confirm the `market-scout-postgres` MCP server exposes `enrichment_preview`, `query`, `strip_boilerplate`, `taxonomy_search`, and `save_enrichment`. Stop if any is unavailable. Do not bypass the MCP action boundary.
2. Call `enrichment_preview` once with these exact arguments:

   ```json
   {
     "count": 25,
     "focus": "<focus>",
     "force": false
   }
   ```

   Use the normalized count and flags: `force` is the parsed boolean. Add `"sort": "oldest_first"` only for `--backlog`; omitting `sort` resolves to newest-first. Add `"max_per_company": <N>` only when `--per-company` was given; omitting it applies the server default of 5.
3. Require `ok: true`. Use the returned ordered `postings` list as the complete cohort. Each row supplies `posting_id`, `company_id`, `company_name`, and `title`. Do not write selection SQL or select a second cohort.
4. If `selected_count` is zero, report that no eligible postings matched and stop. The shared selector already requires a latest non-null description; do not invent a skipped-no-description count that preview does not return.

Selection semantics are server-owned: focus filters title and description; normal runs select unclassified postings; force runs include already-classified postings; sort is by first-seen time in the requested direction, then posting ID so a tied cutoff is stable; and the wave cycles across companies before recency under the per-company cap, so one large board cannot consume it.

Postings whose latest snapshot has no description are invisible to selection rather than skipped by it — the server filters them before selecting. Workday (~120 postings) and Workable (~30) currently store none; that is a fetcher gap, not a classification failure, and does not belong in the report.

## Pack and dispatch

A **chunk** is one worker's sequential list of posting IDs. A **wave** is a concurrently dispatched set of chunks. Dispatch at most 10 workers in a wave and wait for every worker report before starting the next wave.

Start with a ceiling of 15 postings per chunk.

- Group the preview cohort by `company_id`.
- Co-locate an at-most-15-posting company group when it has three or more selected postings, so the worker can strip shared boilerplate, or when titles indicate the postings need a shared taxonomy decision.
- Spread other singleton and pair groups across chunks when practical. This limits within-worker anchoring.
- Split an over-ceiling company group into chunks of at most 15. Schedule its chunks in separate waves, never as sibling chunks in one wave. A later chunk must read the taxonomy written by the earlier chunk.
- Do not split a company into sibling chunks merely to fill a wave. Same-wave workers load taxonomy independently and can mint divergent slugs.
- Pass each worker only its chunk's posting IDs, company IDs, titles, focus guidance, and the literal Luna provenance values. Do not pass descriptions, taxonomy, or another chunk's results.

Delegate each normal chunk to `gpt-5.6-luna`. Workers process assigned IDs sequentially. A terminated run is safe: each successful save is append-only, and unreached postings are eligible for a later run.

## Luna worker contract

The worker may use only `query`, `strip_boilerplate`, `taxonomy_search`, and `save_enrichment` from the project MCP server. It must not inspect or save a posting outside its assigned IDs.

### Setup

At chunk start, load fresh `canonical_roles`, `specializations`, and `role_dimensions` wholesale, plus the live cross-table collision list, with `query`. Run each statement as a separate tool call:

```sql
SELECT slug, name FROM canonical_roles ORDER BY slug;
SELECT slug, name FROM specializations ORDER BY slug;
SELECT slug, name FROM role_dimensions ORDER BY slug;
SELECT slug FROM (
  SELECT slug FROM canonical_roles
  UNION ALL SELECT slug FROM specializations
  UNION ALL SELECT slug FROM skills
) AS taxonomy_slugs
GROUP BY slug
HAVING count(*) > 1;
```

These four reads stay wholesale because they stay small: `canonical_roles`, `specializations`, and `role_dimensions` are nowhere near the MCP `query` tool's 1000-row cap, and the collision query returns only the slugs that collide, not the tables behind them, so it stays tiny no matter how large `skills` grows.

Do not load `skills` this way. `SELECT slug, name FROM skills ORDER BY slug` passed that same 1000-row cap on 2026-09-13 and `skills` now holds ~2,260 rows — the call comes back `truncated: true`, silently missing every slug alphabetically after the cutoff, with nothing in the response forcing a worker to notice. Look up `skills` concepts with `taxonomy_search` instead, at the point you're about to tag them — see Classification discipline below.

For every company subgroup of three or more assigned postings, call `strip_boilerplate` once before the posting loop:

```json
{
  "company_id": 123,
  "selected_ids": [456, 457, 458]
}
```

Use returned `cleaned_text` for those posting IDs. The tool derives repeated text from that company's complete latest-description corpus, but returns text only for the selected IDs. If it returns `ok: false`, report the affected IDs as failures; do not substitute another database read or a shell command. Use raw latest text for assigned company groups smaller than three.

For raw-text postings, query only the assigned ID's latest title and description:

```sql
SELECT s.title, s.description_text
FROM job_postings AS jp
JOIN LATERAL (
  SELECT title, description_text
  FROM posting_snapshots
  WHERE job_posting_id = jp.id
  ORDER BY fetched_at DESC
  LIMIT 1
) AS s ON true
WHERE jp.id = <assigned_posting_id>;
```

If this query returns no row, or its `description_text` is null or empty, return that posting as `failed` with reason `raw_text_unavailable`; do not classify it and do not call `save_enrichment`. The coordinator records it as an ultimate unsaved failure unless a later worker run provides a successful persisted save.

### Classification discipline

- Treat `canonical_roles` as an array. Blended roles may have multiple roles.
- Always emit `classification.seniority`: `intern`, `junior`, `mid`, `senior`, `staff`, `principal`, `lead`, `director`, or `unknown`. Use `unknown` when evidence is insufficient.
- `notes` is optional. Omit it or use `null` when there are none.
- Every canonical role has a non-empty `dimensions` array drawn only from the live `role_dimensions` set. Never mint a dimension.
- Prefer an existing taxonomy slug. New slugs are ASCII kebab-case and stable across reruns. Avoid every live cross-table collision slug.
- Before tagging a skill, call `taxonomy_search` with `tables: ["skills"]` for the concepts you're about to use — pass the concept as you'd name it ("continuous integration"), not a substring, and batch up to 10 concepts from the posting in one call. `canonical_roles` and `specializations` already loaded wholesale in Setup; search those in-context instead of calling the tool for them.
- Read the response as clusters, one per concept: a `representative` entry with its `variants` nested underneath. Attach the representative's slug and name, never a variant alongside it — a representative and its variant are one skill already in the table, not two, and tagging both is the duplication the clustering exists to prevent.
- A cluster `score` near 1.0 means the table already names your concept: reuse the representative. A score below roughly 0.5 is related vocabulary, not a match — it tells you the concept probably isn't in the table yet, not something to attach. No cluster clears the tool's 0.3 floor: mint a new slug. When two candidates score close, prefer the one with the higher `usage_count` — more classifications already point at it.
- Canonical roles name only the job function. Put seniority, deployment relationship, geographic scope, and specialization suffixes in seniority, notes, or specializations. Examples: Staff Software Engineer is `software-engineer` plus `staff`; Partner Deployed Engineer is `software-engineer`; GTM Enablement is an account-executive role plus a specialization.
- Use `specializations` for domain, industry, or product area. Use `skills` for named technology, tool, framework, language, or competency. A framework is a skill, not a specialization.
- Ground skills, specializations, and seniority in the description. Do not infer typical tools from a role stereotype, or seniority from how advanced the subject matter sounds. Sparse tags are valid.
- Ground seniority in scope of influence, not in years. Work three steps in order and stop at the first that resolves.
  1. **A level word acting as a level in the title** — Intern, Junior, Senior, Staff, Principal, Lead, Director, Head, VP modifying the role — is authoritative. It is the word the role slug strips, so it lands in `seniority` rather than being discarded. Compounds resolve by table: senior staff→staff, senior principal→principal, senior director→director, senior manager→senior, staff principal→principal, lead staff→staff, principal architect→principal, senior lead→lead, associate director→director, deputy director→director, head of \<function\>→director, vp of \<function\>→director, general manager→director. A band needs an explicit separator (`/`, `+`, `or`, `to`); take its lower bound.
  2. **The scope the description asks the person to hold.** Junior and mid postings are written about craft — doing assigned, scoped work to a defined bar. Senior and above are written about alignment — setting direction and bringing others to it. Assigned and reviewed work → `junior`; owning well-scoped features or accounts against someone else's roadmap → `mid`; owning an ambiguous problem area, setting direction, aligning stakeholders across teams → `senior`; setting technical direction across teams as an IC → `staff`/`principal`, only when named; owning a function, org, or P&L → `director`.
  3. Nothing resolves → `unknown`. A correct and common answer.
- People management is a second axis, not a row on the first. Managing ICs puts the floor at `senior`; managing managers or owning a function or geography ("Head of X", "VP of X", General Manager) is `director`.
- **Years of experience is corroboration, never a basis.** A years figure may confirm a rung steps 1–2 already produced; it may never produce one. "5+ years" on a title with no level word is `unknown`, not `senior`.
- Never read a level off subject matter, company stage, team adjectives ("a small, gritty team"), compensation, or inclusive-sourcing blurbs ("experience can come from student clubs or side projects").
- Every non-`unknown` seniority needs a grounding phrase copied verbatim from the title or description — not a paraphrase or inference. Write it into `classification.notes` as `seniority: "<verbatim phrase>"`. No verbatim phrase carrying the level → `unknown`.
- The word must function as a level, not sit inside a compound noun: "Chief of Staff", "Staff Accountant", and "Member of Technical Staff" carry no staff level.
- A band needs an explicit separator (`/`, `+`, `or`, `to`): "Senior/Staff Engineer", "Mid-Senior" — take the lower bound. Two adjacent level words are one compound level and resolve to the more senior component: "Senior Staff" is `staff`, "Senior Director" is `director`, "Senior Principal" is `principal`.
- Write a 100–200 token summary: role, seniority, required and preferred skills, domain, and role type. Avoid marketing and company framing. The tool echoes it for the report but does not persist it.

### Save and retry

For each posting, call `save_enrichment` with the complete classifier payload and explicit provenance — the worker's actual model and the pinned `PROMPT_VERSION`. `save_enrichment` rejects a call that omits either; it no longer substitutes a default, because a manufactured provenance value cannot be told apart from a real one:

```json
{
  "posting_id": 456,
  "provenance": {
    "model": "<LUNA_MODEL>",
    "prompt_version": "<PROMPT_VERSION>"
  },
  "classification": {
    "seniority": "senior",
    "notes": null
  },
  "canonical_roles": [
    {
      "slug": "software-engineer",
      "name": "Software Engineer",
      "dimensions": ["engineering"]
    }
  ],
  "specializations": [],
  "skills": [
    {
      "slug": "go",
      "name": "Go",
      "requirement": "required"
    }
  ],
  "summary": "<100-200 token grounded summary>"
}
```

`save_enrichment` is the only write path. It validates payloads, preserves classification history, and returns `ok: false` with structured errors rather than a transport failure.

On `ok: false`, read the errors, refresh taxonomy and collision state when relevant, and make a real correction. Do not blindly rename a slug or change its table axis. Retry at most twice after the first failed call: three attempts total. If no save succeeds, return the posting as `needs_terra`; do not keep looping and do not write a failure file.

**`name_collision`.** Migration `000038` blocks minting a **new** `specializations` or `skills` slug whose name — normalized for case and whitespace — is already held by a row in that table. Reusing an existing slug is never blocked; `canonical_roles` is not checked. The error carries `table`, `proposed_slug`, `proposed_name`, `existing_slug`, `existing_name`, `slug_similarity` and `name_similarity`, so the collider is named and needs no lookup.

Two responses, and only these two. Choose on the concept, not on the scores:

- **Reuse `existing_slug`** when it is the same concept — swap in the existing slug and name, re-save. A low `slug_similarity` means the vocabulary spells the concept differently, not that it differs.
- **Rename** when the concept is genuinely new — state the distinction in the name and keep your slug. Adding punctuation or a filler word to clear the gate is not a distinction; it recreates the duplicate.

The gate blocks instead of substituting so that nothing is guessed. If neither response is defensible, return the posting as `needs_terra` and name the collider in the reason.

Return a structured chunk report to the coordinator:

```text
posting_id, title, outcome (saved | needs_terra | failed), attempt count,
reason when not saved, summary when saved, actual model, and every entry in
save_enrichment.new_taxonomy.
```

## Terra escalation and audit

For each `needs_terra` result, delegate a separate Terra task with only the still-unsaved posting ID, focus guidance, and these provenance values, substituted from the pins block:

```text
model=<TERRA_MODEL>
prompt_version=<PROMPT_VERSION>
```

Terra re-reads that ID, fresh taxonomy, and collision state through MCP, then follows the same classification and save contract. It must not receive or reclassify a successfully saved Luna posting. If Terra cannot save it, return one ultimate failure with its reason; do not cascade to another model.

After all saves and escalations, Terra may review a small sample with `query`. This audit is read-only: it must not call `save_enrichment`, repair data, or alter taxonomy. Report its findings separately as follow-up candidates.

## Coordinator reconciliation, report, and failure history

Before dispatch, capture an ISO-8601 `run_started_at` and a unique `run_id` of the form `YYYY-MM-DD-HHMMSS-<six-random-hex>`. The unique suffix prevents two same-second invocations from overwriting a report.

After every Luna or Terra wave has finished or a worker has been confirmed terminated, reconcile the wave's dispatched IDs with persisted results before escalation or failure reporting. Use one read-only `query` call with the literal IDs from that wave and the invocation start time:

```sql
SELECT DISTINCT ON (job_posting_id)
  job_posting_id, model, prompt_version, classified_at
FROM classifications
WHERE job_posting_id = ANY(ARRAY[<dispatched_posting_ids>]::bigint[])
  AND prompt_version = '<PROMPT_VERSION>'
  AND classified_at >= '<run_started_at>'::timestamptz
ORDER BY job_posting_id, classified_at DESC, id DESC;
```

- A returned row is the source of truth that the posting saved in this invocation, even if its worker report was lost. Record a recovered persisted save with the returned model; omit a summary if the worker did not report one.
- A worker-reported `saved` result without a matching row is unsaved and becomes `needs_terra` for a Luna wave or an ultimate failure for a Terra wave.
- A reported `needs_terra` result without a matching row is eligible for its one Terra escalation.
- An ID with neither a report nor a matching row is an ultimate `worker_exited_without_report` failure. Do not guess a classification or write a replacement result.

After all Luna and Terra reports and reconciliations, the coordinator alone creates `agent-output/batch-enrich/` if needed and writes `agent-output/batch-enrich/<run_id>.md`.

The report includes:

- normalized count, focus, `force`, `backlog`, and `per_company`;
- selected, dispatched, saved, Luna-escalated, Terra-saved, failed, and re-enriched counts;
- failure breakdown and a note that unsaved postings reselect on a later non-force run;
- new roles, specializations, and skills, deduplicated by slug and name;
- per-posting saved summaries with ID and title;
- the read-only Terra audit, if run;
- recurring failures from `agent-output/batch-enrich/failures.jsonl`.

Then append one JSON object for every ultimate unsaved posting to `agent-output/batch-enrich/failures.jsonl`. Never truncate or let workers write it. Each line has:

```json
{
  "run_timestamp": "<run_id>",
  "posting_id": 456,
  "reason": "<short final reason>",
  "prompt_version": "<PROMPT_VERSION>",
  "model": "<the model that actually attempted the save>",
  "attempted_at": "<ISO-8601 report-collection time>"
}
```

Read the existing JSONL tolerantly. Group rows by `posting_id` and distinct `run_timestamp`; report only postings with two or more failed runs, including their failed-run count and distinct reasons. Old rows remain history even if a posting later succeeds.

Finish with a one-line result: saved and failed counts, plus report path.

## Rules

- Preflight the five required MCP tools. Stop if the server or action boundary is unavailable.
- Use preview's complete ordered `postings` response; never copy selection SQL.
- Pass the pinned `PROMPT_VERSION` and the actual saving model on every `save_enrichment` call. Substitute from the pins block; never reuse a version string copied from an example.
- Luna is the normal bounded worker. Terra is only an isolated unsaved-posting escalation or a read-only audit.
- Keep company chunks from racing in one wave. Fresh taxonomy becomes visible to later waves, not sibling workers.
- Save once per posting through MCP. Classifications and coordinator failure history are append-only.
- Reconcile each dispatched wave against persisted classifications; a worker message alone never proves a save.
- The coordinator reports; workers classify and return facts. Neither role improvises a second persistence path.
