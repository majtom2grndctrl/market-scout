// Package ats defines the adapter interface for applicant tracking systems
// and hosts per-vendor implementations.
// See: agent-context/lib/project.md
package ats

import (
	"context"
	"encoding/json"
	"time"
)

type Adapter interface {
	FetchPostings(ctx context.Context, boardToken string) ([]Posting, error)
}

type Posting struct {
	SourceID       string
	SourceURL      string
	Title          string
	LocationText   *string
	Department     *string
	Team           *string
	EmploymentType *string
	WorkplaceType  *string
	PostedAt       *time.Time
	JobURL         string
	RawData        json.RawMessage
}
