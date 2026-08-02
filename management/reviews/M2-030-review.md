# Code Review: M2-030 (Iteration 2)

**Task:** GraphQL external query files and fragment support
**Reviewer:** AI
**Date:** 2026-04-10
**Branch:** feature/M2-030-graphql-external-files
**Iteration:** 2 (re-review after `/improve`)

## Verdict: PASS

All five Low-severity findings from iteration 1 have been resolved. No new
findings. Tests pass, lint is clean, coverage increased.

## Iteration 1 Findings Verification

| # | Severity | Finding | Status | Evidence |
|---|----------|---------|--------|----------|
| 1 | Low | Dead `fragments/no_decl.graphql` fixture in `TestLoadQuery` | Resolved | Removed from `files_test.go`; the separate `TestLoadQueryFragmentMissingDeclaration` test owns its own tempdir fixture |
| 2 | Low | `resolvePath` absolute-path branch untested | Resolved | New subtest `absolute query_file path is honored` (files_test.go:115-125) passes an absolute path with empty `BaseDir`; coverage now 100% |
| 3 | Low | Topological ordering only tested with one dependency level | Resolved | New subtest `transitive fragment chain ordered A before B before C` (files_test.go:99-114) declares A,B,C in reverse order and asserts dependency-first ordering |
| 4 | Low | `fragmentSpreadPattern` captures `on` from inline-fragment syntax | Resolved | Regex tightened to `\.\.\.\s*([A-Za-z_]\w*)` plus explicit `on` filter (files.go:23, files.go:175); new unit cases in `TestExtractFragmentSpreads` and integration subtest `inline fragment syntax does not become a dependency` |
| 5 | Low | Multi-declaration fragment file behavior undocumented by tests | Resolved | New fixture `fragments/multi_decl.graphql` and subtest `multi-declaration fragment file records only first name`; `LoadQuery` doc comment (files.go:72-76) now spells out the single-declaration recommendation |

## Findings (iteration 2)

None.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; sentinel errors defined and asserted via `errors.Is`; parser wraps `LoadQuery` failures in `apierrors.Structured` preserving the inner error. No panics on expected failures. |
| Input Validation | PASS | Nil `GraphQLConfig` guarded (`if gql == nil { continue }`); empty query/query_file surfaces `ErrMissingQuery`; mutually-exclusive combinations produce `ErrQueryMutuallyExclusive`; missing files surface sentinel errors carrying the original user-supplied path; duplicate fragment names rejected with `ErrDuplicateFragment`. |
| Naming | PASS | `files.LoadQuery`, `LoadQueryInput`, `LoadQueryResult` follow Effective Go with no stuttering. Sentinel errors use the `ErrXxx` convention. Doc comments present on every exported symbol. Unexported helpers `loadedFragment`, `extractFragmentSpreads`, `topoSortFragments`, `resolvePath` clearly named. |
| Code Organization | PASS | `internal/graphql/files` subpackage isolates external-file loading from the `internal/graphql` adapter, avoiding an import cycle when the parser consumes it. Parser integration is a single block placed after external request resolution and before GraphQL validation. No unused imports, no circular dependencies. |
| Correctness | PASS | DFS with three-color marking is the standard topological-sort-with-cycle-detection formulation. Cycle message rebuilds the chain via a fresh slice (`append([]string(nil), stack...)`) to avoid aliasing. Recursive `visit` passes `stack` by value so parallel children do not see each other's appends beyond their own frame. Deduplication of spread references prevents redundant walks. Self-references excluded. Unknown spread targets tolerated. Parser mutates the shared `GraphQLConfig` through `gql *GraphQLConfig`, so downstream validation sees the populated query. |
| Test Quality | PASS | Table-driven tests with descriptive `t.Run` names; `errors.Is` for sentinel assertions; real `testdata/` fixtures for parser integration; smoke test exercises the real binary. Transitive ordering, absolute-path branch, inline-fragment syntax, multi-declaration files, and duplicate fragment names all covered. Missing-path error message verified via substring. |

## Test Coverage

- `internal/graphql/files`: 94.8% of statements (was 93.7% in iteration 1)
  - `LoadQuery`: 96.1%
  - `extractFragmentSpreads`: 94.4%
  - `topoSortFragments`: 92.0%
  - `resolvePath`: 100.0% (was 66.7%)
- `internal/parser` graphql subtests cover all six behaviors from the task YAML, plus `ExternalFiles` count and error-message substring checks.
- `internal/requtil` regression test `TestInterpolateRequest_graphql_query_from_file` guards variable interpolation on loaded query content.

## Behavior-to-Test Matrix

| Behavior | Test |
|----------|------|
| `graphql.query_file` loads from external file | `TestLoadQuery/query_file loads from disk`, `TestParseFile_graphql/graphql query_file loads external query` |
| `graphql.fragments` concatenated when query uses `...Name` | `TestLoadQuery/query_file with single fragment concatenates` |
| Multiple fragments resolved in dependency order | `TestLoadQuery/fragment dependency ordered before dependent`, `TestLoadQuery/transitive fragment chain ordered A before B before C`, `TestParseFile_graphql/graphql fragments concatenated in dependency order` |
| Circular fragment dependency -> validation error | `TestLoadQuery/circular fragment dependency returns ErrFragmentCycle`, `TestParseFile_graphql/graphql circular fragment dependency returns ErrFragmentCycle` |
| Missing query file -> clear error with path | `TestLoadQuery/missing query_file returns ErrQueryFileNotFound`, `TestParseFile_graphql/graphql missing query_file returns clear error` (asserts substring) |
| Variable interpolation in loaded query files | `TestLoadQuery/variable placeholders preserved in loaded content` + `TestInterpolateRequest_graphql_query_from_file` |

All six behaviors have dedicated test coverage. Extra behaviors (mutual
exclusion, duplicate fragment detection, inline-fragment syntax filtering,
multi-declaration file handling, absolute path support, transitive chain
ordering) are also tested.

## Quality Gate Verification

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` (race) | PASS |
| `golangci-lint run` (affected packages) | PASS (0 issues) |
| Coverage >= 80% | PASS (94.8% for `internal/graphql/files`) |
| M2-030 smoke subtest manually verified | PASS (parses successfully, then hits Professional tier gate as expected) |

Note: the end-to-end `./smoke/run.sh` script aborts before reaching the
M2-030 subtest on an unrelated pre-existing failure in the TAP help-text
check (`FAIL: --help missing tap in --format description`). That failure
reproduces on `main` and is not introduced by M2-030. The M2-030 smoke
block (lines 1263-1296 of `smoke/run.sh`) was exercised manually against
the built binary and passes.

## Summary

Iteration 2 cleanly resolves all five Low-severity findings from iteration 1.
Coverage for the new package rose from 93.7% to 94.8% with `resolvePath` now
fully covered. The fragment spread extractor no longer treats inline-fragment
`... on Type` syntax as a dependency on a fragment named `on`, and the
topological sort is now exercised by a transitive three-level chain that
would catch a DFS post-order vs pre-order regression. Code is ready for
verification.
