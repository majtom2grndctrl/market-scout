---
name: discovery-run
description: >
  Turns a source into fetcher-ready companies. Extracts candidate companies from
  an article, list page, pasted notes, or explicit names; dedups them through the
  Market Scout MCP server; investigates survivors in a connected browser to find
  ATS job boards; detects supported ATS platforms from observed URL evidence; and
  adds validated companies to the fetch list. Use when onboarding many company
  leads at once from a single source, or scouting a named handful.
argument-hint: "<source url | company names | pasted notes>"
---

# Discovery Run

Turn one source into fetcher candidates. A source is an article, company list
page, pasted notes, a query prompt, or an explicit list of company names.

You coordinate four MCP tools and a browser. The MCP tools own dedup, detection,
and the write gate. The browser is only for gathering URL evidence — it never
writes anything.

## Start

Read `agent-context/lib/watchlist.md`. Read `agent-context/lib/project.md` only
when ATS or fetcher boundaries are unclear.

Confirm the Market Scout MCP server is connected. Required tools:

- `mcp__market-scout-postgres__dedup_candidates`
- `mcp__market-scout-postgres__detect_ats`
- `mcp__market-scout-postgres__add_company`
- `mcp__market-scout-postgres__record_unsupported_company`

If those are unavailable, stop and tell the user the MCP server must be configured
for this client before the skill can write to the fetch list. Do not invent DB
writes, or edit the seed file, as a substitute for a real `add_company` call.

Confirm a browser MCP is connected for step 6. Prefer **claude-in-chrome**
(`mcp__claude-in-chrome__*`); the **chrome-devtools** plugin also works. If neither
is connected, ask the user to connect one — do not guess ATS URLs from company
names alone.

## Workflow

1. Extract candidate companies from the source.
2. Keep lightweight provenance per candidate: source URL, observed homepage,
   careers page, and notes.
3. Call `dedup_candidates` before any browser work. Pass `name` for every
   candidate; add `ats` / `board_token` only when already known.
4. Drop `duplicate` results.
5. Set aside `stale` results for human merge or disambiguation review. Do not call
   `add_company` for a stale or name-only match unless the user explicitly asks.
6. For each `new` survivor, review `known_unsupported` before browser work. Weigh
   its `stale` flag and reason trust tier: `unsupported_ats` is Deterministic;
   `no_careers` is Agent-asserted prior browser judgment. This is advisory only —
   re-investigate when the agent judges fresh evidence is warranted.
7. Browser-investigate `new` survivors — **one at a time**, since the browser is a
   single shared surface. Find the homepage, careers page, any redirect target, and
   visible ATS job-board URLs. Inspect redirects, page links, scripts, and network
   requests for the board URL.
8. Call `detect_ats` with the observed URL evidence (`careers_url` plus
   `observed_urls`). It parses evidence only — it does not browse, probe, or write.
9. When `detect_ats` returns `unsupported-ats`, call
   `record_unsupported_company` before recording final status. Pass the candidate
   `name`, `reason: "unsupported_ats"`, and the observed careers or ATS URL as
   `url`. Omit `detected_platform` unless browser evidence independently identifies
   it.
10. When browser investigation concludes `no-careers`, call
   `record_unsupported_company` before recording final status. Pass the candidate
   `name`, `reason: "no_careers"`, and the observed homepage URL when available;
   otherwise omit `url`. Omit `detected_platform`.
11. Do not call `record_unsupported_company` for `invalid-token` or `ambiguous`
    outcomes.
12. Call `add_company` only when `detect_ats` returns a supported ATS and a board
    token that matches the company under investigation. Leave `probe` at its default
    (true).
13. Treat `add_company` probe success as the write gate. A failed probe inserts
   nothing — report it as unresolved.
14. After a successful `add_company`, append a matching seed entry to
    `apps/tools/internal/db/seeds/companies.sql` so a fresh database can
    reproduce the current fetch list. Mirror the inserted row exactly — `name`,
    `ats`, `board_token`, `industry` (use the SQL literal `NULL`, unquoted, when
    the DB row's industry is null; never invent one). Append a new
    `INSERT ... ON CONFLICT (ats, board_token) DO NOTHING;` block under a dated
    comment (`-- <Ats> (<context>, <Month Year>)`), following the file's existing
    per-batch grouping — don't rewrite or reformat unrelated existing entries.

## Safety rules

- Do not skip `dedup_candidates`.
- Do not treat name-only matches as safe duplicates. They are review items.
- Do not add a company from name similarity alone.
- Do not add unsupported ATS platforms.
- Call `record_unsupported_company` for `unsupported-ats` and `no-careers` so
  later runs do not repeat the same browser investigation.
- Do not add a board token when the careers page belongs to a parent, subsidiary,
  staffing firm, or unrelated company unless the user confirms the relationship.
- Do not parallelize browser investigation across subagents — one browser, one
  candidate at a time.
- Do not silently ignore ambiguous cases. Report them.
- Do not add a seed-file entry for a company `add_company` did not successfully
  insert. The seed file mirrors the DB; it never leads it.
- If an unrelated seed/DB mismatch surfaces while editing (a miscased token, a
  stale duplicate), report it — do not silently "fix" or delete rows outside the
  current candidate's scope.

## Candidate handling

Use this status set in the report:

| Status | Meaning |
|---|---|
| `added` | `add_company` accepted the company |
| `duplicate` | `dedup_candidates` found a token match with recent snapshots |
| `stale-needs-merge` | Existing DB match needs human review or refresh |
| `unsupported-ats` | Careers page uses an unsupported platform |
| `no-careers` | No usable careers page or board found |
| `invalid-token` | Supported ATS detected, but `add_company` rejected the probe |
| `ambiguous` | Evidence points to multiple possible companies or boards |
| `pending` | Investigation ended before a terminal result |

## Report

End with a concise table:

| Company | Status | ATS | Board token | Evidence |
|---|---|---|---|---|

Then list:

- Companies added, and whether the seed file was updated for each
- Review items
- Any MCP or browser tooling failures

If no companies were added, say so directly and explain whether the blockers were
duplicates, unsupported ATS, missing careers pages, or tool availability.

## Example prompts

```text
/discovery-run Find companies from this article and add valid ATS-backed ones to the fetch list:
<url>
```

```text
/discovery-run Scout these candidates: Modal, Baseten, Replicate, Railway, Neon. Dedup first, investigate survivors, add valid supported ATS boards.
```
