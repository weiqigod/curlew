# Implementation Plan: M7-001

## Overview

Fix a latent bug where `curlew run`, `curlew watch`, and `curlew exec` derive the color flag for `stderr` printers from `os.Stdout`'s TTY state. When stdout is a pipe and stderr is a pipe too, the code path at `main.go:552-553`, `main.go:1545`, and `main.go:2710-2828` still emits ANSI escape sequences into piped stderr. This task introduces a `newStderrPrinter` helper, renames the stdout-derived flag to `stdoutUseColor`, switches all affected call sites, and adds a four-case (TTY x pipe) regression test.

## Task Details

- **ID:** M7-001
- **Title:** Stderr printers derive color flag from stderr's TTY state, not stdout's
- **Phase:** M7: Output Discipline
- **Priority:** 1
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| (none) | — | — |

The task YAML defines no `dependencies:` key. M7-001 is one of four parallelizable tasks in the output_discipline capability group.

## Code Exploration Findings

### Current state of stderr printer construction in `cmd/curlew/main.go`

Twenty call sites currently construct a printer on `os.Stderr`. Three categories:

| Category | Line(s) | Color-flag source | Task impact |
|----------|---------|-------------------|-------------|
| **Buggy — stdout-derived** | 553, 2722, 2754, 2761, 2769, 2828 | `useColor` (a variable set via `shouldUseColor(os.Stdout, noColor)`) | **Must switch** to stderr-derived |
| **Already correct — stderr-derived** | 432, 453, 469, 477, 531, 542, 1537, 2699, 2705, 2927, 3244, 3251 | `shouldUseColor(os.Stderr, ...)` | Adopt helper for uniformity |
| **stdout-bound (correct)** | 982, 1044, 2864 | `useColor` for `os.Stdout` printer | **Do not change** the stdout sites; only rename the variable to `stdoutUseColor` |

Additionally, `cmd/curlew/perf.go:61` has one `shouldUseColor(os.Stderr, false)` call — already correct, but should adopt the helper for uniformity and test coverage.

### Watch integration (`main.go:1545` + `internal/watch`)

`watchCmd` at `main.go:1522-1569` passes `useColor` to `watch.Config.UseColor`. That config field is used *inside* the watch package — but the watch package emits terminal-style summary output. The stdout-derived flag is semantically correct *for that field* because watch writes to `Stdout` in `watch.Config`. The bug is only in the sibling `errOut` at `main.go:1537`, which already derives from stderr. The task scope says to rename the variable at line 1545 to `stdoutUseColor` for defensive clarity — this does **not** require changing `watch.Config.UseColor`'s semantics.

### exec subcommand (`main.go:2691-2889`)

`execCmd` computes `useColor := shouldUseColor(os.Stdout, opts.NoColor)` at line 2710, then reuses it for both stdout (`main.go:2864`) and five stderr error paths (2722, 2754, 2761, 2769, 2828). The fix: rename to `stdoutUseColor`, route each stderr site through `newStderrPrinter`.

### Test patterns available in cmd/curlew

- `main_test.go:32-67` defines `captureRunCmd` which replaces `os.Stdout`/`os.Stderr` with pipes. Perfect for testing the non-TTY case (pipes force `IsTerminal` to false). Cannot synthesize a TTY.
- `main_test.go:78-88` defines `buildBinary(t *testing.T) string` — builds the real `curlew` binary. Used by the TTY regression variants via `creack/pty`.
- No existing test exercises the stdout-TTY + stderr-pipe combination.

### PTY dependency decision

`creack/pty` is **not** currently in `go.mod`. Task scope says:
> TTY cases use a pseudo-TTY helper (creack/pty or equivalent); if unavailable restrict TTY assertions to Unix and document.

**Decision:** Avoid adding a new dependency. Instead:
- **Non-TTY cases** (3 of 4 combinations): use `os.Pipe` directly — always portable.
- **TTY cases** (stderr-TTY branch): open `/dev/tty` on Unix (darwin, linux) to acquire a character-device file descriptor and assert against it directly in a unit test of `shouldUseColor(os.Stderr, ...)` via `os.NewFile` / direct TTY detection. Gate with `//go:build darwin || linux` and `t.Skip()` when `/dev/tty` cannot be opened (CI containers without a TTY).
- **End-to-end coverage**: the full-binary test uses three combinations (stdout-pipe+stderr-pipe, NO_COLOR set, --no-color) exercised through `exec.Command` with piped stdout and stderr. Add one Unix-only subtest that allocates a TTY via `/dev/tty` when available for the fourth case. Document the skip behaviour in the test file header.

This avoids introducing `creack/pty`. If later M7 tasks warrant PTY infrastructure, it can be added separately.

### `shouldUseColor` contract (unchanged)

`main.go:319-327` accepts any `io.Writer` and only treats `*os.File` as possibly-TTY. Safe to pass `os.Stderr` directly; no modification required to this function.

### `internal/output.Printer` contract (unchanged)

`NewPrinter(w io.Writer, color bool, verbosity ...Verbosity)` at `internal/output/terminal.go:26`. Task scope confirms: "No changes to `internal/output/` — the Printer abstraction is already correct; bugs live at the call sites."

## Implementation Steps

### Step 1: Add `newStderrPrinter` helper and rename the stdout-derived variable

**Rationale:** Lowest-blast-radius starting point — introduces the abstraction without changing any existing behaviour. Subsequent steps route call sites through it.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `newStderrPrinter` next to `shouldUseColor` at line 319; no behaviour change elsewhere yet |

#### Current Code (main.go:317-327)

```go
// shouldUseColor returns true when color output is appropriate.
// Color is disabled by --no-color flag, NO_COLOR env var, or non-TTY output.
func shouldUseColor(w io.Writer, noColorFlag bool) bool {
	if noColorFlag {
		return false
	}
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	return output.IsTerminal(w)
}
```

#### New Code

```go
// shouldUseColor returns true when color output is appropriate.
// Color is disabled by --no-color flag, NO_COLOR env var, or non-TTY output.
func shouldUseColor(w io.Writer, noColorFlag bool) bool {
	if noColorFlag {
		return false
	}
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	return output.IsTerminal(w)
}

// newStderrPrinter constructs an output.Printer bound to os.Stderr whose
// color flag is derived from os.Stderr's own TTY state, never from stdout.
// This prevents ANSI escape sequences from leaking into piped stderr when
// stdout happens to be a TTY (regression guard for M7-001).
//
// Prefer this helper over output.NewPrinter(os.Stderr, ...) at every new
// call site so the "use stdout-derived color on stderr" anti-pattern cannot
// be reintroduced silently.
func newStderrPrinter(noColor bool) *output.Printer {
	return output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, noColor))
}
```

#### Tests to Write FIRST (RED phase)

Add to `cmd/curlew/main_test.go` (extends `TestShouldUseColor` area):

```go
func TestNewStderrPrinter_NoColorFlag(t *testing.T) {
	// Given --no-color is true, color is disabled regardless of TTY.
	p := newStderrPrinter(true)
	if p == nil {
		t.Fatal("newStderrPrinter returned nil")
	}
	// Printer has unexported fields; assert behaviour by rendering a
	// colorable method and checking absence of ANSI escapes.
	var sb strings.Builder
	// Indirect probe: StructuredError renders an error which includes no
	// ANSI when color is disabled. The printer is bound to os.Stderr here,
	// so we cannot inspect output directly — instead verify the NO_COLOR
	// contract via a parallel call to shouldUseColor(os.Stderr, true).
	if shouldUseColor(os.Stderr, true) {
		t.Errorf("shouldUseColor(os.Stderr, true) = true, want false")
	}
	_ = sb // silence unused
}

func TestNewStderrPrinter_NoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if shouldUseColor(os.Stderr, false) {
		t.Errorf("shouldUseColor(os.Stderr, false) with NO_COLOR set = true, want false")
	}
	// Smoke: helper must not panic.
	if p := newStderrPrinter(false); p == nil {
		t.Fatal("newStderrPrinter returned nil")
	}
}
```

The real regression coverage lives in Step 4's `stream_color_test.go`; these two unit tests only verify the helper exists and composes `shouldUseColor(os.Stderr, ...)` with `output.NewPrinter(os.Stderr, ...)`.

#### Impact on Existing Tests

- No existing test references `newStderrPrinter` (doesn't exist yet). No breakage.
- `TestShouldUseColor` at `main_test.go:679` is unchanged.

---

### Step 2: Rewire `runCmdInner` and `watchCmd` call sites

**Rationale:** Smaller, more isolated than `execCmd`. Tested by the existing comprehensive `captureRunCmd`-based tests in `main_test.go` (750+ tests that exercise `runCmd`).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | (a) Rename `useColor` -> `stdoutUseColor` at line 552 (runCmdInner) and line 1545 (watchCmd); (b) replace `errOut := output.NewPrinter(os.Stderr, useColor)` at line 553 with `errOut := newStderrPrinter(noColor)`; (c) adopt `newStderrPrinter` at the twelve already-correct stderr sites (432, 453, 469, 477, 531, 542, 1537) for uniformity |

#### Current Code (main.go:550-554)

```go
	useColor := shouldUseColor(os.Stdout, noColor)
	errOut := output.NewPrinter(os.Stderr, useColor)
```

#### New Code

```go
	stdoutUseColor := shouldUseColor(os.Stdout, noColor)
	errOut := newStderrPrinter(noColor)
```

Then every reference to `useColor` between lines 553 and the end of `runCmdInner` (lines 982 and 1044 are the stdout `NewPrinter` call sites; renamed to `stdoutUseColor`) is updated.

#### Current Code (main.go:1537-1545)

```go
		errOut := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, watchFlags.noColor))
		errOut.StructuredError(err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	useColor := shouldUseColor(os.Stdout, watchFlags.noColor)
```

#### New Code

```go
		errOut := newStderrPrinter(watchFlags.noColor)
		errOut.StructuredError(err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	stdoutUseColor := shouldUseColor(os.Stdout, watchFlags.noColor)
```

Then `UseColor: useColor` at `main.go:1554` becomes `UseColor: stdoutUseColor`. This field remains stdout-derived because `watch.Config.UseColor` drives stdout-facing terminal output inside the watch loop — that's semantically correct.

#### Tests to Write FIRST (RED phase)

No new tests at this step (tests are in Step 4). The existing test suite in `main_test.go` will catch any regression in `runCmdInner` / `watchCmd` behaviour because the rename is a pure refactor at the correct call sites.

#### Impact on Existing Tests

- All `TestRunCmdDirect_*` tests (28 tests) must still pass — behaviour is unchanged when stdout is piped (test harness uses `os.Pipe`, so stdoutUseColor is false, matching the old useColor value in test conditions).
- No test relies on the private variable name.

---

### Step 3: Rewire `execCmd` call sites

**Rationale:** Five stderr printer sites in one function (`main.go:2710-2828`). Largest mechanical change — held for last to isolate any regression bisect.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Rename `useColor` -> `stdoutUseColor` at line 2710; switch the five stderr printer constructions at 2722, 2754, 2761, 2769, 2828 to `newStderrPrinter(opts.NoColor)`; adopt `newStderrPrinter` at lines 2699, 2705, 2927, 3244, 3251 for uniformity |

#### Current Code (main.go:2710 and 2722)

```go
	useColor := shouldUseColor(os.Stdout, opts.NoColor)

	// Determine request source
	var req *parser.Request
	if opts.Stdin {
		var err error
		req, err = parseStdinRequest(os.Stdin)
		if err != nil {
			if opts.Format == "json" {
				jsonOut := buildJSONOutput("", nil, nil, err, output.VerbosityDefault)
				_ = output.WriteJSON(os.Stdout, jsonOut)
			} else {
				errOut := output.NewPrinter(os.Stderr, useColor)
				errOut.StructuredError(err)
			}
			return 3
		}
```

#### New Code

```go
	stdoutUseColor := shouldUseColor(os.Stdout, opts.NoColor)

	// Determine request source
	var req *parser.Request
	if opts.Stdin {
		var err error
		req, err = parseStdinRequest(os.Stdin)
		if err != nil {
			if opts.Format == "json" {
				jsonOut := buildJSONOutput("", nil, nil, err, output.VerbosityDefault)
				_ = output.WriteJSON(os.Stdout, jsonOut)
			} else {
				errOut := newStderrPrinter(opts.NoColor)
				errOut.StructuredError(err)
			}
			return 3
		}
```

Same pattern applies at each of the other four stderr sites. The one remaining `useColor` usage at `main.go:2864` (`out := output.NewPrinter(os.Stdout, useColor, opts.Verbosity)`) becomes `stdoutUseColor`.

Also adopt the helper at lines 2699, 2705, 2927 (vaultCmd), 3244 and 3251 (openapiImport) — mechanical rewrite of already-correct sites for uniformity.

#### Tests to Write FIRST (RED phase)

No new unit tests at this step beyond those added in Step 4 (integration-level regression coverage for all four combinations).

#### Impact on Existing Tests

- No existing test asserts against ANSI escapes in exec's stderr output, so no breakage.

---

### Step 4: Regression test — four TTY/pipe combinations

**Rationale:** The observable contract from the task YAML. Written as a sibling file `cmd/curlew/stream_color_test.go` per scope. Covers the bug directly.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/stream_color_test.go` | create | Table-driven test building the real binary via `buildBinary`, running against a deliberately broken collection (`{{UNDEFINED}}`), asserting presence/absence of `\x1b[` ANSI escape markers in stderr |

#### Test Structure

```go
//go:build !windows
// +build !windows

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestStderrColorFlag is the primary regression gate for M7-001. It exercises
// four stdout/stderr TTY-vs-pipe combinations through the real curlew binary
// and asserts whether stderr contains ANSI escape sequences.
//
// The TTY cases require /dev/tty. When unavailable (CI containers, sandboxes)
// the affected subtests t.Skip. Pipe-pipe and env-gate cases are portable.
func TestStderrColorFlag(t *testing.T) {
	binary := buildBinary(t)

	dir := t.TempDir()
	collection := filepath.Join(dir, "stream-color-check.yaml")
	if err := os.WriteFile(collection, []byte(
		"name: stream-color-check\n"+
			"requests:\n"+
			"  - name: missing-var\n"+
			"    method: GET\n"+
			"    url: \"{{UNDEFINED}}\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	esc := "\x1b["

	tests := []struct {
		name            string
		env             []string
		args            []string
		stdoutTTY       bool // when true, stdout connected to /dev/tty
		stderrTTY       bool // when true, stderr connected to /dev/tty
		wantStderrColor bool
	}{
		{
			name:            "stdout pipe, stderr pipe, no flags -> no color anywhere",
			env:             nil,
			args:            []string{"run", collection},
			wantStderrColor: false,
		},
		{
			name:            "stdout pipe, stderr pipe, NO_COLOR env -> no color",
			env:             []string{"NO_COLOR=1"},
			args:            []string{"run", collection},
			wantStderrColor: false,
		},
		{
			name:            "stdout pipe, stderr pipe, --no-color flag -> no color",
			env:             nil,
			args:            []string{"run", "--no-color", collection},
			wantStderrColor: false,
		},
		{
			name:            "stdout TTY, stderr pipe -> no color on stderr (regression guard)",
			env:             nil,
			args:            []string{"run", collection},
			stdoutTTY:       true,
			wantStderrColor: false, // the M7-001 bug: today this would be true
		},
		{
			name:            "stdout pipe, stderr TTY -> color on stderr (preserved)",
			env:             nil,
			args:            []string{"run", collection},
			stderrTTY:       true,
			wantStderrColor: true,
		},
		{
			name:            "both TTY -> color on stderr",
			env:             nil,
			args:            []string{"run", collection},
			stdoutTTY:       true,
			stderrTTY:       true,
			wantStderrColor: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tty *os.File
			if tt.stdoutTTY || tt.stderrTTY {
				f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
				if err != nil {
					t.Skipf("/dev/tty not available: %v", err)
				}
				tty = f
				defer func() { _ = tty.Close() }()
			}

			cmd := exec.Command(binary, tt.args...)
			if tt.env != nil {
				cmd.Env = append(os.Environ(), tt.env...)
			}

			// Capture whichever stream is piped; attach /dev/tty for TTY legs.
			var stdoutBuf, stderrBuf strings.Builder
			if tt.stdoutTTY {
				cmd.Stdout = tty
			} else {
				cmd.Stdout = &stdoutBuf
			}
			if tt.stderrTTY {
				cmd.Stderr = tty
			} else {
				cmd.Stderr = &stderrBuf
			}

			_ = cmd.Run() // expected to exit non-zero (undefined var)

			// Only inspect captured buffers for pipe legs.
			if !tt.stderrTTY {
				hasColor := strings.Contains(stderrBuf.String(), esc)
				if hasColor != tt.wantStderrColor {
					t.Errorf("stderr color = %v, want %v (stderr=%q)",
						hasColor, tt.wantStderrColor, stderrBuf.String())
				}
			}
		})
	}
}
```

Note: `stdoutTTY:true, stderrTTY:false` is the central regression case — stderr must not contain ANSI codes even though stdout is wired to a real TTY. Before the fix, this case fails.

#### Impact on Existing Tests

- New file; no existing test affected.
- Builds the real binary via `buildBinary(t)` — adds ~1 s to the test suite (one `go build`).
- Windows is excluded via build tag because `/dev/tty` is Unix-only; task YAML allows this trade-off when no PTY library is available.

---

### Step 5: Add CHANGELOG entry

**Rationale:** Definition of Done item.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add entry under Unreleased/Fixed describing the stderr color-flag regression fix |

#### Proposed Entry

```
## [Unreleased]

### Fixed

- `curlew run`/`watch`/`exec`: stderr printers now derive their color flag
  from `os.Stderr`'s own TTY state. Previously, when stdout was a TTY and
  stderr was a pipe, ANSI escape sequences leaked into piped stderr. A new
  `newStderrPrinter` helper centralizes the pattern across every call site
  in `cmd/curlew/`. (M7-001)
```

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `cmd/curlew/main_test.go` | `TestShouldUseColor` | none | unchanged (already tests the underlying function) |
| `cmd/curlew/main_test.go` | `TestRunCmdDirect_*` (28 tests) | none | harness uses piped stdout/stderr, so behaviour is identical before and after the rename |
| `cmd/curlew/main_test.go` | `TestNewStderrPrinter_NoColorFlag` | new | assert helper composes correctly |
| `cmd/curlew/main_test.go` | `TestNewStderrPrinter_NoColorEnv` | new | assert NO_COLOR env suppresses color |
| `cmd/curlew/stream_color_test.go` | `TestStderrColorFlag` | new | four-combination regression |
| `cmd/curlew/run_test.go` | (all tests) | none | no assertion on ANSI content of stderr |
| `cmd/curlew/perf_test.go` | (all tests) | none | `perf.go:61` is an uniformity rewrite only |

## Risks and Edge Cases

- **Risk:** CI environments without `/dev/tty` (docker containers, rootless sandboxes) fail the TTY subtests.
  **Mitigation:** `t.Skipf` when `/dev/tty` open fails. The pipe-pipe, NO_COLOR, and --no-color subtests still exercise the bug-relevant code paths on any environment. The task YAML explicitly permits restricting TTY assertions to Unix.

- **Risk:** Windows build — `/dev/tty` does not exist and build tag excludes the file.
  **Mitigation:** `//go:build !windows` at the top of `stream_color_test.go`. Documented in the file's doc comment. Not a regression — Windows users already did not get TTY-specific color detection; `output.IsTerminal` uses a `*os.File`/`Stat` path that is Windows-compatible at runtime.

- **Risk:** `runCmdInner` has 552+ lines between the `useColor` declaration (line 552) and its last use (line 1044). Missing a rename causes a compile error — not a silent regression.
  **Mitigation:** `go build ./cmd/curlew` is the first verification gate. Compile errors surface immediately.

- **Edge case:** `watch.Config.UseColor` consumer.
  **Handling:** That field drives stdout-facing terminal output *inside* the watch package. Its value must remain stdout-derived — we only rename the variable, not change the flow. Documented inline in Step 2.

- **Edge case:** `shouldUseColor(os.Stderr, false)` at `perf.go:61` passes `false` as the no-color flag — hardcoded because perf has no `--no-color` parser override. Retain that `false` when switching to `newStderrPrinter(false)`.

- **Edge case:** `main.go:2699` and `main.go:2705` use `shouldUseColor(os.Stderr, false)` and `shouldUseColor(os.Stderr, opts.NoColor)` respectively. The former is a parseErr path (NoColor not yet resolved, `false` is a safe default). Preserve the `false` literal when switching to `newStderrPrinter(false)`.

- **Edge case:** NO_COLOR env var takes precedence over TTY detection. Already handled inside `shouldUseColor`; no change needed in the helper.

- **Risk:** `cmd/curlew/main.go` has call sites at lines 2927, 3244, 3251 outside the task's named functions (vaultCmd, openapiImport). Task scope lists the early-exit sites "stay as-is or adopt the new helper for uniformity."
  **Mitigation:** Adopt the helper for uniformity so the DoD grep (`NewPrinter(os.Stderr, useColor)` returns zero matches) is trivially satisfied and future call sites gravitate toward the helper. Non-behavioural change.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
grep -n 'NewPrinter(os.Stderr, useColor)' cmd/curlew/*.go || echo "clean"
./scripts/ci-local.sh --go
```

Observable verification (from task YAML):

```bash
cat > /tmp/m7-001-check.yaml <<'YAML'
name: stream-color-check
requests:
  - name: missing-var
    method: GET
    url: "{{UNDEFINED}}"
YAML
./curlew run /tmp/m7-001-check.yaml > /tmp/out.txt 2> /tmp/err.txt || true
grep -c $'\033\[' /tmp/err.txt   # Expected: 0
go test -run TestStderrColorFlag ./cmd/curlew/...  # Expected: PASS
```

## Decisions Recorded

- **PTY dependency:** Not adding `creack/pty`. Using `/dev/tty` on Unix with `t.Skipf` fallback, plus three pipe-based non-TTY subtests. Rationale in "PTY dependency decision" above.
- **Uniformity rewrite of already-correct sites:** Adopt `newStderrPrinter` at all nine already-correct stderr sites (including lines 2927, 3244, 3251, and `perf.go:61`) so the DoD grep returns zero and the anti-pattern can't be reintroduced by copy-paste later.
- **`watch.Config.UseColor` semantics preserved:** That field feeds stdout-facing watch output; only the local variable name changes (`useColor` -> `stdoutUseColor`).
- **`perf.go` inclusion:** perf.go has one stderr-printer site. The task scope names three functions (runCmdInner, watchCmd, execCmd) but perf.go:61 sits in the same uniformity-rewrite bucket. Including it keeps the DoD grep trivially green.
