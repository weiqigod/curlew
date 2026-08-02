# Code Review: M1-027

**Task:** AI introspection commands (info, schema)
**Reviewer:** AI
**Date:** 2026-03-18
**Branch:** feature/M1-027-ai-introspection-commands

## Verdict: PASS

## Findings

No findings. All previous findings from the initial review have been resolved.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors properly returned with context. `ListCollections` correctly returns nil for missing dir (documented). Errors in `infoCmd` and `schemaCmd` are printed to stderr with appropriate exit codes. |
| Input Validation | PASS | `parseInfoArgs` and `parseSchemaArgs` reject unknown arguments with clear error messages. Nil-to-empty-slice coercion prevents JSON null output. |
| Naming | PASS | No stuttering. Follows established patterns (`InfoJSONOutput` mirrors `ValidationJSONOutput`, `WriteInfoJSON` mirrors `WriteValidationJSON`). Doc comments on all exports. |
| Code Organization | PASS | Clean package boundaries. `internal/schema/` is single-purpose. `internal/config/discovery.go` fits naturally in config package. New code follows existing patterns throughout. |
| Correctness | PASS | Nil-to-empty-slice coercion (lines 776-782) prevents JSON null. Schema trailing newline handled. `chdir` tests properly restore working directory with defer. |
| Test Quality | PASS | 16 info tests, 5 schema tests, 1 help test, 6 discovery tests, 4 info-output tests, 4 schema-content tests. All 5 task behaviors covered. Error paths and edge cases tested. Table-driven with `t.Run`. Smoke tests exercise real binary. |

## Test Coverage
- `internal/config`: 96.8%
- `internal/output`: 91.8%
- `internal/schema`: no statements (only `go:embed` declaration)
- `cmd/curlew`: 87.2%
- All above 80% threshold

## Summary

Clean, well-structured implementation following established patterns. Previous review findings (dead `--no-color` code, missing `no_environments_shows_none` test) have been fully resolved. All 5 task behaviors are covered by tests, the JSON Schema accurately reflects the collection format, and the code passes lint with zero issues.
