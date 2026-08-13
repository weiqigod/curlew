package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"time"

	"github.com/weiqigod/curlew/internal/output"
	"github.com/weiqigod/curlew/internal/runner"
)

// exitCodeSeverity maps exit codes to their severity rank for comparison.
// Higher rank = more severe. Ordering: 0 < 4 < 2 < 5 < 3 < 1 < 6.
var exitCodeSeverity = map[int]int{
	0: 0, // success
	4: 1, // network error
	2: 2, // guard rail
	5: 3, // config/var error
	3: 4, // collection parse error
	1: 5, // assertion failure
	6: 6, // feature gate
}

// worseExitCode returns the more-severe of two exit codes using the
// severity ordering 0 < 4 < 2 < 5 < 3 < 1 < 6.
func worseExitCode(a, b int) int {
	sa, ok := exitCodeSeverity[a]
	if !ok {
		sa = 0
	}
	sb, ok := exitCodeSeverity[b]
	if !ok {
		sb = 0
	}
	if sb > sa {
		return b
	}
	return a
}

// aggregateExitCodes reduces per-collection exit codes to a single code
// using the worst-wins severity ordering.
func aggregateExitCodes(codes []int) int {
	worst := 0
	for _, c := range codes {
		worst = worseExitCode(worst, c)
	}
	return worst
}

// aggregateSummaries folds per-collection summaries into one combined summary.
// LimitExceeded is true if any single collection tripped the guard rail.
// RequestsExecuted is the sum across all collections.
func aggregateSummaries(items []*runner.Summary) *runner.Summary {
	result := &runner.Summary{}
	for _, s := range items {
		if s == nil {
			continue
		}
		result.Total += s.Total
		result.Passed += s.Passed
		result.Failed += s.Failed
		result.Skipped += s.Skipped
		result.AssertionFailures += s.AssertionFailures
		result.TeardownErrors += s.TeardownErrors
		result.TeardownAssertionErrors += s.TeardownAssertionErrors
		result.Duration += s.Duration
		result.RequestsExecuted += s.RequestsExecuted
		if s.LimitExceeded {
			result.LimitExceeded = true
		}
	}
	return result
}

// collectionOutcome holds the fully-rendered result of running one
// collection within a glob batch.
type collectionOutcome struct {
	Path     string
	Name     string
	ExitCode int
	Summary  *runner.Summary
	// jsonDoc is non-nil when --format json is in effect.
	jsonDoc *output.JSONOutput
}

// buildMultiJSONOutput constructs a MultiJSONOutput from a slice of outcomes.
func buildMultiJSONOutput(outcomes []collectionOutcome, totalDuration time.Duration) *output.MultiJSONOutput {
	collections := make([]output.JSONOutput, 0, len(outcomes))
	passed, failed := 0, 0
	for _, o := range outcomes {
		if o.jsonDoc != nil {
			collections = append(collections, *o.jsonDoc)
		}
		if o.ExitCode == 0 {
			passed++
		} else {
			failed++
		}
	}

	status := "passed"
	if failed > 0 {
		status = "failed"
	}

	return &output.MultiJSONOutput{
		Status:      status,
		DurationMs:  totalDuration.Milliseconds(),
		Collections: collections,
		Total:       len(outcomes),
		Passed:      passed,
		Failed:      failed,
	}
}

// runDiscoveredCollections executes each matched collection file sequentially,
// accumulates results, and returns an aggregated exit code and summary.
// stdout and stderr are threaded through to each per-collection runCmdInner
// call; for JSON format each collection uses its own buffer so the per-collection
// documents can be parsed and re-emitted as a single MultiJSON envelope.
func runDiscoveredCollections(
	matches []string,
	envName, format, report string,
	cliVars, envVarVars map[string]string,
	seed *int64,
	color colorMode,
	verbosity output.Verbosity,
	allowSensitive, showDeps, dryRun, runParallel, confirmLargeDS bool,
	stdout, stderr io.Writer,
) (int, *runner.Summary) {
	start := time.Now()
	outcomes := make([]collectionOutcome, 0, len(matches))
	exitCodes := make([]int, 0, len(matches))

	for _, path := range matches {
		// Build args for this collection (the path is always the first positional arg).
		args := buildArgsForCollection(path, envName, format, report, cliVars, envVarVars, seed,
			color, verbosity, allowSensitive, showDeps, dryRun, runParallel, confirmLargeDS)

		// For JSON format, capture stdout so we can parse the per-collection JSON.
		var collJSON *output.JSONOutput
		if format == "json" {
			captured, capturedSummary, code := captureJSONCollection(args, stderr)
			collJSON = captured
			o := collectionOutcome{
				Path:     path,
				Name:     filepath.Base(path),
				ExitCode: code,
				Summary:  capturedSummary,
				jsonDoc:  collJSON,
			}
			outcomes = append(outcomes, o)
			exitCodes = append(exitCodes, code)
			continue
		}

		// Non-JSON: run inline. Collection header is printed by runCmdInner.
		code, summary := runCmdInner(args, stdout, stderr)
		o := collectionOutcome{
			Path:     path,
			Name:     filepath.Base(path),
			ExitCode: code,
			Summary:  summary,
		}
		outcomes = append(outcomes, o)
		exitCodes = append(exitCodes, code)
	}

	// Aggregate summaries once and reuse for both output and return value.
	summaries := make([]*runner.Summary, 0, len(outcomes))
	for _, o := range outcomes {
		summaries = append(summaries, o.Summary)
	}
	aggSummary := aggregateSummaries(summaries)

	// Emit aggregated output per format.
	switch format {
	case "json":
		totalDuration := time.Since(start)
		multiOut := buildMultiJSONOutput(outcomes, totalDuration)
		if err := output.WriteMultiJSON(stdout, multiOut); err != nil {
			_, _ = fmt.Fprintf(stderr, "json encode error: %v\n", err)
			return 1, nil
		}
	case "", "terminal":
		useColor := shouldUseColor(stdout, color)
		sumOut := output.NewPrinter(stdout, useColor)
		sumOut.SummaryWithDuration(aggSummary.Total, aggSummary.Passed, aggSummary.Failed, aggSummary.Skipped, aggSummary.Duration)
	}

	return aggregateExitCodes(exitCodes), aggSummary
}

// buildArgsForCollection reconstructs the args slice for a single collection path,
// using the already-parsed flag values.
func buildArgsForCollection(
	path, envName, format, report string,
	cliVars, envVarVars map[string]string,
	seed *int64,
	color colorMode,
	verbosity output.Verbosity,
	allowSensitive, showDeps, dryRun, runParallel, confirmLargeDS bool,
) []string {
	args := []string{path}
	if envName != "" {
		args = append(args, "--env", envName)
	}
	if format != "" {
		args = append(args, "--format", format)
	}
	if report != "" {
		args = append(args, "--report", report)
	}
	cliVarKeys := make([]string, 0, len(cliVars))
	for k := range cliVars {
		cliVarKeys = append(cliVarKeys, k)
	}
	sort.Strings(cliVarKeys)
	for _, k := range cliVarKeys {
		args = append(args, "--var", k+"="+cliVars[k])
	}
	envVarKeys := make([]string, 0, len(envVarVars))
	for k := range envVarVars {
		envVarKeys = append(envVarKeys, k)
	}
	sort.Strings(envVarKeys)
	for _, k := range envVarKeys {
		// Re-pass as key=value since the value was already resolved.
		args = append(args, "--var", k+"="+envVarVars[k])
	}
	if seed != nil {
		args = append(args, "--seed", fmt.Sprintf("%d", *seed))
	}
	// Forwarded whenever it is not the default, so a `--color=always` on the
	// outer invocation reaches each discovered collection. Previously only the
	// "off" case could be forwarded at all.
	if color != colorAuto {
		args = append(args, "--color="+color.String())
	}
	switch verbosity {
	case output.VerbosityVerbose:
		args = append(args, "-v")
	case output.VerbosityDebug:
		args = append(args, "-vv")
	case output.VerbosityQuiet:
		args = append(args, "-q")
	}
	if allowSensitive {
		args = append(args, "--allow-sensitive")
	}
	if showDeps {
		args = append(args, "--show-dependencies")
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	if runParallel {
		args = append(args, "--parallel")
	}
	if confirmLargeDS {
		args = append(args, "--confirm-large-dataset")
	}
	return args
}

// captureJSONCollection runs one collection in JSON format into an in-memory
// buffer and returns the parsed JSON document, summary, and exit code. No
// process-global file-descriptor manipulation — safe to invoke concurrently.
func captureJSONCollection(args []string, stderr io.Writer) (*output.JSONOutput, *runner.Summary, int) {
	var buf bytes.Buffer
	code, summary := runCmdInner(args, &buf, stderr)

	var jsonOut output.JSONOutput
	if jsonErr := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &jsonOut); jsonErr != nil {
		// Return nil doc — will be omitted from multi-output.
		return nil, summary, code
	}
	return &jsonOut, summary, code
}
