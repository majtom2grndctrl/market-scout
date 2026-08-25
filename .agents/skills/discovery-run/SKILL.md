---
name: discovery-run
description: Turn a source into validated, fetcher-ready companies through Market Scout's MCP tools and a browser. Use to scout companies from an article, list, notes, URL, or explicit names.
---

# Discovery Run

Turn one source into fetcher candidates. The MCP tools own deduplication, ATS detection, and writes. The browser only gathers URL evidence.

## Start

Read `agent-context/lib/watchlist.md`. Read `agent-context/lib/project.md` only when ATS or fetcher boundaries are unclear.

Confirm access to `dedup_candidates`, `detect_ats`, `add_company`, and `record_unsupported_company`, plus a browser. If unavailable, stop. Do not invent database writes or edit seed files as a substitute.

## Workflow

1. Extract candidates. Retain source URL, homepage, careers page, and notes.
2. Deduplicate before browser work. Drop `duplicate`; hold `stale` for human review.
3. For new candidates, use `known_unsupported` as advisory history. Re-investigate when fresh evidence warrants it.
4. Investigate survivors one at a time in the browser. Collect homepage, careers page, redirects, and observed board URLs.
5. Give the observed URL evidence to `detect_ats`.
6. Record `unsupported-ats` and `no-careers` with `record_unsupported_company`. Do not record `invalid-token` or `ambiguous` outcomes.
7. Call `add_company` only for a supported ATS and matching board token. Its successful probe is the write gate.
8. After a successful insert, mirror the exact row in `apps/tools/internal/db/seeds/companies.sql`. Use `NULL` for a missing industry. Add a dated `INSERT ... ON CONFLICT (ats, board_token) DO NOTHING` block without reformatting unrelated entries.

## Rules

- Never treat a name-only match as a safe duplicate.
- Never add a board belonging to a parent, subsidiary, staffing firm, or unrelated company without confirmation.
- Never parallelize browser investigation. It is a shared surface.
- Seed data follows a successful database insert; it never leads it.
- Report unrelated seed or database drift. Do not repair it silently.

## Report

End with:

| Company | Status | ATS | Board token | Evidence |
|---|---|---|---|---|

Then name added companies and seed updates, review items, and MCP or browser failures. If none were added, explain why.
