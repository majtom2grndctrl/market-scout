# Migrate Recovery Verbs

## Goal

Add `force <version>` and `version` verbs to `cmd/migrate` so a stranded `dirty`
database can be recovered without hand-editing `schema_migrations`. Today a `down`
fails at the RESTRICT FKs in migrations 000007/000008 and leaves the DB `dirty`
with no command to clear it. `force` is the missing escape hatch; `version` shows
what to force to.

## Scope

### In scope

- Add a `version` verb: print current migration version and the dirty flag.
- Add a `force <version>` verb: clear the dirty flag by pinning the recorded version.
- Validate the `force` argument: reject non-integer and negative values before
  calling into the migrator.
- Handle the fresh-database case (`ErrNilVersion`) gracefully in the `version` verb,
  mirroring how `up`/`down` already special-case `ErrNoChange`.
- Keep the existing `[migrate]`-tagged structured `slog` logging on the new verbs.
- Document the teardown footgun and the recovery recipe in
  `agent-context/lib/developer-guide.md` §2.

### Out of scope

- Changing `up` or `down`. `down` stays a full teardown via `m.Down()`; the
  RESTRICT FKs remain the guardrail that blocks destroying enrichment provenance.
- Step counts (`up N`, `down N`), a `down all` verb, or partial migration.
- Confirmation prompts, typed acknowledgements, or `MIGRATE_CONFIRM`-style env gates.
- Bounds-checking the `force` version against the embedded migration set (an
  out-of-set version is the operator's responsibility; golang-migrate does not
  verify it either).
- Forcing to a pre-initial nil state (negative versions). Recovery always targets a
  real recorded version.

## Acceptance criteria

- [ ] `migrate version` prints the current version and dirty flag on a migrated DB.
- [ ] `migrate version` on a fresh DB reports the no-migrations-applied case
      rather than erroring opaquely.
- [ ] `migrate force <version>` clears the dirty flag: after a `down` strands the DB
      `dirty`, `force <version>` followed by `up` returns the DB to a clean,
      fully-migrated state.
- [ ] `migrate force` with a missing, non-integer, or negative argument exits
      non-zero with a clear usage error and does not call the migrator.
- [ ] `up` and `down` behavior is unchanged: `down` still reverts all migrations and
      still surfaces the RESTRICT-FK block at 000007/000008.
- [ ] New verbs log through the existing `[migrate]` `slog` tagging with structured
      key/values, not `fmt.Println`.
- [ ] The usage string lists all four verbs.
- [ ] `developer-guide.md` §2 documents the teardown footgun and the
      `version → resolve FK refs → force <N> → up` recovery recipe.
- [ ] From `apps/tools`: `go vet ./cmd/migrate` and `go build ./cmd/migrate` pass;
      `gofmt` clean.

## Tasks

### Task 1: Add `version` and `force` verbs

Extend `run()` in `apps/tools/cmd/migrate/main.go`. The current flow parses
`os.Args[1]` as a direction and switches on `up`/`down`; widen that to four verbs.

`version` calls `m.Version()`, which returns `(version uint, dirty bool, err error)`.
On `ErrNilVersion`, report no migrations applied and exit zero. Otherwise log the
version and dirty flag as structured `slog` key/values.

`force` checks `len(os.Args) < 3` and emits the usage error before indexing
`os.Args[2]`, then parses that arg as a non-negative integer, rejecting a
non-integer or negative value with a usage error before touching the migrator. Keep
the parsed value as `int` — what `m.Force(version int)` takes — independent of
`Version()`'s `uint` return. On a valid argument, call `m.Force(version)`: a nil
return is success, a non-nil error fails with a clear `[migrate]` message. `Force`
does not report `ErrNoChange`/`ErrNilVersion`, so the `force` path needs no fresh-DB
special-case.

Update the usage strings (the `len(os.Args) < 2` guard and the unknown-direction
error) to list `up|down|force|version`.

Exact log wording and `slog` key names are the implementer's choice. The contract is
structured key/values under the `[migrate]` tag, never `fmt.Println`; the fresh-DB
`version` message is illustrative, not a literal string contract.

### Task 2: Document the recovery recipe

In `agent-context/lib/developer-guide.md` §2 (Schema Migrations), add a short
subsection covering: `down` is a full teardown and intentionally blocks at the
RESTRICT FKs (000007/000008) when enrichment history exists, leaving the DB `dirty`.
Recovery: run `migrate version` to read the stuck version; manually delete the
blocking rows the down migrations' own comments name — the `canonical_role_dimensions`
rows for the stuck dimension (000007) or the `job_posting_roles` rows for the stuck
role (000008) — so the down `DELETE` can proceed; then `migrate force <N>` and
`migrate up`. Point operators to those down-migration comments for the exact `DELETE`
statements rather than duplicating them. Keep it durable prose — name the recovery
steps and the verbs, not internal function names.

## Sequencing

**Phase 1 (sequential):** Task 1 — adds the verbs the recipe references.
**Phase 2 (sequential):** Task 2 — documents the recipe against the shipped verbs.

## Rough sketch

`cmd/migrate/main.go` keeps its single `run()` function and one `switch`. The
migrator construction (`migrate.NewWithSourceInstance`) and `defer m.Close()` are
already shared and stay shared — `force` and `version` need the same `*migrate.Migrate`
instance `up`/`down` use. The `version`/`force` branches sit alongside the existing
`up`/`down` cases; argument parsing for `force` uses `strconv.Atoi` with an explicit
non-negative check. golang-migrate API (v4.19.1, confirmed): `Force(version int) error`,
`Version() (uint, bool, error)`, sentinel `migrate.ErrNilVersion`.

`strconv` is not currently imported in `main.go`; the `force` path adds it.
Throughout this spec, `cmd/migrate` is shorthand for `apps/tools/cmd/migrate`.

This stays a flat verb switch on purpose — no cobra/flag framework. Readability-first,
single-operator tool (developer-guide §1.3, §5.1).

## Open questions

None. The minimal surface is settled: two new read/recover verbs, `up`/`down`
untouched, no confirmation machinery.
