package skillseed

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)

func matchedSlugs(m *Matcher, text string) []string {
	var out []string
	for _, e := range m.Match(text) {
		out = append(out, e.SkillSlug)
	}
	return out
}

// ruledSeed reaches the five ambiguous names by their rules, as a generated
// seed would on an install that has them.
func ruledSeed() Seed {
	var s Seed
	for _, sk := range []SkillStat{{"go", "Go", 0}, {"r", "R", 0}, {"c", "C", 0}, {"c-plus-plus", "C++", 0}, {"c-sharp", "C#", 0}} {
		r, _ := ruleForName(sk.Name)
		s.Entries = append(s.Entries, Entry{SkillSlug: sk.Slug, SkillName: sk.Name, Reach: ReachLexical, Aliases: []string{sk.Name}, ContextRule: r.Key})
	}
	return s
}

func TestLexical_ContextRules(t *testing.T) {
	m := NewMatcher(ruledSeed())
	cases := []struct {
		skill    string
		mustHit  []string
		mustMiss []string
	}{
		{"go",
			[]string{"Backend services in Python, Go and Rust.", "Experience with Golang.", "Strong Go programming skills.", "Services written in Go."},
			[]string{"Ready to go on day one.", "Own our go-to-market motion.", "Go to market with confidence.", "You'll go deep on data."}},
		{"r",
			[]string{"Fluent in Python or R for analysis.", "SQL, R, and Excel.", "Comfortable in RStudio.", "Uses the tidyverse daily."},
			[]string{"Lead R&D planning.", "R & D budget owner.", "Series R funding? Not likely.", "Report to the VP, R. Smith."}},
		{"c",
			[]string{"Embedded C firmware.", "Proficient in C and C++.", "C/C++ on microcontrollers.", "Python, C, Rust."},
			[]string{"Report to C-suite executives.", "Grade C or better.", "Strong C++ background only.", "Expert in C# and .NET."}},
		{"c-plus-plus",
			[]string{"Strong C++ background.", "C/C++ on microcontrollers.", "Modern C++17 codebase."},
			[]string{"Proficient in C and Rust.", "Earned a C+ in physics.", "Expert in C# and .NET."}},
		{"c-sharp",
			[]string{"Expert in C# and .NET.", "C#, TypeScript, SQL.", "Uses C-sharp daily."},
			[]string{"Proficient in C and C++.", "Plays sonatas in C# minor.", "Strong C++ background."}},
	}
	for _, tc := range cases {
		for _, text := range tc.mustHit {
			t.Run(tc.skill+" matches "+text, func(t *testing.T) {
				if !slices.Contains(matchedSlugs(m, text), tc.skill) {
					t.Errorf("%s not matched in %q (got %v)", tc.skill, text, matchedSlugs(m, text))
				}
			})
		}
		for _, text := range tc.mustMiss {
			t.Run(tc.skill+" does not match "+text, func(t *testing.T) {
				if slices.Contains(matchedSlugs(m, text), tc.skill) {
					t.Errorf("%s matched in %q", tc.skill, text)
				}
			})
		}
	}
}

func TestLexical_AliasesMatchAcrossPunctuation(t *testing.T) {
	m := NewMatcher(Seed{Entries: []Entry{
		{SkillSlug: "nodejs", Reach: ReachLexical, Aliases: aliases("nodejs", "Node.js")},
		{SkillSlug: "aws", Reach: ReachLexical, Aliases: aliases("amazon-web-services", "Amazon Web Services (AWS)")},
		{SkillSlug: "kubernetes", Reach: ReachNoul, Aliases: []string{"Kubernetes"}},
		{SkillSlug: "k8s-orchestration", Reach: ReachLexical, Aliases: aliases("k8s-orchestration", "Helm & Container Orchestration")},
		{SkillSlug: "ci-cd", Reach: ReachLexical, Aliases: aliases("ci-cd", "CI/CD")},
	}})
	if got := matchedSlugs(m, "We template with Helm."); !slices.Equal(got, []string{"k8s-orchestration"}) {
		t.Errorf("matched %v, want a conjoined name reached by one part", got)
	}
	if got := matchedSlugs(m, "Owns CI/CD pipelines."); !slices.Equal(got, []string{"ci-cd"}) {
		t.Errorf("matched %v, want CI/CD kept whole", got)
	}
	got := matchedSlugs(m, "We run node-js on AWS; kubernetes too.")
	if !slices.Equal(got, []string{"aws", "nodejs"}) {
		t.Errorf("matched %v, want [aws nodejs] (noul entries never match lexically)", got)
	}
	if got := matchedSlugs(m, "Nodes and jobs"); len(got) != 0 {
		t.Errorf("matched %v inside other words", got)
	}
}

// corpusOf builds n documents; the first `mentions` contain phrase, and the
// first `labelled` of those carry slug as an assigned skill.
func corpusOf(n, mentions, labelled int, phrase, slug string) []Doc {
	docs := make([]Doc, n)
	for i := range docs {
		text := fmt.Sprintf("posting %d about widgets", i)
		skills := map[string]bool{}
		if i < mentions {
			text += " with " + phrase
			if i < labelled {
				skills[slug] = true
			}
		}
		docs[i] = Doc{Text: text, Skills: skills}
	}
	return docs
}

func reachOf(s Seed, slug string) Reach {
	for _, e := range s.Entries {
		if e.SkillSlug == slug {
			return e.Reach
		}
	}
	return ""
}

func TestGenerate(t *testing.T) {
	t.Run("a precise name is lexical; a common word is not", func(t *testing.T) {
		corpus := append(corpusOf(100, 30, 27, "Terraform", "terraform"), corpusOf(100, 60, 5, "communication", "communication")...)
		seed := Generate([]SkillStat{{"terraform", "Terraform", 40}, {"communication", "Communication", 500}}, corpus, DefaultParams, now)
		if reachOf(seed, "terraform") != ReachLexical {
			t.Errorf("terraform reach = %q, want lexical", reachOf(seed, "terraform"))
		}
		if reachOf(seed, "communication") != ReachNoul {
			t.Errorf("communication reach = %q, want noul", reachOf(seed, "communication"))
		}
	})

	t.Run("a name rare in this corpus is lexical, including one never seen", func(t *testing.T) {
		seed := Generate([]SkillStat{{"pulumi", "Pulumi", 1}, {"apache-flink", "Apache Flink", 0}}, corpusOf(200, 3, 0, "Pulumi", "pulumi"), DefaultParams, now)
		for _, slug := range []string{"pulumi", "apache-flink"} {
			if reachOf(seed, slug) != ReachLexical {
				t.Errorf("%s reach = %q, want lexical", slug, reachOf(seed, slug))
			}
		}
	})

	t.Run("a skill labelled often but rarely named goes to the noul head", func(t *testing.T) {
		corpus := corpusOf(100, 3, 3, "product thinking", "product-thinking")
		for i := 3; i < 40; i++ {
			corpus[i].Skills["product-thinking"] = true
		}
		seed := Generate([]SkillStat{{"product-thinking", "Product Thinking", 381}}, corpus, DefaultParams, now)
		if reachOf(seed, "product-thinking") != ReachNoul {
			t.Errorf("reach = %q, want noul", reachOf(seed, "product-thinking"))
		}
	})

	t.Run("the noul head is chosen by this install's link mass", func(t *testing.T) {
		var skills []SkillStat
		var corpus []Doc
		for i := range 5 {
			name := fmt.Sprintf("Generic Method %d", i)
			slug := fmt.Sprintf("generic-method-%d", i)
			skills = append(skills, SkillStat{slug, name, int64(10 * (i + 1))})
			corpus = append(corpus, corpusOf(40, 40, 0, name, slug)...)
		}
		p := DefaultParams
		p.NoulHeadSize = 2
		seed := Generate(skills, corpus, p, now)
		var head []string
		for _, e := range seed.Noul() {
			head = append(head, e.SkillSlug)
		}
		if !slices.Equal(head, []string{"generic-method-3", "generic-method-4"}) {
			t.Errorf("noul head = %v, want the two most-linked", head)
		}
		if seed.Stats.Unreachable != 3 {
			t.Errorf("unreachable = %d, want 3", seed.Stats.Unreachable)
		}
	})

	t.Run("a context rule applies only where its skill exists", func(t *testing.T) {
		seed := Generate([]SkillStat{{"golang", "Go", 50}, {"python", "Python", 50}}, nil, DefaultParams, now)
		rules := map[string]string{}
		for _, e := range seed.Entries {
			rules[e.SkillSlug] = e.ContextRule
		}
		if rules["golang"] != "go" || rules["python"] != "" || len(seed.Entries) != 2 || seed.Stats.RuleMatched != 1 {
			t.Errorf("entries = %+v", seed.Entries)
		}
	})
}

func TestSeedHash(t *testing.T) {
	a := Generate([]SkillStat{{"terraform", "Terraform", 1}, {"pulumi", "Pulumi", 1}}, nil, DefaultParams, now)
	b := Generate([]SkillStat{{"pulumi", "Pulumi", 9}, {"terraform", "Terraform", 9}}, nil, DefaultParams, now.Add(time.Hour))
	if a.Hash() != b.Hash() {
		t.Error("identical reach hashed differently across generation time, order, or link mass")
	}
	c := Generate([]SkillStat{{"terraform", "Terraform", 1}}, nil, DefaultParams, now)
	if a.Hash() == c.Hash() {
		t.Error("a changed seed kept its hash")
	}
	if !strings.HasPrefix(a.Hash(), "") || len(a.Hash()) != 64 {
		t.Errorf("hash %q is not a sha256 hex digest", a.Hash())
	}
}
