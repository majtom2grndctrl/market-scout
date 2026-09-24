package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
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
const methodJev = "jev"

// postingResult is everything a run learned about one posting. Candidates are
// kept on deferred postings too, so a later re-derive can move a floor either
// way.
type postingResult struct {
	PostingID        int64        `json:"posting_id"`
	CompanyID        int64        `json:"company_id"`
	Title            string       `json:"title"`
	Outcome          outcome      `json:"outcome"`
	Reason           string       `json:"reason,omitempty"`
	Role             *label       `json:"role,omitempty"`
	RoleDistribution []scoredRole `json:"role_distribution,omitempty"`
	NoneFit          float64      `json:"none_fit_probability"`
	Model            string       `json:"model,omitempty"`
	RequestKeys      []string     `json:"request_keys,omitempty"`
}

// classifier holds what every posting in a run shares: the decider, the option
// sets read at run start, and the pinned rules.
type classifier struct {
	decider    *sharedDecider
	model      string
	roles      roleOptions
	thresholds thresholds
}

// classify runs every instrument for one posting. It is all or nothing: any
// failed Jev call fails the posting, and nothing partial is kept.
func (c *classifier) classify(ctx context.Context, p posting) postingResult {
	res := postingResult{PostingID: p.ID, CompanyID: p.CompanyID, Title: p.Title}

	state := strings.TrimSpace(maskTitle(p.Description, p.Title))
	if state == "" || state == maskPlaceholder {
		res.Outcome, res.Reason = outcomeSkipped, reasonEmptyDescription
		return res
	}

	role, err := runTournament(ctx, c.decider, c.model, state, c.roles)
	if err != nil {
		res.Outcome, res.Reason = outcomeFailed, err.Error()
		return res
	}
	res.RoleDistribution = role.Distribution
	res.NoneFit = role.NoneFit
	res.RequestKeys = role.RequestKeys

	// model is the id each response reports. One classification row carries
	// one model, so a posting answered by two dated models cannot be recorded
	// honestly and is failed for the next run to retry.
	models := slices.Compact(slices.Sorted(slices.Values(role.Models)))
	if len(models) != 1 {
		res.Outcome, res.Reason = outcomeFailed, fmt.Sprintf("%s: %s", reasonMixedModels, strings.Join(models, ", "))
		return res
	}
	res.Model = models[0]

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
	res.Outcome = outcomeWouldWrite
	return res
}
