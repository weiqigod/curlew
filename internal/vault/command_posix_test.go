//go:build !windows

package vault_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestCommandCLIInterruptCleanup(t *testing.T) {
	cli := buildProviderProgram(t, "../../cmd/curlew")
	project := t.TempDir()
	t.Setenv("CURLEW_CONFIG_DIR", filepath.Join(project, "user-config"))
	t.Setenv("CURLEW_TEAM_CONFIG", "")
	t.Setenv("CURLEW_PLUGINS", "")
	t.Setenv("CURLEW_VAULT_STUB", "")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	pidFile := filepath.Join(project, "command child.pid")
	quotedPIDFile := "'" + strings.ReplaceAll(pidFile, "'", "'\\''") + "'"
	writeProviderYAML(t, filepath.Join(project, "curlew.yaml"), map[string]any{
		"project_name": "interrupt-cleanup",
	})
	writeProviderYAML(t, filepath.Join(project, "interrupt.yaml"), map[string]any{
		"name": "interrupt cleanup",
		"variables": map[string]any{
			"command_value": map[string]any{"from_command": "echo $$ > " + quotedPIDFile + "; sleep 60"},
		},
		"requests": []any{map[string]any{
			"name":    "must not run",
			"request": map[string]any{"method": "GET", "url": server.URL},
		}},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, cli, "run", "interrupt.yaml")
	command.Dir = project
	command.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			cancel()
			_ = command.Wait()
		}
	}()
	childPID := 0
	defer func() {
		if childPID <= 1 {
			return
		}
		if err := syscall.Kill(-childPID, syscall.SIGKILL); err == nil {
			t.Logf("fallback cleanup: killed command process group %d", childPID)
		} else if errors.Is(err, syscall.ESRCH) {
			t.Logf("cleanup: command process group %d already gone", childPID)
		} else {
			t.Errorf("cleanup command process group %d: %v", childPID, err)
		}
	}()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	ready := time.NewTimer(5 * time.Second)
	defer ready.Stop()
	for childPID == 0 {
		contents, err := os.ReadFile(pidFile)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if text := strings.TrimSpace(string(contents)); text != "" {
			pid, err := strconv.Atoi(text)
			if err != nil || pid <= 1 {
				t.Fatalf("invalid command child PID %q: %v", text, err)
			}
			childPID = pid
			break
		}
		select {
		case <-ticker.C:
		case <-ready.C:
			cancel()
			waitErr := command.Wait()
			waited = true
			t.Fatalf("command child did not become ready: %v\nstdout: %s\nstderr: %s", waitErr, stdout.String(), stderr.String())
		}
	}
	if err := syscall.Kill(childPID, 0); err != nil {
		t.Fatalf("command child %d must be alive before interrupt: %v", childPID, err)
	}
	t.Logf("CLI PID %d; live command child PID/PGID %d", command.Process.Pid, childPID)
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	waitErr := command.Wait()
	waited = true
	if ctx.Err() != nil {
		t.Errorf("CLI did not exit before timeout: %v", ctx.Err())
	}
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) || exitErr.ExitCode() != 5 {
		t.Errorf("interrupted command resolution must exit 5, got %v\nstdout: %s\nstderr: %s", waitErr, stdout.String(), stderr.String())
	} else {
		t.Logf("CLI exit code 5\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}
	if count := requests.Load(); count != 0 {
		t.Errorf("sent %d HTTP requests before command resolution completed", count)
	}

	exited := time.NewTimer(3 * time.Second)
	defer exited.Stop()
	for {
		if err := syscall.Kill(childPID, 0); errors.Is(err, syscall.ESRCH) {
			t.Logf("command child %d exited after CLI interrupt", childPID)
			return
		} else if err != nil {
			t.Fatalf("check command child %d: %v", childPID, err)
		}
		select {
		case <-ticker.C:
		case <-exited.C:
			t.Fatalf("command child %d survived CLI interrupt", childPID)
		}
	}
}
