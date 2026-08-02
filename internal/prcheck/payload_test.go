package prcheck

import (
	"fmt"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/assertion"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/runner"
)

func makePassResult(name string, durationMs int64) runner.RequestResult {
	return runner.RequestResult{
		Name:   name,
		Method: "GET",
		URL:    "http://example.com",
		Result: &httpexec.Result{
			StatusCode: 200,
			Duration:   time.Duration(durationMs) * time.Millisecond,
		},
		AssertionResults: &assertion.Results{Passed: true},
	}
}

func makeFailResult(name, _ string) runner.RequestResult {
	return runner.RequestResult{
		Name:   name,
		Method: "GET",
		URL:    "http://example.com",
		Result: &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond},
		AssertionResults: &assertion.Results{
			Passed: false,
			Items: []assertion.Result{
				{Type: "status_code", Expected: "200", Actual: "404", Passed: false},
			},
		},
	}
}

func makeErrResult(name string, err error) runner.RequestResult {
	return runner.RequestResult{
		Name:   name,
		Method: "GET",
		URL:    "http://example.com",
		Err:    err,
	}
}

func makeSkippedResult(name string) runner.RequestResult {
	return runner.RequestResult{
		Name:    name,
		Skipped: true,
	}
}

func makeTeardownResult(name string, passed bool) runner.RequestResult {
	r := runner.RequestResult{
		Name:   name,
		Phase:  runner.PhaseTeardown,
		Method: "DELETE",
		URL:    "http://example.com",
		Result: &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond},
	}
	if passed {
		r.AssertionResults = &assertion.Results{Passed: true}
	} else {
		r.AssertionResults = &assertion.Results{Passed: false}
	}
	return r
}

func TestBuildPayload(t *testing.T) {
	fixedTime := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		results       []runner.RequestResult
		summary       *runner.Summary
		info          TriggerInfo
		wantPass      int
		wantFail      int
		wantSkipped   int
		wantItemCount int
		wantState     string   // status of first item if len > 0
		wantItemNames []string // expected item names (index-matched); empty string means skip
		wantGitSha    string
		wantTriggered string
	}{
		{
			name: "all_passing",
			results: []runner.RequestResult{
				makePassResult("req1", 100),
				makePassResult("req2", 200),
				makePassResult("req3", 300),
			},
			summary:       &runner.Summary{Total: 3, Passed: 3, Failed: 0, Skipped: 0, Duration: 600 * time.Millisecond},
			info:          TriggerInfo{CollectionName: "test-suite", RunAt: fixedTime, TriggeredBy: "cli"},
			wantPass:      3,
			wantFail:      0,
			wantSkipped:   0,
			wantItemCount: 3,
			wantState:     "passed",
			wantTriggered: "cli",
		},
		{
			name: "one_failing_assertion",
			results: []runner.RequestResult{
				makePassResult("req1", 100),
				makePassResult("req2", 100),
				makeFailResult("req3", "expected 200 got 404"),
			},
			summary:       &runner.Summary{Total: 3, Passed: 2, Failed: 1, Skipped: 0, Duration: 300 * time.Millisecond},
			info:          TriggerInfo{CollectionName: "test-suite", RunAt: fixedTime, TriggeredBy: "cli"},
			wantPass:      2,
			wantFail:      1,
			wantSkipped:   0,
			wantItemCount: 3,
			wantState:     "passed",
		},
		{
			name: "skipped_request",
			results: []runner.RequestResult{
				makePassResult("req1", 100),
				makeSkippedResult("req2"),
			},
			summary:       &runner.Summary{Total: 2, Passed: 1, Failed: 0, Skipped: 1, Duration: 100 * time.Millisecond},
			info:          TriggerInfo{CollectionName: "test-suite", RunAt: fixedTime, TriggeredBy: "cli"},
			wantPass:      1,
			wantFail:      0,
			wantSkipped:   1,
			wantItemCount: 2,
		},
		{
			name: "teardown_failure_excluded_from_fail_count",
			results: []runner.RequestResult{
				makePassResult("main-req", 100),
				makeTeardownResult("teardown-req", false),
			},
			summary:       &runner.Summary{Total: 2, Passed: 1, Failed: 1, Skipped: 0, TeardownErrors: 1, TeardownAssertionErrors: 1, Duration: 200 * time.Millisecond},
			info:          TriggerInfo{CollectionName: "test-suite", RunAt: fixedTime, TriggeredBy: "cli"},
			wantPass:      1,
			wantFail:      0, // teardown failures excluded
			wantSkipped:   0,
			wantItemCount: 2, // both items included
		},
		{
			name:          "trigger_info_defaults",
			results:       []runner.RequestResult{makePassResult("req1", 100)},
			summary:       &runner.Summary{Total: 1, Passed: 1, Duration: 100 * time.Millisecond},
			info:          TriggerInfo{CollectionName: "test-suite"}, // RunAt zero, TriggeredBy empty
			wantPass:      1,
			wantTriggered: "cli", // defaults to "cli"
		},
		{
			name:       "git_sha_propagated",
			results:    []runner.RequestResult{makePassResult("req1", 100)},
			summary:    &runner.Summary{Total: 1, Passed: 1, Duration: 100 * time.Millisecond},
			info:       TriggerInfo{CollectionName: "test-suite", RunAt: fixedTime, TriggeredBy: "cli", GitSha: "abc1234"},
			wantPass:   1,
			wantGitSha: "abc1234",
		},
		{
			name: "data_driven_iteration_folded",
			results: []runner.RequestResult{
				{
					Name:             "login [1/2]",
					Method:           "POST",
					URL:              "http://example.com/login",
					IsDataDriven:     true,
					DataDrivenName:   "login",
					IterationIndex:   0,
					IterationTotal:   2,
					Result:           &httpexec.Result{StatusCode: 200, Duration: 50 * time.Millisecond},
					AssertionResults: &assertion.Results{Passed: true},
				},
				{
					Name:             "login [2/2]",
					Method:           "POST",
					URL:              "http://example.com/login",
					IsDataDriven:     true,
					DataDrivenName:   "login",
					IterationIndex:   1,
					IterationTotal:   2,
					Result:           &httpexec.Result{StatusCode: 200, Duration: 50 * time.Millisecond},
					AssertionResults: &assertion.Results{Passed: true},
				},
			},
			summary:       &runner.Summary{Total: 2, Passed: 2, Duration: 100 * time.Millisecond},
			info:          TriggerInfo{CollectionName: "test-suite", RunAt: fixedTime, TriggeredBy: "ci"},
			wantPass:      2,
			wantItemCount: 2,
			wantItemNames: []string{"login [1/2]", "login [2/2]"},
			wantTriggered: "ci",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := BuildPayload(tc.results, tc.summary, tc.info)
			if payload == nil {
				t.Fatal("BuildPayload() returned nil")
			}
			if payload.PassCount != tc.wantPass {
				t.Errorf("PassCount = %d; want %d", payload.PassCount, tc.wantPass)
			}
			if payload.FailCount != tc.wantFail {
				t.Errorf("FailCount = %d; want %d", payload.FailCount, tc.wantFail)
			}
			if payload.SkippedCount != tc.wantSkipped {
				t.Errorf("SkippedCount = %d; want %d", payload.SkippedCount, tc.wantSkipped)
			}
			if tc.wantItemCount > 0 && len(payload.Items) != tc.wantItemCount {
				t.Errorf("len(Items) = %d; want %d", len(payload.Items), tc.wantItemCount)
			}
			if tc.wantGitSha != "" && payload.GitSha != tc.wantGitSha {
				t.Errorf("GitSha = %q; want %q", payload.GitSha, tc.wantGitSha)
			}
			if tc.wantTriggered != "" && payload.TriggeredBy != tc.wantTriggered {
				t.Errorf("TriggeredBy = %q; want %q", payload.TriggeredBy, tc.wantTriggered)
			}
			if payload.RunAt == "" {
				t.Error("RunAt must not be empty")
			}
			if payload.CollectionName == "" {
				t.Error("CollectionName must not be empty")
			}
			if tc.wantState != "" && len(payload.Items) > 0 && payload.Items[0].Status != tc.wantState {
				t.Errorf("Items[0].Status = %q; want %q", payload.Items[0].Status, tc.wantState)
			}
			for i, wantName := range tc.wantItemNames {
				if wantName == "" {
					continue
				}
				if i >= len(payload.Items) {
					t.Errorf("Items[%d] does not exist; want name %q", i, wantName)
					continue
				}
				if payload.Items[i].Name != wantName {
					t.Errorf("Items[%d].Name = %q; want %q", i, payload.Items[i].Name, wantName)
				}
			}
		})
	}
}

func TestBuildPayload_ItemStatuses(t *testing.T) {
	networkErr := fmt.Errorf("connection refused")
	results := []runner.RequestResult{
		makePassResult("pass-req", 100),
		makeFailResult("fail-req", "assertion mismatch"),
		makeErrResult("err-req", networkErr),
		makeSkippedResult("skip-req"),
	}
	summary := &runner.Summary{Total: 4, Passed: 1, Failed: 2, Skipped: 1, Duration: 300 * time.Millisecond}
	info := TriggerInfo{CollectionName: "test-suite", RunAt: time.Now().UTC(), TriggeredBy: "cli"}

	payload := BuildPayload(results, summary, info)
	if payload == nil {
		t.Fatal("BuildPayload() returned nil")
	}
	if len(payload.Items) != 4 {
		t.Fatalf("len(Items) = %d; want 4", len(payload.Items))
	}

	statuses := map[string]string{}
	for _, item := range payload.Items {
		statuses[item.Name] = item.Status
	}

	if statuses["pass-req"] != "passed" {
		t.Errorf("pass-req status = %q; want passed", statuses["pass-req"])
	}
	if statuses["fail-req"] != "failed" {
		t.Errorf("fail-req status = %q; want failed", statuses["fail-req"])
	}
	if statuses["err-req"] != "failed" {
		t.Errorf("err-req status = %q; want failed", statuses["err-req"])
	}
	if statuses["skip-req"] != "skipped" {
		t.Errorf("skip-req status = %q; want skipped", statuses["skip-req"])
	}
}
