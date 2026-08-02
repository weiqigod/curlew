package events

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	apierrors "github.com/peterlindqvist/apitest/internal/errors"
	"github.com/peterlindqvist/apitest/internal/output/ids"
)

// ErrEmitterClosed is returned when Emit* is called after Close.
var ErrEmitterClosed = errors.New("events: emitter closed")

// Options configures an Emitter. Zero values are valid.
type Options struct {
	// Clock overrides time.Now. Used for deterministic tests.
	Clock func() time.Time
	// RunID overrides the randomly-generated run id. Used for deterministic tests.
	RunID string
	// BodyLimit overrides DefaultBodyLimit. Zero means use DefaultBodyLimit.
	BodyLimit int
	// ApitestVersion is recorded in RunStart.
	ApitestVersion string
}

// Emitter serializes events to an io.Writer as NDJSON. Safe for concurrent use.
//
// The atomic id counter (idCounter) assigns monotonic event ids without
// taking the writer mutex, keeping id assignment cheap under high concurrency.
// The writer mutex (mu) guards the io.Writer itself — needed because
// io.Writer implementations (e.g. os.File) are not guaranteed to be atomically
// writable for multi-byte sequences.
type Emitter struct {
	w         io.Writer
	mu        sync.Mutex // guards w and closed
	idCounter atomic.Int64
	startTime time.Time
	runID     string
	opts      Options
	closed    bool
	bodyLimit int
}

// NewEmitter constructs an Emitter writing to w. w must be non-nil.
// opts.ApitestVersion must be non-empty; it is recorded in the run.start event
// and the JSON Schema requires minLength: 1.
func NewEmitter(w io.Writer, opts Options) (*Emitter, error) {
	if w == nil {
		return nil, errors.New("events: nil writer")
	}
	if opts.ApitestVersion == "" {
		return nil, errors.New("events: Options.ApitestVersion is required")
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	runID := opts.RunID
	if runID == "" {
		runID = newRunID()
	}
	bodyLimit := opts.BodyLimit
	if bodyLimit <= 0 {
		bodyLimit = DefaultBodyLimit
	}
	return &Emitter{
		w:         w,
		startTime: clock(),
		runID:     runID,
		opts:      opts,
		bodyLimit: bodyLimit,
	}, nil
}

// RunID returns the run identifier.
func (e *Emitter) RunID() string { return e.runID }

// BodyLimit returns the effective body truncation threshold.
func (e *Emitter) BodyLimit() int { return e.bodyLimit }

// Close marks the Emitter as closed. Subsequent Emit* calls return ErrEmitterClosed.
func (e *Emitter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
	return nil
}

// RunStartInput packages the inputs to EmitRunStartWithInput.
type RunStartInput struct {
	// CLIArgs is recorded as-is; callers are responsible for redacting sensitive
	// flag values before passing. A nil slice is normalised to an empty slice so
	// that cli_args always serializes as a JSON array (never null).
	CLIArgs []string
	// CollectionFile is the path to the collection being run (omitted when empty).
	CollectionFile string
	// EnvName is the --env value for this run (omitted when empty).
	EnvName string
	// Selection carries --only values (M8-004). Nil or empty causes the field to
	// be omitted from the emitted event per the v1.1 schema.
	Selection []string
}

// EmitRunStartWithInput emits a run.start event with optional selection. Added
// in schema v1.1 (M8-004). Callers that do not need --only can continue to use
// the backwards-compatible EmitRunStart(cliArgs, collectionFile, envName).
func (e *Emitter) EmitRunStartWithInput(in RunStartInput) error {
	cliArgs := in.CLIArgs
	if cliArgs == nil {
		cliArgs = []string{}
	}
	id := e.nextID()
	ev := RunStart{
		Header: Header{
			SchemaVersion: SchemaVersion,
			RunID:         e.runID,
			ID:            id,
			AtMs:          0, // RunStart is the anchor event; at_ms is always 0 by definition.
			Kind:          KindRunStart,
		},
		StartedAt:      e.startTime.UTC().Format(time.RFC3339Nano),
		ApitestVersion: e.opts.ApitestVersion,
		CLIArgs:        cliArgs,
		CollectionFile: in.CollectionFile,
		EnvName:        in.EnvName,
		Selection:      in.Selection,
	}
	return e.writeEvent(ev)
}

// EmitRunStart emits a run.start event. CLIArgs is recorded as-is; callers are
// responsible for redacting sensitive flag values before passing.
// A nil cliArgs is normalized to an empty slice so that cli_args always
// serializes as a JSON array (never null), satisfying the JSON Schema.
// Retained for backwards-compatibility. Equivalent to calling
// EmitRunStartWithInput with Selection unset.
func (e *Emitter) EmitRunStart(cliArgs []string, collectionFile, envName string) error {
	return e.EmitRunStartWithInput(RunStartInput{
		CLIArgs:        cliArgs,
		CollectionFile: collectionFile,
		EnvName:        envName,
	})
}

// EmitRunError emits a run.error event. err is classified via apierrors.ClassifyError.
// Returns an error if err is nil.
func (e *Emitter) EmitRunError(err error) error {
	if err == nil {
		return fmt.Errorf("events: nil error for run.error")
	}
	id := e.nextID()
	ev := RunError{
		Header: Header{
			SchemaVersion: SchemaVersion,
			RunID:         e.runID,
			ID:            id,
			AtMs:          e.atMs(),
			Kind:          KindRunError,
		},
		Error: classifyForEvent(err),
	}
	return e.writeEvent(ev)
}

// EmitRequestStart emits a request.start event. requestSlug is the v1.2
// additive field derived from name; pass "" to omit it (omitempty).
func (e *Emitter) EmitRequestStart(requestID, requestSlug, name, method, url, phase, sourceFile string, sourceLine int) error {
	id := e.nextID()
	ev := RequestStart{
		Header: Header{
			SchemaVersion: SchemaVersion,
			RunID:         e.runID,
			ID:            id,
			AtMs:          e.atMs(),
			Kind:          KindRequestStart,
		},
		RequestID:   requestID,
		RequestSlug: requestSlug,
		Name:        name,
		Method:      method,
		URL:         url,
		Phase:       phase,
		SourceFile:  sourceFile,
		SourceLine:  sourceLine,
	}
	return e.writeEvent(ev)
}

// RequestEndInput packages the many inputs to EmitRequestEnd.
type RequestEndInput struct {
	RequestID    string
	RequestSlug  string // M9-001: v1.2 additive; empty → omitted via omitempty
	Outcome      Outcome
	StatusCode   int
	Duration     time.Duration
	WaveIndex    int
	RequestBody  []byte
	ResponseBody []byte
	// Timing is the optional v1.3 connection-phase breakdown; nil → omitted.
	Timing *TimingInfo
	// Err is non-nil when Outcome is OutcomeError or OutcomeFailed due to a
	// non-assertion error. It is classified via apierrors.ClassifyError.
	Err error
}

// EmitRequestEnd emits a request.end event.
func (e *Emitter) EmitRequestEnd(in RequestEndInput) error {
	id := e.nextID()

	reqContent, reqSize, reqTrunc, reqEncoding := truncateBody(in.RequestBody, e.bodyLimit)
	respContent, respSize, respTrunc, respEncoding := truncateBody(in.ResponseBody, e.bodyLimit)

	ev := RequestEnd{
		Header: Header{
			SchemaVersion: SchemaVersion,
			RunID:         e.runID,
			ID:            id,
			AtMs:          e.atMs(),
			Kind:          KindRequestEnd,
		},
		RequestID:             in.RequestID,
		RequestSlug:           in.RequestSlug,
		Outcome:               in.Outcome,
		StatusCode:            in.StatusCode,
		DurationMs:            in.Duration.Milliseconds(),
		WaveIndex:             in.WaveIndex,
		RequestBody:           reqContent,
		RequestBodySize:       reqSize,
		RequestBodyTruncated:  reqTrunc,
		RequestBodyEncoding:   reqEncoding,
		ResponseBody:          respContent,
		ResponseBodySize:      respSize,
		ResponseBodyTruncated: respTrunc,
		ResponseBodyEncoding:  respEncoding,
		Timing:                in.Timing,
	}

	if in.Err != nil {
		classified := classifyForEvent(in.Err)
		ev.Error = &classified
	}

	return e.writeEvent(ev)
}

// EmitAssertionResult emits an assertion.result event.
func (e *Emitter) EmitAssertionResult(requestID, aType, expected, actual string, passed bool) error {
	id := e.nextID()
	ev := AssertionResult{
		Header: Header{
			SchemaVersion: SchemaVersion,
			RunID:         e.runID,
			ID:            id,
			AtMs:          e.atMs(),
			Kind:          KindAssertionResult,
		},
		RequestID: requestID,
		Type:      aType,
		Passed:    passed,
		Expected:  expected,
		Actual:    actual,
	}
	return e.writeEvent(ev)
}

// EmitRunEnd emits a run.end event. The EventCount on the emitted event is the
// total number of events including RunStart and RunEnd itself.
func (e *Emitter) EmitRunEnd(total, passed, failed, skipped, exitCode int) error {
	id := e.nextID() // this is the final id; also the total event count
	atMs := e.atMs() // capture once so AtMs and DurationMs reflect the same instant
	ev := RunEnd{
		Header: Header{
			SchemaVersion: SchemaVersion,
			RunID:         e.runID,
			ID:            id,
			AtMs:          atMs,
			Kind:          KindRunEnd,
		},
		DurationMs: atMs,
		Total:      total,
		Passed:     passed,
		Failed:     failed,
		Skipped:    skipped,
		ExitCode:   exitCode,
		EventCount: id,
	}
	return e.writeEvent(ev)
}

// nextID atomically increments and returns the next event id.
func (e *Emitter) nextID() int64 { return e.idCounter.Add(1) }

// atMs returns milliseconds elapsed since startTime using the configured clock.
func (e *Emitter) atMs() int64 {
	return e.clock().Sub(e.startTime).Milliseconds()
}

// clock returns the current time via opts.Clock or time.Now.
func (e *Emitter) clock() time.Time {
	if e.opts.Clock != nil {
		return e.opts.Clock()
	}
	return time.Now()
}

// writeEvent marshals v and writes the NDJSON line under the mutex.
func (e *Emitter) writeEvent(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("events: marshal: %w", err)
	}
	data = append(data, '\n')
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrEmitterClosed
	}
	if _, err := e.w.Write(data); err != nil {
		return fmt.Errorf("events: write: %w", err)
	}
	return nil
}

// newRunID produces a 32-char lowercase hex identifier. Delegates to
// internal/output/ids so events, exec --log, and markdown sentinels share
// one canonical implementation. M11-004.
func newRunID() string {
	return ids.NewRunID()
}

// isBinary reports whether the raw bytes appear to be binary data
// (contains NUL bytes or high bytes that are not valid UTF-8).
func isBinary(raw []byte) bool {
	for _, b := range raw {
		if b == 0x00 {
			return true
		}
	}
	return !utf8.Valid(raw)
}

// truncateBody returns the content for emission, the original size in bytes,
// whether truncation occurred, and the body_encoding ("base64" for binary,
// empty for UTF-8 text).
func truncateBody(raw []byte, limit int) (content string, size int, truncated bool, encoding string) {
	if len(raw) == 0 {
		return "", 0, false, ""
	}

	binary := isBinary(raw)

	if binary {
		// For binary content, use base64 encoding.
		var payload []byte
		trunc := false
		size := 0
		if len(raw) > limit {
			payload = raw[:limit]
			trunc = true
			size = len(raw)
		} else {
			payload = raw
			// size stays 0: omitempty suppresses the field when not truncated.
		}
		return base64.StdEncoding.EncodeToString(payload), size, trunc, "base64"
	}

	// Text content.
	if len(raw) <= limit {
		return string(raw), 0, false, ""
	}

	// Truncate at byte boundary, ensuring valid UTF-8.
	head := raw[:limit]
	content = strings.ToValidUTF8(string(head), "�")
	return content, len(raw), true, ""
}

// classifyForEvent converts err into an EventError using apierrors.ClassifyError.
func classifyForEvent(err error) EventError {
	s := apierrors.ClassifyError(err)
	ev := EventError{
		Category: string(s.Category),
		Code:     s.Code,
		Message:  s.Message,
		Hint:     s.Hint,
		File:     s.FilePath,
		Line:     s.Line,
	}
	return ev
}
