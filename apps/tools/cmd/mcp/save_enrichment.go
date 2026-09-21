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

// Save-enrichment action error codes beyond the validation codes the shared
// classifier rules emit. invalid_provenance and posting_not_found are checked
// here; codeDBError (add_company.go) covers unexpected DB faults.
// codeNameCollision is raised by the function, not here — it is named so the
// mapping below can attach that error's structured detail.
const (
	codeInvalidProvenance = "invalid_provenance"
	codePostingNotFound   = "posting_not_found"
	codeNameCollision     = "name_collision"
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

// provenanceInput carries the MCP-only model/prompt_version. Both are required
// and neither is defaulted; see the provenance comment above.
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

// substitution mirrors one entry in the function's substitutions[]: a proposal
// the near-duplicate gate (migration 000033, narrowed to the slug axis by
// 000034) silently remapped onto an existing taxonomy row because its slug
// cleared the substitution threshold against that row. Since 000034, Match is
// always "<axis>_similarity" (currently "slug_similarity" — 000034 pins the
// axis to slug and demoted the old exact-name trigger to an advisory-only
// signal inside similarity_candidates); the field stays a string here rather
// than a constant because the axis is a migration-owned knob, not a Go one.
//
// path indexes the payload AS SUBMITTED by the agent, not the gate's reshaped
// working copy — do not renumber it. That is what lets the agent correlate
// this report with the array element it sent.
type substitution struct {
	Path            string  `json:"path"`
	Table           string  `json:"table"`
	ProposedSlug    string  `json:"proposed_slug"`
	ProposedName    string  `json:"proposed_name"`
	SubstitutedSlug string  `json:"substituted_slug"`
	SubstitutedName string  `json:"substituted_name"`
	Match           string  `json:"match"`
	Similarity      float64 `json:"similarity"`
}

// droppedEntry mirrors one entry in the function's dropped[]: a near-duplicate
// entry the gate removed from the payload because another entry in the same
// array (post-substitution) already covered the same concept.
//
// path indexes the payload AS SUBMITTED, same caveat as substitution.
type droppedEntry struct {
	Path        string  `json:"path"`
	Table       string  `json:"table"`
	DroppedSlug string  `json:"dropped_slug"`
	DroppedName string  `json:"dropped_name"`
	KeptSlug    string  `json:"kept_slug"`
	Similarity  float64 `json:"similarity"`
	Reason      string  `json:"reason"`
}

// similarityMatch is one near-match reported for a minted slug inside
// similarityCandidateGroup.Candidates. Match is "exact_name" or "similarity" —
// since migration 000034 an exact normalized-name match no longer substitutes
// on its own, so it surfaces here instead, alongside SlugSimilarity (the axis
// that decides substitution) and Similarity (greatest(slug, name), the
// combined advisory score).
type similarityMatch struct {
	Slug           string  `json:"slug"`
	Name           string  `json:"name"`
	Match          string  `json:"match"`
	SlugSimilarity float64 `json:"slug_similarity"`
	Similarity     float64 `json:"similarity"`
}

// similarityCandidateGroup mirrors one entry in the function's
// similarity_candidates[]: a proposal that still minted a new taxonomy row, but
// whose near matches are reported advisory-only — nothing was blocked or
// changed.
//
// path indexes the payload AS SUBMITTED, same caveat as substitution.
type similarityCandidateGroup struct {
	Path       string            `json:"path"`
	Table      string            `json:"table"`
	MintedSlug string            `json:"minted_slug"`
	MintedName string            `json:"minted_name"`
	Candidates []similarityMatch `json:"candidates"`
}

// saveEnrichmentEnvelope is the JSON the tool returns. Validation and DB failures
// set Ok=false here — they are not MCP transport errors. Summary is echoed from
// the request, never persisted. NewTaxonomy carries only entries this call minted.
//
// Substitutions, Dropped, and SimilarityCandidates are the migration 000033
// near-duplicate gate's feedback. Each is omitted entirely (not an empty array)
// when the gate did not touch the payload, mirroring the function's own
// omit-when-empty contract — so an untouched save's envelope is byte-identical
// to what it was before this gate existed.
// nameCollisionDetail is the machine-readable half of a name_collision error
// (migration 000038): the existing row a mint would have duplicated, and both
// similarity axes scored against it. The message states the same thing in
// prose; this is what a worker branches on. slug_similarity is the number that
// distinguishes the two correct responses — a low score means the vocabulary
// already spells the concept under a different slug, which is a reuse, not a
// licence to mint.
type nameCollisionDetail struct {
	Table          string  `json:"table"`
	ProposedSlug   string  `json:"proposed_slug"`
	ProposedName   string  `json:"proposed_name"`
	ExistingSlug   string  `json:"existing_slug"`
	ExistingName   string  `json:"existing_name"`
	SlugSimilarity float64 `json:"slug_similarity"`
	NameSimilarity float64 `json:"name_similarity"`
}

// saveEnrichmentError is an envelope error that may carry a structured detail.
// The embedded actionError keeps path/code/message identical to every other
// action's error shape; the detail rides the error rather than a parallel
// array because a worker acts on it at the path the error already names.
type saveEnrichmentError struct {
	actionError
	Collision *nameCollisionDetail `json:"collision,omitempty"`
}

// saveErr lifts a plain action error into the save envelope's error type.
func saveErr(e actionError) saveEnrichmentError {
	return saveEnrichmentError{actionError: e}
}

// functionError is one entry in mcp.save_enrichment's errors[]. The collision
// fields are populated only for code "name_collision" and are zero otherwise.
type functionError struct {
	Path           string  `json:"path"`
	Code           string  `json:"code"`
	Message        string  `json:"message"`
	Table          string  `json:"table"`
	ProposedSlug   string  `json:"proposed_slug"`
	ProposedName   string  `json:"proposed_name"`
	ExistingSlug   string  `json:"existing_slug"`
	ExistingName   string  `json:"existing_name"`
	SlugSimilarity float64 `json:"slug_similarity"`
	NameSimilarity float64 `json:"name_similarity"`
}

type saveEnrichmentEnvelope struct {
	Ok                   bool                       `json:"ok"`
	ClassificationID     *int64                     `json:"classification_id"`
	PostingID            int64                      `json:"posting_id"`
	Summary              string                     `json:"summary"`
	NewTaxonomy          newTaxonomy                `json:"new_taxonomy"`
	Errors               []saveEnrichmentError      `json:"errors"`
	Substitutions        []substitution             `json:"substitutions,omitempty"`
	Dropped              []droppedEntry             `json:"dropped,omitempty"`
	SimilarityCandidates []similarityCandidateGroup `json:"similarity_candidates,omitempty"`
}

// functionResult is the JSON envelope mcp.save_enrichment returns. ok=false
// carries structured errors[]; ok=true carries the ids, new_taxonomy, and the
// 000033 gate's feedback (substitutions/dropped/similarity_candidates), each
// present only when non-empty.
type functionResult struct {
	Ok                   bool                       `json:"ok"`
	ClassificationID     *int64                     `json:"classification_id"`
	PostingID            *int64                     `json:"posting_id"`
	NewTaxonomy          newTaxonomy                `json:"new_taxonomy"`
	Errors               []functionError            `json:"errors"`
	Substitutions        []substitution             `json:"substitutions"`
	Dropped              []droppedEntry             `json:"dropped"`
	SimilarityCandidates []similarityCandidateGroup `json:"similarity_candidates"`
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

// validateProvenance requires both provenance fields and checks their shape.
//
// Provenance is required, not defaulted. The server used to substitute
// "mcp-agent" / "mcp-save-enrichment-v1" for an omitted provenance, which
// manufactured an agent-asserted signal the agent never asserted: 231 rows
// carry that pair and no query can recover which caller or contract wrote
// them. Under the trust tiers (project.md §Evidence trust tiers) an absent
// signal is recoverable and a fabricated one is not, so a caller that will not
// name its contract is refused rather than labelled for it.
//
// It deliberately does not check membership in a set of known prompt versions.
// The recognized versions live in each writer's own pin — the
// `classification-pins` block of a batch-enrich SKILL.md, or the PromptVersion
// constant in cmd/batch-enrich/config.go — and a pin is bumped in the same
// commit as the prompt change it describes. An allowlist compiled into this
// server would make that cheap, frequent edit require a rebuild and a restart
// of a long-lived MCP process before the new version could be written at all.
// The predictable response is not to bump the pin, which is the exact drift
// this validation would be trying to prevent. So the boundary enforces that a
// contract is named and well-formed; whether the name is the right one is the
// pin's job, and auditing the cohorts is developer-guide §6.2's.
func validateProvenance(model, promptVersion string) []saveEnrichmentError {
	var errs []saveEnrichmentError
	for _, f := range []struct{ path, value, missing string }{
		{"provenance.model", model, "provenance.model is required: name the model that produced this classification"},
		{"provenance.prompt_version", promptVersion, "provenance.prompt_version is required: name the classifier contract that produced this classification, from your pinned PROMPT_VERSION"},
	} {
		switch {
		case f.value == "":
			errs = append(errs, saveErr(actionError{Path: f.path, Code: codeInvalidProvenance, Message: f.missing}))
		case !provenancePattern.MatchString(f.value):
			errs = append(errs, saveErr(actionError{Path: f.path, Code: codeInvalidProvenance,
				Message: f.path + " must match ^[A-Za-z0-9._-]+$"}))
		}
	}
	return errs
}

// runSaveEnrichment validates provenance and the classifier payload (loading
// taxonomy and confirming the posting via the read-only source), then calls the
// approved function through the saver. Every failure mode returns an ok=false
// envelope; it never returns a transport error.
func runSaveEnrichment(ctx context.Context, req saveEnrichmentRequest, tax taxonomySource, saver enrichmentSaver) saveEnrichmentEnvelope {
	model := strings.TrimSpace(req.Provenance.Model)
	promptVersion := strings.TrimSpace(req.Provenance.PromptVersion)

	errs := validateProvenance(model, promptVersion)

	// Load taxonomy for the shared validation. A load failure is a DB fault, not
	// a validation rejection — surface it as db_error.
	taxonomy, err := tax.load(ctx)
	if err != nil {
		return failureSaveEnvelope(req, append(errs, saveErr(actionError{Path: "db", Code: codeDBError, Message: err.Error()})))
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
		errs = append(errs, saveErr(actionError{Path: f.Path, Code: string(f.Code), Message: f.Message}))
	}

	// Posting existence is a DB-backed validation: confirm it before the write.
	exists, err := tax.postingExists(ctx, req.PostingID)
	if err != nil {
		return failureSaveEnvelope(req, append(errs, saveErr(actionError{Path: "db", Code: codeDBError, Message: err.Error()})))
	}
	if !exists {
		errs = append(errs, saveErr(actionError{Path: "posting_id", Code: codePostingNotFound,
			Message: fmt.Sprintf("job posting %d does not exist", req.PostingID)}))
	}

	if len(errs) > 0 {
		return failureSaveEnvelope(req, errs)
	}

	// Build the function payload: classifier shape only. Provenance travels as
	// separate function arguments; skills[].requirement is stripped here.
	payload, err := buildPayload(req)
	if err != nil {
		return failureSaveEnvelope(req, []saveEnrichmentError{saveErr(actionError{Path: "payload", Code: codeDBError, Message: err.Error()})})
	}

	raw, err := saver.save(ctx, payload, model, promptVersion)
	if err != nil {
		return failureSaveEnvelope(req, []saveEnrichmentError{saveErr(actionError{Path: "db", Code: codeDBError, Message: err.Error()})})
	}

	var fr functionResult
	if err := json.Unmarshal(raw, &fr); err != nil {
		return failureSaveEnvelope(req, []saveEnrichmentError{saveErr(actionError{Path: "db", Code: codeDBError,
			Message: fmt.Sprintf("decoding save_enrichment result: %v", err)})})
	}

	if !fr.Ok {
		// SQL-level invariant violations come back as structured errors, not as a
		// raised exception, so they map straight into the envelope.
		mapped := make([]saveEnrichmentError, 0, len(fr.Errors))
		for _, e := range fr.Errors {
			m := saveErr(actionError{Path: e.Path, Code: e.Code, Message: e.Message})
			// name_collision (migration 000038) carries the collider and both
			// axis scores. Forwarded as a typed detail so a worker reads it
			// rather than parsing the message prose.
			if e.Code == codeNameCollision {
				m.Collision = &nameCollisionDetail{
					Table:          e.Table,
					ProposedSlug:   e.ProposedSlug,
					ProposedName:   e.ProposedName,
					ExistingSlug:   e.ExistingSlug,
					ExistingName:   e.ExistingName,
					SlugSimilarity: e.SlugSimilarity,
					NameSimilarity: e.NameSimilarity,
				}
			}
			mapped = append(mapped, m)
		}
		return failureSaveEnvelope(req, mapped)
	}

	return saveEnrichmentEnvelope{
		Ok:                   true,
		ClassificationID:     fr.ClassificationID,
		PostingID:            req.PostingID,
		Summary:              req.Summary,
		NewTaxonomy:          normalizeNewTaxonomy(fr.NewTaxonomy),
		Errors:               []saveEnrichmentError{},
		Substitutions:        fr.Substitutions,
		Dropped:              fr.Dropped,
		SimilarityCandidates: fr.SimilarityCandidates,
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
func failureSaveEnvelope(req saveEnrichmentRequest, errs []saveEnrichmentError) saveEnrichmentEnvelope {
	if errs == nil {
		errs = []saveEnrichmentError{}
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
