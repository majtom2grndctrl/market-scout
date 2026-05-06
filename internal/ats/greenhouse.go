package ats

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// greenhouseBaseURL is the public Greenhouse Job Board API root.
// Exposed as a var (not const) so tests can override it to point at httptest.Server.
var greenhouseBaseURL = "https://boards-api.greenhouse.io/v1/boards"

// Greenhouse is an ats.Adapter that reads from the Greenhouse public Job Board API.
// The API requires no auth and returns all jobs for a board in a single request.
type Greenhouse struct {
	client *http.Client
}

// New returns a Greenhouse adapter. If client is nil, http.DefaultClient is used.
// The adapter does not set a client-level timeout; cancellation flows from the context
// passed to FetchPostings.
func New(client *http.Client) *Greenhouse {
	if client == nil {
		client = http.DefaultClient
	}
	return &Greenhouse{client: client}
}

// ghResponse mirrors the wire shape of the Greenhouse jobs endpoint.
// Jobs are kept as raw messages so the original bytes can be preserved as RawData
// without losing fields the typed wire struct does not declare.
type ghResponse struct {
	Jobs []json.RawMessage `json:"jobs"`
}

// ghJob is the subset of the per-job wire shape the adapter extracts into Posting.
type ghJob struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	AbsoluteURL string `json:"absolute_url"`
	Location    struct {
		Name string `json:"name"`
	} `json:"location"`
	Departments []struct {
		Name string `json:"name"`
	} `json:"departments"`
}

// FetchPostings retrieves all jobs for the given Greenhouse board token and
// normalizes them into Postings. Network failures and non-2xx responses return
// a wrapped error; the adapter performs no retries.
func (g *Greenhouse) FetchPostings(ctx context.Context, boardToken string) ([]Posting, error) {
	url := fmt.Sprintf("%s/%s/jobs?content=true", greenhouseBaseURL, boardToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("greenhouse: building request for %s: %w", boardToken, err)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("greenhouse: fetching postings for %s: %w", boardToken, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("greenhouse: unexpected status %d fetching postings for %s", resp.StatusCode, boardToken)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("greenhouse: reading response body for %s: %w", boardToken, err)
	}

	var wire ghResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("greenhouse: decoding response for %s: %w", boardToken, err)
	}

	postings := make([]Posting, 0, len(wire.Jobs))
	for i, raw := range wire.Jobs {
		var job ghJob
		if err := json.Unmarshal(raw, &job); err != nil {
			return nil, fmt.Errorf("greenhouse: decoding job at index %d for %s: %w", i, boardToken, err)
		}

		idStr := strconv.FormatInt(job.ID, 10)
		posting := Posting{
			SourceID:  idStr,
			SourceURL: fmt.Sprintf("%s/%s/jobs/%s", greenhouseBaseURL, boardToken, idStr),
			Title:     job.Title,
			JobURL:    job.AbsoluteURL,
			RawData:   raw,
		}

		if job.Location.Name != "" {
			loc := job.Location.Name
			posting.LocationText = &loc
		}
		if len(job.Departments) > 0 && job.Departments[0].Name != "" {
			dept := job.Departments[0].Name
			posting.Department = &dept
		}

		postings = append(postings, posting)
	}

	return postings, nil
}
