package report

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// nowFn is the time source for JSON generation timestamps. Override in tests
// via package-level assignment for deterministic golden-file output.
var nowFn = time.Now

type jsonReport struct {
	Schema             string       `json:"schema"`
	GeneratedAt        string       `json:"generated_at"`
	Title              string       `json:"title"`
	Metrics            jsonMetrics  `json:"metrics"`
	Buckets            []jsonBucket `json:"buckets"`
	SmallSampleWarning bool         `json:"small_sample_warning"`
}

type jsonMetrics struct {
	Requests   int     `json:"requests"`
	Successes  int     `json:"successes"`
	Failures   int     `json:"failures"`
	ElapsedMs  int64   `json:"elapsed_ms"`
	P50Ms      int64   `json:"p50_ms"`
	P95Ms      int64   `json:"p95_ms"`
	P99Ms      int64   `json:"p99_ms"`
	MinMs      int64   `json:"min_ms"`
	MaxMs      int64   `json:"max_ms"`
	MeanMs     int64   `json:"mean_ms"`
	ErrorRate  float64 `json:"error_rate"`
	Throughput float64 `json:"throughput_rps"`
}

type jsonBucket struct {
	Second   int   `json:"second"`
	Requests int   `json:"requests"`
	Errors   int   `json:"errors"`
	P50Ms    int64 `json:"p50_ms"`
	P95Ms    int64 `json:"p95_ms"`
}

// WriteJSON encodes the metrics as the curlew perf JSON report.
// The caller is responsible for writing to a truncated file (via os.WriteFile
// or os.O_TRUNC) to guarantee overwrite semantics.
func WriteJSON(w io.Writer, m Metrics, title string) error {
	r := jsonReport{
		Schema:             "curlew.perf.v1",
		GeneratedAt:        nowFn().UTC().Format(time.RFC3339),
		Title:              title,
		SmallSampleWarning: m.SmallSampleWarning,
		Metrics: jsonMetrics{
			Requests:   m.Requests,
			Successes:  m.Successes,
			Failures:   m.Failures,
			ElapsedMs:  m.Elapsed.Milliseconds(),
			P50Ms:      m.P50.Milliseconds(),
			P95Ms:      m.P95.Milliseconds(),
			P99Ms:      m.P99.Milliseconds(),
			MinMs:      m.Min.Milliseconds(),
			MaxMs:      m.Max.Milliseconds(),
			MeanMs:     m.Mean.Milliseconds(),
			ErrorRate:  m.ErrorRate,
			Throughput: m.Throughput,
		},
		Buckets: make([]jsonBucket, 0, len(m.Buckets)),
	}
	for _, b := range m.Buckets {
		r.Buckets = append(r.Buckets, jsonBucket{
			Second:   b.SecondOffset,
			Requests: b.Requests,
			Errors:   b.Errors,
			P50Ms:    b.P50.Milliseconds(),
			P95Ms:    b.P95.Milliseconds(),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("encode perf report: %w", err)
	}
	return nil
}
