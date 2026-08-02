package report

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteHTML_ContainsRequiredElements(t *testing.T) {
	a := New()
	for i := 1; i <= 10; i++ {
		a.Observe(Sample{
			StartOffsetNs: 0,
			LatencyNs:     int64(time.Duration(i) * time.Millisecond),
			Success:       true,
		})
	}
	m := a.Metrics(2 * time.Second)

	var buf bytes.Buffer
	if err := WriteHTML(&buf, m, ""); err != nil {
		t.Fatalf("WriteHTML() error = %v", err)
	}
	out := buf.String()

	checks := []struct {
		desc    string
		contain string
	}{
		{"title tag", "<title>apitest perf report</title>"},
		{"Chart.js CDN", "cdn.jsdelivr.net/npm/chart.js"},
		{"canvas id", "perf-latency-chart"},
		{"p50_ms series data", `"p50_ms"`},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.contain) {
			t.Errorf("WriteHTML() output missing %s: %q not found", c.desc, c.contain)
		}
	}
}

func TestWriteHTML_CustomTitle(t *testing.T) {
	m := Metrics{}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, m, "my custom title"); err != nil {
		t.Fatalf("WriteHTML() error = %v", err)
	}
	if !strings.Contains(buf.String(), "my custom title") {
		t.Error("WriteHTML() output missing custom title")
	}
}

func TestWriteHTML_DefaultTitle(t *testing.T) {
	// Empty title defaults to "apitest perf report"
	m := Metrics{}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, m, ""); err != nil {
		t.Fatalf("WriteHTML() error = %v", err)
	}
	if !strings.Contains(buf.String(), "<title>apitest perf report</title>") {
		t.Error("WriteHTML() output missing default title in <title> tag")
	}
}

func TestWriteHTML_EmptyMetrics_NoSamplesBanner(t *testing.T) {
	// Zero Metrics: no samples banner should be present
	m := Metrics{}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, m, ""); err != nil {
		t.Fatalf("WriteHTML() error = %v", err)
	}
	if !strings.Contains(buf.String(), "No samples") {
		t.Error("WriteHTML() missing 'No samples' banner for zero-request metrics")
	}
}

func TestWriteHTML_SmallSampleWarning(t *testing.T) {
	m := Metrics{Requests: 10, SmallSampleWarning: true}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, m, ""); err != nil {
		t.Fatalf("WriteHTML() error = %v", err)
	}
	if !strings.Contains(buf.String(), "p99 may be imprecise") {
		t.Error("WriteHTML() missing small sample warning text")
	}
}
