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

// TestRunSaveEnrichment_GateFeedbackSurfaced proves the near-duplicate gate's
// (migration 000033, slug axis narrowed by 000034) three advisory keys
// (substitutions, dropped, similarity_candidates) thread all the way from the
// function result into the tool envelope, path and all fields intact — the
// whole point of the gate being advisory rather than a silent hard rejection.
func TestRunSaveEnrichment_GateFeedbackSurfaced(t *testing.T) {
	fr := functionResult{
		Ok:               true,
		ClassificationID: int64Ptr(100),
		PostingID:        int64Ptr(42),
		NewTaxonomy:      newTaxonomy{},
		Substitutions: []substitution{
			{
				Path:            "specializations[0].slug",
				Table:           "specializations",
				ProposedSlug:    "security-infrastructure",
				ProposedName:    "Security Infrastructure",
				SubstitutedSlug: "infrastructure-security",
				SubstitutedName: "Infrastructure Security",
				Match:           "slug_similarity",
				Similarity:      0.958,
			},
		},
		Dropped: []droppedEntry{
			{
				Path:        "skills[2].slug",
				Table:       "skills",
				DroppedSlug: "cloud-platform-architecture",
				DroppedName: "Cloud Platform Architecture",
				KeptSlug:    "platform-architecture",
				Similarity:  0.9,
				Reason:      "near_duplicate_in_payload",
			},
		},
		SimilarityCandidates: []similarityCandidateGroup{
			{
				Path:       "specializations[1].slug",
				Table:      "specializations",
				MintedSlug: "data-orchestration",
				MintedName: "Data Pipeline Architecture",
				Candidates: []similarityMatch{
					{
						Slug:           "data-pipeline-architecture",
						Name:           "Data Pipeline Architecture",
						Match:          "exact_name",
						SlugSimilarity: 0.150,
						Similarity:     1,
					},
				},
			},
		},
	}
	raw, err := json.Marshal(fr)
	if err != nil {
		t.Fatalf("marshaling function result: %v", err)
	}

	tax := &fakeTaxonomySource{tax: testTaxonomy(), exists: true}
	saver := &fakeSaver{result: raw}

	env := runSaveEnrichment(context.Background(), validSaveRequest(), tax, saver)
	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}

	if len(env.Substitutions) != 1 || env.Substitutions[0] != fr.Substitutions[0] {
		t.Fatalf("substitutions = %+v, want %+v", env.Substitutions, fr.Substitutions)
	}
	if len(env.Dropped) != 1 || env.Dropped[0] != fr.Dropped[0] {
		t.Fatalf("dropped = %+v, want %+v", env.Dropped, fr.Dropped)
	}
	if len(env.SimilarityCandidates) != 1 {
		t.Fatalf("similarity_candidates = %+v, want 1 entry", env.SimilarityCandidates)
	}
	got := env.SimilarityCandidates[0]
	want := fr.SimilarityCandidates[0]
	if got.Path != want.Path || got.Table != want.Table || got.MintedSlug != want.MintedSlug ||
		got.MintedName != want.MintedName || len(got.Candidates) != len(want.Candidates) ||
		got.Candidates[0] != want.Candidates[0] {
		t.Fatalf("similarity_candidates[0] = %+v, want %+v", got, want)
	}

	// The envelope actually serializes the three keys, not just holds them in
	// memory — that is what makes them visible to the calling agent.
	payload := string(mustMarshalEnvelope(t, env))
	for _, key := range []string{`"substitutions"`, `"dropped"`, `"similarity_candidates"`} {
		if !strings.Contains(payload, key) {
			t.Fatalf("envelope JSON missing %s: %s", key, payload)
		}
	}
}

// TestRunSaveEnrichment_NoGateFeedback_EnvelopeUnchanged is the regression
// guard: a function result that carries none of the three 000033 gate keys
// (the shape every save returned before that migration, and what an untouched
// payload still returns today) must produce an envelope with those three keys
// entirely absent from the JSON — not present as `null` or `[]`. omitempty on
// a nil slice is what gets this; asserting it here is what keeps it that way.
func TestRunSaveEnrichment_NoGateFeedback_EnvelopeUnchanged(t *testing.T) {
	tax := &fakeTaxonomySource{tax: testTaxonomy(), exists: true}
	saver := &fakeSaver{result: okEnvelope(t, 100, 42, newTaxonomy{})}

	env := runSaveEnrichment(context.Background(), validSaveRequest(), tax, saver)
	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if env.Substitutions != nil || env.Dropped != nil || env.SimilarityCandidates != nil {
		t.Fatalf("gate fields should be nil when untouched: substitutions=%+v dropped=%+v similarity_candidates=%+v",
			env.Substitutions, env.Dropped, env.SimilarityCandidates)
	}

	payload := string(mustMarshalEnvelope(t, env))
	for _, key := range []string{`"substitutions"`, `"dropped"`, `"similarity_candidates"`} {
		if strings.Contains(payload, key) {
			t.Fatalf("untouched envelope must omit %s entirely, got: %s", key, payload)
		}
	}

	// Same assertion the pre-000033 envelope shape would have satisfied: the
	// full JSON round-trips to exactly the fields the envelope carried before
	// this gate's feedback existed.
	want := saveEnrichmentEnvelope{
		Ok:               true,
		ClassificationID: env.ClassificationID,
		PostingID:        42,
		Summary:          "a summary",
		NewTaxonomy:      newTaxonomy{CanonicalRoles: []newTaxonomyEntry{}, Specializations: []newTaxonomyEntry{}, Skills: []newTaxonomyEntry{}},
		Errors:           []actionError{},
	}
	wantPayload := string(mustMarshalEnvelope(t, want))
	if payload != wantPayload {
		t.Fatalf("untouched envelope JSON changed:\n got:  %s\n want: %s", payload, wantPayload)
	}
}

func int64Ptr(v int64) *int64 { return &v }

func mustMarshalEnvelope(t *testing.T, env saveEnrichmentEnvelope) []byte {
	t.Helper()
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshaling envelope: %v", err)
	}
	return b
}
