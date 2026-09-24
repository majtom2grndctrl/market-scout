package main

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
)

// runHashes identify the inputs that change between runs without a
// PromptVersion bump. Each hashes one input alone, so a change to one moves
// only its own hash and a reader can tell which input moved.
type runHashes struct {
	RoleOptions  string `json:"role_options"`
	SpecOptions  string `json:"specialization_options"`
	Seed         string `json:"seed"`
	QuestionText string `json:"question_wording"`
}

func computeHashes(o options) runHashes {
	var roles []termOption
	for _, c := range o.roles.chunks {
		for _, r := range c {
			roles = append(roles, termOption{Slug: r.Slug, Name: r.Name})
		}
	}
	return runHashes{
		RoleOptions:  hashOf(sortedTerms(roles)),
		SpecOptions:  hashOf(sortedTerms(o.specs)),
		Seed:         o.seed.Hash(),
		QuestionText: hashOf(questionWording()),
	}
}

// questionWording renders every question template with fixed placeholders, so
// the hash moves when wording changes and never with the terms asked about.
func questionWording() []string {
	return []string{
		rolePass1Instructions,
		rolePass2Instructions,
		noneFit,
		*noneFitDescription,
		specInstructions("{name}"),
		skillInstructions("{name}"),
		seniorityInstructions("{phrase}", "{context}"),
		seniorityState(posting{Title: "{title}", Description: "{description}"}),
		maskPlaceholder,
	}
}

func sortedTerms(ts []termOption) []termOption {
	out := slices.Clone(ts)
	slices.SortFunc(out, func(a, b termOption) int { return cmp.Compare(a.Slug, b.Slug) })
	return out
}

func hashOf(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
