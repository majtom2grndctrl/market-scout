# Work-Unit Dedup Edges: Review and Proposal

> **Status:** draft proposal. Nothing here is implemented. Numbers are from the live database on 2026-09-21 (10,316 postings, 9,619 with a latest-snapshot description, 107 companies, 3,731 unclassified).
> **Implementation of record:** `apps/tools/internal/db/queries/enrich.sql` (header comment states the current algorithm).
> **Related:** `.claude/skills/batch-enrich/SKILL.md` §3 (rationale and failure history), `agent-output/batch-enrich/2026-09-21-0924.md` §7 (the reported misses).

## Goal

Decide, with evidence, which changes to the work-unit rule are worth their false-merge risk. The last change in this area shipped a silent over-merge (requisition key alone). A silent over-merge is worse than the inconsistency dedup exists to prevent: three jobs get one answer instead of one job getting three visibly different ones. Every proposal below is judged against that bar first and against missed merges second.

## Headline

Ship two small changes to the text edge. Keep every other miss.

| # | Change | Backlog effect (3,731 unclassified postings) | Confidence |
|---|---|---|---|
| 1 | Hash description text with all whitespace removed, not raw bytes | 3,518 → 3,487 work units (31 fewer reads); catches Harvey 50603/50609 | High |
| 2 | Gate the text edge on a matching title *level signature* | +7 units (3,487 → 3,494); stops writing `staff` onto a `Senior` posting | Medium-high |
| — | Do not add a near-duplicate (trigram similarity) edge | would touch 241 unclassified postings; risk analysis below says no | High that it should not ship now |
| — | Do not strip regional suffixes from titles | would wrongly merge 471 of 498 candidate groups | High |
| — | Do not add a title-only edge | 626 groups, 1,832 postings, mostly distinct jobs | High |

Net: 24 fewer classifier reads on the current backlog and one live silent over-merge closed. The payoff is small. The point is correctness in the direction the failure history says matters.

## Scope

### In scope

- The two edges in `ListUnclassifiedPostings` / `ListUnclassifiedPostingsForced` and their gates and normalization.
- Test fixtures and a before/after shadow comparison over real data.
- Comment and skill-prose corrections where the current text is wrong about the data.

### Out of scope

- Re-classifying postings already written under the over-merge (listed under *Follow-on* with the query that finds them).
- Changing what `description_text` the fetcher stores. Normalization happens at comparison time.
- Any change to the requisition edge. It is correctly gated and no evidence here says otherwise.
- Cross-company edges.

## 1. What the current rule actually misses

Measured against the latest snapshot per posting, company-scoped, as the rule is.

### 1a. Whitespace-only text differences (confirmed miss)

Harvey 50603 and 50609 ("Law Schools Founding CSM", San Francisco and Dallas) differ by exactly one byte: `Compensation$160,000` vs `Compensation $160,000` (hex `6e2431` vs `6e2024`). The difference is present in all six snapshot pairs since 2026-07-20, so it is a source-side HTML difference, not drift. Same team, same department, same publish minute. One job.

Corpus-wide, hashing with all whitespace removed (`[[:space:]]`, U+00A0, U+200B, U+FEFF):

| Measure | Raw md5 | Whitespace-stripped md5 |
|---|---|---|
| Text units, whole corpus | 8,808 | 8,741 |
| Units that absorb >1 raw group | — | 67 (134 raw groups, 268 postings) |
| ...spanning >1 normalized title | — | 17 of 67 (already the text edge's behaviour today; see §3) |
| Work units, unclassified backlog (full algorithm) | 3,518 | 3,487 |

The shape is concentrated at Harvey and OpenAI (Ashby) as one-posting-per-location pairs; 25 of the 67 units have at least one unclassified member. This is the only miss with a self-evidencing fix.

### 1b. Text that drifted between snapshots (mostly not a miss)

3,790 of 9,619 postings (39%) have more than one distinct description text across their snapshots. Most of that is extraction-wide: Harvey's lengths moved 4,625 → 4,593 for *both* twins on 2026-08-05, which changes nothing about their equality.

The case that matters is "were once identical, are not now". Counting pairs whose latest hashes differ but which shared a hash in some earlier snapshot:

| | Pairs |
|---|---|
| All such pairs | 2,507 |
| ...inside Snap! Raise's 122-posting "Independent Sales Representative" clique | ~1,700 (already one work unit today via text equality among the current majority) |
| Same normalized title, outside that clique | ~123 |
| ...already rescued by the requisition edge (Greenhouse) | 20 |

Residual: on the order of 100 pairs corpus-wide, about 1% of postings, concentrated at Ashby where no requisition key exists. Exact count blocked by the 5-second query cap on hashing 87K snapshots; the estimate is from eight company-sliced runs. Tolerate (see §5).

### 1c. Trailing regional suffixes in titles (not a miss — a trap)

498 company-scoped title groups collapse to one base title after stripping a trailing parenthetical or ` - X` suffix (2,032 postings).

| Texts inside the group | Groups | What it means |
|---|---|---|
| Exactly one | 27 | Already merged by the text edge; a title rule adds nothing |
| Several | 471 (1,963 postings) | Different bodies under regional titles |
| Exactly one text per title | 324 | Each region has its own description |

Every example cited as a miss on 2026-09-21 is in the second row and has different text: ElevenLabs "Revenue Partnerships - Southern Europe" vs "- France" (similarity 0.984, different territory language), "General Manager - Italy" vs "- Brazil" (0.901), the Sweden "Financial Services / Retail / Industrials" trio (6,428 / 6,377 / 6,471 chars). The reported fix ("strip a trailing parenthetical before the requisition-key gate") would also have been a no-op for every cited case: Hiya, ElevenLabs and Harvey are Ashby boards, and **Ashby carries no requisition key in this database** (0 of 3,288 postings; Lever 0 of 503; Greenhouse 5,800 of 5,820). The `enrich.sql` header's "at Greenhouse and Ashby the requisition key identifies a job family" is wrong about Ashby; the requisition edge only ever fires on Greenhouse and Gem. Correct the comment.

Hiya "Account Executive (Munich)" vs "(Berlin)" (similarity 0.9966; city name in the body, 0x20 apart in length) is the same shape as ElevenLabs Southern Europe/France, which the operator has adjudicated as different jobs. Treat them the same way: territory in the title is a distinct posting; consistency across them is the classifier's job, not dedup's.

### 1d. Near-identical same-title text (real, sizeable, and not safely mergeable)

Same company, same normalized title, not whitespace-equal, length within 300 chars, not already joined by the requisition edge, Snap! Raise excluded:

| Trigram similarity | Pairs | Postings | Unclassified postings |
|---|---|---|---|
| ≥ 0.98 | 411 | 570 | 241 |
| ≥ 0.99 | 204 | | |

Concentrated: Anthropic 94 pairs, Harvey 89, OpenAI 73, Stripe 62, LiveKit 18, Chainguard 14. This is the largest miss by volume. §3 explains why it stays a miss.

### 1e. Cross-chunk (not a dedup miss)

Harvey 50660/50662/50663 "Technical Program Manager, Customer Trust": 50660 and 50662 are byte-identical and were one unit. 50663 omits the compensation line (0.985). It is case 1d, not an orchestration split.

## 2. Proposed change

Two edits to `candidate` and the text edge, applied identically to both query variants. Everything else in the query is unchanged: company scoping, the requisition edge and its title gate, the recursive closure, representative choice, limit semantics.

```sql
-- Proposed design (candidate CTE)
md5(regexp_replace(s.description_text, '[[:space:] ​﻿]+', '', 'g')) AS text_hash,
lower(regexp_replace(btrim(s.title), '\s+', ' ', 'g'))                            AS norm_title,
coalesce((
    SELECT string_agg(m[1], ',' ORDER BY m[1])
    FROM regexp_matches(
        lower(regexp_replace(btrim(s.title), '\s+', ' ', 'g')),
        '\m(senior|sr|staff|principal|lead|junior|jr|associate|intern|director|head|vp|vice president|manager|chief|distinguished|founding)\M',
        'g') m
), '') AS level_sig

-- Proposed design (text edge)
JOIN candidate b ON b.company_id = a.company_id
                AND b.text_hash  = a.text_hash
                AND b.level_sig  = a.level_sig
                AND b.posting_id <> a.posting_id
```

Definitions:

- **Whitespace-stripped hash.** Remove every whitespace code point before hashing. Stripping (not collapsing) is required: the Harvey pair differs by a space present on one side and absent on the other.
- **Level signature.** The sorted multiset of level words found in the normalized title, joined with commas; empty when none. `Staff Product Security Engineer` → `staff`; `Applied AI Architect - Tokyo` → empty; `Senior/Staff Software Engineer` → `senior,staff`; `Head of GTM Enablement - Global Lead` → `head,lead`.
- **Text edge gate.** Identical stripped text merges only under an identical level signature. Titles that differ in location, segment, punctuation or wording still merge when the text is identical. Titles that differ in a level word never merge on text alone.

The requisition edge already requires identical normalized titles, so it is unaffected by the gate.

### Why the gate belongs on the text edge

The header's premise "identical text always classifies identically" is false when the title is also an input and carries the level. Under batch-enrich-v6, step 1 of seniority is the title level word. Identical text under `Senior X` and `Staff X` must classify differently, and today it does not:

- Chainguard: postings 6211, 6215, 62358, 62362 (`Staff Product Security Engineer`), 34701 (`Senior Product Security Engineer`) and 27789 (`Principal Product Security Researcher`) share one description hash. **34701 is recorded as `staff`.** The Senior posting took the Staff representative's answer.
- 23 identical-text units in the corpus span more than one level signature (~70 postings, 20 unclassified): Stripe `Product Manager` / `Staff Product Manager, Local Processor Acquiring`; Mistral `Commercial Legal Counsel` / `Senior ...`; Anthropic `Engineering Manager` / `Tech Lead Manager, Agent Runtime Platform`; Conversica `Senior` / `Staff Backend Platform Engineer` over a 688-char boilerplate stub; OpenAI `Engineering Manager, MLE` / `Machine Learning Engineer, Integrity`.

The gate turns those 23 units into 53. On the current backlog it costs 7 extra reads.

## 3. False-merge risk analysis

For each edge that would exist after the change, and each edge considered and not adopted: what it would wrongly merge, how often that case occurs here, and what gates it.

### 3a. Text edge with whitespace-stripped hash (proposed)

- **Wrong merge it could add over today:** two different jobs whose descriptions differ only in whitespace placement. Not observed. The 67 newly merged units are one-per-location pairs and reposts; the 17 spanning titles span them exactly as the raw-hash edge already does.
- **Wrong merge it inherits from today:** identical text under different titles. 138 units, 496 postings, 127 unclassified. Three sub-shapes:
  - *Location or segment in the title* (OpenAI `Applied AI Architect - Tokyo`, Place Technology `Transaction Coordinator (Alaska)`, Stripe `AE, Product Sales - Billing`): the fan-out dedup exists for. Correct merge.
  - *Level word differs* (23 units): silent seniority over-merge. Closed by the level gate.
  - *Role differs, same level signature* (Stripe `Analytics Engineer` / `Data Analyst, Payments Performance`; super.AI `Marketing Data Specialist` / `Talent Acquisition Specialist`; Harvey `Premium Support Specialist` / `Technical Account Manager`; roughly 6–8 units, mostly Lever boards whose description is a company boilerplate stub): remains merged. Tolerated because a stricter gate (identical base title) would split 85 units and add 108 reads to fix these few, and because the audit query in §4 surfaces them.
- **Gate:** company scope; level signature.

### 3b. Level-signature gate (proposed)

The gate can only split, never merge, so it has no false-merge risk. Its failure mode is a *missed* merge: one job listed per location under titles that differ in a level word (`Senior/Staff SWE` vs `Senior SWE`). Not observed in the corpus. Cost if it happens is one extra read and a visible inconsistency, the original failure, not the silent one. `manager`, `lead` and `head` are in the word list deliberately: they split an IC from its manager over shared text (OpenAI EM/MLE, Anthropic EM/TLM), and the conservative direction is to split. Numeric levels (`II`, `III`, `L5`) are not handled; none appear in the multi-title units.

### 3c. Near-duplicate edge: same title + trigram similarity ≥ 0.98 (considered, not adopted)

This is the edge that would recover the biggest miss (241 unclassified postings). It is not adopted because the band above 0.98 contains different jobs interleaved with same jobs, and no threshold separates them:

| Pair | Sim | What differs | Same job? |
|---|---|---|---|
| Anthropic 412/415 `Applied AI Architect, Industries` | 0.986 | `$240,000—$315,000 USD` vs `£150,000—£190,000 GBP` | Yes (per-country band) |
| Harvey 50660/50663 `TPM, Customer Trust` | 0.985 | compensation line present vs absent | Yes |
| OpenAI 51005/91718 `FDE - NYC` | 0.993 | spacing, `behaviour`/`behavior` | Yes (repost) |
| Anthropic 386/67179 `Applied AI Architect` | 0.984 | Japan: native Japanese required, `5+` years; Singapore: `10+` years | **No** (skill differs) |
| Harvey 870/68906 `AE, Mid Market, EMEA` | 0.982 | Spanish fluency required in one | **No** (skill differs) |
| Stripe 1216/1219 `AE, AI Sales` | 0.994 | `6+` vs `13+` years | Doubtful |
| LiveKit 25542/46976 `Forward Deployed Engineer` | 0.996 | `7+` vs `5+` years | Doubtful |
| ElevenLabs 50022/50027 `Revenue Partnerships - S. Europe / - France` | 0.984 | territory | No (operator-adjudicated; title differs, so a title gate excludes it) |

Three of the twelve sampled ≥0.98 same-title pairs carry a requirement difference the classifier is supposed to read (a language skill). A language-requirement variant merged with its sibling drops or invents a skill silently. That is the requisition-key failure again in a new coat.

Supporting distribution (same title, whitespace-different, four company slices covering about half the corpus): ≥0.99: 202 pairs; 0.97–0.99: 251; 0.95–0.97: 53; 0.90–0.95: 62; <0.90: 114. There is no gap to put a threshold in. A rule whose boundary sits at 0.98 vs 0.975 cannot be verified by a human reading two postings, which the byte-equality rule can.

Two more reasons: pg_trgm `similarity()` over 5 KB texts is expensive enough that a per-company all-pairs pass over the backlog took more than 5 s per eighth of the corpus here; and the closest thing to ground truth, Greenhouse same-title pairs with *different* requisition keys, has 202 pairs at ≥0.98 against 136 that are whitespace-identical — the platform itself does not treat these as one job.

- **Wrong merge:** language, years, or one-paragraph scope variants under one title.
- **Frequency here:** roughly a quarter of sampled ≥0.98 pairs; hundreds of pairs corpus-wide.
- **Gate that would make it safe:** none found. Title equality removes the territory cases but not the language/years cases.

### 3d. Compensation-amount normalization (considered, not adopted)

Idea: strip currency amounts and codes before hashing, since v6 forbids reading seniority off compensation. Measured: it changes the backlog unit count by **zero** (Greenhouse per-location comp variants are already joined by the requisition edge), and where it merges across titles it merges `Senior Software Engineer, Inference` / `Staff Software Engineer, Inference` (Anthropic), `Senior` / `Staff Software Engineer, Agents` (Harvey), `Delivery Consultant` / `Senior Delivery Consultant` (Airtable), `Manager` / `Senior Manager` (Scale AI). The comp band is how boards distinguish levels over a shared description. Not classification-neutral in effect.

### 3e. Title-only edge: same company + same normalized title (considered, not adopted)

626 groups, 1,832 postings, 625 unclassified; 58 groups hold four or more distinct texts, one holds 17. Same-title different-text similarity goes down to 0.66 in the sample (Scale AI `Engagement Manager`, Harvey `Senior Product Security Engineer` at 0.727). This is the Stripe Staff SWE problem by another route.

### 3f. Suffix-stripped title (considered, not adopted)

§1c. 471 of 498 candidate groups have differing text. Would merge ElevenLabs GM Italy/Brazil at 0.90. Also a no-op as a requisition-gate relaxation because the cited boards have no requisition key.

### 3g. Historical-identity edge: were once byte-identical, same title now (considered, not adopted)

Recovers ~100 pairs (§1b). Requires hashing every snapshot (87K rows, 916 MB) on each selection, or a new persisted column. The Snap! Raise clique shows how a historical hash degenerates: 122 postings share several past hashes in overlapping waves. Payoff does not justify a third edge and a schema change.

### 3h. Punctuation-folded title normalization (considered, not adopted)

Folding `&`→`and` and stripping punctuation in the requisition gate would newly connect 1 of 796 title-differing same-requisition pairs, and that one already shares text. No effect.

## 4. Validation before shipping

### Fixtures

Extend `apps/tools/internal/db/enrich_selection_dedup_integration_test.go` (same seeding helpers, one marker per test, both query variants asserted equal):

| Fixture | Expect |
|---|---|
| Bodies `...Compensation$160,000...` and `...Compensation $160,000...`, same title | one unit |
| Bodies differing by U+00A0 vs U+0020 and by a trailing newline | one unit |
| Identical body, titles `Staff Product Security Engineer` / `Senior Product Security Engineer` / `Principal Product Security Researcher` | three units |
| Identical body, titles `Applied AI Architect` / `Applied AI Architect - Tokyo` / `Applied AI Architect, Enterprise` | one unit |
| Identical body, titles `Product Manager, X` / `Staff Product Manager, X` | two units |
| Chain: A(body X, `FDE`) — B(body X, `fde - emea`, req R) — C(body X′, `FDE - EMEA`, req R) | one unit (gate does not break closure) |
| Identical body, `Senior/Staff Software Engineer` in both | one unit (`senior,staff` = `senior,staff`) |

### Shadow comparison over real data

Run old and new component assignment side by side (read-only; run through `psql`, not the 5-second MCP `query` tool) and diff per posting:

- Every newly merged pair has equal whitespace-stripped text. Assert, do not assume.
- Every newly split pair has different level signatures. List them with titles for eyeball review; expected ~23 units corpus-wide, ~7 in the backlog.
- Expected counts: backlog 3,518 → 3,494 units; corpus multi-title identical-text units 138 → 168 (23 become 53).
- Post-change invariant query: no work unit contains two distinct level signatures. Add it to the run report's dedup line if cheap.

### Preview parity

`enrichment_preview` with `count: 500`, before and after: `work_unit_count` and `selected_count` move by the shadow's predicted delta and no unit is split across the limit.

### Ongoing audit for the tolerated misses (not a selection edge)

A report-time query, run after a batch, listing same-company same-title pairs at similarity ≥ 0.98 whose classifications disagree on seniority or role, plus identical-text units whose titles differ in base title. Output goes to the run report as consistency candidates for a same-chunk `--force` re-run. This converts the residual miss into a visible finding instead of a silent merge.

## 5. Considered and rejected, with the "change nothing" option

| Miss | Tolerate? | Why |
|---|---|---|
| 1a whitespace | No — fix | Self-evidencing, zero added risk, 31 reads on the backlog, exact reported case |
| 1b drift twins | Yes | ~1% of postings; only fix is a historical hash or a similarity rule; Greenhouse already rescued |
| 1c regional suffix | Yes | The "miss" is mostly distinct jobs; the fix merges ElevenLabs GMs; a no-op on Ashby |
| 1d near-identical | Yes, for now | Largest miss, but language/years variants sit above 0.98; no gate found; audit instead |
| 1e cross-chunk | n/a | It was 1d |
| existing level over-merge | No — fix | Concrete harm (34701); costs 7 reads |
| existing same-level role over-merge | Yes | 6–8 units; stricter gate costs 108 reads; audit surfaces it |

"Change nothing" loses the whitespace fix and leaves 34701's shape open. Both fixes are additive to the existing self-evidencing rule and neither adds a judgement threshold. That is the bar.

## Acceptance criteria

- [ ] Both selection variants hash description text with all whitespace removed, and a fixture with a single-space difference lands in one work unit.
- [ ] Both selection variants gate the text edge on an identical title level signature, and a fixture with identical text under `Staff` and `Senior` titles lands in two work units.
- [ ] A fixture with identical text under a bare title and a location-suffixed title still lands in one work unit.
- [ ] The transitive-closure fixture still collapses a text edge chained to a requisition edge into one unit under the new gate.
- [ ] The forced variant produces the same unit assignment as the unforced variant over the same fixtures.
- [ ] A shadow comparison over the whole corpus shows every newly merged pair whitespace-equal and every newly split pair level-different, with counts recorded in this plan before it moves to `in-progress/`.
- [ ] The `enrich.sql` header no longer claims Ashby carries a requisition key and states the text-edge premise as "identical text under a level-compatible title".
- [ ] Both `batch-enrich` skill copies (`.claude/` and `.agents/`) describe the two edges as amended in §3 of their Step 3 rationale.
- [ ] The durable rule (work unit = component over a company-scoped text edge gated on level and a requisition edge gated on title; title level words are classification input) is captured in `agent-context/lib/project.md` §Settled architecture before this plan is promoted to `ready/`.

## Tasks

### Task 1: Amend the selection query

- Change `text_hash` in both `candidate` CTEs to the whitespace-stripped hash; the Harvey pair differs by one space, so collapsing is not enough.
- Add `level_sig` to both `candidate` CTEs using the word list in §2; sorted and comma-joined so `senior/staff` and `staff/senior` compare equal.
- Add `b.level_sig = a.level_sig` to the text edge in both variants; the requisition edge already gates on the full title and is unchanged.
- Keep the two variants byte-identical in their dedup body; the integration test asserts parity and a drift here is the failure Step 3 of the skill exists to prevent.
- Regenerate sqlc output per `agent-context/lib/developer-guide.md` §5.8; `DedupKey` and `IsRepresentative` semantics are unchanged, so `internal/enrich/selection` needs no code change.
- Rewrite the header paragraph on edges: Ashby and Lever expose no requisition key (project.md §ATS targets), so the requisition edge fires only on Greenhouse and Gem; identical text classifies identically only when the title level agrees.

Do not:
- Add a similarity, suffix, title-only or historical edge.
- Touch the requisition edge's gate.
- Hand-edit `apps/tools/internal/db/enrich.sql.go`.

### Task 2: Fixtures

- Add the seven fixtures in §4 to `enrich_selection_dedup_integration_test.go`, reusing `seedDedupPostings` and `dedupUnits`; the marker rides in the body so the focus prefilter isolates each test.
- Assert forced-variant parity in at least the level-gate test, mirroring the existing requisition-gate test.

### Task 3: Shadow comparison and prose

- Run the old-vs-new comparison in §4 over the full corpus and record the four counts in this plan.
- Update Step 3 of both skill copies: add the whitespace and level-gate bullets, and correct the Ashby requisition claim.
- Capture the durable rule in `project.md` §Settled architecture (one paragraph; no SQL, no column names).

Coordination: `agent-context/plans/ready/codex-native-batch-enrichment/` Task 1 also edits `enrich.sql` (posting-id tie breaker). Whichever lands second rebases; both variants must stay identical after both land.

## Follow-on, once this ships

- **Retro-correct the over-merge victims.** Classified postings whose title level word disagrees with their recorded seniority and whose latest text is shared with a posting of a different level signature. Chainguard 34701 is the first row. A `--force` re-run per posting, provenance intact.
- **Consistency audit in the run report** (§4, last block): surfaces 1d without merging it. Decide after two or three runs whether the divergence rate justifies revisiting a gated near-duplicate rule with a human-verifiable definition, not a threshold.
- **Persisted normalized hash.** If selection latency grows with the corpus, a generated column on `posting_snapshots` removes the per-selection regex. Not now.

## Open questions

- Is `manager` a level word for this purpose? Keeping it splits IC/manager pairs over shared text (safe direction) but also splits `Account Manager` from `Account Executive` when a board reuses one description, which is arguably also right. Default: keep.
- Should the level-signature check also feed the classifier prompt as a sanity assertion (title level word must not contradict `seniority`)? Out of scope here; noted for the v7 contract.

## Confidence

| Recommendation | Confidence | What would change it |
|---|---|---|
| Whitespace-stripped hash | High | Nothing found; the only observed effect is the intended one |
| Level-signature gate | Medium-high | Word list is heuristic; if the shadow diff shows a fan-out split by a level word, narrow the list |
| No similarity edge now | High | A gate that separates language/years variants from comp/location variants without a numeric threshold |
| Keep suffix, title-only, historical, comp, punctuation misses | High | New evidence of a self-evidencing key for Ashby fan-outs (none in the raw payload today: only a per-posting `id`) |
| Size of the tolerated 1d miss | Medium | 241 unclassified postings have a ≥0.98 same-title partner; the fraction that are truly one job is unmeasured beyond a 12-pair sample |
