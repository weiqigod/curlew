# Improvement Report: M3-002 (Iteration 2)

**Task:** Glob pattern discovery for apitest run
**Date:** 2026-04-11
**Review:** management/reviews/M3-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `buildArgsForCollection` at 57.9% coverage — 16 branches untested, no dedicated unit test | Added `TestBuildArgsForCollection` with 18 table-driven cases covering all optional branches: `--env`, `--report`, `--format`, `cliVars`, `envVarVars`, `--seed`, `--no-color`, `-v`, `-vv`, `-q`, `--allow-sensitive`, `--show-dependencies`, `--dry-run`, `--parallel`, `--confirm-large-dataset`, and all flags combined | ✓ `buildArgsForCollection` now 100% |
| 2 | Medium | I/O errors in `discovery.go` propagated bare without `fmt.Errorf("context: %w", err)` wrapping at lines 64, 73, 79, 107, 134 | Wrapped all I/O error return sites: `LoadIgnore` call with `"loading .apitestignore at %s: %w"`, WalkDir `err` with `"accessing %s: %w"`, `filepath.Rel` error with `"computing relative path for %s: %w"`, `walkErr` with `"walking %s: %w"`, `scanner.Err()` with `"reading .apitestignore: %w"`, file open error with `"opening .apitestignore: %w"` | ✓ tests pass |
| 3 | Low | Traversal check misses patterns ending with `/..` (e.g., `"foo/.."` produced `ErrNoMatches` instead of `ErrTraversalOutsideRoot`) | Added `strings.HasSuffix(normalized, "/..")` to the traversal-rejection condition; added two test cases (`"foo/.."` and `"a/b/.."`) to `TestExpand` | ✓ tests pass |
| 4 | Low | `containsGlobMeta` in `main.go` is an exact duplicate of `discovery.IsGlob` | Replaced `containsGlobMeta(p)` call in `expandGlobs` with `discovery.IsGlob(p)` and deleted the `containsGlobMeta` function | ✓ tests pass, lint clean |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (total) | 89.3% |
| Coverage `buildArgsForCollection` | 100% (was 57.9%) |
| Coverage `internal/discovery` | 88.9% |
| Coverage `cmd/apitest` (overall) | 84.3% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 307445b | fix(discovery): wrap I/O errors, fix traversal detection, consolidate IsGlob | #1, #2, #3, #4 |

## Summary

4/4 findings resolved. 0 deferred.
