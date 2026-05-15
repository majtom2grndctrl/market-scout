// Validate runs every validation rule against an AgentResponse and returns a
// ValidatedClassification on success or a ValidationFailure listing every
// failure on rejection. Rules are evaluated independently so the agent
// receives all failure hints at once during the retry loop.
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
)

// BatchedAgentResponse is the top-level wrapper the Haiku agent emits when
// processing multiple postings in a single call.
type BatchedAgentResponse struct {
	Results []AgentResponse `json:"results"`
}

// ParseBatchedResponse unmarshals a batched agent response and routes each
// per-posting result by posting_id. agentText must already be stripped of any
// fenced-code markers — callers in dispatch.go handle that step.
//
// On JSON parse failure the function returns a wrapped error and no partial
// results; this is the signal callers use to retry the whole batch in
// single-posting mode.
//
// On parse success, the function returns the slice of per-posting results
// keyed by posting_id alongside the subset of expected IDs that were absent
// from the response. Missing IDs are not an error: a partial batch is a
// success for the IDs that landed, and the caller routes missing IDs to
// single-posting retry. Unexpected IDs in the response are silently dropped
// (debug-logged) so a hallucinated posting_id can't poison the batch.
func ParseBatchedResponse(agentText string, expected []int64) (results map[int64]AgentResponse, missing []int64, err error) {
	var wrapper BatchedAgentResponse
	if err := json.Unmarshal([]byte(agentText), &wrapper); err != nil {
		return nil, nil, fmt.Errorf("parsing batched agent response: %w", err)
	}

	expectedSet := make(map[int64]struct{}, len(expected))
	for _, id := range expected {
		expectedSet[id] = struct{}{}
	}

	results = make(map[int64]AgentResponse, len(wrapper.Results))
	for _, r := range wrapper.Results {
		if _, ok := expectedSet[r.PostingID]; !ok {
			slog.Debug("[batch-enrich] dropping unexpected posting_id in batched response", "posting_id", r.PostingID)
			continue
		}
		results[r.PostingID] = r
	}

	for _, id := range expected {
		if _, ok := results[id]; !ok {
			missing = append(missing, id)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })

	return results, missing, nil
}

// AgentResponse mirrors the JSON schema the Haiku classifier emits per
// posting. The skills[].requirement field is parsed but not persisted — no
// storage column exists. Kept in the schema so agent outputs carry the signal;
// write it through once the column is added.
type AgentResponse struct {
	PostingID       int64                 `json:"posting_id"`
	Classification  AgentClassification   `json:"classification"`
	CanonicalRoles  []AgentCanonicalRole  `json:"canonical_roles"`
	Specializations []AgentSpecialization `json:"specializations"`
	Skills          []AgentSkill          `json:"skills"`
	Summary         string                `json:"summary"`
}

// AgentClassification carries the posting-level classification fields.
// `Seniority` is required and validated against a closed set; `Notes` is
// optional and may arrive as the empty string when the agent omitted it.
type AgentClassification struct {
	Seniority string `json:"seniority"`
	Notes     string `json:"notes"`
}

// AgentCanonicalRole is one canonical-role entry from the agent. A posting
// may emit multiple roles (blended roles are first-class). `Dimensions`
// must be non-empty and each slug must exist in the role_dimensions
// taxonomy.
type AgentCanonicalRole struct {
	Slug       string   `json:"slug"`
	Name       string   `json:"name"`
	Dimensions []string `json:"dimensions"`
	Notes      string   `json:"notes,omitempty"`
}

// AgentSpecialization is one specialization entry from the agent.
type AgentSpecialization struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// AgentSkill is one skill entry from the agent. Requirement is parsed but not
// persisted — no storage column exists. Kept in the schema so agent outputs
// carry the signal; write it through once the column is added.
type AgentSkill struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Requirement string `json:"requirement,omitempty"`
}

// ValidatedClassification is a parsed AgentResponse that passed every
// Phase A rule. Writeback resolves taxonomy IDs from the slugs inside
// AgentResponse — no IDs are carried at this stage.
type ValidatedClassification struct {
	AgentResponse AgentResponse
}

// Outcome is the terminal state of one posting in a batch-enrich run.
// Logged in failures.jsonl and surfaced in the per-run report.
type Outcome string

const (
	OutcomeEnriched         Outcome = "enriched"
	OutcomeJSONFailed       Outcome = "json_failed"
	OutcomeValidationFailed Outcome = "validation_failed"
	OutcomeDBFailed         Outcome = "db_failed"
)

// ValidationFailure carries the operator-facing hints produced by every
// rule that fired during Validate. Returned as an error so callers can
// flow it through normal error paths; the retry loop reads .Hints
// directly to render guidance back to the agent.
type ValidationFailure struct {
	Hints []string
}

// Error joins the hints into one line. Useful for logging; programmatic
// callers should read .Hints to preserve structure.
func (vf ValidationFailure) Error() string { return strings.Join(vf.Hints, "; ") }

// slugPattern matches the required kebab-case slug shape: lowercase ASCII
// letters and digits, optional single-hyphen separators, no
// leading/trailing/consecutive hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// maxSlugLen is the 64-char ceiling for slug fields.
const maxSlugLen = 64

// maxNotesLen caps the notes field length to prevent multi-MB values from
// reaching the DB. 4096 characters accommodates any reasonable annotation.
const maxNotesLen = 4096

// validSeniorities is the closed set for the seniority field. Order here
// matches the order the agent contract presents to the model.
var validSeniorities = map[string]struct{}{
	"intern":    {},
	"junior":    {},
	"mid":       {},
	"senior":    {},
	"staff":     {},
	"principal": {},
	"lead":      {},
	"director":  {},
	"unknown":   {},
}

// Validate runs every Phase A rule against resp and returns a
// ValidatedClassification on success. All rules are evaluated regardless
// of earlier failures so the retry prompt can address every problem in
// one round-trip. The returned error is always a ValidationFailure (or
// nil) — callers can type-assert without inspecting wrap chains.
func Validate(resp AgentResponse, taxonomy Taxonomy) (ValidatedClassification, error) {
	var hints []string

	// Seniority rules. Treat whitespace-only as missing — the agent
	// contract requires the canonical "unknown" sentinel when undeterminable.
	seniority := resp.Classification.Seniority
	if strings.TrimSpace(seniority) == "" {
		hints = append(hints, FormatHint(FailSeniorityMissing))
	} else if _, ok := validSeniorities[seniority]; !ok {
		hints = append(hints, FormatHint(FailSeniorityInvalid, seniority))
	}

	// Null-byte checks. Parameterized sqlc queries handle SQL injection;
	// these checks catch null bytes that Postgres rejects at the protocol level.
	if strings.ContainsRune(resp.Classification.Notes, '\x00') {
		hints = append(hints, FormatHint(FailNullByte, "classification.notes"))
	}
	if len(resp.Classification.Notes) > maxNotesLen {
		hints = append(hints, FormatHint(FailNotesTooLong, "classification.notes"))
	}

	// Within-response cross-table check. Runs before the per-array loops so
	// a slug that appears in two minting arrays produces one clear rejection
	// rather than a confusing pair of cross-table hints against the snapshot.
	hints = append(hints, checkWithinResponseCrossTable(resp)...)

	// Canonical roles: slug, name, notes, dimensions, cross-table, duplicates.
	seenRoleSlugs := make(map[string]struct{}, len(resp.CanonicalRoles))
	for _, role := range resp.CanonicalRoles {
		isDup := false
		if _, dup := seenRoleSlugs[role.Slug]; dup {
			hints = append(hints, FormatHint(FailDuplicateSlug, role.Slug))
			isDup = true
		}
		if !isDup {
			seenRoleSlugs[role.Slug] = struct{}{}
		}
		hints = append(hints, checkSlug(role.Slug)...)
		if strings.ContainsRune(role.Name, '\x00') {
			hints = append(hints, FormatHint(FailNullByte, "canonical_role.name"))
		}
		if strings.ContainsRune(role.Notes, '\x00') {
			hints = append(hints, FormatHint(FailNullByte, "canonical_role.notes"))
		}
		if len(role.Notes) > maxNotesLen {
			hints = append(hints, FormatHint(FailNotesTooLong, "canonical_role.notes"))
		}
		if len(role.Dimensions) == 0 {
			hints = append(hints, FormatHint(FailEmptyDimensions, role.Slug))
		}
		for _, dim := range role.Dimensions {
			if _, ok := taxonomy.RoleDimensions[dim]; !ok {
				hints = append(hints, FormatHint(FailUnknownDimension, dim, knownDimensionSlugs(taxonomy)))
			}
		}
		hints = append(hints, checkCrossTable(role.Slug, "canonical_roles", taxonomy)...)
	}

	// Specializations.
	seenSpecSlugs := make(map[string]struct{}, len(resp.Specializations))
	for _, spec := range resp.Specializations {
		isDup := false
		if _, dup := seenSpecSlugs[spec.Slug]; dup {
			hints = append(hints, FormatHint(FailDuplicateSlug, spec.Slug))
			isDup = true
		}
		if !isDup {
			seenSpecSlugs[spec.Slug] = struct{}{}
		}
		hints = append(hints, checkSlug(spec.Slug)...)
		if strings.ContainsRune(spec.Name, '\x00') {
			hints = append(hints, FormatHint(FailNullByte, "specialization.name"))
		}
		hints = append(hints, checkCrossTable(spec.Slug, "specializations", taxonomy)...)
	}

	// Skills.
	seenSkillSlugs := make(map[string]struct{}, len(resp.Skills))
	for _, skill := range resp.Skills {
		isDup := false
		if _, dup := seenSkillSlugs[skill.Slug]; dup {
			hints = append(hints, FormatHint(FailDuplicateSlug, skill.Slug))
			isDup = true
		}
		if !isDup {
			seenSkillSlugs[skill.Slug] = struct{}{}
		}
		hints = append(hints, checkSlug(skill.Slug)...)
		if strings.ContainsRune(skill.Name, '\x00') {
			hints = append(hints, FormatHint(FailNullByte, "skill.name"))
		}
		hints = append(hints, checkCrossTable(skill.Slug, "skills", taxonomy)...)
	}

	if len(hints) > 0 {
		// De-duplicate hints while preserving order. Duplicate hints arise
		// when two roles share an unknown dimension slug — the agent should see
		// the hint once, not once per occurrence.
		seen := make(map[string]struct{}, len(hints))
		deduped := hints[:0]
		for _, h := range hints {
			if _, ok := seen[h]; !ok {
				seen[h] = struct{}{}
				deduped = append(deduped, h)
			}
		}
		hints = deduped
		return ValidatedClassification{}, ValidationFailure{Hints: hints}
	}
	return ValidatedClassification{AgentResponse: resp}, nil
}

// checkSlug applies Rule A: slug must match slugPattern and not exceed
// maxSlugLen. Returns zero or one hint.
func checkSlug(slug string) []string {
	if len(slug) > maxSlugLen || !slugPattern.MatchString(slug) {
		return []string{FormatHint(FailInvalidSlug, slug)}
	}
	return nil
}

// checkCrossTable fires FailCrossTableCollision when slug is already
// owned by a different taxonomy table. A slug that the agent emits as a
// canonical_role and that already exists as a canonical_role is fine —
// the collision rule is cross-table only.
func checkCrossTable(slug, emittingTable string, taxonomy Taxonomy) []string {
	owner, ok := taxonomy.CrossTableOwner(slug)
	if !ok || owner == emittingTable {
		return nil
	}
	return []string{FormatHint(FailCrossTableCollision, slug, owner)}
}

// checkWithinResponseCrossTable rejects slugs that appear in more than one
// of the three minting arrays in a single agent response. role_dimensions is
// a closed, seeded set the agent never mints into and is excluded.
//
// Each collision produces one hint naming both tables so the agent can
// choose which array the slug truly belongs in. Pairs are emitted in
// alphabetical table order for stable, de-duplicate-friendly hint strings.
func checkWithinResponseCrossTable(resp AgentResponse) []string {
	// tableOf maps slug → first table name that claimed it.
	tableOf := make(map[string]string)
	var hints []string

	add := func(slug, table string) {
		if existing, conflict := tableOf[slug]; conflict {
			// Alphabetical order so "canonical_roles / skills" is always
			// the same string regardless of which array was iterated first.
			first, second := existing, table
			if first > second {
				first, second = second, first
			}
			hints = append(hints, FormatHint(FailWithinResponseCrossTable, slug, first, second))
			return
		}
		tableOf[slug] = table
	}

	for _, role := range resp.CanonicalRoles {
		add(role.Slug, "canonical_roles")
	}
	for _, spec := range resp.Specializations {
		add(spec.Slug, "specializations")
	}
	for _, skill := range resp.Skills {
		add(skill.Slug, "skills")
	}
	return hints
}

// knownDimensionSlugs returns a sorted, comma-separated list of every
// dimension slug in the taxonomy. Sorted so retry hints are stable across
// runs — the agent sees the same string each time. Returns a fallback
// message if the table is empty so hints don't end with "use one of: .".
func knownDimensionSlugs(taxonomy Taxonomy) string {
	if len(taxonomy.RoleDimensions) == 0 {
		return "(none — role_dimensions table is empty)"
	}
	slugs := make([]string, 0, len(taxonomy.RoleDimensions))
	for slug := range taxonomy.RoleDimensions {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	return strings.Join(slugs, ", ")
}
