# Improvement Report: M2-011 (Round 3)

**Task:** Watch mode with file system monitoring
**Date:** 2026-04-01
**Review:** management/reviews/M2-011-review.md

## Prior Rounds

- Round 1: Resolved 6 findings (naming stutter, missing behavior tests, silent error discard, reimplemented `strings.Contains`).
- Round 2: Resolved 2 findings (fsnotify indirect dependency, stale directory leak in watcher).

## Resolved Findings (Round 3)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `Config.RunFunc` missing inline comment | Added comment: `// function called on each run; receives Args, returns exit code` | ✓ lint pass |
| 2 | Low | `TestCollectPaths` missing test case for env file specified but not found | Added test case `"env file specified but not found - error"` with `wantErr: true` | ✓ tests pass |
| 3 | Low | `preserves_args_across_reruns` only checks arg length, not values | Added element-by-element comparison with `continue` on length mismatch | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (watch package) | 86.0% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 545d207 | fix(watch): resolve round 3 review findings | #1, #2, #3 |

## Summary

3/3 findings resolved. 0 deferred.
