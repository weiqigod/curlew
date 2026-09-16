package variable

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func trimCommandOutput(output string) string {
	for strings.HasSuffix(output, "\n") {
		output = strings.TrimSuffix(output, "\n")
		output = strings.TrimSuffix(output, "\r")
	}
	return output
}
