# Improvement Report: M4-001

**Task:** Shared vault configuration template format
**Date:** 2026-04-14
**Review:** management/reviews/M4-001-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `parse.go:22` — `fmt.Errorf("%w: %v", ...)` uses `%v` for inner error, breaking error chain | Changed `%v` to `%w` so callers can unwrap to the underlying yaml decode error via `errors.As` | ✓ tests pass |
| 2 | Medium | `parse.go:65` — same `%v` instead of `%w` for environment-level decode error | Same fix — changed `%v` to `%w` | ✓ tests pass |
| 3 | Low | `teamtemplate.go:76` — `ParseReader` exported with 0% coverage and no callers | Removed entirely (including the `io` import). M4-002 can re-introduce when a runtime consumer needs it. | ✓ tests pass, lint clean |
| 4 | Low | `validate.go:72` — `for alias, raw := range env.Keys` iterates a `map[string]string` non-deterministically | Collect aliases into `[]string`, sort with `sort.Strings`, then iterate | ✓ tests pass |
| 5 | Low | `main.go:1517` — `validateCmd` doc comment omits exit code 2 | Updated comment to: "Exit codes: 0 = valid (or warnings only), 1 = usage error, 2 = team template validation error, 3 = collection validation errors." | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (`internal/vault/teamtemplate`) | 87.8% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `387ef20` | fix(teamtemplate): preserve error chain in parseBytes with %w | #1, #2 |
| `57ad07c` | fix(teamtemplate): remove unexported ParseReader with no callers | #3 |
| `a4423f4` | fix(teamtemplate): sort key aliases before iteration for deterministic output | #4 |
| `fda1310` | fix(validate): add exit code 2 to validateCmd doc comment | #5 |

## Summary

5/5 findings resolved. 0 deferred.
