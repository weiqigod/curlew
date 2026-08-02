# Improvement Report: M17-001

**Task:** Signer registry foundation and `signing:` request field
**Date:** 2026-04-29
**Review:** management/reviews/M17-001-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `SetSigningExplicitNullForTest` exported from production `collection.go`, polluting public API surface and godoc | Removed the function from `collection.go`; rewrote `TestRunner_SignerStep_CollectionDefault_RequestNullDisables` to use inline YAML fixture parsed via `parser.ParseFile` from a temp file — `signing: ~` decoded by `UnmarshalYAML` naturally, no test-only helper needed | ✓ tests pass |
| 2 | Low | `explicitNull bool` field on `SigningSpec` missing `yaml:"-"` struct tag | Added `yaml:"-"` tag: `explicitNull bool \`yaml:"-"\`` | ✓ tests pass |
| 3 | Low | `fired` counter in `TestRunner_SignerStep_CollectionDefault_RequestNullDisables` was unprotected plain `int` inside exec closure | Changed to `var fired int32` with `atomic.AddInt32` / `atomic.LoadInt32`, matching the project pattern for shared counters in concurrent contexts | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage `internal/signer` | 100% |
| Coverage `internal/parser` | 90.1% |
| Coverage `internal/runner` | 85.0% |
| Overall total | 87.5% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 1cc3f51 | fix(parser,runner): remove test-only helper from production code and improve test safety | #1, #2, #3 |

## Summary

3/3 findings resolved. 0 deferred.
