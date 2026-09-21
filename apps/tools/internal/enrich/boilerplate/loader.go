package boilerplate

import (
	"context"
	"errors"
	"fmt"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

// CorpusPosting is one latest description in the company-wide corpus used to
// find repeated boilerplate. It intentionally contains no data from other
// companies.
type CorpusPosting struct {
	PostingID       int64
	DescriptionText string
}

// Loader is the narrow read-only seam shared by the CLI preprocessor and the
// MCP tool. It loads every latest description for the requested company and
// enough posting ownership data to reject invalid selected ids.
type Loader interface {
	LoadCompanyCorpus(ctx context.Context, companyID int64) ([]CorpusPosting, error)
	FindPostingCompanies(ctx context.Context, postingIDs []int64) (map[int64]int64, error)
}

type descriptionQuerier interface {
	ListLatestDescriptionsByCompany(ctx context.Context, companyID int64) ([]db.ListLatestDescriptionsByCompanyRow, error)
	ListPostingCompaniesByIDs(ctx context.Context, postingIDs []int64) ([]db.ListPostingCompaniesByIDsRow, error)
}

type dbLoader struct {
	queries descriptionQuerier
}

// NewDBLoader adapts sqlc's read-only description queries to Loader. Callers
// supply the pool-bound db.Queries, keeping database handles outside this
// package's cleaning operation.
func NewDBLoader(queries descriptionQuerier) Loader {
	return dbLoader{queries: queries}
}

func (l dbLoader) LoadCompanyCorpus(ctx context.Context, companyID int64) ([]CorpusPosting, error) {
	rows, err := l.queries.ListLatestDescriptionsByCompany(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("loading descriptions for company %d: %w", companyID, err)
	}

	corpus := make([]CorpusPosting, 0, len(rows))
	for _, row := range rows {
		corpus = append(corpus, CorpusPosting{
			PostingID:       row.JobPostingID,
			DescriptionText: row.DescriptionText,
		})
	}
	return corpus, nil
}

func (l dbLoader) FindPostingCompanies(ctx context.Context, postingIDs []int64) (map[int64]int64, error) {
	rows, err := l.queries.ListPostingCompaniesByIDs(ctx, postingIDs)
	if err != nil {
		return nil, fmt.Errorf("loading selected posting ownership: %w", err)
	}

	companies := make(map[int64]int64, len(rows))
	for _, row := range rows {
		companies[row.PostingID] = row.CompanyID
	}
	return companies, nil
}

// SelectionIssueCode is a stable, machine-readable reason a selected posting
// cannot safely be cleaned for the requested company.
type SelectionIssueCode string

const (
	CodeDuplicateSelectedID      SelectionIssueCode = "duplicate_selected_id"
	CodeUnknownPostingID         SelectionIssueCode = "unknown_posting_id"
	CodeCrossCompanyPostingID    SelectionIssueCode = "cross_company_posting_id"
	CodeMissingLatestDescription SelectionIssueCode = "missing_latest_description"
)

// SelectionIssue identifies one invalid selected posting id. The MCP adapter
// turns these into its ordinary structured errors[] response shape.
type SelectionIssue struct {
	Index     int
	PostingID int64
	Code      SelectionIssueCode
	Message   string
}

// InvalidSelectionError groups all invalid selected ids so callers can report
// every repairable input error in one response.
type InvalidSelectionError struct {
	Issues []SelectionIssue
}

func (e *InvalidSelectionError) Error() string {
	return "invalid selected posting ids"
}

// CleanSelected derives boilerplate evidence from the complete latest
// description corpus for companyID and returns only the requested posting ids,
// preserving their input order. It validates ownership before exposing text, so
// a caller cannot use a company corpus to clean another company's posting.
func CleanSelected(ctx context.Context, loader Loader, companyID int64, selectedIDs []int64) ([]CorpusPosting, error) {
	if companyID == 0 {
		return nil, errors.New("company_id is required and must be non-zero")
	}

	issues := duplicateIssues(selectedIDs)

	companies, err := loader.FindPostingCompanies(ctx, selectedIDs)
	if err != nil {
		return nil, err
	}
	for index, id := range selectedIDs {
		owner, ok := companies[id]
		switch {
		case !ok:
			issues = append(issues, SelectionIssue{Index: index, PostingID: id, Code: CodeUnknownPostingID, Message: "posting_id does not exist"})
		case owner != companyID:
			issues = append(issues, SelectionIssue{Index: index, PostingID: id, Code: CodeCrossCompanyPostingID, Message: "posting_id belongs to a different company"})
		}
	}
	if len(issues) > 0 {
		return nil, &InvalidSelectionError{Issues: issues}
	}

	corpus, err := loader.LoadCompanyCorpus(ctx, companyID)
	if err != nil {
		return nil, err
	}
	descriptions := make([]string, len(corpus))
	corpusByID := make(map[int64]int, len(corpus))
	for i, posting := range corpus {
		descriptions[i] = posting.DescriptionText
		corpusByID[posting.PostingID] = i
	}
	for index, id := range selectedIDs {
		if _, ok := corpusByID[id]; !ok {
			issues = append(issues, SelectionIssue{Index: index, PostingID: id, Code: CodeMissingLatestDescription, Message: "posting_id has no latest description for this company"})
		}
	}
	if len(issues) > 0 {
		return nil, &InvalidSelectionError{Issues: issues}
	}

	cleaned := Strip(descriptions)
	if len(cleaned) != len(corpus) {
		return nil, fmt.Errorf("boilerplate.Strip returned %d entries for %d inputs", len(cleaned), len(corpus))
	}

	result := make([]CorpusPosting, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		posting := corpus[corpusByID[id]]
		posting.DescriptionText = cleaned[corpusByID[id]]
		result = append(result, posting)
	}
	return result, nil
}

func duplicateIssues(selectedIDs []int64) []SelectionIssue {
	seen := make(map[int64]struct{}, len(selectedIDs))
	var issues []SelectionIssue
	for index, id := range selectedIDs {
		if _, exists := seen[id]; exists {
			issues = append(issues, SelectionIssue{Index: index, PostingID: id, Code: CodeDuplicateSelectedID, Message: "posting_id appears more than once"})
			continue
		}
		seen[id] = struct{}{}
	}
	return issues
}
