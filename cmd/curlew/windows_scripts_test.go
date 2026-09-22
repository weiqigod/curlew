package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestGoSourcesUseLFOnWindowsCheckouts(t *testing.T) {
	paths := []string{"cmd/curlew/main.go", "internal/variable/process_posix_test.go"}
	args := []string{"-C", readmeRepoRoot(t), "-c", "core.autocrlf=true", "check-attr", "-z", "eol", "--"}
	output, err := exec.Command("git", append(args, paths...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("check checkout attributes: %v\n%s", err, output)
	}
	fields := strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
	if len(fields) != 3*len(paths) {
		t.Fatalf("unexpected attribute output: %q", output)
	}
	for index := range paths {
		if fields[3*index+2] != "lf" {
			t.Errorf("%s eol = %q, want lf for native gofumpt", fields[3*index], fields[3*index+2])
		}
	}
}

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

func TestWindowsVerificationProfiles(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell verification entry point is native Windows coverage")
	}
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "verify-windows.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		profile   string
		missing   []string
		preflight bool
		failTests bool
		wantError string
		marker    string
		required  []string
		forbidden []string
	}{
		{
			name:      "default local needs neither compiler nor release tools",
			missing:   []string{"CCompilerCommand", "GoReleaserCommand"},
			marker:    "WINDOWS_LOCAL_VERIFY_PASS",
			required:  []string{"go build ./cmd/curlew cgo=0", "-coverprofile=coverage-windows.out", "lint run", "npm --prefix ui run check", "npm --prefix ui run lint", "npm --prefix ui test", "npm --prefix ui run build", "smoke/run.ps1"},
			forbidden: []string{"compiler ", "goreleaser ", "-race", "WINDOWS_VERIFY_PASS"},
		},
		{
			name: "race only needs Go and compiler", profile: "Race",
			missing: []string{"NodeCommand", "NpmCommand", "PythonCommand", "LintCommand", "LintGoCommand", "GoReleaserCommand"},
			marker:  "WINDOWS_RACE_VERIFY_PASS", required: []string{"compiler --version", "go test -p 1 -race ./... -count=1 -timeout=40m cgo=1"},
			forbidden: []string{"npm ", "lint ", "goreleaser ", "smoke/run.ps1", "-coverprofile"},
		},
		{
			name: "release only needs GoReleaser", profile: "Release",
			missing: []string{"GoCommand", "NodeCommand", "NpmCommand", "PythonCommand", "LintCommand", "LintGoCommand", "CCompilerCommand"},
			marker:  "WINDOWS_RELEASE_VERIFY_PASS", required: []string{"goreleaser check"},
			forbidden: []string{"go build", "go test", "compiler ", "npm ", "smoke/run.ps1"},
		},
		{
			name: "full keeps all gates", profile: "Full", marker: "WINDOWS_VERIFY_PASS",
			required: []string{"go build ./cmd/curlew cgo=0", "go test -p 1 -race ./... -count=1 -timeout=40m cgo=1", "go test -p 1 -coverprofile=coverage-windows.out ./... -timeout=30m cgo=0", "lint run", "goreleaser check", "smoke/run.ps1"},
		},
		{name: "local preflight ignores optional tools", preflight: true, missing: []string{"CCompilerCommand", "GoReleaserCommand"}, marker: "WINDOWS_PREFLIGHT_PASS profile=Local", forbidden: []string{"go build", "go test", "goreleaser ", "compiler "}},
		{name: "race fails without compiler", profile: "Race", preflight: true, missing: []string{"CCompilerCommand"}, wantError: "curlew-missing-CCompilerCommand"},
		{name: "release fails without release tool", profile: "Release", preflight: true, missing: []string{"GoReleaserCommand"}, wantError: "curlew-missing-GoReleaserCommand"},
		{name: "full fails without compiler", profile: "Full", preflight: true, missing: []string{"CCompilerCommand"}, wantError: "curlew-missing-CCompilerCommand"},
		{name: "local test failures propagate", failTests: true, wantError: "exited 7", forbidden: []string{"WINDOWS_LOCAL_VERIFY_PASS", "goreleaser check"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			logPath := filepath.Join(root, "commands.log")
			toolScripts := map[string]string{
				"go":         "if \"%~1\"==\"version\" echo go version go1.26.8 windows/amd64\r\nif \"%~1\"==\"tool\" echo total: statements 85.0%%\r\nif \"%~1\"==\"test\" if \"%CURLEW_VERIFY_FAIL_TESTS%\"==\"1\" exit /b 7\r\n",
				"node":       "echo v24.18.1\r\n",
				"npm":        "echo 11.7.0\r\n",
				"python":     "echo Python 3.14.7\r\n",
				"lint":       "echo golangci-lint has version 2.11.2\r\n",
				"goreleaser": "echo GitVersion: 2.17.1\r\n",
				"compiler":   "echo fixture compiler\r\n",
				"pwsh":       "",
			}
			for name, body := range toolScripts {
				content := "@echo off\r\necho " + name + " %* cgo=%CGO_ENABLED% >>\"%CURLEW_VERIFY_LOG%\"\r\n" + body + "exit /b 0\r\n"
				if err := os.WriteFile(filepath.Join(root, name+".cmd"), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"-NoProfile", "-File", script}
			if test.profile != "" {
				args = append(args, "-Profile", test.profile)
			}
			if test.preflight {
				args = append(args, "-PreflightOnly")
			}
			for flag, tool := range map[string]string{"GoCommand": "go", "NodeCommand": "node", "NpmCommand": "npm", "PythonCommand": "python", "LintCommand": "lint", "LintGoCommand": "go", "GoReleaserCommand": "goreleaser", "CCompilerCommand": "compiler"} {
				command := filepath.Join(root, tool+".cmd")
				for _, missing := range test.missing {
					if flag == missing {
						command = "curlew-missing-" + flag
					}
				}
				args = append(args, "-"+flag, command)
			}
			invocation := "& '" + strings.ReplaceAll(script, "'", "''") + "'"
			for _, argument := range args[3:] {
				if strings.HasPrefix(argument, "-") {
					invocation += " " + argument
				} else {
					invocation += " '" + strings.ReplaceAll(argument, "'", "''") + "'"
				}
			}
			harness := `& {
Remove-Item Env:GOROOT -ErrorAction SilentlyContinue
$env:CC = 'original-compiler'
$before = @{}
foreach ($name in @('GOROOT', 'PATH', 'CGO_ENABLED', 'CC')) {
    $before[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
$directory = (Get-Location).Path
$result = 0
try { ` + invocation + ` } catch { Write-Output $_; $result = 1 }
foreach ($name in $before.Keys) {
    if ([Environment]::GetEnvironmentVariable($name, 'Process') -cne $before[$name]) { throw "Environment leaked: $name" }
}
if ((Get-Location).Path -ne $directory) { throw 'Working directory leaked' }
Write-Output 'ENVIRONMENT_RESTORED'
exit $result
}`
			cmd := exec.Command(pwsh, "-NoProfile", "-Command", harness)
			cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "CURLEW_VERIFY_LOG="+logPath, "CGO_ENABLED=1", "CURLEW_VERIFY_FAIL_TESTS=0")
			if test.failTests {
				cmd.Env = append(cmd.Env, "CURLEW_VERIFY_FAIL_TESTS=1")
			}
			output, runErr := cmd.CombinedOutput()
			if !strings.Contains(string(output), "ENVIRONMENT_RESTORED") {
				t.Fatalf("verifier did not restore its caller's environment: %v\n%s", runErr, output)
			}
			if test.wantError != "" {
				if runErr == nil || !strings.Contains(string(output), test.wantError) {
					t.Fatalf("wanted %q failure; got %v\n%s", test.wantError, runErr, output)
				}
			} else if runErr != nil || !strings.Contains(string(output), test.marker) {
				t.Fatalf("wanted %s; got %v\n%s", test.marker, runErr, output)
			}
			commands, err := os.ReadFile(logPath)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			text := strings.ReplaceAll(string(commands)+string(output), "\\", "/")
			for _, required := range test.required {
				if !strings.Contains(text, required) {
					t.Errorf("missing %q in verifier calls:\n%s", required, text)
				}
			}
			for _, forbidden := range test.forbidden {
				if strings.Contains(text, forbidden) {
					t.Errorf("unexpected %q in verifier calls:\n%s", forbidden, text)
				}
			}
		})
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
