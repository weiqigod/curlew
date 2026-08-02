# Verification Report: M3-003

**Task:** include directive: compose collections with snapshot variable scoping
**Verified by:** AI
**Date:** 2026-04-14
**Branch:** feature/M3-003-include-directive
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/curlew` | PASS | Clean build, no warnings |
| `go test ./...` | PASS | All 26 packages pass |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS* | 1 pre-existing TAP-help failure on `main` too; not introduced by M3-003 |
| Coverage | 89.2% | Meets >= 80% threshold (parser: 88.9%, auth: 88.8%, watch: 89.7%) |

\* `FAIL: --help missing tap in --format description` exists on `main` before this branch; confirmed not introduced by M3-003.

## Observable Output

```
Collection: Parent Collection
  ✓ Auth Request  200  707ms
  ✓ Common Request  200  705ms

────────────────────────────────
  2 request(s): 2 passed, 0 failed (1412ms)
```

Professional tier: 2 included requests execute with parent's `base_url` interpolated — MATCH

```
[ERROR] include: directive requires Professional tier ($19/month)
Exit code: 6
```

Free tier: exit code 6 with `include_directive` gate — MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | setup/requests/teardown items appended in include order | `TestResolveIncludes/single_include_splices_child_requests_in_order`, `TestResolveIncludes_setup_and_teardown_spliced` | PASS |
| 2 | child local override wins for child requests (b==99) | `TestResolveIncludes/child_variable_overrides_parent_for_child_request_only` | PASS |
| 3 | child changes do NOT leak back to parent scope | `TestResolveIncludes_parent_requests_unchanged_by_child_overrides` | PASS |
| 4 | child resolves base_url from parent snapshot | `TestResolveIncludes/child_with_no_vars_resolves_base_url_from_parent_snapshot` | PASS |
| 5 | transitive (grandchild) includes resolved recursively | `TestResolveIncludes/transitive_include_resolves_with_cumulative_overrides`, `TestResolveIncludes_grandchild_sees_child_overrides` | PASS |
| 6 | circular include returns structured error naming both files | `TestResolveIncludes/circular_include_detected` | PASS |
| 7 | relative path resolved from including file's directory | `TestResolveIncludes/include_relative_path_resolved_from_including_file`, `TestParseFile_include_path_relative_to_parent_not_cwd` | PASS |
| 8 | missing include file returns structured error with line number | `TestResolveIncludes/include_file_not_found` | PASS |
| 9 | Free/Solo tier blocked with exit code 6 and include_directive gate | `TestParseFile_include_gate_blocks_at_free_tier`, `TestCLIIntegration_include_directive_free_tier_gated` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — all 26 packages PASS | PASS |
| 2 | Observable output works as specified | Professional tier: 2 requests execute with `base_url` resolved; Free tier: exit 6 | PASS |
| 3 | Test coverage >= 80% | 89.2% total; parser 88.9%, auth 88.8%, watch 89.7% | PASS |
| 4 | No build warnings or lint errors | `go build` clean; `golangci-lint run` 0 issues | PASS |
| 5 | Help text updated (if user-facing) | No new user-facing commands added; include is a YAML directive | N/A |
| 6 | Smoke test updated (if new capability) | Smoke test added for include Professional tier and Free tier gate | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — `apierrors.Structured` used consistently; `ErrCircularInclude`/`ErrIncludeNotFound` sentinel errors defined; no panics |
| Naming conventions | PASS — no stuttering; all exported symbols have doc comments; unexported helpers clearly named |
| Code organization | PASS — `include.go` self-contained; dependency inversion via `ParseOptions.IncludeGate` callback keeps parser free of auth import |
| Test quality | PASS — table-driven tests; TDD pattern visible in commits; binary-level integration tests added |
| Correctness | PASS — snapshot scoping model correct; `cloneVisited` prevents sibling mutation; child vars do not leak to parent |

Branch A: Review PASS trusted (verdict in `management/reviews/M3-003-review.md`), spot-check clean.

## Commits

| Hash | Message |
|------|---------|
| 20333d9 | docs(review): add passing review for M3-003 |
| 56da72b | docs(review): add improvement report for M3-003 |
| 6458c79 | test(main): add binary-level integration test for include directive |
| 60e316b | test(parser): add setup/teardown splicing coverage for include directive |
| 6386a8f | fix(parser): remove hardcoded absolute path in include_step4_test |
| 4740b1e | docs(review): add review with findings for M3-003 |
| dc28798 | chore(task): mark M3-003 as review |
| c364d5a | refactor(parser): fix gofumpt alignment in include_test.go struct literals |
| 28c0426 | feat(smoke): add include directive smoke test case and CHANGELOG entry |
| ee319d1 | feat(schema): add include property to collection JSON schema |
| f70c04b | feat(cli): wire IncludeGate from CLI and watch into ParseFileWithOptions |
| 06f02a8 | feat(auth): register include_directive as Professional tier feature |
| e1166a0 | test(parser): add include resolution integration tests with fixtures |
| f103fa4 | feat(parser): add ParseOptions/ParseFileWithOptions and include resolution |
| a2a9e7d | test(parser): add failing tests for include sentinels and Collection.Include field |
| d4422ea | chore(task): mark M3-003 as in_progress |
| 6a7e512 | chore(task): mark M3-003 as planned |
| 8aaec4c | docs(plan): add implementation plan for M3-003 |

## Files Changed

| File | Action |
|------|--------|
| `internal/parser/include.go` | added — include resolution implementation |
| `internal/parser/errors.go` | modified — ErrCircularInclude, ErrIncludeNotFound sentinels |
| `internal/parser/collection.go` | modified — Include []string field added |
| `internal/parser/parser.go` | modified — ParseFileWithOptions, ParseOptions |
| `internal/parser/include_test.go` | added — behavior tests |
| `internal/parser/include_step1_test.go` | added — TDD step 1 tests |
| `internal/parser/include_step2_test.go` | added — TDD step 2 tests |
| `internal/parser/include_step4_test.go` | added — TDD step 4 tests |
| `internal/parser/testdata/include/` | added — test fixtures (22 files) |
| `internal/auth/registry.go` | modified — include_directive Professional tier registration |
| `internal/auth/registry_test.go` | modified — test coverage |
| `internal/schema/collection.json` | modified — include property added |
| `internal/schema/schema_test.go` | modified — schema test coverage |
| `internal/watch/paths.go` | modified — include paths tracked |
| `internal/watch/watch.go` | modified — IncludeGate wired |
| `cmd/curlew/main.go` | modified — IncludeGate wired from CLI |
| `cmd/curlew/main_test.go` | modified — binary-level integration tests |
| `smoke/run.sh` | modified — include smoke test cases added |
| `CHANGELOG.md` | modified — M3-003 entry added |

## Issues Found
None. All findings from review iterations were resolved.

## Recommendation
PASS — ready for PR and merge.
