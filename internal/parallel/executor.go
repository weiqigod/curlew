package parallel

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/weiqigod/curlew/internal/assertion"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/requtil"
	"github.com/weiqigod/curlew/internal/retry"
	"github.com/weiqigod/curlew/internal/variable"
)

// RequestOutcome holds the result of executing a single request within a wave.
type RequestOutcome struct {
	Index            int
	Name             string
	RequestID        string // per-run id minted via Config.NextRequestID; "" for skipped requests (none minted)
	RequestSlug      string // URL-safe slug from the RequestItem; always set
	Method           string
	URL              string
	RequestHeaders   map[string]string
	RequestBody      any
	Result           *httpexec.Result
	Err              error
	Skipped          bool
	SkipReason       string
	AssertionResults *assertion.Results
	RetryCount       int
	RetryWarnings    []string
	AttemptDetails   []retry.AttemptDetail
	WaveIndex        int
	Warnings         []string

	// SourceFile / SourceLine propagate the originating RequestItem's
	// location (M6-002). Copied verbatim on every wave-level outcome,
	// including skipped / context-cancelled entries.
	SourceFile string
	SourceLine int
}

// WaveResult holds the outcome of executing a single wave.
type WaveResult struct {
	WaveIndex int
	Outcomes  []RequestOutcome
	Duration  time.Duration
}

// ExecutionResult holds the complete parallel execution result.
type ExecutionResult struct {
	Waves    []WaveResult
	Duration time.Duration
	Impact   []ImpactEntry // aggregated skip impact per failed request
}

// DataDrivenOutcome holds the result of executing a data-driven item within
// parallel wave execution. It wraps multiple iteration outcomes.
type DataDrivenOutcome struct {
	Outcomes  []RequestOutcome  // one per iteration
	Extracted map[string]string // accumulated extracted variables
}

// DataDrivenFunc executes a data-driven item and returns iteration outcomes.
// The function receives the item, scope snapshot, and wave index.
// If nil, data-driven items are executed as regular single requests.
type DataDrivenFunc func(ctx context.Context, item parser.RequestItem, scope *variable.Scope, waveIdx int) (*DataDrivenOutcome, error)

// WebSocketFunc executes a WebSocket request item and returns a single outcome.
// It is called concurrently (one goroutine per WebSocket item in a wave).
// When nil, WebSocket items are routed through ExecFunc instead.
type WebSocketFunc func(ctx context.Context, item parser.RequestItem, scope *variable.Scope, waveIdx int) RequestOutcome

// RetryConfigFunc returns the resolved retry.Config for a given request item.
// The runner builds this from the merged precedence chain.
type RetryConfigFunc func(item parser.RequestItem) retry.Config

// EndExtra carries the additive RequestEnd metadata introduced for mid-run
// inspection (curlew ui, events schema v1.3). All fields are optional.
type EndExtra struct {
	RequestHeaders  map[string]string // interpolated request headers
	ResponseHeaders http.Header       // nil on error/skip
	Timing          *httpexec.Timing  // connection-phase breakdown; nil when unavailable
	Attempts        int               // retry attempts; 1 when no retry
}

// EventSink is the parallel executor's narrow callback for per-request events.
// It is shaped identically to runner.EventSink but is duplicated here to avoid
// an import cycle from parallel → runner. The runner adapts between the two.
type EventSink interface {
	// RequestStart fires before execution begins for a request.
	RequestStart(requestID, requestSlug, name, method, url, phase, sourceFile string, sourceLine, waveIndex int)
	// RequestEnd fires after execution and assertion evaluation complete.
	// extra may be nil; it carries the additive metadata introduced for
	// mid-run inspection (curlew ui, events schema v1.3).
	RequestEnd(requestID, requestSlug, outcome string, statusCode int, duration time.Duration, waveIndex int, reqBody, respBody []byte, err error, extra *EndExtra)
	// AssertionResult fires once per individual assertion item.
	AssertionResult(ev AssertionEvent)
}

// AssertionEvent carries one assertion outcome across the sink boundary.
// Type is a discriminator ("body", "header", ...); Target and Operator hold the
// parts that vary, so consumers never have to parse a composite string.
type AssertionEvent struct {
	RequestID   string
	RequestSlug string
	SourceFile  string
	SourceLine  int
	Type        string
	Target      string
	Operator    string
	Expected    string
	Actual      string
	Passed      bool
}

// Config holds parallel execution configuration.
type Config struct {
	Graph           *DependencyGraph
	Items           []parser.RequestItem
	Scope           *variable.Scope
	ExecFunc        requtil.ExecuteFunc
	MaxRequests     int
	DataDrivenFunc  DataDrivenFunc  // optional handler for data-driven items in waves
	WebSocketFunc   WebSocketFunc   // optional handler for websocket items in waves; nil = fall through to ExecFunc
	RetryConfigFunc RetryConfigFunc // nil = no retries for regular requests
	SleepFunc       retry.SleepFunc // nil = retry.DefaultSleep

	// EventSink receives per-request callbacks for the --events NDJSON stream.
	// When nil, no event emission occurs (zero overhead).
	EventSink EventSink
	// NextRequestID is called once per request to allocate a stable string ID.
	// When nil (or EventSink is nil), no emission occurs. The runner provides
	// an atomic-counter-backed implementation so IDs stay monotonic across goroutines.
	NextRequestID func() string

	// PreExec, if non-nil, is invoked just before each ExecFunc call to
	// derive the per-item context (e.g. attaching a per-request signer spec
	// via signer.WithRequestSpec). When nil, the wave context is passed
	// unchanged (zero overhead — no allocation for runs without signing).
	PreExec func(ctx context.Context, item parser.RequestItem) context.Context
}

// ExecuteWaves runs requests in waves. Within each wave, requests execute
// concurrently. Between waves, extracted variables are propagated to the scope.
// Setup and teardown are handled externally by the caller (runner package).
func ExecuteWaves(ctx context.Context, cfg Config) (*ExecutionResult, error) {
	if cfg.Graph == nil || !cfg.Graph.IsValid {
		return nil, fmt.Errorf("invalid dependency graph")
	}

	result := &ExecutionResult{}
	start := time.Now()
	counter := 0
	failedIndices := make(map[int]bool)

	for waveIdx, wave := range cfg.Graph.Waves {
		if ctx.Err() != nil {
			// Skip remaining waves on cancellation
			for ri, remainingWave := range cfg.Graph.Waves[waveIdx:] {
				wr := WaveResult{WaveIndex: waveIdx + ri}
				for _, idx := range remainingWave {
					wr.Outcomes = append(wr.Outcomes, RequestOutcome{
						Index:       idx,
						Name:        cfg.Items[idx].Name,
						RequestSlug: cfg.Items[idx].Slug,
						Skipped:     true,
						SkipReason:  "context cancelled",
						WaveIndex:   waveIdx + ri,
						SourceFile:  cfg.Items[idx].SourceFile,
						SourceLine:  cfg.Items[idx].SourceLine,
					})
				}
				result.Waves = append(result.Waves, wr)
			}
			break
		}

		waveStart := time.Now()
		wr := WaveResult{WaveIndex: waveIdx}

		// Filter requests: check dependencies, guard rail, and create scope snapshots.
		// Counter is only incremented for requests that pass all checks including
		// scope creation, ensuring scope failures do not consume guard rail slots.
		type readyRequest struct {
			index      int // index into cfg.Items
			scope      *variable.Scope
			dataDriven bool // true when this is a data-driven item
		}
		ready := make([]readyRequest, 0, len(wave))
		for _, idx := range wave {
			if shouldSkip, reason := checkDependencyFailure(cfg.Graph, idx, failedIndices); shouldSkip {
				wr.Outcomes = append(wr.Outcomes, RequestOutcome{
					Index:       idx,
					Name:        cfg.Items[idx].Name,
					RequestSlug: cfg.Items[idx].Slug,
					Skipped:     true,
					SkipReason:  reason,
					WaveIndex:   waveIdx,
					SourceFile:  cfg.Items[idx].SourceFile,
					SourceLine:  cfg.Items[idx].SourceLine,
				})
				failedIndices[idx] = true
				continue
			}
			if counter >= cfg.MaxRequests {
				wr.Outcomes = append(wr.Outcomes, RequestOutcome{
					Index:       idx,
					Name:        cfg.Items[idx].Name,
					RequestSlug: cfg.Items[idx].Slug,
					Skipped:     true,
					SkipReason:  "request limit exceeded",
					WaveIndex:   waveIdx,
					SourceFile:  cfg.Items[idx].SourceFile,
					SourceLine:  cfg.Items[idx].SourceLine,
				})
				continue
			}

			// Create scope snapshot (sequential, before goroutines start).
			// This prevents data races on Scope.funcCache (BeginRequest/EndRequest).
			item := cfg.Items[idx]
			s, oErr := snapshotScope(cfg.Scope, item.Variables.Values)
			if oErr != nil {
				wr.Outcomes = append(wr.Outcomes, RequestOutcome{
					Index:       idx,
					Name:        item.Name,
					RequestSlug: item.Slug,
					WaveIndex:   waveIdx,
					Err:         fmt.Errorf("request %q variables: %w", item.Name, oErr),
					SourceFile:  item.SourceFile,
					SourceLine:  item.SourceLine,
				})
				failedIndices[idx] = true
				continue
			}
			isDD := item.DataDriven != nil && cfg.DataDrivenFunc != nil
			ready = append(ready, readyRequest{index: idx, scope: s, dataDriven: isDD})
			counter++
		}

		// Separate data-driven and WebSocket items from regular items.
		// Data-driven and WebSocket items are treated as atomic concurrent units.
		var regularReady []readyRequest
		var ddReady []readyRequest
		var wsReady []readyRequest
		for _, rr := range ready {
			switch {
			case rr.dataDriven:
				ddReady = append(ddReady, rr)
			case cfg.Items[rr.index].Request.Protocol == "websocket" && cfg.WebSocketFunc != nil:
				wsReady = append(wsReady, rr)
			default:
				regularReady = append(regularReady, rr)
			}
		}

		// Execute regular requests in this wave concurrently.
		outcomes := make([]RequestOutcome, len(regularReady))
		var wg sync.WaitGroup
		for i, rr := range regularReady {
			wg.Add(1)
			go func(i, idx int, scope *variable.Scope) {
				defer wg.Done()
				outcomes[i] = executeOneRequest(ctx, cfg, idx, waveIdx, scope)
			}(i, rr.index, rr.scope)
		}

		// Execute data-driven items concurrently alongside regular requests.
		// Each data-driven item produces multiple outcomes (one per iteration).
		ddResults := make([]*DataDrivenOutcome, len(ddReady))
		for i, rr := range ddReady {
			wg.Add(1)
			go func(i int, rr readyRequest) {
				defer wg.Done()
				ddOut, ddErr := cfg.DataDrivenFunc(ctx, cfg.Items[rr.index], rr.scope, waveIdx)
				if ddErr != nil {
					ddResults[i] = &DataDrivenOutcome{
						Outcomes: []RequestOutcome{{
							Index:       rr.index,
							Name:        cfg.Items[rr.index].Name,
							RequestSlug: cfg.Items[rr.index].Slug,
							WaveIndex:   waveIdx,
							Err:         ddErr,
							SourceFile:  cfg.Items[rr.index].SourceFile,
							SourceLine:  cfg.Items[rr.index].SourceLine,
						}},
					}
					return
				}
				ddResults[i] = ddOut
			}(i, rr)
		}

		// Execute WebSocket items concurrently alongside regular and data-driven requests.
		wsOutcomes := make([]RequestOutcome, len(wsReady))
		for i, rr := range wsReady {
			wg.Add(1)
			go func(i int, rr readyRequest) {
				defer wg.Done()
				wsOutcomes[i] = cfg.WebSocketFunc(ctx, cfg.Items[rr.index], rr.scope, waveIdx)
				wsOutcomes[i].Index = rr.index
			}(i, rr)
		}
		wg.Wait()

		// Process regular outcomes: record failures and extract variables (sequentially).
		for _, outcome := range outcomes {
			wr.Outcomes = append(wr.Outcomes, outcome)
			if outcome.Err != nil || (outcome.AssertionResults != nil && !outcome.AssertionResults.Passed) {
				failedIndices[outcome.Index] = true
			}
		}

		// Process data-driven outcomes: record all iteration outcomes and extract variables.
		for i, ddOut := range ddResults {
			if ddOut == nil {
				continue
			}
			ddIdx := ddReady[i].index
			anyFailed := false
			for _, outcome := range ddOut.Outcomes {
				wr.Outcomes = append(wr.Outcomes, outcome)
				if outcome.Err != nil || (outcome.AssertionResults != nil && !outcome.AssertionResults.Passed) {
					anyFailed = true
				}
			}
			if anyFailed {
				failedIndices[ddIdx] = true
			}
			// Set accumulated extracted variables into scope
			for k, v := range ddOut.Extracted {
				cfg.Scope.Set(k, v)
			}
		}

		// Process WebSocket outcomes: record outcome and mark failures.
		for i, outcome := range wsOutcomes {
			wr.Outcomes = append(wr.Outcomes, outcome)
			if outcome.Err != nil || (outcome.AssertionResults != nil && !outcome.AssertionResults.Passed) {
				failedIndices[wsReady[i].index] = true
			}
		}

		// Extract variables from regular requests after the wave completes (sequential, no race)
		for _, outcome := range outcomes {
			if outcome.Err == nil && outcome.Result != nil {
				item := cfg.Items[outcome.Index]
				if len(item.Extract) > 0 {
					extResult, extErr := variable.Extract(variable.ExtractionInput{
						Extractions: item.Extract,
						Body:        outcome.Result.Body,
					})
					if extErr != nil {
						failedIndices[outcome.Index] = true
						continue
					}
					variable.MarkExtractedSensitive(cfg.Scope.RuntimeSensitiveSet(), extResult.Variables, item.ExtractSensitive)
					for k, v := range extResult.Variables {
						cfg.Scope.Set(k, v)
					}
				}
			}
		}

		wr.Duration = time.Since(waveStart)
		result.Waves = append(result.Waves, wr)
	}

	result.Duration = time.Since(start)
	result.Impact = computeImpact(cfg.Graph, failedIndices, result.Waves)
	return result, nil
}

// executeOneRequest interpolates and executes a single request, evaluating assertions.
// scope is the per-request scope snapshot — safe for concurrent use by a single goroutine.
func executeOneRequest(ctx context.Context, cfg Config, idx, waveIdx int, scope *variable.Scope) RequestOutcome {
	item := cfg.Items[idx]

	// Interpolate request fields using per-request scope
	req := item.Request
	interpolated, interpErr := requtil.InterpolateRequest(scope, &req)
	if interpErr != nil {
		ro := RequestOutcome{
			Index:       idx,
			Name:        item.Name,
			RequestSlug: item.Slug,
			Method:      req.Method,
			URL:         req.URL,
			WaveIndex:   waveIdx,
			Err:         fmt.Errorf("request %q: %w", item.Name, interpErr),
			SourceFile:  item.SourceFile,
			SourceLine:  item.SourceLine,
		}
		return ro
	}
	req = *interpolated

	// Mint the request id (always, matching the sequential path) and emit
	// request.start when an EventSink is configured.
	var reqID string
	if cfg.NextRequestID != nil {
		reqID = cfg.NextRequestID()
	}
	if cfg.EventSink != nil && reqID != "" {
		cfg.EventSink.RequestStart(reqID, item.Slug, item.Name, req.Method, req.URL, "main",
			item.SourceFile, item.SourceLine, waveIdx)
	}

	// Execute the HTTP request, optionally with retry wrapping.
	retryCfg := retry.Config{}
	if cfg.RetryConfigFunc != nil {
		retryCfg = cfg.RetryConfigFunc(item)
	}
	sleepFn := cfg.SleepFunc
	if sleepFn == nil {
		sleepFn = retry.DefaultSleep
	}

	// Derive per-item ctx (e.g. attach signer spec) just before exec.
	execCtx := ctx
	if cfg.PreExec != nil {
		execCtx = cfg.PreExec(ctx, item)
	}

	outcome := retry.ExecuteWithRetry(execCtx, retryCfg, req.Method, func(rCtx context.Context) (*httpexec.Result, error) {
		hr := requtil.ToHTTPRequest(&req)
		r, e := cfg.ExecFunc(rCtx, hr)
		req.Headers = hr.Headers // sync signer-injected headers back to parser.Request
		return r, e
	}, sleepFn)

	result := outcome.Result
	execErr := outcome.Err
	retryCount := outcome.Attempts - 1

	// Assertion expected values interpolate against the same scope the request
	// used. Folding a failure into execErr rather than returning early keeps one
	// error path: the outcome, the event stream and the retry bookkeeping below
	// are all built in the block that follows.
	headerInputs, bodyInputs, assertErr := requtil.ToAssertionInputs(scope, item.Assertions)
	if execErr == nil && assertErr != nil {
		execErr = fmt.Errorf("request %q: %w", item.Name, assertErr)
	}

	if execErr != nil {
		ro := RequestOutcome{
			Index:          idx,
			Name:           item.Name,
			RequestID:      reqID,
			RequestSlug:    item.Slug,
			Method:         req.Method,
			URL:            req.URL,
			RequestHeaders: req.Headers,
			RequestBody:    req.Body,
			WaveIndex:      waveIdx,
			Err:            execErr,
			RetryCount:     retryCount,
			RetryWarnings:  outcome.Warnings,
			AttemptDetails: outcome.AttemptDetails,
			SourceFile:     item.SourceFile,
			SourceLine:     item.SourceLine,
		}
		if cfg.EventSink != nil && reqID != "" {
			cfg.EventSink.RequestEnd(reqID, item.Slug, "error", 0, 0, waveIdx, nil, nil, execErr, &EndExtra{
				RequestHeaders: req.Headers,
				Attempts:       outcome.Attempts,
			})
		}
		return ro
	}

	// Evaluate assertions
	ar := assertion.Evaluate(assertion.EvalInput{
		StatusCodes:      item.Assertions.Status.Codes,
		ActualStatus:     result.StatusCode,
		HeaderAssertions: headerInputs,
		Headers:          result.Headers,
		BodyAssertions:   bodyInputs,
		Body:             result.Body,
		MaxDurationMs:    item.Assertions.Timing.MaxDurationMs,
		ActualDuration:   result.Duration,
		Schema:           item.Assertions.CompiledSchema,
		StatusLine:       item.Assertions.Status.Line,
		TimingLine:       item.Assertions.Timing.Line,
		SchemaLine:       item.Assertions.SchemaLine,
	})

	// Emit assertion results and request.end.
	if cfg.EventSink != nil && reqID != "" {
		if ar != nil {
			for _, a := range ar.Items {
				cfg.EventSink.AssertionResult(AssertionEvent{
					RequestID:   reqID,
					RequestSlug: item.Slug,
					SourceFile:  item.SourceFile,
					SourceLine:  a.SourceLine,
					Type:        a.Type,
					Target:      a.Target,
					Operator:    a.Operator,
					Expected:    a.Expected,
					Actual:      a.Actual,
					Passed:      a.Passed,
				})
			}
		}
		outcomeStr := "passed"
		if ar != nil && !ar.Passed {
			outcomeStr = "failed"
		}
		var respBody []byte
		if result != nil {
			respBody = result.Body
		}
		var reqBodyBytes []byte
		if req.Body != nil {
			if b, ok := req.Body.([]byte); ok {
				reqBodyBytes = b
			}
		}
		var dur time.Duration
		if result != nil {
			dur = result.Duration
		}
		statusCode := 0
		if result != nil {
			statusCode = result.StatusCode
		}
		extra := &EndExtra{RequestHeaders: req.Headers, Attempts: outcome.Attempts}
		if result != nil {
			extra.ResponseHeaders = result.Headers
			extra.Timing = result.Timing
		}
		cfg.EventSink.RequestEnd(reqID, item.Slug, outcomeStr, statusCode, dur, waveIdx, reqBodyBytes, respBody, nil, extra)
	}

	return RequestOutcome{
		Index:            idx,
		Name:             item.Name,
		RequestID:        reqID,
		RequestSlug:      item.Slug,
		Method:           req.Method,
		URL:              req.URL,
		RequestHeaders:   req.Headers,
		RequestBody:      req.Body,
		Result:           result,
		AssertionResults: ar,
		RetryCount:       retryCount,
		RetryWarnings:    outcome.Warnings,
		AttemptDetails:   outcome.AttemptDetails,
		WaveIndex:        waveIdx,
		SourceFile:       item.SourceFile,
		SourceLine:       item.SourceLine,
	}
}

// snapshotScope creates an independent scope snapshot for a single request.
// Each goroutine must have its own scope to avoid data races on funcCache.
// If overrides are provided, they are applied on top of the base scope.
func snapshotScope(base *variable.Scope, overrides map[string]string) (*variable.Scope, error) {
	if len(overrides) > 0 {
		return base.WithOverrides(overrides)
	}
	return base.Snapshot(), nil
}

// checkDependencyFailure checks if any of the given request's dependencies have failed.
// When edge variable information is available, the skip reason includes variable names.
func checkDependencyFailure(graph *DependencyGraph, idx int, failedIndices map[int]bool) (bool, string) {
	if idx >= len(graph.Nodes) {
		return false, ""
	}
	node := graph.Nodes[idx]
	for dep := range node.Dependencies {
		if failedIndices[dep] {
			vars := findEdgeVariables(graph, dep, idx)
			if len(vars) > 0 {
				return true, fmt.Sprintf("depends on %s from %q, which failed",
					formatVarList(vars), graph.Nodes[dep].Name)
			}
			return true, fmt.Sprintf("dependency %q failed", graph.Nodes[dep].Name)
		}
	}
	return false, ""
}

// findEdgeVariables returns the variable names on the edge from producer to consumer.
func findEdgeVariables(graph *DependencyGraph, from, to int) []string {
	for _, edge := range graph.Edges {
		if edge.From == from && edge.To == to {
			return edge.Variables
		}
	}
	return nil
}

// formatVarList formats a list of variable names for human-readable output.
func formatVarList(vars []string) string {
	if len(vars) == 1 {
		return fmt.Sprintf("'%s'", vars[0])
	}
	quoted := make([]string, len(vars))
	for i, v := range vars {
		quoted[i] = fmt.Sprintf("'%s'", v)
	}
	return strings.Join(quoted, ", ")
}

// computeImpact builds impact entries from failed indices and skipped outcomes.
// Each entry describes how many requests were skipped due to a specific failed request.
func computeImpact(graph *DependencyGraph, failedIndices map[int]bool, waves []WaveResult) []ImpactEntry {
	causeCount := make(map[int]int)
	for _, wave := range waves {
		for _, o := range wave.Outcomes {
			if !o.Skipped {
				continue
			}
			if o.Index >= len(graph.Nodes) {
				continue
			}
			node := graph.Nodes[o.Index]
			for dep := range node.Dependencies {
				if failedIndices[dep] {
					causeCount[dep]++
					break // count each skip once for primary cause
				}
			}
		}
	}

	entries := make([]ImpactEntry, 0, len(causeCount))
	for idx, count := range causeCount {
		entries = append(entries, ImpactEntry{
			FailedIndex:  idx,
			FailedName:   graph.Nodes[idx].Name,
			SkippedCount: count,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].SkippedCount > entries[j].SkippedCount
	})
	return entries
}
