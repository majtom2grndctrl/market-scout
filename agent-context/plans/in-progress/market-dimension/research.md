# Market Dimension — Research

> Investigation notes behind the `index.md` spec. Findings that inform decisions but do not belong in the spec body. Measured against the live cluster over the read-only DSN on 2026-08-21.

## The location columns

`posting_snapshots` carries two location forms (confirmed against `information_schema.columns`):

| Column | Type | Shape |
|---|---|---|
| `location_text` | `text` | Scalar. Often a *concatenation* of several locations. |
| `location_texts` | `text[]` (`_text`) | Array. **Not reliably atomic** — elements themselves still hold concatenations. |

The array is not a clean split of the scalar. Both carry multi-location strings joined by `|`, `•`, and `;`. Example element that occurs 103 times as a single array member: `"San Francisco, CA | New York City, NY"`. Consequence for the spec: the derivation cannot trust `location_texts` to be one-place-per-element. It must pattern-match against the raw text (scalar or array joined) with word-boundary regexes, and let a multi-location string match multiple markets naturally.

## Scale of the open cohort (2026-08-21)

- Open postings: **4,595** (`open_postings` join).
- Every open posting has both `location_text` and a non-empty `location_texts` (0 nulls).
- Multi-location postings (`array_length > 1`): **329**, max array length **22**.

## Real location strings (top of the open cohort)

Scalar `location_text`, most frequent:

| String | Count |
|---|---|
| San Francisco | 762 |
| San Francisco, CA | 141 |
| New York | 140 |
| Seattle, WA | 124 |
| United States | 116 |
| Everett, WA | 106 |
| San Francisco, CA \| New York City, NY | 103 |
| Singapore | 98 |
| San Francisco, CA • New York, NY • United States | 84 |
| London | 80 |
| Remote | 63 |
| United States - Remote | 49 |

Observations that shape the dictionary:
- **Hub spellings vary wildly**: `San Francisco`, `San Francisco, CA`, `San Francisco, California, United States` all mean the Bay Area. Bay-Area suburbs appear as their own strings: `Mountain View, CA`, `Fremont, CA` (`Hybrid- Fremont, CA` ×24), `Milpitas, California`. A Bay-Area market must fold in suburb spellings, not just "San Francisco".
- **Seattle metro** fragments across `Seattle, WA`, `Everett, WA` (106), `Kent, Washington` (31), `Kirkland, WA` (28), `Bellevue, WA` (21), `Bellingham, WA`.
- **Remote is expressed many ways**: `Remote`, `Remote - US`, `US - Remote`, `USA - Remote`, `US-Remote`, `United States - Remote`, `Americas Remote`, `United States (Any Time Zone)`.
- **Noise floor is small**: `N/A` (22), `2 Locations` (8), empty — total hard-unmappable ≈ **36 postings (~0.8%)**. Almost every posting carries *some* location signal; the coverage gap is dictionary breadth, not absent data.

## Coverage measurement

A seed of ~25 hubs + a `remote` catch-all matched **3,728 / 4,595 = 81.1%** of open postings (union: a posting counts as covered if it matches any one market). Per-market hit counts (regex over `location_text`, word-boundary):

| Seed market | Open postings matched |
|---|---|
| SF Bay Area | 1,535 |
| New York | 760 |
| Seattle metro | 661 |
| any "remote" | 595 |
| London | 204 |
| India (Bengaluru/Hyderabad/…) | 138 |
| Singapore | 112 |

The unmatched ~19% (867 postings) breaks down as:
- **Bare macro-regions / countries** — the largest share: `United States` / `US` / `North America` / `NAMER` ≈ **717**; `United Kingdom` / `Europe` / `EMEA` etc. ≈ **103**. These are real signal, just coarser than a hub. Whether to admit them as macro-region markets is a design choice (see open questions in the spec).
- **Addable hubs the seed missed**: Fremont/Milpitas (→ Bay Area), Mexico City (25+), Tel Aviv/Israel, Seoul, São Paulo/Brazil, Taipei, Doha, Manila, Warsaw.
- **Genuine noise** (~0.8%): `N/A`, `2 Locations`, empty.

Interpretation for the spec: a well-curated seed (≈25–30 hubs + a small macro-region set + a `remote` bucket) can honestly reach **~90%+**, and the residual is dominated by coarse-but-real region strings, not missing data. The honest move is an explicit `unmapped` bucket (or a coverage denominator) for the residual, so named markets never silently imply the whole cohort.

## Grammar wiring surface (confirmed against source)

- `apps/web/lib/composition/vocabulary.ts`: `GROUPINGS` and `FILTER_DIMENSIONS` are `as const` tuples. The zod enums in `schema.ts` build directly from them (`z.enum(GROUPINGS)`, `z.enum(FILTER_DIMENSIONS)`), so adding `market` threads automatically into `compositionSchema`, `COMPOSITION_JSON_SCHEMA`, `orderGroupings`, and `orderFilters`. `FILTER_DIMENSIONS` is `satisfies readonly Grouping[]`, so a filter dim must also be a grouping.
- `rules.ts`: `REQUIRES_DENOMINATOR` is `Record<Grouping, boolean> satisfies` — omitting a `market:` entry is a **compile error**, not a silent full-corpus read.
- `composition.test.ts` (line 244): a local `EXPECTED: Record<Grouping, boolean>` mirrors the same table and asserts `Object.keys(EXPECTED).sort() === [...GROUPINGS].sort()` — adding `market` to `GROUPINGS` forces a `market:` line here or the suite fails.
- `json-schema.test.ts`: derives from the schema; does not hard-enumerate grouping members, so it will not break on the addition.

## Read-model wiring surface (confirmed against migrations)

- Highest migration is `000019_workplace_type_derivation`; next number is **000020**.
- `000019` already establishes the precedent for deriving a first-class dimension in SQL from noisy location text without touching append-only snapshots (`workplace_type_resolved` + a `workplace_type_source` provenance column). The market derivation is the same shape.
- `open_posting_taxonomy` (in `000017`) already carries a `term_kind` text discriminator (`role` / `specialization` / `skill` / `dimension`) and is already **many-rows-per-posting**. Adding a `'market'` branch fits the existing shape — but every existing branch joins through `classification_id`. The market branch must join on `job_posting_id` instead, because a market exists for postings with no classification.
- `read-model-views.db.test.ts` is the db-test pattern: owner DSN seeds, read-only DSN reads, `context.skip()` when either DSN is absent, marker-scoped cleanup in `finally`. `test:db` runs it (`vitest run --mode db`); plain `test` is the non-db suite.
