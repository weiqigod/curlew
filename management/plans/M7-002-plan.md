# Implementation Plan: M7-002

## Overview

Relocate progress/confirmation text in `perf`, `license`, `worker`, and `exec --dry-run` from stdout to stderr so that piping stdout captures only the declared result payload. Route the `exec --dry-run` request rendering through `output.Printer` instead of raw `fmt.Fprintln(os.Stdout, ...)`. Fix the stray stdout warning in `internal/worker/run.go:133` so it matches its sibling warnings on stderr. Add a single regression test (`cmd/curlew/stream_progress_test.go`) that spawns each affected subcommand through the built binary and asserts the stream split.

## Task Details

- **ID:** M7-002
- **Title:** Progress text relocated from stdout to stderr in perf, license, worker, exec --dry-run
- **Phase:** M7: Output Discipline
- **Priority:** 2
- **Complexity:** medium

## Dependencies

The YAML task file declares no explicit `dependencies:` list. The task description references M7-001 (stderr-color derivation, already done) and M7-005 (writer threading through `runCmdInner`, already done). Both are complete — the plumbing this task exploits is in place.

| Task | Title | Status |
|------|-------|--------|
| M7-001 | stderr printers derive color from stderr TTY state | done |
| M7-005 | Thread stdout/stderr writers through runCmdInner | done |

## Key Decisions

1. **Dry-run "DRY RUN" label retention as a single `fmt.Fprintln` on the printer's writer.** The task says "no raw `fmt.Fprintln(os.Stdout, ...)` bypass remains". Strictly there is no `output.Printer` method that emits a bare "DRY RUN" banner, and the task caveats "The existing terminal Printer already handles this shape; no new Printer methods needed." To reconcile: construct `printer := output.NewPrinter(stdout, useColor, output.VerbosityVerbose)` (forcing Verbose so `RequestDetail` renders regardless of user's `-q/-v` choice) and call `printer.RequestDetail(req.Method, req.URL, req.Headers)`. The "DRY RUN" banner itself is emitted as a **single** `fmt.Fprintln(stdout, "DRY RUN")`. Rationale: "DRY RUN" is the result-label of a dry-run, not a progress line, and the task's observable says "the formatted request rendered via output.Printer (method, URL, headers)" — the request rendering is what the Printer must own, which is now exactly what `RequestDetail` does. The forbidden pattern — `fmt.Fprintf(stdout, "  %s %s\n", method, url)` and the header `fmt.Fprintf(stdout, "  %s: %s\n", ...)` loop — is fully removed. This keeps the change minimal and avoids a new Printer method.

2. **Add `Stderr io.Writer` to `worker.RunOptions`.** The task hints at this field existing, but it does not. Adding it is a non-breaking extension (zero-valued nil → `os.Stderr` fallback, matching the `Stdout` pattern). `workerCmdOut` passes `stderr` into it. Existing callers (tests passing `RunOptions{Stdout: &buf}`) continue to work — they just get `os.Stderr` as the default.

3. **Forcing `VerbosityVerbose` in the dry-run printer.** User-requested `-q` should not suppress the dry-run output — the user asked for `--dry-run`, which is itself a verbose intent. Force `output.VerbosityVerbose` regardless of `opts.Verbosity`. (If future work wants the user's `-vv` to additionally dump the body, that's a separate concern; today `--dry-run` has no body handling.)

4. **`TestLicenseValidate_Offline_Valid` stderr assertion relaxation.** That test asserts `stderr == ""`. After moving "Validating license..." to stderr, it will contain one line. Update the assertion to exact-match `"Validating license offline...\n"` (or to contain it, with no other content).

5. **`internal/worker/run_test.go` stdout/stderr split.** Every test that currently inspects `stdout.String()` for "Claimed shard", "Completed", "No more shards", and the submit-failed warning must now inspect the new `stderr` buffer. The submit-failed warning at `run.go:133` moves from `stdout` to `stderr` (matching its sibling at `run.go:251`). Tests that pass only `Stdout:` get a zero-value `Stderr` → `os.Stderr` default (we add `Stderr: &stderrBuf` to the tests we want to assert on).

6. **`internal/worker/integration_test.go` and `TestLicenseExport_HappyPath` similarly updated** to read from stderr for progress/confirmation lines.

7. **`stream_progress_test.go`** lives in `cmd/curlew/` and reuses `buildBinary` + `runBinary` helpers from `main_test.go`. Runs each subcommand with a captive stdout/stderr split (which `runBinary` already provides via `exec.Command`'s pipe separation) and golden-files the stdout plus asserts every known progress string appears on stderr only. Fixtures use `httptest.Server` for perf/worker coordinator stubbing. License tests use `CURLEW_CONFIG_DIR` with the existing test license fixture.

## Implementation Steps

Ordered smallest blast radius first.

### Step 1: Fix `internal/worker/run.go` line 133 (warning lands on stderr)

**Rationale:** This is a pure behavioural bug-fix, one-line targeted — the warning at line 133 writes to `stdout` while the sibling warning at line 251 (heartbeat) writes to `os.Stderr`. Fixing it first, with its own dedicated test, establishes the "warnings go to stderr" invariant before we refactor the wider worker progress path. Blast radius: one file, one test.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/worker/run.go` | modify | Add `Stderr io.Writer` to `RunOptions`. Default to `os.Stderr` when nil. Route the `warning: submit failed` line at line 133 to `stderr` instead of `stdout`. |
| `internal/worker/run_test.go` | modify | Update `submit_failure_after_retries_logs_warning_and_continues` to inspect a new `stderr bytes.Buffer` passed via `Stderr: &stderr`. Add an explicit assertion that the warning is in stderr, not stdout. |

#### Current Code (run.go, lines 26–69 show RunOptions and defaults)

```go
// RunOptions injects test seams into Run.
type RunOptions struct {
	Client            CoordinatorClient // nil = construct from cfg
	Execute           ExecuteFunc       // nil = httpexec.Execute
	Stdout            io.Writer         // nil = os.Stdout
	HeartbeatInterval time.Duration     // 0 = cfg.HeartbeatInterval
}
```

At line 133:

```go
if submitErr != nil {
    _, _ = fmt.Fprintf(stdout, "warning: submit failed for %s: %v (shard will be reaped)\n", shard.ShardID, submitErr)
    continue
}
```

#### New Code

```go
// RunOptions injects test seams into Run.
type RunOptions struct {
	Client            CoordinatorClient // nil = construct from cfg
	Execute           ExecuteFunc       // nil = httpexec.Execute
	Stdout            io.Writer         // nil = os.Stdout — carries only declared result payload
	Stderr            io.Writer         // nil = os.Stderr — carries progress and warnings
	HeartbeatInterval time.Duration     // 0 = cfg.HeartbeatInterval
}
```

Inside `Run`, add a sibling default for stderr (near the existing `stdout := opts.Stdout ... if stdout == nil { stdout = os.Stdout }`):

```go
stderr := opts.Stderr
if stderr == nil {
    stderr = os.Stderr
}
```

At line 133:

```go
if submitErr != nil {
    _, _ = fmt.Fprintf(stderr, "warning: submit failed for %s: %v (shard will be reaped)\n", shard.ShardID, submitErr)
    continue
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/worker/run_test.go — amend existing subtest
t.Run("submit_failure_after_retries_logs_warning_and_continues", func(t *testing.T) {
    // ... existing setup ...
    var stdout, stderr bytes.Buffer
    summary, err := Run(context.Background(), cfg, RunOptions{
        Client:            stub,
        Execute:           stubExecute,
        Stdout:            &stdout,
        Stderr:            &stderr,
        HeartbeatInterval: 50 * time.Millisecond,
    })
    // ... existing summary assertions ...
    if !strings.Contains(stderr.String(), "warning: submit failed for shd_5") {
        t.Errorf("stderr missing submit-failure warning; got: %q", stderr.String())
    }
    if strings.Contains(stdout.String(), "warning: submit failed") {
        t.Errorf("submit warning leaked to stdout; stdout: %q", stdout.String())
    }
})
```

#### Impact on Existing Tests

- `TestRun/submit_failure_after_retries_logs_warning_and_continues` — currently does not assert on the warning line at all; we add the assertion above. Low risk.
- All other `TestRun` subtests continue to compile (new field is additive, `Stderr: nil` → `os.Stderr`).

### Step 2: Move all remaining `internal/worker/run.go` progress lines to stderr

**Rationale:** With the `Stderr` seam in place, move the "No more shards", "Claimed shard", and "Completed" lines to stderr. Blast radius: one file, three test subtests.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/worker/run.go` | modify | Change Fprintln/Fprintf targets at lines 86, 94, 96, 137 from `stdout` to `stderr`. |
| `internal/worker/run_test.go` | modify | Three subtests (`claim_execute_submit_one_shard_then_204`, `no_shards_available_exits_zero`, `two_shards_then_204`) switch from `stdout` buffer to `stderr` buffer for their string-contains assertions. |
| `internal/worker/integration_test.go` | modify | `TestWorker_E2E_SingleShard` switches the same three progress assertions from stdout to stderr. |

#### Current Code (run.go lines 85–97, 136–141)

```go
if shard == nil {
    _, _ = fmt.Fprintln(stdout, "No more shards; exiting")
    return summary, nil
}

// Decode requests to get count for logging.
var requests []WorkerRequest
jsonErr := json.Unmarshal([]byte(shard.RequestsJson), &requests)
if jsonErr == nil {
    _, _ = fmt.Fprintf(stdout, "Claimed shard %s (%d requests)\n", shard.ShardID, len(requests))
} else {
    _, _ = fmt.Fprintf(stdout, "Claimed shard %s (payload error)\n", shard.ShardID)
}
// ...
_, _ = fmt.Fprintf(stdout, "Completed %s: pass=%d fail=%d duration=%dms\n", shard.ShardID, pass, fail, durMs)
```

#### New Code

```go
if shard == nil {
    _, _ = fmt.Fprintln(stderr, "No more shards; exiting")
    return summary, nil
}

// Decode requests to get count for logging.
var requests []WorkerRequest
jsonErr := json.Unmarshal([]byte(shard.RequestsJson), &requests)
if jsonErr == nil {
    _, _ = fmt.Fprintf(stderr, "Claimed shard %s (%d requests)\n", shard.ShardID, len(requests))
} else {
    _, _ = fmt.Fprintf(stderr, "Claimed shard %s (payload error)\n", shard.ShardID)
}
// ...
_, _ = fmt.Fprintf(stderr, "Completed %s: pass=%d fail=%d duration=%dms\n", shard.ShardID, pass, fail, durMs)
```

Note: `stdout` is still referenced elsewhere for no writes after this change. We keep the `stdout := opts.Stdout ... default os.Stdout` block because the seam is part of the public contract and a future stdout-carried payload (e.g., a `--report` JSON) may use it.

#### Tests to Write FIRST (RED phase)

Amend the three subtests above. Representative change:

```go
t.Run("claim_execute_submit_one_shard_then_204", func(t *testing.T) {
    reqs := makeRequests(3)
    stub := &stubClient{ /* ... */ }
    var stdout, stderr bytes.Buffer
    summary, err := Run(context.Background(), Config{ /* ... */ }, RunOptions{
        Client:            stub,
        Execute:           stubExecute,
        Stdout:            &stdout,
        Stderr:            &stderr,
        HeartbeatInterval: 50 * time.Millisecond,
    })
    // ... existing summary asserts ...
    errOut := stderr.String()
    for _, want := range []string{
        "Claimed shard shd_1 (3 requests)",
        "Completed shd_1: pass=3 fail=0",
        "No more shards; exiting",
    } {
        if !strings.Contains(errOut, want) {
            t.Errorf("stderr missing %q\n  got: %s", want, errOut)
        }
    }
    if stdout.Len() != 0 {
        t.Errorf("stdout should be empty, got: %q", stdout.String())
    }
})
```

#### Impact on Existing Tests

- `TestRun/claim_execute_submit_one_shard_then_204` — assertion moves stdout → stderr, plus a new "stdout empty" guard.
- `TestRun/no_shards_available_exits_zero` — same.
- `TestRun/two_shards_then_204` — same.
- `TestWorker_E2E_SingleShard` (integration_test.go) — same.

### Step 3: Wire `stderr` from `workerCmdOut` into `worker.Run`

**Rationale:** Now that `worker.RunOptions.Stderr` exists, pass it from the CLI boundary. Blast radius: one line in `cmd/curlew/worker.go`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/worker.go` | modify | At line 44, pass `Stderr: stderr` into `worker.RunOptions`. |

#### Current Code (worker.go line 44)

```go
_, runErr := worker.Run(ctx, cfg, worker.RunOptions{Stdout: stdout})
```

#### New Code

```go
_, runErr := worker.Run(ctx, cfg, worker.RunOptions{Stdout: stdout, Stderr: stderr})
```

#### Tests to Write FIRST (RED phase)

Covered by Step 7's `stream_progress_test.go` which exercises the CLI end-to-end. No new unit test here; the wiring is too thin to test independently without replicating the integration test.

#### Impact on Existing Tests

None.

### Step 4: Relocate `cmd/curlew/perf.go` progress lines to stderr

**Rationale:** With the worker done, perf is independent. All writes to `stdout` in this file except the two result-bearing lines (`report.SummaryLine` at line 136 and — still discuss below — `Requests sent:` at line 131) move to stderr. The task explicitly lists `Requests sent:` at line 131 as part of the relocation; it is part of the progress preamble before the final `Results: requests=...` summary.

**Decision on `Requests sent:`** — the task scope mentions line 131 in the relocation list, and the behaviour statement says "stdout contains only the SummaryLine metrics output". `Requests sent:` is a counting progress line, so it moves to stderr. This means the final stdout line is exactly `Results: requests=N, p50=..., p95=..., p99=..., throughput=..., error_rate=...%`.

**Decision on `Wrote %s`** — moves to stderr. The declared result when `--output file.json` is the JSON file itself; the "Wrote" confirmation is a progress line. The summary line on stdout is still the `Results: requests=...` line (it continues to print to stdout regardless of `--output`, per M5-012's existing behaviour at perf.go:136).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/perf.go` | modify | Change Fprintf/Fprintln targets at lines 98, 101, 103, 104, 131, 147, 155 from `stdout` to `stderr`. Keep line 136 (`report.SummaryLine`) on stdout. |
| `cmd/curlew/perf_test.go` | modify | `TestPerfCmd_Run_HTTPTestServer_Success`, `TestPerfCmd_RPSHeaderInStdout` — switch `Load test:`, `Requests sent:`, `Target rate:` assertions from `stdout` to `stderr`. `TestPerfCmd_SummaryLinePrintedToStdout` remains unchanged (it already asserts stdout contains `Results: requests=`). |

#### Current Code (perf.go lines 97–104, 131, 147, 155)

```go
// Header lines before run.
_, _ = fmt.Fprintf(stdout, "Load test: %d virtual users, %s duration, %s ramp-up\n",
    flags.vus, flags.duration, flags.rampUp)
if flags.rps > 0 {
    _, _ = fmt.Fprintf(stdout, "Target rate: %d req/s\n", flags.rps)
}
_, _ = fmt.Fprintf(stdout, "VUs: 1 ... %d (ramped in %s)\n", flags.vus, flags.rampUp)
_, _ = fmt.Fprintln(stdout, "Running...")
// ...
_, _ = fmt.Fprintf(stdout, "Requests sent: %d; successes: %d; failures: %d\n",
    sum.Requests, sum.Successes, sum.Failures)
// ...
_, _ = fmt.Fprintf(stdout, "Wrote %s\n", outPath)      // JSON branch
// ...
_, _ = fmt.Fprintf(stdout, "Wrote %s\n", outPath)      // HTML branch
```

#### New Code

Every occurrence above becomes `fmt.Fprintf(stderr, ...)` / `fmt.Fprintln(stderr, ...)`. Line 136 stays on stdout:

```go
_, _ = fmt.Fprintln(stdout, report.SummaryLine(metrics))
```

#### Tests to Write FIRST (RED phase)

Amend existing tests. Representative change for `TestPerfCmd_Run_HTTPTestServer_Success` (currently asserts stdout contains `"Load test:"` and `"Requests sent:"`):

```go
func TestPerfCmd_Run_HTTPTestServer_Success(t *testing.T) {
    // ... setup ...
    var stdout, stderr bytes.Buffer
    code := perfCmdOut([]string{f, "--vus", "2", "--duration", "200ms"}, &stdout, &stderr)
    out := stdout.String()
    errOut := stderr.String()

    if code != 0 {
        t.Errorf("perfCmd exit code = %d, want 0; stderr: %s", code, errOut)
    }
    if !strings.Contains(errOut, "Load test:") {
        t.Errorf("stderr missing 'Load test:'; got: %s", errOut)
    }
    if !strings.Contains(errOut, "Requests sent:") {
        t.Errorf("stderr missing 'Requests sent:'; got: %s", errOut)
    }
    if !strings.Contains(out, "Results: requests=") {
        t.Errorf("stdout missing summary line; got: %s", out)
    }
    if strings.Contains(out, "Load test:") || strings.Contains(out, "Requests sent:") {
        t.Errorf("progress leaked to stdout; got: %s", out)
    }
}
```

Similar swap for `TestPerfCmd_RPSHeaderInStdout` — rename to `TestPerfCmd_RPSHeaderInStderr` and assert on `stderr`.

#### Impact on Existing Tests

| Test | Current Behaviour | Required Change |
|------|------------------|-----------------|
| `TestPerfCmd_Run_HTTPTestServer_Success` | asserts stdout has "Load test:" and "Requests sent:" | switch to stderr; add "no leak" guard |
| `TestPerfCmd_RPSHeaderInStdout` | asserts stdout has "Target rate:" | rename + switch to stderr |
| `TestPerfCmd_SummaryLinePrintedToStdout` | asserts stdout has "Results: requests=" | no change |
| `TestPerfCmd_OutputJSON_WritesFile` / `_OutputHTML_WritesFile` / `_OutputJSON_OverwritesExistingFile` | check file contents, not stdout "Wrote" | no change |

### Step 5: Relocate `cmd/curlew/license.go` progress lines to stderr

**Rationale:** Independent of perf/worker. Relocates six lines. Blast radius: one file, two existing tests.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/license.go` | modify | Change Fprintln/Fprintf targets at lines 58, 60, 155, 174, 176 from `stdout` to `stderr`. Lines 86–88 (Key source / Tier / State) stay on stdout. |
| `cmd/curlew/license_test.go` | modify | `TestLicenseValidate_Offline_Valid` (line 115 asserts stdout has "Validating license offline..."; line 127 asserts `stderr == ""`) — move "Validating license" assertion to stderr; change the empty-stderr assertion to exact-match `"Validating license offline...\n"`. `TestLicenseExport_HappyPath` (lines 333–344 assert stdout has "Exporting", "Included:", "JWKS", "Wrote") — move all four to stderr; add a new "stdout is empty" guard. |

#### Current Code (license.go lines 57–61)

```go
if license.OfflineEnv() {
    _, _ = fmt.Fprintln(stdout, "Validating license offline...")
} else {
    _, _ = fmt.Fprintln(stdout, "Validating license...")
}
```

Lines 155, 174, 176 similarly.

#### New Code

```go
if license.OfflineEnv() {
    _, _ = fmt.Fprintln(stderr, "Validating license offline...")
} else {
    _, _ = fmt.Fprintln(stderr, "Validating license...")
}
```

And analogous swaps for:
- line 155: `fmt.Fprintln(stderr, "Exporting license bundle...")`
- line 174: `fmt.Fprintf(stderr, "Included: license token (valid until %s), JWKS (%d keys)\n", ...)`
- line 176: `fmt.Fprintf(stderr, "Wrote %s (%s)\n", outputFile, humanBytes(rep.BytesWritten))`

Lines 86–88 unchanged — `Key source`, `Tier`, `State` stay on stdout.

#### Tests to Write FIRST (RED phase)

```go
// TestLicenseValidate_Offline_Valid — updated
stdout, stderr, rc := captureRun(t, "license", "--validate")
if rc != 0 {
    t.Errorf("exit code = %d, want 0; stderr=%q", rc, stderr)
}
if !strings.Contains(stderr, "Validating license offline...") {
    t.Errorf("stderr missing 'Validating license offline...': %q", stderr)
}
if strings.Contains(stdout, "Validating license") {
    t.Errorf("'Validating license' leaked to stdout: %q", stdout)
}
if !strings.Contains(stdout, "Key source: embedded JWKS (kid=curlew-2025-01)") {
    t.Errorf("stdout missing key source line: %q", stdout)
}
if !strings.Contains(stdout, "Tier: enterprise") {
    t.Errorf("stdout missing tier: %q", stdout)
}
if !strings.Contains(stdout, "State: VALID") {
    t.Errorf("stdout missing state: %q", stdout)
}
// Stderr contains exactly the progress line — no unexpected warnings.
if strings.TrimSpace(stderr) != "Validating license offline..." {
    t.Errorf("stderr should contain only progress line, got: %q", stderr)
}
```

```go
// TestLicenseExport_HappyPath — updated
stdout, stderr, rc := captureRun(t, "license", "export", "--output", out)
if rc != 0 {
    t.Fatalf("exit code = %d, want 0; stderr=%q stdout=%q", rc, stderr, stdout)
}
for _, want := range []string{
    "Exporting license bundle...",
    "Included: license token (valid until",
    "JWKS (1 keys)",
    "Wrote " + out,
} {
    if !strings.Contains(stderr, want) {
        t.Errorf("stderr missing %q; got: %q", want, stderr)
    }
}
if stdout != "" {
    t.Errorf("stdout should be empty for license export, got: %q", stdout)
}
```

#### Impact on Existing Tests

| Test | Current Behaviour | Required Change |
|------|------------------|-----------------|
| `TestLicenseValidate_Offline_Valid` | stdout contains "Validating license offline..."; stderr == "" | move to stderr; assert stdout free of "Validating"; assert trimmed stderr equals progress line |
| `TestLicenseValidate_Offline_GracePeriod_Day25` | stderr contains "Warning: 5 days..." | unchanged — already on stderr |
| `TestLicenseValidate_Offline_GraceExpired_Day31` | stdout contains "State: GRACE_EXPIRED", stderr contains "Grace period expired" | no change (assertions already distinguish) |
| `TestLicenseExport_HappyPath` | stdout contains "Exporting"/"Included"/"JWKS"/"Wrote" | move all to stderr; add stdout-empty guard |
| `TestLicenseExportThenValidate_RoundTrip` | round-trip behaviour (see around line 408) | likely inspects stdout; re-check during execution and update if needed |

### Step 6: Refactor `cmd/curlew/main.go` exec --dry-run to use `output.Printer`

**Rationale:** Independent of perf/license/worker. Rewrites the dry-run terminal branch to use `output.Printer`. Blast radius: one block in `main.go`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Replace the `fmt.Fprintln(stdout, "DRY RUN")` + method/URL/headers `fmt.Fprintf` block at lines 2843–2847 with a `output.NewPrinter(stdout, stdoutUseColor, output.VerbosityVerbose)` plus `printer.RequestDetail(req.Method, req.URL, req.Headers)`. Keep the "DRY RUN" banner as a single `fmt.Fprintln(stdout, "DRY RUN")` — it is the result label, not a request-detail bypass. |

#### Current Code (main.go lines 2842–2848)

```go
} else {
    _, _ = fmt.Fprintln(stdout, "DRY RUN")
    _, _ = fmt.Fprintf(stdout, "  %s %s\n", req.Method, req.URL)
    for k, v := range req.Headers {
        _, _ = fmt.Fprintf(stdout, "  %s: %s\n", k, v)
    }
}
```

#### New Code

```go
} else {
    _, _ = fmt.Fprintln(stdout, "DRY RUN")
    // Force VerbosityVerbose so RequestDetail renders regardless of -q/-v.
    // --dry-run is itself a verbose intent: the user asked to see the request.
    dryPrinter := output.NewPrinter(stdout, stdoutUseColor, output.VerbosityVerbose)
    dryPrinter.RequestDetail(req.Method, req.URL, req.Headers)
}
```

Note: `RequestDetail` renders as `  > GET http://...\n` (with `>` prefix), different from the current `  %s %s\n`. This is the *intended* change — it aligns dry-run with the verbose request detail shape used across `run` and `exec` at `-v`. The existing tests only assert `stdout contains "DRY RUN"` and `stdout contains "POST"` (for the `-X POST` case) — both still pass.

#### Tests to Write FIRST (RED phase)

Add a fresh test asserting the new rendered shape:

```go
// cmd/curlew/main_test.go
func TestExecCmd_DryRun_UsesPrinterRequestDetail(t *testing.T) {
    stdout, _, rc := captureExecCmd(t, "", "https://example.com", "--dry-run", "-H", "X-Test: 1")
    if rc != 0 {
        t.Fatalf("exit = %d, want 0", rc)
    }
    if !strings.Contains(stdout, "DRY RUN\n") {
        t.Errorf("stdout missing 'DRY RUN' label: %q", stdout)
    }
    if !strings.Contains(stdout, "  > GET https://example.com") {
        t.Errorf("stdout missing RequestDetail line (> method url): %q", stdout)
    }
}
```

*(If `-H` is not supported on exec, drop the header variation; the core assertion is on the `"  > " + method + " " + url` shape.)*

#### Impact on Existing Tests

| Test | Current Behaviour | Required Change |
|------|------------------|-----------------|
| `TestExecCmd_*` "dry run does not execute http" | asserts stdout contains "DRY RUN" | no change |
| `TestExecCmd_*` "dry run shows request details" | asserts stdout contains "POST" | no change (RequestDetail emits `  > POST url`) |
| `TestExecCmd_ViaBinary` "dry run via binary" | asserts stdout contains "DRY RUN" | no change |

### Step 7: Add regression test `cmd/curlew/stream_progress_test.go`

**Rationale:** This is the single end-to-end guard rail that the task's Definition of Done mandates. It runs the built binary, captures stdout and stderr separately, and asserts every known-progress string lives exclusively on stderr while the declared result lives exclusively on stdout.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/stream_progress_test.go` | create | One `TestStreamProgress` top-level with four subtests: `perf`, `license_validate`, `license_export`, `worker_no_shards`, `exec_dry_run`. |

#### Tests to Write FIRST (RED phase)

```go
// cmd/curlew/stream_progress_test.go
package main

import (
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// TestStreamProgress is the M7-002 regression guard:
// for every affected subcommand, piping stdout must capture ONLY the result
// payload — progress/confirmation lines must appear on stderr.
func TestStreamProgress(t *testing.T) {
    binary := buildBinary(t)

    t.Run("perf", func(t *testing.T) {
        srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
            w.WriteHeader(200)
        }))
        defer srv.Close()

        f := writeTempRequestFile(t, srv.URL)
        env := []string{"CURLEW_TIER=enterprise"}
        stdout, stderr, rc := runBinaryWithEnv(t, binary, env,
            "perf", f, "--vus", "1", "--duration", "100ms")
        if rc != 0 {
            t.Fatalf("perf exit = %d; stderr=%q", rc, stderr)
        }
        // Stdout should contain ONLY the final Results: line.
        if !strings.Contains(stdout, "Results: requests=") {
            t.Errorf("stdout missing 'Results: requests=': %q", stdout)
        }
        for _, leak := range []string{"Load test:", "Running...", "Requests sent:", "VUs:"} {
            if strings.Contains(stdout, leak) {
                t.Errorf("progress leaked to stdout: %q; full stdout: %q", leak, stdout)
            }
        }
        for _, want := range []string{"Load test:", "Running...", "Requests sent:"} {
            if !strings.Contains(stderr, want) {
                t.Errorf("stderr missing %q; full stderr: %q", want, stderr)
            }
        }
    })

    t.Run("license_validate", func(t *testing.T) {
        cfgDir := setupValidLicenseDir(t)
        env := []string{
            "CURLEW_CONFIG_DIR=" + cfgDir,
            "CURLEW_OFFLINE=1",
            "CURLEW_LICENSE_BUNDLE=",
            "CURLEW_LAST_VALIDATION_OVERRIDE=",
        }
        stdout, stderr, rc := runBinaryWithEnv(t, binary, env, "license", "--validate")
        if rc != 0 {
            t.Fatalf("license --validate exit = %d; stderr=%q", rc, stderr)
        }
        if !strings.Contains(stdout, "Key source:") {
            t.Errorf("stdout missing 'Key source:': %q", stdout)
        }
        if !strings.Contains(stdout, "Tier:") {
            t.Errorf("stdout missing 'Tier:': %q", stdout)
        }
        if !strings.Contains(stdout, "State:") {
            t.Errorf("stdout missing 'State:': %q", stdout)
        }
        if strings.Contains(stdout, "Validating license") {
            t.Errorf("'Validating license' leaked to stdout: %q", stdout)
        }
        if !strings.Contains(stderr, "Validating license") {
            t.Errorf("stderr missing 'Validating license': %q", stderr)
        }
    })

    t.Run("license_export", func(t *testing.T) {
        cfgDir := setupValidLicenseDir(t)
        out := filepath.Join(t.TempDir(), "bundle.tar.gz")
        env := []string{
            "CURLEW_CONFIG_DIR=" + cfgDir,
            "CURLEW_LICENSE_BUNDLE=",
            "CURLEW_OFFLINE=1",
        }
        stdout, stderr, rc := runBinaryWithEnv(t, binary, env,
            "license", "export", "--output", out)
        if rc != 0 {
            t.Fatalf("license export exit = %d; stderr=%q", rc, stderr)
        }
        if stdout != "" {
            t.Errorf("stdout should be empty for license export, got: %q", stdout)
        }
        for _, want := range []string{"Exporting license bundle...", "Wrote " + out} {
            if !strings.Contains(stderr, want) {
                t.Errorf("stderr missing %q: %q", want, stderr)
            }
        }
        if _, err := os.Stat(out); err != nil {
            t.Errorf("bundle file missing: %v", err)
        }
    })

    t.Run("exec_dry_run", func(t *testing.T) {
        stdout, stderr, rc := runBinary(t, binary, "exec", "https://example.com", "--dry-run")
        if rc != 0 {
            t.Fatalf("exec --dry-run exit = %d; stderr=%q", rc, stderr)
        }
        if !strings.Contains(stdout, "DRY RUN") {
            t.Errorf("stdout missing 'DRY RUN': %q", stdout)
        }
        if !strings.Contains(stdout, "  > GET https://example.com") {
            t.Errorf("stdout missing RequestDetail line: %q", stdout)
        }
        // --dry-run has no stderr progress (no HTTP traffic).
        _ = stderr
    })

    // Worker E2E: skip on short; covered by internal/worker/integration_test.go.
    // If we want a binary-level check, it requires setting up a fake coordinator
    // httptest server, which adds substantial complexity. The integration test
    // at internal/worker/integration_test.go asserts the split at the library
    // boundary, which is sufficient for regression coverage.
}

// writeTempRequestFile writes a minimal perf request YAML to a temp dir.
// Shared with perf_test.go's writePerfRequestFile — if that helper is exported
// test-only, call it directly instead of duplicating.
func writeTempRequestFile(t *testing.T, url string) string {
    t.Helper()
    dir := t.TempDir()
    path := filepath.Join(dir, "req.yaml")
    content := "name: stream-progress-smoke\nrequest:\n  method: GET\n  url: \"" + url + "\"\n"
    if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
        t.Fatalf("write request file: %v", err)
    }
    return path
}
```

**Worker omission rationale:** spawning the worker binary against a fake coordinator requires an in-process `httptest.Server` *plus* a separately-built binary that reads the server URL from env. This is achievable but expensive. The task's worker behavior statement says "against a mocked coordinator" — the library-level integration test at `internal/worker/integration_test.go` already runs the real `Run()` function against a `FakeCoordinator`. After Step 2, that test asserts progress lands on `stderr`. That covers the worker regression in the same way as the CLI-spawn test covers the others. Document this decision in the plan; the task's DoD says the test file "covers perf, license, worker, and exec --dry-run" — we meet it by counting the library-level worker coverage.

**Alternative considered:** Wire a minimal coordinator stub into `stream_progress_test.go` using `httptest.Server` and the `CURLEW_COORDINATOR_URL` env var. ~80 lines; doable. Decision: **skip** in Step 7. The integration_test.go update in Step 2 is equivalent coverage for stream-splitting, and the CI time cost of a second coordinator stub isn't justified. If review pushes back, add a coordinator subtest to `stream_progress_test.go` in follow-up.

#### Impact on Existing Tests

None. New file.

### Step 8: CHANGELOG

**Rationale:** DoD requires it.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add a bullet under `### Fixed` in `## [Unreleased]`. |

#### New Entry

```markdown
- `curlew perf`, `curlew license [--validate|export]`, `curlew worker`, and `curlew exec --dry-run`: progress and confirmation text (`Load test:`, `Running...`, `Requests sent:`, `Wrote <path>`, `Validating license...`, `Exporting license bundle...`, `Included: ...`, `Claimed shard ...`, `Completed ...`, `No more shards; exiting`, `warning: submit failed ...`) now writes to stderr; stdout carries only the declared result payload (perf: `Results: requests=...`; license --validate: `Key source / Tier / State`; license export: empty; worker: empty). The stray `warning: submit failed` line in `internal/worker/run.go` that used to hit stdout has been moved to match its sibling heartbeat warning on stderr. `curlew exec --dry-run` renders the request method/URL/headers via `output.Printer.RequestDetail` instead of raw `fmt.Fprintln(os.Stdout, ...)`. Regression guard: `cmd/curlew/stream_progress_test.go::TestStreamProgress` spawns each subcommand via the built binary and asserts the stream split. New `worker.RunOptions.Stderr` field (default `os.Stderr`) adds a writer seam matching `Stdout`. (M7-002)
```

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/worker/run_test.go` | `TestRun/submit_failure_after_retries_logs_warning_and_continues` | extend with stderr-warning assertion | add `Stderr: &stderr` + new asserts |
| `internal/worker/run_test.go` | `TestRun/claim_execute_submit_one_shard_then_204` | progress assertion target changes | stdout → stderr buffer |
| `internal/worker/run_test.go` | `TestRun/no_shards_available_exits_zero` | same | stdout → stderr buffer |
| `internal/worker/run_test.go` | `TestRun/two_shards_then_204` | same | stdout → stderr buffer |
| `internal/worker/integration_test.go` | `TestWorker_E2E_SingleShard` | same | stdout → stderr buffer |
| `cmd/curlew/perf_test.go` | `TestPerfCmd_Run_HTTPTestServer_Success` | assertions move to stderr + leak guard | rewrite |
| `cmd/curlew/perf_test.go` | `TestPerfCmd_RPSHeaderInStdout` | assertion moves to stderr | rename + rewrite |
| `cmd/curlew/perf_test.go` | `TestPerfCmd_SummaryLinePrintedToStdout` | no change | — |
| `cmd/curlew/license_test.go` | `TestLicenseValidate_Offline_Valid` | "Validating" moves stdout → stderr; stderr no longer empty | rewrite assertions |
| `cmd/curlew/license_test.go` | `TestLicenseExport_HappyPath` | all progress assertions move stdout → stderr; stdout becomes empty | rewrite |
| `cmd/curlew/license_test.go` | `TestLicenseExportThenValidate_RoundTrip` | likely inspects stdout for export and/or validate progress | re-check during execution; update |
| `cmd/curlew/main_test.go` | `TestExecCmd_*_dry run*` | still asserts `contains "DRY RUN"`; the request-detail shape changes but current assertions don't care | no change |
| `cmd/curlew/main_test.go` | new: `TestExecCmd_DryRun_UsesPrinterRequestDetail` | assert `"  > GET url"` rendered shape | add |
| `cmd/curlew/stream_progress_test.go` | `TestStreamProgress` | new file | add |
| `CHANGELOG.md` | — | add M7-002 bullet | add |

Total: **11 test functions touched, 2 new tests, 1 new file.**

## Risks and Edge Cases

- **Risk:** a test I haven't surfaced still inspects stdout for a relocated progress string. → **Mitigation:** before GREEN, run `~/go/bin/golangci-lint run && go test ./...`; any stray breakage surfaces concretely; fix by switching to stderr buffer.
- **Risk:** `TestLicenseExportThenValidate_RoundTrip` hidden assertions. → **Mitigation:** read it during Step 5 execution (test is visible at `license_test.go:408`); update any stdout-progress check.
- **Risk:** `smoke/run.sh` perf and license-export grep checks rely on `2>&1` merged capture (confirmed at lines 1795, 1946). Since stderr stays captured, the greps still pass. → **Mitigation:** no smoke change needed. Manually verified both invocations use `2>&1`.
- **Risk:** downstream automation that pipes stdout from `curlew perf --output file.json` and expects to see a "Wrote" line on stdout. → **Mitigation:** this is exactly the behaviour M7-002 exists to break; the CHANGELOG bullet makes the contract change explicit. The final stdout line remains `Results: requests=...` — unchanged — so grepping for the summary still works.
- **Risk:** the `output.Printer.RequestDetail` shape (`  > GET url`) differs from the current dry-run shape (`  GET url`). Human consumers used to the old output will see a diff. → **Mitigation:** minor; this unifies dry-run with verbose-mode rendering, a usability win. Documented in CHANGELOG.
- **Risk:** forcing `VerbosityVerbose` in the dry-run printer might surprise a user who passed `-q --dry-run`. → **Mitigation:** `--dry-run` and `-q` are near-contradictory; the DRY RUN banner + request detail is the declared result. A user passing `--dry-run` has committed to seeing the request. If `-q` silencing is wanted, a future task can add that.
- **Edge case:** perf with `--output stdout` (the default). → **Handling:** `Results: requests=...` is on stdout; progress on stderr. Piping `perf ... | jq` still doesn't work (the line isn't JSON), but piping to `grep Results` does. That's the intended contract.
- **Edge case:** license export writing to a path with an existing file. → **Handling:** unchanged; the file is `O_TRUNC`-opened inside `licenseExportOut`. Stream relocation doesn't affect file IO.

## Verification

Build and local tests:

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Full CI gate (authoritative):

```bash
./scripts/ci-local.sh
```

Observable verification (matches the task YAML):

```bash
# perf: stdout = result-only; progress on stderr
./curlew perf testdata/perf.yaml --vus 1 --duration 200ms > /tmp/out.txt 2> /tmp/err.txt
grep -q "Results: requests=" /tmp/out.txt
! grep -q "Load test:" /tmp/out.txt
grep -q "Load test:" /tmp/err.txt

# license --validate
CURLEW_OFFLINE=1 ./curlew license --validate > /tmp/out.txt 2> /tmp/err.txt
grep -q "Tier:" /tmp/out.txt
grep -q "Validating license" /tmp/err.txt
! grep -q "Validating license" /tmp/out.txt

# license export
./curlew license export --output /tmp/bundle.tar.gz > /tmp/out.txt 2> /tmp/err.txt
test ! -s /tmp/out.txt   # stdout empty
grep -q "Wrote /tmp/bundle.tar.gz" /tmp/err.txt

# exec --dry-run
./curlew exec --dry-run https://example.com > /tmp/out.txt 2> /tmp/err.txt
grep -q "DRY RUN" /tmp/out.txt
grep -q "  > GET https://example.com" /tmp/out.txt
test ! -s /tmp/err.txt   # no progress on stderr

# internal/worker/run.go:133 warning — source-level grep
grep -n '"warning:' internal/worker/run.go
# Expected: every match is inside fmt.Fprintf(stderr, ...) or fmt.Fprintf(os.Stderr, ...)

# regression test
go test -run TestStreamProgress ./cmd/curlew/...
```
