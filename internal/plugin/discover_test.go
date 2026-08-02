package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscover(t *testing.T) {
	// Helper: make a temp file with the given mode.
	mk := func(t *testing.T, dir, name string, mode os.FileMode) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), mode); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("empty env returns nothing", func(t *testing.T) {
		cands, errs := discover("")
		if len(cands) != 0 || len(errs) != 0 {
			t.Errorf("got cands=%v errs=%v", cands, errs)
		}
	})

	t.Run("single executable file", func(t *testing.T) {
		dir := t.TempDir()
		p := mk(t, dir, "plug", 0o755)
		cands, errs := discover(p)
		if len(cands) != 1 || cands[0] != p {
			t.Errorf("cands=%v", cands)
		}
		if len(errs) != 0 {
			t.Errorf("errs=%v", errs)
		}
	})

	t.Run("non-executable file produces fatal error", func(t *testing.T) {
		dir := t.TempDir()
		p := mk(t, dir, "plug", 0o644)
		cands, errs := discover(p)
		if len(cands) != 0 || len(errs) != 1 || !errs[0].Fatal ||
			!strings.Contains(errs[0].Message, "is not executable") {
			t.Errorf("cands=%v errs=%+v", cands, errs)
		}
	})

	t.Run("missing path produces fatal error", func(t *testing.T) {
		_, errs := discover("/does/not/exist/xyz")
		if len(errs) != 1 || !errs[0].Fatal ||
			!strings.Contains(errs[0].Message, "not found") {
			t.Errorf("errs=%+v", errs)
		}
	})

	t.Run("directory expands to sorted executables", func(t *testing.T) {
		dir := t.TempDir()
		_ = mk(t, dir, "zplug", 0o755)
		_ = mk(t, dir, "aplug", 0o755)
		_ = mk(t, dir, "readme.txt", 0o644) // ignored: not executable
		cands, errs := discover(dir)
		if len(cands) != 2 {
			t.Fatalf("cands=%v", cands)
		}
		if filepath.Base(cands[0]) != "aplug" || filepath.Base(cands[1]) != "zplug" {
			t.Errorf("not sorted: %v", cands)
		}
		if len(errs) != 0 {
			t.Errorf("errs=%+v", errs)
		}
	})

	t.Run("directory skips non-executable files silently", func(t *testing.T) {
		// Non-executable files inside a directory are silently skipped; only
		// executable files become plugin candidates. The user did not explicitly
		// name the non-executable file, so no error is emitted.
		dir := t.TempDir()
		_ = mk(t, dir, "good", 0o755)
		_ = mk(t, dir, "bad", 0o644)
		cands, errs := discover(dir)
		if len(cands) != 1 {
			t.Errorf("cands=%v", cands)
		}
		if len(errs) != 0 {
			t.Errorf("unexpected errs=%+v", errs)
		}
	})

	t.Run("list separator splits entries", func(t *testing.T) {
		dir := t.TempDir()
		a := mk(t, dir, "a", 0o755)
		b := mk(t, dir, "b", 0o755)
		env := a + string(filepath.ListSeparator) + b
		cands, _ := discover(env)
		if len(cands) != 2 {
			t.Errorf("cands=%v", cands)
		}
	})
}
