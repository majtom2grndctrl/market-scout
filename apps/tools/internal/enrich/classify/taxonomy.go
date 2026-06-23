package classify

import "log/slog"

// CrossTableOwner returns the table name owning slug, or ("", false) if not
// found in any taxonomy table.
func (t Taxonomy) CrossTableOwner(slug string) (string, bool) {
	owner, ok := t.crossTable[slug]
	return owner, ok
}

// BuildCrossTableIndex builds the cross-table slug index from the four taxonomy
// maps and returns a new Taxonomy with crossTable populated. A slug that appears
// in more than one table keeps the first owner encountered in this fixed order:
// canonical_roles, specializations, skills, role_dimensions.
func (t Taxonomy) BuildCrossTableIndex() Taxonomy {
	idx := make(map[string]string, len(t.CanonicalRoles)+len(t.Specializations)+len(t.Skills)+len(t.RoleDimensions))
	add := func(slugs map[string]TaxonomyEntry, table string) {
		for slug := range slugs {
			if existing, exists := idx[slug]; exists {
				slog.Warn("[classify] cross-table slug collision in taxonomy",
					"slug", slug,
					"first_owner", existing,
					"skipped_owner", table,
				)
				continue
			}
			idx[slug] = table
		}
	}
	add(t.CanonicalRoles, "canonical_roles")
	add(t.Specializations, "specializations")
	add(t.Skills, "skills")
	add(t.RoleDimensions, "role_dimensions")
	t.crossTable = idx
	return t
}
