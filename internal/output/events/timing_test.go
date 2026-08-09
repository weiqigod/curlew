package events_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/output/events"
)

func TestSchemaVersion_Is14(t *testing.T) {
	if events.SchemaVersion != "1.6" {
		t.Fatalf("SchemaVersion = %q, want \"1.3\"", events.SchemaVersion)
	}
}

func int64p(v int64) *int64 { return &v }

func TestEmitRequestEnd_TimingField(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{CurlewVersion: "test", RunID: "r1"})
	if err != nil {
		t.Fatal(err)
	}

	err = em.EmitRequestEnd(events.RequestEndInput{
		RequestID: "req-1",
		Outcome:   events.OutcomePassed,
		Duration:  10 * time.Millisecond,
		Timing: &events.TimingInfo{
			DNSUs:      int64p(120),
			ConnectUs:  int64p(800),
			TTFBUs:     int64p(4500),
			DownloadUs: int64p(90),
			TotalUs:    5600,
			Attempts:   3,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var ev map[string]any
	if err := json.Unmarshal(buf.Bytes(), &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	timing, ok := ev["timing"].(map[string]any)
	if !ok {
		t.Fatalf("timing field missing or wrong type in %s", buf.String())
	}
	if timing["total_us"] != float64(5600) {
		t.Errorf("total_us = %v, want 5600", timing["total_us"])
	}
	if timing["dns_us"] != float64(120) {
		t.Errorf("dns_us = %v, want 120", timing["dns_us"])
	}
	if timing["attempts"] != float64(3) {
		t.Errorf("attempts = %v, want 3", timing["attempts"])
	}
	if _, present := timing["tls_us"]; present {
		t.Error("tls_us present despite nil pointer (phase did not occur)")
	}
	if _, present := timing["connection_reused"]; present {
		t.Error("connection_reused present despite false (omitempty)")
	}
}

func TestEmitRequestEnd_NoTimingOmitted(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{CurlewVersion: "test", RunID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := em.EmitRequestEnd(events.RequestEndInput{
		RequestID: "req-1", Outcome: events.OutcomeSkipped,
	}); err != nil {
		t.Fatal(err)
	}
	var ev map[string]any
	if err := json.Unmarshal(buf.Bytes(), &ev); err != nil {
		t.Fatal(err)
	}
	if _, present := ev["timing"]; present {
		t.Error("timing present on event without timing input")
	}
}

func TestTimingInfoFromExec_ZeroAndReused(t *testing.T) {
	// Distinguishing "0 µs" from "did not occur": pointer fields stay nil when
	// the phase is absent, and a measured-but-fast phase serializes as 0.
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{CurlewVersion: "test", RunID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := em.EmitRequestEnd(events.RequestEndInput{
		RequestID: "req-1", Outcome: events.OutcomePassed,
		Timing: &events.TimingInfo{TTFBUs: int64p(0), TotalUs: 12, Reused: true},
	}); err != nil {
		t.Fatal(err)
	}
	var ev struct {
		Timing map[string]any `json:"timing"`
	}
	if err := json.Unmarshal(buf.Bytes(), &ev); err != nil {
		t.Fatal(err)
	}
	if v, present := ev.Timing["ttfb_us"]; !present || v != float64(0) {
		t.Errorf("ttfb_us = %v (present=%v), want explicit 0", v, present)
	}
	if ev.Timing["connection_reused"] != true {
		t.Error("connection_reused not true")
	}
}
