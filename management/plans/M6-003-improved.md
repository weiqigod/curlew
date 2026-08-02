# Improvement Report: M6-003

**Task:** Redact sensitive values from request and response bodies
**Date:** 2026-04-21
**Review:** management/reviews/M6-003-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `TestRedact_BodyAcrossFormats` missing `junit_format` subtest | Added `junit_format` subtest in `cmd/curlew/main_test.go`; elevated tier to `TierProfessional` to bypass the feature gate so redaction is proven for JUnit XML output | ✓ tests pass |
| 2 | Low | Unreachable `nil` guards in `AddValue` (line 69-71) and `Merge` (lines 116-118) go untested because `NewSensitiveSet()` always initialises maps | Added `TestSensitiveSet_ZeroValue` with `AddValue_on_zero_value` and `Merge_on_zero_value` subtests exercising a raw `var s SensitiveSet`; also added a `s.names == nil` guard to `Merge` to prevent panic on zero-value structs | ✓ tests pass |
| 3 | Low | `Names()` nil receiver path untested | Extended `TestSensitiveSet_NilSafe` to assert `s.Names() == nil` for a nil receiver | ✓ tests pass |
| 4 | Low | Dead `enc.Encode` error branch in `redactJSONString` — unreachable for `json.Unmarshal`-produced values, reducing `redactJSONString` function coverage | Added an explanatory comment documenting why the branch cannot fire with standard JSON-unmarshalable values, and why it is retained as a defensive guard | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (`internal/variable`) | 96.7% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 862a7bc | fix(variable): resolve all M6-003 review findings | #1, #2, #3, #4 |

## Summary

4/4 findings resolved. 0 deferred.
