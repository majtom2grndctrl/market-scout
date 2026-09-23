---
name: build-session
description: >
  Runs a feature conversation, then builds it in the same session — working
  directly, and dispatching parallel agents for tracks that are genuinely
  independent. Surfaces the owner's blind spots on details that change the
  details they care about, writes a design contract every agent reads before
  dispatch, and folds durable decisions into agent-context/lib as each track
  lands. Use when the user wants to talk a feature through and have it built
  now, without a spec.
disable-model-invocation: true
argument-hint: "[feature]"
---

# Build Session

Two moves: settle the design with the owner, then build it.

Process is yours to choose. Track boundaries, how wide to go, when to stop — judgment calls, and you have better information than this file does. The rules below are the ones that cost real work when broken.

Arriving from `/draft-session` with a direct-build handoff? The conversation is done. Start at §2 and seed the contract from the handoff block.

## 1. Conversation

The owner cares about a few details. Not all of them. Get up to speed from the repo, not from their turns.

- Route through `agent-context/lib/index.md`, read the two or three docs that matter, then read source. Grounding is yours.
- **Find the blind spots.** Name the unraised details that change the details they *did* raise. Carry each as a recommendation with its consequence — "X means Y; I'd do Z" — not as an open question. Two or three, not a questionnaire.
- Ask only what the repo cannot answer: product behavior, policy, an agent-facing surface, a one-way door. Recommend a direction with every question.
- Everything else: decide from project context, state the call in one line, keep moving.
- Where a call turns on a Go idiom, name the idiom in a clause. The owner is learning Go on this project.

A detail the owner never raised, touching nothing they did raise, is yours to settle silently.

**Check the lane before building.** A schema change, an agent-facing contract (an MCP tool, a view the agent reads), or a data write path is where a wrong decision outlives the code (`developer-guide.md` §1.5). This skill can still build it — but say so, and confirm the owner wants no reviewed artifact first. Offer `/draft-session` when they hesitate.

End the conversation with the whole specification settled. Long-horizon work goes best when the complete task spec exists in one place before execution starts, rather than accumulating across interactive turns.

## 2. Design contract

Start a feature branch off `main` before anything is written — `feat/<slug>`, where `<slug>` names the feature. Everything below lands there — the contract, every track, the review.

Then write `agent-context/plans/in-progress/<slug>/index.md`. The line under the title reads `> Build-session contract — decisions, invariants, track ownership.` Other skills enumerate `in-progress/`; the marker keeps this from reading as a spec. Commit it before dispatching, so every track starts from the same committed state. Every agent reads this file first, before its own brief.

Agents that share a written contract build compatible pieces with zero communication; agents inferring the design from their own prompt do not. A prompt also drifts as you retype it across tracks. A file does not.

Contents:

- Goal — one orienting paragraph.
- Decisions from the conversation, each with the consequence that makes it load-bearing.
- Invariants no agent may break: names, units, orderings, conventions.
- File ownership per track.
- Acceptance per track, as commands with expected output wherever it can be one.
- Open questions, marked as open.

Pin every convention with two plausible readings, even the ones obvious to you: Go field vs JSON key vs SQL column casing, zero vs absent (NULL, omitted key, `0`), inclusive vs exclusive window bounds, UTC vs local, which side of a seam owns validation, whether a count is postings or requisitions. Two agents guessing opposite ways is a silent integration bug. When names cross Go ↔ JSON ↔ SQL, pin them in a boundary table (`/draft-plan` §Boundary inventory).

Match the contract's length to the decisions it carries. Padding it with restated background, redundant summaries, or boilerplate sections costs every agent that reads it.

Put the decisions and invariants in front of the owner before the first dispatch. They settled them in conversation, but a contract they have not read is a spec you wrote alone, and every track is about to build on it.

Amend the file when a track changes a decision — before the next dispatch, not after.

## 3. Build

**Do the work yourself by default.** A subagent re-establishes context from nothing, re-explores, reports back, and then you read the report. Anything you could finish in a handful of tool calls is cheaper and more reliable done directly — a few file reads, a handful of edits, a focused search, a check.

**Dispatch along the repo's seams.** One feature here routinely spans several layers with a typed contract between each: a numbered migration and its SQL queries, the sqlc output generated from them, the Go package that consumes that output (an adapter in `internal/ats`, the fetcher, an MCP tool), a read-model view, and the `apps/web` Server Component and components that read the view. Package boundaries are real (`developer-guide.md` Agent TL;DR — `internal/ats` never touches the DB), and generated types enforce the SQL ↔ Go seam. Those are real tracks: different layers, different expertise, a pinned contract between them. That is what makes the parallelism real and the overhead repaid.

Size alone is not the signal. A large change inside one package is one track, however many lines it runs to. A migration and the web page reading its view are two tracks once the view's columns are pinned, even when each is small. Give every track a whole vertical slice with its own acceptance — never one agent per file or per step.

**Keep the count low, then choose the shape.** One agent beats several on the same job, and one modest job is never split across parallel agents. Past that the shape is yours, and the two are worth different things.

**Concurrent** when the tracks are independent and the contract between them is already pinned. Launch them in a single message so they genuinely overlap rather than queueing, and give each an isolated worktree. You buy wall-clock; the costs are the shared-database rules under *Repo physics*.

**Sequential** when one track's output is another's input, when the first track's findings would narrow the next brief, or when the design is uncertain enough that you might stop after the first. Sequential tracks work on the branch directly and test as they go. You buy early exit and briefs that get sharper as you learn.

Mixing is the normal case: land the migration and its queries in one track, then fan its consumers out.

**Brief precisely the first time.** Launching, waiting, and re-briefing costs a full context rebuild each round. Give the whole slice up front — outcome, constraints, acceptance, the contract — then let it run.

**Commit to the dispatch.** When a track reports, do not redo its work or re-derive its findings.

**Writes stay at depth 2.** A track may read as widely as it likes, including by fanning out read-only. It never spawns an agent that writes — you cannot partition file ownership among agents you did not create, and a child you cannot see outlives the parent that made it.

### What goes in a brief

**Intent, not procedure.** Ordered step lists, prescribed search strategies, and told-you-how decomposition lower output quality. They compensate for a weakness the agent does not have.

**Constraints alongside instructions, even when the instruction already satisfies them.** This redundancy is what lets an agent catch you. "Add the column to the snapshot insert" paired with "`posting_snapshots` is append-only; never an upsert" gets refused and corrected when the insert turns out to be an upsert. The same instruction alone gets followed, and the breakage surfaces later as corrupted history. An unstated constraint turns the agent into an amplifier of your errors instead of a check on them.

**A defect by its symptom and how you found it** — never by file and line. A location inherited from another report is a hypothesis you are laundering into a fact. An agent told "the defect is in `open_postings`" looks there. An agent told "a company whose last fetch failed shows zero open postings on the status page, and here is the query that shows it" reproduces first, and finds the real fault next door.

**Hard constraints marked apart from preferences.** A hard constraint breaks the build, the data, or the design when violated. Name it as one, justify it, and expect the agent to turn it into a test assertion that binds every future change. A preference stated in the same register gets enforced just as hard, and costs the flexibility you wanted.

**The acceptance gate as commands with expected output.** This is what makes a track self-correcting: given a gate it can run, an agent loops until the gate is green without being told to be careful. "Iterate until `go test ./internal/ats -run Workday -count=1 -v` passes with the new cases listed, and `git diff --stat apps/tools/internal/db/migrations/` stays empty" is a task with a built-in stopping condition. Name the check that would catch the failure you actually fear — a rejected NULL, an unchanged directory, a view column that must not move.

**No instruction to verify.** `opus` verifies its own work unprompted; telling it to verify, re-check, or confirm buys extra work and no coverage. Delete that scaffolding rather than rewording it. This inverts the usual self-check advice and rides on the tier — `fable` is the opposite and wants an explicit checking harness on a cadence.

**Scope discipline, when a track has room to wander.** Deliver what was asked at the scope intended; make routine judgment calls; say so in a sentence and keep going if the ask looks mistaken, rather than quietly narrowing or widening it; report completion only when it is actually done.

### What to require back

- **What the agent could not verify, and where it would look first.** Asking what it *couldn't* reach is not asking it to verify. A track's most valuable output is often the edge it could not test — live ATS behavior, a migration against real data, a render only visible in the browser. That list is your first stop when the thing runs.
- **A report you can read once.** Your context is spent reading reports, not writing briefs, and that is the budget that decides how long you can hold the whole picture. Ask for what changed, what the gate returned, what could not be verified, and what surprised them — not a replay of how the work went. Take it as given and move on; re-deriving a track's findings costs your context twice and buys nothing.
- **Environment findings, forwarded.** A test that already fails on unmodified HEAD, a missing `.env.local` link, a test database behind on migrations. Carry each into the next brief. Otherwise every agent rediscovers the same pothole at full price.
- **Artifacts, not only tests.** Tests prove the code does what it says; an artifact proves the thing works. For `web`, name the stories and routes a person should look at. For a view, the rows it returns against real data. When the artifact is visual, require the agent to look at it — a chart whose axis runs backwards passes every type check.
- **Compile-forced spillover, reported.** A new struct field or interface method breaks call sites elsewhere. The minimal change that keeps the module compiling is allowed outside a brief, and must be flagged. A strict lane that leaves the tree uncompilable is worse.

### Sizing

Set `model:` on every Agent call. Omitting it inherits the session model: top tier spent on plumbing, or a contract track handed to a scout.

Split on blast radius, never on size. Does the track **establish** a contract that other code consumes — a migration, a view's columns, an MCP tool's envelope, a cross-package type? Or does it **execute inside** one already settled?

| `model:` | Use for |
|---|---|
| `opus` | Establishing contracts: migrations, views, MCP tool shapes, write paths. Ambiguity resolved by reading code. |
| `sonnet` | Execution inside a settled contract, however large. Components against a pinned view, tests for specified behavior. |
| `haiku` | Read-only sweeps too wide to run yourself. |
| `fable` | A reasoning track that genuinely exceeds `opus`. Rare; costs accordingly. |

The aliases are durable; what backs them is not. When one stops resolving, fix this table — don't route around it.

### Load-bearing facts

Read them from source yourself. A column's nullability, a view's run predicate, a function's grant — anything the design turns on is two tool calls, and you are the one who has to hold it. Dispatch a scout only when *finding* the fact needs a wide sweep, and open the file yourself before designing against what it returns. A stale comment sitting directly above the query it describes reads exactly like the truth.

### Repo physics

The machine's limits, not the roster's.

- **Worktrees share one Postgres.** The development and test databases are cluster-wide, not per worktree. A worktree isolates files, never schema. Two tracks running `migrate up` against the same database collide, and a migration applied from a track you later discard leaves the database ahead of every branch.
- **One track owns migrations and sqlc.** The contract assigns migration numbers and owns `internal/db/queries/` plus its generated output. Concurrent tracks never pick a number or run `sqlc generate` — two picking the same next number is a merge conflict at best and a silently skipped migration at worst. Never hand-edit sqlc output (`developer-guide.md` §5.8).
- **Applying migrations is yours.** After the migration track merges, you run `migrate up` against development and test databases, then the role scripts `developer-guide.md` §2 names for that kind of migration. Tracks consume the applied schema; they do not apply it.
- **Database-backed tests run one at a time.** `go test -tags=integration` and `pnpm test:db` hit the shared test database. Concurrent tracks gate on unit tests only; database-backed suites run on the branch after merge.
- **Fresh worktrees lack gitignored setup.** No `.env.local` links and no `apps/web/node_modules`. Link `.env.local` from the main checkout by absolute path, and run `pnpm install` in `apps/web` before any web command. Go builds are cheap; a worktree track can pay its own build and test.
- **Free tier only in tracks.** `cmd/fetcher` hits live ATS APIs; batch-enrich spends model budget. Live-surface commands are yours or the owner's (`developer-guide.md` §2 Cost map).
- **Focused tests, with a count.** `go test ./<pkg> -run <Filter> -count=1 -v` from `apps/tools/`; `pnpm test <path>` from `apps/web/`. Require the test count — a Go filter matching nothing prints `ok` with `[no tests to run]`.
- **The batch-enrich skill is pinned.** Touching `.claude/skills/batch-enrich/SKILL.md` bumps `PROMPT_VERSION` (`/preflight` §Batch-enrich pin gate).

**Every agent reads `agent-context/lib/style-guide.md` and its surface's guide** — `developer-guide.md` §4 for file size on `tools`, `web-guide.md` on `web`. Style governs code comments and any prose; §4 governs splitting. Whole-slice tracks make god files the likely failure — an agent authoring new code never edits an already-large file, so nothing trips the usual threshold.

## 4. Context maintenance

When a track lands: merge, then fold what it made durable into `agent-context/lib/` before the next dispatch. Defer it all to the end and the next agent reads a stale library, while you write from memory instead of from code.

Record only what survives refactoring (`style-guide.md` §Durable vs. Ephemeral Content). A sentence that breaks when a file is renamed belongs in a code comment. Update the `index.md` router when the work adds a concept someone would search for. Amend the contract with anything that changed.

## 5. Landing

**One review track, not a panel.** `opus` finds real bugs at high recall in a single pass, so several reviewers on one diff is one modest job split across parallel agents. A panel also returns claims you then have to re-verify, which spends the context you spent the whole session protecting.

Dispatch one `opus` agent to review the diff and fix what it finds — the recall above is that tier's. Keep three stages distinct in its brief.

**Find.** Several lenses in one pass. Coverage is the job at this stage:

- Correctness — trace one data flow end to end: ordering, producer/consumer mismatch, swallowed errors, per-company isolation.
- The contract's invariants, and agreement across every layer a changed name crosses: migration, SQL, generated Go, struct tags, JSON, view, web read.
- Data integrity, wherever the diff writes rows: append-only, atomicity, provenance, trust tiers kept distinguishable. A code bug costs a rerun; a write-path bug corrupts history that cannot be refetched.
- Package boundaries and hand-edited generated files.
- Test coverage of the contract's acceptance rows, both sides of every predicate.
- Comments and prose against `style-guide.md`, quoting the on-disk text.

> Report every issue you find, including ones you are uncertain about or consider low-severity. Do not filter for importance here — a later stage does that. For each finding, give a confidence and a severity, and quote the line with its file and symbol.

**Filter.** Rank the findings and drop what does not hold: a quote that cannot be located in the tree, a finding the contract already answers, a preference dressed as a defect.

**Fix.** Apply what survives. Stop at anything that would change a decision in the contract — those come back to you, unfixed, with the reasoning.

Require back what it found, what it changed, and what it left for a decision. Read the first and last; take the middle as given.

A track that lands mid-session can take the same brief on `sonnet` as a cheap early pass. Accuracy holds at lower cost, so a quick pass per track and a thorough one at landing beats a single review at the end.

Then run the `/preflight` gates once, as the single full-suite gate.

Record each acceptance row's result and any outstanding manual proof in the contract, under a `## Result` heading. No silent gaps. Name the two or three choices this feature made that are worth the owner's understanding — idiomatic Go on `tools`, interaction and design-system decisions on `web`.

**When the owner says "land the plane":** rerun `/preflight` if code changed after the gate, move the contract to `agent-context/plans/done/` (or delete it once `agent-context/lib/` absorbs it), remove session worktrees, then commit and push the branch.

## Reporting to the owner

The owner reads your text between tool calls and sees neither your thinking nor the raw results. Lead with the outcome — what happened, what you found — then the detail. Say what you are about to do before a long dispatch, and speak up mid-track when you hit something load-bearing or change direction. Write in complete sentences, spelled out, without shorthand or labels they would have to cross-reference.

## Never

- Never dispatch an agent for work you could finish in a handful of tool calls.
- Never split one modest job across parallel agents.
- Never dispatch a track with no gate it can run.
- Never omit `model:` on an Agent call.
- Never let a track spawn an agent that writes.
- Never launch, wait, then re-brief — the whole slice goes in the first brief.
- Never redo a track's work after it reports.
- Never ask an agent to verify, re-check, or confirm its own work.
- Never write a step-by-step procedure into a brief.
- Never give an instruction without the constraint that would catch it if it is wrong.
- Never hand an agent a file and line for a defect you have not reproduced.
- Never state a preference in the register of a hard constraint.
- Never tell a reviewer to report only what matters, or to skip the nits.
- Never design against a fact you have not read in source.
- Never dispatch before the contract file exists.
- Never let an agent infer a decision that belongs in the contract.
- Never dispatch against a contract an earlier track invalidated.
- Never let two tracks own migrations, or let a track apply one.
- Never interrogate the owner on details the repo already settles.
- Never present a blind spot as an open question when you have a recommendation.
- Never make a product or architectural decision on the owner's behalf. Surface it.
- Never delegate the integration — contracts, merges, migrations, and commits stay yours.
- Never batch all context updates to the end of the session.
