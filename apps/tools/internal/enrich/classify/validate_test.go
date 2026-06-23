package classify

import (
	"strings"
	"testing"
)

// newTestTaxonomy builds an in-memory Taxonomy with a small but realistic set
// of slugs in each table. The cross-table index is built so collision checks
// fire as they would in production.
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

// validResponse returns an AgentResponse that passes every rule. Tests mutate
// copies to isolate one rule per case.
func validResponse() AgentResponse {
	return AgentResponse{
		PostingID: 1,
		Classification: AgentClassification{
			Seniority: "senior",
			Notes:     "design-engineering hybrid",
		},
		CanonicalRoles: []AgentCanonicalRole{
			{Slug: "software-engineer", Name: "Software Engineer", Dimensions: []string{"ic", "engineering"}},
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

// failureWith returns the first failure whose code matches, or fails the test.
func failureWith(t *testing.T, fails []Failure, code Code) Failure {
	t.Helper()
	for _, f := range fails {
		if f.Code == code {
			return f
		}
	}
	t.Fatalf("expected a %q failure, got %+v", code, fails)
	return Failure{}
}

func hasCode(fails []Failure, code Code) bool {
	for _, f := range fails {
		if f.Code == code {
			return true
		}
	}
	return false
}

func TestValidate_ValidResponse_NoFailures(t *testing.T) {
	if fails := Validate(validResponse(), newTestTaxonomy()); len(fails) != 0 {
		t.Fatalf("expected no failures, got %+v", fails)
	}
}

func TestValidate_MissingSeniority(t *testing.T) {
	resp := validResponse()
	resp.Classification.Seniority = "   "
	f := failureWith(t, Validate(resp, newTestTaxonomy()), CodeMissingSeniority)
	if f.Path != "classification.seniority" {
		t.Fatalf("path = %q, want classification.seniority", f.Path)
	}
}

func TestValidate_InvalidSeniority(t *testing.T) {
	resp := validResponse()
	resp.Classification.Seniority = "ultra"
	f := failureWith(t, Validate(resp, newTestTaxonomy()), CodeInvalidSeniority)
	if f.Path != "classification.seniority" || !strings.Contains(f.Message, "ultra") {
		t.Fatalf("unexpected failure %+v", f)
	}
}

func TestValidate_PaddedSeniority_Rejected(t *testing.T) {
	resp := validResponse()
	resp.Classification.Seniority = " senior "
	f := failureWith(t, Validate(resp, newTestTaxonomy()), CodeInvalidSeniority)
	if f.Path != "classification.seniority" {
		t.Fatalf("path = %q, want classification.seniority", f.Path)
	}
}

func TestValidate_InvalidSlugFormat(t *testing.T) {
	resp := validResponse()
	resp.CanonicalRoles[0].Slug = "Bad_Slug"
	f := failureWith(t, Validate(resp, newTestTaxonomy()), CodeInvalidSlug)
	if f.Path != "canonical_roles[0].slug" {
		t.Fatalf("path = %q, want canonical_roles[0].slug", f.Path)
	}
}

func TestValidate_EmptySlug_InvalidSlug(t *testing.T) {
	resp := validResponse()
	resp.Specializations[0].Slug = ""
	f := failureWith(t, Validate(resp, newTestTaxonomy()), CodeInvalidSlug)
	if f.Path != "specializations[0].slug" {
		t.Fatalf("path = %q, want specializations[0].slug", f.Path)
	}
}

func TestValidate_SlugTooLong_DistinctCode(t *testing.T) {
	resp := validResponse()
	resp.Skills[0].Slug = strings.Repeat("a", MaxSlugLen+1)
	fails := Validate(resp, newTestTaxonomy())
	f := failureWith(t, fails, CodeSlugTooLong)
	if f.Path != "skills[0].slug" {
		t.Fatalf("path = %q, want skills[0].slug", f.Path)
	}
	if hasCode(fails, CodeInvalidSlug) {
		t.Fatalf("over-length slug must report slug_too_long only, got %+v", fails)
	}
}

func TestValidate_DuplicateSlug(t *testing.T) {
	resp := validResponse()
	resp.Skills = append(resp.Skills, AgentSkill{Slug: "typescript", Name: "TypeScript Dup"})
	f := failureWith(t, Validate(resp, newTestTaxonomy()), CodeDuplicateSlug)
	if f.Path != "skills[1].slug" {
		t.Fatalf("path = %q, want skills[1].slug (duplicate occurrence)", f.Path)
	}
}

func TestValidate_SlugCollision_AgainstExistingTaxonomy(t *testing.T) {
	resp := validResponse()
	// "frontend" exists as a specialization; emitting it as a canonical_role collides.
	resp.CanonicalRoles = append(resp.CanonicalRoles, AgentCanonicalRole{
		Slug: "frontend", Name: "Frontend", Dimensions: []string{"ic"},
	})
	f := failureWith(t, Validate(resp, newTestTaxonomy()), CodeSlugCollision)
	if !strings.Contains(f.Message, "specializations") {
		t.Fatalf("collision message should name owning table specializations, got %q", f.Message)
	}
}

func TestValidate_SlugCollision_WithinPayload(t *testing.T) {
	resp := validResponse()
	resp.Specializations = append(resp.Specializations, AgentSpecialization{Slug: "rlhf", Name: "RLHF"})
	resp.Skills = append(resp.Skills, AgentSkill{Slug: "rlhf", Name: "RLHF"})
	f := failureWith(t, Validate(resp, newTestTaxonomy()), CodeSlugCollision)
	if !strings.Contains(f.Message, "specializations") || !strings.Contains(f.Message, "skills") ||
		!strings.Contains(f.Message, "belongs to one table only") {
		t.Fatalf("within-payload collision message unexpected: %q", f.Message)
	}
}

func TestValidate_EmptyDimensions(t *testing.T) {
	resp := validResponse()
	resp.CanonicalRoles[0].Dimensions = nil
	f := failureWith(t, Validate(resp, newTestTaxonomy()), CodeEmptyDimensions)
	if f.Path != "canonical_roles[0].dimensions" {
		t.Fatalf("path = %q, want canonical_roles[0].dimensions", f.Path)
	}
}

func TestValidate_UnknownDimension(t *testing.T) {
	resp := validResponse()
	resp.CanonicalRoles[0].Dimensions = []string{"ic", "moonshot"}
	f := failureWith(t, Validate(resp, newTestTaxonomy()), CodeUnknownDimension)
	if f.Path != "canonical_roles[0].dimensions[1]" {
		t.Fatalf("path = %q, want canonical_roles[0].dimensions[1]", f.Path)
	}
	if !strings.Contains(f.Message, "design, engineering, ic") {
		t.Fatalf("expected sorted dimension list in message, got %q", f.Message)
	}
}

func TestValidate_UnknownDimension_EmptyTable_NoTruncatedList(t *testing.T) {
	tax := Taxonomy{
		CanonicalRoles:  map[string]TaxonomyEntry{"software-engineer": {ID: 1, Name: "Software Engineer"}},
		Specializations: map[string]TaxonomyEntry{},
		Skills:          map[string]TaxonomyEntry{},
		RoleDimensions:  map[string]TaxonomyEntry{},
	}.BuildCrossTableIndex()
	resp := validResponse()
	resp.CanonicalRoles[0].Dimensions = []string{"something"}
	f := failureWith(t, Validate(resp, tax), CodeUnknownDimension)
	if strings.HasSuffix(f.Message, "Use one of: .") {
		t.Fatalf("empty-table guard did not fire: %q", f.Message)
	}
}

func TestValidate_NullByte_InNames(t *testing.T) {
	resp := validResponse()
	resp.CanonicalRoles[0].Name = "Soft\x00ware"
	f := failureWith(t, Validate(resp, newTestTaxonomy()), CodeNullByte)
	if f.Path != "canonical_roles[0].name" {
		t.Fatalf("path = %q, want canonical_roles[0].name", f.Path)
	}
}

func TestValidate_NullByte_InNotes(t *testing.T) {
	resp := validResponse()
	resp.Classification.Notes = "has a \x00 byte"
	f := failureWith(t, Validate(resp, newTestTaxonomy()), CodeNullByte)
	if f.Path != "classification.notes" {
		t.Fatalf("path = %q, want classification.notes", f.Path)
	}
}

func TestValidate_NotesTooLong(t *testing.T) {
	resp := validResponse()
	resp.Classification.Notes = strings.Repeat("x", MaxNotesLen+1)
	f := failureWith(t, Validate(resp, newTestTaxonomy()), CodeNotesTooLong)
	if f.Path != "classification.notes" {
		t.Fatalf("path = %q, want classification.notes", f.Path)
	}
}

func TestValidate_MultipleFailures_AllReported(t *testing.T) {
	resp := validResponse()
	resp.Classification.Seniority = ""
	resp.CanonicalRoles[0].Slug = "Bad_Slug"
	resp.CanonicalRoles[0].Name = "x\x00y"
	resp.CanonicalRoles[0].Dimensions = []string{"moonshot"}
	fails := Validate(resp, newTestTaxonomy())
	for _, code := range []Code{CodeMissingSeniority, CodeInvalidSlug, CodeNullByte, CodeUnknownDimension} {
		if !hasCode(fails, code) {
			t.Errorf("expected %q in failures, got %+v", code, fails)
		}
	}
}
