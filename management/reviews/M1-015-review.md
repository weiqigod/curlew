# Code Review: M1-015

**Task:** External request file references
**Reviewer:** AI
**Date:** 2026-03-12
**Branch:** feature/M1-015-external-request-files

## Verdict: PASS

## Findings

No findings. All issues from the previous review have been resolved.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`, context includes path/name, double-`%w` preserves YAML diagnostics |
| Input Validation | PASS | Mutual exclusivity check, missing URL, file not found, invalid YAML, circular reference all handled |
| Naming | PASS | No stuttering, `externalRequest` correctly unexported, doc comments on all exported and key unexported symbols |
| Code Organization | PASS | External file logic cleanly separated into `external.go`, minimal exported surface, no circular deps |
| Correctness | PASS | Self-reference detected via seeded `visited` map, external files are leaf nodes (no nested references possible), same file reusable with different variables |
| Test Quality | PASS | Table-driven tests, error paths covered, edge cases tested, 3 CLI integration tests, testdata fixtures |

## Test Coverage
- Coverage: 91.1% (parser package)
- `parseExternalFile`: 90.9%
- `resolveExternalReferences`: 96.2%
- Uncovered: permission-denied error path in `parseExternalFile` (acceptable, hard to test portably)

## Behavior Coverage

| Behavior | Test(s) | Status |
|----------|---------|--------|
| External file loaded and executed via `path:` | `external reference loads and merges`, `TestCLIIntegration_external_request_reference` | PASS |
| External behaves identically to inline | `external behaves identically to inline` (verifies Path cleared, method normalized) | PASS |
| Inline variable overrides applied | `external with variable overrides`, `TestCLIIntegration_external_request_with_variables` | PASS |
| Non-existent file error includes path and collection | `external reference not found`, `TestCLIIntegration_external_request_not_found` | PASS |
| Relative path resolved from collection dir | `external reference relative to collection dir` (subdir with `../` path) | PASS |
| External file extract: works normally | `external with extract block`, `preserves_extract`, `preserves_assertions` | PASS |
| Circular reference detected | `circular self-reference detected` (only possible case — external files are leaf nodes) | PASS |

## Summary

Clean implementation after improvements. External file resolution is well-structured with clear separation between `parseExternalFile` (leaf node parsing) and `resolveExternalReferences` (collection-level resolution). The `visited` map correctly handles self-reference detection without blocking valid reuse. All 7 behaviors from the task spec are covered by tests. Coverage is 91.1%.
