# Improvement Report: M3-004

**Task:** JSON Schema response body validation assertion
**Date:** 2026-04-14
**Review:** management/reviews/M3-004-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `gorilla/websocket` and `jsonschema/v6` marked `// indirect` in `go.mod` despite being direct imports | Ran `go mod tidy`; both moved to direct `require` block | ✓ tests pass |
| 2 | High | `defer f.Close()` in `CompileSchemaFile` — return value unchecked; `errcheck` lint failure | Replaced `defer` with explicit `f.Close()` after `UnmarshalJSON`; errors now returned | ✓ tests pass |
| 3 | High | No binary-level integration test for `schema_validation` feature gate at Free tier | Added `TestCLIIntegration_schema_assertion_free_tier_gated` asserting exit code 6 and "Professional" in stderr | ✓ tests pass |
| 4 | Medium | `schema_test.go` did not pass `gofumpt` (line 29 struct field alignment) | Ran `gofumpt -w` on the file | ✓ lint clean |
| 5 | Medium | `leafToResult` default branch (non-type, non-required violations like `minimum`, `enum`) uncovered (58.3%) | Added `TestCheckSchema_default_kind` with `minimum` and `enum` violation cases; also fixed a nil-pointer panic (`LocalizedString(nil)` → `leaf.Error()`) that was triggered by these tests | ✓ leafToResult at 100% |
| 6 | Medium | `CheckSchema` non-`*jsonschema.ValidationError` branch untestable (library always returns `*ValidationError`) | Added inline comment documenting the branch as an unreachable defensive fallback against future library changes | ✓ documented |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/assertion`) | 93.0% |
| Coverage (total) | 89.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| c265411 | fix(deps): promote direct dependencies from indirect in go.mod | #1 |
| a262029 | fix(assertion): check f.Close() return value in CompileSchemaFile | #2 |
| 19264f0 | fix(assertion): apply gofumpt formatting to schema_test.go | #4 |
| a30b3b6 | fix(assertion): cover leafToResult default branch and fix nil printer panic | #5, #6 |
| 257106d | test(cmd): add binary integration test for schema_validation free tier gate | #3 |

## Summary

6/6 findings resolved. 0 deferred.

Notable: Finding #5 exposed a real latent bug — `leafToResult` called `leaf.ErrorKind.LocalizedString(nil)` which panics for any schema keyword using `x/text/message.Printer` (e.g., `Minimum`, `Maximum`, `Enum`). The fix replaces the nil-printer call with `leaf.Error()`, which uses the library's default English printer, matching its own output behaviour.
