# Improvement Report: M5-013 (Iteration 2)

**Task:** go-cli: offline JWT verification and grace-period state machine
**Date:** 2026-04-19
**Review:** management/reviews/M5-013-review.md

## Iteration 1 — Previously Resolved (confirmed by iteration 2 review)

| # | Severity | Finding | Status |
|---|----------|---------|--------|
| 1 | Critical | Smoke test hardcoded `last_validated_at` expires after 24h | FIXED |
| 2 | High | `jwt.go` used `%v` in 5 `fmt.Errorf` calls | FIXED |
| 3 | High | `jwks.go` used `%v` for inner `json.Unmarshal` error | FIXED |
| 4 | High | `resolver.go` used `%v` for inner fetcher error | FIXED |
| 5 | Medium | `Token.signed` and `Token.signature` were exported fields | FIXED |

## Iteration 2 — Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Behavior 6 (GRACE_EXPIRED → exit 9 "feature_gated" on premium commands) was explicitly planned but never implemented. `runCmd` and `execCmd` had no grace-expired gate. | Added `checkGraceExpired()` helper in `cmd/curlew/license.go` that calls `NewValidator`, evaluates state, and returns exit code 9 with `"feature_gated"` stderr message when `StateGraceExpired`. Wired it at the top of `runCmd` and `execCmd` in `cmd/curlew/main.go`. Added `TestRunCmd_GraceExpired_ExitsNine` and `TestExecCmd_GraceExpired_ExitsNine` tests. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage `cmd/curlew` | 83.3% |
| Coverage `internal/license` | 87.8% |
| Coverage `internal/license/jwks` | 92.0% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| cf9d4e5 | fix(license): gate premium commands with exit 9 on GRACE_EXPIRED | #1 (iter 2) |

## Summary

1/1 iteration-2 findings resolved. 0 deferred. All quality gates pass. All 8 spec behaviors now covered.
