package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/atsdetect"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

const (
	dedupDefaultRecencyDays = 30
	dedupMaxCandidates      = 200
	// Short company names make trigram matches noisy, so use a stricter threshold
	// than pg_trgm's 0.3 default.
	dedupFuzzyNameSimilarityThreshold = 0.4

	dedupVerdictNew       = "new"
	dedupVerdictDuplicate = "duplicate"
	dedupVerdictStale     = "stale"
	dedupVerdictInvalid   = "invalid"

	dedupMatchKindNone      = "none"
	dedupMatchKindToken     = "token"
	dedupMatchKindNameOnly  = "name_only"
	dedupMatchKindDomain    = "domain"
	dedupMatchKindFuzzyName = "fuzzy_name"

	dedupReasonNoMatch              = "no_match"
	dedupReasonMatchedByTokenRecent = "matched_by_token_with_recent_snapshot"
	dedupReasonMatchedByTokenStale  = "matched_by_token_without_recent_snapshot"
	dedupReasonMatchedByNameOnly    = "matched_by_name_only"
	dedupReasonMatchedByDomain      = "matched_by_domain"
	dedupReasonMatchedByFuzzyName   = "matched_by_fuzzy_name"
	dedupReasonMissingRequiredName  = "missing_required_name"
	codeInvalidRecencyDays          = "invalid_recency_days"
	codeTooManyCandidates           = "too_many_candidates"
)

type dedupCandidatesRequest struct {
	Candidates  []dedupCandidateInput `json:"candidates"`
	RecencyDays *int                  `json:"recency_days"`
}

type dedupCandidateInput struct {
	Name       string `json:"name"`
	ATS        string `json:"ats"`
	BoardToken string `json:"board_token"`
	CareersURL string `json:"careers_url"`
}

type dedupCandidatesEnvelope struct {
	Ok      bool                   `json:"ok"`
	Results []dedupCandidateResult `json:"results"`
	Errors  []actionError          `json:"errors"`
}

type dedupCandidateResult struct {
	Name       string                `json:"name"`
	ATS        string                `json:"ats"`
	BoardToken string                `json:"board_token"`
	Verdict    string                `json:"verdict"`
	MatchKind  string                `json:"match_kind"`
	Reason     string                `json:"reason"`
	Matched    *dedupMatchedCompany  `json:"matched"`
	MatchCount int                   `json:"match_count"`
	Matches    []dedupMatchedCompany `json:"matches"`
	Error      *actionError          `json:"error"`
}

type dedupMatchedCompany struct {
	ID                int64    `json:"id"`
	Name              string   `json:"name"`
	ATS               string   `json:"ats"`
	BoardToken        string   `json:"board_token"`
	Industry          *string  `json:"industry"`
	CareersPageURL    *string  `json:"careers_page_url"`
	HasRecentSnapshot bool     `json:"has_recent_snapshot"`
	MatchKind         string   `json:"match_kind"`
	SimilarityScore   *float64 `json:"similarity_score"`
}

type dedupSource interface {
	FindByToken(ctx context.Context, ats, boardToken string, recencyDays int32) (*dedupMatchedCompany, error)
	FindByNames(ctx context.Context, names []string, recencyDays int32) (map[int][]dedupMatchedCompany, error)
	FindByCareersURLHost(ctx context.Context, careersURLs []string, recencyDays int32) (map[int][]dedupMatchedCompany, error)
	FindByNameSimilarity(ctx context.Context, names []string, recencyDays int32) (map[int][]dedupMatchedCompany, error)
}

func (s poolDedupSource) FindByCareersURLHost(ctx context.Context, careersURLs []string, recencyDays int32) (map[int][]dedupMatchedCompany, error) {
	queryCtx, cancel := context.WithTimeout(ctx, statementTimeout)
	defer cancel()

	rows, err := db.New(s.pool).FindCompaniesByCareersURLHost(queryCtx, db.FindCompaniesByCareersURLHostParams{
		RecencyDays:   recencyDays,
		CandidateUrls: careersURLs,
	})
	if err != nil {
		return nil, err
	}

	matches := make(map[int][]dedupMatchedCompany, len(rows))
	for _, row := range rows {
		inputIndex := int(row.InputIndex) - 1
		matches[inputIndex] = append(matches[inputIndex], dedupMatchedCompany{
			ID:                row.CompanyID,
			Name:              row.Name,
			ATS:               row.Ats,
			BoardToken:        row.BoardToken,
			Industry:          nullStringPtr(row.Industry),
			CareersPageURL:    nullStringPtr(row.CareersPageUrl),
			HasRecentSnapshot: row.HasRecentSnapshot,
		})
	}
	return matches, nil
}

func (s poolDedupSource) FindByNameSimilarity(ctx context.Context, names []string, recencyDays int32) (map[int][]dedupMatchedCompany, error) {
	queryCtx, cancel := context.WithTimeout(ctx, statementTimeout)
	defer cancel()

	rows, err := db.New(s.pool).FindCompaniesByNameSimilarity(queryCtx, db.FindCompaniesByNameSimilarityParams{
		RecencyDays:         recencyDays,
		CandidateNames:      names,
		SimilarityThreshold: dedupFuzzyNameSimilarityThreshold,
	})
	if err != nil {
		return nil, err
	}

	matches := make(map[int][]dedupMatchedCompany, len(rows))
	for _, row := range rows {
		inputIndex := int(row.InputIndex) - 1
		score := row.Score
		matches[inputIndex] = append(matches[inputIndex], dedupMatchedCompany{
			ID:                row.CompanyID,
			Name:              row.Name,
			ATS:               row.Ats,
			BoardToken:        row.BoardToken,
			Industry:          nullStringPtr(row.Industry),
			CareersPageURL:    nullStringPtr(row.CareersPageUrl),
			HasRecentSnapshot: row.HasRecentSnapshot,
			SimilarityScore:   &score,
		})
	}
	return matches, nil
}

type poolDedupSource struct {
	pool *sql.DB
}

func (s poolDedupSource) FindByToken(ctx context.Context, ats, boardToken string, recencyDays int32) (*dedupMatchedCompany, error) {
	queryCtx, cancel := context.WithTimeout(ctx, statementTimeout)
	defer cancel()

	row, err := db.New(s.pool).FindCompanyDedupStatus(queryCtx, db.FindCompanyDedupStatusParams{
		RecencyDays: recencyDays,
		Ats:         ats,
		BoardToken:  boardToken,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &dedupMatchedCompany{
		ID:                row.CompanyID,
		Name:              row.Name,
		ATS:               row.Ats,
		BoardToken:        row.BoardToken,
		Industry:          nullStringPtr(row.Industry),
		CareersPageURL:    nullStringPtr(row.CareersPageUrl),
		HasRecentSnapshot: row.HasRecentSnapshot,
	}, nil
}

func (s poolDedupSource) FindByNames(ctx context.Context, names []string, recencyDays int32) (map[int][]dedupMatchedCompany, error) {
	queryCtx, cancel := context.WithTimeout(ctx, statementTimeout)
	defer cancel()

	rows, err := db.New(s.pool).FindCompaniesByNormalizedNames(queryCtx, db.FindCompaniesByNormalizedNamesParams{
		RecencyDays:    recencyDays,
		CandidateNames: names,
	})
	if err != nil {
		return nil, err
	}

	matches := make(map[int][]dedupMatchedCompany, len(rows))
	for _, row := range rows {
		inputIndex := int(row.InputIndex) - 1
		matches[inputIndex] = append(matches[inputIndex], dedupMatchedCompany{
			ID:                row.CompanyID,
			Name:              row.Name,
			ATS:               row.Ats,
			BoardToken:        row.BoardToken,
			Industry:          nullStringPtr(row.Industry),
			CareersPageURL:    nullStringPtr(row.CareersPageUrl),
			HasRecentSnapshot: row.HasRecentSnapshot,
		})
	}
	return matches, nil
}

func dedupCandidatesHandler(pool *sql.DB) server.ToolHandlerFunc {
	return dedupCandidatesHandlerWithDeps(poolDedupSource{pool: pool})
}

func dedupCandidatesHandlerWithDeps(source dedupSource) server.ToolHandlerFunc {
	return func(ctx context.Context, mcpReq mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req dedupCandidatesRequest
		if err := mcpReq.BindArguments(&req); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("decoding dedup_candidates arguments: %v", err)), nil
		}

		env := runDedupCandidates(ctx, req, source)
		payload, err := json.Marshal(env)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encoding dedup_candidates result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(payload)), nil
	}
}

func runDedupCandidates(ctx context.Context, req dedupCandidatesRequest, source dedupSource) dedupCandidatesEnvelope {
	if len(req.Candidates) > dedupMaxCandidates {
		return dedupCandidatesEnvelope{
			Ok:      false,
			Results: []dedupCandidateResult{},
			Errors: []actionError{{
				Path:    "candidates",
				Code:    codeTooManyCandidates,
				Message: fmt.Sprintf("candidates must include at most %d items", dedupMaxCandidates),
			}},
		}
	}

	recencyDays := dedupDefaultRecencyDays
	if req.RecencyDays != nil {
		recencyDays = *req.RecencyDays
	}
	if recencyDays <= 0 || recencyDays > maxInt32 {
		return dedupCandidatesEnvelope{
			Ok:      false,
			Results: []dedupCandidateResult{},
			Errors: []actionError{{
				Path:    "recency_days",
				Code:    codeInvalidRecencyDays,
				Message: "recency_days must be between 1 and 2147483647",
			}},
		}
	}

	results := make([]dedupCandidateResult, len(req.Candidates))
	var nameLookupNames []string
	var nameLookupResultIndexes []int
	var domainLookupURLs []string
	var domainLookupResultIndexes []int

	for i, candidate := range req.Candidates {
		name := strings.TrimSpace(candidate.Name)
		ats := strings.TrimSpace(candidate.ATS)
		boardToken := strings.TrimSpace(candidate.BoardToken)
		careersURL := strings.TrimSpace(candidate.CareersURL)
		if careersURL != "" && atsdetect.ValidateURL(careersURL) != nil {
			careersURL = ""
		}
		results[i] = dedupCandidateResult{
			Name:       name,
			ATS:        ats,
			BoardToken: boardToken,
			MatchKind:  dedupMatchKindNone,
			Matches:    []dedupMatchedCompany{},
		}

		if name == "" {
			err := actionError{
				Path:    fmt.Sprintf("candidates[%d].name", i),
				Code:    codeMissingRequired,
				Message: "name is required",
			}
			results[i].Verdict = dedupVerdictInvalid
			results[i].Reason = dedupReasonMissingRequiredName
			results[i].Error = &err
			continue
		}

		if ats != "" && boardToken != "" {
			match, err := source.FindByToken(ctx, ats, boardToken, int32(recencyDays))
			if err != nil {
				return dedupCandidatesDBFailure(err)
			}
			if match != nil {
				match.MatchKind = dedupMatchKindToken
				results[i].Matched = match
				results[i].MatchCount = 1
				results[i].Matches = []dedupMatchedCompany{*match}
				results[i].MatchKind = dedupMatchKindToken
				if match.HasRecentSnapshot {
					results[i].Verdict = dedupVerdictDuplicate
					results[i].Reason = dedupReasonMatchedByTokenRecent
				} else {
					results[i].Verdict = dedupVerdictStale
					results[i].Reason = dedupReasonMatchedByTokenStale
				}
				continue
			}
		}

		nameLookupResultIndexes = append(nameLookupResultIndexes, i)
		nameLookupNames = append(nameLookupNames, name)
		if careersURL != "" {
			domainLookupResultIndexes = append(domainLookupResultIndexes, i)
			domainLookupURLs = append(domainLookupURLs, careersURL)
		}
	}

	nameMatches := map[int][]dedupMatchedCompany{}
	if len(nameLookupNames) > 0 {
		var err error
		nameMatches, err = source.FindByNames(ctx, nameLookupNames, int32(recencyDays))
		if err != nil {
			return dedupCandidatesDBFailure(err)
		}
	}

	domainMatches := map[int][]dedupMatchedCompany{}
	if len(domainLookupURLs) > 0 {
		var err error
		domainMatches, err = source.FindByCareersURLHost(ctx, domainLookupURLs, int32(recencyDays))
		if err != nil {
			return dedupCandidatesDBFailure(err)
		}
	}

	matchesByResult := make(map[int][]dedupMatchedCompany, len(nameLookupResultIndexes))
	matchPositions := make(map[int]map[int64]int, len(nameLookupResultIndexes))
	for lookupIndex, resultIndex := range nameLookupResultIndexes {
		mergeDedupMatches(matchesByResult, matchPositions, resultIndex, nameMatches[lookupIndex], dedupMatchKindNameOnly)
	}
	for lookupIndex, resultIndex := range domainLookupResultIndexes {
		mergeDedupMatches(matchesByResult, matchPositions, resultIndex, domainMatches[lookupIndex], dedupMatchKindDomain)
	}

	var fuzzyLookupNames []string
	var fuzzyLookupResultIndexes []int
	for lookupIndex, resultIndex := range nameLookupResultIndexes {
		if len(matchesByResult[resultIndex]) == 0 {
			fuzzyLookupNames = append(fuzzyLookupNames, nameLookupNames[lookupIndex])
			fuzzyLookupResultIndexes = append(fuzzyLookupResultIndexes, resultIndex)
		}
	}

	if len(fuzzyLookupNames) > 0 {
		fuzzyMatches, err := source.FindByNameSimilarity(ctx, fuzzyLookupNames, int32(recencyDays))
		if err != nil {
			return dedupCandidatesDBFailure(err)
		}
		for lookupIndex, resultIndex := range fuzzyLookupResultIndexes {
			mergeDedupMatches(matchesByResult, matchPositions, resultIndex, fuzzyMatches[lookupIndex], dedupMatchKindFuzzyName)
		}
	}

	for _, resultIndex := range nameLookupResultIndexes {
		allMatches := matchesByResult[resultIndex]
		if len(allMatches) == 0 {
			results[resultIndex].Verdict = dedupVerdictNew
			results[resultIndex].Reason = dedupReasonNoMatch
			continue
		}

		results[resultIndex].Verdict = dedupVerdictStale
		results[resultIndex].MatchCount = len(allMatches)
		results[resultIndex].Matches = capDedupFuzzyMatches(allMatches, 3)
		results[resultIndex].MatchKind = strongestDedupMatchKind(allMatches)
		results[resultIndex].Reason = dedupReasonForMatchKind(results[resultIndex].MatchKind)
		for matchIndex := range results[resultIndex].Matches {
			if results[resultIndex].Matches[matchIndex].MatchKind == results[resultIndex].MatchKind {
				results[resultIndex].Matched = &results[resultIndex].Matches[matchIndex]
				break
			}
		}
	}

	return dedupCandidatesEnvelope{
		Ok:      true,
		Results: results,
		Errors:  []actionError{},
	}
}

func mergeDedupMatches(matchesByResult map[int][]dedupMatchedCompany, positionsByResult map[int]map[int64]int, resultIndex int, rows []dedupMatchedCompany, matchKind string) {
	positions := positionsByResult[resultIndex]
	if positions == nil {
		positions = make(map[int64]int)
		positionsByResult[resultIndex] = positions
	}
	for _, row := range rows {
		row.MatchKind = matchKind
		position, exists := positions[row.ID]
		if exists {
			if dedupMatchKindPriority(matchKind) > dedupMatchKindPriority(matchesByResult[resultIndex][position].MatchKind) {
				matchesByResult[resultIndex][position] = row
			}
			continue
		}
		positions[row.ID] = len(matchesByResult[resultIndex])
		matchesByResult[resultIndex] = append(matchesByResult[resultIndex], row)
	}
}

func capDedupFuzzyMatches(matches []dedupMatchedCompany, cap int) []dedupMatchedCompany {
	nonFuzzy := make([]dedupMatchedCompany, 0, len(matches))
	fuzzy := make([]dedupMatchedCompany, 0, len(matches))
	for _, match := range matches {
		if match.MatchKind == dedupMatchKindFuzzyName {
			fuzzy = append(fuzzy, match)
		} else {
			nonFuzzy = append(nonFuzzy, match)
		}
	}
	sort.SliceStable(fuzzy, func(i, j int) bool {
		if fuzzy[i].SimilarityScore == nil {
			return false
		}
		if fuzzy[j].SimilarityScore == nil {
			return true
		}
		return *fuzzy[i].SimilarityScore > *fuzzy[j].SimilarityScore
	})
	if len(fuzzy) > cap {
		fuzzy = fuzzy[:cap]
	}
	return append(nonFuzzy, fuzzy...)
}

func strongestDedupMatchKind(matches []dedupMatchedCompany) string {
	strongest := dedupMatchKindNone
	for _, match := range matches {
		if dedupMatchKindPriority(match.MatchKind) > dedupMatchKindPriority(strongest) {
			strongest = match.MatchKind
		}
	}
	return strongest
}

func dedupMatchKindPriority(matchKind string) int {
	switch matchKind {
	case dedupMatchKindToken:
		return 4
	case dedupMatchKindDomain:
		return 3
	case dedupMatchKindNameOnly:
		return 2
	case dedupMatchKindFuzzyName:
		return 1
	default:
		return 0
	}
}

func dedupReasonForMatchKind(matchKind string) string {
	switch matchKind {
	case dedupMatchKindDomain:
		return dedupReasonMatchedByDomain
	case dedupMatchKindNameOnly:
		return dedupReasonMatchedByNameOnly
	case dedupMatchKindFuzzyName:
		return dedupReasonMatchedByFuzzyName
	default:
		return dedupReasonNoMatch
	}
}

func dedupCandidatesDBFailure(err error) dedupCandidatesEnvelope {
	return dedupCandidatesEnvelope{
		Ok:      false,
		Results: []dedupCandidateResult{},
		Errors: []actionError{{
			Path:    "db",
			Code:    codeDBError,
			Message: err.Error(),
		}},
	}
}

const maxInt32 = 1<<31 - 1

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
