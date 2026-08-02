package output

import (
	"fmt"
	"html/template"
	"io"
	"math"
	"sort"
)

// HTMLReport is the top-level data structure passed to the HTML template.
type HTMLReport struct {
	Name        string
	Status      string // "passed", "failed", "error"
	Total       int
	Passed      int
	Failed      int
	Skipped     int
	DurationMs  int64
	Requests    []HTMLRequest
	GeneratedAt string // RFC3339 timestamp

	// Data-driven visualization (nil/empty when no data-driven requests ran).
	DataDrivenGroups []HTMLDataDrivenGroup

	// Parallel execution visualization (nil/empty when --parallel was not used).
	Waves               []HTMLWave
	IsParallel          bool
	WaveCount           int
	MaxParallelism      int
	SpeedupFactor       string // formatted "2.34x" or "N/A"
	TotalWaveDurationMs int64
}

// HTMLRequest represents one request in the HTML report.
type HTMLRequest struct {
	Name       string
	Status     string // "passed", "failed", "skipped", "error"
	Method     string
	URL        string
	StatusCode int
	DurationMs int64
	Assertions []HTMLAssertion
	Error      string // non-empty when Status == "error"
	SkipReason string // non-empty when Status == "skipped"
}

// HTMLAssertion represents one assertion outcome in the HTML report.
type HTMLAssertion struct {
	Type     string
	Expected string
	Actual   string
	Passed   bool
}

// HTMLDataDrivenGroup represents one data-driven request and its iterations.
type HTMLDataDrivenGroup struct {
	Name             string // base request name
	Total            int    // total iterations
	Passed           int
	Failed           int
	Skipped          int
	Errored          int
	PassRatePercent  int // 0..100 integer
	AvgDurationMs    int64
	TotalDurationMs  int64
	ThroughputPerSec float64  // iterations per second
	DataColumns      []string // sorted column names across iterations
	Iterations       []HTMLIteration
	TimelineBars     []HTMLTimelineBar
}

// HTMLIteration is one data-driven iteration row in the filterable table.
type HTMLIteration struct {
	Index      int
	Label      string // "1/100"
	Status     string // "passed", "failed", "skipped", "error"
	StatusCode int
	DurationMs int64
	Data       map[string]string
	Error      string
}

// HTMLTimelineBar is one bar in the inline SVG iteration timeline.
type HTMLTimelineBar struct {
	XPercent      float64
	YPercent      float64
	WidthPercent  float64
	HeightPercent float64
	Color         string
	Title         string
}

// HTMLWave represents one parallel execution wave.
type HTMLWave struct {
	Index      int // 0-based wave number
	DurationMs int64
	Items      []HTMLWaveItem
}

// HTMLWaveItem is one request within a wave.
type HTMLWaveItem struct {
	Name       string
	Status     string
	DurationMs int64
	StatusCode int
}

// IterationInput is the minimal shape the output package needs from the runner
// to build data-driven groups, decoupling it from runner.RequestResult.
type IterationInput struct {
	GroupName  string
	Index      int
	Total      int
	Status     string // "passed"|"failed"|"skipped"|"error"
	StatusCode int
	DurationMs int64
	Data       map[string]string
	Error      string
}

// WaveInput is one request-within-a-wave input for BuildWaves.
type WaveInput struct {
	WaveIndex  int
	Name       string
	Status     string
	DurationMs int64
	StatusCode int
}

// BuildDataDrivenGroups aggregates flat iteration inputs into groups, one per
// data-driven request name, preserving input order. Returns nil for empty input.
func BuildDataDrivenGroups(iters []IterationInput) []HTMLDataDrivenGroup {
	if len(iters) == 0 {
		return nil
	}

	// Preserve group order using an ordered key list.
	order := make([]string, 0)
	groups := make(map[string]*HTMLDataDrivenGroup)
	colSets := make(map[string]map[string]struct{})

	for _, it := range iters {
		g, exists := groups[it.GroupName]
		if !exists {
			g = &HTMLDataDrivenGroup{Name: it.GroupName}
			groups[it.GroupName] = g
			colSets[it.GroupName] = make(map[string]struct{})
			order = append(order, it.GroupName)
		}

		g.Total++
		g.TotalDurationMs += it.DurationMs

		switch it.Status {
		case "passed":
			g.Passed++
		case "failed":
			g.Failed++
		case "skipped":
			g.Skipped++
		default:
			g.Errored++
		}

		iter := HTMLIteration{
			Index:      it.Index,
			Label:      fmt.Sprintf("%d/%d", it.Index+1, it.Total),
			Status:     it.Status,
			StatusCode: it.StatusCode,
			DurationMs: it.DurationMs,
			Data:       it.Data,
			Error:      it.Error,
		}
		g.Iterations = append(g.Iterations, iter)

		for k := range it.Data {
			colSets[it.GroupName][k] = struct{}{}
		}
	}

	out := make([]HTMLDataDrivenGroup, 0, len(order))
	for _, name := range order {
		g := groups[name]

		// Pass rate
		if g.Total > 0 {
			g.PassRatePercent = int(math.Round(float64(g.Passed) / float64(g.Total) * 100))
		}

		// Average duration
		if g.Total > 0 {
			g.AvgDurationMs = g.TotalDurationMs / int64(g.Total)
		}

		// Throughput (iterations per second)
		if g.TotalDurationMs > 0 {
			g.ThroughputPerSec = float64(g.Total) / (float64(g.TotalDurationMs) / 1000.0)
		}

		// Sorted data columns
		cols := make([]string, 0, len(colSets[name]))
		for k := range colSets[name] {
			cols = append(cols, k)
		}
		sort.Strings(cols)
		g.DataColumns = cols

		// Timeline bars
		g.TimelineBars = buildTimelineBars(g.Iterations)

		out = append(out, *g)
	}

	return out
}

// buildTimelineBars creates inline SVG bar data for the iteration duration timeline.
func buildTimelineBars(iters []HTMLIteration) []HTMLTimelineBar {
	if len(iters) == 0 {
		return nil
	}

	// Find max duration for scaling.
	var maxDur int64
	for _, it := range iters {
		if it.DurationMs > maxDur {
			maxDur = it.DurationMs
		}
	}

	n := len(iters)
	bars := make([]HTMLTimelineBar, n)
	widthPct := 100.0 / float64(n)

	for i, it := range iters {
		heightPct := 100.0
		if maxDur > 0 {
			heightPct = float64(it.DurationMs) / float64(maxDur) * 100.0
		}
		if heightPct < 0.5 {
			heightPct = 0.5 // minimum visible height
		}

		color := "#22863a" // passed
		switch it.Status {
		case "failed", "error":
			color = "#cb2431"
		case "skipped":
			color = "#959da5"
		}

		bars[i] = HTMLTimelineBar{
			XPercent:      float64(i) * widthPct,
			YPercent:      100.0 - heightPct,
			WidthPercent:  widthPct,
			HeightPercent: heightPct,
			Color:         color,
			Title:         fmt.Sprintf("%s: %dms", it.Label, it.DurationMs),
		}
	}

	return bars
}

// BuildWaves groups wave inputs by WaveIndex, with durations from waveDurations
// (indexed by wave). Returns nil when len(waveInputs) == 0.
func BuildWaves(waveInputs []WaveInput, waveDurations []int64) []HTMLWave {
	if len(waveInputs) == 0 {
		return nil
	}

	// Preserve wave order.
	waveOrder := make([]int, 0)
	waveMap := make(map[int]*HTMLWave)

	for _, wi := range waveInputs {
		w, exists := waveMap[wi.WaveIndex]
		if !exists {
			w = &HTMLWave{Index: wi.WaveIndex}
			if wi.WaveIndex < len(waveDurations) {
				w.DurationMs = waveDurations[wi.WaveIndex]
			}
			waveMap[wi.WaveIndex] = w
			waveOrder = append(waveOrder, wi.WaveIndex)
		}
		w.Items = append(w.Items, HTMLWaveItem{
			Name:       wi.Name,
			Status:     wi.Status,
			DurationMs: wi.DurationMs,
			StatusCode: wi.StatusCode,
		})
	}

	out := make([]HTMLWave, 0, len(waveOrder))
	for _, idx := range waveOrder {
		out = append(out, *waveMap[idx])
	}
	return out
}

// ComputeSpeedup returns a formatted speedup string like "2.34x" or "N/A" when
// wave durations sum to zero (avoids divide-by-zero for skipped or empty runs).
func ComputeSpeedup(totalRequestDurationMs, totalWaveDurationMs int64) string {
	if totalWaveDurationMs == 0 {
		return "N/A"
	}
	speedup := float64(totalRequestDurationMs) / float64(totalWaveDurationMs)
	return fmt.Sprintf("%.2fx", speedup)
}

// WriteHTML generates a self-contained HTML report and writes it to w.
func WriteHTML(w io.Writer, report *HTMLReport) error {
	if report == nil {
		return fmt.Errorf("html report: nil report")
	}
	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("parse html template: %w", err)
	}
	if err := tmpl.Execute(w, report); err != nil {
		return fmt.Errorf("execute html template: %w", err)
	}
	return nil
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Name}} — curlew report</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background: #f5f6f7; color: #24292e; line-height: 1.5; }
  .container { max-width: 960px; margin: 0 auto; padding: 24px 16px; }
  h1 { font-size: 1.5rem; margin-bottom: 8px; }
  h2 { font-size: 1.1rem; margin-bottom: 12px; color: #24292e; }
  .meta { color: #586069; font-size: 0.85rem; margin-bottom: 16px; }

  /* Summary dashboard */
  .summary { display: flex; gap: 16px; flex-wrap: wrap; margin-bottom: 24px; }
  .summary-card { background: #fff; border: 1px solid #e1e4e8; border-radius: 6px; padding: 16px 20px; min-width: 120px; text-align: center; }
  .summary-card .label { font-size: 0.75rem; text-transform: uppercase; color: #586069; }
  .summary-card .value { font-size: 1.6rem; font-weight: 600; }
  .summary-card.passed .value { color: #22863a; }
  .summary-card.failed .value { color: #cb2431; }
  .summary-card.skipped .value { color: #959da5; }
  .summary-card.total .value { color: #24292e; }
  .summary-card.duration .value { color: #0366d6; font-size: 1.1rem; }

  /* Status badge */
  .badge { display: inline-block; padding: 2px 8px; border-radius: 12px; font-size: 0.75rem; font-weight: 600; text-transform: uppercase; }
  .badge.passed { background: #dcffe4; color: #22863a; }
  .badge.failed { background: #ffdce0; color: #cb2431; }
  .badge.skipped { background: #f1f1f1; color: #586069; }
  .badge.error { background: #ffdce0; color: #cb2431; }

  /* Request table */
  .requests { background: #fff; border: 1px solid #e1e4e8; border-radius: 6px; overflow: hidden; margin-bottom: 24px; }
  .req-row { padding: 12px 16px; border-bottom: 1px solid #e1e4e8; cursor: pointer; }
  .req-row:last-child { border-bottom: none; }
  .req-row:hover { background: #f6f8fa; }
  .req-header { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .req-name { font-weight: 600; }
  .req-method { font-family: monospace; font-size: 0.85rem; color: #0366d6; }
  .req-url { font-family: monospace; font-size: 0.8rem; color: #586069; word-break: break-all; }
  .req-status-code { font-family: monospace; font-size: 0.85rem; }
  .req-duration { font-size: 0.8rem; color: #586069; }

  /* Details (collapsible) */
  .req-details { display: none; padding: 8px 16px 12px 16px; background: #f6f8fa; border-top: 1px solid #e1e4e8; }
  .req-details.open { display: block; }
  .assertion-row { padding: 4px 0; font-size: 0.85rem; font-family: monospace; }
  .assertion-row.pass { color: #22863a; }
  .assertion-row.fail { color: #cb2431; }
  .error-msg { color: #cb2431; font-family: monospace; font-size: 0.85rem; padding: 4px 0; }
  .skip-msg { color: #586069; font-style: italic; font-size: 0.85rem; padding: 4px 0; }

  /* Overall status bar */
  .status-bar { padding: 8px 16px; border-radius: 6px; margin-bottom: 16px; font-weight: 600; }
  .status-bar.passed { background: #dcffe4; color: #22863a; }
  .status-bar.failed { background: #ffdce0; color: #cb2431; }
  .status-bar.error { background: #ffdce0; color: #cb2431; }

  /* Data-driven group */
  .dd-group { background: #fff; border: 1px solid #e1e4e8; border-radius: 6px; padding: 20px; margin-bottom: 24px; }
  .dd-name { margin-bottom: 12px; }
  .dd-summary { display: flex; gap: 12px; flex-wrap: wrap; margin-bottom: 16px; }
  .dd-timeline { width: 100%; height: 40px; display: block; margin-bottom: 12px; }
  .dd-filters { display: flex; gap: 8px; margin-bottom: 12px; }
  .dd-filter { padding: 4px 12px; border: 1px solid #e1e4e8; border-radius: 4px; background: #f6f8fa; cursor: pointer; font-size: 0.85rem; }
  .dd-filter.active { background: #0366d6; color: #fff; border-color: #0366d6; }
  .dd-table { width: 100%; border-collapse: collapse; font-size: 0.85rem; }
  .dd-table th { text-align: left; padding: 6px 10px; border-bottom: 2px solid #e1e4e8; color: #586069; font-size: 0.75rem; text-transform: uppercase; }
  .dd-table td { padding: 6px 10px; border-bottom: 1px solid #e1e4e8; }
  .dd-table tr:last-child td { border-bottom: none; }

  /* Parallel wave diagram */
  .parallel-summary { background: #fff; border: 1px solid #e1e4e8; border-radius: 6px; padding: 20px; margin-bottom: 24px; }
  .waves { display: flex; flex-direction: column; gap: 12px; margin-top: 16px; }
  .wave { border: 1px solid #e1e4e8; border-radius: 4px; overflow: hidden; }
  .wave-header { padding: 6px 12px; background: #f6f8fa; font-size: 0.85rem; font-weight: 600; display: flex; align-items: center; gap: 8px; }
  .wave-duration { color: #586069; font-weight: normal; }
  .wave-items { display: flex; flex-wrap: wrap; gap: 8px; padding: 10px 12px; }
  .wave-item { border: 1px solid #e1e4e8; border-radius: 4px; padding: 6px 10px; font-size: 0.8rem; min-width: 120px; }
  .wave-item.passed { border-left: 3px solid #22863a; }
  .wave-item.failed { border-left: 3px solid #cb2431; }
  .wave-item.skipped { border-left: 3px solid #959da5; }
  .wave-item.error { border-left: 3px solid #cb2431; }
  .wave-item-name { display: block; font-weight: 600; }
  .wave-item-duration { color: #586069; }

  .footer { text-align: center; color: #959da5; font-size: 0.75rem; margin-top: 32px; }
</style>
</head>
<body>
<div class="container">
  <h1>{{.Name}}</h1>
  <div class="meta">Generated: {{.GeneratedAt}}</div>

  <div class="status-bar {{.Status}}">
    {{if eq .Status "passed"}}All tests passed{{else if eq .Status "failed"}}Some tests failed{{else}}Error{{end}}
  </div>

  <div class="summary">
    <div class="summary-card total"><div class="label">Total</div><div class="value">{{.Total}}</div></div>
    <div class="summary-card passed"><div class="label">Passed</div><div class="value">{{.Passed}} passed</div></div>
    <div class="summary-card failed"><div class="label">Failed</div><div class="value">{{.Failed}} failed</div></div>
    <div class="summary-card skipped"><div class="label">Skipped</div><div class="value">{{.Skipped}} skipped</div></div>
    <div class="summary-card duration"><div class="label">Duration</div><div class="value">{{.DurationMs}}ms</div></div>
  </div>

  {{if .Requests}}
  <div class="requests">
    {{range $i, $r := .Requests}}
    <div class="req-row" onclick="toggle('details-{{$i}}')">
      <div class="req-header">
        <span class="badge {{$r.Status}}">{{$r.Status}}</span>
        <span class="req-name">{{$r.Name}}</span>
        {{if $r.Method}}<span class="req-method">{{$r.Method}}</span>{{end}}
        {{if $r.URL}}<span class="req-url">{{$r.URL}}</span>{{end}}
        {{if $r.StatusCode}}<span class="req-status-code">{{$r.StatusCode}}</span>{{end}}
        {{if $r.DurationMs}}<span class="req-duration">{{$r.DurationMs}}ms</span>{{end}}
      </div>
    </div>
    <div id="details-{{$i}}" class="req-details">
      {{if $r.Error}}<div class="error-msg">Error: {{$r.Error}}</div>{{end}}
      {{if $r.SkipReason}}<div class="skip-msg">Skipped: {{$r.SkipReason}}</div>{{end}}
      {{range $a := $r.Assertions}}
      <div class="assertion-row {{if $a.Passed}}pass{{else}}fail{{end}}">
        {{if $a.Passed}}✓{{else}}✗{{end}} {{$a.Type}}: expected {{$a.Expected}}, got {{$a.Actual}}
      </div>
      {{end}}
      {{if not $r.Assertions}}{{if not $r.Error}}{{if not $r.SkipReason}}<div class="skip-msg">No assertion details</div>{{end}}{{end}}{{end}}
    </div>
    {{end}}
  </div>
  {{end}}

  {{range $gi, $g := .DataDrivenGroups}}
  <div class="dd-group" data-group="{{$gi}}">
    <h2 class="dd-name">{{$g.Name}}</h2>
    <div class="dd-summary">
      <div class="summary-card"><div class="label">Iterations</div><div class="value">{{$g.Total}}</div></div>
      <div class="summary-card"><div class="label">Pass rate</div><div class="value">{{$g.PassRatePercent}}%</div></div>
      <div class="summary-card"><div class="label">Avg duration</div><div class="value">{{$g.AvgDurationMs}}ms</div></div>
      <div class="summary-card"><div class="label">Throughput</div><div class="value">{{printf "%.2f" $g.ThroughputPerSec}}/s</div></div>
    </div>

    {{if $g.TimelineBars}}
    <svg class="dd-timeline" viewBox="0 0 100 100" preserveAspectRatio="none" aria-label="Iteration timeline">
      {{range $b := $g.TimelineBars}}
      <rect x="{{printf "%.3f" $b.XPercent}}" y="{{printf "%.3f" $b.YPercent}}"
            width="{{printf "%.3f" $b.WidthPercent}}" height="{{printf "%.3f" $b.HeightPercent}}"
            fill="{{$b.Color}}"><title>{{$b.Title}}</title></rect>
      {{end}}
    </svg>
    {{end}}

    <div class="dd-filters" data-group="{{$gi}}">
      <button class="dd-filter active" data-filter="all" onclick="ddFilter({{$gi}},'all',this)">All</button>
      <button class="dd-filter" data-filter="passed" onclick="ddFilter({{$gi}},'passed',this)">Passed</button>
      <button class="dd-filter" data-filter="failed" onclick="ddFilter({{$gi}},'failed',this)">Failed</button>
    </div>

    <table class="dd-table">
      <thead>
        <tr>
          <th>#</th><th>Status</th><th>Duration</th>
          {{range $col := $g.DataColumns}}<th>{{$col}}</th>{{end}}
        </tr>
      </thead>
      <tbody>
        {{range $it := $g.Iterations}}
        <tr class="dd-row dd-status-{{$it.Status}}" data-status="{{$it.Status}}">
          <td>{{$it.Label}}</td>
          <td><span class="badge {{$it.Status}}">{{$it.Status}}</span></td>
          <td>{{$it.DurationMs}}ms</td>
          {{range $col := $g.DataColumns}}<td>{{index $it.Data $col}}</td>{{end}}
        </tr>
        {{end}}
      </tbody>
    </table>
  </div>
  {{end}}

  {{if .IsParallel}}
  <div class="parallel-summary">
    <h2>Parallel execution</h2>
    <div class="summary">
      <div class="summary-card"><div class="label">Waves</div><div class="value">{{.WaveCount}}</div></div>
      <div class="summary-card"><div class="label">Max parallelism</div><div class="value">{{.MaxParallelism}}</div></div>
      <div class="summary-card"><div class="label">Speedup</div><div class="value">{{.SpeedupFactor}}</div></div>
    </div>

    <div class="waves">
      {{range $wi, $w := .Waves}}
      <div class="wave">
        <div class="wave-header">Wave {{$w.Index}} <span class="wave-duration">{{$w.DurationMs}}ms</span></div>
        <div class="wave-items">
          {{range $item := $w.Items}}
          <div class="wave-item {{$item.Status}}" title="{{$item.Name}} ({{$item.DurationMs}}ms)">
            <span class="wave-item-name">{{$item.Name}}</span>
            <span class="wave-item-duration">{{$item.DurationMs}}ms</span>
          </div>
          {{end}}
        </div>
      </div>
      {{end}}
    </div>
  </div>
  {{end}}

  <div class="footer">Generated by curlew</div>
</div>
<script>
function toggle(id) {
  var el = document.getElementById(id);
  if (el) { el.classList.toggle('open'); }
}
function ddFilter(gi, status, btn) {
  var group = document.querySelector('.dd-group[data-group="' + gi + '"]');
  if (!group) return;
  var rows = group.querySelectorAll('.dd-row');
  for (var i = 0; i < rows.length; i++) {
    if (status === 'all' || rows[i].getAttribute('data-status') === status) {
      rows[i].style.display = '';
    } else {
      rows[i].style.display = 'none';
    }
  }
  var btns = group.querySelectorAll('.dd-filter');
  for (var j = 0; j < btns.length; j++) btns[j].classList.remove('active');
  btn.classList.add('active');
}
</script>
</body>
</html>`
