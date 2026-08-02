package prcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// RunResult holds the outcome of a pr-check run.
type RunResult struct {
	ResultID string
	State    string // "success" or "failure"
	Pass     int
	Fail     int
}

// Run orchestrates the full pr-check flow:
//  1. Validate config
//  2. Load results file
//  3. Upload results to backend (unless dry-run)
//  4. Post PR check status (unless dry-run)
//
// Returns a RunResult on success. On dry-run, prints the payloads to w.
func Run(ctx context.Context, cfg Config, w io.Writer) (*RunResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	payload, err := LoadResultsFile(cfg.ResultsFile)
	if err != nil {
		return nil, err
	}

	state := "success"
	if payload.HasFailures() {
		state = "failure"
	}

	if cfg.DryRun {
		printDryRun(w, cfg, payload, state)
		return &RunResult{State: state, Pass: payload.PassCount, Fail: payload.FailCount}, nil
	}

	client := &Client{
		BaseURL: cfg.BackendURL,
		Token:   cfg.BackendToken,
	}

	resp, err := client.UploadResults(ctx, cfg.Org, payload)
	if err != nil {
		return nil, err
	}

	prPayload := &PrCheckPayload{
		Repo:     cfg.Repo,
		PR:       cfg.PR,
		State:    state,
		ResultID: resp.ResultID,
	}
	if err := client.PostPrCheck(ctx, cfg.Org, prPayload); err != nil {
		return nil, err
	}

	return &RunResult{
		ResultID: resp.ResultID,
		State:    state,
		Pass:     payload.PassCount,
		Fail:     payload.FailCount,
	}, nil
}

// UploadConfig holds configuration for uploading in-memory results.
// Unlike Config, it does not require ResultsFile — the payload is supplied by the caller.
type UploadConfig struct {
	// BackendURL is the base URL of the backend (e.g. http://backend:5000).
	BackendURL string
	// BackendToken is the Bearer token for authentication.
	BackendToken string
	// Org is the organisation slug (required).
	Org string
	// PR is the pull-request number. Zero means no PR-check will be posted.
	PR int
	// Repo is the repo slug (e.g. owner/name). Empty means no PR-check will be posted.
	Repo string
	// HTTPClient overrides the default HTTP client (nil = use default).
	HTTPClient *http.Client
}

// UploadRun uploads an in-memory payload to the backend and optionally posts a
// PR check. It does not validate the config — callers must ensure required fields
// are non-empty. Returns the RunResult on success.
func UploadRun(ctx context.Context, cfg UploadConfig, payload *ResultsPayload) (*RunResult, error) {
	client := &Client{
		BaseURL:    cfg.BackendURL,
		Token:      cfg.BackendToken,
		HTTPClient: cfg.HTTPClient,
	}

	resp, err := client.UploadResults(ctx, cfg.Org, payload)
	if err != nil {
		return nil, err
	}

	state := "success"
	if payload.HasFailures() {
		state = "failure"
	}

	if cfg.PR > 0 && cfg.Repo != "" {
		prPayload := &PrCheckPayload{
			Repo:     cfg.Repo,
			PR:       cfg.PR,
			State:    state,
			ResultID: resp.ResultID,
		}
		if err := client.PostPrCheck(ctx, cfg.Org, prPayload); err != nil {
			return nil, err
		}
	}

	return &RunResult{
		ResultID: resp.ResultID,
		State:    state,
		Pass:     payload.PassCount,
		Fail:     payload.FailCount,
	}, nil
}

// printDryRun writes the payloads that would be sent to w.
func printDryRun(w io.Writer, cfg Config, payload *ResultsPayload, state string) {
	_, _ = fmt.Fprintln(w, "--- dry-run: results payload ---")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)

	_, _ = fmt.Fprintln(w, "--- dry-run: pr-check payload ---")
	prPayload := &PrCheckPayload{
		Repo:     cfg.Repo,
		PR:       cfg.PR,
		State:    state,
		ResultID: "(not uploaded)",
	}
	_ = enc.Encode(prPayload)
}
