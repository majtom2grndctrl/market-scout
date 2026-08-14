# Research — workplace-type derivation

Survey run 2026-08-12 against the live local database. 8,115 postings (latest
snapshot each), 106 companies. Numbers here are the evidence behind the spec's
decisions; the spec itself carries only the decisions.

## Coverage today

| ATS | postings | distinct `location_text` | `workplace_type` populated |
|---|---|---|---|
| greenhouse | 4,531 | 954 | 0 |
| ashby | 2,610 | 148 | 2,196 |
| lever | 474 | 63 | 474 |
| workday | 452 | 45 | 0 |
| workable | 48 | 8 | 0 |

2,670 of 8,115 postings (32.9%) carry a source-supplied `workplace_type`.

## Regex precision and recall

Ground-truth set: 3,357 rows — Lever 474 + Ashby 2,196 + 687 Greenhouse rows
recovered from `raw_data.metadata` "Location Type" (Anthropic).

| Candidate | fires | correct | precision | recall |
|---|---|---|---|---|
| A — naive hybrid > remote > onsite | 291 | 233 | 80.1% | 6.9% |
| B — A with `remote-friendly` stripped first | 230 | 219 | **95.2%** | 6.5% |

Per-class for B: remote 95.2% precision / 28.2% recall; hybrid 95.5% / 1.4%;
onsite 0 predictions against 1,165 labeled onsite.

Recall is capped by the data, not the pattern: only 16.8% of `location_text`
values contain a modality token at all. Ashby and Lever put modality in a
separate field and leave a bare city in the string — `San Francisco` maps to
hybrid 553×, onsite 65×, remote 11×.

## The `remote-friendly` trap

61 labeled rows contain `Remote-Friendly`. Truth: onsite 45, remote 14, hybrid 2.
In this corpus it means "we will consider remote candidates for an otherwise
onsite role," not "this role is remote." Mapping it to `remote` produces 47 wrong
answers and is the entire difference between 95.2% and 80.1% precision.

## Traps checked and cleared

| Trap | Finding |
|---|---|
| `Remote, OR` (Oregon vs. conjunction) | Does not occur in `location_text`. But `OR` means Oregon in 100 rows and "or" in 12 — never split on `\yOR\y`. Does not affect the modality regex, which never splits. |
| Negations (`no remote`, `not remote`) | Zero occurrences corpus-wide. No guard needed. |
| Modality substrings in city names | None. Word boundaries handle `Springfield` / `Bakersfield`. |
| Company names carrying a modality word | None. The only company-name leak is `Read AI - Seattle HQ`, which has no modality token. |
| `HQ` / `headquarters` as onsite tokens | Rejected — misfires on `US - Headquarters - Maryland - Columbia` (22) and `Read AI - Seattle HQ` (25). |
| `work from home`, `wfh`, `telecommut*`, `anywhere`, `distributed`, `virtual`, `flexible` | Zero incremental matches. Dead weight. |

Two modality tokens co-occur in only 3 postings, and `hybrid` is the true answer
in all 3 — which fixes the precedence order.

## The onsite branch is unvalidated

Fires on 34 postings across 2 companies (`Onsite- Salem, OR`,
`Vancouver, BC (on-site)`). None are in the labeled set, so precision is
unmeasurable. All 34 look correct by inspection. Meanwhile 1,165 labeled onsite
postings produce zero predictions. Nearly free, nearly worthless — ship it, but
no acceptance criterion can validate it.

## Character hygiene

| Issue | Postings | Note |
|---|---|---|
| Leading/trailing whitespace | 168 | Causes duplicate distinct values (`US-Remote` vs `US-Remote `) |
| Non-breaking space U+00A0 | 1 | `btrim()` will **not** remove it |
| Doubled spaces | 4 | |
| Accented Latin | ~30 | `São Paulo`, `Zürich, CH` |
| En/em dash, CR/LF/tab | 0 | |

## `raw_data` holds signal the adapters drop

| Source | JSON path | Postings | Quality |
|---|---|---|---|
| Greenhouse | `metadata[]` name `Location Type` | 687 | Ground truth (`On-Site` 657 / `Remote` 21 / `Hybrid (Travel-Required)` 9) |
| Greenhouse | `metadata[]` name `Job Posting Location` | 127 | Ground truth, `Region - Modality [- City]` slots |
| Greenhouse | `metadata[]` name `Remote` | 40 | Boolean — 11 true, 29 false |
| Workable | `telecommuting` | 48 of 48 | Boolean — 47 true, 1 false |
| Greenhouse | `offices[].location` | 2,253 | Canonical `City, Region, Country` — best place source, unmapped |
| Ashby | `address.postalAddress` | 2,557 | Structured city/region/country, unmapped |
| Workday | `externalPath` | 452 | Embeds a location slug; resolves the 46 `N Locations` placeholder rows |

`metadata` is `jsonb 'null'` rather than an array on 2,812 Greenhouse postings —
scanning it without a `jsonb_typeof(...) = 'array'` guard errors. Individual
entries need the same guard on `value`: 110 `Location Type` and 4 `Job Posting
Location` entries are present with a JSON null `value`.

### Follow-up survey, same day

The first pass counted the two boolean fields as ground truth and missed a third
modality field. Corrected here; the spec carries the corrected numbers.

**The booleans are one-sided.** `telecommuting` is a JSON boolean and Carbon
Robotics' `Remote` is Greenhouse's `yes_no` type — neither hides a richer value.
`true` means remote. `false` means only "not remote," which spans hybrid and
onsite. All 30 `false` rows carry a bare place string in `location_text`
(`Seattle, WA` ×22, `Scottsdale, AZ`, `IJmuiden, Netherlands`) and not one
contains a modality token, so tier 3 cannot split them either. Bare-city strings
map hybrid 553× / onsite 65× in the labeled set, so reading `false` as `onsite`
would be wrong more often than right — the same defect that disqualified Ashby's
`isRemote`. Decision: `true` resolves, `false` abstains. Cost: 30 postings.

**Tenable `Job Posting Location`** — `multi_select`, array of
`Region - Modality [- City]` strings. 89 office slots, 38 remote slots, across
127 postings; every posting carries exactly one slot and none mixes the classes.
Zero slots fail the token set `remote|office|headquarters|hq`. The modality
token is not positionally fixed (`Israel - Office` vs.
`MD - Columbia - Headquarters`), so match tokens anywhere in the slot rather
than splitting on the hyphen. Note that `Headquarters` was rejected as a
`location_text` token because a company name leaked into that string
(`Read AI - Seattle HQ`); this field carries no company name, so the token is
safe here.

**Field-name generalization.** 30 distinct company/field pairs across 13
companies carry `metadata`. Exactly three encode modality, and all three differ
in name, `value_type`, and value shape. Matching on literal names covers today
and nothing else.

Rejected: Greenhouse `offices[].name` as a modality source (+152 postings, all
ambiguous). Ashby `isRemote` (true for all hybrid *and* all remote — it means
"not strictly onsite"). Greenhouse `location.name` (byte-identical to
`location_text` in all 4,531 rows).

## Place extraction — why it is deferred

Splitting on hard delimiters (`;` `|` `•`) is reliable and covers 857 Greenhouse
rows. Everything else fails:

| Input | Result |
|---|---|
| `Remote-Friendly, United States; San Francisco, CA \| New York City, NY` | clean split |
| `Hybrid- Any Office (Fremont, CA, Salem, OR, or Pittsburgh, PA)` | unsplit, unbalanced paren |
| `SF, SEA, NYC, CHI, US-Remote` | unsplit — same comma separates intra- and inter-place |
| `United States - Remote Opportunity` | stranded word: `United States -  Opportunity` |
| `2 Locations` | no place at all (46 Workday rows) |

The comma-ambiguous class (286 Greenhouse rows) is unresolvable without a city
gazetteer. A strip-and-split leaves ~430 garbage segments corpus-wide.

`location_texts` does not help: it is a one-element copy of `location_text` for
Greenhouse, Workday, and Workable. It is a genuinely clean per-market array for
Ashby (446 multi) and Lever (97 multi) — the two sources that need it least.

## Residue after the derivation

This breakdown predates the follow-up survey and measures the wrong set. Its
4,315 counts everything the ATS field and the regex miss (8,115 − 2,670 − 1,130)
and never subtracts the `raw_data` tier. The true residue is 3,496 postings
(43.1%). The four-way split below was not recomputed against the smaller set, so
treat each figure as an upper bound — the 656 prose-recoverable count in
particular overlaps rows the `raw_data` tier now resolves.

Against the original 4,315-posting figure:

- 1,834 (42.5%) — description never mentions modality. Genuinely silent.
- 1,442 (33.4%) — modality word in prose, but boilerplate ("remote work stipend").
- 656 (15.2%) — high-confidence prose signal, recoverable by an enrichment pass.
- 383 (8.9%) — no `description_text` stored at all (all Workday and Workable).

## Projected coverage

Two populations, and they must not be confused. Everything else in this document
is corpus-wide: all 8,115 `job_postings`. But the derivation ships in
`open_postings_display`, which restricts to the latest successful fetch run per
company and emits **4,457** rows. A view change reaches only those.

Measured by simulating the full derivation, not projected:

| Source | Corpus (8,115) | View (4,457) |
|---|---|---|
| ATS `workplace_type` | 2,670 (32.9%) | 1,609 (36.1%) |
| `raw_data` structured signals | 872 | 405 |
| Regex on `location_text` | 1,077 | 511 |
| **Resolved after this plan** | **4,619 (56.9%)** | **2,525 (56.7%)** |

The two percentages landing a fifth of a point apart is a coincidence, not a
check — the counts differ by nearly half.

Corpus-wide, the `raw_data` tier resolves 746 onsite, 117 remote, and 9 hybrid,
and abstains on 31 rows (30 false booleans, 1 empty slot array); the
`location_text` tier resolves 938 remote, 105 hybrid, and 34 onsite. Within the
view the `raw_data` tier reaches 419 rows and resolves 405.

The spec quotes the view figures, since those are what the change delivers.
Deferred description-prose extraction (≤656) and the unreachable remainder are
corpus-wide counts and were never rescoped to the view.

## Query cost — why the tier-2 formulation is prescribed

Measured on the local DB at ~66k snapshots / 4,457 view rows, warm cache,
`EXPLAIN (ANALYZE, TIMING OFF)`, three or more runs each.

| Variant | Median |
|---|---|
| Current view, unmodified | 28ms |
| + tier 3 regex only, `raw_data` never referenced | 49ms |
| + `raw_data` in the lateral, one trivial probe | 160ms |
| Naive four-branch derivation, direct `raw_data` refs | 750ms |
| Single fenced materialization + one aggregate pass | 180ms |
| Same, gated to companies carrying tier-2 fields | 75ms |

**Detoast is per access, not per row.** One probe costs ~135ms; two cost ~310ms;
four cost ~580ms. Postgres does not cache a detoasted datum across expression
evaluations, so every branch that names `raw_data` decompresses the whole
document again. This is the entire difference between 750ms and 180ms, and it is
why the spec prescribes a lateral chain rather than a `CASE` over direct
references.

The cost is CPU, not I/O — `EXPLAIN (ANALYZE, BUFFERS)` shows ~19,600 buffer
accesses at the projection node, all shared hits, zero reads. Roughly 80% of it
is pglz decompression.

| Compression | Detoast pass | Stored size |
|---|---|---|
| pglz (current) | ~203ms | 29 MB |
| lz4 | ~55-70ms | 26 MB |

Measured by rebuilding the exact working set into temp tables. Note that
`INSERT ... SELECT` copies an already-compressed datum verbatim rather than
recompressing it — a test that does not force a rebuild (`raw_data || '{}'`)
silently measures pglz twice and shows no difference.

Rejected optimizations, each tested:

| Approach | Result |
|---|---|
| Narrowing what the lateral projects | No effect. jsonb is one TOASTed value; extracting one key still decompresses all of it. |
| Computing the derivation inside the snapshot lateral | No effect. Same access count. |
| Expression index on the derivation | Planner uses a plain index scan and recomputes from the heap. Index-only scans require every column referenced by the indexed expression to be an index column, and `raw_data` cannot be. |
| Gating tier 2 on `companies.ats` | Works — 75ms, output row-identical. Rejected because it must be maintained in step with the field-name list, in a place nobody editing that list would look. See open question 2. |

Filtered reads stay cheap regardless: one company is 31ms, `LIMIT 50` is 8ms.
Only `listOpenPostings()` selects the view unbounded.
