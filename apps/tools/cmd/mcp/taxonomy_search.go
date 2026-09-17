// taxonomy_search is a read-only MCP tool: it answers "does a slug for this
// concept already exist?" without making the agent load a whole taxonomy table.
// The skills table passed the generic query tool's rowCap in September 2026, so
// agents reading `SELECT slug, name FROM skills` were silently seeing a truncated
// taxonomy and re-minting slugs that already existed.
//
// Two properties matter more than raw ranking:
//
//   - Scoring is against BOTH slug and name, via pg_trgm similarity(). The agent
//     passes the concept it has in mind, not a guessed substring, so a concept
//     whose slug reads nothing like its name (skills has `data-orchestration`
//     named "Data Pipeline Architecture") is still found.
//   - Results are CLUSTERED, not merely ranked. A flat menu of near-identical
//     slugs makes agents attach several of them to one posting — measured on real
//     data as one posting carrying cloud-platform-architecture plus
//     platform-architecture. Variants are therefore nested under a representative
//     so the response can only be read as "one concept, several spellings".
//
// It binds the read-only pool and never writes.
// See: agent-context/lib/developer-guide.md §5.7 (database access)
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	// taxonomySearchMaxTerms bounds the batch. Each term costs a full pass over
	// every taxonomy row (~2,900 today) times two similarity() calls, so the cap
	// is what keeps one call inside statementTimeout.
	taxonomySearchMaxTerms = 10

	// taxonomySearchMaxTermLength rejects prose pasted in place of a concept.
	// Trigram similarity against a paragraph is meaningless, not just slow.
	taxonomySearchMaxTermLength = 120

	// taxonomySearchMinScore is pg_trgm's own default threshold. Below it a match
	// shares so few trigrams that showing it would be noise the agent has to filter.
	taxonomySearchMinScore = 0.3

	// taxonomySearchClusterThreshold is the pairwise similarity at which two
	// entries are treated as spellings of one concept rather than as peers.
	//
	// Chosen at 0.7 rather than 0.6 by measuring real pairs in the live tables.
	// Both values group the cases this tool exists for — data-pipeline-architecture
	// with data-orchestration (1.000, identical names) and
	// cloud-platform-architecture with platform-architecture (0.786). But 0.6 also
	// merges genuinely distinct concepts that merely share a suffix:
	// data-architecture with data-pipeline-architecture (0.667),
	// data-architecture with data-warehouse-architecture (0.643), and kubernetes
	// with kubernetes-docker (0.611). Burying a distinct slug as someone else's
	// variant is the more expensive error here, because the agent reaches for the
	// representative.
	taxonomySearchClusterThreshold = 0.7

	// taxonomySearchCandidatesPerTable bounds the rows clustered for one term in
	// one table. Clustering is O(n^2) in neighbour lookups, and past ~20 the tail
	// is all sub-0.35 noise.
	taxonomySearchCandidatesPerTable = 20

	// taxonomySearchMaxClusters and taxonomySearchMaxVariants bound the response.
	// Worst case is terms x tables x candidates = 10 x 3 x 20 = 600 entries, well
	// under the generic query tool's rowCap of 1000 — returning at all is the
	// point of this tool.
	taxonomySearchMaxClusters = 5
	taxonomySearchMaxVariants = 6

	// taxonomySearchRepresentativeBand is how far below its cluster's best score a
	// member may fall and still be eligible to represent it. Usage count decides
	// among equally good matches — established vocabulary wins a tie — but a
	// well-used loose match never displaces a better one. Without the band, a term
	// like "data pipeline architecture" would be represented by whichever
	// "* Architecture" slug happened to be most attached.
	taxonomySearchRepresentativeBand = 0.1

	codeInvalidTerms = "invalid_terms"
	codeUnknownTable = "unknown_table"
)

// taxonomySearchTables is the closed set of searchable taxonomy tables, in the
// order results are reported. An omitted `tables` input means all three.
var taxonomySearchTables = []string{"canonical_roles", "specializations", "skills"}

// taxonomySearchRequest is the MCP tool DTO. Tables is nil-or-empty for "all
// three"; there is no way to ask for none, because a search over nothing is a
// malformed request, not an empty result.
type taxonomySearchRequest struct {
	Terms  []string `json:"terms"`
	Tables []string `json:"tables"`
}

// taxonomySearchEcho mirrors the resolved inputs back so the agent can see which
// tables the server actually searched after the default was applied.
type taxonomySearchEcho struct {
	Terms  []string `json:"terms"`
	Tables []string `json:"tables"`
}

// taxonomyEntry is one existing taxonomy row as the agent sees it. UsageCount is
// how many classification links point at it, so an agent can prefer established
// vocabulary; Score is its trigram similarity to the term that found it.
type taxonomyEntry struct {
	Table      string  `json:"table"`
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	UsageCount int64   `json:"usage_count"`
	Score      float64 `json:"score"`
}

// taxonomyCluster is one concept. Variants are nested under Representative
// deliberately: a flat sibling array is what produced postings carrying three
// spellings of the same skill. Score is the cluster's best match to the term.
type taxonomyCluster struct {
	Score          float64         `json:"score"`
	Representative taxonomyEntry   `json:"representative"`
	VariantCount   int             `json:"variant_count"`
	Variants       []taxonomyEntry `json:"variants"`
}

// taxonomyTableResult holds one term's clusters within one table. Clustering
// never crosses tables: a skill nested under a specialization would invite the
// agent to write the representative's slug into the wrong bucket.
type taxonomyTableResult struct {
	Table        string            `json:"table"`
	ClusterCount int               `json:"cluster_count"`
	Clusters     []taxonomyCluster `json:"clusters"`
}

// taxonomyTermResult is one input term's results, grouped by table so a match in
// canonical_roles is never crowded out by the much larger skills table.
type taxonomyTermResult struct {
	Term   string                `json:"term"`
	Tables []taxonomyTableResult `json:"tables"`
}

// taxonomySearchEnvelope is the JSON the tool returns. Invalid input sets
// Ok=false with errors[] rather than raising an MCP transport error. EntryCount
// is every representative plus variant across the whole response, so a caller can
// see at a glance that the result is small.
type taxonomySearchEnvelope struct {
	Ok         bool                 `json:"ok"`
	Input      taxonomySearchEcho   `json:"input"`
	EntryCount int                  `json:"entry_count"`
	Results    []taxonomyTermResult `json:"results"`
	Errors     []actionError        `json:"errors"`
}

// taxonomyRef identifies one taxonomy row. Ids are per-table bigserials, so the
// table is part of the identity.
type taxonomyRef struct {
	Table string `json:"table"`
	ID    int64  `json:"id"`
}

// taxonomyCandidate is one scored row plus the same-table candidates it is
// mutually similar to. Neighbors comes from SQL because the trigram arithmetic
// belongs in one place — reimplementing pg_trgm in Go would drift from the
// scoring the same query already did.
type taxonomyCandidate struct {
	TermIndex  int
	Ref        taxonomyRef
	Slug       string
	Name       string
	UsageCount int64
	Score      float64
	Neighbors  []taxonomyRef
}

// taxonomySearchSource is the lookup seam the handler depends on. Production
// scores and pairs rows in Postgres against the read-only pool; tests inject a
// fake to exercise validation, clustering, and representative choice without a
// database.
type taxonomySearchSource interface {
	Candidates(ctx context.Context, terms []string, tables []string) ([]taxonomyCandidate, error)
}

// poolTaxonomySearchSource binds the query to the read-only pool.
type poolTaxonomySearchSource struct {
	pool *sql.DB
}

// taxonomySearchSQL is a fixed, fully-parameterized statement. The caller never
// supplies SQL; terms, tables, and every threshold are bound values. It is not a
// sqlc query because it would force regenerating the db package while unrelated
// migrations are in flight — see the note in the accompanying report; moving it
// into internal/db/queries/ is a mechanical follow-up.
//
// $1 terms, $2 tables, $3 min score, $4 candidates per table, $5 cluster threshold.
const taxonomySearchSQL = `
WITH search_terms AS (
    SELECT ordinality::int AS term_index, term
    FROM unnest($1::text[]) WITH ORDINALITY AS t(term, ordinality)
), role_usage AS (
    SELECT role_id AS id, count(*)::bigint AS usage_count FROM job_posting_roles GROUP BY role_id
), specialization_usage AS (
    SELECT specialization_id AS id, count(*)::bigint AS usage_count FROM job_posting_specializations GROUP BY specialization_id
), skill_usage AS (
    SELECT skill_id AS id, count(*)::bigint AS usage_count FROM job_posting_skills GROUP BY skill_id
), entries AS MATERIALIZED (
    -- Usage comes from grouped aggregates rather than a correlated count per row:
    -- one pass over each link table instead of one scan per taxonomy entry.
    SELECT 'canonical_roles'::text AS table_name, r.id, r.slug, r.name,
           COALESCE(u.usage_count, 0) AS usage_count
    FROM canonical_roles r
    LEFT JOIN role_usage u ON u.id = r.id
    WHERE 'canonical_roles' = ANY($2::text[])
    UNION ALL
    SELECT 'specializations', s.id, s.slug, s.name, COALESCE(u.usage_count, 0)
    FROM specializations s
    LEFT JOIN specialization_usage u ON u.id = s.id
    WHERE 'specializations' = ANY($2::text[])
    UNION ALL
    SELECT 'skills', k.id, k.slug, k.name, COALESCE(u.usage_count, 0)
    FROM skills k
    LEFT JOIN skill_usage u ON u.id = k.id
    WHERE 'skills' = ANY($2::text[])
), scored AS (
    SELECT st.term_index, e.table_name, e.id, e.slug, e.name, e.usage_count,
           greatest(similarity(e.slug, st.term), similarity(e.name, st.term))::float8 AS score
    FROM search_terms st
    CROSS JOIN entries e
), ranked AS (
    SELECT scored.*,
           row_number() OVER (
               PARTITION BY scored.term_index, scored.table_name
               ORDER BY scored.score DESC, scored.usage_count DESC, scored.id
           ) AS rank_in_table
    FROM scored
    WHERE scored.score >= $3::float8
), candidates AS MATERIALIZED (
    SELECT * FROM ranked WHERE rank_in_table <= $4::int
)
SELECT c.term_index, c.table_name, c.id, c.slug, c.name, c.usage_count, c.score,
       COALESCE((
           SELECT jsonb_agg(jsonb_build_object('table', g.table_name, 'id', g.id) ORDER BY g.id)
           FROM candidates g
           WHERE g.term_index = c.term_index
             AND g.table_name = c.table_name
             AND g.id <> c.id
             AND greatest(
                   similarity(c.slug, g.slug), similarity(c.slug, g.name),
                   similarity(c.name, g.slug), similarity(c.name, g.name)
                 ) >= $5::float8
       ), '[]'::jsonb) AS neighbors
FROM candidates c
ORDER BY c.term_index, c.table_name, c.score DESC, c.usage_count DESC, c.id
`

func (s poolTaxonomySearchSource) Candidates(ctx context.Context, terms, tables []string) ([]taxonomyCandidate, error) {
	queryCtx, cancel := context.WithTimeout(ctx, statementTimeout)
	defer cancel()

	rows, err := s.pool.QueryContext(queryCtx, taxonomySearchSQL,
		terms, tables,
		taxonomySearchMinScore,
		taxonomySearchCandidatesPerTable,
		taxonomySearchClusterThreshold,
	)
	if err != nil {
		return nil, fmt.Errorf("scoring taxonomy candidates: %w", err)
	}
	defer rows.Close()

	var out []taxonomyCandidate
	for rows.Next() {
		var (
			cand      taxonomyCandidate
			neighbors []byte
		)
		if err := rows.Scan(
			&cand.TermIndex, &cand.Ref.Table, &cand.Ref.ID,
			&cand.Slug, &cand.Name, &cand.UsageCount, &cand.Score,
			&neighbors,
		); err != nil {
			return nil, fmt.Errorf("scanning taxonomy candidate: %w", err)
		}
		if err := json.Unmarshal(neighbors, &cand.Neighbors); err != nil {
			return nil, fmt.Errorf("decoding taxonomy neighbors: %w", err)
		}
		out = append(out, cand)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading taxonomy candidates: %w", err)
	}
	return out, nil
}

// taxonomySearchHandler wires the tool to the read-only pool. taxonomy_search
// never writes, so it deliberately receives the read-only handle.
func taxonomySearchHandler(pool *sql.DB) server.ToolHandlerFunc {
	return taxonomySearchHandlerWithSource(poolTaxonomySearchSource{pool: pool})
}

// taxonomySearchHandlerWithSource is the testable handler body.
func taxonomySearchHandlerWithSource(source taxonomySearchSource) server.ToolHandlerFunc {
	return func(ctx context.Context, mcpReq mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req taxonomySearchRequest
		if err := mcpReq.BindArguments(&req); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("decoding taxonomy_search arguments: %v", err)), nil
		}

		env := runTaxonomySearch(ctx, req, source)
		payload, err := json.Marshal(env)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encoding taxonomy_search result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(payload)), nil
	}
}

// runTaxonomySearch validates the request, runs the scored lookup, and clusters
// each term's candidates per table. Bad input returns an ok=false envelope; it
// never returns an MCP transport error.
func runTaxonomySearch(ctx context.Context, req taxonomySearchRequest, source taxonomySearchSource) taxonomySearchEnvelope {
	terms, tables, errs := validateTaxonomySearch(req)
	echo := taxonomySearchEcho{Terms: terms, Tables: tables}
	if len(errs) > 0 {
		return taxonomySearchFailure(echo, errs)
	}

	candidates, err := source.Candidates(ctx, terms, tables)
	if err != nil {
		return taxonomySearchFailure(echo, []actionError{{
			Path: "db", Code: codeDBError, Message: err.Error(),
		}})
	}

	// Bucket by (term, table). The source orders rows this way already, but
	// bucketing here keeps the clustering independent of that ordering so a fake
	// source can hand back candidates in any order.
	byTerm := make(map[int]map[string][]taxonomyCandidate, len(terms))
	for _, cand := range candidates {
		if cand.TermIndex < 1 || cand.TermIndex > len(terms) {
			continue
		}
		if byTerm[cand.TermIndex] == nil {
			byTerm[cand.TermIndex] = make(map[string][]taxonomyCandidate, len(tables))
		}
		byTerm[cand.TermIndex][cand.Ref.Table] = append(byTerm[cand.TermIndex][cand.Ref.Table], cand)
	}

	entryCount := 0
	results := make([]taxonomyTermResult, 0, len(terms))
	for i, term := range terms {
		tableResults := make([]taxonomyTableResult, 0, len(tables))
		for _, table := range tables {
			clusters := clusterTaxonomyCandidates(byTerm[i+1][table])
			if len(clusters) > taxonomySearchMaxClusters {
				clusters = clusters[:taxonomySearchMaxClusters]
			}
			for _, cluster := range clusters {
				entryCount += 1 + len(cluster.Variants)
			}
			tableResults = append(tableResults, taxonomyTableResult{
				Table:        table,
				ClusterCount: len(clusters),
				Clusters:     clusters,
			})
		}
		results = append(results, taxonomyTermResult{Term: term, Tables: tableResults})
	}

	return taxonomySearchEnvelope{
		Ok:         true,
		Input:      echo,
		EntryCount: entryCount,
		Results:    results,
		Errors:     []actionError{},
	}
}

// validateTaxonomySearch normalizes the request and reports every violation at
// once. It returns the resolved terms and tables even on failure so the echo
// still shows what the server understood.
func validateTaxonomySearch(req taxonomySearchRequest) (terms, tables []string, errs []actionError) {
	terms = make([]string, 0, len(req.Terms))
	for i, raw := range req.Terms {
		term := strings.TrimSpace(raw)
		switch {
		case term == "":
			errs = append(errs, actionError{
				Path: fmt.Sprintf("terms[%d]", i), Code: codeInvalidTerms,
				Message: "term must not be blank",
			})
		case len(term) > taxonomySearchMaxTermLength:
			errs = append(errs, actionError{
				Path: fmt.Sprintf("terms[%d]", i), Code: codeInvalidTerms,
				Message: fmt.Sprintf("term must be at most %d characters", taxonomySearchMaxTermLength),
			})
		default:
			terms = append(terms, term)
		}
	}
	switch {
	case len(req.Terms) == 0:
		errs = append(errs, actionError{
			Path: "terms", Code: codeInvalidTerms,
			Message: "terms is required and must contain at least one candidate term",
		})
	case len(req.Terms) > taxonomySearchMaxTerms:
		errs = append(errs, actionError{
			Path: "terms", Code: codeInvalidTerms,
			Message: fmt.Sprintf("terms must contain at most %d terms", taxonomySearchMaxTerms),
		})
	}

	tables = resolveTaxonomySearchTables(req.Tables, &errs)
	return terms, tables, errs
}

// resolveTaxonomySearchTables maps the requested tables onto the closed set,
// dropping duplicates and reporting unknown names. An omitted list means all three.
func resolveTaxonomySearchTables(requested []string, errs *[]actionError) []string {
	if len(requested) == 0 {
		return append([]string(nil), taxonomySearchTables...)
	}

	wanted := make(map[string]bool, len(requested))
	for i, raw := range requested {
		table := strings.TrimSpace(strings.ToLower(raw))
		if !taxonomySearchTableAllowed(table) {
			*errs = append(*errs, actionError{
				Path: fmt.Sprintf("tables[%d]", i), Code: codeUnknownTable,
				Message: fmt.Sprintf("table must be one of %s", strings.Join(taxonomySearchTables, ", ")),
			})
			continue
		}
		wanted[table] = true
	}

	// Report in the canonical order rather than the caller's, so two callers
	// asking for the same set get the same response shape.
	resolved := make([]string, 0, len(wanted))
	for _, table := range taxonomySearchTables {
		if wanted[table] {
			resolved = append(resolved, table)
		}
	}
	return resolved
}

func taxonomySearchTableAllowed(table string) bool {
	for _, allowed := range taxonomySearchTables {
		if table == allowed {
			return true
		}
	}
	return false
}

// clusterTaxonomyCandidates groups one term's candidates within one table into
// concepts.
//
// The algorithm is leader clustering, not single-link agglomeration, and that
// choice is load-bearing. Single-link chains through hubs: in the live skills
// table the slug `architecture` sits above the threshold against a dozen other
// "* Architecture" slugs, so transitive merging collapses most of an
// architecture search into one meaningless blob. Leader clustering instead walks
// candidates best-match-first; each unassigned candidate opens a cluster and
// claims only the still-unassigned candidates similar to that leader itself.
// Membership is therefore always "similar to the thing you were shown", which is
// the claim the nested response makes.
//
// Candidates arrive with their above-threshold neighbours already computed by
// the same query that scored them.
func clusterTaxonomyCandidates(candidates []taxonomyCandidate) []taxonomyCluster {
	if len(candidates) == 0 {
		return []taxonomyCluster{}
	}

	ordered := append([]taxonomyCandidate(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return lessTaxonomyCandidate(ordered[i], ordered[j])
	})

	assigned := make(map[taxonomyRef]bool, len(ordered))
	clusters := make([]taxonomyCluster, 0, len(ordered))

	for i, leader := range ordered {
		if assigned[leader.Ref] {
			continue
		}
		assigned[leader.Ref] = true

		members := []taxonomyCandidate{leader}
		neighbors := make(map[taxonomyRef]bool, len(leader.Neighbors))
		for _, ref := range leader.Neighbors {
			neighbors[ref] = true
		}
		for _, other := range ordered[i+1:] {
			if assigned[other.Ref] || !neighbors[other.Ref] {
				continue
			}
			assigned[other.Ref] = true
			members = append(members, other)
		}

		clusters = append(clusters, buildTaxonomyCluster(members))
	}

	return clusters
}

// lessTaxonomyCandidate is the deterministic candidate order: best match first,
// then established vocabulary, then the older row. Id ascending is the age proxy
// — the taxonomy tables use bigserial, and a cluster never spans tables, so
// comparing ids inside one is meaningful.
func lessTaxonomyCandidate(a, b taxonomyCandidate) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.UsageCount != b.UsageCount {
		return a.UsageCount > b.UsageCount
	}
	return a.Ref.ID < b.Ref.ID
}

// buildTaxonomyCluster picks the representative and nests the rest beneath it.
// members[0] is the leader, the best match to the term.
//
// The representative is the most-used member among those scoring within
// taxonomySearchRepresentativeBand of the leader. Usage is what "established
// vocabulary" means, but only among members that match the term about as well —
// otherwise a heavily attached loose match would front a cluster it does not
// describe.
func buildTaxonomyCluster(members []taxonomyCandidate) taxonomyCluster {
	best := members[0].Score
	representative := 0
	for i, member := range members {
		if member.Score < best-taxonomySearchRepresentativeBand {
			continue
		}
		if lessTaxonomyRepresentative(member, members[representative]) {
			representative = i
		}
	}

	variants := make([]taxonomyEntry, 0, len(members)-1)
	for i, member := range members {
		if i == representative {
			continue
		}
		if len(variants) >= taxonomySearchMaxVariants {
			break
		}
		variants = append(variants, taxonomyEntryOf(member))
	}

	return taxonomyCluster{
		Score:          best,
		Representative: taxonomyEntryOf(members[representative]),
		VariantCount:   len(members) - 1,
		Variants:       variants,
	}
}

// lessTaxonomyRepresentative reports whether a is the better representative of
// the two: more used, then older.
func lessTaxonomyRepresentative(a, b taxonomyCandidate) bool {
	if a.UsageCount != b.UsageCount {
		return a.UsageCount > b.UsageCount
	}
	return a.Ref.ID < b.Ref.ID
}

func taxonomyEntryOf(c taxonomyCandidate) taxonomyEntry {
	return taxonomyEntry{
		Table:      c.Ref.Table,
		Slug:       c.Slug,
		Name:       c.Name,
		UsageCount: c.UsageCount,
		Score:      c.Score,
	}
}

func taxonomySearchFailure(echo taxonomySearchEcho, errs []actionError) taxonomySearchEnvelope {
	return taxonomySearchEnvelope{
		Ok:      false,
		Input:   echo,
		Results: []taxonomyTermResult{},
		Errors:  errs,
	}
}
