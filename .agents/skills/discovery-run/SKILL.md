---
name: discovery-run
description: Run browser-led Market Scout company discovery. Use when Codex should extract candidate companies from a source, dedup them through the project's MCP server, investigate survivors in a browser, detect supported ATS boards, and add validated companies to the fetch list.
---

# Discovery Run

Use this skill to turn a source into fetcher candidates. Sources can be an article, company list page, pasted notes, query prompt, or explicit list of company names.

## Start

Read:

- `agent-context/lib/watchlist.md`
- `agent-context/lib/project.md` only when ATS or fetcher boundaries are unclear

Confirm the Market Scout MCP server is available to the current client. Expected tools:

- `dedup_candidates`
- `detect_ats`
- `add_company`

If those tools are not available, explain that the MCP server must be configured for the client before this skill can write to the fetch list. Do not invent DB writes or edit seed files as a substitute.

## Workflow

1. Extract candidate companies from the source.
2. Keep lightweight provenance for each candidate: source URL, observed homepage, careers page, and notes.
3. Call `dedup_candidates` before browser investigation. Include `name` for every candidate and `ats` / `board_token` only when already known.
4. Drop `duplicate` results.
5. Set aside `stale` results for human merge or disambiguation review. Do not call `add_company` for a stale/name-only match unless the user explicitly asks.
6. Browser-investigate `new` candidates. Find the homepage, careers page, redirect target, and visible ATS job-board URLs.
7. Call `detect_ats` with observed URL evidence. The tool parses evidence; it does not browse.
8. Call `add_company` only when `detect_ats` returns a supported ATS and board token that matches the company being investigated.
9. Treat `add_company` probe success as the write gate. Failed probes insert nothing and should be reported as unresolved.

## Safety Rules

- Do not skip `dedup_candidates`.
- Do not treat name-only matches as safe duplicates. They are review items.
- Do not add a company from name similarity alone.
- Do not add unsupported ATS platforms.
- Do not add a board token when the careers page belongs to a parent, subsidiary, staffing firm, or unrelated company unless the user confirms the relationship.
- Do not silently ignore ambiguous cases. Report them.

## Candidate Handling

Use this status set in the final report:

| Status | Meaning |
|---|---|
| `added` | `add_company` accepted the company |
| `duplicate` | `dedup_candidates` found a token match with recent snapshots |
| `stale-needs-merge` | Existing DB match needs human review or refresh |
| `unsupported-ats` | Careers page uses an unsupported platform |
| `no-careers` | No usable careers page or board was found |
| `invalid-token` | Supported ATS was detected, but `add_company` rejected the probe |
| `ambiguous` | Evidence points to multiple possible companies or boards |
| `pending` | Investigation ended before a terminal result |

## Report

End with a concise table:

| Company | Status | ATS | Board token | Evidence |
|---|---|---|---|---|

Then list:

- Companies added
- Review items
- Any MCP/tooling failures

If no companies were added, say so directly and explain whether the blockers were duplicates, unsupported ATS, missing careers pages, or tool availability.

## Example Prompts

```text
[$discovery-run] Find companies from this article and add valid ATS-backed ones to the fetch list:
<url>
```

```text
[$discovery-run] Scout these candidates: Modal, Baseten, Replicate, Railway, Neon. Dedup first, investigate survivors, and add valid supported ATS boards.
```
