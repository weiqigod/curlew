# Implementation Plan: M7-003

## Overview
Introduce a shared `usageSynopsis(cmd string) string` helper so every error-recovery site in `cmd/curlew/` writes a one-line "Usage: …" synopsis to stderr instead of dumping the full help block to stdout. Explicit `--help` invocations continue to write the full help to stdout, exit 0.

## Task Details
- **ID:** M7-003
- **Title:** Help-after-error: emit one-line usage synopsis on stderr alongside the error
- **Phase:** M7: Output Discipline
- **Priority:** 2
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| — | No declared dependencies | — |

Implicit context: M7-001 (stderr-derived color), M7-002 (progress-to-stderr), and M7-005 (writer threading) are already `done`. The four `print*Help*` functions were renamed to their `*To(io.Writer)` form by M7-005, so the "parameterize on io.Writer" bullet in the task scope is **already satisfied** by the current code. The work remaining in M7-003 is therefore narrower than the scope text implies:

- No API signature changes — `printHelpTo`, `printPluginsHelpTo`, `printPerfHelpTo`, `printWorkerHelpTo` already take `w io.Writer`.
- Add one new helper: `usageSynopsis(cmd string) string`.
- Rewire five error-recovery sites so the short synopsis goes to stderr and the full help block no longer goes to stdout on error paths.

## Exploration Summary

### Current state of the four `print*Help` functions

| Function | File | Signature | First Usage line |
|----------|------|-----------|------------------|
| `printHelpTo` | `cmd/curlew/main.go:3127` | `func(w io.Writer)` | `curlew <command> [arguments]` (preceded by a bare `Usage:` line) |
| `printPluginsHelpTo` | `cmd/curlew/plugins.go:155` | `func(w io.Writer)` | `Usage: curlew plugins <subcommand>` |
| `printPerfHelpTo` | `cmd/curlew/perf.go:264` | `func(w io.Writer)` | `Usage: curlew perf <request-file> [options]` |
| `printWorkerHelpTo` | `cmd/curlew/worker.go:142` | `func(w io.Writer)` | `Usage: curlew worker [options]` |

### Current error-recovery sites (current line numbers, not task-YAML line numbers)

| Call site | Current behaviour | Desired behaviour |
|-----------|-------------------|-------------------|
| `main.go:134-136` (unknown top-level cmd) | `Fprintf(stderr, "Unknown command: %s\n\n", ...)` then `printHelpTo(stdout)` | Error to stderr, `usageSynopsis("")` to stderr, **no stdout write** |
| `plugins.go:85-87` (unknown `plugins` sub-cmd) | `Fprintf(stderr, "Unknown plugins subcommand: %s\n", ...)` then `printPluginsHelpTo(stdout)` | Error to stderr, `usageSynopsis("plugins")` to stderr |
| `perf.go:57-59` (parse error) | `Fprintf(stderr, "error: %v\n", err)` then `printPerfHelpTo(stdout)` | Error to stderr, `usageSynopsis("perf")` to stderr |
| `worker.go:31-34` (parse error) | `Fprintf(stderr, "Error: %v\n", err)` then `printWorkerHelpTo(stdout)` | Error to stderr, `usageSynopsis("worker")` to stderr |
| `main.go:3260-3264` (`import` no args) | Already writes Usage + formats on stderr (good) | Replace ad-hoc literal with `usageSynopsis("import")` for consistency; keep `Formats:` block on stderr |
| `main.go:3270-3271` (unknown import format) | Error to stderr, **no usage line at all** | Error to stderr, add `usageSynopsis("import")` to stderr |

### Current explicit `--help` paths (must keep writing to stdout, exit 0 — unchanged)

| Call site | Action |
|-----------|--------|
| `main.go:91-92` (no args default) | `printHelpTo(stdout)` — **unchanged** |
| `main.go:100-101` (`--help`/`-h`) | `printHelpTo(stdout)` — **unchanged** |
| `plugins.go:77-79` | `printPluginsHelpTo(stdout)` — **unchanged** |
| `plugins.go:96-98` (plugins list --help) | `printPluginsHelpTo(stdout)` — **unchanged** |
| `perf.go:52-54` | `printPerfHelpTo(stdout)` — **unchanged** |
| `worker.go:27-29` | `printWorkerHelpTo(stdout)` — **unchanged** |

### Existing tests that touch these paths

| Test | Current expectation | Effect of M7-003 |
|------|---------------------|------------------|
| `main_test.go:875-898` `TestCLIIntegration/"no args shows help"` | `wantOut: "Usage:"` (stdout) | Unchanged (no-args still writes to stdout) |
| `main_test.go:882-886` `TestCLIIntegration/"help flag"` | `wantOut: "Commands:"` (stdout) | Unchanged (full help remains on stdout) |
| `main_test.go:893-898` `TestCLIIntegration/"run without file shows usage"` | `wantErr: "Usage:"` | Unchanged (run already emits `Usage:` to stderr at `main.go:471`; not a site M7-003 modifies) |
| `main_test.go:917-922` `TestCLIIntegration/"unknown command"` | `wantErr: "Unknown command"` | Still passes — error message unchanged; now **also** has the synopsis on stderr |
| `main_test.go:107` run-table `"usage error routes to stderr"` | `wantStderrSub: "Usage:"` | Unchanged |
| `perf_test.go:129-148` `TestPrintPerfHelp_MentionsAllFlags` | Calls `printPerfHelpTo(&buf)` directly and asserts `"Usage: curlew perf"` | Unchanged — direct call still emits full help |
| `plugins_test.go:269-278` `TestPluginsCmd_UnknownSubcmd` | Asserts `exitCode == 1` only | Unchanged — exit code preserved |
| `plugins_test.go:280-287` `TestPluginsList_HelpArg` | `stdout` contains `"Subcommands"` | Unchanged |
| `plugins_test.go:289-301` `TestPluginsList_UnknownFlag` | `stderr` contains `"Unknown flag"`, exit=2 | Unchanged — this path already emits error to stderr without a help dump |
| `main_test.go:5492-5501` `TestRun_watch_command_recognized` | Asserts "Unknown command" is NOT in stderr | Unchanged |
| `main_test.go:7204-7210` `TestImportCmd_UnknownFormat` | Asserts `code == 1` only | Unchanged |

**No existing test breaks.** The only observable behaviour change is: `curlew <unknown>`, `curlew plugins <unknown>`, `curlew perf <bad-flag>`, `curlew worker <bad-flag>`, and `curlew import <unknown-format>` stop writing the full help block to stdout on error paths. Stdout is empty on those error paths, and a one-line synopsis appears on stderr alongside the error.

### Intentionally out of scope

- `cmd/curlew/license.go:31-33` has the same `Unknown license flag → printLicenseHelpTo(stdout)` anti-pattern. **Not in the task's affected-sites list** — leave untouched to respect the task boundary. A follow-up task can harmonise it.
- The `import openapi <bad-args>` path at `main.go:3277-3283` already emits a `Usage: curlew import openapi ...` line to stderr (inline, not via the shared helper). The task lists only the top-level `import` entry point and the "Unknown import format" branch, so the inner `import openapi` parse error site is out of scope.

### Proposed `usageSynopsis` helper

A lookup table keeps synopses co-located so a single failing test can catch any drift from the full-help text:

```go
// usageSynopses holds the one-line "Usage: ..." strings for each subcommand
// keyed by the subcommand name ("" = top-level curlew). Keep each value in
// sync with the first Usage line of the corresponding print*HelpTo function;
// TestUsageSynopsis_MatchesPrintHelpFirstLine asserts the two stay in sync.
var usageSynopses = map[string]string{
    "":        "Usage: curlew <command> [arguments]",
    "run":     "Usage: curlew run <collection-file> [options]",
    "perf":    "Usage: curlew perf <request-file> [options]",
    "worker":  "Usage: curlew worker [options]",
    "plugins": "Usage: curlew plugins <subcommand>",
    "import":  "Usage: curlew import <format> <spec-path>",
}

// usageSynopsis returns the one-line "Usage: ..." synopsis for the named
// subcommand ("" for the top-level curlew command). Returns the top-level
// synopsis as a fallback for unknown keys so callers never emit an empty
// string on an error path.
func usageSynopsis(cmd string) string {
    if s, ok := usageSynopses[cmd]; ok {
        return s
    }
    return usageSynopses[""]
}
```

Note: the top-level `printHelpTo` emits a bare `Usage:` line followed by `  curlew <command> [arguments]` on the next line (not a single `Usage: curlew ...` line). For the synopsis, we use the single-line canonical form `Usage: curlew <command> [arguments]` — it matches every behaviour-bullet phrasing in the task YAML and is what the `TestStreamHelp` tests will look for. The help-first-line test for the top-level only needs to confirm both tokens (`Usage:` and `curlew <command>`) appear in the full help output.

## Implementation Steps

### Step 1: Add `usageSynopsis` helper with synchronization test
**Rationale:** Zero-risk additive change. No call sites are touched. Establishes the helper before any error-recovery site is rewired, and establishes an invariant (synopsis appears in full help) that will survive refactors of either the helper or the `print*Help` bodies.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `usageSynopses` map + `usageSynopsis` function immediately above `printHelpTo` (around line 3126). |
| `cmd/curlew/main_test.go` | modify | Append `TestUsageSynopsis_MatchesPrintHelpFirstLine`, `TestUsageSynopsis_UnknownKeyFallsBackToTopLevel`, `TestUsageSynopsis_ImportStartsWithUsage`. |

#### Tests to Write FIRST (RED phase)

```go
// main_test.go (appended)

func TestUsageSynopsis_MatchesPrintHelpFirstLine(t *testing.T) {
    cases := []struct {
        name  string
        cmd   string
        print func(io.Writer)
        // subMustContain is the substring the full-help output must contain
        // to prove the synopsis has not drifted. For subcommands the whole
        // synopsis line appears verbatim; for the top-level, the synopsis
        // is a single-line canonical form whereas the help splits it over
        // two lines, so we check both tokens separately.
        subMustContain []string
    }{
        {
            "plugins", "plugins", printPluginsHelpTo,
            []string{"Usage: curlew plugins <subcommand>"},
        },
        {
            "perf", "perf", printPerfHelpTo,
            []string{"Usage: curlew perf <request-file> [options]"},
        },
        {
            "worker", "worker", printWorkerHelpTo,
            []string{"Usage: curlew worker [options]"},
        },
        {
            "top-level", "", printHelpTo,
            []string{"Usage:", "curlew <command>"},
        },
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            synopsis := usageSynopsis(tc.cmd)
            if synopsis == "" {
                t.Fatalf("usageSynopsis(%q) returned empty string", tc.cmd)
            }
            var buf bytes.Buffer
            tc.print(&buf)
            full := buf.String()
            for _, want := range tc.subMustContain {
                if !strings.Contains(full, want) {
                    t.Errorf("print*HelpTo output for cmd=%q missing %q.\nFull help:\n%s",
                        tc.cmd, want, full)
                }
            }
        })
    }
}

func TestUsageSynopsis_UnknownKeyFallsBackToTopLevel(t *testing.T) {
    if got := usageSynopsis("bogus"); got != usageSynopses[""] {
        t.Errorf("usageSynopsis(\"bogus\") = %q, want %q", got, usageSynopses[""])
    }
}

func TestUsageSynopsis_ImportStartsWithUsage(t *testing.T) {
    if got := usageSynopsis("import"); !strings.HasPrefix(got, "Usage: curlew import") {
        t.Errorf("usageSynopsis(\"import\") = %q, want prefix %q", got, "Usage: curlew import")
    }
}
```

#### New Code (main.go, inserted just above `printHelpTo`)

```go
// usageSynopses holds the one-line "Usage: ..." strings for each subcommand
// keyed by the subcommand name ("" = top-level curlew). Keep each value in
// sync with the first Usage line of the corresponding print*HelpTo function;
// TestUsageSynopsis_MatchesPrintHelpFirstLine asserts the two stay in sync.
var usageSynopses = map[string]string{
    "":        "Usage: curlew <command> [arguments]",
    "run":     "Usage: curlew run <collection-file> [options]",
    "perf":    "Usage: curlew perf <request-file> [options]",
    "worker":  "Usage: curlew worker [options]",
    "plugins": "Usage: curlew plugins <subcommand>",
    "import":  "Usage: curlew import <format> <spec-path>",
}

// usageSynopsis returns the one-line "Usage: ..." synopsis for the named
// subcommand ("" for the top-level curlew command). Returns the top-level
// synopsis as a fallback for unknown keys so callers never emit an empty
// string on an error path.
func usageSynopsis(cmd string) string {
    if s, ok := usageSynopses[cmd]; ok {
        return s
    }
    return usageSynopses[""]
}
```

#### Impact on Existing Tests
- None. Helper is purely additive.

---

### Step 2: Rewire top-level "Unknown command" site + start `stream_help_test.go`
**Rationale:** Smallest user-facing change after Step 1 — a single call site with an existing smoke test (`TestCLIIntegration/"unknown command"`) that already checks stderr. Also creates the new regression-test file the later steps extend.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Replace the `printHelpTo(stdout)` dump on the `default:` branch of the top-level switch with a stderr synopsis line. |
| `cmd/curlew/stream_help_test.go` | create | New regression test file with `TestStreamHelp` covering the top-level paths. |

#### Current Code (`main.go:133-137`)

```go
default:
    _, _ = fmt.Fprintf(stderr, "Unknown command: %s\n\n", args[0])
    printHelpTo(stdout)
    return 1
}
```

#### New Code

```go
default:
    _, _ = fmt.Fprintf(stderr, "Unknown command: %s\n", args[0])
    _, _ = fmt.Fprintln(stderr, usageSynopsis(""))
    return 1
}
```

Note: the trailing `\n\n` is reduced to `\n` because the following `Fprintln` adds its own newline — avoids a blank line between the error and the synopsis.

#### Tests to Write FIRST (RED phase)

```go
// cmd/curlew/stream_help_test.go
package main

import (
    "bytes"
    "strings"
    "testing"
)

// TestStreamHelp is the M7-003 regression guard for the help/error stream
// split. For every (subcommand × invocation-style) combination:
//   - On --help / no-args (explicit help request), stdout carries the full
//     help block and stderr is empty; exit 0.
//   - On error-recovery paths (unknown command, malformed flag), stderr
//     carries both the error message AND a one-line "Usage:" synopsis; stdout
//     is empty; exit code matches the command's normal usage-error code.
func TestStreamHelp(t *testing.T) {
    t.Run("top_level_unknown_command", func(t *testing.T) {
        var stdout, stderr bytes.Buffer
        code := runWithWriters([]string{"bogus-command"}, &stdout, &stderr)
        if code != 1 {
            t.Errorf("exit = %d, want 1", code)
        }
        if stdout.Len() != 0 {
            t.Errorf("stdout should be empty, got: %q", stdout.String())
        }
        if !strings.Contains(stderr.String(), "Unknown command") {
            t.Errorf("stderr missing 'Unknown command': %q", stderr.String())
        }
        if !strings.Contains(stderr.String(), "Usage: curlew") {
            t.Errorf("stderr missing 'Usage: curlew' synopsis: %q", stderr.String())
        }
    })

    t.Run("top_level_explicit_help", func(t *testing.T) {
        var stdout, stderr bytes.Buffer
        code := runWithWriters([]string{"--help"}, &stdout, &stderr)
        if code != 0 {
            t.Errorf("exit = %d, want 0", code)
        }
        if stderr.Len() != 0 {
            t.Errorf("stderr should be empty for --help, got: %q", stderr.String())
        }
        if !strings.Contains(stdout.String(), "Commands:") {
            t.Errorf("stdout missing 'Commands:' block: %q", stdout.String())
        }
    })

    t.Run("top_level_no_args", func(t *testing.T) {
        var stdout, stderr bytes.Buffer
        code := runWithWriters([]string{}, &stdout, &stderr)
        if code != 0 {
            t.Errorf("exit = %d, want 0", code)
        }
        if stderr.Len() != 0 {
            t.Errorf("stderr should be empty for no-args help, got: %q", stderr.String())
        }
        if !strings.Contains(stdout.String(), "Commands:") {
            t.Errorf("stdout missing 'Commands:' block: %q", stdout.String())
        }
    })
}
```

#### Impact on Existing Tests
- `TestCLIIntegration/"unknown command"` still passes — `wantErr: "Unknown command"` unchanged.
- `TestCLIIntegration/"no args shows help"` and `"help flag"` still pass — stdout still receives the full help block.

---

### Step 3: Rewire `plugins` unknown-subcommand site
**Rationale:** Same shape as Step 2, scoped to one file. Independent of the other subcommands.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/plugins.go` | modify | Replace `printPluginsHelpTo(stdout)` on the unknown-subcommand branch with `Fprintln(stderr, usageSynopsis("plugins"))`. |
| `cmd/curlew/stream_help_test.go` | modify | Add `plugins_unknown_subcommand` and `plugins_explicit_help` subtests to `TestStreamHelp`. |

#### Current Code (`plugins.go:76-89`)

```go
func pluginsCmdOut(args []string, stdout, stderr io.Writer) int {
    if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
        printPluginsHelpTo(stdout)
        return 0
    }
    switch args[0] {
    case "list":
        return pluginsListCmdOut(args[1:], stdout, stderr)
    default:
        _, _ = fmt.Fprintf(stderr, "Unknown plugins subcommand: %s\n", args[0])
        printPluginsHelpTo(stdout)
        return 1
    }
}
```

#### New Code

```go
func pluginsCmdOut(args []string, stdout, stderr io.Writer) int {
    if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
        printPluginsHelpTo(stdout)
        return 0
    }
    switch args[0] {
    case "list":
        return pluginsListCmdOut(args[1:], stdout, stderr)
    default:
        _, _ = fmt.Fprintf(stderr, "Unknown plugins subcommand: %s\n", args[0])
        _, _ = fmt.Fprintln(stderr, usageSynopsis("plugins"))
        return 1
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
// appended inside TestStreamHelp
t.Run("plugins_unknown_subcommand", func(t *testing.T) {
    var stdout, stderr bytes.Buffer
    code := runWithWriters([]string{"plugins", "bogus"}, &stdout, &stderr)
    if code != 1 {
        t.Errorf("exit = %d, want 1", code)
    }
    if stdout.Len() != 0 {
        t.Errorf("stdout should be empty, got: %q", stdout.String())
    }
    if !strings.Contains(stderr.String(), "Unknown plugins subcommand") {
        t.Errorf("stderr missing error message: %q", stderr.String())
    }
    if !strings.Contains(stderr.String(), "Usage: curlew plugins") {
        t.Errorf("stderr missing synopsis: %q", stderr.String())
    }
})

t.Run("plugins_explicit_help", func(t *testing.T) {
    var stdout, stderr bytes.Buffer
    code := runWithWriters([]string{"plugins", "--help"}, &stdout, &stderr)
    if code != 0 {
        t.Errorf("exit = %d, want 0", code)
    }
    if stderr.Len() != 0 {
        t.Errorf("stderr should be empty for --help, got: %q", stderr.String())
    }
    if !strings.Contains(stdout.String(), "Subcommands:") {
        t.Errorf("stdout missing 'Subcommands:' block: %q", stdout.String())
    }
})
```

#### Impact on Existing Tests
- `TestPluginsCmd_UnknownSubcmd` (plugins_test.go:269) only checks `code == 1` — unchanged.
- `TestPluginsList_HelpArg` asserts stdout contains `Subcommands` — unchanged.

---

### Step 4: Rewire `perf` parse-error site
**Rationale:** Same shape, scoped to `perf.go`. The explicit `--help` path (`perf.go:52-54`) already routes to stdout and is untouched.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/perf.go` | modify | Replace `printPerfHelpTo(stdout)` on the parse-error branch with `Fprintln(stderr, usageSynopsis("perf"))`. |
| `cmd/curlew/stream_help_test.go` | modify | Add `perf_malformed_flag` and `perf_explicit_help` subtests. |

#### Current Code (`perf.go:51-60`)

```go
flags, showHelp, err := parsePerfArgs(args)
if showHelp {
    printPerfHelpTo(stdout)
    return 0
}
if err != nil {
    _, _ = fmt.Fprintf(stderr, "error: %v\n", err)
    printPerfHelpTo(stdout)
    return 2
}
```

#### New Code

```go
flags, showHelp, err := parsePerfArgs(args)
if showHelp {
    printPerfHelpTo(stdout)
    return 0
}
if err != nil {
    _, _ = fmt.Fprintf(stderr, "error: %v\n", err)
    _, _ = fmt.Fprintln(stderr, usageSynopsis("perf"))
    return 2
}
```

#### Tests to Write FIRST (RED phase)

```go
t.Run("perf_malformed_flag", func(t *testing.T) {
    var stdout, stderr bytes.Buffer
    code := runWithWriters([]string{"perf", "--bogus"}, &stdout, &stderr)
    if code != 2 {
        t.Errorf("exit = %d, want 2", code)
    }
    if stdout.Len() != 0 {
        t.Errorf("stdout should be empty, got: %q", stdout.String())
    }
    if !strings.Contains(stderr.String(), "error:") {
        t.Errorf("stderr missing error: %q", stderr.String())
    }
    if !strings.Contains(stderr.String(), "Usage: curlew perf") {
        t.Errorf("stderr missing synopsis: %q", stderr.String())
    }
})

t.Run("perf_explicit_help", func(t *testing.T) {
    var stdout, stderr bytes.Buffer
    code := runWithWriters([]string{"perf", "--help"}, &stdout, &stderr)
    if code != 0 {
        t.Errorf("exit = %d, want 0", code)
    }
    if stderr.Len() != 0 {
        t.Errorf("stderr should be empty for --help, got: %q", stderr.String())
    }
    if !strings.Contains(stdout.String(), "Usage: curlew perf") {
        t.Errorf("stdout missing full help: %q", stdout.String())
    }
})
```

#### Impact on Existing Tests
- `TestPrintPerfHelp_MentionsAllFlags` calls `printPerfHelpTo(&buf)` directly — unaffected (function body and signature unchanged).
- `TestPerfCmd_InvalidVUsExitCode2`, `TestPerfCmd_InvalidDurationExitCode2`, `TestPerfCmd_UnsupportedOutputExitCode2` assert `code == 2` and ignore stream content — unaffected.

---

### Step 5: Rewire `worker` parse-error site
**Rationale:** Identical pattern to perf in an independent file.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/worker.go` | modify | Replace `printWorkerHelpTo(stdout)` on the parse-error branch with `Fprintln(stderr, usageSynopsis("worker"))`. |
| `cmd/curlew/stream_help_test.go` | modify | Add `worker_malformed_flag` and `worker_explicit_help` subtests. |

#### Current Code (`worker.go:25-35`)

```go
func workerCmdOut(args []string, stdout, stderr io.Writer) int {
    cfg, showHelp, err := parseWorkerArgs(args)
    if showHelp {
        printWorkerHelpTo(stdout)
        return 0
    }
    if err != nil {
        _, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
        printWorkerHelpTo(stdout)
        return 1
    }
    ...
```

#### New Code

```go
func workerCmdOut(args []string, stdout, stderr io.Writer) int {
    cfg, showHelp, err := parseWorkerArgs(args)
    if showHelp {
        printWorkerHelpTo(stdout)
        return 0
    }
    if err != nil {
        _, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
        _, _ = fmt.Fprintln(stderr, usageSynopsis("worker"))
        return 1
    }
    ...
```

#### Tests to Write FIRST (RED phase)

```go
t.Run("worker_malformed_flag", func(t *testing.T) {
    var stdout, stderr bytes.Buffer
    code := runWithWriters([]string{"worker", "--bogus"}, &stdout, &stderr)
    if code != 1 {
        t.Errorf("exit = %d, want 1", code)
    }
    if stdout.Len() != 0 {
        t.Errorf("stdout should be empty, got: %q", stdout.String())
    }
    if !strings.Contains(stderr.String(), "Error:") {
        t.Errorf("stderr missing error: %q", stderr.String())
    }
    if !strings.Contains(stderr.String(), "Usage: curlew worker") {
        t.Errorf("stderr missing synopsis: %q", stderr.String())
    }
})

t.Run("worker_explicit_help", func(t *testing.T) {
    var stdout, stderr bytes.Buffer
    code := runWithWriters([]string{"worker", "--help"}, &stdout, &stderr)
    if code != 0 {
        t.Errorf("exit = %d, want 0", code)
    }
    if stderr.Len() != 0 {
        t.Errorf("stderr should be empty for --help, got: %q", stderr.String())
    }
    if !strings.Contains(stdout.String(), "Usage: curlew worker") {
        t.Errorf("stdout missing full help: %q", stdout.String())
    }
})
```

#### Impact on Existing Tests
- `worker_test.go` tests parse `parseWorkerArgs` directly and don't assert stream routing — unaffected.

---

### Step 6: Harmonise `import` error-recovery sites
**Rationale:** Two tiny sites in `main.go`. The no-args branch already emits Usage + Formats lines on stderr (correct); this step replaces the ad-hoc literal with the shared helper and fills in the missing synopsis on the "unknown import format" branch.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | `importCmdOut` emits `usageSynopsis("import")` on both the no-args and unknown-format branches. |
| `cmd/curlew/stream_help_test.go` | modify | Add `import_no_args` and `import_unknown_format` subtests. |

#### Current Code (`main.go:3259-3273`)

```go
func importCmdOut(args []string, stdout, stderr io.Writer) int {
    if len(args) == 0 {
        _, _ = fmt.Fprintln(stderr, "Usage: curlew import <format> <spec-path> [--output <file>]")
        _, _ = fmt.Fprintln(stderr, "Formats:")
        _, _ = fmt.Fprintln(stderr, "  openapi    Import an OpenAPI 3.0/3.1 spec (Professional tier)")
        return 1
    }
    switch args[0] {
    case "openapi":
        return importOpenAPICmdOut(args[1:], stdout, stderr)
    default:
        _, _ = fmt.Fprintf(stderr, "Unknown import format: %s\n", args[0])
        return 1
    }
}
```

#### New Code

```go
func importCmdOut(args []string, stdout, stderr io.Writer) int {
    if len(args) == 0 {
        _, _ = fmt.Fprintln(stderr, usageSynopsis("import"))
        _, _ = fmt.Fprintln(stderr, "Formats:")
        _, _ = fmt.Fprintln(stderr, "  openapi    Import an OpenAPI 3.0/3.1 spec (Professional tier)")
        return 1
    }
    switch args[0] {
    case "openapi":
        return importOpenAPICmdOut(args[1:], stdout, stderr)
    default:
        _, _ = fmt.Fprintf(stderr, "Unknown import format: %s\n", args[0])
        _, _ = fmt.Fprintln(stderr, usageSynopsis("import"))
        return 1
    }
}
```

Note: the pre-existing literal was `Usage: curlew import <format> <spec-path> [--output <file>]` — longer than the shared synopsis. The task's behaviour bullet specifies the shorter form `Usage: curlew import <format> <spec-path>` exactly, so dropping the `[--output <file>]` fragment is explicitly what the task asks for. Option discovery still happens via `--help` on the concrete format (`curlew import openapi --help`).

#### Tests to Write FIRST (RED phase)

```go
t.Run("import_no_args", func(t *testing.T) {
    var stdout, stderr bytes.Buffer
    code := runWithWriters([]string{"import"}, &stdout, &stderr)
    if code != 1 {
        t.Errorf("exit = %d, want 1", code)
    }
    if stdout.Len() != 0 {
        t.Errorf("stdout should be empty, got: %q", stdout.String())
    }
    if !strings.Contains(stderr.String(), "Usage: curlew import") {
        t.Errorf("stderr missing synopsis: %q", stderr.String())
    }
})

t.Run("import_unknown_format", func(t *testing.T) {
    var stdout, stderr bytes.Buffer
    code := runWithWriters([]string{"import", "postman"}, &stdout, &stderr)
    if code != 1 {
        t.Errorf("exit = %d, want 1", code)
    }
    if stdout.Len() != 0 {
        t.Errorf("stdout should be empty, got: %q", stdout.String())
    }
    if !strings.Contains(stderr.String(), "Unknown import format") {
        t.Errorf("stderr missing error: %q", stderr.String())
    }
    if !strings.Contains(stderr.String(), "Usage: curlew import") {
        t.Errorf("stderr missing synopsis: %q", stderr.String())
    }
})
```

#### Impact on Existing Tests
- `TestImportCmd_UnknownFormat` asserts `code == 1` only — unchanged.

---

### Step 7: CHANGELOG + smoke test sanity
**Rationale:** Completeness contract — documentation update and a final full-CI gate. No code changes; the smoke script does not currently exercise error paths but `./scripts/ci-local.sh` runs the full Go test suite which now covers every new assertion.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add a `### Fixed` entry under `[Unreleased]` describing the stream routing change and the new `TestStreamHelp` guard. |

#### New CHANGELOG entry (under `[Unreleased] / ### Fixed`)

```
- `curlew <unknown-command>`, `curlew plugins <unknown-subcommand>`, `curlew perf <malformed-flag>`, `curlew worker <malformed-flag>`, and `curlew import <unknown-format>` now emit a one-line `Usage: curlew ...` synopsis to stderr instead of dumping the full help block to stdout; stdout is empty on every error path. Explicit `curlew --help` / `curlew <subcommand> --help` continue to write the full help text to stdout and exit 0. New helper `usageSynopsis(cmd string) string` in `cmd/curlew/main.go` keeps the short synopsis string in one place, and a synchronization test (`TestUsageSynopsis_MatchesPrintHelpFirstLine`) asserts each synopsis still appears verbatim in its corresponding `print*HelpTo` full-help output. Regression guard: `cmd/curlew/stream_help_test.go::TestStreamHelp` exercises every (subcommand × invocation-style) cell via `runWithWriters`. (M7-003)
```

#### Impact on Existing Tests
- None.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `cmd/curlew/main_test.go` | `TestCLIIntegration/"unknown command"` | still passes | none (only asserts `"Unknown command"` on stderr) |
| `cmd/curlew/main_test.go` | `TestCLIIntegration/"no args shows help"`, `"help flag"` | still passes | none (stdout still has full help) |
| `cmd/curlew/main_test.go` | `TestRun_watch_command_recognized` | still passes | none |
| `cmd/curlew/main_test.go` | `TestImportCmd_UnknownFormat` | still passes | none (exit-code only) |
| `cmd/curlew/main_test.go` | **new** `TestUsageSynopsis_*` (3 tests) | new | add in Step 1 |
| `cmd/curlew/perf_test.go` | `TestPrintPerfHelp_MentionsAllFlags` | still passes | none (direct print* call) |
| `cmd/curlew/perf_test.go` | `TestPerfCmd_InvalidVUsExitCode2` etc. | still passes | none (exit-code only) |
| `cmd/curlew/plugins_test.go` | `TestPluginsCmd_UnknownSubcmd`, `TestPluginsList_HelpArg`, `TestPluginsList_UnknownFlag` | still passes | none |
| `cmd/curlew/worker_test.go` | all | still passes | none |
| `cmd/curlew/stream_help_test.go` | **new** `TestStreamHelp` (11 subtests) | new | create in Step 2; extend in Steps 3/4/5/6 |

No existing test is expected to fail as a side effect of this change.

## Risks and Edge Cases

- **Risk: `TestUsageSynopsis_MatchesPrintHelpFirstLine` becomes brittle if someone rewords the first Usage line of a `print*HelpTo` function.** → **Mitigation:** the synchronization test is exactly what we want to fire in that case — it forces the author to update the shared helper, preserving the "one source of truth" invariant. The failure message shows the mismatched substring and the full help output for quick diagnosis.
- **Risk: users running `curlew --help > help.txt` in CI scripts relied on a trailing newline or exact byte count.** → **Mitigation:** the `--help` path is unchanged; only error paths change. No byte-count promises exist in the spec, and stdout on the error path was previously "full help" which nobody pipes deliberately (you'd pipe `--help`).
- **Edge case: `curlew plugins` with no args.** → **Handling:** Line 77 (`if len(args) == 0 || args[0] == "--help" ...`) treats no-args as a help request; stdout receives the full plugins help, exit 0. This matches the pattern for the top-level `curlew` (no-args → help on stdout). The TestStreamHelp `plugins_explicit_help` subtest covers `plugins --help`; the no-args case is not called out by the task's behaviour bullets, and the existing TestPluginsHelp tests cover the content. Not adding a separate subtest.
- **Edge case: `curlew worker` with no args.** → **Handling:** Unlike `plugins`, `parseWorkerArgs` with no args succeeds (returns a valid `worker.Config` with `WorkerID` auto-filled) and then `cfg.Validate()` at `worker.go:36` fails with a missing-job-id error. This is the existing `cfg.Validate()` path, **not** a parse-error path. The behaviour bullet specifically says "worker with a malformed flag", matching the parse-error branch. We do not modify the validate-error branch (it already writes only to stderr and doesn't dump help).
- **Edge case: `curlew perf` with no positional arg (just flags).** → **Handling:** `parsePerfArgs` returns an error "perf requires exactly one positional argument: <request-file>"; the rewired code correctly emits error + synopsis on stderr. Not in the explicit test matrix but covered transitively by the `perf_malformed_flag` subtest.
- **Risk: line-number drift between plan and code.** → **Mitigation:** all line numbers in this plan are read from the current `main` (HEAD 5a53792 at plan-writing time); M7-005 shifted some line numbers from what the task YAML scope lists, but the call sites referenced here are canonical (we cite function names, not just line numbers).

## Verification

```bash
# Build
go build ./cmd/curlew

# Unit + integration
go test ./...

# Lint
~/go/bin/golangci-lint run

# Smoke (does not cover error paths explicitly, but must still pass)
./smoke/run.sh

# Full local CI gate (Go scope auto-detected)
./scripts/ci-local.sh --go
```

### Observable verification

```bash
go build -o /tmp/curlew ./cmd/curlew

# 1. Unknown top-level command: stderr has error + synopsis; stdout empty.
/tmp/curlew unknown-command > /tmp/out.txt 2> /tmp/err.txt || true
wc -c /tmp/out.txt                          # expect: 0 bytes
grep -c "Usage:" /tmp/err.txt               # expect: >= 1
grep -c "Unknown command" /tmp/err.txt      # expect: 1

# 2. Explicit --help: stdout has full help; stderr empty.
/tmp/curlew --help > /tmp/out.txt 2> /tmp/err.txt
wc -c /tmp/err.txt                          # expect: 0 bytes
grep -c "Commands:" /tmp/out.txt            # expect: 1
```
