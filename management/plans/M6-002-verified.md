# Verification Report: M6-002

**Task:** Source-location plumbing: file and line onto parsed items and results
**Verified by:** AI
**Date:** 2026-04-21
**Branch:** feature/M6-002-source-location-plumbing
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage `internal/parser` | 89.8% | Meets >= 80% threshold |
| Coverage `internal/runner` | 85.6% | Meets >= 80% threshold |
| Coverage `internal/parallel` | 90.6% | Meets >= 80% threshold |
| Coverage total | 86.6% | Meets >= 80% threshold |

## Observable Output

```
=== RUN   TestParse_CarriesSourceLocation
--- PASS: TestParse_CarriesSourceLocation (0.00s)
PASS
ok      github.com/peterlindqvist/apitest/internal/parser       0.192s

=== RUN   TestParse_IncludesCarryIncludedFilePath
--- PASS: TestParse_IncludesCarryIncludedFilePath (0.00s)
PASS
ok      github.com/peterlindqvist/apitest/internal/parser       0.193s

=== RUN   TestRunner_RequestResultCarriesSourceLocation
--- PASS: TestRunner_RequestResultCarriesSourceLocation (0.00s)
PASS
ok      github.com/peterlindqvist/apitest/internal/runner       0.214s
```

Expected: PASS for all three observable tests
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Direct file: SourceFile = abs collection path, SourceLine = YAML node line | `TestParse_CarriesSourceLocation`, `TestRequestItem_UnmarshalYAML_capturesLine` | PASS |
| 2 | include:: requests from included file carry included file's SourceFile | `TestParse_IncludesCarryIncludedFilePath` | PASS |
| 3 | request_file: (path:): SourceFile = external file, SourceLine = 1 | `TestParse_ExternalRequestCarriesExternalFilePath` | PASS |
| 4 | Data-driven iterations copy base item's (SourceFile, SourceLine) verbatim | `TestRunner_DataDrivenIterationsCarrySourceLocation`, `TestRunner_ParallelDataDrivenCarriesSourceLocation` | PASS |
| 5 | Runner RequestResult.SourceFile/SourceLine match originating RequestItem | `TestRunner_RequestResultCarriesSourceLocation`, `TestRunner_ParallelWebSocketCarriesSourceLocation` | PASS |
| 6 | Existing output formatters produce identical output (additive-only change) | All existing tests green, no golden changes | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | 8 behavior tests pass (parser + runner packages) | PASS |
| 2 | Existing test suite remains green | `go test ./...` all packages pass | PASS |
| 3 | golangci-lint run passes with 0 issues | `golangci-lint run` output: 0 issues | PASS |
| 4 | Coverage for internal/parser and internal/runner >= 80% | parser: 89.8%, runner: 85.6% | PASS |
| 5 | New parser tests cover: direct file, include:, request_file:, data-driven | 4 new parser tests + 2 runner tests + 2 parallel tests | PASS |
| 6 | Smoke test passes | `./smoke/run.sh` all checks pass | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling (`%w` wrapping) | PASS |
| Naming conventions (no stuttering, doc comments) | PASS |
| Code organization (internal/ boundaries, private helpers) | PASS |
| Test quality (table-driven, covers parallel and sequential paths) | PASS |
| Sentinel errors | PASS |
| No goroutine leaks | PASS |
| No shared mutable state | PASS |

Branch A: Review PASS trusted, spot-check clean.
- `parser.go`: `%w` wrapping confirmed on all error sites; `stampSourceFile` helper has doc comment.
- `collection.go`: `SourceFile` and `SourceLine` fields have doc comments; `UnmarshalYAML` documented.
- `runner.go`: All 15+ `RequestResult` construction sites carry `SourceFile`/`SourceLine`.

## Commits

| Hash | Message |
|------|---------|
| ccfa2b3 | docs(review): add passing review for M6-002 |
| 44719cd | docs(review): add improvement report for M6-002 |
| ecd7c7e | fix(runner): thread SourceFile/SourceLine through parallel WebSocket and data-driven paths |
| d641f36 | docs(review): add review with findings for M6-002 |
| 8a89cde | chore(task): mark M6-002 as review |
| 20d9365 | refactor(runner): fix gofumpt alignment in data-driven RequestResult literals |
| 9c8b07e | feat(runner): thread SourceFile/SourceLine into RequestResult and RequestOutcome |
| 8cab24b | test(runner): add failing tests for source location threading |
| 35c67a6 | feat(parser): add SourceFile/SourceLine to RequestItem with stamp pass |
| 73e5e27 | test(parser): add failing tests for source location plumbing |
| 6b1f731 | chore(task): mark M6-002 as in_progress |
| ac042fd | chore(task): mark M6-002 as planned |
| 8a52992 | docs(plan): add implementation plan for M6-002 |

## Files Changed

| File | Action |
|------|--------|
| `internal/parser/collection.go` | modified — added `SourceFile`, `SourceLine` to `RequestItem`, `UnmarshalYAML` |
| `internal/parser/parser.go` | modified — added `stampSourceFile` helper; call after unmarshal |
| `internal/parser/external.go` | modified — stamp `SourceFile`/`SourceLine = 1` on external items |
| `internal/parser/collection_test.go` | created — 4 new behavior tests |
| `internal/parser/parser_test.go` | modified — added `stripSourceLocations` helper for DeepEqual compat |
| `internal/runner/runner.go` | modified — added `SourceFile`/`SourceLine` to `RequestResult`; 15+ construction sites updated |
| `internal/runner/runner_test.go` | modified — 4 new behavior tests (sequential + parallel) |
| `internal/parallel/executor.go` | modified — added `SourceFile`/`SourceLine` to `RequestOutcome` |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
