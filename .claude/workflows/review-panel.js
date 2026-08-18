export const meta = {
  name: 'review-panel',
  description: 'Lens-based review panel: triage slices, dispatch depth and breadth reviewers, seam pass, dedupe, refute findings against source',
  phases: [
    { title: 'Triage', detail: 'name flows, surfaces, and write paths per slice' },
    { title: 'Review', detail: 'depth lenses + hygiene/drift per slice' },
    { title: 'Seam', detail: 'trace flows that cross slices' },
    { title: 'Aggregate', detail: 'dedupe and merge severity' },
    { title: 'Refute', detail: 'attack red/yellow findings against live source' },
  ],
}

// args contract (coordinator: .claude/skills/review-panel/SKILL.md):
//   target      one-line description of what is being reviewed
//   diffRef     git ref/range reviewers diff against, e.g. "HEAD" or "main...HEAD"
//   slices      [{ name, paths: [string] }] — coordinator partitions from the diff stat
//   reviewers   optional int — force depth-agent count per slice
//   depthModel  model for triage, depth, and refuter agents (default 'opus'; hygiene and dedupe stay 'sonnet')
const target = (args && args.target) || 'uncommitted changes'
const diffRef = (args && args.diffRef) || 'HEAD'
const slices = (args && args.slices) || []
const depthModel = (args && args.depthModel) || 'opus'
const forcedReviewers = (args && args.reviewers) || null

if (!slices.length) throw new Error('args.slices is required: [{ name, paths: [...] }]')

// ---- schemas ----

const TRIAGE_SCHEMA = {
  type: 'object',
  required: ['mechanical', 'flows', 'surfaces', 'exitFlows', 'subtleInvariants', 'dataWrite'],
  properties: {
    mechanical: { type: 'boolean' },
    flows: {
      type: 'array',
      items: {
        type: 'object',
        required: ['name', 'entry', 'files'],
        properties: {
          name: { type: 'string' },
          entry: { type: 'string' },
          files: { type: 'array', items: { type: 'string' } },
        },
      },
    },
    surfaces: { type: 'array', items: { type: 'string' } },
    exitFlows: {
      type: 'array',
      items: {
        type: 'object',
        required: ['name', 'path'],
        properties: { name: { type: 'string' }, path: { type: 'string' } },
      },
    },
    subtleInvariants: { type: 'boolean' },
    dataWrite: { type: 'boolean' },
  },
}

const FINDING = {
  type: 'object',
  required: ['file', 'severity', 'lens', 'summary'],
  properties: {
    file: { type: 'string' },
    line: { type: 'integer' },
    severity: { enum: ['red', 'yellow', 'green'] },
    lens: { type: 'string' },
    summary: { type: 'string' },
    detail: { type: 'string' },
  },
}

const FINDINGS_SCHEMA = {
  type: 'object',
  required: ['findings'],
  properties: {
    findings: { type: 'array', items: FINDING },
    commentDrift: { type: 'array', items: FINDING },
    praise: { type: 'array', items: { type: 'string' } },
  },
}

const AGG_FINDING = {
  type: 'object',
  required: ['file', 'severity', 'lens', 'summary', 'agreement'],
  properties: {
    file: { type: 'string' },
    line: { type: 'integer' },
    severity: { enum: ['red', 'yellow', 'green'] },
    lens: { type: 'string' },
    summary: { type: 'string' },
    detail: { type: 'string' },
    agreement: { type: 'integer' },
    slice: { type: 'string' },
  },
}

const AGG_SCHEMA = {
  type: 'object',
  required: ['findings', 'commentDrift'],
  properties: {
    findings: { type: 'array', items: AGG_FINDING },
    commentDrift: { type: 'array', items: AGG_FINDING },
  },
}

const REFUTE_SCHEMA = {
  type: 'object',
  required: ['results'],
  properties: {
    results: {
      type: 'array',
      items: {
        type: 'object',
        required: ['id', 'verdict'],
        properties: {
          id: { type: 'integer' },
          verdict: { enum: ['STANDS', 'REFUTED'] },
          citation: { type: 'string' },
        },
      },
    },
  },
}

// ---- lens blocks ----

const LENS = {
  tracer: `You are a Correctness Tracer. Review by execution, not by scanning.

Your brief names one data flow — an entry point and the files it crosses. Trace it end to end. Mentally execute it: at each step, what is the state, what does this step hand the next, does the consumer expect that shape and that order? Review code outside the flow only where it feeds or reads the flow.

Hunt for: ordering and lifecycle bugs (X runs before Y but depends on it), producer/consumer mismatches (a caller passes what the callee never reads), swallowed errors (an err assigned and never checked, a wrapped error that drops the failure mode callers branch on), and broken per-source isolation (one company's failure aborting a batch the contract says continues).

A finding names the step, what you expected when executing it, and what the code does instead.`,

  verifier: `You are a Contract Verifier. Your brief names one surface the diff changes — an MCP tool envelope, a sqlc query contract, the adapter interface, a JSON wire shape, a documented watchlist rule.

Check that every layer of that surface agrees: the migration schema, the SQL query, the sqlc-generated function, the Go struct tags, the JSON the caller sees, the doc that promises the behavior, and the runtime behavior itself. The contract is broken when any two disagree — the doc promises what the code doesn't ship, validation accepts what the runtime rejects, an error names the wrong field, a struct tag drifts from the pinned casing.

Where a rule has more than one consumer — a dedup rule shared by the sidecar and an MCP tool, a casing pinned in a boundary inventory — verify every consumer reads one definition. A drifted copy is a broken contract.

A finding names the two layers that disagree and quotes both.`,

  adversarial: `You are an Adversarial Tester. Try to break the change.

Construct edge cases the happy path ignores: empty batch, a company with zero postings, NULL columns crossing the sql.Null-to-pointer translation, context cancellation mid-run, duplicate or colliding slugs, repeated calls, a partial ATS response, boundary values. For each, trace what the code actually does — panic, corrupt rows, silently drop, or hold the invariant?

Tie every case to code; do not list hypotheticals you can't ground. A finding is a concrete input sequence, the invariant it violates, and where in the code it goes wrong.`,

  dataIntegrity: `You are a Data-Integrity Reviewer. In this project the database is the product: code is replaceable, collected history is not. A code bug costs a rerun; a write-path bug corrupts rows that cannot be refetched.

Your brief names the write paths this slice touches. Check each against the storage model: snapshots are append-only (no update path, no upsert — ON CONFLICT DO UPDATE on a snapshot table is a finding by itself), writes are atomic per fetch, first-observed timestamps are set once and never overwritten, snapshots link to their fetch run, classifications carry their provenance keys, and NULL-vs-absent stays distinguishable across the sql.Null boundary.

A finding names the invariant, the write site, and the sequence of events that violates it.`,
}

// ---- prompts ----

function preamble(paths) {
  return `Before reviewing, read: agent-context/lib/index.md (architecture, subsystem boundaries), agent-context/lib/developer-guide.md (conventions, constraints), agent-context/lib/testing-guide.md (test expectations).

You run one lens, not a general review. Stay in it — the broad completeness/correctness checklist is another agent's pass; don't duplicate it.

Review target: ${target}.
This slice: ${paths.join(', ')}
See the change: \`git diff ${diffRef} -- ${paths.join(' ')}\`. Read surrounding source as needed. If the diff is empty for a file, review its current source.

Severity: red = must fix, yellow = should fix, green = nit. Findings are data — no padding. Genuine strengths go in the praise array, not in findings.`
}

function triagePrompt(slice) {
  return `Triage one diff slice before review — a fast scan, not a review.

Slice: ${slice.name} — ${slice.paths.join(', ')}
Run \`git diff --stat ${diffRef} -- ${slice.paths.join(' ')}\`, then read the diff.

Produce:
- flows: 1–3 data paths worth tracing — paths that cross files or carry ordering, lifecycle, or error-isolation logic. Name each flow's entry point and files. If the slice carries logic but no cross-file flow, name the main code path through the change as one flow.
- surfaces: contract surfaces the slice changes (MCP tool envelope, sqlc query contract, adapter interface, JSON wire shape, documented watchlist rule).
- exitFlows: flows that leave this slice's paths. Describe each flow's full path, including where it exits to.
- subtleInvariants: true when the slice carries subtle invariants or many edge conditions.
- dataWrite: true when the slice touches a migration, an INSERT or UPDATE query, or Go code that writes rows — snapshots, classifications, companies, taxonomy.
- mechanical: true only when there are no flows, no surfaces, and no writes — pure refactor, comment-only, formatting.`
}

function depthPrompt(lens, slice) {
  return `${preamble(slice.paths)}

${LENS[lens.kind]}

Your brief: ${lens.brief}`
}

function seamPrompt(flow, allPaths) {
  return `${preamble(allPaths)}

${LENS.tracer}

Your brief: cross-slice flow "${flow.name}" (starts in slice ${flow.from}). Full path: ${flow.path}. Trace it across every file it crosses — the bug hides at the seam, not at either end.`
}

function hygienePrompt(slice) {
  return `You are the Hygiene + Drift reviewer — the breadth pass.

Read first:
- agent-context/lib/developer-guide.md §7 (Code Comments)
- agent-context/lib/style-guide.md (durable vs ephemeral content)

Review target: ${target}.
This slice: ${slice.paths.join(', ')}
See the change: \`git diff ${diffRef} -- ${slice.paths.join(' ')}\`. If the diff is empty for a file, review its current source.

Breadth checklist: missing error handling or wrapping, nil dereferences, input validation at boundaries, dead code, misleading identifiers, unnecessary abstraction, goroutine and defer misuse, missing tests for changed behavior.

Beyond the checklist, audit comment integrity. Comments are living documentation; a stale comment is worse than none.

Verify before you report. For every drift finding, read the current on-disk text and decide: is it wrong now, or already fixed? Report only what is wrong now, and quote the live text in the finding.

Check, in changed and adjacent code:
- file headers pointing at the wrong context file
- comments describing behavior the code no longer implements
- non-obvious new code missing a "why"
- orphan TODOs, comments that merely restate code, spec pointers to nonexistent docs
- adjacent comments naming a contract this diff changed (importers/importees, moved package boundaries, stale "consumed by Y" cross-refs)

Severity: red = must fix / would mislead an agent now, yellow = should fix / weakened but not yet wrong, green = nit / context worth adding.
Code findings go in \`findings\`; comment-drift findings go in \`commentDrift\` — never mixed.`
}

function dedupePrompt(findings, drift) {
  return `Deduplicate review-panel findings. Merge only — never invent, soften, or drop.

When two findings flag the same issue (same file, same concern), merge them: keep the most specific description, set agreement to the count merged, take the higher severity (red > yellow > green). Keep the surviving finding's lens; on a cross-lens merge, join the lens names with "+". A finding no other lens duplicated passes through unchanged with agreement 1. Keep code findings and commentDrift separate.

Code findings:
${JSON.stringify(findings, null, 1)}

Comment drift:
${JSON.stringify(drift, null, 1)}`
}

function refutePrompt(batch) {
  return `You are a Refuter. One job: disprove each finding below.

Read the live code at each finding's location and along its claimed flow. Attack the claim: does the defect exist in current source? Does a guard, an ordering, or a caller contract prevent the claimed failure? Was it already fixed? The change under review: \`git diff ${diffRef}\`.

Verdict per finding id: REFUTED only with a concrete citation — file, line, and the mechanism that prevents the failure. Anything less is STANDS. Do not soften severities or propose fixes.

Findings:
${JSON.stringify(batch, null, 1)}`
}

// ---- dispatch table ----
// One tracer per flow (cap 3), one verifier per surface (cap 2),
// one adversarial tester when triage flags subtle invariants,
// one data-integrity reviewer when triage flags a write path.
// args.reviewers forces the depth count: truncate, or pad with independent second passes.

function buildLenses(t) {
  const lenses = []
  for (const f of (t.flows || []).slice(0, 3)) {
    lenses.push({ kind: 'tracer', brief: `flow "${f.name}" — entry: ${f.entry}; files: ${(f.files || []).join(', ')}` })
  }
  for (const s of (t.surfaces || []).slice(0, 2)) {
    lenses.push({ kind: 'verifier', brief: `surface: ${s}` })
  }
  if (t.subtleInvariants) {
    lenses.push({ kind: 'adversarial', brief: 'the subtle invariants and edge conditions this slice carries' })
  }
  if (t.dataWrite) {
    lenses.push({ kind: 'dataIntegrity', brief: 'the migrations and row-writing code paths in this slice' })
  }
  if (forcedReviewers) {
    // Thin round-robin across kinds rather than popping the tail. Lenses are
    // pushed in a fixed order — tracers, verifiers, adversarial, data-integrity
    // — so popping dropped whole kinds whenever the earlier ones filled the
    // quota: three flows meant reviewers:3 ran three tracers and no verifier,
    // adversarial, or data-integrity pass at all. A lens panel running one lens
    // is not a smaller panel, it is a different and much weaker review.
    if (lenses.length > forcedReviewers) {
      const byKind = []
      for (const l of lenses) {
        let bucket = byKind.find((b) => b.kind === l.kind)
        if (!bucket) byKind.push((bucket = { kind: l.kind, items: [] }))
        bucket.items.push(l)
      }
      const kept = []
      while (kept.length < forcedReviewers) {
        const before = kept.length
        for (const b of byKind) {
          if (!b.items.length) continue
          kept.push(b.items.shift())
          if (kept.length === forcedReviewers) break
        }
        if (kept.length === before) break // every bucket drained; nothing left to take
      }
      // Never silently. A cap that eliminates a kind changes what the review
      // can find, and the report otherwise lists only the lenses that ran.
      const dropped = byKind.filter((b) => b.items.length).map((b) => `${b.kind}×${b.items.length}`)
      if (dropped.length) log(`reviewers:${forcedReviewers} dropped ${dropped.join(', ')}`)
      lenses.length = 0
      lenses.push(...kept)
    }
    if (!lenses.length) lenses.push({ kind: 'tracer', brief: 'the main code path through this slice' })
    const base = lenses.slice()
    let i = 0
    while (lenses.length < forcedReviewers) {
      const src = base[i % base.length]
      lenses.push({ kind: src.kind, brief: `${src.brief} (independent second pass)` })
      i += 1
    }
  }
  return lenses
}

// ---- run ----

const allPaths = slices.flatMap((s) => s.paths)
log(`Review panel: ${slices.length} slice(s) against ${diffRef}`)

const sliceResults = (await pipeline(
  slices,
  (slice) => agent(triagePrompt(slice), { label: `triage:${slice.name}`, phase: 'Triage', model: depthModel, schema: TRIAGE_SCHEMA }),
  (t, slice) => {
    if (!t) return null
    const lenses = t.mechanical ? [] : buildLenses(t)
    const jobs = lenses.map((l) => () =>
      agent(depthPrompt(l, slice), { label: `${l.kind}:${slice.name}`, phase: 'Review', model: depthModel, schema: FINDINGS_SCHEMA }))
    jobs.push(() =>
      agent(hygienePrompt(slice), { label: `hygiene:${slice.name}`, phase: 'Review', model: 'sonnet', schema: FINDINGS_SCHEMA }))
    return parallel(jobs).then((rs) => ({
      slice,
      triage: t,
      lenses: lenses.map((l) => l.kind).concat('hygiene'),
      reports: rs.filter(Boolean),
    }))
  },
)).filter(Boolean)

// Seam pass: cross-slice flows named by triage. Needs every slice's exit points, so it runs after the pipeline.
let seamReports = []
if (slices.length > 1) {
  const seen = new Set()
  const seamFlows = []
  for (const r of sliceResults) {
    for (const f of r.triage.exitFlows || []) {
      const key = f.name.toLowerCase()
      if (!seen.has(key)) {
        seen.add(key)
        seamFlows.push({ name: f.name, path: f.path, from: r.slice.name })
      }
    }
  }
  if (seamFlows.length > 3) log(`Seam pass capped at 3 of ${seamFlows.length} cross-slice flows`)
  seamReports = (await parallel(seamFlows.slice(0, 3).map((f) => () =>
    agent(seamPrompt(f, allPaths), { label: `seam:${f.name}`, phase: 'Seam', model: depthModel, schema: FINDINGS_SCHEMA })
  ))).filter(Boolean)
}

// Collect raw output from every reviewer.
const rawFindings = []
const rawDrift = []
const praise = []
function collect(reports, sliceName) {
  for (const rep of reports) {
    for (const f of rep.findings || []) rawFindings.push({ ...f, slice: sliceName })
    for (const d of rep.commentDrift || []) rawDrift.push({ ...d, slice: sliceName })
    for (const p of rep.praise || []) praise.push(p)
  }
}
for (const r of sliceResults) collect(r.reports, r.slice.name)
collect(seamReports, 'seam')
log(`${rawFindings.length} raw code findings, ${rawDrift.length} drift findings`)

// Aggregate: semantic dedupe needs judgment, not a file:line key.
let findings = rawFindings.map((f) => ({ ...f, agreement: 1 }))
let drift = rawDrift.map((d) => ({ ...d, agreement: 1 }))
if (rawFindings.length + rawDrift.length > 1) {
  const agg = await agent(dedupePrompt(rawFindings, rawDrift), { label: 'dedupe', phase: 'Aggregate', model: 'sonnet', schema: AGG_SCHEMA })
  if (agg) {
    findings = agg.findings
    drift = agg.commentDrift
  }
}
findings = findings.map((f, i) => ({ ...f, id: i }))

// Refute red/yellow findings; greens skip. REFUTED requires a citation — default is STANDS.
const hot = findings.filter((f) => f.severity !== 'green')
const refuted = []
if (hot.length) {
  // One refuter per file, always. A refuter adjudicating several findings in a
  // file reads that file once instead of once per finding, and REFUTE_SCHEMA
  // already returns a verdict per finding id. Batching only above a threshold
  // made the agent count non-monotonic in findings — eight findings across two
  // files spawned eight agents, while ten across seven spawned seven.
  const byFile = {}
  for (const f of hot) (byFile[f.file] = byFile[f.file] || []).push(f)
  const batches = Object.values(byFile)
  const verdictSets = (await parallel(batches.map((b) => () =>
    agent(refutePrompt(b), { label: `refute:${b[0].file}`, phase: 'Refute', model: depthModel, schema: REFUTE_SCHEMA })
  ))).filter(Boolean)
  const killed = new Map()
  for (const vs of verdictSets) {
    for (const v of vs.results || []) {
      if (v.verdict === 'REFUTED' && v.citation) killed.set(v.id, v.citation)
    }
  }
  for (const f of findings) {
    if (killed.has(f.id)) refuted.push({ ...f, refutation: killed.get(f.id) })
  }
  findings = findings.filter((f) => !killed.has(f.id))
  log(`Refutation dropped ${refuted.length} of ${hot.length} red/yellow findings`)
}

return {
  target,
  diffRef,
  panels: sliceResults.map((r) => ({ slice: r.slice.name, mechanical: !!r.triage.mechanical, lenses: r.lenses })),
  seamFlowsTraced: seamReports.length,
  findings,
  commentDrift: drift,
  refuted,
  praise,
}
