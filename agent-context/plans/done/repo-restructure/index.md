# Repo Restructure: apps/ Layout

## Goal

Relocate the Go module into `apps/tools/` and establish an `apps/` directory so the coming Next.js app (`apps/web/`) sits as a sibling. Purely structural — no behavior changes, no reversed architecture decisions. Establishes a home for both the existing Go tools and the web app without coupling them.

## Scope

### In scope

- Move the Go module (`cmd/`, `internal/`, `go.mod`, `go.sum`, `sqlc.yaml`) under `apps/tools/`.
- Rewrite the module path `github.com/majtom2grndctrl/market-scout` → `.../apps/tools` across all source and `go.mod`.
- Establish the working-directory convention: Go commands run from `apps/tools/`.
- Relocate local artifact output (`agent-output/`, build `bin/`) under `apps/tools/`.
- `.env.local` stays canonical at repo root (docker-compose reads it); `apps/tools/.env.local` is a symlink to it.
- Update root `.gitignore` path anchors.
- Update path references in `agent-context/lib/` and `CLAUDE.md`.

### Out of scope

- `apps/web/` Next.js scaffold — separate spec.
- MCP server — separate spec (`drafts/mcp-server`).
- Splitting into multiple Go modules or adopting `go.work`. Stays a single module.
- Renaming binaries, packages, or commands.
- Reworking env loading to be CWD-independent in code.
- `.claude/settings.local.json` permission entries (path-specific allow entries may stop matching after the move — harmless re-prompts, not repo state).
- `research/` — stays at repo root. It is a deliberate low-attention pocket: quasi-documentation that agents should not read unless explicitly pointed at it. Keeping it at root (not under `agent-context/`, which agents read by default, and not under `apps/`) preserves that separation. Referenced by tools via CLI arg, never a hardcoded path.

## What moves vs. stays

| Path | Destination | Notes |
|---|---|---|
| `cmd/`, `internal/`, `go.mod`, `go.sum`, `sqlc.yaml` | `apps/tools/` | The Go module root. `sqlc.yaml` paths are relative to itself — resolve unchanged after move. |
| `agent-output/` | `apps/tools/agent-output/` | `report.go` writes a CWD-relative path; running from `apps/tools/` lands it here. Gitignored. |
| build `bin/` | `apps/tools/bin/` | Gitignored build artifacts; regenerate in place. |
| `.env.local` | repo root (canonical) + `apps/tools/.env.local` symlink | docker-compose at root reads root copy; Go run from `apps/tools/` reads the symlink. Single source of truth. |
| `docker-compose.yml`, `.env.example` | repo root | Shared infra (Postgres serves both web and tools). Unchanged. |
| `research/`, `agent-context/`, `.claude/`, `CLAUDE.md` | repo root | Not module code, not web. |

## Acceptance criteria

- [ ] Go module root is `apps/tools/`; `go.mod` module path is `github.com/majtom2grndctrl/market-scout/apps/tools`.
- [ ] No file references the old module path `github.com/majtom2grndctrl/market-scout` (grep across the repo returns nothing outside `agent-context/plans/done/**` and the active sibling draft `agent-context/plans/drafts/mcp-server/`).
- [ ] `go build ./...` and `go vet ./...` succeed when run from `apps/tools/`.
- [ ] `go test ./...` passes from `apps/tools/` (unit; integration/e2e tags if a DB is available).
- [ ] `sqlc generate` from `apps/tools/` produces no diff against checked-in generated code.
- [ ] Each binary runs from `apps/tools/` and resolves env + relative paths without error: `go run ./cmd/migrate up`, `go run ./cmd/fetcher`, `go run ./cmd/onboard`, `go run ./cmd/batch-enrich`, `go run ./cmd/strip-boilerplate`. Seed file (`internal/db/seeds/companies.sql`) and `agent-output/` resolve under `apps/tools/`.
- [ ] Moved files retain history: `git log --follow` on a moved file shows pre-move commits.
- [ ] `docker compose up` from repo root still starts Postgres (compose file and root `.env.local` unchanged).
- [ ] `agent-context/lib/` and `CLAUDE.md` carry no stale paths: real-path references to `cmd/…` and `internal/…` are updated to `apps/tools/…`, and dev-command examples show the `cd apps/tools` convention.
- [ ] `agent-context/lib/developer-guide.md` documents the `ln -s ../../.env.local apps/tools/.env.local` step for fresh clones.
- [ ] `.gitignore` Go-artifact and `agent-output` anchors point under `apps/tools/`.

## Tasks

### Task 1: Relocate the Go module

`git mv` `cmd/`, `internal/`, `go.mod`, `go.sum`, and `sqlc.yaml` into `apps/tools/`. Use `git mv` (not plain `mv`) so history follows. This creates `apps/`.

### Task 2: Rewrite the module path

`go mod edit -module github.com/majtom2grndctrl/market-scout/apps/tools` on the moved `go.mod`. Then prefix-swap the import string across the 20 Go files (29 occurrences): `github.com/majtom2grndctrl/market-scout` → `github.com/majtom2grndctrl/market-scout/apps/tools`. The string appears nowhere else in the repo, so it is a safe blind swap. Restrict the rewrite to `*.go` files; `go.mod` is already handled by `go mod edit` and must not be sed-rewritten. Run `gofmt -w` / `goimports` after.

### Task 3: Wire env and artifact paths

Create `apps/tools/.env.local` as a symlink to `../../.env.local`. Confirm the two CWD-relative literals resolve correctly when run from `apps/tools/`: the seed path (`cmd/onboard/main.go`) and the failures-log path (`cmd/batch-enrich/report.go`) need no edit — both resolve correctly *because* the run-from-`apps/tools/` convention puts CWD at the new module root (the seed file moves with the module; the failures log is created on write).

### Task 4: Update root config

Update `.gitignore`: repoint the Go build-artifact anchors and `agent-output` entry to their new locations — `/bin/` → `/apps/tools/bin/`, `/cmd/fetcher/fetcher` → `/apps/tools/cmd/fetcher/fetcher`, `/fetcher` → `/apps/tools/fetcher`, `/migrate` → `/apps/tools/migrate`, `/strip-boilerplate` → `/apps/tools/strip-boilerplate`, `/batch-enrich` → `/apps/tools/batch-enrich`, `/agent-output/` → `/apps/tools/agent-output/`. Confirm `docker-compose.yml` needs no change (it reads root `.env.local`).

### Task 5: Update docs

Update `agent-context/lib/` (`index.md`, `project.md`, `developer-guide.md`, `testing-guide.md`, `watchlist.md`, `style-guide.md`) and `CLAUDE.md`: real-path references to `cmd/…` and `internal/…` become `apps/tools/…`; dev-command examples gain the `cd apps/tools` step. Leave the durable architecture *decisions* (no Go API server, no ORM) untouched — only paths change. Active sibling drafts under `agent-context/plans/drafts/` (e.g. `mcp-server/`) are out of scope — they update on their own promotion.

### Task 6: Verify

From `apps/tools/`: `go build ./...`, `go vet ./...`, `go test ./...`, `sqlc generate` (expect no diff). Run each binary far enough to confirm env load and path resolution. From repo root: `docker compose up` starts Postgres.

## Sequencing

**Phase 1 (sequential):** Task 1 — the physical move blocks everything downstream.
**Phase 2 (sequential):** Task 2 — import rewrite operates on the moved files.
**Phase 3 (concurrent):** Tasks 3, 4, 5 — independent edits to env/artifacts, root config, and docs.
**Phase 4 (sequential):** Task 6 — verification consumes all prior phases.

## Rough sketch

The whole change is a relocation plus a deterministic string swap. No logic changes.

```sh
# Phase 1
mkdir -p apps/tools
git mv cmd internal go.mod go.sum sqlc.yaml apps/tools/

# Phase 2
cd apps/tools
go mod edit -module github.com/majtom2grndctrl/market-scout/apps/tools
grep -rl 'github.com/majtom2grndctrl/market-scout' --include='*.go' . \
  | xargs sed -i '' 's#github.com/majtom2grndctrl/market-scout#&/apps/tools#g'
gofmt -w .
go build ./...
```

`//go:embed` directives (migrations, seeds) are relative to their `.go` file and move intact. `sqlc.yaml` paths are relative to the config file and resolve unchanged once the file sits at `apps/tools/sqlc.yaml` and `sqlc generate` runs from there.

## Decisions

- **`.env.local` uses a symlink.** `apps/tools/.env.local` → `../../.env.local`. The root copy is canonical (docker-compose reads it); the symlink gives the Go tools a single source of truth, avoiding DB-credential drift. Both are gitignored; the symlink is a documented local-setup step.
- **`research/` stays at repo root as a low-attention pocket.** It is not folded into `agent-context/` or `apps/`. Its role — quasi-docs that agents read only when explicitly directed — should be written into `agent-context/lib/` at promotion, since it is durable project intent not derivable from the folder's contents.
