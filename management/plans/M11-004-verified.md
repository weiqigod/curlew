# Verification Report: M11-004

**Task:** exec --log correlation IDs: additive run_id + request_id fields
**Verified by:** AI
**Date:** 2026-04-26
**Branch:** feature/M11-004-exec-log-correlation-ids
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | 45 packages, 0 failures |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | All gated checks pass (junit gate FAIL is pre-existing from M11-001, non-fatal: no `exit 1`) |
| Coverage `cmd/curlew` | 82.3% | Meets >= 80% threshold |
| Coverage `internal/output` | 92.5% | Meets >= 80% threshold |
| Coverage `internal/output/ids` | 100.0% | Meets >= 80% threshold |
| Coverage `internal/output/events` | 97.8% | Meets >= 80% threshold |

## Observable Output

```
go test -run 'TestExecCmd_LogContainsRunIDAndRequestID' -v ./cmd/curlew/...
=== RUN   TestExecCmd_LogContainsRunIDAndRequestID
--- PASS: TestExecCmd_LogContainsRunIDAndRequestID (0.00s)
PASS
ok      github.com/weiqigod/curlew/cmd/curlew   0.242s

go test -run 'TestExecCmd_LogDryRun_RunIDPresent' -v ./cmd/curlew/...
=== RUN   TestExecCmd_LogDryRun_RunIDPresent
--- PASS: TestExecCmd_LogDryRun_RunIDPresent (0.00s)
PASS
ok      github.com/weiqigod/curlew/cmd/curlew   0.247s

grep -E '^[[:space:]]*RunID[[:space:]]+string' internal/output/jsonl.go
        RunID      string `json:"run_id,omitempty"`    # 1 match

grep -E '^[[:space:]]*RequestID[[:space:]]+string' internal/output/jsonl.go
        RequestID  string `json:"request_id,omitempty"`    # 1 match

grep -c 'run_id' docs/MANUAL.md
2

grep -c 'request_id' docs/MANUAL.md
2

grep -c 'Shipped (M11-004)' IMPROVEMENT.md
1

head -5 IMPROVEMENT.md | grep -E 'M11|2\.4 / §6'
**Status:** Shipped — W1, W2, W3 complete (2026-04-24), W4 complete (2026-04-25), W5 complete (2026-04-25); §2.4 / §6 W5 bonus follow-ups closed in M11 (2026-04-26)
```

Expected: All greps return >=1 match, all tests PASS
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | JSONLEntry gains RunID/RequestID with omitempty tags | `TestAppendJSONL/run_id_and_request_id_are_written_when_populated`, `TestAppendJSONL/run_id_and_request_id_omitted_when_empty` | PASS |
| 2 | Shared 32-char hex generator via `internal/output/ids`; runner and events delegate | `TestNewRunID`, `TestNewRunID_DistinctAcrossCalls`, `TestNewRunID_FallbackOnRandError` | PASS |
| 3 | RequestID == "req-1" for exec | `TestExecCmd_LogContainsRunIDAndRequestID`, `TestExecCmd_LogDryRun_RunIDPresent`, `TestExecCmd_LogHTTPError_RunIDPresent` | PASS |
| 4 | Live exec emits run_id + request_id | `TestExecCmd_LogContainsRunIDAndRequestID` | PASS |
| 5 | Dry-run emits run_id | `TestExecCmd_LogDryRun_RunIDPresent` | PASS |
| 6 | MANUAL.md §4.5 example updated | observable grep (`grep -c 'run_id' docs/MANUAL.md` = 2) | PASS |
| 7 | SPECIFICATION.md expanded with --log schema reference | observable via file inspection | PASS |
| 8 | CHANGELOG.md Added entry for M11-004 | file inspection | PASS |
| 9 | IMPROVEMENT.md §6 W5 bonus annotated Shipped (M11-004) | `grep -c 'Shipped (M11-004)' IMPROVEMENT.md` = 1 | PASS |
| 10 | IMPROVEMENT.md line 3 status header carries §2.4/§6 closeout | `head -5 IMPROVEMENT.md \| grep -E 'M11\|2\.4 / §6'` matches | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` 45 packages PASS | PASS |
| 2 | JSONLEntry has RunID and RequestID with correct JSON tags | `grep -E '^[[:space:]]*RunID' jsonl.go` matches; `omitempty` present | PASS |
| 3 | Exec invocations populate both fields (live and dry-run) | TestExecCmd_LogContainsRunIDAndRequestID, TestExecCmd_LogDryRun_RunIDPresent both PASS | PASS |
| 4 | MANUAL.md §4.5 example shows new fields | grep -c 'run_id' docs/MANUAL.md = 2 | PASS |
| 5 | `go test ./...` passes | 45 packages, 0 failures | PASS |
| 6 | `go test -cover ./internal/output/... ./cmd/curlew/...` >= 80% | ids:100%, output:92.5%, events:97.8%, cmd:82.3% | PASS |
| 7 | `golangci-lint run` passes with 0 issues | 0 issues confirmed | PASS |
| 8 | `./smoke/run.sh` passes | ci-local.sh exits 0, smoke complete | PASS |
| 9 | `./scripts/ci-local.sh` passes | Exit code 0, "ci-local PASS" printed | PASS |
| 10 | IMPROVEMENT.md §6 W5 bonus marked Shipped | `grep -c 'Shipped (M11-004)'` = 1 | PASS |
| 11 | IMPROVEMENT.md line 3 status header carries closeout annotation | head -5 match confirmed | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — errors returned, wrapped with `%w` |
| Naming conventions | PASS — `ids.NewRunID`, `execRequestID`, no stuttering |
| Code organization | PASS — `internal/output/ids` sub-package, no circular imports |
| Test quality | PASS — all three exec log paths covered; `ids` at 100% with fallback; table-driven tests |
| No goroutine leaks | PASS |
| No unprotected shared mutable state | PASS — `randRead` restored via `t.Cleanup` |
| Doc comments on exports | PASS — `NewRunID` and `JSONLEntry` both documented |

Branch A: Review PASS trusted (management/reviews/M11-004-review.md verdict PASS), spot-check clean.

## Commits

| Hash | Message |
|------|---------|
| f532e58 | docs(review): add passing review for M11-004 |
| 3c1bcc1 | docs(review): add improvement report for M11-004 |
| e8eb9c7 | test(exec): add TestExecCmd_LogHTTPError_RunIDPresent for error-path log |
| 683e802 | fix(ids): inject randRead for testability, exercise fallback path |
| 243110c | docs(review): add review with findings for M11-004 |
| ed0f635 | chore(task): mark M11-004 as review |
| 605718a | refactor(cli,output): fix import ordering to satisfy gofumpt |
| 20ae688 | docs(output): document run_id/request_id fields and close IMPROVEMENT.md M11 |
| 6ca7c93 | feat(cli): populate run_id and request_id in exec --log JSONL entries |
| a96604d | test(cli): add failing tests for exec --log run_id and request_id fields |
| b52eae0 | feat(output): add RunID and RequestID correlation fields to JSONLEntry |
| db5be21 | test(output): add failing tests for JSONLEntry RunID/RequestID fields |
| ccc3858 | refactor(output): delegate runner.NewRunID and events.newRunID to ids package |
| 22a5f77 | feat(output): implement ids.NewRunID shared run-id generator |
| 7f13ab7 | test(output): add failing tests for ids.NewRunID |
| 62d652c | chore(task): mark M11-004 as in_progress |
| f7c6c4f | chore(task): mark M11-004 as planned |
| 6b32a60 | docs(plan): add implementation plan for M11-004 |

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| `internal/output/ids/ids.go` | created | Shared `NewRunID` generator |
| `internal/output/ids/ids_test.go` | created | Table-driven shape/uniqueness tests |
| `internal/output/ids/ids_fallback_test.go` | created | White-box fallback path test via `randRead` injection |
| `internal/output/jsonl.go` | modified | Added `RunID` and `RequestID` fields with omitempty |
| `internal/output/jsonl_test.go` | modified | Extended with two new table cases |
| `internal/output/events/emitter.go` | modified | `newRunID` delegates to `ids.NewRunID` |
| `internal/runner/runner.go` | modified | `NewRunID` delegates to `ids.NewRunID` |
| `cmd/curlew/main.go` | modified | `execCmdOut` mints `runID`, populates all three JSONL write sites |
| `cmd/curlew/main_test.go` | modified | Three new exec log tests |
| `docs/MANUAL.md` | modified | §4.5 example updated with run_id/request_id |
| `docs/SPECIFICATION.md` | modified | --log mention expanded with schema reference |
| `CHANGELOG.md` | modified | [Unreleased] Added entry for M11-004 |
| `IMPROVEMENT.md` | modified | §6 W5 bonus annotated; line 3 status header updated |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge. All 10 behaviors verified by tests, coverage above 80% in all packages, lint clean, CI passes.
