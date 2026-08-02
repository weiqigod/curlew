# Improvement Report: M7-003

**Task:** Help-after-error: emit one-line usage synopsis on stderr alongside the error
**Date:** 2026-04-22
**Review:** management/reviews/M7-003-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | Dead `"run"` entry in `usageSynopses` map had no production call site and no sync test, misleading readers. | Removed the `"run"` key from `usageSynopses` in `cmd/apitest/main.go`. Every remaining entry is wired to an active error-recovery site covered by `TestStreamHelp`. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage | 86.6% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 6409089 | fix(cli): remove unused "run" entry from usageSynopses map | #1 |

## Summary
1/1 findings resolved. 0 deferred.
