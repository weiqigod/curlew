# Code Review: M26-002

**Task:** One exit-code truth across binary, specification, manual and skill
**Reviewer:** AI
**Date:** 2026-08-16
**Branch:** feature/M26-002-one-exit-code-truth

## Verdict: PASS

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| — | — | — | — | — | No findings. | — |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `internal/exitcodes` uses sentinel errors (`ErrRootNotFound`, `ErrNoSources`), wraps with `%w` throughout (`Reachable`, `parseDir`), never panics on malformed input. `record()`'s deliberate non-decimal-literal skip is documented rather than silent. |
| Input Validation | PASS | `Set(nil)`, `Find(nil, x)`, `MaxDepth(nil)` all tested and safe. Unknown root and empty-package directories return `ErrRootNotFound`/`ErrNoSources` rather than panicking. |
| Naming | PASS | No stuttering (`exitcodes.Code`, not `exitcodes.ExitCode`). Every exported symbol in `internal/exitcodes` carries a doc comment. Package-private helpers use tight, Effective-Go-consistent names (`w`, `fn`, `id`). |
| Code Organization | PASS | `internal/exitcodes` has zero non-stdlib imports. `cmd/curlew` changes are test-only plus a documented map-entry deletion; no package reaches into another's internals. Single extractor discipline honoured (see Summary). |
| Correctness | PASS | AST-walk edge cases (multi-result forwarding, local-identifier resolution, function-literal scoping, mutual-recursion cycle guard) are each backed by a purpose-built `testdata/` fixture. Verified independently by mutation (see Summary) rather than by trusting the fixtures alone. |
| Test Quality | PASS | Table-driven throughout, descriptive `t.Run` subtest names, a real-binary integration test (`TestDocTables_everyDocumentedExitCodeIsProduced` via `os/exec`), vacuity guards at every layer (walk-level in `reachableExitCodes`, table-level in `documentedExitCodes`, statement-level in `readStatement`). |

## Test Coverage
- `internal/exitcodes`: 87.4% of statements
- `cmd/curlew`: 81.1% of statements
- Both above the project's 80% gate. `go build ./...`, `go vet ./...`, `golangci-lint run ./cmd/curlew/... ./internal/exitcodes/...` (0 issues), and `go test ./...` (0 failures) all confirmed clean, both before and after this review's own mutation testing.

## Verification performed (beyond static reading)

The task's four named surfaces were reported green-on-arrival, so the mutation
harness is the only evidence any guard actually fires. Nine independent
mutations were hand-applied against the working tree (not copied from the
CHANGELOG), each under `trap 'git checkout -- <file>' EXIT INT TERM`, with
`git status --porcelain` confirmed empty after every one and at the end of
the session:

1. **M3b (the central claim).** Dropped the `130` row from all three doc
   tables (`MANUAL.md` §4.3, `MANUAL.md` appendix D, `CLI_SPECIFICATION.md`
   §17) at once. The pre-existing pairwise check
   (`TestDocTables_theThreeExitCodeTablesAgree`) **passed** — three documents
   agreeing with each other while all being wrong. The new binary-anchored
   check (`TestExitCodes_all_surfaces_agree`) **failed independently on all
   three**, each citing `perf.go:142` by file and line. This is the property
   the task exists to establish, and it holds.
2. **Coverage guard RED path.** Added an unclassified `## Exit codes for the
   temp probe` heading to a new doc file.
   `TestExitCodes_everyExitCodeSectionIsRegistered` failed, naming the file
   and line; removing the file restored green. The plan recorded this test
   as green-on-arrival (a documented deviation from its own expected RED),
   so this had never been observed failing before — now it has.
3. **Vacuity guard.** Emptied all four data rows from
   `docs/UI_SPECIFICATION.md`'s §2.4 table, leaving header and separator
   only. `documentedExitCodes` — the single extractor reused across all five
   `docs.TableUnder`-backed surfaces — failed loudly ("no exit codes read
   from the table") rather than passing on zero rows.
4. **Decoys.** Confirmed both stay unflagged, by source inspection and by
   the passing suite: `MANUAL.md:1820`'s `| Code | Meaning |` CEL table sits
   under heading "Validation error codes," which the heading regex never
   matches, and it is reached (correctly) only via `docs.TableUnder` scoped
   to "4.3 Exit codes" for the real table, never via an unscoped
   `docs.Table`. `CLI_SPECIFICATION.md` Appendix B's "Exit code `6`" / "`10`"
   lines are singular and colon-less; the exact three occurrences of the
   literal anchor "Exit codes: " in the whole file (verified by direct
   grep) are §18.9, §20, §21 — Appendix B is not among them.
5. **New binary code (plan's M1).** Inserted `if flags.vus < 0 { return 7 }`
   into `perfCmdOut`. All four full-contract surfaces failed at once
   (`MANUAL.md` §4.3, `MANUAL.md` appendix D, `CLI_SPECIFICATION.md` §17,
   the skill), plus M26-001's own `TestSkill_exit_codes_are_reachable` —
   matching the CHANGELOG's claimed transcript exactly.
6. **Phantom documented code, Direction 1 (plan's M2).** Added a `99` row to
   `CLI_SPECIFICATION.md` §17. The new surface check failed
   ("a reader told to expect it will wait forever"), and — since only one
   table changed — the pre-existing pairwise check fired too, confirming the
   two guards are complementary rather than redundant.
7. **Original defect shape, generalized.** Reintroduced an unreachable entry
   (`99: 99`) into `exitCodeSeverity` in `discovery_run.go`.
   `TestExitCodes_no_unreachable_mapping` failed, proving the guard catches
   the general pattern, not just the specific value `6` that was deleted.
8. **Per-command prose reader (plan's M5).** Added a stale `` `6` feature
   gate `` clause to §21's "Exit codes: ..." sentence for `perf`.
   `TestExitCodes_perCommandStatementsAreReachable` failed, reproducing the
   original defect shape in the one reader that specifically guards against
   it recurring.
9. **Coverage-guard exemption integrity.** Independently swept `docs/` with
   the same heading regex outside the test binary and got the same 7
   headings the code claims ("measured 7 on this tree"), each accounted for
   either by a registered surface or by `docsWithoutExitCodeContract`'s two
   entries. Spot-checked both exemption reasons against the files
   themselves: `SPECIFICATION.md`'s claimed 2026-08-04 scope note exists
   verbatim; `docs/history/IMPROVEMENT.md` is explicitly marked
   `> **Status: ARCHIVED**` at its top and its "Failure playbook by exit
   code" section **still lists** a stale `6: Feature gate` entry — proving
   the exemption mechanism is doing real work (correctly keeping a genuinely
   wrong historical claim out of live enforcement, with the reason on
   record) rather than being a rubber stamp.

`go build ./...`, `go vet ./...`, and the full `go test ./...` (0 failures)
were re-run after this mutation campaign to confirm nothing was left in a
broken state.

## Other checks

- **Reuse.** Exactly one call site of `exitcodes.Reachable` in the whole
  repo (`cmd/curlew/skill_exit_codes_test.go`'s `reachableExitCodes`, from
  M26-001), reused by this task rather than re-invoked. Exactly one
  doc-table extractor, `documentedExitCodes`, reused across all five
  `docs.TableUnder`-backed surfaces (the three original tables plus
  `UI_SPECIFICATION.md` §2.4 and `plugins.md`, both added by this task). The
  only signature change is the added `column string` parameter, confirmed
  by diff — matches the brief exactly.
- **Deletion judgment.** `discovery_run_test.go` lost two cases
  ("feature gate beats all," "feature gate wins all") that asserted `6`'s
  now-removed special severity ranking. Generic unranked-code behaviour for
  `worseExitCode` remains covered by pre-existing sentinel cases (`99`,
  `98`) that were already in the file before this task, unrelated to the
  deleted `6`-specific ones. `aggregateExitCodes` is a thin fold over
  `worseExitCode` with no independent logic, so pairwise coverage is
  sufficient; no gap.
- **Third false claim, and whether a fourth survives.** Confirmed
  `checkGraceExpired`/exit-9 residue is gone from
  `docs/UI_SPECIFICATION.md` (all three sites: §2 wiring bullet, §2.3
  startup step — correctly renumbered 1-6 after the deletion — and §2.4's
  table row) and from the actual shipped skill templates
  (`templates/skills/agent/curlew/*.md`, zero matches). The only remaining
  "grace"/exit-9 mentions anywhere under `docs/` are in documents that are
  either explicitly historical (`docs/history/IMPROVEMENT.md`,
  `docs/PRODUCT_ROADMAP.md`'s own problem statement for the M26 milestone,
  `docs/M14_INVESTIGATION.md`, `docs/REVIEW.md` dated before the backend
  removal) or platform-scoped (`docs/SPECIFICATION.md`, exempted on the
  record) — none read as a live claim about the shipped CLI.
- **Deferred follow-ups.** Both are recorded in
  `management/plans/M26-002-plan.md` (D7, D8) and in `CHANGELOG.md`:
  `CLI_SPECIFICATION.md` §20 omits exit `5` though `cmd/curlew/ui.go:149`
  returns it (an omission, not a false claim, so the containment-only
  per-command check still correctly passes), and `MANUAL.md:3483`'s
  differently-worded `pr-check` sentence sits outside the `"Exit codes: "`
  anchor the per-command reader requires. Leaving both was the right call:
  fixing either would need an unproven per-command reachable-code walk this
  task does not build, and both are recorded as follow-up candidates rather
  than silently dropped.

## Summary

M26-002 does exactly what it says: one source-derived exit-code set, checked
against every surface that publishes one, with the four originally-named
surfaces green on arrival and the mutation harness as the only evidence any
guard can actually fail. I re-ran that harness myself rather than trusting
the CHANGELOG's transcripts — nine independent mutations, including the
central M3b case (three documents agreeing with each other while all being
wrong, caught only by the binary-anchored check) — and every one reproduced
exactly as documented. Reuse discipline holds (one reachable-set extractor,
one doc-table extractor, one sanctioned signature change), the two decoys
stay unflagged for the structural reasons claimed, the vacuity guard fires
on a genuinely empty table, and the two deliberately-deferred follow-ups are
honestly recorded rather than swept under the rug. No findings at any
severity.
