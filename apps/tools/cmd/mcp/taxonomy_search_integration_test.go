//go:build integration

package main

import (
	"context"
	"database/sql"
	"testing"
)

// taxonomySearchClusterPair is a real pair in the live skills table that
// clusters correctly on slug similarity alone: cloud-platform-architecture and
// platform-architecture describe the same concept under different names, and
// score 0.786 on slug similarity — above taxonomySearchClusterThreshold. It
// proves clustering still functions once name is out of the neighbours query.
const (
	taxonomySearchClusterTerm  = "cloud platform architecture"
	taxonomySearchClusterSlugA = "cloud-platform-architecture"
	taxonomySearchClusterSlugB = "platform-architecture"
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

// TestTaxonomySearch_LiveSlugSimilarPairLandsInOneCluster proves clustering
// still functions against real data once name is out of the neighbours query:
// two rows that are genuinely the same concept, expressed under different
// names, still nest together because their slugs are similar.
func TestTaxonomySearch_LiveSlugSimilarPairLandsInOneCluster(t *testing.T) {
	pool := openReadOnlyTestDB(t)
	requireLiveSkillSlugs(t, pool, taxonomySearchClusterSlugA, taxonomySearchClusterSlugB)

	env := runTaxonomySearch(t.Context(), taxonomySearchRequest{
		Terms:  []string{taxonomySearchClusterTerm},
		Tables: []string{"skills"},
	}, poolTaxonomySearchSource{pool: pool})

	if !env.Ok {
		t.Fatalf("env.Ok = false, errors = %+v", env.Errors)
	}
	skills := env.Results[0].Tables[0]
	if skills.ClusterCount == 0 {
		t.Fatalf("no clusters for %q", taxonomySearchClusterTerm)
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

	idxA, okA := clusterOf[taxonomySearchClusterSlugA]
	idxB, okB := clusterOf[taxonomySearchClusterSlugB]
	if !okA || !okB {
		t.Fatalf("clusters = %+v; want both %q and %q present", skills.Clusters, taxonomySearchClusterSlugA, taxonomySearchClusterSlugB)
	}
	if idxA != idxB {
		t.Fatalf("%q is in cluster %d and %q in cluster %d; the slug-similar pair must share one cluster",
			taxonomySearchClusterSlugA, idxA, taxonomySearchClusterSlugB, idxB)
	}
	if roleOf[taxonomySearchClusterSlugA] == roleOf[taxonomySearchClusterSlugB] {
		t.Fatalf("both slugs came back as %q; one must represent the other",
			roleOf[taxonomySearchClusterSlugA])
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
