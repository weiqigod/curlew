# Improvement Report: M17-004

**Task:** Webhook-signature dynamic functions: $webhookSign.stripe / .github / .slack
**Date:** 2026-04-29
**Review:** management/reviews/M17-004-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `management/tasks/M17-004.yaml` had `status: planned` — backlog.yaml was correctly updated by the execute phase but the task YAML was not | Updated `status: planned` → `status: review` in `management/tasks/M17-004.yaml` | ✓ tests pass |
| 2 | Low | `TestRegistry_WebhookSign_ArityErrors` did not assert that `err.Error()` contains `$funcName` — the plan Step 2 explicitly specified this check to verify `Evaluate`'s `fmt.Errorf("$%s: %w", name, err)` wrapper propagates correctly | Added `if !strings.Contains(err.Error(), "$"+tt.funcName)` check inside the `t.Run` body in `dynamic_test.go` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/variable/...`) | 97.4% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 62b8d554 | fix(task): resolve review findings for M17-004 | #1, #2 |

## Summary
2/2 findings resolved. 0 deferred.
