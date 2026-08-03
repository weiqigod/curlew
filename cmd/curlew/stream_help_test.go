package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestStreamHelp is the M7-003 regression guard for the help/error stream
// split. For every (subcommand × invocation-style) combination:
//   - On --help / no-args (explicit help request), stdout carries the full
//     help block and stderr is empty; exit 0.
//   - On error-recovery paths (unknown command, malformed flag), stderr
//     carries both the error message AND a one-line "Usage:" synopsis; stdout
//     is empty; exit code matches the command's normal usage-error code.
func TestStreamHelp(t *testing.T) {
	t.Run("top_level_unknown_command", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runWithWriters([]string{"bogus-command"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("exit = %d, want 1", code)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), "Unknown command") {
			t.Errorf("stderr missing 'Unknown command': %q", stderr.String())
		}
		if !strings.Contains(stderr.String(), "Usage: curlew") {
			t.Errorf("stderr missing 'Usage: curlew' synopsis: %q", stderr.String())
		}
	})

	t.Run("top_level_explicit_help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runWithWriters([]string{"--help"}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("exit = %d, want 0", code)
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr should be empty for --help, got: %q", stderr.String())
		}
		if !strings.Contains(stdout.String(), "Commands:") {
			t.Errorf("stdout missing 'Commands:' block: %q", stdout.String())
		}
	})

	t.Run("top_level_no_args", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runWithWriters([]string{}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("exit = %d, want 0", code)
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr should be empty for no-args help, got: %q", stderr.String())
		}
		if !strings.Contains(stdout.String(), "Commands:") {
			t.Errorf("stdout missing 'Commands:' block: %q", stdout.String())
		}
	})

	t.Run("plugins_unknown_subcommand", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runWithWriters([]string{"plugins", "bogus"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("exit = %d, want 1", code)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), "Unknown plugins subcommand") {
			t.Errorf("stderr missing error message: %q", stderr.String())
		}
		if !strings.Contains(stderr.String(), "Usage: curlew plugins") {
			t.Errorf("stderr missing synopsis: %q", stderr.String())
		}
	})

	t.Run("plugins_explicit_help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runWithWriters([]string{"plugins", "--help"}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("exit = %d, want 0", code)
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr should be empty for --help, got: %q", stderr.String())
		}
		if !strings.Contains(stdout.String(), "Subcommands:") {
			t.Errorf("stdout missing 'Subcommands:' block: %q", stdout.String())
		}
	})

	t.Run("perf_malformed_flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runWithWriters([]string{"perf", "--bogus"}, &stdout, &stderr)
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), "error:") {
			t.Errorf("stderr missing error: %q", stderr.String())
		}
		if !strings.Contains(stderr.String(), "Usage: curlew perf") {
			t.Errorf("stderr missing synopsis: %q", stderr.String())
		}
	})

	t.Run("perf_explicit_help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runWithWriters([]string{"perf", "--help"}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("exit = %d, want 0", code)
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr should be empty for --help, got: %q", stderr.String())
		}
		if !strings.Contains(stdout.String(), "Usage: curlew perf") {
			t.Errorf("stdout missing full help: %q", stdout.String())
		}
	})

	t.Run("import_no_args", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runWithWriters([]string{"import"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("exit = %d, want 1", code)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), "Usage: curlew import") {
			t.Errorf("stderr missing synopsis: %q", stderr.String())
		}
	})

	t.Run("import_unknown_format", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runWithWriters([]string{"import", "postman"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("exit = %d, want 1", code)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout should be empty, got: %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), "Unknown import format") {
			t.Errorf("stderr missing error: %q", stderr.String())
		}
		if !strings.Contains(stderr.String(), "Usage: curlew import") {
			t.Errorf("stderr missing synopsis: %q", stderr.String())
		}
	})
}
