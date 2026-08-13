package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The large-dataset guard exits 2.
//
// All three exit-code tables say a tripped safety guard is exit 2, and the
// specification's own CI column reads "Fail — fix the invocation". The guard
// refused correctly and exited 5 — the code reserved for variable resolution —
// because it returned a bare error and every bare error from a run is a 5.
//
// The distinction is the whole reason the codes exist. A pipeline branching on
// 5 retries with different variables; the fix here is to pass a flag.
//
// Found by executing the exit-code tables rather than reading them.

func writeLargeDataset(t *testing.T) (collection string) {
	t.Helper()
	dir := t.TempDir()

	var rows strings.Builder
	rows.WriteString("id\n")
	for i := 0; i < 10_001; i++ {
		fmt.Fprintf(&rows, "%d\n", i)
	}
	data := filepath.Join(dir, "big.csv")
	if err := os.WriteFile(data, []byte(rows.String()), 0o600); err != nil {
		t.Fatalf("write data: %v", err)
	}

	collection = filepath.Join(dir, "guarded.yaml")
	body := fmt.Sprintf(
		"name: guarded\nrequests:\n  - name: r\n    data_driven:\n      source: %q\n"+
			"    request:\n      method: GET\n      url: \"http://127.0.0.1:1/{{id}}\"\n", data)
	if err := os.WriteFile(collection, []byte(body), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}
	return collection
}

func TestLargeDatasetGuard_exitsTwo(t *testing.T) {
	bin := buildBinary(t)
	collection := writeLargeDataset(t)

	cmd := exec.Command(bin, "run", collection)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, _ := cmd.CombinedOutput()
	code := cmd.ProcessState.ExitCode()

	if code != 2 {
		t.Errorf("large-dataset guard exited %d, want 2 (a tripped safety guard):\n%s", code, out)
	}
	// The advice must survive the classification: a guard that exits correctly
	// and stops saying how to proceed is no better for the person who hit it.
	if !strings.Contains(string(out), "--confirm-large-dataset") {
		t.Errorf("guard message no longer names --confirm-large-dataset:\n%s", out)
	}
}

// And the opt-in still works: confirming must get past the guard rather than
// exit 2 for a different reason.
func TestLargeDatasetGuard_confirmingGetsPast(t *testing.T) {
	bin := buildBinary(t)
	collection := writeLargeDataset(t)

	cmd := exec.Command(bin, "run", collection, "--confirm-large-dataset", "--dry-run")
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, _ := cmd.CombinedOutput()

	if strings.Contains(string(out), "--confirm-large-dataset to proceed") {
		t.Errorf("--confirm-large-dataset did not get past the guard:\n%s", out)
	}
}
