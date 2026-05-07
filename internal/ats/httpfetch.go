package ats

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// Response body size caps. The 32 MiB ceiling guards against OOM from a
// pathological response; the 4 KiB cap on error snippets keeps wrapped error
// strings bounded while preserving enough body to debug 4xx/5xx replies.
const (
	maxResponseBytes = 32 * 1024 * 1024
	maxErrBodyBytes  = 4 * 1024
)

// httpFetch issues a GET against url with the given context and client, validates
// the response status, and returns the response body up to maxResponseBytes.
// Errors are wrapped with a "httpfetch: ..." prefix and include the URL but no
// caller-domain context; callers re-wrap with their own subsystem prefix.
func httpFetch(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("httpfetch: building request for %s: %w", url, err)
	}
	// Some job-board CDNs (Cloudflare) reset connections from Go's default UA.
	req.Header.Set("User-Agent", "market-scout/0.1 (job board fetcher; +https://github.com/majtom2grndctrl/market-scout)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("httpfetch: requesting %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBodyBytes)) // body read error is non-actionable; status code is the signal
		return nil, fmt.Errorf("httpfetch: unexpected status %d for %s: %s", resp.StatusCode, url, strconv.Quote(string(bytes.TrimSpace(snippet))))
	}

	// Read up to maxResponseBytes+1 so we can detect truncation by overflow:
	// reading exactly maxResponseBytes back is a legitimate response at the cap,
	// but maxResponseBytes+1 means the upstream had more to send.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("httpfetch: reading response body for %s: %w", url, err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("httpfetch: response body for %s exceeded %d bytes (got %d)", url, maxResponseBytes, len(body))
	}

	return body, nil
}
