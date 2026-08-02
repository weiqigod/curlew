# Implementation Plan: M1-019

## Overview
Enhance `internal/output/` with ANSI color support via a `Printer` struct, add TTY detection, `--no-color` flag, and `NO_COLOR` env var support — delivering green/red indicators, colored summary, and automatic stripping when piped.

## Task Details
- **ID:** M1-019
- **Title:** Terminal output with colors and formatting
- **Phase:** M1: Core CLI
- **Priority:** 19
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-006 | Basic output formatting | done |

---

## Architecture Decisions

### Decision 1: `Printer` struct (not threaded bool)

All 11 existing package-level `Print*` functions take `io.Writer` as first arg. Threading a `colorEnabled bool` through each would touch ~20 call sites in `main.go` plus every function signature. Instead, create a `Printer` struct holding the writer and color mode; construct it once in `runCmd` and call methods.

### Decision 2: Zero-dependency TTY detection

Use `os.ModeCharDevice` check — no new imports beyond `os`:

```go
func IsTerminal(w io.Writer) bool {
    f, ok := w.(*os.File)
    if !ok {
        return false
    }
    fi, err := f.Stat()
    if err != nil {
        return false
    }
    return fi.Mode()&os.ModeCharDevice != 0
}
```

A `*bytes.Buffer` is not `*os.File` → returns `false` → tests automatically use no-color paths without any special wiring.

### Decision 3: Color mode resolution in `main.go`

```go
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

### Decision 4: File layout

- `internal/output/color.go` — new: ANSI constants, `IsTerminal`, `colorize` helper
- `internal/output/terminal.go` — replace 11 functions with `Printer` struct + methods
- `internal/output/terminal_test.go` — rewrite: migrate to `Printer` method tests

### Decision 5: Summary separator

Add a gray `────` separator line before the summary counts. This satisfies behavior 7 ("clear visual separator") by visually separating results from the summary.

---

## Implementation Steps

### Step 1: Create `internal/output/color.go` — ANSI constants and TTY detection
**Rationale:** Zero blast radius — new file, no existing code changes. Unblocks all subsequent color work.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/color.go` | create | ANSI constants, `IsTerminal()`, `colorize()` |
| `internal/output/color_test.go` | create | Tests for TTY detection and colorize helper |

#### New Code

```go
// color.go
package output

import (
	"io"
	"os"
)

// ANSI SGR escape codes.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiGray   = "\033[90m"
)

// IsTerminal reports whether w is connected to a terminal device.
// Returns false for *bytes.Buffer, pipes, and redirected files.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// colorize wraps s with an ANSI code and reset when enabled.
// Returns s unchanged when enabled is false or s is empty.
func colorize(s, code string, enabled bool) string {
	if !enabled || s == "" {
		return s
	}
	return code + s + ansiReset
}
```

#### Tests to Write FIRST (RED phase)

```go
// color_test.go
func TestIsTerminal(t *testing.T) {
    tests := []struct {
        name string
        w    io.Writer
        want bool
    }{
        {"bytes.Buffer is not terminal", &bytes.Buffer{}, false},
        {"strings.Builder is not terminal", &strings.Builder{}, false},
        {"nil-type writer is not terminal", (*bytes.Buffer)(nil), false},
    }
    // ...
}

func TestColorize(t *testing.T) {
    tests := []struct {
        name    string
        s       string
        code    string
        enabled bool
        want    string
    }{
        {"enabled wraps with code and reset", "hello", ansiGreen, true, "\033[32mhello\033[0m"},
        {"disabled returns unchanged", "hello", ansiGreen, false, "hello"},
        {"empty string with enabled returns empty", "", ansiGreen, true, ""},
        {"empty string with disabled returns empty", "", ansiGreen, false, ""},
    }
    // ...
}
```

#### Impact on Existing Tests
- None — new file only.

---

### Step 2: Create `Printer` struct — replace `terminal.go` content
**Rationale:** Core behavior change. By rewriting `terminal.go` (not adding a new file), we keep the file count low and avoid a dead-code period.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify (replace all content) | Replace 11 functions with `Printer` struct + 10 methods |
| `internal/output/terminal_test.go` | modify (replace all content) | Migrate tests to `Printer` methods, add color/no-color variants |

#### Current Code (to be replaced)

```go
// All 11 package-level functions:
func PrintResult(w io.Writer, ...) { ... }
func PrintAssertionDetail(w io.Writer, ...) { ... }
func PrintCollectionHeader(w io.Writer, ...) { ... }
// ... etc.
```

#### New Code

```go
// terminal.go
package output

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/httpexec"
)

// Printer renders test output to a writer with optional ANSI color support.
type Printer struct {
	w     io.Writer
	color bool
}

// NewPrinter creates a Printer. If color is true, ANSI codes are emitted.
func NewPrinter(w io.Writer, color bool) *Printer {
	return &Printer{w: w, color: color}
}

func (p *Printer) Result(name string, result *httpexec.Result, passed bool) {
	indicator := "✓"
	code := ansiGreen
	if !passed {
		indicator = "✗"
		code = ansiRed
	}
	_, _ = fmt.Fprintf(p.w, "  %s %s  %d  %dms\n",
		colorize(indicator, code, p.color), name, result.StatusCode, result.Duration.Milliseconds())
}

func (p *Printer) AssertionDetail(assertType, expected, actual string) {
	detail := fmt.Sprintf("✗ %s: expected %s, got %s", assertType, expected, actual)
	_, _ = fmt.Fprintf(p.w, "    %s\n", colorize(detail, ansiRed, p.color))
}

func (p *Printer) CollectionHeader(name string) {
	_, _ = fmt.Fprintf(p.w, "%s\n", colorize("Collection: "+name, ansiBold, p.color))
}

func (p *Printer) SectionHeader(section string) {
	_, _ = fmt.Fprintf(p.w, "\n%s\n", colorize(section+":", ansiBold+ansiCyan, p.color))
}

func (p *Printer) Warning(msg string) {
	_, _ = fmt.Fprintf(p.w, "%s\n", colorize("Warning: "+msg, ansiYellow, p.color))
}

func (p *Printer) Skipped(name string) {
	_, _ = fmt.Fprintf(p.w, "  %s  SKIPPED\n", colorize(name, ansiGray, p.color))
}

func (p *Printer) SummaryWithDuration(total, passed, failed, skipped int, duration time.Duration) {
	sep := strings.Repeat("─", 32)
	_, _ = fmt.Fprintf(p.w, "\n%s\n", colorize(sep, ansiGray, p.color))

	passedStr := fmt.Sprintf("%d passed", passed)
	failedStr := fmt.Sprintf("%d failed", failed)
	durationStr := fmt.Sprintf("(%dms)", duration.Milliseconds())

	if p.color {
		passedStr = colorize(passedStr, ansiGreen, true)
		if failed > 0 {
			failedStr = colorize(failedStr, ansiRed, true)
		}
		durationStr = colorize(durationStr, ansiGray, true)
	}

	if skipped > 0 {
		skippedStr := fmt.Sprintf("%d skipped", skipped)
		if p.color {
			skippedStr = colorize(skippedStr, ansiYellow, true)
		}
		_, _ = fmt.Fprintf(p.w, "  %d request(s): %s, %s, %s %s\n",
			total, passedStr, failedStr, skippedStr, durationStr)
	} else {
		_, _ = fmt.Fprintf(p.w, "  %d request(s): %s, %s %s\n",
			total, passedStr, failedStr, durationStr)
	}
}

func (p *Printer) Error(msg string) {
	_, _ = fmt.Fprintf(p.w, "%s\n", colorize("Error: "+msg, ansiRed, p.color))
}

func (p *Printer) StructuredError(err error) {
	formatted := apierrors.Format(err)
	_, _ = fmt.Fprintln(p.w, colorize(formatted, ansiRed, p.color))
}

func (p *Printer) RequestError(name string, err error) {
	var netErr *apierrors.NetworkError
	if errors.As(err, &netErr) {
		_, _ = fmt.Fprintf(p.w, "%s\n", colorize(fmt.Sprintf("[ERROR] %s — %s", name, netErr.Message), ansiRed, p.color))
		if netErr.Hint != "" {
			_, _ = fmt.Fprintf(p.w, "  Hint: %s\n", colorize(netErr.Hint, ansiGray, p.color))
		}
		return
	}
	_, _ = fmt.Fprintf(p.w, "%s\n", colorize(fmt.Sprintf("[ERROR] %s — %s", name, err), ansiRed, p.color))
}
```

#### Tests to Write FIRST (RED phase)

```go
// terminal_test.go (rewritten)
// Pattern: table-driven, use Printer{color: false} to match current no-color behaviour,
// use Printer{color: true} to verify ANSI codes are emitted.

func TestPrinterResult(t *testing.T) {
    tests := []struct {
        name      string
        rName     string
        result    *httpexec.Result
        passed    bool
        color     bool
        wantParts []string
        wantANSI  bool
    }{
        {"passing no-color shows checkmark", "Get Users", &httpexec.Result{StatusCode: 200, Duration: 150 * time.Millisecond}, true, false, []string{"✓", "Get Users", "200", "150ms"}, false},
        {"failing no-color shows X", "Get Users", &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, false, false, []string{"✗", "Get Users"}, false},
        {"passing with-color emits green ANSI", "Get", &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond}, true, true, []string{"✓", "\033[32m"}, true},
        {"failing with-color emits red ANSI", "Get", &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond}, false, true, []string{"✗", "\033[31m"}, true},
    }
    // ...
}

func TestPrinterAssertionDetail(t *testing.T) { ... }
func TestPrinterCollectionHeader(t *testing.T) { ... }
func TestPrinterSectionHeader(t *testing.T) { ... }
func TestPrinterWarning(t *testing.T) { ... }
func TestPrinterSkipped(t *testing.T) { ... }

func TestPrinterSummaryWithDuration(t *testing.T) {
    tests := []struct {
        name      string
        total     int
        passed    int
        failed    int
        skipped   int
        duration  time.Duration
        color     bool
        wantParts []string
        wantNot   []string
    }{
        {"all passed no-color", 3, 3, 0, 0, 150*time.Millisecond, false, []string{"3 request(s)", "3 passed", "0 failed", "150ms"}, []string{"skipped", "\033["}},
        {"some failed no-color", 3, 2, 1, 0, 200*time.Millisecond, false, []string{"3 request(s)", "2 passed", "1 failed", "200ms"}, []string{"\033["}},
        {"with skipped no-color", 3, 1, 1, 1, 100*time.Millisecond, false, []string{"3 request(s)", "1 passed", "1 failed", "1 skipped", "100ms"}, nil},
        {"separator line present", 3, 3, 0, 0, 50*time.Millisecond, false, []string{"───"}, nil},
        {"passed green with color", 3, 3, 0, 0, 50*time.Millisecond, true, []string{"\033[32m", "3 passed"}, nil},
        {"failed red with color", 3, 2, 1, 0, 50*time.Millisecond, true, []string{"\033[31m", "1 failed"}, nil},
        {"passed green no red when no failures", 3, 3, 0, 0, 50*time.Millisecond, true, []string{"\033[32m"}, []string{"\033[31m"}},
        {"skipped yellow with color", 3, 1, 1, 1, 50*time.Millisecond, true, []string{"\033[33m", "1 skipped"}, nil},
        {"duration gray with color", 3, 3, 0, 0, 50*time.Millisecond, true, []string{"\033[90m"}, nil},
    }
    // ...
}

func TestPrinterError(t *testing.T) { ... }

func TestPrinterStructuredError(t *testing.T) {
    // Must match exact format (without ANSI codes when color: false)
    // e.g., "[ERROR] f.yaml:3 — bad\n"
}

func TestPrinterRequestError(t *testing.T) {
    // Must match exact format when color: false
}
```

#### Impact on Existing Tests

All existing tests in `terminal_test.go` will be **replaced** — the function signatures change from `Print*(buf, ...)` to `NewPrinter(buf, false).Method(...)`. The logical assertions remain the same; only the call site changes.

Tests that checked exact output strings (`TestPrintStructuredError`, `TestPrintRequestError`, `TestPrintSectionHeader`) must be re-expressed with `Printer{color: false}` — they will continue to pass because no-color output matches the previous plain-text format.

---

### Step 3: Update `cmd/curlew/main.go` — wire `Printer` and `--no-color`
**Rationale:** After Steps 1–2 compile and all `output.Print*` package-level functions are removed, `main.go` won't compile. This step re-wires everything. Keeping it last for `main.go` minimises time in broken-build state.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `--no-color` flag, `shouldUseColor()`, construct `Printer`, replace 20 call sites, update help |
| `cmd/curlew/main_test.go` | modify | Update `TestParseRunArgs` for new return value, add no-color tests |

#### Current Code (excerpt)

```go
func parseRunArgs(args []string) (file, envName string, vars, envVarVars map[string]string, seed *int64, err error) {
    // ...
    switch args[i] {
    case "--var": ...
    case "--env-var": ...
    case "--env": ...
    case "--seed": ...
    default:
        positional = append(positional, args[i])
    }
}
```

#### New Code

```go
func parseRunArgs(args []string) (file, envName string, vars, envVarVars map[string]string, seed *int64, noColor bool, err error) {
    vars = make(map[string]string)
    envVarVars = make(map[string]string)
    var positional []string
    for i := 0; i < len(args); i++ {
        switch args[i] {
        case "--no-color":
            noColor = true
        case "--var": ...
        // ... (unchanged)
        }
    }
    // ...
}

func shouldUseColor(w io.Writer, noColorFlag bool) bool {
    if noColorFlag {
        return false
    }
    if _, set := os.LookupEnv("NO_COLOR"); set {
        return false
    }
    return output.IsTerminal(w)
}

func runCmd(args []string) int {
    file, envName, cliVars, envVarVars, seed, noColor, parseErr := parseRunArgs(args)
    // ...
    useColor := shouldUseColor(os.Stdout, noColor)
    out := output.NewPrinter(os.Stdout, useColor)
    errOut := output.NewPrinter(os.Stderr, useColor)

    // Replace all output.PrintX(os.Stdout, ...) with out.X(...)
    // Replace all output.PrintX(os.Stderr, ...) with errOut.X(...)
}
```

**All 20 call site replacements in `runCmd`:**

| Before | After |
|--------|-------|
| `output.PrintStructuredError(os.Stderr, parseErr)` | `errOut.StructuredError(parseErr)` |
| `output.PrintStructuredError(os.Stderr, err)` | `errOut.StructuredError(err)` |
| `output.PrintWarning(os.Stderr, "collection has no requests")` | `errOut.Warning("collection has no requests")` |
| `output.PrintCollectionHeader(os.Stdout, col.Name)` | `out.CollectionHeader(col.Name)` |
| `output.PrintSectionHeader(os.Stdout, "Setup")` | `out.SectionHeader("Setup")` |
| `output.PrintSectionHeader(os.Stdout, "Teardown")` | `out.SectionHeader("Teardown")` |
| `output.PrintSkipped(os.Stdout, r.Name)` | `out.Skipped(r.Name)` |
| `output.PrintRequestError(os.Stderr, r.Name, r.Err)` | `errOut.RequestError(r.Name, r.Err)` |
| `output.PrintResult(os.Stdout, r.Name, r.Result, passed)` | `out.Result(r.Name, r.Result, passed)` |
| `output.PrintAssertionDetail(os.Stdout, ar.Type, ar.Expected, ar.Actual)` | `out.AssertionDetail(ar.Type, ar.Expected, ar.Actual)` |
| `output.PrintSummaryWithDuration(os.Stdout, ...)` | `out.SummaryWithDuration(...)` |

#### Help text addition

```go
fmt.Println("  --no-color          Disable colored output (also respects NO_COLOR env var)")
```

#### Tests to Write FIRST (RED phase)

```go
// main_test.go updates

func TestParseRunArgs(t *testing.T) {
    // All existing cases get `noColor: false` added to expected values
    // New cases:
    {
        name:    "no-color flag sets noColor",
        args:    []string{"file.yaml", "--no-color"},
        want:    parseRunResult{file: "file.yaml", noColor: true},
    },
    {
        name: "no-color alongside other flags",
        args: []string{"file.yaml", "--no-color", "--env", "dev"},
        want: parseRunResult{file: "file.yaml", envName: "dev", noColor: true},
    },
}

func TestShouldUseColor(t *testing.T) {
    tests := []struct {
        name        string
        noColorFlag bool
        noColorEnv  bool  // whether to set NO_COLOR env var
        writer      io.Writer
        want        bool
    }{
        {"buffer non-TTY returns false", false, false, &bytes.Buffer{}, false},
        {"no-color flag returns false", true, false, &bytes.Buffer{}, false},
        {"NO_COLOR env returns false", false, true, &bytes.Buffer{}, false},
    }
    // ...
}

func TestHelpText_noColor(t *testing.T) {
    // Capture stdout, run printHelp(), verify --no-color present
}

func TestIntegration_noColorFlag(t *testing.T) {
    // Run binary with --no-color, verify no "\033[" in output
}

func TestIntegration_NOCOLOREnv(t *testing.T) {
    // Run binary with NO_COLOR=1, verify no "\033[" in output
}

func TestIntegration_pipeStripANSI(t *testing.T) {
    // Run binary capturing stdout via pipe (os/exec), verify no "\033[" in output
    // (TTY detection returns false when stdout is a pipe)
}
```

#### Impact on Existing Tests

- `TestParseRunArgs` — every existing test case needs `noColor: false` added to expected output (mechanical change).
- Integration tests in `main_test.go` that capture output via `os/exec` will still work — piped stdout is not a TTY, so no ANSI codes.
- `TestRunCmd_*` tests using `bytes.Buffer` for output capture are unaffected.

---

### Step 4: Update smoke test
**Rationale:** Final verification that the observable behavior works end-to-end.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add `--no-color` and `NO_COLOR` verification sections |

#### New Code (append to `smoke/run.sh` before final "=== Smoke Test Complete ===")

```bash
echo "--- Running with --no-color flag (expect no ANSI codes) ---"
OUTPUT=$(./curlew run sample/hello.yaml --no-color 2>&1)
if printf '%s' "$OUTPUT" | grep -qP '\x1b\['; then
  echo "FAIL: ANSI codes found with --no-color"
  exit 1
fi
echo "PASS: no ANSI codes with --no-color"
echo

echo "--- Running with NO_COLOR env var (expect no ANSI codes) ---"
OUTPUT=$(NO_COLOR=1 ./curlew run sample/hello.yaml 2>&1)
if printf '%s' "$OUTPUT" | grep -qP '\x1b\['; then
  echo "FAIL: ANSI codes found with NO_COLOR=1"
  exit 1
fi
echo "PASS: no ANSI codes with NO_COLOR=1"
echo

echo "--- Help text shows --no-color ---"
HELP_OUTPUT=$(./curlew --help)
echo "$HELP_OUTPUT" | grep -q "\-\-no-color" && echo "PASS: --no-color in help" || { echo "FAIL: Missing --no-color in help output"; exit 1; }
echo
```

#### Impact on Existing Tests
- All existing smoke tests use piped subshells — TTY detection returns false — so no ANSI codes appear in existing test output. No changes needed to existing checks.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/output/terminal_test.go` | All `TestPrint*` functions | replaced | Rewrite using `Printer{color: false}` — same assertions, new call sites |
| `cmd/curlew/main_test.go` | `TestParseRunArgs` | breaks | Add `noColor bool` to expected struct, add `false` to all existing cases |
| `cmd/curlew/main_test.go` | New tests | new | `TestShouldUseColor`, `TestIntegration_noColorFlag`, `TestIntegration_NOCOLOREnv`, `TestHelpText_noColor` |

---

## Risks and Edge Cases

- **Risk:** `SectionHeader` format change from `"\nSetup:\n"` to `"\nSetup:\n"` (with bold ANSI when enabled) — smoke test `grep -q "Setup:"` still passes because ANSI wraps the whole string but "Setup:" is inside it. Piped output has no ANSI so exact match works.
  → **Mitigation:** With `color: false`, `SectionHeader` must produce `"\nSetup:\n"` exactly — verified by existing-style tests.

- **Risk:** `SummaryWithDuration` output format changes from `"\n3 request(s): 2 passed, 1 failed (150ms)\n"` to `"\n───\n  3 request(s): 2 passed, 1 failed (150ms)\n"` — smoke test checks `grep -q "3 request(s)"` which still passes. But any test checking the exact string format will break.
  → **Mitigation:** Update `TestPrinterSummaryWithDuration` to use `wantParts` (substring checks) not exact string equality.

- **Risk:** Smoke test `grep -qP '\x1b\['` uses Perl regex — not available on all macOS installations.
  → **Mitigation:** Use `grep -q $'\033\['` (ANSI literal in shell) or `LC_ALL=C grep -q $'\033'` as fallback. Or use `printf '%s' | od -c | grep -q 033` for portability.

- **Risk:** `ansiBold+ansiCyan` concatenation produces `"\033[1m\033[36m"` — the `colorize` helper wraps the whole string in one code+reset, not two. Bold+cyan requires either combining them or a second wrap.
  → **Mitigation:** For `SectionHeader`, use `colorize(colorize(section+":", ansiBold, p.color), ansiCyan, p.color)` or define a combined constant `ansiBoldCyan = "\033[1;36m"`.

- **Edge case:** `NO_COLOR` spec says any non-empty value disables color (even `NO_COLOR=0`). The `os.LookupEnv("NO_COLOR")` check returns `(value, true)` if set at all — the value is ignored. This matches the spec.

- **Edge case:** `--no-color` should appear in `parseRunArgs` return values even when passed before or after other flags. All flag positions handled by the loop.

---

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# In terminal — see colored output
./curlew run sample/hello.yaml

# Piped — no ANSI codes
./curlew run sample/hello.yaml | cat

# --no-color flag
./curlew run sample/hello.yaml --no-color

# NO_COLOR env var
NO_COLOR=1 ./curlew run sample/hello.yaml
```
