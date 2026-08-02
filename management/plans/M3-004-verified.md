# Verification Report: M3-004

**Task:** JSON Schema response body validation assertion
**Verified by:** AI
**Date:** 2026-04-14
**Branch:** feature/M3-004-json-schema-assertion
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/curlew` | PASS | Clean build, no warnings |
| `go test ./...` | PASS | 27 packages, all pass |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS (pre-existing failure noted) | One pre-existing failure (`--help missing tap in --format description`) exists on `main` and is not introduced by this branch |
| Coverage (total) | 89.2% | Exceeds >= 80% threshold |
| Coverage `internal/assertion` | 93.0% | |
| Coverage `internal/parser` | 89.3% | |
| Coverage `internal/runner` | 86.2% | |
| Coverage `internal/auth` | 88.9% | |
| Coverage `cmd/curlew` | 83.8% | |

## Observable Output

```
=== Step 1: Valid response at Professional tier (should PASS, exit 0) ===
Collection: Schema Valid Test
  ✓ Get User  200  2ms

────────────────────────────────
  1 request(s): 1 passed, 0 failed (3ms)
Exit code: 0

=== Step 2: Invalid response at Professional tier (should FAIL with structured error) ===
Collection: Schema Invalid Test
  ✗ Get User  200  0ms
    ✗ schema $: expected required: email, got missing
    ✗ schema $.id: expected type integer, got string (not-an-int)

────────────────────────────────
  1 request(s): 0 passed, 1 failed (0ms)
Exit code: 1

=== Step 3: Free tier gate (should exit 6 with schema_validation) ===
[ERROR] JSON Schema response body assertions require Professional tier ($19/month)
Exit code: 6
```

Expected: Pass with valid body; fail with structured error showing JSONPath + expected type + actual value for invalid body; exit 6 at Free tier.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test(s) | Status |
|---|----------|---------|--------|
| 1 | Valid body → assertion passes | `TestCheckSchema/valid_body`, `TestRun_schema_assertion_pass` | PASS |
| 2 | Missing required field → error names property | `TestCheckSchema/missing_required_email`, `TestRun_schema_assertion_fail_missing_field` | PASS |
| 3 | Type mismatch → JSONPath + expected type + actual value | `TestCheckSchema/type_mismatch_id_string_for_int`, `TestRun_schema_assertion_fail_wrong_type` | PASS |
| 4 | Schema file not found → structured parse error at parse time | `TestCompileSchemaFile/file_missing`, `TestParseFile_schema_missing_file_returns_parse_error` | PASS |
| 5 | Invalid JSON Schema syntax → structured parse error | `TestCompileSchemaFile/invalid_schema_syntax`, `TestParseFile_schema_invalid_json_returns_parse_error` | PASS |
| 6 | Non-JSON body → fails with "response body is not JSON" | `TestCheckSchema/body_not_JSON`, `TestCheckSchema/empty_body` | PASS |
| 7 | Schema path resolved relative to collection dir | `TestParseFile_schema_path_relative_to_collection_dir` | PASS |
| 8 | Free/Solo tier → exit code 6 with schema_validation gate | `TestParseFile_schema_gate_blocks_at_free_tier` (unit), `TestCLIIntegration_schema_assertion_free_tier_gated` (binary) | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — 27 packages PASS | PASS |
| 2 | Observable output works as specified | Three-step scenario verified above | PASS |
| 3 | Test coverage >= 80% | 89.2% total; all changed packages >= 80% | PASS |
| 4 | No build warnings or lint errors | Clean `go build`; `golangci-lint run` 0 issues | PASS |
| 5 | Help text updated (if user-facing) | No new user-facing flags added; `schema:` is a collection-file field | PASS |
| 6 | Smoke test updated (if new capability) | No new smoke test required; pre-existing smoke failure is on `main` | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS trusted (iteration 2, post-improve). Spot-check clean:
- Error handling: `ErrSchemaFileNotFound`/`ErrSchemaInvalid` sentinel errors defined; all errors wrapped with `fmt.Errorf("context: %w", err)`; `f.Close()` return value explicitly checked
- Exported symbols: `CompiledSchema`, `CompileSchemaFile`, `CheckSchema`, `ErrSchemaFileNotFound`, `ErrSchemaInvalid` all have doc comments
- Test quality: Table-driven tests (`TestCheckSchema`, `TestCompileSchemaFile`); binary integration test for tier gate; `TestParseFile_schema_path_relative_to_collection_dir` uses `t.Chdir` for realistic path resolution

## Commits

| Hash | Message |
|------|---------|
| 1471b4a | docs(review): add passing review for M3-004 |
| 406934f | docs(review): add improvement report for M3-004 |
| 257106d | test(cmd): add binary integration test for schema_validation free tier gate |
| a30b3b6 | fix(assertion): cover leafToResult default branch and fix nil printer panic |
| 19264f0 | fix(assertion): apply gofumpt formatting to schema_test.go |
| a262029 | fix(assertion): check f.Close() return value in CompileSchemaFile |
| c265411 | fix(deps): promote direct dependencies from indirect in go.mod |
| 3cd299b | docs(review): add review with findings for M3-004 |
| 03bb52c | chore(task): mark M3-004 as review |
| 1637282 | docs(changelog): document M3-004 JSON Schema response body validation |
| bcf3375 | feat(runner): integration tests and main.go wiring for schema validation |
| 239adcf | chore(schema): add schema assertion field to collection.json |
| 37978b4 | feat(runner): pass compiled schema into assertion.EvalInput |
| 48fad6f | feat(parser): compile assertions.schema path and wire SchemaGate (GREEN) |
| 1bcb646 | test(parser): add failing tests for schema assertion parsing (RED) |
| 3a5b4fb | feat(assertion): add Schema field to EvalInput and wire CheckSchema in Evaluate |
| 520d66d | test(assertion): add failing tests for Evaluate with Schema field |
| 9b47732 | feat(assertion): implement CompiledSchema, CompileSchemaFile, CheckSchema |
| de177e4 | test(assertion): add failing tests for schema compilation and validation |
| b5ff44d | chore(deps): add github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 |
| b7ab3d1 | feat(auth): register schema_validation feature at Professional tier |
| 7ce164d | test(auth): add failing test for schema_validation feature registration |
| 6492d88 | chore(task): mark M3-004 as in_progress |
| 1fa6c34 | chore(task): mark M3-004 as planned |
| 7df0535 | docs(plan): add implementation plan for M3-004 |

## Files Changed

| File | Action |
|------|--------|
| `cmd/curlew/main.go` | modified — `schemaGateFor`, wiring |
| `cmd/curlew/main_test.go` | modified — binary integration test |
| `go.mod` / `go.sum` | modified — add jsonschema/v6 direct dep |
| `internal/assertion/schema.go` | added — `CompiledSchema`, `CompileSchemaFile`, `CheckSchema` |
| `internal/assertion/schema_test.go` | added — table-driven tests |
| `internal/assertion/assertion.go` | modified — `EvalInput.Schema`, wire `CheckSchema` |
| `internal/assertion/assertion_test.go` | modified — tests for Schema in EvalInput |
| `internal/assertion/testdata/user.json` | added — valid schema fixture |
| `internal/assertion/testdata/user_bad.json` | added — invalid schema fixture |
| `internal/auth/registry.go` | modified — register `schema_validation` |
| `internal/auth/registry_test.go` | modified — test for new registration |
| `internal/parallel/executor.go` | modified — propagate CompiledSchema |
| `internal/parser/collection.go` | modified — `Assertions.Schema` field |
| `internal/parser/parser.go` | modified — `CompileSchemaFile` at parse time, `SchemaGate` |
| `internal/parser/schema_test.go` | added — parser-level schema tests |
| `internal/parser/testdata/schema/` | added — test schema fixtures |
| `internal/parser/testdata/with_schema_assertion.yaml` | added — fixture |
| `internal/parser/testdata/with_schema_invalid.yaml` | added — fixture |
| `internal/parser/testdata/with_schema_missing.yaml` | added — fixture |
| `internal/schema/collection.json` | modified — add `schema` under assertions |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
