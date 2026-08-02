package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestWriteHTML(t *testing.T) {
	tests := []struct {
		name       string
		input      *HTMLReport
		wantSubstr []string
		wantAbsent []string
	}{
		{
			"self-contained HTML with no external references",
			&HTMLReport{
				Name: "My Collection", Status: "passed",
				Total: 2, Passed: 2, DurationMs: 500,
				Requests: []HTMLRequest{
					{Name: "Get Users", Status: "passed", Method: "GET", URL: "http://api/users", StatusCode: 200, DurationMs: 250},
					{Name: "Get Items", Status: "passed", Method: "GET", URL: "http://api/items", StatusCode: 200, DurationMs: 250},
				},
			},
			[]string{"<!DOCTYPE html>", "<html", "</html>", "<style>", "My Collection", "Get Users", "Get Items", "2 passed"},
			[]string{"<link rel=\"stylesheet\"", "<script src="},
		},
		{
			"summary shows total passed failed skipped duration",
			&HTMLReport{
				Name: "Test", Status: "failed",
				Total: 4, Passed: 2, Failed: 1, Skipped: 1, DurationMs: 1234,
				Requests: []HTMLRequest{
					{Name: "A", Status: "passed", StatusCode: 200, DurationMs: 100},
					{Name: "B", Status: "failed", StatusCode: 500, DurationMs: 200},
					{Name: "C", Status: "skipped"},
					{Name: "D", Status: "passed", StatusCode: 200, DurationMs: 300},
				},
			},
			[]string{"4", "2 passed", "1 failed", "1 skipped", "1234"},
			nil,
		},
		{
			"request shows name status method URL duration",
			&HTMLReport{
				Name: "Detail", Status: "passed",
				Total: 1, Passed: 1, DurationMs: 456,
				Requests: []HTMLRequest{
					{Name: "Create User", Status: "passed", Method: "POST", URL: "http://api/users", StatusCode: 201, DurationMs: 456},
				},
			},
			[]string{"Create User", "POST", "http://api/users", "201", "456"},
			nil,
		},
		{
			"failed assertion details shown",
			&HTMLReport{
				Name: "Assert", Status: "failed",
				Total: 1, Failed: 1, DurationMs: 100,
				Requests: []HTMLRequest{
					{
						Name: "Check Status", Status: "failed", Method: "GET",
						URL: "http://api/health", StatusCode: 500, DurationMs: 100,
						Assertions: []HTMLAssertion{
							{Type: "status", Expected: "200", Actual: "500", Passed: false},
						},
					},
				},
			},
			[]string{"status", "200", "500"},
			nil,
		},
		{
			"no external CSS or JS dependencies",
			&HTMLReport{
				Name: "Standalone", Status: "passed",
				Total: 0, DurationMs: 0,
			},
			[]string{"<style>"},
			[]string{"<link rel=\"stylesheet\"", "<script src=\"http", "<script src=\"/"},
		},
		{
			"skipped request shows skip reason",
			&HTMLReport{
				Name: "Skip", Status: "passed",
				Total: 1, Skipped: 1, DurationMs: 0,
				Requests: []HTMLRequest{
					{Name: "Dep Failed", Status: "skipped", SkipReason: "dependency failed"},
				},
			},
			[]string{"skipped", "dependency failed"},
			nil,
		},
		{
			"error request shows error message",
			&HTMLReport{
				Name: "ErrTest", Status: "failed",
				Total: 1, Failed: 1, DurationMs: 50,
				Requests: []HTMLRequest{
					{Name: "Bad Call", Status: "error", Error: "connection refused"},
				},
			},
			[]string{"error", "connection refused"},
			nil,
		},
		{
			"special HTML characters escaped",
			&HTMLReport{
				Name: "Test <script>", Status: "passed",
				Total: 1, Passed: 1, DurationMs: 10,
				Requests: []HTMLRequest{
					{Name: "XSS & \"Test\"", Status: "passed", Method: "GET", URL: "http://api/<endpoint>", StatusCode: 200, DurationMs: 10},
				},
			},
			[]string{"&lt;script&gt;", "XSS &amp;", "&lt;endpoint&gt;"},
			[]string{"<script>alert"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteHTML(&buf, tt.input); err != nil {
				t.Fatalf("WriteHTML() error = %v", err)
			}
			got := buf.String()
			for _, sub := range tt.wantSubstr {
				if !strings.Contains(got, sub) {
					t.Errorf("output missing expected substring %q", sub)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("output contains unexpected substring %q", absent)
				}
			}
		})
	}
}

func TestWriteHTML_validHTML(t *testing.T) {
	report := &HTMLReport{
		Name: "Valid", Status: "passed",
		Total: 1, Passed: 1, DurationMs: 100,
		Requests: []HTMLRequest{
			{Name: "Test", Status: "passed", Method: "GET", URL: "http://test", StatusCode: 200, DurationMs: 100},
		},
	}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, report); err != nil {
		t.Fatalf("WriteHTML() error = %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "<!DOCTYPE html>") {
		t.Error("output should start with <!DOCTYPE html>")
	}
	if !strings.Contains(got, "</html>") {
		t.Error("output should contain closing </html> tag")
	}
}

func TestWriteHTML_nilReport(t *testing.T) {
	var buf bytes.Buffer
	err := WriteHTML(&buf, nil)
	if err == nil {
		t.Fatal("WriteHTML(nil) should return an error")
	}
	if !strings.Contains(err.Error(), "nil report") {
		t.Errorf("error should mention nil report, got: %v", err)
	}
}

// errWriter is an io.Writer that always returns an error.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("simulated write error")
}

func TestWriteHTML_writeError(t *testing.T) {
	report := &HTMLReport{
		Name: "Test", Status: "passed",
		Total: 1, Passed: 1, DurationMs: 100,
	}
	err := WriteHTML(errWriter{}, report)
	if err == nil {
		t.Fatal("WriteHTML with failing writer should return an error")
	}
	if !strings.Contains(err.Error(), "execute html template") {
		t.Errorf("error should be wrapped with context, got: %v", err)
	}
}

func TestWriteHTML_generatedAt(t *testing.T) {
	report := &HTMLReport{
		Name: "Time", Status: "passed",
		Total: 0, DurationMs: 0,
		GeneratedAt: "2026-04-09T12:00:00Z",
	}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, report); err != nil {
		t.Fatalf("WriteHTML() error = %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "2026-04-09T12:00:00Z") {
		t.Error("output should contain generated-at timestamp")
	}
}

// ---- M2-028: data-driven and parallel visualization tests ----

func TestBuildDataDrivenGroups(t *testing.T) {
	tests := []struct {
		name  string
		input []IterationInput
		check func(t *testing.T, got []HTMLDataDrivenGroup)
	}{
		{
			name:  "empty input returns nil",
			input: nil,
			check: func(t *testing.T, got []HTMLDataDrivenGroup) {
				if got != nil {
					t.Errorf("want nil, got %v", got)
				}
			},
		},
		{
			name: "single group with mixed statuses",
			input: []IterationInput{
				{GroupName: "Login", Index: 0, Total: 3, Status: "passed", DurationMs: 100, Data: map[string]string{"user": "alice"}},
				{GroupName: "Login", Index: 1, Total: 3, Status: "failed", DurationMs: 200, Data: map[string]string{"user": "bob"}},
				{GroupName: "Login", Index: 2, Total: 3, Status: "skipped", DurationMs: 0, Data: map[string]string{"user": "charlie"}},
			},
			check: func(t *testing.T, got []HTMLDataDrivenGroup) {
				if len(got) != 1 {
					t.Fatalf("want 1 group, got %d", len(got))
				}
				g := got[0]
				if g.Name != "Login" {
					t.Errorf("Name = %q, want %q", g.Name, "Login")
				}
				if g.Total != 3 {
					t.Errorf("Total = %d, want 3", g.Total)
				}
				if g.Passed != 1 {
					t.Errorf("Passed = %d, want 1", g.Passed)
				}
				if g.Failed != 1 {
					t.Errorf("Failed = %d, want 1", g.Failed)
				}
				if g.Skipped != 1 {
					t.Errorf("Skipped = %d, want 1", g.Skipped)
				}
				if len(g.Iterations) != 3 {
					t.Errorf("Iterations = %d, want 3", len(g.Iterations))
				}
			},
		},
		{
			name: "two groups preserve input order",
			input: []IterationInput{
				{GroupName: "A", Index: 0, Total: 1, Status: "passed", DurationMs: 50},
				{GroupName: "B", Index: 0, Total: 1, Status: "passed", DurationMs: 80},
			},
			check: func(t *testing.T, got []HTMLDataDrivenGroup) {
				if len(got) != 2 {
					t.Fatalf("want 2 groups, got %d", len(got))
				}
				if got[0].Name != "A" {
					t.Errorf("group[0].Name = %q, want A", got[0].Name)
				}
				if got[1].Name != "B" {
					t.Errorf("group[1].Name = %q, want B", got[1].Name)
				}
			},
		},
		{
			name: "pass rate rounds to nearest integer",
			input: []IterationInput{
				{GroupName: "X", Index: 0, Total: 3, Status: "passed", DurationMs: 100},
				{GroupName: "X", Index: 1, Total: 3, Status: "passed", DurationMs: 100},
				{GroupName: "X", Index: 2, Total: 3, Status: "failed", DurationMs: 100},
			},
			check: func(t *testing.T, got []HTMLDataDrivenGroup) {
				if len(got) != 1 {
					t.Fatalf("want 1 group, got %d", len(got))
				}
				// 2/3 = 66.67% rounds to 67
				if got[0].PassRatePercent != 67 {
					t.Errorf("PassRatePercent = %d, want 67", got[0].PassRatePercent)
				}
			},
		},
		{
			name: "throughput handles zero total duration",
			input: []IterationInput{
				{GroupName: "Z", Index: 0, Total: 2, Status: "passed", DurationMs: 0},
				{GroupName: "Z", Index: 1, Total: 2, Status: "passed", DurationMs: 0},
			},
			check: func(t *testing.T, got []HTMLDataDrivenGroup) {
				if len(got) != 1 {
					t.Fatalf("want 1 group, got %d", len(got))
				}
				// TotalDurationMs == 0, throughput should be 0 (not a divide-by-zero panic)
				if got[0].ThroughputPerSec != 0 {
					t.Errorf("ThroughputPerSec = %f, want 0", got[0].ThroughputPerSec)
				}
			},
		},
		{
			name: "data columns sorted alphabetically",
			input: []IterationInput{
				{GroupName: "G", Index: 0, Total: 2, Status: "passed", DurationMs: 10, Data: map[string]string{"zebra": "z", "apple": "a"}},
				{GroupName: "G", Index: 1, Total: 2, Status: "passed", DurationMs: 10, Data: map[string]string{"mango": "m", "apple": "aa"}},
			},
			check: func(t *testing.T, got []HTMLDataDrivenGroup) {
				if len(got) != 1 {
					t.Fatalf("want 1 group, got %d", len(got))
				}
				cols := got[0].DataColumns
				want := []string{"apple", "mango", "zebra"}
				if len(cols) != len(want) {
					t.Fatalf("DataColumns = %v, want %v", cols, want)
				}
				for i, c := range cols {
					if c != want[i] {
						t.Errorf("DataColumns[%d] = %q, want %q", i, c, want[i])
					}
				}
			},
		},
		{
			name: "timeline bars scaled to max duration",
			input: []IterationInput{
				{GroupName: "T", Index: 0, Total: 2, Status: "passed", DurationMs: 100},
				{GroupName: "T", Index: 1, Total: 2, Status: "passed", DurationMs: 200},
			},
			check: func(t *testing.T, got []HTMLDataDrivenGroup) {
				if len(got) != 1 {
					t.Fatalf("want 1 group, got %d", len(got))
				}
				bars := got[0].TimelineBars
				if len(bars) != 2 {
					t.Fatalf("TimelineBars = %d, want 2", len(bars))
				}
				// Second bar has max duration so its HeightPercent should be 100
				if bars[1].HeightPercent != 100.0 {
					t.Errorf("bars[1].HeightPercent = %f, want 100.0", bars[1].HeightPercent)
				}
				// First bar should be 50% height
				if bars[0].HeightPercent != 50.0 {
					t.Errorf("bars[0].HeightPercent = %f, want 50.0", bars[0].HeightPercent)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildDataDrivenGroups(tt.input)
			tt.check(t, got)
		})
	}
}

func TestBuildWaves(t *testing.T) {
	tests := []struct {
		name          string
		inputs        []WaveInput
		waveDurations []int64
		want          []HTMLWave
	}{
		{
			name:          "empty returns nil",
			inputs:        nil,
			waveDurations: nil,
			want:          nil,
		},
		{
			name: "single wave with one request",
			inputs: []WaveInput{
				{WaveIndex: 0, Name: "Get Users", Status: "passed", DurationMs: 100, StatusCode: 200},
			},
			waveDurations: []int64{120},
			want: []HTMLWave{
				{Index: 0, DurationMs: 120, Items: []HTMLWaveItem{
					{Name: "Get Users", Status: "passed", DurationMs: 100, StatusCode: 200},
				}},
			},
		},
		{
			name: "three waves grouped by index",
			inputs: []WaveInput{
				{WaveIndex: 0, Name: "A", Status: "passed", DurationMs: 50},
				{WaveIndex: 1, Name: "B", Status: "passed", DurationMs: 60},
				{WaveIndex: 1, Name: "C", Status: "failed", DurationMs: 70},
				{WaveIndex: 2, Name: "D", Status: "passed", DurationMs: 80},
			},
			waveDurations: []int64{55, 75, 85},
			want: []HTMLWave{
				{Index: 0, DurationMs: 55, Items: []HTMLWaveItem{{Name: "A", Status: "passed", DurationMs: 50}}},
				{Index: 1, DurationMs: 75, Items: []HTMLWaveItem{
					{Name: "B", Status: "passed", DurationMs: 60},
					{Name: "C", Status: "failed", DurationMs: 70},
				}},
				{Index: 2, DurationMs: 85, Items: []HTMLWaveItem{{Name: "D", Status: "passed", DurationMs: 80}}},
			},
		},
		{
			name: "missing wave duration defaults to zero",
			inputs: []WaveInput{
				{WaveIndex: 0, Name: "A", Status: "passed", DurationMs: 50},
			},
			waveDurations: nil, // no durations provided
			want: []HTMLWave{
				{Index: 0, DurationMs: 0, Items: []HTMLWaveItem{{Name: "A", Status: "passed", DurationMs: 50}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildWaves(tt.inputs, tt.waveDurations)
			if tt.want == nil {
				if got != nil {
					t.Errorf("want nil, got %v", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len(waves) = %d, want %d", len(got), len(tt.want))
			}
			for i, w := range got {
				if w.Index != tt.want[i].Index {
					t.Errorf("wave[%d].Index = %d, want %d", i, w.Index, tt.want[i].Index)
				}
				if w.DurationMs != tt.want[i].DurationMs {
					t.Errorf("wave[%d].DurationMs = %d, want %d", i, w.DurationMs, tt.want[i].DurationMs)
				}
				if len(w.Items) != len(tt.want[i].Items) {
					t.Fatalf("wave[%d] items = %d, want %d", i, len(w.Items), len(tt.want[i].Items))
				}
				for j, item := range w.Items {
					wantItem := tt.want[i].Items[j]
					if item.Name != wantItem.Name {
						t.Errorf("wave[%d].item[%d].Name = %q, want %q", i, j, item.Name, wantItem.Name)
					}
					if item.Status != wantItem.Status {
						t.Errorf("wave[%d].item[%d].Status = %q, want %q", i, j, item.Status, wantItem.Status)
					}
					if item.DurationMs != wantItem.DurationMs {
						t.Errorf("wave[%d].item[%d].DurationMs = %d, want %d", i, j, item.DurationMs, wantItem.DurationMs)
					}
				}
			}
		})
	}
}

func TestComputeSpeedup(t *testing.T) {
	tests := []struct {
		name                        string
		totalRequestMs, totalWaveMs int64
		want                        string
	}{
		{"2x speedup", 1000, 500, "2.00x"},
		{"fractional speedup", 1234, 500, "2.47x"},
		{"wave time zero returns N/A", 1000, 0, "N/A"},
		{"no requests returns N/A", 0, 0, "N/A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeSpeedup(tt.totalRequestMs, tt.totalWaveMs)
			if got != tt.want {
				t.Errorf("ComputeSpeedup(%d, %d) = %q, want %q", tt.totalRequestMs, tt.totalWaveMs, got, tt.want)
			}
		})
	}
}

func TestWriteHTML_DataDrivenSection(t *testing.T) {
	threeIterGroup := HTMLDataDrivenGroup{
		Name:             "Login",
		Total:            3,
		Passed:           2,
		Failed:           1,
		PassRatePercent:  67,
		AvgDurationMs:    100,
		ThroughputPerSec: 10.0,
		DataColumns:      []string{"role", "user"},
		Iterations: []HTMLIteration{
			{Index: 0, Label: "1/3", Status: "passed", DurationMs: 100, Data: map[string]string{"user": "alice", "role": "admin"}},
			{Index: 1, Label: "2/3", Status: "passed", DurationMs: 100, Data: map[string]string{"user": "bob", "role": "user"}},
			{Index: 2, Label: "3/3", Status: "failed", DurationMs: 100, Data: map[string]string{"user": "carol", "role": "guest"}},
		},
		TimelineBars: []HTMLTimelineBar{
			{XPercent: 0, YPercent: 0, WidthPercent: 33.0, HeightPercent: 100, Color: "#22863a", Title: "1/3: 100ms"},
			{XPercent: 33, YPercent: 0, WidthPercent: 33.0, HeightPercent: 100, Color: "#22863a", Title: "2/3: 100ms"},
			{XPercent: 66, YPercent: 0, WidthPercent: 33.0, HeightPercent: 100, Color: "#cb2431", Title: "3/3: 100ms"},
		},
	}

	tests := []struct {
		name       string
		input      *HTMLReport
		wantSubstr []string
		wantAbsent []string
	}{
		{
			name:       "data-driven summary card shows iterations, pass rate, avg, throughput",
			input:      &HTMLReport{Name: "R", Status: "passed", DataDrivenGroups: []HTMLDataDrivenGroup{threeIterGroup}},
			wantSubstr: []string{"Iterations", "3", "Pass rate", "67%", "Avg duration", "100ms", "Throughput", "/s"},
			wantAbsent: nil,
		},
		{
			name:       "data-driven table shows iteration rows with data columns",
			input:      &HTMLReport{Name: "R", Status: "passed", DataDrivenGroups: []HTMLDataDrivenGroup{threeIterGroup}},
			wantSubstr: []string{"<th>role</th>", "<th>user</th>", "alice", "admin"},
			wantAbsent: nil,
		},
		{
			name:       "filter buttons rendered with onclick handler",
			input:      &HTMLReport{Name: "R", Status: "passed", DataDrivenGroups: []HTMLDataDrivenGroup{threeIterGroup}},
			wantSubstr: []string{`data-filter="all"`, `data-filter="passed"`, `data-filter="failed"`, "ddFilter"},
			wantAbsent: nil,
		},
		{
			name:       "failed iteration row carries data-status=failed",
			input:      &HTMLReport{Name: "R", Status: "passed", DataDrivenGroups: []HTMLDataDrivenGroup{threeIterGroup}},
			wantSubstr: []string{`data-status="passed"`, `data-status="failed"`},
			wantAbsent: nil,
		},
		{
			name:       "inline SVG timeline rendered when bars present",
			input:      &HTMLReport{Name: "R", Status: "passed", DataDrivenGroups: []HTMLDataDrivenGroup{threeIterGroup}},
			wantSubstr: []string{"<svg", "<rect", "</svg>"},
			wantAbsent: nil,
		},
		{
			name:       "no data-driven section when groups empty",
			input:      &HTMLReport{Name: "R", Status: "passed"},
			wantSubstr: nil,
			wantAbsent: []string{`class="dd-group"`, `<svg class="dd-timeline"`, `data-filter="all"`},
		},
		{
			name:       "report remains self-contained with data-driven section",
			input:      &HTMLReport{Name: "R", Status: "passed", DataDrivenGroups: []HTMLDataDrivenGroup{threeIterGroup}},
			wantSubstr: nil,
			wantAbsent: []string{`<link rel="stylesheet"`, `<script src="http`, `<script src="/`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteHTML(&buf, tt.input); err != nil {
				t.Fatalf("WriteHTML() error = %v", err)
			}
			got := buf.String()
			for _, sub := range tt.wantSubstr {
				if !strings.Contains(got, sub) {
					t.Errorf("output missing expected substring %q", sub)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("output contains unexpected substring %q", absent)
				}
			}
		})
	}
}

func TestWriteHTML_ParallelSection(t *testing.T) {
	tests := []struct {
		name       string
		input      *HTMLReport
		wantSubstr []string
		wantAbsent []string
	}{
		{
			name: "parallel summary shows waves, max parallelism, speedup",
			input: &HTMLReport{
				Name: "R", Status: "passed",
				IsParallel: true, WaveCount: 3, MaxParallelism: 4, SpeedupFactor: "2.50x",
				Waves: []HTMLWave{
					{Index: 0, DurationMs: 100, Items: []HTMLWaveItem{{Name: "A", Status: "passed", DurationMs: 100}}},
				},
			},
			wantSubstr: []string{"Waves", "3", "Max parallelism", "4", "Speedup", "2.50x"},
			wantAbsent: nil,
		},
		{
			name: "wave diagram shows each wave with its items",
			input: &HTMLReport{
				Name: "R", Status: "passed",
				IsParallel: true,
				Waves: []HTMLWave{
					{Index: 0, DurationMs: 100, Items: []HTMLWaveItem{{Name: "A", Status: "passed", DurationMs: 100}}},
					{Index: 1, DurationMs: 50, Items: []HTMLWaveItem{{Name: "B", Status: "passed", DurationMs: 50}}},
				},
			},
			wantSubstr: []string{"Wave 0", "Wave 1", "100ms", "50ms"},
			wantAbsent: nil,
		},
		{
			name:       "no parallel section when IsParallel false",
			input:      &HTMLReport{Name: "R", Status: "passed", IsParallel: false},
			wantSubstr: nil,
			wantAbsent: []string{"Parallel execution", "Max parallelism", `class="parallel-summary"`},
		},
		{
			name: "speedup N/A rendered when formatted as N/A",
			input: &HTMLReport{
				Name: "R", Status: "passed",
				IsParallel: true, SpeedupFactor: "N/A",
				Waves: []HTMLWave{{Index: 0, DurationMs: 0, Items: []HTMLWaveItem{}}},
			},
			wantSubstr: []string{"N/A"},
			wantAbsent: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteHTML(&buf, tt.input); err != nil {
				t.Fatalf("WriteHTML() error = %v", err)
			}
			got := buf.String()
			for _, sub := range tt.wantSubstr {
				if !strings.Contains(got, sub) {
					t.Errorf("output missing expected substring %q", sub)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("output contains unexpected substring %q", absent)
				}
			}
		})
	}
}

func TestBuildHTMLReport_DataDriven(t *testing.T) {
	// Compile-time check that the types exist and fields are addressable.
	_ = HTMLDataDrivenGroup{
		Name:             "G",
		Total:            1,
		Passed:           1,
		PassRatePercent:  100,
		AvgDurationMs:    50,
		TotalDurationMs:  50,
		ThroughputPerSec: 20.0,
		DataColumns:      []string{"key"},
		Iterations: []HTMLIteration{
			{Index: 0, Label: "1/1", Status: "passed", StatusCode: 200, DurationMs: 50, Data: map[string]string{"key": "val"}},
		},
		TimelineBars: []HTMLTimelineBar{
			{XPercent: 0, YPercent: 0, WidthPercent: 100, HeightPercent: 100, Color: "#22863a", Title: "1/1: 50ms"},
		},
	}
	_ = HTMLWave{
		Index:      0,
		DurationMs: 100,
		Items:      []HTMLWaveItem{{Name: "A", Status: "passed", DurationMs: 100, StatusCode: 200}},
	}
	_ = IterationInput{GroupName: "G", Index: 0, Total: 1, Status: "passed", StatusCode: 200, DurationMs: 50, Data: map[string]string{"k": "v"}}
	_ = WaveInput{WaveIndex: 0, Name: "A", Status: "passed", DurationMs: 100, StatusCode: 200}
}
