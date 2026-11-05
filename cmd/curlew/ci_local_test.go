//go:build !windows

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestCiLocalDownIdempotent asserts that ./scripts/ci-local.sh --down succeeds
// even when no stack is running, and again on a second invocation.
func TestCiLocalDownIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration script test; skipped under -short")
	}
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	script := filepath.Join(repoRoot, "scripts", "ci-local.sh")

	for i := 0; i < 2; i++ {
		cmd := exec.Command("bash", script, "--down")
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("iteration %d: --down failed: %v\noutput: %s", i, err, out)
		}
	}
}

// The gate script's cleanup contract, exercised by running the real lines.
//
// The tests below do not re-implement scripts/ci-local.sh's cleanup logic —
// they cut the actual region out of the script and execute it against a stub
// scripts/test-stack.sh. That keeps the assertion honest (a regression in the
// script itself fails the test) without paying for a docker stack. Extraction
// failures are fatal rather than silently vacuous: if the anchors below stop
// matching, the script has been restructured and these tests must be revisited
// rather than quietly passing over nothing.

// gateScriptRegion returns the lines of the file at path from the first line
// whose trimmed content equals start through the first following line whose
// trimmed content equals end, inclusive.
func gateScriptRegion(t *testing.T, path, start, end string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(src), "\n")
	from := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == start {
			from = i
			break
		}
	}
	if from < 0 {
		t.Fatalf("%s: no line matching %q — the script has been restructured", path, start)
	}
	for i := from + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == end {
			return strings.Join(lines[from:i+1], "\n")
		}
	}
	t.Fatalf("%s: found %q at line %d but no following %q — the script has been restructured",
		path, start, from+1, end)
	return ""
}

// gateScriptLine returns the single line of the file at path whose trimmed
// content equals marker.
func gateScriptLine(t *testing.T, path, marker string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, line := range strings.Split(string(src), "\n") {
		if strings.TrimSpace(line) == marker {
			return line
		}
	}
	t.Fatalf("%s: no line matching %q — the script has been restructured", path, marker)
	return ""
}

// stackStubSandbox builds a directory holding a stub scripts/test-stack.sh that
// logs every subcommand it is handed to $STACK_LOG and fails `up` when upFails.
// It returns the sandbox root and the log path.
func stackStubSandbox(t *testing.T, upFails bool) (root, log string) {
	t.Helper()
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	upExit := "0"
	if upFails {
		upExit = "1"
	}
	stub := `#!/usr/bin/env bash
echo "$1" >> "$STACK_LOG"
case "$1" in
  up)
    if [ ` + upExit + ` -ne 0 ]; then
      echo "ERROR: backend did not become healthy within 60s" >&2
      exit 1
    fi
    ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(root, "scripts", "test-stack.sh"), []byte(stub), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return root, filepath.Join(root, "stack.log")
}

// runStackLifecycle executes the real cleanup and stack-start regions of
// scripts/ci-local.sh with run_e2e set to the given value, inside a sandbox
// whose scripts/test-stack.sh is a stub. tail is appended after the stack-start
// region to stand in for a later gate. It returns the script's combined output,
// the stub's invocation log, and the script's error (nil on exit 0).
func runStackLifecycle(t *testing.T, runE2E int, upFails bool, tail string) (string, []string, error) {
	t.Helper()
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	gate := filepath.Join(repoRoot, "scripts", "ci-local.sh")

	cleanupRegion := gateScriptRegion(t, gate, `mudflat_pid=""`, "trap cleanup EXIT")
	stackRegion := gateScriptRegion(t, gate, "if (( run_e2e )); then", "fi")

	root, log := stackStubSandbox(t, upFails)
	harness := fmt.Sprintf(`set -euo pipefail
step() { echo; echo "=== $* ==="; }
%s

run_e2e=%d
%s
%s
echo "reached-end"
`, cleanupRegion, runE2E, stackRegion, tail)

	path := filepath.Join(root, "harness.sh")
	if err := os.WriteFile(path, []byte(harness), 0o644); err != nil {
		t.Fatalf("write harness: %v", err)
	}

	cmd := exec.Command("bash", path)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "STACK_LOG="+log)
	out, runErr := cmd.CombinedOutput()

	var calls []string
	if data, err := os.ReadFile(log); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line != "" {
				calls = append(calls, line)
			}
		}
	}
	return string(out), calls, runErr
}

// TestCiLocal_failed_stack_start_is_torn_down is the regression this file
// exists for. `test-stack.sh up` starts containers and *then* waits for them to
// report healthy, so a timed-out start leaves a running stack behind. Arming
// the cleanup flag only after `up` returns meant that exact failure — the one
// that leaves containers up — was the one the trap ignored, and the next
// ci-local.sh run inherited them.
func TestCiLocal_failed_stack_start_is_torn_down(t *testing.T) {
	out, calls, err := runStackLifecycle(t, 1, true, "")

	if err == nil {
		t.Fatalf("expected a failed `test-stack.sh up` to fail the gate; it exited 0\noutput: %s", out)
	}
	if len(calls) == 0 || calls[0] != "up" {
		t.Fatalf("expected the stack start to be attempted; stub calls = %v\noutput: %s", calls, out)
	}
	found := false
	for _, c := range calls[1:] {
		if c == "down" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("a failed `test-stack.sh up` was never torn down: stub calls = %v\n"+
			"cleanup must be armed before `up` is invoked, since a start that times out "+
			"still leaves containers running\noutput: %s", calls, out)
	}
}

// TestCiLocal_started_stack_is_torn_down_when_a_later_gate_fails guards the
// behaviour the original flag placement did get right, so a fix for the failed-
// start case cannot regress it.
func TestCiLocal_started_stack_is_torn_down_when_a_later_gate_fails(t *testing.T) {
	out, calls, err := runStackLifecycle(t, 1, false, "false  # stand-in for a failing backend/web/e2e gate")

	if err == nil {
		t.Fatalf("expected the failing tail gate to fail the run; it exited 0\noutput: %s", out)
	}
	want := []string{"up", "down"}
	if len(calls) != len(want) || calls[0] != want[0] || calls[1] != want[1] {
		t.Errorf("stub calls = %v, want %v\noutput: %s", calls, want, out)
	}
}

// TestCiLocal_untouched_stack_is_not_torn_down keeps the fix from overreaching:
// `--go` never runs the stack step, and must not shell out to docker on the way
// out. Arming the flag unconditionally at the top of the script would pass the
// test above and fail this one.
func TestCiLocal_untouched_stack_is_not_torn_down(t *testing.T) {
	out, calls, err := runStackLifecycle(t, 0, false, "")
	if err != nil {
		t.Fatalf("run with run_e2e=0 failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "reached-end") {
		t.Fatalf("harness did not run to completion\noutput: %s", out)
	}
	if len(calls) != 0 {
		t.Errorf("a run that never started the stack invoked test-stack.sh %v; "+
			"cleanup must stay scoped to a stack this run actually touched\noutput: %s", calls, out)
	}
}

// TestTestStack_health_wait covers the second half of the report, which did not
// survive measurement. The window was suspected of being too short; it is not.
// On 2026-08-19 the backend container reached "Application started" 1.2s after
// `docker compose up -d` returned, while the host-side probe of
// http://localhost:5000/... was answered 403 by macOS AirPlay Receiver
// (Server: AirTunes/950.7.1) for the full window. Raising the default to 180s
// changed a 65s false failure into a 197s one and fixed nothing. So the window
// stays at 60s and becomes an env var, and the script now says which of the two
// failures actually happened.
func TestTestStack_health_wait(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	script := filepath.Join(repoRoot, "scripts", "test-stack.sh")

	defaultLine := gateScriptLine(t, script, `STACK_HEALTH_TIMEOUT="${STACK_HEALTH_TIMEOUT:-60}"`)
	waitRegion := gateScriptRegion(t, script, "wait_for_url() {", "}")
	diagnoseRegion := gateScriptRegion(t, script, "diagnose_url() {", "}")
	preamble := defaultLine + "\n" + diagnoseRegion + "\n" + waitRegion + "\n"

	t.Run("an unset timeout resolves to the measured default", func(t *testing.T) {
		out, err := exec.Command("bash", "-c",
			"set -euo pipefail\n"+defaultLine+"\necho \"$STACK_HEALTH_TIMEOUT\"").Output()
		if err != nil {
			t.Fatalf("resolve default: %v", err)
		}
		got, convErr := strconv.Atoi(strings.TrimSpace(string(out)))
		if convErr != nil {
			t.Fatalf("default timeout %q is not a number: %v", strings.TrimSpace(string(out)), convErr)
		}
		if got != 60 {
			t.Errorf("default health window is %ds, want 60s — the backend reaches "+
				"\"Application started\" in ~1.2s, so a longer default only delays "+
				"the report of a failure it cannot fix", got)
		}
	})

	t.Run("an explicit timeout bounds the wait and is named in the failure", func(t *testing.T) {
		cmd := exec.Command("bash", "-c", preamble+
			`wait_for_url "http://127.0.0.1:1/" "unreachable" && echo UNEXPECTED-OK`)
		cmd.Env = append(os.Environ(), "STACK_HEALTH_TIMEOUT=1")
		start := time.Now()
		out, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		if err == nil {
			t.Fatalf("wait_for_url returned success for an unreachable URL\noutput: %s", out)
		}
		if elapsed > 20*time.Second {
			t.Errorf("wait took %v with STACK_HEALTH_TIMEOUT=1; the env var is not "+
				"bounding the loop", elapsed)
		}
		if !strings.Contains(string(out), "within 1s") {
			t.Errorf("failure message does not report the window actually used "+
				"(want \"within 1s\")\noutput: %s", out)
		}
	})

	t.Run("a dead port is diagnosed as a service that never started", func(t *testing.T) {
		out, err := exec.Command("bash", "-c", preamble+`diagnose_url "http://127.0.0.1:1/"`).CombinedOutput()
		if err != nil {
			t.Fatalf("diagnose_url: %v\noutput: %s", err, out)
		}
		if !strings.Contains(string(out), "Nothing is listening") {
			t.Errorf("a closed port should be diagnosed as nothing listening\noutput: %s", out)
		}
		if !strings.Contains(string(out), "STACK_HEALTH_TIMEOUT") {
			t.Errorf("the one case where raising the window IS the fix should say so\noutput: %s", out)
		}
	})

	// The regression that cost the investigation: a port squatter answering
	// every probe looks exactly like a slow start in the old message. This
	// stands a real HTTP server in for macOS AirPlay Receiver.
	t.Run("a port squatter is diagnosed as a squatter, not a slow start", func(t *testing.T) {
		squatter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Server", "AirTunes/950.7.1")
			w.WriteHeader(http.StatusForbidden)
		}))
		defer squatter.Close()

		out, err := exec.Command("bash", "-c", preamble+`diagnose_url "`+squatter.URL+`/swagger"`).CombinedOutput()
		if err != nil {
			t.Fatalf("diagnose_url: %v\noutput: %s", err, out)
		}
		got := string(out)
		for _, want := range []string{"Something IS answering", "403", "AirTunes/950.7.1", "owns that port"} {
			if !strings.Contains(got, want) {
				t.Errorf("diagnosis is missing %q — this is the message that would have "+
					"identified the real cause on the first run\noutput: %s", want, got)
			}
		}
		if strings.Contains(got, "Nothing is listening") {
			t.Errorf("a squatter was diagnosed as a dead port\noutput: %s", got)
		}
	})
}
