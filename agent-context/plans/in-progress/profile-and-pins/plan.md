# profile-and-pins — plan of record

mode: compact
status: active
read at: 2da46d6

Built in worktree `../market-scout-profile-and-pins` on `feat/profile-and-pins`, rebased onto `main` at 8bf6eac. Migrations and role scripts go to the shared `market_scout` and `market_scout_test` (owner decision, 2026-10-01).

## Corrections

- Brief Decisions table, "Posting links; `hybrid-classifier` candidate and seed rows, where present" → `hybrid-classifier` stopped (`plans/done/hybrid-classifier`); its tables never land. Planning around it by handling posting links only. Meaning unchanged: "where present" is now never.
- Brief Path, "`hybrid-classifier` has reserved migration 000044" → reservation void; both databases sit at 43, clean. This brief takes 000044.
- AC "Where `hybrid-classifier`'s tables exist…" → dropped by the owner (2026-10-01). The foreign-key census row covers any later taxonomy-referencing table.
- Source since 2bde04f: `main` changed the Go save handler and enrichment queries (batch-enrich v10), with no migration. `mcp.save_enrichment`'s retired-slug gate, `taxonomy_search.go`, `test-dsn.ts`, and both role scripts are unchanged. Decision reads stand.
- AC "A merge whose survivor is absent changes no row" → read as no taxonomy, link, dimension, profile, or retired-slug row. The repair log records the skipped pair, so the notice has a durable trace. Same meaning: no data the repair governs moves.

## Delegated answers

- **Schema `app`, three tables.** `app.past_titles` (title text, optional role, optional title seniority), `app.claimed_skills` (text, optional skill), `app.pins` (optional role, pinned-at name). Identity primary keys, `created_at`/`pinned_at` stamps.
- **Past titles order by entry** (`id`). No dates exist to order by, and the person reads them back in the order they typed them.
- **Claim text uniqueness** is a unique index on `lower(btrim(text))`; stored text is trimmed. `lower()` suffices: PG 17 has no `casefold()`.
- **Archive in `public`, keyed by a repair id.** `taxonomy_repairs` (one row per call: operation, table, label, `undone_at`), `taxonomy_repair_terms` (deleted or absent terms with id, name, created_at, survivor), `taxonomy_repair_links` (classification links with `collided`), `taxonomy_repair_role_dimensions` (with `collided`). `retired_slugs` gains a nullable `retired_by_repair` reference, so undo removes exactly its own records. 000037's `retired_slug_links` is not reused: its own down migration drops it.
- **Functions in `public`, owner-only.** `taxonomy_merge(table, map jsonb, retired_by text)`, `taxonomy_retire(table, terms jsonb, retired_by text)` return the repair id; `taxonomy_undo(repair_id)`. Invoker rights, EXECUTE revoked from PUBLIC in the migration. Each takes `save_enrichment`'s advisory lock `(734771, 26)`, then `FOR UPDATE` on the terms, so a concurrent save either lands before the move or waits behind it (P10). A non-locking writer is caught by the row lock: its FK check holds `FOR KEY SHARE` on the term.
- **Census guard in SQL.** The repair functions refuse to run while any foreign key references a taxonomy table outside their handled list. The handled list lives once, in the migration.
- **App role script** `setup/app_role.sql`, shaped like `readonly_role.sql`: guards, CONNECT, USAGE on `app`, explicit per-table grants, SELECT on `canonical_roles`, `skills`, `title_seniority_seeds`. No default privileges. DSNs `DATABASE_URL_APP`, `DATABASE_URL_TEST_APP`.
- **Unpin by pin id**, so a retired pin (no role) can be removed too.
- **Taxonomy search latency** — measured in task 4 against the development database before deciding on an index.

## AC-to-proof

All automated rows run under `pnpm test:db` against `market_scout_test`. A skip is not a pass.

| AC | Proof | Status |
|---|---|---|
| G1 App role CRUD on each profile table; SELECT on named taxonomy | `lib/db/profile-grants.db.test.ts` | achievable as stated |
| G2 App role refused writes outside `app`; refused EXECUTE on repair/undo | `profile-grants.db.test.ts` (catalog over every table, plus a real refused write) | achievable as stated |
| G3 Later `app` table grants nothing; no membership, owns nothing (P14) | `profile-grants.db.test.ts` (owner creates a table in a rolled-back tx) | achievable as stated |
| G4 Read-only role refused all on profile tables and EXECUTE on repair/undo | `profile-grants.db.test.ts` | achievable as stated |
| G5 Profile read via app role, taxonomy search via read-only, both return fixture rows | `lib/db/profile.db.test.ts` | achievable as stated |
| V1 No selectable view depends on `app` (catalog) | `profile-privacy.db.test.ts` | achievable as stated |
| V2 No executable function names `app` in its body | `profile-privacy.db.test.ts` | achievable as stated |
| C1 RESTRICT on role and skill delete | `profile-constraints.db.test.ts` | achievable as stated |
| C2 Pin uniqueness | `profile-constraints.db.test.ts` | achievable as stated |
| C3 Claimed-skill text and skill rules | `profile-constraints.db.test.ts` | achievable as stated |
| C4 Past-title seniority and text rules | `profile-constraints.db.test.ts` | achievable as stated |
| M1 Role merge moves link, dimension, past title, pin; deletes role; records slug | `taxonomy-repair.db.test.ts` | achievable as stated |
| M2 Both pinned / both claimed / only merged claimed | `taxonomy-repair.db.test.ts` | achievable as stated |
| M3 Specialization merge | `taxonomy-repair.db.test.ts` | achievable as stated |
| M4 Role retire and skill retire | `taxonomy-repair.db.test.ts` | achievable as stated |
| ~~M5 `hybrid-classifier` candidate and seed rows~~ | dropped by owner, 2026-10-01 | n/a |
| M6 `save_enrichment` refuses a retired slug after merge and retire | `taxonomy-repair.db.test.ts` (owner calls `mcp.save_enrichment`) | achievable as stated |
| M7 Absent term recorded, nothing else changes; absent survivor changes nothing (P12) | `taxonomy-repair.db.test.ts` | achievable as stated |
| M8 Chained map and self-merge refused | `taxonomy-repair.db.test.ts` | achievable as stated |
| M9 Save racing a merge (P10) | `taxonomy-repair.db.test.ts` (two connections, uncommitted save) | achievable as stated |
| M10 FK census lists and exercises every reference | `taxonomy-repair.db.test.ts` | achievable as stated |
| U1 Merge then undo vs snapshot; profile rows stay (P3) | `taxonomy-undo.db.test.ts` | achievable as stated |
| U2 Retire then undo (P6) | `taxonomy-undo.db.test.ts` | achievable as stated |
| U3 Merge, undo, merge, undo (P4) | `taxonomy-undo.db.test.ts` | achievable as stated |
| U4 Chain undone in reverse; out-of-order undo refused (P5) | `taxonomy-undo.db.test.ts` | achievable as stated |
| U5 Post-merge survivor link survives undo (P8) | `taxonomy-undo.db.test.ts` | achievable as stated |
| U6 Undo refused on re-mint; second undo no-op | `taxonomy-undo.db.test.ts` | achievable as stated |
| W1 Idempotent pin, unpin, claim | `profile.db.test.ts` | achievable as stated |
| W2 Concurrent double pin (P1) | `profile.db.test.ts` (two app connections) | achievable as stated |
| W3 Pin of a vanished role (P2) | `profile.db.test.ts` | achievable as stated |
| Manual: empty state on a fresh test install | owner, `pnpm dev` against test DSNs | manual |
| Manual: add skills, title, two pins; reload | owner | manual |
| Manual: unpin persists | owner | manual |
| Manual: submit disabled while pending | owner | manual |
| Manual: retired pin renders with pinned-at name | owner, after `taxonomy_retire` on a fixture role | manual |
| Manual: keyboard and focus | owner | manual |
| Manual: MCP `query` refused on a profile table | owner, MCP session | manual |
| Manual: parity on both databases (P13) | integrating executor runs, owner confirms | manual |
| Manual: `pnpm preflight` | integrating executor | manual |

## Tasks

| # | Task | Owner | Depends on | Status |
|---|---|---|---|---|
| 1 | **Riskiest slice.** Migration 000044: `app` schema and tables, archive, `retired_slugs.retired_by_repair`, merge/retire/undo functions with census guard. `setup/app_role.sql`. Apply to the test database. App test DSN. Grant, privacy, constraint, merge and retire tests (G1–G4, V1–V2, C1–C4, M1–M4, M6–M10). | integrating executor | — | |
| 2 | Undo tests (U1–U6); fix what they find. | integrating executor | 1 | |
| 3 | `sqlc generate`; `go build ./... && go vet ./...`; apply to the development database; parity check (P13). | integrating executor | 2 | |
| 4 | Web data layer: app client, profile read and write cores, taxonomy search on the read-only client, latency check. Tests G5, W1–W3; DB-free guard that no client module imports the app client. | integrating executor | 1 | |
| 5 | `/profile`: page, Server Actions, taxonomy combobox, nav entry, loading/empty/error states, stories. | integrating executor | 4 | |
| 6 | Docs: `developer-guide.md` §2 (app role, DSNs, parity), `web-testing-guide.md` (app test DSN), `project.md` repair-function contract check. Preflight, review loop, landing. | integrating executor | 3, 5 | |
