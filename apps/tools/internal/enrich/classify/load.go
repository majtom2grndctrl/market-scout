package classify

import (
	"context"
	"fmt"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

// TaxonomyLoader is the subset of the sqlc db.Queries API LoadTaxonomy needs.
// Declaring it here lets tests inject a fake without a live database;
// *db.Queries satisfies it implicitly.
type TaxonomyLoader interface {
	ListCanonicalRoles(ctx context.Context) ([]db.ListCanonicalRolesRow, error)
	ListSpecializations(ctx context.Context) ([]db.ListSpecializationsRow, error)
	ListSkills(ctx context.Context) ([]db.ListSkillsRow, error)
	ListRoleDimensions(ctx context.Context) ([]db.RoleDimension, error)
}

// LoadTaxonomy loads all four taxonomy tables into a Taxonomy and builds the
// cross-table slug index before returning. It is the shared loader behind
// cmd/batch-enrich and the MCP save_enrichment action; both pass a *db.Queries
// built from their pool (read-only for MCP pre-validation).
func LoadTaxonomy(ctx context.Context, q TaxonomyLoader) (Taxonomy, error) {
	roles, err := q.ListCanonicalRoles(ctx)
	if err != nil {
		return Taxonomy{}, fmt.Errorf("loading canonical_roles: %w", err)
	}
	specs, err := q.ListSpecializations(ctx)
	if err != nil {
		return Taxonomy{}, fmt.Errorf("loading specializations: %w", err)
	}
	skills, err := q.ListSkills(ctx)
	if err != nil {
		return Taxonomy{}, fmt.Errorf("loading skills: %w", err)
	}
	dims, err := q.ListRoleDimensions(ctx)
	if err != nil {
		return Taxonomy{}, fmt.Errorf("loading role_dimensions: %w", err)
	}

	t := Taxonomy{
		CanonicalRoles:  make(map[string]TaxonomyEntry, len(roles)),
		Specializations: make(map[string]TaxonomyEntry, len(specs)),
		Skills:          make(map[string]TaxonomyEntry, len(skills)),
		RoleDimensions:  make(map[string]TaxonomyEntry, len(dims)),
	}
	for _, r := range roles {
		t.CanonicalRoles[r.Slug] = TaxonomyEntry{ID: r.ID, Name: r.Name}
	}
	for _, s := range specs {
		t.Specializations[s.Slug] = TaxonomyEntry{ID: s.ID, Name: s.Name}
	}
	for _, s := range skills {
		t.Skills[s.Slug] = TaxonomyEntry{ID: s.ID, Name: s.Name}
	}
	for _, d := range dims {
		t.RoleDimensions[d.Slug] = TaxonomyEntry{ID: d.ID, Name: d.Name}
	}
	return t.BuildCrossTableIndex(), nil
}
