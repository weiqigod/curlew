package scaffold

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestInit_OutputAllFormats verifies that each supported output format
// produces a scaffold whose curlew.yaml contains the correct output: block.
func TestInit_OutputAllFormats(t *testing.T) {
	cases := []struct {
		format   string
		wantBody string // exact substring expected in curlew.yaml
	}{
		{"terminal", "format: terminal"},
		{"json", "format: json\n  report: results.json"},
		{"tap", "format: tap"},
		{"junit", "format: junit\n  report: results.xml"},
		{"html", "format: html\n  report: report.html"},
		{"markdown", "format: markdown\n  report: responses/"},
	}
	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			dir := t.TempDir()
			err := Init(Options{Dir: dir, OutputFormat: tc.format})
			if err != nil {
				t.Fatalf("Init(format=%s): %v", tc.format, err)
			}
			body, err := os.ReadFile(filepath.Join(dir, "curlew.yaml"))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !strings.Contains(string(body), tc.wantBody) {
				t.Fatalf("curlew.yaml missing %q\ngot:\n%s", tc.wantBody, body)
			}
		})
	}
}

// TestInit_OutputMarkdownFlag is the targeted observable: --output markdown
// produces the exact block in the task YAML's first observable step.
func TestInit_OutputMarkdownFlag(t *testing.T) {
	dir := t.TempDir()
	if err := Init(Options{Dir: dir, OutputFormat: "markdown"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "curlew.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := "output:\n  format: markdown\n  report: responses/"
	if !strings.Contains(string(body), want) {
		t.Fatalf("want %q in curlew.yaml; got:\n%s", want, body)
	}
}

// TestInit_DefaultUnchanged verifies bare init (no OutputFormat) writes the
// pre-M9 byte-for-byte scaffold.
func TestInit_DefaultUnchanged(t *testing.T) {
	dir := t.TempDir()
	if err := Init(Options{Dir: dir, ProjectName: "demo"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "curlew.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := "project_name: \"demo\"\n" +
		"variables:\n" +
		"  base_url: \"https://httpbin.org\"\n" +
		"output:\n" +
		"  format: terminal\n" +
		"  verbosity: normal\n"
	if string(got) != want {
		t.Fatalf("bare-init curlew.yaml diverged from M8-003 baseline\nwant:\n%s\ngot:\n%s", want, got)
	}
}

// TestInit_DevEnvironmentDoesNotShadowProject guards the first-run experience:
// environment variables take precedence over curlew.yaml's variables: block,
// so an active base_url in the scaffolded dev.yaml silently shadows the
// project-level value. The scaffold must ship the override commented out,
// with the precedence rule explained, while remaining a valid environment
// file that yields zero active variables.
func TestInit_DevEnvironmentDoesNotShadowProject(t *testing.T) {
	dir := t.TempDir()
	if err := Init(Options{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "environments", "dev.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "base_url:") {
			t.Errorf("scaffolded dev.yaml has an active base_url override (shadows curlew.yaml): %q", line)
		}
	}
	if !strings.Contains(string(body), "# base_url:") && !strings.Contains(string(body), "#   base_url:") {
		t.Errorf("dev.yaml should keep a commented-out base_url example:\n%s", body)
	}
	if !strings.Contains(string(body), "precedence") {
		t.Errorf("dev.yaml should explain that environment values take precedence:\n%s", body)
	}
	var ef struct {
		Variables map[string]any `yaml:"variables"`
	}
	if err := yaml.Unmarshal(body, &ef); err != nil {
		t.Fatalf("scaffolded dev.yaml is not valid YAML: %v", err)
	}
	if len(ef.Variables) != 0 {
		t.Errorf("scaffolded dev.yaml should define zero active variables, got %v", ef.Variables)
	}
}

// TestOutputBlock_DefaultFallback exercises the defensive default branch in
// outputBlock: an unrecognised format string (which callers should never pass
// because validation happens at the CLI layer) falls back to the terminal block
// rather than returning empty or panicking.
func TestOutputBlock_DefaultFallback(t *testing.T) {
	got := outputBlock("unrecognised-format", false)
	want := "output:\n  format: terminal\n  verbosity: normal\n"
	if got != want {
		t.Fatalf("outputBlock(unrecognised) = %q, want %q", got, want)
	}
}

func TestInit(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T, dir string)
		opts         func(dir string) Options
		wantErr      error
		wantFiles    []string
		checkContent map[string]string // relative file path -> required substring
	}{
		{
			name: "creates all files in empty directory",
			wantFiles: []string{
				"curlew.yaml",
				".gitignore",
				".env.example",
				"environments/dev.yaml",
				"collections/sample.yaml",
			},
		},
		{
			name: "curlew.yaml contains project name from directory basename",
			checkContent: map[string]string{
				"curlew.yaml": "project_name:",
			},
		},
		{
			name: "curlew.yaml uses custom project name when provided",
			opts: func(dir string) Options { return Options{Dir: dir, ProjectName: "MyAPI"} },
			checkContent: map[string]string{
				// project_name is quoted to prevent YAML from interpreting numeric names as numbers
				"curlew.yaml": `project_name: "MyAPI"`,
			},
		},
		{
			name: "curlew.yaml contains active output block",
			checkContent: map[string]string{
				"curlew.yaml": "output:\n  format: terminal\n  verbosity: normal",
			},
		},
		{
			name: "gitignore contains .env entry",
			checkContent: map[string]string{
				".gitignore": ".env",
			},
		},
		{
			name: "env.example documents variables without real values",
			checkContent: map[string]string{
				".env.example": "NEVER commit .env",
			},
		},
		{
			name: "sample collection is valid yaml with base_url variable",
			checkContent: map[string]string{
				"collections/sample.yaml": "{{base_url}}",
			},
		},
		{
			name: "error when curlew.yaml already exists",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "curlew.yaml"), []byte("project_name: Existing\n"), 0o600); err != nil {
					t.Fatalf("setup: %v", err)
				}
			},
			wantErr: ErrProjectExists,
		},
		{
			name: "error when curlew.yml already exists",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "curlew.yml"), []byte("project_name: Existing\n"), 0o600); err != nil {
					t.Fatalf("setup: %v", err)
				}
			},
			wantErr: ErrProjectExists,
		},
		{
			name: "appends .env to existing gitignore without losing original content",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0o600); err != nil {
					t.Fatalf("setup: %v", err)
				}
			},
			checkContent: map[string]string{
				".gitignore": "node_modules/",
			},
		},
		{
			name: "does not duplicate .env when gitignore already has it",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".env\n"), 0o600); err != nil {
					t.Fatalf("setup: %v", err)
				}
			},
			// no extra assertion needed — just verify Init succeeds and .env appears once
			checkContent: map[string]string{
				".gitignore": ".env",
			},
		},
		{
			name: "env.example contains placeholder not real secret values",
			checkContent: map[string]string{
				".env.example": "your-api-key-here",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.setup != nil {
				tc.setup(t, dir)
			}
			opts := Options{Dir: dir}
			if tc.opts != nil {
				opts = tc.opts(dir)
			}

			err := Init(opts)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for _, f := range tc.wantFiles {
				path := filepath.Join(dir, filepath.FromSlash(f))
				if _, statErr := os.Stat(path); statErr != nil {
					t.Errorf("expected file %s to exist: %v", f, statErr)
				}
			}

			for rel, substr := range tc.checkContent {
				path := filepath.Join(dir, filepath.FromSlash(rel))
				data, readErr := os.ReadFile(path)
				if readErr != nil {
					t.Fatalf("read %s: %v", rel, readErr)
				}
				if !strings.Contains(string(data), substr) {
					t.Errorf("file %s: expected to contain %q\ngot:\n%s", rel, substr, data)
				}
			}
		})
	}
}

func TestInit_error_when_directory_not_writable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission checks don't apply")
	}
	dir := t.TempDir()
	// Make the directory read-only so MkdirAll / WriteFile fail
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	err := Init(Options{Dir: dir})
	if err == nil {
		t.Fatal("expected error for non-writable directory, got nil")
	}
}

func TestInit_error_writing_curlew_yaml(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission checks don't apply")
	}
	dir := t.TempDir()
	// Pre-create subdirs so MkdirAll succeeds, then make root read-only so WriteFile fails
	for _, sub := range []string{"environments", "collections"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o750); err != nil {
			t.Fatalf("setup mkdir %s: %v", sub, err)
		}
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	err := Init(Options{Dir: dir})
	if err == nil {
		t.Fatal("expected error when root dir is read-only, got nil")
	}
}

func TestInit_error_when_environments_dir_is_a_file(t *testing.T) {
	dir := t.TempDir()
	// Put a regular file where the "environments" subdirectory should be
	if err := os.WriteFile(filepath.Join(dir, "environments"), []byte("conflict"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := Init(Options{Dir: dir})
	if err == nil {
		t.Fatal("expected error when environments is a file, got nil")
	}
}

func TestInit_gitignore_no_duplicate_env_entry(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".env\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := Init(Options{Dir: dir}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}

	count := strings.Count(string(data), ".env")
	if count != 1 {
		t.Errorf(".gitignore contains .env %d times, want exactly 1\ncontent:\n%s", count, data)
	}
}

// TestOutputBlock_DefaultFallback exercises the defensive default branch in
// outputBlock with the events flag disabled.
func TestOutputBlock_EnableEvents(t *testing.T) {
	got := outputBlock("markdown", true)
	if !strings.Contains(got, "events: .curlew/run.ndjson") {
		t.Fatalf("outputBlock with enableEvents=true should contain events line; got:\n%s", got)
	}
}

func TestInit_SkillClaude_CopiesFile(t *testing.T) {
	dir := t.TempDir()
	if err := Init(Options{Dir: dir, SkillName: "claude", CurlewVersion: "9.9.9"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "curlew", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(body), "curlew-skill: agent v1.0 (curlew 9.9.9)") {
		t.Errorf("missing version comment with substituted version:\n%s", body)
	}
	if strings.Contains(string(body), "{{curlew_version}}") {
		t.Errorf("untouched template token in scaffolded SKILL.md")
	}
}

func TestInit_SkillClaude_DefaultsMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := Init(Options{Dir: dir, SkillName: "claude", CurlewVersion: "0.0.1"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "curlew.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "format: markdown") {
		t.Errorf("missing format: markdown:\n%s", body)
	}
	if !strings.Contains(string(body), "report: responses/") {
		t.Errorf("missing report: responses/:\n%s", body)
	}
	if !strings.Contains(string(body), "events: .curlew/run.ndjson") {
		t.Errorf("missing events: .curlew/run.ndjson:\n%s", body)
	}
}

func TestInit_SkillClaude_ExtendsOutputBlock(t *testing.T) {
	formats := []string{"terminal", "json", "tap", "junit", "html", "markdown"}
	for _, f := range formats {
		t.Run(f, func(t *testing.T) {
			dir := t.TempDir()
			if err := Init(Options{Dir: dir, SkillName: "claude", OutputFormat: f, CurlewVersion: "0"}); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(filepath.Join(dir, "curlew.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), "events: .curlew/run.ndjson") {
				t.Errorf("format=%s missing events line:\n%s", f, body)
			}
			if !strings.Contains(string(body), "format: "+f) {
				t.Errorf("format=%s body missing format line:\n%s", f, body)
			}
		})
	}
}

func TestInit_SkillClaude_ExtendsGitignore(t *testing.T) {
	dir := t.TempDir()
	if err := Init(Options{Dir: dir, SkillName: "claude", CurlewVersion: "0"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	want := ".env\n.curlew/\n"
	if string(body) != want {
		t.Errorf(".gitignore = %q, want %q", body, want)
	}
}

func TestInit_BareSkill_GitignoreOnlyEnv(t *testing.T) {
	dir := t.TempDir()
	if err := Init(Options{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	want := ".env\n"
	if string(body) != want {
		t.Errorf("bare-init .gitignore diverged: got %q want %q", body, want)
	}
}

func TestInit_SkillClaude_PreservesExistingResponsesDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "responses"), 0o750); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "responses", "keep.md")
	if err := os.WriteFile(keep, []byte("preexisting"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Init(Options{Dir: dir, SkillName: "claude", CurlewVersion: "0"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(keep)
	if err != nil {
		t.Fatalf("expected pre-existing file to survive: %v", err)
	}
	if string(body) != "preexisting" {
		t.Errorf("responses/keep.md mutated: got %q want %q", body, "preexisting")
	}
}

func TestInit_SkillClaude_OutputOverride(t *testing.T) {
	dir := t.TempDir()
	if err := Init(Options{Dir: dir, SkillName: "claude", OutputFormat: "json", CurlewVersion: "0"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "curlew.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"format: json", "report: results.json", "events: .curlew/run.ndjson"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q:\n%s", want, body)
		}
	}
}

func TestInit_SkillClaude_PreservesExistingSkillFile(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".claude", "skills", "curlew")
	if err := os.MkdirAll(skillDir, 0o750); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("user-edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Init(Options{Dir: dir, SkillName: "claude", CurlewVersion: "0"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "user-edited\n" {
		t.Errorf("user-edited SKILL.md was overwritten: got %q", body)
	}
}

func TestInit_SkillClaude_WritesAllTopicFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Init(Options{Dir: dir, SkillName: "claude", CurlewVersion: "9.9.9"}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, ".claude", "skills", "curlew")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 11 {
		t.Errorf("want >= 11 files under %s; got %d", root, len(entries))
	}
	// Assert each expected topic file is present.
	wantFiles := []string{
		"SKILL.md", "variables.md", "output-formats.md", "assertions.md",
		"retry.md", "parallel.md", "vault.md", "signing.md",
		"expressions.md", "exit-codes.md", "failure-playbook.md",
	}
	for _, name := range wantFiles {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Errorf("expected %s to be materialised: %v", name, err)
		}
	}
}

func TestInit_SkillClaude_PreservesExistingTopicFile(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".claude", "skills", "curlew")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, "variables.md")
	if err := os.WriteFile(keep, []byte("user-edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Init(Options{Dir: dir, SkillName: "claude", CurlewVersion: "0"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(keep)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "user-edited\n" {
		t.Errorf("user-edited variables.md was overwritten: got %q", body)
	}
	// Sibling files should still be scaffolded.
	if _, err := os.Stat(filepath.Join(root, "expressions.md")); err != nil {
		t.Errorf("sibling expressions.md should have been written: %v", err)
	}
}

func TestEnsureGitignore_MultipleEntries_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	if err := ensureGitignore(path, []string{".env", ".curlew/"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := ".env\n.curlew/\n"
	if string(got) != want {
		t.Errorf("ensureGitignore wrote %q, want %q", got, want)
	}
}

func TestEnsureGitignore_MultipleEntries_AppendsMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(path, []byte(".env\nnode_modules/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureGitignore(path, []string{".env", ".curlew/"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := ".env\nnode_modules/\n.curlew/\n"
	if string(got) != want {
		t.Errorf("ensureGitignore wrote %q, want %q", got, want)
	}
}

func TestEnsureGitignore_NoOpWhenAllEntriesPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	original := ".env\n.curlew/\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureGitignore(path, []string{".env", ".curlew/"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("ensureGitignore mutated file with no missing entries: got %q want %q", got, original)
	}
}

func TestEnsureGitignore_NoTrailingNewline_AppendsCleanly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(path, []byte("node_modules/"), 0o600); err != nil { // no trailing newline
		t.Fatal(err)
	}
	if err := ensureGitignore(path, []string{".env"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "node_modules/\n.env\n"
	if string(got) != want {
		t.Errorf("ensureGitignore wrote %q, want %q", got, want)
	}
}
