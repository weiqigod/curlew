package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// README.md's "## Install" section used to offer exactly one command
// (`go install github.com/weiqigod/curlew/cmd/curlew@latest`) and nothing had
// ever run it. It does not work as written: the module is private, so the
// public checksum database cannot verify it. This file extracts every fenced
// command block under "## Install" and holds the section's prose to two
// source-of-truth files -- .goreleaser.yaml (archive name/extension shape)
// and go.mod (module path) -- rather than trusting hand-typed strings to stay
// in sync with either.
//
// The extractor and its hermetic shape test live here, untagged, because
// they cost nothing and touch neither the network nor a subprocess.
// TestReadme_install_commands_execute (cmd/curlew/readme_install_exec_test.go)
// actually runs the blocks this file extracts; it sits behind
// //go:build readme_install because it costs ~14s and needs network, gh and
// git credentials -- see that file's own doc comment for the full reasoning
// (M25-003 D1).

// readmeBlock is one fenced code block found under a markdown heading.
type readmeBlock struct {
	lang string // the fence info string, verbatim ("bash", "text", "")
	body string // the block's contents, without the fence lines
	line int    // 1-based line of the opening fence, absolute within the document
}

// readmeSectionBounds returns the [start, end) line indices, into lines, of
// the body of the "## <heading>" section: from the line after the heading
// through the line before the next top-level "## " heading (or end of
// document). ok is false when the heading itself is not found. A "### "
// subheading does not end the section -- only another "## " does.
func readmeSectionBounds(lines []string, heading string) (start, end int, ok bool) {
	want := "## " + heading
	for i, line := range lines {
		if strings.TrimRight(line, " \t") != want {
			continue
		}
		start = i + 1
		end = len(lines)
		for j := start; j < len(lines); j++ {
			if strings.HasPrefix(lines[j], "## ") {
				end = j
				break
			}
		}
		return start, end, true
	}
	return 0, 0, false
}

// readmeSectionText returns the raw text of the "## <heading>" section of
// doc, used for prose assertions. Empty when the heading is not found.
func readmeSectionText(doc, heading string) string {
	// STUB (RED phase): always empty, so every prose assertion against it
	// fails until this is implemented for real.
	_ = doc
	_ = heading
	return ""
}

// readmeSectionBlocks returns every fenced code block that appears under the
// "## <heading>" section of doc, stopping at the next top-level "## "
// heading. Inline `code spans` are never returned, only fenced ``` blocks.
func readmeSectionBlocks(doc, heading string) []readmeBlock {
	// STUB (RED phase): always empty, so TestReadme_install_blocks_extraction
	// fails on every non-empty-want case until this is implemented for real.
	_ = doc
	_ = heading
	return nil
}

// readmeRepoRoot resolves the repository root from a test in cmd/curlew, the
// same technique release_artifacts_test.go's releaseFindRepoRoot uses.
func readmeRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// readmeReadFileOrFatal reads path or fails the test. Shared by this file and
// the //go:build readme_install exec test, which is why it lives in the
// untagged file (M25-003 D1: the untagged file is compiled into the tagged
// build too).
func readmeReadFileOrFatal(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

// readmeModuleRE matches go.mod's `module <path>` directive.
var readmeModuleRE = regexp.MustCompile(`(?m)^module\s+(\S+)\s*$`)

// readmeModulePath returns the module path declared in go.mod, so every
// assertion against it fails loudly if the README's install lines and go.mod
// ever disagree, instead of silently trusting a copy typed into markdown.
func readmeModulePath(t *testing.T, repoRoot string) string {
	t.Helper()
	raw := readmeReadFileOrFatal(t, filepath.Join(repoRoot, "go.mod"))
	m := readmeModuleRE.FindStringSubmatch(raw)
	if m == nil {
		t.Fatalf("go.mod: no `module <path>` directive found")
	}
	return m[1]
}

// readmeArchiveConfig is the subset of .goreleaser.yaml this test holds the
// README to. Deliberately separate from releaseConfig in
// release_artifacts_test.go, which is behind a different build tag
// (release_artifacts) and parses archives[].files only -- not name_template,
// which is what this file needs.
type readmeArchiveConfig struct {
	ProjectName string `yaml:"project_name"`
	Archives    []struct {
		NameTemplate    string   `yaml:"name_template"`
		Formats         []string `yaml:"formats"`
		FormatOverrides []struct {
			GOOS    string   `yaml:"goos"`
			Formats []string `yaml:"formats"`
		} `yaml:"format_overrides"`
	} `yaml:"archives"`
}

// loadReadmeArchiveConfig parses .goreleaser.yaml and fails loudly on the
// shapes that would make the rest of this test's expectations meaningless:
// no project_name, no archives[] entry, no name_template, no formats.
func loadReadmeArchiveConfig(path string) (readmeArchiveConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return readmeArchiveConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg readmeArchiveConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return readmeArchiveConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.ProjectName == "" {
		return readmeArchiveConfig{}, fmt.Errorf("%s: project_name is empty", path)
	}
	if len(cfg.Archives) == 0 {
		return readmeArchiveConfig{}, fmt.Errorf("%s: no archives[] entries", path)
	}
	if cfg.Archives[0].NameTemplate == "" {
		return readmeArchiveConfig{}, fmt.Errorf("%s: archives[0].name_template is empty", path)
	}
	if len(cfg.Archives[0].Formats) == 0 {
		return readmeArchiveConfig{}, fmt.Errorf("%s: archives[0].formats is empty", path)
	}
	return cfg, nil
}

// readmeTemplateActionRE matches one goreleaser {{ .Field }} action.
var readmeTemplateActionRE = regexp.MustCompile(`\{\{\s*\.(\w+)\s*\}\}`)

// renderNameTemplate substitutes the documentation placeholders into tmpl and
// reports any {{ ... }} action it did not recognise in unknown, so a
// name_template that grows a new action (e.g. {{ .Tag }}) fails this test
// loudly instead of silently comparing against a half-rendered string.
func renderNameTemplate(tmpl, projectName string) (rendered string, unknown []string) {
	// STUB (RED phase): return the template completely unrendered, so every
	// rendered-name assertion fails until this is implemented for real.
	_ = projectName
	return tmpl, nil
}

func TestReadme_install_blocks_extraction(t *testing.T) {
	tests := []struct {
		name       string
		doc        string
		heading    string
		wantLangs  []string
		wantBodies []string
	}{
		{"single bash block", "## Install\n\n```bash\nls\n```\n", "Install",
			[]string{"bash"}, []string{"ls"}},
		{"stops at the next ## heading", "## Install\n\n```bash\na\n```\n\n## Next\n\n```bash\nb\n```\n",
			"Install", []string{"bash"}, []string{"a"}},
		{"### subheadings do not end the section", "## Install\n\n### One\n\n```bash\na\n```\n\n### Two\n\n```bash\nb\n```\n",
			"Install", []string{"bash", "bash"}, []string{"a", "b"}},
		{"inline code spans are not blocks", "## Install\n\nrun `curlew run x` first\n\n```bash\na\n```\n",
			"Install", []string{"bash"}, []string{"a"}},
		{"fence language is preserved", "## Install\n\n```text\nnot a command\n```\n",
			"Install", []string{"text"}, []string{"not a command"}},
		{"multi-line block keeps its lines", "## Install\n\n```bash\ncd x\nmake\n```\n",
			"Install", []string{"bash"}, []string{"cd x\nmake"}},
		{"edge case - missing heading yields nothing", "## Other\n\n```bash\na\n```\n",
			"Install", nil, nil},
		{"edge case - heading with no blocks", "## Install\n\njust prose\n\n## Next\n",
			"Install", nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			blocks := readmeSectionBlocks(tc.doc, tc.heading)
			var gotLangs, gotBodies []string
			for _, b := range blocks {
				gotLangs = append(gotLangs, b.lang)
				gotBodies = append(gotBodies, b.body)
			}
			if !reflect.DeepEqual(gotLangs, tc.wantLangs) {
				t.Errorf("langs = %#v, want %#v", gotLangs, tc.wantLangs)
			}
			if !reflect.DeepEqual(gotBodies, tc.wantBodies) {
				t.Errorf("bodies = %#v, want %#v", gotBodies, tc.wantBodies)
			}
		})
	}
}

// TestReadme_documents_a_binary_download holds README.md's "## Install"
// section to two source-of-truth files rather than to hand-typed strings:
// .goreleaser.yaml for the archive name/extension shape, go.mod for the
// module path. Each sub-assertion fails independently so a broken check
// names exactly what drifted.
func TestReadme_documents_a_binary_download(t *testing.T) {
	repoRoot := readmeRepoRoot(t)
	doc := readmeReadFileOrFatal(t, filepath.Join(repoRoot, "README.md"))
	section := readmeSectionText(doc, "Install")

	t.Run("install section exists and is non-empty", func(t *testing.T) {
		if strings.TrimSpace(section) == "" {
			t.Fatal("README.md has no non-empty '## Install' section -- the extraction is broken, not the README")
		}
	})

	cfg, err := loadReadmeArchiveConfig(filepath.Join(repoRoot, ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("load .goreleaser.yaml: %v", err)
	}
	archive := cfg.Archives[0]

	rendered, unknown := renderNameTemplate(archive.NameTemplate, cfg.ProjectName)
	t.Run("name_template renders without unknown actions", func(t *testing.T) {
		if len(unknown) != 0 {
			t.Fatalf(".goreleaser.yaml archives[0].name_template %q uses action(s) %v that renderNameTemplate does not know how to substitute -- update its known-actions map", archive.NameTemplate, unknown)
		}
	})

	wantName := rendered + "." + archive.Formats[0]
	t.Run("documented archive name matches name_template", func(t *testing.T) {
		if !strings.Contains(section, wantName) {
			t.Errorf("Install section does not mention %q, rendered from .goreleaser.yaml's name_template %q", wantName, archive.NameTemplate)
		}
	})

	t.Run("documented extension matches archives[0].formats", func(t *testing.T) {
		ext := "." + archive.Formats[0]
		if !strings.Contains(section, ext) {
			t.Errorf("Install section does not mention the %q extension declared in .goreleaser.yaml archives[0].formats", ext)
		}
	})

	t.Run("windows override is documented", func(t *testing.T) {
		var windowsFormat string
		for _, ov := range archive.FormatOverrides {
			if ov.GOOS == "windows" && len(ov.Formats) > 0 {
				windowsFormat = ov.Formats[0]
			}
		}
		if windowsFormat == "" {
			t.Fatalf(".goreleaser.yaml has no windows entry under archives[0].format_overrides -- cannot check the README's windows claim against it")
		}
		ext := "." + windowsFormat
		if !strings.Contains(section, ext) {
			t.Errorf("Install section does not mention the windows override extension %q declared in .goreleaser.yaml archives[0].format_overrides", ext)
		}
	})

	modulePath := readmeModulePath(t, repoRoot)
	ownerRepo := strings.TrimPrefix(modulePath, "github.com/")

	t.Run("documented download URL matches the go.mod module path", func(t *testing.T) {
		want := "https://github.com/" + ownerRepo + "/releases/download/v"
		if !strings.Contains(section, want) {
			t.Errorf("Install section does not contain a release download URL starting with %q, derived from go.mod's module path %q", want, modulePath)
		}
	})

	t.Run("go install line matches the go.mod module path", func(t *testing.T) {
		want := modulePath + "/cmd/curlew@latest"
		if !strings.Contains(section, want) {
			t.Errorf("Install section does not contain %q, derived from go.mod's module path %q -- the go install line is stale", want, modulePath)
		}
	})
}
