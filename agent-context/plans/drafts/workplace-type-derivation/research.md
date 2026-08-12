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
| Greenhouse | `metadata[]` name `Location Type` | 687 | Ground truth (`On-Site` / `Remote` / `Hybrid (Travel-Required)`) |
| Greenhouse | `metadata[]` name `Remote` | 40 | Ground truth, boolean |
| Workable | `telecommuting` | 48 of 48 | Ground truth, boolean |
| Greenhouse | `offices[].location` | 2,253 | Canonical `City, Region, Country` — best place source, unmapped |
| Ashby | `address.postalAddress` | 2,557 | Structured city/region/country, unmapped |
| Workday | `externalPath` | 452 | Embeds a location slug; resolves the 46 `N Locations` placeholder rows |

`metadata` is `jsonb 'null'` rather than an array on 2,812 Greenhouse postings —
scanning it without a `jsonb_typeof(...) = 'array'` guard errors.

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

4,315 postings (53.2%) remain unresolved from `location_text` alone:

- 1,834 (42.5%) — description never mentions modality. Genuinely silent.
- 1,442 (33.4%) — modality word in prose, but boilerplate ("remote work stipend").
- 656 (15.2%) — high-confidence prose signal, recoverable by an enrichment pass.
- 383 (8.9%) — no `description_text` stored at all (all Workday and Workable).

## Projected coverage

| Source | Postings | Cumulative |
|---|---|---|
| ATS `workplace_type` | 2,670 (32.9%) | 32.9% |
| `raw_data` structured signals | ~775 | ~42.4% |
| Regex on `location_text` | ~1,130 | **~54.8%** |
| Deferred: description prose | 656 | 62.9% |
| Unreachable | ~3,000 (37%) | |
