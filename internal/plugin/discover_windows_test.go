//go:build windows

package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

func TestDiscoverWindowsExecutablePolicy(t *testing.T) {
	if _, err := docs.Prose("CLI_SPECIFICATION.md", "Windows plugin candidates must be regular `.exe` files"); err != nil {
		t.Fatal(err)
	}
	write := func(t *testing.T, dir, name string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	t.Run("supported executable suffixes", func(t *testing.T) {
		for _, name := range []string{"plugin.exe", "plugin.EXE"} {
			t.Run(name, func(t *testing.T) {
				path := write(t, t.TempDir(), name)
				candidates, errs := discover(path)
				if len(errs) != 0 || len(candidates) != 1 || candidates[0] != path {
					t.Fatalf("candidates=%v errs=%+v", candidates, errs)
				}
			})
		}
	})

	t.Run("unsupported launcher suffixes", func(t *testing.T) {
		for _, name := range []string{"plugin", "plugin.cmd", "plugin.bat", "plugin.ps1", "plugin.txt"} {
			t.Run(strings.ReplaceAll(name, ".", "_"), func(t *testing.T) {
				path := write(t, t.TempDir(), name)
				candidates, errs := discover(path)
				if len(candidates) != 0 || len(errs) != 1 || !errs[0].Fatal {
					t.Fatalf("candidates=%v errs=%+v", candidates, errs)
				}
			})
		}
	})

	t.Run("directory silently skips unsupported files", func(t *testing.T) {
		dir := t.TempDir()
		accepted := write(t, dir, "curlew plugin å.exe")
		_ = write(t, dir, "plugin.cmd")
		_ = write(t, dir, "README.txt")
		candidates, errs := discover(dir)
		if len(errs) != 0 || len(candidates) != 1 || candidates[0] != accepted {
			t.Fatalf("candidates=%v errs=%+v", candidates, errs)
		}
	})
}
