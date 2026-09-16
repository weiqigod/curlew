# Code Review: M30-004

Date: 2026-09-16. Branch: feature/M30-004-windows-commands.

## Verdict: FAIL

The required pre-audit Go gate was attempted and failed. The established review
workflow stops here; no official static-audit PASS or task-completion claim follows.

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Critical | Pre-audit Gate | [perf_test.go](../../cmd/curlew/perf_test.go#L256) | 256 | scripts/ci-local.sh --go fails at README quickstart test discovery because syscall.Kill is unavailable on Windows. This existed before M30-004. | Repair the native gate prerequisites, then rerun the gate before official review. Do not hide the failure with test filtering. |

## Gate Output

The preceding build, backlog, fuzz-corpus, layout, and front-door steps ran.
Final output:

```text
=== front-door files (M28-002) ===
=== RUN   TestRepo_front_door_files_are_present
--- PASS: TestRepo_front_door_files_are_present (0.00s)
PASS
ok github.com/weiqigod/curlew/internal/docs 0.934s

=== README quickstart, executed (M28-002) ===
# github.com/weiqigod/curlew/cmd/curlew [github.com/weiqigod/curlew/cmd/curlew.test]
cmd\curlew\perf_test.go:256:15: undefined: syscall.Kill
expected 1 quickstart test in ./cmd/curlew, found 0.
```

The standalone baseline and final CLI-package compile checks both reproduced the
same failure. Linter, C compiler, and release tool are also absent from native PATH.

## Supplementary Evidence

A separately labelled read-only source audit was used during implementation, not
as a substitute for this gate. It found a POSIX output-drain ordering issue and a
platform-newline fixture issue. Both were repaired and exercised on Linux; native
Windows affected suites also pass. A real CLI interrupt regression was added and
fixed with RED/GREEN evidence. Known batch/fault-injection limits remain explicit.

See the [verification report](../plans/M30-004-verified.md) for scoped results and
coverage. M30-004 remains `in_progress`; no passing workflow review, /verify, PR,
or merge was performed.