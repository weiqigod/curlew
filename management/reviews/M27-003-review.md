# Code Review: M27-003

**Task:** Mudflat Phase 4 - the next dogfooding surface has to be designed
**Reviewer:** AI
**Date:** 2026-08-19
**Branch:** feature/M27-003-mudflat-phase-4
**Iteration:** 1

## Verdict: PASS

## Pre-audit Gate

`./scripts/ci-local.sh --go` was run once, streamed to the terminal, and ran
to completion: `go build`, `go test`, `go test -race`, coverage, lint,
`./smoke/run.sh`, and the full dogfood block including the new
`dogfood: curlew's report agrees with mudflat's ledger` step, which printed:

```
internal consistency: total=6, passed+failed+skipped=6
internal consistency: total=6, len(requests[])=6
external agreement: mudflat independently reports 3 resources
external agreement: attempt_details (3) matches mudflat's $.attempt (3)
=== ledger PASS: all 4 check(s) agreed ===
```

Ended `=== ci-local PASS ===`. Static audit and independent reproduction
proceeded.

## Changed Files

`CHANGELOG.md`, `cmd/curlew/ledger_summary_test.go`,
`docs/TESTAPI_SPECIFICATION.md`, `internal/runner/runner.go`,
`internal/runner/summary_invariant_test.go`,
`internal/uiserver/datadriven_test.go`, `management/backlog.yaml`,
`management/plans/M27-003-plan.md`, `management/tasks/M27-003.yaml`,
`scripts/ci-local.sh`, `testapi/README.md`,
`testapi/collections/25-ledger.yaml`, `testapi/harness/ledger.sh`. All read
in full.

## Independent Verification Performed

**The task's third behavior — "when it runs on a clean tree, then it
passes, and when a guarded behaviour is broken, then it fails" — was
verified by hand, not just read.** Built `curlew` and `mudflat`, started
mudflat on a scratch port, and ran `testapi/harness/ledger.sh` three times:

1. Clean tree: `=== ledger PASS: all 4 check(s) agreed ===`, exit 0.
2. With the one-line fix in `computeSummary` (`internal/runner/runner.go`)
   reverted via `Edit`: two `LEDGER MISMATCH` lines —
   `summary.total=4 but passed(6)+failed(0)+skipped(0)=6` and
   `summary.total=4 but requests[] holds 6 entries` — `=== ledger FAILED: 2
   of 4 check(s) disagreed ===`, exit 1.
3. Fix restored (`git diff --stat internal/runner/runner.go` empty
   afterward, confirming a clean revert), rebuilt, reran: PASS again, exit
   0.

The reverted-state numbers (`total=4`, `passed=6`) match
`docs/TESTAPI_SPECIFICATION.md`'s own §18 Phase 4 claim
(`total=4 but passed(6)+failed(0)+skipped(0)=6`) exactly — the specification's
measurement reproduces independently rather than being an invented
transcript.

Also independently ran, rather than trusted from the gate log alone:

- `go test ./internal/runner/ -run TestSummary_TotalEqualsWhatWasReported -v`
  — 8/8 subtests pass.
- `go test ./cmd/curlew/ -run TestJSONOutput_SummaryTotalMatchesRequestCount -v`
  — pass.
- `go test ./internal/uiserver/ -run TestRun_DataDrivenIterations -v` — pass
  (this is the pre-existing test whose expectation the task corrected from
  `total=1` to `total=3`).
- `go test ./testapi/... -run TestParity -v` — all four parity checks pass;
  the new collection resolves only to already-documented endpoints, adding
  zero new mudflat routes.
- `go test ./cmd/curlew/... -run TestDoc -v` — all doc-register tests pass;
  the new §11D table did not require registration changes.
- `go test ./internal/backlog/ -run TestBacklog_repository_is_consistent -v`
  — 244 tasks, M27-003 correctly shown `OPEN ... (review)`.
- `~/go/bin/golangci-lint run` on the touched packages — 0 issues.
- `go vet` and `gofmt -l` on all touched Go files — clean.
- Coverage: `internal/runner` 84.9% (`computeSummary` itself 84.6%),
  `cmd/curlew` 81.1% — both above the 80% floor.

Working tree left clean; build artifacts (`curlew`, `mudflat`, both
gitignored) removed; the background mudflat process killed before finishing.

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| — | — | — | — | — | No findings. | — |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | The fix is one line (`s.Total = len(results)`) inside the function that already owns the other three counters, so the invariant is now structural rather than two distant sites agreeing. The doc comment names the root cause and cites §11D.1. |
| Input Validation | PASS | No new external input surface. Edge cases (empty collection, setup-only, single-row data-driven, expansion across all three phases, teardown-runs-after-main-fails) are covered by the table-driven test. |
| Naming | PASS | `computeSummary`, `summaryTestItem`, `summaryTestDataDrivenItem` — no stuttering, descriptive at call-site scope, package-appropriate casing. |
| Code Organization | PASS | `TestJSONOutput_SummaryTotalMatchesRequestCount` correctly lives in `cmd/curlew` (not `internal/runner`, which cannot see `buildSummaryJSON`/`buildJSONOutput` without an import cycle) — a deliberate, documented deviation from the plan's draft location, and the right call. `ledger.sh` follows the exact conventions of its three siblings (`set -euo pipefail`, `--url` flag, `mktemp -d` + `trap`, `FAILURES`/`CHECKED` counters, the "zero checks is not success" guard). Zero new mudflat endpoints, so `testapi/parity_test.go` and the specification's §16 anti-bloat rule both stay satisfied without modification. |
| Correctness | PASS | Independently reproduced RED and GREEN by hand (see above) rather than trusting the plan's or the spec doc's transcript; the numbers matched exactly. The four other early-return paths in `runPhases` (setup/main/teardown fatal errors) all call `computeSummary(all, summary)` before returning, so they inherit the corrected `Total` for free — verified by reading each call site. |
| Test Quality | PASS | Table-driven with named edge cases; asserts both halves of the invariant (`Total == Passed+Failed+Skipped` and `Total == len(results)`) rather than one; the JSON-surface test exercises the real `--format json` path via `captureRunCmd`, one level above the internal-package test; the pre-existing `internal/uiserver` test was corrected with a comment explaining why, not silently changed. |

## Test Coverage

- `internal/runner`: 84.9% (package), `computeSummary` 84.6%.
- `cmd/curlew`: 81.1% (package).
- Both measured directly via `go test -coverprofile` in this session, not
  taken from a prior report.

## Spec / Behavior Compliance

All four behaviors in `management/tasks/M27-003.yaml` are covered:

1. "Exercises a surface no existing phase reaches" — the plan's rejected
   alternatives (streaming, adversarial-but-legal, bare concurrency) are
   argued against Phases 1–3's actual coverage, and the ledger's oracle
   (curlew's own report vs. a read-only cross-check against mudflat) has no
   precedent in Phases 1–3, which only ever asked whether a request failed.
2. "Reproduction moved, never deleted" — the one defect found (§11D.1) was
   moved into `internal/runner/summary_invariant_test.go`,
   `cmd/curlew/ledger_summary_test.go`, and a passing collection +
   harness (`25-ledger.yaml` / `ledger.sh`) — belt-and-suspenders across
   two of the three promotion routes the plan lays out.
3. "Passes clean, fails when broken" — verified by hand this session, not
   just asserted (see above).
4. "Wired into ci-local.sh, failure stops the gate" — confirmed by diff
   (inserted after the crosscheck step, before `kill "$mudflat_pid"`) and by
   the script's pre-existing `set -euo pipefail`.

## Summary

A minimal, well-targeted fix (`computeSummary` now derives `Total` from the
result slice it already summarizes, one line) paired with thorough,
independently-reproducible verification at every level: a table-driven unit
test, a `--format json` surface test, a corrected pre-existing test with an
explanation rather than a silent change, a new dogfooding collection and
harness that add zero new endpoints, and a specification section whose
measured numbers I reproduced by hand rather than took on faith. No findings.
