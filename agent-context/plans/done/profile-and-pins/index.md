# profile-and-pins

Brief · compact · reads: `agent-context/lib/project.md` §Settled architecture, `agent-context/lib/web-guide.md` §Data access, `agent-context/lib/web-testing-guide.md` §Database Tests, `agent-context/lib/developer-guide.md` §2 · read at 2bde04f

## Problem

The owner is replanning the web surface around a person: a résumé becomes a profile, the profile suggests roles, the person pins the roles worth watching, and a dashboard follows the pins (`project.md` §Audience, §Settled architecture). Explore, role detail, the dashboard, and later chat all need to know who the person is and what they pinned. Nothing stores that. The web app has no write path and no app-owned tables; per-person data exists only as a mock in `apps/web/app/prototypes/_personas.ts`. When this is done, one profile per install — past titles, claimed skills, pinned canonical roles — lives in private app-owned tables, written through Server Actions on a dedicated role. The owner enters their real profile on `/profile`. Pins and skill matches survive taxonomy repair on every install and never silently empty.

## Decisions

**Storage and access**
- **Schema comes from the Go migration stream.** It is the only stream; `apps/web` owns none. Undo cost: the down migration drops any profile entered since.
- **Profile tables live in a new `app` schema.** `market_scout_readonly` serves both the web app and the MCP read gateway, and reads every `public` table by default privilege. Profile rows in `public` would reach every MCP session and its model provider.
- **No object an application role other than `market_scout_app` can SELECT or EXECUTE reads `app`.** Owner-rights code leaks what it reads: a view in `public`, a `SECURITY DEFINER` function in `mcp`. Owner-only functions may read `app`. A measure that depends on the profile takes it as input from the app client.
- **Profile tables are mutable.** They hold what the person claims now. The append-only rule governs posting history, not app-owned state.
- **Install-scoped, no profile key.** One profile per install (`project.md` §Non-goals). A key is cheap to add to small tables later.
- **New role `market_scout_app`.** Usage on `app`; read and write on its tables by explicit grant, never default privilege, so a later table is not writable by accident; SELECT on the taxonomy tables the profile names. Plain grants, not `SECURITY DEFINER` (`project.md`: app-owned tables have one writer and no provenance requirement).
- **Profile reads and writes use the app client; taxonomy search stays on the read-only client.** Diverges from `web-guide.md` §Data access, which assumed every table fit the shared role. Private app state does not. No Client Component imports the app client.

**Profile rows**
- **A past title** is the person's text, an optional canonical role, and an optional seniority from `title_seniority_seeds`. A résumé title is a title, so it takes the title-stated rank vocabulary. Absent is NULL; `unstated` is a grouping value and is refused.
- **A claimed skill** is the person's text and an optional skill. **A pin** is a canonical role and the role's name at pin time.
- **Every taxonomy reference is a foreign key, `ON DELETE RESTRICT`.** A slug resolved at read time empties on the first merge, and nothing any reader consults maps a merged slug to its survivor. RESTRICT is the backstop: a repair that skips the functions below fails instead of orphaning rows.
- **Unmatched and retired stay visible.** A skill or past title with no reference shows its text as unmatched. A pin with no role shows as retired under its pinned-at name.
- **Uniqueness.** One pin per canonical role. One claimed skill per matched skill, and per text after trimming and case-folding. Text empty after trimming is refused. Past titles carry no key: two stints under one title are real.
- **Writes are idempotent where a natural key exists.** Pinning a pinned role, unpinning an unpinned one, or claiming an already-claimed skill succeeds and writes nothing. A duplicate past title is visible and deletable.

**Taxonomy repair**
- **Owner-only functions merge and retire roles, specializations, and skills. They are the only repair path.** Each install grows its own taxonomy, so a profile clause hand-written into each repair would run only on the owner's database and break on a fork (`research.md` §Why repair needs functions). Past repair migrations stay as written.
- **A table that references a taxonomy term extends the functions in the same migration.** The functions are complete only while they know every reference.
- **What each reference does:**

  | Reference | Merge | Retire | Undo |
  |---|---|---|---|
  | Posting links; `hybrid-classifier` candidate and seed rows, where present | Move to the survivor; the survivor's row stands on collision | Deleted | Restored |
  | Role dimensions | Move to the survivor | Deleted | Restored |
  | Past title | Re-points | Loses its reference, keeps its text | Stays as the repair left it |
  | Claimed skill, pin | Re-points; deleted when the survivor already holds one | Loses its reference; a pin keeps its pinned-at name | Stays as the repair left it |
  | Retired-slug record | Merged slug recorded | Slug recorded | The repair's own record removed |

- **Undo leaves profile rows where the repair put them.** A re-pointed pin stays on the survivor; a retired pin stays retired and can be re-pinned. The archive therefore holds no profile text (`research.md` §Why undo leaves profile rows).
- **Undo restores only what the repair recorded.** Postings linked to the survivor after the merge stay there. A retired-slug record older than the repair stays.
- **Undo refuses rather than guesses.** It refuses when a later repair touched the same terms, or when the slug has been minted again since. A second undo of the same repair changes nothing and raises a notice.
- **A repair records its retired slugs on every install, even where the term is absent.** The record stops a fork minting the duplicate later, as in 000037 and 000040. Nothing else changes; a merge whose survivor is absent changes nothing and raises a notice.
- **A merge map cannot chain.** A survivor may not itself be merged in the same map. Merging a term into itself is refused.

**Consumer**
- **`/profile` is the consumer and the seed.** It lists and edits past titles and claimed skills (taxonomy search, with unmatched text allowed), pins and unpins roles, and shows retired pins. The owner's real profile enters here, so no personal data is committed. Submit is disabled while a save is pending.

**Non-goals**
- Résumé storage and extraction: `resume-onboarding` owns them.
- Match provenance (person vs extraction): arrives with extraction, as an additive column.
- Seniority on a pin, and narrowing a pin to particular title heads: Explore and the dashboard read the role alone until one shows a need.
- Auth: one operator, local-first (`project.md` §Non-goals).
- Fit scoring and any analysis on `/profile`: `role-fit-measure` and `explore-page` own them.

## Acceptance

### Automated

Run by `pnpm test:db` against `market_scout_test`, each statement through the role named; repair and undo rows call the functions as the owner. A row skipped for a missing test DSN is not met. Pins cite `research.md` §Ordering pins.

**Grant boundary** — both directions, because existing suites only catch a missing grant.
- [ ] The app role can insert, update, select, and delete rows in each profile table, and can select the taxonomy tables the profile names.
- [ ] The app role is refused INSERT, UPDATE, and DELETE on every table outside `app`, and EXECUTE on every repair and undo function.
- [ ] A table the owner creates in `app` after provisioning grants the app role no privilege. The app role holds no role membership and owns no object. (P14)
- [ ] The read-only role is refused SELECT, INSERT, UPDATE, and DELETE on every profile table, and EXECUTE on every repair and undo function.
- [ ] The profile read runs through the app role and the taxonomy search through the read-only role, and each returns the fixture's rows.

**Privacy boundary**
- [ ] No view the read-only or action role can select depends on an object in `app`, checked through the catalog.
- [ ] No function the read-only or action role can execute names `app` in its body, checked by scanning function bodies; the catalog records no dependency for a text body.

**Profile constraints**
- [ ] Deleting a canonical role that a pin or past title references is refused. Deleting a skill that a claimed skill references is refused.
- [ ] A second pin of the same role is refused. A pin of a different role is accepted.
- [ ] A claimed skill with no skill reference and non-empty text is accepted. One with empty or whitespace-only text, text matching an existing claim after trimming and case-folding, or an already-claimed skill under different text is refused.
- [ ] A past title with no seniority is accepted. One naming `unstated`, or a seniority absent from `title_seniority_seeds`, is refused. One with empty text is refused.

**Merge and retire**
- [ ] A role merge with a posting link, a role dimension, a past title, and a pin on the merged role leaves all four on the survivor, deletes the merged role, and records its slug as retired.
- [ ] A role merge where both roles are pinned leaves one pin, on the survivor. A skill merge where both skills are claimed leaves the survivor's claim, text unchanged. A skill merge where only the merged skill is claimed leaves that claim on the survivor, text unchanged.
- [ ] A specialization merge moves its posting links to the survivor, deletes the merged specialization, and records its slug as retired.
- [ ] A role retire leaves the pin with no role and its pinned-at name, and the past title unmatched with its text. A skill retire leaves the claimed skill unmatched with its text and deletes the skill's posting links.
- [ ] Where `hybrid-classifier`'s tables exist, a merge moves candidate and seed rows to the survivor, keeping the survivor's on collision, and a retire deletes them.
- [ ] After a merge or a retire, `mcp.save_enrichment` refuses to mint the retired slug.
- [ ] A repair naming a term absent from the install records the slug as retired and changes no taxonomy, posting, or profile row. A merge whose survivor is absent changes no row. (P12)
- [ ] A merge map that chains terms is refused. Merging a term into itself is refused.
- [ ] A save that links a posting to the merged term, left uncommitted while a merge starts and committed before it finishes, ends with the link on the survivor or with the merge refused and nothing changed. (P10)
- [ ] The foreign-key census lists every constraint referencing `canonical_roles`, `specializations`, or `skills`, and fails on one the functions do not handle. For each listed constraint, a merge and a retire over a fixture holding a referencing row leave no row pointing at a deleted term.

**Undo**
- [ ] A role merge followed by its undo leaves the term row, posting links, role dimensions, and retired-slug records identical to a pre-merge snapshot, and leaves the past title and pin on the survivor. The fixture includes a posting linked to both roles, a dimension both hold, and an older retired-slug record for the merged slug; all three survive the undo. (P3)
- [ ] A role retire followed by its undo restores the role, its posting links, and its role dimensions; the pin stays retired and the past title unmatched. (P6)
- [ ] A merge, its undo, the same merge, and its undo leave the posting side identical to the pre-merge snapshot. (P4)
- [ ] A merge of A into B, then of B into C, undone in reverse order, leaves the posting side identical to the snapshot before the first merge. Undoing the first merge while the second stands is refused. (P5)
- [ ] A posting linked to the survivor after a merge stays linked to it after the undo. (P8)
- [ ] Undo is refused when the retired slug has been minted again since the repair. A second undo of the same repair changes nothing.

**Write path**
- [ ] Pinning a pinned role, unpinning an unpinned one, and claiming an already-claimed skill each report success and write nothing.
- [ ] Two pin writes for the same role issued at the same moment both report success and leave one pin. (P1)
- [ ] A pin write naming a role that no longer exists returns an error the page can show and writes no row. (P2)

### Manual

- [ ] On a fresh test install, `/profile` renders its empty state.
- [ ] Add a matched skill, an unmatched skill, a past title with a matched role, and two pins; reload; all persist as entered.
- [ ] Unpin one role; reload; it is gone and the other remains.
- [ ] Submit is disabled while a save is pending.
- [ ] A retired pin, set up by the retire function, renders as retired with its pinned-at name.
- [ ] Taxonomy search, add, remove, and pin controls are keyboard-operable with visible focus.
- [ ] An MCP `query` session selecting from a profile table is refused with a permission error.
- [ ] After the migration and the app role script run against both databases, each reports the same clean migration version, and the objects the app role can use match. (P13)
- [ ] `pnpm preflight` passes.

## Path

- Precedents: merge in 000037 §5 and 000040 §4; retire in 000029; undo in the downs of 000037 and 000040, which read `retired_slug_links` and `retired_role_dimensions`. The `mcp` schema shows a schema with its own grant boundary. `setup/readonly_role.sql` shows role hardening; the parity procedure in `developer-guide.md` §2 gains the app role.
- `sqlc generate` emits a model for every table; commit the regenerated models with the migration.
- The archive may live in `public`; it holds no profile text.
- Taxonomy search: port the ranking of `cmd/mcp/taxonomy_search.go` `taxonomySearchSQL` into `apps/web/lib/db`. Drift only reorders a picker. The read-only role already holds EXECUTE on `similarity(text,text)`.
- Test DSNs: `testDsns()` in `apps/web/lib/db/test-dsn.ts` gains an app DSN under the same `_test` guard. The write core of each action takes a client, so tests call it without the Server Action wrapper.
- Fresh installs are not empty: migrations 000001 and 000008 seed two roles. Absent-term fixtures use slugs no seed names.
- Rival shape, rejected: profile rows store slugs, resolved through a survivor redirect at read time. It drops the functions, but loses referential integrity, and a slug the legacy runner mints again would take over pins.
- `hybrid-classifier` has reserved migration 000044 behind its own probe gate. Whichever lands second renumbers; if `hybrid-classifier` lands second, its migration extends the repair functions to its candidate and seed tables.
- First slice: schema, app role script, merge and retire, and the grant, privacy, and merge rows. They falsify the riskiest premises before any UI exists.
- A combobox is likely a new shadcn add; migrate its colour classes per `web-guide.md` §Colour.

## Open questions

- Table, column, and function names; the archive's shape; ordering of past titles — **delegated**: the executor decides and reports them in the plan of record.
- Taxonomy search latency over the full skill list, and whether it needs a trigram index — **delegated**: measure, and index only if the picker is visibly slow.
