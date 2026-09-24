package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/jev"
)

// outcome is what a run did with one posting. Every selected posting gets
// exactly one.
type outcome string

const (
	// outcomeWouldWrite is a dry-run's written: the posting passed every rule
	// and a live run would save it.
	outcomeWouldWrite outcome = "would_write"
	outcomeDeferred   outcome = "deferred"
	outcomeSkipped    outcome = "skipped"
	outcomeFailed     outcome = "failed"
)

// Deferral and skip reasons.
const (
	reasonNoneFit          = "none_fit"
	reasonBelowRoleFloor   = "below_role_floor"
	reasonEmptyDescription = "empty_description"
	reasonMixedModels      = "mixed_models"
)

// posting is one selected posting with its boilerplate-cleaned description.
// The title travels only so it can be masked; it is never sent.
type posting struct {
	ID          int64
	CompanyID   int64
	Title       string
	Description string
}

// label is one assigned term and the instrument that produced it.
type label struct {
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Method      string  `json:"method"`
	Probability float64 `json:"probability,omitempty"`
}

// Label instruments, as the boundary inventory names them.
const (
	methodJev     = "jev"
	methodLexical = "lexical"
	methodRule    = "rule"
)

// candidate is one model judgment kept as evidence: a term, its dimension, and
// the probability Jev gave it. Every candidate at or above the recording floor
// is kept, labelled or not, and deferred postings keep theirs too, so a later
// re-derive can move a threshold either way.
type candidate struct {
	Dimension string `json:"dimension"`
	label
}

// Candidate dimensions. The role's pass-2 distribution is stored whole in its
// own field; these are the noul dimensions.
const (
	dimSpecialization = "specialization"
	dimSkill          = "skill"
)

// postingResult is everything a run learned about one posting.
type postingResult struct {
	PostingID        int64        `json:"posting_id"`
	CompanyID        int64        `json:"company_id"`
	Title            string       `json:"title"`
	Outcome          outcome      `json:"outcome"`
	Reason           string       `json:"reason,omitempty"`
	Role             *label       `json:"role,omitempty"`
	Specializations  []label      `json:"specializations,omitempty"`
	Skills           []label      `json:"skills,omitempty"`
	RoleDistribution []scoredRole `json:"role_distribution,omitempty"`
	NoneFit          float64      `json:"none_fit_probability"`
	Candidates       []candidate  `json:"candidates,omitempty"`
	Model            string       `json:"model,omitempty"`
	RequestKeys      []string     `json:"request_keys,omitempty"`
}

// classifier holds what every posting in a run shares: the decider, the option
// sets read at run start, and the pinned rules.
type classifier struct {
	decider    *sharedDecider
	model      string
	opts       options
	dups       duplicateFinder
	thresholds thresholds
}

// noulQuestions are the specialization and skill questions that ride on the
// pass-1 request.
func (c *classifier) noulQuestions() map[string]jev.Question {
	qs := make(map[string]jev.Question, len(c.opts.specs)+len(c.opts.skills))
	for _, t := range c.opts.specs {
		qs[specQuestionID(t.Slug)] = jev.Question{Type: jev.TypeNoul, Instructions: specInstructions(t.Name)}
	}
	for _, t := range c.opts.skills {
		qs[skillQuestionID(t.Slug)] = jev.Question{Type: jev.TypeNoul, Instructions: skillInstructions(t.Name)}
	}
	return qs
}

// classify runs every instrument for one posting. It is all or nothing: any
// failed Jev call fails the posting, and nothing partial is kept.
func (c *classifier) classify(ctx context.Context, p posting) postingResult {
	res := postingResult{PostingID: p.ID, CompanyID: p.CompanyID, Title: p.Title}
	fail := func(reason string) postingResult {
		return postingResult{PostingID: p.ID, CompanyID: p.CompanyID, Title: p.Title, Outcome: outcomeFailed, Reason: reason}
	}

	state := strings.TrimSpace(maskTitle(p.Description, p.Title))
	if state == "" || state == maskPlaceholder {
		res.Outcome, res.Reason = outcomeSkipped, reasonEmptyDescription
		return res
	}

	role, p1, err := runTournament(ctx, c.decider, c.model, state, c.opts.roles, c.noulQuestions())
	if err != nil {
		return fail(err.Error())
	}
	res.RoleDistribution = role.Distribution
	res.NoneFit = role.NoneFit
	res.RequestKeys = role.RequestKeys

	// model is the id each response reports. One classification row carries
	// one model, so a posting answered by two dated models cannot be recorded
	// honestly and is failed for the next run to retry.
	models := slices.Compact(slices.Sorted(slices.Values(role.Models)))
	if len(models) != 1 {
		return fail(fmt.Sprintf("%s: %s", reasonMixedModels, strings.Join(models, ", ")))
	}
	res.Model = models[0]

	specCands := noulCandidates(p1.Answers, c.opts.specs, specQuestionID)
	skillCands := noulCandidates(p1.Answers, c.opts.skills, skillQuestionID)
	for _, l := range recorded(specCands, c.thresholds.RecordingFloor) {
		res.Candidates = append(res.Candidates, candidate{Dimension: dimSpecialization, label: l})
	}
	for _, l := range recorded(skillCands, c.thresholds.RecordingFloor) {
		res.Candidates = append(res.Candidates, candidate{Dimension: dimSkill, label: l})
	}

	top, ok := role.top()
	switch {
	case !ok:
		res.Outcome, res.Reason = outcomeDeferred, reasonNoneFit
		return res
	case top.Probability < c.thresholds.RoleFloor:
		res.Outcome, res.Reason = outcomeDeferred, reasonBelowRoleFloor
		return res
	}
	res.Role = &label{Slug: top.Slug, Name: top.Name, Method: methodJev, Probability: top.Probability}

	// Lexical skills read the cleaned description, not the masked state: the
	// mask exists for the role question, and a skill named in the title's
	// words is still quotable evidence.
	skills := c.thresholds.Skills.apply(skillCands)
	for _, e := range c.opts.matcher.Match(p.Description) {
		skills = append(skills, label{Slug: e.SkillSlug, Name: e.SkillName, Method: methodLexical})
	}
	if res.Skills, err = dropNearDuplicates(ctx, c.dups, skills); err != nil {
		return fail(err.Error())
	}
	if res.Specializations, err = dropNearDuplicates(ctx, c.dups, c.thresholds.Specializations.apply(specCands)); err != nil {
		return fail(err.Error())
	}
	res.Outcome = outcomeWouldWrite
	return res
}
