package main

import (
	"cmp"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/classify"
)

// runTaxonomyExport writes the live option sets as TSV for gold labelers:
// roles, specializations, and skills, each slug<TAB>name, leaving out slugs
// that live in two tables. It reads only.
func runTaxonomyExport(args []string) int {
	fs := flag.NewFlagSet("classify taxonomy-export", flag.ContinueOnError)
	out := fs.String("out", "agent-output/classify/probe", "directory for roles.tsv, specializations.tsv, skills.tsv")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, stop, pool, code := openReadOnly()
	if code != 0 {
		return code
	}
	defer stop()
	defer pool.Close()
	q := db.New(pool)

	tax, err := classify.LoadTaxonomy(ctx, q)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[classify] load taxonomy: %v\n", err)
		return 1
	}
	cross, err := q.ListCrossTableSlugs(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[classify] load cross-table slugs: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[classify] create output dir: %v\n", err)
		return 1
	}
	for name, m := range map[string]map[string]classify.TaxonomyEntry{
		"roles.tsv": tax.CanonicalRoles, "specializations.tsv": tax.Specializations, "skills.tsv": tax.Skills,
	} {
		var b strings.Builder
		for _, slug := range slices.Sorted(maps.Keys(m)) {
			if !slices.Contains(cross, slug) {
				fmt.Fprintf(&b, "%s\t%s\n", slug, m[slug].Name)
			}
		}
		if err := os.WriteFile(filepath.Join(*out, name), []byte(b.String()), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "[classify] %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "[classify] wrote roles.tsv, specializations.tsv, skills.tsv to %s\n", *out)
	return 0
}

// labelLine is one labeler's v9 reading of one posting, title hidden. Role is
// a live role's slug, or empty with RoleUncovered when no live role fits.
type labelLine struct {
	PostingID        int64    `json:"posting_id"`
	Labeler          string   `json:"labeler"`
	Role             string   `json:"role"`
	RoleUncovered    bool     `json:"role_uncovered"`
	ProposedRoleName string   `json:"proposed_role_name,omitempty"`
	Specializations  []string `json:"specializations"`
	Skills           []string `json:"skills"`
	Seniority        string   `json:"seniority"`
	SeniorityNote    string   `json:"seniority_note,omitempty"`
	RoleRationale    string   `json:"role_rationale,omitempty"`
}

// roleKey compares two labelers' role calls: the slug, or none_fit.
func (l labelLine) roleKey() string {
	if l.RoleUncovered || l.Role == "" {
		return noneFit
	}
	return l.Role
}

// disagreement is one posting the owner settles. Only role and seniority are
// adjudicated; specializations and skills keep both readings (see goldLine).
type disagreement struct {
	PostingID         int64        `json:"posting_id"`
	MaskedDescription string       `json:"masked_description"`
	Role              bool         `json:"role"`
	Seniority         bool         `json:"seniority"`
	Labels            [2]labelLine `json:"labels"`
}

// decision is the owner's call on one disagreement.
type decision struct {
	PostingID int64  `json:"posting_id"`
	Role      string `json:"role,omitempty"` // a slug, or none_fit
	Seniority string `json:"seniority,omitempty"`
}

// runGoldMerge merges two labelers' files. Without --decisions it writes the
// disagreements to settle; with them it writes the final gold file.
func runGoldMerge(args []string) int {
	fs := flag.NewFlagSet("classify gold-merge", flag.ContinueOnError)
	labelsGlob := fs.String("labels", "agent-output/classify/probe/labels/*.jsonl", "labeler JSONL files")
	samplePath := fs.String("sample", "agent-output/classify/probe/sample.jsonl", "the probe sample")
	decisionsPath := fs.String("decisions", "", "the owner's decisions JSON; omit to write disagreements instead")
	out := fs.String("out", "agent-output/classify/probe", "output directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	fail := func(err error) int { fmt.Fprintf(os.Stderr, "[classify] %v\n", err); return 1 }

	files, err := filepath.Glob(*labelsGlob)
	if err != nil || len(files) == 0 {
		return fail(fmt.Errorf("no labeler files match %s", *labelsGlob))
	}
	var labels []labelLine
	for _, f := range files {
		ls, err := readJSONL[labelLine](f)
		if err != nil {
			return fail(err)
		}
		labels = append(labels, ls...)
	}
	sample, err := readJSONL[sampleLine](*samplePath)
	if err != nil {
		return fail(err)
	}

	var decisions map[int64]decision
	if *decisionsPath != "" {
		ds, err := readJSONArray[decision](*decisionsPath)
		if err != nil {
			return fail(err)
		}
		decisions = map[int64]decision{}
		for _, d := range ds {
			decisions[d.PostingID] = d
		}
	}

	gold, open, err := mergeLabels(sample, labels, decisions)
	if err != nil {
		return fail(err)
	}
	if decisions == nil {
		if err := writeJSONFile(filepath.Join(*out, "disagreements.json"), open); err != nil {
			return fail(err)
		}
		fmt.Fprintf(os.Stderr, "[classify] %d of %d postings need a decision; wrote disagreements.json\n", len(open), len(sample))
		return 0
	}
	if len(open) > 0 {
		return fail(fmt.Errorf("%d disagreements have no decision, first posting %d", len(open), open[0].PostingID))
	}
	lines := make([]any, len(gold))
	for i, g := range gold {
		lines[i] = g
	}
	if err := writeJSONLFile(filepath.Join(*out, "gold.jsonl"), lines); err != nil {
		return fail(err)
	}
	fmt.Fprintf(os.Stderr, "[classify] wrote %d gold lines\n", len(gold))
	return 0
}

// mergeLabels pairs each sample posting's two labels. Role and seniority come
// from agreement or the owner's decision. Specializations and skills keep the
// labels both labelers gave (the agreed set, for recall) and the labels
// either gave (for precision), rather than asking the owner to settle every
// tag.
func mergeLabels(sample []sampleLine, labels []labelLine, decisions map[int64]decision) ([]goldLine, []disagreement, error) {
	byPosting := map[int64][]labelLine{}
	for _, l := range labels {
		byPosting[l.PostingID] = append(byPosting[l.PostingID], l)
	}
	var gold []goldLine
	var open []disagreement
	for _, s := range sample {
		ls := byPosting[s.PostingID]
		if len(ls) != 2 || ls[0].Labeler == ls[1].Labeler {
			return nil, nil, fmt.Errorf("posting %d has %d labels, want one from each of two labelers", s.PostingID, len(ls))
		}
		slices.SortFunc(ls, func(a, b labelLine) int { return cmp.Compare(a.Labeler, b.Labeler) })
		a, b := ls[0], ls[1]
		roleAgreed := a.roleKey() == b.roleKey()
		seniorityAgreed := a.Seniority == b.Seniority

		g := goldLine{PostingID: s.PostingID, RoleAgreed: &roleAgreed,
			Specializations: intersect(a.Specializations, b.Specializations), SpecializationsAny: union(a.Specializations, b.Specializations),
			Skills: intersect(a.Skills, b.Skills), SkillsAny: union(a.Skills, b.Skills)}
		role, sen := a.roleKey(), a.Seniority
		if !roleAgreed || !seniorityAgreed {
			d, ok := decisions[s.PostingID]
			if !ok || (!roleAgreed && d.Role == "") || (!seniorityAgreed && d.Seniority == "") {
				open = append(open, disagreement{PostingID: s.PostingID, MaskedDescription: s.MaskedDescription,
					Role: !roleAgreed, Seniority: !seniorityAgreed, Labels: [2]labelLine{a, b}})
				continue
			}
			if !roleAgreed {
				role = d.Role
			}
			if !seniorityAgreed {
				sen = d.Seniority
			}
		}
		if role == noneFit {
			g.RoleUncovered = true
		} else {
			g.Role = role
		}
		g.Seniority = sen
		gold = append(gold, g)
	}
	return gold, open, nil
}

func intersect(a, b []string) []string {
	var out []string
	for _, s := range a {
		if slices.Contains(b, s) && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return out
}

func union(a, b []string) []string {
	out := slices.Clone(a)
	for _, s := range b {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
