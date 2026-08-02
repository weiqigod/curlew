// Package prcheck implements the pr-check subcommand: uploading test results
// to the ApiTool backend and posting a PR status check.
package prcheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Sentinel errors for well-known failure modes.
var (
	// ErrBackendURLMissing is returned when APITEST_BACKEND_URL is not set.
	ErrBackendURLMissing = errors.New("backend URL not configured")
	// ErrUnauthorized is returned when the backend responds with 401.
	ErrUnauthorized = errors.New("unauthorized: refresh APITEST_BACKEND_TOKEN")
	// ErrNetworkFailure is returned when the backend is unreachable after retries.
	ErrNetworkFailure = errors.New("network error: backend unreachable after retries")
)

// Config holds the pr-check configuration derived from flags and env vars.
type Config struct {
	BackendURL   string // APITEST_BACKEND_URL
	BackendToken string // APITEST_BACKEND_TOKEN
	Org          string // --org
	PR           int    // --pr
	Repo         string // --repo
	ResultsFile  string // --results
	DryRun       bool   // --dry-run
}

// ConfigFromEnv populates Config fields from environment variables.
// Flags must be set separately by the caller.
func ConfigFromEnv() Config {
	return Config{
		BackendURL:   os.Getenv("APITEST_BACKEND_URL"),
		BackendToken: os.Getenv("APITEST_BACKEND_TOKEN"),
	}
}

// Validate checks required fields. Returns an error if any are missing.
func (c *Config) Validate() error {
	if c.BackendURL == "" {
		return ErrBackendURLMissing
	}
	if c.BackendToken == "" {
		return fmt.Errorf("APITEST_BACKEND_TOKEN is required")
	}
	if c.Org == "" {
		return fmt.Errorf("--org is required")
	}
	if c.PR <= 0 {
		return fmt.Errorf("--pr must be a positive integer")
	}
	if c.Repo == "" {
		return fmt.Errorf("--repo is required")
	}
	if c.ResultsFile == "" {
		return fmt.Errorf("--results is required")
	}
	return nil
}

// ResultsPayload is the JSON body POSTed to /api/v1/organizations/{org}/results.
// Matches the backend UploadResultRequest (snake_case JSON).
type ResultsPayload struct {
	CollectionName string       `json:"collection_name"`
	RunAt          string       `json:"run_at"`
	DurationMs     int64        `json:"duration_ms"`
	PassCount      int          `json:"pass_count"`
	FailCount      int          `json:"fail_count"`
	SkippedCount   int          `json:"skipped_count"`
	TriggeredBy    string       `json:"triggered_by"`
	GitSha         string       `json:"git_sha,omitempty"`
	Items          []ResultItem `json:"items"`
}

// ResultItem is a single test case in the results payload.
type ResultItem struct {
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	DurationMs int64   `json:"duration_ms"`
	Message    *string `json:"message"`
}

// IngestResponse is the response from POST /results (202 Accepted).
type IngestResponse struct {
	ResultID string `json:"result_id"`
	Status   string `json:"status"`
}

// PrCheckPayload is the body POSTed to /api/v1/organizations/{org}/pr-checks.
type PrCheckPayload struct {
	Repo     string `json:"repo"`
	PR       int    `json:"pr"`
	State    string `json:"state"` // "success" or "failure"
	ResultID string `json:"result_id"`
}

// LoadResultsFile reads and parses a JSON results file.
// Returns the payload and whether all tests passed (fail_count == 0).
func LoadResultsFile(path string) (*ResultsPayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading results file: %w", err)
	}
	var payload ResultsPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parsing results JSON: %w", err)
	}
	if payload.CollectionName == "" {
		return nil, fmt.Errorf("results file missing collection_name")
	}
	return &payload, nil
}

// HasFailures returns true if the payload contains any failing tests.
func (p *ResultsPayload) HasFailures() bool {
	return p.FailCount > 0
}
