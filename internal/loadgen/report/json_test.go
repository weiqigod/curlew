package report

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestWriteJSON_GeneratedAtUsesNowFn(t *testing.T) {
	// Override nowFn so generated_at is deterministic in the output.
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	orig := nowFn
	nowFn = func() time.Time { return fixed }
	defer func() { nowFn = orig }()

	var buf bytes.Buffer
	if err := WriteJSON(&buf, Metrics{}, "ts-test"); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	want := "2026-01-02T03:04:05Z"
	if result["generated_at"] != want {
		t.Errorf("generated_at = %v, want %q", result["generated_at"], want)
	}
}

func TestWriteJSON_SchemaAndFields(t *testing.T) {
	a := New()
	for i := 1; i <= 10; i++ {
		a.Observe(Sample{
			StartOffsetNs: 0,
			LatencyNs:     int64(time.Duration(i) * time.Millisecond),
			Success:       i%2 == 0,
		})
	}
	m := a.Metrics(2 * time.Second)

	var buf bytes.Buffer
	if err := WriteJSON(&buf, m, "test run"); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if result["schema"] != "curlew.perf.v1" {
		t.Errorf("schema = %v, want curlew.perf.v1", result["schema"])
	}
	if result["title"] != "test run" {
		t.Errorf("title = %v, want 'test run'", result["title"])
	}

	metrics, ok := result["metrics"].(map[string]interface{})
	if !ok {
		t.Fatalf("metrics field missing or wrong type")
	}
	if int(metrics["requests"].(float64)) != 10 {
		t.Errorf("metrics.requests = %v, want 10", metrics["requests"])
	}
	if _, ok := metrics["p95_ms"]; !ok {
		t.Error("metrics.p95_ms field missing")
	}

	buckets, ok := result["buckets"].([]interface{})
	if !ok {
		t.Fatalf("buckets field missing or wrong type")
	}
	if len(buckets) == 0 {
		t.Error("buckets empty, want at least one bucket")
	}
	b0 := buckets[0].(map[string]interface{})
	if _, ok := b0["second"]; !ok {
		t.Error("bucket missing 'second' field")
	}
}

func TestWriteJSON_DeterministicBucketOrder(t *testing.T) {
	// Add samples into buckets 2, 0, 1 in that order; verify JSON
	// buckets field is ordered 0, 1, 2 by Second.
	a := New()
	offsets := []int64{
		int64(2 * time.Second), // bucket 2
		0,                      // bucket 0
		int64(time.Second),     // bucket 1
	}
	for _, off := range offsets {
		a.Observe(Sample{StartOffsetNs: off, LatencyNs: int64(time.Millisecond), Success: true})
	}
	m := a.Metrics(3 * time.Second)

	var buf bytes.Buffer
	if err := WriteJSON(&buf, m, "order test"); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	buckets := result["buckets"].([]interface{})
	if len(buckets) != 3 {
		t.Fatalf("len(buckets) = %d, want 3", len(buckets))
	}
	for i, want := range []float64{0, 1, 2} {
		b := buckets[i].(map[string]interface{})
		got := b["second"].(float64)
		if got != want {
			t.Errorf("buckets[%d].second = %v, want %v", i, got, want)
		}
	}
}

func TestWriteJSON_EmptyMetricsValidJSON(t *testing.T) {
	// Zero Metrics still produces valid JSON with requests=0.
	m := Metrics{}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, m, "empty"); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, output: %s", err, buf.String())
	}
	metrics := result["metrics"].(map[string]interface{})
	if int(metrics["requests"].(float64)) != 0 {
		t.Errorf("metrics.requests = %v, want 0", metrics["requests"])
	}
}
