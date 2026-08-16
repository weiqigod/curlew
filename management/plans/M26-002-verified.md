# Verification Report: M26-002

**Task:** One exit-code truth across binary, specification, manual and skill
**Verified by:** AI
**Date:** 2026-08-16
**Branch:** feature/M26-002-one-exit-code-truth
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/curlew` | PASS | Clean build, no warnings |
| `go test ./...` | PASS | All packages `ok`, 0 failures |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Full smoke suite clean, incl. dogfood/gaps/redaction/openapi/crosscheck harnesses |
| `./scripts/ci-local.sh` | PASS | Auto-scoped `go=1 backend=0 web=0 ui=0 e2e=0` (diff vs main touches only Go + docs + management files); ended `=== ci-local PASS ===` |
| Coverage | 86.2% total | `cmd/curlew` 81.1%, `internal/exitcodes` 87.4% — both above the 80% gate |

## Observable Output

```
$ go test ./cmd/curlew/ -run TestExitCodes_all_surfaces_agree -v
--- PASS: TestExitCodes_all_surfaces_agree (0.01s)
    --- PASS: .../docs-MANUAL.md_under_4.3_Exit_codes (0.00s)
    --- PASS: .../docs-MANUAL.md_under_D._Exit_codes (0.00s)
    --- PASS: .../docs-CLI_SPECIFICATION.md_under_17._Exit_Codes (0.00s)
    --- PASS: .../the_scaffolded_agent_skill (0.00s)
    --- PASS: .../docs-UI_SPECIFICATION.md_under_2.4_Exit_codes (0.00s)
    --- PASS: .../docs-plugins.md_under_Exit_codes_for (0.00s)
PASS

$ go test ./cmd/curlew/ -run TestExitCodes_no_unreachable_mapping -v
--- PASS: TestExitCodes_no_unreachable_mapping (0.00s)
PASS
```

Expected: four full-contract surfaces (spec, manual x2, skill) plus two
per-command surfaces (UI spec, plugins) all agree with the source-derived
reachable set `{0,1,2,3,4,5,130}`, and no branch in `exitCodeSeverity` maps a
code the binary cannot return.
Result: MATCH.

## Behaviors Verified

| # | Behavior | Test(s) | Status |
|---|----------|---------|--------|
| 1 | Exit codes vs `docs/CLI_SPECIFICATION.md` table are equal | `TestExitCodes_all_surfaces_agree/docs-CLI_SPECIFICATION.md_under_17._Exit_Codes` | PASS |
| 2 | Same set vs `docs/MANUAL.md` and the shipped agent skill, all four surfaces agree | `TestExitCodes_all_surfaces_agree` (full, 6 subtests) | PASS |
| 3 | `exitCodeSeverity` cases enumerated, none maps an unreachable code | `TestExitCodes_no_unreachable_mapping` | PASS |
| 4 | A new exit code anywhere in `cmd/curlew` fails the suite until every surface documents it | `TestExitCodes_all_surfaces_agree` — independently reproduced live (see below) | PASS |

Behavior 4 has no static always-red assertion (the plan records all four named
surfaces as green-on-arrival — G2 in the plan), so it can only be demonstrated
by mutation. Rather than trust the review's transcript alone, I independently
reproduced plan mutation M1 in this session: inserted a statically-reachable,
dynamically-unreachable `case flags.vus < 0: return 7` into
`cmd/curlew/perf.go`'s final switch (guarded by a `trap 'git checkout --
cmd/curlew/perf.go' EXIT INT TERM`), ran the full parity suite, and confirmed:

```
--- FAIL: TestExitCodes_all_surfaces_agree
    --- FAIL: .../docs-MANUAL.md_under_4.3_Exit_codes
    --- FAIL: .../docs-MANUAL.md_under_D._Exit_codes
    --- FAIL: .../docs-CLI_SPECIFICATION.md_under_17._Exit_Codes
    --- FAIL: .../the_scaffolded_agent_skill
    --- PASS: .../docs-UI_SPECIFICATION.md_under_2.4_Exit_codes   (per-command: correct by omission)
    --- PASS: .../docs-plugins.md_under_Exit_codes_for            (per-command: correct by omission)
```

All four full-contract surfaces failed, citing `perf.go:142` by file and line;
both per-command (containment-only) surfaces correctly stayed green. The
mutation was then reverted; `git status --porcelain` was empty afterward, and
the full `TestExitCodes_` suite (5 functions) was re-run green.

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | Reachable exit-code set derived from source, not hand-listed | `internal/exitcodes.Reachable` — exactly one call site in the repo (`cmd/curlew/skill_exit_codes_test.go:102`, wrapped by `reachableExitCodes(t)`), reused rather than re-implemented | PASS |
| 2 | Specification, manual and skill each asserted equal to it | `TestExitCodes_all_surfaces_agree`, both directions, all 4 full-contract subtests green | PASS |
| 3 | Unreachable code-6 mapping in `discovery_run.go` removed | `grep -n "6: 6\|feature gate"` in `discovery_run.go` returns nothing; both ordering comments read `0 < 4 < 2 < 5 < 3 < 1` (no trailing `< 6`); two dead test cases (`discovery_run_test.go`) deleted, not re-baselined | PASS |
| 4 | Guards verified by mutation on each of the four surfaces | Review documents 9 independent mutations (M3b central case: three docs agreeing with each other while all wrong, caught only by the binary-anchored check); this session independently reproduced the M1 mutation live | PASS |
| 5 | `./scripts/ci-local.sh --go` passes | Ran full `./scripts/ci-local.sh`; auto-scope resolved to `go=1` only (diff touches no backend/web/stack files) — ended `=== ci-local PASS ===` | PASS |
| 6 | `CHANGELOG.md` updated | `## [Unreleased] / ### Fixed` carries a full M26-002 entry (lines 76-156): the deleted `6:6` mapping, the `UI_SPECIFICATION.md` exit-9 residue, that all four named surfaces were green on arrival, 130's deliberate reachable-but-unranked status, and `SPECIFICATION.md`'s platform-scoped exclusion | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Review verdict on file | PASS, no findings at any severity (`management/reviews/M26-002-review.md`, commit `a1cc12a`) |
| Spot-check: error handling | PASS — `internal/exitcodes/reachable.go`'s `Reachable`/`parseDir` wrap with `fmt.Errorf("...: %w", ...)`; sentinel errors `ErrRootNotFound`, `ErrNoSources` |
| Spot-check: exported symbols have doc comments | PASS — `Code`, `ErrRootNotFound`, `ErrNoSources`, `Reachable`, `Set`, `Find`, `MaxDepth` all documented |
| Spot-check: a test verifies what it claims | PASS — `TestExitCodes_no_unreachable_mapping` read in full and independently exercised via mutation; matches its own doc comment exactly |

Branch A (review PASS trusted): spot-check clean, no escalation to a full
Branch B review needed. The existing review additionally re-ran 9 independent
mutations of its own (`management/reviews/M26-002-review.md`), including the
central M3b case (dropping the real `130` row from all three doc tables at
once — pairwise agreement between the docs passes while the binary-anchored
check still fails on each, independently, by file and line).

## Commits

| Hash | Message |
|------|---------|
| `068160e` | docs(plan): add implementation plan for M26-002 |
| `86dc03d` | chore(task): mark M26-002 as planned |
| `c3ff853` | docs(plan): reconcile plan with a third UI_SPECIFICATION.md grace-check claim |
| `0042767` | chore(task): mark M26-002 as in_progress |
| `066019a` | test(cli): add failing test for unreachable exit-code severity mapping (RED) |
| `af26211` | fix(cli): delete the unreachable exit-code-6 severity mapping (GREEN) |
| `cbf0d8c` | test(cli): hold every exit-code surface to the source-derived reachable set |
| `1f70351` | test(cli): hold the skill's topic-file prose to the reachable exit-code set |
| `726a112` | test(cli): fail the build on an exit-code section no surface registry reads (RED) |
| `2948413` | fix(docs): remove UI_SPECIFICATION.md's exit 9 for a deleted grace check (GREEN) |
| `355f93f` | test(cli): hold CLI_SPECIFICATION.md's per-command exit-code prose to reachable |
| `fe2634c` | docs(changelog): record the exit-code parity fix for M26-002 |
| `6b33d2b` | chore(task): mark M26-002 as review |
| `a1cc12a` | docs(review): add passing review for M26-002 |

All 14 commits reference `Refs: M26-002`, use conventional `type(scope):
description` format, and the RED-then-GREEN TDD pairing is visible twice
(`066019a`→`af26211`, `726a112`→`2948413`).

## Files Changed

| File | Action | Lines +/- |
|------|--------|-----------|
| `CHANGELOG.md` | modified | +82 |
| `cmd/curlew/discovery_run.go` | modified | +9/-5 |
| `cmd/curlew/discovery_run_test.go` | modified | -2 |
| `cmd/curlew/doc_exit_codes_test.go` | modified | +21/-12 |
| `cmd/curlew/exit_codes_parity_test.go` | created | +484 |
| `docs/UI_SPECIFICATION.md` | modified | +8/-8 |
| `internal/exitcodes/reachable.go` | modified | +9/-7 |
| `internal/exitcodes/testdata/mapliteral/main.go` | modified | +8/-6 |
| `management/backlog.yaml` | modified | +3/-3 |
| `management/plans/M26-002-plan.md` | created | +1201 |
| `management/reviews/M26-002-review.md` | created | +160 |
| `management/tasks/M26-002.yaml` | modified | +3/-3 |

12 files changed, 1993 insertions(+), 41 deletions(-).

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.

This task unblocks **M25-002** (cut v0.1.0), deliberately sequenced behind it
so a release archive is never the moment the shipped skill and the shipped
binary are discovered to disagree about exit codes.
