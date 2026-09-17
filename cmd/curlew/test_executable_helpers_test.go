package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testExecutableName(goos, base string) string {
	if goos == "windows" && !strings.EqualFold(filepath.Ext(base), ".exe") {
		return base + ".exe"
	}
	return base
}

func testExecutablePath(dir, base string) string {
	return filepath.Join(dir, testExecutableName(runtime.GOOS, base))
}

func testBash(t *testing.T) string {
	t.Helper()
	if path, err := exec.LookPath("bash"); err == nil {
		return path
	}
	if runtime.GOOS == "windows" {
		for _, path := range []string{
			filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Git", "usr", "bin", "bash.exe"),
		} {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path
			}
		}
	}
	t.Fatal("bash is required for this documented POSIX recipe")
	return ""
}

func testPythonCommand(t *testing.T, ctx context.Context, args ...string) *exec.Cmd {
	t.Helper()
	path, prefix := testPython(t)
	return exec.CommandContext(ctx, path, append(prefix, args...)...)
}

func testPython(t *testing.T) (string, []string) {
	t.Helper()
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	if path, err := exec.LookPath("py"); err == nil {
		return path, []string{"-3"}
	}
	t.Fatal("Python 3 is required for this local fixture")
	return "", nil
}

func testPythonShim(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("python3"); err == nil {
		return ""
	}
	python, prefix := testPython(t)
	dir := t.TempDir()
	shim := filepath.Join(dir, "python3")
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(filepath.ToSlash(value), "'", "'\\''") + "'"
	}
	command := "exec " + quote(python)
	for _, arg := range prefix {
		command += " " + quote(arg)
	}
	command += " \"$@\""
	if err := os.WriteFile(shim, []byte("#!/usr/bin/env bash\n"+command+"\n"), 0o755); err != nil {
		t.Fatalf("write python3 test shim: %v", err)
	}
	return dir
}
