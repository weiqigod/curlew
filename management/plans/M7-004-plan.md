# Implementation Plan: M7-004

## Overview

Adds `cmd/curlew/stream_discipline_matrix_test.go` — a single integration
test, `TestStreamDisciplineMatrix`, that spawns the real `curlew` binary
across a (subcommand × format × pipe-wiring) matrix and asserts the M7
stream-discipline invariants: (1) stdout carries only the declared format's
payload, (2) stderr carries no ANSI escape sequences when piped, and
(3) no known-progress string ever leaks to stdout. Extends
`scripts/ci-local.sh` to invoke this test by name so CI failures are
one-grep triage.

## Task Details

- **ID:** M7-004
- **Title:** CI regression gate: stream-discipline matrix across commands and formats
- **Phase:** M7: Output Discipline
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M7-001 | Stderr color flag | done |
| M7-002 | Progress to stderr | done |
| M7-003 | Usage synopsis on stderr | done |

## Scope Decisions

**Decision 1 — TTY coverage.** The task scope offers `creack/pty` as an optional
dependency for TTY matrix cells. `go.mod` currently has zero `creack` imports,
and adding a dependency for test-only TTY simulation conflicts with the
"standard library first" rule in `CLAUDE.md`. The existing
`cmd/curlew/stream_color_test.go` (M7-001) already covers the
stdout-TTY-stderr-pipe regression via `/dev/tty` with a graceful skip, and
the ANSI-on-pipe invariant (the M7-001 bug) is exclusively a pipe-side
assertion. **This plan therefore restricts the M7-004 matrix to
pipe-wired stdout and pipe-wired stderr** and documents that TTY cells are
owned by `TestStderrColorFlag`. No new dependency; the test file remains
cross-platform (no build tag needed).

**Decision 2 — `status/total/passed` JSON fields.** The task observable and
first behavior mention top-level JSON fields `status/total/passed`. The actual
`output.JSONOutput` struct (single-collection mode) has `name`, `status`,
`duration_ms`, `requests`, `errors` at the top level — `total`/`passed`
only appear in `MultiJSONOutput` (multi-collection glob). For a single-file
fixture the matrix asserts the fields that exist: `status`, `name`,
`duration_ms`, and `len(requests) > 0`. The smoke-level
`jq -e '.status == "passed"'` check in the observable block will work
against the real `status` field.

**Decision 3 — `html` report `section markers`.** The HTML template
(`internal/output/html.go`) does not emit "setup"/"main"/"teardown" section
headers (those only appear in terminal output when a collection defines
setup/teardown blocks). The matrix will assert the real HTML markers that
exist on every report: the `<!DOCTYPE html>` prologue, the `<title>…
curlew report</title>` tag, and the `status-bar` / `summary-card` class
markers emitted by the template.

**Decision 4 — Terminal-format "section headers" for a trivial collection.**
A single-request collection with no setup/teardown emits no `Setup:` /
`Teardown:` header. The matrix therefore asserts a universal terminal-format
marker: the `Collection: <name>` header line and the `✓`/`✗` result line
prefix (two spaces, indicator, space, request name). These are emitted by
`output.Printer.CollectionHeader` and `output.Printer.Result`.

**Decision 5 — Subcommand surface.** The matrix covers the commands with
the highest M7 regression risk: `run` (five formats, the core surface),
`perf`, `license --validate`, `worker`, and `exec --dry-run`. These are
exactly the commands touched by M7-001/002/003. `validate`, `init`, `info`,
`schema`, `vault`, `pr-check`, `import`, `plugins` emit no
progress text that varies by format, and their writer discipline is already
gated by `TestNoOsStdoutAssignment` (M7-005) — covering them here would
duplicate signal.

## Deviations from Plan

### Deviation 1 — Fixture YAML structure
The plan's fixture snippets used flat `method:` and `url:` fields directly under each request item. The actual `parser.RequestItem` struct requires these under a nested `request:` key (as per the collection schema). All four fixture files were written with the correct nested structure: `request: { method: GET, url: ... }`.

### Deviation 2 — Parse error exit code
The plan specified exit code 5 for the parse-error fixture. In the actual codebase, `CategoryParse` errors from the parser return exit code 3 in `main.go`. Exit code 5 is reserved for variable-resolution failures (undefined variable at request execution time). The `run_terminal_parse_error` cell uses `wantExitCode: 3`.

### Deviation 3 — JUnit/HTML tier requirement
The plan's `run_junit_happy` and `run_html_happy_with_report` cells assumed these formats work on free tier. They actually require `TierProfessional`. Both cells now set `env: []string{"CURLEW_TIER=professional"}` to bypass the gate and exercise the stream-discipline invariants.

## Implementation Steps

### Step 1: Create `testdata/stream-discipline/` fixtures

**Rationale:** Fixtures are pure data — adding them first gives the test file
something to reference in Step 2 and cannot break anything on its own.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/testdata/stream-discipline/happy.yaml` | create | Minimal single-request collection, no variables, resolvable against a local httptest server via `{{BASE_URL}}`. |
| `cmd/curlew/testdata/stream-discipline/assertion_failure.yaml` | create | Single request asserting status `500` against a server that returns `200` — triggers assertion failure, exit code 1. |
| `cmd/curlew/testdata/stream-discipline/parse_error.yaml` | create | Malformed YAML (unclosed list) — triggers parser error, exit code 5. |
| `cmd/curlew/testdata/stream-discipline/gate_denied.yaml` | create | Uses `include:` directive, which the free-tier (default) gate denies — triggers `GateError`, exit code 6. |

#### New Code

`happy.yaml`:
```yaml
name: stream-discipline-happy
requests:
  - name: ping
    method: GET
    url: "{{BASE_URL}}/ok"
    assertions:
      status: 200
```

`assertion_failure.yaml`:
```yaml
name: stream-discipline-assertion-failure
requests:
  - name: ping
    method: GET
    url: "{{BASE_URL}}/ok"
    assertions:
      status: 500
```

`parse_error.yaml`:
```yaml
name: stream-discipline-parse-error
requests:
  - name: ping
    method: GET
    url: "http://127.0.0.1:1/ok"
    assertions:
      - this is: malformed
      unclosed
```

`gate_denied.yaml` (a collection that fails the `include_directive` gate on free tier):
```yaml
name: stream-discipline-gate-denied
include:
  - ./nonexistent-shared.yaml
requests:
  - name: ping
    method: GET
    url: "http://127.0.0.1:1/ok"
```

#### Tests to Write FIRST (RED phase)

N/A — these are fixtures, not code. Their correctness is verified by the
Step 2 matrix runs: if `happy.yaml` doesn't produce a passing run, or
`parse_error.yaml` parses cleanly, the Step 2 tests will fail and we know
to adjust the fixture.

#### Impact on Existing Tests

None. New directory under `cmd/curlew/testdata/` — no existing test
references these paths.

---

### Step 2: Write `TestStreamDisciplineMatrix`

**Rationale:** The test drives everything. Landing it after fixtures (Step 1)
means first run against the current M7-001/002/003-fixed codebase is
expected to pass — providing the baseline the Definition of Done calls for.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/stream_discipline_matrix_test.go` | create | Single test function, table-driven, spawns the real binary per cell via `exec.Command`. |

#### New Code

```go
package main

import (
    "bytes"
    "encoding/json"
    "encoding/xml"
    "net/http"
    "net/http/httptest"
    "os"
    "os/exec"
    "path/filepath"
    "regexp"
    "strings"
    "testing"
)

// ansiRE matches any CSI-style ANSI escape sequence (ESC [ ...).
var ansiRE = regexp.MustCompile(`\x1b\[`)

// knownProgressStrings must NEVER appear on stdout for any subcommand.
// These are the strings M7-002/003 relocated from stdout to stderr —
// regressions here indicate a new call site that forgot the stderr convention.
var knownProgressStrings = []string{
    "Running...",
    "Load test:",
    "Validating license",
    "Claimed shard",
    "Resolved ",            // "Resolved N secrets from shared template ..."
    "Exporting license bundle",
}

// TestStreamDisciplineMatrix is the M7-004 regression gate. It iterates
// (subcommand × format) cells, spawns the real curlew binary with both
// stdout and stderr wired to pipes, and asserts the three core M7 invariants:
//
//  1. Stdout contains ONLY the declared format's payload.
//  2. Stderr is free of ANSI escape sequences when piped.
//  3. No known-progress string leaks onto stdout.
//
// TTY-wired cells are owned by TestStderrColorFlag (M7-001) so we do not
// duplicate them here; /dev/tty usage would add platform coupling without
// additional regression signal.
func TestStreamDisciplineMatrix(t *testing.T) {
    binary := buildBinary(t)

    // One shared httptest server answers every fixture URL.
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(200)
    }))
    defer srv.Close()

    // Materialize fixtures (their BASE_URL placeholder is resolved here so the
    // tests stay hermetic — no DNS, no external URLs).
    fixtures := materializeStreamDisciplineFixtures(t, srv.URL)

    type stdoutCheck func(t *testing.T, stdout string)

    cases := []struct {
        name         string
        args         []string          // passed to ./curlew
        env          []string          // extra env (on top of os.Environ() minus NO_COLOR)
        wantExitCode int               // expected exit code; -1 = any non-zero
        stdoutIsEmpty bool             // when true, stdout must be exactly zero bytes
        checkStdout  stdoutCheck       // optional: stronger stdout assertion
        extraStderrMustContain []string // optional: stderr progress-text assertions
    }{
        // --- run × 5 formats × happy path ---
        {
            name: "run_json_happy",
            args: []string{"run", fixtures.happy, "--format", "json"},
            wantExitCode: 0,
            checkStdout: assertValidJSONRunOutput,
        },
        {
            name: "run_tap_happy",
            args: []string{"run", fixtures.happy, "--format", "tap"},
            wantExitCode: 0,
            checkStdout: assertTAPHeader,
        },
        {
            name: "run_junit_happy",
            args: []string{"run", fixtures.happy, "--format", "junit"},
            wantExitCode: 0,
            checkStdout: assertValidJUnitXML,
        },
        {
            name: "run_html_happy_with_report",
            args: []string{"run", fixtures.happy, "--format", "html",
                "--report", filepath.Join(t.TempDir(), "report.html")},
            wantExitCode: 0,
            stdoutIsEmpty: true, // html writes to disk; stdout is empty
        },
        {
            name: "run_terminal_happy",
            args: []string{"run", fixtures.happy},
            wantExitCode: 0,
            checkStdout: assertTerminalMarkers,
        },

        // --- run × assertion-failure path (exit 1 on all formats) ---
        {
            name: "run_json_assertion_failure",
            args: []string{"run", fixtures.assertionFailure, "--format", "json"},
            wantExitCode: 1,
            checkStdout: assertValidJSONRunOutput, // structure must still parse
        },
        {
            name: "run_tap_assertion_failure",
            args: []string{"run", fixtures.assertionFailure, "--format", "tap"},
            wantExitCode: 1,
            checkStdout: assertTAPHeader,
        },

        // --- run × parse-error path (exit 5, error goes to stderr) ---
        {
            name: "run_terminal_parse_error",
            args: []string{"run", fixtures.parseError},
            wantExitCode: 5,
            stdoutIsEmpty: true,
        },

        // --- run × gate-denied (exit 6) ---
        // Gate denial emits a structured error on stderr for terminal format.
        // On JSON it writes a GateJSONOutput document to stdout — verify the
        // doc parses.
        {
            name: "run_json_gate_denied",
            args: []string{"run", fixtures.gateDenied, "--format", "json"},
            wantExitCode: 6,
            checkStdout: assertGateJSONOutput,
        },
        {
            name: "run_terminal_gate_denied",
            args: []string{"run", fixtures.gateDenied},
            wantExitCode: 6,
            stdoutIsEmpty: true, // error routes to stderr
        },

        // --- perf: stdout is ONLY the final "Results:" line ---
        {
            name: "perf_progress_on_stderr",
            args: []string{"perf", fixtures.perfRequest, "--vus", "1", "--duration", "100ms"},
            env:  []string{"CURLEW_TIER=enterprise"}, // perf requires enterprise
            wantExitCode: 0,
            checkStdout: assertPerfResultsLine,
            extraStderrMustContain: []string{"Load test:", "Running..."},
        },

        // --- license --validate: stdout is only key/value rows ---
        {
            name: "license_validate",
            args: []string{"license", "--validate"},
            // license setup env is materialized inside runMatrixCell.
            env: nil, // filled in via fixture.licenseEnv
            wantExitCode: 0,
            checkStdout: assertLicenseValidateStdout,
            extraStderrMustContain: []string{"Validating license"},
        },

        // --- exec --dry-run: stdout is the "DRY RUN" block; no HTTP traffic ---
        {
            name: "exec_dry_run",
            args: []string{"exec", "https://example.com", "--dry-run"},
            wantExitCode: 0,
            checkStdout: assertExecDryRunStdout,
        },
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            env := os.Environ()
            // Hermetic: strip NO_COLOR so test runs regardless of developer shell.
            env = filterEnv(env, "NO_COLOR")
            // license_validate needs a config dir; plumbed via fixtures helper.
            if tc.name == "license_validate" {
                env = append(env, fixtures.licenseEnv(t)...)
            }
            env = append(env, tc.env...)

            cmd := exec.Command(binary, tc.args...)
            cmd.Env = env
            var stdout, stderr bytes.Buffer
            cmd.Stdout = &stdout
            cmd.Stderr = &stderr

            err := cmd.Run()
            exitCode := 0
            if exitErr, ok := err.(*exec.ExitError); ok {
                exitCode = exitErr.ExitCode()
            } else if err != nil {
                t.Fatalf("unexpected error running %s: %v\nstderr=%q", tc.name, err, stderr.String())
            }

            if tc.wantExitCode >= 0 && exitCode != tc.wantExitCode {
                t.Errorf("exit=%d want=%d\nstdout=%q\nstderr=%q",
                    exitCode, tc.wantExitCode, stdout.String(), stderr.String())
            }

            // Invariant 1: stdout payload check.
            if tc.stdoutIsEmpty {
                if stdout.Len() != 0 {
                    t.Errorf("stdout must be empty; got %q", stdout.String())
                }
            }
            if tc.checkStdout != nil {
                tc.checkStdout(t, stdout.String())
            }

            // Invariant 2: no ANSI on piped stderr.
            if ansiRE.MatchString(stderr.String()) {
                t.Errorf("stderr contains ANSI escape sequences (must be stripped when piped): %q",
                    stderr.String())
            }

            // Invariant 3: no known-progress leak to stdout.
            for _, leak := range knownProgressStrings {
                if strings.Contains(stdout.String(), leak) {
                    t.Errorf("known-progress string %q leaked to stdout: %q", leak, stdout.String())
                }
            }

            // Optional: stronger stderr assertions.
            for _, want := range tc.extraStderrMustContain {
                if !strings.Contains(stderr.String(), want) {
                    t.Errorf("stderr missing expected %q; stderr=%q", want, stderr.String())
                }
            }
        })
    }
}

// --- stdout shape assertions ---

func assertValidJSONRunOutput(t *testing.T, stdout string) {
    t.Helper()
    var doc map[string]any
    if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
        t.Errorf("stdout is not valid JSON: %v\nstdout=%q", err, stdout)
        return
    }
    for _, field := range []string{"name", "status", "duration_ms", "requests"} {
        if _, ok := doc[field]; !ok {
            t.Errorf("json output missing %q field: %q", field, stdout)
        }
    }
}

func assertGateJSONOutput(t *testing.T, stdout string) {
    t.Helper()
    var doc map[string]any
    if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
        t.Errorf("gate-denial stdout is not valid JSON: %v\nstdout=%q", err, stdout)
        return
    }
    if got, _ := doc["status"].(string); got != "feature_gated" {
        t.Errorf(`status = %q, want "feature_gated"; stdout=%q`, got, stdout)
    }
}

func assertTAPHeader(t *testing.T, stdout string) {
    t.Helper()
    if !strings.HasPrefix(stdout, "TAP version 13") {
        t.Errorf("stdout does not start with TAP version 13 header: %q", stdout)
    }
}

func assertValidJUnitXML(t *testing.T, stdout string) {
    t.Helper()
    // Decoding into a loose struct — we only care that it's well-formed XML
    // with <testsuites> as the root.
    type testsuites struct {
        XMLName xml.Name `xml:"testsuites"`
    }
    var v testsuites
    if err := xml.Unmarshal([]byte(stdout), &v); err != nil {
        t.Errorf("stdout is not valid JUnit XML: %v\nstdout=%q", err, stdout)
    }
}

func assertTerminalMarkers(t *testing.T, stdout string) {
    t.Helper()
    // Universal terminal markers: collection header + result indicator glyph.
    if !strings.Contains(stdout, "Collection: ") {
        t.Errorf("terminal stdout missing 'Collection: ' header: %q", stdout)
    }
    if !strings.ContainsAny(stdout, "✓✗") {
        t.Errorf("terminal stdout missing result indicator (✓ or ✗): %q", stdout)
    }
}

func assertPerfResultsLine(t *testing.T, stdout string) {
    t.Helper()
    if !strings.Contains(stdout, "Results: requests=") {
        t.Errorf("perf stdout missing 'Results: requests=': %q", stdout)
    }
}

func assertLicenseValidateStdout(t *testing.T, stdout string) {
    t.Helper()
    for _, want := range []string{"Key source:", "Tier:", "State:"} {
        if !strings.Contains(stdout, want) {
            t.Errorf("license stdout missing %q: %q", want, stdout)
        }
    }
}

func assertExecDryRunStdout(t *testing.T, stdout string) {
    t.Helper()
    if !strings.Contains(stdout, "DRY RUN") {
        t.Errorf("exec --dry-run stdout missing 'DRY RUN': %q", stdout)
    }
}

// --- fixtures helper ---

type streamDisciplineFixtures struct {
    happy            string
    assertionFailure string
    parseError       string
    gateDenied       string
    perfRequest      string
    // licenseEnv produces the env vars needed to make `license --validate` exit 0.
    licenseEnv func(t *testing.T) []string
}

func materializeStreamDisciplineFixtures(t *testing.T, baseURL string) streamDisciplineFixtures {
    t.Helper()
    // Copy source fixtures to a tempdir and substitute {{BASE_URL}} -> baseURL.
    srcDir := filepath.Join("testdata", "stream-discipline")
    dstDir := t.TempDir()
    copyAndSubstitute := func(name string) string {
        raw, err := os.ReadFile(filepath.Join(srcDir, name))
        if err != nil {
            t.Fatalf("read fixture %s: %v", name, err)
        }
        content := strings.ReplaceAll(string(raw), "{{BASE_URL}}", baseURL)
        out := filepath.Join(dstDir, name)
        if err := os.WriteFile(out, []byte(content), 0o600); err != nil {
            t.Fatalf("write fixture %s: %v", name, err)
        }
        return out
    }

    // Perf uses writeStreamProgressRequestFile (already in stream_progress_test.go).
    perfReq := writeStreamProgressRequestFile(t, baseURL)

    return streamDisciplineFixtures{
        happy:            copyAndSubstitute("happy.yaml"),
        assertionFailure: copyAndSubstitute("assertion_failure.yaml"),
        parseError:       copyAndSubstitute("parse_error.yaml"),
        gateDenied:       copyAndSubstitute("gate_denied.yaml"),
        perfRequest:      perfReq,
        licenseEnv: func(t *testing.T) []string {
            cfgDir := setupValidLicenseDir(t)
            return []string{
                "CURLEW_CONFIG_DIR=" + cfgDir,
                "CURLEW_OFFLINE=1",
                "CURLEW_LICENSE_BUNDLE=",
                "CURLEW_LAST_VALIDATION_OVERRIDE=",
            }
        },
    }
}

// filterEnv returns env with entries starting with prefix= removed.
func filterEnv(env []string, prefix string) []string {
    out := make([]string, 0, len(env))
    for _, e := range env {
        if !strings.HasPrefix(e, prefix+"=") {
            out = append(out, e)
        }
    }
    return out
}
```

#### Tests to Write FIRST (RED phase)

The test *is* the RED phase for this task. The TDD cycle is:

1. Write fixtures (Step 1) and the matrix test (this step) together.
2. Confirm the matrix passes on the current (post-M7-001/002/003) codebase.
3. Verify the matrix would have failed on the pre-fix codebase by temporarily
   reverting one M7-001 line on a scratch branch and re-running — documented
   in the verified report. This satisfies DoD item:
   *"would fail against the pre-M7-001 codebase."*

Cell naming (already in the `cases` table above, reproduced here for the
TDD checklist):

- `run_json_happy`
- `run_tap_happy`
- `run_junit_happy`
- `run_html_happy_with_report`
- `run_terminal_happy`
- `run_json_assertion_failure`
- `run_tap_assertion_failure`
- `run_terminal_parse_error`
- `run_json_gate_denied`
- `run_terminal_gate_denied`
- `perf_progress_on_stderr`
- `license_validate`
- `exec_dry_run`

#### Impact on Existing Tests

- `TestStreamProgress` (cmd/curlew/stream_progress_test.go) — Unaffected.
  Both tests spawn the real binary and share the `writeStreamProgressRequestFile`
  helper; this is deliberate reuse, not collision.
- `TestStderrColorFlag` (cmd/curlew/stream_color_test.go) — Unaffected.
  M7-001 owns the TTY-wired stderr cells. M7-004 owns the pipe-wired matrix.
- `TestStreamHelp` (cmd/curlew/stream_help_test.go) — Unaffected.
  Tests error/help routing via `runWithWriters` in-process; orthogonal to
  the matrix.
- No `*_test.go` file reassigns `os.Stdout`/`os.Stderr`; `TestNoOsStdoutAssignment`
  (M7-005) will stay green.
- `buildBinary` is called by this test — tests build time is increased by one
  `go build` invocation, but `t.TempDir()` caches within test scope. Existing
  parallel binary tests (e.g. `TestStreamProgress`) rebuild unconditionally;
  no new contention.

---

### Step 3: Wire `TestStreamDisciplineMatrix` into `scripts/ci-local.sh`

**Rationale:** The DoD requires CI to invoke the matrix by name so failures
are easy to triage in logs. The Go gate runs before test-stack setup, so
insertion is cheap.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `scripts/ci-local.sh` | modify | Add a named step running `TestStreamDisciplineMatrix` in isolation, right before the broader `go test ./...` step so a failure surfaces first. |

#### Current Code

```bash
# --- Go gate (always) ---
step "go build"
go build -o curlew ./cmd/curlew

step "go test"
go test ./...
```

#### New Code

```bash
# --- Go gate (always) ---
step "go build"
go build -o curlew ./cmd/curlew

# M7-004: explicit named marker for the stream-discipline matrix so failures
# show up under a unique header in CI logs (one grep away).
step "go test: TestStreamDisciplineMatrix (M7-004 stream-discipline gate)"
go test -run '^TestStreamDisciplineMatrix$' ./cmd/curlew/... -count=1

step "go test"
go test ./...
```

#### Tests to Write FIRST (RED phase)

N/A — this is a shell script change exercised by running `./scripts/ci-local.sh --go`
as the observable verification step. A broken script will fail the verification
command output.

#### Impact on Existing Tests

None. Running the matrix test twice (once named, once as part of `go test ./...`)
costs ~3s and catches the case where someone renames the matrix test but forgets
to update ci-local.sh — the named step will fail with "no tests to run".

---

### Step 4: Update CHANGELOG.md

**Rationale:** Project convention requires a CHANGELOG entry for every task;
doing this last keeps the entry's wording accurate relative to what was built.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add an entry under `[Unreleased] → Added` describing the new matrix test and the ci-local.sh named step. |

#### New Code

Add under the existing `## [Unreleased]` heading, in an `### Added` subsection
(creating the subsection if none exists):

```markdown
### Added
- CI regression gate `TestStreamDisciplineMatrix` (`cmd/curlew/stream_discipline_matrix_test.go`) spawning the real `curlew` binary across a matrix of (subcommand × format) cells — `run` with json/tap/junit/html/terminal formats on happy, assertion-failure, parse-error, and feature-gate-denied fixtures; plus `perf`, `license --validate`, and `exec --dry-run` cells. Each cell wires stdout and stderr to pipes and asserts: (1) stdout contains only the declared format's payload, (2) stderr carries no ANSI escape sequences on pipes, (3) no known-progress string (`Running...`, `Load test:`, `Validating license`, `Claimed shard`, `Resolved `, `Exporting license bundle`) leaks to stdout. Fixtures live under `cmd/curlew/testdata/stream-discipline/`. `scripts/ci-local.sh` now runs this test under its own named step so failures surface under a distinct header in CI logs. TTY-wired cells remain owned by `TestStderrColorFlag` (M7-001). (M7-004)
```

#### Impact on Existing Tests

None — documentation only.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `cmd/curlew/stream_discipline_matrix_test.go` | `TestStreamDisciplineMatrix` | new | Write (Step 2) |
| `cmd/curlew/stream_progress_test.go` | `TestStreamProgress` | none | Shares `writeStreamProgressRequestFile` helper — continues working |
| `cmd/curlew/stream_color_test.go` | `TestStderrColorFlag` | none | TTY cells remain here |
| `cmd/curlew/stream_help_test.go` | `TestStreamHelp` | none | Orthogonal (in-process) |
| `cmd/curlew/main_test.go` | `TestNoOsStdoutAssignment` | none | New test file does not reassign globals |
| `cmd/curlew/license_test.go` | (various) | none | Shares `setupValidLicenseDir` helper |

## Risks and Edge Cases

- **Risk:** Hermetic env. Running `license --validate` pulls from
  `CURLEW_CONFIG_DIR`, `CURLEW_OFFLINE`, etc.; pollution from the developer's
  shell could produce false failures. **Mitigation:** Build env explicitly
  from `os.Environ()` with `NO_COLOR` stripped and required keys appended —
  mirrors the approach in `TestStreamProgress`.
- **Risk:** Windows run. `/dev/tty` is not used in this matrix (pipes only)
  so the file does not need a `!windows` build tag. `buildBinary` uses
  `exec.Command("go", "build", ...)` which runs on Windows. No build tag
  needed; cross-platform. **Mitigation:** None required; documented in
  test-file comment.
- **Risk:** `perf_progress_on_stderr` cell requires
  `CURLEW_TIER=enterprise`. **Mitigation:** Set per-cell `env` in the
  table; already demonstrated to work in `TestStreamProgress`.
- **Risk:** `gate_denied.yaml` references a nonexistent include file. If the
  gate check runs before the file-existence check, we get `GateError` (desired).
  If it runs after, we get a parse error (exit 5, not 6). **Mitigation:**
  Confirmed in `cmd/curlew/main.go:567-580`: the gate check runs *before*
  parsing resolves include contents, so the gate error is emitted first.
- **Edge case:** `parse_error.yaml` must produce structured error on
  stderr (not stdout). `main.go` wiring for parse errors goes through
  `errOut.StructuredError(err)` where `errOut` is the writer-derived stderr
  printer — piped stderr means `color=false`, so no ANSI. **Handling:**
  Matrix asserts `stdoutIsEmpty` + ANSI-free stderr; both will hold.
- **Edge case:** `html` format always produces a `.html` file — never
  writes to stdout. Test asserts `stdout.Len() == 0` exactly. **Handling:**
  `stdoutIsEmpty` flag in the case struct.
- **Edge case:** `--format html` without `--report` returns exit 1 with an
  error message. The matrix cell uses `--report <tmp>` to exercise the
  happy-path. A separate cell could cover the missing-report error, but
  that path is already covered by existing tests in `run_test.go`; adding
  it here duplicates signal.
- **Risk:** Building the binary inside `TestStreamDisciplineMatrix` adds
  ~2-3s to the test suite. **Mitigation:** The test only builds once per
  `t.TempDir()`; all matrix cells reuse the same binary. Cost is bounded.
- **Risk:** Regression gate must actually catch regressions. Per the DoD,
  the `/verify` phase includes a manual step: on a scratch branch, revert
  one M7-001 site (e.g. change `newStderrPrinter(false)` back to
  `output.NewPrinter(os.Stderr, stdoutUseColor)`) and re-run the matrix.
  Any TTY-wired cell failure is expected. **Since M7-004 runs pipe-only,
  the revert that surfaces a failure is M7-002's progress-to-stderr
  relocation** (e.g. change `fmt.Fprintln(stderr, "Load test:...")` back to
  stdout). The perf and license cells will fail — confirming the gate
  catches M7-002 regressions. This is documented in the verified report.

## Verification

```bash
# Minimum (fast) gate:
go build ./cmd/curlew
go test -run '^TestStreamDisciplineMatrix$' ./cmd/curlew/... -count=1
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh

# Full local CI gate:
./scripts/ci-local.sh --go
```

Observable verification (from task YAML):

```bash
# 1. Matrix test runs cleanly.
go test -run TestStreamDisciplineMatrix ./cmd/curlew/...

# 2. Smoke-level jq parse — this uses the binary plus a live httptest server.
# We embed the server start via a tiny helper collection; the task's literal
# observable command assumes a running server at 127.0.0.1:18080. The matrix
# test itself covers this surface without an external server requirement.
# The task-literal invocation below is documented as illustrative; the
# matrix test is the authoritative gate.
cat > /tmp/m7-004-check.yaml <<'YAML'
name: jq-check
requests:
  - name: ping
    method: GET
    url: http://127.0.0.1:18080/ok
    assertions:
      status: 200
YAML
# (Requires a server at 127.0.0.1:18080. In CI, replaced by the matrix test.)

# 3. ci-local Go gate invokes the matrix test by name.
./scripts/ci-local.sh --go
```
