# Improvement Report: M7-001

**Task:** Stderr printers derive color flag from stderr's TTY state, not stdout's
**Date:** 2026-04-22
**Review:** management/reviews/M7-001-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Behavior 2 ("stderr=TTY + stdout=pipe → color preserved") had no test case. The `stderrTTY` struct field was declared but never set to `true` in any test row. | Added two new test cases: `{stderrTTY: true, stdoutTTY: false, wantStderrColor: true}` and `{stderrTTY: true, stdoutTTY: true, wantStderrColor: true}` to `TestStderrColorFlag`. Both use the existing `/dev/tty` skip path when unavailable. | ✓ tests pass |
| 2 | Low | Doc comment claimed "four TTY-vs-pipe combinations" but listed only three pipe-based variants plus one TTY×pipe case, creating a false impression of completeness. | Rewrote the file header comment to enumerate all six combinations by name, distinguishing portable pipe-based cases from `/dev/tty`-dependent ones. | ✓ tests pass |
| 3 | Low | Task `status` field read `backlog` — should have been updated during execute phase. | Updated `management/tasks/M7-001.yaml` status from `backlog` to `review`. | ✓ file updated |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`cmd/curlew`) | 80.3% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 39c7bda | fix(test): add stderrTTY=true test cases and clarify doc comment | #1, #2, #3 |

## Summary

3/3 findings resolved. 0 deferred.
