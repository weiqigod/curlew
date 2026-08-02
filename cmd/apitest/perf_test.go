package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParsePerfArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		want     perfFlags
		wantErr  bool
		wantHelp bool
	}{
		{
			name: "basic vus+duration",
			args: []string{"req.yaml", "--vus", "5", "--duration", "30s"},
			want: perfFlags{file: "req.yaml", vus: 5, duration: 30 * time.Second},
		},
		{
			name: "ramp-up parsed",
			args: []string{"req.yaml", "--vus", "10", "--duration", "1m", "--ramp-up", "10s"},
			want: perfFlags{file: "req.yaml", vus: 10, duration: time.Minute, rampUp: 10 * time.Second},
		},
		{
			name: "rps parsed",
			args: []string{"req.yaml", "--vus", "5", "--duration", "5s", "--rps", "100"},
			want: perfFlags{file: "req.yaml", vus: 5, duration: 5 * time.Second, rps: 100},
		},
		{
			name:     "--help",
			args:     []string{"--help"},
			wantHelp: true,
		},
		{
			name:    "zero vus errors",
			args:    []string{"req.yaml", "--vus", "0", "--duration", "5s"},
			wantErr: true,
		},
		{
			name:    "zero duration errors",
			args:    []string{"req.yaml", "--vus", "5", "--duration", "0s"},
			wantErr: true,
		},
		{
			name:    "unknown flag errors",
			args:    []string{"--bogus"},
			wantErr: true,
		},
		{
			name:    "missing positional errors",
			args:    []string{"--vus", "5", "--duration", "5s"},
			wantErr: true,
		},
		{
			name:    "non-integer --vus errors",
			args:    []string{"req.yaml", "--vus", "abc", "--duration", "5s"},
			wantErr: true,
		},
		{
			name:    "invalid duration errors",
			args:    []string{"req.yaml", "--vus", "5", "--duration", "not-a-dur"},
			wantErr: true,
		},
		{
			name: "output stdout accepted",
			args: []string{"req.yaml", "--vus", "2", "--duration", "1s", "--output", "stdout"},
			want: perfFlags{file: "req.yaml", vus: 2, duration: time.Second, output: "stdout"},
		},
		{
			name:     "-h short help",
			args:     []string{"-h"},
			wantHelp: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, showHelp, err := parsePerfArgs(tc.args)
			switch {
			case tc.wantErr:
				if err == nil {
					t.Errorf("parsePerfArgs(%v) error = nil, want error", tc.args)
				}
			case tc.wantHelp:
				if !showHelp {
					t.Errorf("parsePerfArgs(%v) showHelp = false, want true", tc.args)
				}
			default:
				if err != nil {
					t.Fatalf("parsePerfArgs(%v) error = %v, want nil", tc.args, err)
				}
				if showHelp {
					t.Errorf("parsePerfArgs(%v) showHelp = true, want false", tc.args)
				}
				if got.file != tc.want.file {
					t.Errorf("file = %q, want %q", got.file, tc.want.file)
				}
				if got.vus != tc.want.vus {
					t.Errorf("vus = %d, want %d", got.vus, tc.want.vus)
				}
				if got.duration != tc.want.duration {
					t.Errorf("duration = %v, want %v", got.duration, tc.want.duration)
				}
				if got.rampUp != tc.want.rampUp {
					t.Errorf("rampUp = %v, want %v", got.rampUp, tc.want.rampUp)
				}
				if got.rps != tc.want.rps {
					t.Errorf("rps = %d, want %d", got.rps, tc.want.rps)
				}
				if got.output != tc.want.output {
					t.Errorf("output = %q, want %q", got.output, tc.want.output)
				}
			}
		})
	}
}

func TestPrintPerfHelp_MentionsAllFlags(t *testing.T) {
	var buf bytes.Buffer
	printPerfHelpTo(&buf)
	out := buf.String()

	for _, flag := range []string{"--vus", "--duration", "--ramp-up", "--rps", "--output"} {
		if !strings.Contains(out, flag) {
			t.Errorf("printPerfHelp() output missing %q", flag)
		}
	}
	if !strings.Contains(out, "Usage: apitest perf") {
		t.Error("printPerfHelp() output missing usage line")
	}
	if !strings.Contains(out, "Examples:") {
		t.Error("printPerfHelp() output missing Examples block")
	}
	if !strings.Contains(out, "apitest perf") || !strings.Contains(out, "--output report.json") {
		t.Error("printPerfHelp() output missing example invocation with --output")
	}
}

func TestPerfCmd_InvalidVUsExitCode2(t *testing.T) {
	f := writePerfRequestFile(t, "http://stub")
	code := perfCmd([]string{f, "--vus", "0", "--duration", "1s"})
	if code != 2 {
		t.Errorf("perfCmd exit code = %d, want 2", code)
	}
}

func TestPerfCmd_InvalidDurationExitCode2(t *testing.T) {
	f := writePerfRequestFile(t, "http://stub")
	code := perfCmd([]string{f, "--vus", "1", "--duration", "0s"})
	if code != 2 {
		t.Errorf("perfCmd exit code = %d, want 2", code)
	}
}

func TestPerfCmd_UnsupportedOutputExitCode2(t *testing.T) {
	f := writePerfRequestFile(t, "http://stub")
	code := perfCmd([]string{f, "--vus", "1", "--duration", "1s", "--output", "html"})
	if code != 2 {
		t.Errorf("perfCmd with --output html exit code = %d, want 2", code)
	}
}

func TestPerfCmd_NonExistentFileExitCode3(t *testing.T) {
	code := perfCmd([]string{"/tmp/definitely_missing_perf_file.yaml", "--vus", "1", "--duration", "1s"})
	if code != 3 {
		t.Errorf("perfCmd with missing file exit code = %d, want 3", code)
	}
}

func TestPerfCmd_Run_HTTPTestServer_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := writePerfRequestFile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	code := perfCmdOut([]string{f, "--vus", "2", "--duration", "200ms"}, &stdout, &stderr)
	out := stdout.String()
	errOut := stderr.String()

	if code != 0 {
		t.Errorf("perfCmd exit code = %d, want 0; stderr: %s", code, errOut)
	}
	if !strings.Contains(errOut, "Load test:") {
		t.Errorf("stderr missing 'Load test:'; got: %s", errOut)
	}
	if !strings.Contains(errOut, "Requests sent:") {
		t.Errorf("stderr missing 'Requests sent:'; got: %s", errOut)
	}
	if !strings.Contains(out, "Results: requests=") {
		t.Errorf("stdout missing summary line; got: %s", out)
	}
	if strings.Contains(out, "Load test:") || strings.Contains(out, "Requests sent:") {
		t.Errorf("progress leaked to stdout; got: %s", out)
	}
}

func TestPerfCmd_Run_HTTPTestServer_AllFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	f := writePerfRequestFile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	code := perfCmdOut([]string{f, "--vus", "2", "--duration", "100ms"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("perfCmd with all-500 server exit code = %d, want 1", code)
	}
}

func TestPerfCmd_ContextCancelExitCode130(t *testing.T) {
	// Start a slow server (simulates in-flight requests) and send SIGINT to the
	// current process shortly after perfCmd begins. signal.NotifyContext cancels
	// the run context, Run returns with Aborted=true, and perfCmd returns 130.
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}: // signal that at least one request arrived
		default:
		}
		// Block until client disconnects (context cancel).
		<-r.Context().Done()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := writePerfRequestFile(t, srv.URL)

	// Send SIGINT once the server has received at least one request.
	// Buffer size 2: one slot for the timeout sentinel (-1) and one for the
	// main-goroutine return code, so neither sender ever blocks.
	done := make(chan int, 2)
	go func() {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Errorf("test server never received a request; SIGINT not sent")
			done <- -1
			return
		}
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}()

	var stdout, stderr bytes.Buffer
	code := perfCmdOut([]string{f, "--vus", "2", "--duration", "30s"}, &stdout, &stderr)
	done <- code

	got := <-done
	if got != 130 {
		t.Errorf("perfCmd exit code = %d, want 130 (SIGINT)", got)
	}
}

func TestPerfCmd_RPSHeaderInStderr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := writePerfRequestFile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	code := perfCmdOut([]string{f, "--vus", "2", "--duration", "200ms", "--rps", "5"}, &stdout, &stderr)
	errOut := stderr.String()

	if code != 0 {
		t.Errorf("perfCmd exit code = %d, want 0; stderr: %s", code, errOut)
	}
	if !strings.Contains(errOut, "Target rate:") {
		t.Errorf("stderr missing 'Target rate:' when --rps > 0; got: %s", errOut)
	}
	if strings.Contains(stdout.String(), "Target rate:") {
		t.Errorf("'Target rate:' leaked to stdout; got: %s", stdout.String())
	}
}

func TestPerfCmd_OutputJSON_WritesFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "perf.json")
	f := writePerfRequestFile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	code := perfCmdOut([]string{f, "--vus", "2", "--duration", "200ms", "--output", out}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("perfCmd exit code = %d, want 0", code)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v", out, err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal() = %v, output: %s", err, data)
	}
	metrics, ok := result["metrics"].(map[string]interface{})
	if !ok {
		t.Fatal("metrics field missing from JSON")
	}
	if int(metrics["requests"].(float64)) == 0 {
		t.Error("metrics.requests = 0, want > 0")
	}
}

func TestPerfCmd_OutputHTML_WritesFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "perf.html")
	f := writePerfRequestFile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	code := perfCmdOut([]string{f, "--vus", "2", "--duration", "200ms", "--output", out}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("perfCmd exit code = %d, want 0", code)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v", out, err)
	}
	html := string(data)
	if !strings.Contains(html, "<title>apitest perf report</title>") {
		t.Error("HTML output missing <title>apitest perf report</title>")
	}
	if !strings.Contains(html, "chart.js") {
		t.Error("HTML output missing chart.js CDN reference")
	}
}

func TestPerfCmd_UnsupportedExtensionExitCode2(t *testing.T) {
	f := writePerfRequestFile(t, "http://stub")

	var stdout, stderr bytes.Buffer
	code := perfCmdOut([]string{f, "--vus", "1", "--duration", "1s", "--output", "report.xyz"}, &stdout, &stderr)

	if code != 2 {
		t.Errorf("perfCmd exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unsupported report format") {
		t.Errorf("stderr = %q, want to contain 'unsupported report format'", stderr.String())
	}
}

func TestPerfCmd_OutputJSON_OverwritesExistingFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "perf.json")
	// Write dummy content that won't survive an overwrite.
	if err := os.WriteFile(out, []byte("THIS_IS_DUMMY_CONTENT"), 0o600); err != nil {
		t.Fatalf("write dummy: %v", err)
	}

	f := writePerfRequestFile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	code := perfCmdOut([]string{f, "--vus", "2", "--duration", "200ms", "--output", out}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("perfCmd exit code = %d, want 0", code)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v", out, err)
	}
	// Dummy content must be gone; file must be valid JSON.
	if strings.Contains(string(data), "THIS_IS_DUMMY_CONTENT") {
		t.Error("file still contains dummy content; overwrite failed")
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal() = %v", err)
	}
}

func TestPerfCmd_SummaryLinePrintedToStdout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := writePerfRequestFile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	code := perfCmdOut([]string{f, "--vus", "2", "--duration", "200ms"}, &stdout, &stderr)
	out := stdout.String()

	if code != 0 {
		t.Errorf("perfCmd exit code = %d, want 0; stdout: %s", code, out)
	}
	if !strings.Contains(out, "Results: requests=") {
		t.Errorf("stdout missing 'Results: requests='; got: %s", out)
	}
}

// writePerfRequestFile writes a minimal YAML request file to a temp dir
// and returns its path.
// TestPerfCmd_EventsFlagRejected verifies that --events is not accepted by the
// perf subcommand; it must fail with exit 2 (parse error) and the canonical
// rejection message.
func TestPerfCmd_EventsFlagRejected(t *testing.T) {
	_, _, err := parsePerfArgs([]string{"req.yaml", "--vus", "1", "--duration", "1s", "--events", "/tmp/x.jsonl"})
	if err == nil {
		t.Fatal("parsePerfArgs --events: want error, got nil")
	}
	const wantMsg = "--events is supported only on run; use --format jsonl for streaming samples"
	if !strings.Contains(err.Error(), wantMsg) {
		t.Errorf("error = %q; want containing %q", err.Error(), wantMsg)
	}
}

func writePerfRequestFile(t *testing.T, url string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "req.yaml")
	content := fmt.Sprintf("name: test\nrequest:\n  method: GET\n  url: %s\n", url)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write perf request file: %v", err)
	}
	return path
}
