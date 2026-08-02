# Improvement Report: M4-012

**Task:** E2E: CLI run -> backend ingest -> web dashboard
**Date:** 2026-04-17
**Review:** management/reviews/M4-012-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `wantState` field in `payload_test.go` declared but never asserted — item-level status regressions silently missed | Added `if tc.wantState != "" && len(payload.Items) > 0 { assert Items[0].Status == tc.wantState }` in the test loop | ✓ tests pass |
| 2 | High | `data_driven_iteration_folded` test case doesn't assert item names contain `[1/2]`/`[2/2]` markers | Added `wantItemNames []string` field to the test struct with `{"login [1/2]", "login [2/2]"}` for the data-driven case; added index-matched assertion loop | ✓ tests pass |
| 3 | High | `smoke/run.sh` missing stack lifecycle idempotency assertion (DoD item not met) | Added `=== Stack idempotency (M4-012) ===` section at end of `smoke/run.sh` gated by `APITEST_MANAGE_STACK=1`; prints SKIP with instructions when stack not available, runs COUNT_BEFORE/COUNT_AFTER org-count check when stack is up | ✓ SKIP path passes in CI |
| 4 | Medium | `scripts/seed-test-data.sh` results fixtures re-posted on every `up` — not idempotent | Added existing-results count check before fixture loop; skips all fixture posts if `EXISTING_RESULTS >= FIXTURE_COUNT` (5 non-failing fixtures) | ✓ logic verified |
| 5 | Medium | gofumpt formatting violation in `cmd/apitest/main.go` `runFlags` struct field alignment | Applied `~/go/bin/gofumpt -w cmd/apitest/main.go` | ✓ golangci-lint 0 issues |
| 6 | Medium | gofumpt trailing newline at end of `internal/prcheck/payload.go` | Removed trailing blank line; applied `~/go/bin/gofumpt -w internal/prcheck/payload.go` | ✓ golangci-lint 0 issues |
| 7 | Low | `makeFailResult(name string, _ string)` should be `makeFailResult(name, _ string)` per gofumpt extra-rules | Applied `~/go/bin/gofumpt -extra -w` to `payload_test.go` and `report_upload_test.go` (extra-rules also flagged named return param grouping in the latter) | ✓ golangci-lint 0 issues |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage `internal/prcheck` | 85.4% |
| Coverage `cmd/apitest` | 83.5% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 66ab6ac | fix(prcheck): resolve test quality and gofumpt findings | #1, #2, #5, #6, #7 |
| abc839b | fix(e2e): resolve seed idempotency and smoke stack lifecycle findings | #3, #4 |

## Summary

7/7 findings resolved. 0 deferred.
