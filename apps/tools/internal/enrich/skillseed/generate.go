package skillseed

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Params tune the generator. They are recorded in the seed, and a change to
// them changes the seed hash only through the entries it produces.
type Params struct {
	// MinEvidence is how many corpus documents an alias must appear in before
	// its precision decides. Below it, the name is rare in this install's
	// postings and is treated as a specific, distinctive name.
	MinEvidence int `json:"min_evidence"`
	// MinPrecision is the share of documents mentioning a name whose latest
	// classification assigned the skill. Below it the name is a common word in
	// this corpus ("communication" appears in most postings and labels few).
	MinPrecision float64 `json:"min_precision"`
	// MaxDocShare caps the share of the corpus an alias may appear in at all.
	MaxDocShare float64 `json:"max_doc_share"`
	// MinRecall applies once a skill is labelled on MinEvidence corpus
	// documents: at least this share of them must name it. A skill labelled
	// often but rarely named ("Product Thinking") is inferred, not quoted, so a
	// name match would miss most of it.
	MinRecall float64 `json:"min_recall"`
	// NoulHeadSize and NoulMinLinkMass bound the noul head: the most-linked
	// skills a name match cannot reach.
	NoulHeadSize    int   `json:"noul_head_size"`
	NoulMinLinkMass int64 `json:"noul_min_link_mass"`
}

// DefaultParams are the generator's starting values. research.md §Skill
// coverage sizes the noul head near 300 questions per posting.
var DefaultParams = Params{
	MinEvidence:     20,
	MinPrecision:    0.5,
	MaxDocShare:     0.25,
	MinRecall:       0.5,
	NoulHeadSize:    300,
	NoulMinLinkMass: 3,
}

// SkillStat is one live skill as the generator sees it.
type SkillStat struct {
	Slug     string
	Name     string
	LinkMass int64
}

// Doc is one corpus posting: its description and the skills its latest
// classification assigned.
type Doc struct {
	Text   string
	Skills map[string]bool
}

// Generate builds a seed from an install's live skills and a sample of its
// classified postings. A skill is reached lexically when its name is
// distinctive in this corpus, by its context rule when a curated rule names it,
// and by a noul when it is among the most-linked of the rest. Everything else
// is unreachable until the next generation.
func Generate(skills []SkillStat, corpus []Doc, p Params, now time.Time) Seed {
	aliasSets := make([][]string, len(skills))
	probe := Seed{Entries: make([]Entry, len(skills))}
	for i, s := range skills {
		aliasSets[i] = aliases(s.Slug, s.Name)
		probe.Entries[i] = Entry{SkillSlug: s.Slug, Reach: ReachLexical, Aliases: aliasSets[i]}
	}
	matched, labelled := measure(NewMatcher(probe), corpus)
	labels := map[string]int{}
	for _, d := range corpus {
		for slug := range d.Skills {
			labels[slug]++
		}
	}

	seed := Seed{GeneratedAt: now.UTC(), Params: p}
	seed.Stats.LiveSkills, seed.Stats.CorpusDocs = len(skills), len(corpus)
	var rest []SkillStat
	for i, s := range skills {
		e := Entry{SkillSlug: s.Slug, SkillName: s.Name, Aliases: aliasSets[i], LinkMass: s.LinkMass}
		if r, ok := ruleForName(s.Name); ok {
			e.Reach, e.ContextRule = ReachLexical, r.Key
			seed.Entries = append(seed.Entries, e)
			seed.Stats.RuleMatched++
			continue
		}
		if distinctive(aliasSets[i], evidence{matched[s.Slug], labelled[s.Slug], labels[s.Slug], len(corpus)}, p) {
			e.Reach = ReachLexical
			seed.Entries = append(seed.Entries, e)
			seed.Stats.Lexical++
			continue
		}
		rest = append(rest, s)
	}

	slices.SortFunc(rest, func(a, b SkillStat) int {
		if c := cmp.Compare(b.LinkMass, a.LinkMass); c != 0 {
			return c
		}
		return cmp.Compare(a.Slug, b.Slug)
	})
	for _, s := range rest {
		if seed.Stats.NoulHead >= p.NoulHeadSize || s.LinkMass < p.NoulMinLinkMass {
			seed.Stats.Unreachable++
			continue
		}
		seed.Entries = append(seed.Entries, Entry{SkillSlug: s.Slug, SkillName: s.Name, Reach: ReachNoul, LinkMass: s.LinkMass})
		seed.Stats.NoulHead++
	}
	slices.SortFunc(seed.Entries, func(a, b Entry) int { return cmp.Compare(a.SkillSlug, b.SkillSlug) })
	return seed
}

// evidence is what the corpus says about one skill's name.
type evidence struct {
	matched    int // documents naming it
	labelled   int // of those, documents labelled with it
	labels     int // documents labelled with it at all
	corpusSize int
}

// distinctive decides whether a name match is evidence of the skill here:
// not so common it is an ordinary word, precise when named, and named on
// most of the postings that carry it.
func distinctive(aliases []string, e evidence, p Params) bool {
	switch {
	case len(aliases) == 0:
		return false
	case e.corpusSize > 0 && float64(e.matched)/float64(e.corpusSize) > p.MaxDocShare:
		return false
	case e.labels >= p.MinEvidence && float64(e.labelled)/float64(e.labels) < p.MinRecall:
		return false
	case e.matched < p.MinEvidence:
		return true
	}
	return float64(e.labelled)/float64(e.matched) >= p.MinPrecision
}

// measure counts, per skill, the documents an alias matches and how many of
// those the latest classification labelled with that skill.
func measure(m *Matcher, corpus []Doc) (matched, labelled map[string]int) {
	matched, labelled = map[string]int{}, map[string]int{}
	for _, d := range corpus {
		for _, e := range m.Match(d.Text) {
			matched[e.SkillSlug]++
			if d.Skills[e.SkillSlug] {
				labelled[e.SkillSlug]++
			}
		}
	}
	return matched, labelled
}

// parenthetical splits "Amazon Web Services (AWS)" into its two names.
var parenthetical = regexp.MustCompile(`^(.*?)\s*\(([^()]+)\)\s*$`)

// conjoined splits a name that joins two concepts, "Kubernetes & Container
// Orchestration". Only a spaced "&" or "/" splits: "CI/CD" is one term.
var conjoined = regexp.MustCompile(`\s+[&/]\s+`)

// aliases derives a skill's match phrases from its name and slug. A phrase
// whose tokens total fewer than two characters is dropped: a lone letter is
// never evidence without a context rule.
func aliases(slug, name string) []string {
	raw := []string{name, strings.ReplaceAll(slug, "-", " ")}
	if m := parenthetical.FindStringSubmatch(name); m != nil {
		raw = append(raw, m[1], m[2])
	}
	if parts := conjoined.Split(name, -1); len(parts) > 1 {
		raw = append(raw, parts...)
	}
	var out []string
	seen := map[string]bool{}
	for _, a := range raw {
		key := strings.Join(tokens(a), " ")
		if len(strings.ReplaceAll(key, " ", "")) < 2 || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, strings.TrimSpace(a))
	}
	return out
}
