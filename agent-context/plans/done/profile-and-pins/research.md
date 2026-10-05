# profile-and-pins — research

Read at 2bde04f. Findings that inform the brief but do not decide it.

## Taxonomy repair mechanics

| Migration | Operation | What happens to references |
|---|---|---|
| 000025 | Merge a role (staff-engineer) | Join rows copied to survivor with `ON CONFLICT DO NOTHING`, old join rows deleted, role row deleted |
| 000029 | Retire skills and specializations, no survivor | Join rows deleted, term deleted |
| 000031 | `retired_slugs` table | Blocks re-minting a retired slug through `mcp.save_enrichment`. No survivor column; a merge's survivor appears only in `reason` prose |
| 000037 | Merge 64 skills and 7 specializations | Same merge pattern. Creates `retired_slug_links` (retired slug → survivor slug) as its own undo record; its down migration drops the table. Nothing else reads it |
| 000040 | Merge 3 roles and 2 skills | Same pattern. Reuses `retired_slug_links`; role-dimension tags go to `retired_role_dimensions` |

Repairs are hand-written, one per migration, each with a temporary merge map. No reusable function or Go tool exists. Later merges are expected: 000037 leaves pairs "a human can merge later".

Slugs are never renamed by any migration, but no constraint makes them immutable. `cmd/batch-enrich` writes taxonomy directly and bypasses the retired-slug gate (`project.md` §Settled architecture).

Retirement without a survivor is real, not hypothetical: `retired_slugs` rows include bundled slugs (`aws-gcp-azure`) and umbrella skills (`technical-expertise`) with no single successor. That is why a pin needs a retired state, not only a re-point.

## Why repair needs functions, not a rule

The repair migrations above name slugs from the owner's taxonomy, and every install runs them. On the test database and on a fresh install, the taxonomy holds only the two roles 000001 and 000008 seed, and the merge maps match nothing (000040's merge-map join simply finds no rows). A profile clause written by hand into each repair would therefore run against real rows only on the owner's development database. A fork whose owner pinned the merged role would hit the RESTRICT foreign key and leave a dirty migration. Moving every reference into one function means the profile clause is written and tested once, and the same statements run on every install.

The same fact is why a missing term changes only the retired-slug record: a repair written for one taxonomy must not fail on another, and the record stops the fork minting the duplicate later. 000037 and 000040 register retirements unconditionally for the same reason.

## Why undo leaves profile rows

`developer-guide.md` §Schema Migrations checks reversibility by running a migration's down, then its up, and diffing against the state before the down. Restoring the term and its posting-side references passes that check; profile rows already sit where the up puts them. Restoring profile rows as well would add conflict cases — a person's edits, deletions, and re-matches since the repair — and a second store of profile text, for an operator action that is rare. A pin left on the survivor, or left retired, is visible and re-pinnable, so nothing silently empties.

## Why a separate schema

`setup/readonly_role.sql` grants SELECT on all tables in `public`, plus a default privilege scoped `IN SCHEMA public`, to the role both the web app and the MCP read gateway use. Any table a migration adds to `public` is therefore readable over MCP with no further step. A new schema receives nothing from that script. The script's other default privilege — the global revoke of PUBLIC EXECUTE on functions — is unscoped, so owner-created repair functions are callable by no application role.

## Roles and grants

- `market_scout_readonly` (`setup/readonly_role.sql`): SELECT on all `public` tables plus `ALTER DEFAULT PRIVILEGES ... GRANT SELECT ON TABLES`, EXECUTE on `similarity(text,text)` and `open_postings_as_of(timestamptz)` only. Shared by the web app and the MCP read gateway.
- `market_scout_actions` (`setup/action_role.sql`): EXECUTE on `mcp` SECURITY DEFINER functions, no table writes. Not provisioned on the test database.
- Migrations never grant. Roles are cluster-level and hand-provisioned; grants are per database, so parity is a repeated action (`developer-guide.md` §2).
- `web-data-layer` (done) floated `market_scout_app` as the eventual write role and deferred preferences until a screen named its filters. This brief is that first screen.

## Why the grant tests run both ways

`web-testing-guide.md` §Database Tests: existing suites fail only on a missing privilege. A widened grant leaves every result identical. The app role is the first role that can write tables, so its refusal on core tables has to be asserted directly.

## Operational interaction

`setup/purge_test_fixtures.sql` deletes `%vitest%` taxonomy rows in the development database and stops on any foreign-key abort. A profile row in the development database pointing at a fixture role would stop it — correctly, since that would be real data referencing a fixture. Test fixtures belong in `market_scout_test`, where this cannot arise.

## Precedent shape

`apps/web/app/prototypes/_personas.ts` `Persona`: `past` as `{role, level, years}` with role as a canonical slug; `have` as skill slugs; one `target`. The brief departs deliberately: past titles and skills keep the person's own text beside an optional reference, because extraction and hand entry both produce items no term matches, and several pins replace the single target. Years of experience are omitted; nothing downstream in the epic reads them yet.

## Ordering pins

Orderings the Decisions imply. Acceptance rows cite them by id.

| id | scenario | ordering | expected outcome |
|---|---|---|---|
| P1 | Double pin | Two tabs, or a double submit, send a pin for the same role at the same moment. | One pin exists. Both submits report success. |
| P2 | Role removed under the page | The page renders a role in the picker; the role is merged away or deleted; the person then submits a pin for it. | The submit reports a readable error. No pin row is written. |
| P3 | Undo over pre-existing survivor state | Before the merge the survivor already holds a posting link and a role dimension the merged role also holds, and the merged slug already has a retired-slug record. Merge, then undo. | The posting side reads as the pre-merge snapshot. The survivor keeps the link, the dimension, and the record it held before. Profile rows stay on the survivor. |
| P4 | Repeat cycle | Merge, undo, merge again, undo again. | After each undo the posting side matches the pre-merge snapshot. The second undo restores only what the second merge recorded. |
| P5 | Chain undone in reverse | Merge A into B, then B into C. Undo the second repair, then the first. | The posting side reads as the snapshot taken before the first merge. Undoing the first repair while the second stands is refused. |
| P6 | Role retire, then undo | Retire a role carrying posting links, role dimensions, a pin, and a past title. Undo. | The role, its links, and its dimensions return. The pin stays retired and the past title unmatched. Role dimensions cascade on delete (000001), so a retire that does not archive them loses them silently. |
| P8 | Classifier writes after repair | Merge; a new classification links a posting to the survivor; undo. | The new link stays on the survivor. |
| P10 | Classifier races a merge | A save linking a posting to the merged term commits while the merge is in progress. | The link ends on the survivor, or the merge is refused and nothing changes. The precedent copies links in one statement and deletes them in the next, so a link committed between them would be deleted unrecorded. |
| P12 | Same repair, two taxonomies | One repair migration runs on the owner's database, which holds both terms, and on a fork that lacks the merged term. | The owner's database is repaired. The fork records the slug as retired; its taxonomy, postings, and profile rows are unchanged. |
| P13 | Two databases after the migration | The migration and the app role script run against the development and test databases. | Each reports the same migration version, clean, and the same objects the app role can use. |
| P14 | Table added after provisioning | A later migration adds a table to `app` after the app role script ran. | The app role has no privilege on it until a grant names it. |

## Promotion capture

What moves into `agent-context/lib/` when this brief is promoted:

- `project.md`: the `app` schema and `market_scout_app`; the rule that no object another application role can reach reads `app`; profile-dependent measures take the profile as input.
- `project.md`: the repair functions as the only repair path, their undo contract, and the rule that a new taxonomy-referencing table extends them.
- `web-guide.md` §Data access: profile reads and writes use the app client.
- `developer-guide.md` §2: the app role script, `DATABASE_URL_APP`, and parity for both databases. Lands with the build: setup steps for a script that does not exist yet would mislead.
- `web-testing-guide.md` §Database Tests: the app test DSN. Lands with the build, for the same reason.

Captured at promotion: the `project.md` and `web-guide.md` items above.
