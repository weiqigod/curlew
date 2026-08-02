# Improvement Report: M1-001

**Task:** Run a single GET request from a collection file
**Date:** 2026-03-10
**Review:** management/reviews/M1-001-review.md

## Resolved Findings

### Round 1 (review #1 → improve #1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | YAML error wrapped with `%v` instead of `%w` in parser | Changed to `%w` for Go 1.20+ multi-error wrapping | ✓ tests pass |
| 2 | Medium | Inner errors wrapped with `%v` instead of `%w` in executor | Changed both `fmt.Errorf` calls to use `%w` | ✓ tests pass |
| 3 | Medium | Package `internal/http` shadows stdlib `net/http` | Renamed directory and package to `internal/httpexec` | ✓ tests pass |
| 4 | Low | Redundant `errors.Is` check — dead code in run loop | Removed conditional, kept single `continue` | ✓ tests pass |
| 5 | Low | Response body not drained before close | Added `io.Copy(io.Discard, resp.Body)` before close | ✓ tests pass |

### Round 2 (review #2 → improve #2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `go.mod` `// indirect` on direct dep + missing `go.sum` entry | Ran `go mod tidy` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 94.8% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 5e55941 | refactor(httpexec): rename internal/http to internal/httpexec | R1 #3 |
| fac43f6 | fix(parser): use %w for inner YAML error to preserve error chain | R1 #1 |
| 48e2da6 | fix(httpexec): use %w for inner errors to preserve error chain | R1 #2 |
| 59f9a65 | fix(httpexec): drain response body before close for connection reuse | R1 #5 |
| ee1f1d6 | refactor(cli): remove redundant errors.Is branch in run loop | R1 #4 |
| cbf6d7e | chore(deps): fix go.mod indirect marker and go.sum | R2 #1 |

## Summary
6/6 findings resolved across 2 review rounds. 0 deferred.
