// Package scaffold creates a new curlew project directory structure.
package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/weiqigod/curlew/templates"
)

// ErrProjectExists indicates that an curlew.yaml or curlew.yml already exists
// in the target directory.
var ErrProjectExists = errors.New("project already exists: curlew.yaml found in target directory")

// Options configures project initialization.
type Options struct {
	// Dir is the target directory. Empty defaults to ".".
	Dir string
	// ProjectName overrides the project name. Empty defaults to the directory basename.
	ProjectName string
	// OutputFormat selects the output: block emitted in curlew.yaml. Empty
	// selects the M8-003 default (format: terminal, verbosity: normal).
	// Validated by the caller against output.SupportedFormats.
	OutputFormat string
	// SkillName, when non-empty, scaffolds an agent skill template at
	// .claude/skills/curlew/SKILL.md, defaults the output: block format to
	// markdown when OutputFormat is empty, appends the events line to the
	// output: block, and adds .curlew/ to .gitignore. Empty preserves the
	// pre-M10 byte-identical scaffold. Validated by the caller against
	// templates.SupportedSkills.
	SkillName string
	// CurlewVersion is substituted into the SKILL.md template at the
	// {{curlew_version}} token. Required when SkillName is non-empty;
	// ignored otherwise. The CLI layer threads cmd/curlew/main.go's
	// version const into this field.
	CurlewVersion string
}

// Init creates a new curlew project in the specified directory.
// It creates curlew.yaml, .gitignore (or appends .env if it exists),
// .env.example, environments/dev.yaml, and collections/sample.yaml.
// Returns ErrProjectExists if curlew.yaml or curlew.yml already exists.
func Init(opts Options) error {
	dir := opts.Dir
	if dir == "" {
		dir = "."
	}

	// Detect existing project
	for _, name := range []string{"curlew.yaml", "curlew.yml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return ErrProjectExists
		}
	}

	projectName := opts.ProjectName
	if projectName == "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("resolving directory: %w", err)
		}
		projectName = filepath.Base(abs)
	}

	// Determine the format that drives the output: block. Keyed on "a skill
	// was requested", not on which name was used — the names are aliases for
	// one payload (templates.SupportedSkills), and the CLI layer has already
	// rejected anything outside that set. Matching a single literal here
	// silently gave later names a lesser scaffold.
	formatForBlock := opts.OutputFormat
	enableEvents := opts.SkillName != ""
	if enableEvents && formatForBlock == "" {
		formatForBlock = "markdown"
	}

	gitignoreEntries := []string{".env"}
	if opts.SkillName != "" {
		gitignoreEntries = append(gitignoreEntries, ".curlew/")
	}

	// Create subdirectories
	for _, d := range []string{"environments", "collections"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o750); err != nil {
			return fmt.Errorf("creating %s directory: %w", d, err)
		}
	}

	if err := writeFile(filepath.Join(dir, "curlew.yaml"), curlewYAML(projectName, formatForBlock, enableEvents)); err != nil {
		return err
	}
	if err := ensureGitignore(filepath.Join(dir, ".gitignore"), gitignoreEntries); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, ".env.example"), envExample()); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, "environments", "dev.yaml"), devEnvironment()); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, "collections", "sample.yaml"), sampleCollection()); err != nil {
		return err
	}

	if opts.SkillName != "" {
		if err := installSkill(dir, opts.SkillName, opts.CurlewVersion); err != nil {
			return err
		}
	}

	return nil
}

// installSkill walks the named skill's embedded directory and writes each file
// to its canonical relative path under dir. Per-file skip-if-exists semantics
// match ErrProjectExists behaviour: never overwrite user-edited content. A
// pre-existing file is left untouched while its siblings are still scaffolded.
// Creates parent directories as needed (supporting recursive skill layouts).
func installSkill(dir, skillName, version string) error {
	seq, err := templates.Walk(skillName)
	if err != nil {
		return fmt.Errorf("walking skill %q: %w", skillName, err)
	}
	rootRel := templates.SkillRootDir(skillName) // e.g. ".claude/skills/curlew"
	for rel, body := range seq {
		dest := filepath.Join(dir, filepath.FromSlash(rootRel), filepath.FromSlash(rel))
		if _, statErr := os.Stat(dest); statErr == nil {
			// Pre-existing file: never overwrite user-edited content.
			continue
		}
		if mkErr := os.MkdirAll(filepath.Dir(dest), 0o750); mkErr != nil {
			return fmt.Errorf("creating skill directory: %w", mkErr)
		}
		// Apply {{curlew_version}} substitution to .md files only; binary
		// assets (if any are added in future) must not be modified.
		out := body
		if strings.HasSuffix(rel, ".md") {
			out = []byte(strings.ReplaceAll(string(body), "{{curlew_version}}", version))
		}
		if writeErr := os.WriteFile(dest, out, 0o644); writeErr != nil { //nolint:gosec
			return fmt.Errorf("writing skill file %s: %w", rel, writeErr)
		}
	}
	return nil
}

func writeFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec
		return fmt.Errorf("writing %s: %w", filepath.Base(path), err)
	}
	return nil
}

// ensureGitignore creates .gitignore with the given entries (one per line),
// or appends any missing entries if the file already exists. The order of
// entries in the appended block matches the order in the entries slice.
// Existing entries (already present on their own line, trimmed) are skipped
// — entries are matched exactly, not as substrings, so ".env" never matches
// ".env.example" or similar.
func ensureGitignore(path string, entries []string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("reading .gitignore: %w", err)
	}

	have := make(map[string]bool)
	if err == nil {
		for _, line := range strings.Split(string(existing), "\n") {
			have[strings.TrimSpace(line)] = true
		}
	}

	var missing []string
	for _, e := range entries {
		if !have[e] {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 && err == nil {
		return nil
	}

	var content string
	if err == nil {
		content = string(existing)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
	}
	for _, e := range missing {
		content += e + "\n"
	}
	if writeErr := os.WriteFile(path, []byte(content), 0o644); writeErr != nil { //nolint:gosec
		return fmt.Errorf("appending to .gitignore: %w", writeErr)
	}
	return nil
}

func curlewYAML(projectName, outputFormat string, enableEvents bool) string {
	return fmt.Sprintf("project_name: %q\n"+
		"variables:\n"+
		"  base_url: \"https://httpbin.org\"\n"+
		"%s", projectName, outputBlock(outputFormat, enableEvents))
}

// outputBlock returns the YAML output: section for a scaffolded curlew.yaml.
// An empty format yields the M8-003 default block (terminal, verbosity: normal).
// Any non-empty value is assumed valid (caller validates against
// output.SupportedFormats). When enableEvents is true, an events:
// .curlew/run.ndjson line is spliced before the verbosity: line.
func outputBlock(format string, enableEvents bool) string {
	var base string
	switch format {
	case "", "terminal":
		base = "output:\n  format: terminal\n  verbosity: normal\n"
	case "json":
		base = "output:\n  format: json\n  report: results.json\n  verbosity: normal\n"
	case "tap":
		base = "output:\n  format: tap\n  verbosity: normal\n"
	case "junit":
		base = "output:\n  format: junit\n  report: results.xml\n  verbosity: normal\n"
	case "html":
		base = "output:\n  format: html\n  report: report.html\n  verbosity: normal\n"
	case "markdown":
		base = "output:\n  format: markdown\n  report: responses/\n  verbosity: normal\n"
	default:
		// Unreachable: caller is expected to validate against output.SupportedFormats
		// before reaching here. Fall back to default for defensive behaviour.
		base = "output:\n  format: terminal\n  verbosity: normal\n"
	}
	if !enableEvents {
		return base
	}
	// Splice the events: line in before the verbosity: line so the output
	// keeps the existing field order: format, report?, events, verbosity.
	const verbosityLine = "  verbosity: normal\n"
	const eventsLine = "  events: .curlew/run.ndjson\n"
	if i := strings.Index(base, verbosityLine); i >= 0 {
		return base[:i] + eventsLine + base[i:]
	}
	// Defensive: if verbosity is missing, append events at the end.
	return base + eventsLine
}

func envExample() string {
	return "# Local secrets for this project.\n" +
		"# Copy this file to .env and fill in real values.\n" +
		"# NEVER commit .env to version control.\n" +
		"#\n" +
		"# API_KEY=your-api-key-here\n" +
		"# SECRET_TOKEN=your-secret-here\n"
}

func devEnvironment() string {
	// The override ships commented out: environment values take precedence
	// over curlew.yaml's variables:, so an active base_url here would
	// silently shadow the project-level default on every `--env dev` run.
	return "# Overrides for the \"dev\" environment.\n" +
		"# Values here take precedence over the variables: block in curlew.yaml.\n" +
		"# Uncomment to point this environment at a different host:\n" +
		"#\n" +
		"# variables:\n" +
		"#   base_url: \"https://dev.example.com\"\n" +
		"variables: {}\n"
}

func sampleCollection() string {
	return "name: Sample Collection\n" +
		"description: A sample collection generated by curlew init\n\n" +
		"requests:\n" +
		"  - name: Hello World\n" +
		"    request:\n" +
		"      method: GET\n" +
		"      url: \"{{base_url}}/get\"\n" +
		"    assertions:\n" +
		"      status: 200\n"
}
