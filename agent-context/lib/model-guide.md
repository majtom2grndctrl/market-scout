# Codex Model Guide

> **Read this when:** choosing a model or reasoning effort for a project Codex skill.
> **Key invariant:** match capability to the task; preserve explicit user choices and record the actual saving model.
> **Related:** `.agents/skills/`, [Developer Guide](./developer-guide.md) §6.2 (classification provenance).

Reviewed 2026-09-29. These are project starting points, not measured Market Scout benchmarks. Scope: Codex skill execution; API-backed application models and Claude skills have separate configuration.

## Models

| Model | Identifier | Use |
|---|---|---|
| GPT-6 Luna | `gpt-6-luna` | Bounded classification, extraction, mechanical edits, and focused coding with clear checks. |
| GPT-6.1 Sol | `gpt-6.1-sol` | Default for implementation, planning, review, and coordination across files. |
| GPT-6 Astra | `gpt-6-astra` | Escalate difficult architecture, conflicting evidence, or unresolved correctness and integrity questions. |

OpenAI recommends Luna for focused, repeatable work and GPT-6.1 Sol for complex coding and sustained work. Astra remains the strongest choice for demanding workflows. Availability varies by account and client. [Official Codex model guidance](https://learn.chatgpt.com/docs/models).

## Skill defaults

| Skill or phase | Model | Effort | Escalation trigger |
|---|---|---|---|
| `batch-enrich`: coordinator | Sol | `medium` | Complex reconciliation or conflicting run evidence. |
| `batch-enrich`: normal workers | Luna | `high` | Exhausted save retries; follow the skill's one-posting escalation. |
| `batch-enrich`: escalation and read-only audit | Sol | `high` | Report unresolved failures; keep the skill's retry limit. |
| `data-audit`: coordinator and reviewers | Sol | `high` | Ambiguous taxonomy or disputed evidence: Astra `high`. |
| `discovery-run` | Sol | `medium` | Ambiguous company identity or browser evidence: Astra `medium`. |
| `spec-session`, `draft-plan` | Sol | `high` | Architectural tradeoffs or conflicting requirements: Astra `high`. |
| `one-pager` | Sol | `medium` | Complex interaction or design decisions: Sol `high`. |
| `review-draft-spec`, `review-implementability` | Sol | `high` | Unresolved architectural or implementability questions: Astra `high`. |
| `orchestrate`: coordinator | Sol | `high` | Cross-phase contract or architecture conflicts: Astra `high`. |
| `implement-task`: task delegates or direct work | Sol | `medium` | Cross-package contracts or subtle lifecycle changes: Sol `high`; unresolved defects: Astra `high`. |
| `fix-findings`: coordinator and cross-cutting fixes | Sol | `high` | Architectural uncertainty: Astra `high`. |
| `fix-findings`: mechanical, isolated edits | Luna | `low` | Knock-on effects or contract changes: Sol `high`. |
| `review-panel`: triage and mechanical hygiene | Luna | `low` | Semantic comments or non-mechanical behavior: Sol `high`. |
| `review-panel`: correctness, contracts, adversarial cases, integrity, refutation | Sol | `high` | Disputed high-impact findings: Astra `high`. |
| `preflight` | Luna | `low` | Non-mechanical failures: report them for a Sol implementation task. |
| `create-skill` | Sol | `medium` | Complex delegation or mutation boundaries: Sol `high`. |

The workflow mapping is a project judgment based on [OpenAI's model-selection guidance](https://developers.openai.com/api/docs/guides/model-selection). Start with a familiar task; retain the lightest setting that meets its checks. High effort is appropriate for Luna classification because the taxonomy and grounding rules require judgment.

## Execution

- Respect an explicit model or effort chosen by the user. Recommendations do not switch the current chat automatically.
- For delegated work, pass the selected model and effort explicitly when the tool supports them. Inline the applicable guidance in each task packet. Model choice does not authorize extra delegation or writes.
- Check the callable model list before dispatch. If Sol 6.1 is unavailable, use GPT-6 Sol (`gpt-6-sol`); if Luna 6 is unavailable, use GPT-5.6 Luna (`gpt-5.6-luna`) when offered. Report the fallback. If neither is available, resolve a supported model with the user.
- Use `low`, `medium`, `high`, and `xhigh` as configuration names. Desktop Light means `low`; Extra High means `xhigh`. Increase effort for harder tasks; do not assume effort levels behave identically across generations.
- Reserve Max for exceptionally difficult single tasks. Ultra adds subagent work and requires a divisible task and supported client; Luna has no Ultra. Most skills need neither. [Reasoning controls](https://learn.chatgpt.com/docs/models#pick-a-reasoning-effort).
- In batch enrichment, resolve worker availability before selecting the cohort. Pass resolved model identifiers as invocation pins; provenance must name the model that actually saved. Model or effort changes alone do not bump `PROMPT_VERSION`; changed classification rules do.

## Refresh

Recheck official model guidance when a model becomes unavailable or the user requests an update. Update this table and batch worker pins together. GPT-5.6 models remain rollout fallbacks, not preferred defaults. GPT-5.4 and GPT-5.4 mini retired from Codex with ChatGPT sign-in on 2026-08-31; Spark retired on 2026-09-14; GPT-5.5 retires on 2026-10-14. These Codex retirements do not establish API retirement. [Availability and retirement notices](https://learn.chatgpt.com/docs/models#deprecated-codex-models).
