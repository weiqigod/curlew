# Code Review: M8-002

**Task:** Schema completeness: auth, retry, data_driven, section/variables object forms, status union
**Reviewer:** AI
**Date:** 2026-04-24
**Branch:** feature/M8-002-schema-completeness

## Verdict: PASS

## Pre-audit Gate

`./scripts/ci-local.sh --go` passes cleanly: `go build`, `go test`, `go test -race`, coverage, `golangci-lint` (0 issues), and all smoke assertions.

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Test-only code; no production error paths changed. `filepath.Rel` error now handled explicitly (fallback to absolute path). |
| Input Validation | PASS | Schema is a data artifact. All eight gaps covered with acceptance fixtures; negative controls for malformed status and mutual-exclusion variable entries. |
| Naming | PASS | No stuttering. Exported `CollectionSchema` has doc comment. All helper functions have descriptive names. `repoRoot` defined once in `validate_coverage_test.go`, reused across both test files. |
| Code Organization | PASS | Changes confined to `internal/schema/` test files and `schemas/collection-v1.json`. No cross-package leakage. `compileSchema` duplicate removed; `publishedSchemaPath` now delegates to `repoRoot`. |
| Correctness | PASS | Schema fields verified against `internal/retry.FullConfig`, `internal/datadriven.Config`, and `internal/parser.SensitiveVars`. `backoff_strategy` enum (`exponential`, `linear`, `constant`) confirmed against `internal/retry/backoff.go`. `variableEntry not-constraint` correctly enforces mutual exclusion, confirmed by live test. Section object branch correctly rejects unknown fields via `additionalProperties: false`. |
| Test Quality | PASS | All 8 task behaviors covered. `TestSchema_accepts` has 10 sub-tests (8 gaps, gap 6 split into section and requests object forms, gap 8 split into integer and array). `TestSchema_examples` validates 10 gap fixtures + `sample/hello.yaml`. Two negative tests: `TestSchema_rejects_malformed_status` and `TestSchema_rejects_variable_with_both_value_and_from_command`. `smoke/run.sh` guarded with `trap 'rm -f "$SEED_FILE"' EXIT`. |

## Previous Review Findings — All Resolved

| # | Severity | Finding | Resolution |
|---|----------|---------|------------|
| 1 | Critical | `smoke/run.sh` no trap for stale temp file | `SEED_FILE=""` + `trap 'rm -f "$SEED_FILE"' EXIT` added before `mktemp` |
| 2 | Medium | Duplicate `compileSchema` in `validate_coverage_test.go` | Removed; all call-sites use `compileCollectionSchema` from `validate_test.go` |
| 3 | Medium | `repoRoot` logic duplicated across both test files | Single `repoRoot` in `validate_coverage_test.go`; `publishedSchemaPath` delegates to it |
| 4 | Medium | No acceptance test for `requests:` in object form | `gap_6b_requests_object_form.yaml` + `requests_object_form` sub-test added |
| 5 | Low | `filepath.Rel` error silently discarded | Explicit `if err != nil { rel = path }` fallback |

## Test Coverage

- Coverage: `internal/schema` package reports `[no statements]` — the package is a single `var` alias (`var CollectionSchema = schemas.CollectionV1`) with no executable statements. This is correct behavior; the DoD threshold (`≥ 80%`) is not violated.
- All 8 gap-closing behaviors from the task YAML are exercised by dedicated sub-tests in `TestSchema_accepts`.
- Regression suite `TestSchema_examples` covers 11 YAML files (10 gap fixtures + `sample/hello.yaml`) and guards against schema changes that break existing valid collections.
- Negative controls present for Gap 7 (variable mutual exclusion) and Gap 8 (malformed status).
- `golangci-lint` reports 0 issues on both `internal/schema` and `schemas` packages.

## Spec Compliance

| Behavior | Test | Status |
|----------|------|--------|
| `requestItem.auth` as string ref to auth profile | `TestSchema_accepts/request_auth_string`, `gap_1_request_auth.yaml` | PASS |
| `retry` at collection scope | `TestSchema_accepts/collection_retry`, `gap_2_collection_retry.yaml` | PASS |
| `retry` at section scope | `TestSchema_accepts/section_retry`, `gap_3_section_retry.yaml` | PASS |
| `retry` at request scope | `TestSchema_accepts/request_retry`, `gap_4_request_retry.yaml` | PASS |
| `data_driven` at request scope | `TestSchema_accepts/data_driven_request`, `gap_5_data_driven.yaml` | PASS |
| `setup`/`teardown`/`requests` accept object form `{retry, items}` | `TestSchema_accepts/section_object_form`, `requests_object_form` | PASS |
| `variables` accept object form with `from_command`/`value`/`sensitive`/`cache` | `TestSchema_accepts/variables_object_form`, `gap_7_variables_object.yaml` | PASS |
| `assertions.status` constrained to `oneOf[integer, array[integer]]` | `TestSchema_accepts/status_integer`, `status_array` | PASS |
| Every fixture in examples/ validates | `TestSchema_examples` (11 files) | PASS |

## Summary

All five findings from the prior review are resolved: the `smoke/run.sh` trap prevents stale temp files, both helper-function duplicates are eliminated, the `requests` object-form gap is covered, and the silently-discarded error is handled. The schema additions are accurate and exhaustive — every field is verified against the corresponding Go struct. No new issues found.
