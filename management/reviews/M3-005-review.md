# Code Review: M3-005 (Iteration 4)

**Task:** apitest import openapi: parse spec and emit collection skeleton
**Reviewer:** AI
**Date:** 2026-04-14
**Branch:** feature/M3-005-openapi-import

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; sentinel errors `ErrSpecInvalid` and `ErrNoOperations` correctly wired via `errors.Is` through `Structured.Unwrap()`; `defer enc.Close()` documented as accepted coverage gap (yaml.v3 Close() never returns error for plain structs); no swallowed errors; `fmt.Errorf("%w: %v", ErrSpecInvalid, err)` pattern correctly preserves sentinel for `errors.Is` while appending original error message. |
| Input Validation | PASS | File-not-found and invalid-YAML both handled with `ErrSpecInvalid`; no-servers case defaults to empty `base_url`; no-operations returns `ErrNoOperations`; `doc.Validate()` catches spec constraint violations. |
| Naming | PASS | All exported symbols have doc comments (`Import`, `Emit`, `ErrSpecInvalid`, `ErrNoOperations`); no stuttering; package name `openapi` is lowercase single-word; unexported helper structs (`writerCollection`, `writerRequestItem`, `writerRequest`) correctly scoped; test function typo from iteration 3 fixed (`TestChosenName_Collision`). |
| Code Organization | PASS | `internal/openapi` package is well-scoped with narrow public interface (`Import`, `Emit`, two sentinels); writer-view structs are unexported projections avoiding blast-radius changes to the parser package; no circular dependencies; `kin-openapi` in direct require block; `defer f.Close()` on output file uses `//nolint:errcheck` correctly. |
| Correctness | PASS | All 9 behaviors verified with passing tests; path sorting ensures deterministic output; `0o644` file permissions correctly enforced; feature gate returns exit 6 for both Free and Solo tiers; structured error output used for import failures via `printer.StructuredError`; binary integration test exercises real binary round-trip; `loader.Context` initialized to `context.Background()` by kin-openapi's `NewLoader`. |
| Test Quality | PASS | All 9 task behaviors covered by named tests; table-driven tests used for `TestSynthName` and `TestInterpolatePath`; `t.Run()` subtests with descriptive names; error paths covered (file not found, invalid spec, no operations); binary integration test via `os/exec`; `testdata/` fixtures used throughout; deferred close coverage gap documented as accepted exception. |

## Test Coverage
- Coverage (`internal/openapi`): **95.8%** — well above the 80% threshold.
- Single accepted gap: `emit.go` line 56 (`err = cerr` in deferred close). yaml.v3's `Close()` cannot return an error for the plain struct types encoded here; documented in source code with explanatory comment.

## Behavior Coverage

All 9 behaviors from the task YAML are covered by at least one test:

| Behavior | Tests |
|----------|-------|
| Collection name from info.title | `TestImport_Petstore`, `TestImportOpenAPI_WritesToStdout` |
| base_url from servers[0], URL uses `{{base_url}}` | `TestImport_Petstore`, `TestEmit_ContainsExpectedKeys` |
| operationId → request name | `TestImport_Petstore` |
| Synthesised name when no operationId | `TestImport_NoOperationId`, `TestSynthName` |
| Path params → `{{name}}` interpolation | `TestInterpolatePath`, `TestEmit_ContainsExpectedKeys` |
| OpenAPI 3.1 accepted without error | `TestImport_OpenAPI31` |
| Missing/invalid spec → exit code 3 | `TestImportOpenAPI_MissingSpec`, `TestImportOpenAPI_InvalidSpec`, `TestImport_FileNotFound`, `TestImport_InvalidSpec` |
| Free/Solo tier → exit code 6 | `TestImportOpenAPI_FreeTierGated`, `TestImportOpenAPI_SoloTierGated` |
| `--output` writes 0644 file | `TestImportOpenAPI_WritesCollection` |

## Summary

All findings from iterations 1-3 have been fully resolved. The implementation is correct, well-tested at 95.8% coverage, lint-clean, and all 9 task behaviors are verified. The single uncovered branch in `emit.go` (deferred `enc.Close()` error path) is documented in source as an accepted gap, since yaml.v3's encoder cannot produce a Close error for the plain struct types used here. The binary integration test exercises a full import-then-validate round-trip against the real binary.
