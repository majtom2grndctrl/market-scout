# Research Notes — Repo Restructure

Findings that grounded the spec. Not decisions; kept for the implementer's reference.

## Import-path blast radius

- Module path: `github.com/majtom2grndctrl/market-scout` (`go.mod:1`).
- Referenced in **20 Go files, 29 import occurrences**, plus `go.mod`.
- The string appears in **no** non-Go file except `go.mod` (checked `.yaml`, `.yml`, `.mod`). Safe to prefix-swap blind.

## CWD-relative path literals

Two literals resolve against the process working directory, not the module path:

- `cmd/onboard/main.go:39` — `defaultSeedPath = "internal/db/seeds/companies.sql"`. `seeds/` lives under `internal/`, so it moves with the module; resolves under `apps/tools/` with no edit when run from there.
- `cmd/batch-enrich/report.go:26` — `failuresFilePath = "agent-output/batch-enrich/failures.jsonl"`, created via `os.MkdirAll` at `report.go:395`. Running from `apps/tools/` writes it to `apps/tools/agent-output/`.

These two literals are why the working-directory convention (run from `apps/tools/`) was the pivotal decision: it keeps both edit-free.

## Env loading

All five binaries call `godotenv.Load(".env.local")`, CWD-relative, commented "no-op if absent; prod sets env vars directly":
`cmd/fetcher/main.go:58`, `cmd/onboard/main.go:55`, `cmd/strip-boilerplate/main.go:53`, `cmd/migrate/main.go:27`, `cmd/batch-enrich/select.go:65`.

Running from `apps/tools/` means each looks for `apps/tools/.env.local` → resolved by the symlink to the root canonical file.

## sqlc

`sqlc.yaml` (repo root) uses paths relative to itself: `queries: internal/db/queries`, `schema: internal/db/migrations`, `out: internal/db`. sqlc resolves these relative to the config-file directory, so moving `sqlc.yaml` into `apps/tools/` keeps them valid; run `sqlc generate` from `apps/tools/`.

## Things that do NOT need touching

- `docker-compose.yml` — reads `.env.local` relative to itself (repo root); stays put.
- `//go:embed` directives (migrations embed, seeds) — relative to their `.go` file; move intact.
- No Makefile, no shell build scripts, no README, no CI config (`.github/`) exist — nothing else to repoint.

## Doc references to repoint (Task 5)

Real-path mentions of `cmd/…` / `internal/…` found in: `agent-context/lib/index.md`, `project.md`, `developer-guide.md`, `testing-guide.md`, `watchlist.md`, `style-guide.md`, and `CLAUDE.md`. Most are durable boundary-package references (e.g. `internal/ats/<adapter>.go`) and dev-command examples (`go run ./cmd/fetcher`). Only the path prefix and the `cd apps/tools` step change — the architectural prose does not.

## .gitignore anchors to repoint (Task 4)

Root-anchored entries that break after the move: `/bin/`, `/cmd/fetcher/fetcher`, `/fetcher`, `/migrate`, `/strip-boilerplate`, `/batch-enrich`, `/agent-output/`. Node/Next.js ignores are already present (added speculatively) and need no change.
