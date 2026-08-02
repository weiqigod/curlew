# Improvement Report: M15-001 (Iteration 2)

**Task:** plugin_loading Enterprise gate enforced at APITEST_PLUGINS load sites
**Date:** 2026-05-07
**Review:** management/reviews/M15-001-review.md

## Resolved Findings (Iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `errors.As(gateErr, &ge)` return value discarded in `pluginsListCmdOut` (lines 123–125). If `errors.As` returns false, `ge` remains nil and `printer.FeatureGate(&ge.Result)` panics with nil pointer dereference. | Wrapped in `if errors.As(gateErr, &ge) { ... }` guard; added else fallback `_, _ = fmt.Fprintf(stderr, "error: %v\n", gateErr); return 1` — matching the `main.go:1242` pattern exactly. | ✓ tests pass |

## Findings from Previous Iterations

| # | Severity | Finding | Status |
|---|----------|---------|--------|
| 1 (iter 1) | Medium | Workaround text not asserted at CLI level; run-path used `StructuredError` instead of `FeatureGate` | RESOLVED in iter 1 |
| 2 (iter 1) | Low | Dead `errors.As` fallback branch in `pluginsListCmdOut` | Partially addressed in iter 1; residual nil-panic issue resolved in iter 2 |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (`cmd/apitest`) | 81.5% |
| Coverage (`internal/auth`) | 89.6% |
| Coverage (total) | 87.3% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 5cd3341f | fix(plugins): resolve review findings #1 and #2 | iter1 #1, iter1 #2 |
| a2f0e423 | fix(plugins): guard errors.As return value in pluginsListCmdOut gate | iter2 #1 |

## Summary

1/1 findings resolved (iteration 2). 0 deferred.
