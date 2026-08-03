package prcheck

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// RunResult holds the outcome of a pr-check run.
type RunResult struct {
	State string // "success" or "failure"
	Pass  int
	Fail  int
}

// Run evaluates a results file as a CI gate:
//  1. Validate config
//  2. Load the results file
//  3. Write the summary file (unless dry-run or --summary was omitted)
//
// On dry-run, the summary that would be written is printed to w instead.
// The returned RunResult carries the verdict; the caller maps it to an exit code.
func Run(cfg Config, w io.Writer) (*RunResult, error) {
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
	summary := &CheckSummary{
		CollectionName: payload.CollectionName,
		State:          state,
		PassCount:      payload.PassCount,
		FailCount:      payload.FailCount,
		SkippedCount:   payload.SkippedCount,
		GitSha:         payload.GitSha,
	}

	switch {
	case cfg.DryRun:
		if err := writeSummary(w, summary); err != nil {
			return nil, err
		}
	case cfg.SummaryFile != "":
		f, err := os.Create(cfg.SummaryFile)
		if err != nil {
			return nil, fmt.Errorf("creating summary file: %w", err)
		}
		defer func() { _ = f.Close() }()
		if err := writeSummary(f, summary); err != nil {
			return nil, err
		}
	}

	return &RunResult{State: state, Pass: payload.PassCount, Fail: payload.FailCount}, nil
}

// writeSummary encodes the check summary as indented JSON.
func writeSummary(w io.Writer, summary *CheckSummary) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(summary); err != nil {
		return fmt.Errorf("encoding summary: %w", err)
	}
	return nil
}
