// Package prcheck implements the pr-check subcommand: turning a run's results
// file into a CI gate. It reads results produced by `curlew run`, reports the
// pass/fail state, and optionally writes a summary file for a CI job to pick
// up. Nothing is transmitted — curlew has no results backend.
package prcheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// ErrResultsFileMissing is returned when --results was not supplied.
var ErrResultsFileMissing = errors.New("--results is required")

// Config holds the pr-check configuration derived from flags.
type Config struct {
	ResultsFile string // --results
	SummaryFile string // --summary; empty = do not write one
	DryRun      bool   // --dry-run
}

// Validate checks required fields. Returns an error if any are missing.
func (c *Config) Validate() error {
	if c.ResultsFile == "" {
		return ErrResultsFileMissing
	}
	return nil
}

// ResultsPayload is the normalised shape of a run's results, as written by
// BuildPayload and read back by LoadResultsFile.
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

// CheckSummary is the compact verdict written to --summary. It is the file a
// CI job reads to decide whether to mark a build green.
type CheckSummary struct {
	CollectionName string `json:"collection_name"`
	State          string `json:"state"` // "success" or "failure"
	PassCount      int    `json:"pass_count"`
	FailCount      int    `json:"fail_count"`
	SkippedCount   int    `json:"skipped_count"`
	GitSha         string `json:"git_sha,omitempty"`
}

// runOutput is the subset of `curlew run --format json` output that pr-check
// needs. That command is the only producer of results in a backend-free CLI,
// so its output is a first-class input here.
type runOutput struct {
	Name       string `json:"name"`
	DurationMs int64  `json:"duration_ms"`
	Summary    *struct {
		Total   int `json:"total"`
		Passed  int `json:"passed"`
		Failed  int `json:"failed"`
		Skipped int `json:"skipped"`
	} `json:"summary"`
	Requests []struct {
		Name       string `json:"name"`
		Status     string `json:"status"`
		DurationMs int64  `json:"duration_ms"`
	} `json:"requests"`
}

// toPayload converts run output into the results payload shape.
func (r *runOutput) toPayload() *ResultsPayload {
	p := &ResultsPayload{
		CollectionName: r.Name,
		DurationMs:     r.DurationMs,
		TriggeredBy:    "cli",
		Items:          make([]ResultItem, 0, len(r.Requests)),
	}
	if r.Summary != nil {
		p.PassCount = r.Summary.Passed
		p.FailCount = r.Summary.Failed
		p.SkippedCount = r.Summary.Skipped
	}
	for _, req := range r.Requests {
		p.Items = append(p.Items, ResultItem{
			Name:       req.Name,
			Status:     req.Status,
			DurationMs: req.DurationMs,
		})
	}
	return p
}

// LoadResultsFile reads and parses a JSON results file. Two shapes are
// accepted: the results-payload shape (`collection_name`, `pass_count`, …) and
// the output of `curlew run --format json` (`name`, `summary`, `requests`).
func LoadResultsFile(path string) (*ResultsPayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading results file: %w", err)
	}
	var payload ResultsPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parsing results JSON: %w", err)
	}
	if payload.CollectionName != "" {
		return &payload, nil
	}

	// No collection_name: try the `curlew run --format json` shape.
	var run runOutput
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("parsing results JSON: %w", err)
	}
	if run.Name == "" || run.Summary == nil {
		return nil, fmt.Errorf("results file is neither a results payload (collection_name) nor `curlew run --format json` output (name + summary)")
	}
	return run.toPayload(), nil
}

// HasFailures returns true if the payload contains any failing tests.
func (p *ResultsPayload) HasFailures() bool {
	return p.FailCount > 0
}
