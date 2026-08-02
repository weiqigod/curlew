package report

import (
	"strings"
	"testing"
	"time"
)

func TestSummaryLine_MatchesObservable(t *testing.T) {
	m := Metrics{
		Requests:   42,
		P50:        11 * time.Millisecond,
		P95:        22 * time.Millisecond,
		P99:        33 * time.Millisecond,
		ErrorRate:  0,
		Throughput: 14.0,
	}
	got := SummaryLine(m)
	want := "Results: requests=42, p50=11ms, p95=22ms, p99=33ms, throughput=14.0req/s, error_rate=0%"
	if got != want {
		t.Errorf("SummaryLine() = %q, want %q", got, want)
	}
}

func TestSummaryLine_IncludesThroughput(t *testing.T) {
	m := Metrics{
		Requests:   100,
		Throughput: 50.0,
	}
	got := SummaryLine(m)
	if !strings.Contains(got, "throughput=50.0req/s") {
		t.Errorf("SummaryLine() missing throughput; got: %q", got)
	}
}

func TestSummaryLine_SmallSampleSuffix(t *testing.T) {
	m := Metrics{
		Requests:           10,
		SmallSampleWarning: true,
	}
	got := SummaryLine(m)
	if !strings.Contains(got, "(p99 may be imprecise for small samples)") {
		t.Errorf("SummaryLine() missing small sample warning; got: %q", got)
	}
}

func TestSummaryLine_ErrorRateRounding(t *testing.T) {
	tests := []struct {
		name      string
		errorRate float64
		wantPct   string
	}{
		{"12.3% rounds down to 12%", 0.123, "error_rate=12%"},
		{"12.9% rounds up to 13%", 0.129, "error_rate=13%"},
		{"0%", 0.0, "error_rate=0%"},
		{"100%", 1.0, "error_rate=100%"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := Metrics{ErrorRate: tc.errorRate}
			got := SummaryLine(m)
			if !strings.Contains(got, tc.wantPct) {
				t.Errorf("SummaryLine() = %q, want to contain %q", got, tc.wantPct)
			}
		})
	}
}
