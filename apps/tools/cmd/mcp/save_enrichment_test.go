package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/classify"
)

// fakeTaxonomySource returns a canned taxonomy and posting-existence answer.
type fakeTaxonomySource struct {
	tax        classify.Taxonomy
	loadErr    error
	exists     bool
	existsErr  error
	gotPosting int64
}

func (f *fakeTaxonomySource) load(ctx context.Context) (classify.Taxonomy, error) {
	return f.tax, f.loadErr
}

func (f *fakeTaxonomySource) postingExists(ctx context.Context, id int64) (bool, error) {
	f.gotPosting = id
	return f.exists, f.existsErr
}

// fakeSaver records the payload/provenance it received and returns a canned
// envelope (or error) instead of touching a database.
type fakeSaver struct {
	gotPayload       json.RawMessage
	gotModel         string
	gotPromptVersion string
	result           json.RawMessage
	err              error
	called           bool
}

func (f *fakeSaver) save(ctx context.Context, payload json.RawMessage, model, promptVersion string) (json.RawMessage, error) {
	f.called = true
	f.gotPayload = payload
	f.gotModel = model
	f.gotPromptVersion = promptVersion
	return f.result, f.err
}

// neverSaver fails the test if the saver runs — asserts validation aborts first.
type neverSaver struct{ t *testing.T }

func (n neverSaver) save(ctx context.Context, payload json.RawMessage, model, promptVersion string) (json.RawMessage, error) {
	n.t.Helper()
	n.t.Fatalf("saver called, want validation to abort first")
	return nil, nil
}

func testTaxonomy() classify.Taxonomy {
	t := classify.Taxonomy{
		CanonicalRoles:  map[string]classify.TaxonomyEntry{"software-engineer": {ID: 1, Name: "Software Engineer"}},
		Specializations: map[string]classify.TaxonomyEntry{"frontend": {ID: 10, Name: "Frontend"}},
		Skills:          map[string]classify.TaxonomyEntry{"typescript": {ID: 20, Name: "TypeScript"}},
		RoleDimensions: map[string]classify.TaxonomyEntry{
			"ic": {ID: 30, Name: "IC"}, "engineering": {ID: 31, Name: "Engineering"},
		},
	}
	return t.BuildCrossTableIndex()
}

func validSaveRequest() saveEnrichmentRequest {
	return saveEnrichmentRequest{
		PostingID:      42,
		Classification: classify.AgentClassification{Seniority: "senior", Notes: "hybrid"},
		CanonicalRoles: []classify.AgentCanonicalRole{
			{Slug: "software-engineer", Name: "Software Engineer", Dimensions: []string{"ic", "engineering"}},
		},
		Specializations: []classify.AgentSpecialization{{Slug: "frontend", Name: "Frontend"}},
		Skills:          []classify.AgentSkill{{Slug: "typescript", Name: "TypeScript", Requirement: "required"}},
		Summary:         "a summary",
	}
}

// okEnvelope builds the function's success JSON with the given new taxonomy.
func okEnvelope(t *testing.T, classID int64, postingID int64, nt newTaxonomy) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(functionResult{
		Ok:               true,
		ClassificationID: &classID,
		PostingID:        &postingID,
		NewTaxonomy:      nt,
	})
	if err != nil {
		t.Fatalf("marshaling ok envelope: %v", err)
	}
	return b
}

func TestRunSaveEnrichment_ProvenanceDefaultsApplied(t *testing.T) {
	tax := &fakeTaxonomySource{tax: testTaxonomy(), exists: true}
	saver := &fakeSaver{result: okEnvelope(t, 100, 42, newTaxonomy{})}

	env := runSaveEnrichment(context.Background(), validSaveRequest(), tax, saver)
	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if saver.gotModel != defaultModel {
		t.Fatalf("model = %q, want default %q", saver.gotModel, defaultModel)
	}
	if saver.gotPromptVersion != defaultPromptVersion {
		t.Fatalf("prompt_version = %q, want default %q", saver.gotPromptVersion, defaultPromptVersion)
	}
}

func TestRunSaveEnrichment_ProvenancePassedSeparately_NotInPayload(t *testing.T) {
	tax := &fakeTaxonomySource{tax: testTaxonomy(), exists: true}
	saver := &fakeSaver{result: okEnvelope(t, 100, 42, newTaxonomy{})}
	req := validSaveRequest()
	req.Provenance = provenanceInput{Model: "claude-test", PromptVersion: "v9.9"}

	env := runSaveEnrichment(context.Background(), req, tax, saver)
	if !env.Ok {
		t.Fatalf("env.Ok = false; errors=%+v", env.Errors)
	}
	if saver.gotModel != "claude-test" || saver.gotPromptVersion != "v9.9" {
		t.Fatalf("provenance not passed separately: model=%q pv=%q", saver.gotModel, saver.gotPromptVersion)
	}
	payload := string(saver.gotPayload)
	if strings.Contains(payload, "claude-test") || strings.Contains(payload, "provenance") || strings.Contains(payload, "v9.9") {
		t.Fatalf("provenance leaked into p_payload: %s", payload)
	}
}

func TestRunSaveEnrichment_RequirementStrippedFromPayload_EchoedInRequest(t *testing.T) {
	tax := &fakeTaxonomySource{tax: testTaxonomy(), exists: true}
	saver := &fakeSaver{result: okEnvelope(t, 100, 42, newTaxonomy{})}

	env := runSaveEnrichment(context.Background(), validSaveRequest(), tax, saver)
	if !env.Ok {
		t.Fatalf("env.Ok = false; errors=%+v", env.Errors)
	}
	if strings.Contains(string(saver.gotPayload), "requirement") || strings.Contains(string(saver.gotPayload), "required") {
		t.Fatalf("skills[].requirement leaked into p_payload: %s", saver.gotPayload)
	}
	if strings.Contains(string(saver.gotPayload), "summary") {
		t.Fatalf("summary must not be persisted in p_payload: %s", saver.gotPayload)
	}
}

func TestRunSaveEnrichment_SummaryEchoed(t *testing.T) {
	tax := &fakeTaxonomySource{tax: testTaxonomy(), exists: true}
	saver := &fakeSaver{result: okEnvelope(t, 100, 42, newTaxonomy{})}

	env := runSaveEnrichment(context.Background(), validSaveRequest(), tax, saver)
	if env.Summary != "a summary" {
		t.Fatalf("summary = %q, want echoed 'a summary'", env.Summary)
	}
}

func TestRunSaveEnrichment_NewTaxonomyMapped(t *testing.T) {
	nt := newTaxonomy{
		CanonicalRoles: []newTaxonomyEntry{{Slug: "ml-engineer", Name: "ML Engineer"}},
		Skills:         []newTaxonomyEntry{{Slug: "rlhf", Name: "RLHF"}},
	}
	tax := &fakeTaxonomySource{tax: testTaxonomy(), exists: true}
	saver := &fakeSaver{result: okEnvelope(t, 100, 42, nt)}

	env := runSaveEnrichment(context.Background(), validSaveRequest(), tax, saver)
	if !env.Ok {
		t.Fatalf("env.Ok = false; errors=%+v", env.Errors)
	}
	if env.ClassificationID == nil || *env.ClassificationID != 100 {
		t.Fatalf("classification_id = %v, want 100", env.ClassificationID)
	}
	if len(env.NewTaxonomy.CanonicalRoles) != 1 || env.NewTaxonomy.CanonicalRoles[0].Slug != "ml-engineer" {
		t.Fatalf("new canonical roles unexpected: %+v", env.NewTaxonomy.CanonicalRoles)
	}
	if len(env.NewTaxonomy.Skills) != 1 || env.NewTaxonomy.Skills[0].Slug != "rlhf" {
		t.Fatalf("new skills unexpected: %+v", env.NewTaxonomy.Skills)
	}
	if env.NewTaxonomy.Specializations == nil {
		t.Fatalf("specializations should be an empty slice, not nil")
	}
}

func TestRunSaveEnrichment_InvalidProvenance(t *testing.T) {
	tax := &fakeTaxonomySource{tax: testTaxonomy(), exists: true}
	req := validSaveRequest()
	req.Provenance = provenanceInput{Model: "bad model!", PromptVersion: "ok-v1"}

	env := runSaveEnrichment(context.Background(), req, tax, neverSaver{t})
	if env.Ok {
		t.Fatalf("env.Ok = true, want false for invalid provenance")
	}
	if !hasError(env.Errors, "provenance.model", codeInvalidProvenance) {
		t.Fatalf("errors = %+v, want invalid_provenance on provenance.model", env.Errors)
	}
}

func TestRunSaveEnrichment_PostingNotFound(t *testing.T) {
	tax := &fakeTaxonomySource{tax: testTaxonomy(), exists: false}

	env := runSaveEnrichment(context.Background(), validSaveRequest(), tax, neverSaver{t})
	if env.Ok {
		t.Fatalf("env.Ok = true, want false for nonexistent posting")
	}
	if !hasError(env.Errors, "posting_id", codePostingNotFound) {
		t.Fatalf("errors = %+v, want posting_not_found on posting_id", env.Errors)
	}
}

func TestRunSaveEnrichment_ValidationCodesFromSharedRules(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*saveEnrichmentRequest)
		wantPath string
		wantCode string
	}{
		{"missing seniority", func(r *saveEnrichmentRequest) { r.Classification.Seniority = "" }, "classification.seniority", string(classify.CodeMissingSeniority)},
		{"invalid seniority", func(r *saveEnrichmentRequest) { r.Classification.Seniority = "ultra" }, "classification.seniority", string(classify.CodeInvalidSeniority)},
		{"invalid slug", func(r *saveEnrichmentRequest) { r.CanonicalRoles[0].Slug = "Bad_Slug" }, "canonical_roles[0].slug", string(classify.CodeInvalidSlug)},
		{"unknown dimension", func(r *saveEnrichmentRequest) { r.CanonicalRoles[0].Dimensions = []string{"moonshot"} }, "canonical_roles[0].dimensions[0]", string(classify.CodeUnknownDimension)},
		{"empty dimensions", func(r *saveEnrichmentRequest) { r.CanonicalRoles[0].Dimensions = nil }, "canonical_roles[0].dimensions", string(classify.CodeEmptyDimensions)},
		{"cross-table collision", func(r *saveEnrichmentRequest) {
			r.CanonicalRoles = append(r.CanonicalRoles, classify.AgentCanonicalRole{Slug: "frontend", Name: "Frontend", Dimensions: []string{"ic"}})
		}, "canonical_roles[1].slug", string(classify.CodeSlugCollision)},
		{"null byte in notes", func(r *saveEnrichmentRequest) { r.Classification.Notes = "x\x00y" }, "classification.notes", string(classify.CodeNullByte)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tax := &fakeTaxonomySource{tax: testTaxonomy(), exists: true}
			req := validSaveRequest()
			tc.mutate(&req)
			env := runSaveEnrichment(context.Background(), req, tax, neverSaver{t})
			if env.Ok {
				t.Fatalf("env.Ok = true, want false")
			}
			if !hasError(env.Errors, tc.wantPath, tc.wantCode) {
				t.Fatalf("errors = %+v, want path=%q code=%q", env.Errors, tc.wantPath, tc.wantCode)
			}
		})
	}
}

func TestRunSaveEnrichment_SQLViolationsMappedFromFunction(t *testing.T) {
	// A payload that passes Go validation but the function rejects at SQL level
	// (e.g. a race deleted a dimension). The function returns ok=false with
	// structured errors; the tool maps them straight into the envelope.
	fr := functionResult{Ok: false}
	fr.Errors = append(fr.Errors, struct {
		Path    string `json:"path"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Path: "canonical_roles[0].dimensions", Code: "unknown_dimension", Message: "x is not a known role dimension"})
	raw, _ := json.Marshal(fr)

	tax := &fakeTaxonomySource{tax: testTaxonomy(), exists: true}
	saver := &fakeSaver{result: raw}

	env := runSaveEnrichment(context.Background(), validSaveRequest(), tax, saver)
	if !saver.called {
		t.Fatalf("saver was not called")
	}
	if env.Ok {
		t.Fatalf("env.Ok = true, want false on SQL-level violation")
	}
	if !hasError(env.Errors, "canonical_roles[0].dimensions", "unknown_dimension") {
		t.Fatalf("errors = %+v, want mapped SQL violation", env.Errors)
	}
}

func TestRunSaveEnrichment_UnexpectedDBErrorIsDBError(t *testing.T) {
	tax := &fakeTaxonomySource{tax: testTaxonomy(), exists: true}
	saver := &fakeSaver{err: errors.New("connection reset")}

	env := runSaveEnrichment(context.Background(), validSaveRequest(), tax, saver)
	if env.Ok {
		t.Fatalf("env.Ok = true, want false on db error")
	}
	if !hasError(env.Errors, "db", codeDBError) {
		t.Fatalf("errors = %+v, want db_error", env.Errors)
	}
}
