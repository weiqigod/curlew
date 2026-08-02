package schedule_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/backend"
	"github.com/weiqigod/curlew/internal/worker/schedule"
)

// fakeHTTPClient is a fake that implements HTTPClient with scripted responses.
type fakeHTTPClient struct {
	nextRunResponses []nextRunResponse
	heartbeatErr     error
	postResults      []postResult
	nextRunIdx       int
	postIdx          int
}

type nextRunResponse struct {
	body  *schedule.NextRunResponse
	found bool
	err   error
}

type postResult struct {
	err error
}

func (f *fakeHTTPClient) GetJSONOptional(_ context.Context, _, _ string, out any) (bool, error) {
	if f.nextRunIdx >= len(f.nextRunResponses) {
		// Default: no more runs
		return false, nil
	}
	r := f.nextRunResponses[f.nextRunIdx]
	f.nextRunIdx++
	if r.err != nil {
		return false, r.err
	}
	if !r.found {
		return false, nil
	}
	if nr, ok := out.(*schedule.NextRunResponse); ok {
		*nr = *r.body
	}
	return true, nil
}

func (f *fakeHTTPClient) PostJSON(_ context.Context, path, _ string, _, _ any) error {
	// Heartbeat and PostResult both use PostJSON.
	if strings.Contains(path, "/heartbeat") {
		return f.heartbeatErr
	}
	// PostResult: if out of scripted responses, repeat the last one.
	if len(f.postResults) == 0 {
		return nil
	}
	if f.postIdx >= len(f.postResults) {
		return f.postResults[len(f.postResults)-1].err
	}
	r := f.postResults[f.postIdx]
	f.postIdx++
	return r.err
}

// fakeCollectionExecutor is a CollectionExecutor that returns canned outcomes.
type fakeCollectionExecutor struct {
	outcome *schedule.ExecutionOutcome
	err     error
	called  bool
}

func (f *fakeCollectionExecutor) Execute(_ context.Context, _ string, _ map[string]string) (*schedule.ExecutionOutcome, error) {
	f.called = true
	return f.outcome, f.err
}

// makeTestRunner builds a Runner with fakes and a temp-dir queue.
func makeTestRunner(t *testing.T, client *fakeHTTPClient, exec *fakeCollectionExecutor) (*schedule.Runner, *bytes.Buffer, *bytes.Buffer, *schedule.Queue) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	q := &schedule.Queue{Dir: t.TempDir()}
	r := &schedule.Runner{
		Client:            &schedule.Client{HTTP: client, AccessToken: "tok"},
		Queue:             q,
		Executor:          exec,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour, // never fires in unit tests
		PostMaxElapsed:    time.Millisecond,
		WorkingDir:        t.TempDir(),
		Stdout:            &stdout,
		Stderr:            &stderr,
		Now:               time.Now,
	}
	return r, &stdout, &stderr, q
}

func claimResponse(runID, collectionRef string) nextRunResponse {
	return nextRunResponse{
		found: true,
		body: &schedule.NextRunResponse{
			RunID:         runID,
			ScheduleID:    "sched_1",
			CollectionRef: collectionRef,
			EnvVars:       map[string]string{},
			ClaimToken:    "claim-tok-1",
			Deadline:      time.Now().Add(5 * time.Minute),
		},
	}
}

func TestRunner_RunOnce(t *testing.T) {
	tests := []struct {
		name             string
		clientNextRuns   []nextRunResponse
		clientPostResult []postResult
		queueSeed        []schedule.Pending
		executor         *fakeCollectionExecutor
		wantStdout       []string
		wantStderr       []string
		wantQueueLen     int
		wantErr          error
		wantExecCalled   bool
	}{
		{
			name: "happy path: claim → execute → post",
			clientNextRuns: []nextRunResponse{
				claimResponse("run_x", "file:./testdata/api.yaml"),
			},
			clientPostResult: []postResult{{err: nil}},
			executor: &fakeCollectionExecutor{
				outcome: &schedule.ExecutionOutcome{PassCount: 3, FailCount: 0, DurationMs: 500},
			},
			wantStdout: []string{
				"claimed run run_x",
				"executing collection file:./testdata/api.yaml",
				"posted result for run_x",
				"pass=3",
				"fail=0",
			},
			wantExecCalled: true,
			wantQueueLen:   0,
		},
		{
			name: "204 from poll is silent; cycle returns nil",
			clientNextRuns: []nextRunResponse{
				{found: false},
			},
			executor:       &fakeCollectionExecutor{},
			wantQueueLen:   0,
			wantExecCalled: false,
		},
		{
			name: "git ref is rejected; result posted with fail=1",
			clientNextRuns: []nextRunResponse{
				claimResponse("run_git", "git:https://github.com/org/repo.git"),
			},
			clientPostResult: []postResult{{err: nil}},
			executor:         &fakeCollectionExecutor{},
			wantStderr:       []string{"git: collection refs are not yet supported in M16"},
			wantExecCalled:   false,
			wantQueueLen:     0,
		},
		{
			name: "post fails with network error; payload enqueued",
			clientNextRuns: []nextRunResponse{
				claimResponse("run_net", "file:./testdata/api.yaml"),
			},
			clientPostResult: []postResult{{err: backend.ErrNetworkFailure}},
			executor: &fakeCollectionExecutor{
				outcome: &schedule.ExecutionOutcome{PassCount: 1, FailCount: 0, DurationMs: 100},
			},
			wantStderr:     []string{"backend unreachable; queued result locally"},
			wantExecCalled: true,
			wantQueueLen:   1,
		},
		{
			name: "queue drained on next cycle",
			clientNextRuns: []nextRunResponse{
				{found: false}, // no new runs
			},
			clientPostResult: []postResult{{err: nil}}, // drain succeeds
			queueSeed: []schedule.Pending{
				{RunID: "run_old", Payload: &schedule.ResultRequest{ClaimToken: "x", RunAt: time.Now(), PassCount: 1}},
			},
			executor:       &fakeCollectionExecutor{},
			wantExecCalled: false,
			wantQueueLen:   0, // drained
		},
		{
			name: "queue already-completed is removed silently",
			clientNextRuns: []nextRunResponse{
				{found: false},
			},
			clientPostResult: []postResult{{err: schedule.ErrAlreadyCompleted}},
			queueSeed: []schedule.Pending{
				{RunID: "run_dup", Payload: &schedule.ResultRequest{ClaimToken: "x", RunAt: time.Now()}},
			},
			executor:       &fakeCollectionExecutor{},
			wantExecCalled: false,
			wantQueueLen:   0, // server had it; removed
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeHTTPClient{
				nextRunResponses: tc.clientNextRuns,
				postResults:      tc.clientPostResult,
			}

			runner, stdoutBuf, stderrBuf, q := makeTestRunner(t, fake, tc.executor)

			// Seed queue if needed.
			for _, p := range tc.queueSeed {
				if _, err := q.Enqueue(p.RunID, p.Payload); err != nil {
					t.Fatalf("seed queue: %v", err)
				}
			}

			err := runner.RunOnce(context.Background())

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("err = %v; want %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			out := stdoutBuf.String()
			errOut := stderrBuf.String()

			for _, want := range tc.wantStdout {
				if !strings.Contains(out, want) {
					t.Errorf("stdout missing %q\ngot:\n%s", want, out)
				}
			}
			for _, want := range tc.wantStderr {
				if !strings.Contains(errOut, want) {
					t.Errorf("stderr missing %q\ngot:\n%s", want, errOut)
				}
			}

			if tc.executor.called != tc.wantExecCalled {
				t.Errorf("executor.called = %v; want %v", tc.executor.called, tc.wantExecCalled)
			}

			qItems, err := q.List()
			if err != nil {
				t.Fatalf("Queue.List: %v", err)
			}
			if len(qItems) != tc.wantQueueLen {
				t.Errorf("queue len = %d; want %d", len(qItems), tc.wantQueueLen)
			}
		})
	}
}

// TestRunner_Run_CancelledContext verifies that Run returns context.Canceled
// when the context is cancelled.
func TestRunner_Run_CancelledContext(t *testing.T) {
	fake := &fakeHTTPClient{
		// Poll returns 204 so no execution happens.
		nextRunResponses: []nextRunResponse{{found: false}},
	}
	runner, _, _, _ := makeTestRunner(t, fake, &fakeCollectionExecutor{})
	runner.PollInterval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := runner.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() with cancelled ctx: err = %v; want context.Canceled", err)
	}
}

// TestRunner_Defaults verifies that zero-value Runner fields are filled in.
func TestRunner_Defaults(t *testing.T) {
	fake := &fakeHTTPClient{
		nextRunResponses: []nextRunResponse{{found: false}},
	}
	// Runner with zero-value durations and writers.
	q := &schedule.Queue{Dir: t.TempDir()}
	r := &schedule.Runner{
		Client:   &schedule.Client{HTTP: fake, AccessToken: "tok"},
		Queue:    q,
		Executor: &fakeCollectionExecutor{},
		// PollInterval, HeartbeatInterval, PostMaxElapsed all zero → defaults applied.
		// Stdout/Stderr nil → io.Discard applied.
		// Now nil → time.Now applied.
		WorkingDir: t.TempDir(),
	}
	// RunOnce must not panic with nil writers.
	err := r.RunOnce(context.Background())
	if err != nil {
		t.Errorf("RunOnce with defaults: %v", err)
	}
}

// TestRunner_DrainQueue_NetworkFailure verifies that a network failure during
// drain stops draining for this cycle but does not return an error.
func TestRunner_DrainQueue_NetworkFailure(t *testing.T) {
	fake := &fakeHTTPClient{
		nextRunResponses: []nextRunResponse{{found: false}},
		postResults:      []postResult{{err: backend.ErrNetworkFailure}},
	}
	runner, _, stderrBuf, q := makeTestRunner(t, fake, &fakeCollectionExecutor{})

	// Seed two items in the queue.
	p := &schedule.ResultRequest{ClaimToken: "x", RunAt: time.Now()}
	if _, err := q.Enqueue("run_old1", p); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := q.Enqueue("run_old2", p); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Run one cycle — drain should stop on first network failure.
	if err := runner.RunOnce(context.Background()); err != nil {
		t.Errorf("RunOnce: %v", err)
	}

	// Both items should still be in the queue (drain stopped).
	items, err := q.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) < 1 {
		t.Errorf("expected items to remain in queue; got %d", len(items))
	}
	if !strings.Contains(stderrBuf.String(), "backend still unreachable") {
		t.Errorf("stderr missing drain-failure message; got:\n%s", stderrBuf.String())
	}
}

// slowExecutor is a CollectionExecutor that blocks for a fixed duration before
// returning. Used to test heartbeat-reaped abandonment: the heartbeat fires
// during the sleep, signals abandonCh, and the runner discards the result.
type slowExecutor struct {
	delay   time.Duration
	outcome *schedule.ExecutionOutcome
}

func (s *slowExecutor) Execute(_ context.Context, _ string, _ map[string]string) (*schedule.ExecutionOutcome, error) {
	time.Sleep(s.delay)
	return s.outcome, nil
}

// TestRunner_ClaimReaped_AbandonWithoutPosting verifies behavior 4: when the
// heartbeat receives a 409 claim-reaped response while the executor is running,
// the runner abandons the run and does NOT call PostResult.
func TestRunner_ClaimReaped_AbandonWithoutPosting(t *testing.T) {
	postCalled := false
	fake := &fakeHTTPClient{
		nextRunResponses: []nextRunResponse{
			claimResponse("run_reaped", "file:./testdata/api.yaml"),
		},
		heartbeatErr: schedule.ErrClaimReaped,
	}

	// Wrap the fake to detect PostResult (/result) calls.
	wrapper := &postDetectHTTPClient{fakeHTTPClient: fake, postCalledPtr: &postCalled}

	var stdout, stderr bytes.Buffer
	q := &schedule.Queue{Dir: t.TempDir()}
	runner := &schedule.Runner{
		Client: &schedule.Client{HTTP: wrapper, AccessToken: "tok"},
		Queue:  q,
		// slowExecutor blocks for 30ms; heartbeat fires at 5ms so the claim-
		// reaped signal arrives while the executor is still running.
		Executor:          &slowExecutor{delay: 30 * time.Millisecond, outcome: &schedule.ExecutionOutcome{PassCount: 1}},
		PollInterval:      time.Millisecond,
		HeartbeatInterval: 5 * time.Millisecond,
		PostMaxElapsed:    time.Millisecond,
		WorkingDir:        t.TempDir(),
		Stdout:            &stdout,
		Stderr:            &stderr,
		Now:               time.Now,
	}

	err := runner.RunOnce(context.Background())
	if err != nil {
		t.Errorf("RunOnce: unexpected error %v", err)
	}
	if postCalled {
		t.Errorf("PostResult was called after claim reaped; expected no post")
	}
	if !strings.Contains(stderr.String(), "claim reaped") {
		t.Errorf("stderr missing claim-reaped message; got:\n%s", stderr.String())
	}
	// Queue must be empty — no result was queued.
	items, err := q.List()
	if err != nil {
		t.Fatalf("Queue.List: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("queue len = %d; want 0 (no result queued on claim reaped)", len(items))
	}
}

// postDetectHTTPClient wraps fakeHTTPClient and records whether PostResult
// (POST .../result) was invoked.
type postDetectHTTPClient struct {
	*fakeHTTPClient
	postCalledPtr *bool
}

func (p *postDetectHTTPClient) PostJSON(ctx context.Context, path, token string, body, out any) error {
	if strings.Contains(path, "/result") {
		*p.postCalledPtr = true
	}
	return p.fakeHTTPClient.PostJSON(ctx, path, token, body, out)
}

// TestRunner_NetworkFailure_DuringPoll verifies that a network failure during
// polling is logged but does not return an error (retry on next cycle).
func TestRunner_NetworkFailure_DuringPoll(t *testing.T) {
	fake := &fakeHTTPClient{
		nextRunResponses: []nextRunResponse{
			{err: backend.ErrNetworkFailure},
		},
	}
	runner, _, stderrBuf, _ := makeTestRunner(t, fake, &fakeCollectionExecutor{})
	if err := runner.RunOnce(context.Background()); err != nil {
		t.Errorf("RunOnce: %v", err)
	}
	if !strings.Contains(stderrBuf.String(), "backend unreachable during poll") {
		t.Errorf("stderr missing poll-failure message; got:\n%s", stderrBuf.String())
	}
}

// TestRunner_NilOutcome_IsGuarded verifies that when CollectionExecutor.Execute
// returns (nil, nil) — a contract violation — the runner does not panic but
// instead synthesises a single-item error result and posts it.
func TestRunner_NilOutcome_IsGuarded(t *testing.T) {
	fake := &fakeHTTPClient{
		nextRunResponses: []nextRunResponse{
			claimResponse("run_nil", "file:./testdata/api.yaml"),
		},
		postResults: []postResult{{err: nil}},
	}
	// Execute returns (nil, nil) — contract violation.
	nilExec := &fakeCollectionExecutor{outcome: nil, err: nil}
	runner, stdoutBuf, _, _ := makeTestRunner(t, fake, nilExec)

	err := runner.RunOnce(context.Background())
	if err != nil {
		t.Errorf("RunOnce: unexpected error %v", err)
	}
	// The result should have been posted (stdout contains "posted result for run_nil").
	if !strings.Contains(stdoutBuf.String(), "posted result for run_nil") {
		t.Errorf("stdout missing posted-result line; got:\n%s", stdoutBuf.String())
	}
	// fail=1 because the synthesised outcome records the contract violation.
	if !strings.Contains(stdoutBuf.String(), "fail=1") {
		t.Errorf("stdout missing fail=1; got:\n%s", stdoutBuf.String())
	}
}

// TestRunner_Unauthorized_DuringPoll_IsFatal verifies that a 401 ProblemDetails
// error from PollNextRun bubbles up from RunOnce as a non-nil error rather than
// being swallowed or treated as a retryable network failure.
func TestRunner_Unauthorized_DuringPoll_IsFatal(t *testing.T) {
	pollErr := &backend.ProblemDetails{Status: 401, Title: "Unauthorized", Code: "UNAUTHORIZED"}
	fake := &fakeHTTPClient{
		nextRunResponses: []nextRunResponse{
			{err: pollErr},
		},
	}
	runner, _, _, _ := makeTestRunner(t, fake, &fakeCollectionExecutor{})
	err := runner.RunOnce(context.Background())
	if err == nil {
		t.Fatal("RunOnce: expected non-nil error for 401 from poll, got nil")
	}
	var pd *backend.ProblemDetails
	if !errors.As(err, &pd) {
		t.Errorf("RunOnce: expected *backend.ProblemDetails; got %T: %v", err, err)
	}
	if pd.Status != 401 {
		t.Errorf("ProblemDetails.Status = %d; want 401", pd.Status)
	}
}
