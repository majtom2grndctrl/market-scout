# Work-Unit Dedup Edges: Review and Proposal

> **Status:** parked 2026-09-22, not implemented. Superseded by the move toward a non-LLM enrichment tool, which can classify every posting on its own, so work units no longer need to share one model answer. Kept for its findings on what counts as the same job (whitespace-only twins, the regional-suffix trap, Ashby carrying no requisition keys) and for the whitespace-normalized text hash, which is input to that tool's spec. Only the Claude skill path still shares answers across a work unit. Numbers are from the live database on 2026-09-21 (10,316 postings, 9,619 with a latest-snapshot description, 107 companies, 3,731 unclassified), re-verified on 2026-09-22 against the recency-first selection query; no fetch or classification landed in between.
> **Implementation of record:** `apps/tools/internal/db/queries/enrich.sql` (header comment states the current algorithm).
> **Related:** `.claude/skills/batch-enrich/SKILL.md` Step 3 (rationale and failure history), `agent-output/batch-enrich/2026-09-21-0924.md` §7 (the reported misses), `apps/tools/internal/db/migrations/000036_title_dimension.up.sql` (the title-rank vocabulary this plan reuses), `agent-context/lib/developer-guide.md` §6.2 (prompt-version pin register).

## Goal

Decide, with evidence, which changes to the work-unit rule are worth their false-merge risk. The last change in this area shipped a silent over-merge (requisition key alone). A silent over-merge is worse than the inconsistency dedup exists to prevent: three jobs get one answer instead of one job getting three visibly different ones. Every proposal below is judged against that bar first and against missed merges second.

## Headline

Ship two small changes to the text edge. Keep every other miss.

| # | Change | Backlog effect (3,731 unclassified postings) | Confidence |
|---|---|---|---|
| 1 | Hash description text with all whitespace removed, not raw bytes | 3,518 → 3,487 work units (31 fewer reads); catches Harvey 50603/50609 | High |
| 2 | Gate the text edge on a matching title *rank signature*, read from the read model's title-rank vocabulary | +6 units (3,487 → 3,493); stops writing one rank onto postings whose titles state another (Outreach 47087/47089: `Staff` titles recorded `senior`) | Medium-high |
| — | Do not add a near-duplicate (trigram similarity) edge | would touch 241 unclassified postings; risk analysis below says no | High that it should not ship now |
| — | Do not strip regional suffixes from titles | would wrongly merge 471 of 498 candidate groups | High |
| — | Do not add a title-only edge | 626 groups, 1,832 postings, mostly distinct jobs | High |

Net: 25 fewer classifier reads on the current backlog and one live silent over-merge closed. The payoff is small. The point is correctness in the direction the failure history says matters.

Read savings and the over-merge both exist only where a harness classifies one representative and copies the answer to siblings. The Claude skill does. The Codex skill does not: it dispatches every selected posting to a worker individually, so work units shape its cohort (how many postings a `count` returns) but never copy a classification.

## Scope

### In scope

- The two edges in `ListUnclassifiedPostings` / `ListUnclassifiedPostingsForced` and their gates and normalization.
- A migration adding one SQL function that projects the title-rank vocabulary into the gate's signature.
- Test fixtures and a before/after shadow comparison over real data.
- A `PROMPT_VERSION` bump in both batch-enrich skills, which their pin rule requires for any change to selection semantics.
- Comment and skill-prose corrections where the current text is wrong about the data.

### Out of scope

- Re-classifying postings already written under the over-merge (listed under *Follow-on* with the query that finds them).
- Changing what `description_text` the fetcher stores. Normalization happens at comparison time.
- Any change to the requisition edge. It is correctly gated and no evidence here says otherwise.
- Cross-company edges.
- Changing `open_posting_titles` or `title_seniority_seeds`. The gate reads the seeds; it does not edit them.
- Selection ordering, the per-company cap, and the wave CTEs (`grouped`, `capped`, `waved`). Grouping happens before them; they consume `dedup_key` unchanged.

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

**Cost.** Stripping is not free. Over the full corpus (the forced variant's candidate set, 9,619 descriptions) the raw md5 pass takes 0.35 s; the regex-stripped pass takes 4.7 s and a `translate`-based strip 3.5 s (measured 2026-09-22). The unforced backlog (3,731) runs the whole selection, gate included, inside 5 s. `enrichment_preview` sets no statement timeout, so a slower forced preview completes, but it is roughly ten times slower. See *Follow-on*.

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

Every example cited as a miss on 2026-09-21 is in the second row and has different text: ElevenLabs "Revenue Partnerships - Southern Europe" vs "- France" (similarity 0.984, different territory language), "General Manager - Italy" vs "- Brazil" (0.901), the Sweden "Financial Services / Retail / Industrials" trio (6,428 / 6,377 / 6,471 chars). The reported fix ("strip a trailing parenthetical before the requisition-key gate") would also have been a no-op for every cited case: Hiya, ElevenLabs and Harvey are Ashby boards, and **Ashby carries no requisition key in this database** (0 of 3,288 postings; Lever 0 of 503; Greenhouse 5,800 of 5,820; Gem 0 of 8). The `enrich.sql` header's "at Greenhouse and Ashby the requisition key identifies a job family" is wrong about Ashby. Per `project.md` §ATS targets, only Greenhouse, Workday, Workable and Gem expose a requisition key; Workday stores no description and never reaches selection, so in the current data the requisition edge fires on Greenhouse only. Correct the comment.

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

Two edits to the `candidate` and `edge` CTEs, applied identically to both query variants, plus one migration. Everything else in the query is unchanged: company scoping, the requisition edge and its title gate, the recursive closure (`reach`, `unit`), representative choice, the ordering and per-company cap CTEs, limit semantics.

```sql
-- Proposed design (new migration): the gate's projection of the title-rank vocabulary
CREATE FUNCTION title_rank_signature(title text) RETURNS text
LANGUAGE sql STABLE AS $$
    SELECT coalesce(string_agg(seed.slug, ',' ORDER BY seed.slug), '')
    FROM (
        SELECT title_seniority_seeds.slug,
               '(?<![[:alnum:]])(?:'
                   || string_agg(pattern.value, '|' ORDER BY length(pattern.value) DESC, pattern.value)
                   || ')(?![[:alnum:]])(?!\s+(?:of|and|to|for|the)\y)(?!\s*&)' AS expression
        FROM title_seniority_seeds
        CROSS JOIN LATERAL unnest(title_seniority_seeds.patterns) AS pattern(value)
        GROUP BY title_seniority_seeds.slug
    ) AS seed
    WHERE title ~* seed.expression
$$;

-- Proposed design (candidate CTE, both variants)
md5(regexp_replace(s.description_text, '[[:space:] ​﻿]+', '', 'g')) AS text_hash,
lower(regexp_replace(btrim(s.title), '\s+', ' ', 'g'))                              AS norm_title,
title_rank_signature(lower(regexp_replace(btrim(s.title), '\s+', ' ', 'g')))        AS rank_sig,

-- Proposed design (text edge, both variants)
JOIN candidate b ON b.company_id = a.company_id
                AND b.text_hash  = a.text_hash
                AND b.rank_sig   = a.rank_sig
                AND b.posting_id <> a.posting_id
```

Definitions:

- **Whitespace-stripped hash.** Remove every whitespace code point before hashing. Stripping (not collapsing) is required: the Harvey pair differs by a space present on one side and absent on the other. Spell the non-ASCII code points as `\uXXXX` regex escapes, never as literal invisible characters. `[[:space:]]` is locale-dependent: on this database (`en_US.utf8`) it matches U+2009 but not U+00A0 or U+200B, so those must be listed explicitly.
- **Rank signature.** The sorted set of `title_seniority_seeds` slugs whose pattern matches the normalized title, comma-joined; empty when none. Patterns map synonyms onto one slug, so `Sr.` and `Senior` agree, as do `VP` and `Vice President`. `Staff Product Security Engineer` → `staff`; `Applied AI Architect - Tokyo` → empty; `Senior/Staff Software Engineer` → `senior,staff`; `Head of GTM Enablement - Global Lead` → `head,lead`; `Software Engineer, New Grad` → `junior`.
- **Text edge gate.** Identical stripped text merges only under an identical rank signature. Titles that differ in location, segment, punctuation or wording still merge when the text is identical. Titles that differ in a stated rank never merge on text alone.

The requisition edge already requires identical normalized titles, so it is unaffected by the gate. Both edges therefore preserve the signature, and no work unit can contain two signatures by construction.

### Sharing boundary with the title dimension

Migration 000036 already derives title seniority in the read model from `title_seniority_seeds`. The gate reuses that vocabulary. It does not reuse the view.

| | `open_posting_titles.title_seniority` | Gate signature |
|---|---|---|
| Vocabulary | `title_seniority_seeds` | `title_seniority_seeds` (shared) |
| Projection | Highest-rank slug, read from peeled rank text | Set of every matched slug, read from the whole normalized title |
| Postings covered | Open postings only | Every posting with a latest description: the unforced backlog includes closed postings, and the forced variant covers all 9,619 |
| Title read | Snapshot from the run that established openness | Latest snapshot, as every other selection column |

The projections differ on purpose. A dual rank must stay distinguishable for the gate (`senior,staff` is not `staff`), while the dimension resolves it to one value. The coverage and snapshot choice differ too, so joining the view would silently drop closed postings from the gate.

One vocabulary, two projections. A rank added to the seeds reaches both. The function keeps the gate's projection in one definition for both query variants, beside the table it reads. The regex shape mirrors 000036's `seniority_patterns` CTE; the view keeps its own copy for now (see *Follow-on*).

Measured against the draft's original hand-written word list, the seed vocabulary splits the same 23 identical-text units, gives 52 sub-units instead of 53, and yields 3,493 backlog units instead of 3,494. The differences are the synonym collapses above, `new grad` → `junior`, and `founding`, which is not a rank in the seeds or in the classifier contract.

### Why the gate belongs on the text edge

The header's premise "identical text always classifies identically" is false when the title is also an input and carries the level. Under the current classifier contract (Step 1 of the seniority ladder in both batch-enrich skills), a level word in the title is authoritative. Identical text under `Senior X` and `Staff X` must classify differently, and today it does not:

- Outreach: postings 47064 (`Senior Applied Scientist - Knowledge Graphs & AI`), 47087 (`Staff Applied Scientist - Knowledge Graphs & AI`) and 47089 (`Staff Data Scientist (LLM, GenAI, MLOps, LLMOps and Python)`) share one description hash. The Claude skill wrote all three within five seconds on 2026-09-21 as one work unit. **47087 and 47089 are recorded as `senior`.** The Staff postings took the Senior representative's answer.
- 23 identical-text units in the corpus span more than one rank signature (77 postings, 18 unclassified): Stripe `Product Manager` / `Staff Product Manager, Local Processor Acquiring`; Mistral `Commercial Legal Counsel` / `Senior ...`; Anthropic `Engineering Manager` / `Tech Lead Manager, Agent Runtime Platform`; Conversica `Senior` / `Staff Backend Platform Engineer` over a 688-char boilerplate stub; OpenAI `Engineering Manager, MLE` / `Machine Learning Engineer, Integrity`; Chainguard `Staff` / `Senior Product Security Engineer` / `Principal Product Security Researcher`.

Chainguard 34701 (`Senior Product Security Engineer`, recorded `staff`) is not an over-merge victim. It was classified alone on 2026-09-13, before dedup existed, and its body names the Staff level. It shows the shape, not the harm.

The gate turns those 23 units into 52. On the current backlog it costs 6 extra reads.

## 3. False-merge risk analysis

For each edge that would exist after the change, and each edge considered and not adopted: what it would wrongly merge, how often that case occurs here, and what gates it.

### 3a. Text edge with whitespace-stripped hash (proposed)

- **Wrong merge it could add over today:** two different jobs whose descriptions differ only in whitespace placement. Not observed. The 67 newly merged units are one-per-location pairs and reposts; the 17 spanning titles span them exactly as the raw-hash edge already does.
- **Wrong merge it inherits from today:** identical text under different titles. 138 units, 496 postings, 127 unclassified. Three sub-shapes:
  - *Location or segment in the title* (OpenAI `Applied AI Architect - Tokyo`, Place Technology `Transaction Coordinator (Alaska)`, Stripe `AE, Product Sales - Billing`): the fan-out dedup exists for. Correct merge.
  - *Rank differs* (23 units): silent seniority over-merge. Closed by the rank gate.
  - *Role differs, same rank signature* (Stripe `Analytics Engineer` / `Data Analyst, Payments Performance`; super.AI `Marketing Data Specialist` / `Talent Acquisition Specialist`; Harvey `Premium Support Specialist` / `Technical Account Manager`; Highspot `Manager, Solution Consulting Operations` / `Technical Account Manager`; roughly 6–8 units, mostly boards whose description is a company boilerplate stub): remains merged. Tolerated because a stricter gate (identical base title) would split 85 units and add 108 reads to fix these few, and because the audit query in §4 surfaces them. The harm is live: on 2026-09-21 the Claude skill wrote `technical-account-manager` onto five Harvey postings including two Premium Support Specialists, and `frontend-engineer` onto Highspot's Technical Account Manager.
- **Gate:** company scope; rank signature.

### 3b. Rank-signature gate (proposed)

The gate can only split, never merge, so it has no false-merge risk. Its failure mode is a *missed* merge: one job listed per location under titles that differ in a rank word (`Senior/Staff SWE` vs `Senior SWE`). Not observed in the corpus. Cost if it happens is one extra read and a visible inconsistency, the original failure, not the silent one. The seeds rank `manager`, `lead` and `head`, so the gate splits an IC from its manager over shared text (OpenAI EM/MLE, Anthropic EM/TLM); the conservative direction is to split. Trailing roman numerals are not seeds, and `L5`-style levels are not handled; none appear in the multi-title units.

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

Idea: strip currency amounts and codes before hashing, since the classifier contract forbids reading seniority off compensation. Measured: it changes the backlog unit count by **zero** (Greenhouse per-location comp variants are already joined by the requisition edge), and where it merges across titles it merges `Senior Software Engineer, Inference` / `Staff Software Engineer, Inference` (Anthropic), `Senior` / `Staff Software Engineer, Agents` (Harvey), `Delivery Consultant` / `Senior Delivery Consultant` (Airtable), `Manager` / `Senior Manager` (Scale AI). The comp band is how boards distinguish levels over a shared description. Not classification-neutral in effect.

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
| Identical body, titles `Sr. Sales Engineer - Federal` / `Senior Sales Engineer - Strategic` | one unit (synonyms share a slug) |
| Chain: A(body X, `FDE`) — B(body X, `fde - emea`, req R) — C(body X′, `FDE - EMEA`, req R) | one unit (gate does not break closure) |
| Identical body, `Senior/Staff Software Engineer` / `Staff/Senior Software Engineer` | one unit (`senior,staff` = `senior,staff`) |

### Shadow comparison over real data

Run old and new component assignment side by side (read-only; run through `psql`, not the 5-second MCP `query` tool) and diff per posting:

- Every newly merged pair has equal whitespace-stripped text. Assert, do not assume.
- Every newly split pair has different rank signatures. List them with titles for eyeball review; expected 23 units corpus-wide, 6 extra units in the backlog.
- Expected counts: backlog 3,518 → 3,493 units; corpus identical-text units spanning more than one rank signature 23 → 0, splitting into 52.
- Post-change invariant query: no work unit contains two distinct rank signatures. Add it to the run report's dedup line if cheap.
- Wall-clock of each variant before and after, unforced and forced, with dedup on. The forced run is the one §1a's hashing cost lands on.

### Preview check

`enrichment_preview` with `count: 500` after the change: no unit is split across the limit, and no returned unit mixes rank signatures. Do not compare `work_unit_count` against the shadow delta. The preview window is recency-first and capped per company, so a whole-backlog delta does not carry over to one window.

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
| existing rank over-merge | No — fix | Concrete harm (Outreach 47087/47089); costs 6 reads |
| existing same-rank role over-merge | Yes | 6–8 units; stricter gate costs 108 reads; audit surfaces it |

"Change nothing" loses the whitespace fix and leaves the Outreach shape open. Both fixes are additive to the existing self-evidencing rule and neither adds a judgement threshold. That is the bar.

## Acceptance criteria

- [ ] Both selection variants hash description text with all whitespace removed, and a fixture with a single-space difference lands in one work unit.
- [ ] A fixture whose bodies differ only by U+00A0 versus U+0020 lands in one work unit.
- [ ] Both selection variants gate the text edge on an identical title rank signature, and a fixture with identical text under `Staff` and `Senior` titles lands in two work units.
- [ ] The rank signature reads its vocabulary from `title_seniority_seeds`, and a fixture with identical text under `Sr.` and `Senior` titles lands in one work unit.
- [ ] A fixture with identical text under a bare title and a location-suffixed title still lands in one work unit.
- [ ] The transitive-closure fixture still collapses a text edge chained to a requisition edge into one unit under the new gate.
- [ ] The forced variant produces the same unit assignment as the unforced variant over the same fixtures.
- [ ] A shadow comparison over the whole corpus shows every newly merged pair whitespace-equal and every newly split pair rank-different, with counts and before/after wall-clock recorded in this plan before it moves to `in-progress/`.
- [ ] The `enrich.sql` header no longer claims Ashby carries a requisition key and states the text-edge premise as "identical text under a rank-compatible title".
- [ ] `PROMPT_VERSION` in the `classification-pins` block of `.claude/skills/batch-enrich/SKILL.md` and of `.agents/skills/batch-enrich/SKILL.md` is incremented by one, to the same value, in the same commit as the selection change.
- [ ] The Step 3 rationale of `.claude/skills/batch-enrich/SKILL.md` describes the whitespace-stripped text edge and the rank gate, and no longer claims Ashby carries a requisition key.
- [ ] The durable rule (work unit = component over a company-scoped text edge gated on title rank and a requisition edge gated on title; title rank words are classification input; the gate and the title dimension share one rank vocabulary) is captured in `agent-context/lib/project.md` §Settled architecture before this plan is promoted to `ready/`.

## Tasks

### Task 1: Amend the selection query

- Add a migration defining `title_rank_signature(title text)`, shaped as in §2. It returns the sorted, comma-joined set of `title_seniority_seeds` slugs whose patterns match the title, or the empty string.
- Build each seed's expression the way 000036's `seniority_patterns` CTE does: unanchored, word-bounded, refusing a rank word that heads a connective phrase. The two must agree on what counts as a rank match.
- In both `candidate` CTEs of `apps/tools/internal/db/queries/enrich.sql`, change `text_hash` to the whitespace-stripped hash. The Harvey pair differs by one space, so collapsing is not enough.
- Write the non-ASCII whitespace as ` `, `​` and `﻿` regex escapes. `[[:space:]]` misses U+00A0 and U+200B on this database's locale.
- Add `rank_sig` to both `candidate` CTEs by calling the function on the normalized title.
- Add `b.rank_sig = a.rank_sig` to the text-edge half of both `edge` CTEs. The requisition half already gates on the full normalized title.
- Keep the two variants byte-identical in their dedup body. The integration test asserts parity, and drift here is the failure Step 3 of the Claude skill exists to prevent.
- Regenerate sqlc output per `agent-context/lib/developer-guide.md` §5.8. `DedupKey` and `IsRepresentative` semantics are unchanged, so `internal/enrich/selection` needs no code change.
- Update the `Posting` doc comment in `apps/tools/internal/enrich/selection/selection.go`. It names the two edges and must mention the whitespace strip and the rank gate.
- Rewrite the `enrich.sql` header paragraphs on edges. Ashby and Lever expose no requisition key (project.md §ATS targets). Identical text classifies identically only when the title rank agrees. Name `title_seniority_seeds` as the gate's vocabulary.
- Increment `PROMPT_VERSION` by one in the `classification-pins` block of `.claude/skills/batch-enrich/SKILL.md` and of `.agents/skills/batch-enrich/SKILL.md`. Both skills' pin rules require a bump in the same commit as any selection-semantics change, and require the two to move together.
- Read the current pin value from the block at implementation time. Its value is not restated in this plan.

Do not:
- Add a similarity, suffix, title-only or historical edge.
- Touch the requisition edge's gate.
- Touch `grouped`, `capped`, `waved`, `ranked`, or the ordering and cap parameters.
- Read `open_posting_titles` from selection. It covers open postings only and reads a different snapshot.
- Edit `title_seniority_seeds`, `open_posting_titles`, or migration 000036.
- Write a second rank word list.
- Hand-edit `apps/tools/internal/db/enrich.sql.go`.
- Bump the Go runner's `PromptVersion` in `apps/tools/cmd/batch-enrich/config.go`. It selects with dedup off, so its selection semantics do not change.

### Task 2: Fixtures

- Add the eight fixtures in §4 to `enrich_selection_dedup_integration_test.go`, reusing `seedDedupPostings` and `dedupUnits`. The helper appends the marker to the body after a space, so whitespace fixtures still compare equal after stripping.
- Assert forced-variant parity in at least the rank-gate test, mirroring `TestListUnclassifiedPostings_DedupRequisitionEdgeRequiresMatchingTitle`.
- Correct two stale test comments. The requisition-gate test says the key names a job family "at Greenhouse and Ashby"; Ashby carries none. The text-edge test says identical text classifies identically; that now holds only under a matching rank.

### Task 3: Shadow comparison and prose

- Run the old-vs-new comparison in §4 over the full corpus and record the counts and wall-clock in this plan.
- Update Step 3 of `.claude/skills/batch-enrich/SKILL.md`: add the whitespace and rank-gate bullets, and correct the Ashby requisition claim.
- Leave `.agents/skills/batch-enrich/SKILL.md` prose as is. It states selection is server-owned and describes no edges; its only change is Task 1's pin bump.

## Follow-on, once this ships

- **Retro-correct the over-merge victims.** Classified postings whose latest classification was written as a sibling copy inside an identical-text unit spanning more than one rank signature. Outreach 47087 and 47089 are the first rows. A `--force` re-run per posting, provenance intact. If a forced re-enrichment of open postings runs after this plan lands, it re-classifies the open victims on their own titles. Run this query after that wave and re-run only what it did not cover, which will be closed postings.
- **Consistency audit in the run report** (§4, last block): surfaces 1d and the same-rank role over-merge without merging them. Decide after two or three runs whether the divergence rate justifies revisiting a gated near-duplicate rule with a human-verifiable definition, not a threshold.
- **Persisted normalized hash.** Stripping adds about 4 s to a forced selection over the full corpus (§1a). A generated column on `posting_snapshots` removes the per-selection regex. Take it up if forced waves become routine.
- **One regex shape for both projections.** `open_posting_titles` builds the same per-seed expression inline. Moving the view onto a shared expression builder is a read-model change with its own performance budget (000036 measured the view at 521 ms), so it is not bundled here.

## Open questions

- Is `manager` a rank for this purpose? `title_seniority_seeds` ranks it, so the gate splits on it by default. The skill contract under development at review time says a bare `Manager` is role-forming, not a level word, so `Engineering Manager` and `Machine Learning Engineer` over one body would get the same seniority. They would still get different roles, which is the reason to keep splitting. Keeping it also splits `Account Manager` from `Account Executive` when a board reuses one description, which is arguably also right. Default: keep, by reading the seeds unchanged.
- Should the rank signature also feed the classifier prompt as a sanity assertion (title rank must not contradict `seniority`)? Out of scope here; noted for a future classifier contract bump.

## Confidence

| Recommendation | Confidence | What would change it |
|---|---|---|
| Whitespace-stripped hash | High | Nothing found; the only observed effect is the intended one |
| Rank-signature gate | Medium-high | Vocabulary is the seeds' heuristic; if the shadow diff shows a fan-out split by a rank word, raise it against the seeds rather than special-casing the gate |
| No similarity edge now | High | A gate that separates language/years variants from comp/location variants without a numeric threshold |
| Keep suffix, title-only, historical, comp, punctuation misses | High | New evidence of a self-evidencing key for Ashby fan-outs (none in the raw payload today: only a per-posting `id`) |
| Size of the tolerated 1d miss | Medium | 241 unclassified postings have a ≥0.98 same-title partner; the fraction that are truly one job is unmeasured beyond a 12-pair sample |

## Review log

2026-09-22, staleness and implementability review.

- Re-anchored to the recency-first query: `candidate`/`edge` are unchanged by it; added `grouped`/`capped`/`waved` to scope and the Do-not list.
- Re-verified backlog counts (3,518 → 3,487 → 3,494 under the old word list), 138/496/127, 23 split units, Ashby/Lever/Greenhouse key counts. All held. Corrected split-unit postings to 77 (18 unclassified), added Gem 0 of 8, and corrected "fires on Greenhouse and Gem" to Greenhouse only in current data.
- Replaced the hand-written level word list with the `title_seniority_seeds` vocabulary via one SQL function. Renamed level signature to rank signature, and changed it from a multiset to a set. New counts: +6 units (3,493), 52 sub-units. Added the `Sr.`/`Senior` fixture.
- Corrected the headline evidence. Chainguard 34701 was classified alone before dedup existed; Outreach 47087/47089 are the real victims.
- Replaced literal invisible characters in the regex with `\u` escapes and recorded the locale finding.
- Recorded the stripping cost on the forced variant and moved the persisted hash follow-on from "not now" to conditional.
- Replaced version literals (`batch-enrich-v6`, "v7 contract") with references to the contract and pin locations. Added the pin bump to Task 1 and its AC.
- Split the skills AC. The Codex skill has no edge rationale to amend and gets only the pin bump.
- Replaced the preview-parity delta check, which the capped recency-first window invalidates.
- Removed the coordination note with `codex-native-batch-enrichment`. Its posting-id tie breaker has already landed in `enrich.sql`.
- Added the stale test comments and the `selection.go` doc comment to the tasks.

- 2026-09-22 (after 000041): `title_seniority_seeds` now carries lookaround patterns (Leader, Apprentice, University Grad, Mid-Senior band), so the rank-signature counts above (3,493 units / 52 sub-units) may shift slightly; re-measure at implementation. Open defect in this plan's per-seed expression: matching bare seed patterns reads "Chief of Staff" as `staff`, because it lacks the view's object-of-connective guard from 000036. `Chief of Staff` and `Staff X` could then share a rank signature. Either reuse the view's guard or exclude connective objects before matching.
