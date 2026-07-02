package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/atsdetect"
)

func TestDetectATSHandler_ResponseMapping(t *testing.T) {
	handler := detectATSHandler()
	req := newDetectATSCallRequest(map[string]any{
		"careers_url":   "https://boards.greenhouse.io/acme",
		"observed_urls": []string{"https://boards.greenhouse.io/acme/jobs"},
	})

	env, raw := callDetectATS(t, handler, req)

	if env.Status != atsdetect.StatusDetected {
		t.Fatalf("status = %q, want %q", env.Status, atsdetect.StatusDetected)
	}
	if env.Selected == nil {
		t.Fatalf("selected = nil, want greenhouse/acme")
	}
	if env.Selected.ATS != "greenhouse" || env.Selected.BoardToken != "acme" {
		t.Fatalf("selected = %+v, want greenhouse/acme", *env.Selected)
	}
	if len(env.Matches) != 1 {
		t.Fatalf("len(matches) = %d, want 1", len(env.Matches))
	}
	match := env.Matches[0]
	if match.SourceURL != "https://boards.greenhouse.io/acme" {
		t.Fatalf("source_url = %q, want careers_url evidence first", match.SourceURL)
	}
	if match.SourceKind != "careers_url" {
		t.Fatalf("source_kind = %q, want careers_url", match.SourceKind)
	}
	if match.Pattern == "" {
		t.Fatalf("pattern = empty, want stable rule label")
	}
	assertDetectATSBoundaryKeys(t, raw)
	if strings.Contains(raw, "recognized") {
		t.Fatalf("raw response leaked detector-internal recognized field: %s", raw)
	}
}

func TestDetectATSHandler_AmbiguousEvidence(t *testing.T) {
	env, _ := callDetectATS(t, detectATSHandler(), newDetectATSCallRequest(map[string]any{
		"careers_url": "https://boards.greenhouse.io/acme",
		"observed_urls": []string{
			"https://jobs.lever.co/acme",
		},
	}))

	if env.Status != atsdetect.StatusAmbiguous {
		t.Fatalf("status = %q, want %q", env.Status, atsdetect.StatusAmbiguous)
	}
	if env.Selected != nil {
		t.Fatalf("selected = %+v, want nil for ambiguous evidence", *env.Selected)
	}
	if len(env.Matches) != 2 {
		t.Fatalf("len(matches) = %d, want 2", len(env.Matches))
	}
	if env.Matches[0].ATS != "greenhouse" || env.Matches[1].ATS != "lever" {
		t.Fatalf("matches = %+v, want caller evidence order", env.Matches)
	}
}

func TestDetectATSHandler_UnsupportedEvidence(t *testing.T) {
	env, _ := callDetectATS(t, detectATSHandler(), newDetectATSCallRequest(map[string]any{
		"careers_url": "https://careers.example.com/jobs",
	}))

	if env.Status != atsdetect.StatusUnsupportedATS {
		t.Fatalf("status = %q, want %q", env.Status, atsdetect.StatusUnsupportedATS)
	}
	if env.Selected != nil {
		t.Fatalf("selected = %+v, want nil for unsupported evidence", *env.Selected)
	}
	if len(env.Matches) != 0 {
		t.Fatalf("len(matches) = %d, want 0", len(env.Matches))
	}
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", env.Errors)
	}
}

func TestDetectATSHandler_MalformedURLsReturnEnvelope(t *testing.T) {
	res, err := detectATSHandler()(t.Context(), newDetectATSCallRequest(map[string]any{
		"careers_url":   "not a url",
		"observed_urls": []string{"ftp://example.com/jobs"},
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("handler result = %+v, want non-error JSON envelope", res)
	}

	var env detectATSEnvelope
	raw := decodeDetectATSResult(t, res, &env)
	if env.Status != atsdetect.StatusInvalidInput {
		t.Fatalf("status = %q, want %q", env.Status, atsdetect.StatusInvalidInput)
	}
	if env.Selected != nil {
		t.Fatalf("selected = %+v, want nil for invalid input", *env.Selected)
	}
	if !hasATSDetectError(env.Errors, "careers_url", atsdetect.CodeInvalidURL) {
		t.Fatalf("errors = %+v, want careers_url invalid_url", env.Errors)
	}
	if !hasATSDetectError(env.Errors, "observed_urls[0]", atsdetect.CodeInvalidURL) {
		t.Fatalf("errors = %+v, want observed_urls[0] invalid_url", env.Errors)
	}
	assertDetectATSBoundaryKeys(t, raw)
}

func TestDetectATSHandler_DoesNotValidateCapturedBoardToken(t *testing.T) {
	env, _ := callDetectATS(t, detectATSHandler(), newDetectATSCallRequest(map[string]any{
		"careers_url": "https://apply.workable.com/Acme_Co",
	}))

	if env.Status != atsdetect.StatusDetected {
		t.Fatalf("status = %q, want %q", env.Status, atsdetect.StatusDetected)
	}
	if env.Selected == nil {
		t.Fatalf("selected = nil, want workable/acme_co")
	}
	if env.Selected.ATS != "workable" || env.Selected.BoardToken != "acme_co" {
		t.Fatalf("selected = %+v, want workable/acme_co", *env.Selected)
	}
}

func TestNewMCPServer_DetectATSDoesNotRequirePools(t *testing.T) {
	s := newMCPServer(dbPools{})
	tools := s.ListTools()

	tool, ok := tools["detect_ats"]
	if !ok {
		t.Fatalf("detect_ats not registered; tools=%v", toolNames(tools))
	}

	env, _ := callDetectATS(t, tool.Handler, newDetectATSCallRequest(map[string]any{
		"careers_url": "https://jobs.ashbyhq.com/acme",
	}))
	if env.Status != atsdetect.StatusDetected {
		t.Fatalf("status = %q, want %q", env.Status, atsdetect.StatusDetected)
	}
}

func newDetectATSCallRequest(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

func callDetectATS(t *testing.T, handler server.ToolHandlerFunc, req mcp.CallToolRequest) (detectATSEnvelope, string) {
	t.Helper()
	res, err := handler(t.Context(), req)
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	var env detectATSEnvelope
	raw := decodeDetectATSResult(t, res, &env)
	return env, raw
}

func decodeDetectATSResult(t *testing.T, res *mcp.CallToolResult, out any) string {
	t.Helper()
	if res == nil || len(res.Content) != 1 {
		t.Fatalf("handler result = %+v, want exactly one content block", res)
	}
	text, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("handler content = %T, want TextContent", res.Content[0])
	}
	if err := json.Unmarshal([]byte(text.Text), out); err != nil {
		t.Fatalf("decoding detect_ats envelope %q: %v", text.Text, err)
	}
	return text.Text
}

func assertDetectATSBoundaryKeys(t *testing.T, raw string) {
	t.Helper()
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}
	want := map[string]bool{
		"status":   true,
		"selected": true,
		"matches":  true,
		"errors":   true,
	}
	if len(got) != len(want) {
		t.Fatalf("top-level keys = %v, want only status/selected/matches/errors", keys(got))
	}
	for key := range want {
		if _, ok := got[key]; !ok {
			t.Fatalf("top-level keys = %v, missing %q", keys(got), key)
		}
	}

	assertDetectATSMatchKeys(t, "selected", got["selected"])

	var matches []json.RawMessage
	if err := json.Unmarshal(got["matches"], &matches); err != nil {
		t.Fatalf("matches JSON = %s, want array: %v", got["matches"], err)
	}
	for i, match := range matches {
		assertDetectATSMatchKeys(t, "matches", match)
		if i > 0 {
			break
		}
	}
}

func assertDetectATSMatchKeys(t *testing.T, field string, raw json.RawMessage) {
	t.Helper()
	if string(raw) == "null" {
		return
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%s JSON = %s, want object: %v", field, raw, err)
	}
	want := map[string]bool{
		"ats":         true,
		"board_token": true,
		"source_url":  true,
		"source_kind": true,
		"pattern":     true,
	}
	if len(got) != len(want) {
		t.Fatalf("%s keys = %v, want only ats/board_token/source_url/source_kind/pattern", field, keys(got))
	}
	for key := range want {
		if _, ok := got[key]; !ok {
			t.Fatalf("%s keys = %v, missing %q", field, keys(got), key)
		}
	}
}

func hasATSDetectError(errs []atsdetect.ActionError, path, code string) bool {
	for _, err := range errs {
		if err.Path == path && err.Code == code {
			return true
		}
	}
	return false
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}

func toolNames(tools map[string]*server.ServerTool) []string {
	out := make([]string, 0, len(tools))
	for name := range tools {
		out = append(out, name)
	}
	return out
}
