//go:build windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsSmokeCleansUpAfterReadinessTimeout(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available")
	}
	python, prefix := testPython(t)
	if len(prefix) != 0 {
		t.Skip("cleanup test requires a direct Python executable")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "fixture.pid")
	fixture := filepath.Join(dir, "fixture with spaces.py")
	body := fmt.Sprintf("import os, pathlib, time\npathlib.Path(%q).write_text(str(os.getpid()))\ntime.sleep(60)\n", pidFile)
	if err := os.WriteFile(fixture, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join("..", "..", "smoke", "run.ps1")
	cmd := exec.Command(pwsh, "-NoProfile", "-File", script,
		"-PythonCommand", python,
		"-FixtureScript", fixture,
		"-ReadyTimeoutSeconds", "1")
	output, err := cmd.CombinedOutput()
	if err == nil || !bytes.Contains(output, []byte("did not publish its port")) {
		t.Fatalf("readiness timeout = %v, output=%s", err, output)
	}
	pidBody, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("fixture did not report pid: %v\n%s", err, output)
	}
	var pid int
	if _, err := fmt.Sscanf(string(pidBody), "%d", &pid); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		handle, openErr := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
		if errors.Is(openErr, windows.ERROR_INVALID_PARAMETER) {
			return
		}
		if openErr != nil {
			t.Fatalf("open fixture process %d: %v", pid, openErr)
		}
		status, waitErr := windows.WaitForSingleObject(handle, 0)
		_ = windows.CloseHandle(handle)
		if waitErr != nil {
			t.Fatalf("wait fixture process %d: %v", pid, waitErr)
		}
		if status == uint32(windows.WAIT_OBJECT_0) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("fixture process %d survived smoke failure", pid)
}

func TestWindowsSmokeEndToEnd(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available")
	}
	goCommand, err := exec.LookPath("go")
	if err != nil {
		t.Fatal("Go is required for native smoke")
	}
	python, prefix := testPython(t)
	if len(prefix) != 0 {
		t.Skip("native smoke test requires a direct Python executable")
	}
	t.Setenv("CURLEW_CONFIG_DIR", filepath.Join(t.TempDir(), "ambient-config"))
	t.Setenv("CURLEW_PLUGINS", filepath.Join(t.TempDir(), "ambient-plugin.exe"))
	t.Setenv("CURLEW_TEAM_CONFIG", filepath.Join(t.TempDir(), "ambient-team.yaml"))

	script := filepath.Join("..", "..", "smoke", "run.ps1")
	cmd := exec.Command(pwsh, "-NoProfile", "-File", script,
		"-GoCommand", goCommand,
		"-PythonCommand", python)
	output, err := cmd.CombinedOutput()
	if err != nil || !bytes.Contains(output, []byte("WINDOWS_SMOKE_PASS")) {
		t.Fatalf("native smoke = %v\n%s", err, output)
	}
}
