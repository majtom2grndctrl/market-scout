// Package skillseed owns the unattended classifier's skill reach: which live
// skills a lexical match can find, which a decision-model noul judges, and the
// context rules that disambiguate short names. Each install generates its own
// seed from its own taxonomy; only the context rules ship as curated data.
// See: agent-context/plans/in-progress/hybrid-classifier/index.md
package skillseed

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"
)

// Reach is how the classifier can assign a skill.
type Reach string

const (
	// ReachLexical skills are assigned when an alias, or their context rule,
	// matches the description.
	ReachLexical Reach = "lexical"
	// ReachNoul skills are judged one yes/no question each.
	ReachNoul Reach = "noul"
)

// Entry is one reachable skill. Skills in no entry are unreachable until a
// regenerated seed reaches them.
type Entry struct {
	SkillSlug string   `json:"skill_slug"`
	SkillName string   `json:"skill_name"`
	Reach     Reach    `json:"reach"`
	Aliases   []string `json:"aliases,omitempty"`
	// ContextRule names a curated rule. A skill with a rule is matched by the
	// rule alone; its plain aliases never match on their own.
	ContextRule string `json:"context_rule,omitempty"`
	// LinkMass is the skill's link count when the seed was generated. It chose
	// the noul head and is kept so a reader can see why.
	LinkMass int64 `json:"link_mass"`
}

// Seed is a generated seed table.
type Seed struct {
	GeneratedAt time.Time `json:"generated_at"`
	Params      Params    `json:"params"`
	Stats       Stats     `json:"stats"`
	Entries     []Entry   `json:"entries"`
}

// Stats summarize what the generator saw, for the operator.
type Stats struct {
	LiveSkills  int `json:"live_skills"`
	CorpusDocs  int `json:"corpus_docs"`
	Lexical     int `json:"lexical"`
	RuleMatched int `json:"rule_matched"`
	NoulHead    int `json:"noul_head"`
	Unreachable int `json:"unreachable"`
}

// Hash identifies the seed's reach: entries only, sorted, so regenerating an
// identical seed at another time yields the same hash. Each run records it and
// warns when it changes under one prompt version.
func (s Seed) Hash() string {
	entries := slices.Clone(s.Entries)
	slices.SortFunc(entries, func(a, b Entry) int { return cmp.Compare(a.SkillSlug, b.SkillSlug) })
	for i := range entries {
		entries[i].LinkMass = 0
	}
	b, _ := json.Marshal(entries)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Noul returns the noul head in a stable order.
func (s Seed) Noul() []Entry {
	var out []Entry
	for _, e := range s.Entries {
		if e.Reach == ReachNoul {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b Entry) int { return cmp.Compare(a.SkillSlug, b.SkillSlug) })
	return out
}

// Load reads a seed file.
func Load(path string) (Seed, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Seed{}, fmt.Errorf("reading seed: %w", err)
	}
	var s Seed
	if err := json.Unmarshal(b, &s); err != nil {
		return Seed{}, fmt.Errorf("decoding seed %s: %w", path, err)
	}
	for _, e := range s.Entries {
		if e.ContextRule != "" {
			if _, ok := ruleByKey[e.ContextRule]; !ok {
				return Seed{}, fmt.Errorf("seed %s names unknown context rule %q for %s", path, e.ContextRule, e.SkillSlug)
			}
		}
	}
	return s, nil
}

// Save writes a seed file.
func (s Seed) Save(path string) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding seed: %w", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing seed: %w", err)
	}
	return nil
}
