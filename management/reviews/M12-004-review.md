# Code Review: M12-004

**Task:** $sha256 and $md5 dynamic functions
**Reviewer:** AI
**Date:** 2026-04-28
**Branch:** feature/M12-004-sha256-md5-dynamic-functions

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new error paths introduced; sha256/md5 are infallible over any []byte input. Arity errors go through the pre-existing `arityError` → `Evaluate` wrapping chain unchanged. |
| Input Validation | PASS | Arity-zero and arity-two cases tested for both functions. Empty string and UTF-8 multibyte inputs covered with golden vector assertions. `nil` cache handled by pre-existing `Evaluate` guard. |
| Naming | PASS | Registry keys `"sha256"` and `"md5"` are lowercase, matching existing convention. No new exported symbols. Existing exported symbols (`DynFunc`, `Registry`, `NewRegistry`, `Evaluate`, `Available`) retain their doc comments. |
| Code Organization | PASS | Two registrations appended inside `register()`, reusing `oneArg` from M12-002 unchanged. No new packages, no package-boundary violations, no circular dependencies. |
| Correctness | PASS | `fmt.Sprintf("%x", sha256.Sum256([]byte(s)))` and `fmt.Sprintf("%x", md5.Sum([]byte(s)))` produce lowercase hex over the UTF-8 byte sequence, exactly as specified. Hash functions have no runtime failure path for any input bytes. Golden vectors verified against `sha256sum(1)` / `md5(1)`. Registry count assertion bumped 19 → 21 correctly. |
| Test Quality | PASS | Five new test functions. Table-driven tests with golden vectors for both functions. UTF-8 multibyte and empty-string edge cases covered. Arity errors for zero and two args tested for both functions. Per-request cache behaviour verified with a call-counter wrapper (mirroring the M12-002 pattern). Cross-request determinism asserted. `TestRegistry_Sha256_NestedVar` exercises the full `Interpolate` path with a nested variable argument. All six task behaviours have explicit test coverage. |

## Test Coverage

- Coverage: 96.2% (well above the required 80%)
- `dynamic.go` functions introduced in this task: 100% (`oneArg` wrapper, `Evaluate`, and both new registrations fully exercised)
- The 98.3% on `register()` is pre-existing (one unreachable branch in the timestamp/seeded path), not introduced by this task

## Spec Compliance

| Behavior | Test(s) | Status |
|----------|---------|--------|
| 1. `$sha256(s)` returns lowercase 64-char hex | `TestRegistry_Sha256` (hello, abc, format checks) | PASS |
| 2. `$md5(s)` returns lowercase 32-char hex | `TestRegistry_Md5` (hello, abc, format checks) | PASS |
| 3. `$sha256('')` returns SHA-256 of empty | `TestRegistry_Sha256` ("empty string" case) | PASS |
| 4. UTF-8 multibyte `héllo` hash over UTF-8 bytes | `TestRegistry_Sha256` ("utf-8 multibyte" case, golden vector pinned) | PASS |
| 5. Arity errors for zero or two args (both functions) | `TestRegistry_HashFns_arity_errors` | PASS |
| 6. Cross-request determinism (pure function) | `TestRegistry_Sha256_caches_per_request` | PASS |

## Summary

The implementation is minimal, correct, and well-tested. Both helpers are thin one-line wrappers over `crypto/sha256.Sum256` and `crypto/md5.Sum`, formatted with `%x` — exactly the approach specified in the task YAML. The code reuses `oneArg` from M12-002 without modification, introduces no new sentinel error codes (none warranted for infallible hash operations), and follows the established patterns from M12-002 and M12-003 in every respect. The MANUAL.md update is complete: two new table rows, the MD5 deprecation caveat, and the parenthetical updated to reference M12-005 onward.
