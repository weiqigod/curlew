# Verification Report: M2-030

**Task:** GraphQL external query files and fragment support
**Verified by:** AI
**Date:** 2026-04-10
**Branch:** feature/M2-030-graphql-external-files
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/curlew` | PASS | Clean build, no warnings |
| `go test ./...` | PASS | All packages pass |
| `go test -coverprofile=coverage.out ./...` | PASS | See coverage below |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` (M2-030 subtest) | PASS | Manually verified (see notes) |
| Coverage (total) | 89.2% | Meets >= 80% threshold |
| Coverage (`internal/graphql/files`) | 94.8% | `LoadQuery` 96.1%, `extractFragmentSpreads` 94.4%, `topoSortFragments` 92.0%, `resolvePath` 100.0% |

### Smoke test note

The end-to-end `./smoke/run.sh` script exhibits a pre-existing, flaky
`SIGPIPE`-related failure on the `Help text lists tap` check
(`./curlew --help | grep -q "tap"`). This reproduces on `main` as well
(observed ~40% failure rate across repeated runs with the `main` binary)
and is unrelated to M2-030. The failure is caused by `grep -q` closing
stdin after its first match while `curlew` is still writing the rest
of its help output, resulting in a non-zero exit that `set -o pipefail`
propagates.

The M2-030 smoke block (lines 1263-1297 of `smoke/run.sh`) was executed
manually against the branch binary:

```
Collection: GraphQL Files Test
[ERROR] GraphQL protocol support requires Professional tier ($19/month)
```

The collection with `query_file:` and `fragments:` parses successfully
and the execution correctly hits the Professional tier gate, which is
the expected observable behavior for M2-030 (GraphQL execution is
gated; parsing/loading is not).

## Observable Output

```
Collection: GraphQL Files Test
[ERROR] GraphQL protocol support requires Professional tier ($19/month)
```

Expected: `.graphql` query file referenced via `query_file:` plus a
fragment file referenced via `fragments:` parses successfully; running
the collection exercises the new loader; `go test ./internal/graphql/...`
passes.

Result: MATCH. Parsing succeeds with the external query and fragment
files; go tests for `internal/graphql/files` pass.

## Behaviors Verified

| # | Behavior | Test(s) | Status |
|---|----------|---------|--------|
| 1 | `graphql.query_file` loads from external file | `TestLoadQuery/query_file_loads_from_disk`, `TestParseFile_graphql/graphql_query_file_loads_external_query` | PASS |
| 2 | `graphql.fragments` concatenated when query uses `...Name` | `TestLoadQuery/query_file_with_single_fragment_concatenates` | PASS |
| 3 | Multiple fragments resolved in dependency order | `TestLoadQuery/fragment_dependency_ordered_before_dependent`, `TestLoadQuery/transitive_fragment_chain_ordered_A_before_B_before_C`, `TestParseFile_graphql/graphql_fragments_concatenated_in_dependency_order` | PASS |
| 4 | Circular fragment dependency -> validation error | `TestLoadQuery/circular_fragment_dependency_returns_ErrFragmentCycle`, `TestParseFile_graphql/graphql_circular_fragment_dependency_returns_ErrFragmentCycle` | PASS |
| 5 | Missing query file -> clear error with path | `TestLoadQuery/missing_query_file_returns_ErrQueryFileNotFound`, `TestParseFile_graphql/graphql_missing_query_file_returns_clear_error` | PASS |
| 6 | Variable interpolation preserved in loaded query files | `TestLoadQuery/variable_placeholders_preserved_in_loaded_content`, `TestInterpolateRequest_graphql_query_from_file` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` output (all packages OK) | PASS |
| 2 | Observable output works | Manual smoke subtest output shown above | PASS |
| 3 | Test coverage >= 80% | `internal/graphql/files` 94.8%, total 89.2% | PASS |
| 4 | No build warnings or lint errors | `go build` clean, `golangci-lint run` 0 issues | PASS |
| 5 | Help text updated (if user-facing) | N/A - loader is config-driven, no new CLI flags | PASS |
| 6 | Smoke test updated (if new capability) | `smoke/run.sh` lines 1263-1297 added | PASS |

## Code Review

Review `management/reviews/M2-030-review.md` verdict: **PASS**
(iteration 2, all iteration 1 findings resolved).

Spot-checks:

1. Error wrapping — `files.go:LoadQuery` wraps file-read errors with
   `fmt.Errorf("%w: %s: %v", ErrQueryFileNotFound, path, err)` style;
   parser integration wraps with `apierrors.Structured` preserving
   the inner `%w` chain. PASS.
2. Exported symbol doc comments — `LoadQuery`, `LoadQueryInput`,
   `LoadQueryResult`, and the sentinel errors all have doc comments
   including the single-declaration recommendation. PASS.
3. Test quality — `TestLoadQuery/transitive_fragment_chain_ordered_A_before_B_before_C`
   declares A,B,C in reverse input order and asserts dependency-first
   ordering in the concatenated output, which is a meaningful
   structural check rather than "no error". PASS.

| Check | Status |
|-------|--------|
| Error handling (wrap + sentinel) | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality (table-driven, real fixtures) | PASS |
| TDD commit pattern visible | PASS |

## Commits

| Hash | Message |
|------|---------|
| 32e03da | docs(review): add passing review for M2-030 |
| d13c73a | docs(review): add improvement report for M2-030 |
| c2d31d1 | fix(graphql): tighten spread regex and expand files tests |
| bc4a700 | docs(review): add review with findings for M2-030 |
| fa16b79 | chore(task): mark M2-030 as review |
| d5dd53a | docs(changelog): add M2-030 GraphQL external query files entry |
| 6c7e196 | test(requtil): regression guard for variable interpolation in loaded GraphQL queries |
| f86261c | feat(parser): resolve GraphQL query_file and fragments during parse |
| f37c70f | test(parser): add failing tests for GraphQL query_file and fragments |
| c3ca508 | feat(graphql): implement LoadQuery for external query files and fragments |
| e1080ca | test(graphql): add failing tests for query_file and fragment loading |
| 11aa946 | chore(task): mark M2-030 as in_progress |
| 4b8c38f | chore(task): mark M2-030 as planned |
| 0da3ff7 | docs(plan): add implementation plan for M2-030 |

TDD pattern verified: `test(...)` commits precede corresponding `feat(...)`
commits for both the graphql subpackage and parser integration.

## Files Changed

| File | Action |
|------|--------|
| `internal/graphql/files/errors.go` | added |
| `internal/graphql/files/files.go` | added |
| `internal/graphql/files/files_test.go` | added |
| `internal/parser/collection.go` | modified (GraphQLConfig fields) |
| `internal/parser/parser.go` | modified (LoadQuery integration) |
| `internal/parser/parser_test.go` | modified (graphql subtests) |
| `internal/parser/testdata/graphql/queries/get_user.graphql` | added |
| `internal/parser/testdata/graphql/fragments/*.graphql` | added (4 files) |
| `internal/parser/testdata/graphql_*.yaml` | added (5 fixtures) |
| `internal/requtil/requtil_test.go` | modified (regression test) |
| `smoke/run.sh` | modified (M2-030 smoke block) |
| `CHANGELOG.md` | modified |
| `management/...` | plan/review/improved/backlog updates |

## Issues Found

None that affect M2-030.

A pre-existing flaky `SIGPIPE` smoke failure in the `Help text lists tap`
check reproduces on `main` and is unrelated to this task. Recommend
filing a follow-up to either drop `-q` from `grep` or pipe through
`head` to avoid SIGPIPE racing `curlew --help`.

## Recommendation

PASS — ready for PR and merge.
