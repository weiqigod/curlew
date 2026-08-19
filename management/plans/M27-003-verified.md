# Verification Report: M27-003

**Task:** Mudflat Phase 4 - the next dogfooding surface has to be designed
**Verified by:** AI
**Date:** 2026-08-19
**Branch:** feature/M27-003-mudflat-phase-4
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `./scripts/ci-local.sh` (auto-scoped) | PASS | `Gates: go=1 backend=0 web=0 ui=0 e2e=0`; ended `=== ci-local PASS ===`, including the new `dogfood: curlew's report agrees with mudflat's ledger` step (4/4 checks agreed) |
| `./scripts/ci-local.sh --go` (exact task observable) | PASS | Re-run cleanly after a first attempt stalled (see Anomalies below); ended `=== ci-local PASS ===`, ledger step 4/4 agreed |
| `go test ./...` (via ci-local coverage step) | PASS | All packages `ok`, no unexpected `FAIL` (the `FAIL ... is invalid` lines are expected negative-test output from smoke fixtures) |
| `golangci-lint run` | PASS | `0 issues.` |
| `./smoke/run.sh` (embedded in ci-local) | PASS | Ran to completion inside both ci-local runs |
| `go vet` (changed packages) | PASS | Clean, run directly this session |
| `gofmt -l` (changed files) | PASS | No output — all changed Go files already formatted |
| Coverage: `internal/runner` | 84.9% | Meets >= 80% threshold |
| Coverage: `cmd/curlew` | 81.1% | Meets >= 80% threshold |

## Observable Output

Observable 1 — the phase is specified before it is built:

```
$ grep -n "Phase 4 — Stateful sequences and the ledger" docs/TESTAPI_SPECIFICATION.md
2047:### Phase 4 — Stateful sequences and the ledger ✅ implemented

$ grep -n "^## 11D\." docs/TESTAPI_SPECIFICATION.md
1589:## 11D. What Phase 4 Found
```

Observable 2 — it runs as a gate step:

```
$ ./scripts/ci-local.sh --go
...
=== dogfood: curlew's report agrees with mudflat's ledger ===
=== ledger: http://127.0.0.1:18080 (session ledger13717-ledger) ===
  internal consistency: total=6, passed+failed+skipped=6
  internal consistency: total=6, len(requests[])=6
  external agreement: mudflat independently reports 3 resources
  external agreement: attempt_details (3) matches mudflat's $.attempt (3)

=== ledger PASS: all 4 check(s) agreed ===

mudflat: terminated, shutting down

=== ci-local PASS ===
```

Expected: both markers present, and `./scripts/ci-local.sh --go` exits 0 with the ledger step passing.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test(s) | Status |
|---|----------|---------|--------|
| 1 | Given the Phase 4 harness, when it runs against mudflat, then it exercises a surface no existing phase reaches | `testapi/harness/ledger.sh` (checks `summary.total` arithmetic and mudflat-independent counts — no prior phase checked curlew's own report numbers) | PASS |
| 2 | Given a defect it finds, when the defect is fixed, then the reproduction is moved into a passing collection, an inverted expected-failure, or a unit test — never deleted | `internal/runner/summary_invariant_test.go` (`TestSummary_TotalEqualsWhatWasReported`), `cmd/curlew/ledger_summary_test.go` (`TestJSONOutput_SummaryTotalMatchesRequestCount`), `testapi/collections/25-ledger.yaml` + `testapi/harness/ledger.sh` | PASS |
| 3 | Given the harness, when it runs on a clean tree, then it passes, and when a guarded behaviour is broken, then it fails | Manually reproduced this session: reverted the one-line fix in `computeSummary` (`internal/runner/runner.go`), rebuilt, ran `ledger.sh` → 2 `LEDGER MISMATCH` lines, `total=4 but passed(6)+failed(0)+skipped(0)=6`, exit 1. Restored fix, rebuilt, reran → `=== ledger PASS: all 4 check(s) agreed ===`, exit 0. `git diff` on `runner.go` empty after restore | PASS |
| 4 | Given the phase, when it is added to ci-local.sh, then a failure stops the gate rather than printing a warning | `grep -n "ledger.sh" scripts/ci-local.sh` → line 530, inside a script with `set -euo pipefail` at line 28 and no error-suppression around the call | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | Phase 4 specified in docs/TESTAPI_SPECIFICATION.md before implementation | Commit `6963af5 docs(testapi): specify Phase 4 -- stateful sequences and the ledger` precedes `d26da95 test(runner): add failing tests` and `38c9de2 feat(runner): derive summary.total...` in `git log --oneline main..HEAD` | PASS |
| 2 | Harness implemented in testapi/ and wired into ci-local.sh as a failing gate step | `testapi/harness/ledger.sh` exists (executable), `scripts/ci-local.sh:530` invokes it under `set -euo pipefail` | PASS |
| 3 | Every defect found is fixed and its reproduction moved, never deleted | One defect (§11D.1) found; moved into `internal/runner/summary_invariant_test.go`, `cmd/curlew/ledger_summary_test.go`, and the passing `25-ledger.yaml`/`ledger.sh` pair — nothing deleted | PASS |
| 4 | Findings recorded in a new section alongside 11A/11B/11C | `docs/TESTAPI_SPECIFICATION.md` §11D, in the same shape as §11A–§11C, with a promotion table and an affirmative-result subsection | PASS |
| 5 | `./scripts/ci-local.sh` passes | Two independent full runs this session, both ending `=== ci-local PASS ===` (see Test Results) | PASS |
| 6 | CHANGELOG.md updated | `## [Unreleased]` → `### Fixed` entry describing the `computeSummary` fix and citing M27-003 | PASS |
| 7 | Test coverage >= 80% | `internal/runner` 84.9%, `cmd/curlew` 81.1% | PASS |
| 8 | No build warnings | `go build ./cmd/curlew` clean; `go vet` clean; `gofmt -l` empty | PASS |

## Plan Completion

All 7 implementation steps in `management/plans/M27-003-plan.md` were executed in order, matching the commit history:

| Step | Commit(s) | Status |
|---|---|---|
| 1. Specify Phase 4 in the spec | `6963af5` | Complete |
| 2. RED — failing summary-invariant tests | `d26da95`, `8190af8` (deviation note) | Complete |
| 3. GREEN — derive `Total` from results | `38c9de2` | Complete |
| 4. Stateful collection (`25-ledger.yaml`) | `ea353ff` | Complete |
| 5. Ledger harness (`ledger.sh`) | `ea353ff` | Complete |
| 6. Wire into `ci-local.sh` | `bd3322e` | Complete |
| 7. Record findings + CHANGELOG | `dc4b309` | Complete |

Two deviations are documented in the plan's own "Deviations" section (test file placement for the JSON-surface test, moved to `cmd/curlew` to avoid an import cycle; and a table-row correction distinguishing two originally-duplicate test cases) — both are reasoned and consistent with what was actually implemented.

## Code Review

Review file `management/reviews/M27-003-review.md` exists with **Verdict: PASS** and no findings (Branch A). Spot-checked rather than blindly trusted:

| Check | Status | Note |
|-------|--------|------|
| Error handling / doc comments | PASS | The `computeSummary` diff is a minimal, well-commented one-line change (`s.Total = len(results)`) with a doc comment naming the root cause and citing §11D.1 |
| Test file organization | PASS | `TestJSONOutput_SummaryTotalMatchesRequestCount` correctly lives in `cmd/curlew` (not `internal/runner`, which cannot import `buildSummaryJSON` without a cycle) — matches the plan's documented deviation |
| Test quality | PASS | `internal/runner/summary_invariant_test.go` is table-driven with named edge cases (empty collection, setup-only, teardown-after-failure); asserts both halves of the invariant |
| gofmt / go vet | PASS | Independently re-run this session on all changed Go files, clean |

## Commits

| Hash | Message |
|------|---------|
| `319ff4b` | docs(plan): add implementation plan for M27-003 |
| `da1875e` | chore(task): mark M27-003 as planned |
| `688ecea` | chore(task): mark M27-003 as in_progress |
| `6963af5` | docs(testapi): specify Phase 4 -- stateful sequences and the ledger |
| `d26da95` | test(runner): add failing tests for the summary.total invariant |
| `8190af8` | docs(plan): record RED-phase deviations for M27-003 |
| `38c9de2` | feat(runner): derive summary.total from executed results, not declared items |
| `ea353ff` | feat(testapi): the ledger collection and its cross-checking harness |
| `bd3322e` | feat(ci): wire the ledger harness into ci-local.sh as a gate step |
| `dc4b309` | docs(testapi): finalize Phase 4 as implemented, and the CHANGELOG entry |
| `a124f79` | chore(task): mark M27-003 as review |
| `97bce2f` | docs(review): add passing review for M27-003 |

All 12 commits carry `Refs: M27-003`, use conventional commit format, and show the RED-before-GREEN and spec-before-build ordering required by TDD.

## Files Changed

| File | Action |
|------|--------|
| `CHANGELOG.md` | modified |
| `cmd/curlew/ledger_summary_test.go` | created |
| `docs/TESTAPI_SPECIFICATION.md` | modified |
| `internal/runner/runner.go` | modified (+1 line) |
| `internal/runner/summary_invariant_test.go` | created |
| `internal/uiserver/datadriven_test.go` | modified (expectation correction, per §11D.1) |
| `scripts/ci-local.sh` | modified (+ledger gate step) |
| `testapi/README.md` | modified |
| `testapi/collections/25-ledger.yaml` | created |
| `testapi/harness/ledger.sh` | created |
| `management/*` (plan, review, task, backlog) | process files |

## Anomalies Encountered During Verification

- The first attempt at `./scripts/ci-local.sh --go` exceeded the foreground timeout and was auto-backgrounded. After 15+ minutes with no active `go test`/`golangci-lint` process (only a leftover `httpbin_server.py` fixture from an earlier smoke step), it was judged stalled — most likely resource contention from running this alongside the concurrent manual RED/GREEN reproduction (a second `mudflat` instance, `go build` invocations) rather than a defect in the change itself. It was killed, the environment confirmed clean (no orphaned processes, no stray listeners), and the command was re-run cleanly in the foreground, where it completed normally and passed. No code change was needed to resolve this; it was a session-environment issue, not a task defect.

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
