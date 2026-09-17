//go:build integration

package main

import (
	"context"
	"database/sql"
	"testing"
)

// taxonomySearchDuplicatePair is a real duplicate that exists in the live skills
// table: two rows carrying the identical name "Data Pipeline Architecture" under
// different slugs. It is exactly the failure this tool exists to catch — an agent
// stemming `%orchestr%` or `%pipeline%` finds one and mints the other. Their
// slugs share only 0.15 trigram similarity, so nothing that ranks on slug alone
// can group them; scoring against the name is what makes them one concept.
const (
	taxonomySearchDuplicateTerm  = "data pipeline architecture"
	taxonomySearchDuplicateSlugA = "data-pipeline-architecture"
	taxonomySearchDuplicateSlugB = "data-orchestration"
)

// requireLiveSkillSlugs skips when the live taxonomy no longer holds the rows the
// assertion is written against. The point is to prove clustering on real data; if
// the data has moved on, a skip is honest and a hand-built fixture would not be.
func requireLiveSkillSlugs(t *testing.T, pool *sql.DB, slugs ...string) {
	t.Helper()
	for _, slug := range slugs {
		var name string
		err := pool.QueryRowContext(t.Context(), `SELECT name FROM skills WHERE slug = $1`, slug).Scan(&name)
		if err == sql.ErrNoRows {
			t.Skipf("live skills table no longer has slug %q; re-pick a duplicate pair", slug)
		}
		if err != nil {
			t.Fatalf("looking up slug %q: %v", slug, err)
		}
	}
}

func TestTaxonomySearch_LiveDuplicatePairLandsInOneCluster(t *testing.T) {
	pool := openReadOnlyTestDB(t)
	requireLiveSkillSlugs(t, pool, taxonomySearchDuplicateSlugA, taxonomySearchDuplicateSlugB)

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{
		Terms:  []string{taxonomySearchDuplicateTerm},
		Tables: []string{"skills"},
	}, poolTaxonomySearchSource{pool: pool})

	if !env.Ok {
		t.Fatalf("env.Ok = false, errors = %+v", env.Errors)
	}
	skills := env.Results[0].Tables[0]
	if skills.ClusterCount == 0 {
		t.Fatalf("no clusters for %q", taxonomySearchDuplicateTerm)
	}

	// Both slugs must appear, and in the same cluster: one as the representative,
	// the other nested beneath it. Two sibling clusters would be the flat-menu
	// failure this tool replaces.
	clusterOf := map[string]int{}
	roleOf := map[string]string{}
	for i, cluster := range skills.Clusters {
		clusterOf[cluster.Representative.Slug] = i
		roleOf[cluster.Representative.Slug] = "representative"
		for _, variant := range cluster.Variants {
			clusterOf[variant.Slug] = i
			roleOf[variant.Slug] = "variant"
		}
	}

	idxA, okA := clusterOf[taxonomySearchDuplicateSlugA]
	idxB, okB := clusterOf[taxonomySearchDuplicateSlugB]
	if !okA || !okB {
		t.Fatalf("clusters = %+v; want both %q and %q present", skills.Clusters, taxonomySearchDuplicateSlugA, taxonomySearchDuplicateSlugB)
	}
	if idxA != idxB {
		t.Fatalf("%q is in cluster %d and %q in cluster %d; the duplicate pair must share one cluster",
			taxonomySearchDuplicateSlugA, idxA, taxonomySearchDuplicateSlugB, idxB)
	}
	if roleOf[taxonomySearchDuplicateSlugA] == roleOf[taxonomySearchDuplicateSlugB] {
		t.Fatalf("both slugs came back as %q; one must represent the other",
			roleOf[taxonomySearchDuplicateSlugA])
	}

	// The whole point is that this returns rather than truncating: the generic
	// query tool caps at rowCap and the skills table is past it.
	if env.EntryCount >= rowCap {
		t.Fatalf("entry count = %d, want well under rowCap %d", env.EntryCount, rowCap)
	}
}

// The read-only role must be able to run similarity() over the taxonomy tables
// and count the classification links. A permission gap here would only surface at
// runtime, since the unit tests fake the source.
func TestTaxonomySearch_ReadOnlyRoleCanScoreAndCountAcrossAllTables(t *testing.T) {
	pool := openReadOnlyTestDB(t)

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{
		Terms: []string{"engineering manager", "figma", "kubernetes"},
	}, poolTaxonomySearchSource{pool: pool})

	if !env.Ok {
		t.Fatalf("env.Ok = false, errors = %+v", env.Errors)
	}
	if len(env.Results) != 3 {
		t.Fatalf("results = %d, want 3", len(env.Results))
	}
	for _, result := range env.Results {
		if len(result.Tables) != len(taxonomySearchTables) {
			t.Fatalf("term %q searched %d tables, want %d", result.Term, len(result.Tables), len(taxonomySearchTables))
		}
	}
	if env.EntryCount == 0 {
		t.Fatalf("entry count = 0; expected real matches for these terms")
	}
	if env.EntryCount >= rowCap {
		t.Fatalf("entry count = %d, want well under rowCap %d", env.EntryCount, rowCap)
	}
}

// A full-width batch must finish inside statementTimeout: every term costs a pass
// over all three tables. This is the bound that justifies taxonomySearchMaxTerms.
func TestTaxonomySearch_MaxTermBatchCompletesWithinStatementTimeout(t *testing.T) {
	pool := openReadOnlyTestDB(t)

	terms := []string{
		"data pipeline architecture", "cloud platform architecture", "kubernetes",
		"design systems", "product management", "machine learning engineering",
		"incident response", "accessibility", "technical writing", "growth marketing",
	}
	if len(terms) != taxonomySearchMaxTerms {
		t.Fatalf("test batch is %d terms, want taxonomySearchMaxTerms (%d)", len(terms), taxonomySearchMaxTerms)
	}

	ctx, cancel := context.WithTimeout(t.Context(), statementTimeout)
	defer cancel()

	env := runTaxonomySearch(ctx, taxonomySearchRequest{Terms: terms}, poolTaxonomySearchSource{pool: pool})
	if !env.Ok {
		t.Fatalf("env.Ok = false, errors = %+v", env.Errors)
	}
	if env.EntryCount >= rowCap {
		t.Fatalf("entry count = %d, want well under rowCap %d", env.EntryCount, rowCap)
	}
}
