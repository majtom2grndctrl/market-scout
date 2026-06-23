// save_enrichment is the MCP action tool that persists an agent-supplied
// classification through the approved mcp.save_enrichment SECURITY DEFINER
// function, run by the locked-down action role. The agent sends a
// classifier-shaped payload plus MCP-only provenance — never SQL. The server
// validates the payload with the shared rules (internal/enrich/classify) using
// the read-only pool to load the taxonomy and confirm the posting exists, then
// calls the fixed parameterized function against the action pool. Validation
// and DB failures return ok=false in the JSON envelope, never a transport error.
//
// Classifications are append-only: the function INSERTs one new row per call and
// never updates or deletes prior history.
// See: agent-context/lib/developer-guide.md §5.7 (Database access), §6.2.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/classify"
)

// Provenance defaults applied before validation. Both fields must match
// provenancePattern after defaulting.
const (
	defaultModel         = "mcp-agent"
	defaultPromptVersion = "mcp-save-enrichment-v1"
)

// Save-enrichment action error codes beyond the validation codes the shared
// classifier rules emit. invalid_provenance and posting_not_found are checked
// here; codeDBError (add_company.go) covers unexpected DB faults.
const (
	codeInvalidProvenance = "invalid_provenance"
	codePostingNotFound   = "posting_not_found"
)

// provenancePattern constrains model and prompt_version to a stable identifier
// shape so provenance values are safe audit keys.
var provenancePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// saveEnrichmentRequest is the MCP tool DTO. It wraps the classifier
// AgentResponse shape and adds MCP-only provenance. JSON keys are the wire
// contract with the agent. skills[].requirement is accepted and echoed but
// stripped before persistence — no storage column exists. summary is echoed but
// not persisted.
type saveEnrichmentRequest struct {
	PostingID       int64                          `json:"posting_id"`
	Provenance      provenanceInput                `json:"provenance"`
	Classification  classify.AgentClassification   `json:"classification"`
	CanonicalRoles  []classify.AgentCanonicalRole  `json:"canonical_roles"`
	Specializations []classify.AgentSpecialization `json:"specializations"`
	Skills          []classify.AgentSkill          `json:"skills"`
	Summary         string                         `json:"summary"`
}

// provenanceInput carries the MCP-only model/prompt_version. Both default when
// omitted; the defaults are applied before pattern validation.
type provenanceInput struct {
	Model         string `json:"model"`
	PromptVersion string `json:"prompt_version"`
}

// newTaxonomyEntry mirrors one freshly-minted taxonomy row the function reports.
type newTaxonomyEntry struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// newTaxonomy groups the freshly-minted taxonomy by table, echoing the function's
// new_taxonomy shape. Always non-nil slices so the JSON has explicit empty arrays.
type newTaxonomy struct {
	CanonicalRoles  []newTaxonomyEntry `json:"canonical_roles"`
	Specializations []newTaxonomyEntry `json:"specializations"`
	Skills          []newTaxonomyEntry `json:"skills"`
}

// saveEnrichmentEnvelope is the JSON the tool returns. Validation and DB failures
// set Ok=false here — they are not MCP transport errors. Summary is echoed from
// the request, never persisted. NewTaxonomy carries only entries this call minted.
type saveEnrichmentEnvelope struct {
	Ok               bool          `json:"ok"`
	ClassificationID *int64        `json:"classification_id"`
	PostingID        int64         `json:"posting_id"`
	Summary          string        `json:"summary"`
	NewTaxonomy      newTaxonomy   `json:"new_taxonomy"`
	Errors           []actionError `json:"errors"`
}

// functionResult is the JSON envelope mcp.save_enrichment returns. ok=false
// carries structured errors[]; ok=true carries the ids and new_taxonomy.
type functionResult struct {
	Ok               bool        `json:"ok"`
	ClassificationID *int64      `json:"classification_id"`
	PostingID        *int64      `json:"posting_id"`
	NewTaxonomy      newTaxonomy `json:"new_taxonomy"`
	Errors           []struct {
		Path    string `json:"path"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// enrichmentSaver runs the approved mcp.save_enrichment function and returns the
// raw JSON envelope. Injectable so tests can supply canned envelopes without a
// database. The production implementation binds the action pool.
type enrichmentSaver interface {
	save(ctx context.Context, payload json.RawMessage, model, promptVersion string) (json.RawMessage, error)
}

// taxonomySource loads the taxonomy snapshot and confirms a posting exists.
// Injectable so tests can validate without a database. The production
// implementation binds the read-only pool.
type taxonomySource interface {
	load(ctx context.Context) (classify.Taxonomy, error)
	postingExists(ctx context.Context, id int64) (bool, error)
}

// poolEnrichmentSaver calls mcp.save_enrichment via the sqlc-generated query.
// sqlc codegen runs offline: a function's return type must be resolvable without
// a live database. mcp.save_enrichment returns a scalar jsonb, so sqlc can
// generate it offline. mcp.add_company uses RETURNS TABLE, which sqlc cannot
// expand offline, so that call is a raw parameterized statement instead (see
// add_company.go). The statement here is constant and fully parameterized.
type poolEnrichmentSaver struct {
	pool *sql.DB
}

func (s poolEnrichmentSaver) save(ctx context.Context, payload json.RawMessage, model, promptVersion string) (json.RawMessage, error) {
	queryCtx, cancel := context.WithTimeout(ctx, statementTimeout)
	defer cancel()

	result, err := db.New(s.pool).SaveEnrichment(queryCtx, db.SaveEnrichmentParams{
		PPayload:       payload,
		PModel:         model,
		PPromptVersion: promptVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("calling mcp.save_enrichment: %w", err)
	}
	return result, nil
}

// poolTaxonomySource loads taxonomy and checks posting existence against the
// read-only pool.
type poolTaxonomySource struct {
	pool *sql.DB
}

func (s poolTaxonomySource) load(ctx context.Context) (classify.Taxonomy, error) {
	queryCtx, cancel := context.WithTimeout(ctx, statementTimeout)
	defer cancel()
	return classify.LoadTaxonomy(queryCtx, db.New(s.pool))
}

func (s poolTaxonomySource) postingExists(ctx context.Context, id int64) (bool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, statementTimeout)
	defer cancel()
	return db.New(s.pool).PostingExists(queryCtx, id)
}

// saveEnrichmentHandler wires the tool to its two pools: the read-only pool for
// pre-validation reads (taxonomy + posting existence) and the action pool for the
// approved write. The action pool is the only handle that can perform the write.
func saveEnrichmentHandler(roPool, actionPool *sql.DB) server.ToolHandlerFunc {
	return saveEnrichmentHandlerWithDeps(poolTaxonomySource{pool: roPool}, poolEnrichmentSaver{pool: actionPool})
}

// saveEnrichmentHandlerWithDeps is the testable handler body. Tests inject a fake
// taxonomy source and saver; production wiring passes the pools via
// saveEnrichmentHandler.
func saveEnrichmentHandlerWithDeps(tax taxonomySource, saver enrichmentSaver) server.ToolHandlerFunc {
	return func(ctx context.Context, mcpReq mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req saveEnrichmentRequest
		if err := mcpReq.BindArguments(&req); err != nil {
			// A request the server cannot decode is a genuinely malformed tool
			// call — the one case that warrants an MCP transport error.
			return mcp.NewToolResultError(fmt.Sprintf("decoding save_enrichment arguments: %v", err)), nil
		}

		env := runSaveEnrichment(ctx, req, tax, saver)
		payload, err := json.Marshal(env)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encoding save_enrichment result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(payload)), nil
	}
}

// runSaveEnrichment applies provenance defaults, validates provenance and the
// classifier payload (loading taxonomy and confirming the posting via the
// read-only source), then calls the approved function through the saver. Every
// failure mode returns an ok=false envelope; it never returns a transport error.
func runSaveEnrichment(ctx context.Context, req saveEnrichmentRequest, tax taxonomySource, saver enrichmentSaver) saveEnrichmentEnvelope {
	model := defaultIfBlank(req.Provenance.Model, defaultModel)
	promptVersion := defaultIfBlank(req.Provenance.PromptVersion, defaultPromptVersion)

	var errs []actionError
	if !provenancePattern.MatchString(model) {
		errs = append(errs, actionError{Path: "provenance.model", Code: codeInvalidProvenance,
			Message: "provenance.model must match ^[A-Za-z0-9._-]+$"})
	}
	if !provenancePattern.MatchString(promptVersion) {
		errs = append(errs, actionError{Path: "provenance.prompt_version", Code: codeInvalidProvenance,
			Message: "provenance.prompt_version must match ^[A-Za-z0-9._-]+$"})
	}

	// Load taxonomy for the shared validation. A load failure is a DB fault, not
	// a validation rejection — surface it as db_error.
	taxonomy, err := tax.load(ctx)
	if err != nil {
		return failureSaveEnvelope(req, append(errs, actionError{Path: "db", Code: codeDBError, Message: err.Error()}))
	}

	resp := classify.AgentResponse{
		PostingID:       req.PostingID,
		Classification:  req.Classification,
		CanonicalRoles:  req.CanonicalRoles,
		Specializations: req.Specializations,
		Skills:          req.Skills,
		Summary:         req.Summary,
	}
	for _, f := range classify.Validate(resp, taxonomy) {
		errs = append(errs, actionError{Path: f.Path, Code: string(f.Code), Message: f.Message})
	}

	// Posting existence is a DB-backed validation: confirm it before the write.
	exists, err := tax.postingExists(ctx, req.PostingID)
	if err != nil {
		return failureSaveEnvelope(req, append(errs, actionError{Path: "db", Code: codeDBError, Message: err.Error()}))
	}
	if !exists {
		errs = append(errs, actionError{Path: "posting_id", Code: codePostingNotFound,
			Message: fmt.Sprintf("job posting %d does not exist", req.PostingID)})
	}

	if len(errs) > 0 {
		return failureSaveEnvelope(req, errs)
	}

	// Build the function payload: classifier shape only. Provenance travels as
	// separate function arguments; skills[].requirement is stripped here.
	payload, err := buildPayload(req)
	if err != nil {
		return failureSaveEnvelope(req, []actionError{{Path: "payload", Code: codeDBError, Message: err.Error()}})
	}

	raw, err := saver.save(ctx, payload, model, promptVersion)
	if err != nil {
		return failureSaveEnvelope(req, []actionError{{Path: "db", Code: codeDBError, Message: err.Error()}})
	}

	var fr functionResult
	if err := json.Unmarshal(raw, &fr); err != nil {
		return failureSaveEnvelope(req, []actionError{{Path: "db", Code: codeDBError,
			Message: fmt.Sprintf("decoding save_enrichment result: %v", err)}})
	}

	if !fr.Ok {
		// SQL-level invariant violations come back as structured errors, not as a
		// raised exception, so they map straight into the envelope.
		mapped := make([]actionError, 0, len(fr.Errors))
		for _, e := range fr.Errors {
			mapped = append(mapped, actionError{Path: e.Path, Code: e.Code, Message: e.Message})
		}
		return failureSaveEnvelope(req, mapped)
	}

	return saveEnrichmentEnvelope{
		Ok:               true,
		ClassificationID: fr.ClassificationID,
		PostingID:        req.PostingID,
		Summary:          req.Summary,
		NewTaxonomy:      normalizeNewTaxonomy(fr.NewTaxonomy),
		Errors:           []actionError{},
	}
}

// buildPayload marshals the exact classifier payload the function expects:
// posting_id, classification, and the three minting arrays with slug/name (plus
// dimensions for roles). skills[].requirement is dropped; provenance and summary
// are not included.
func buildPayload(req saveEnrichmentRequest) (json.RawMessage, error) {
	type payloadRole struct {
		Slug       string   `json:"slug"`
		Name       string   `json:"name"`
		Dimensions []string `json:"dimensions"`
	}
	type payloadNamed struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	type payloadClassification struct {
		Seniority string  `json:"seniority"`
		Notes     *string `json:"notes"`
	}
	type payload struct {
		PostingID       int64                 `json:"posting_id"`
		Classification  payloadClassification `json:"classification"`
		CanonicalRoles  []payloadRole         `json:"canonical_roles"`
		Specializations []payloadNamed        `json:"specializations"`
		Skills          []payloadNamed        `json:"skills"`
	}

	roles := make([]payloadRole, 0, len(req.CanonicalRoles))
	for _, r := range req.CanonicalRoles {
		dims := r.Dimensions
		if dims == nil {
			dims = []string{}
		}
		roles = append(roles, payloadRole{Slug: r.Slug, Name: r.Name, Dimensions: dims})
	}
	specs := make([]payloadNamed, 0, len(req.Specializations))
	for _, s := range req.Specializations {
		specs = append(specs, payloadNamed{Slug: s.Slug, Name: s.Name})
	}
	skills := make([]payloadNamed, 0, len(req.Skills))
	for _, s := range req.Skills {
		skills = append(skills, payloadNamed{Slug: s.Slug, Name: s.Name})
	}

	var notes *string
	if trimmed := strings.TrimSpace(req.Classification.Notes); trimmed != "" {
		notes = &req.Classification.Notes
	}

	p := payload{
		PostingID:       req.PostingID,
		Classification:  payloadClassification{Seniority: req.Classification.Seniority, Notes: notes},
		CanonicalRoles:  roles,
		Specializations: specs,
		Skills:          skills,
	}
	return json.Marshal(p)
}

// normalizeNewTaxonomy guarantees non-nil slices so the response always carries
// explicit empty arrays rather than JSON null.
func normalizeNewTaxonomy(nt newTaxonomy) newTaxonomy {
	if nt.CanonicalRoles == nil {
		nt.CanonicalRoles = []newTaxonomyEntry{}
	}
	if nt.Specializations == nil {
		nt.Specializations = []newTaxonomyEntry{}
	}
	if nt.Skills == nil {
		nt.Skills = []newTaxonomyEntry{}
	}
	return nt
}

// failureSaveEnvelope builds the ok=false envelope. Summary is still echoed so the
// agent can correlate the rejected call; new_taxonomy is empty.
func failureSaveEnvelope(req saveEnrichmentRequest, errs []actionError) saveEnrichmentEnvelope {
	if errs == nil {
		errs = []actionError{}
	}
	return saveEnrichmentEnvelope{
		Ok:               false,
		ClassificationID: nil,
		PostingID:        req.PostingID,
		Summary:          req.Summary,
		NewTaxonomy:      normalizeNewTaxonomy(newTaxonomy{}),
		Errors:           errs,
	}
}

// defaultIfBlank returns fallback when s is empty or whitespace-only, else s
// trimmed. Provenance is trimmed so a stray space does not defeat the pattern.
func defaultIfBlank(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return strings.TrimSpace(s)
}
