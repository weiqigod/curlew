# Code Review: M6-002

**Task:** Source-location plumbing: file and line onto parsed items and results
**Reviewer:** AI
**Date:** 2026-04-21
**Branch:** feature/M6-001-error-taxonomy

## Verdict: PASS

## Findings

_No findings._

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors correctly wrapped with `%w`; no swallowed errors; sentinel errors used for well-known failures. `UnmarshalYAML` propagates decode errors from `node.Decode` unchanged. |
| Input Validation | PASS | nil collection, missing file, empty collection name all handled. YAML alias / unexpected node kind handled via type-alias recursion-guard pattern. `stampSourceFile` skips already-stamped items (guards external + include items). |
| Naming | PASS | No stuttering; exported symbols have doc comments; `stampSourceFile` and `stripSourceLocations` are concise, correctly scoped private helpers. |
| Code Organization | PASS | `internal/` package boundaries respected. `stampSourceFile` lives in `parser.go` (domain owner). `stripSourceLocations` is test-only, unexported. `buildWSOutcome` correctly extracted as a private helper. |
| Correctness | PASS | All parallel paths fixed: `buildWSOutcome` sets `SourceFile`/`SourceLine`; all three early-return sites in `buildWebSocketFunc` (gate error, interpolation error, rate-limit cancellation) set the fields; `buildDataDrivenFunc` conversion loop copies `rr.SourceFile`/`rr.SourceLine`. `filterDataDrivenResults` preserves the fields in both `summary` and `failed_only` branches. |
| Test Quality | PASS | All 8 observable tests pass. Sequential and parallel paths covered for both WebSocket and data-driven cases. Table-driven tests used where appropriate. Symlink portability handled via `filepath.EvalSymlinks`. |

## Test Coverage

- `internal/parser`: 89.8% — above the 80% floor.
- `internal/runner`: 85.6% — above the 80% floor.
- `internal/parallel`: 90.6% — no regression.

All coverage above the 80% threshold required by the Definition of Done.

## Behavior Coverage

| Behavior | Test(s) | Status |
|---------|---------|--------|
| Direct file: SourceFile = abs collection path, SourceLine = YAML node line | `TestParse_CarriesSourceLocation`, `TestRequestItem_UnmarshalYAML_capturesLine` | PASS |
| include:: requests from included file carry included file's SourceFile | `TestParse_IncludesCarryIncludedFilePath` | PASS |
| request_file: (path:): SourceFile = external file, SourceLine = 1 | `TestParse_ExternalRequestCarriesExternalFilePath` | PASS |
| Data-driven iterations copy base item's (SourceFile, SourceLine) verbatim | `TestRunner_DataDrivenIterationsCarrySourceLocation`, `TestRunner_ParallelDataDrivenCarriesSourceLocation` | PASS |
| Runner RequestResult.SourceFile/SourceLine match originating RequestItem | `TestRunner_RequestResultCarriesSourceLocation`, `TestRunner_ParallelWebSocketCarriesSourceLocation` | PASS |
| Existing output formatters produce identical output (additive-only change) | All existing tests green, no golden changes | PASS |

## Summary

All findings from the first review iteration (three correctness gaps in parallel WebSocket and data-driven paths, one missing test coverage finding) were resolved by the `/improve` pass. The implementation is correct across all execution paths — sequential, parallel wave, parallel WebSocket, parallel data-driven — with matching test coverage. No new findings identified.
