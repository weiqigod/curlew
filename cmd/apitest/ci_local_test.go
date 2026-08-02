package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCiLocalDownIdempotent asserts that ./scripts/ci-local.sh --down succeeds
// even when no stack is running, and again on a second invocation.
func TestCiLocalDownIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration script test; skipped under -short")
	}
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	script := filepath.Join(repoRoot, "scripts", "ci-local.sh")

	for i := 0; i < 2; i++ {
		cmd := exec.Command("bash", script, "--down")
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("iteration %d: --down failed: %v\noutput: %s", i, err, out)
		}
	}
}
