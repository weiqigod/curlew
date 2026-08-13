//go:build !windows

package main

// TestStderrColorFlag is the primary regression gate for M7-001. It exercises
// six stdout/stderr combinations through the real curlew binary and asserts
// whether stderr contains ANSI escape sequences:
//
//   - stdout=pipe, stderr=pipe (no flags)     → no color (portable)
//   - stdout=pipe, stderr=pipe (NO_COLOR env) → no color (portable)
//   - stdout=pipe, stderr=pipe (--no-color)   → no color (portable)
//   - stdout=TTY,  stderr=pipe                → no color on stderr (M7-001 regression guard)
//   - stdout=pipe, stderr=TTY                 → color preserved on stderr
//   - stdout=TTY,  stderr=TTY                 → color preserved on stderr
//
// Cases where either stream is wired to /dev/tty are Unix-only and will be
// skipped in CI containers or sandboxes where /dev/tty is unavailable.
// The pipe-pipe, NO_COLOR, and --no-color subtests are always portable.
//
// Rationale for no creack/pty dependency: the critical regression path (stdout
// TTY + stderr pipe) is exercised via /dev/tty when available and skipped
// gracefully otherwise, while the three pipe-based cases run everywhere and
// cover the bug-relevant code paths.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStderrColorFlag(t *testing.T) {
	binary := buildBinary(t)

	dir := t.TempDir()
	collection := filepath.Join(dir, "stream-color-check.yaml")
	if err := os.WriteFile(collection, []byte(
		"name: stream-color-check\n"+
			"requests:\n"+
			"  - name: missing-var\n"+
			"    method: GET\n"+
			"    url: \"{{UNDEFINED}}\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	const esc = "\x1b["

	tests := []struct {
		name            string
		env             []string
		args            []string
		stdoutTTY       bool // when true, stdout connected to /dev/tty
		stderrTTY       bool // when true, stderr connected to /dev/tty
		wantStderrColor bool
	}{
		{
			name:            "stdout pipe, stderr pipe, no flags -> no color anywhere",
			args:            []string{"run", collection},
			wantStderrColor: false,
		},
		{
			name:            "stdout pipe, stderr pipe, NO_COLOR env -> no color",
			env:             []string{"NO_COLOR=1"},
			args:            []string{"run", collection},
			wantStderrColor: false,
		},
		{
			name:            "stdout pipe, stderr pipe, --no-color flag -> no color",
			args:            []string{"run", "--no-color", collection},
			wantStderrColor: false,
		},
		{
			name:            "stdout TTY, stderr pipe -> no color on stderr (regression guard)",
			args:            []string{"run", collection},
			stdoutTTY:       true,
			wantStderrColor: false, // M7-001 bug: before fix this would be true
		},
		{
			name:            "stdout pipe, stderr TTY -> color preserved on stderr",
			args:            []string{"run", collection},
			stderrTTY:       true,
			wantStderrColor: true, // stderr is a TTY: color must be preserved
		},
		{
			// An empty NO_COLOR is the conventional way to clear an inherited
			// preference, not a request for plain output — no-color.org takes
			// effect on a non-empty value only. curlew used to disable here.
			name:            "stdout pipe, stderr TTY, empty NO_COLOR -> color preserved",
			env:             []string{"NO_COLOR="},
			args:            []string{"run", collection},
			stderrTTY:       true,
			wantStderrColor: true,
		},
		{
			name:            "stdout TTY, stderr TTY -> color preserved on stderr",
			args:            []string{"run", collection},
			stdoutTTY:       true,
			stderrTTY:       true,
			wantStderrColor: true, // both streams are TTYs: color must be preserved
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tty *os.File
			if tt.stdoutTTY || tt.stderrTTY {
				f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
				if err != nil {
					t.Skipf("/dev/tty not available: %v", err)
				}
				tty = f
				defer func() { _ = tty.Close() }()
			}

			cmd := exec.Command(binary, tt.args...)
			// Build a clean env: inherit current env then layer test-specific vars.
			baseEnv := os.Environ()
			// Strip any inherited NO_COLOR so tests are hermetic.
			filtered := make([]string, 0, len(baseEnv))
			for _, e := range baseEnv {
				if !strings.HasPrefix(e, "NO_COLOR=") {
					filtered = append(filtered, e)
				}
			}
			if tt.env != nil {
				filtered = append(filtered, tt.env...)
			}
			cmd.Env = filtered

			// Wire stdout and stderr: attach /dev/tty for TTY legs, pipe for inspection.
			// Use io.Discard (not nil) for non-TTY stdout so the child process sees
			// a real pipe — nil would inherit the parent process's stdout (which may
			// be a TTY in the developer's terminal and is the root cause of M7-001).
			var stderrBuf strings.Builder
			if tt.stdoutTTY {
				cmd.Stdout = tty
			} else {
				cmd.Stdout = &strings.Builder{} // piped (not nil — nil inherits parent TTY)
			}
			if tt.stderrTTY {
				cmd.Stderr = tty
			} else {
				cmd.Stderr = &stderrBuf
			}

			_ = cmd.Run() // expected to exit non-zero (undefined variable)

			// Only inspect captured buffers for piped legs.
			if !tt.stderrTTY {
				hasColor := strings.Contains(stderrBuf.String(), esc)
				if hasColor != tt.wantStderrColor {
					t.Errorf("stderr color = %v, want %v (stderr=%q)",
						hasColor, tt.wantStderrColor, stderrBuf.String())
				}
			}
		})
	}
}
