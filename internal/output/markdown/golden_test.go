package markdown

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// goldenDir returns the path to testdata/golden/ relative to this test file.
func goldenDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "golden")
}

// compareOrUpdateGolden compares got to the golden file at testdata/golden/name.
// When the golden file does not exist it is created automatically.
// Set env var UPDATE_GOLDEN=1 to overwrite all goldens.
func compareOrUpdateGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	dir := goldenDir(t)
	path := filepath.Join(dir, name)

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", dir, err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
		t.Logf("updated golden %s", name)
		return
	}

	existing, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// Auto-create on first run.
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", dir, err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
		t.Logf("created golden %s", name)
		return
	}
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}

	if !bytes.Equal(existing, got) {
		t.Errorf("golden %s mismatch:\n--- want ---\n%s\n--- got ---\n%s", name, existing, got)
	}
}
