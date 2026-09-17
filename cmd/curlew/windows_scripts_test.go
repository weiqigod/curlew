package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsVerificationPreflightRejectsMissingTool(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell verification entry point is native Windows coverage")
	}
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available")
	}
	script := filepath.Join("..", "..", "scripts", "verify-windows.ps1")
	cmd := exec.Command(pwsh, "-NoProfile", "-File", script, "-PreflightOnly", "-GoCommand", "curlew-tool-that-does-not-exist")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("missing required Go tool exited 0\n%s", output)
	}
	if !strings.Contains(string(output), "missing required tool") || !strings.Contains(string(output), "curlew-tool-that-does-not-exist") {
		t.Fatalf("missing-tool diagnostic = %q", output)
	}
}

func TestWindowsVerificationPreflightChecksDeveloperTools(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell verification entry point is native Windows coverage")
	}
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available")
	}
	script := filepath.Join("..", "..", "scripts", "verify-windows.ps1")
	cmd := exec.Command(pwsh, "-NoProfile", "-File", script, "-PreflightOnly", "-LintCommand", "curlew-linter-that-does-not-exist")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("missing required linter exited 0\n%s", output)
	}
	if !strings.Contains(string(output), "curlew-linter-that-does-not-exist") {
		t.Fatalf("missing-linter diagnostic = %q", output)
	}
}

func TestWindowsVerificationRunsAllUIQualityChecks(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "scripts", "verify-windows.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, command := range []string{`"run" "check"`, `"run" "lint"`, `"test"`, `"run" "build"`} {
		if !strings.Contains(text, command) {
			t.Errorf("Windows verifier is missing UI command %s", command)
		}
	}
}

func TestWindowsVerificationEnforcesPinnedToolsAndCoverage(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "scripts", "verify-windows.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, contract := range []string{
		"golangci-lint 2.11.2 is required",
		"GoReleaser 2.17.1 is required",
		"Go 1.26.x is required for golangci-lint",
		"$coveragePercent -lt 80",
		"coverage is below 80 percent",
	} {
		if !strings.Contains(text, contract) {
			t.Errorf("Windows verifier is missing contract %q", contract)
		}
	}
}

func TestSmokeScriptsOwnOnlyTheirResources(t *testing.T) {
	tests := []struct {
		path      string
		forbidden []string
		required  []string
	}{
		{
			path: filepath.Join("..", "..", "smoke", "run.sh"),
			forbidden: []string{
				`lsof -ti`,
				`xargs kill`,
				`rm -f /tmp/curlew_*`,
				`trap - EXIT`,
			},
			required: []string{"SMOKE_ROOT", "cleanup", "127.0.0.1"},
		},
		{
			path: filepath.Join("..", "..", "smoke", "run.ps1"),
			forbidden: []string{
				"Get-NetTCPConnection",
				"Get-Process |",
				"taskkill",
				"Start-Process -FilePath $PythonCommand -ArgumentList",
			},
			required: []string{
				"New-TemporaryFile",
				"127.0.0.1",
				"Stop-Process -Id $server.Id",
				"ProcessStartInfo",
				"ArgumentList.Add",
				"CURLEW_CONFIG_DIR",
				"CURLEW_TELEMETRY_FILE",
				"CURLEW_PLUGINS",
				"CURLEW_TEAM_CONFIG",
			},
		},
	}

	for _, test := range tests {
		t.Run(filepath.Base(test.path), func(t *testing.T) {
			content, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatalf("read %s: %v", test.path, err)
			}
			text := string(content)
			for _, forbidden := range test.forbidden {
				if strings.Contains(text, forbidden) {
					t.Errorf("%s contains unsafe pattern %q", test.path, forbidden)
				}
			}
			for _, required := range test.required {
				if !strings.Contains(text, required) {
					t.Errorf("%s is missing ownership marker %q", test.path, required)
				}
			}
		})
	}
}

func TestPosixSmokeRegistersEveryChild(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "smoke", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(content), "\n")
	assignment := regexp.MustCompile(`^\s*([A-Z][A-Z0-9_]*_PID)=\$!\s*$`)
	for index, line := range lines {
		match := assignment.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		end := min(index+4, len(lines))
		nearby := strings.Join(lines[index+1:end], "\n")
		if !strings.Contains(nearby, `register_pid "$`+match[1]+`"`) {
			t.Errorf("%s is not registered immediately after process start", match[1])
		}
	}
}

func TestPosixSmokeUsesDynamicPortsAndOwnedTempRoot(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "smoke", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	fixedPort := regexp.MustCompile(`(?m)^[A-Z][A-Z0-9_]*_PORT=[0-9]+\s*$`)
	if matches := fixedPort.FindAllString(text, -1); len(matches) > 0 {
		t.Errorf("POSIX smoke contains fixed listener assignments: %v", matches)
	}
	if strings.Contains(text, "127.0.0.1:9190") {
		t.Error("POSIX smoke contains the historical fixed HTTP fixture port")
	}
	if strings.Contains(text, "/tmp/") {
		t.Error("POSIX smoke creates artifacts outside SMOKE_ROOT")
	}
}

func TestHTTPBinFixtureSupportsDynamicPortFile(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "smoke", "fixtures", "httpbin_server.py"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{"--port-file", "server_address[1]"} {
		if !strings.Contains(text, required) {
			t.Errorf("httpbin fixture is missing %q", required)
		}
	}
}

func TestWindowsSmokeCleansUpAfterReadinessTimeout(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell smoke is native Windows coverage")
	}
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
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell smoke is native Windows coverage")
	}
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
