package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/peterlindqvist/apitest/internal/httpexec"
)

// syncBuffer is a bytes.Buffer safe for concurrent use. heartbeatLoop and Run()
// write to the same Stderr writer from different goroutines, so a plain
// bytes.Buffer triggers the race detector. syncBuffer serialises all writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *syncBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *syncBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

func (sb *syncBuffer) Len() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Len()
}

// stubClient implements CoordinatorClient for testing.
type stubClient struct {
	mu           sync.Mutex
	shards       []*ShardResponse // shards to return in order; nil means no more shards (204)
	claimIdx     int
	submits      []*SubmitResultBody
	heartbeats   []string // shard IDs that received heartbeats
	submitErr    error
	claimErr     error
	heartbeatErr error // if non-nil, Heartbeat returns this error
}

func (s *stubClient) Claim(_ context.Context, _, _ string, _ *ClaimRequestBody) (*ShardResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	if s.claimIdx >= len(s.shards) {
		return nil, nil // no more shards
	}
	shard := s.shards[s.claimIdx]
	s.claimIdx++
	return shard, nil
}

func (s *stubClient) SubmitResult(_ context.Context, _, _, _ string, body *SubmitResultBody) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.submits = append(s.submits, body)
	return s.submitErr
}

func (s *stubClient) Heartbeat(_ context.Context, _, _, shardID string, _ *HeartbeatBody) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heartbeats = append(s.heartbeats, shardID)
	return s.heartbeatErr
}

func mustMarshalRequests(reqs []WorkerRequest) string {
	data, err := json.Marshal(reqs)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func makeRequests(n int) []WorkerRequest {
	reqs := make([]WorkerRequest, n)
	for i := range reqs {
		reqs[i] = WorkerRequest{Name: "r" + string(rune('0'+i+1)), Method: "GET", URL: "http://example.com"}
	}
	return reqs
}

func TestRun(t *testing.T) {
	t.Run("claim_execute_submit_one_shard_then_204", func(t *testing.T) {
		reqs := makeRequests(3)
		stub := &stubClient{
			shards: []*ShardResponse{
				{ShardID: "shd_1", JobID: "job_abc", RequestsJson: mustMarshalRequests(reqs)},
			},
		}
		var stdout, stderr bytes.Buffer
		summary, err := Run(context.Background(), Config{
			CoordinatorURL: "http://x", Token: "t", JobID: "job_abc",
			Org: "acme", Concurrency: 1, HeartbeatInterval: 50 * time.Millisecond,
		}, RunOptions{
			Client: stub,
			Execute: func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
				return &httpexec.Result{StatusCode: 200}, nil
			},
			Stdout:            &stdout,
			Stderr:            &stderr,
			HeartbeatInterval: 50 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if summary.ShardsCompleted != 1 {
			t.Errorf("ShardsCompleted = %d; want 1", summary.ShardsCompleted)
		}
		if summary.TotalPass != 3 || summary.TotalFail != 0 {
			t.Errorf("TotalPass/TotalFail = %d/%d; want 3/0", summary.TotalPass, summary.TotalFail)
		}
		errOut := stderr.String()
		for _, want := range []string{"Claimed shard shd_1 (3 requests)", "Completed shd_1: pass=3 fail=0", "No more shards; exiting"} {
			if !strings.Contains(errOut, want) {
				t.Errorf("stderr missing %q\n  got: %s", want, errOut)
			}
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
	})

	t.Run("submit_failures_recorded_as_fail", func(t *testing.T) {
		reqs := makeRequests(3)
		stub := &stubClient{
			shards: []*ShardResponse{
				{ShardID: "shd_2", JobID: "job_abc", RequestsJson: mustMarshalRequests(reqs)},
			},
		}
		var callIdx int32
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		_, err := Run(context.Background(), Config{
			CoordinatorURL: "http://x", Token: "t", JobID: "job_abc",
			Org: "acme", Concurrency: 1, HeartbeatInterval: 50 * time.Millisecond,
		}, RunOptions{
			Client: stub,
			Execute: func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
				i := atomic.AddInt32(&callIdx, 1)
				if i == 2 {
					return &httpexec.Result{StatusCode: 500}, nil
				}
				return &httpexec.Result{StatusCode: 200}, nil
			},
			Stdout:            &stdout,
			Stderr:            &stderr,
			HeartbeatInterval: 50 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if len(stub.submits) == 0 {
			t.Fatal("no submit recorded")
		}
		sub := stub.submits[0]
		if sub.PassCount != 2 || sub.FailCount != 1 {
			t.Errorf("pass/fail = %d/%d; want 2/1", sub.PassCount, sub.FailCount)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
	})

	t.Run("executor_error_recorded_as_error_status", func(t *testing.T) {
		reqs := makeRequests(1)
		stub := &stubClient{
			shards: []*ShardResponse{
				{ShardID: "shd_3", RequestsJson: mustMarshalRequests(reqs)},
			},
		}
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		_, err := Run(context.Background(), Config{
			CoordinatorURL: "http://x", Token: "t", JobID: "job_abc",
			Org: "acme", Concurrency: 1, HeartbeatInterval: 50 * time.Millisecond,
		}, RunOptions{
			Client: stub,
			Execute: func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
				return nil, errors.New("connection refused")
			},
			Stdout:            &stdout,
			Stderr:            &stderr,
			HeartbeatInterval: 50 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if len(stub.submits) == 0 {
			t.Fatal("no submit")
		}
		sub := stub.submits[0]
		if sub.FailCount != 1 {
			t.Errorf("FailCount = %d; want 1", sub.FailCount)
		}
		if len(sub.Items) == 0 || sub.Items[0].Status != "error" {
			t.Errorf("items[0].Status = %v; want 'error'", sub.Items)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
	})

	t.Run("shard_with_invalid_requests_json_continues", func(t *testing.T) {
		stub := &stubClient{
			shards: []*ShardResponse{
				{ShardID: "shd_bad", RequestsJson: "{not json}"},
			},
		}
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		summary, err := Run(context.Background(), Config{
			CoordinatorURL: "http://x", Token: "t", JobID: "job_abc",
			Org: "acme", Concurrency: 1, HeartbeatInterval: 50 * time.Millisecond,
		}, RunOptions{
			Client:            stub,
			Execute:           nil, // should not be called
			Stdout:            &stdout,
			Stderr:            &stderr,
			HeartbeatInterval: 50 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		_ = summary
		// Submit should record fail=1 with error status
		if len(stub.submits) == 0 {
			t.Fatal("no submit recorded")
		}
		sub := stub.submits[0]
		if sub.FailCount != 1 {
			t.Errorf("FailCount = %d; want 1", sub.FailCount)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
	})

	t.Run("no_shards_available_exits_zero", func(t *testing.T) {
		stub := &stubClient{
			shards: []*ShardResponse{}, // empty, first claim returns nil
		}
		var stdout, stderr bytes.Buffer
		summary, err := Run(context.Background(), Config{
			CoordinatorURL: "http://x", Token: "t", JobID: "job_abc",
			Org: "acme", Concurrency: 1, HeartbeatInterval: 50 * time.Millisecond,
		}, RunOptions{
			Client:            stub,
			Stdout:            &stdout,
			Stderr:            &stderr,
			HeartbeatInterval: 50 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if summary.ShardsClaimed != 0 {
			t.Errorf("ShardsClaimed = %d; want 0", summary.ShardsClaimed)
		}
		if !strings.Contains(stderr.String(), "No more shards; exiting") {
			t.Errorf("stderr missing 'No more shards; exiting'\n  got: %s", stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
		if len(stub.submits) != 0 {
			t.Errorf("SubmitResult called %d times; want 0", len(stub.submits))
		}
	})

	t.Run("unauthorized_propagates_err", func(t *testing.T) {
		stub := &stubClient{claimErr: ErrUnauthorized}
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		_, err := Run(context.Background(), Config{
			CoordinatorURL: "http://x", Token: "t", JobID: "job_abc",
			Org: "acme", Concurrency: 1, HeartbeatInterval: 50 * time.Millisecond,
		}, RunOptions{
			Client:            stub,
			Stdout:            &stdout,
			Stderr:            &stderr,
			HeartbeatInterval: 50 * time.Millisecond,
		})
		if !errors.Is(err, ErrUnauthorized) {
			t.Errorf("Run() error = %v; want ErrUnauthorized", err)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
	})

	t.Run("submit_failure_after_retries_logs_warning_and_continues", func(t *testing.T) {
		reqs := makeRequests(1)
		stub := &stubClient{
			shards: []*ShardResponse{
				{ShardID: "shd_5", RequestsJson: mustMarshalRequests(reqs)},
			},
			submitErr: ErrNetworkExhausted,
		}
		var stdout, stderr bytes.Buffer
		summary, err := Run(context.Background(), Config{
			CoordinatorURL: "http://x", Token: "t", JobID: "job_abc",
			Org: "acme", Concurrency: 1, HeartbeatInterval: 50 * time.Millisecond,
		}, RunOptions{
			Client: stub,
			Execute: func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
				return &httpexec.Result{StatusCode: 200}, nil
			},
			Stdout:            &stdout,
			Stderr:            &stderr,
			HeartbeatInterval: 50 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if summary.ShardsClaimed != 1 {
			t.Errorf("ShardsClaimed = %d; want 1", summary.ShardsClaimed)
		}
		if summary.ShardsCompleted != 0 {
			t.Errorf("ShardsCompleted = %d; want 0 (submit failed)", summary.ShardsCompleted)
		}
		if !strings.Contains(stderr.String(), "warning: submit failed for shd_5") {
			t.Errorf("stderr missing submit-failure warning; got: %q", stderr.String())
		}
		if strings.Contains(stdout.String(), "warning: submit failed") {
			t.Errorf("submit warning leaked to stdout; stdout: %q", stdout.String())
		}
	})

	t.Run("heartbeats_sent_during_long_shard", func(t *testing.T) {
		reqs := makeRequests(1)
		stub := &stubClient{
			shards: []*ShardResponse{
				{ShardID: "shd_hb", RequestsJson: mustMarshalRequests(reqs)},
			},
		}
		blockCh := make(chan struct{})
		var stdout bytes.Buffer
		var stderr syncBuffer
		_, err := Run(context.Background(), Config{
			CoordinatorURL: "http://x", Token: "t", JobID: "job_abc",
			Org: "acme", Concurrency: 1, HeartbeatInterval: 20 * time.Millisecond,
		}, RunOptions{
			Client: stub,
			Execute: func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
				// Block long enough to get multiple heartbeats
				timer := time.NewTimer(120 * time.Millisecond)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-blockCh:
				}
				return &httpexec.Result{StatusCode: 200}, nil
			},
			Stdout:            &stdout,
			Stderr:            &stderr,
			HeartbeatInterval: 20 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		stub.mu.Lock()
		hbCount := len(stub.heartbeats)
		stub.mu.Unlock()
		if hbCount < 2 {
			t.Errorf("heartbeat count = %d; want >= 2", hbCount)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
	})

	t.Run("heartbeat_failure_warning_uses_stderr_seam", func(t *testing.T) {
		// Verifies that heartbeatLoop writes its warning to the injected Stderr
		// writer rather than bypassing the seam via os.Stderr directly.
		// Uses syncBuffer because heartbeatLoop and Run() write to stderr
		// concurrently from separate goroutines — bytes.Buffer is not safe for
		// concurrent use and triggers the race detector.
		reqs := makeRequests(1)
		stub := &stubClient{
			shards: []*ShardResponse{
				{ShardID: "shd_hbw", RequestsJson: mustMarshalRequests(reqs)},
			},
			heartbeatErr: errors.New("timeout"),
		}
		var stdout bytes.Buffer
		var stderr syncBuffer
		_, err := Run(context.Background(), Config{
			CoordinatorURL: "http://x", Token: "t", JobID: "job_abc",
			Org: "acme", Concurrency: 1, HeartbeatInterval: 20 * time.Millisecond,
		}, RunOptions{
			Client: stub,
			Execute: func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
				// Block long enough to trigger at least one heartbeat failure.
				time.Sleep(80 * time.Millisecond)
				return &httpexec.Result{StatusCode: 200}, nil
			},
			Stdout:            &stdout,
			Stderr:            &stderr,
			HeartbeatInterval: 20 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		// The heartbeat warning must appear in the injected stderr buffer.
		if !strings.Contains(stderr.String(), "warning: heartbeat failed for shd_hbw") {
			t.Errorf("heartbeat warning not captured by Stderr seam; stderr = %q", stderr.String())
		}
	})

	t.Run("concurrency_3_runs_three_requests_in_parallel", func(t *testing.T) {
		reqs := makeRequests(6)
		stub := &stubClient{
			shards: []*ShardResponse{
				{ShardID: "shd_par", RequestsJson: mustMarshalRequests(reqs)},
			},
		}
		var inFlight int32
		var maxInFlight int32
		var mu sync.Mutex
		var stdout bytes.Buffer
		var stderr syncBuffer
		_, err := Run(context.Background(), Config{
			CoordinatorURL: "http://x", Token: "t", JobID: "job_abc",
			Org: "acme", Concurrency: 3, HeartbeatInterval: 50 * time.Millisecond,
		}, RunOptions{
			Client: stub,
			Execute: func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
				cur := atomic.AddInt32(&inFlight, 1)
				mu.Lock()
				if cur > maxInFlight {
					maxInFlight = cur
				}
				mu.Unlock()
				time.Sleep(10 * time.Millisecond)
				atomic.AddInt32(&inFlight, -1)
				return &httpexec.Result{StatusCode: 200}, nil
			},
			Stdout:            &stdout,
			Stderr:            &stderr,
			HeartbeatInterval: 50 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if maxInFlight < 2 {
			t.Errorf("maxInFlight = %d; want >= 2 (concurrency=3)", maxInFlight)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
	})

	t.Run("context_cancel_aborts_loop_cleanly", func(t *testing.T) {
		reqs := makeRequests(1)
		stub := &stubClient{
			shards: []*ShardResponse{
				{ShardID: "shd_cancel", RequestsJson: mustMarshalRequests(reqs)},
			},
		}
		ctx, cancel := context.WithCancel(context.Background())
		var stdout bytes.Buffer
		var stderr syncBuffer
		_, err := Run(ctx, Config{
			CoordinatorURL: "http://x", Token: "t", JobID: "job_abc",
			Org: "acme", Concurrency: 1, HeartbeatInterval: 50 * time.Millisecond,
		}, RunOptions{
			Client: stub,
			Execute: func(execCtx context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
				cancel() // cancel while executing
				return &httpexec.Result{StatusCode: 200}, nil
			},
			Stdout:            &stdout,
			Stderr:            &stderr,
			HeartbeatInterval: 50 * time.Millisecond,
		})
		// Should either return ctx.Err() or nil (completed just before cancel took effect)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v; want nil or context.Canceled", err)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
	})

	t.Run("two_shards_then_204", func(t *testing.T) {
		// Exercises the primary worker loop: claim shard 1 → complete → claim shard 2 → complete → 204 → exit.
		reqs1 := makeRequests(3)
		reqs2 := makeRequests(2)
		stub := &stubClient{
			shards: []*ShardResponse{
				{ShardID: "shd_a", JobID: "job_abc", RequestsJson: mustMarshalRequests(reqs1)},
				{ShardID: "shd_b", JobID: "job_abc", RequestsJson: mustMarshalRequests(reqs2)},
			},
		}
		var stdout, stderr bytes.Buffer
		summary, err := Run(context.Background(), Config{
			CoordinatorURL: "http://x", Token: "t", JobID: "job_abc",
			Org: "acme", Concurrency: 1, HeartbeatInterval: 50 * time.Millisecond,
		}, RunOptions{
			Client: stub,
			Execute: func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
				return &httpexec.Result{StatusCode: 200}, nil
			},
			Stdout:            &stdout,
			Stderr:            &stderr,
			HeartbeatInterval: 50 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if summary.ShardsCompleted != 2 {
			t.Errorf("ShardsCompleted = %d; want 2", summary.ShardsCompleted)
		}
		if summary.TotalPass != 5 || summary.TotalFail != 0 {
			t.Errorf("TotalPass/TotalFail = %d/%d; want 5/0", summary.TotalPass, summary.TotalFail)
		}
		errOut := stderr.String()
		for _, want := range []string{
			"Claimed shard shd_a (3 requests)",
			"Completed shd_a: pass=3 fail=0",
			"Claimed shard shd_b (2 requests)",
			"Completed shd_b: pass=2 fail=0",
			"No more shards; exiting",
		} {
			if !strings.Contains(errOut, want) {
				t.Errorf("stderr missing %q\n  got: %s", want, errOut)
			}
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
		// Verify both shards were submitted to the coordinator.
		stub.mu.Lock()
		submitCount := len(stub.submits)
		stub.mu.Unlock()
		if submitCount != 2 {
			t.Errorf("SubmitResult called %d times; want 2", submitCount)
		}
	})
}
