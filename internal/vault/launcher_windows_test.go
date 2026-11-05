package vault_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/docs"
	"github.com/weiqigod/curlew/internal/variable"
	"github.com/weiqigod/curlew/internal/vault"
)

// Azure CLI 2.90.0 MSI and ZIP use this forwarding pattern, including -I and
// the parenthesized IF. Only the installer value differs; neither uses CALL.
// https://github.com/Azure/azure-cli/blob/azure-cli-2.90.0/build_scripts/windows/scripts/az_msi.cmd
// https://github.com/Azure/azure-cli/blob/azure-cli-2.90.0/build_scripts/windows/scripts/az_zip.cmd
const windowsAzureLauncher = `@IF EXIST "%~dp0\..\python.exe" (
  SET AZ_INSTALLER=@INSTALLER@
  "%~dp0\..\python.exe" -IBm azure.cli %*
) ELSE (
  echo Failed to load python executable.
  exit /b 1
)
`

// Google Cloud CLI 585.0.0: the reduced bootstrap uses a supplied interpreter,
// retains the version probe and delayed-expansion transition (lines 38/109/142),
// and preserves the final forwarding and exit lines (158/161/162), without CALL.
// Python discovery and unrelated SDK initialization are intentionally omitted.
// https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-sdk-585.0.0-windows-x86_64-bundled-python.zip
// Archive SHA256: 42ab5eb7ccc4c217f96afcc17f427f177b58af5334854e358d2b909115c2fd79
const windowsGcloudLauncher = `@echo off
SETLOCAL EnableDelayedExpansion
"%CLOUDSDK_PYTHON%" -c "import sys; print(sys.version_info[0])" >NUL
SETLOCAL DisableDelayedExpansion
"%CLOUDSDK_PYTHON%" %CLOUDSDK_PYTHON_ARGS% "%CLOUDSDK_ROOT_DIR%\lib\gcloud.py" %* & goto lastline 2>NUL || "%COMSPEC%" /C exit 0
:lastline
"%COMSPEC%" /C exit %ERRORLEVEL%
`

const windowsLauncherPython = `import json
import os
import sys

invocation = {
    "args": sys.argv[1:],
    "env": {
        "python": sys.executable,
        "isolated": str(sys.flags.isolated),
        "dont_write_bytecode": str(sys.dont_write_bytecode),
        "AZ_INSTALLER": os.environ.get("AZ_INSTALLER", ""),
        "FOO": os.environ.get("FOO", ""),
        "inherited": os.environ.get("CURLEW_LAUNCHER_INHERITED", ""),
    },
}
with open(os.environ["CURLEW_LAUNCHER_CAPTURE"], "a", encoding="utf-8") as capture:
    capture.write(json.dumps(invocation, ensure_ascii=True) + "\n")
sys.stdout.buffer.write((os.environ["CURLEW_LAUNCHER_SECRET"] + "\r\n").encode("utf-8"))
diagnostic = os.environ.get("CURLEW_LAUNCHER_ERROR", "")
if diagnostic:
    sys.stderr.buffer.write((diagnostic + "\n").encode("utf-8"))
    sys.exit(42)
`

func windowsLauncherInterpreter(t *testing.T) string {
	t.Helper()
	var failures []string
	for _, candidate := range []struct {
		program string
		args    []string
	}{
		{"python", nil},
		{"py", []string{"-3"}},
	} {
		program, err := exec.LookPath(candidate.program)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		args := append(candidate.args, "-I", "-c", "import json, sys; assert sys.version_info.major == 3; print(json.dumps(sys.executable))")
		output, err := exec.CommandContext(ctx, program, args...).CombinedOutput()
		cancel()
		var interpreter string
		if err == nil && json.Unmarshal(output, &interpreter) == nil && filepath.IsAbs(interpreter) {
			t.Logf("real Python argv oracle: %s", interpreter)
			return interpreter
		}
		failures = append(failures, fmt.Sprintf("%s: %v; output=%q", program, err, output))
	}
	t.Fatalf("Windows provider launcher compatibility requires real Python 3 with the standard-library venv module. Put Python on PATH (on this host: C:\\Python314), or make 'py -3' available, then rerun go test ./internal/vault/ -run TestWindowsProviderLaunchers. No cloud SDK or network installation is needed. Attempts: %s", strings.Join(failures, "; "))
	return ""
}

func writeWindowsLauncherFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(filepath.Ext(path), ".cmd") {
		content = strings.ReplaceAll(content, "\n", "\r\n")
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func setupWindowsLauncher(t *testing.T, interpreter, installer string) (program, capture, childPython string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "provider tools O'Brien \u96ea")
	childPython = interpreter
	if installer != "" {
		venv := filepath.Join(root, "venv")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, interpreter, "-I", "-m", "venv", "--without-pip", venv)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("create offline Python venv: %v\n%s\nUse a full Python 3 installation with the standard-library venv module; no pip or cloud SDK is required", err, output)
		}
		childPython = filepath.Join(venv, "Scripts", "python.exe")
		packageRoot := filepath.Join(venv, "Lib", "site-packages", "azure")
		writeWindowsLauncherFixture(t, filepath.Join(packageRoot, "__init__.py"), "")
		writeWindowsLauncherFixture(t, filepath.Join(packageRoot, "cli", "__init__.py"), "")
		writeWindowsLauncherFixture(t, filepath.Join(packageRoot, "cli", "__main__.py"), windowsLauncherPython)
		program = filepath.Join(venv, "Scripts", "wbin", "az.cmd")
		writeWindowsLauncherFixture(t, program, strings.ReplaceAll(windowsAzureLauncher, "@INSTALLER@", installer))
	} else {
		program = filepath.Join(root, "bin", "gcloud.cmd")
		writeWindowsLauncherFixture(t, program, windowsGcloudLauncher)
		writeWindowsLauncherFixture(t, filepath.Join(root, "lib", "gcloud.py"), windowsLauncherPython)
	}
	capture = filepath.Join(root, "capture.jsonl")
	writeWindowsLauncherFixture(t, capture, "")
	t.Setenv("PATH", filepath.Dir(program)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	t.Setenv("COMSPEC", filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"))
	t.Setenv("CLOUDSDK_ROOT_DIR", root)
	t.Setenv("CLOUDSDK_PYTHON", childPython)
	t.Setenv("CLOUDSDK_PYTHON_ARGS", "-I -B")
	t.Setenv("AZ_INSTALLER", "")
	t.Setenv("FOO", "must-not-expand")
	t.Setenv("CURLEW_BATCH_INVOCATION_0", "parent-transport-value")
	t.Setenv("CURLEW_LAUNCHER_CAPTURE", capture)
	t.Setenv("CURLEW_LAUNCHER_SECRET", stubSecret)
	t.Setenv("CURLEW_LAUNCHER_ERROR", "")
	t.Setenv("CURLEW_LAUNCHER_INHERITED", "inherited-value")
	return program, capture, childPython
}

func TestWindowsProviderLaunchers(t *testing.T) {
	if _, err := docs.Prose("MANUAL.md", "provider launchers always preserve quoted arguments"); err != nil {
		t.Fatal(err)
	}
	interpreter := windowsLauncherInterpreter(t)
	t.Chdir(t.TempDir())
	for _, launcher := range []struct {
		name      string
		installer string
	}{
		{"azure-msi-2.90.0", "MSI"},
		{"azure-zip-2.90.0", "ZIP"},
		{"gcloud-585.0.0", ""},
	} {
		t.Run(launcher.name, func(t *testing.T) {
			program, capture, childPython := setupWindowsLauncher(t, interpreter, launcher.installer)
			marker := filepath.Join(t.TempDir(), "command-injection-marker")
			arguments := []string{
				"", "spaces here", "O'Brien", "\u00e5\u96ea\U0001f642", "tab\there", "  spaced  ",
				`C:\path with spaces\`, `\\`, `double"quote`, `"`, `"quoted words"`,
				`a""b`, `\"`, `trailing\"`, `a"&b|c^d<e>f`, "\u96ea\"\u00e5",
				`two\\"&|^<>()end`, `three\\\"&|^<>()end`, `"quoted"\`, `"quoted"\\`, `"^"`, `^"^`,
				"%FOO%", "!FOO!", `"%FOO%"`, `"!FOO!"`, "%PATH%", "%1", "%*", "%%",
				"%CURLEW_BATCH_INVOCATION_0%", "&", "|", "^", "<", ">", "()", "=", "/?",
				"a&b|c^d<e>f", `" & echo injected>"` + marker + `" & rem "`,
				`" | echo injected>"` + marker + `" & rem "`,
				`" & (echo injected)>"` + marker + `" & rem "`, "",
			}
			for character := byte(1); character < 32; character++ {
				if character != '\r' && character != '\n' {
					arguments = append(arguments, "before"+string(character)+"after")
				}
			}
			t.Cleanup(func() {
				if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("launcher interpreted user arguments as commands: marker stat=%v", err)
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			t.Run("python-exact-argv", func(t *testing.T) {
				output, err := variable.ExecuteProgram(ctx, program, arguments, nil)
				if err != nil {
					t.Fatalf("launcher output=%q error=%v diagnostic=%q", output, err, variable.CommandDiagnostic(err))
				}
				if output != stubSecret {
					t.Errorf("output=%q, want exact secret %q", output, stubSecret)
				}
				invocations := readProviderInvocations(t, capture)
				if len(invocations) != 1 {
					t.Fatalf("Python invocation count=%d, want 1", len(invocations))
				}
				if !reflect.DeepEqual(invocations[0].Args, arguments) {
					t.Errorf("Python argv=%#v\nwant=%#v", invocations[0].Args, arguments)
				}
				wantEnv := map[string]string{
					"python": childPython, "isolated": "1", "dont_write_bytecode": "True",
					"AZ_INSTALLER": launcher.installer, "FOO": "must-not-expand", "inherited": "inherited-value",
				}
				if !reflect.DeepEqual(invocations[0].Env, wantEnv) {
					t.Errorf("Python environment=%#v, want=%#v", invocations[0].Env, wantEnv)
				}
			})
			t.Run("nil-executor-provider", func(t *testing.T) {
				defer requireProviderNoPanic(t)
				before := len(readProviderInvocations(t, capture))
				scope := "scope '\u96ea \"quoted\" &|<>^()%FOO%!FOO! trailing\\"
				var provider vault.Provider
				var validateArgs []string
				var fetchArgs func(string) []string
				if launcher.installer != "" {
					provider = vault.NewAzureProvider(scope, nil)
					validateArgs = []string{"account", "show"}
					fetchArgs = func(path string) []string {
						return []string{"keyvault", "secret", "show", "--name", path, "--vault-name", scope, "--query", "value", "-o", "tsv"}
					}
				} else {
					provider = vault.NewGCPProvider(scope, nil)
					validateArgs = []string{"config", "get-value", "project"}
					fetchArgs = func(path string) []string {
						return []string{"secrets", "versions", "access", "latest", "--secret=" + path, "--project=" + scope}
					}
				}
				if err := provider.ValidateConfig(); err != nil {
					t.Fatalf("ValidateConfig: %v; diagnostic=%q", err, variable.CommandDiagnostic(err))
				}
				paths := []string{scope, arguments[len(arguments)-2]}
				for _, path := range paths {
					value, err := provider.Fetch(ctx, path)
					if err != nil || value != stubSecret {
						t.Fatalf("Fetch output=%q error=%v diagnostic=%q", value, err, variable.CommandDiagnostic(err))
					}
				}
				values, err := provider.BulkFetch(ctx, paths)
				if err != nil || !reflect.DeepEqual(values, map[string]string{paths[0]: stubSecret, paths[1]: stubSecret}) {
					t.Fatalf("BulkFetch=%#v error=%v diagnostic=%q", values, err, variable.CommandDiagnostic(err))
				}
				invocations := readProviderInvocations(t, capture)[before:]
				wantArgs := [][]string{validateArgs, fetchArgs(paths[0]), fetchArgs(paths[1]), fetchArgs(paths[0]), fetchArgs(paths[1])}
				if len(invocations) != len(wantArgs) {
					t.Fatalf("provider invocation count=%d, want %d", len(invocations), len(wantArgs))
				}
				for index, invocation := range invocations {
					if !reflect.DeepEqual(invocation.Args, wantArgs[index]) {
						t.Errorf("provider argv[%d]=%#v, want=%#v", index, invocation.Args, wantArgs[index])
					}
				}
				t.Setenv("CURLEW_LAUNCHER_ERROR", "private-stderr-marker")
				value, err := provider.Fetch(ctx, "private-secret-path")
				requireProviderSafeError(t, err, "private-secret-path", scope, program)
				if value != "" || !strings.Contains(err.Error(), "code 42") || variable.CommandDiagnostic(err) != "private-stderr-marker\n" {
					t.Errorf("provider failure output=%q error=%v diagnostic=%q", value, err, variable.CommandDiagnostic(err))
				}
			})
			t.Run("native-exit42-safe-diagnostics", func(t *testing.T) {
				before := len(readProviderInvocations(t, capture))
				t.Setenv("CURLEW_LAUNCHER_ERROR", "private-stderr-marker")
				output, err := variable.ExecuteProgram(ctx, program, []string{"private-native-argument"}, nil)
				requireProviderSafeError(t, err, program, "private-native-argument")
				if output != "" || !strings.Contains(err.Error(), "code 42") || variable.CommandDiagnostic(err) != "private-stderr-marker\n" {
					t.Errorf("native failure output=%q error=%v diagnostic=%q", output, err, variable.CommandDiagnostic(err))
				}
				invocations := readProviderInvocations(t, capture)
				if len(invocations) != before+1 || !reflect.DeepEqual(invocations[before].Args, []string{"private-native-argument"}) {
					t.Fatalf("exit-42 Python invocation=%#v", invocations[before:])
				}
			})
			t.Run("large-aggregate-argv", func(t *testing.T) {
				before := len(readProviderInvocations(t, capture))
				large := make([]string, 96)
				for index := range large {
					large[index] = fmt.Sprintf("%03d-%s", index, strings.Repeat("a b", 16))
				}
				output, err := variable.ExecuteProgram(ctx, program, large, nil)
				if err != nil || output != stubSecret {
					t.Fatalf("large aggregate output=%q error=%v diagnostic=%q", output, err, variable.CommandDiagnostic(err))
				}
				invocations := readProviderInvocations(t, capture)
				if len(invocations) != before+1 || !reflect.DeepEqual(invocations[before].Args, large) {
					t.Fatalf("large aggregate changed argv: %#v", invocations[before:])
				}
				for _, oversized := range [][]string{
					{strings.Repeat("a", 4100), strings.Repeat("b", 4100)},
					{strings.Repeat("\U0001f642", 2100), strings.Repeat("\U0001f642", 2100)},
				} {
					output, err := variable.ExecuteProgram(ctx, program, oversized, nil)
					requireProviderSafeError(t, err, program, oversized[0])
					if output != "" || !strings.Contains(err.Error(), "unsupported batch invocation") {
						t.Errorf("oversized aggregate output=%q error=%v", output, err)
					}
				}
				if got := len(readProviderInvocations(t, capture)); got != before+1 {
					t.Errorf("oversized aggregate launched Python: invocations=%d, want %d", got, before+1)
				}
			})
			if os.Getenv("CURLEW_BATCH_INVOCATION_0") != "parent-transport-value" || os.Getenv("AZ_INSTALLER") != "" {
				t.Error("launcher changed parent environment")
			}
		})
	}
}
