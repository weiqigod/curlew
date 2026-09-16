package variable

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func buildProgramHelper(t *testing.T) string {
	t.Helper()
	name := "native helper"
	goName := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
		goName += ".exe"
	}
	program := filepath.Join(t.TempDir(), name)
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", goName), "build", "-o", program, "./testdata/programhelper")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build native helper: %v\n%s", err, output)
	}
	return program
}

func helperShellCommand(program, mode string) string {
	quoted := "'" + strings.ReplaceAll(program, "'", "'\\''") + "'"
	if runtime.GOOS == "windows" {
		quoted = "& '" + strings.ReplaceAll(program, "'", "''") + "'"
	}
	return quoted + " " + mode
}

func TestExecuteCommandNativeHelper(t *testing.T) {
	program := buildProgramHelper(t)
	t.Run("output", func(t *testing.T) {
		output, err := ExecuteCommand(context.Background(), helperShellCommand(program, "hello"))
		if err != nil || output != "hello" {
			t.Fatalf("output = %q, error = %v", output, err)
		}
	})
	t.Run("safe_exit_42", func(t *testing.T) {
		output, err := ExecuteCommand(context.Background(), helperShellCommand(program, "exit42"))
		if output != "" || !errors.Is(err, ErrCommandFailed) {
			t.Fatalf("output = %q, error = %v", output, err)
		}
		if !strings.Contains(err.Error(), "code 42") {
			t.Fatalf("missing exit code: %v", err)
		}
		for _, secret := range []string{program, "private-stdout", "private-stderr", "exit42"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("error leaks %q: %v", secret, err)
			}
		}
	})
}