package ats

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestHTTPFetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`hello`))
	}))
	t.Cleanup(srv.Close)

	body, err := httpFetch(t.Context(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("httpFetch: %v", err)
	}
	if string(body) != "hello" {
		t.Errorf("body: got %q, want %q", string(body), "hello")
	}
}

func TestHTTPFetch_Non2xx_IncludesStatusAndSnippet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "company not found", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	body, err := httpFetch(t.Context(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatalf("httpFetch: got nil error, want non-nil")
	}
	if body != nil {
		t.Errorf("body: got %v, want nil", body)
	}
	msg := err.Error()
	if !strings.Contains(msg, "httpfetch:") {
		t.Errorf("error %q missing httpfetch: prefix", msg)
	}
	if !strings.Contains(msg, strconv.Itoa(http.StatusNotFound)) {
		t.Errorf("error %q missing status %d", msg, http.StatusNotFound)
	}
	if !strings.Contains(msg, "company not found") {
		t.Errorf("error %q missing body snippet", msg)
	}
}

func TestHTTPFetch_AtCapBoundary_Succeeds(t *testing.T) {
	// Exactly maxResponseBytes is the legitimate ceiling — the helper reads
	// maxResponseBytes+1 to detect overflow, so a body of exactly
	// maxResponseBytes must succeed.
	if testing.Short() {
		t.Skip("skipping 32 MiB allocation in -short mode")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(make([]byte, maxResponseBytes))
	}))
	t.Cleanup(srv.Close)

	body, err := httpFetch(t.Context(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("httpFetch: %v", err)
	}
	if len(body) != maxResponseBytes {
		t.Errorf("body length: got %d, want %d", len(body), maxResponseBytes)
	}
}

func TestHTTPFetch_OverCap_ReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 32 MiB allocation in -short mode")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(make([]byte, maxResponseBytes+1))
	}))
	t.Cleanup(srv.Close)

	body, err := httpFetch(t.Context(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatalf("httpFetch: got nil error, want oversize error")
	}
	if body != nil {
		t.Errorf("body: got %d bytes, want nil on error", len(body))
	}
	msg := err.Error()
	if !strings.Contains(msg, "httpfetch:") {
		t.Errorf("error %q missing httpfetch: prefix", msg)
	}
	if !strings.Contains(msg, "exceeded") {
		t.Errorf("error %q does not mention exceeding the cap", msg)
	}
}

func TestHTTPFetch_CancelledContext_ReturnsError(t *testing.T) {
	unblock := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-unblock
	}))
	t.Cleanup(func() {
		close(unblock)
		srv.Close()
	})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	body, err := httpFetch(ctx, srv.Client(), srv.URL)
	if err == nil {
		t.Fatalf("httpFetch: got nil error, want non-nil after context cancel")
	}
	if body != nil {
		t.Errorf("body: got %v, want nil", body)
	}
	if !strings.Contains(err.Error(), "httpfetch:") {
		t.Errorf("error %q missing httpfetch: prefix", err.Error())
	}
}
