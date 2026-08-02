//go:build !short

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestStandalone_HelpExits0WithMetadata builds the plugin binary and verifies
// that invoking it with --help exits 0 and prints the plugin's name, version,
// and supported hooks.
func TestStandalone_HelpExits0WithMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("skip in -short mode")
	}
	bin := filepath.Join(t.TempDir(), "datadog-metrics")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help: %v\n%s", err, out)
	}
	s := string(out)
	for _, want := range []string{"datadog-metrics", "0.1.0", "on_response"} {
		if !strings.Contains(s, want) {
			t.Errorf("help output missing %q:\n%s", want, s)
		}
	}
}
