package output

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/retry"
)

func TestPrinterResult(t *testing.T) {
	tests := []struct {
		name      string
		rName     string
		result    *httpexec.Result
		passed    bool
		color     bool
		wantParts []string
		wantNot   []string
	}{
		{
			name:      "passing no-color shows checkmark",
			rName:     "Get Users",
			result:    &httpexec.Result{StatusCode: 200, Duration: 150 * time.Millisecond},
			passed:    true,
			color:     false,
			wantParts: []string{"✓", "Get Users", "200", "150ms"},
			wantNot:   []string{"\033["},
		},
		{
			name:      "failing no-color shows X",
			rName:     "Get Users",
			result:    &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond},
			passed:    false,
			color:     false,
			wantParts: []string{"✗", "Get Users"},
			wantNot:   []string{"\033["},
		},
		{
			name:      "passing with-color emits green ANSI",
			rName:     "Get",
			result:    &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond},
			passed:    true,
			color:     true,
			wantParts: []string{"✓", "\033[32m"},
		},
		{
			name:      "failing with-color emits red ANSI",
			rName:     "Get",
			result:    &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond},
			passed:    false,
			color:     true,
			wantParts: []string{"✗", "\033[31m"},
		},
	}

	// Retry count tests
	t.Run("retry count shown when > 0", func(t *testing.T) {
		var buf bytes.Buffer
		p := NewPrinter(&buf, false)
		p.Result("Flaky", &httpexec.Result{StatusCode: 200, Duration: 100 * time.Millisecond}, true, 2)
		got := buf.String()
		if !strings.Contains(got, "(retry: 2)") {
			t.Errorf("output %q should contain (retry: 2)", got)
		}
	})

	t.Run("no retry suffix when count is 0", func(t *testing.T) {
		var buf bytes.Buffer
		p := NewPrinter(&buf, false)
		p.Result("Normal", &httpexec.Result{StatusCode: 200, Duration: 100 * time.Millisecond}, true, 0)
		got := buf.String()
		if strings.Contains(got, "retry") {
			t.Errorf("output %q should not contain retry when count=0", got)
		}
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color)
			p.Result(tt.rName, tt.result, tt.passed, 0)
			got := buf.String()
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
			for _, part := range tt.wantNot {
				if strings.Contains(got, part) {
					t.Errorf("output %q should not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinterAssertionDetail(t *testing.T) {
	tests := []struct {
		name       string
		assertType string
		expected   string
		actual     string
		color      bool
		wantParts  []string
		wantNot    []string
	}{
		{
			name:       "status mismatch no-color",
			assertType: "status",
			expected:   "200",
			actual:     "404",
			color:      false,
			wantParts:  []string{"✗", "status", "expected 200", "got 404"},
			wantNot:    []string{"\033["},
		},
		{
			name:       "list expected no-color",
			assertType: "status",
			expected:   "[200, 201]",
			actual:     "500",
			color:      false,
			wantParts:  []string{"expected [200, 201]", "got 500"},
		},
		{
			name:       "with-color emits red ANSI",
			assertType: "status",
			expected:   "200",
			actual:     "404",
			color:      true,
			wantParts:  []string{"\033[31m"},
		},
		{
			// CEL failure messages are multiline: the expression source on the
			// first line followed by "  ref = value" lines. AssertionDetail must
			// preserve the full string so the failure context is visible.
			name:       "CEL failure message multiline preserved",
			assertType: "assertions[0].cel",
			expected:   "response.body.total == 10.0",
			actual:     "response.body.total == 10.0\n  response.body.total = 9.5",
			color:      false,
			wantParts: []string{
				"assertions[0].cel",
				"response.body.total == 10.0",
				"response.body.total = 9.5",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color)
			p.AssertionDetail(tt.assertType, tt.expected, tt.actual)
			got := buf.String()
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
			for _, part := range tt.wantNot {
				if strings.Contains(got, part) {
					t.Errorf("output %q should not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinterCollectionHeader(t *testing.T) {
	tests := []struct {
		name      string
		colName   string
		color     bool
		wantParts []string
		wantNot   []string
	}{
		{"no-color contains name", "My Collection", false, []string{"My Collection"}, []string{"\033["}},
		{"with-color emits bold ANSI", "My Collection", true, []string{"My Collection", "\033[1m"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color)
			p.CollectionHeader(tt.colName)
			got := buf.String()
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
			for _, part := range tt.wantNot {
				if strings.Contains(got, part) {
					t.Errorf("output %q should not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinterSectionHeader(t *testing.T) {
	tests := []struct {
		name      string
		section   string
		color     bool
		wantExact string
		wantParts []string
		wantNot   []string
	}{
		{"setup no-color", "Setup", false, "\nSetup:\n", nil, []string{"\033["}},
		{"teardown no-color", "Teardown", false, "\nTeardown:\n", nil, []string{"\033["}},
		{"setup with-color emits bold cyan ANSI", "Setup", true, "", []string{"Setup:", "\033[1;36m", "\033[0m"}, nil},
		{"teardown with-color emits bold cyan ANSI", "Teardown", true, "", []string{"Teardown:", "\033[1;36m", "\033[0m"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color)
			p.SectionHeader(tt.section)
			got := buf.String()
			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("output = %q, want %q", got, tt.wantExact)
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
			for _, part := range tt.wantNot {
				if strings.Contains(got, part) {
					t.Errorf("output %q should not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinterWarning(t *testing.T) {
	tests := []struct {
		name      string
		msg       string
		color     bool
		wantParts []string
		wantNot   []string
	}{
		{"no-color has warning prefix", "collection has no requests", false, []string{"Warning:", "collection has no requests"}, []string{"\033["}},
		{"with-color emits yellow ANSI", "something", true, []string{"\033[33m"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color)
			p.Warning(tt.msg)
			got := buf.String()
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
			for _, part := range tt.wantNot {
				if strings.Contains(got, part) {
					t.Errorf("output %q should not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinterSkipped(t *testing.T) {
	tests := []struct {
		name      string
		reqName   string
		color     bool
		wantParts []string
		wantNot   []string
	}{
		{"no-color shows name and SKIPPED", "Skipped Request", false, []string{"Skipped Request", "SKIPPED"}, []string{"\033["}},
		{"with-color emits gray ANSI", "Skipped Request", true, []string{"\033[90m"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color)
			p.Skipped(tt.reqName)
			got := buf.String()
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
			for _, part := range tt.wantNot {
				if strings.Contains(got, part) {
					t.Errorf("output %q should not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinterSummaryWithDuration(t *testing.T) {
	tests := []struct {
		name      string
		total     int
		passed    int
		failed    int
		skipped   int
		duration  time.Duration
		color     bool
		wantParts []string
		wantNot   []string
	}{
		{
			name:      "all passed no-color",
			total:     3,
			passed:    3,
			failed:    0,
			skipped:   0,
			duration:  150 * time.Millisecond,
			color:     false,
			wantParts: []string{"3 request(s)", "3 passed", "0 failed", "150ms"},
			wantNot:   []string{"skipped", "\033["},
		},
		{
			name:      "some failed no-color",
			total:     3,
			passed:    2,
			failed:    1,
			skipped:   0,
			duration:  200 * time.Millisecond,
			color:     false,
			wantParts: []string{"3 request(s)", "2 passed", "1 failed", "200ms"},
			wantNot:   []string{"\033["},
		},
		{
			name:      "with skipped no-color",
			total:     3,
			passed:    1,
			failed:    1,
			skipped:   1,
			duration:  100 * time.Millisecond,
			color:     false,
			wantParts: []string{"3 request(s)", "1 passed", "1 failed", "1 skipped", "100ms"},
		},
		{
			name:      "separator line present",
			total:     3,
			passed:    3,
			failed:    0,
			skipped:   0,
			duration:  50 * time.Millisecond,
			color:     false,
			wantParts: []string{"───"},
		},
		{
			name:      "passed green with color",
			total:     3,
			passed:    3,
			failed:    0,
			skipped:   0,
			duration:  50 * time.Millisecond,
			color:     true,
			wantParts: []string{"\033[32m", "3 passed"},
		},
		{
			name:      "failed red with color",
			total:     3,
			passed:    2,
			failed:    1,
			skipped:   0,
			duration:  50 * time.Millisecond,
			color:     true,
			wantParts: []string{"\033[31m", "1 failed"},
		},
		{
			name:      "passed green no red when no failures",
			total:     3,
			passed:    3,
			failed:    0,
			skipped:   0,
			duration:  50 * time.Millisecond,
			color:     true,
			wantParts: []string{"\033[32m"},
			wantNot:   []string{"\033[31m"},
		},
		{
			name:      "skipped yellow with color",
			total:     3,
			passed:    1,
			failed:    1,
			skipped:   1,
			duration:  50 * time.Millisecond,
			color:     true,
			wantParts: []string{"\033[33m", "1 skipped"},
		},
		{
			name:      "duration gray with color",
			total:     3,
			passed:    3,
			failed:    0,
			skipped:   0,
			duration:  50 * time.Millisecond,
			color:     true,
			wantParts: []string{"\033[90m"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color)
			p.SummaryWithDuration(tt.total, tt.passed, tt.failed, tt.skipped, tt.duration)
			got := buf.String()
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
			for _, part := range tt.wantNot {
				if strings.Contains(got, part) {
					t.Errorf("output %q should not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinterError(t *testing.T) {
	tests := []struct {
		name      string
		msg       string
		color     bool
		wantParts []string
		wantNot   []string
	}{
		{"no-color has error prefix", "something went wrong", false, []string{"Error:", "something went wrong"}, []string{"\033["}},
		{"with-color emits red ANSI", "bad", true, []string{"\033[31m"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color)
			p.Error(tt.msg)
			got := buf.String()
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
			for _, part := range tt.wantNot {
				if strings.Contains(got, part) {
					t.Errorf("output %q should not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinterStructuredError(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		color bool
		want  string
	}{
		{
			"structured with file and line no-color",
			&apierrors.Structured{FilePath: "f.yaml", Line: 3, Message: "bad"},
			false,
			"[ERROR] f.yaml:3 — bad\n",
		},
		{
			"plain error no-color",
			errors.New("boom"),
			false,
			"[ERROR] boom\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color)
			p.StructuredError(tt.err)
			got := buf.String()
			if got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrinterRequestError(t *testing.T) {
	tests := []struct {
		name    string
		reqName string
		err     error
		color   bool
		want    string
	}{
		{
			"network error with hint no-color",
			"GET /users",
			&apierrors.NetworkError{Message: "DNS failed", Hint: "check host"},
			false,
			"[ERROR] GET /users — DNS failed\n  Hint: check host\n",
		},
		{
			"plain error no-color",
			"GET /users",
			errors.New("boom"),
			false,
			"[ERROR] GET /users — boom\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color)
			p.RequestError(tt.reqName, tt.err)
			got := buf.String()
			if got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrinterVerbosityQuiet(t *testing.T) {
	tests := []struct {
		name    string
		action  func(p *Printer)
		wantOut string
	}{
		{"quiet_suppresses_collection_header", func(p *Printer) { p.CollectionHeader("suite") }, ""},
		{"quiet_suppresses_section_header", func(p *Printer) { p.SectionHeader("setup") }, ""},
		{"quiet_suppresses_result", func(p *Printer) {
			p.Result("req", &httpexec.Result{StatusCode: 200}, true, 0)
		}, ""},
		{"quiet_suppresses_assertion_detail", func(p *Printer) {
			p.AssertionDetail("status", "200", "404")
		}, ""},
		{"quiet_suppresses_skipped", func(p *Printer) { p.Skipped("req") }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, false, VerbosityQuiet)
			tt.action(p)
			if got := buf.String(); got != tt.wantOut {
				t.Errorf("got %q, want %q", got, tt.wantOut)
			}
		})
	}
}

func TestPrinterVerbosityQuiet_SummaryStillShown(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, false, VerbosityQuiet)
	p.SummaryWithDuration(1, 1, 0, 0, 50*time.Millisecond)
	if buf.Len() == 0 {
		t.Error("SummaryWithDuration should output even at quiet verbosity")
	}
}

func TestPrinterVerbosityVerbose_ShowsRequestDetail(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, false, VerbosityVerbose)
	p.RequestDetail("GET", "https://api.example.com/users", map[string]string{"Accept": "application/json"})
	out := buf.String()
	if !strings.Contains(out, "> GET https://api.example.com/users") {
		t.Errorf("expected request line, got: %q", out)
	}
	if !strings.Contains(out, "Accept: application/json") {
		t.Errorf("expected header line, got: %q", out)
	}
}

func TestPrinterVerbosityDefault_NoRequestDetail(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, false)
	p.RequestDetail("GET", "https://api.example.com", nil)
	if buf.Len() != 0 {
		t.Error("RequestDetail should be silent at default verbosity")
	}
}

func TestPrinterVerbosityDebug_ShowsResponseBody(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, false, VerbosityDebug)
	p.ResponseBodyDump([]byte(`{"id":1}`))
	if !strings.Contains(buf.String(), `{"id":1}`) {
		t.Errorf("expected response body, got: %q", buf.String())
	}
}

func TestPrinterVerbosityVerbose_NoResponseBody(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, false, VerbosityVerbose)
	p.ResponseBodyDump([]byte(`{"id":1}`))
	if buf.Len() != 0 {
		t.Error("ResponseBodyDump should be silent at verbose (not debug) verbosity")
	}
}

func TestPrinterVerbosityDebug_ResponseBodyDump_Truncation(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, false, VerbosityDebug)
	// Build a body larger than the 10 KB truncation limit.
	body := bytes.Repeat([]byte("x"), 10*1024+1)
	p.ResponseBodyDump(body)
	out := buf.String()
	if !strings.Contains(out, "(body, truncated)") {
		t.Errorf("expected truncation marker, got: %q", out[:min(len(out), 100)])
	}
	if !strings.Contains(out, fmt.Sprintf("[%d bytes total]", len(body))) {
		t.Errorf("expected byte-count line, got: %q", out[:min(len(out), 100)])
	}
}

func TestPrinterVerbosityVerbose_ShowsResponseDetail(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, false, VerbosityVerbose)
	headers := http.Header{
		"Content-Type": []string{"application/json"},
		"X-Request-Id": []string{"abc123"},
	}
	p.ResponseDetail(200, headers)
	out := buf.String()
	if !strings.Contains(out, "< 200") {
		t.Errorf("expected status line, got: %q", out)
	}
	if !strings.Contains(out, "Content-Type: application/json") {
		t.Errorf("expected Content-Type header, got: %q", out)
	}
	if !strings.Contains(out, "X-Request-Id: abc123") {
		t.Errorf("expected X-Request-Id header, got: %q", out)
	}
}

func TestPrinterVerbosityDefault_NoResponseDetail(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, false)
	p.ResponseDetail(200, http.Header{"Content-Type": []string{"text/html"}})
	if buf.Len() != 0 {
		t.Error("ResponseDetail should be silent at default verbosity")
	}
}

func TestPrinterVerbosityDebug_ShowsRequestBodyDump(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, false, VerbosityDebug)
	p.RequestBodyDump(map[string]string{"key": "value"})
	out := buf.String()
	if !strings.Contains(out, "> (body):") {
		t.Errorf("expected body dump prefix, got: %q", out)
	}
}

func TestPrinterVerbosityVerbose_NoRequestBodyDump(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, false, VerbosityVerbose)
	p.RequestBodyDump(map[string]string{"key": "value"})
	if buf.Len() != 0 {
		t.Error("RequestBodyDump should be silent at verbose (not debug) verbosity")
	}
}

func TestPrinterVerbosityDebug_RequestBodyDump_NilBody(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, false, VerbosityDebug)
	p.RequestBodyDump(nil)
	if buf.Len() != 0 {
		t.Error("RequestBodyDump should be silent when body is nil")
	}
}

func TestPrinter_SkippedWithReason(t *testing.T) {
	tests := []struct {
		name      string
		reqName   string
		reason    string
		wantParts []string
		wantNot   []string
	}{
		{
			"with reason shows reason in parens on single line",
			"Request B", `depends on 'user_id' from "A", which failed`,
			[]string{"Request B", "SKIPPED", "user_id"},
			[]string{"Reason:"},
		},
		{
			"empty reason omits reason",
			"Request B", "",
			[]string{"Request B", "SKIPPED"},
			[]string{"Reason:", "("},
		},
		{
			"quiet verbosity suppresses output",
			"Request B", "dependency failed",
			nil,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			var p *Printer
			if tt.name == "quiet verbosity suppresses output" {
				p = NewPrinter(&buf, false, VerbosityQuiet)
			} else {
				p = NewPrinter(&buf, false)
			}
			p.SkippedWithReason(tt.reqName, tt.reason)
			got := buf.String()
			if tt.name == "quiet verbosity suppresses output" {
				if got != "" {
					t.Errorf("expected empty output at quiet verbosity, got %q", got)
				}
				return
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
			for _, part := range tt.wantNot {
				if strings.Contains(got, part) {
					t.Errorf("output %q should not contain %q", got, part)
				}
			}
		})
	}
}

// TestPrinter_SkippedWithReason_RendersSingleLine verifies the exact format of
// the SKIPPED line when a reason is provided (M19-001 observable format).
func TestPrinter_SkippedWithReason_RendersSingleLine(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, false)
	p.SkippedWithReason("confirm-pending-order", "if: false")
	want := "  SKIPPED  confirm-pending-order  (if: false)\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrinter_GuardRail(t *testing.T) {
	tests := []struct {
		name      string
		executed  int
		limit     int
		color     bool
		wantParts []string
		wantNot   []string
	}{
		{
			name:      "plain text shows limit exceeded",
			executed:  1000,
			limit:     1000,
			color:     false,
			wantParts: []string{"Request limit exceeded"},
			wantNot:   []string{"\033["},
		},
		{
			name:      "includes hint",
			executed:  1000,
			limit:     1000,
			color:     false,
			wantParts: []string{"Split this collection"},
		},
		{
			name:      "shows count and limit",
			executed:  500,
			limit:     500,
			color:     false,
			wantParts: []string{"Executed 500 requests (limit: 500)"},
		},
		{
			name:      "color mode has ANSI codes",
			executed:  1000,
			limit:     1000,
			color:     true,
			wantParts: []string{"\033[31m", "Request limit exceeded"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color)
			p.GuardRail(tt.executed, tt.limit)
			got := buf.String()
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
			for _, part := range tt.wantNot {
				if strings.Contains(got, part) {
					t.Errorf("output %q should not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinter_ImpactSummary(t *testing.T) {
	tests := []struct {
		name      string
		entries   []ImpactLine
		verbosity Verbosity
		wantParts []string
		wantEmpty bool
	}{
		{
			"single failure with 2 skips",
			[]ImpactLine{{FailedName: "Request A", SkippedCount: 2}},
			VerbosityDefault,
			[]string{"Impact Analysis:", "2 request(s) skipped", "Request A"},
			false,
		},
		{
			"empty entries - no output",
			nil,
			VerbosityDefault,
			nil,
			true,
		},
		{
			"quiet verbosity suppresses output",
			[]ImpactLine{{FailedName: "Request A", SkippedCount: 1}},
			VerbosityQuiet,
			nil,
			true,
		},
		{
			"multiple failures",
			[]ImpactLine{
				{FailedName: "Request A", SkippedCount: 3},
				{FailedName: "Request B", SkippedCount: 1},
			},
			VerbosityDefault,
			[]string{"Impact Analysis:", "3 request(s) skipped", "Request A", "1 request(s) skipped", "Request B"},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, false, tt.verbosity)
			p.ImpactSummary(tt.entries)
			got := buf.String()
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty output, got %q", got)
				}
				return
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinter_WaveHeader(t *testing.T) {
	tests := []struct {
		name            string
		waveNum         int
		concurrentCount int
		verbosity       Verbosity
		color           bool
		wantParts       []string
		wantEmpty       bool
	}{
		{"shows wave number and concurrent count", 1, 3, VerbosityDefault, false, []string{"Wave 1", "3 concurrent"}, false},
		{"wave 2 with 1 request", 2, 1, VerbosityDefault, false, []string{"Wave 2", "1 concurrent"}, false},
		{"quiet verbosity suppresses output", 1, 2, VerbosityQuiet, false, nil, true},
		{"color mode emits bold cyan ANSI", 1, 2, VerbosityDefault, true, []string{"\033[1;36m"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color, tt.verbosity)
			p.WaveHeader(tt.waveNum, tt.concurrentCount)
			got := buf.String()
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty output, got %q", got)
				}
				return
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinter_ParallelSummary(t *testing.T) {
	tests := []struct {
		name       string
		waveCount  int
		maxPar     int
		duration   time.Duration
		waveDurs   []time.Duration
		verbosity  Verbosity
		wantParts  []string
		wantAbsent []string
		wantEmpty  bool
	}{
		{
			name:      "shows wave count and max parallelism",
			waveCount: 3, maxPar: 2,
			duration:  100 * time.Millisecond,
			waveDurs:  []time.Duration{50 * time.Millisecond, 30 * time.Millisecond, 20 * time.Millisecond},
			verbosity: VerbosityDefault,
			wantParts: []string{"Waves: 3", "Max parallelism: 2"},
		},
		{
			name:      "shows speedup factor for multi-wave",
			waveCount: 2, maxPar: 2,
			duration:  50 * time.Millisecond,
			waveDurs:  []time.Duration{50 * time.Millisecond, 50 * time.Millisecond},
			verbosity: VerbosityDefault,
			wantParts: []string{"Speedup:", "2.0x"},
		},
		{
			name:      "no speedup for single wave",
			waveCount: 1, maxPar: 3,
			duration:   50 * time.Millisecond,
			waveDurs:   []time.Duration{50 * time.Millisecond},
			verbosity:  VerbosityDefault,
			wantParts:  []string{"Waves: 1"},
			wantAbsent: []string{"Speedup:"},
		},
		{
			name:      "quiet verbosity suppresses output",
			waveCount: 2, maxPar: 2,
			duration:  50 * time.Millisecond,
			verbosity: VerbosityQuiet,
			wantEmpty: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, false, tt.verbosity)
			p.ParallelSummary(tt.waveCount, tt.maxPar, tt.duration, tt.waveDurs)
			got := buf.String()
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty output, got %q", got)
				}
				return
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("output %q should not contain %q", got, absent)
				}
			}
		})
	}
}

func TestPrinter_DataDrivenHeader(t *testing.T) {
	tests := []struct {
		name      string
		ddName    string
		total     int
		verbosity Verbosity
		color     bool
		wantParts []string
		wantEmpty bool
	}{
		{"shows name and total", "Create Users", 100, VerbosityDefault, false, []string{"Data-Driven:", "Create Users", "100 iterations"}, false},
		{"quiet suppresses output", "Create Users", 100, VerbosityQuiet, false, nil, true},
		{"color emits bold cyan ANSI", "Create Users", 100, VerbosityDefault, true, []string{"\033[1;36m"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color, tt.verbosity)
			p.DataDrivenHeader(tt.ddName, tt.total)
			got := buf.String()
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty output, got %q", got)
				}
				return
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinter_DataDrivenCompactSummary(t *testing.T) {
	tests := []struct {
		name          string
		passed        int
		failed        int
		total         int
		avgDurationMs int64
		failedIndices []int
		color         bool
		verbosity     Verbosity
		wantParts     []string
		wantEmpty     bool
	}{
		{"all passed compact", 100, 0, 100, 135, nil, false, VerbosityDefault, []string{"100 passed", "0 failed", "100 total", "avg 135ms"}, false},
		{"some failed compact with indices", 97, 3, 100, 135, []int{15, 48, 72}, false, VerbosityDefault, []string{"97 passed", "3 failed", "Failed iterations:", "15", "48", "72"}, false},
		{"color green passed red failed", 90, 10, 100, 50, []int{1}, true, VerbosityDefault, []string{"\033[32m", "\033[31m"}, false},
		{"quiet suppresses output", 100, 0, 100, 135, nil, false, VerbosityQuiet, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color, tt.verbosity)
			p.DataDrivenCompactSummary(tt.passed, tt.failed, tt.total, tt.avgDurationMs, tt.failedIndices)
			got := buf.String()
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty output, got %q", got)
				}
				return
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinter_DataDrivenVerboseResult(t *testing.T) {
	tests := []struct {
		name         string
		iterationNum int
		label        string
		durationMs   int64
		passed       bool
		color        bool
		verbosity    Verbosity
		wantParts    []string
		wantEmpty    bool
	}{
		{"passing iteration", 1, "valid@example.com", 145, true, false, VerbosityDefault, []string{"✓", "Iteration 1", "valid@example.com", "145ms"}, false},
		{"failing iteration", 3, "invalid-email", 98, false, false, VerbosityDefault, []string{"✗", "Iteration 3", "invalid-email", "98ms"}, false},
		{"passing with color", 1, "test", 50, true, true, VerbosityDefault, []string{"\033[32m"}, false},
		{"failing with color", 2, "test", 50, false, true, VerbosityDefault, []string{"\033[31m"}, false},
		{"quiet suppresses output", 1, "test", 50, true, false, VerbosityQuiet, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color, tt.verbosity)
			p.DataDrivenVerboseResult(tt.iterationNum, tt.label, tt.durationMs, tt.passed)
			got := buf.String()
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty output, got %q", got)
				}
				return
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinter_DataDrivenSummary(t *testing.T) {
	tests := []struct {
		name          string
		passed        int
		failed        int
		total         int
		avgDurationMs int64
		failedIndices []int
		verbosity     Verbosity
		color         bool
		wantParts     []string
		wantEmpty     bool
	}{
		{"shows summary line", 4, 1, 5, 128, nil, VerbosityDefault, false, []string{"4 passed", "1 failed", "5 total", "Average duration:", "128ms"}, false},
		{"quiet suppresses", 4, 1, 5, 128, nil, VerbosityQuiet, false, nil, true},
		{"shows failed iteration numbers", 2, 2, 4, 100, []int{2, 4}, VerbosityDefault, false, []string{"Failed iterations:", "2", "4"}, false},
		{"no failed iterations line when all pass", 5, 0, 5, 100, nil, VerbosityDefault, false, []string{"5 passed", "0 failed"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, tt.color, tt.verbosity)
			p.DataDrivenSummary(tt.passed, tt.failed, tt.total, tt.avgDurationMs, tt.failedIndices)
			got := buf.String()
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty output, got %q", got)
				}
				return
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
		})
	}
}

func TestPrinterRetryAttemptDetails(t *testing.T) {
	twoAttempts := []retry.AttemptDetail{
		{Number: 1, StatusCode: 503, Duration: 20 * time.Millisecond, Delay: 0},
		{Number: 2, StatusCode: 200, Duration: 30 * time.Millisecond, Delay: 100 * time.Millisecond},
	}
	oneAttempt := []retry.AttemptDetail{
		{Number: 1, StatusCode: 200, Duration: 50 * time.Millisecond, Delay: 0},
	}
	errorAttempts := []retry.AttemptDetail{
		{Number: 1, StatusCode: 0, Duration: 10 * time.Millisecond, Delay: 0, Err: fmt.Errorf("connection refused")},
		{Number: 2, StatusCode: 200, Duration: 30 * time.Millisecond, Delay: 100 * time.Millisecond},
	}

	tests := []struct {
		name      string
		verbosity Verbosity
		details   []retry.AttemptDetail
		wantParts []string
		wantNot   []string
	}{
		{
			"verbose shows attempt details",
			VerbosityVerbose,
			twoAttempts,
			[]string{"Attempt 1:", "503", "Attempt 2:", "200"},
			nil,
		},
		{
			"default verbosity hides attempt details",
			VerbosityDefault,
			twoAttempts,
			nil,
			[]string{"Attempt"},
		},
		{
			"single attempt not shown",
			VerbosityVerbose,
			oneAttempt,
			nil,
			[]string{"Attempt"},
		},
		{
			"error attempt shows error",
			VerbosityVerbose,
			errorAttempts,
			[]string{"Attempt 1:", "error"},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(&buf, false, tt.verbosity)
			p.RetryAttemptDetails(tt.details)
			got := buf.String()
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
			for _, notPart := range tt.wantNot {
				if strings.Contains(got, notPart) {
					t.Errorf("output %q should not contain %q", got, notPart)
				}
			}
		})
	}
}
