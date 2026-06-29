# Plans Roadmap

> **Read this when:** choosing the next plan to draft, review, promote, or orchestrate.
> **Key invariant:** checkboxes are roadmap rollup only.
> **Related:** `agent-context/lib/style-guide.md` §Documentation Lifecycle, `roadmap-archive/`

---

## How To Read This

Spec folder names are stable IDs.

Checkbox meaning:

| Marker | Meaning |
|---|---|
| `[ ]` | Active roadmap item. Still relevant to planning. |
| `[x]` | Finished. Safe to look past unless doing history. |

If checkbox and folder location disagree, folder location wins. Fix the roadmap when noticed.

Spec folder names must be unique across plan lifecycle folders.

Move completed epics out of this file at the end of the quarter. Archive them in `roadmap-archive/YYYY-QN.md`.

## Epic: Browser-Led Market Scouting

Goal: Agent can discover companies from web sources, investigate them in a browser, and safely onboard supported ATS boards.

### Milestone: Discovery Run Foundation

- [ ] `browser-led-discovery-runs`
  Define the browser-led discovery run. It should cover source inputs, candidate records, provenance, statuses, dedup preflight shape, and run summaries. It is the foundation for multi-company pages and recent-news scouting.

- [ ] `discover-and-onboard-agent-loop`
  Connect discovered candidates to the fetcher set. It should cover browser-observed URL evidence, shared ATS detection, the `detect_ats` MCP preflight, and `add_company` as the verification and write gate.

### Milestone: Safe Onboarding

- [ ] `stage-company-seed-patch`
  Reconcile companies added through MCP with the canonical seed file. It should produce a human-reviewed patch, not mutate source files as a hidden side effect of `add_company`.

### Milestone: Source Recipes

- [ ] `discovery-source-recipes`
  Add browser recipes for common sourcing modes. Cover one page that mentions many companies, and recent articles from a tech-news source. Recipes should feed the discovery run workflow rather than define a separate pipeline.
