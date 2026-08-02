package distributed_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/runner/distributed"
)

func makeCollection(n int) *parser.Collection {
	col := &parser.Collection{Name: "test-collection"}
	col.Requests.Items = make([]parser.RequestItem, n)
	for i := range col.Requests.Items {
		col.Requests.Items[i] = parser.RequestItem{
			Name: fmt.Sprintf("req_%d", i+1),
			Request: parser.Request{
				Method: "GET",
				URL:    "http://example.com/test",
			},
		}
	}
	return col
}

func TestRun_HappyPath12Requests4Shards(t *testing.T) {
	srv, _ := startFakeCoordinator(t, fakeCoordinatorOpts{
		org:              "acme",
		token:            "tok",
		finalWorkerCount: 4,
	})

	col := makeCollection(12)
	var stdout bytes.Buffer
	results, summary, err := distributed.Run(context.Background(), distributed.Config{
		CoordinatorURL: srv.URL,
		Token:          "tok",
		Org:            "acme",
		Workers:        4,
		CollectionSha:  "sha_abc",
		Collection:     col,
		Stdout:         &stdout,
		JoinTimeout:    5 * time.Second,
		PollInterval:   10 * time.Millisecond,
	}, distributed.Deps{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if summary == nil {
		t.Fatal("summary is nil")
	}

	out := stdout.String()

	if !strings.Contains(out, "Sharding 12 requests across 4 workers") {
		t.Errorf("stdout missing sharding line; got:\n%s", out)
	}
	if !strings.Contains(out, "Waiting for workers... 4/4 joined") {
		t.Errorf("stdout missing workers joined line; got:\n%s", out)
	}
	if !strings.Contains(out, "All shards complete") {
		t.Errorf("stdout missing completion line; got:\n%s", out)
	}

	_ = results
	// Each shard has 3 requests (12/4); 4 shards.
	if summary.Total != 12 {
		t.Errorf("summary.Total = %d, want 12", summary.Total)
	}
	if summary.Failed != 0 {
		t.Errorf("summary.Failed = %d, want 0", summary.Failed)
	}
	if summary.Passed != 12 {
		t.Errorf("summary.Passed = %d, want 12", summary.Passed)
	}
}

func TestRun_WorkerJoinTimeout(t *testing.T) {
	srv, _ := startFakeCoordinator(t, fakeCoordinatorOpts{
		org:            "acme",
		token:          "tok",
		maxWorkerCount: 1, // never reaches 4
	})

	col := makeCollection(4)
	var stdout bytes.Buffer
	_, _, err := distributed.Run(context.Background(), distributed.Config{
		CoordinatorURL: srv.URL,
		Token:          "tok",
		Org:            "acme",
		Workers:        4,
		CollectionSha:  "sha_abc",
		Collection:     col,
		Stdout:         &stdout,
		JoinTimeout:    100 * time.Millisecond,
		PollInterval:   10 * time.Millisecond,
	}, distributed.Deps{})

	if err == nil {
		t.Fatal("expected error on worker join timeout, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error message should mention timeout; got: %v", err)
	}
}

func TestRun_ShardReassignment(t *testing.T) {
	srv, _ := startFakeCoordinator(t, fakeCoordinatorOpts{
		org:                "acme",
		token:              "tok",
		finalWorkerCount:   2,
		reassignActive:     true,
		reassignShardIndex: 1, // shard index 1 (shd_2) gets reassigned first
	})

	col := makeCollection(6)
	var stdout bytes.Buffer
	_, summary, err := distributed.Run(context.Background(), distributed.Config{
		CoordinatorURL: srv.URL,
		Token:          "tok",
		Org:            "acme",
		Workers:        2,
		CollectionSha:  "sha_abc",
		Collection:     col,
		Stdout:         &stdout,
		JoinTimeout:    5 * time.Second,
		PollInterval:   10 * time.Millisecond,
	}, distributed.Deps{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "reassigned") {
		t.Errorf("stdout should mention reassignment; got:\n%s", out)
	}
	if !strings.Contains(out, "All shards complete") {
		t.Errorf("stdout missing completion line; got:\n%s", out)
	}
	if summary == nil {
		t.Fatal("summary is nil")
	}
	// Behavior 4: final result still includes the reassigned shard's outcomes.
	if summary.Total != 6 {
		t.Errorf("summary.Total = %d, want 6", summary.Total)
	}
	if summary.Passed != 6 {
		t.Errorf("summary.Passed = %d, want 6", summary.Passed)
	}
	if summary.Failed != 0 {
		t.Errorf("summary.Failed = %d, want 0", summary.Failed)
	}
}

func TestRun_AggregatedFailurePropagates(t *testing.T) {
	// Override: manipulate the fake to return one failed item.
	// We use the standard fake which returns all pass; we need to customize.
	// Use a custom HTTP handler.
	srv := startFakeCoordinatorWithFailure(t, "acme", "tok", 4, 1)

	col := makeCollection(4)
	var stdout bytes.Buffer
	_, summary, err := distributed.Run(context.Background(), distributed.Config{
		CoordinatorURL: srv.URL,
		Token:          "tok",
		Org:            "acme",
		Workers:        4,
		CollectionSha:  "sha_abc",
		Collection:     col,
		Stdout:         &stdout,
		JoinTimeout:    5 * time.Second,
		PollInterval:   10 * time.Millisecond,
	}, distributed.Deps{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if summary == nil {
		t.Fatal("summary is nil")
	}
	if summary.Failed != 1 {
		t.Errorf("summary.Failed = %d, want 1", summary.Failed)
	}
	if summary.Passed != 3 {
		t.Errorf("summary.Passed = %d, want 3", summary.Passed)
	}
}

func TestRun_AggregatedRequestResultOrderMatchesInput(t *testing.T) {
	srv, _ := startFakeCoordinator(t, fakeCoordinatorOpts{
		org:              "acme",
		token:            "tok",
		finalWorkerCount: 2,
	})

	col := makeCollection(6)
	var stdout bytes.Buffer
	results, _, err := distributed.Run(context.Background(), distributed.Config{
		CoordinatorURL: srv.URL,
		Token:          "tok",
		Org:            "acme",
		Workers:        2,
		CollectionSha:  "sha_abc",
		Collection:     col,
		Stdout:         &stdout,
		JoinTimeout:    5 * time.Second,
		PollInterval:   10 * time.Millisecond,
	}, distributed.Deps{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(results) != 6 {
		t.Fatalf("expected 6 results, got %d", len(results))
	}
	// Results should be in input order: req_1..req_6.
	for i, r := range results {
		want := fmt.Sprintf("req_%d", i+1)
		if r.Name != want {
			t.Errorf("results[%d].Name = %q, want %q", i, r.Name, want)
		}
	}
}

func TestRun_ContextCancellationStopsPolling(t *testing.T) {
	// The fake never completes — starts in running state.
	srv := startFakeCoordinatorNeverComplete(t, "acme", "tok")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	col := makeCollection(4)
	var stdout bytes.Buffer
	_, _, err := distributed.Run(ctx, distributed.Config{
		CoordinatorURL: srv.URL,
		Token:          "tok",
		Org:            "acme",
		Workers:        4,
		CollectionSha:  "sha_abc",
		Collection:     col,
		Stdout:         &stdout,
		JoinTimeout:    5 * time.Second,
		PollInterval:   10 * time.Millisecond,
	}, distributed.Deps{})

	if err == nil {
		t.Fatal("expected error on context cancellation, got nil")
	}
	if !strings.Contains(err.Error(), "context") && err != context.DeadlineExceeded {
		t.Errorf("expected context error, got: %v", err)
	}
}
