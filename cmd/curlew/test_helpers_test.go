package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTestExecutableName(t *testing.T) {
	tests := []struct {
		name string
		goos string
		base string
		want string
	}{
		{name: "windows adds suffix", goos: "windows", base: "curlew", want: "curlew.exe"},
		{name: "windows preserves suffix", goos: "windows", base: "curlew.exe", want: "curlew.exe"},
		{name: "windows suffix is case insensitive", goos: "windows", base: "curlew.EXE", want: "curlew.EXE"},
		{name: "linux unchanged", goos: "linux", base: "curlew", want: "curlew"},
		{name: "darwin unchanged", goos: "darwin", base: "curlew", want: "curlew"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := testExecutableName(test.goos, test.base); got != test.want {
				t.Fatalf("testExecutableName(%q, %q) = %q, want %q", test.goos, test.base, got, test.want)
			}
		})
	}
}

func TestBuildBinaryFromSpacedUnicodeOutputPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "curlew build å")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("create output directory: %v", err)
	}
	binary := testExecutablePath(dir, "curlew")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %q: %v\n%s", binary, err, output)
	}
	run := exec.Command(binary, "--version")
	if output, err := run.CombinedOutput(); err != nil {
		t.Fatalf("run %q: %v\n%s", binary, err, output)
	}
}
