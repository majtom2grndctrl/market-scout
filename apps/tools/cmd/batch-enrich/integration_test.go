//go:build integration

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// integrationToken returns a per-test unique token used to namespace seed
// rows so concurrent tests and real data never collide. The token threads
// through company.board_token, job_postings.source_url, and skill/role
// slugs that the test inserts and later cleans up.
func integrationToken(t *testing.T) string {
	t.Helper()
	// Replace characters t.Name() may contain that are not slug-safe.
	return fmt.Sprintf("be-integ-%d", time.Now().UTC().UnixNano())
}

// openTestDB opens the pool via the production OpenDB helper, or skips the
// test when DATABASE_URL is not set.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := OpenDB(ctx)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}

// seedCompany inserts one row in companies and returns its id. board_token
// embeds the per-test token so cleanup deletes only this test's rows.
func seedCompany(t *testing.T, ctx context.Context, pool *sql.DB, token string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRowContext(ctx, `
		INSERT INTO companies (name, ats, board_token)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "Batch Enrich Test Co", "greenhouse", token).Scan(&id)
	if err != nil {
		t.Fatalf("insert company: %v", err)
	}
	return id
}

// seedPosting inserts one job_posting and one posting_snapshot. The
// description argument is written to posting_snapshots.description_text as
// a nullable column — pass an empty *string to seed a NULL description.
// firstSeenAt fixes job_postings.first_seen_at so selection-order tests can
// assert deterministic ordering.
func seedPosting(t *testing.T, ctx context.Context, pool *sql.DB, companyID int64, token string, suffix string, title string, description *string, firstSeenAt time.Time) int64 {
	t.Helper()
	sourceURL := fmt.Sprintf("https://example.com/jobs/%s/%s", token, suffix)
	var postingID int64
	err := pool.QueryRowContext(ctx, `
		INSERT INTO job_postings (company_id, source_type, source_url, first_seen_at)
		VALUES ($1, 'ats', $2, $3)
		RETURNING id
	`, companyID, sourceURL, firstSeenAt).Scan(&postingID)
	if err != nil {
		t.Fatalf("insert job_posting: %v", err)
	}

	var descArg any
	if description != nil {
		descArg = *description
	} else {
		descArg = nil
	}

	if _, err := pool.ExecContext(ctx, `
		INSERT INTO posting_snapshots (job_posting_id, fetched_at, title, description_text, raw_data)
		VALUES ($1, $2, $3, $4, '{}'::jsonb)
	`, postingID, firstSeenAt, title, descArg); err != nil {
		t.Fatalf("insert posting_snapshot: %v", err)
	}
	return postingID
}

// seedClassification inserts one classifications row pointing at postingID
// so force/non-force selection tests can verify the NOT EXISTS filter.
func seedClassification(t *testing.T, ctx context.Context, pool *sql.DB, postingID int64) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRowContext(ctx, `
		INSERT INTO classifications (job_posting_id, model, prompt_version, seniority)
		VALUES ($1, $2, $3, 'mid')
		RETURNING id
	`, postingID, "test-model", "test-prompt").Scan(&id)
	if err != nil {
		t.Fatalf("insert classification: %v", err)
	}
	return id
}

// cleanupByToken removes every row this test seeded. Run via t.Cleanup so
// it fires even on Fatal. Deletes follow FK order: join tables, then
// classifications, then snapshots, then postings, then company.
func cleanupByToken(t *testing.T, pool *sql.DB, token string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	urlPattern := fmt.Sprintf("https://example.com/jobs/%s/%%", token)

	stmts := []struct {
		label string
		sql   string
		args  []any
	}{
		{"job_posting_skills", `
			DELETE FROM job_posting_skills WHERE classification_id IN (
				SELECT c.id FROM classifications c
				JOIN job_postings jp ON jp.id = c.job_posting_id
				WHERE jp.source_url LIKE $1
			)`, []any{urlPattern}},
		{"job_posting_specializations", `
			DELETE FROM job_posting_specializations WHERE classification_id IN (
				SELECT c.id FROM classifications c
				JOIN job_postings jp ON jp.id = c.job_posting_id
				WHERE jp.source_url LIKE $1
			)`, []any{urlPattern}},
		{"job_posting_roles", `
			DELETE FROM job_posting_roles WHERE classification_id IN (
				SELECT c.id FROM classifications c
				JOIN job_postings jp ON jp.id = c.job_posting_id
				WHERE jp.source_url LIKE $1
			)`, []any{urlPattern}},
		{"classifications", `
			DELETE FROM classifications WHERE job_posting_id IN (
				SELECT id FROM job_postings WHERE source_url LIKE $1
			)`, []any{urlPattern}},
		{"posting_snapshots", `
			DELETE FROM posting_snapshots WHERE job_posting_id IN (
				SELECT id FROM job_postings WHERE source_url LIKE $1
			)`, []any{urlPattern}},
		{"job_postings", `DELETE FROM job_postings WHERE source_url LIKE $1`, []any{urlPattern}},
		{"companies", `DELETE FROM companies WHERE board_token = $1`, []any{token}},
		// Taxonomy rows are slug-namespaced separately; see cleanupTaxonomy.
	}

	for _, s := range stmts {
		if _, err := pool.ExecContext(ctx, s.sql, s.args...); err != nil {
			t.Logf("cleanup %s: %v", s.label, err)
		}
	}
}

// cleanupTaxonomy removes taxonomy rows whose slug carries the per-test
// token. Taxonomy is global, so the namespace is enforced by the slug
// content rather than a join.
func cleanupTaxonomy(t *testing.T, pool *sql.DB, token string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pattern := "%" + token + "%"

	// canonical_role_dimensions FK -> canonical_roles ON DELETE CASCADE.
	stmts := []struct {
		label string
		sql   string
	}{
		{"canonical_roles", `DELETE FROM canonical_roles WHERE slug LIKE $1`},
		{"specializations", `DELETE FROM specializations WHERE slug LIKE $1`},
		{"skills", `DELETE FROM skills WHERE slug LIKE $1`},
	}
	for _, s := range stmts {
		if _, err := pool.ExecContext(ctx, s.sql, pattern); err != nil {
			t.Logf("cleanup taxonomy %s: %v", s.label, err)
		}
	}
}

// containsID reports whether ids contains target.
func containsID(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func TestSelectPostings_ForceFalseSkipsClassified(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	token := integrationToken(t)
	t.Cleanup(func() { cleanupByToken(t, pool, token) })

	companyID := seedCompany(t, ctx, pool, token)
	now := time.Now().UTC().Truncate(time.Microsecond)
	desc := "We are hiring a designer for our team."
	p1 := seedPosting(t, ctx, pool, companyID, token, "a", "Designer A", &desc, now)
	p2 := seedPosting(t, ctx, pool, companyID, token, "b", "Designer B", &desc, now.Add(1*time.Minute))
	p3 := seedPosting(t, ctx, pool, companyID, token, "c", "Designer C", &desc, now.Add(2*time.Minute))

	// Mark the middle posting as already classified.
	seedClassification(t, ctx, pool, p2)

	// Count is intentionally large: the dev DB carries unclassified postings
	// from prior runs, and the query orders by first_seen_at ASC. Our seeded
	// rows have current timestamps so they sit at the tail of the ordering;
	// we filter the result down to this test's company_id for assertions.
	cfg := Config{Count: 1000000, Focus: "", Force: false}
	postings, already, err := SelectPostings(ctx, pool, cfg)
	if err != nil {
		t.Fatalf("SelectPostings: %v", err)
	}
	if already != nil {
		t.Errorf("expected nil alreadyClassified for force=false, got %v", already)
	}

	var got []int64
	for _, p := range postings {
		if p.CompanyID == companyID {
			got = append(got, p.PostingID)
		}
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 unclassified postings, got %d (%v)", len(got), got)
	}
	if got[0] != p1 || got[1] != p3 {
		t.Errorf("expected ordering [p1=%d, p3=%d], got %v", p1, p3, got)
	}
	if containsID(got, p2) {
		t.Errorf("classified posting p2=%d unexpectedly returned", p2)
	}
}

func TestSelectPostings_ForceTrueIncludesClassified(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	token := integrationToken(t)
	t.Cleanup(func() { cleanupByToken(t, pool, token) })

	companyID := seedCompany(t, ctx, pool, token)
	now := time.Now().UTC().Truncate(time.Microsecond)
	desc := "Hiring a senior engineer."
	p1 := seedPosting(t, ctx, pool, companyID, token, "a", "Eng A", &desc, now)
	p2 := seedPosting(t, ctx, pool, companyID, token, "b", "Eng B", &desc, now.Add(1*time.Minute))
	p3 := seedPosting(t, ctx, pool, companyID, token, "c", "Eng C", &desc, now.Add(2*time.Minute))
	seedClassification(t, ctx, pool, p2)

	cfg := Config{Count: 1000000, Focus: "", Force: true}
	postings, already, err := SelectPostings(ctx, pool, cfg)
	if err != nil {
		t.Fatalf("SelectPostings: %v", err)
	}

	var got []int64
	for _, p := range postings {
		if p.CompanyID == companyID {
			got = append(got, p.PostingID)
		}
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 postings with force=true, got %d (%v)", len(got), got)
	}
	if got[0] != p1 || got[1] != p2 || got[2] != p3 {
		t.Errorf("expected ordering [%d,%d,%d], got %v", p1, p2, p3, got)
	}

	// alreadyClassified should contain exactly p2 from this test's seed set.
	// Other rows in the dev DB may add unrelated ids; filter to ours.
	var alreadyMine []int64
	mine := map[int64]bool{p1: true, p2: true, p3: true}
	for _, id := range already {
		if mine[id] {
			alreadyMine = append(alreadyMine, id)
		}
	}
	if len(alreadyMine) != 1 || alreadyMine[0] != p2 {
		t.Errorf("expected alreadyClassified to include only p2=%d, got %v", p2, alreadyMine)
	}
}

func TestSelectPostings_FocusFilter(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	token := integrationToken(t)
	t.Cleanup(func() { cleanupByToken(t, pool, token) })

	companyID := seedCompany(t, ctx, pool, token)
	now := time.Now().UTC().Truncate(time.Microsecond)
	goDesc := "We use golang and postgres."
	rubyDesc := "We use ruby and rails."

	pGo := seedPosting(t, ctx, pool, companyID, token, "go", "Backend Engineer (golang)", &goDesc, now)
	pRuby := seedPosting(t, ctx, pool, companyID, token, "rb", "Backend Engineer (ruby)", &rubyDesc, now.Add(1*time.Minute))

	// Focus must be unique enough that it does not match unrelated dev-DB rows.
	focus := "golang-" + token
	// Re-seed the go posting's description to include the unique focus token
	// so the ILIKE matches without depending on the literal "golang".
	if _, err := pool.ExecContext(ctx, `
		UPDATE posting_snapshots SET description_text = $1
		WHERE job_posting_id = $2
	`, "We use "+focus+" and postgres.", pGo); err != nil {
		t.Fatalf("update description: %v", err)
	}

	cfg := Config{Count: 1000000, Focus: focus, Force: false}
	postings, _, err := SelectPostings(ctx, pool, cfg)
	if err != nil {
		t.Fatalf("SelectPostings: %v", err)
	}

	var got []int64
	for _, p := range postings {
		if p.CompanyID == companyID {
			got = append(got, p.PostingID)
		}
	}
	if len(got) != 1 || got[0] != pGo {
		t.Errorf("expected only pGo=%d, got %v (pRuby=%d should be filtered out)", pGo, got, pRuby)
	}
}

func TestSelectPostings_NullDescriptionExcluded(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	token := integrationToken(t)
	t.Cleanup(func() { cleanupByToken(t, pool, token) })

	companyID := seedCompany(t, ctx, pool, token)
	now := time.Now().UTC().Truncate(time.Microsecond)
	desc := "Real description text."
	pWith := seedPosting(t, ctx, pool, companyID, token, "with", "With desc", &desc, now)
	pNull := seedPosting(t, ctx, pool, companyID, token, "null", "Null desc", nil, now.Add(1*time.Minute))

	for _, force := range []bool{false, true} {
		cfg := Config{Count: 1000000, Focus: "", Force: force}
		postings, _, err := SelectPostings(ctx, pool, cfg)
		if err != nil {
			t.Fatalf("SelectPostings(force=%v): %v", force, err)
		}
		var got []int64
		for _, p := range postings {
			if p.CompanyID == companyID {
				got = append(got, p.PostingID)
			}
		}
		if containsID(got, pNull) {
			t.Errorf("force=%v: null-description posting pNull=%d unexpectedly returned", force, pNull)
		}
		if !containsID(got, pWith) {
			t.Errorf("force=%v: expected pWith=%d to be returned, got %v", force, pWith, got)
		}
	}
}

// loadTestTaxonomy loads taxonomy after seeding so newly-inserted slugs are
// visible. Returned Taxonomy is the input shape WriteBack consumes.
func loadTestTaxonomy(t *testing.T, ctx context.Context, pool *sql.DB) Taxonomy {
	t.Helper()
	tx, err := LoadTaxonomy(ctx, pool)
	if err != nil {
		t.Fatalf("LoadTaxonomy: %v", err)
	}
	return tx
}

func TestWriteBack_HappyPath(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	token := integrationToken(t)
	// Cleanup runs LIFO: cleanupByToken (deletes join rows) must run BEFORE
	// cleanupTaxonomy (deletes taxonomy parents) to satisfy FK constraints.
	t.Cleanup(func() { cleanupTaxonomy(t, pool, token) })
	t.Cleanup(func() { cleanupByToken(t, pool, token) })

	companyID := seedCompany(t, ctx, pool, token)
	now := time.Now().UTC().Truncate(time.Microsecond)
	desc := "Description for writeback test."
	postingID := seedPosting(t, ctx, pool, companyID, token, "wb", "Designer", &desc, now)

	// role_dimensions are seeded via migration 000001 ("design", "engineering", ...).
	// Use the seeded slugs so the dimension lookup at writeback time succeeds.
	taxonomy := loadTestTaxonomy(t, ctx, pool)
	if _, ok := taxonomy.RoleDimensions["design"]; !ok {
		t.Fatalf("expected seeded role_dimension 'design' to be present")
	}
	if _, ok := taxonomy.RoleDimensions["engineering"]; !ok {
		t.Fatalf("expected seeded role_dimension 'engineering' to be present")
	}

	roleSlug := "role-" + token
	specSlug := "spec-" + token
	skillSlug := "skill-" + token

	vc := ValidatedClassification{
		AgentResponse: AgentResponse{
			PostingID: postingID,
			Classification: AgentClassification{
				Seniority: "senior",
				Notes:     "  ", // whitespace -> NULL
			},
			CanonicalRoles: []AgentCanonicalRole{{
				Slug:       roleSlug,
				Name:       "Test Role",
				Dimensions: []string{"design", "engineering"},
			}},
			Specializations: []AgentSpecialization{{Slug: specSlug, Name: "Test Spec"}},
			Skills:          []AgentSkill{{Slug: skillSlug, Name: "Test Skill"}},
			Summary:         "summary text",
		},
	}

	results := []PostingResult{{
		PostingID:      postingID,
		CompanyID:      companyID,
		Title:          "Designer",
		Outcome:        OutcomeEnriched,
		Classification: &vc,
	}}

	cfg := Config{PromptVersion: PromptVersion, Runner: RunnerCodexExec, Model: CodexExecModel}
	out := WriteBack(ctx, results, pool, cfg, taxonomy)

	if out[0].Outcome != OutcomeEnriched {
		t.Fatalf("expected OutcomeEnriched, got %q (reason=%s)", out[0].Outcome, out[0].LastReason)
	}

	// One classifications row with correct fields.
	var classCount int
	var gotModel, gotPromptVersion, gotSeniority string
	var gotNotes sql.NullString
	if err := pool.QueryRowContext(ctx, `
		SELECT count(*) FROM classifications WHERE job_posting_id = $1
	`, postingID).Scan(&classCount); err != nil {
		t.Fatalf("count classifications: %v", err)
	}
	if classCount != 1 {
		t.Errorf("expected 1 classifications row, got %d", classCount)
	}
	var classID int64
	if err := pool.QueryRowContext(ctx, `
		SELECT id, model, prompt_version, seniority, notes
		FROM classifications WHERE job_posting_id = $1
	`, postingID).Scan(&classID, &gotModel, &gotPromptVersion, &gotSeniority, &gotNotes); err != nil {
		t.Fatalf("select classification: %v", err)
	}
	if gotModel != cfg.Model {
		t.Errorf("model = %q, want %q", gotModel, cfg.Model)
	}
	if gotPromptVersion != cfg.PromptVersion {
		t.Errorf("prompt_version = %q, want %q", gotPromptVersion, cfg.PromptVersion)
	}
	if gotSeniority != "senior" {
		t.Errorf("seniority = %q, want senior", gotSeniority)
	}
	if gotNotes.Valid {
		t.Errorf("notes should be NULL for whitespace-only input, got %q", gotNotes.String)
	}

	// canonical_roles, specializations, skills each have exactly one row for our slug.
	var n int
	if err := pool.QueryRowContext(ctx, `SELECT count(*) FROM canonical_roles WHERE slug = $1`, roleSlug).Scan(&n); err != nil {
		t.Fatalf("count canonical_roles: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 canonical_roles row for %q, got %d", roleSlug, n)
	}
	if err := pool.QueryRowContext(ctx, `SELECT count(*) FROM specializations WHERE slug = $1`, specSlug).Scan(&n); err != nil {
		t.Fatalf("count specializations: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 specializations row for %q, got %d", specSlug, n)
	}
	if err := pool.QueryRowContext(ctx, `SELECT count(*) FROM skills WHERE slug = $1`, skillSlug).Scan(&n); err != nil {
		t.Fatalf("count skills: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 skills row for %q, got %d", skillSlug, n)
	}

	// One job_posting_roles, one job_posting_specializations, one job_posting_skills row.
	if err := pool.QueryRowContext(ctx, `SELECT count(*) FROM job_posting_roles WHERE classification_id = $1`, classID).Scan(&n); err != nil {
		t.Fatalf("count job_posting_roles: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 job_posting_roles row, got %d", n)
	}
	if err := pool.QueryRowContext(ctx, `SELECT count(*) FROM job_posting_specializations WHERE classification_id = $1`, classID).Scan(&n); err != nil {
		t.Fatalf("count job_posting_specializations: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 job_posting_specializations row, got %d", n)
	}
	if err := pool.QueryRowContext(ctx, `SELECT count(*) FROM job_posting_skills WHERE classification_id = $1`, classID).Scan(&n); err != nil {
		t.Fatalf("count job_posting_skills: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 job_posting_skills row, got %d", n)
	}

	// canonical_role_dimensions: one row per dimension slug we passed.
	if err := pool.QueryRowContext(ctx, `
		SELECT count(*) FROM canonical_role_dimensions crd
		JOIN canonical_roles cr ON cr.id = crd.canonical_role_id
		WHERE cr.slug = $1
	`, roleSlug).Scan(&n); err != nil {
		t.Fatalf("count canonical_role_dimensions: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 canonical_role_dimensions rows (design + engineering), got %d", n)
	}
}

func TestWriteBack_IdempotentTaxonomyUpsert(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	token := integrationToken(t)
	t.Cleanup(func() { cleanupTaxonomy(t, pool, token) })
	t.Cleanup(func() { cleanupByToken(t, pool, token) })

	companyID := seedCompany(t, ctx, pool, token)
	now := time.Now().UTC().Truncate(time.Microsecond)
	desc := "Idempotent writeback test."
	postingA := seedPosting(t, ctx, pool, companyID, token, "a", "Designer A", &desc, now)
	postingB := seedPosting(t, ctx, pool, companyID, token, "b", "Designer B", &desc, now.Add(1*time.Minute))

	taxonomy := loadTestTaxonomy(t, ctx, pool)
	roleSlug := "role-" + token
	specSlug := "spec-" + token
	skillSlug := "skill-" + token

	makeVC := func(postingID int64) *ValidatedClassification {
		return &ValidatedClassification{
			AgentResponse: AgentResponse{
				PostingID:      postingID,
				Classification: AgentClassification{Seniority: "mid"},
				CanonicalRoles: []AgentCanonicalRole{{
					Slug:       roleSlug,
					Name:       "Shared Role",
					Dimensions: []string{"design"},
				}},
				Specializations: []AgentSpecialization{{Slug: specSlug, Name: "Shared Spec"}},
				Skills:          []AgentSkill{{Slug: skillSlug, Name: "Shared Skill"}},
			},
		}
	}

	cfg := Config{PromptVersion: PromptVersion, Runner: RunnerCodexExec, Model: CodexExecModel}
	// First writeback creates taxonomy rows.
	first := WriteBack(ctx, []PostingResult{{
		PostingID:      postingA,
		CompanyID:      companyID,
		Title:          "Designer A",
		Outcome:        OutcomeEnriched,
		Classification: makeVC(postingA),
	}}, pool, cfg, taxonomy)
	if first[0].Outcome != OutcomeEnriched {
		t.Fatalf("first writeback: expected OutcomeEnriched, got %q (%s)", first[0].Outcome, first[0].LastReason)
	}

	// Second writeback re-uses the same slugs from a different posting.
	second := WriteBack(ctx, []PostingResult{{
		PostingID:      postingB,
		CompanyID:      companyID,
		Title:          "Designer B",
		Outcome:        OutcomeEnriched,
		Classification: makeVC(postingB),
	}}, pool, cfg, taxonomy)
	if second[0].Outcome != OutcomeEnriched {
		t.Fatalf("second writeback: expected OutcomeEnriched, got %q (%s)", second[0].Outcome, second[0].LastReason)
	}

	// Exactly one canonical_roles row for the shared slug.
	var n int
	if err := pool.QueryRowContext(ctx, `SELECT count(*) FROM canonical_roles WHERE slug = $1`, roleSlug).Scan(&n); err != nil {
		t.Fatalf("count canonical_roles: %v", err)
	}
	if n != 1 {
		t.Errorf("expected exactly 1 canonical_roles row for shared slug, got %d", n)
	}

	// Two classifications across the two postings.
	if err := pool.QueryRowContext(ctx, `
		SELECT count(*) FROM classifications WHERE job_posting_id IN ($1, $2)
	`, postingA, postingB).Scan(&n); err != nil {
		t.Fatalf("count classifications: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 classifications rows, got %d", n)
	}

	// Two job_posting_roles rows, one per classification, both pointing at the shared role.
	if err := pool.QueryRowContext(ctx, `
		SELECT count(*) FROM job_posting_roles jpr
		JOIN classifications c ON c.id = jpr.classification_id
		JOIN canonical_roles cr ON cr.id = jpr.role_id
		WHERE c.job_posting_id IN ($1, $2) AND cr.slug = $3
	`, postingA, postingB, roleSlug).Scan(&n); err != nil {
		t.Fatalf("count job_posting_roles: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 job_posting_roles rows, got %d", n)
	}
}

func TestWriteBack_FailedTransactionCancelledContext(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	token := integrationToken(t)
	t.Cleanup(func() { cleanupTaxonomy(t, pool, token) })
	t.Cleanup(func() { cleanupByToken(t, pool, token) })

	companyID := seedCompany(t, ctx, pool, token)
	now := time.Now().UTC().Truncate(time.Microsecond)
	desc := "Cancelled writeback test."
	postingID := seedPosting(t, ctx, pool, companyID, token, "x", "Designer X", &desc, now)

	taxonomy := loadTestTaxonomy(t, ctx, pool)
	roleSlug := "role-" + token
	specSlug := "spec-" + token
	skillSlug := "skill-" + token

	vc := &ValidatedClassification{
		AgentResponse: AgentResponse{
			PostingID:      postingID,
			Classification: AgentClassification{Seniority: "senior"},
			CanonicalRoles: []AgentCanonicalRole{{
				Slug:       roleSlug,
				Name:       "Role X",
				Dimensions: []string{"design"},
			}},
			Specializations: []AgentSpecialization{{Slug: specSlug, Name: "Spec X"}},
			Skills:          []AgentSkill{{Slug: skillSlug, Name: "Skill X"}},
		},
	}

	// Cancel before calling WriteBack so BeginTx (or its first query) fails.
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	cfg := Config{PromptVersion: PromptVersion, Runner: RunnerCodexExec, Model: CodexExecModel}
	out := WriteBack(cancelCtx, []PostingResult{{
		PostingID:      postingID,
		CompanyID:      companyID,
		Title:          "Designer X",
		Outcome:        OutcomeEnriched,
		Classification: vc,
	}}, pool, cfg, taxonomy)

	// WriteBack's loop checks ctx.Err() before each posting. On cancellation
	// it stamps every remaining OutcomeEnriched result as OutcomeDBFailed with
	// a "cancelled" reason so the run report can't claim success for postings
	// that were never persisted.
	if out[0].Outcome != OutcomeDBFailed {
		t.Errorf("expected OutcomeDBFailed for cancelled posting, got %q", out[0].Outcome)
	}
	if !strings.Contains(out[0].LastReason, "cancelled") {
		t.Errorf("expected LastReason to contain \"cancelled\", got %q", out[0].LastReason)
	}

	// Verify no rows were committed under the cancelled context.
	var n int
	verifyCtx, vcancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer vcancel()
	if err := pool.QueryRowContext(verifyCtx, `SELECT count(*) FROM classifications WHERE job_posting_id = $1`, postingID).Scan(&n); err != nil {
		t.Fatalf("count classifications: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 classifications after cancelled writeback, got %d", n)
	}
	if err := pool.QueryRowContext(verifyCtx, `SELECT count(*) FROM canonical_roles WHERE slug = $1`, roleSlug).Scan(&n); err != nil {
		t.Fatalf("count canonical_roles: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 canonical_roles for slug %q after cancelled writeback, got %d", roleSlug, n)
	}
}
