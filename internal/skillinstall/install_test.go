package skillinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

func put(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, path string) string {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}

func TestUpdate(t *testing.T) {
	if _, err := docs.Prose("CLI_SPECIFICATION.md", "can replace files whose current SHA-256 matches the prior"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	opts := Options{Dir: dir, Agent: "codex", Version: "old"}
	root, err := Install(opts)
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(root, "team.md"), "team rules")
	opts.Version = "new"
	if _, err = Install(opts); err == nil {
		t.Fatal("install overwrote older payload without update")
	}
	opts.Update = true
	if _, err = Install(opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, filepath.Join(root, "SKILL.md")), "curlew new") {
		t.Fatal("did not update version")
	}
	if get(t, filepath.Join(root, "team.md")) != "team rules" {
		t.Fatal("lost custom file")
	}
	// Missing managed files are restored.
	if err = os.Remove(filepath.Join(root, "variables.md")); err != nil {
		t.Fatal(err)
	}
	if _, err = Install(opts); err != nil {
		t.Fatal(err)
	}
	_ = get(t, filepath.Join(root, "variables.md"))
}

func TestConflictPreflight(t *testing.T) {
	dir := t.TempDir()
	opts := Options{Dir: dir, Agent: "claude", Version: "old"}
	root, err := Install(opts)
	if err != nil {
		t.Fatal(err)
	}
	before := get(t, filepath.Join(root, "SKILL.md"))
	state := get(t, filepath.Join(root, manifestName))
	put(t, filepath.Join(root, "variables.md"), "local customization")
	opts.Version = "new"
	opts.Update = true
	if _, err = Install(opts); err == nil || !strings.Contains(err.Error(), "variables.md") {
		t.Fatal(err)
	}
	if get(t, filepath.Join(root, "SKILL.md")) != before || get(t, filepath.Join(root, manifestName)) != state {
		t.Fatal("partial conflict writes")
	}
}

func TestUnmanaged(t *testing.T) {
	dir := t.TempDir()
	opts := Options{Dir: dir, Agent: "copilot", Version: "v"}
	root, err := Install(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(root, manifestName)); err != nil {
		t.Fatal(err)
	}
	// Identical legacy files may be adopted; unknown edits cannot.
	if _, err = Install(opts); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(root, manifestName)); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(root, "SKILL.md"), "custom")
	opts.Update = true
	if _, err = Install(opts); err == nil {
		t.Fatal("adopted customized legacy skill")
	}
	if get(t, filepath.Join(root, "SKILL.md")) != "custom" {
		t.Fatal("lost legacy edit")
	}
}

func TestInvalidStateAndPaths(t *testing.T) {
	for _, body := range []string{"{", `{"schema":2,"files":{}}`, `{"schema":1}`, `{"schema":1,"files":{"../escape":"bad"}}`} {
		t.Run(body, func(t *testing.T) {
			dir := t.TempDir()
			root := filepath.Join(dir, ".agents", "skills", "curlew")
			put(t, filepath.Join(root, manifestName), body)
			if _, err := Install(Options{Dir: dir, Agent: "codex", Version: "v", Update: true}); err == nil {
				t.Fatal("accepted invalid state")
			}
			if _, err := os.Stat(filepath.Join(root, "SKILL.md")); !os.IsNotExist(err) {
				t.Fatal("wrote before validating")
			}
		})
	}
	for _, rel := range []string{".agents", ".agents/skills/curlew", ".agents/skills/curlew/SKILL.md", ".agents/skills/curlew/" + manifestName} {
		t.Run(rel, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, rel)
			if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
				t.Fatal(err)
			}
			outside := t.TempDir()
			if err := os.Symlink(outside, dest); err != nil {
				t.Skip(err)
			}
			if _, err := Install(Options{Dir: dir, Agent: "codex", Version: "v"}); err == nil {
				t.Fatal("followed symlink")
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatal("wrote outside target")
			}
		})
	}
	dir := t.TempDir()
	put(t, filepath.Join(dir, ".agents"), "file")
	if _, err := Install(Options{Dir: dir, Agent: "codex"}); err == nil {
		t.Fatal("accepted file as parent")
	}
	if _, err := Install(Options{Dir: t.TempDir(), Agent: "bad"}); err == nil {
		t.Fatal("accepted agent")
	}
	if _, err := Install(Options{Dir: t.TempDir(), Agent: "codex", Update: true}); err == nil {
		t.Fatal("update silently installed missing skill")
	}
}
