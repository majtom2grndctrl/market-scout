package classify

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Code is a stable validation failure code. Consumers branch on it; the agent
// or operator reads it to know exactly which rule fired. The code → path
// mapping is fixed by the MCP Safe Actions plan and re-checked SQL-side for the
// invariants the database also owns.
type Code string

const (
	CodeMissingSeniority Code = "missing_seniority"
	CodeInvalidSeniority Code = "invalid_seniority"
	CodeInvalidSlug      Code = "invalid_slug"
	CodeSlugTooLong      Code = "slug_too_long"
	CodeDuplicateSlug    Code = "duplicate_slug"
	CodeSlugCollision    Code = "slug_collision"
	CodeEmptyDimensions  Code = "empty_dimensions"
	CodeUnknownDimension Code = "unknown_dimension"
	CodeNullByte         Code = "null_byte"
	CodeNotesTooLong     Code = "notes_too_long"
)

// Failure is one structured validation failure: a stable code, the JSON path of
// the offending field, and an operator-facing message. Both consumers convert
// these into their own surface — batch-enrich into retry hints, MCP into the
// action error envelope.
type Failure struct {
	Path    string `json:"path"`
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

// slugPattern matches the required kebab-case slug shape: lowercase ASCII
// letters and digits, optional single-hyphen separators, no
// leading/trailing/consecutive hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// MaxSlugLen is the 64-char ceiling for slug fields.
const MaxSlugLen = 64

// MaxNotesLen caps the notes field length to prevent multi-MB values from
// reaching the DB. 4096 characters accommodates any reasonable annotation.
const MaxNotesLen = 4096

// validSeniorities is the closed set for the seniority field. It mirrors the
// CHECK constraint on classifications.seniority.
var validSeniorities = map[string]struct{}{
	"intern":    {},
	"junior":    {},
	"mid":       {},
	"senior":    {},
	"staff":     {},
	"principal": {},
	"lead":      {},
	"director":  {},
	"unknown":   {},
}

// SeniorityList returns the closed seniority set in contract order, for use in
// operator-facing messages.
func SeniorityList() string {
	return "intern, junior, mid, senior, staff, principal, lead, director, unknown"
}

// Validate runs every payload rule against resp using taxonomy and returns a
// structured failure per violation. All rules are evaluated regardless of
// earlier failures so a single pass surfaces every problem at once. An empty
// slice means the payload is valid. Failures are de-duplicated by (code, path,
// message) while preserving first-seen order.
//
// This is the validation both batch-enrich and the MCP save_enrichment action
// reuse. The DB function re-checks the cross-table and dimension invariants it
// also owns; the codes here match the action error envelope.
func Validate(resp AgentResponse, taxonomy Taxonomy) []Failure {
	var fails []Failure
	add := func(path string, code Code, msg string) {
		fails = append(fails, Failure{Path: path, Code: code, Message: msg})
	}

	// Seniority. Whitespace-only counts as missing — the contract requires the
	// canonical "unknown" sentinel when undeterminable. The closed-set lookup
	// uses the original (untrimmed) value so padding like " senior " is rejected
	// with invalid_seniority rather than silently accepted; the DB CHECK constraint
	// and mcp.save_enrichment both compare the raw value.
	if strings.TrimSpace(resp.Classification.Seniority) == "" {
		add("classification.seniority", CodeMissingSeniority,
			"seniority is required. Emit one of: "+SeniorityList()+".")
	} else if _, ok := validSeniorities[resp.Classification.Seniority]; !ok {
		add("classification.seniority", CodeInvalidSeniority,
			fmt.Sprintf("`%s` is not a valid seniority. Use one of: %s.", resp.Classification.Seniority, SeniorityList()))
	}

	// Notes: null bytes (Postgres rejects them at the protocol level) and length.
	if strings.ContainsRune(resp.Classification.Notes, '\x00') {
		add("classification.notes", CodeNullByte, "classification.notes contains a null byte (\\x00). Strip it.")
	}
	if len(resp.Classification.Notes) > MaxNotesLen {
		add("classification.notes", CodeNotesTooLong,
			fmt.Sprintf("classification.notes is too long. Notes max %d characters.", MaxNotesLen))
	}

	// Within-payload cross-array collisions. A slug appearing in two minting
	// arrays is reported once against the second occurrence's slug path.
	fails = append(fails, checkWithinPayloadCollisions(resp)...)

	// Canonical roles.
	seenRoleSlugs := make(map[string]struct{}, len(resp.CanonicalRoles))
	for i, role := range resp.CanonicalRoles {
		path := fmt.Sprintf("canonical_roles[%d].slug", i)
		if _, dup := seenRoleSlugs[role.Slug]; dup {
			add(path, CodeDuplicateSlug,
				fmt.Sprintf("`%s` appears more than once. Each slug must be unique within its array.", role.Slug))
		} else {
			seenRoleSlugs[role.Slug] = struct{}{}
		}
		fails = append(fails, checkSlug(role.Slug, path)...)
		if strings.ContainsRune(role.Name, '\x00') {
			add(fmt.Sprintf("canonical_roles[%d].name", i), CodeNullByte,
				"canonical_roles name contains a null byte (\\x00). Strip it.")
		}
		if strings.ContainsRune(role.Notes, '\x00') {
			add(fmt.Sprintf("canonical_roles[%d].notes", i), CodeNullByte,
				"canonical_roles notes contains a null byte (\\x00). Strip it.")
		}
		if len(role.Notes) > MaxNotesLen {
			add(fmt.Sprintf("canonical_roles[%d].notes", i), CodeNotesTooLong,
				fmt.Sprintf("canonical_roles notes is too long. Notes max %d characters.", MaxNotesLen))
		}
		if len(role.Dimensions) == 0 {
			add(fmt.Sprintf("canonical_roles[%d].dimensions", i), CodeEmptyDimensions,
				fmt.Sprintf("canonical_role `%s` has no dimensions. Every role needs at least one dimension slug.", role.Slug))
		}
		for j, dim := range role.Dimensions {
			if _, ok := taxonomy.RoleDimensions[dim]; !ok {
				add(fmt.Sprintf("canonical_roles[%d].dimensions[%d]", i, j), CodeUnknownDimension,
					fmt.Sprintf("`%s` is not a known dimension. Use one of: %s.", dim, knownDimensionSlugs(taxonomy)))
			}
		}
		if f, ok := checkCrossTable(role.Slug, "canonical_roles", path, taxonomy); ok {
			fails = append(fails, f)
		}
	}

	// Specializations.
	seenSpecSlugs := make(map[string]struct{}, len(resp.Specializations))
	for i, spec := range resp.Specializations {
		path := fmt.Sprintf("specializations[%d].slug", i)
		if _, dup := seenSpecSlugs[spec.Slug]; dup {
			add(path, CodeDuplicateSlug,
				fmt.Sprintf("`%s` appears more than once. Each slug must be unique within its array.", spec.Slug))
		} else {
			seenSpecSlugs[spec.Slug] = struct{}{}
		}
		fails = append(fails, checkSlug(spec.Slug, path)...)
		if strings.ContainsRune(spec.Name, '\x00') {
			add(fmt.Sprintf("specializations[%d].name", i), CodeNullByte,
				"specializations name contains a null byte (\\x00). Strip it.")
		}
		if f, ok := checkCrossTable(spec.Slug, "specializations", path, taxonomy); ok {
			fails = append(fails, f)
		}
	}

	// Skills.
	seenSkillSlugs := make(map[string]struct{}, len(resp.Skills))
	for i, skill := range resp.Skills {
		path := fmt.Sprintf("skills[%d].slug", i)
		if _, dup := seenSkillSlugs[skill.Slug]; dup {
			add(path, CodeDuplicateSlug,
				fmt.Sprintf("`%s` appears more than once. Each slug must be unique within its array.", skill.Slug))
		} else {
			seenSkillSlugs[skill.Slug] = struct{}{}
		}
		fails = append(fails, checkSlug(skill.Slug, path)...)
		if strings.ContainsRune(skill.Name, '\x00') {
			add(fmt.Sprintf("skills[%d].name", i), CodeNullByte,
				"skills name contains a null byte (\\x00). Strip it.")
		}
		if f, ok := checkCrossTable(skill.Slug, "skills", path, taxonomy); ok {
			fails = append(fails, f)
		}
	}

	return dedupe(fails)
}

// checkSlug applies the slug shape rules: non-empty, ≤ MaxSlugLen, and matching
// slugPattern. An over-length slug reports slug_too_long; any other malformed
// slug (including empty) reports invalid_slug. Returns zero or one failure.
func checkSlug(slug, path string) []Failure {
	if slug == "" {
		return []Failure{{Path: path, Code: CodeInvalidSlug, Message: "slug is missing or empty. Use lowercase letters, digits, and hyphens only."}}
	}
	if len(slug) > MaxSlugLen {
		return []Failure{{Path: path, Code: CodeSlugTooLong,
			Message: fmt.Sprintf("`%s` is too long. Slugs are at most %d characters.", slug, MaxSlugLen)}}
	}
	if !slugPattern.MatchString(slug) {
		return []Failure{{Path: path, Code: CodeInvalidSlug,
			Message: fmt.Sprintf("`%s` is not a valid slug. Use lowercase letters, digits, and hyphens only. Max %d chars.", slug, MaxSlugLen)}}
	}
	return nil
}

// checkCrossTable fires slug_collision when slug is already owned by a different
// taxonomy table. A slug emitted as a canonical_role that already exists as a
// canonical_role is fine — the collision rule is cross-table only.
func checkCrossTable(slug, emittingTable, path string, taxonomy Taxonomy) (Failure, bool) {
	owner, ok := taxonomy.CrossTableOwner(slug)
	if !ok || owner == emittingTable {
		return Failure{}, false
	}
	return Failure{Path: path, Code: CodeSlugCollision,
		Message: fmt.Sprintf("`%s` is already a %s. Choose a different slug.", slug, owner)}, true
}

// checkWithinPayloadCollisions rejects slugs that appear in more than one of the
// three minting arrays in a single payload. role_dimensions is a closed, seeded
// set the agent never mints into and is excluded. Each collision produces one
// slug_collision failure naming both arrays, reported against the slug path of
// the second occurrence. Table names are alphabetised so the message is stable.
func checkWithinPayloadCollisions(resp AgentResponse) []Failure {
	type origin struct {
		table string
	}
	tableOf := make(map[string]origin)
	var fails []Failure

	add := func(slug, table, path string) {
		if existing, conflict := tableOf[slug]; conflict {
			first, second := existing.table, table
			if first > second {
				first, second = second, first
			}
			fails = append(fails, Failure{
				Path: path,
				Code: CodeSlugCollision,
				Message: fmt.Sprintf("`%s` appears in both %s and %s. A slug belongs to one table only.",
					slug, first, second),
			})
			return
		}
		tableOf[slug] = origin{table: table}
	}

	for i, role := range resp.CanonicalRoles {
		add(role.Slug, "canonical_roles", fmt.Sprintf("canonical_roles[%d].slug", i))
	}
	for i, spec := range resp.Specializations {
		add(spec.Slug, "specializations", fmt.Sprintf("specializations[%d].slug", i))
	}
	for i, skill := range resp.Skills {
		add(skill.Slug, "skills", fmt.Sprintf("skills[%d].slug", i))
	}
	return fails
}

// knownDimensionSlugs returns a sorted, comma-separated list of every dimension
// slug in the taxonomy. Sorted so messages are stable across runs. Returns a
// fallback when the table is empty so messages don't end with "use one of: .".
func knownDimensionSlugs(taxonomy Taxonomy) string {
	if len(taxonomy.RoleDimensions) == 0 {
		return "(none — role_dimensions table is empty)"
	}
	slugs := make([]string, 0, len(taxonomy.RoleDimensions))
	for slug := range taxonomy.RoleDimensions {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	return strings.Join(slugs, ", ")
}

// dedupe removes failures with an identical (code, path, message) triple while
// preserving first-seen order. Duplicates arise when two roles share an unknown
// dimension slug or the same within-payload collision is detectable twice.
func dedupe(fails []Failure) []Failure {
	if len(fails) == 0 {
		return fails
	}
	seen := make(map[Failure]struct{}, len(fails))
	out := fails[:0]
	for _, f := range fails {
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	return out
}
