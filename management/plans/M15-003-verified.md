# Verification Report: M15-003

**Task:** Retier from_command from Solo to Free across registry, tests, and MANUAL.md
**Verified by:** AI
**Date:** 2026-05-07
**Branch:** feature/M15-003-from-command-free-tier
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass, 0 failures |
| `go test -race ./...` | PASS | No races detected (ci-local.sh) |
| `golangci-lint run` | PASS | No findings (ci-local.sh) |
| `./smoke/run.sh` | PASS | Smoke test clean including new from_command Free-tier case |
| Coverage `internal/auth` | 89.6% | Meets >= 80% threshold |
| Coverage `internal/variable` | 97.4% | Meets >= 80% threshold |
| Coverage `internal/runner` | 84.8% | Meets >= 80% threshold |

## Observable Output

```
Collection: m15-003-from-command
  ✓ echo-back  200  506ms

────────────────────────────────
  1 request(s): 1 passed, 0 failed (506ms)
exit=0
```

Expected: request runs, status code printed, no gate error; exit 0.
Result: MATCH

```
grep -n "from_command.*\[Solo\]\|from_command.*Solo tier\|| \`from_command\` | Solo" docs/MANUAL.md
# (no output)
```

Expected: no matches.
Result: MATCH — zero `[Solo]` adjacency for `from_command` in MANUAL.md

## Behaviors Verified

| # | Behavior | Test(s) | Status |
|---|----------|---------|--------|
| 1 | `from_command` entry has `RequiredTier=TierFree` with refreshed Description/Workaround | `TestDefaultRegistry_FromCommand` | PASS |
| 2 | Free-tier license: command shells out, variable resolves, no GateError | `TestRunFromCommand/from_command_at_free_tier_resolves`, `TestRunFromCommand_at_free_tier_no_longer_gated` | PASS |
| 3 | Solo/Professional/Team/Enterprise: behavior unchanged (still allowed) | `TestDefaultRegistry_FromCommand_TierMatrix` (all five tiers), `TestRunFromCommand/from_command_resolves_at_solo_tier` | PASS |
| 4 | `internal/variable/command.go`: no implementation changes required | `command.go` unchanged (verified by git diff) | PASS |
| 5 | `docs/MANUAL.md`: all `[Solo]` references for `from_command` updated | Zero matches on grep observable | PASS |
| 6 | TOC slug consistent with section heading | TOC `#63-from_command-variables` matches heading `### 6.3 \`from_command\` variables` | PASS |
| 7 | `registry_test.go` tier-matrix asserts `from_command` allowed at all five tiers | `TestDefaultRegistry_FromCommand_TierMatrix` (5/5 subtests pass) | PASS |
| 8 | Backwards compatibility: Solo+ collections continue to work | `from_command_resolves_at_solo_tier`; TierMatrix "solo allows" row | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | 8/8 behaviors verified above | PASS |
| 2 | `go test ./...` passes | All packages pass, 0 failures | PASS |
| 3 | Coverage >= 80% for auth/variable/runner | auth=89.6%, variable=97.4%, runner=84.8% | PASS |
| 4 | `golangci-lint run` passes with 0 issues | ci-local.sh lint gate clean | PASS |
| 5 | `./smoke/run.sh` passes | Smoke complete including new from_command Free-tier block | PASS |
| 6 | `./scripts/ci-local.sh` passes | `=== ci-local PASS ===` | PASS |
| 7 | `registry.go` from_command has RequiredTier=TierFree | `registry.go:43` confirmed | PASS |
| 8 | `registry_test.go` tier-matrix updated | `TestDefaultRegistry_FromCommand_TierMatrix` covers all 5 tiers | PASS |
| 9 | `docs/MANUAL.md` has no `[Solo]` adjacency for from_command | grep observable returns empty | PASS |
| 10 | TOC slug consistent | `#63-from_command-variables` matches heading slug | PASS |
| 11 | smoke/run.sh extended with Free-tier from_command case | Block added near existing tier-gate cases | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Doc comments on exports | PASS |
| Code organization | PASS |
| Test quality | PASS |
| No stale Solo framing in code | PASS |

Branch A: Review PASS trusted (management/reviews/M15-003-review.md verdict=PASS), spot-check clean.
- `registry.go:41-46`: `from_command` at `TierFree`, no `Solo` or `$9/month` in copy
- `TestDefaultRegistry_FromCommand`: screens both Description and Workaround strings
- `TestRunFromCommand/from_command_at_free_tier_resolves`: asserts resolution not gate error

## Commits

| Hash | Message |
|------|---------|
| f2ff017c | docs(review): add passing review for M15-003 |
| 8917ac51 | docs(review): add improvement report for M15-003 |
| b3ac230e | fix(runner,auth): resolve review findings from M15-003 |
| 4d056bdc | docs(review): add review with findings for M15-003 |
| dba453b9 | chore(task): mark M15-003 as review |
| 226dec0b | feat(smoke): add Free-tier from_command happy-path case (M15-003) |
| 1a257b23 | docs(manual): sweep from_command [Solo] badges and Solo-tier prose to Free |
| 3007abe2 | test(cli): rewrite from_command gate tests to assert Free-tier no longer gated |
| 0c2b3757 | test(runner): invert from_command_at_free_tier test to assert resolution not gate |
| 8e296a4d | feat(auth): retier from_command from Solo to Free with refreshed copy |
| a3aab764 | test(auth): add failing tests for from_command TierFree registry contract |

## Files Changed

| File | Action |
|------|--------|
| `internal/auth/registry.go` | modified — RequiredTier TierSolo → TierFree, refreshed Description/Workaround |
| `internal/auth/registry_test.go` | modified — added TestDefaultRegistry_FromCommand and TierMatrix |
| `internal/runner/runner.go` | modified — stale Solo-tier comment updated |
| `internal/runner/runner_test.go` | modified — inverted free-tier subtest to assert resolution |
| `cmd/curlew/main_test.go` | modified — rewrote two gate tests to assert no-longer-gated |
| `docs/MANUAL.md` | modified — 6 surgical edits (TOC, precedence table, mental-model, heading, body, tier-matrix row removed) |
| `smoke/run.sh` | modified — added Free-tier from_command happy-path block |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
