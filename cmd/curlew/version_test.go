package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The version must be settable at link time. A release binary has to report
// the tag it was cut from, and a `const` cannot carry that: -ldflags -X is
// silently ignored for constants, so a release pipeline would produce
// binaries that all claim to be the development version without any build
// step failing to warn about it.

const testVersion = "9.9.9-ldflags-test"

// buildBinaryWithVersion builds the real binary with the version injected,
// exactly as a release build would.
func buildBinaryWithVersion(t *testing.T, v string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "curlew")
	cmd := exec.Command("go", "build",
		"-ldflags", "-X main.version="+v,
		"-o", binary, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build with -ldflags failed: %v\n%s", err, out)
	}
	return binary
}

func TestVersion_is_injectable_at_link_time(t *testing.T) {
	binary := buildBinaryWithVersion(t, testVersion)

	stdout, _, code := runBinary(t, binary, "--version")
	if code != 0 {
		t.Fatalf("--version exited %d", code)
	}
	got := strings.TrimSpace(stdout)
	want := "curlew " + testVersion
	if got != want {
		t.Errorf("--version = %q, want %q — the linker flag did not take effect, so release builds would misreport their version",
			got, want)
	}
}

// runBinaryIn runs the binary with a working directory, which the shared
// runBinary helper does not support. `info` resolves a project relative to
// the process working directory, so it must run inside one.
func runBinaryIn(t *testing.T, dir, binary string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// The injected version must reach the surfaces that report it, not only
// `--version`. Both of these are read by humans deciding which build they are
// looking at, so a release binary reporting the development version here
// would be actively misleading.
func TestVersion_injection_reaches_help(t *testing.T) {
	binary := buildBinaryWithVersion(t, testVersion)

	stdout, _, code := runBinary(t, binary, "--help")
	if code != 0 {
		t.Fatalf("--help exited %d", code)
	}
	if !strings.Contains(stdout, "Version: "+testVersion) {
		t.Errorf("--help does not report the injected version %q\n---\n%s", testVersion, stdout)
	}
}

// `info --format json` carries the version as machine-readable provenance —
// the plain text form deliberately does not. This is the surface a script or
// results consumer reads to record which build produced an artifact, so an
// uninjected version here is wrong long after the run.
func TestVersion_injection_reaches_info_json(t *testing.T) {
	binary := buildBinaryWithVersion(t, testVersion)
	dir := t.TempDir()

	if _, stderr, code := runBinaryIn(t, dir, binary, "init"); code != 0 {
		t.Fatalf("init exited %d: %s", code, stderr)
	}

	stdout, stderr, code := runBinaryIn(t, dir, binary, "info", "--format", "json")
	if code != 0 {
		t.Fatalf("info --format json exited %d: %s", code, stderr)
	}

	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("info --format json is not valid JSON: %v\n---\n%s", err, stdout)
	}
	if payload.Version != testVersion {
		t.Errorf("info JSON version = %q, want %q", payload.Version, testVersion)
	}
}

// Without an override the binary still reports a sensible default, so a
// plain `go build` or `go install` is never versionless.
func TestVersion_default_when_not_injected(t *testing.T) {
	binary := buildBinary(t)

	stdout, _, code := runBinary(t, binary, "--version")
	if code != 0 {
		t.Fatalf("--version exited %d", code)
	}
	got := strings.TrimSpace(stdout)
	if got == "curlew " || !strings.HasPrefix(got, "curlew ") {
		t.Errorf("--version = %q, want a non-empty default version", got)
	}
}
