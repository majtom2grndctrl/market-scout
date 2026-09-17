package main

import (
	"context"
	"errors"
	"testing"
)

// fakeTaxonomySearchSource returns canned candidates (or an error) and records the
// resolved terms and tables it was called with, so tests can assert validation,
// clustering, and representative choice without a database.
type fakeTaxonomySearchSource struct {
	candidates []taxonomyCandidate
	err        error
	called     bool
	gotTerms   []string
	gotTables  []string
}

func (f *fakeTaxonomySearchSource) Candidates(ctx context.Context, terms, tables []string) ([]taxonomyCandidate, error) {
	f.called = true
	f.gotTerms = terms
	f.gotTables = tables
	if f.err != nil {
		return nil, f.err
	}
	return f.candidates, nil
}

// skillCandidate builds a candidate in the skills table for term 1. Neighbours
// are the ids this entry is mutually similar to, which is what the scoring query
// supplies in production.
func skillCandidate(id int64, slug, name string, usage int64, score float64, neighbors ...int64) taxonomyCandidate {
	refs := make([]taxonomyRef, 0, len(neighbors))
	for _, n := range neighbors {
		refs = append(refs, taxonomyRef{Table: "skills", ID: n})
	}
	return taxonomyCandidate{
		TermIndex:  1,
		Ref:        taxonomyRef{Table: "skills", ID: id},
		Slug:       slug,
		Name:       name,
		UsageCount: usage,
		Score:      score,
		Neighbors:  refs,
	}
}

func skillsResult(t *testing.T, env taxonomySearchEnvelope, termIndex int) taxonomyTableResult {
	t.Helper()
	if !env.Ok {
		t.Fatalf("env.Ok = false, errors = %+v", env.Errors)
	}
	if termIndex >= len(env.Results) {
		t.Fatalf("results has %d terms, want at least %d", len(env.Results), termIndex+1)
	}
	for _, tr := range env.Results[termIndex].Tables {
		if tr.Table == "skills" {
			return tr
		}
	}
	t.Fatalf("no skills result for term %d", termIndex)
	return taxonomyTableResult{}
}

func TestRunTaxonomySearch_EmptyTermListRejected(t *testing.T) {
	source := &fakeTaxonomySearchSource{}

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{Terms: []string{}}, source)

	if env.Ok {
		t.Fatalf("env.Ok = true, want false for an empty term list")
	}
	if source.called {
		t.Fatalf("source was queried for an empty term list")
	}
	if !hasError(env.Errors, "terms", codeInvalidTerms) {
		t.Fatalf("errors = %+v, want path=terms code=%s", env.Errors, codeInvalidTerms)
	}
}

func TestRunTaxonomySearch_TermValidation(t *testing.T) {
	longTerm := ""
	for len(longTerm) <= taxonomySearchMaxTermLength {
		longTerm += "architecture "
	}
	tooMany := make([]string, taxonomySearchMaxTerms+1)
	for i := range tooMany {
		tooMany[i] = "kubernetes"
	}

	tests := []struct {
		name      string
		terms     []string
		wantOk    bool
		wantPath  string
		wantCalls bool
	}{
		{"nil list rejected", nil, false, "terms", false},
		{"blank term rejected", []string{"  "}, false, "terms[0]", false},
		{"over-long term rejected", []string{longTerm}, false, "terms[0]", false},
		{"over max terms rejected", tooMany, false, "terms", false},
		{"single term accepted", []string{"kubernetes"}, true, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := &fakeTaxonomySearchSource{}
			env := runTaxonomySearch(t.Context(), taxonomySearchRequest{Terms: tc.terms}, source)

			if env.Ok != tc.wantOk {
				t.Fatalf("env.Ok = %v, want %v; errors = %+v", env.Ok, tc.wantOk, env.Errors)
			}
			if source.called != tc.wantCalls {
				t.Fatalf("source called = %v, want %v", source.called, tc.wantCalls)
			}
			if !tc.wantOk && !hasError(env.Errors, tc.wantPath, codeInvalidTerms) {
				t.Fatalf("errors = %+v, want path=%s code=%s", env.Errors, tc.wantPath, codeInvalidTerms)
			}
		})
	}
}

func TestRunTaxonomySearch_TermsAreTrimmedBeforeLookup(t *testing.T) {
	source := &fakeTaxonomySearchSource{}

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{Terms: []string{"  data pipeline architecture\n"}}, source)

	if !env.Ok {
		t.Fatalf("env.Ok = false, errors = %+v", env.Errors)
	}
	if got := source.gotTerms; len(got) != 1 || got[0] != "data pipeline architecture" {
		t.Fatalf("source terms = %#v, want the trimmed term", got)
	}
	if env.Results[0].Term != "data pipeline architecture" {
		t.Fatalf("echoed term = %q, want the trimmed term", env.Results[0].Term)
	}
}

func TestRunTaxonomySearch_UnknownTableRejected(t *testing.T) {
	source := &fakeTaxonomySearchSource{}

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{
		Terms:  []string{"kubernetes"},
		Tables: []string{"skills", "job_postings"},
	}, source)

	if env.Ok {
		t.Fatalf("env.Ok = true, want false for an unknown table")
	}
	if source.called {
		t.Fatalf("source was queried despite an unknown table")
	}
	if !hasError(env.Errors, "tables[1]", codeUnknownTable) {
		t.Fatalf("errors = %+v, want path=tables[1] code=%s", env.Errors, codeUnknownTable)
	}
}

func TestRunTaxonomySearch_TablesDefaultToAllThreeInCanonicalOrder(t *testing.T) {
	source := &fakeTaxonomySearchSource{}

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{Terms: []string{"kubernetes"}}, source)

	if !env.Ok {
		t.Fatalf("env.Ok = false, errors = %+v", env.Errors)
	}
	want := []string{"canonical_roles", "specializations", "skills"}
	if len(source.gotTables) != len(want) {
		t.Fatalf("source tables = %#v, want %#v", source.gotTables, want)
	}
	for i, table := range want {
		if source.gotTables[i] != table {
			t.Fatalf("source tables = %#v, want %#v", source.gotTables, want)
		}
	}
	if len(env.Results[0].Tables) != len(want) {
		t.Fatalf("result tables = %d, want %d", len(env.Results[0].Tables), len(want))
	}
}

func TestRunTaxonomySearch_RequestedTablesAreDedupedAndReordered(t *testing.T) {
	source := &fakeTaxonomySearchSource{}

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{
		Terms:  []string{"kubernetes"},
		Tables: []string{"skills", "Skills", "canonical_roles"},
	}, source)

	if !env.Ok {
		t.Fatalf("env.Ok = false, errors = %+v", env.Errors)
	}
	want := []string{"canonical_roles", "skills"}
	if len(source.gotTables) != len(want) || source.gotTables[0] != want[0] || source.gotTables[1] != want[1] {
		t.Fatalf("source tables = %#v, want %#v", source.gotTables, want)
	}
}

func TestRunTaxonomySearch_DBErrorReturnsEnvelopeNotTransportError(t *testing.T) {
	source := &fakeTaxonomySearchSource{err: errors.New("connection refused")}

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{Terms: []string{"kubernetes"}}, source)

	if env.Ok {
		t.Fatalf("env.Ok = true, want false when the lookup fails")
	}
	if !hasError(env.Errors, "db", codeDBError) {
		t.Fatalf("errors = %+v, want path=db code=%s", env.Errors, codeDBError)
	}
}

// The failure this tool exists to prevent: three spellings of one concept
// offered as peers, which agents then attached to the same posting. All three
// are mutually similar, so they must come back as one cluster with two variants.
func TestClusterTaxonomyCandidates_MutuallySimilarEntriesShareOneRepresentative(t *testing.T) {
	source := &fakeTaxonomySearchSource{candidates: []taxonomyCandidate{
		skillCandidate(1, "cloud-platform-architecture", "Cloud Platform Architecture", 3, 1.0, 2, 3),
		skillCandidate(2, "platform-architecture", "Platform Architecture", 5, 0.79, 1, 3),
		skillCandidate(3, "cloud-platform-arch", "Cloud Platform Arch", 1, 0.72, 1, 2),
	}}

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{Terms: []string{"cloud platform architecture"}}, source)
	skills := skillsResult(t, env, 0)

	if skills.ClusterCount != 1 {
		t.Fatalf("cluster count = %d, want 1; clusters = %+v", skills.ClusterCount, skills.Clusters)
	}
	cluster := skills.Clusters[0]
	if cluster.Representative.Slug != "cloud-platform-architecture" {
		t.Fatalf("representative = %q, want cloud-platform-architecture", cluster.Representative.Slug)
	}
	if cluster.VariantCount != 2 || len(cluster.Variants) != 2 {
		t.Fatalf("variants = %+v (count %d), want the other two spellings", cluster.Variants, cluster.VariantCount)
	}
	if env.EntryCount != 3 {
		t.Fatalf("entry count = %d, want 3", env.EntryCount)
	}
}

// Leader clustering must not chain through a hub. `architecture` is similar to
// both `data-architecture` and `api-architecture`, but those two are not similar
// to each other, so single-link agglomeration would collapse all three into one
// cluster and hide two distinct concepts.
func TestClusterTaxonomyCandidates_DoesNotChainThroughAHub(t *testing.T) {
	source := &fakeTaxonomySearchSource{candidates: []taxonomyCandidate{
		skillCandidate(10, "architecture", "Software Architecture", 9, 0.80, 11, 12),
		skillCandidate(11, "data-architecture", "Data Architecture", 5, 0.75, 10),
		skillCandidate(12, "api-architecture", "API Architecture", 9, 0.70, 10),
	}}

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{Terms: []string{"architecture"}}, source)
	skills := skillsResult(t, env, 0)

	// The hub claims both neighbours because both are similar to the hub itself;
	// what must not happen is a fourth entry joining by transitivity alone.
	if skills.ClusterCount != 1 {
		t.Fatalf("cluster count = %d, want 1", skills.ClusterCount)
	}

	// Drop the hub and the two remaining entries are not mutually similar, so
	// they must stay separate rather than merging through the absent hub.
	source = &fakeTaxonomySearchSource{candidates: []taxonomyCandidate{
		skillCandidate(11, "data-architecture", "Data Architecture", 5, 0.75),
		skillCandidate(12, "api-architecture", "API Architecture", 9, 0.70),
	}}
	env = runTaxonomySearch(t.Context(), taxonomySearchRequest{Terms: []string{"architecture"}}, source)
	skills = skillsResult(t, env, 0)

	if skills.ClusterCount != 2 {
		t.Fatalf("cluster count = %d, want 2 separate concepts; clusters = %+v", skills.ClusterCount, skills.Clusters)
	}
}

func TestBuildTaxonomyCluster_RepresentativePrefersHigherUsageCount(t *testing.T) {
	// Two spellings of one concept, equally good matches. The established one —
	// 40 classification links against 1 — must front the cluster.
	source := &fakeTaxonomySearchSource{candidates: []taxonomyCandidate{
		skillCandidate(20, "data-pipeline-architecture", "Data Pipeline Architecture", 1, 1.0, 21),
		skillCandidate(21, "data-orchestration", "Data Pipeline Architecture", 40, 1.0, 20),
	}}

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{Terms: []string{"data pipeline architecture"}}, source)
	skills := skillsResult(t, env, 0)

	if skills.ClusterCount != 1 {
		t.Fatalf("cluster count = %d, want 1", skills.ClusterCount)
	}
	cluster := skills.Clusters[0]
	if cluster.Representative.Slug != "data-orchestration" {
		t.Fatalf("representative = %q, want the most-used slug data-orchestration", cluster.Representative.Slug)
	}
	if cluster.Representative.UsageCount != 40 {
		t.Fatalf("representative usage = %d, want 40", cluster.Representative.UsageCount)
	}
	if len(cluster.Variants) != 1 || cluster.Variants[0].Slug != "data-pipeline-architecture" {
		t.Fatalf("variants = %+v, want the less-used spelling nested underneath", cluster.Variants)
	}
}

func TestBuildTaxonomyCluster_UsageDoesNotPromoteAWeakerMatch(t *testing.T) {
	// The heavily used member is a materially worse match for the term, so it
	// stays a variant. Usage breaks ties among comparable matches, not across them.
	source := &fakeTaxonomySearchSource{candidates: []taxonomyCandidate{
		skillCandidate(30, "kubernetes", "Kubernetes", 2, 0.95, 31),
		skillCandidate(31, "container-orchestration", "Container Orchestration", 500, 0.72, 30),
	}}

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{Terms: []string{"kubernetes"}}, source)
	cluster := skillsResult(t, env, 0).Clusters[0]

	if cluster.Representative.Slug != "kubernetes" {
		t.Fatalf("representative = %q, want the better match kubernetes", cluster.Representative.Slug)
	}
}

func TestRunTaxonomySearch_ClustersAreCappedAndCountedHonestly(t *testing.T) {
	candidates := make([]taxonomyCandidate, 0, taxonomySearchMaxClusters+3)
	for i := 0; i < taxonomySearchMaxClusters+3; i++ {
		// No neighbours: every entry is its own concept.
		candidates = append(candidates, skillCandidate(int64(100+i), "slug", "Name", 0, 0.9-float64(i)/100))
	}
	source := &fakeTaxonomySearchSource{candidates: candidates}

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{Terms: []string{"anything"}}, source)
	skills := skillsResult(t, env, 0)

	if skills.ClusterCount != taxonomySearchMaxClusters {
		t.Fatalf("cluster count = %d, want the cap of %d", skills.ClusterCount, taxonomySearchMaxClusters)
	}
	if skills.Clusters[0].Score != 0.9 {
		t.Fatalf("first cluster score = %v, want the best match 0.9", skills.Clusters[0].Score)
	}
}

func TestRunTaxonomySearch_CandidatesAreRoutedToTheirOwnTerm(t *testing.T) {
	source := &fakeTaxonomySearchSource{candidates: []taxonomyCandidate{
		{TermIndex: 2, Ref: taxonomyRef{Table: "skills", ID: 1}, Slug: "figma", Name: "Figma", Score: 1.0},
		{TermIndex: 1, Ref: taxonomyRef{Table: "skills", ID: 2}, Slug: "golang", Name: "Go", Score: 0.8},
		// Out-of-range indexes are dropped rather than panicking the handler.
		{TermIndex: 9, Ref: taxonomyRef{Table: "skills", ID: 3}, Slug: "noise", Name: "Noise", Score: 1.0},
	}}

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{Terms: []string{"go", "figma"}}, source)

	if got := skillsResult(t, env, 0).Clusters[0].Representative.Slug; got != "golang" {
		t.Fatalf("term 0 representative = %q, want golang", got)
	}
	if got := skillsResult(t, env, 1).Clusters[0].Representative.Slug; got != "figma" {
		t.Fatalf("term 1 representative = %q, want figma", got)
	}
	if env.EntryCount != 2 {
		t.Fatalf("entry count = %d, want 2 (the out-of-range candidate dropped)", env.EntryCount)
	}
}

func TestRunTaxonomySearch_ResponseStaysWellUnderRowCap(t *testing.T) {
	// Worst case the contract allows: every term saturating every table.
	terms := make([]string, taxonomySearchMaxTerms)
	var candidates []taxonomyCandidate
	id := int64(0)
	for i := range terms {
		terms[i] = "term"
		for _, table := range taxonomySearchTables {
			for j := 0; j < taxonomySearchCandidatesPerTable; j++ {
				id++
				candidates = append(candidates, taxonomyCandidate{
					TermIndex: i + 1,
					Ref:       taxonomyRef{Table: table, ID: id},
					Slug:      "slug", Name: "Name", Score: 0.9,
				})
			}
		}
	}
	source := &fakeTaxonomySearchSource{candidates: candidates}

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{Terms: terms}, source)

	if !env.Ok {
		t.Fatalf("env.Ok = false, errors = %+v", env.Errors)
	}
	if env.EntryCount >= rowCap {
		t.Fatalf("entry count = %d, want well under rowCap %d", env.EntryCount, rowCap)
	}
}
