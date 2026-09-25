package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/seniority"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/skillseed"
)

// goldLine is one adjudicated gold label, written under the v9 contract with
// the title hidden. Role is a live role's slug; RoleUncovered marks a posting
// whose job no live role fits, where deferring is the right answer.
// RoleAgreed records whether the two labelers agreed on the role before the
// owner settled it; its mean is the gold set's own agreement. RoleAny lists
// further answers (slugs, or none_fit) that are equally right, for a posting
// that fits several roles.
//
// Specializations and Skills are the labels both labelers gave, and recall is
// scored against them; the Any sets are the labels either gave, and precision
// is scored against those. Tags are not adjudicated one by one, so the gold
// for them is a band, not a point, and the report says so.
type goldLine struct {
	PostingID          int64    `json:"posting_id"`
	Role               string   `json:"role"`
	RoleUncovered      bool     `json:"role_uncovered"`
	RoleAny            []string `json:"role_any,omitempty"`
	Specializations    []string `json:"specializations"`
	SpecializationsAny []string `json:"specializations_any,omitempty"`
	Skills             []string `json:"skills"`
	SkillsAny          []string `json:"skills_any,omitempty"`
	Seniority          string   `json:"seniority"`
	RoleAgreed         *bool    `json:"role_agreed"`
}

// acceptsRole reports whether a role answer, a slug or none_fit, is right.
func (g goldLine) acceptsRole(answer string) bool {
	want := g.Role
	if g.RoleUncovered {
		want = noneFit
	}
	return answer == want || slices.Contains(g.RoleAny, answer)
}

// anyOr returns the either-labeler set when the gold file carries one.
func anyOr(anySet, agreed []string) []string {
	if anySet != nil {
		return anySet
	}
	return agreed
}

// runLine decodes any line of a dry-run file.
type runLine struct {
	Type string `json:"type"`
	postingResult
	Usage runUsage `json:"usage"`
}

// dryRunFile is one dry-run's postings and usage.
type dryRunFile struct {
	postings map[int64]postingResult
	usage    runUsage
}

func readDryRun(path string) (dryRunFile, error) {
	lines, err := readJSONL[runLine](path)
	if err != nil {
		return dryRunFile{}, err
	}
	f := dryRunFile{postings: map[int64]postingResult{}}
	for _, l := range lines {
		switch l.Type {
		case "posting":
			f.postings[l.PostingID] = l.postingResult
		case "summary":
			f.usage = l.Usage
		}
	}
	return f, nil
}

// probeInputs are everything the report scores.
type probeInputs struct {
	gold    map[int64]goldLine
	run     dryRunFile
	run2    *dryRunFile // a second name-only run, for the flip rate
	descRun *dryRunFile // the description arm
	sample  map[int64]sampleLine
	seed    *skillseed.Seed
	cofire  func(labels []label) (int, error) // near-synonym pairs among one posting's labels
}

// ratio is a count over a count, printed as a percentage.
type ratio struct{ Hit, Of int }

func (r ratio) pct() float64 {
	if r.Of == 0 {
		return 0
	}
	return 100 * float64(r.Hit) / float64(r.Of)
}

func (r ratio) String() string {
	if r.Of == 0 {
		return "n/a (0)"
	}
	return fmt.Sprintf("%.1f%% (%d/%d)", r.pct(), r.Hit, r.Of)
}

type probeReport struct {
	Postings, Scored      int
	Outcomes              map[outcome]int
	Top1, Top3, Answered  ratio
	Thirds                [3]ratio // by top probability: low, middle, high
	Flip                  ratio
	GoldAgreement         ratio
	SpecP, SpecR          ratio
	SkillP, SkillR        map[string]*[2]ratio // method → precision, recall
	SeniorityP            ratio
	SeniorityFromTitle    int
	SeniorityUncovered    int
	Cofire                ratio
	CofirePairs           int
	CostPer1k, TokensPer1 float64
	SingleWordTitles      ratio // top-1 on postings whose title strips to one word
	DescB, NameB          ratio // description arm vs name-only on half B
	Kill                  []string
}

// topRole is a posting's most probable pass-2 answer, ignoring the floor:
// none_fit when it beats every role.
func topRole(r postingResult) string {
	if len(r.RoleDistribution) == 0 {
		return ""
	}
	if r.NoneFit >= r.RoleDistribution[0].Probability {
		return noneFit
	}
	return r.RoleDistribution[0].Slug
}

// correctRole scores the run's decision: the written role must be one gold
// accepts, and a deferral is right only when gold accepts none_fit.
func correctRole(r postingResult, g goldLine) bool {
	switch r.Outcome {
	case outcomeWouldWrite:
		return r.Role != nil && r.Role.Slug != noneFit && g.acceptsRole(r.Role.Slug)
	case outcomeDeferred:
		return g.acceptsRole(noneFit)
	}
	return false
}

func scoreProbe(in probeInputs) (probeReport, error) {
	rep := probeReport{Outcomes: map[outcome]int{}, SkillP: map[string]*[2]ratio{}, SkillR: map[string]*[2]ratio{}}
	rep.Postings = len(in.run.postings)

	lexicalReach, noulReach := map[string]bool{}, map[string]bool{}
	if in.seed != nil {
		for _, e := range in.seed.Entries {
			if e.Reach == skillseed.ReachNoul {
				noulReach[e.SkillSlug] = true
			} else {
				lexicalReach[e.SkillSlug] = true
			}
		}
	}

	type conf struct {
		p  float64
		ok bool
	}
	var confs []conf
	for _, id := range slices.Sorted(maps.Keys(in.run.postings)) {
		r := in.run.postings[id]
		rep.Outcomes[r.Outcome]++
		g, ok := in.gold[id]
		if !ok {
			continue
		}
		rep.Scored++
		rep.Top1.Of++
		if correctRole(r, g) {
			rep.Top1.Hit++
		}
		if r.Outcome == outcomeWouldWrite {
			rep.Answered.Of++
			if correctRole(r, g) {
				rep.Answered.Hit++
			}
		}
		if !g.RoleUncovered && len(r.RoleDistribution) > 0 {
			rep.Top3.Of++
			for _, d := range r.RoleDistribution[:min(3, len(r.RoleDistribution))] {
				if g.acceptsRole(d.Slug) {
					rep.Top3.Hit++
					break
				}
			}
		}
		if len(r.RoleDistribution) > 0 {
			confs = append(confs, conf{p: max(r.RoleDistribution[0].Probability, r.NoneFit), ok: g.acceptsRole(topRole(r))})
		}
		if s, ok := in.sample[id]; ok && len(strings.Fields(seniority.StripLevelWords(s.Title))) == 1 {
			rep.SingleWordTitles.Of++
			if correctRole(r, g) {
				rep.SingleWordTitles.Hit++
			}
		}

		if r.Outcome == outcomeWouldWrite {
			scoreSet(&rep.SpecP, nil, slugSet(r.Specializations, ""), setOf(anyOr(g.SpecializationsAny, g.Specializations)), nil)
			scoreSet(nil, &rep.SpecR, slugSet(r.Specializations, ""), setOf(g.Specializations), nil)
			for _, m := range []string{methodLexical, methodJev} {
				if rep.SkillP[m] == nil {
					rep.SkillP[m], rep.SkillR[m] = &[2]ratio{}, &[2]ratio{}
				}
				reach := lexicalReach
				if m == methodJev {
					reach = noulReach
				}
				if in.seed == nil {
					reach = nil
				}
				scoreSet(&rep.SkillP[m][0], nil, slugSet(r.Skills, m), setOf(anyOr(g.SkillsAny, g.Skills)), reach)
				scoreSet(nil, &rep.SkillR[m][0], slugSet(r.Skills, m), setOf(g.Skills), reach)
			}
			// Gold labelers never saw the title, so only body-sourced seniority
			// can be checked against them; title-sourced answers are counted.
			switch {
			case strings.HasPrefix(r.SeniorityNote, "seniority[step1-title]"):
				rep.SeniorityFromTitle++
			case r.Seniority != "" && r.Seniority != seniority.Unknown && g.Seniority != "":
				rep.SeniorityP.Of++
				if r.Seniority == g.Seniority {
					rep.SeniorityP.Hit++
				}
			}
			if in.cofire != nil {
				pairs, err := in.cofire(append(slices.Clone(r.Specializations), r.Skills...))
				if err != nil {
					return rep, err
				}
				rep.Cofire.Of++
				rep.CofirePairs += pairs
				if pairs > 0 {
					rep.Cofire.Hit++
				}
			}
		}
		if r.SeniorityUncovered {
			rep.SeniorityUncovered++
		}
	}
	for _, g := range in.gold {
		if g.RoleAgreed != nil {
			rep.GoldAgreement.Of++
			if *g.RoleAgreed {
				rep.GoldAgreement.Hit++
			}
		}
	}

	// Confidence thirds: equal-sized bins by top probability.
	slices.SortStableFunc(confs, func(a, b conf) int { return cmp.Compare(a.p, b.p) })
	for i, c := range confs {
		bin := min(2, i*3/max(1, len(confs)))
		rep.Thirds[bin].Of++
		if c.ok {
			rep.Thirds[bin].Hit++
		}
	}

	if in.run2 != nil {
		for id, a := range in.run.postings {
			b, ok := in.run2.postings[id]
			if !ok || topRole(a) == "" || topRole(b) == "" {
				continue
			}
			rep.Flip.Of++
			if topRole(a) != topRole(b) {
				rep.Flip.Hit++
			}
		}
	}

	if in.descRun != nil {
		for id, s := range in.sample {
			g, ok := in.gold[id]
			if !ok || s.Half != "B" {
				continue
			}
			if r, ok := in.run.postings[id]; ok {
				rep.NameB.Of++
				if correctRole(r, g) {
					rep.NameB.Hit++
				}
			}
			if r, ok := in.descRun.postings[id]; ok {
				rep.DescB.Of++
				if correctRole(r, g) {
					rep.DescB.Hit++
				}
			}
		}
	}

	if rep.Postings > 0 {
		rep.CostPer1k = in.run.usage.Cost / float64(rep.Postings) * 1000
		rep.TokensPer1 = float64(in.run.usage.Tokens) / float64(rep.Postings)
	}
	rep.Kill = killCriteria(rep)
	return rep, nil
}

// killCriteria returns every criterion the probe breaks. Empty means it
// passes. Criteria: role top-1 more than ten points below the gold set's own
// agreement; a high-confidence third not at least ten points more accurate
// than the low; more than 5% of role answers flipping across two runs.
func killCriteria(r probeReport) []string {
	var out []string
	switch {
	case r.GoldAgreement.Of == 0:
		out = append(out, "cannot evaluate: the gold file carries no role_agreed values")
	case r.Top1.pct() < r.GoldAgreement.pct()-10:
		out = append(out, fmt.Sprintf("role top-1 %.1f%% is more than 10 points below gold agreement %.1f%%", r.Top1.pct(), r.GoldAgreement.pct()))
	}
	if r.Thirds[2].pct() < r.Thirds[0].pct()+10 {
		out = append(out, fmt.Sprintf("high-confidence third %.1f%% is not 10 points above the low third %.1f%%", r.Thirds[2].pct(), r.Thirds[0].pct()))
	}
	switch {
	case r.Flip.Of == 0:
		out = append(out, "cannot evaluate: no second run to measure flips")
	case r.Flip.pct() > 5:
		out = append(out, fmt.Sprintf("flip rate %.1f%% exceeds 5%%", r.Flip.pct()))
	}
	return out
}

// scoreSet adds one posting's precision or recall counts; pass nil for the one
// not wanted. Recall counts only gold labels the instrument can reach, when
// reach is known.
func scoreSet(p, r *ratio, pred, gold map[string]bool, reach map[string]bool) {
	for s := range pred {
		if p == nil {
			break
		}
		p.Of++
		if gold[s] {
			p.Hit++
		}
	}
	if r == nil {
		return
	}
	for s := range gold {
		if reach != nil && !reach[s] {
			continue
		}
		r.Of++
		if pred[s] {
			r.Hit++
		}
	}
}

func slugSet(ls []label, method string) map[string]bool {
	out := map[string]bool{}
	for _, l := range ls {
		if method == "" || l.Method == method {
			out[l.Slug] = true
		}
	}
	return out
}

func setOf(ss []string) map[string]bool {
	out := make(map[string]bool, len(ss))
	for _, s := range ss {
		out[s] = true
	}
	return out
}

func (r probeReport) markdown() string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("# Probe report")
	w("")
	if len(r.Kill) == 0 {
		w("**Kill criteria: pass.**")
	} else {
		w("**Kill criteria: STOP.**")
		for _, k := range r.Kill {
			w("- %s", k)
		}
	}
	w("")
	w("Postings in run: %d; scored against gold: %d. Outcomes: %v.", r.Postings, r.Scored, r.Outcomes)
	w("")
	w("| Measure | Value |")
	w("|---|---|")
	w("| Role top-1 (all scored; a deferral is right only when gold has no live role) | %s |", r.Top1)
	w("| Role top-1 among written | %s |", r.Answered)
	w("| Role top-3 (gold role in the pass-2 top three) | %s |", r.Top3)
	w("| Gold set's own role agreement | %s |", r.GoldAgreement)
	w("| Top-1 by confidence third: low / middle / high | %s / %s / %s |", r.Thirds[0], r.Thirds[1], r.Thirds[2])
	w("| Role flip rate across two runs | %s |", r.Flip)
	w("| Specializations (jev): precision vs either labeler / recall vs both | %s / %s |", r.SpecP, r.SpecR)
	for _, m := range []string{methodLexical, methodJev} {
		if pr := r.SkillP[m]; pr != nil {
			w("| Skills (%s): precision vs either labeler / recall vs both, over reachable gold | %s / %s |", m, pr[0], r.SkillR[m][0])
		}
	}
	w("| Near-synonym co-fire: postings with a pair / pairs | %s / %d |", r.Cofire, r.CofirePairs)
	w("| Precision of non-unknown seniority read from the body (gold never saw titles) | %s |", r.SeniorityP)
	w("| Seniority read from the title (not checkable against gold) | %d |", r.SeniorityFromTitle)
	w("| Seniority left unknown for an uncovered compound | %d |", r.SeniorityUncovered)
	w("| Top-1 on titles that strip to one word | %s |", r.SingleWordTitles)
	w("| Cost per 1,000 postings | $%.3f (%.0f input tokens per posting) |", r.CostPer1k, r.TokensPer1)
	if r.DescB.Of > 0 {
		w("| Description arm, half B: name-only / with descriptions | %s / %s |", r.NameB, r.DescB)
	}
	return b.String()
}

// runProbeReport reads the files, scores them, and prints the report. It asks
// the database only for co-fire similarity, on the read-only role.
func runProbeReport(args []string) int {
	fs := flag.NewFlagSet("classify probe-report", flag.ContinueOnError)
	goldPath := fs.String("gold", "", "adjudicated gold JSONL (required)")
	runPath := fs.String("run", "", "name-only dry-run JSONL (required)")
	run2Path := fs.String("run2", "", "a second name-only dry-run over the same sample, for the flip rate")
	descPath := fs.String("desc-run", "", "description-arm dry-run over the same sample")
	samplePath := fs.String("sample", "", "the probe sample.jsonl (halves and titles)")
	seedPath := fs.String("seed", "", "the seed the runs used, for reach-aware skill recall")
	noDB := fs.Bool("no-db", false, "skip the co-fire measure, which reads the database")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *goldPath == "" || *runPath == "" {
		fmt.Fprintln(os.Stderr, "[classify] probe-report needs --gold and --run")
		return 2
	}

	in := probeInputs{gold: map[int64]goldLine{}, sample: map[int64]sampleLine{}}
	fail := func(err error) int { fmt.Fprintf(os.Stderr, "[classify] %v\n", err); return 1 }
	gold, err := readJSONL[goldLine](*goldPath)
	if err != nil {
		return fail(err)
	}
	for _, g := range gold {
		in.gold[g.PostingID] = g
	}
	if in.run, err = readDryRun(*runPath); err != nil {
		return fail(err)
	}
	for _, opt := range []struct {
		path string
		dst  **dryRunFile
	}{{*run2Path, &in.run2}, {*descPath, &in.descRun}} {
		if opt.path == "" {
			continue
		}
		f, err := readDryRun(opt.path)
		if err != nil {
			return fail(err)
		}
		*opt.dst = &f
	}
	if *samplePath != "" {
		lines, err := readJSONL[sampleLine](*samplePath)
		if err != nil {
			return fail(err)
		}
		for _, l := range lines {
			in.sample[l.PostingID] = l
		}
	}
	if *seedPath != "" {
		s, err := skillseed.Load(*seedPath)
		if err != nil {
			return fail(err)
		}
		in.seed = &s
	}
	if !*noDB {
		ctx, stop, pool, code := openReadOnly()
		if code != 0 {
			return code
		}
		defer stop()
		defer pool.Close()
		f := dbDuplicateFinder{q: db.New(pool)}
		in.cofire = func(ls []label) (int, error) { return countCofire(ctx, f, ls) }
	}

	rep, err := scoreProbe(in)
	if err != nil {
		return fail(err)
	}
	fmt.Print(rep.markdown())
	return 0
}

// cofireThreshold is the save's advisory level (c_advisory_at, 000043): the
// similarity at which the name gate starts reporting near matches.
const cofireThreshold = "0.60"

func countCofire(ctx context.Context, f dbDuplicateFinder, ls []label) (int, error) {
	if len(ls) < 2 {
		return 0, nil
	}
	rows, err := f.pairsAt(ctx, ls, cofireThreshold)
	return len(rows), err
}
