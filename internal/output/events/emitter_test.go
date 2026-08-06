package events_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/output/events"
	"github.com/weiqigod/curlew/internal/parser"
)

// fixedClock returns a clock function that always returns the parsed time.
func fixedClock(t *testing.T, rfc3339 string) func() time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		t.Fatalf("fixedClock: parse %q: %v", rfc3339, err)
	}
	return func() time.Time { return ts }
}

// parseLine unmarshals a single JSON line into a map.
func parseLine(t *testing.T, line string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("parseLine: %v (line: %q)", err, line)
	}
	return m
}

// splitLines returns non-empty trimmed lines from buf.
func splitLines(buf *bytes.Buffer) []string {
	var lines []string
	for _, l := range strings.Split(buf.String(), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func TestEmitter_RunStart_MinimalFields(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "test-run-001",
		CurlewVersion: "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	if err := em.EmitRunStart([]string{"run", "x.yaml"}, "", ""); err != nil {
		t.Fatalf("EmitRunStart: %v", err)
	}

	lines := splitLines(&buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	m := parseLine(t, lines[0])

	checks := map[string]any{
		"kind":           "run.start",
		"schema_version": "1.5",
		"run_id":         "test-run-001",
		"curlew_version": "0.1.0-dev",
	}
	for k, want := range checks {
		if got := m[k]; got != want {
			t.Errorf("field %q: want %v, got %v", k, want, got)
		}
	}

	if id, ok := m["id"].(float64); !ok || id != 1 {
		t.Errorf("expected id=1, got %v", m["id"])
	}
	if atMs, ok := m["at_ms"].(float64); !ok || atMs != 0 {
		t.Errorf("expected at_ms=0, got %v", m["at_ms"])
	}
	if _, ok := m["started_at"]; !ok {
		t.Error("missing started_at field")
	}
	cliArgs, ok := m["cli_args"].([]any)
	if !ok || len(cliArgs) != 2 {
		t.Errorf("expected cli_args with 2 elements, got %v", m["cli_args"])
	}
}

func TestEmitter_RequestStartEnd_PairedIDs(t *testing.T) {
	var buf bytes.Buffer
	tick := 0
	clock := func() time.Time {
		tick++
		return time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC).Add(time.Duration(tick-1) * 10 * time.Millisecond)
	}
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         clock,
		RunID:         "test-run-002",
		CurlewVersion: "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	if err := em.EmitRequestStart("req-1", "get-user", "Get user", "GET", "https://example.com/user", "", "", 0); err != nil {
		t.Fatalf("EmitRequestStart: %v", err)
	}
	if err := em.EmitRequestEnd(events.RequestEndInput{
		RequestID:  "req-1",
		Outcome:    events.OutcomePassed,
		StatusCode: 200,
		Duration:   50 * time.Millisecond,
	}); err != nil {
		t.Fatalf("EmitRequestEnd: %v", err)
	}

	lines := splitLines(&buf)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %s", len(lines), buf.String())
	}

	start := parseLine(t, lines[0])
	end := parseLine(t, lines[1])

	// Both should have same run_id
	if start["run_id"] != end["run_id"] {
		t.Errorf("run_id mismatch: %v vs %v", start["run_id"], end["run_id"])
	}
	// Both should reference same request_id
	if start["request_id"] != end["request_id"] {
		t.Errorf("request_id mismatch: %v vs %v", start["request_id"], end["request_id"])
	}
	if start["request_id"] != "req-1" {
		t.Errorf("request_id expected req-1, got %v", start["request_id"])
	}

	// IDs must be monotonically increasing
	startID := start["id"].(float64)
	endID := end["id"].(float64)
	if startID >= endID {
		t.Errorf("expected startID(%v) < endID(%v)", startID, endID)
	}

	// at_ms values must be non-decreasing
	startAtMs := start["at_ms"].(float64)
	endAtMs := end["at_ms"].(float64)
	if startAtMs > endAtMs {
		t.Errorf("at_ms decreased: start=%v end=%v", startAtMs, endAtMs)
	}
}

func TestEmitter_RequestEnd_RegisteredSentinelHint(t *testing.T) {
	// Wrap the real parser.ErrInvalidYAML sentinel so ClassifyError must walk
	// the error chain and resolve the hint from the registry. This exercises
	// behaviour 3: "a failed request whose chain contains a registered sentinel".
	wrappedErr := fmt.Errorf("context: %w", parser.ErrInvalidYAML)

	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "test-run-003",
		CurlewVersion: "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	if err := em.EmitRequestEnd(events.RequestEndInput{
		RequestID: "req-1",
		Outcome:   events.OutcomeError,
		Duration:  10 * time.Millisecond,
		Err:       wrappedErr,
	}); err != nil {
		t.Fatalf("EmitRequestEnd: %v", err)
	}

	lines := splitLines(&buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	m := parseLine(t, lines[0])

	errObj, ok := m["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %T: %v", m["error"], m["error"])
	}
	if errObj["category"] != "parse" {
		t.Errorf("expected category=parse, got %v", errObj["category"])
	}
	if errObj["code"] != "PARSE_INVALID_YAML" {
		t.Errorf("expected code=PARSE_INVALID_YAML, got %v", errObj["code"])
	}
	hint, _ := errObj["hint"].(string)
	if hint == "" {
		t.Error("expected non-empty hint resolved from registry for parser.ErrInvalidYAML")
	}
}

func TestEmitter_RequestEnd_NetworkErrorKinds(t *testing.T) {
	tests := []struct {
		name     string
		kind     apierrors.NetworkErrorKind
		wantCode string
	}{
		{"dns", apierrors.NetworkDNS, "NETWORK_DNS"},
		{"timeout", apierrors.NetworkTimeout, "NETWORK_TIMEOUT"},
		{"tls", apierrors.NetworkTLS, "NETWORK_TLS"},
		{"refused", apierrors.NetworkConnectionRefused, "NETWORK_CONNECTION_REFUSED"},
		{"other", apierrors.NetworkOther, "NETWORK_OTHER"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			netErr := &apierrors.NetworkError{
				Kind:    tc.kind,
				Message: "network failure",
				Hint:    "check connection",
			}

			var buf bytes.Buffer
			em, err := events.NewEmitter(&buf, events.Options{
				Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
				RunID:         "test-run-net",
				CurlewVersion: "0.1.0-dev",
			})
			if err != nil {
				t.Fatalf("NewEmitter: %v", err)
			}

			if err := em.EmitRequestEnd(events.RequestEndInput{
				RequestID: "req-1",
				Outcome:   events.OutcomeError,
				Duration:  5 * time.Millisecond,
				Err:       netErr,
			}); err != nil {
				t.Fatalf("EmitRequestEnd: %v", err)
			}

			lines := splitLines(&buf)
			m := parseLine(t, lines[0])

			errObj, ok := m["error"].(map[string]any)
			if !ok {
				t.Fatalf("expected error object, got %v", m["error"])
			}
			if errObj["category"] != "network" {
				t.Errorf("expected category=network, got %v", errObj["category"])
			}
			if errObj["code"] != tc.wantCode {
				t.Errorf("expected code=%s, got %v", tc.wantCode, errObj["code"])
			}
		})
	}
}

func TestEmitter_ConcurrentEmitMonotonicIDs(t *testing.T) {
	const N = 200
	var buf safeBuffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "test-run-concurrent",
		CurlewVersion: "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	// Emit RunStart first (id=1)
	if err := em.EmitRunStart([]string{"run"}, "", ""); err != nil {
		t.Fatalf("EmitRunStart: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = em.EmitRequestEnd(events.RequestEndInput{
				RequestID:  fmt.Sprintf("req-%d", i),
				Outcome:    events.OutcomePassed,
				StatusCode: 200,
				Duration:   1 * time.Millisecond,
			})
		}(i)
	}
	wg.Wait()

	lines := splitLines(buf.Buffer())
	if len(lines) != N+1 {
		t.Fatalf("expected %d lines, got %d", N+1, len(lines))
	}

	// Parse all ids
	ids := make([]int, 0, len(lines))
	for _, line := range lines {
		m := parseLine(t, line)
		id, ok := m["id"].(float64)
		if !ok {
			t.Fatalf("missing/invalid id in line: %s", line)
		}
		ids = append(ids, int(id))
	}

	sort.Ints(ids)
	for i, id := range ids {
		if id != i+1 {
			t.Errorf("ids not monotonic: ids[%d]=%d, want %d", i, id, i+1)
		}
	}
}

// safeBuffer is a thread-safe bytes.Buffer wrapper.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) Buffer() *bytes.Buffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := bytes.NewBuffer(s.buf.Bytes())
	return cp
}

func TestEmitter_BodyTruncation_OverLimit(t *testing.T) {
	body := make([]byte, 4096)
	for i := range body {
		body[i] = 'A' + byte(i%26)
	}

	t.Run("response_body", func(t *testing.T) {
		var buf bytes.Buffer
		em, err := events.NewEmitter(&buf, events.Options{
			Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
			RunID:         "test-run-trunc",
			CurlewVersion: "0.1.0-dev",
		})
		if err != nil {
			t.Fatalf("NewEmitter: %v", err)
		}

		if err := em.EmitRequestEnd(events.RequestEndInput{
			RequestID:    "req-1",
			Outcome:      events.OutcomePassed,
			StatusCode:   200,
			Duration:     5 * time.Millisecond,
			ResponseBody: body,
		}); err != nil {
			t.Fatalf("EmitRequestEnd: %v", err)
		}

		lines := splitLines(&buf)
		m := parseLine(t, lines[0])

		if trunc, ok := m["response_body_truncated"].(bool); !ok || !trunc {
			t.Errorf("expected response_body_truncated=true, got %v", m["response_body_truncated"])
		}
		if size, ok := m["response_body_size"].(float64); !ok || int(size) != 4096 {
			t.Errorf("expected response_body_size=4096, got %v", m["response_body_size"])
		}
		respBody, ok := m["response_body"].(string)
		if !ok {
			t.Fatalf("response_body missing or wrong type: %v", m["response_body"])
		}
		if len(respBody) != events.DefaultBodyLimit {
			t.Errorf("expected response_body length=%d, got %d", events.DefaultBodyLimit, len(respBody))
		}
	})

	t.Run("request_body", func(t *testing.T) {
		var buf bytes.Buffer
		em, err := events.NewEmitter(&buf, events.Options{
			Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
			RunID:         "test-run-trunc-req",
			CurlewVersion: "0.1.0-dev",
		})
		if err != nil {
			t.Fatalf("NewEmitter: %v", err)
		}

		if err := em.EmitRequestEnd(events.RequestEndInput{
			RequestID:   "req-1",
			Outcome:     events.OutcomePassed,
			StatusCode:  200,
			Duration:    5 * time.Millisecond,
			RequestBody: body,
		}); err != nil {
			t.Fatalf("EmitRequestEnd: %v", err)
		}

		lines := splitLines(&buf)
		m := parseLine(t, lines[0])

		if trunc, ok := m["request_body_truncated"].(bool); !ok || !trunc {
			t.Errorf("expected request_body_truncated=true, got %v", m["request_body_truncated"])
		}
		if size, ok := m["request_body_size"].(float64); !ok || int(size) != 4096 {
			t.Errorf("expected request_body_size=4096, got %v", m["request_body_size"])
		}
		reqBody, ok := m["request_body"].(string)
		if !ok {
			t.Fatalf("request_body missing or wrong type: %v", m["request_body"])
		}
		if len(reqBody) != events.DefaultBodyLimit {
			t.Errorf("expected request_body length=%d, got %d", events.DefaultBodyLimit, len(reqBody))
		}
	})
}

func TestEmitter_BodyTruncation_BinaryOverLimit(t *testing.T) {
	// Binary body (contains NUL) larger than the default limit triggers base64 + truncation.
	body := make([]byte, 4096)
	body[0] = 0x00 // NUL byte makes isBinary return true
	for i := 1; i < len(body); i++ {
		body[i] = byte(i % 256)
	}

	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "test-run-bintrunc",
		CurlewVersion: "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	if err := em.EmitRequestEnd(events.RequestEndInput{
		RequestID:    "req-1",
		Outcome:      events.OutcomePassed,
		StatusCode:   200,
		Duration:     5 * time.Millisecond,
		ResponseBody: body,
	}); err != nil {
		t.Fatalf("EmitRequestEnd: %v", err)
	}

	lines := splitLines(&buf)
	m := parseLine(t, lines[0])

	if m["response_body_encoding"] != "base64" {
		t.Errorf("expected response_body_encoding=base64, got %v", m["response_body_encoding"])
	}
	if trunc, ok := m["response_body_truncated"].(bool); !ok || !trunc {
		t.Errorf("expected response_body_truncated=true, got %v", m["response_body_truncated"])
	}
	if size, ok := m["response_body_size"].(float64); !ok || int(size) != 4096 {
		t.Errorf("expected response_body_size=4096, got %v", m["response_body_size"])
	}
}

func TestEmitter_BodyTruncation_UnderLimit(t *testing.T) {
	body := []byte("hello world")

	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "test-run-trunc2",
		CurlewVersion: "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	if err := em.EmitRequestEnd(events.RequestEndInput{
		RequestID:   "req-1",
		Outcome:     events.OutcomePassed,
		StatusCode:  200,
		Duration:    5 * time.Millisecond,
		RequestBody: body,
	}); err != nil {
		t.Fatalf("EmitRequestEnd: %v", err)
	}

	lines := splitLines(&buf)
	m := parseLine(t, lines[0])

	if _, ok := m["request_body_truncated"]; ok {
		t.Error("request_body_truncated should be omitted when false")
	}
	if _, ok := m["request_body_size"]; ok {
		t.Error("request_body_size should be omitted when small")
	}
	if m["request_body"] != "hello world" {
		t.Errorf("expected request_body=hello world, got %v", m["request_body"])
	}
}

func TestEmitter_BodyTruncation_BinaryBase64(t *testing.T) {
	// Body with NUL bytes is binary
	body := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}

	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "test-run-binary",
		CurlewVersion: "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	if err := em.EmitRequestEnd(events.RequestEndInput{
		RequestID:    "req-1",
		Outcome:      events.OutcomePassed,
		StatusCode:   200,
		Duration:     5 * time.Millisecond,
		ResponseBody: body,
	}); err != nil {
		t.Fatalf("EmitRequestEnd: %v", err)
	}

	lines := splitLines(&buf)
	m := parseLine(t, lines[0])

	if m["response_body_encoding"] != "base64" {
		t.Errorf("expected response_body_encoding=base64, got %v", m["response_body_encoding"])
	}
	// Separate request_body_encoding must not be set (no request body was provided).
	if v, ok := m["request_body_encoding"]; ok {
		t.Errorf("expected request_body_encoding absent, got %v", v)
	}
}

func TestEmitter_RunEnd_EventCount(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "test-run-count",
		CurlewVersion: "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	// RunStart (1)
	if err := em.EmitRunStart([]string{"run"}, "", ""); err != nil {
		t.Fatalf("EmitRunStart: %v", err)
	}

	// 3x RequestStart/End = 6 more events (ids 2-7)
	for i := 0; i < 3; i++ {
		reqID := fmt.Sprintf("req-%d", i)
		if err := em.EmitRequestStart(reqID, "", fmt.Sprintf("request-%d", i), "GET", "https://example.com", "", "", 0); err != nil {
			t.Fatalf("EmitRequestStart: %v", err)
		}
		if err := em.EmitRequestEnd(events.RequestEndInput{
			RequestID:  reqID,
			Outcome:    events.OutcomePassed,
			StatusCode: 200,
			Duration:   5 * time.Millisecond,
		}); err != nil {
			t.Fatalf("EmitRequestEnd: %v", err)
		}
	}

	// RunEnd (id=8)
	if err := em.EmitRunEnd(3, 3, 0, 0, 0); err != nil {
		t.Fatalf("EmitRunEnd: %v", err)
	}

	lines := splitLines(&buf)
	// Find RunEnd line (last)
	lastLine := lines[len(lines)-1]
	m := parseLine(t, lastLine)

	if m["kind"] != "run.end" {
		t.Fatalf("last event expected run.end, got %v", m["kind"])
	}
	// event_count should be 8: RunStart + 3*RequestStart + 3*RequestEnd + RunEnd
	if count, ok := m["event_count"].(float64); !ok || int(count) != 8 {
		t.Errorf("expected event_count=8, got %v", m["event_count"])
	}
}

func TestEmitter_EmitAfterClose(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "test-run-closed",
		CurlewVersion: "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	if err := em.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	err = em.EmitRunEnd(0, 0, 0, 0, 0)
	if !errors.Is(err, events.ErrEmitterClosed) {
		t.Errorf("expected ErrEmitterClosed, got %v", err)
	}
}

func TestEmitter_NilWriter(t *testing.T) {
	_, err := events.NewEmitter(nil, events.Options{})
	if err == nil {
		t.Fatal("expected error for nil writer")
	}
}

func TestEmitter_EmptyCurlewVersion(t *testing.T) {
	var buf bytes.Buffer
	_, err := events.NewEmitter(&buf, events.Options{
		RunID: "test-run-noversion",
		// CurlewVersion intentionally omitted (empty string)
	})
	if err == nil {
		t.Fatal("expected error when CurlewVersion is empty, got nil")
	}
}

func TestEmitter_RunID_Default(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{CurlewVersion: "0.1.0-dev"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	runID := em.RunID()
	if len(runID) != 32 {
		t.Errorf("expected 32-char run_id, got len=%d: %q", len(runID), runID)
	}
	// Must be lowercase hex
	for _, c := range runID {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("run_id contains non-hex char %q: %s", c, runID)
		}
	}
}

func TestEmitter_BodyLimit(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		var buf bytes.Buffer
		em, err := events.NewEmitter(&buf, events.Options{CurlewVersion: "0.1.0-dev"})
		if err != nil {
			t.Fatalf("NewEmitter: %v", err)
		}
		if got := em.BodyLimit(); got != events.DefaultBodyLimit {
			t.Errorf("expected BodyLimit=%d, got %d", events.DefaultBodyLimit, got)
		}
	})

	t.Run("override", func(t *testing.T) {
		var buf bytes.Buffer
		em, err := events.NewEmitter(&buf, events.Options{CurlewVersion: "0.1.0-dev", BodyLimit: 512})
		if err != nil {
			t.Fatalf("NewEmitter: %v", err)
		}
		if got := em.BodyLimit(); got != 512 {
			t.Errorf("expected BodyLimit=512, got %d", got)
		}
	})
}

func TestEmitter_RunError_ClassifiesError(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "test-run-error",
		CurlewVersion: "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	runErr := &apierrors.NetworkError{
		Kind:    apierrors.NetworkDNS,
		Message: "DNS failed",
		Hint:    "check DNS",
	}

	if err := em.EmitRunError(runErr); err != nil {
		t.Fatalf("EmitRunError: %v", err)
	}

	lines := splitLines(&buf)
	m := parseLine(t, lines[0])

	if m["kind"] != "run.error" {
		t.Errorf("expected kind=run.error, got %v", m["kind"])
	}

	errObj, ok := m["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %v", m["error"])
	}
	if errObj["category"] != "network" {
		t.Errorf("expected category=network, got %v", errObj["category"])
	}
	if errObj["code"] != "NETWORK_DNS" {
		t.Errorf("expected code=NETWORK_DNS, got %v", errObj["code"])
	}
}

func TestEmitter_RunError_NilErr(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "test-run-nilerr",
		CurlewVersion: "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	err = em.EmitRunError(nil)
	if err == nil {
		t.Error("expected error for nil err on EmitRunError")
	}
}

func TestEmitter_RunStart_NilCLIArgs(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "test-run-nilargs",
		CurlewVersion: "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	// Passing nil cliArgs must produce cli_args:[] (not null) to be schema-valid.
	if err := em.EmitRunStart(nil, "", ""); err != nil {
		t.Fatalf("EmitRunStart: %v", err)
	}

	lines := splitLines(&buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	m := parseLine(t, lines[0])

	// cli_args must be an array (not null) for JSON Schema compliance.
	cliArgs, ok := m["cli_args"].([]any)
	if !ok {
		t.Fatalf("cli_args must be a JSON array (not null), got %T: %v", m["cli_args"], m["cli_args"])
	}
	if len(cliArgs) != 0 {
		t.Errorf("expected empty cli_args array, got %v", cliArgs)
	}
}

func TestEmitter_AssertionResult(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "test-run-assert",
		CurlewVersion: "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	if err := em.EmitAssertionResult(events.AssertionResultInput{
		RequestID: "req-1", Type: "status", Expected: "200", Actual: "404", Passed: false,
	}); err != nil {
		t.Fatalf("EmitAssertionResult: %v", err)
	}

	lines := splitLines(&buf)
	m := parseLine(t, lines[0])

	if m["kind"] != "assertion.result" {
		t.Errorf("expected kind=assertion.result, got %v", m["kind"])
	}
	if m["passed"] != false {
		t.Errorf("expected passed=false, got %v", m["passed"])
	}
	if m["type"] != "status" {
		t.Errorf("expected type=status, got %v", m["type"])
	}
	if m["expected"] != "200" {
		t.Errorf("expected expected=200, got %v", m["expected"])
	}
	if m["actual"] != "404" {
		t.Errorf("expected actual=404, got %v", m["actual"])
	}
}

// TestEvents_v11_Selection verifies that EmitRunStartWithInput correctly
// populates the optional "selection" field in the run.start event, that the
// field is omitted when selection is nil or empty, and that schema_version is
// "1.1" on every emitted event.
func TestEvents_v11_Selection(t *testing.T) {
	tests := []struct {
		name       string
		selection  []string
		wantField  bool
		wantValues []string
	}{
		{
			name:       "selection present with one value",
			selection:  []string{"Get user"},
			wantField:  true,
			wantValues: []string{"Get user"},
		},
		{
			name:       "selection present with two values",
			selection:  []string{"Get user", "Update user"},
			wantField:  true,
			wantValues: []string{"Get user", "Update user"},
		},
		{
			name:      "selection nil omits field",
			selection: nil,
			wantField: false,
		},
		{
			name:      "selection empty omits field",
			selection: []string{},
			wantField: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			em, err := events.NewEmitter(&buf, events.Options{
				Clock:         fixedClock(t, "2026-04-24T10:00:00Z"),
				RunID:         "test-v11-" + tt.name,
				CurlewVersion: "0.1.0-test",
			})
			if err != nil {
				t.Fatalf("NewEmitter: %v", err)
			}

			if err := em.EmitRunStartWithInput(events.RunStartInput{
				CLIArgs:        []string{"run", "test.yaml"},
				CollectionFile: "test.yaml",
				Selection:      tt.selection,
			}); err != nil {
				t.Fatalf("EmitRunStartWithInput: %v", err)
			}

			lines := splitLines(&buf)
			if len(lines) != 1 {
				t.Fatalf("expected 1 line, got %d", len(lines))
			}
			m := parseLine(t, lines[0])

			// schema_version must be "1.5" (current version).
			if sv := m["schema_version"]; sv != "1.5" {
				t.Errorf("schema_version = %q, want %q", sv, "1.5")
			}

			// selection field presence.
			_, hasField := m["selection"]
			if tt.wantField && !hasField {
				t.Errorf("selection field missing from run.start event")
			}
			if !tt.wantField && hasField {
				t.Errorf("selection field should be omitted when nil/empty, but was present: %v", m["selection"])
			}

			if tt.wantField {
				rawSel, ok := m["selection"].([]any)
				if !ok {
					t.Fatalf("selection is not an array: %T %v", m["selection"], m["selection"])
				}
				got := make([]string, len(rawSel))
				for i, v := range rawSel {
					s, ok := v.(string)
					if !ok {
						t.Fatalf("selection[%d] is not a string: %T", i, v)
					}
					got[i] = s
				}
				if len(got) != len(tt.wantValues) {
					t.Errorf("selection len = %d, want %d; got %v", len(got), len(tt.wantValues), got)
				} else {
					for i, want := range tt.wantValues {
						if got[i] != want {
							t.Errorf("selection[%d] = %q, want %q", i, got[i], want)
						}
					}
				}
			}
		})
	}
}

// TestEvents_v12_SchemaVersionOnAllEvents verifies that schema_version is "1.5"
// on every emitted event kind (M9-001 promoted the schema from 1.1 to 1.2).
func TestEvents_v12_SchemaVersionOnAllEvents(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-24T10:00:00Z"),
		RunID:         "schema-v12-test",
		CurlewVersion: "0.1.0-test",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	_ = em.EmitRunStart([]string{"run"}, "test.yaml", "")
	_ = em.EmitRequestStart("r1", "test", "Test", "GET", "https://example.com", "main", "test.yaml", 1)
	_ = em.EmitAssertionResult(events.AssertionResultInput{
		RequestID: "r1", Type: "status", Expected: "200", Actual: "200", Passed: true,
	})
	_ = em.EmitRequestEnd(events.RequestEndInput{RequestID: "r1", Outcome: events.OutcomePassed, StatusCode: 200, Duration: 1 * time.Millisecond})
	_ = em.EmitRunError(fmt.Errorf("some error"))
	_ = em.EmitRunEnd(1, 1, 0, 0, 0)

	for i, line := range splitLines(&buf) {
		m := parseLine(t, line)
		if sv := m["schema_version"]; sv != "1.5" {
			t.Errorf("line %d: schema_version = %q, want %q (kind: %v)", i, sv, "1.5", m["kind"])
		}
	}
}

// TestEmitter_RequestStartCarriesSlug verifies that EmitRequestStart emits
// request_slug and schema_version 1.2 when a non-empty slug is provided.
func TestEmitter_RequestStartCarriesSlug(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-25T10:00:00Z"),
		RunID:         "rs-slug-001",
		CurlewVersion: "0.1.0-test",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	if err := em.EmitRequestStart("req-1", "get-user", "Get user",
		"GET", "https://example.com", "main", "test.yaml", 5); err != nil {
		t.Fatalf("EmitRequestStart: %v", err)
	}
	line := strings.TrimSpace(buf.String())
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["schema_version"] != "1.5" {
		t.Errorf("schema_version = %v, want 1.3", got["schema_version"])
	}
	if got["request_slug"] != "get-user" {
		t.Errorf("request_slug = %v, want get-user", got["request_slug"])
	}
}

// TestEmitter_RequestStartOmitsEmptySlug verifies that an empty requestSlug
// is not included in the output (omitempty behaviour).
func TestEmitter_RequestStartOmitsEmptySlug(t *testing.T) {
	var buf bytes.Buffer
	em, _ := events.NewEmitter(&buf, events.Options{CurlewVersion: "0.1.0-test"})
	_ = em.EmitRequestStart("req-1", "", "n", "GET", "u", "", "", 0)
	if strings.Contains(buf.String(), "request_slug") {
		t.Error("expected request_slug omitted when empty, got:", buf.String())
	}
}

// TestEmitter_RequestEndCarriesSlug verifies that EmitRequestEnd emits request_slug
// when provided via RequestEndInput.
func TestEmitter_RequestEndCarriesSlug(t *testing.T) {
	var buf bytes.Buffer
	em, _ := events.NewEmitter(&buf, events.Options{
		CurlewVersion: "0.1.0-test",
	})
	_ = em.EmitRequestEnd(events.RequestEndInput{
		RequestID:   "req-1",
		RequestSlug: "create-post",
		Outcome:     events.OutcomePassed,
		StatusCode:  201,
		Duration:    5 * time.Millisecond,
	})
	if !strings.Contains(buf.String(), `"request_slug":"create-post"`) {
		t.Error("expected request_slug field in output:", buf.String())
	}
}

// TestEvents_v12_RequestSlug verifies the DoD requirement:
// "TestEvents_v12_RequestSlug passes: emitted events report schema_version '1.3';
// request_slug field present and matches derivation."
//
// The test covers every emit path that produces request.start / request.end:
//   - EmitRequestStart with a slug → slug appears in request.start
//   - EmitRequestEnd with a slug → slug appears in request.end
//   - A full paired start+end sequence verifies schema_version "1.5" on both
//   - omitempty: empty slug is omitted from both events
//   - Multiple request pairs with distinct slugs (simulating sequential run output)
func TestEvents_v12_RequestSlug(t *testing.T) {
	t.Run("schema_version is 1.3 on request.start and request.end", func(t *testing.T) {
		var buf bytes.Buffer
		em, err := events.NewEmitter(&buf, events.Options{
			Clock:         fixedClock(t, "2026-04-25T10:00:00Z"),
			RunID:         "v12-rs-001",
			CurlewVersion: "0.1.0-test",
		})
		if err != nil {
			t.Fatalf("NewEmitter: %v", err)
		}
		_ = em.EmitRequestStart("req-1", "get-user", "Get user", "GET", "https://example.com/users/1", "main", "test.yaml", 5)
		_ = em.EmitRequestEnd(events.RequestEndInput{
			RequestID:   "req-1",
			RequestSlug: "get-user",
			Outcome:     events.OutcomePassed,
			StatusCode:  200,
			Duration:    10 * time.Millisecond,
		})
		lines := splitLines(&buf)
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}
		for _, line := range lines {
			m := parseLine(t, line)
			if sv := m["schema_version"]; sv != "1.5" {
				t.Errorf("schema_version = %v, want 1.3 (kind: %v)", sv, m["kind"])
			}
		}
	})

	t.Run("request_slug present on request.start matches derivation", func(t *testing.T) {
		cases := []struct {
			name         string
			requestName  string
			expectedSlug string
		}{
			{"simple", "Get user", "get-user"},
			{"punctuation stripped", "Create Post!", "create-post"},
			{"unicode normalized", "Héllo", "hello"},
			{"data-driven iteration", "Create user [2/3]", "create-user-2-3"},
			{"GET path pattern", "GET /users/{id}", "get-users-id"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var buf bytes.Buffer
				em, _ := events.NewEmitter(&buf, events.Options{CurlewVersion: "0.1.0-test"})
				_ = em.EmitRequestStart("req-1", tc.expectedSlug, tc.requestName, "GET", "https://example.com", "main", "", 0)
				m := parseLine(t, strings.TrimSpace(buf.String()))
				if m["request_slug"] != tc.expectedSlug {
					t.Errorf("request_slug = %v, want %q (name=%q)", m["request_slug"], tc.expectedSlug, tc.requestName)
				}
			})
		}
	})

	t.Run("request_slug present on request.end matches paired start", func(t *testing.T) {
		cases := []struct {
			slug string
		}{
			{"get-user"},
			{"create-post"},
			{"delete-resource"},
		}
		for _, tc := range cases {
			t.Run(tc.slug, func(t *testing.T) {
				var buf bytes.Buffer
				em, _ := events.NewEmitter(&buf, events.Options{CurlewVersion: "0.1.0-test"})
				_ = em.EmitRequestStart("req-1", tc.slug, "irrelevant name", "GET", "https://example.com", "main", "", 0)
				_ = em.EmitRequestEnd(events.RequestEndInput{
					RequestID:   "req-1",
					RequestSlug: tc.slug,
					Outcome:     events.OutcomePassed,
					StatusCode:  200,
					Duration:    5 * time.Millisecond,
				})
				lines := splitLines(&buf)
				if len(lines) != 2 {
					t.Fatalf("expected 2 lines, got %d", len(lines))
				}
				startM := parseLine(t, lines[0])
				endM := parseLine(t, lines[1])
				if startM["request_slug"] != tc.slug {
					t.Errorf("start request_slug = %v, want %q", startM["request_slug"], tc.slug)
				}
				if endM["request_slug"] != tc.slug {
					t.Errorf("end request_slug = %v, want %q", endM["request_slug"], tc.slug)
				}
				if startM["request_slug"] != endM["request_slug"] {
					t.Errorf("start slug %v != end slug %v (must match)", startM["request_slug"], endM["request_slug"])
				}
			})
		}
	})

	t.Run("empty slug is omitted from request.start (omitempty)", func(t *testing.T) {
		var buf bytes.Buffer
		em, _ := events.NewEmitter(&buf, events.Options{CurlewVersion: "0.1.0-test"})
		_ = em.EmitRequestStart("req-1", "", "Name Without Slug", "GET", "https://example.com", "main", "", 0)
		m := parseLine(t, strings.TrimSpace(buf.String()))
		if _, ok := m["request_slug"]; ok {
			t.Errorf("request_slug should be absent when empty, got: %v", m["request_slug"])
		}
	})

	t.Run("empty slug is omitted from request.end (omitempty)", func(t *testing.T) {
		var buf bytes.Buffer
		em, _ := events.NewEmitter(&buf, events.Options{CurlewVersion: "0.1.0-test"})
		_ = em.EmitRequestEnd(events.RequestEndInput{
			RequestID:  "req-1",
			Outcome:    events.OutcomePassed,
			StatusCode: 200,
			Duration:   5 * time.Millisecond,
		})
		m := parseLine(t, strings.TrimSpace(buf.String()))
		if _, ok := m["request_slug"]; ok {
			t.Errorf("request_slug should be absent when empty, got: %v", m["request_slug"])
		}
	})

	t.Run("multiple requests have distinct slugs matching their names", func(t *testing.T) {
		var buf bytes.Buffer
		em, err := events.NewEmitter(&buf, events.Options{
			Clock:         fixedClock(t, "2026-04-25T10:00:00Z"),
			RunID:         "v12-multi-001",
			CurlewVersion: "0.1.0-test",
		})
		if err != nil {
			t.Fatalf("NewEmitter: %v", err)
		}

		wantPairs := []struct {
			id   string
			slug string
			name string
		}{
			{"req-1", "get-user", "Get user"},
			{"req-2", "create-post", "Create Post!"},
			{"req-3", "delete-resource", "Delete resource"},
		}
		for _, p := range wantPairs {
			_ = em.EmitRequestStart(p.id, p.slug, p.name, "GET", "https://example.com", "main", "", 0)
			_ = em.EmitRequestEnd(events.RequestEndInput{
				RequestID:   p.id,
				RequestSlug: p.slug,
				Outcome:     events.OutcomePassed,
				StatusCode:  200,
				Duration:    5 * time.Millisecond,
			})
		}

		lines := splitLines(&buf)
		if len(lines) != 6 {
			t.Fatalf("expected 6 lines (3 pairs), got %d", len(lines))
		}
		// Pairs come in order: start, end, start, end, start, end.
		for i, p := range wantPairs {
			startM := parseLine(t, lines[i*2])
			endM := parseLine(t, lines[i*2+1])
			if startM["request_slug"] != p.slug {
				t.Errorf("pair %d start: request_slug = %v, want %q", i, startM["request_slug"], p.slug)
			}
			if endM["request_slug"] != p.slug {
				t.Errorf("pair %d end: request_slug = %v, want %q", i, endM["request_slug"], p.slug)
			}
		}
	})
}
