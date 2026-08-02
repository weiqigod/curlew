# Improvement Report: M3-003

**Task:** include directive: compose collections with snapshot variable scoping
**Date:** 2026-04-14
**Review:** management/reviews/M3-003-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Hardcoded absolute path `/Users/peterlindqvist/...` in `include_step4_test.go:68` breaks CI portability | Replaced hardcoded path with `filepath.Abs("testdata/include/subdir/parent_relative.yaml")` computed before `t.Chdir()` while CWD is still the package directory | ✓ tests pass |
| 2 | Medium | No fixture or test exercising setup/teardown section splicing (Behavior 1 of task spec) | Added `parent_setup_teardown.yaml` and `child_setup_teardown.yaml` fixtures; added `TestResolveIncludes_setup_and_teardown_spliced` in `include_test.go` asserting correct item counts and names in all three sections | ✓ tests pass |
| 3 | Medium | No binary-level integration test for Professional-tier include success path (Completeness Contract) | Added `TestCLIIntegration_include_directive_professional_tier` (builds binary, sets `APITEST_TIER=professional`, runs parent+child include, asserts both requests execute and "2 passed") and `TestCLIIntegration_include_directive_free_tier_gated` (exit 6 + "Professional" in stderr) in `cmd/apitest/main_test.go` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage | 89.2% (parser: 88.9%+, all changed packages combined) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 6386a8f | fix(parser): remove hardcoded absolute path in include_step4_test | #1 |
| 60e316b | test(parser): add setup/teardown splicing coverage for include directive | #2 |
| 6458c79 | test(main): add binary-level integration test for include directive | #3 |

## Summary
3/3 findings resolved. 0 deferred.
