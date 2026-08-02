# Code Review: M12-005

**Task:** $hmacSha256 dynamic function with sensitive-key propagation
**Reviewer:** AI
**Date:** 2026-04-28
**Branch:** feature/M12-005-hmacsha256

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; `arityError` returns `apierrors.Structured` with `DYNFN_ARITY` code; no swallowed errors; `SensitiveArgIndex` nil-receiver guard present |
| Input Validation | PASS | Arity validated via `twoArgs`; zero, one, and three arg cases produce structured `DYNFN_ARITY` errors; empty strings are valid HMAC inputs and handled correctly |
| Naming | PASS | No stuttering; all exported symbols (`SensitiveArgIndex`, `WithRuntimeSensitive`, `RuntimeSensitive`, `twoArgs`) have doc comments; package names and field names follow Effective Go conventions |
| Code Organization | PASS | `dynamic.go` owns HMAC registration; `variable.go` owns sensitivity wiring; `sensitive.go` owns `SensitiveSet` with mutex; runner change is minimal (two lines + one field); no circular dependencies |
| Correctness | PASS | HMAC computation (`h.Write(payload)` then `h.Sum(nil)`) is cryptographically correct; verified by RFC-style golden vectors; `SensitiveSet` protected by `sync.RWMutex` on all exported methods; `go test -race ./...` passes including parallel integration test |
| Test Quality | PASS | All 7 task behaviors have dedicated tests; golden vectors pinned; arity errors, caching, literal-key-not-marked, payload-sensitive-not-key, secrets-namespace, and two-key parallel all covered; `TestRun_HmacSha256_Parallel_BothKeysRegistered` catches concurrent races under `-race` |

## Test Coverage
- Coverage: `internal/variable` 96.4%, `internal/runner` 85.2%, total 87.1% — all above the 80% threshold
- All 7 `behaviors` from the task YAML have corresponding named test functions

## Behavior Coverage

| # | Behavior | Test |
|---|----------|------|
| 1 | Two strings → 64-char lowercase hex HMAC-SHA-256 | `TestRegistry_HmacSha256` |
| 2 | RFC-style fox test vector = `f7bc83f4…` | `TestRegistry_HmacSha256/rfc-style_fox_vector` |
| 3 | Zero/one/three args → arity-mismatch error naming arity 2 | `TestRegistry_HmacSha256_arity_errors` |
| 4 | Key from sensitive variable or `{{secrets.X}}` → `AddValue` on `runtimeSensitive` | `TestRegistry_HmacSha256_KeyIsSensitive_HeuristicName`, `TestRegistry_HmacSha256_KeyIsSensitive_SecretsNamespace` |
| 5 | Literal key → no `SensitiveSet` mutation | `TestRegistry_HmacSha256_LiteralKey_NotMarked` |
| 6 | Payload from sensitive variable → no mutation (only key triggers) | `TestRegistry_HmacSha256_PayloadSensitive_NotKey` |
| 7 | Two calls with different keys → both correct, both keys captured | `TestRegistry_HmacSha256_TwoCallsDifferentKeys`, `TestRun_HmacSha256_Parallel_BothKeysRegistered` |

## Summary

The `$hmacSha256` implementation is cryptographically correct and complete. The sensitive-key propagation mechanism is cleanly architected: the `sensitiveArgIdx` registry table decouples credential policy from the cryptographic function; per-call sensitivity flags are captured before `{{secrets.X}}` substitution to preserve origin information; `SensitiveSet` is correctly mutex-protected for concurrent parallel execution; and the runner exposes `RuntimeSensitive` on `Summary` so `cmd/apitest` merges it into the post-run redaction set. All previous review findings (SensitiveSet data race, missing parallel test, incorrect MANUAL.md pseudocode) have been resolved in prior iterations. No new findings.
