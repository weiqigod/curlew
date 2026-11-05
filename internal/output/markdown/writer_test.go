package markdown

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestMarkdown_EnsureReportDir verifies that EnsureReportDir creates nested
// directories with mode 0755.
func TestMarkdown_EnsureReportDir(t *testing.T) {
	base := t.TempDir()

	t.Run("creates_directory", func(t *testing.T) {
		target := filepath.Join(base, "reports", "run1")
		if err := EnsureReportDir(target); err != nil {
			t.Fatalf("EnsureReportDir: %v", err)
		}
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if !info.IsDir() {
			t.Error("expected directory to be created")
		}
	})

	t.Run("subpath_variadic", func(t *testing.T) {
		target := filepath.Join(base, "multi")
		if err := EnsureReportDir(target, "sub1", "sub2"); err != nil {
			t.Fatalf("EnsureReportDir with subpath: %v", err)
		}
		// The subpath is appended when len(subpath) > 0.
		full := filepath.Join(target, "sub1", "sub2")
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", full)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		target := filepath.Join(base, "idempotent")
		if err := EnsureReportDir(target); err != nil {
			t.Fatalf("first call: %v", err)
		}
		if err := EnsureReportDir(target); err != nil {
			t.Fatalf("second call: %v", err)
		}
	})
}

// TestMarkdown_Concurrent verifies that two goroutines writing the same slug
// to the same --report directory via WriteReport produce no corrupt sentinels.
func TestMarkdown_Concurrent(t *testing.T) {
	dir := t.TempDir()
	report := makePassReport()

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = WriteReport(report, dir, WriteOptions{})
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: WriteReport error: %v", i, err)
		}
	}

	// The resulting file must have exactly one BEGIN and one END.
	got, err := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(got)

	countOccurrences := func(s, sub string) int {
		return strings.Count(s, sub)
	}
	beginCount := countOccurrences(content, SentinelBeginPrefix)
	endCount := countOccurrences(content, SentinelEndPrefix)

	if beginCount != 1 {
		t.Errorf("expected 1 BEGIN sentinel, got %d:\n%s", beginCount, content)
	}
	if endCount != 1 {
		t.Errorf("expected 1 END sentinel, got %d:\n%s", endCount, content)
	}

	// Verify the file is byte-complete (parseSentinels should return a valid location).
	loc, parseErr := parseSentinels(got)
	if parseErr != nil {
		t.Errorf("parseSentinels after concurrent write: %v", parseErr)
	}
	if loc == nil {
		t.Error("no sentinels found in concurrently written file")
	}
}

// TestMarkdown_WriteAtomic verifies that writeAtomic creates the file with
// the correct content.
func TestMarkdown_WriteAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	data := []byte("hello world\n")

	if err := writeAtomic(path, data); err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, data) {
		t.Errorf("writeAtomic content mismatch: got %q, want %q", got, data)
	}
}

// TestMarkdown_WriteAtomic_CreateTempError verifies that writeAtomic returns
// an error when the directory does not exist (os.CreateTemp fails).
func TestMarkdown_WriteAtomic_CreateTempError(t *testing.T) {
	// Use a path whose parent directory does not exist.
	nonExistent := filepath.Join(t.TempDir(), "no-such-dir", "file.md")
	err := writeAtomic(nonExistent, []byte("data"))
	if err == nil {
		t.Fatal("expected error when directory does not exist, got nil")
	}
	if !strings.Contains(err.Error(), "create temp file") {
		t.Errorf("expected 'create temp file' in error, got: %v", err)
	}
}

// TestMarkdown_WriteAtomic_RenameError verifies that writeAtomic returns an
// error and cleans up the temp file when the destination is a directory.
func TestMarkdown_WriteAtomic_RenameError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.md")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("create destination directory: %v", err)
	}

	err := writeAtomic(path, []byte("data"))
	if err == nil {
		t.Fatal("expected error when destination is a directory, got nil")
	}
	if !strings.Contains(err.Error(), "rename temp file") {
		t.Errorf("expected 'rename temp file' in error, got: %v", err)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("read output directory: %v", readErr)
	}
	if len(entries) != 1 || entries[0].Name() != "target.md" || !entries[0].IsDir() {
		t.Errorf("temporary file was not cleaned up: %v", entries)
	}
}
