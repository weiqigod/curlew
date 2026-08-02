package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestWorker_E2E_Observable is the end-to-end integration test that validates
// the observable behaviour from the task YAML:
//
//	"Claimed shard shd_1 (3 requests)"
//	"Completed shd_1: pass=3 fail=0 duration=...ms"
//	"No more shards; exiting"
func TestWorker_E2E_Observable(t *testing.T) {
	// Spin up an "API under test" that returns 200 for the worker's HTTP requests.
	apiTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	t.Cleanup(apiTS.Close)

	// Build the requests payload for the shard pointing at apiTS.
	requests := []WorkerRequest{
		{Name: "r1", Method: "GET", URL: apiTS.URL + "/a"},
		{Name: "r2", Method: "GET", URL: apiTS.URL + "/b"},
		{Name: "r3", Method: "GET", URL: apiTS.URL + "/c"},
	}
	requestsJSON, err := json.Marshal(requests)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	fake, state := startFakeCoordinator(t, FakeOpts{
		Org:   "acme",
		Token: "svc_token_dev",
		Shards: []FakeShard{
			{ID: "shd_1", Index: 0, RequestsJson: string(requestsJSON)},
		},
	})

	cfg := Config{
		CoordinatorURL:    fake.URL,
		Token:             "svc_token_dev",
		JobID:             "job_abc",
		Org:               "acme",
		WorkerID:          "wkr_test",
		Concurrency:       1,
		HeartbeatInterval: 50 * time.Millisecond,
	}

	var stdout, stderr bytes.Buffer
	summary, runErr := Run(context.Background(), cfg, RunOptions{
		Stdout:            &stdout,
		Stderr:            &stderr,
		HeartbeatInterval: 50 * time.Millisecond,
	})

	// Observable assertions
	if runErr != nil {
		t.Fatalf("Run() = %v; want nil", runErr)
	}
	if summary.ShardsCompleted != 1 {
		t.Errorf("ShardsCompleted = %d; want 1", summary.ShardsCompleted)
	}
	if summary.TotalPass != 3 || summary.TotalFail != 0 {
		t.Errorf("pass/fail = %d/%d; want 3/0", summary.TotalPass, summary.TotalFail)
	}
	errOut := stderr.String()
	for _, want := range []string{
		"Claimed shard shd_1 (3 requests)",
		"Completed shd_1: pass=3 fail=0",
		"No more shards; exiting",
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q\n  got: %s", want, errOut)
		}
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty, got: %q", stdout.String())
	}

	// Coordinator-side assertions
	if state.SubmitsByShard == nil || state.SubmitsByShard["shd_1"] == nil {
		t.Fatalf("no submit recorded for shd_1")
	}
	got := state.SubmitsByShard["shd_1"]
	if got.PassCount != 3 {
		t.Errorf("submit.PassCount = %d; want 3", got.PassCount)
	}
	if got.FailCount != 0 {
		t.Errorf("submit.FailCount = %d; want 0", got.FailCount)
	}
}

// TestWorker_E2E_Unauthorized verifies exit-code-10 behaviour.
func TestWorker_E2E_Unauthorized(t *testing.T) {
	fake, _ := startFakeCoordinator(t, FakeOpts{
		Org:    "acme",
		Token:  "correct_token",
		Shards: []FakeShard{{ID: "shd_1", RequestsJson: "[]"}},
	})

	cfg := Config{
		CoordinatorURL:    fake.URL,
		Token:             "wrong_token", // mismatch → 401
		JobID:             "job_abc",
		Org:               "acme",
		WorkerID:          "wkr_test",
		Concurrency:       1,
		HeartbeatInterval: 50 * time.Millisecond,
	}

	_, runErr := Run(context.Background(), cfg, RunOptions{
		Stdout:            &bytes.Buffer{},
		HeartbeatInterval: 50 * time.Millisecond,
	})
	if runErr == nil {
		t.Fatal("Run() = nil; want ErrUnauthorized")
	}
	if !strings.Contains(runErr.Error(), "unauthorized") {
		t.Errorf("Run() error = %v; want containing 'unauthorized'", runErr)
	}
}
