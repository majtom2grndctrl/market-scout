// Prompt rendering for the Haiku classifier agent. RenderSystemPrompt builds
// the cache-friendly system prompt (taxonomy + agent contract) shared across
// every posting in a wave; RenderBatchedUserMessage builds the user message
// for a batch of postings; RenderRetryPrompt appends targeted hints when
// Phase A validation rejects a parsed response. The skill at
// .claude/skills/batch-enrich/SKILL.md is deprecated for batched runs — this
// file is now the source of truth for the agent contract. Update
// agentContract directly here and bump PromptVersion when the contract
// changes.
package main

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

// ValidationFailureKind identifies a Phase A failure reason. Every kind
// has a matching hint template in FormatHint so retry prompts can speak
// the same language as the validator.
type ValidationFailureKind string

const (
	FailInvalidSlug              ValidationFailureKind = "invalid_slug"
	FailNullByte                 ValidationFailureKind = "null_byte"
	FailSeniorityInvalid         ValidationFailureKind = "seniority_invalid"
	FailSeniorityMissing         ValidationFailureKind = "seniority_missing"
	FailEmptyDimensions          ValidationFailureKind = "empty_dimensions"
	FailUnknownDimension         ValidationFailureKind = "unknown_dimension"
	FailCrossTableCollision      ValidationFailureKind = "cross_table_collision"
	FailWithinResponseCrossTable ValidationFailureKind = "within_response_cross_table"
	FailNotesTooLong             ValidationFailureKind = "notes_too_long"
	FailDuplicateSlug            ValidationFailureKind = "duplicate_slug"
)

// RenderSystemPrompt builds the cache-friendly system prompt for a wave. It
// holds the focus guidance, taxonomy sections, and the agent contract — all
// content that is stable across every posting in the wave, so the Claude CLI
// can reuse the prompt cache between calls. If focus is empty, the focus
// guidance block is omitted. The closing block is the agent contract; the
// SKILL.md file is deprecated for batched runs and this file is the source
// of truth.
func RenderSystemPrompt(taxonomy Taxonomy, focus string) string {
	var b strings.Builder

	if strings.TrimSpace(focus) != "" {
		b.WriteString("## Focus guidance\n\n")
		b.WriteString(focus)
		b.WriteString("\n\n")
	}

	b.WriteString("## Canonical roles (existing)\n\n")
	writeTaxonomyList(&b, taxonomy.CanonicalRoles)
	b.WriteString("\n")

	b.WriteString("## Role dimensions (closed set)\n\n")
	writeTaxonomyList(&b, taxonomy.RoleDimensions)
	b.WriteString("\n")

	b.WriteString("## Specializations (existing)\n\n")
	writeTaxonomyList(&b, taxonomy.Specializations)
	b.WriteString("\n")

	b.WriteString("## Skills (existing)\n\n")
	writeTaxonomyList(&b, taxonomy.Skills)
	b.WriteString("\n")

	b.WriteString(agentContract)

	return b.String()
}

// RenderBatchedUserMessage builds the user message for a batch of postings.
// Each posting is rendered as a posting heading followed by its description
// body, in input order, with a blank line between postings. Everything else
// (taxonomy, contract, focus) lives in the system prompt so the cache stays
// warm between calls.
func RenderBatchedUserMessage(postings []SelectedPosting) string {
	var b strings.Builder
	for i, p := range postings {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "# Posting %d: %s\n\n", p.PostingID, p.Title)
		b.WriteString("## Description\n\n")
		b.WriteString(p.DescriptionText)
		b.WriteString("\n")
	}
	return b.String()
}

// RenderUserMessage builds the per-posting user message: a posting heading
// followed by the description body. Everything else (taxonomy, contract,
// focus) lives in the system prompt so the cache stays warm between calls.
//
// Deprecated: use RenderBatchedUserMessage. This function will be deleted in
// the dispatch refactor (Task 5).
func RenderUserMessage(posting SelectedPosting) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Posting %d: %s\n\n", posting.PostingID, posting.Title)
	b.WriteString("## Description\n\n")
	b.WriteString(posting.DescriptionText)
	b.WriteString("\n")
	return b.String()
}

// RenderRetryPrompt appends a retry-guidance block listing every hint to the
// original user message. The agent sees the full original user message plus
// the new block so it can correct its previous output without losing context.
// The system prompt (taxonomy + contract) is unchanged across retries so the
// prompt cache stays warm.
func RenderRetryPrompt(originalUserMessage string, hints []string) string {
	var b strings.Builder
	b.WriteString(originalUserMessage)
	if !strings.HasSuffix(originalUserMessage, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n## Retry guidance\n\n")
	b.WriteString("Previous response failed validation. Fix every item below. Return corrected JSON only.\n\n")
	for _, h := range hints {
		b.WriteString("- ")
		b.WriteString(h)
		b.WriteString("\n")
	}
	return b.String()
}

// FormatHint produces the operator-facing hint string for a validation
// failure. args carry the kind-specific values: the offending slug, the
// owning table for collisions, the list of valid dimension slugs, etc.
//
// Argument shape per kind:
//   - FailInvalidSlug:         args[0] = slug
//   - FailNullByte:            args[0] = field name (e.g. "name", "notes")
//   - FailSeniorityInvalid:    args[0] = offending value
//   - FailSeniorityMissing:    no args
//   - FailEmptyDimensions:     args[0] = canonical_role slug
//   - FailUnknownDimension:    args[0] = slug, args[1] = comma-separated list of valid dimension slugs
//   - FailCrossTableCollision:      args[0] = slug, args[1] = owning table (snapshot)
//   - FailWithinResponseCrossTable: args[0] = slug, args[1] = first table, args[2] = second table
//   - FailNotesTooLong:             args[0] = field name (e.g. "classification.notes"), args[1] = field length as string
//   - FailDuplicateSlug:       args[0] = slug
//
// Missing args degrade to a placeholder rather than panicking — hints are
// operator guidance, not load-bearing protocol.
func FormatHint(kind ValidationFailureKind, args ...string) string {
	get := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return "?"
	}
	warnMissing := func(wantArgs int) {
		slog.Warn("[batch-enrich] FormatHint: missing required arg",
			"kind", kind,
			"have_args", len(args),
			"want_args", wantArgs,
		)
	}
	switch kind {
	case FailInvalidSlug:
		if len(args) < 1 {
			warnMissing(1)
		}
		return fmt.Sprintf("`%s` is not a valid slug. Use lowercase letters, digits, and hyphens only. Max 64 chars.", get(0))
	case FailNullByte:
		if len(args) < 1 {
			warnMissing(1)
		}
		return fmt.Sprintf("`%s` contains a null byte (\\x00). Strip it.", get(0))
	case FailSeniorityInvalid:
		if len(args) < 1 {
			warnMissing(1)
		}
		return fmt.Sprintf("`%s` is not a valid seniority. Use one of: intern, junior, mid, senior, staff, principal, lead, director, unknown.", get(0))
	case FailSeniorityMissing:
		return "seniority is required. Emit one of: intern, junior, mid, senior, staff, principal, lead, director, unknown."
	case FailEmptyDimensions:
		if len(args) < 1 {
			warnMissing(1)
		}
		return fmt.Sprintf("canonical_role `%s` has no dimensions. Every role needs at least one dimension slug.", get(0))
	case FailUnknownDimension:
		if len(args) < 2 {
			warnMissing(2)
		}
		return fmt.Sprintf("`%s` is not a known dimension. Use one of: %s.", get(0), get(1))
	case FailCrossTableCollision:
		if len(args) < 2 {
			warnMissing(2)
		}
		return fmt.Sprintf("`%s` is already a %s. Choose a different slug.", get(0), get(1))
	case FailWithinResponseCrossTable:
		if len(args) < 3 {
			warnMissing(3)
		}
		return fmt.Sprintf("`%s` appears in both %s and %s. A slug belongs to one table only.", get(0), get(1), get(2))
	case FailNotesTooLong:
		if len(args) < 2 {
			warnMissing(2)
		}
		return fmt.Sprintf("`%s` is too long. Notes max 4096 characters.", get(0))
	case FailDuplicateSlug:
		if len(args) < 1 {
			warnMissing(1)
		}
		return fmt.Sprintf("`%s` appears more than once. Each slug must be unique within its array.", get(0))
	default:
		return fmt.Sprintf("unknown validation failure: %s", kind)
	}
}

// writeTaxonomyList renders one bullet per entry as "- slug: name" with
// slugs sorted so prompts are byte-stable across runs (cache-friendly).
func writeTaxonomyList(b *strings.Builder, entries map[string]TaxonomyEntry) {
	if len(entries) == 0 {
		b.WriteString("- (none)\n")
		return
	}
	slugs := make([]string, 0, len(entries))
	for slug := range entries {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		fmt.Fprintf(b, "- %s: %s\n", slug, entries[slug].Name)
	}
}

// agentContract contains the Output schema, Classification discipline, Slug
// discipline, Summary contract, and Batched output schema sections. The
// SKILL.md file is deprecated for batched runs — this constant is the source
// of truth. Bump PromptVersion when the contract changes.
const agentContract = "## Agent contract\n" +
	"\n" +
	"Return JSON only. No prose. No code fences.\n" +
	"\n" +
	"**Output schema:**\n" +
	"\n" +
	"```json\n" +
	"{\n" +
	"  \"posting_id\": <int>,\n" +
	"  \"classification\": {\n" +
	"    \"seniority\": \"intern|junior|mid|senior|staff|principal|lead|director|unknown\",\n" +
	"    \"notes\": \"<optional freeform>\"\n" +
	"  },\n" +
	"  \"canonical_roles\": [\n" +
	"    {\n" +
	"      \"slug\": \"<existing or new>\",\n" +
	"      \"name\": \"<human-readable>\",\n" +
	"      \"dimensions\": [\"design\", \"engineering\"]\n" +
	"    }\n" +
	"  ],\n" +
	"  \"specializations\": [{\"slug\": \"...\", \"name\": \"...\"}],\n" +
	"  \"skills\": [{\"slug\": \"...\", \"name\": \"...\", \"requirement\": \"required|preferred\"}],\n" +
	"  \"summary\": \"<100–200 tokens>\"\n" +
	"}\n" +
	"```\n" +
	"\n" +
	"**Classification discipline:**\n" +
	"- `canonical_roles` is an array. Blended roles are first-class — emit multiple when a posting spans them (e.g. design-engineering hybrids). Prefer one role; use two only when the posting clearly spans two genuinely distinct functions; three or more is almost always wrong and signals over-classification.\n" +
	"- Always emit `seniority`. If undetermined, emit `\"unknown\"`. Never omit the field.\n" +
	"- `notes` is optional. Omit the key or emit `null`. Empty or whitespace-only strings are treated as null.\n" +
	"- `dimensions` is a **closed set**. Use slugs from the prompt's list only. Never invent new dimension slugs.\n" +
	"- Every canonical_role — new or existing — must carry a non-empty `dimensions` array. Empty or missing fails validation and the orchestrator skips the posting.\n" +
	"\n" +
	"**Slug discipline:**\n" +
	"- Prefer existing slugs. Mint new ones only when no existing slug fits.\n" +
	"- Before minting, scan the taxonomy for the same concept. Reuse synonyms, near-synonyms, and minor rephrasings. Mint only when the concept is clearly absent.\n" +
	"- Canonical roles are a small, stable set. The bar for minting a new role is higher than for skills. Mint only when the function has no analog in the existing list — not because the title is unfamiliar. Unfamiliar titles almost always map to an existing slug.\n" +
	"- Kebab-case, ASCII, stable across re-runs.\n" +
	"- New canonical_roles must include at least one `dimensions` slug from the closed set.\n" +
	"- Each slug lives in exactly one of the three minting tables. If a concept could fit two, place it in the most specific and omit it from the others.\n" +
	"- `specializations`: domain focus areas describing what the role works on (e.g. `enterprise-sales`, `developer-tools`). Not transferable skills.\n" +
	"- `skills`: discrete, transferable competencies a candidate must or should have (e.g. `python`, `rlhf`, `distributed-training`). Not domain descriptions.\n" +
	"- Canonical roles describe job function only. Strip these modifier types from the role slug — they belong in `seniority` or `notes`:\n" +
	"  - Seniority titles: Director, Head, Lead, VP, Senior, Junior, Staff, Principal\n" +
	"  - Relationship/deployment context: Partner, Deployed, Embedded, Federated\n" +
	"  - Scope qualifiers: Global, Regional, Federal, APAC, LATAM\n" +
	"  - Specialization-as-suffix: Enablement, Strategy, Operations (when modifying a role, not naming one)\n" +
	"  Examples: \"Director of Enterprise Sales\" → `account-executive`, seniority: `director`. \"Partner Deployed Engineer\" → `software-engineer`. \"Enterprise Sales Leader\" → `account-executive`. \"GTM Enablement\" → `account-executive` + specialization, not a new role slug.\n" +
	"\n" +
	"**Summary contract:**\n" +
	"- 100–200 tokens.\n" +
	"- Cover: role, seniority, required skills, preferred skills, domain, role type (IC vs lead, full-time vs contract).\n" +
	"- No marketing fluff. No company-specific framing.\n" +
	"- This text gets embedded. Write for semantic similarity, not human reading.\n" +
	"\n" +
	"**Batched output schema:**\n" +
	"\n" +
	"When multiple postings are provided, wrap all results in a top-level object:\n" +
	"\n" +
	"{\"results\": [<per-posting object>, ...]}\n" +
	"\n" +
	"Each entry in \"results\" matches the single-posting schema above and must carry its own \"posting_id\". Emit exactly one entry per input posting_id — no extras, no omissions. The \"posting_id\" must echo back the value from the input unchanged.\n"
