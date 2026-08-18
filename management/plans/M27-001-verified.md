# Verification Report: M27-001

**Task:** The prose register - checkable claims that are sentences, not rows
**Verified by:** AI
**Date:** 2026-08-18
**Branch:** feature/M27-001-prose-register
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/curlew` | PASS | Clean build, no warnings |
| `go test ./...` | PASS | All packages ok (`cmd/curlew` 101.2s incl. all subpackages) |
| `go test -race ./...` | PASS | No races detected across all packages |
| `golangci-lint run` | PASS | `0 issues.` |
| `./smoke/run.sh` | PASS | Full smoke suite clean, ends `=== Smoke Test Complete ===` |
| `./scripts/ci-local.sh` | PASS | Go-only scope auto-detected (no backend/web/E2E changes); ends `=== ci-local PASS ===`, including the testapi dogfood/gaps/redaction/openapi/crosscheck harnesses |
| Coverage (`internal/docs`) | 81.1% | Meets >= 80% threshold |

## Observable Output

```
$ go test ./internal/docs/ -run TestProse_inventory_is_complete -v
=== RUN   TestProse_inventory_is_complete
    prose_test.go:580: 103 prose claims: 14 executed, 3 exempt (cap 12), 86 owed an executor (.../docs/prose-claim-baseline.txt)
--- PASS: TestProse_inventory_is_complete (0.10s)
PASS

$ go test ./internal/docs/ -run TestProse_register_cannot_grow -v
=== RUN   TestProse_register_cannot_grow
    (7 subtests: clean x2, MUTATION new-debt, MUTATION stale-debt, MUTATION both-at-once,
     exempt-is-neither, unresolved-executor)
--- PASS: TestProse_register_cannot_grow (0.00s)
PASS

$ cat docs/prose-claim-baseline.txt
# Checkable prose claims still owed an executor. Started at 86 of 103.
... (127 lines: header + 86 tab-separated data lines)
```

Expected: task's `observable` field, verbatim — inventory-completeness test, register-growth test,
and the baseline file's stated count.
Result: MATCH. 103 total claims (MANUAL.md 70 + CLI_SPECIFICATION.md 33), 14 executed, 3 exempt
(cap 12), 86 owed — header states "Started at 86 of 103", and `grep -c $'\t'` on the data lines
confirms 86.

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Every checkable claim is either executed by a test or listed in the baseline register | `TestProse_inventory_is_complete` | PASS |
| 2 | An executed claim still listed in the register fails the build (stale debt) | `TestProse_register_cannot_grow/MUTATION_stale_debt:_executed_claim_still_registered` | PASS |
| 3 | A new checkable claim with no executor and no register line fails the build (new debt) | `TestProse_register_cannot_grow/MUTATION_new_debt:_unexecuted_claim_absent_from_register` | PASS |
| 4 | A marked claim (`<!-- doc-check: prose-not-executable <why> -->`) is exempt, counted against a cap | `TestExtractProse/marker_exempts_every_claim_in_next_block`, `TestExtractProse/marker_is_spent_by_the_following_block`, `TestProse_register_cannot_grow/exempt_claim_is_neither_new_nor_stale`, cap check inline in `TestProse_inventory_is_complete` (`103 claims: ... 3 exempt (cap 12)`) | PASS |
| 5 | Zero claims extracted fails rather than reads as a clean register | Inline `t.Fatal` guards in `TestProse_inventory_is_complete` (`prose_test.go:526,534`) plus every `want: nil` case in `TestExtractProse` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | Checkable prose claims extracted from both CLI documents, source-derived | `internal/docs/prose.go` (`ExtractProse`), 103 claims (MANUAL.md 70, CLI_SPECIFICATION.md 33) | PASS |
| 2 | `docs/prose-claim-baseline.txt` created with the initial debt, count stated | File exists, header states "Started at 86 of 103", 86 data lines confirmed by `grep -c` | PASS |
| 3 | New debt, stale debt, and zero-claim extraction each fail the build | `TestProse_register_cannot_grow` (new/stale via mutation) + inline zero-claim guards in `TestProse_inventory_is_complete` | PASS |
| 4 | Marker exemption implemented with a cap and a stated reason per marker | `ProseMarker` const, cap = 12, enforced in `TestProse_inventory_is_complete` (`3 exempt (cap 12)`) | PASS |
| 5 | Guards verified by mutation in all three directions | Review's re-audit mutated the *real* baseline/docs (not just synthetic fixtures) in all three directions and reverted cleanly each time (see `management/reviews/M27-001-review.md`) | PASS |
| 6 | Extractor's precision measured and reported | Plan's "Measured extraction experiment" section (~92% precision on prototype); Deviations section reconciles against the real 103-claim run | PASS |
| 7 | `./scripts/ci-local.sh --go` passes | Full `./scripts/ci-local.sh` (superset) run this session, ends `=== ci-local PASS ===` | PASS |
| 8 | CHANGELOG.md updated | `CHANGELOG.md` under `## [Unreleased]` / `### Added`, "A debt register for checkable claims stated in sentences, not rows" | PASS |

## Code Review

Branch A: `management/reviews/M27-001-review.md` exists with **Verdict: PASS** (iteration 2, after
`/improve` resolved the single Medium finding from iteration 1 — a self-inconsistent
`table-execution-baseline.txt` path in `CHANGELOG.md:219`, fixed in `76393ba`).

Spot-checks performed this pass (not merely re-reading the review):

| Check | Result |
|-------|--------|
| Error wrapping (`internal/docs/proseclaims.go`) | `fmt.Errorf("parsing %s: %w", path, parseErr)` present; `Prose`'s no-match/ambiguous-match errors are constructed, not silently swallowed |
| Doc comments on exported symbols (`internal/docs/prose.go`) | No missing doc comments found via automated scan of every `func`/`type`/`const` beginning with an uppercase letter |
| Random test exercises what it claims (`TestExtractProse`) | Table-driven with exact expected-output assertions (not just error-nil checks) — e.g. "hard-wrapped paragraph is reflowed" asserts the joined single-line string, "semicolon splits two claims" asserts both resulting claim texts |

Spot-check clean; Branch A trust upheld, no escalation to Branch B needed.

## Commits

| Hash | Message |
|------|---------|
| `4700ef2` | docs(plan): add implementation plan for M27-001 |
| `e866fba` | chore(task): mark M27-001 as planned |
| `549861c` | test(docs): add failing tests for prose claim extraction |
| `ebc8f4f` | feat(docs): implement prose claim extraction core |
| `40d7f2d` | chore(task): mark M27-001 as in_progress |
| `1c9dc61` | test(docs): add failing tests for source-derived prose executors |
| `4656479` | feat(docs): implement source-derived prose executor claims |
| `707fd99` | test(docs): add failing tests for the prose debt register audit |
| `9f8d243` | feat(docs): implement the prose debt register audit |
| `23017fb` | feat(docs): mark narrative prose claims, register the debt, pay off a tranche |
| `5f3d306` | docs(changelog): document the prose claim register; note plan deviations |
| `840afa7` | test(docs): close coverage gaps to 100% on every prose.go function |
| `0c4b744` | chore(task): mark M27-001 as review |
| `77949aa` | docs(review): add review with findings for M27-001 |
| `76393ba` | fix(docs): correct table-execution-baseline.txt path in CHANGELOG |
| `16ee8a7` | docs(review): add improvement report for M27-001 |
| `1ec881e` | docs(review): add passing review for M27-001 |

All 17 commits carry `Refs: M27-001`. TDD pattern visible throughout: each `test(...)` commit
precedes its corresponding `feat(...)` commit (RED before GREEN).

## Files Changed

| File | Action | Lines +/- |
|------|--------|-----------|
| `internal/docs/prose.go` | created | +501 |
| `internal/docs/prose_test.go` | created | +582 |
| `internal/docs/proseclaims.go` | created | +124 |
| `docs/prose-claim-baseline.txt` | created | +127 |
| `docs/CLI_SPECIFICATION.md` | modified | +6 |
| `CHANGELOG.md` | modified | +57 |
| `cmd/curlew/main_test.go` | modified | +6 |
| `cmd/curlew/markdown_correlation_test.go` | modified | +14 |
| `cmd/curlew/redaction_response_test.go` | modified | +6 |
| `cmd/curlew/run_test.go` | modified | +6 |
| `cmd/curlew/stream_discipline_matrix_test.go` | modified | +17 |
| `cmd/curlew/ui_test.go` | modified | +6 |
| `internal/variable/doc_functions_test.go` | modified | +9 |
| `internal/variable/dynamic_test.go` | modified | +5 |
| `management/backlog.yaml` | modified | +3/-1 |
| `management/tasks/M27-001.yaml` | modified | +1/-1 |
| `management/plans/M27-001-plan.md` | created | +554 |
| `management/plans/M27-001-improved.md` | created | +37 |
| `management/reviews/M27-001-review.md` | created | +145 |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
