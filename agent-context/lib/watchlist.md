# Company Watchlist

> **Read this when:** adding or removing companies from the scrape run, or evaluating ATS coverage.
> **Key invariant:** `internal/db/seeds/companies.sql` is the canonical source of truth for active companies. This file tracks integration status and the information needed to onboard new ones.
> **Related:** `project.md` §ATS targets, `index.md`

---

## Standard scrape run

Companies currently seeded and included in every fetcher run.

| Company    | ATS        | Board token  | Notes                          |
|------------|------------|--------------|--------------------------------|
| Anthropic  | greenhouse | `anthropic`  | AI lab                         |
| Stripe     | greenhouse | `stripe`     | Fintech, large eng org         |
| Figma      | greenhouse | `figma`      | Design tools                   |
| Scale AI   | greenhouse | `scaleai`    | AI data and infra              |
| Glean      | greenhouse | `gleanwork`  | AI-powered enterprise search   |
| Cognition  | ashby      | `cognition`  | AI coding agent (Devin)        |
| Harvey     | ashby      | `harvey`     | AI for legal                   |
| ElevenLabs | ashby      | `elevenlabs` | AI voice                       |
| Linear     | ashby      | `linear`     | Dev tooling, product mgmt      |
| Mistral    | lever      | `mistral`    | Open-weight AI models          |

To add a company: insert a row in `internal/db/seeds/companies.sql` (idempotent on re-run), then add it to this table.

---

## Candidates — not yet integrated

Companies of interest whose board token or ATS has not been verified, or that haven't been added to the seed yet.

| Company        | ATS (suspected) | Notes                                          |
|----------------|-----------------|------------------------------------------------|
| Perplexity     | Ashby           | AI search; slug likely `perplexity-ai` — unverified |
| Cursor         | Ashby           | AI code editor; parent co Anysphere — slug unverified |
| OpenAI         | Greenhouse      | Board token unverified                         |
| Hugging Face   | Unknown         | ATS unconfirmed                                |
| Cohere         | Unknown         | ATS unconfirmed                                |
| Character.ai   | Unknown         | ATS unconfirmed                                |
| Weights & Biases | Unknown       | ATS unconfirmed                                |
| Runway ML      | Unknown         | ATS unconfirmed; video/image generation        |
| Together AI    | Unknown         | ATS unconfirmed; AI infra and inference        |

To verify a candidate: check `jobs.ashbyhq.com/<slug>`, `api.lever.co/v0/postings/<slug>?mode=json`, or `boards-api.greenhouse.io/v1/boards/<token>/jobs`. Confirmed board tokens go into the standard scrape run table above and the seed file.

---

## Adding a new ATS adapter

Companies on ATS platforms without an adapter (Workday, Rippling, Jobvite, etc.) cannot be integrated until an adapter exists. See `project.md` §ATS targets and `internal/ats/` for the adapter interface.
