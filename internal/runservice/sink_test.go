package runservice_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/peterlindqvist/apitest/internal/httpexec"
	"github.com/peterlindqvist/apitest/internal/output/events"
	"github.com/peterlindqvist/apitest/internal/runner"
	"github.com/peterlindqvist/apitest/internal/runservice"
)

func TestEmitterSink_AllEventKinds(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{ApitestVersion: "t", RunID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	sink := runservice.NewEmitterSink(em, &bytes.Buffer{}, nil, false)
	sink.RequestStart(runner.RequestEvent{RequestID: "req-1", RequestSlug: "a", Name: "A", Method: "GET", URL: "http://x", Phase: "main"})
	sink.AssertionResult(runner.AssertionEvent{RequestID: "req-1", Type: "status", Expected: "200", Actual: "200", Passed: true})
	sink.RequestEnd(runner.RequestEndEvent{
		RequestID: "req-1", RequestSlug: "a", Outcome: "passed", StatusCode: 200,
		Duration: 5 * time.Millisecond, Attempts: 2,
		Timing: &httpexec.Timing{TTFB: time.Millisecond, Total: 2 * time.Millisecond, Reused: true},
	})
	out := buf.String()
	for _, want := range []string{"request.start", "assertion.result", "request.end", `"connection_reused":true`, `"attempts":2`} {
		if !strings.Contains(out, want) {
			t.Errorf("stream missing %q:\n%s", want, out)
		}
	}
}

func TestEmitterSink_FailedOutcomeGetsAssertionSentinel(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{ApitestVersion: "t", RunID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	sink := runservice.NewEmitterSink(em, &bytes.Buffer{}, nil, false)
	sink.RequestEnd(runner.RequestEndEvent{RequestID: "req-1", Outcome: "failed", StatusCode: 500})
	var ev struct {
		Error *struct {
			Category string `json:"category"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Error == nil || ev.Error.Category != "assertion" {
		t.Errorf("failed outcome error = %+v, want assertion sentinel", ev.Error)
	}
}

func TestTimingInfoFromExec(t *testing.T) {
	if runservice.TimingInfoFromExec(nil, 3) != nil {
		t.Error("nil timing must map to nil")
	}
	info := runservice.TimingInfoFromExec(&httpexec.Timing{
		DNS: time.Millisecond, Connect: 2 * time.Millisecond, TLS: 3 * time.Millisecond,
		TTFB: 4 * time.Millisecond, Download: 5 * time.Millisecond, Total: 15 * time.Millisecond,
	}, 1)
	if info.Attempts != 0 {
		t.Errorf("attempts = %d, want 0 (omitted when no retries)", info.Attempts)
	}
	if info.DNSUs == nil || *info.DNSUs != 1000 || info.TotalUs != 15000 {
		t.Errorf("info = %+v", info)
	}
	reused := runservice.TimingInfoFromExec(&httpexec.Timing{Total: time.Millisecond, Reused: true}, 4)
	if reused.DNSUs != nil || reused.ConnectUs != nil || reused.TLSUs != nil {
		t.Error("absent phases must stay nil pointers")
	}
	if !reused.Reused || reused.Attempts != 4 {
		t.Errorf("reused = %+v", reused)
	}
}
