// Validate runs the shared classifier-payload rules (internal/enrich/classify)
// against an AgentResponse and adapts the structured failures into the
// operator-facing retry hints the batch-enrich retry loop already speaks. It
// returns a ValidatedClassification on success or a ValidationFailure listing
// every hint on rejection. Rules are evaluated independently so the agent
// receives all failure hints at once during the retry loop.
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/classify"
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
// single-posting retry. Unexpected IDs in the response are dropped with a
// Warn-level log so hallucinated posting_ids surface at the default log level.
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
			slog.Warn("[batch-enrich] dropping unexpected posting_id in batched response", "posting_id", r.PostingID)
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

// ValidatedClassification is a parsed AgentResponse that passed every shared
// validation rule. Writeback resolves taxonomy IDs from the slugs inside
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

// Validate runs the shared classifier-payload rules against resp and returns a
// ValidatedClassification on success. On rejection it returns a
// ValidationFailure whose hints are adapted from the shared structured failures
// — preserving the exact retry-prompt wording the agent contract expects. The
// returned error is always a ValidationFailure (or nil) so callers can
// type-assert without inspecting wrap chains.
func Validate(resp AgentResponse, taxonomy Taxonomy) (ValidatedClassification, error) {
	fails := classify.Validate(resp, taxonomy)
	if len(fails) == 0 {
		return ValidatedClassification{AgentResponse: resp}, nil
	}
	return ValidatedClassification{}, ValidationFailure{Hints: failuresToHints(resp, taxonomy, fails)}
}

// failuresToHints adapts shared structured failures into the batch-enrich retry
// hints. It reuses the existing FormatHint templates (prompt.go) so the agent
// keeps seeing the same guidance text, and de-duplicates while preserving order.
//
// The shared slug_collision code covers two cases the retry prompt distinguishes:
// a collision against existing taxonomy (FailCrossTableCollision) and a slug that
// appears in two payload arrays (FailWithinResponseCrossTable). The within-payload
// message is the only one naming two tables, so the message text disambiguates
// them without threading extra state through the structured failure.
func failuresToHints(resp AgentResponse, taxonomy Taxonomy, fails []classify.Failure) []string {
	hints := make([]string, 0, len(fails))
	seen := make(map[string]struct{}, len(fails))
	addHint := func(h string) {
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		hints = append(hints, h)
	}

	for _, f := range fails {
		var hint string
		switch f.Code {
		case classify.CodeMissingSeniority:
			hint = FormatHint(FailSeniorityMissing)
		case classify.CodeInvalidSeniority:
			hint = FormatHint(FailSeniorityInvalid, resp.Classification.Seniority)
		case classify.CodeInvalidSlug:
			hint = slugHintFor(resp, f.Path)
		case classify.CodeSlugTooLong:
			// Over-length slugs map to the same invalid-slug guidance, which already
			// names the 64-char ceiling.
			hint = slugHintFor(resp, f.Path)
		case classify.CodeDuplicateSlug:
			hint = FormatHint(FailDuplicateSlug, slugAt(resp, f.Path))
		case classify.CodeSlugCollision:
			hint = collisionHintFor(f)
		case classify.CodeEmptyDimensions:
			hint = FormatHint(FailEmptyDimensions, roleSlugForDimsPath(resp, f.Path))
		case classify.CodeUnknownDimension:
			hint = FormatHint(FailUnknownDimension, dimAt(resp, f.Path), knownDimensionSlugsHint(taxonomy))
		case classify.CodeNullByte:
			hint = FormatHint(FailNullByte, nullByteField(f.Path))
		case classify.CodeNotesTooLong:
			hint = FormatHint(FailNotesTooLong, notesField(f.Path), "")
		default:
			hint = f.Message
		}
		addHint(hint)
	}
	return hints
}

// slugHintFor renders the invalid-slug hint. An empty slug keeps the legacy
// "missing or empty" wording; otherwise it names the offending slug.
func slugHintFor(resp AgentResponse, path string) string {
	slug := slugAt(resp, path)
	if slug == "" {
		return "slug is missing or empty"
	}
	return FormatHint(FailInvalidSlug, slug)
}

// collisionHintFor renders the retry hint for a slug_collision failure. The
// shared validator already formats both flavours to match the batch-enrich
// templates: within-payload collisions ("`x` appears in both A and B. A slug
// belongs to one table only.") match FailWithinResponseCrossTable, and
// existing-taxonomy collisions ("`x` is already a A. Choose a different slug.")
// match FailCrossTableCollision. The message is returned straight through so we
// do not re-derive owner state already captured structurally.
func collisionHintFor(f classify.Failure) string {
	return f.Message
}

// slugAt extracts the slug value referenced by a "<array>[i].slug" path so the
// hint names the offending value rather than the path.
func slugAt(resp AgentResponse, path string) string {
	arr, i, ok := parseArrayIndex(path)
	if !ok {
		return ""
	}
	switch arr {
	case "canonical_roles":
		if i < len(resp.CanonicalRoles) {
			return resp.CanonicalRoles[i].Slug
		}
	case "specializations":
		if i < len(resp.Specializations) {
			return resp.Specializations[i].Slug
		}
	case "skills":
		if i < len(resp.Skills) {
			return resp.Skills[i].Slug
		}
	}
	return ""
}

// roleSlugForDimsPath resolves the canonical_role slug for an empty-dimensions
// failure path ("canonical_roles[i].dimensions").
func roleSlugForDimsPath(resp AgentResponse, path string) string {
	arr, i, ok := parseArrayIndex(path)
	if ok && arr == "canonical_roles" && i < len(resp.CanonicalRoles) {
		return resp.CanonicalRoles[i].Slug
	}
	return ""
}

// dimAt resolves the dimension slug for an unknown-dimension failure path
// ("canonical_roles[i].dimensions[j]").
func dimAt(resp AgentResponse, path string) string {
	i, j, ok := parseNestedIndex(path)
	if !ok || i >= len(resp.CanonicalRoles) {
		return ""
	}
	dims := resp.CanonicalRoles[i].Dimensions
	if j < len(dims) {
		return dims[j]
	}
	return ""
}

// nullByteField maps a structured path to the field label the FailNullByte
// template expects (e.g. "canonical_role.name", "classification.notes").
func nullByteField(path string) string {
	switch {
	case path == "classification.notes":
		return "classification.notes"
	case strings.HasPrefix(path, "canonical_roles") && strings.HasSuffix(path, ".name"):
		return "canonical_role.name"
	case strings.HasPrefix(path, "canonical_roles") && strings.HasSuffix(path, ".notes"):
		return "canonical_role.notes"
	case strings.HasPrefix(path, "specializations"):
		return "specialization.name"
	case strings.HasPrefix(path, "skills"):
		return "skill.name"
	default:
		return path
	}
}

// notesField maps a notes_too_long path to the field label the FailNotesTooLong
// template expects.
func notesField(path string) string {
	if strings.HasPrefix(path, "canonical_roles") {
		return "canonical_role.notes"
	}
	return "classification.notes"
}

// knownDimensionSlugsHint returns the sorted, comma-separated dimension slug
// list for the FailUnknownDimension hint, mirroring the shared validator's
// empty-table fallback.
func knownDimensionSlugsHint(taxonomy Taxonomy) string {
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

// parseArrayIndex parses "<array>[i].<field>" into the array name and index.
func parseArrayIndex(path string) (arr string, idx int, ok bool) {
	openIdx := strings.IndexByte(path, '[')
	closeIdx := strings.IndexByte(path, ']')
	if openIdx < 0 || closeIdx < openIdx {
		return "", 0, false
	}
	arr = path[:openIdx]
	if _, err := fmt.Sscanf(path[openIdx+1:closeIdx], "%d", &idx); err != nil {
		return "", 0, false
	}
	return arr, idx, true
}

// parseNestedIndex parses "canonical_roles[i].dimensions[j]" into i and j.
func parseNestedIndex(path string) (i, j int, ok bool) {
	if _, err := fmt.Sscanf(path, "canonical_roles[%d].dimensions[%d]", &i, &j); err != nil {
		return 0, 0, false
	}
	return i, j, true
}
