# Improvement Report: M12-005

**Task:** $hmacSha256 dynamic function with sensitive-key propagation
**Date:** 2026-04-28
**Review:** management/reviews/M12-005-review.md

## Resolved Findings (iteration 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `SensitiveSet.AddValue` (and other mutating methods) wrote to a plain `map[string]struct{}` without mutex protection. In parallel mode, multiple goroutines sharing the same `runtimeSensitive` pointer race on `AddValue`, producing a data race. Doc comment on `WithRuntimeSensitive` incorrectly claimed AddValue was safe for concurrent calls. | Added `sync.RWMutex` to `SensitiveSet`. All exported methods (`AddValue`, `Values`, `Add`, `IsSensitive`, `Merge`, `Names`, `AddHeuristicNames`) now acquire appropriate read or write lock. Doc comment on `WithRuntimeSensitive` now accurately reflects thread safety. | ✓ tests pass with `-race` |
| 2 | Medium | No parallel-mode concurrency test for the `runtimeSensitive` mutation path. The single integration test only exercised the sequential single-request path, leaving the race in finding #1 undetectable by `-race`. | Added `TestRun_HmacSha256_Parallel_BothKeysRegistered` in `internal/runner/runner_test.go`. Two requests run concurrently in `--parallel` mode (TierProfessional), both using `$hmacSha256` with different sensitive keys. Verifies both MACs are correct and both keys appear in `RuntimeSensitive`. This test catches the race with `go test -race`. | ✓ tests pass with `-race` |

## Resolved Findings (iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `docs/MANUAL.md` line 1059: pseudocode `crypto/hmac.New(sha256.New, []byte(key)).Sum([]byte(payload))` is factually incorrect — `Sum(b)` appends the digest to `b`, it does not hash `b`. A developer copying this into another language would produce a wrong result. | Replaced with accurate pseudocode: `h := hmac.New(sha256.New, []byte(key)); h.Write([]byte(payload)); hex(h.Sum(nil))` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `go test -race ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage `internal/variable` | 96.4% |
| Coverage `internal/runner` | 85.2% |
| Coverage (total) | 87.1% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 2887a28 | fix(variable): add sync.RWMutex to SensitiveSet for concurrent safety | iter1 #1 |
| fda1697 | test(runner): add parallel-mode HMAC test to catch concurrent sensitive-key races | iter1 #2 |
| 9bfc069 | fix(docs): correct $hmacSha256 pseudocode in MANUAL.md | iter2 #1 |

## Summary

3/3 findings resolved across 2 review iterations. 0 deferred.
