package jev

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func choiceRequest() Request {
	return Request{
		Model: "typesafe/jev-1.13",
		State: "We are hiring someone to design our product.",
		Questions: map[string]Question{
			"role": {Type: TypeChoice, Instructions: "Which role?", Criteria: map[string]*string{
				"Product Designer": nil, "Data Engineer": nil,
			}},
		},
	}
}

const choiceResponse = `{"model":"typesafe/jev-1.13-20260917","answers":{"role":{"type":"choice","choice":"Product Designer","confidence":0.8,"probabilities":{"Product Designer":0.9,"Data Engineer":0.1}}},"usage":{"input_tokens":120,"cost":0.000005}}`

// recordSleeps returns a sleep stub that records each requested wait.
func recordSleeps(waits *[]time.Duration) Option {
	return WithSleep(func(_ context.Context, d time.Duration) error {
		*waits = append(*waits, d)
		return nil
	})
}

func TestClient_SendsRequestShapeAndDecodesDatedModel(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(choiceResponse))
	}))
	defer srv.Close()

	resp, err := New("sk-test", WithEndpoint(srv.URL)).Decide(t.Context(), choiceRequest())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if auth != "Bearer sk-test" {
		t.Errorf("Authorization = %q", auth)
	}
	crit := got["questions"].(map[string]any)["role"].(map[string]any)["criteria"].(map[string]any)
	if v, ok := crit["Product Designer"]; !ok || v != nil {
		t.Errorf("option sent as %v, want a null description", v)
	}
	if resp.Model != "typesafe/jev-1.13-20260917" {
		t.Errorf("Model = %q, want the dated id the endpoint reported", resp.Model)
	}
	if resp.Usage.Tokens() != 120 {
		t.Errorf("Tokens = %d", resp.Usage.Tokens())
	}
}

func TestClient_Retry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
	}{
		{"429 rate limited", http.StatusTooManyRequests},
		{"529 overloaded", StatusSiteOverloaded},
	} {
		t.Run(tc.name+" is retried after retry-after", func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("Retry-After", "7")
					w.WriteHeader(tc.status)
					return
				}
				_, _ = w.Write([]byte(choiceResponse))
			}))
			defer srv.Close()

			var waits []time.Duration
			_, err := New("k", WithEndpoint(srv.URL), recordSleeps(&waits)).Decide(t.Context(), choiceRequest())
			if err != nil {
				t.Fatalf("Decide: %v", err)
			}
			if calls.Load() != 2 {
				t.Errorf("calls = %d, want 2", calls.Load())
			}
			if len(waits) != 1 || waits[0] != 7*time.Second {
				t.Errorf("waits = %v, want [7s]", waits)
			}
		})
	}

	t.Run("exhausted retries return ErrRetriesExhausted", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()

		var waits []time.Duration
		_, err := New("k", WithEndpoint(srv.URL), WithMaxRetries(2), recordSleeps(&waits)).Decide(t.Context(), choiceRequest())
		if !errors.Is(err, ErrRetriesExhausted) {
			t.Fatalf("err = %v, want ErrRetriesExhausted", err)
		}
		var se *StatusError
		if !errors.As(err, &se) || se.StatusCode != http.StatusTooManyRequests {
			t.Errorf("err does not carry the last 429: %v", err)
		}
		if calls.Load() != 3 {
			t.Errorf("calls = %d, want 3", calls.Load())
		}
	})

	t.Run("other errors are not retried", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer srv.Close()

		_, err := New("k", WithEndpoint(srv.URL)).Decide(t.Context(), choiceRequest())
		if err == nil || errors.Is(err, ErrRetriesExhausted) || calls.Load() != 1 {
			t.Errorf("err = %v after %d calls, want one non-retried failure", err, calls.Load())
		}
	})
}

func TestClient_RejectsIncompleteResponse(t *testing.T) {
	for name, body := range map[string]string{
		"missing answer":      `{"model":"m","answers":{}}`,
		"missing model":       `{"answers":{"role":{"type":"choice","choice":"Data Engineer","probabilities":{"Data Engineer":1}}}}`,
		"unknown option":      `{"model":"m","answers":{"role":{"type":"choice","choice":"Chef","probabilities":{"Chef":1}}}}`,
		"distribution not ~1": `{"model":"m","answers":{"role":{"type":"choice","choice":"Data Engineer","probabilities":{"Data Engineer":0.4}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			if _, err := New("k", WithEndpoint(srv.URL)).Decide(t.Context(), choiceRequest()); err == nil {
				t.Error("Decide accepted an incomplete response")
			}
		})
	}
}
