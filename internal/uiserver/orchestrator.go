package uiserver

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/weiqigod/curlew/internal/config"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/output/events"
	"github.com/weiqigod/curlew/internal/parallel"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/runner"
	"github.com/weiqigod/curlew/internal/runservice"
	"github.com/weiqigod/curlew/internal/validator"
	"github.com/weiqigod/curlew/internal/variable"
)

// StartParams is the POST /runs request body (spec §4.7).
type StartParams struct {
	Collection *string  `json:"collection"` // root-relative, or null = batch run
	Env        string   `json:"env"`
	Parallel   bool     `json:"parallel"`
	Mode       string   `json:"mode"` // "all" | "selection" | "rerun_failed"
	Selection  []string `json:"selection"`
	RerunOf    *string  `json:"rerun_of"`
}

// runSummaryJSON is the §4.8 summary block.
type runSummaryJSON struct {
	Total           int     `json:"total"`
	Passed          int     `json:"passed"`
	Failed          int     `json:"failed"`
	Skipped         int     `json:"skipped"`
	Error           int     `json:"error"`
	DurationMs      int64   `json:"duration_ms"`
	Parallel        bool    `json:"parallel"`
	WaveCount       int     `json:"wave_count,omitempty"`
	MaxParallelism  int     `json:"max_parallelism,omitempty"`
	WaveDurationsMs []int64 `json:"wave_durations_ms,omitempty"`
}

// gitJSON is the §8.2 git block.
type gitJSON struct {
	Branch *string `json:"branch"`
	Commit *string `json:"commit"`
}

// RunMeta is the §8.2 meta.json shape (store schema v1).
type RunMeta struct {
	SchemaVersion       int            `json:"schema_version"`
	RunID               string         `json:"run_id"`
	CreatedAt           string         `json:"created_at"`
	CurlewVersion       string         `json:"curlew_version"`
	EventsSchemaVersion string         `json:"events_schema_version"`
	CollectionFile      *string        `json:"collection_file"`
	CollectionName      *string        `json:"collection_name"`
	EnvName             string         `json:"env_name"`
	Selection           []string       `json:"selection"`
	Parallel            bool           `json:"parallel"`
	ExitStatus          string         `json:"exit_status"`
	Git                 *gitJSON       `json:"git"`
	Summary             runSummaryJSON `json:"summary"`
}

// ActiveRun is the single in-flight run (spec §6.3).
type ActiveRun struct {
	RunID     string
	State     string // "running" | "cancelling"
	Cancel    context.CancelFunc
	Params    StartParams
	StartedAt time.Time
	Log       *EventLog
	Details   *DetailCollector
	done      chan struct{}
}

// CompletedRun is a ring entry: full event log + details, in memory.
type CompletedRun struct {
	RunID   string
	State   string // "completed" | "cancelled" | "error"
	Meta    RunMeta
	Log     *EventLog
	Details *DetailCollector
}

// startError carries an HTTP-mappable run-start failure.
type startError struct {
	status  int
	code    string
	message string
	hint    string
	details any
}

func (e *startError) Error() string { return e.message }

// Orchestrator owns the single-flight run lifecycle, the 5-run memory ring,
// and the optional persisted store (spec §6.3).
type Orchestrator struct {
	mu     sync.Mutex
	active *ActiveRun
	ring   []*CompletedRun // newest first
	server *Server
}

func newOrchestrator(s *Server) *Orchestrator {
	return &Orchestrator{server: s}
}

// fanoutSink forwards runner events to multiple sinks and propagates the
// pre-run sensitive set runservice.Execute installs.
type fanoutSink struct {
	sinks []runner.EventSink
}

func (f *fanoutSink) RequestStart(e runner.RequestEvent) {
	for _, s := range f.sinks {
		s.RequestStart(e)
	}
}

func (f *fanoutSink) RequestEnd(e runner.RequestEndEvent) {
	for _, s := range f.sinks {
		s.RequestEnd(e)
	}
}

func (f *fanoutSink) AssertionResult(e runner.AssertionEvent) {
	for _, s := range f.sinks {
		s.AssertionResult(e)
	}
}

func (f *fanoutSink) SetSensitive(set *variable.SensitiveSet) {
	for _, s := range f.sinks {
		if ss, ok := s.(interface{ SetSensitive(*variable.SensitiveSet) }); ok {
			ss.SetSensitive(set)
		}
	}
}

// Current returns the active run (or nil).
func (o *Orchestrator) Current() *ActiveRun {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.active
}

// FromRing returns the ring entry for id (or nil).
func (o *Orchestrator) FromRing(id string) *CompletedRun {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, c := range o.ring {
		if c.RunID == id {
			return c
		}
	}
	return nil
}

// Ring returns a copy of the ring (newest first).
func (o *Orchestrator) Ring() []*CompletedRun {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]*CompletedRun(nil), o.ring...)
}

// Cancel requests cancellation of the active run. Returns false when id is
// not the active run. Idempotent while cancelling.
func (o *Orchestrator) Cancel(id string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.active == nil || o.active.RunID != id {
		return false
	}
	if o.active.State != "cancelling" {
		o.active.State = "cancelling"
		o.active.Cancel()
		o.server.hub.Broadcast(marshalFrame("run.state", id, map[string]any{"state": "cancelling"}))
	}
	return true
}

// Shutdown cancels the active run and waits up to 5 s for it to finish.
func (o *Orchestrator) Shutdown() {
	o.mu.Lock()
	active := o.active
	o.mu.Unlock()
	if active == nil {
		return
	}
	active.Cancel()
	select {
	case <-active.done:
	case <-time.After(5 * time.Second):
	}
}

// runTarget is one collection scheduled within a run.
type runTarget struct {
	rel       string // root-relative path
	abs       string
	col       *parser.Collection
	selection []string // per-collection selection (nil = all)
	prefix    string   // request-id prefix ("", or "c<i>-" for batch)
}

// Start validates params, seeds the collector, and launches the run
// goroutine. Returns the run id or a *startError (spec §4.7).
func (o *Orchestrator) Start(params StartParams) (string, *startError) {
	s := o.server

	if params.Mode == "" {
		params.Mode = "all"
	}
	switch params.Mode {
	case "all", "selection", "rerun_failed":
	default:
		return "", &startError{400, "bad_request", fmt.Sprintf("unknown mode %q", params.Mode), "", nil}
	}

	// Validate the environment synchronously so the start call returns 422
	// env_not_found instead of a run that errors after 202 (§4.7).
	if params.Env != "" {
		if _, err := config.FindEnvironmentFile(params.Env, s.opts.Root); err != nil {
			return "", &startError{
				422, "env_not_found",
				fmt.Sprintf("environment %q not found", params.Env), "", nil,
			}
		}
	}

	targets, serr := o.resolveTargets(&params)
	if serr != nil {
		return "", serr
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	if o.active != nil {
		return "", &startError{
			409, "run_active", "a run is already in progress", "",
			map[string]string{"run_id": o.active.RunID},
		}
	}

	runID := runner.NewRunID()
	details := NewDetailCollector(s.opts.Root)

	// Seed every planned request (outcome null) before the first event.
	for _, t := range targets {
		details.Seed(o.planItems(t, params.Parallel))
	}

	log := NewEventLog(func(id int64, line []byte) {
		s.hub.BroadcastRunEvent(marshalRunEventFrame(runID, line))
	})
	emitter, err := events.NewEmitter(log, events.Options{CurlewVersion: s.opts.Version, RunID: runID})
	if err != nil {
		return "", &startError{500, "internal", "events emitter: " + err.Error(), "", nil}
	}

	runCtx, cancel := context.WithCancel(context.Background())
	active := &ActiveRun{
		RunID:     runID,
		State:     "running",
		Cancel:    cancel,
		Params:    params,
		StartedAt: time.Now().UTC(),
		Log:       log,
		Details:   details,
		done:      make(chan struct{}),
	}
	o.active = active

	go o.execute(runCtx, active, emitter, targets)
	return runID, nil
}

// resolveTargets maps StartParams to the per-collection execution plan.
func (o *Orchestrator) resolveTargets(params *StartParams) ([]runTarget, *startError) {
	s := o.server

	// rerun_failed: derive collection(s) + selection from the source run.
	if params.Mode == "rerun_failed" {
		if params.RerunOf == nil || *params.RerunOf == "" {
			return nil, &startError{400, "bad_request", "rerun_failed requires rerun_of", "", nil}
		}
		return o.resolveRerun(params)
	}

	if params.Mode == "selection" {
		if params.Collection == nil {
			return nil, &startError{400, "bad_request", "mode selection requires a collection", "", nil}
		}
		if len(params.Selection) == 0 {
			return nil, &startError{400, "bad_request", "mode selection requires a non-empty selection", "", nil}
		}
	}

	if params.Collection != nil {
		rel := *params.Collection
		abs := s.resolveInRoot(rel)
		if abs == "" {
			return nil, &startError{400, "bad_request", "collection path outside project root", "", nil}
		}
		col, err := parser.ParseFile(abs)
		if err != nil {
			return nil, o.collectionInvalidError(rel, abs)
		}
		return []runTarget{{rel: rel, abs: abs, col: col, selection: params.Selection}}, nil
	}

	// Batch run: every valid collection, sequentially, in tree order (§6.4).
	paths := s.collectionPaths()
	var targets []runTarget
	var firstInvalidRel, firstInvalidAbs string
	valid := 0
	for _, rel := range paths {
		abs := filepath.Join(s.opts.Root, rel)
		col, err := parser.ParseFile(abs)
		if err != nil {
			if firstInvalidRel == "" {
				firstInvalidRel, firstInvalidAbs = rel, abs
			}
			continue
		}
		valid++
		targets = append(targets, runTarget{rel: rel, abs: abs, col: col})
	}
	if valid == 0 {
		if firstInvalidRel != "" {
			return nil, o.collectionInvalidError(firstInvalidRel, firstInvalidAbs)
		}
		return nil, &startError{400, "bad_request", "no collections in this project", "", nil}
	}
	// With >1 valid collection, prefix request IDs per collection.
	if valid > 1 {
		for i := range targets {
			targets[i].prefix = fmt.Sprintf("c%d-", i+1)
		}
	}
	return targets, nil
}

// resolveRerun computes per-collection failed/error main-phase selections
// from a known prior run.
func (o *Orchestrator) resolveRerun(params *StartParams) ([]runTarget, *startError) {
	src := o.findDetails(*params.RerunOf)
	if src == nil {
		return nil, &startError{400, "bad_request", fmt.Sprintf("unknown run %q", *params.RerunOf), "", nil}
	}
	byCollection := map[string][]string{}
	seen := map[string]bool{}
	for _, e := range src.List() {
		if e.Phase != "main" || e.Outcome == nil {
			continue
		}
		if *e.Outcome != "failed" && *e.Outcome != "error" {
			continue
		}
		name := e.Name
		if e.Iteration != nil {
			name = e.Iteration.BaseName
		}
		key := e.SourceFile + "\x00" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		byCollection[e.SourceFile] = append(byCollection[e.SourceFile], name)
	}
	if len(byCollection) == 0 {
		return nil, &startError{400, "bad_request", "the source run has no failed or errored requests", "", nil}
	}
	var targets []runTarget
	i := 0
	for rel, names := range byCollection {
		abs := o.server.resolveInRoot(rel)
		if abs == "" {
			continue
		}
		col, err := parser.ParseFile(abs)
		if err != nil {
			return nil, o.collectionInvalidError(rel, abs)
		}
		t := runTarget{rel: rel, abs: abs, col: col, selection: names}
		if len(byCollection) > 1 {
			t.prefix = fmt.Sprintf("c%d-", i+1)
		}
		targets = append(targets, t)
		i++
	}
	if len(targets) == 0 {
		return nil, &startError{400, "bad_request", "no rerunnable collections found", "", nil}
	}
	// Surface the effective selection on the params for run meta.
	if len(targets) == 1 {
		params.Collection = &targets[0].rel
		params.Selection = targets[0].selection
	}
	return targets, nil
}

// findDetails locates a run's detail collector across active, ring, store.
func (o *Orchestrator) findDetails(runID string) *DetailCollector {
	o.mu.Lock()
	if o.active != nil && o.active.RunID == runID {
		d := o.active.Details
		o.mu.Unlock()
		return d
	}
	for _, c := range o.ring {
		if c.RunID == runID {
			d := c.Details
			o.mu.Unlock()
			return d
		}
	}
	o.mu.Unlock()
	if o.server.store != nil {
		if d, err := o.server.store.LoadDetails(runID, o.server.opts.Root); err == nil {
			return d
		}
	}
	return nil
}

// collectionInvalidError builds the 422 collection_invalid envelope with
// validator issues in details (§4.1).
func (o *Orchestrator) collectionInvalidError(rel, abs string) *startError {
	issues := []treeIssue{}
	if res := validator.ValidateAuto(abs, nil); res != nil {
		issues = issuesFromValidator(res.Issues)
	}
	details := map[string]any{"file": rel, "issues": issues}
	return &startError{422, "collection_invalid", fmt.Sprintf("collection %s failed validation", rel), "", details}
}

// planItems builds the planned-request seed for one target collection.
func (o *Orchestrator) planItems(t runTarget, parallelRun bool) []plannedItem {
	var out []plannedItem
	add := func(item parser.RequestItem, phase string, wave int) {
		out = append(out, plannedItem{
			Slug:       item.Slug,
			Name:       item.Name,
			Phase:      phase,
			Method:     item.Request.Method,
			SourceFile: t.rel,
			SourceLine: item.SourceLine,
			WaveIndex:  wave,
			DataDriven: item.DataDriven != nil,
		})
	}
	for _, item := range t.col.Setup.Items {
		add(item, "setup", -1)
	}
	mainItems := t.col.Requests.Items
	if len(t.selection) > 0 {
		selected := make([]parser.RequestItem, 0, len(t.selection))
		want := map[string]bool{}
		for _, n := range t.selection {
			want[n] = true
		}
		for _, item := range mainItems {
			if want[item.Name] {
				selected = append(selected, item)
			}
		}
		mainItems = selected
	}
	waveOf := map[int]int{}
	if parallelRun {
		// Provisional wave grouping for live display; the authoritative
		// wave_index comes from RequestResult at reconcile (spec §3.2).
		preExec := map[string]bool{}
		for name := range t.col.Variables.Values {
			preExec[name] = true
		}
		if cfg, _, err := config.LoadProjectConfig(o.server.opts.Root); err == nil && cfg != nil {
			for name := range cfg.Variables {
				preExec[name] = true
			}
		}
		graph := parallel.Analyze(mainItems, preExec)
		if graph != nil && graph.IsValid {
			for waveIdx, wave := range graph.Waves {
				for _, itemIdx := range wave {
					waveOf[itemIdx] = waveIdx
				}
			}
		}
	}
	for i, item := range mainItems {
		wave := -1
		if parallelRun {
			if w, ok := waveOf[i]; ok {
				wave = w
			} else {
				wave = 0
			}
		}
		add(item, "main", wave)
	}
	for _, item := range t.col.Teardown.Items {
		add(item, "teardown", -1)
	}
	return out
}

// execute runs the target collections sequentially under one run id, one
// event stream, and one aggregated summary (spec §6.4), then finalizes.
func (o *Orchestrator) execute(ctx context.Context, active *ActiveRun, emitter *events.Emitter, targets []runTarget) {
	defer close(active.done)
	s := o.server

	emitterSink := runservice.NewEmitterSink(emitter, diagWriter{s.opts.Diagnostics}, nil, false)
	sink := &fanoutSink{sinks: []runner.EventSink{emitterSink, active.Details}}

	// run.start: command-level boundary. collection_file omitted for batch.
	collectionFile := ""
	if len(targets) == 1 {
		collectionFile = targets[0].rel
	}
	cliArgs := []string{"ui-run"}
	if collectionFile != "" {
		cliArgs = append(cliArgs, collectionFile)
	}
	if active.Params.Env != "" {
		cliArgs = append(cliArgs, "--env", active.Params.Env)
	}
	if active.Params.Parallel {
		cliArgs = append(cliArgs, "--parallel")
	}
	_ = emitter.EmitRunStartWithInput(events.RunStartInput{
		CLIArgs:        cliArgs,
		CollectionFile: collectionFile,
		EnvName:        active.Params.Env,
		Selection:      active.Params.Selection,
	})

	exec := s.opts.Exec
	if exec == nil {
		exec = httpexec.Execute
	}

	var (
		allResults []runner.RequestResult
		summaries  []*runner.Summary
		runErr     error
		sensitive  = variable.NewSensitiveSet()
	)
	started := time.Now()
	for _, t := range targets {
		res, err := runservice.Execute(ctx, runservice.Request{
			CollectionPath:  t.abs,
			EnvName:         active.Params.Env,
			Selection:       t.selection,
			Parallel:        active.Params.Parallel,
			RunID:           active.RunID,
			RequestIDPrefix: t.prefix,
			Sink:            sink,
			Diagnostics:     diagWriter{s.opts.Diagnostics},
		}, exec)
		if res != nil {
			allResults = append(allResults, res.Results...)
			if res.Summary != nil {
				summaries = append(summaries, res.Summary)
			}
			if res.Sensitive != nil {
				sensitive.Merge(res.Sensitive)
			}
		}
		if err != nil && ctx.Err() == nil {
			runErr = err
			_ = emitter.EmitRunError(err)
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	duration := time.Since(started)

	// Reconcile authoritative results over the live-collected details.
	active.Details.Reconcile(allResults, sensitive)

	// Aggregate summary + exit status (§6.3).
	summary := aggregateSummary(summaries, allResults, active.Params.Parallel, len(targets) > 1, duration)
	exitStatus := "passed"
	state := "completed"
	switch {
	case ctx.Err() != nil:
		exitStatus, state = "cancelled", "cancelled"
	case runErr != nil:
		exitStatus, state = "error", "error"
	case summary.Failed+summary.Error > 0:
		exitStatus = "failed"
	}
	exitCode := map[string]int{"passed": 0, "failed": 1, "error": 5, "cancelled": 130}[exitStatus]
	_ = emitter.EmitRunEnd(summary.Total, summary.Passed, summary.Failed+summary.Error, summary.Skipped, exitCode)

	meta := RunMeta{
		SchemaVersion:       1,
		RunID:               active.RunID,
		CreatedAt:           active.StartedAt.Format(time.RFC3339Nano),
		CurlewVersion:       s.opts.Version,
		EventsSchemaVersion: events.SchemaVersion,
		EnvName:             active.Params.Env,
		Selection:           active.Params.Selection,
		Parallel:            active.Params.Parallel,
		ExitStatus:          exitStatus,
		Git:                 readGitInfo(s.opts.Root),
		Summary:             summary,
	}
	if len(targets) == 1 {
		meta.CollectionFile = &targets[0].rel
		name := targets[0].col.Name
		meta.CollectionName = &name
	}

	completed := &CompletedRun{
		RunID:   active.RunID,
		State:   state,
		Meta:    meta,
		Log:     active.Log,
		Details: active.Details,
	}

	if s.store != nil {
		if err := s.store.Persist(completed); err != nil {
			s.opts.Diagnostics("curlew ui: persisting run %s: %v", active.RunID, err)
		}
	}

	o.mu.Lock()
	o.ring = append([]*CompletedRun{completed}, o.ring...)
	if len(o.ring) > MemoryRuns {
		o.ring = o.ring[:MemoryRuns]
	}
	o.active = nil
	o.mu.Unlock()

	s.hub.Broadcast(marshalFrame("run.state", active.RunID, map[string]any{
		"state": state, "exit_status": exitStatus,
	}))
}

// aggregateSummary folds per-collection summaries into the §4.8 shape,
// splitting execution errors out of the runner's Failed count.
func aggregateSummary(summaries []*runner.Summary, results []runner.RequestResult, isParallel, batch bool, wallDuration time.Duration) runSummaryJSON {
	out := runSummaryJSON{Parallel: isParallel}
	var durationMs int64
	for _, s := range summaries {
		out.Total += s.Total
		out.Passed += s.Passed
		out.Failed += s.Failed
		out.Skipped += s.Skipped
		durationMs += s.Duration.Milliseconds()
	}
	if durationMs == 0 {
		durationMs = wallDuration.Milliseconds()
	}
	out.DurationMs = durationMs
	// Split execution errors out of Failed.
	errorCount := 0
	for i := range results {
		if !results[i].Skipped && results[i].Err != nil {
			errorCount++
		}
	}
	if errorCount > out.Failed {
		errorCount = out.Failed
	}
	out.Error = errorCount
	out.Failed -= errorCount
	// Wave fields: single-collection parallel runs only (§6.4).
	if isParallel && !batch && len(summaries) == 1 {
		s := summaries[0]
		out.WaveCount = s.WaveCount
		out.MaxParallelism = s.MaxParallelism
		for _, d := range s.WaveDurations {
			out.WaveDurationsMs = append(out.WaveDurationsMs, d.Milliseconds())
		}
	}
	return out
}

// diagWriter adapts the Diagnostics func to io.Writer.
type diagWriter struct {
	fn func(format string, args ...any)
}

func (d diagWriter) Write(p []byte) (int, error) {
	if d.fn != nil {
		d.fn("%s", string(p))
	}
	return len(p), nil
}
