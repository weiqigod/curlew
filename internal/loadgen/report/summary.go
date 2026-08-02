package report

import (
	"fmt"
	"strings"
)

// SummaryLine returns the stdout one-liner. Matches behavior 1 of M5-012:
//
//	"Results: requests=N, p50=Xms, p95=Yms, p99=Zms, throughput=X.Xreq/s, error_rate=0%"
//
// With the small-sample note appended when Metrics.SmallSampleWarning is true.
func SummaryLine(m Metrics) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Results: requests=%d, p50=%dms, p95=%dms, p99=%dms, throughput=%.1freq/s, error_rate=%d%%",
		m.Requests,
		m.P50.Milliseconds(),
		m.P95.Milliseconds(),
		m.P99.Milliseconds(),
		m.Throughput,
		int(m.ErrorRate*100+0.5),
	)
	if m.SmallSampleWarning {
		b.WriteString(" (p99 may be imprecise for small samples)")
	}
	return b.String()
}
