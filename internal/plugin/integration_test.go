//go:build !short

package plugin

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// TestHost_Load_WithRealFixture builds the hello-plugin fixture and tests it
// against the real execSpawner. Skipped in -short mode or when go is not on PATH.
func TestHost_Load_WithRealFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("skip in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	tmp := t.TempDir()
	binPath := filepath.Join(tmp, discoveryExecutableName("hello-plugin"))

	// Build the fixture relative to this test file.
	cmd := exec.Command("go", "build", "-o", binPath,
		"../../testdata/plugins/hello-plugin")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build fixture: %v", err)
	}

	host := NewHost(io.Discard)
	loaded, errs, err := host.Load(context.Background(), binPath)
	if err != nil {
		t.Fatalf("Host.Load: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("unexpected loadErrs: %+v", errs)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 loaded plugin, got %d: %+v", len(loaded), loaded)
	}
	p := loaded[0]
	if p.Name != "hello-plugin" {
		t.Errorf("name: got %q, want %q", p.Name, "hello-plugin")
	}
	if p.Version != "0.1.0" {
		t.Errorf("version: got %q, want %q", p.Version, "0.1.0")
	}
	if !reflect.DeepEqual(p.Hooks, []string{"on_request", "on_response"}) {
		t.Errorf("hooks: got %v, want [on_request, on_response]", p.Hooks)
	}
	if p.Path != binPath {
		t.Errorf("path: got %q, want %q", p.Path, binPath)
	}
}
