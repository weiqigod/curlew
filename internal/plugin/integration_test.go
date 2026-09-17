//go:build !short

package plugin

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestHost_Load_WithRealFixture builds the hello-plugin fixture and tests it
// against the real execSpawner. Skipped in -short mode or when go is not on PATH.
func TestHost_Load_WithRealFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("skip in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	tmp := t.TempDir()
	binPath := filepath.Join(tmp, discoveryExecutableName("hello-plugin"))

	// Build the fixture relative to this test file.
	cmd := exec.Command("go", "build", "-o", binPath,
		"../../testdata/plugins/hello-plugin")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build fixture: %v", err)
	}
	warmPluginBinary(t, binPath)

	host := NewHost(io.Discard)
	loaded, errs, err := host.Load(context.Background(), binPath)
	if err != nil {
		t.Fatalf("Host.Load: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("unexpected loadErrs: %+v", errs)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 loaded plugin, got %d: %+v", len(loaded), loaded)
	}
	p := loaded[0]
	if p.Name != "hello-plugin" {
		t.Errorf("name: got %q, want %q", p.Name, "hello-plugin")
	}
	if p.Version != "0.1.0" {
		t.Errorf("version: got %q, want %q", p.Version, "0.1.0")
	}
	if !reflect.DeepEqual(p.Hooks, []string{"on_request", "on_response"}) {
		t.Errorf("hooks: got %v, want [on_request, on_response]", p.Hooks)
	}
	if p.Path != binPath {
		t.Errorf("path: got %q, want %q", p.Path, binPath)
	}
}

func TestHost_Close_RealProcessTree(t *testing.T) {
	t.Run("cooperative exit", func(t *testing.T) {
		testHostCloseRealProcessTree(t, false)
	})
	t.Run("stalled after EOF", func(t *testing.T) {
		testHostCloseRealProcessTree(t, true)
	})
}

func testHostCloseRealProcessTree(t *testing.T, stallAfterEOF bool) {
	if testing.Short() {
		t.Skip("skip in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, discoveryExecutableName("lifecycle-plugin"))
	build := exec.Command("go", "build", "-buildvcs=false", "-o", binary,
		"../../testdata/plugins/lifecycle-plugin")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build lifecycle plugin: %v\n%s", err, output)
	}
	warmPluginBinary(t, binary, "probe")

	pidFile := filepath.Join(dir, "child.pid")
	eofFile := filepath.Join(dir, "eof.marker")
	t.Setenv("CURLEW_PLUGIN_CHILD_PID_FILE", pidFile)
	t.Setenv("CURLEW_PLUGIN_EOF_FILE", eofFile)
	if stallAfterEOF {
		t.Setenv("CURLEW_PLUGIN_STALL_AFTER_EOF", "1")
	}

	host := NewHost(io.Discard)
	plugins, channels, loadErrs, err := host.LoadForRun(context.Background(), binary)
	if err != nil {
		t.Fatalf("LoadForRun: %v", err)
	}
	if len(loadErrs) != 0 || len(plugins) != 1 || len(channels) != 1 {
		t.Fatalf("plugins=%v channels=%d loadErrs=%+v", plugins, len(channels), loadErrs)
	}

	childPID := readPIDFile(t, pidFile)
	t.Cleanup(func() {
		if processAlive(childPID) {
			if process, findErr := os.FindProcess(childPID); findErr == nil {
				_ = process.Kill()
			}
		}
	})

	started := time.Now()
	if err := host.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*shutdownTimeout {
		t.Fatalf("Close took %s, want at most %s", elapsed, 2*shutdownTimeout)
	}
	if _, err := os.Stat(eofFile); err != nil {
		t.Fatalf("plugin did not observe stdin EOF before termination: %v", err)
	}
	waitForProcessExit(t, childPID)
}

func warmPluginBinary(t *testing.T, binary string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := exec.CommandContext(ctx, binary, args...).Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("warm plugin fixture: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("warm plugin fixture: %v", ctx.Err())
	}
}

func readPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(body)))
			if parseErr != nil {
				t.Fatalf("parse child pid %q: %v", body, parseErr)
			}
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child pid file %s was not created", path)
	return 0
}

func waitForProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("plugin descendant %d is still running", pid)
}
