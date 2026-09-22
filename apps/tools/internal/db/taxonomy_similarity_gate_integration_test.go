//go:build integration

package db_test

// Exercises the two things migrations 000039 and 000040 changed about the
// taxonomy gate in mcp.save_enrichment: that every near match it reports is
// also recorded, and that the mint-only name collision rule now covers
// canonical_roles and compares names with all whitespace removed.
//
// Every test runs inside a transaction that is rolled back, so the taxonomy
// rows they seed never reach the real vocabulary. That matters more here than
// elsewhere: a leaked fixture skill is a permanent entry in a table the
// classifier reads on every run.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql
)

const (
	gateTestModel  = "integration-test"
	gateTestPrompt = "v-integration-test"
)

// saveEnrichmentEnvelope is the subset of mcp.save_enrichment's reply these
// tests read. The full envelope carries more; decoding only what is asserted
// keeps the test from failing on an unrelated additive change.
type saveEnrichmentEnvelope struct {
	OK     bool `json:"ok"`
	Errors []struct {
		Code         string `json:"code"`
		Table        string `json:"table"`
		ProposedSlug string `json:"proposed_slug"`
		ExistingSlug string `json:"existing_slug"`
	} `json:"errors"`
	SimilarityCandidates []struct {
		Table      string `json:"table"`
		MintedSlug string `json:"minted_slug"`
		Candidates []struct {
			Slug string `json:"slug"`
		} `json:"candidates"`
	} `json:"similarity_candidates"`
	Substitutions []struct {
		Table           string `json:"table"`
		ProposedSlug    string `json:"proposed_slug"`
		SubstitutedSlug string `json:"substituted_slug"`
	} `json:"substitutions"`
}

// advisoryRow mirrors one taxonomy_similarity_advisories row.
type advisoryRow struct {
	TableName          string
	ProposedSlug       string
	CandidateSlug      string
	CandidateRank      int
	Source             string
	MatchKind          string
	SlugSimilarity     float64
	NameSimilarity     float64
	AdvisorySimilarity float64
	Outcome            string
	Model              string
	PromptVersion      string
}

func gateTx(t *testing.T) (context.Context, *sql.Tx) {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx := t.Context()
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	return ctx, tx
}

// gateSeedPosting creates the company and job posting a classification needs,
// and returns the posting id plus a suffix unique to this test run. The suffix
// is digits only because the gate validates slugs against
// ^[a-z0-9]+(-[a-z0-9]+)*$.
func gateSeedPosting(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, string) {
	t.Helper()

	suffix := time.Now().UTC().Format("20060102150405000000000")

	var companyID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO companies (name, ats, board_token)
		VALUES ($1, 'greenhouse', $2)
		RETURNING id
	`, "MS Gate "+suffix, "ms-gate-"+suffix).Scan(&companyID); err != nil {
		t.Fatalf("insert company: %v", err)
	}

	var postingID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO job_postings (company_id, source_type, source_url)
		VALUES ($1, 'ats', $2)
		RETURNING id
	`, companyID, "https://example.invalid/ms-gate/"+suffix).Scan(&postingID); err != nil {
		t.Fatalf("insert job posting: %v", err)
	}

	return postingID, suffix
}

func gateSeedSkill(ctx context.Context, t *testing.T, tx *sql.Tx, slug, name string) {
	t.Helper()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO skills (slug, name) VALUES ($1, $2)`, slug, name); err != nil {
		t.Fatalf("seed skill %s: %v", slug, err)
	}
}

func gateSeedRole(ctx context.Context, t *testing.T, tx *sql.Tx, slug, name string) {
	t.Helper()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO canonical_roles (slug, name) VALUES ($1, $2)`, slug, name); err != nil {
		t.Fatalf("seed role %s: %v", slug, err)
	}
}

func gateSave(ctx context.Context, t *testing.T, tx *sql.Tx, payload string) saveEnrichmentEnvelope {
	t.Helper()

	var raw []byte
	if err := tx.QueryRowContext(ctx,
		`SELECT mcp.save_enrichment($1::jsonb, $2, $3)`,
		payload, gateTestModel, gateTestPrompt).Scan(&raw); err != nil {
		t.Fatalf("mcp.save_enrichment: %v", err)
	}

	var env saveEnrichmentEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode envelope %s: %v", raw, err)
	}
	return env
}

func gateAdvisories(ctx context.Context, t *testing.T, tx *sql.Tx, postingID int64) []advisoryRow {
	t.Helper()

	rows, err := tx.QueryContext(ctx, `
		SELECT table_name, proposed_slug, candidate_slug, candidate_rank,
		       candidate_source, match_kind, slug_similarity, name_similarity,
		       advisory_similarity, outcome, model, prompt_version
		FROM taxonomy_similarity_advisories
		WHERE job_posting_id = $1
		ORDER BY table_name, proposed_slug, candidate_rank
	`, postingID)
	if err != nil {
		t.Fatalf("select advisories: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var out []advisoryRow
	for rows.Next() {
		var r advisoryRow
		if err := rows.Scan(&r.TableName, &r.ProposedSlug, &r.CandidateSlug, &r.CandidateRank,
			&r.Source, &r.MatchKind, &r.SlugSimilarity, &r.NameSimilarity,
			&r.AdvisorySimilarity, &r.Outcome, &r.Model, &r.PromptVersion); err != nil {
			t.Fatalf("scan advisory: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate advisories: %v", err)
	}
	return out
}

// A worker shown a near match that mints anyway is the case the audit table
// exists for: before 000039 the only record of it was the worker's own
// account.
func TestSaveEnrichment_RecordsAdvisoryCandidateWhenWorkerMintsAnyway(t *testing.T) {
	ctx, tx := gateTx(t)
	postingID, suffix := gateSeedPosting(ctx, t, tx)

	existingSlug := "msg" + suffix + "-widget-analysis"
	gateSeedSkill(ctx, t, tx, existingSlug, "MS Gate "+suffix+" Widget Analysis")

	// Slug-similar enough to be reported (>= 0.60), far enough from the
	// substitution threshold (0.95) that the gate leaves the decision to the
	// worker. The names differ, so the mint-only name collision rule does not
	// fire.
	mintedSlug := "msg" + suffix + "-widget-analytics"
	payload := fmt.Sprintf(`{
		"posting_id": %d,
		"classification": {"seniority": "mid", "notes": "gate audit fixture"},
		"canonical_roles": [],
		"specializations": [],
		"skills": [{"slug": %q, "name": "MS Gate %s Widget Analytics"}]
	}`, postingID, mintedSlug, suffix)

	env := gateSave(ctx, t, tx, payload)
	if !env.OK {
		t.Fatalf("save_enrichment returned errors: %+v", env.Errors)
	}
	if len(env.SimilarityCandidates) == 0 {
		t.Fatalf("expected an advisory candidate for %s, envelope reported none", mintedSlug)
	}

	got := gateAdvisories(ctx, t, tx, postingID)
	if len(got) == 0 {
		t.Fatalf("the gate reported %d advisory subjects and persisted nothing",
			len(env.SimilarityCandidates))
	}

	var found *advisoryRow
	for i := range got {
		if got[i].ProposedSlug == mintedSlug && got[i].CandidateSlug == existingSlug {
			found = &got[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no audit row for %s -> %s; got %+v", mintedSlug, existingSlug, got)
	}

	if found.TableName != "skills" {
		t.Errorf("table_name = %q, want %q", found.TableName, "skills")
	}
	if found.Source != "advisory" {
		t.Errorf("candidate_source = %q, want %q", found.Source, "advisory")
	}
	// The worker was shown the neighbour and minted regardless. That is the
	// number the vocabulary-health report could previously only ask an agent
	// for.
	if found.Outcome != "minted" {
		t.Errorf("outcome = %q, want %q", found.Outcome, "minted")
	}
	if found.CandidateRank < 1 {
		t.Errorf("candidate_rank = %d, want >= 1", found.CandidateRank)
	}
	// Both axes are stored, not just the combined figure the envelope shows:
	// 000034 demoted the name axis precisely because the two disagree.
	if found.SlugSimilarity <= 0 || found.SlugSimilarity > 1 {
		t.Errorf("slug_similarity = %v, want within (0, 1]", found.SlugSimilarity)
	}
	if found.NameSimilarity <= 0 || found.NameSimilarity > 1 {
		t.Errorf("name_similarity = %v, want within (0, 1]", found.NameSimilarity)
	}
	if found.AdvisorySimilarity < found.SlugSimilarity && found.AdvisorySimilarity < found.NameSimilarity {
		t.Errorf("advisory_similarity = %v, want the greater of slug %v and name %v",
			found.AdvisorySimilarity, found.SlugSimilarity, found.NameSimilarity)
	}
	if found.Model != gateTestModel || found.PromptVersion != gateTestPrompt {
		t.Errorf("provenance = (%q, %q), want (%q, %q)",
			found.Model, found.PromptVersion, gateTestModel, gateTestPrompt)
	}
}

// The contrasting case: the gate remapped the proposal onto an existing row,
// so nothing was minted. Both cases land in one table so the mint rate is a
// GROUP BY rather than a join.
func TestSaveEnrichment_RecordsSubstitutionAsReused(t *testing.T) {
	ctx, tx := gateTx(t)
	postingID, suffix := gateSeedPosting(ctx, t, tx)

	existingSlug := "msg" + suffix + "-alpha-beta"
	gateSeedSkill(ctx, t, tx, existingSlug, "MS Gate "+suffix+" Alpha Beta")

	// A word-order restatement: trigram-identical on the slug axis, so Phase A
	// substitutes rather than advises.
	proposedSlug := "msg" + suffix + "-beta-alpha"
	payload := fmt.Sprintf(`{
		"posting_id": %d,
		"classification": {"seniority": "mid", "notes": "gate audit fixture"},
		"canonical_roles": [],
		"specializations": [],
		"skills": [{"slug": %q, "name": "MS Gate %s Beta Alpha"}]
	}`, postingID, proposedSlug, suffix)

	env := gateSave(ctx, t, tx, payload)
	if !env.OK {
		t.Fatalf("save_enrichment returned errors: %+v", env.Errors)
	}
	if len(env.Substitutions) == 0 {
		t.Fatalf("expected %s to be substituted onto %s, envelope reported no substitution",
			proposedSlug, existingSlug)
	}

	got := gateAdvisories(ctx, t, tx, postingID)
	var found *advisoryRow
	for i := range got {
		if got[i].ProposedSlug == proposedSlug {
			found = &got[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no audit row for substituted %s; got %+v", proposedSlug, got)
	}

	if found.Source != "substitution" {
		t.Errorf("candidate_source = %q, want %q", found.Source, "substitution")
	}
	if found.CandidateSlug != existingSlug {
		t.Errorf("candidate_slug = %q, want %q", found.CandidateSlug, existingSlug)
	}
	// outcome is read off the write that happened, not off which gate branch
	// produced the row -- so a substitution that somehow still minted would
	// show up here rather than being assumed away.
	if found.Outcome != "reused" {
		t.Errorf("outcome = %q, want %q", found.Outcome, "reused")
	}

	var exists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM skills WHERE slug = $1)`, proposedSlug).Scan(&exists); err != nil {
		t.Fatalf("check minted slug: %v", err)
	}
	if exists {
		t.Errorf("%s was minted despite being substituted", proposedSlug)
	}
}

// 000038 excluded canonical_roles because three collision clusters were still
// live in it. 000040 merged them and dropped the exclusion.
func TestSaveEnrichment_BlocksNameCollisionInCanonicalRoles(t *testing.T) {
	ctx, tx := gateTx(t)
	postingID, suffix := gateSeedPosting(ctx, t, tx)

	existingSlug := "msgrole" + suffix + "-alpha"
	roleName := "MS Gate " + suffix + " Operations Lead"
	gateSeedRole(ctx, t, tx, existingSlug, roleName)

	proposedSlug := "msgrole" + suffix + "-bravo"
	payload := fmt.Sprintf(`{
		"posting_id": %d,
		"classification": {"seniority": "mid", "notes": "gate audit fixture"},
		"canonical_roles": [{"slug": %q, "name": %q, "dimensions": ["operations"]}],
		"specializations": [],
		"skills": []
	}`, postingID, proposedSlug, roleName)

	env := gateSave(ctx, t, tx, payload)
	if env.OK {
		t.Fatalf("minting %q under the name of %q was allowed", proposedSlug, existingSlug)
	}

	var matched bool
	for _, e := range env.Errors {
		if e.Code == "name_collision" && e.Table == "canonical_roles" &&
			e.ProposedSlug == proposedSlug && e.ExistingSlug == existingSlug {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("expected a name_collision naming %q; got %+v", existingSlug, env.Errors)
	}

	// Blocking writes nothing at all -- not the role, not the classification.
	var roleExists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM canonical_roles WHERE slug = $1)`, proposedSlug).Scan(&roleExists); err != nil {
		t.Fatalf("check minted role: %v", err)
	}
	if roleExists {
		t.Errorf("%s was minted despite the block", proposedSlug)
	}
}

// 000040's normalizer removes all whitespace and leaves punctuation alone. The
// two subtests are the two halves of that: it must catch a name that differs
// only in spacing, and it must not fold names that differ only in punctuation.
func TestSaveEnrichment_NameKeyIgnoresSpacingButNotPunctuation(t *testing.T) {
	t.Run("spacing variant collides", func(t *testing.T) {
		ctx, tx := gateTx(t)
		postingID, suffix := gateSeedPosting(ctx, t, tx)

		existingSlug := "msg" + suffix + "-power-tool"
		gateSeedSkill(ctx, t, tx, existingSlug, "MSG"+suffix+" Power Tool")

		// Identical once whitespace is removed; different under 000038's
		// collapse-and-trim key, which is what let 'powerbi' and 'power-bi'
		// coexist.
		proposedSlug := "msg" + suffix + "-powertool"
		payload := fmt.Sprintf(`{
			"posting_id": %d,
			"classification": {"seniority": "mid", "notes": "gate audit fixture"},
			"canonical_roles": [],
			"specializations": [],
			"skills": [{"slug": %q, "name": "MSG%s PowerTool"}]
		}`, postingID, proposedSlug, suffix)

		env := gateSave(ctx, t, tx, payload)
		if env.OK {
			t.Fatalf("minting %q was allowed although only its spacing differs from %q",
				proposedSlug, existingSlug)
		}
		var matched bool
		for _, e := range env.Errors {
			if e.Code == "name_collision" && e.Table == "skills" && e.ExistingSlug == existingSlug {
				matched = true
			}
		}
		if !matched {
			t.Fatalf("expected a name_collision naming %q; got %+v", existingSlug, env.Errors)
		}
	})

	t.Run("punctuation variants stay distinct", func(t *testing.T) {
		ctx, tx := gateTx(t)
		postingID, suffix := gateSeedPosting(ctx, t, tx)

		// The c / cpp / csharp shape, under names this test owns. A
		// punctuation-stripping normalizer would give all three one key and
		// merge three languages; similarity() already scores 'C++' against
		// 'C#' at 1.000, which is why the comparison is on text and not on
		// trigrams.
		bare := "msg" + suffix + "lang"
		gateSeedSkill(ctx, t, tx, bare+"-c", "MSG"+suffix)
		gateSeedSkill(ctx, t, tx, bare+"-cpp", "MSG"+suffix+"++")
		gateSeedSkill(ctx, t, tx, bare+"-csharp", "MSG"+suffix+"#")

		// Proposing the "++" name must collide with the "++" row specifically,
		// not with either of its neighbours -- which is only true if the three
		// keys are distinct.
		proposedSlug := bare + "-cplusplus"
		payload := fmt.Sprintf(`{
			"posting_id": %d,
			"classification": {"seniority": "mid", "notes": "gate audit fixture"},
			"canonical_roles": [],
			"specializations": [],
			"skills": [{"slug": %q, "name": "MSG%s++"}]
		}`, postingID, proposedSlug, suffix)

		env := gateSave(ctx, t, tx, payload)
		if env.OK {
			t.Fatalf("expected %q to collide with %q", proposedSlug, bare+"-cpp")
		}
		for _, e := range env.Errors {
			if e.Code != "name_collision" {
				continue
			}
			if e.ExistingSlug != bare+"-cpp" {
				t.Fatalf("collided with %q; want %q -- the punctuation-bearing names were folded together",
					e.ExistingSlug, bare+"-cpp")
			}
		}

		// And a name that shares no punctuation with any of them mints freely.
		freeSlug := bare + "-cdollar"
		freePayload := fmt.Sprintf(`{
			"posting_id": %d,
			"classification": {"seniority": "mid", "notes": "gate audit fixture"},
			"canonical_roles": [],
			"specializations": [],
			"skills": [{"slug": %q, "name": "MSG%s$"}]
		}`, postingID, freeSlug, suffix)

		freeEnv := gateSave(ctx, t, tx, freePayload)
		if !freeEnv.OK {
			t.Fatalf("a name differing only in punctuation was blocked: %+v", freeEnv.Errors)
		}
	})
}
