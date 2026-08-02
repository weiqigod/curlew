# Code Review: M3-004

**Task:** JSON Schema response body validation assertion
**Reviewer:** AI
**Date:** 2026-04-14
**Branch:** feature/M3-004-json-schema-assertion
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `fmt.Errorf("context: %w", err)`; double `%w` wrapping (Go 1.20+) is intentional for `ErrSchemaInvalid`; `f.Close()` return value explicitly checked (finding #2 from iteration 1 resolved); sentinel errors `ErrSchemaFileNotFound` and `ErrSchemaInvalid` defined and used correctly |
| Input Validation | PASS | Nil `CompiledSchema` returns nil (no-op); empty body returns "response body is not JSON"; missing schema file returns structured `ErrSchemaFileNotFound` at parse time; invalid JSON schema returns `ErrSchemaInvalid`; schema gate fires before any file I/O |
| Naming | PASS | No stuttering; `CompiledSchema`, `CompileSchemaFile`, `CheckSchema`, `ErrSchemaFileNotFound`, `ErrSchemaInvalid` all follow Effective Go; all exported symbols have doc comments; `schemaGateFor` follows `includeGateFor` naming pattern |
| Code Organization | PASS | `go.mod` direct dependencies correctly classified (gorilla/websocket and jsonschema/v6 are in the direct `require` block — finding #1 from iteration 1 resolved); `internal/` boundaries respected; schema compilation encapsulated in `assertion.CompileSchemaFile`; gate logic in `parser.ParseOptions.SchemaGate` mirrors the `IncludeGate` pattern |
| Correctness | PASS | Schema path resolution relative to collection dir (not CWD) verified by `TestParseFile_schema_path_relative_to_collection_dir` using `t.Chdir`; `CompiledSchema` is concurrent-safe per doc comment; race detector passes; all three runner call sites (sequential, parallel, data-driven) propagate `CompiledSchema`; `leafToResult` default branch no longer calls `LocalizedString(nil)` (nil-printer panic fixed) |
| Test Quality | PASS | All 8 behaviors covered; binary-level integration test `TestCLIIntegration_schema_assertion_free_tier_gated` added (finding #3 resolved); `leafToResult` default branch covered by `TestCheckSchema_default_kind` (finding #5 resolved); `gofumpt` formatting applied (finding #4 resolved); unreachable defensive branch in `CheckSchema` documented inline (finding #6 resolved) |

## Test Coverage

- `internal/assertion`: 93.0%
- `internal/parser`: 89.3%
- `internal/runner`: 86.2%
- `internal/auth`: 88.9%
- `cmd/apitest`: 83.8%

All packages exceed the >= 80% threshold.

Remaining sub-function gaps (not findings — all acceptable):
- `CompileSchemaFile`: 82.4% — uncovered branch is `f.Close()` returning an error on a read-only file handle (not practically triggerable in tests without OS-level mocking)
- `CheckSchema`: 88.9% — uncovered branch is the defensive `!errors.As` fallback (documented as unreachable; library always returns `*ValidationError`)
- `lookupAt`: 76.9% — uncovered branch is the `default` case (traversing into a scalar value), which is an internal helper not reachable from the current schema violation types the library produces

## Behavior Coverage

| Behavior | Test(s) | Status |
|----------|---------|--------|
| 1. Valid body → assertion passes | `TestCheckSchema/valid_body`, `TestRun_schema_assertion_pass` | PASS |
| 2. Missing required field → error names property | `TestCheckSchema/missing_required_email`, `TestRun_schema_assertion_fail_missing_field` | PASS |
| 3. Type mismatch → JSONPath + expected type + actual value | `TestCheckSchema/type_mismatch_id_string_for_int`, `TestRun_schema_assertion_fail_wrong_type` | PASS |
| 4. Schema file not found → structured parse error at parse time | `TestCompileSchemaFile/file_missing`, `TestParseFile_schema_missing_file_returns_parse_error` | PASS |
| 5. Invalid JSON Schema syntax → structured parse error | `TestCompileSchemaFile/invalid_schema_syntax`, `TestParseFile_schema_invalid_json_returns_parse_error` | PASS |
| 6. Non-JSON body → fails with "response body is not JSON" | `TestCheckSchema/body_not_JSON`, `TestCheckSchema/empty_body` | PASS |
| 7. Schema path resolved relative to collection dir | `TestParseFile_schema_path_relative_to_collection_dir` | PASS |
| 8. Free/Solo tier → exit code 6 with schema_validation gate | `TestParseFile_schema_gate_blocks_at_free_tier` (unit), `TestCLIIntegration_schema_assertion_free_tier_gated` (binary) | PASS |

## Quality Gate Results

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS (all 27 packages) |
| `go test -race ./internal/assertion/... ./internal/parser/... ./internal/runner/...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| `go vet ./...` | PASS |
| `go mod tidy --diff` | PASS (no changes needed) |
| Coverage >= 80% (all changed packages) | PASS |

## Summary

All 6 findings from iteration 1 are resolved. The implementation is architecturally sound: schema compilation is fail-fast at parse time, the `CompiledSchema` wrapper correctly encapsulates the library dependency and is concurrent-safe, all three runner call sites propagate the compiled schema, the tier gate mirrors the `include_directive` pattern, and a latent nil-printer panic in `leafToResult` was discovered and fixed during improvement. No new issues were introduced.
