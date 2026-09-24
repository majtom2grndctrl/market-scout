package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"unicode"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/jev"
)

// decider is what the classifier needs from the decision model. It is declared
// here, where it is used, so tests substitute a fake and *jev.Client satisfies
// it without knowing this package exists.
type decider interface {
	Decide(ctx context.Context, req jev.Request) (jev.Response, error)
}

// sharedDecider gives one answer per distinct request within a run. Jev is not
// deterministic, so two identical postings asked twice can split; asking once
// and sharing the answer keeps them together. A failed request fails every
// posting that shares it rather than being re-sent. A new run builds a new
// sharedDecider, so no answer carries across runs.
type sharedDecider struct {
	next decider

	mu    sync.Mutex
	calls map[string]*sharedCall
	usage runUsage
}

type sharedCall struct {
	done chan struct{}
	resp jev.Response
	err  error
}

// runUsage totals what the run actually sent, counting a shared request once.
type runUsage struct {
	Requests int64   `json:"requests"`
	Tokens   int64   `json:"input_tokens"`
	Cost     float64 `json:"cost_usd"`
}

func newSharedDecider(next decider) *sharedDecider {
	return &sharedDecider{next: next, calls: make(map[string]*sharedCall)}
}

// decide returns the answer for req and the request key it was shared under.
func (s *sharedDecider) decide(ctx context.Context, req jev.Request) (jev.Response, string, error) {
	key := requestKey(req)

	s.mu.Lock()
	if c, ok := s.calls[key]; ok {
		s.mu.Unlock()
		select {
		case <-c.done:
			return c.resp, key, c.err
		case <-ctx.Done():
			return jev.Response{}, key, ctx.Err()
		}
	}
	c := &sharedCall{done: make(chan struct{})}
	s.calls[key] = c
	s.mu.Unlock()

	c.resp, c.err = s.next.Decide(ctx, req)
	close(c.done)

	s.mu.Lock()
	s.usage.Requests++
	if c.err == nil {
		s.usage.Tokens += c.resp.Usage.Tokens()
		s.usage.Cost += c.resp.Usage.Cost
	}
	s.mu.Unlock()
	return c.resp, key, c.err
}

func (s *sharedDecider) totals() runUsage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage
}

// requestKey identifies a request by its state and questions with all
// whitespace removed, the text identity the parked work-unit-dedup draft
// proposed. Whitespace is removed from each string before encoding, because
// JSON would otherwise escape a newline into two visible characters.
func requestKey(req jev.Request) string {
	qs := make(map[string]jev.Question, len(req.Questions))
	for id, q := range req.Questions {
		crit := make(map[string]*string, len(q.Criteria))
		for opt, desc := range q.Criteria {
			if desc != nil {
				desc = ptr(stripSpace(*desc))
			}
			crit[stripSpace(opt)] = desc
		}
		qs[stripSpace(id)] = jev.Question{Type: q.Type, Instructions: stripSpace(q.Instructions), Criteria: crit}
	}
	// encoding/json sorts map keys, so equal question sets encode equally.
	enc, _ := json.Marshal(struct {
		State     string                  `json:"state"`
		Questions map[string]jev.Question `json:"questions"`
	}{stripSpace(req.State), qs})
	sum := sha256.Sum256(enc)
	return hex.EncodeToString(sum[:])
}

func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}
