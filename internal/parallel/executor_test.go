package parallel

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/requtil"
	"github.com/weiqigod/curlew/internal/retry"
	"github.com/weiqigod/curlew/internal/variable"
)

// makeScope creates a scope with the given variables pre-resolved.
func makeScope(t *testing.T, vars map[string]string) *variable.Scope {
	t.Helper()
	s := variable.NewScope(vars)
	if err := s.Resolve(); err != nil {
		t.Fatalf("scope resolve: %v", err)
	}
	return s
}

// fakeExecFunc returns an ExecuteFunc that responds with the given status and body.
func fakeExecFunc(status int, body string) requtil.ExecuteFunc {
	return func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{
			StatusCode: status,
			Body:       []byte(body),
			Duration:   10 * time.Millisecond,
		}, nil
	}
}

// overlapProbe proves concurrent dispatch without wall-clock assumptions
// (a duration bound flakes under CI load). Each enter() call blocks until at
// least `want` calls are in flight simultaneously — or a generous timeout
// expires — then returns. Sequential execution can never overlap, so its
// peak stays at 1 no matter how fast or slow the machine is.
type overlapProbe struct {
	mu       sync.Mutex
	want     int
	inFlight int
	peak     int
	release  chan struct{}
	once     sync.Once
}

func newOverlapProbe(want int) *overlapProbe {
	return &overlapProbe{want: want, release: make(chan struct{})}
}

func (p *overlapProbe) enter() {
	p.mu.Lock()
	p.inFlight++
	if p.inFlight > p.peak {
		p.peak = p.inFlight
	}
	if p.inFlight >= p.want {
		p.once.Do(func() { close(p.release) })
	}
	p.mu.Unlock()
	select {
	case <-p.release:
	case <-time.After(5 * time.Second):
	}
	p.mu.Lock()
	p.inFlight--
	p.mu.Unlock()
}

func (p *overlapProbe) max() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.peak
}

// failExecFunc returns an ExecuteFunc that always fails.
func failExecFunc(errMsg string) requtil.ExecuteFunc {
	return func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return nil, fmt.Errorf("%s", errMsg)
	}
}

func TestExecuteWaves_AllIndependent(t *testing.T) {
	tests := []struct {
		name         string
		requestCount int
		wantWaves    int
		wantOutcomes int
	}{
		{"three independent requests in single wave", 3, 1, 3},
		{"single request in single wave", 1, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := make([]parser.RequestItem, tt.requestCount)
			for i := range items {
				items[i] = parser.RequestItem{
					Name:    fmt.Sprintf("req%d", i),
					Request: parser.Request{Method: "GET", URL: "http://example.com"},
				}
			}

			preExecVars := make(map[string]bool)
			graph := Analyze(items, preExecVars)
			if !graph.IsValid {
				t.Fatalf("graph invalid: %v", graph.Errors)
			}

			scope := makeScope(t, nil)
			result, err := ExecuteWaves(context.Background(), Config{
				Graph:       graph,
				Items:       items,
				Scope:       scope,
				ExecFunc:    fakeExecFunc(200, "{}"),
				MaxRequests: 1000,
			})
			if err != nil {
				t.Fatalf("ExecuteWaves error: %v", err)
			}
			if len(result.Waves) != tt.wantWaves {
				t.Errorf("waves = %d, want %d", len(result.Waves), tt.wantWaves)
			}
			total := 0
			for _, w := range result.Waves {
				total += len(w.Outcomes)
			}
			if total != tt.wantOutcomes {
				t.Errorf("outcomes = %d, want %d", total, tt.wantOutcomes)
			}
		})
	}
}

func TestExecuteWaves_LinearChain(t *testing.T) {
	// A extracts token, B uses {{token}} and extracts session, C uses {{session}}
	items := []parser.RequestItem{
		{
			Name:    "A",
			Request: parser.Request{Method: "GET", URL: "http://example.com/a"},
			Extract: map[string]string{"token": "$.token"},
		},
		{
			Name:    "B",
			Request: parser.Request{Method: "GET", URL: "http://example.com/b/{{token}}"},
			Extract: map[string]string{"session": "$.session"},
		},
		{
			Name:    "C",
			Request: parser.Request{Method: "GET", URL: "http://example.com/c/{{session}}"},
		},
	}

	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}
	if len(graph.Waves) != 3 {
		t.Fatalf("waves = %d, want 3", len(graph.Waves))
	}

	scope := makeScope(t, nil)
	callCount := 0
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		callCount++
		var body string
		switch callCount {
		case 1:
			body = `{"token":"abc123"}`
		case 2:
			body = `{"session":"sess456"}`
		default:
			body = `{}`
		}
		return &httpexec.Result{StatusCode: 200, Body: []byte(body), Duration: time.Millisecond}, nil
	}

	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    exec,
		MaxRequests: 1000,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}
	if len(result.Waves) != 3 {
		t.Errorf("waves = %d, want 3", len(result.Waves))
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestExecuteWaves_DiamondDependency(t *testing.T) {
	// Login -> (GetUser, GetOrders) -> GetDetail
	// Dependencies expressed through URL references (scanner only looks at request fields).
	items := []parser.RequestItem{
		{
			Name:    "Login",
			Request: parser.Request{Method: "POST", URL: "http://example.com/login"},
			Extract: map[string]string{"token": "$.token"},
		},
		{
			Name:    "Get User",
			Request: parser.Request{Method: "GET", URL: "http://example.com/user?auth={{token}}"},
			Extract: map[string]string{"user_id": "$.id"},
		},
		{
			Name:    "Get Orders",
			Request: parser.Request{Method: "GET", URL: "http://example.com/orders?auth={{token}}"},
			Extract: map[string]string{"order_id": "$.order_id"},
		},
		{
			Name: "Get Detail",
			Request: parser.Request{
				Method: "GET",
				URL:    "http://example.com/detail/{{user_id}}/{{order_id}}",
			},
		},
	}

	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}
	// Wave 0: Login, Wave 1: Get User + Get Orders, Wave 2: Get Detail
	if len(graph.Waves) != 3 {
		t.Fatalf("waves = %d, want 3 (waves: %v)", len(graph.Waves), graph.Waves)
	}

	scope := makeScope(t, nil)
	var mu sync.Mutex
	responses := map[string]string{
		"/login":  `{"token":"tok1"}`,
		"/user":   `{"id":"u1"}`,
		"/orders": `{"order_id":"o1"}`,
	}
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		mu.Lock()
		defer mu.Unlock()
		for suffix, body := range responses {
			if strings.Contains(req.URL, suffix) {
				return &httpexec.Result{StatusCode: 200, Body: []byte(body), Duration: time.Millisecond}, nil
			}
		}
		return &httpexec.Result{StatusCode: 200, Body: []byte(`{}`), Duration: time.Millisecond}, nil
	}

	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    exec,
		MaxRequests: 1000,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}
	if len(result.Waves) != 3 {
		t.Errorf("waves = %d, want 3", len(result.Waves))
	}
}

func TestExecuteWaves_VariableExtraction_PropagatesBetweenWaves(t *testing.T) {
	items := []parser.RequestItem{
		{
			Name:    "Extract",
			Request: parser.Request{Method: "GET", URL: "http://example.com/a"},
			Extract: map[string]string{"token": "$.token"},
		},
		{
			Name:    "Use",
			Request: parser.Request{Method: "GET", URL: "http://example.com/b/{{token}}"},
		},
	}

	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}

	scope := makeScope(t, nil)
	var lastURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		lastURL = req.URL
		return &httpexec.Result{StatusCode: 200, Body: []byte(`{"token":"extracted_val"}`), Duration: time.Millisecond}, nil
	}

	_, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    exec,
		MaxRequests: 1000,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}
	if lastURL != "http://example.com/b/extracted_val" {
		t.Errorf("last URL = %q, want URL with extracted value", lastURL)
	}
}

func TestExecuteWaves_FailedRequest_SkipsDependents(t *testing.T) {
	items := []parser.RequestItem{
		{
			Name:    "A",
			Request: parser.Request{Method: "GET", URL: "http://example.com/a"},
			Extract: map[string]string{"token": "$.token"},
		},
		{
			Name:    "B",
			Request: parser.Request{Method: "GET", URL: "http://example.com/b/{{token}}"},
		},
	}

	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}

	scope := makeScope(t, nil)
	exec := failExecFunc("network error")

	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    exec,
		MaxRequests: 1000,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}

	// Wave 1 request B should be skipped because A failed
	if len(result.Waves) < 2 {
		t.Fatalf("expected at least 2 waves, got %d", len(result.Waves))
	}
	wave1 := result.Waves[1]
	if len(wave1.Outcomes) != 1 {
		t.Fatalf("wave 1 outcomes = %d, want 1", len(wave1.Outcomes))
	}
	if !wave1.Outcomes[0].Skipped {
		t.Error("expected wave 1 request to be skipped")
	}
	if wave1.Outcomes[0].SkipReason == "" {
		t.Error("expected skip reason to be set")
	}
}

func TestExecuteWaves_GuardRail_StopsAtLimit(t *testing.T) {
	items := make([]parser.RequestItem, 3)
	for i := range items {
		items[i] = parser.RequestItem{
			Name:    fmt.Sprintf("req%d", i),
			Request: parser.Request{Method: "GET", URL: "http://example.com"},
		}
	}

	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}

	scope := makeScope(t, nil)
	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    fakeExecFunc(200, "{}"),
		MaxRequests: 2,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}

	// Should have executed 2 and skipped 1
	executed := 0
	skipped := 0
	for _, w := range result.Waves {
		for _, o := range w.Outcomes {
			if o.Skipped {
				skipped++
			} else {
				executed++
			}
		}
	}
	if executed != 2 {
		t.Errorf("executed = %d, want 2", executed)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
}

func TestExecuteWaves_ContextCancellation(t *testing.T) {
	items := []parser.RequestItem{
		{
			Name:    "A",
			Request: parser.Request{Method: "GET", URL: "http://example.com/a"},
			Extract: map[string]string{"token": "$.token"},
		},
		{
			Name:    "B",
			Request: parser.Request{Method: "GET", URL: "http://example.com/b/{{token}}"},
		},
	}

	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}

	ctx, cancel := context.WithCancel(context.Background())

	scope := makeScope(t, nil)
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		cancel() // cancel after first request
		return &httpexec.Result{StatusCode: 200, Body: []byte(`{"token":"v"}`), Duration: time.Millisecond}, nil
	}

	result, err := ExecuteWaves(ctx, Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    exec,
		MaxRequests: 1000,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}

	// Wave 1 should be skipped due to context cancellation
	if len(result.Waves) < 2 {
		t.Fatalf("expected at least 2 waves, got %d", len(result.Waves))
	}
	wave1 := result.Waves[1]
	if len(wave1.Outcomes) != 1 {
		t.Fatalf("wave 1 outcomes = %d, want 1", len(wave1.Outcomes))
	}
	if !wave1.Outcomes[0].Skipped {
		t.Error("expected wave 1 request to be skipped due to cancellation")
	}
}

func TestExecuteWaves_ConcurrentExecution_OverlapVerification(t *testing.T) {
	items := make([]parser.RequestItem, 3)
	for i := range items {
		items[i] = parser.RequestItem{
			Name:    fmt.Sprintf("req%d", i),
			Request: parser.Request{Method: "GET", URL: "http://example.com"},
		}
	}

	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}

	probe := newOverlapProbe(2)
	execFunc := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		probe.enter()
		return &httpexec.Result{StatusCode: 200, Body: []byte("{}")}, nil
	}

	scope := makeScope(t, nil)
	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    execFunc,
		MaxRequests: 1000,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}

	if got := probe.max(); got < 2 {
		t.Errorf("max concurrent in-flight requests = %d, want >= 2 (wave items must run in parallel)", got)
	}

	total := 0
	for _, w := range result.Waves {
		total += len(w.Outcomes)
	}
	if total != 3 {
		t.Errorf("outcomes = %d, want 3", total)
	}
}

func TestExecuteWaves_InvalidGraph(t *testing.T) {
	scope := makeScope(t, nil)
	_, err := ExecuteWaves(context.Background(), Config{
		Graph:       nil,
		Items:       nil,
		Scope:       scope,
		ExecFunc:    fakeExecFunc(200, "{}"),
		MaxRequests: 1000,
	})
	if err == nil {
		t.Fatal("expected error for nil graph")
	}
}

func TestExecuteWaves_EmptyGraph(t *testing.T) {
	graph := &DependencyGraph{IsValid: true, Waves: [][]int{}}
	scope := makeScope(t, nil)
	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       nil,
		Scope:       scope,
		ExecFunc:    fakeExecFunc(200, "{}"),
		MaxRequests: 1000,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}
	if len(result.Waves) != 0 {
		t.Errorf("waves = %d, want 0", len(result.Waves))
	}
}

func TestExecuteWaves_AssertionFailure_SkipsDependents(t *testing.T) {
	// A has assertions that will fail (status 500 when expecting 200),
	// B depends on A and should be skipped.
	items := []parser.RequestItem{
		{
			Name:    "A",
			Request: parser.Request{Method: "GET", URL: "http://example.com/a"},
			Assertions: parser.Assertions{
				Status: parser.StatusCodes{Codes: []int{200}},
			},
			Extract: map[string]string{"token": "$.token"},
		},
		{
			Name:    "B",
			Request: parser.Request{Method: "GET", URL: "http://example.com/b/{{token}}"},
		},
	}

	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}

	scope := makeScope(t, nil)
	exec := fakeExecFunc(500, `{"token":"v"}`)

	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    exec,
		MaxRequests: 1000,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}

	if len(result.Waves) < 2 {
		t.Fatalf("expected at least 2 waves, got %d", len(result.Waves))
	}

	// Wave 1 request B should be skipped
	wave1 := result.Waves[1]
	if len(wave1.Outcomes) != 1 {
		t.Fatalf("wave 1 outcomes = %d, want 1", len(wave1.Outcomes))
	}
	if !wave1.Outcomes[0].Skipped {
		t.Error("expected B to be skipped when A's assertions failed")
	}
}

func TestExecuteWaves_ConcurrentExecution_NoRace(t *testing.T) {
	// This test must be run with -race to detect data races.
	// Two independent requests extract different variables concurrently.
	items := make([]parser.RequestItem, 5)
	for i := range items {
		items[i] = parser.RequestItem{
			Name:    fmt.Sprintf("req%d", i),
			Request: parser.Request{Method: "GET", URL: "http://example.com"},
			Extract: map[string]string{fmt.Sprintf("var%d", i): "$.val"},
		}
	}

	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}

	scope := makeScope(t, nil)
	var counter int64
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		n := atomic.AddInt64(&counter, 1)
		body := fmt.Sprintf(`{"val":"v%d"}`, n)
		return &httpexec.Result{StatusCode: 200, Body: []byte(body), Duration: time.Millisecond}, nil
	}

	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    exec,
		MaxRequests: 1000,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}

	total := 0
	for _, w := range result.Waves {
		total += len(w.Outcomes)
	}
	if total != 5 {
		t.Errorf("outcomes = %d, want 5", total)
	}
}

func TestExecuteWaves_ScopeCreationFailure_NoZeroValuedOutcomes(t *testing.T) {
	// When scope creation fails for a request (e.g. circular variable reference),
	// only the error outcome should appear — no spurious zero-valued outcome.
	items := []parser.RequestItem{
		{
			Name:    "Good",
			Request: parser.Request{Method: "GET", URL: "http://example.com/good"},
		},
		{
			Name:    "Bad",
			Request: parser.Request{Method: "GET", URL: "http://example.com/bad"},
			Variables: parser.SensitiveVars{
				Values: map[string]string{
					"a": "{{b}}",
					"b": "{{a}}",
				},
			},
		},
	}

	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}

	scope := makeScope(t, nil)
	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    fakeExecFunc(200, "{}"),
		MaxRequests: 1000,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}

	// Count all outcomes across all waves
	total := 0
	var badOutcomes []RequestOutcome
	for _, w := range result.Waves {
		for _, o := range w.Outcomes {
			total++
			if o.Name == "" && o.Method == "" && o.URL == "" && !o.Skipped && o.Err == nil {
				t.Errorf("found zero-valued outcome at wave %d: %+v", w.WaveIndex, o)
			}
			if o.Name == "Bad" {
				badOutcomes = append(badOutcomes, o)
			}
		}
	}

	// There should be exactly 2 outcomes: one for Good (success) and one for Bad (error)
	if total != 2 {
		t.Errorf("total outcomes = %d, want 2", total)
	}
	// Bad should have exactly one outcome with an error
	if len(badOutcomes) != 1 {
		t.Errorf("bad outcomes = %d, want 1", len(badOutcomes))
	}
	if len(badOutcomes) > 0 && badOutcomes[0].Err == nil {
		t.Error("expected error for Bad request (circular var), got nil")
	}
}

func TestExecuteWaves_GuardRailNotOvercounted_OnScopeFailure(t *testing.T) {
	// When scope creation fails for a request, the guard rail counter should
	// not be incremented for that request.
	items := []parser.RequestItem{
		{
			Name:    "Bad",
			Request: parser.Request{Method: "GET", URL: "http://example.com/bad"},
			Variables: parser.SensitiveVars{
				Values: map[string]string{
					"a": "{{b}}",
					"b": "{{a}}",
				},
			},
		},
		{
			Name:    "Good",
			Request: parser.Request{Method: "GET", URL: "http://example.com/good"},
		},
	}

	// Put them all in one wave (no dependencies)
	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}

	scope := makeScope(t, nil)
	callCount := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		callCount++
		return &httpexec.Result{StatusCode: 200, Body: []byte("{}"), Duration: time.Millisecond}, nil
	}

	// MaxRequests=1: Bad should fail scope creation (no counter increment),
	// Good should still be able to execute since counter is still 0.
	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    exec,
		MaxRequests: 1,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}

	// Good should have executed (1 HTTP call made)
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (Good should execute despite Bad's scope failure)", callCount)
	}

	// Verify we get 2 outcomes: Bad (error) and Good (success)
	total := 0
	for _, w := range result.Waves {
		total += len(w.Outcomes)
	}
	if total != 2 {
		t.Errorf("total outcomes = %d, want 2", total)
	}
}

func TestExecuteWaves_RequestOutcomeFields(t *testing.T) {
	// Verify that RequestOutcome is correctly populated
	items := []parser.RequestItem{
		{
			Name:    "Check",
			Request: parser.Request{Method: "POST", URL: "http://example.com/check"},
		},
	}

	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	scope := makeScope(t, nil)

	respBody := `{"status":"ok"}`
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 201, Body: []byte(respBody), Duration: 5 * time.Millisecond}, nil
	}

	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    exec,
		MaxRequests: 1000,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}
	if len(result.Waves) != 1 || len(result.Waves[0].Outcomes) != 1 {
		t.Fatal("expected 1 wave with 1 outcome")
	}

	o := result.Waves[0].Outcomes[0]
	if o.Name != "Check" {
		t.Errorf("name = %q, want Check", o.Name)
	}
	if o.Index != 0 {
		t.Errorf("index = %d, want 0", o.Index)
	}
	if o.WaveIndex != 0 {
		t.Errorf("waveIndex = %d, want 0", o.WaveIndex)
	}
	if o.Result == nil {
		t.Fatal("result is nil")
	}
	if o.Result.StatusCode != 201 {
		t.Errorf("status = %d, want 201", o.Result.StatusCode)
	}
	if o.Err != nil {
		t.Errorf("unexpected error: %v", o.Err)
	}
	if o.Skipped {
		t.Error("unexpected skip")
	}
	if o.Method != "POST" {
		t.Errorf("method = %q, want POST", o.Method)
	}
	if o.URL != "http://example.com/check" {
		t.Errorf("url = %q, want http://example.com/check", o.URL)
	}
}

func TestComputeImpact(t *testing.T) {
	tests := []struct {
		name         string
		setupFunc    func() (*DependencyGraph, map[int]bool, []WaveResult)
		wantEntries  int
		wantTopCount int
		wantTopName  string
	}{
		{
			"single failure cascading to two dependents",
			func() (*DependencyGraph, map[int]bool, []WaveResult) {
				// A fails, B and C depend on A => impact: A caused 2 skips
				g := noEdgeGraph(3)
				g.Nodes[1].Dependencies[0] = true
				g.Nodes[2].Dependencies[0] = true
				g.Edges = []Edge{
					{From: 0, To: 1, Variables: []string{"x"}},
					{From: 0, To: 2, Variables: []string{"y"}},
				}
				failed := map[int]bool{0: true}
				waves := []WaveResult{
					{WaveIndex: 0, Outcomes: []RequestOutcome{{Index: 0, Name: "A", Err: fmt.Errorf("fail")}}},
					{WaveIndex: 1, Outcomes: []RequestOutcome{
						{Index: 1, Name: "B", Skipped: true, SkipReason: "dep A failed"},
						{Index: 2, Name: "C", Skipped: true, SkipReason: "dep A failed"},
					}},
				}
				return g, failed, waves
			},
			1, 2, "A",
		},
		{
			"no failures - empty impact",
			func() (*DependencyGraph, map[int]bool, []WaveResult) {
				g := noEdgeGraph(2)
				waves := []WaveResult{
					{WaveIndex: 0, Outcomes: []RequestOutcome{
						{Index: 0, Name: "A"},
						{Index: 1, Name: "B"},
					}},
				}
				return g, map[int]bool{}, waves
			},
			0, 0, "",
		},
		{
			"multiple failures with different skip counts - sorted by count descending",
			func() (*DependencyGraph, map[int]bool, []WaveResult) {
				// A fails (2 skipped), B fails (1 skipped)
				g := noEdgeGraph(5)
				g.Nodes[2].Dependencies[0] = true
				g.Nodes[3].Dependencies[0] = true
				g.Nodes[4].Dependencies[1] = true
				failed := map[int]bool{0: true, 1: true}
				waves := []WaveResult{
					{WaveIndex: 0, Outcomes: []RequestOutcome{
						{Index: 0, Name: "A", Err: fmt.Errorf("fail")},
						{Index: 1, Name: "B", Err: fmt.Errorf("fail")},
					}},
					{WaveIndex: 1, Outcomes: []RequestOutcome{
						{Index: 2, Name: "C", Skipped: true},
						{Index: 3, Name: "D", Skipped: true},
						{Index: 4, Name: "E", Skipped: true},
					}},
				}
				return g, failed, waves
			},
			2, 2, "A",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph, failed, waves := tt.setupFunc()
			entries := computeImpact(graph, failed, waves)
			if len(entries) != tt.wantEntries {
				t.Errorf("entries = %d, want %d: %+v", len(entries), tt.wantEntries, entries)
			}
			if tt.wantEntries > 0 {
				if entries[0].SkippedCount != tt.wantTopCount {
					t.Errorf("top entry SkippedCount = %d, want %d", entries[0].SkippedCount, tt.wantTopCount)
				}
				if entries[0].FailedName != tt.wantTopName {
					t.Errorf("top entry FailedName = %q, want %q", entries[0].FailedName, tt.wantTopName)
				}
			}
		})
	}
}

func TestCheckDependencyFailure_VariableSpecificMessage(t *testing.T) {
	tests := []struct {
		name       string
		graph      *DependencyGraph
		idx        int
		failed     map[int]bool
		wantSkip   bool
		wantReason string
	}{
		{
			"single variable dependency failed - includes var name",
			func() *DependencyGraph {
				g := noEdgeGraph(2)
				g.Nodes[1].Dependencies[0] = true
				g.Edges = []Edge{{From: 0, To: 1, Variables: []string{"user_id"}}}
				return g
			}(), 1,
			map[int]bool{0: true},
			true, `depends on 'user_id' from "A", which failed`,
		},
		{
			"multiple variable dependency failed - lists all vars",
			func() *DependencyGraph {
				g := noEdgeGraph(2)
				g.Nodes[1].Dependencies[0] = true
				g.Edges = []Edge{{From: 0, To: 1, Variables: []string{"token", "user_id"}}}
				return g
			}(), 1,
			map[int]bool{0: true},
			true, `depends on 'token', 'user_id' from "A", which failed`,
		},
		{
			"no failed dependencies",
			noEdgeGraph(2), 1,
			map[int]bool{},
			false, "",
		},
		{
			"out of range index",
			noEdgeGraph(1), 5,
			map[int]bool{0: true},
			false, "",
		},
		{
			"dependency failed but no edge variables - fallback message",
			func() *DependencyGraph {
				g := noEdgeGraph(2)
				g.Nodes[1].Dependencies[0] = true
				// No edges defined
				return g
			}(), 1,
			map[int]bool{0: true},
			true, `dependency "A" failed`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, reason := checkDependencyFailure(tt.graph, tt.idx, tt.failed)
			if skip != tt.wantSkip {
				t.Errorf("skip = %v, want %v", skip, tt.wantSkip)
			}
			if reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}

func TestExecuteWaves_DefaultValueDoesNotPreventSkipOnFailedProducer(t *testing.T) {
	// Request A extracts user_id. Request B uses {{user_id|default:fallback}}.
	// A fails => B is still skipped (producer failed, regardless of default).
	items := []parser.RequestItem{
		{
			Name:    "A",
			Request: parser.Request{Method: "GET", URL: "http://example.com/a"},
			Extract: map[string]string{"user_id": "$.id"},
		},
		{
			Name:    "B",
			Request: parser.Request{Method: "GET", URL: "http://example.com/b/{{user_id|default:fallback}}"},
		},
	}
	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}

	scope := makeScope(t, nil)
	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    failExecFunc("server error"),
		MaxRequests: 1000,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}

	// B should be skipped despite having a default value
	if len(result.Waves) < 2 {
		t.Fatalf("expected at least 2 waves, got %d", len(result.Waves))
	}
	wave1 := result.Waves[1]
	if len(wave1.Outcomes) != 1 {
		t.Fatalf("wave 1 outcomes = %d, want 1", len(wave1.Outcomes))
	}
	if !wave1.Outcomes[0].Skipped {
		t.Error("expected B to be skipped when producer A failed, even with default")
	}
	if wave1.Outcomes[0].SkipReason == "" {
		t.Error("expected skip reason to be set")
	}
}

func TestExecuteWaves_RetryWithinWave(t *testing.T) {
	t.Run("retry in wave does not block other wave members", func(t *testing.T) {
		// Two independent requests in one wave. One fails then succeeds on retry.
		// Both should complete.
		items := []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "http://example.com/a"}},
			{Name: "B", Request: parser.Request{Method: "GET", URL: "http://example.com/b"}},
		}
		preExecVars := make(map[string]bool)
		graph := Analyze(items, preExecVars)
		if !graph.IsValid {
			t.Fatalf("graph invalid: %v", graph.Errors)
		}

		var callsA int32
		execFunc := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
			if strings.Contains(req.URL, "/a") {
				n := atomic.AddInt32(&callsA, 1)
				if n == 1 {
					return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
				}
				return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond, Body: []byte("{}")}, nil
			}
			return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond, Body: []byte("{}")}, nil
		}

		retryConfigFunc := func(_ parser.RequestItem) retry.Config {
			return retry.Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 10}
		}
		noSleep := func(_ context.Context, _ time.Duration) error { return nil }

		result, err := ExecuteWaves(context.Background(), Config{
			Graph:           graph,
			Items:           items,
			Scope:           makeScope(t, nil),
			ExecFunc:        execFunc,
			MaxRequests:     1000,
			RetryConfigFunc: retryConfigFunc,
			SleepFunc:       noSleep,
		})
		if err != nil {
			t.Fatalf("ExecuteWaves error: %v", err)
		}

		// Both should succeed
		for _, wave := range result.Waves {
			for _, o := range wave.Outcomes {
				if o.Err != nil {
					t.Errorf("request %s failed: %v", o.Name, o.Err)
				}
				if o.Skipped {
					t.Errorf("request %s was skipped", o.Name)
				}
			}
		}
		// A should have been retried
		found := false
		for _, wave := range result.Waves {
			for _, o := range wave.Outcomes {
				if o.Name == "A" && o.RetryCount == 1 {
					found = true
				}
			}
		}
		if !found {
			t.Error("expected request A to have RetryCount=1")
		}
	})

	t.Run("retried request succeeds - dependent wave 2 proceeds", func(t *testing.T) {
		// A extracts a variable -> B depends on it. A fails first then succeeds.
		items := []parser.RequestItem{
			{
				Name:    "A",
				Request: parser.Request{Method: "GET", URL: "http://example.com/a"},
				Extract: map[string]string{"token": "$.token"},
			},
			{
				Name:    "B",
				Request: parser.Request{Method: "GET", URL: "http://example.com/b/{{token}}"},
			},
		}
		preExecVars := make(map[string]bool)
		graph := Analyze(items, preExecVars)
		if !graph.IsValid {
			t.Fatalf("graph invalid: %v", graph.Errors)
		}

		var callsA int32
		execFunc := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
			if strings.Contains(req.URL, "/a") {
				n := atomic.AddInt32(&callsA, 1)
				if n == 1 {
					return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
				}
				return &httpexec.Result{
					StatusCode: 200,
					Duration:   5 * time.Millisecond,
					Body:       []byte(`{"token":"abc"}`),
				}, nil
			}
			return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond, Body: []byte("{}")}, nil
		}

		retryConfigFunc := func(_ parser.RequestItem) retry.Config {
			return retry.Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 10}
		}
		noSleep := func(_ context.Context, _ time.Duration) error { return nil }

		result, err := ExecuteWaves(context.Background(), Config{
			Graph:           graph,
			Items:           items,
			Scope:           makeScope(t, nil),
			ExecFunc:        execFunc,
			MaxRequests:     1000,
			RetryConfigFunc: retryConfigFunc,
			SleepFunc:       noSleep,
		})
		if err != nil {
			t.Fatalf("ExecuteWaves error: %v", err)
		}

		// B should not be skipped
		for _, wave := range result.Waves {
			for _, o := range wave.Outcomes {
				if o.Name == "B" {
					if o.Skipped {
						t.Error("expected B to NOT be skipped when A succeeded on retry")
					}
					if o.Err != nil {
						t.Errorf("expected B to succeed, got error: %v", o.Err)
					}
				}
			}
		}
	})

	t.Run("retry exhausted - dependent requests skipped", func(t *testing.T) {
		items := []parser.RequestItem{
			{
				Name:    "A",
				Request: parser.Request{Method: "GET", URL: "http://example.com/a"},
				Extract: map[string]string{"token": "$.token"},
			},
			{
				Name:    "B",
				Request: parser.Request{Method: "GET", URL: "http://example.com/b/{{token}}"},
			},
		}
		preExecVars := make(map[string]bool)
		graph := Analyze(items, preExecVars)

		// A always returns 503
		execFunc := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
			if strings.Contains(req.URL, "/a") {
				return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
			}
			return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond, Body: []byte("{}")}, nil
		}

		retryConfigFunc := func(_ parser.RequestItem) retry.Config {
			return retry.Config{Enabled: true, MaxAttempts: 2, InitialDelayMs: 10}
		}
		noSleep := func(_ context.Context, _ time.Duration) error { return nil }

		result, err := ExecuteWaves(context.Background(), Config{
			Graph:           graph,
			Items:           items,
			Scope:           makeScope(t, nil),
			ExecFunc:        execFunc,
			MaxRequests:     1000,
			RetryConfigFunc: retryConfigFunc,
			SleepFunc:       noSleep,
		})
		if err != nil {
			t.Fatalf("ExecuteWaves error: %v", err)
		}

		// B should be skipped since A failed all retries
		for _, wave := range result.Waves {
			for _, o := range wave.Outcomes {
				if o.Name == "B" && !o.Skipped {
					t.Error("expected B to be skipped when A exhausted all retries")
				}
			}
		}
	})

	t.Run("retry count propagated to outcome", func(t *testing.T) {
		items := []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "http://example.com/a"}},
		}
		preExecVars := make(map[string]bool)
		graph := Analyze(items, preExecVars)

		var calls int32
		execFunc := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
			n := atomic.AddInt32(&calls, 1)
			if n < 3 {
				return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
			}
			return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond, Body: []byte("{}")}, nil
		}

		retryConfigFunc := func(_ parser.RequestItem) retry.Config {
			return retry.Config{Enabled: true, MaxAttempts: 5, InitialDelayMs: 10}
		}
		noSleep := func(_ context.Context, _ time.Duration) error { return nil }

		result, err := ExecuteWaves(context.Background(), Config{
			Graph:           graph,
			Items:           items,
			Scope:           makeScope(t, nil),
			ExecFunc:        execFunc,
			MaxRequests:     1000,
			RetryConfigFunc: retryConfigFunc,
			SleepFunc:       noSleep,
		})
		if err != nil {
			t.Fatalf("ExecuteWaves error: %v", err)
		}

		o := result.Waves[0].Outcomes[0]
		if o.RetryCount != 2 {
			t.Errorf("expected RetryCount=2, got %d", o.RetryCount)
		}
		if len(o.AttemptDetails) != 3 {
			t.Errorf("expected 3 AttemptDetails, got %d", len(o.AttemptDetails))
		}
	})

	t.Run("nil RetryConfigFunc means no retry", func(t *testing.T) {
		items := []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "http://example.com/a"}},
		}
		preExecVars := make(map[string]bool)
		graph := Analyze(items, preExecVars)

		var calls int32
		execFunc := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
			atomic.AddInt32(&calls, 1)
			return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
		}

		result, err := ExecuteWaves(context.Background(), Config{
			Graph:       graph,
			Items:       items,
			Scope:       makeScope(t, nil),
			ExecFunc:    execFunc,
			MaxRequests: 1000,
			// RetryConfigFunc is nil — no retry
		})
		if err != nil {
			t.Fatalf("ExecuteWaves error: %v", err)
		}

		// Should have been called exactly once (no retry)
		if atomic.LoadInt32(&calls) != 1 {
			t.Errorf("expected 1 call with nil RetryConfigFunc, got %d", atomic.LoadInt32(&calls))
		}
		o := result.Waves[0].Outcomes[0]
		if o.RetryCount != 0 {
			t.Errorf("expected RetryCount=0, got %d", o.RetryCount)
		}
	})
}

func TestExecuteWaves_websocketItemsRunConcurrently(t *testing.T) {
	items := []parser.RequestItem{
		{Name: "ws1", Request: parser.Request{Protocol: "websocket", Method: "WS", URL: "ws://fake/ws1"}},
		{Name: "ws2", Request: parser.Request{Protocol: "websocket", Method: "WS", URL: "ws://fake/ws2"}},
	}

	graph := Analyze(items, nil)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}

	probe := newOverlapProbe(2)
	wsFunc := func(_ context.Context, item parser.RequestItem, _ *variable.Scope, waveIdx int) RequestOutcome {
		probe.enter()
		return RequestOutcome{
			Name:      item.Name,
			WaveIndex: waveIdx,
			Result:    &httpexec.Result{StatusCode: 101},
		}
	}

	result, err := ExecuteWaves(context.Background(), Config{
		Graph:         graph,
		Items:         items,
		Scope:         makeScope(t, nil),
		ExecFunc:      fakeExecFunc(200, "{}"),
		MaxRequests:   1000,
		WebSocketFunc: wsFunc,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}
	if len(result.Waves) != 1 {
		t.Fatalf("waves = %d, want 1", len(result.Waves))
	}
	if len(result.Waves[0].Outcomes) != 2 {
		t.Fatalf("outcomes = %d, want 2", len(result.Waves[0].Outcomes))
	}
	if got := probe.max(); got < 2 {
		t.Errorf("max concurrent in-flight WS items = %d, want 2 (same-wave items must run in parallel)", got)
	}
}

func TestExecuteWaves_websocketFailurePropagatesSkip(t *testing.T) {
	// ws1 is depended on by http1 via variable extraction.
	items := []parser.RequestItem{
		{Name: "ws1", Request: parser.Request{Protocol: "websocket", Method: "WS", URL: "ws://fake/ws1"}},
		{Name: "http1", Request: parser.Request{Method: "GET", URL: "http://{{ws_val}}"}, Extract: map[string]string{"ws_val": "$.val"}},
	}
	// Make http1 depend on ws1 by adding extract on ws1 item.
	items[0].Extract = map[string]string{"ws_val": "$.val"}

	graph := Analyze(items, nil)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}

	wsFunc := func(_ context.Context, item parser.RequestItem, _ *variable.Scope, waveIdx int) RequestOutcome {
		return RequestOutcome{
			Name:      item.Name,
			WaveIndex: waveIdx,
			Err:       fmt.Errorf("ws connection refused"),
		}
	}

	result, err := ExecuteWaves(context.Background(), Config{
		Graph:         graph,
		Items:         items,
		Scope:         makeScope(t, nil),
		ExecFunc:      fakeExecFunc(200, "{}"),
		MaxRequests:   1000,
		WebSocketFunc: wsFunc,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}

	// ws1 fails, http1 in wave 1 should be skipped due to ws1 failing.
	var ws1Outcome, http1Outcome *RequestOutcome
	for i := range result.Waves {
		for j := range result.Waves[i].Outcomes {
			o := &result.Waves[i].Outcomes[j]
			if o.Name == "ws1" {
				ws1Outcome = o
			}
			if o.Name == "http1" {
				http1Outcome = o
			}
		}
	}
	if ws1Outcome == nil || ws1Outcome.Err == nil {
		t.Error("ws1 should have failed")
	}
	if http1Outcome == nil || !http1Outcome.Skipped {
		t.Errorf("http1 should be skipped, got: %+v", http1Outcome)
	}
}

func TestExecuteWaves_nilWebSocketFunc_fallsThrough(t *testing.T) {
	// Protocol=websocket but WebSocketFunc=nil: item routed through HTTP ExecFunc.
	items := []parser.RequestItem{
		{Name: "ws1", Request: parser.Request{Protocol: "websocket", Method: "WS", URL: "ws://fake/ws1"}},
	}
	graph := Analyze(items, nil)

	var called bool
	execFunc := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		called = true
		return &httpexec.Result{StatusCode: 101}, nil
	}

	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       makeScope(t, nil),
		ExecFunc:    execFunc,
		MaxRequests: 1000,
		// WebSocketFunc is nil
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}
	if !called {
		t.Error("ExecFunc should have been called when WebSocketFunc is nil")
	}
	_ = result
}

// --- M6-005: parallel EventSink tests ---

// parallelRecordingSink captures callbacks from the parallel EventSink interface.
type parallelRecordingSink struct {
	mu     sync.Mutex
	starts []struct {
		reqID     string
		waveIndex int
	}
	ends []struct {
		reqID     string
		outcome   string
		waveIndex int
	}
	asserts []struct {
		reqID string
	}
}

func (r *parallelRecordingSink) RequestStart(reqID, requestSlug, name, method, url, phase, sourceFile string, sourceLine, waveIndex int) {
	r.mu.Lock()
	r.starts = append(r.starts, struct {
		reqID     string
		waveIndex int
	}{reqID, waveIndex})
	r.mu.Unlock()
}

func (r *parallelRecordingSink) RequestEnd(reqID, requestSlug, outcome string, statusCode int, duration time.Duration, waveIndex int, reqBody, respBody []byte, err error, extra *EndExtra) {
	r.mu.Lock()
	r.ends = append(r.ends, struct {
		reqID     string
		outcome   string
		waveIndex int
	}{reqID, outcome, waveIndex})
	r.mu.Unlock()
}

func (r *parallelRecordingSink) AssertionResult(reqID, aType, expected, actual string, passed bool) {
	r.mu.Lock()
	r.asserts = append(r.asserts, struct{ reqID string }{reqID})
	r.mu.Unlock()
}

// TestExecuteWaves_EventSink_MonotonicIDsAcrossGoroutines verifies that 3 requests
// in one wave each receive distinct, monotonic request IDs from the EventSink.
func TestExecuteWaves_EventSink_MonotonicIDsAcrossGoroutines(t *testing.T) {
	items := []parser.RequestItem{
		{Name: "r1", Request: parser.Request{Method: "GET", URL: "http://example.com"}},
		{Name: "r2", Request: parser.Request{Method: "GET", URL: "http://example.com"}},
		{Name: "r3", Request: parser.Request{Method: "GET", URL: "http://example.com"}},
	}
	graph := Analyze(items, nil)
	if !graph.IsValid {
		t.Fatalf("invalid graph: %v", graph.Errors)
	}

	sink := &parallelRecordingSink{}
	counter := &atomic.Int64{}
	nextID := func() string { return fmt.Sprintf("req-%d", counter.Add(1)) }

	_, err := ExecuteWaves(context.Background(), Config{
		Graph:         graph,
		Items:         items,
		Scope:         makeScope(t, nil),
		ExecFunc:      fakeExecFunc(200, "ok"),
		MaxRequests:   1000,
		EventSink:     sink,
		NextRequestID: nextID,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves: %v", err)
	}

	if len(sink.starts) != 3 {
		t.Fatalf("want 3 request.start events, got %d", len(sink.starts))
	}
	if len(sink.ends) != 3 {
		t.Fatalf("want 3 request.end events, got %d", len(sink.ends))
	}

	// All IDs should be distinct.
	ids := make(map[string]bool)
	for _, s := range sink.starts {
		if ids[s.reqID] {
			t.Errorf("duplicate request ID: %q", s.reqID)
		}
		ids[s.reqID] = true
		if s.waveIndex != 0 {
			t.Errorf("start waveIndex = %d, want 0", s.waveIndex)
		}
	}

	// All ends should have outcome="passed" (status 200, no assertions).
	for _, e := range sink.ends {
		if e.outcome != "passed" {
			t.Errorf("end outcome = %q, want passed", e.outcome)
		}
	}
}

// TestExecuteWaves_EventSink_WaveIndexPropagates verifies that the wave_index
// field in the RequestEnd event matches the actual wave in which the request executed.
func TestExecuteWaves_EventSink_WaveIndexPropagates(t *testing.T) {
	// A extracts a value, B uses it => A is wave 0, B is wave 1.
	items := []parser.RequestItem{
		{
			Name:    "a",
			Request: parser.Request{Method: "GET", URL: "http://x.com/a"},
			Extract: map[string]string{"a_tok": "$.token"},
		},
		{
			Name:    "b",
			Request: parser.Request{Method: "GET", URL: "http://x.com/{{a_tok}}"},
		},
	}
	graph := Analyze(items, nil)
	if !graph.IsValid {
		t.Fatalf("invalid graph: %v", graph.Errors)
	}
	if len(graph.Waves) != 2 {
		t.Fatalf("want 2 waves, got %d", len(graph.Waves))
	}

	sink := &parallelRecordingSink{}
	counter := &atomic.Int64{}
	nextID := func() string { return fmt.Sprintf("req-%d", counter.Add(1)) }

	_, err := ExecuteWaves(context.Background(), Config{
		Graph:         graph,
		Items:         items,
		Scope:         makeScope(t, nil),
		ExecFunc:      fakeExecFunc(200, `{"token":"abc"}`),
		MaxRequests:   1000,
		EventSink:     sink,
		NextRequestID: nextID,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves: %v", err)
	}

	if len(sink.ends) != 2 {
		t.Fatalf("want 2 request.end events, got %d", len(sink.ends))
	}

	// Events come in execution order but we check both wave indices appear.
	waveIndices := make(map[int]bool)
	for _, e := range sink.ends {
		waveIndices[e.waveIndex] = true
	}
	if !waveIndices[0] || !waveIndices[1] {
		t.Errorf("want wave indices 0 and 1, got %v", waveIndices)
	}
}

// TestExecuteWaves_PreExec_AttachesPerItemCtx verifies that when
// Config.PreExec is set, it is called once per regular request item
// and the returned ctx is what ExecFunc receives.
func TestExecuteWaves_PreExec_AttachesPerItemCtx(t *testing.T) {
	type ctxKey struct{}
	items := []parser.RequestItem{
		{Name: "a", Request: parser.Request{Method: "GET", URL: "http://example.com/a"}},
		{Name: "b", Request: parser.Request{Method: "GET", URL: "http://example.com/b"}},
	}
	graph := Analyze(items, nil)
	if !graph.IsValid {
		t.Fatalf("invalid graph: %v", graph.Errors)
	}

	var mu sync.Mutex
	var seen []string
	execFn := func(ctx context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		if v, ok := ctx.Value(ctxKey{}).(string); ok {
			mu.Lock()
			seen = append(seen, v)
			mu.Unlock()
		}
		return &httpexec.Result{StatusCode: 200}, nil
	}
	cfg := Config{
		Graph:       graph,
		Items:       items,
		Scope:       makeScope(t, nil),
		ExecFunc:    execFn,
		MaxRequests: 100,
		PreExec: func(ctx context.Context, item parser.RequestItem) context.Context {
			return context.WithValue(ctx, ctxKey{}, item.Name)
		},
	}
	_, err := ExecuteWaves(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("PreExec: want 2 item names, got %d: %v", len(seen), seen)
	}
	sort.Strings(seen)
	if seen[0] != "a" || seen[1] != "b" {
		t.Errorf("PreExec item names = %v, want [a b]", seen)
	}
}

// TestExecuteWaves_SignerHeaders_PropagateWhenNilOriginal verifies that headers
// injected by a signer (simulated via ExecFunc mutating req.Headers when it was
// nil) are propagated back to RequestOutcome.RequestHeaders.
// This is the parallel-executor analogue of the runner nil-headers regression.
func TestExecuteWaves_SignerHeaders_PropagateWhenNilOriginal(t *testing.T) {
	// Request with NO headers — parser.Request.Headers is nil.
	items := []parser.RequestItem{
		{Name: "signed", Request: parser.Request{Method: "GET", URL: "http://example.com/"}},
	}
	graph := Analyze(items, nil)
	if !graph.IsValid {
		t.Fatalf("invalid graph: %v", graph.Errors)
	}

	// ExecFunc simulates the signer wrap: allocates req.Headers when nil and
	// injects an Authorization header before dispatching.
	execFn := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		if req.Headers == nil {
			req.Headers = make(map[string]string)
		}
		req.Headers["Authorization"] = "AWS4-HMAC-SHA256 Credential=TEST"
		return &httpexec.Result{StatusCode: 200}, nil
	}

	execResult, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       makeScope(t, nil),
		ExecFunc:    execFn,
		MaxRequests: 100,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves: %v", err)
	}
	if len(execResult.Waves) == 0 || len(execResult.Waves[0].Outcomes) == 0 {
		t.Fatal("no outcomes")
	}
	outcome := execResult.Waves[0].Outcomes[0]
	if outcome.RequestHeaders == nil {
		t.Fatal("RequestHeaders is nil — signer-injected headers were not propagated back from httpexec.Request")
	}
	if got := outcome.RequestHeaders["Authorization"]; got != "AWS4-HMAC-SHA256 Credential=TEST" {
		t.Errorf("RequestHeaders[Authorization] = %q; want AWS4-HMAC-SHA256 Credential=TEST", got)
	}
}

// TestExecuteWaves_PreExec_Nil_Passthrough verifies that Config.PreExec nil
// is safe (no panic, exec proceeds with the original ctx).
func TestExecuteWaves_PreExec_Nil_Passthrough(t *testing.T) {
	items := []parser.RequestItem{
		{Name: "a", Request: parser.Request{Method: "GET", URL: "http://example.com/a"}},
	}
	graph := Analyze(items, nil)
	_, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       makeScope(t, nil),
		ExecFunc:    fakeExecFunc(200, "{}"),
		MaxRequests: 100,
		PreExec:     nil, // explicit nil — must not panic
	})
	if err != nil {
		t.Fatalf("ExecuteWaves with nil PreExec: %v", err)
	}
}
