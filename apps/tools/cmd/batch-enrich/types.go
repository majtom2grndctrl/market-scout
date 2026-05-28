// Shared contract types for the batch-enrich package: postings pulled from
// the database for classification, and the in-memory taxonomy snapshot the
// orchestrator hands to every agent prompt.
//
// These types are the boundary between selection (select.go), prompt
// rendering (prompt.go), and writeback. They live in their own file so no
// one component owns them.
package main

// SelectedPosting is a posting chosen for this run, paired with the latest
// snapshot's title and description text. The description may be the raw
// snapshot text or the boilerplate-stripped variant; see boilerplate.go
// for stripping threshold and logic.
type SelectedPosting struct {
	PostingID       int64
	CompanyID       int64
	Title           string
	DescriptionText string
}

// TaxonomyEntry is one row of any taxonomy table (canonical_roles,
// specializations, skills, role_dimensions). The orchestrator loads each
// table once per run and reuses the snapshot across every agent prompt.
type TaxonomyEntry struct {
	ID   int64
	Name string
}

// Taxonomy is the per-run snapshot of the four taxonomy tables. Every map
// is keyed by slug. crossTable maps slug → owning table name and is used
// to detect cross-table slug collisions during validation. Build the index
// via BuildCrossTableIndex, which returns a new Taxonomy with crossTable populated.
type Taxonomy struct {
	CanonicalRoles  map[string]TaxonomyEntry
	Specializations map[string]TaxonomyEntry
	Skills          map[string]TaxonomyEntry
	RoleDimensions  map[string]TaxonomyEntry
	crossTable      map[string]string
}
