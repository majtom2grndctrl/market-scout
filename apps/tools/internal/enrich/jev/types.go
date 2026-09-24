package jev

import (
	"fmt"
	"math"
)

// Question types this client sends.
const (
	TypeChoice = "choice"
	TypeNoul   = "noul"
)

// Request is the decisions request body. State is sent once and every
// question is judged against it; question ids never reach the model.
type Request struct {
	Model     string              `json:"model"`
	State     string              `json:"state"`
	Questions map[string]Question `json:"questions"`
}

// Question is one typed question. For a choice question, Criteria maps each
// option key to its description; a nil description sends JSON null, which
// offers the option by its key alone.
type Question struct {
	Type         string             `json:"type"`
	Instructions string             `json:"instructions"`
	Criteria     map[string]*string `json:"criteria,omitempty"`
}

// Response is the decisions response body. Model is the dated id the endpoint
// actually served, which can differ from the requested alias.
type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// Answer is one question's answer. Choice, Confidence and Probabilities are set
// for a choice question; Noul, the probability of yes, for a noul question.
type Answer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Noul          *float64           `json:"noul,omitempty"`
}

// Usage reports what the request cost. TypeSafe documents input_tokens and
// OpenRouter documents cost; both are decoded, and prompt_tokens is read as a
// fallback because the alpha endpoint's field names are not yet pinned down.
type Usage struct {
	InputTokens  int64   `json:"input_tokens,omitempty"`
	PromptTokens int64   `json:"prompt_tokens,omitempty"`
	Cost         float64 `json:"cost,omitempty"`
}

// Tokens returns the billed input tokens under whichever name the endpoint
// used.
func (u Usage) Tokens() int64 {
	if u.InputTokens > 0 {
		return u.InputTokens
	}
	return u.PromptTokens
}

// probabilitySlack absorbs float rounding in a distribution the endpoint says
// sums to 1.
const probabilitySlack = 0.02

// validate rejects a response that does not answer every question it was
// asked, in the shape it was asked. A partial response is a failed request:
// callers never classify from a subset of answers.
func (r Response) validate(req Request) error {
	if r.Model == "" {
		return fmt.Errorf("jev response has no model id")
	}
	for id, q := range req.Questions {
		a, ok := r.Answers[id]
		if !ok {
			return fmt.Errorf("jev response has no answer for question %q", id)
		}
		switch q.Type {
		case TypeChoice:
			if len(a.Probabilities) == 0 {
				return fmt.Errorf("jev choice answer %q has no probabilities", id)
			}
			var sum float64
			for opt, p := range a.Probabilities {
				if _, known := q.Criteria[opt]; !known {
					return fmt.Errorf("jev choice answer %q names unknown option %q", id, opt)
				}
				if p < 0 || p > 1 || math.IsNaN(p) {
					return fmt.Errorf("jev choice answer %q has probability %v for %q", id, p, opt)
				}
				sum += p
			}
			if math.Abs(sum-1) > probabilitySlack {
				return fmt.Errorf("jev choice answer %q probabilities sum to %v", id, sum)
			}
		case TypeNoul:
			if a.Noul == nil || *a.Noul < 0 || *a.Noul > 1 {
				return fmt.Errorf("jev noul answer %q has no probability in [0,1]", id)
			}
		}
	}
	return nil
}
