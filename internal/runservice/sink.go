package runservice

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/peterlindqvist/apitest/internal/assertion"
	"github.com/peterlindqvist/apitest/internal/httpexec"
	"github.com/peterlindqvist/apitest/internal/output/events"
	"github.com/peterlindqvist/apitest/internal/runner"
	"github.com/peterlindqvist/apitest/internal/variable"
)

// EmitterSink implements runner.EventSink by forwarding to an events.Emitter.
// Emission errors are written to errOut (non-fatal) so the run still completes
// even if the events writer stalls.
//
// sensitive and allowSensitive are used to redact request/response bodies
// before they are emitted into the NDJSON stream, matching the same redaction
// applied to terminal/JSON/TAP/JUnit output formatters.
//
// Moved out of cmd/apitest (where it was the unexported eventsAdapter)
// unchanged in behavior, including the assertion-failure sentinel injection.
type EmitterSink struct {
	em             *events.Emitter
	errOut         io.Writer // typically os.Stderr; injectable for tests
	mu             sync.RWMutex
	sensitive      *variable.SensitiveSet
	allowSensitive bool
}

// NewEmitterSink creates an EmitterSink wrapping the given emitter.
// sensitive may be nil (treated as an empty set — no body redaction).
// allowSensitive mirrors the --allow-sensitive flag; service callers
// (apitest ui) always pass false.
func NewEmitterSink(em *events.Emitter, errOut io.Writer, sensitive *variable.SensitiveSet, allowSensitive bool) *EmitterSink {
	return &EmitterSink{em: em, errOut: errOut, sensitive: sensitive, allowSensitive: allowSensitive}
}

// SetSensitive swaps the redaction set. Execute calls this with the pre-run
// set once variable sources are known, mirroring the CLI's adapter rebuild.
func (a *EmitterSink) SetSensitive(s *variable.SensitiveSet) {
	a.mu.Lock()
	a.sensitive = s
	a.mu.Unlock()
}

func (a *EmitterSink) redactionSet() (*variable.SensitiveSet, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sensitive, a.allowSensitive
}

// RequestStart implements runner.EventSink.
func (a *EmitterSink) RequestStart(e runner.RequestEvent) {
	if err := a.em.EmitRequestStart(e.RequestID, e.RequestSlug, e.Name, e.Method, e.URL, e.Phase, e.SourceFile, e.SourceLine); err != nil {
		_, _ = fmt.Fprintf(a.errOut, "events: emit request.start: %v\n", err)
	}
}

// RequestEnd implements runner.EventSink.
// Request and response bodies are redacted using the same SensitiveSet that
// is applied to terminal/JSON/TAP/JUnit formatters, so sensitive values do not
// leak into the NDJSON events file.
func (a *EmitterSink) RequestEnd(e runner.RequestEndEvent) {
	sensitive, allow := a.redactionSet()
	var reqBody, respBody []byte
	if rb, ok := variable.RedactBody(e.RequestBody, sensitive, allow).([]byte); ok {
		reqBody = rb
	}
	if rb, ok := variable.RedactBody(e.ResponseBody, sensitive, allow).([]byte); ok {
		respBody = rb
	}
	in := events.RequestEndInput{
		RequestID:    e.RequestID,
		RequestSlug:  e.RequestSlug,
		Outcome:      events.Outcome(e.Outcome),
		StatusCode:   e.StatusCode,
		Duration:     e.Duration,
		WaveIndex:    e.WaveIndex,
		RequestBody:  reqBody,
		ResponseBody: respBody,
		Timing:       TimingInfoFromExec(e.Timing, e.Attempts),
		Err:          e.Err,
	}
	// When a request failed purely because of assertion mismatches (no network
	// or plugin error), surface a sentinel so the request.end event carries a
	// structured error block with category=assertion and a concrete hint.
	// Pre-existing Err (e.g. network error) takes precedence over the sentinel.
	if in.Err == nil && in.Outcome == events.OutcomeFailed {
		in.Err = assertion.ErrAssertionFailed
	}
	if err := a.em.EmitRequestEnd(in); err != nil {
		_, _ = fmt.Fprintf(a.errOut, "events: emit request.end: %v\n", err)
	}
}

// AssertionResult implements runner.EventSink.
func (a *EmitterSink) AssertionResult(e runner.AssertionEvent) {
	if err := a.em.EmitAssertionResult(e.RequestID, e.Type, e.Expected, e.Actual, e.Passed); err != nil {
		_, _ = fmt.Fprintf(a.errOut, "events: emit assertion.result: %v\n", err)
	}
}

// TimingInfoFromExec converts httpexec.Timing plus the retry attempt count
// into the v1.3 events TimingInfo. Returns nil when t is nil (no phase
// capture). Phase pointers are set only when the phase occurred; attempts is
// recorded only when retries happened (>1).
func TimingInfoFromExec(t *httpexec.Timing, attempts int) *events.TimingInfo {
	if t == nil {
		return nil
	}
	us := func(d time.Duration) *int64 {
		if d <= 0 {
			return nil
		}
		v := d.Microseconds()
		return &v
	}
	info := &events.TimingInfo{
		DNSUs:      us(t.DNS),
		ConnectUs:  us(t.Connect),
		TLSUs:      us(t.TLS),
		TTFBUs:     us(t.TTFB),
		DownloadUs: us(t.Download),
		TotalUs:    t.Total.Microseconds(),
		Reused:     t.Reused,
	}
	if attempts > 1 {
		info.Attempts = attempts
	}
	return info
}
