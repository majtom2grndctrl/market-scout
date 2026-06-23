// Package classify holds the classifier payload contract shared by
// cmd/batch-enrich (which produces classifications from agent output) and the
// MCP save_enrichment action (which persists an agent-supplied classification).
// It owns the AgentResponse wire shape, the per-run Taxonomy snapshot, and the
// structured validation rules both consumers enforce before any write. The
// package is HTTP-free and DB-free: callers load the taxonomy and pass it in.
// It never imports cmd/*.
// See: agent-context/lib/developer-guide.md §6.2 (classifications)
package classify

// AgentResponse mirrors the JSON schema the classifier emits per posting. It is
// the shared contract between batch-enrich (agent output) and the MCP
// save_enrichment action (agent-supplied payload). The skills[].requirement
// field is parsed but not persisted — no storage column exists. Kept in the
// schema so agent outputs carry the signal; write it through once the column is
// added.
type AgentResponse struct {
	PostingID       int64                 `json:"posting_id"`
	Classification  AgentClassification   `json:"classification"`
	CanonicalRoles  []AgentCanonicalRole  `json:"canonical_roles"`
	Specializations []AgentSpecialization `json:"specializations"`
	Skills          []AgentSkill          `json:"skills"`
	Summary         string                `json:"summary"`
}

// AgentClassification carries the posting-level classification fields.
// Seniority is required and validated against a closed set; Notes is optional
// and may arrive as the empty string when the agent omitted it.
type AgentClassification struct {
	Seniority string `json:"seniority"`
	Notes     string `json:"notes"`
}

// AgentCanonicalRole is one canonical-role entry from the agent. A posting may
// emit multiple roles (blended roles are first-class). Dimensions must be
// non-empty and each slug must exist in the role_dimensions taxonomy.
type AgentCanonicalRole struct {
	Slug       string   `json:"slug"`
	Name       string   `json:"name"`
	Dimensions []string `json:"dimensions"`
	Notes      string   `json:"notes,omitempty"`
}

// AgentSpecialization is one specialization entry from the agent.
type AgentSpecialization struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// AgentSkill is one skill entry from the agent. Requirement is parsed but not
// persisted — no storage column exists. Kept in the schema so agent outputs
// carry the signal; write it through once the column is added.
type AgentSkill struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Requirement string `json:"requirement,omitempty"`
}

// TaxonomyEntry is one row of any taxonomy table (canonical_roles,
// specializations, skills, role_dimensions). Callers load each table once and
// reuse the snapshot across validations.
type TaxonomyEntry struct {
	ID   int64
	Name string
}

// Taxonomy is the snapshot of the four taxonomy tables used during validation.
// Every map is keyed by slug. crossTable maps slug → owning table name and is
// used to detect cross-table slug collisions against existing taxonomy. Build
// the index via BuildCrossTableIndex, which returns a new Taxonomy with
// crossTable populated.
type Taxonomy struct {
	CanonicalRoles  map[string]TaxonomyEntry
	Specializations map[string]TaxonomyEntry
	Skills          map[string]TaxonomyEntry
	RoleDimensions  map[string]TaxonomyEntry
	crossTable      map[string]string
}
