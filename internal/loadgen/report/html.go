package report

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// htmlReport is the data view passed to the HTML template.
type htmlReport struct {
	Title              string
	Metrics            Metrics
	SeriesJSON         template.JS
	SmallSampleWarning bool
}

// titleOrDefault returns t if non-empty, otherwise the default report title.
func titleOrDefault(t string) string {
	if t == "" {
		return "curlew perf report"
	}
	return t
}

// WriteHTML renders a self-contained HTML perf report to w. The title
// defaults to "curlew perf report" when empty.
func WriteHTML(w io.Writer, m Metrics, title string) error {
	// Build the time-series data for Chart.js from the bucket slice.
	type seriesPoint struct {
		Second int   `json:"second"`
		P50Ms  int64 `json:"p50_ms"`
		P95Ms  int64 `json:"p95_ms"`
		Errors int   `json:"errors"`
	}
	points := make([]seriesPoint, 0, len(m.Buckets))
	for _, b := range m.Buckets {
		points = append(points, seriesPoint{
			Second: b.SecondOffset,
			P50Ms:  b.P50.Milliseconds(),
			P95Ms:  b.P95.Milliseconds(),
			Errors: b.Errors,
		})
	}
	rawJSON, err := json.Marshal(points)
	if err != nil {
		return fmt.Errorf("marshal chart series: %w", err)
	}

	data := htmlReport{
		Title:              titleOrDefault(title),
		Metrics:            m,
		SeriesJSON:         template.JS(rawJSON), //nolint:gosec // values are ints only, no user strings
		SmallSampleWarning: m.SmallSampleWarning,
	}

	tmpl, err := template.New("perf").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("parse html template: %w", err)
	}
	if err := tmpl.Execute(w, data); err != nil {
		return fmt.Errorf("render html report: %w", err)
	}
	return nil
}

// htmlTemplate is the self-contained HTML perf report template. Chart.js is
// loaded via CDN (scope explicitly allows this; --offline bundling is a future
// slice). The canvas id "perf-latency-chart" is referenced in tests and in
// the observable contract.
const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Title}}</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.1"></script>
<style>
  body { font-family: sans-serif; margin: 2rem; background: #f8f9fa; color: #212529; }
  h1 { font-size: 1.5rem; margin-bottom: 1rem; }
  .cards { display: flex; flex-wrap: wrap; gap: 1rem; margin-bottom: 2rem; }
  .card { background: #fff; border-radius: 8px; padding: 1rem 1.5rem; box-shadow: 0 1px 3px rgba(0,0,0,.1); min-width: 140px; }
  .card-label { font-size: .75rem; color: #6c757d; text-transform: uppercase; letter-spacing: .05em; }
  .card-value { font-size: 1.5rem; font-weight: 600; margin-top: .25rem; }
  .chart-container { background: #fff; border-radius: 8px; padding: 1.5rem; box-shadow: 0 1px 3px rgba(0,0,0,.1); max-width: 900px; }
  .warning { background: #fff3cd; border: 1px solid #ffc107; border-radius: 6px; padding: .75rem 1rem; margin-bottom: 1rem; color: #856404; }
  .no-samples { background: #f0f0f0; border-radius: 8px; padding: 2rem; text-align: center; color: #6c757d; font-size: 1.1rem; }
</style>
</head>
<body>
<h1>{{.Title}}</h1>
{{if .SmallSampleWarning}}
<div class="warning">&#9888; p99 may be imprecise for small samples (&lt;100 requests). Showing max instead.</div>
{{end}}
{{if eq .Metrics.Requests 0}}
<div class="no-samples">No samples collected.</div>
{{else}}
<div class="cards">
  <div class="card"><div class="card-label">Requests</div><div class="card-value">{{.Metrics.Requests}}</div></div>
  <div class="card"><div class="card-label">p50</div><div class="card-value">{{.Metrics.P50.Milliseconds}}ms</div></div>
  <div class="card"><div class="card-label">p95</div><div class="card-value">{{.Metrics.P95.Milliseconds}}ms</div></div>
  <div class="card"><div class="card-label">p99</div><div class="card-value">{{.Metrics.P99.Milliseconds}}ms</div></div>
  <div class="card"><div class="card-label">Throughput</div><div class="card-value">{{printf "%.1f" .Metrics.Throughput}} req/s</div></div>
</div>
<div class="chart-container">
  <canvas id="perf-latency-chart"></canvas>
</div>
<script>
(function() {
  var series = {{.SeriesJSON}};
  var labels = series.map(function(p) { return p.second + 's'; });
  var p50 = series.map(function(p) { return p.p50_ms; });
  var p95 = series.map(function(p) { return p.p95_ms; });
  var ctx = document.getElementById('perf-latency-chart').getContext('2d');
  new Chart(ctx, {
    type: 'line',
    data: {
      labels: labels,
      datasets: [
        { label: 'p50 (ms)', data: p50, borderColor: '#0d6efd', backgroundColor: 'rgba(13,110,253,.1)', fill: true, tension: 0.3 },
        { label: 'p95 (ms)', data: p95, borderColor: '#dc3545', backgroundColor: 'rgba(220,53,69,.05)', fill: false, tension: 0.3 }
      ]
    },
    options: { responsive: true, plugins: { legend: { position: 'top' } }, scales: { y: { beginAtZero: true, title: { display: true, text: 'Latency (ms)' } } } }
  });
})();
</script>
{{end}}
</body>
</html>`
