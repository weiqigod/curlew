package prcheck

import (
	"strings"
	"time"

	"github.com/weiqigod/curlew/internal/runner"
)

// TriggerInfo captures the operator context that is not in Summary.
type TriggerInfo struct {
	// CollectionName is the name of the collection being uploaded.
	CollectionName string
	// RunAt is the timestamp of the run. Zero defaults to time.Now().UTC().
	RunAt time.Time
	// TriggeredBy identifies who triggered the run. Defaults to "cli".
	TriggeredBy string
	// GitSha is the optional git commit SHA for the run.
	GitSha string
}

// BuildPayload converts a completed run into a ResultsPayload for upload.
// Teardown failures are excluded from FailCount (they do not affect exit code semantics).
// Skipped requests are reported with status="skipped".
func BuildPayload(results []runner.RequestResult, sum *runner.Summary, info TriggerInfo) *ResultsPayload {
	runAt := info.RunAt
	if runAt.IsZero() {
		runAt = time.Now().UTC()
	}
	triggeredBy := info.TriggeredBy
	if triggeredBy == "" {
		triggeredBy = "cli"
	}

	// Exclude teardown failures from pass/fail counts.
	passCount := 0
	failCount := 0
	skippedCount := 0
	if sum != nil {
		passCount = sum.Passed
		// Subtract teardown errors so they don't inflate the failure count.
		mainFailed := sum.Failed - sum.TeardownErrors
		if mainFailed < 0 {
			mainFailed = 0
		}
		failCount = mainFailed
		skippedCount = sum.Skipped
	}

	var durationMs int64
	if sum != nil {
		durationMs = sum.Duration.Milliseconds()
	}

	items := make([]ResultItem, 0, len(results))
	for _, r := range results {
		items = append(items, resultToItem(r))
	}

	return &ResultsPayload{
		CollectionName: info.CollectionName,
		RunAt:          runAt.UTC().Format(time.RFC3339),
		DurationMs:     durationMs,
		PassCount:      passCount,
		FailCount:      failCount,
		SkippedCount:   skippedCount,
		TriggeredBy:    triggeredBy,
		GitSha:         info.GitSha,
		Items:          items,
	}
}

// resultToItem converts a single RequestResult to a ResultItem for the payload.
func resultToItem(r runner.RequestResult) ResultItem {
	item := ResultItem{
		Name: r.Name,
	}
	if r.Result != nil {
		item.DurationMs = r.Result.Duration.Milliseconds()
	}

	switch {
	case r.Skipped:
		item.Status = "skipped"
		if r.SkipReason != "" {
			msg := r.SkipReason
			item.Message = &msg
		}
	case r.Err != nil:
		item.Status = "failed"
		msg := r.Err.Error()
		item.Message = &msg
	case r.AssertionResults != nil && !r.AssertionResults.Passed:
		item.Status = "failed"
		// Collect assertion failure messages.
		var msgs []string
		for _, ar := range r.AssertionResults.Items {
			if !ar.Passed {
				msgs = append(msgs, ar.Label()+": expected "+ar.Expected+" got "+ar.Actual)
			}
		}
		if len(msgs) > 0 {
			combined := strings.Join(msgs, "; ")
			item.Message = &combined
		}
	default:
		item.Status = "passed"
	}
	return item
}
