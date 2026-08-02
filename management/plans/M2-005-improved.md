# Improvement Report: M2-005

**Task:** Azure Key Vault and HashiCorp Vault providers
**Date:** 2026-03-26
**Review:** management/reviews/M2-005-review.md
**Iteration:** 2

## Resolved Findings

### Iteration 1 (review 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `extractSecretData` error branches untested (68.4% coverage) | Added 4 tests: invalid JSON, missing `data` field, non-object `data` field, missing `data.data` field. Coverage now 89.5%. Remaining 2 branches (re-parse after unmarshal, marshal failure) are unreachable by design. | ✓ tests pass |

### Iteration 2 (review 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | Network error case in `classifyError` uses `%s` instead of `%w`, dropping original error from chain | Changed `fmt.Errorf("hashicorp vault: %s. %s", msg, hashicorpNetworkHint)` to `fmt.Errorf("hashicorp vault: %w. %s", err, hashicorpNetworkHint)`. Added TDD test `fetch_network_error_wraps_original_error` verifying `errors.Unwrap` returns the original error. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 94.6% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 7f34c40 | test(vault): add extractSecretData error branch coverage | Iteration 1, #1 |
| a94c2f9 | fix(vault): wrap original error in HashiCorp network error path | Iteration 2, #1 |

## Summary
2/2 findings resolved across 2 iterations. 0 deferred.
