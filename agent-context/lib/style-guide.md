# Style Guide

Short sentences. Clear words. No waste.

> **Read this when:** writing or updating any file in `agent-context/`. These files are agent context — they exist so AI agents (and future-you) can make informed decisions about the codebase without re-deriving everything from source.
> **Key invariant:** context describes what survives refactoring. If a sentence breaks when a module is reorganized or a function is renamed, it belongs in a task description or code comment — not in `agent-context/`.

---

## Writing Principles

**Follow these principles for all prose in context library.**

- **Direct and brief.** Active voice, short sentences, no filler. One idea per sentence. Fragments in lists. Drop articles where natural. See *Prose* below for examples.
- **Durable.** Context files describe what survives refactoring. If a sentence breaks when a file is renamed or a struct is extracted, it belongs in a task description or code comment — not a context file. See *Durable vs. Ephemeral Content* below.
- **Seamless.** Every edit reads as if the file was always written this way. Restructure surrounding text when new edits reduce overall cohesion.

## Prose

**Direct and brief.**

**Before:**
```
The fetcher binary is responsible for iterating over each configured
ATS adapter, fetching the current set of job postings, and writing
timestamped snapshot rows into the posting_snapshots table for later
trend analysis.
```

**After:**
```
Fetcher iterates ATS adapters, writes timestamped rows to
posting_snapshots. Never updates in place.
```

**Before:**
```
The fetcher uses a worker pool to dispatch concurrent HTTP requests
across companies and it determines per-request rate limits by
looking up the adapter for that company and applying whichever
backoff policy that adapter declares.
```

**After:**
```
Fetcher: bounded worker pool, per-adapter rate limit and backoff.
```

**Before:**
```
What is the Snapshot Ingestion Pipeline
```

**After:**
```
Snapshot Ingestion Pipeline
```

Headers are short nouns, not questions.

## Code and Paths

Name boundary modules and entry points. Describe contract semantics and invariants in prose. Struct layouts and SQL column lists live in code and migrations — don't reproduce them.

**Point to boundary packages** (use actual paths once they exist — never invent paths in examples):
```
ATS adapters: internal/ats/<adapter>.go
Snapshot writer: internal/db/snapshots.go
Fetcher entry:  cmd/fetcher
```

**Describe behavior in prose:**
```
Adapter fetches an ATS endpoint, normalizes its payload into a
common Posting shape. Snapshot writer accepts Postings, inserts a
timestamped row per posting per fetch. Adapters never write to the DB.
```

**When to include code samples:**

- Specs for unbuilt features (mark `// Proposed design`) — remove after implementation
- Convention patterns only when a package reference alone wouldn't convey the pattern

After implementation, point to real code. A stale struct definition or SQL schema in a context file creates mismatches at the worst possible layer. Describe the contract's semantics in prose; reference the source file or migration for the shape.

Package-internal types (anything under `internal/`, unexported identifiers) belong in code comments, not context files.

## Durable vs. Ephemeral Content

Context files describe what survives refactoring. Task descriptions describe what to change right now.

**Litmus test:** "If we rewrote this package with a different approach, would this sentence still be true?" "Does this sentence describe something I wouldn't find quickly by searching strings and rapidly reading code files?" Yes → context file. No → task description or code comment.

**Belongs in context files (durable):**
- Design principles and intent (local-first, snapshot-based, no destructive updates)
- Subsystem boundaries and ownership (fetcher, ATS adapters, DB layer, Next.js app)
- Pipeline topology as semantic stages (not function names)
- Lifecycle ordering constraints and why alternatives fail (e.g. why snapshots are append-only)
- Data contracts at module boundaries (what an adapter promises to emit; what the snapshot writer expects)
- Architectural invariants (Next.js talks to Postgres directly; no Go API server)
- External API contracts (ATS adapter response shapes, auth model, pagination semantics)

**Belongs in plans / task descriptions (ephemeral):**
- Function, struct, and field names internal to a package
- Specific algorithms and implementation choices
- "What to change" instructions with code snippets
- Error messages and log output strings
- SQL column names and migration filenames

Implementation detail helps agents start fast. Put it in plans — consumed once and done. In context files, it becomes maintenance debt.

**Example — pipeline stage description:**

**Before** (names a function and a column — breaks when either is renamed):
```
Stage 3: `writeSnapshots()` iterates `[]Posting`, calls
`tx.Exec("INSERT INTO posting_snapshots (posting_id, fetched_at, ...)")`
once per posting inside a single transaction.
```

**After** (describes the stage's role and contract — breaks only when the architecture changes):
```
Stage 3: Snapshot write. Persists one row per posting per fetch in a
single transaction. Atomicity per fetch is load-bearing: partial
snapshots would corrupt trend queries that assume each fetch is a
complete view of a company's board.
```

## Spec Completeness

Define what you're defining, completely. A spec with scattered "TBD" markers pushes ambiguity to the implementing agent — who has less context than the spec author.

**In-scope items get full coverage.** Edge cases, error states, constraints. If a behavior is worth specifying, specify it completely. If you can't fully specify something yet, move it to non-goals — don't leave a half-defined section.

**Out-of-scope items get a non-goals section.** One list, one place. Not "TBD" annotations sprinkled through the file.

| | Before | After |
|---|---|---|
| **In scope** | "Missing fields TBD" | "Missing department falls back to '(unspecified)', logged at ingest" |
| **Out of scope** | "Lever and Ashby adapters (future)" buried in the adapter section | Non-goals: "Lever and Ashby adapters" |

**The spec is where scope decisions happen.** Implementation inherits those decisions. If a spec is vague, the agent guesses — and guesses compound.

**Constraints, not solutions, for low-level details.** Schema column order, index choices, and embedding dimensions are implementation concerns — the implementor sees the full picture when they land. State the constraint ("snapshot writes must be atomic per fetch") not the solution ("wrap the loop in `BEGIN; ... COMMIT;` at line 42"). Prescribing layout prematurely creates conflicts when two specs touch the same table.

## Document Structure

**Orientation block.** Start each context file with: when to read this, key invariant, related files. Lets any reader (human or agent) decide in seconds whether to keep reading.

**Tables over prose** for mappings, field definitions, option lists. A table with columns `Field | Type | Default | Description` beats four paragraphs.

**Explicit non-goals.** State what the file (or feature) does not cover.

**Terminology tables** when renaming or introducing terms. `Old | New` with concrete identifiers.

**Diagrams.** ASCII diagrams follow the same durable/ephemeral rules as prose. Pipeline topology and subsystem relationships are durable; function call sequences and table schemas are not. Keep diagrams minimal — every box or arrow is a maintenance commitment when an adapter or table is added or renamed.

## Density vs. Clarity

Some topics need more words: external API contracts, cross-package boundaries, non-obvious behavior, snapshot semantics. Add detail for these — but keep the style direct.

**Before:**
```
The fetcher needs to handle the case where a job posting that was
present in a previous fetch is no longer returned by the ATS, because
postings can be unpublished or filled, and in that case we should
not delete the old snapshot rows but should instead just stop
writing new ones for that posting going forward.
```

**After:**
```
Postings disappear between fetches — unpublished, filled, or pulled.
Fetcher does not delete prior snapshots. It simply stops writing new
rows for that posting. Disappearance is itself a signal: the gap
between last-seen and now is what trend queries read as "closed."
Snapshots are append-only; absence is data, not an error.
```

The "after" is longer. That's fine — the append-only contract and its disappearance semantics earn the extra sentences. Brevity means no wasted words, not fewest words.

## Documentation Lifecycle

Context files are the durable layer. Plans are the ephemeral layer.

**Plans** live in `agent-context/plans/`, moving through four stages: `drafts/` → `ready/` → `in-progress/` → `done/`. Plans contain detailed implementation specs — function names, SQL, task breakdowns, acceptance criteria. That detail earns its place during planning but becomes maintenance debt once the code exists.

**Before a plan moves from `drafts/` to `ready/`:** capture durable decisions in `agent-context/lib/`. New architectural constraints, adapter contracts, and pipeline topology belong there — not in the plan. Agents working the plan find full context in the codebase, not in plan documents.

**After a plan ships:** the plan moves to `done/` and stays as a historical record. Don't maintain it. The implementation is the source of truth; `agent-context/lib/` holds the durable architectural layer.

**Code comments** capture implementation-level "why" decisions — rationale a reader can't derive from the code alone.

**What doesn't belong in `agent-context/lib/`:**
- Specs for specific features (use `agent-context/plans/`)
- Implementation plans or task breakdowns
- Content that names specific functions, types, or file paths as load-bearing detail (see *Durable vs. Ephemeral Content* above)
