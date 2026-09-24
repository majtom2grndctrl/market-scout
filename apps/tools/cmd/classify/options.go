package main

import (
	"cmp"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/classify"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/jev"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/skillseed"
)

// termOption is one specialization or skill offered to a noul question.
type termOption struct {
	Slug string
	Name string
}

// options are every option set a run reads once at start. A term minted,
// retired, or merged after this read is invisible to the run; a save naming a
// since-retired term fails that posting, and the next run reads fresh options.
type options struct {
	roles   roleOptions
	specs   []termOption
	skills  []termOption // the seed's noul head
	matcher *skillseed.Matcher
	seed    skillseed.Seed
}

// buildOptions reads the option sets from the live taxonomy and the seed.
// Slugs living in more than one taxonomy table are left out everywhere:
// mcp.save_enrichment rejects any payload that names one, even as reuse. Seed
// entries whose skill is no longer live are dropped with a warning.
func buildOptions(tax classify.Taxonomy, crossTable []string, seed skillseed.Seed) (options, error) {
	excluded := make(map[string]bool, len(crossTable))
	for _, s := range crossTable {
		excluded[s] = true
	}

	roles := classify.Taxonomy{CanonicalRoles: map[string]classify.TaxonomyEntry{}}
	for slug, e := range tax.CanonicalRoles {
		if !excluded[slug] {
			roles.CanonicalRoles[slug] = e
		}
	}
	ro, err := newRoleOptions(roles)
	if err != nil {
		return options{}, err
	}

	var specs []termOption
	for _, slug := range slices.Sorted(maps.Keys(tax.Specializations)) {
		if !excluded[slug] {
			specs = append(specs, termOption{Slug: slug, Name: tax.Specializations[slug].Name})
		}
	}

	live := seed
	live.Entries = nil
	for _, e := range seed.Entries {
		if _, ok := tax.Skills[e.SkillSlug]; !ok || excluded[e.SkillSlug] {
			slog.Warn("[classify] seed entry is not a live, unambiguous skill; skipping", "slug", e.SkillSlug)
			continue
		}
		live.Entries = append(live.Entries, e)
	}
	var skills []termOption
	for _, e := range live.Noul() {
		skills = append(skills, termOption{Slug: e.SkillSlug, Name: tax.Skills[e.SkillSlug].Name})
	}
	if len(live.Entries) == 0 {
		return options{}, fmt.Errorf("the seed reaches no live skill")
	}

	return options{roles: ro, specs: specs, skills: skills, matcher: skillseed.NewMatcher(live), seed: live}, nil
}

// Noul question ids carry the dimension and slug. Ids never reach the model.
func specQuestionID(slug string) string  { return "spec:" + slug }
func skillQuestionID(slug string) string { return "skill:" + slug }

// noulCandidates reads one dimension's noul answers as candidates.
func noulCandidates(answers map[string]jev.Answer, terms []termOption, id func(string) string) []label {
	out := make([]label, 0, len(terms))
	for _, t := range terms {
		a := answers[id(t.Slug)]
		if a.Noul == nil {
			continue
		}
		out = append(out, label{Slug: t.Slug, Name: t.Name, Method: methodJev, Probability: *a.Noul})
	}
	slices.SortFunc(out, func(a, b label) int { return cmp.Compare(a.Slug, b.Slug) })
	return out
}
