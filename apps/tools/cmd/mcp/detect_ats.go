package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/atsdetect"
)

type detectATSRequest struct {
	CareersURL   string   `json:"careers_url"`
	ObservedURLs []string `json:"observed_urls"`
}

type detectATSMatch struct {
	ATS        string `json:"ats"`
	BoardToken string `json:"board_token"`
	SourceURL  string `json:"source_url"`
	SourceKind string `json:"source_kind"`
	Pattern    string `json:"pattern"`
}

type detectATSEnvelope struct {
	Status   string                  `json:"status"`
	Selected *detectATSMatch         `json:"selected"`
	Matches  []detectATSMatch        `json:"matches"`
	Errors   []atsdetect.ActionError `json:"errors"`
}

func detectATSHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, mcpReq mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_ = ctx

		var req detectATSRequest
		if err := mcpReq.BindArguments(&req); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("decoding detect_ats arguments: %v", err)), nil
		}

		result, err := atsdetect.DetectEvidence(req.CareersURL, req.ObservedURLs)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("detecting ATS evidence: %v", err)), nil
		}

		payload, err := json.Marshal(mapDetectATSResult(result))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encoding detect_ats result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(payload)), nil
	}
}

func mapDetectATSResult(result atsdetect.Result) detectATSEnvelope {
	env := detectATSEnvelope{
		Status:  result.Status,
		Matches: make([]detectATSMatch, 0, len(result.Matches)),
		Errors:  result.Errors,
	}
	for _, match := range result.Matches {
		env.Matches = append(env.Matches, mapDetectATSMatch(match))
	}
	if result.Selected != nil {
		selected := mapDetectATSMatch(*result.Selected)
		env.Selected = &selected
	}
	return env
}

func mapDetectATSMatch(match atsdetect.Detection) detectATSMatch {
	return detectATSMatch{
		ATS:        match.ATS,
		BoardToken: match.BoardToken,
		SourceURL:  match.SourceURL,
		SourceKind: match.SourceKind,
		Pattern:    match.Pattern,
	}
}
