# Improvement Report: M2-030

**Task:** GraphQL external query files and fragment support
**Date:** 2026-04-10
**Review:** management/reviews/M2-030-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|-------------|----------|
| 1 | Low | `fragments/no_decl.graphql` fixture written but never referenced in `TestLoadQuery`. | Removed the dead `writeFile` line; the separate `TestLoadQueryFragmentMissingDeclaration` test already owns its own fixture. | `go test ./internal/graphql/files/...` passes |
| 2 | Low | `resolvePath` absolute-path branch untested (66.7% coverage). | Added subtest `absolute query_file path is honored` that passes an absolute path and uses an empty `BaseDir` to prove the path is used verbatim. `resolvePath` coverage is now 100%. | `go test -coverprofile` shows 100% for `resolvePath` |
| 3 | Low | Topological ordering only tested with a single dependency level. | Added subtest `transitive fragment chain ordered A before B before C` with three fragments in a chain declared in reverse input order, asserting `A` precedes `B` precedes `C` precedes the query. | Test passes and would fail on a pre-order DFS regression |
| 4 | Low | `fragmentSpreadPattern = \.\.\.(\w+)` also matches inline-fragment syntax `... on Type`, silently treating the literal `on` as a fragment name. | Tightened the regex to `\.\.\.\s*([A-Za-z_]\w*)` and added an explicit filter for the keyword `on` in `extractFragmentSpreads`. Added `TestExtractFragmentSpreads` cases for inline-fragment-only and mixed cases, plus an integration subtest `inline fragment syntax does not become a dependency` in `TestLoadQuery`. | Unit + integration tests pass |
| 5 | Low | Doc comment states "only the first declaration's name is recorded" but no test covers a multi-declaration fragment file. | Added `fragments/multi_decl.graphql` fixture and a subtest `multi-declaration fragment file records only first name`. Expanded the `LoadQuery` doc comment to spell out that secondary fragments inside a multi-declaration file cannot be targeted by other files. | Test passes |

## Out of Scope (Deferred)

No findings deferred. All five Low-severity findings were resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| `internal/graphql/files` coverage | 94.8% (was 93.7%) |
| Total project coverage | 89.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| c2d31d1 | `fix(graphql): tighten spread regex and expand files tests` | #1, #2, #3, #4, #5 |

## Summary

5/5 findings resolved. 0 deferred. Package coverage for
`internal/graphql/files` rose from 93.7% to 94.8%, with `resolvePath`
now fully covered. The fragment spread extractor no longer treats
`... on Type` inline-fragment syntax as a dependency on a fragment
literally named `on`, eliminating a latent footgun flagged in the
review.
