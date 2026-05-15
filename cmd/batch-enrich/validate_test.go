package main

import (
	"errors"
	"strings"
	"testing"
)

// newTestTaxonomy builds an in-memory Taxonomy with a small but realistic
// set of slugs in each table. Cross-table index is built so collision
// checks fire as they would in production.
func newTestTaxonomy() Taxonomy {
	t := Taxonomy{
		CanonicalRoles: map[string]TaxonomyEntry{
			"software-engineer": {ID: 1, Name: "Software Engineer"},
		},
		Specializations: map[string]TaxonomyEntry{
			"frontend": {ID: 10, Name: "Frontend"},
		},
		Skills: map[string]TaxonomyEntry{
			"typescript": {ID: 20, Name: "TypeScript"},
		},
		RoleDimensions: map[string]TaxonomyEntry{
			"ic":          {ID: 30, Name: "Individual Contributor"},
			"engineering": {ID: 31, Name: "Engineering"},
			"design":      {ID: 32, Name: "Design"},
		},
	}
	return t.BuildCrossTableIndex()
}

// validResponse returns an AgentResponse that should pass every Phase A
// rule. Tests mutate copies of this fixture to isolate one rule per case.
func validResponse() AgentResponse {
	return AgentResponse{
		PostingID: 1,
		Classification: AgentClassification{
			Seniority: "senior",
			Notes:     "design-engineering hybrid",
		},
		CanonicalRoles: []AgentCanonicalRole{
			{
				Slug:       "software-engineer",
				Name:       "Software Engineer",
				Dimensions: []string{"ic", "engineering"},
			},
		},
		Specializations: []AgentSpecialization{
			{Slug: "frontend", Name: "Frontend"},
		},
		Skills: []AgentSkill{
			{Slug: "typescript", Name: "TypeScript", Requirement: "required"},
		},
		Summary: "Senior software engineer role focused on frontend work.",
	}
}

// asValidationFailure asserts err is a ValidationFailure and returns it.
// Centralises the type-assert so individual cases stay focused on which
// hint they expect to see.
func asValidationFailure(t *testing.T, err error) ValidationFailure {
	t.Helper()
	var vf ValidationFailure
	if !errors.As(err, &vf) {
		t.Fatalf("expected ValidationFailure, got %T: %v", err, err)
	}
	return vf
}

// hintsContain reports whether any hint contains substr. Hints are
// operator-facing prose, so substring matching is the right granularity —
// asserting on exact text would make every wording tweak a test churn.
func hintsContain(hints []string, substr string) bool {
	for _, h := range hints {
		if strings.Contains(h, substr) {
			return true
		}
	}
	return false
}

func TestValidate_ValidResponse_NoError(t *testing.T) {
	tax := newTestTaxonomy()
	got, err := Validate(validResponse(), tax)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got.AgentResponse.PostingID != 1 {
		t.Fatalf("expected PostingID 1, got %d", got.AgentResponse.PostingID)
	}
}

func TestValidate_InvalidSlugFormat(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.CanonicalRoles[0].Slug = "Bad_Slug" // uppercase + underscore
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "Bad_Slug") || !hintsContain(vf.Hints, "valid slug") {
		t.Fatalf("expected FailInvalidSlug hint mentioning Bad_Slug, got %v", vf.Hints)
	}
}

func TestValidate_NullByteInName(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.CanonicalRoles[0].Name = "Software\x00Engineer"
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "canonical_role.name") || !hintsContain(vf.Hints, "null byte") {
		t.Fatalf("expected FailNullByte hint on canonical_role.name, got %v", vf.Hints)
	}
}

func TestValidate_NullByteInNotes(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.CanonicalRoles[0].Notes = "has a \x00 byte"
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "canonical_role.notes") || !hintsContain(vf.Hints, "null byte") {
		t.Fatalf("expected FailNullByte hint on canonical_role.notes, got %v", vf.Hints)
	}
}

func TestValidate_MissingSeniority(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.Classification.Seniority = ""
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "seniority is required") {
		t.Fatalf("expected FailSeniorityMissing hint, got %v", vf.Hints)
	}
}

func TestValidate_InvalidSeniority(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.Classification.Seniority = "ultra"
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "ultra") || !hintsContain(vf.Hints, "not a valid seniority") {
		t.Fatalf("expected FailSeniorityInvalid hint mentioning ultra, got %v", vf.Hints)
	}
}

func TestValidate_EmptyDimensions(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.CanonicalRoles[0].Dimensions = nil
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "software-engineer") || !hintsContain(vf.Hints, "no dimensions") {
		t.Fatalf("expected FailEmptyDimensions hint, got %v", vf.Hints)
	}
}

func TestValidate_UnknownDimension(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.CanonicalRoles[0].Dimensions = []string{"ic", "moonshot"}
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "moonshot") || !hintsContain(vf.Hints, "not a known dimension") {
		t.Fatalf("expected FailUnknownDimension hint mentioning moonshot, got %v", vf.Hints)
	}
	// Known-slug list should be sorted, comma-separated.
	if !hintsContain(vf.Hints, "design, engineering, ic") {
		t.Fatalf("expected sorted dimension list in hint, got %v", vf.Hints)
	}
}

func TestValidate_CrossTableCollision_RoleVsSpecialization(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	// Re-use the existing specialization slug "frontend" as a canonical_role
	// slug. The cross-table check should fire because "frontend" is already
	// owned by specializations.
	resp.CanonicalRoles = append(resp.CanonicalRoles, AgentCanonicalRole{
		Slug:       "frontend",
		Name:       "Frontend",
		Dimensions: []string{"ic"},
	})
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "frontend") || !hintsContain(vf.Hints, "specializations") {
		t.Fatalf("expected FailCrossTableCollision hint for frontend vs specializations, got %v", vf.Hints)
	}
}

func TestValidate_CrossTableCollision_WithDimensionSlug(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	// Emit a canonical_role whose slug matches an existing dimension slug.
	resp.CanonicalRoles = append(resp.CanonicalRoles, AgentCanonicalRole{
		Slug:       "engineering",
		Name:       "Engineering",
		Dimensions: []string{"ic"},
	})
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "engineering") || !hintsContain(vf.Hints, "role_dimensions") {
		t.Fatalf("expected FailCrossTableCollision hint for engineering vs role_dimensions, got %v", vf.Hints)
	}
}

func TestValidate_MultipleFailures_AllBundled(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.Classification.Seniority = ""                       // missing seniority
	resp.CanonicalRoles[0].Slug = "Bad_Slug"                 // invalid slug format
	resp.CanonicalRoles[0].Name = "x\x00y"                   // null byte in name
	resp.CanonicalRoles[0].Dimensions = []string{"moonshot"} // unknown dimension
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)

	wantSubstrings := []string{
		"seniority is required",
		"Bad_Slug",
		"null byte",
		"moonshot",
	}
	for _, want := range wantSubstrings {
		if !hintsContain(vf.Hints, want) {
			t.Errorf("expected bundled hint containing %q, got %v", want, vf.Hints)
		}
	}
	if len(vf.Hints) < len(wantSubstrings) {
		t.Errorf("expected at least %d hints, got %d: %v", len(wantSubstrings), len(vf.Hints), vf.Hints)
	}
}

func TestValidate_SlugTooLong(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.CanonicalRoles[0].Slug = strings.Repeat("a", maxSlugLen+1)
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "valid slug") {
		t.Fatalf("expected FailInvalidSlug hint for over-length slug, got %v", vf.Hints)
	}
}

func TestValidate_NotesTooLong_ClassificationNotes(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.Classification.Notes = strings.Repeat("x", maxNotesLen+1)
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "too long") {
		t.Fatalf("expected FailNotesTooLong hint, got %v", vf.Hints)
	}
}

func TestValidate_NotesTooLong_RoleNotes(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.CanonicalRoles[0].Notes = strings.Repeat("x", maxNotesLen+1)
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "too long") {
		t.Fatalf("expected FailNotesTooLong hint for role notes, got %v", vf.Hints)
	}
}

func TestValidate_DuplicateSlug_CanonicalRoles(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	// Add a second canonical role with the same slug as the first.
	resp.CanonicalRoles = append(resp.CanonicalRoles, AgentCanonicalRole{
		Slug:       "software-engineer",
		Name:       "Software Engineer Duplicate",
		Dimensions: []string{"ic"},
	})
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "software-engineer") || !hintsContain(vf.Hints, "more than once") {
		t.Fatalf("expected FailDuplicateSlug hint for software-engineer, got %v", vf.Hints)
	}
}

func TestValidate_DuplicateSlug_Specializations(t *testing.T) {
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.Specializations = append(resp.Specializations, AgentSpecialization{
		Slug: "frontend",
		Name: "Frontend Duplicate",
	})
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "frontend") || !hintsContain(vf.Hints, "more than once") {
		t.Fatalf("expected FailDuplicateSlug hint for frontend, got %v", vf.Hints)
	}
}

func TestValidate_WithinResponseCrossTable_SpecAndSkill(t *testing.T) {
	// A slug that appears in both specializations[] and skills[] must be
	// rejected even when neither slug exists in the snapshot.
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.Specializations = append(resp.Specializations, AgentSpecialization{
		Slug: "rlhf",
		Name: "RLHF",
	})
	resp.Skills = append(resp.Skills, AgentSkill{
		Slug: "rlhf",
		Name: "RLHF",
	})
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "rlhf") ||
		!hintsContain(vf.Hints, "specializations") ||
		!hintsContain(vf.Hints, "skills") {
		t.Fatalf("expected FailWithinResponseCrossTable hint for rlhf, got %v", vf.Hints)
	}
	if !hintsContain(vf.Hints, "belongs to one table only") {
		t.Fatalf("expected 'belongs to one table only' in hint, got %v", vf.Hints)
	}
}

func TestValidate_WithinResponseCrossTable_RoleAndSkill(t *testing.T) {
	// A slug that appears in both canonical_roles[] and skills[] must be
	// rejected.
	tax := newTestTaxonomy()
	resp := validResponse()
	resp.CanonicalRoles = append(resp.CanonicalRoles, AgentCanonicalRole{
		Slug:       "distributed-training",
		Name:       "Distributed Training",
		Dimensions: []string{"ic"},
	})
	resp.Skills = append(resp.Skills, AgentSkill{
		Slug: "distributed-training",
		Name: "Distributed Training",
	})
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "distributed-training") ||
		!hintsContain(vf.Hints, "canonical_roles") ||
		!hintsContain(vf.Hints, "skills") {
		t.Fatalf("expected FailWithinResponseCrossTable hint for distributed-training, got %v", vf.Hints)
	}
}

func TestValidate_UnknownDimension_EmptyRoleDimensions(t *testing.T) {
	// When the role_dimensions table is empty the hint must not end with
	// "use one of: ." — that reads as an operator error rather than guidance.
	tax := Taxonomy{
		CanonicalRoles:  map[string]TaxonomyEntry{"software-engineer": {ID: 1, Name: "Software Engineer"}},
		Specializations: map[string]TaxonomyEntry{},
		Skills:          map[string]TaxonomyEntry{},
		RoleDimensions:  map[string]TaxonomyEntry{},
	}
	tax = tax.BuildCrossTableIndex()
	resp := validResponse()
	resp.CanonicalRoles[0].Dimensions = []string{"something"}
	_, err := Validate(resp, tax)
	vf := asValidationFailure(t, err)
	if !hintsContain(vf.Hints, "something") || !hintsContain(vf.Hints, "not a known dimension") {
		t.Fatalf("expected FailUnknownDimension hint, got %v", vf.Hints)
	}
	// Guard must fire: no hint should end with "use one of: ."
	for _, h := range vf.Hints {
		if strings.HasSuffix(h, "use one of: .") {
			t.Fatalf("hint ends with truncated 'use one of: .' — empty-table guard did not fire: %q", h)
		}
	}
}
