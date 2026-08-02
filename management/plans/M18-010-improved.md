# Improvement Report: M18-010

**Task:** Compliance artefacts (1/2): COMPLIANCE.md umbrella + policy templates + data-classification matrix
**Date:** 2026-05-19
**Review:** management/reviews/M18-010-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `docs/COMPLIANCE.md` line 34: `vendor-inventory.md` referenced as plain text instead of a markdown link, failing behaviour 1 requirement | Added `[Vendor Inventory](security/vendor-inventory.md) (M18-011 — forthcoming)` markdown link; added `vendor-inventory.md` pattern to `.markdown-link-check.json` ignorePatterns so the forthcoming file does not fail link-check | ✓ tests pass |
| 2 | Medium | `DataClassificationMatrixDocTests.cs` method name, field doc-comment, and cross-reference in `data-classification-matrix.md` all said "32 tables" but 36 are hardcoded in `SchemaAppendixTables` | Renamed `Matrix_lists_all_32_schema_appendix_tables` → `Matrix_lists_all_36_schema_appendix_tables`; updated field doc-comment; updated cross-reference in `data-classification-matrix.md` Review Cadence section | ✓ tests pass |
| 3 | Low | `npx markdown-link-check` (without `--config`) fails on `mailto:` links because the tool does not auto-discover `.markdown-link-check.json` | Updated DoD line in `management/tasks/M18-010.yaml` to specify `--config .markdown-link-check.json`; the config file already excluded `^mailto:` | ✓ tests pass |
| 4 | Low | Task `status` was still `planned` instead of `review` | Updated `management/tasks/M18-010.yaml` status field to `review` | ✓ |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 88.8% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 1b7a5adf | fix(compliance): resolve review findings for M18-010 | #1, #2, #3, #4 |

## Summary

4/4 findings resolved. 0 deferred.
