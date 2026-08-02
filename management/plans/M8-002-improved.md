# Improvement Report: M8-002

**Task:** Schema completeness: auth, retry, data_driven, section/variables object forms, status union
**Date:** 2026-04-24
**Review:** management/reviews/M8-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `./scripts/ci-local.sh --go` fails at smoke step: `mktemp` cannot create `/tmp/curlew_seed_XXXXXX.yaml` because a leftover file blocks the call; smoke script has no cleanup on failure | Removed stale `/tmp/curlew_seed_XXXXXX.yaml`; added `SEED_FILE=""` + `trap 'rm -f "$SEED_FILE"' EXIT` in `smoke/run.sh` before `mktemp` so interrupted runs clean up automatically. `SEED_FILE` initialised to `""` first to satisfy `set -u`. | ✓ tests pass |
| 2 | Medium | `compileSchema` in `validate_coverage_test.go` is functionally identical to `compileCollectionSchema` in `validate_test.go` (same package) — silent divergence risk | Removed `compileSchema` from `validate_coverage_test.go`; replaced all four call-sites with `compileCollectionSchema` (already in the same package). Removed now-unused `bytes` and `schema` imports from coverage test file. | ✓ tests pass |
| 3 | Medium | `repoRoot` in `validate_coverage_test.go` duplicates the root-finding logic inline in `publishedSchemaPath` in `validate_test.go` (both use `runtime.Caller(0)` + three `filepath.Dir` walks) | Kept `repoRoot` as the single authoritative helper in `validate_coverage_test.go`; refactored `publishedSchemaPath` in `validate_test.go` to delegate: `return filepath.Join(repoRoot(t), "schemas", "collection-v1.json")`. Removed now-unused `runtime` import from `validate_test.go`. | ✓ tests pass |
| 4 | Medium | No acceptance test for `requests:` in object form despite schema widening `requests` to `$ref: #/$defs/section` (same as `setup`/`teardown`) | Added `internal/schema/testdata/gap_6b_requests_object_form.yaml` fixture with `requests:` in object form `{retry, items}`; added `{"requests_object_form", "gap_6b_requests_object_form.yaml"}` sub-test to `TestSchema_accepts`. | ✓ tests pass |
| 5 | Low | `filepath.Rel(root, path)` error discarded silently (`_, _`) in `TestSchema_examples` | Changed to `rel, err := filepath.Rel(root, path); if err != nil { rel = path }` — explicit fallback that satisfies project convention and avoids future linter violations. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage | `internal/schema` package has `[no statements]` (single `var` alias) — DoD threshold not violated; all other packages cached PASS |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 86bc777 | fix(schema): remove duplicate helpers and add requests object-form test | #2, #3, #4, #5 |
| e4edba3 | fix(smoke): guard SEED_FILE with EXIT trap to prevent stale temp files | #1 |

## Summary
5/5 findings resolved. 0 deferred.
