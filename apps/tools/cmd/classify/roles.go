package main

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/classify"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/jev"
)

const (
	// maxChunkSize bounds a pass-1 question. It sits well under Jev's 255-option
	// cap and inside the ~75-class range TypeSafe's docs demonstrate.
	maxChunkSize = 50
	// leadersPerChunk is how many roles each pass-1 chunk sends to pass 2.
	leadersPerChunk = 5
)

// roleOption is one live role offered by name. The slug never reaches the
// model; it maps the answer back to the taxonomy.
type roleOption struct {
	Slug string
	Name string
}

// roleOptions is the role set read once at run start. Roles minted after that
// read are not offered until the next run.
type roleOptions struct {
	chunks [][]roleOption
	byName map[string]roleOption
}

// newRoleOptions reads every live role into stable chunks: sorted by slug and
// split into the fewest balanced chunks that fit maxChunkSize, so the same role
// set always yields the same chunks. There is no pruning: every role is
// offered, because a usage floor would silently set the taxonomy.
func newRoleOptions(tax classify.Taxonomy) (roleOptions, error) {
	all := make([]roleOption, 0, len(tax.CanonicalRoles))
	byName := make(map[string]roleOption, len(tax.CanonicalRoles))
	seen := make(map[string]string, len(tax.CanonicalRoles))
	for slug, e := range tax.CanonicalRoles {
		key := strings.ToLower(strings.Join(strings.Fields(e.Name), " "))
		if key == noneFit {
			return roleOptions{}, fmt.Errorf("role %s is named %q, the abstain option's key", slug, e.Name)
		}
		// Options are keyed by name, so two roles whose names differ only by
		// case or spacing would be one option. The save gate (000040) blocks
		// such names; this guards a database that predates it.
		if other, dup := seen[key]; dup {
			return roleOptions{}, fmt.Errorf("roles %s and %s share the name %q", other, slug, e.Name)
		}
		seen[key] = slug
		opt := roleOption{Slug: slug, Name: e.Name}
		all = append(all, opt)
		byName[e.Name] = opt
	}
	if len(all) == 0 {
		return roleOptions{}, fmt.Errorf("the taxonomy has no roles to offer")
	}
	slices.SortFunc(all, func(a, b roleOption) int { return cmp.Compare(a.Slug, b.Slug) })

	n := (len(all) + maxChunkSize - 1) / maxChunkSize
	chunks := make([][]roleOption, 0, n)
	for i := range n {
		chunks = append(chunks, all[i*len(all)/n:(i+1)*len(all)/n])
	}
	return roleOptions{chunks: chunks, byName: byName}, nil
}

// choiceQuestion offers roles by name alone, plus the abstain option.
func choiceQuestion(instructions string, roles []roleOption) jev.Question {
	criteria := make(map[string]*string, len(roles)+1)
	for _, r := range roles {
		criteria[r.Name] = nil
	}
	criteria[noneFit] = noneFitDescription
	return jev.Question{Type: jev.TypeChoice, Instructions: instructions, Criteria: criteria}
}

func pass1QuestionID(i int) string { return fmt.Sprintf("role_pass1_%02d", i+1) }

const pass2QuestionID = "role_pass2"

// pass1Request puts every role chunk in one request, so the state is billed
// once. extra carries the other dimensions' questions, which ride along on
// the same state.
func pass1Request(model, state string, opts roleOptions, extra map[string]jev.Question) jev.Request {
	qs := make(map[string]jev.Question, len(opts.chunks)+len(extra))
	for i, c := range opts.chunks {
		qs[pass1QuestionID(i)] = choiceQuestion(rolePass1Instructions, c)
	}
	maps.Copy(qs, extra)
	return jev.Request{Model: model, State: state, Questions: qs}
}

// scoredRole is a role with the probability one answer gave it.
type scoredRole struct {
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Probability float64 `json:"probability"`
}

// ranked returns an answer's roles by probability, highest first, with the
// abstain option split out. Ties break on name so the order is stable.
func ranked(a jev.Answer, opts roleOptions) (roles []scoredRole, noneFitP float64) {
	for name, p := range a.Probabilities {
		if name == noneFit {
			noneFitP = p
			continue
		}
		o := opts.byName[name]
		roles = append(roles, scoredRole{Slug: o.Slug, Name: o.Name, Probability: p})
	}
	slices.SortFunc(roles, func(a, b scoredRole) int {
		if c := cmp.Compare(b.Probability, a.Probability); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return roles, noneFitP
}

// pass1Leaders takes each chunk's top roles. A chunk whose answer favours
// none_fit still sends its leaders: pass 2 decides across chunks.
func pass1Leaders(resp jev.Response, opts roleOptions) []roleOption {
	var leaders []roleOption
	for i := range opts.chunks {
		roles, _ := ranked(resp.Answers[pass1QuestionID(i)], opts)
		for _, r := range roles[:min(leadersPerChunk, len(roles))] {
			leaders = append(leaders, roleOption{Slug: r.Slug, Name: r.Name})
		}
	}
	return leaders
}

// roleResult is the outcome of the tournament for one posting.
type roleResult struct {
	// Distribution is the full pass-2 distribution over the leaders, highest
	// first. Stored whole so a blend is measurable from the gap between the top
	// two, and so thresholds can be re-applied later.
	Distribution []scoredRole
	NoneFit      float64
	Models       []string
	RequestKeys  []string
}

// top is the most probable role, or false when pass 2 favoured none_fit.
func (r roleResult) top() (scoredRole, bool) {
	if len(r.Distribution) == 0 || r.NoneFit >= r.Distribution[0].Probability {
		return scoredRole{}, false
	}
	return r.Distribution[0], true
}

// runTournament asks pass 1 over every chunk, plus extra, then pass 2 over the
// leaders. It returns the pass-1 response so the caller reads extra's answers.
func runTournament(ctx context.Context, d *sharedDecider, model, state string, opts roleOptions, extra map[string]jev.Question) (roleResult, jev.Response, error) {
	p1, key1, err := d.decide(ctx, pass1Request(model, state, opts, extra))
	if err != nil {
		return roleResult{}, jev.Response{}, fmt.Errorf("pass 1: %w", err)
	}
	leaders := pass1Leaders(p1, opts)
	p2Req := jev.Request{Model: model, State: state, Questions: map[string]jev.Question{
		pass2QuestionID: choiceQuestion(rolePass2Instructions, leaders),
	}}
	p2, key2, err := d.decide(ctx, p2Req)
	if err != nil {
		return roleResult{}, jev.Response{}, fmt.Errorf("role pass 2: %w", err)
	}
	dist, nf := ranked(p2.Answers[pass2QuestionID], opts)
	return roleResult{
		Distribution: dist,
		NoneFit:      nf,
		Models:       []string{p1.Model, p2.Model},
		RequestKeys:  []string{key1, key2},
	}, p1, nil
}
