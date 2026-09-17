package main

import (
	"path/filepath"
	"runtime"
	"strings"
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