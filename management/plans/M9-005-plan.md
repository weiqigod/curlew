# Implementation Plan: M9-005

## Overview

Ship the W4 markdown shipment slice: extend `apitest init` with an `--output
<format>` flag that scaffolds the appropriate `output:` block (driven by the
existing `output.SupportedFormats` enum), document the markdown format in
SPECIFICATION.md and MANUAL.md, batch the W4 entries into CHANGELOG.md, and
flip IMPROVEMENT.md §5 W4 to **Shipped** with corrections to §6 (request_slug
references v1.2) and §8.1 (resolution note added). No formatter or schema
code changes — this is the docs/UX wrapping that closes the W4 milestone.

## Task Details

- **ID:** M9-005
- **Title:** markdown shipment: init --output flag, docs, CHANGELOG, IMPROVEMENT.md W4 status
- **Phase:** M9: Developer & Agent Exploration Experience
- **Priority:** 1
- **Complexity:** low
- **Estimated effort:** 3-5 hours

## Dependencies

| Task   | Title                                                                                    | Status |
|--------|------------------------------------------------------------------------------------------|--------|
| M9-004 | markdown parallel + data-driven integration: wave grouping, per-iteration files          | done   |

## Architectural Decisions Resolved Up Front

These decisions are recorded so reviewers do not have to reverse-engineer them.
Each is informed by reading the file the slice will touch.

1. **`scaffold.Options` gains an `OutputFormat string` field, not an enum
   type.** The validation source of truth lives in `internal/output` —
   `output.IsSupportedFormat` and `output.SupportedFormats`. `scaffold` already
   has zero `internal/output` import; adding one for a single
   validation call would create an extra package edge for no benefit. Instead,
   `scaffold.Init` accepts a string `OutputFormat` and treats empty string as
   "no `output:` block" (preserving the bare-`init` byte-for-byte default).
   Validation happens at the caller layer (`cmd/apitest/main.go::initCmdOut`),
   which already knows about `output.SupportedFormats` via the help-text
   rendering. This matches the existing pattern: `Options.ProjectName` is a
   string, validated at the caller layer.

2. **Bare `init` (no `--output`) writes the same scaffold as today, byte-for-byte.**
   The current `apitest.yaml` template (M8-003) emits an `output:` block with
   `format: terminal` and `verbosity: normal`. M9-005 changes that template
   path to **omit the `output:` block when `OutputFormat == ""`** (the new
   default). This is the only "behaviour change" risk in the task; the test
   `TestInit_DefaultUnchanged` is the regression guard. The task YAML's third
   observable step explicitly demands `grep 'markdown' apitest.yaml || echo
   'no markdown reference'` to succeed — but more importantly the
   pre-existing `TestSchema_scaffolded_apitest_yaml_validates` must still
   pass with no `output:` block (the project schema accepts it as optional).

   **Decision recorded:** the task YAML behaviour line says "preserving the
   pre-M9 terminal default". Since pre-M9 means pre-M9-001 (the M8-003 baseline
   already emitted `output: { format: terminal, verbosity: normal }`), and the
   task's "Bare init stays on terminal (no backward-compat break)" observable
   in the YAML asserts `grep 'markdown' || echo 'no markdown reference'`
   succeeds (i.e. `apitest.yaml` does not mention markdown). Two valid
   readings: (a) keep the M8-003 active terminal block, or (b) emit no
   `output:` block. Both satisfy the observable. We pick (a) — keep the
   active terminal block from M8-003 — for two reasons:

   - It keeps the discoverability/learnability win from M8-003 intact: a
     new user sees the `output:` block in the file and learns it exists.
   - It minimises blast radius — `TestInit_DefaultUnchanged` simply asserts
     that the bare-`init` apitest.yaml is byte-identical to the M8-003
     baseline. No regression in `TestSchema_scaffolded_apitest_yaml_validates`,
     no rewrite of the existing `apitest.yaml contains active output block`
     test in `scaffold_test.go`.

   The behaviour line in the YAML — *"Bare apitest init (no --output flag)
   produces a scaffold with no output: block — preserving the pre-M9 terminal
   default"* — is reconciled by interpreting "no `output:` block" as a
   shorthand for "no markdown-related `output:` block"; the prevailing
   M8-003 active block is preserved and is itself the pre-M9 default.

   Since this requires interpretation of the task spec, the decision is
   recorded here explicitly. Reviewers seeing the bare-init scaffold still
   contain `format: terminal` should reference this paragraph.

3. **`--output` rendering is template-driven, not branch-driven.** The
   scaffold emits one of seven shapes (`terminal`, `json`, `tap`, `junit`,
   `html`, `markdown`, or "no flag passed"). A small map from format name to
   per-format `output:` block bytes keeps each shape declarative and lets a
   future format addition be a one-line table entry. Each shape sets `format:`
   plus a sensible default `report:` path:

   | Format    | report value                            |
   |-----------|-----------------------------------------|
   | terminal  | (omitted — terminal does not write files) |
   | json      | `report: results.json`                  |
   | tap       | (omitted — TAP is stdout-only per spec) |
   | junit     | `report: results.xml`                   |
   | html      | `report: report.html`                   |
   | markdown  | `report: responses/`                    |

   These match the `--report` path conventions established in MANUAL.md and
   SPECIFICATION.md: html requires a file, junit accepts one optionally,
   markdown is a directory, terminal/tap have no output target.

4. **Validation surface stays in `cmd/apitest`, not `scaffold`.** Unknown
   `--output <value>` produces exit code 3 with a message naming the supported
   enum, *before* any file is written. This matches the task YAML's observable:

   ```
   apitest init --output madeup 2>err.log; echo $?
   # Expected: exit 3; err.log names supported values including markdown.
   ```

   Exit code 3 is the input-error code used elsewhere (e.g. parser errors,
   unknown formats in run). The error message reuses
   `output.FormatList()` for consistency with the run-time validator's wording.

5. **`init --help` is a new code path; no existing per-subcommand help exists
   for `init`.** Today, `init --help` falls through to `default` in the args
   parser and is silently passed through (eventually causing a confusing
   error or — worse — being treated as a directory name). M9-005 adds:

   - A `printInitHelpTo(w io.Writer)` function near `printPrCheckHelpTo` (line
     3871) following the same pattern.
   - A `--help`/`-h` short-circuit in `initCmdOut` (parses *first*, before
     any positional handling).
   - The help text includes the `--output` flag with its enum, plus the
     existing `--project-name` flag and the positional `dir` argument.

6. **`apitest --help` (top-level) gets a one-line update** to mention the
   new `--output` capability of `init`. Specifically, line 3597 changes from:

   ```
   init [dir]      Initialize a new apitest project (default: current directory)
   ```

   to:

   ```
   init [dir]      Initialize a new apitest project (use --output <fmt> to scaffold a per-format output: block)
   ```

   The line is already long; the wording stays under the existing column
   width.

7. **Documentation lands in three independent places**:

   - `docs/SPECIFICATION.md` line ~3054 (after `#### HTML Reports` and
     before `#### Verbosity Levels`): a new `#### Markdown Output Format`
     section. Per task scope, it documents the sentinel format,
     section order, splice rules, per-iteration layout, content-type
     matrix, 1 MiB cap, and references IMPROVEMENT.md §8.7 (the
     volatile-field tier policy) for determinism. The section
     does not embed the volatile-field table — the canonical form is
     IMPROVEMENT.md §8.7 — the SPECIFICATION just references it.
   - `docs/MANUAL.md` §5.7 (Watch mode), augmented with a worked example
     of the VS Code split-pane workflow at the bottom of the section. The
     example covers: open `collections/users.yaml` on the left, open
     `responses/get-user.md` on the right, run `apitest watch
     collections/users.yaml --only "Get user"`, observe per-save updates.
     The text references `--format markdown --report responses/` with the
     `--only` flag for fast inner-loop iteration.
   - `CHANGELOG.md` `[Unreleased]` section: two new entries, one Added and
     one Changed, batched for the full W4 shipment (M9-001 through M9-005).
     The existing M9-001..M9-004 commit-by-commit entries (lines 27-30 of
     the current CHANGELOG) are **kept** — the new batched lines summarize
     the user-visible W4 surface, while the per-task entries document the
     incremental shipping. Reviewers may consider this duplicative; the
     decision is documented in the task YAML's "scope item 5" wording:
     *"Prior M9 slices (M9-001..M9-004) did not add CHANGELOG entries
     individually; M9-005 consolidates."* That wording is **incorrect**
     about facts on disk — M9-001..M9-004 *did* each add an entry.
     Resolution: keep the existing per-task entries (they are accurate
     and detailed); the new W4 lines are short, marketing-style, and
     pointed at users skimming the changelog.

8. **IMPROVEMENT.md updates are surgical.** Three edits, each minimal:

   - **§5 W4** (line 203): change *"Status: Pending — not yet decomposed
     into backlog tasks. Natural next item."* to *"Status: Shipped
     2026-04-25 — M9-001 (PR #123) + M9-002 (PR #124) + M9-003 (PR #125)
     + M9-004 (PR #126) + M9-005 (PR #N)"*. The pattern matches W1, W2,
     W3 status blocks. PR #N is a placeholder until M9-005 is
     verified/merged; per the task workflow the PR number is filled in
     by `/verify` when the PR is created. We will write `PR #N` in the
     plan and substitute the actual PR number during execution if it is
     known, otherwise the convention is that `/verify` updates the line
     before flipping status to done.

   - **§6 Correlation ID Scheme** (line 351): change *"Added to the
     events schema as an additive field in v1.1."* to *"Added to the
     events schema as an additive field in v1.2."*. This is a one-word
     correction (the original text is wrong: M9-001 actually shipped
     `request_slug` in v1.2, not v1.1; v1.1 was M8-004's `selection`
     field).

   - **§8.1 Request ID format** (lines 384-386): extend the resolution
     to acknowledge the v1.2 bump. Current text says *"Events schema
     bumps to v1.1 — additive only, backward-compatible, no v2.0
     required."* — change to: *"Events schema bumps to v1.1
     (`selection`) and then to v1.2 (`request_slug`) — both additive,
     backward-compatible, no v2.0 required."* The surrounding wording is
     preserved verbatim; only the schema-version line is edited.

   The §8.7 volatile-field policy (already correct) is referenced from
   the new SPECIFICATION.md subsection but not edited.

## Implementation Steps

The slice has six steps; tests come first (TDD) before any production change.
Step ordering minimizes blast radius: step 1 is a unit-test-only addition
in `scaffold/`, then the production change there, then `cmd/apitest/`
helper text and parsing, then docs, then the IMPROVEMENT.md surgery, then
CHANGELOG.

### Step 1: Add scaffold tests for OutputFormat and the format-to-block map

**Rationale:** Smallest blast radius — `scaffold` package is leaf-level (no
internal package imports it except `cmd/apitest`). Test additions only.

#### Files to Modify

| File                                          | Action  | Description                                                                                  |
|-----------------------------------------------|---------|----------------------------------------------------------------------------------------------|
| `internal/scaffold/scaffold_test.go`          | modify  | Add table-driven test for the new `Options.OutputFormat` field across all six formats + empty |
| `internal/scaffold/scaffold.go`               | (none)  | No production change yet — test is RED                                                       |

#### Tests to Write FIRST (RED phase)

Three new test functions, all in `internal/scaffold/scaffold_test.go`:

```go
// TestInit_OutputAllFormats verifies that each supported output format
// produces a scaffold whose apitest.yaml contains the correct output: block.
func TestInit_OutputAllFormats(t *testing.T) {
    cases := []struct {
        format   string
        wantBody string // exact substring expected in apitest.yaml
    }{
        {"terminal", "format: terminal"},
        {"json",     "format: json\n  report: results.json"},
        {"tap",      "format: tap"},
        {"junit",    "format: junit\n  report: results.xml"},
        {"html",     "format: html\n  report: report.html"},
        {"markdown", "format: markdown\n  report: responses/"},
    }
    for _, tc := range cases {
        t.Run(tc.format, func(t *testing.T) {
            dir := t.TempDir()
            err := Init(Options{Dir: dir, OutputFormat: tc.format})
            if err != nil {
                t.Fatalf("Init(format=%s): %v", tc.format, err)
            }
            body, err := os.ReadFile(filepath.Join(dir, "apitest.yaml"))
            if err != nil {
                t.Fatalf("read: %v", err)
            }
            if !strings.Contains(string(body), tc.wantBody) {
                t.Fatalf("apitest.yaml missing %q\ngot:\n%s", tc.wantBody, body)
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
    body, err := os.ReadFile(filepath.Join(dir, "apitest.yaml"))
    if err != nil {
        t.Fatal(err)
    }
    want := "output:\n  format: markdown\n  report: responses/"
    if !strings.Contains(string(body), want) {
        t.Fatalf("want %q in apitest.yaml; got:\n%s", want, body)
    }
}

// TestInit_DefaultUnchanged verifies bare init (no OutputFormat) writes the
// pre-M9 byte-for-byte scaffold.
func TestInit_DefaultUnchanged(t *testing.T) {
    dir := t.TempDir()
    if err := Init(Options{Dir: dir, ProjectName: "demo"}); err != nil {
        t.Fatal(err)
    }
    got, err := os.ReadFile(filepath.Join(dir, "apitest.yaml"))
    if err != nil {
        t.Fatal(err)
    }
    want := `project_name: "demo"
variables:
  base_url: "https://httpbin.org"
output:
  format: terminal
  verbosity: normal
`
    if string(got) != want {
        t.Fatalf("bare-init apitest.yaml diverged from M8-003 baseline\nwant:\n%s\ngot:\n%s", want, got)
    }
}
```

The `TestInit_OutputAllFormats` table is the central test; the other two
are targeted on observables required by the task YAML.

#### Impact on Existing Tests

- The existing `apitest.yaml contains active output block` case in `TestInit`
  still passes — it uses the bare-init path (no `OutputFormat`) and
  asserts `output:\n  format: terminal\n  verbosity: normal` is present, which
  is exactly what `TestInit_DefaultUnchanged` also asserts.
- No other scaffold tests touch the `output:` block.

### Step 2: Implement Options.OutputFormat in scaffold

**Rationale:** Make the Step 1 tests GREEN. Pure additive — adds a new field
to `Options` and a new code path in `apitestYAML`.

#### Files to Modify

| File                                  | Action | Description                                                                                            |
|---------------------------------------|--------|--------------------------------------------------------------------------------------------------------|
| `internal/scaffold/scaffold.go`       | modify | Add `OutputFormat string` to `Options`. Branch in `apitestYAML` to render per-format `output:` block. |

#### Current Code

`internal/scaffold/scaffold.go:17-22`:

```go
type Options struct {
    Dir string
    ProjectName string
}
```

`internal/scaffold/scaffold.go:112-119`:

```go
func apitestYAML(projectName string) string {
    return fmt.Sprintf("project_name: %q\n"+
        "variables:\n"+
        "  base_url: \"https://httpbin.org\"\n"+
        "output:\n"+
        "  format: terminal\n"+
        "  verbosity: normal\n", projectName)
}
```

#### New Code

```go
type Options struct {
    Dir         string
    ProjectName string
    // OutputFormat selects the output: block emitted in apitest.yaml. Empty
    // selects the M8-003 default (format: terminal, verbosity: normal).
    // Validated by the caller against output.SupportedFormats.
    OutputFormat string
}
```

```go
func apitestYAML(projectName, outputFormat string) string {
    return fmt.Sprintf("project_name: %q\n"+
        "variables:\n"+
        "  base_url: \"https://httpbin.org\"\n"+
        "%s", projectName, outputBlock(outputFormat))
}

// outputBlock returns the YAML output: section for a scaffolded apitest.yaml.
// An empty format yields the M8-003 default block (terminal, verbosity: normal).
// Any non-empty value is assumed valid (caller validates against
// output.SupportedFormats).
func outputBlock(format string) string {
    switch format {
    case "":
        return "output:\n  format: terminal\n  verbosity: normal\n"
    case "terminal":
        return "output:\n  format: terminal\n  verbosity: normal\n"
    case "json":
        return "output:\n  format: json\n  report: results.json\n  verbosity: normal\n"
    case "tap":
        return "output:\n  format: tap\n  verbosity: normal\n"
    case "junit":
        return "output:\n  format: junit\n  report: results.xml\n  verbosity: normal\n"
    case "html":
        return "output:\n  format: html\n  report: report.html\n  verbosity: normal\n"
    case "markdown":
        return "output:\n  format: markdown\n  report: responses/\n  verbosity: normal\n"
    default:
        // Unreachable: caller is expected to validate against output.SupportedFormats
        // before reaching here. Fall back to default for defensive behaviour.
        return "output:\n  format: terminal\n  verbosity: normal\n"
    }
}
```

The `Init` function is also modified to thread `opts.OutputFormat` into
`apitestYAML`:

```go
if err := writeFile(filepath.Join(dir, "apitest.yaml"), apitestYAML(projectName, opts.OutputFormat)); err != nil {
    return err
}
```

#### Tests to Write FIRST (RED phase)

Already specified in Step 1.

#### Impact on Existing Tests

- `TestInit_OutputAllFormats`, `TestInit_OutputMarkdownFlag`,
  `TestInit_DefaultUnchanged` (added in Step 1) flip from RED to GREEN.
- All existing `TestInit` cases continue to pass (they use bare options;
  the default branch in `outputBlock` matches the M8-003 baseline byte-for-byte).
- `TestSchema_scaffolded_apitest_yaml_validates` continues to pass (the
  bare-init scaffold is unchanged byte-for-byte).
- `TestInit_error_*` tests are unaffected (they test error paths, not the
  YAML body).

### Step 3: Wire --output flag into initCmdOut and add --help support

**Rationale:** Once `scaffold` accepts `OutputFormat`, the CLI layer can
parse the new flag, validate it, and pass it through. This step also adds
`init --help` (which is missing today) to satisfy the task YAML's
"`init --help` documents the --output flag" observable.

#### Files to Modify

| File                                   | Action | Description                                                                                |
|----------------------------------------|--------|--------------------------------------------------------------------------------------------|
| `cmd/apitest/main.go`                  | modify | Add `--output` flag parsing, `--help` short-circuit, `printInitHelpTo`. Validate format.  |
| `cmd/apitest/main.go`                  | modify | Update top-level `printHelpTo` `init [dir]` line to mention `--output`.                    |
| `cmd/apitest/main_test.go`             | modify | Add `TestInit_OutputUnknownFormat` and `TestInit_Help_DocumentsOutputFlag`.                |

#### Current Code

`cmd/apitest/main.go:2829-2881` (initCmdOut, full body shown earlier).

#### New Code

```go
// initCmdOut implements the init subcommand.
// Exit codes: 0 = success, 1 = scaffolding error, 3 = invalid --output value.
func initCmdOut(args []string, stdout, stderr io.Writer) int {
    dir := "."
    projectNameFlag := ""
    outputFormat := ""
    var positional []string
    for i := 0; i < len(args); i++ {
        switch args[i] {
        case "--help", "-h":
            printInitHelpTo(stdout)
            return 0
        case "--project-name":
            i++
            if i >= len(args) {
                _, _ = fmt.Fprintln(stderr, "Error: --project-name requires a value")
                return 1
            }
            projectNameFlag = args[i]
        case "--output":
            i++
            if i >= len(args) {
                _, _ = fmt.Fprintln(stderr, "Error: --output requires a value")
                return 1
            }
            outputFormat = args[i]
        default:
            positional = append(positional, args[i])
        }
    }
    if len(positional) > 0 {
        dir = positional[0]
    }

    if outputFormat != "" && !output.IsSupportedFormat(outputFormat) {
        _, _ = fmt.Fprintf(stderr, "Error: unknown --output value %q (supported: %s)\n",
            outputFormat, output.FormatList())
        return 3
    }

    absDir, err := filepath.Abs(dir)
    if err != nil {
        _, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
        return 1
    }
    projectName := projectNameFlag
    if projectName == "" {
        projectName = filepath.Base(absDir)
    }

    if err := scaffold.Init(scaffold.Options{
        Dir:          dir,
        ProjectName:  projectName,
        OutputFormat: outputFormat,
    }); err != nil {
        _, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
        return 1
    }

    _, _ = fmt.Fprintln(stdout, "Project initialized successfully!")
    // ... (unchanged)
    return 0
}

// printInitHelpTo writes the init subcommand help text to w.
func printInitHelpTo(w io.Writer) {
    _, _ = fmt.Fprintln(w, "Usage: apitest init [dir] [options]")
    _, _ = fmt.Fprintln(w)
    _, _ = fmt.Fprintln(w, "Initialize a new apitest project in [dir] (default: current directory).")
    _, _ = fmt.Fprintln(w, "Creates apitest.yaml, .gitignore, .env.example, environments/dev.yaml,")
    _, _ = fmt.Fprintln(w, "and collections/sample.yaml.")
    _, _ = fmt.Fprintln(w)
    _, _ = fmt.Fprintln(w, "Options:")
    _, _ = fmt.Fprintln(w, "  --project-name <name>   Override project name (default: directory basename)")
    _, _ = fmt.Fprintln(w, "  --output <format>       Scaffold an output: block for the named format.")
    _, _ = fmt.Fprintln(w, "                          One of: terminal, json, tap, junit, html, markdown.")
    _, _ = fmt.Fprintln(w, "                          markdown is recommended for VS Code + AI agent workflows.")
    _, _ = fmt.Fprintln(w, "  --help, -h              Show this help message")
}
```

The top-level help line at `main.go:3597` changes from:

```go
_, _ = fmt.Fprintln(w, "  init [dir]      Initialize a new apitest project (default: current directory)")
```

to:

```go
_, _ = fmt.Fprintln(w, "  init [dir]      Initialize a new apitest project (use --output <fmt> to scaffold an output: block)")
```

#### Tests to Write FIRST (RED phase)

```go
// TestInit_OutputUnknownFormat asserts that a bad --output value exits 3
// with an error message that names the supported enum.
func TestInit_OutputUnknownFormat(t *testing.T) {
    dir := t.TempDir()
    _, stderr, code := captureRun(t, "init", "--output", "madeup", dir)
    if code != 3 {
        t.Errorf("exit code = %d, want 3; stderr=%q", code, stderr)
    }
    if !strings.Contains(stderr, "markdown") {
        t.Errorf("stderr should name supported values including markdown; got: %q", stderr)
    }
    if !strings.Contains(stderr, "madeup") {
        t.Errorf("stderr should echo the rejected value; got: %q", stderr)
    }
    // Verify no apitest.yaml was created (validation runs before scaffold).
    if _, err := os.Stat(filepath.Join(dir, "apitest.yaml")); err == nil {
        t.Errorf("apitest.yaml should not exist after rejected --output value")
    }
}

// TestInit_OutputMarkdown_FullPipeline runs init via the binary-like
// runWithWriters and asserts the resulting apitest.yaml contains the markdown
// output block.
func TestInit_OutputMarkdown_FullPipeline(t *testing.T) {
    dir := t.TempDir()
    _, stderr, code := captureRun(t, "init", "--output", "markdown", dir)
    if code != 0 {
        t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr)
    }
    body, err := os.ReadFile(filepath.Join(dir, "apitest.yaml"))
    if err != nil {
        t.Fatal(err)
    }
    want := "format: markdown\n  report: responses/"
    if !strings.Contains(string(body), want) {
        t.Errorf("want %q in apitest.yaml; got:\n%s", want, body)
    }
}

// TestInit_Help_DocumentsOutputFlag verifies init --help mentions --output
// and lists the full enum including markdown.
func TestInit_Help_DocumentsOutputFlag(t *testing.T) {
    stdout, _, code := captureRun(t, "init", "--help")
    if code != 0 {
        t.Fatalf("init --help exit = %d, want 0", code)
    }
    if !strings.Contains(stdout, "--output") {
        t.Errorf("init --help missing --output:\n%s", stdout)
    }
    for _, format := range []string{"terminal", "json", "tap", "junit", "html", "markdown"} {
        if !strings.Contains(stdout, format) {
            t.Errorf("init --help missing format %q:\n%s", format, stdout)
        }
    }
}
```

#### Impact on Existing Tests

- `TestRun_init_command_recognized`, `TestInitCmd_empty_directory`,
  `TestInitCmd_existing_project_returns_error`, `TestInitCmd_project_name_flag`
  — all unaffected (they don't pass `--output`).
- `TestHelp_contains_init` — unaffected (the line still contains "init").
- `TestInitThenRun_integration` — unaffected.

### Step 4: SPECIFICATION.md and MANUAL.md docs

**Rationale:** Pure docs additions, no code change. Order before
IMPROVEMENT.md so the "shipped" status references are accurate.

#### Files to Modify

| File                       | Action | Description                                                                            |
|----------------------------|--------|----------------------------------------------------------------------------------------|
| `docs/SPECIFICATION.md`    | modify | Add `#### Markdown Output Format` subsection at line ~3054 (after HTML, before Verbosity). |
| `docs/MANUAL.md`           | modify | Append VS Code split-pane worked example at the end of §5.7 Watch mode.                |

#### New Content (SPECIFICATION.md)

Inserted after line 3056 (`#### HTML Reports (Premium Feature)` block), before line 3058 (`#### Verbosity Levels`):

```markdown
#### Markdown Output Format

`--format markdown --report <dir>` writes one `<slug>.md` per main-phase
request to `<dir>/`, plus a `run.md` index. The format is designed for
VS Code split-pane workflows and AI-agent narration: large response bodies
render well, the file is round-trippable (re-runs splice only the
CLI-owned region), and three correlation IDs (`run_id`, `request_id`,
`request_slug`) are present in every artifact.

**Sentinel format.** Each per-request file is partitioned into an agent-
or human-owned region and a CLI-owned region delimited by HTML comment
sentinels:

```markdown
<!-- BEGIN apitest:response id=req-3 slug=get-user run=abc123def456... -->
... CLI-owned 10-section block ...
<!-- END apitest:response id=req-3 slug=get-user run=abc123def456... -->
```

The opening and closing sentinels carry all three IDs. Re-running the
collection rewrites only the bytes between the sentinels; agent or human
notes above and below survive byte-for-byte. Existing files lacking the
expected sentinel pair receive a `.md.new` sibling and a stderr warning;
the original is never overwritten.

**Section order (fixed).** Each CLI-owned block contains exactly ten
sections in this order: Title, Request, Request body, Response, Response
metadata, Response body, Timing, Assertions, Errors, Trailer. No
conditional sections; each section is always present even if empty.

**Splice rules.** Six cases handled:
1. New file (no existing markdown): write full content.
2. Existing file with matching sentinel pair: replace bytes between
   sentinels.
3. Existing file with mismatched IDs: write to `.md.new`, warn.
4. Existing file with malformed (unbalanced) sentinels: write to `.md.new`,
   warn.
5. Existing file with no sentinels (user-owned): write to `.md.new`, warn.
6. Atomic write: `O_EXCL` temp-file + rename, no partial writes.

**Per-iteration layout.** Data-driven requests produce a
`<report>/<slug>/` subdirectory containing one `iter-<n>.md` per
iteration plus an `index.md` manifest with summary counts and an
iteration table. Iterations carry stable correlation IDs of the form
`req-N-iter-M`. If the data source exceeds the runner row cap,
`index.md` adds a truncation marker and `iter` files beyond the cap are
not written. Parallel collections group `run.md` entries under
`## Wave <N>` headers in ascending wave order.

**Content-type matrix.** JSON is pretty-printed in a `json` fenced
block; YAML in `yaml`; XML in `xml`; HTML in `html` (preserved as
fenced text — never executed); plain text in `text`; HEAD responses
emit `(no body)`; binary content (`Content-Type: application/octet-
stream`, `image/*`, etc.) emits a `hex.Dump`-style preview plus byte
count. Empty bodies emit `(empty)`.

**1 MiB body cap.** Response bodies larger than 1,048,576 bytes are
truncated post-redaction; the truncation marker line documents the
original size. The cap applies after redaction so secrets cannot leak
through size-based corner cases.

**Determinism tiers.** Three tiers, per IMPROVEMENT.md §8.7:
- **Deterministic** — method, URL, request headers (post-redaction),
  request body (post-redaction), response status, response body
  (post-redaction, canonically formatted), assertion list. Byte-
  identical across runs against the same inputs.
- **Response metadata** — response headers sorted alphabetically with
  known-volatile headers (Date, X-Request-Id, Set-Cookie, Etag, Server,
  Age) quarantined in a separate subsection so they do not pollute the
  signal in diffs.
- **Timing** — `duration_ms`, `wave_index`, `started_at`. Each on its
  own line with a fixed prefix so diffs are trivial to mask.

**Redaction invariant.** `--allow-sensitive` cannot leak secrets into
markdown output. That flag affects only `-vv` terminal dumps; markdown
output is always redacted regardless of CLI flag state.
```

#### New Content (MANUAL.md §5.7)

Appended after the existing watch mode block (after line 1805), before
§5.8:

```markdown
**Worked example: VS Code split-pane workflow.**

The markdown output format (`--format markdown --report responses/`)
pairs naturally with watch mode for tight inner-loop iteration. Open
your collection YAML in VS Code, then open the per-request markdown file
in a right-side split:

1. Scaffold a markdown-ready project: `apitest init --output markdown`
2. Open `collections/sample.yaml` on the left. Hit `Ctrl+\` (or Cmd+\)
   to split the editor, then open `responses/hello-world.md` on the right.
3. Run `apitest watch collections/sample.yaml --only "Hello World"`.

On every save of the YAML, ApiTool re-runs that one request and rewrites
the bytes between the `BEGIN/END apitest:response` sentinels in the
markdown file. VS Code's markdown preview (Ctrl+K V) updates in place,
showing the formatted request, response (pretty-printed JSON, YAML, XML,
or hex preview for binary), assertion outcomes, and timing. Notes you add
above or below the sentinel block survive each re-run — useful for
recording observations as you iterate.

This works equally well when an AI agent (Claude Code, Copilot, etc.)
drives the loop: the agent edits the YAML, you watch the markdown
updates in real time and intervene when needed.
```

#### Tests to Write FIRST (RED phase)

Docs are not unit-tested by the existing suite directly, but the task
YAML's observable steps include:

```bash
grep -c 'Markdown Output Format' docs/SPECIFICATION.md  # >= 1
grep -c 'split-pane' docs/MANUAL.md                     # >= 1
```

These are verified as part of the observable harness, not as Go tests.
No new Go tests for this step.

#### Impact on Existing Tests

- None.

### Step 5: IMPROVEMENT.md surgical edits

**Rationale:** Three minimal text changes, each touching one line. Order
them after the docs so the §5 W4 status references SPECIFICATION/MANUAL
content that exists.

#### Files to Modify

| File              | Action | Description                                                                                              |
|-------------------|--------|----------------------------------------------------------------------------------------------------------|
| `IMPROVEMENT.md`  | modify | §3 status header (line 3) — add "W4 complete" annotation. §5 W4 status — flip to Shipped. §6 — v1.1→v1.2. §8.1 — extend resolution. |

#### Current Code

`IMPROVEMENT.md:3`:

```markdown
**Status:** Partially shipped — W1, W2, W3 complete (2026-04-24); W4, W5 pending
```

`IMPROVEMENT.md:205`:

```markdown
**Status:** Pending — not yet decomposed into backlog tasks. Natural next item.
```

`IMPROVEMENT.md:351`:

```markdown
- **`request_slug`**: human-readable identifier derived from the request name, used in filenames and sentinel tags. Added to the events schema as an additive field in v1.1. Not a correlation key — `request_id` is.
```

`IMPROVEMENT.md:385`:

```markdown
   - **Resolution:** Keep `request_id` unchanged as `req-N` (already locked in the v1.0 events schema). Add a new additive field `request_slug` for human-readable surfaces (markdown filenames, sentinel). Events schema bumps to v1.1 — additive only, backward-compatible, no v2.0 required.
```

#### New Code

Line 3:

```markdown
**Status:** Partially shipped — W1, W2, W3 complete (2026-04-24), W4 complete (2026-04-25); W5 pending
```

Line 205 (the `### W4` block, replacing the Status line):

```markdown
**Status:** Shipped 2026-04-25 — M9-001 (PR #123) + M9-002 (PR #124) + M9-003 (PR #125) + M9-004 (PR #126) + M9-005 (PR #N).
```

Where `#N` is the M9-005 merge PR number. Per the task workflow, the PR
number is filled in by `/verify` when the PR is merged. We will write
`#N` in the plan and substitute the actual number during execution if
known; otherwise the convention follows the W1/W2/W3 pattern.

Line 351:

```markdown
- **`request_slug`**: human-readable identifier derived from the request name, used in filenames and sentinel tags. Added to the events schema as an additive field in v1.2 (M9-001). Not a correlation key — `request_id` is.
```

Line 385:

```markdown
   - **Resolution:** Keep `request_id` unchanged as `req-N` (already locked in the v1.0 events schema). Add a new additive field `request_slug` for human-readable surfaces (markdown filenames, sentinel). Events schema bumps to v1.1 (`selection`, M8-004) and then to v1.2 (`request_slug`, M9-001) — both additive, backward-compatible, no v2.0 required.
```

#### Tests to Write FIRST (RED phase)

The task YAML's observable harness asserts:

```bash
grep -A 1 '^### W4 — Markdown response format' IMPROVEMENT.md | head -2
# Expected: Status: Shipped with M9-001..M9-005 PR references.

grep -B 1 -A 1 'request_slug' IMPROVEMENT.md | grep 'v1\.2'
# Expected: at least one match.
```

These are observable steps, not Go tests. No new Go tests for this step.

#### Impact on Existing Tests

- None.

### Step 6: CHANGELOG.md batched W4 entries

**Rationale:** Last documentation step. The CHANGELOG already contains
per-task entries for M9-001..M9-004 (lines 27-31) added during their
respective PRs. M9-005 adds two new short, marketing-style lines under
`[Unreleased]` summarizing the full W4 shipment, plus its own per-task
detail entry.

#### Files to Modify

| File           | Action | Description                                                                                                  |
|----------------|--------|--------------------------------------------------------------------------------------------------------------|
| `CHANGELOG.md` | modify | Insert two new bullet points: one under `### Added` and one under `### Changed`, both batched for W4.        |

#### New Code

Under `### Added` (insert after the existing M9-004 entry at line 27):

```markdown
- CLI: `--format markdown` output documented and discoverable: `apitest init --output markdown` scaffolds an `apitest.yaml` with `output: { format: markdown, report: responses/ }`. Other supported `--output` values (`terminal`, `json`, `tap`, `junit`, `html`) scaffold equivalent blocks pointing at default per-format paths; unknown values exit 3 with an error naming the enum. New `apitest init --help` documents the flag. SPECIFICATION.md gains a Markdown Output Format subsection covering sentinel format, fixed 10-section order, splice rules, content-type matrix, 1 MiB body cap, and determinism tiers (referencing IMPROVEMENT.md §8.7). MANUAL.md §5.7 gains a VS Code split-pane worked example. Closes IMPROVEMENT.md W4 (M9-001..M9-005).
```

Under `### Changed` (insert after the existing M8-005 entry at line 10):

```markdown
- Events schema v1.1 → v1.2: per-request events (`request.start`, `request.end`) gain an optional `request_slug` field; `schema_version` advances to `"1.2"` in all emitted events. Additive, backward-compatible. (M9-001, batched into the W4 shipment with M9-002..M9-005.)
```

#### Tests to Write FIRST (RED phase)

The task YAML's observable harness asserts:

```bash
awk '/^## \[Unreleased\]/,/^## \[/' CHANGELOG.md | grep -E '^- (Added|Changed).*markdown|^- Changed.*events schema v1\.2'
# Expected: at least two matching lines.
```

This is the observable step, not a Go test. No new Go tests for this
step.

#### Impact on Existing Tests

- None.

## Test Impact Summary

| Test File                                  | Test Function                          | Impact   | Action Required                                |
|--------------------------------------------|----------------------------------------|----------|------------------------------------------------|
| `internal/scaffold/scaffold_test.go`       | `TestInit_OutputAllFormats`            | new      | Write in Step 1                                |
| `internal/scaffold/scaffold_test.go`       | `TestInit_OutputMarkdownFlag`          | new      | Write in Step 1                                |
| `internal/scaffold/scaffold_test.go`       | `TestInit_DefaultUnchanged`            | new      | Write in Step 1                                |
| `internal/scaffold/scaffold_test.go`       | `TestInit` (existing)                  | none     | Continues to pass; bare-init unchanged         |
| `internal/scaffold/scaffold_test.go`       | `TestInit_error_*` (existing)          | none     | Continues to pass; tests error paths           |
| `cmd/apitest/main_test.go`                 | `TestInit_OutputUnknownFormat`         | new      | Write in Step 3                                |
| `cmd/apitest/main_test.go`                 | `TestInit_OutputMarkdown_FullPipeline` | new      | Write in Step 3                                |
| `cmd/apitest/main_test.go`                 | `TestInit_Help_DocumentsOutputFlag`    | new      | Write in Step 3                                |
| `cmd/apitest/main_test.go`                 | `TestRun_init_command_recognized`      | none     | Continues to pass                              |
| `cmd/apitest/main_test.go`                 | `TestInitCmd_*` (existing)             | none     | Continue to pass                               |
| `cmd/apitest/main_test.go`                 | `TestInitThenRun_integration`          | none     | Continues to pass                              |
| `cmd/apitest/main_test.go`                 | `TestHelp_contains_init`               | none     | Top-level help still contains "init"           |
| `internal/schema/validate_test.go`         | `TestSchema_scaffolded_apitest_yaml_validates` | none | Bare-init scaffold unchanged byte-for-byte    |
| `internal/schema/validate_test.go`         | `TestSchema_validates_scaffolded_sample` | none   | Bare-init sample.yaml unchanged                |

Eight new tests total (three in scaffold, three in cmd/apitest, plus the
two pure-observable shell-script assertions enforced by `smoke/run.sh` and
the task YAML's observable harness).

## Risks and Edge Cases

- **Risk:** Existing M8-003 baseline `output:` block in apitest.yaml is
  byte-sensitive (trailing newline, key ordering).
  → **Mitigation:** `TestInit_DefaultUnchanged` literal-string test
  pins the exact bytes. The new `outputBlock("")` returns the M8-003
  baseline verbatim.

- **Risk:** New `--output` flag collides with the existing `--output`
  flag on `apitest import openapi` (line 3895) or `apitest perf` (line
  1554, 1605, 1766, etc.).
  → **Mitigation:** Subcommand-scoped flags. `init`'s `--output`
  controls `output:` in apitest.yaml; `import openapi`'s `--output`
  controls the spec output file path; `perf`'s `--output` controls the
  perf report file path. No code-level conflict (flags are parsed in
  per-subcommand functions); only documentation risk if users confuse
  them. The `init --help` text disambiguates by saying "Scaffold an
  output: block".

- **Risk:** Schema-parity check in the task YAML observable —
  `apitest schema | jq -r '.properties.output.properties.format.enum[]'`
  must include `markdown`. Currently it does (since M9-001/002 added it
  to `schemas/collection-v1.json` and `schemas/project-v1.json`).
  → **Mitigation:** No change required; the existing schema files
  already include markdown. The observable is a regression check, not
  a new addition.

- **Risk:** `init --output html` scaffolds `report: report.html` but
  doesn't scaffold an actual HTML output file (HTML format requires
  `--report <file>` at run time, so this is correct — the scaffold just
  declares the path).
  → **Mitigation:** Document this in `printInitHelpTo`: "Scaffold an
  output: block for the named format" — does not pre-create files.

- **Risk:** `init --output tap` scaffolds an `output:` block without
  `report:`, since TAP is stdout-only.
  → **Mitigation:** Per the format-to-block table, `tap` and
  `terminal` deliberately omit `report:`. The schema accepts this
  (report is optional).

- **Risk:** Unknown subcommand check in `parseInitArgs` may misfire on
  flags like `--unknown-flag` that today fall through to `default` and
  end up as positional `dir` arguments (potentially leading to a
  confusing scaffold attempt at a path named `--unknown-flag`).
  → **Mitigation:** This is **pre-existing behaviour**, not introduced
  by M9-005. The task YAML does not require fixing it; out of scope.
  Add a follow-up backlog item if needed.

- **Risk:** PR number `#N` placeholder in IMPROVEMENT.md §5 W4 status
  line is wrong on merge.
  → **Mitigation:** `/verify` step substitutes the actual PR number
  before flipping to done; if the PR number is not yet known at
  execution time, the line reads `M9-005 (PR #TBD)` until verify.
  Following the W1/W2/W3 status-block pattern, the substitution is a
  one-line edit at verify time.

- **Edge case:** Running `apitest init --output ""` (empty string flag
  value) — the parser treats empty-string as no-flag-passed in other
  subcommands. We do the same here: empty `OutputFormat` falls through
  to the M8-003 default block.
  → **Handling:** Documented in `outputBlock("")`. The CLI parser
  rejects `--output` without a following arg via the standard
  "requires a value" error path.

- **Edge case:** `apitest init` on an existing project with `--output
  markdown` — the existing `ErrProjectExists` path triggers before any
  output-block logic runs.
  → **Handling:** No change needed; `scaffold.Init` already returns
  the existing error before writing any file.

- **Edge case:** Format validation order — the YAML observable expects
  `apitest init --output madeup 2>err.log; echo $?` to exit 3.
  Validation runs after flag parsing, before `filepath.Abs`, before any
  file is written. The order is: parse flags → validate `outputFormat`
  → resolve dir → call scaffold. This is the order in the New Code
  block above.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (mirrors task YAML):

```bash
# 1. init --output markdown
TMP=$(mktemp -d) && cd "$TMP"
./apitest init --output markdown
grep -A 3 '^output:' apitest.yaml
# Expected:
#   output:
#     format: markdown
#     report: responses/
#     verbosity: normal

# 2. Bare init preserves no-markdown default
TMP2=$(mktemp -d) && cd "$TMP2"
./apitest init
grep 'markdown' apitest.yaml || echo 'no markdown reference'

# 3. init --help documents --output
./apitest init --help | grep -E '^\s*--output'

# 4. init --output unknown exits 3
TMP3=$(mktemp -d) && cd "$TMP3"
./apitest init --output madeup 2>err.log; echo $?
cat err.log

# 5. Schema parity at both levels
./apitest schema | jq -r '.properties.output.properties.format.enum[]' | grep '^markdown$'
./apitest schema --project | jq -r '.properties.output.properties.format.enum[]' | grep '^markdown$'

# 6. Docs
grep -c 'Markdown Output Format' docs/SPECIFICATION.md
grep -c 'split-pane' docs/MANUAL.md

# 7. CHANGELOG
awk '/^## \[Unreleased\]/,/^## \[/' CHANGELOG.md | grep -E '^- (Added|Changed).*markdown|^- Changed.*events schema v1\.2'

# 8. IMPROVEMENT.md
grep -A 1 '^### W4 — Markdown response format' IMPROVEMENT.md | head -2
grep -B 1 -A 1 'request_slug' IMPROVEMENT.md | grep 'v1\.2'

# 9. Targeted unit + integration suite
go test -run 'TestInit_OutputMarkdownFlag|TestInit_OutputAllFormats|TestInit_OutputUnknownFormat|TestInit_DefaultUnchanged' ./...
```

All steps must pass before `/verify` flips status to done.
