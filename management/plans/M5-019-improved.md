# Improvement Report: M5-019

**Task:** go-cli: example plugin + developer docs
**Date:** 2026-04-21
**Review:** management/reviews/M5-019-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `_ = json.Unmarshal(params, &p)` in `on_response` silently discards parse errors, causing zero-value metric submission with no log warning | Replaced with explicit error check: logs `[plugin:datadog-metrics] on_response: bad params: <err>` and returns early without submitting. Added `TestOnResponse_MalformedParams_LogsWarning` (TDD: RED then GREEN). | tests pass |
| 2 | Low | `out, _ := json.Marshal(resp)` silently drops marshal error; a nil `out` would write a bare newline to stdout causing protocol corruption | Replaced with `out, mErr := json.Marshal(resp)`; on error, logs to stderr and `continue`s the loop rather than writing a malformed response. | tests pass |
| 3 | Low | `err == io.EOF` uses direct equality instead of `errors.Is(err, io.EOF)`, inconsistent with `internal/plugin/channel.go` convention | Replaced with `errors.Is(err, io.EOF)`; added `"errors"` to import block. | tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` (main module) | PASS |
| `golangci-lint run` | PASS |
| `go test ./...` (example module) | PASS |
| Coverage (example module) | 80.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| a38ced8 | fix(plugin): resolve review findings #1, #2, #3 in datadog-metrics example | #1, #2, #3 |

## Summary

3/3 findings resolved. 0 deferred.

All three findings were in `examples/plugins/datadog-metrics/main.go` (in-scope file).
Finding #1 (Medium) required TDD: a failing test was written first, confirming the bug, then the fix was applied and the test made green. Two additional tests were added (`TestOnResponse_MalformedParams_LogsWarning`, `TestRun_ReadError_Propagated`) to cover the new error branches introduced, keeping total coverage at 80.2% (≥80% requirement met).
