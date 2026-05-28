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
- `research/` — stays at repo root (source/research data, passed to tools by CLI arg, not a hardcoded path).

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
- [ ] No file references the old module path `github.com/majtom2grndctrl/market-scout` (grep across the repo returns nothing outside historical plan docs).
- [ ] `go build ./...` and `go vet ./...` succeed when run from `apps/tools/`.
- [ ] `go test ./...` passes from `apps/tools/` (unit; integration/e2e tags if a DB is available).
- [ ] `sqlc generate` from `apps/tools/` produces no diff against checked-in generated code.
- [ ] Each binary runs from `apps/tools/` and resolves env + relative paths without error: `go run ./cmd/migrate up`, `./cmd/fetcher`, `./cmd/onboard`, `./cmd/batch-enrich`, `./cmd/strip-boilerplate`. Seed file (`internal/db/seeds/companies.sql`) and `agent-output/` resolve under `apps/tools/`.
- [ ] Moved files retain history: `git log --follow` on a moved file shows pre-move commits.
- [ ] `docker compose up` from repo root still starts Postgres (compose file and root `.env.local` unchanged).
- [ ] `agent-context/lib/` and `CLAUDE.md` carry no stale paths: real-path references to `cmd/…` and `internal/…` are updated to `apps/tools/…`, and dev-command examples show the `cd apps/tools` convention.
- [ ] `.gitignore` Go-artifact and `agent-output` anchors point under `apps/tools/`.

## Tasks

### Task 1: Relocate the Go module

`git mv` `cmd/`, `internal/`, `go.mod`, `go.sum`, and `sqlc.yaml` into `apps/tools/`. Use `git mv` (not plain `mv`) so history follows. This creates `apps/`.

### Task 2: Rewrite the module path

`go mod edit -module github.com/majtom2grndctrl/market-scout/apps/tools` on the moved `go.mod`. Then prefix-swap the import string across the 20 Go files (29 occurrences): `github.com/majtom2grndctrl/market-scout` → `github.com/majtom2grndctrl/market-scout/apps/tools`. The string appears nowhere else in the repo, so it is a safe blind swap. Run `gofmt -w` / `goimports` after.

### Task 3: Wire env and artifact paths

Create `apps/tools/.env.local` as a symlink to `../../.env.local`. Confirm the two CWD-relative literals resolve correctly when run from `apps/tools/`: the seed path (`cmd/onboard/main.go`) and the failures-log path (`cmd/batch-enrich/report.go`) need no edit — the directories they name move with the module or are created on write under `apps/tools/`.

### Task 4: Update root config

Update `.gitignore`: repoint the Go build-artifact anchors (`/bin/`, `/cmd/fetcher/fetcher`, `/fetcher`, `/migrate`, `/strip-boilerplate`, `/batch-enrich`) and `/agent-output/` to their `apps/tools/` locations. Confirm `docker-compose.yml` needs no change (it reads root `.env.local`).

### Task 5: Update docs

Update `agent-context/lib/` (`index.md`, `project.md`, `developer-guide.md`, `testing-guide.md`, `watchlist.md`, `style-guide.md`) and `CLAUDE.md`: real-path references to `cmd/…` and `internal/…` become `apps/tools/…`; dev-command examples gain the `cd apps/tools` step. Leave the durable architecture *decisions* (no Go API server, no ORM) untouched — only paths change.

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

## Open questions

- **`.env.local`: symlink vs. second file.** Recommending a symlink (`apps/tools/.env.local` → `../../.env.local`) for a single source of truth; both are gitignored and created at local setup. A standalone second file risks DB-credential drift. Confirm at implementation.
- **`research/` long-term home.** Stays at repo root for now. If a `data/` or top-level grouping emerges later, revisit — out of scope here.
