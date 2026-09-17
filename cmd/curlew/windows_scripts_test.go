package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
