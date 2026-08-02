# Improvement Report: M4-007

**Task:** CLI: pr-check subcommand posting status to backend
**Date:** 2026-04-16
**Review:** management/reviews/M4-007-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `TestClient_RetryBackoff` was hollow: created an httptest server and `callTimes` slice but never called any client method, made no assertions, and only emitted a `t.Log` message. The test passed trivially exercising zero code paths. | Replaced the hollow body with a real timing assertion using a closed server (same pattern as `TestClient_RetryOnConnectionRefused`). Now measures elapsed time, asserts `maxRetries+1` (3) attempts were made via `countingRoundTripper`, and asserts elapsed >= `maxRetries * retryInterval` (≥400ms) to confirm backoff sleeps fire. Test ran in 0.40s confirming both retry count and timing. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (`internal/prcheck`) | 91.6% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 0e0e210 | fix(prcheck): replace hollow RetryBackoff test with real timing assertion | #1 |

## Summary
1/1 findings resolved. 0 deferred.
