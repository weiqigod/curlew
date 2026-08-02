# Verification Report: M7-003

**Task:** Help-after-error: emit one-line usage synopsis on stderr alongside the error
**Verified by:** AI
**Date:** 2026-04-22
**Branch:** feature/M7-003-usage-synopsis-on-stderr
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass, 0 failures |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage | 86.6% | Meets >= 80% threshold |

## Observable Output

```
# unknown-command test
$ ./apitest unknown-command > /tmp/out.txt 2> /tmp/err.txt || true
$ wc -c /tmp/out.txt
       0 /tmp/out.txt
$ grep -c "Usage:" /tmp/err.txt
1
$ grep -c "Unknown command" /tmp/err.txt
1

# --help test
$ ./apitest --help > /tmp/out.txt 2> /tmp/err.txt
$ wc -c /tmp/err.txt
       0 /tmp/err.txt
$ grep -c "Commands:" /tmp/out.txt
1
```

Expected: stdout empty on error, Usage+error on stderr; full help on stdout for --help, empty stderr
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | apitest <unknown-command>: stderr has error AND synopsis; stdout empty | `TestStreamHelp/top_level_unknown_command` | PASS |
| 2 | apitest plugins <unknown-subcommand>: stderr has error AND synopsis; stdout empty | `TestStreamHelp/plugins_unknown_subcommand` | PASS |
| 3 | apitest perf with malformed flag: stderr has parse error AND synopsis; stdout empty | `TestStreamHelp/perf_malformed_flag` | PASS |
| 4 | apitest worker with malformed flag: stderr has parse error AND synopsis; stdout empty | `TestStreamHelp/worker_malformed_flag` | PASS |
| 5 | apitest import no args: stderr has synopsis; stdout empty | `TestStreamHelp/import_no_args` | PASS |
| 6 | apitest --help (explicit): stdout has full help, stderr empty, exit 0 | `TestStreamHelp/top_level_explicit_help` | PASS |
| 7 | apitest plugins --help and apitest perf --help: stdout has subcommand help, stderr empty, exit 0 | `TestStreamHelp/plugins_explicit_help`, `TestStreamHelp/perf_explicit_help` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `TestStreamHelp` 11 subtests PASS | PASS |
| 2 | cmd/apitest/stream_help_test.go exercises both explicit --help and error-path invocations | File exists, 11 subtests covering main/plugins/perf/worker/import | PASS |
| 3 | Each of the four print*Help functions takes an io.Writer parameter | `printHelpTo(w io.Writer)`, `printPluginsHelpTo(w io.Writer)`, `printPerfHelpTo(w io.Writer)`, `printWorkerHelpTo(w io.Writer)` | PASS |
| 4 | usageSynopsis helper exists in main.go and is reused by every error-recovery site | `usageSynopsis(cmd string)` at main.go:3142, used at all 5 error sites | PASS |
| 5 | Explicit --help writes to stdout; error-recovery writes synopsis to stderr | Verified by TestStreamHelp subtests and observable | PASS |
| 6 | go test ./... passes with no regressions | All packages pass | PASS |
| 7 | Test coverage does not regress below 80% | 86.6% total | PASS |
| 8 | golangci-lint run passes with 0 issues | 0 issues | PASS |
| 9 | ./smoke/run.sh passes | Smoke test clean | PASS |
| 10 | ./scripts/ci-local.sh passes | ci-local PASS | PASS |
| 11 | CHANGELOG.md updated | Entry added under [Unreleased] | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS — no stuttering, Effective Go style |
| Doc comments on exports | PASS — `usageSynopsis` has full doc comment |
| io.Writer parameterization | PASS — all four print*Help functions take io.Writer |
| Code organization | PASS — `usageSynopses` map contains only entries wired to active call sites |
| Test quality | PASS — table-driven, integration via os/exec, 11 subtests covering all cells |
| Test sync | PASS — `TestUsageSynopsis_MatchesPrintHelpFirstLine` asserts sync with full help |

Branch A: Review PASS trusted (management/reviews/M7-003-review.md), spot-check clean.

## Commits

| Hash | Message |
|------|---------|
| b9c9804 | docs(review): add passing review for M7-003 |
| 7036994 | docs(review): add improvement report for M7-003 |
| 6409089 | fix(cli): remove unused "run" entry from usageSynopses map |
| bb1d5d5 | docs(review): add review with findings for M7-003 |
| 924ad89 | chore(task): mark M7-003 as review |
| 5fbc4c9 | docs(plan): update CHANGELOG for M7-003 |
| 6bcb68b | feat(cli): rewire error-recovery sites to emit synopsis on stderr |
| 095c513 | test(cli): add failing TestStreamHelp regression tests for M7-003 |
| d48c26d | feat(cli): add usageSynopsis helper with synopses map |
| a1861c1 | test(cli): add failing tests for usageSynopsis helper |
| b62f71e | chore(task): mark M7-003 as in_progress |
| 87f7fa9 | chore(task): mark M7-003 as planned |
| 33b1180 | docs(plan): add implementation plan for M7-003 |

## Files Changed

| File | Action |
|------|--------|
| `cmd/apitest/main.go` | modified — usageSynopsis helper, printHelpTo(w io.Writer), error-recovery sites rewired |
| `cmd/apitest/perf.go` | modified — printPerfHelpTo(w io.Writer), perf error site rewired |
| `cmd/apitest/plugins.go` | modified — printPluginsHelpTo(w io.Writer), plugins error site rewired |
| `cmd/apitest/worker.go` | modified — printWorkerHelpTo(w io.Writer), worker error site rewired |
| `cmd/apitest/stream_help_test.go` | added — TestStreamHelp (11 subtests), TestUsageSynopsis_* |
| `cmd/apitest/main_test.go` | modified — TestUsageSynopsis tests moved/added |
| `CHANGELOG.md` | modified — unreleased entry for M7-003 |

## Issues Found
None.

## Merge

| Field | Value |
|-------|-------|
| PR | https://github.com/weiqigod/apitest/pull/115 |
| Merge commit | a27c344 |
| Strategy | squash |
| Merged to | main |
| Date | 2026-04-22 |

## Recommendation
PASS — merged to main.
