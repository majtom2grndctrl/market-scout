// Package jev is a plain net/http client for TypeSafe's Jev decision model,
// served by OpenRouter's decisions endpoint.
// See: agent-context/plans/in-progress/hybrid-classifier/index.md
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// DefaultEndpoint is OpenRouter's alpha decisions endpoint. Jev is not served
// on /chat/completions.
const DefaultEndpoint = "https://openrouter.ai/api/alpha/decisions"

// StatusSiteOverloaded is the non-standard 529 TypeSafe returns under load. It
// is retried like 429.
const StatusSiteOverloaded = 529

const (
	defaultMaxRetries = 4
	defaultBackoff    = 2 * time.Second
	// maxRetryWait caps an honoured retry-after, so a misbehaving header cannot
	// park a worker for an hour.
	maxRetryWait = 2 * time.Minute
	// maxErrorBody bounds how much of a failed response is kept in the error.
	maxErrorBody = 2048
)

// ErrRetriesExhausted marks a request that stayed rate-limited or overloaded
// through every retry. The wrapped *StatusError carries the last response.
var ErrRetriesExhausted = errors.New("jev retries exhausted")

// StatusError is a non-2xx response from the endpoint.
type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("jev returned HTTP %d: %s", e.StatusCode, e.Body)
}

// Client sends decision requests. The zero value is not usable; build one with
// New.
type Client struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
	maxRetries int
	// sleep waits between retries. Tests replace it to observe the requested
	// interval without waiting.
	sleep func(ctx context.Context, d time.Duration) error
}

// Option configures a Client.
type Option func(*Client)

// WithEndpoint points the client at another URL, such as an httptest server.
func WithEndpoint(url string) Option { return func(c *Client) { c.endpoint = url } }

// WithHTTPClient replaces the default *http.Client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithMaxRetries sets how many times a 429 or 529 is retried after the first
// attempt.
func WithMaxRetries(n int) Option { return func(c *Client) { c.maxRetries = n } }

// WithSleep replaces the retry wait.
func WithSleep(f func(ctx context.Context, d time.Duration) error) Option {
	return func(c *Client) { c.sleep = f }
}

// New builds a Client authenticated with an OpenRouter key.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		endpoint:   DefaultEndpoint,
		httpClient: &http.Client{Timeout: 90 * time.Second},
		maxRetries: defaultMaxRetries,
		sleep:      sleepCtx,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Decide sends one request and returns its validated response. A 429 or 529 is
// retried after the response's retry-after interval; any other non-2xx fails at
// once.
func (c *Client) Decide(ctx context.Context, req Request) (Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("encoding jev request: %w", err)
	}

	for attempt := 0; ; attempt++ {
		resp, retryAfter, err := c.post(ctx, body)
		if err == nil {
			if err := resp.validate(req); err != nil {
				return Response{}, err
			}
			return resp, nil
		}

		var se *StatusError
		if !errors.As(err, &se) || !retryable(se.StatusCode) {
			return Response{}, err
		}
		if attempt >= c.maxRetries {
			return Response{}, fmt.Errorf("%w after %d attempts: %w", ErrRetriesExhausted, attempt+1, err)
		}
		wait := retryAfter
		if wait <= 0 {
			wait = defaultBackoff << attempt
		}
		if err := c.sleep(ctx, min(wait, maxRetryWait)); err != nil {
			return Response{}, err
		}
	}
}

func (c *Client) post(ctx context.Context, body []byte) (Response, time.Duration, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, 0, fmt.Errorf("building jev request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Response{}, 0, fmt.Errorf("sending jev request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(httpResp.Body, maxErrorBody))
		return Response{}, parseRetryAfter(httpResp.Header.Get("Retry-After")),
			&StatusError{StatusCode: httpResp.StatusCode, Body: string(snippet)}
	}

	var resp Response
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return Response{}, 0, fmt.Errorf("decoding jev response: %w", err)
	}
	return resp, 0, nil
}

func retryable(code int) bool {
	return code == http.StatusTooManyRequests || code == StatusSiteOverloaded
}

// parseRetryAfter reads either form RFC 9110 allows: delay seconds or an HTTP
// date. An absent or unreadable header returns zero, and the caller backs off
// on its own schedule.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil && secs >= 0 {
		return time.Duration(secs * float64(time.Second))
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
