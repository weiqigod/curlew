# Verification Report: M12-005

**Task:** $hmacSha256 dynamic function with sensitive-key propagation
**Verified by:** AI
**Date:** 2026-04-28
**Branch:** feature/M12-005-hmac-sha256
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | No races detected (including `TestRun_HmacSha256_Parallel_BothKeysRegistered`) |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage `internal/variable` | 96.4% | Meets >= 80% threshold |
| Coverage `internal/runner` | 85.2% | Meets >= 80% threshold |
| Coverage `cmd/curlew` | 82.3% | Meets >= 80% threshold |
| `./scripts/ci-local.sh` | PASS | All gates pass |

## Observable Output

```
go test -run 'TestRegistry_HmacSha256' -v ./internal/variable/...
=== RUN   TestRegistry_HmacSha256
=== RUN   TestRegistry_HmacSha256/rfc-style_fox_vector
=== RUN   TestRegistry_HmacSha256/empty_payload_empty_key
=== RUN   TestRegistry_HmacSha256/empty_payload_non-empty_key
=== RUN   TestRegistry_HmacSha256/utf-8_payload_and_key
--- PASS: TestRegistry_HmacSha256 (0.00s)
    --- PASS: TestRegistry_HmacSha256/rfc-style_fox_vector (0.00s)
    --- PASS: TestRegistry_HmacSha256/empty_payload_empty_key (0.00s)
    --- PASS: TestRegistry_HmacSha256/empty_payload_non-empty_key (0.00s)
    --- PASS: TestRegistry_HmacSha256/utf-8_payload_and_key (0.00s)
...
PASS  ok  github.com/weiqigod/curlew/internal/variable  0.180s

go test -run 'TestRegistry_HmacSha256_KeyIsSensitive' -v ./internal/variable/...
=== RUN   TestRegistry_HmacSha256_KeyIsSensitive_HeuristicName
--- PASS: TestRegistry_HmacSha256_KeyIsSensitive_HeuristicName (0.00s)
=== RUN   TestRegistry_HmacSha256_KeyIsSensitive_SecretsNamespace
--- PASS: TestRegistry_HmacSha256_KeyIsSensitive_SecretsNamespace (0.00s)
PASS  ok  github.com/weiqigod/curlew/internal/variable  (cached)
```

Expected: RFC fox vector = `f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8`; KeyIsSensitive tests pass.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Two strings → 64-char lowercase hex HMAC-SHA-256 | `TestRegistry_HmacSha256` | PASS |
| 2 | RFC fox vector = `f7bc83f4…` | `TestRegistry_HmacSha256/rfc-style_fox_vector` | PASS |
| 3 | Zero/one/three args → arity-mismatch error naming arity 2 | `TestRegistry_HmacSha256_arity_errors` | PASS |
| 4 | Key from sensitive var or `{{secrets.X}}` → `AddValue` on `runtimeSensitive` | `TestRegistry_HmacSha256_KeyIsSensitive_HeuristicName`, `TestRegistry_HmacSha256_KeyIsSensitive_SecretsNamespace` | PASS |
| 5 | Literal key → no `SensitiveSet` mutation | `TestRegistry_HmacSha256_LiteralKey_NotMarked` | PASS |
| 6 | Payload from sensitive var → no mutation (only key triggers) | `TestRegistry_HmacSha256_PayloadSensitive_NotKey` | PASS |
| 7 | Two calls with different keys → both correct, both keys captured | `TestRegistry_HmacSha256_TwoCallsDifferentKeys`, `TestRun_HmacSha256_Parallel_BothKeysRegistered` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | 7/7 tests pass | PASS |
| 2 | `go test ./...` passes | All packages pass | PASS |
| 3 | `go test -cover ./internal/variable/...` >= 80% | 96.4% | PASS |
| 4 | `golangci-lint run` passes with 0 issues | ci-local.sh lint gate passes | PASS |
| 5 | `./smoke/run.sh` passes | ci-local.sh smoke gate passes | PASS |
| 6 | `./scripts/ci-local.sh` passes | `=== ci-local PASS ===` | PASS |
| 7 | `docs/MANUAL.md §3.7` documents `$hmacSha256` and key-sensitivity rule | Section present with literal-key caveat and corrected pseudocode | PASS |
| 8 | Smoke/integration fixture for Stripe-style X-Signature | `TestRun_HmacSha256_Integration` in runner_test.go | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |
| Concurrent safety | PASS (`sync.RWMutex` on `SensitiveSet`) |

Branch A: Review PASS trusted (from `management/reviews/M12-005-review.md`), spot-check clean — `%w` error wrapping confirmed in `dynamic.go:129`, doc comments on all exported symbols confirmed, `TestRun_HmacSha256_Parallel_BothKeysRegistered` confirms it catches the data-race scenario.

## Commits

| Hash | Message |
|------|---------|
| f1a38ed | docs(review): add passing review for M12-005 |
| 760cf67 | docs(review): update improvement report for M12-005 (iteration 2) |
| 9bfc069 | fix(docs): correct $hmacSha256 pseudocode in MANUAL.md |
| d3b3ca0 | docs(review): add review with findings for M12-005 |
| c3f949d | docs(review): add improvement report for M12-005 |
| fda1697 | test(runner): add parallel-mode HMAC test to catch concurrent sensitive-key races |
| 2887a28 | fix(variable): add sync.RWMutex to SensitiveSet for concurrent safety |
| 39382ab | docs(review): add review with findings for M12-005 |
| 6aec193 | chore(task): mark M12-005 as review |
| a3b9474 | refactor(runner): fix gofumpt alignment of RuntimeSensitive field |
| 2f7049b | docs(manual): add $hmacSha256 to §3.7 with sensitive-key callout (Step 5) |
| 46cf636 | test(runner): add end-to-end integration test for $hmacSha256 Stripe-style header (Step 4) |
| aec1115 | feat(variable): add sensitiveArgIdx hook and per-arg sensitivity tracking (Step 3) |
| 34b52b3 | test(variable): add failing tests for sensitive-arg propagation (Step 3) |
| a50f9f5 | feat(variable): wire runtimeSensitive set through Scope and RunSummary (Step 2) |
| b05919f | test(variable): add failing tests for WithRuntimeSensitive (Step 2) |
| 6a18028 | feat(variable): add twoArgs helper and $hmacSha256 registration |
| 1311480 | test(variable): add failing tests for $hmacSha256 (Step 1) |
| b663e70 | chore(task): mark M12-005 as in_progress |
| 63cd8b3 | chore(task): mark M12-005 as planned |

## Files Changed

| File | Action |
|------|--------|
| `internal/variable/dynamic.go` | modified — added `twoArgs`, `$hmacSha256` registration, `SensitiveArgIndex` |
| `internal/variable/variable.go` | modified — per-arg sensitivity tracking, `WithRuntimeSensitive` |
| `internal/variable/sensitive.go` | modified — added `sync.RWMutex` to `SensitiveSet`, all methods lock-safe |
| `internal/variable/dynamic_test.go` | modified — 7 behavior tests |
| `internal/variable/variable_test.go` | modified — sensitivity tracking tests |
| `internal/runner/runner.go` | modified — `RuntimeSensitive` on `Summary`, wires through `runtimeSensitive` |
| `internal/runner/runner_test.go` | modified — integration + parallel tests |
| `cmd/curlew/main.go` | modified — merges `RuntimeSensitive` into post-run redaction |
| `docs/MANUAL.md` | modified — §3.7 `$hmacSha256` row + callout block |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
