# Codex-Native Batch Enrichment

## Goal

Make the maintained batch-enrichment workflow available in Codex. A Codex coordinator selects work and delegates bounded classification chunks to Luna workers, which use the project MCP server for reads and constrained enrichment writes.

The Claude skill remains the behavioral reference. The Go batch runner remains available as a legacy automation path, not the primary interactive workflow.

## Scope

### In scope

- Expose the enrichment preview and constrained enrichment-write MCP tools to Codex.
- Extend enrichment preview with the complete selected posting list required for bounded direct-MCP dispatch.
- Replace the minimal Codex `batch-enrich` skill with the maintained Claude workflow translated into Codex terms.
- Use GPT-5.6 Luna workers for normal classification chunks and reserve the coordinator's stronger model for unresolved collisions or quality review.
- Preserve the Claude workflow's selection, `--recent`, boilerplate, provenance, retry, failure-history, and same-company taxonomy-consistency rules.
- Serialize direct enrichment writes inside the existing database action boundary so concurrent workers cannot mint one slug in different taxonomy tables.
- Support a bounded cohort of up to 500 postings and reconcile each dispatched wave against persisted classifications.
- Update durable project guidance to identify the Codex skill as the interactive enrichment path and the Go runner as a separate automation path.

### Out of scope

- Remove or rewrite `cmd/batch-enrich`.
- Run a live enrichment batch or reclassify existing postings.
- Add a scheduler or model API integration.

## Acceptance criteria

- [ ] Codex can discover `enrichment_preview` and `save_enrichment` from the `market-scout-postgres` MCP server alongside the existing enabled tools.
- [ ] `enrichment_preview` returns up to 500 ordered posting/company records selected by the shared selection core, so the coordinator never reimplements selection SQL or dispatches a different cohort from its preview.
- [ ] The Codex `batch-enrich` skill accepts a count, optional focus, `--force`, and `--recent`, and describes the same selection semantics as the maintained Claude workflow.
- [ ] A normal worker receives only a bounded chunk and uses GPT-5.6 Luna with explicit `batch-enrich-v5` / `gpt-5.6-luna` provenance on every `save_enrichment` call.
- [ ] The skill requires the project MCP server for posting, taxonomy, and enrichment data access; it stops if the required tools are unavailable rather than bypassing the action boundary.
- [ ] Every direct-MCP classification write records `batch-enrich-v5` and the actual worker model; a Terra escalation receives only an unsaved failed posting, while post-run Terra audit reads only.
- [ ] The skill preserves the behavioral safeguards: fresh taxonomy and cross-table collision reads, per-company boilerplate stripping, capped retries, append-only failure history, explicit raw-text failure handling, and same-company handling that avoids same-wave taxonomy divergence.
- [ ] The coordinator alone writes the timestamped Markdown report and append-only failure JSONL after all worker reports arrive; workers never concurrently write either file.
- [ ] Every `mcp.save_enrichment` call takes one transaction-scoped advisory lock before the existing validation and write body, preventing concurrent cross-table taxonomy minting races while retaining the action-role-only external function signature.
- [ ] Selection has a deterministic posting-ID tie breaker in both oldest-first and newest-first modes, reports cannot overwrite another invocation begun in the same minute, and the coordinator reconciles each dispatched wave against persisted results before escalation or final failure reporting.
- [ ] Project guidance distinguishes the direct-MCP Codex workflow from the retained Go runner, and affected Go/MCP tests plus the project skill validation pass without a live enrichment run.

## Tasks

### Task 1: Expose the enrichment MCP surface

Update `.codex/config.toml` to enable `enrichment_preview`, `strip_boilerplate`, and `save_enrichment` for `market-scout-postgres`. Extract the existing command's full-company-corpus loading, selected-ID validation, and ordered cleaned-result mapping behind a small read-only loader seam in `internal/enrich/boilerplate`. Migrate `cmd/strip-boilerplate` to that seam, then add a narrow read-only `strip_boilerplate` MCP handler over the same seam. The tool accepts one company ID and one-to-100 unique selected posting IDs, derives spans from every latest description for that company, and returns cleaned text only for selected IDs. It binds the read-only pool and returns a structured error for duplicate, unknown, or cross-company IDs, so workers never receive a broad database DSN. Test that an unselected company sibling can supply the shared boilerplate evidence used to clean a selected posting. Extend the preview's success envelope with the complete ordered selected-record list, capped at 500 postings, while retaining its bounded sample for lightweight callers. The list must come from the shared selection result so a direct skill never copies selection SQL. Add an explicit `job_postings.id` final tie breaker to both selection variants. Keep the server's action-role boundary intact: the skill preflights `enrichment_preview`, `strip_boilerplate`, `query`, and `save_enrichment`, then stops if any is unavailable. Add unit coverage for the complete-list mapping and stripping handler; preserve existing preview/action tests.

### Task 2: Translate the canonical workflow into the Codex skill

Rewrite `.agents/skills/batch-enrich/SKILL.md` as the direct-MCP coordinator workflow. Preserve the maintained Claude skill's classification discipline and report behavior, but replace Claude-specific controls with Codex actions. Parse exact `--force` and `--recent` tokens, default an absent, non-numeric, zero, or negative count to 10, and reject values above 500. The coordinator preflights and obtains its whole cohort from `enrichment_preview`, then delegates bounded chunks; Luna workers read only their assigned posting data and live taxonomy through MCP, then save one append-only result per posting. Use `batch-enrich-v5` and `gpt-5.6-luna` as explicit provenance; the new pin separates direct per-posting MCP saves from the Go runner's v4 batched-response transport. Include `--recent`, failure retries, explicit no-row/null-description failure behavior, post-wave persistence reconciliation, and the new MCP boilerplate helper.

State the packing rule precisely. Start with a 15-posting chunk ceiling: co-locate an at-most-15 posting company group when it provides the three-posting boilerplate corpus or title evidence of a shared taxonomy decision; spread other singleton and pair groups to limit anchoring. Each worker calls the boilerplate MCP tool for a three-or-more posting company subgroup in its own assigned chunk; the handler derives common text from that company's complete latest-description corpus, so workers retain data ownership. Split an over-ceiling company group into ceiling-sized chunks scheduled in separate waves, never sibling waves, so each later chunk reads prior taxonomy writes. A Luna worker retries an `ok:false` response twice at most. After each wave, the coordinator reconciles every dispatched ID with classifications persisted by this invocation before deciding escalation or reporting an unreported worker exit. Escalate only a reconciled still-unsaved posting ID to Terra, which records `gpt-5.6-terra`; use Terra read-only for the post-run quality sample. Workers return structured per-posting outcomes, summaries, and newly minted taxonomy to the coordinator; only the coordinator writes the Markdown report, the append-only failure JSONL, and its recurring-failure section. Use a seconds-resolution report filename or collision suffix.

### Task 3: Serialize action writes and align durable guidance

Add a new numbered migration that preserves the external `mcp.save_enrichment(jsonb, text, text)` signature as a `SECURITY DEFINER` wrapper. The wrapper must take one fixed transaction-scoped advisory lock and then call the renamed hardened implementation. Revoke `PUBLIC` and `market_scout_actions` access to the implementation, leaving only the wrapper action-reachable; update `action_role.sql` to make reruns self-sufficient. Its down migration restores the original externally named hardened implementation and removes the wrapper. Add an environment-gated action integration test that proves an action-role call waits for the fixed lock. Then update the stale classifier ownership paragraph in `project.md` and matching cost/failure-history guidance in `developer-guide.md`.

### Task 4: Verify the workflow

Run `/Users/dhiester/.codex/skills/.system/skill-creator/scripts/quick_validate.py .agents/skills/batch-enrich`; the display metadata is unchanged, so do not add an `agents/openai.yaml` merely because the skill body changed. Forward-test the skill in a fresh non-writing task through its MCP preflight. Run the relevant MCP unit tests and inspect the final diff for accidental changes to the user's existing Claude workflow or unrelated worktree changes.

## Sequencing

**Phase 1 (sequential):** Task 1 — establishes the complete MCP contract consumed by the skill.

**Phase 2 (sequential):** Task 2 — names and preflights the completed MCP contract.

**Phase 3 (sequential):** Task 3 — makes concurrent action saves safe and aligns durable guidance.

**Phase 4 (sequential):** Task 4 — verifies the completed surface.

## Rough sketch

The MCP server already registers `enrichment_preview` on the read-only pool and `save_enrichment` across the read-only and action pools. This work changes the Codex allowlist, extends the read-only enrichment surface, and serializes the action function without expanding its action-role surface.

The coordinator calls `enrichment_preview` for both its preflight and its complete ordered cohort. The tool's count is capped at 500, matching the direct skill's count limit. Workers use `query` only for their assigned latest snapshots and taxonomy, and call `strip_boilerplate` only for their own three-or-more posting company subgroup. Each write passes the pinned provenance and records the worker actually performing it. Preserve the existing action function as the sole write path, now transaction-serialized before it validates or mints taxonomy.

The coordinator uses 15 postings as its initial chunk ceiling. It retains a small same-company group only when boilerplate stripping or shared taxonomy consistency needs it; an oversized group is serialized across waves. This avoids sibling workers minting incompatible taxonomy from stale per-wave reads without creating unbounded worker tasks.

## Boundary inventory

| Name | Skill argument / action | MCP tool parameter | Stored provenance |
|---|---|---|---|
| Posting identifier | `posting_id` | `posting_id` | `classifications.job_posting_id` |
| Prompt version | `batch-enrich-v5` | `provenance.prompt_version` | `classifications.prompt_version` |
| Worker model | `gpt-5.6-luna` or `gpt-5.6-terra` | `provenance.model` | `classifications.model` |
| Recency mode | `--recent` | `sort: "newest_first"` | report only |
| Full cohort | `count` (1–500) | `postings[]` | report only |

## Open questions

None. The workflow uses the existing MCP action boundary, with a transaction-scoped database lock for concurrent direct saves.
