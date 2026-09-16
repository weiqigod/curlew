package variable

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
)

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	script := "$ErrorActionPreference = 'Stop'; $ProgressPreference = 'SilentlyContinue'; $global:LASTEXITCODE = 0; " +
		"[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false); " +
		"$OutputEncoding = [Console]::OutputEncoding; try { & {\n" + command +
		"\n}; exit $LASTEXITCODE } catch { [Console]::Error.WriteLine($_.Exception.Message); exit 1 }"
	units := utf16.Encode([]rune(script))
	encoded := make([]byte, len(units)*2)
	for index, unit := range units {
		binary.LittleEndian.PutUint16(encoded[index*2:], unit)
	}
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	return exec.CommandContext(ctx, shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
}

func programCommand(ctx context.Context, program string, args, env []string) (*exec.Cmd, error) {
	resolved, err := exec.LookPath(program)
	if err != nil {
		return nil, err
	}
	extension := filepath.Ext(resolved)
	if !strings.EqualFold(extension, ".cmd") && !strings.EqualFold(extension, ".bat") {
		cmd := exec.CommandContext(ctx, resolved, args...)
		cmd.Env = env
		return cmd, nil
	}
	if strings.ContainsAny(resolved, "\"\r\n") {
		return nil, &commandFailure{reason: "unsupported batch program name"}
	}
	invocation := "\"" + resolved + "\""
	for _, arg := range args {
		if strings.ContainsAny(arg, "\"\r\n") {
			return nil, &commandFailure{reason: "unsupported batch argument: double quote or CR/LF"}
		}
		trailingSlashes := len(arg) - len(strings.TrimRight(arg, "\\"))
		invocation += " \"" + arg + strings.Repeat("\\", trailingSlashes) + "\""
	}
	keys := make(map[string]bool, len(env))
	for _, entry := range env {
		if len(utf16.Encode([]rune(entry))) > 8000 {
			return nil, &commandFailure{reason: "unsupported batch environment entry: exceeds 8000 UTF-16 units"}
		}
		name, _, _ := strings.Cut(entry, "=")
		keys[programEnvironmentKey(name)] = true
	}
	transport := "CURLEW_BATCH_INVOCATION_0"
	for index := 1; keys[transport]; index++ {
		transport = fmt.Sprintf("CURLEW_BATCH_INVOCATION_%d", index)
	}
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if len(utf16.Encode([]rune(shell+invocation+transport)))+32 > 8000 {
		return nil, &commandFailure{reason: "unsupported batch invocation: exceeds 8000 UTF-16 units"}
	}
	cmd := exec.CommandContext(ctx, shell)
	cmd.Env = append(env, transport+"="+invocation)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "\"" + shell + "\" /d /v:off /s /c \"%" + transport + "%\""}
	return cmd, nil
}

func programEnvironmentKey(name string) string {
	return strings.ToUpper(name)
}

func trimCommandOutput(output string) string {
	for strings.HasSuffix(output, "\n") {
		output = strings.TrimSuffix(output, "\n")
		output = strings.TrimSuffix(output, "\r")
	}
	return output
}
