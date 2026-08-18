# Code Review: M27-002

**Task:** Fuzz the four surfaces that accept input we did not write
**Reviewer:** AI
**Date:** 2026-08-18
**Branch:** feature/M27-002-fuzz-input-surfaces
**Iteration:** 2

## Verdict: FAIL

## Pre-audit Gate

`./scripts/ci-local.sh --go` was run twice in this session (once streamed, once
captured to a log and grepped). Both runs passed end to end: `go build`,
`go test`, `go test -race`, per-package coverage (all packages >= 80%,
`internal/variable` 97.6%, `internal/parser` 88.8%, `internal/assertion` 93.9%,
`internal/cel` 94.7%, `internal/fuzzseed` 87.6%), `golangci-lint` (0 issues),
`./smoke/run.sh`, and the `fuzz corpora (M27-002)` step, ending
`=== ci-local PASS ===`. Static audit proceeded.

## Scope of this iteration

Iteration 1 found one High finding: the committed corpus entry
`{{$randomBase64('1111111111')}1` had a minimizer-truncated `}1` and never
reached the vulnerable code, so it provided no regression protection. The
improve phase (commit `cd53c6a`) restored the corpus file's `}}` and added a
16 MiB output-size assertion to `FuzzInterpolate`, because the target had been
discarding `Interpolate`'s return value entirely. This iteration independently
re-verifies that fix, checks whether the other three targets share the same
"discards its own result" shape, and reconsiders what the recorded 1h46m
fuzzing campaign actually demonstrated in light of that revelation.

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| 1 | High | Test Quality / Documentation Accuracy | `management/plans/M27-002-improved.md` | 11, 38-58 | The committed "RED/GREEN Experiment" transcript does not reproduce. It documents `git checkout d565474~1 -- internal/variable/dynamic.go` followed by `go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v`, claiming a runtime `FAIL` with `Interpolate(...) produced 1481481484 bytes, want <= 16777216`. Running that exact command sequence in this review instead produces a **compile failure**: `internal/variable/dynamic_test.go:2000:60: undefined: MaxRandomBytes` (and three more identical errors), because `d565474~1` (= commit `68f32c2`) is the commit *before* `MaxRandomBytes` was defined, while `dynamic_test.go` at HEAD already references `MaxRandomBytes` in `TestRandomFunctions_reject_lengths_above_the_cap` (added in that same `68f32c2`, confirmed via `git blame`). `go test` compiles the whole package's test files regardless of `-run` filtering, so this is not environment-dependent — the documented command cannot produce the transcript shown. I confirmed the underlying fix *is* real by reverting the bound check a different way (editing the two `if` conditions to drop `\|\| n > MaxRandomBytes` while leaving the constant declared, so the package still compiles): that reproduces the exact claimed output verbatim (`produced 1481481484 bytes, want <= 16777216`, FAIL), and restoring `dynamic.go` returns it to PASS with zero diff against HEAD. So the fix itself is sound and independently verified — the problem is specifically that the committed transcript in `improved.md` is not achievable by the commands it documents, which is the same class of "looks right but doesn't actually reproduce" failure iteration 1 found in the corpus entry itself, now recurring one level up in the audit trail meant to prove that finding was fixed. | Correct the transcript in `management/plans/M27-002-improved.md` to the command sequence that actually reproduces (e.g. the surgical `if`-condition edit, or `git checkout d565474~1 -- internal/variable/dynamic.go internal/variable/dynamic_test.go` reverting both files together), and re-verify by literally running the corrected commands before committing the report. |
| 2 | Medium | Test Coverage | `management/plans/M27-002-verified.md` | 108-138 | The verification report's claim that the campaign is "meaningful" and its "Zero crashers in the other three targets" framing (line 127) overstates what was actually checked for `FuzzInterpolate`'s post-fix chunks. Chunks 6-8 (rows 6-8 of the campaign log, `FuzzInterpolate (post-fix)`) ran for a combined 142,458,947 execs — 39.4% of the reported 361,385,417-exec total, and roughly 21 of the reported 106 total campaign minutes — recomputed directly from the table's own exec-count column in this review. At the time those chunks ran, `FuzzInterpolate`'s body (verified via `git show cd53c6a~1:internal/variable/fuzz_test.go`) was `_, _ = s.Interpolate(tmpl); _, _ = s.InterpolateMap(...); _, _ = s.InterpolateBody(...)` — every one of those three calls' results and errors was discarded; only `Resolve()`'s sentinel-error check was live. So "zero crashers" in that window is true only in the narrow sense of "no process crash or hang," which is what the task's own stated bar is ("no panic, not no error," per `management/tasks/M27-002.yaml`) — but the report's prose reads as a broader "the campaign was long enough to be meaningful" claim (line 136) that a reader would naturally extend to the specific unbounded-allocation defect class this task's one crasher was about. Post-fix, the only way that class of regression could have been caught during those 142M execs was via an OS-level OOM-kill severe enough to crash the worker (the same mechanism that caught the original bug) — a moderate-scale regression (tens of MB, not GB) would have passed silently through the entire post-fix campaign. Neither `verified.md` nor `improved.md` (written and committed *after* the assertion was added, and specifically about that assertion) notes this caveat. | Add a one-sentence caveat to `management/plans/M27-002-verified.md`'s campaign summary noting that chunks 6-8 predate the 16 MiB output-size assertion and therefore demonstrate crash-freedom, not bounded-output, for that portion of the run. Optionally re-run a short `FuzzInterpolate` chunk now that the assertion exists, to get a campaign result that actually exercises it. |
| 3 | Low | Test Coverage | `internal/variable/fuzz_test.go` | 153-154 | The iteration-1 fix bounds only the direct `s.Interpolate(tmpl)` call (line 150). `s.InterpolateMap(map[string]string{"k": tmpl})` and `s.InterpolateBody(...)` (lines 153-154) still fully discard their results and errors (`_, _ = ...`), even though the function's own doc comment (lines 105-111) says it "fuzzes `Scope.Resolve`, `Interpolate`, `InterpolateMap` and `InterpolateBody` together." Real-world risk is low — I traced `InterpolateMap`/`InterpolateBody` to `internal/variable/variable.go:521` and confirmed both delegate to the same `s.Interpolate(v)` per value, sharing the single per-request `s.funcCache` (`BeginRequest`/`EndRequest`, `variable.go:296-302,436-438`) that the harness's own `s.Interpolate(tmpl)` call already populated with the identical `tmpl` a few lines earlier — so the specific `$randomBase64`/`$randomPassword` regression this task fixed would already be caught via the direct call before `InterpolateMap`/`InterpolateBody` ever run it again. This is a completeness gap in the fuzz target's own stated scope, not a live protection hole for the defect class currently known, and it does not violate the task's literal "no panic" bar (behavior 2 in `management/tasks/M27-002.yaml`) since both calls are still executed and would still surface a genuine panic. | Either extend the same `maxFuzzInterpolateOutputBytes` check to `InterpolateMap`'s and `InterpolateBody`'s results, or narrow the doc comment to state plainly that only `Interpolate`'s output is size-checked and the other two are exercised for panic-freedom only. |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No changes to production error handling since iteration 1; `cd53c6a` touches only test code and a corpus fixture. |
| Input Validation | PASS | Unaffected by this iteration's diff. |
| Naming | PASS | `maxFuzzInterpolateOutputBytes` is descriptive, unexported, well-commented. |
| Code Organization | PASS | No new imports or package boundary changes. |
| Correctness | PASS | The production fix (`MaxRandomBytes` in `internal/variable/dynamic.go`) is correct and independently reverified in this review via an equivalent, compiling revert (see Finding #1's evidence). |
| Test Quality | **FAIL** | See Findings #1-#3. The fuzz-corpus regression guard for the original defect is now real (independently confirmed), but the audit trail documenting that fact does not reproduce as written, the verification report's coverage claims are not fully caveated in light of what was actually asserted during the campaign, and the fix's own scope is narrower than its target function's doc comment claims. |

## Independent Verification Performed This Iteration

1. **Revert experiment, run as literally documented in `improved.md`:**
   ```
   $ git checkout d565474~1 -- internal/variable/dynamic.go
   $ go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v
   # github.com/weiqigod/curlew/internal/variable [github.com/weiqigod/curlew/internal/variable.test]
   internal/variable/dynamic_test.go:2000:60: undefined: MaxRandomBytes
   internal/variable/dynamic_test.go:2001:66: undefined: MaxRandomBytes
   internal/variable/dynamic_test.go:2004:64: undefined: MaxRandomBytes
   internal/variable/dynamic_test.go:2005:70: undefined: MaxRandomBytes
   FAIL	github.com/weiqigod/curlew/internal/variable [build failed]
   ```
   Does not match the transcript in `improved.md` (see Finding #1).

2. **Revert experiment, using a compiling variant** (kept `const MaxRandomBytes` declared; removed only `|| n > MaxRandomBytes` from both `if` guards):
   ```
   $ go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v
   fuzz_test.go:151: Interpolate("{{$randomBase64('1111111111')}}") produced 1481481484 bytes, want <= 16777216 -- an unbounded-allocation regression (see maxFuzzInterpolateOutputBytes)
   --- FAIL: FuzzInterpolate (5.39s)
       --- FAIL: FuzzInterpolate/c5a99887a2d322cc (5.38s)
   FAIL
   ```
   Matches the substance of the claimed transcript exactly. Restored `internal/variable/dynamic.go` to HEAD afterward (`git checkout HEAD -- internal/variable/dynamic.go`); reran the same subtest:
   ```
   $ go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v
   --- PASS: FuzzInterpolate (0.01s)
       --- PASS: FuzzInterpolate/c5a99887a2d322cc (0.00s)
   PASS
   ```
   `git status --short` and `diff <(git show HEAD:internal/variable/dynamic.go) internal/variable/dynamic.go` both confirm the working tree is byte-identical to HEAD after restoration. No orphaned fuzz worker processes at any point (`ps -A -o pid,command | grep -E 'fuzzworker|\.test '` empty after every run).

3. **Toothlessness check on the other three targets**, reading each `f.Fuzz` body directly:
   - `FuzzParseCollection` (`internal/parser/fuzz_test.go:62-74`): every branch of `fuzzParse`'s two return values is checked — `err != nil && col != nil`, `err == nil && col == nil`, and `err == nil && col.Name == ""` are all explicit `t.Fatalf` conditions. Not toothless; also backed by a separate differential test (`TestFuzzParseCollection_matches_ParseFile`) that fails if `fuzzParse` drifts from the real `ParseFile` pipeline.
   - `FuzzJSONPath` (`internal/assertion/fuzz_test.go:43-67`): `jsonpath.Evaluate`'s error is checked against two declared sentinels; `CheckBody`'s result slice is checked for length parity with the input and for the correct `Type` discriminator on every operator in the live operator matrix (`operatorsFromSwitch`, read from `evalBodyAssertion`'s own switch via `go/ast`). Not toothless — real structural and error-contract invariants, though (like any fuzz target without a reference oracle) it does not verify the *semantic correctness* of `Passed`/`Actual`/`Expected` for arbitrary operator+value+body combinations; that would need an independent oracle per operator, which is a reasonable thing to not have.
   - `FuzzCEL` (`internal/cel/fuzz_test.go:44-64`): `Compile`'s error is checked to be a `*CelError` matching one of two declared sentinels, plus the prog-vs-error XOR invariant, across both a `BoolType` and untyped compile. Not toothless. The one panic-only call is `CollectTopLevelRefs(src, ev)` (line 63, result discarded) — traced to `internal/cel/refs.go:27`, a pure string-lexer with no error return and its own dedicated unit tests (`internal/cel/refs_test.go`) for correctness; fuzzing it for panic-freedom only is a reasonable, low-risk scope choice given the function's simplicity and separate coverage, not a repeat of the `FuzzInterpolate` defect shape.

   Conclusion: none of the three other targets share the shape of the original defect (a fuzzed call whose result and error were *both* completely discarded on the primary surface). `FuzzCEL`'s secondary `CollectTopLevelRefs` call is the one panic-only surface among the three, and it is adequately justified. See Finding #3 for the one place this pattern still exists, inside `FuzzInterpolate` itself.

4. **Campaign coverage re-derivation** — recomputed from the campaign log table in `management/plans/M27-002-verified.md` directly (not copied from its prose): `FuzzInterpolate` post-fix chunks 6+7+8 = 50,939,807 + 49,060,978 + 42,458,162 = 142,458,947 execs, 39.4% of the reported 361,385,417-exec total; ~1,263s (~21.05 min) of the reported 6,374s (106.2 min) total wall time. Cross-checked against the pre-fix `FuzzInterpolate` body via `git show cd53c6a~1:internal/variable/fuzz_test.go`, confirming all three interpolation calls were unchecked during that window. See Finding #2.

## Test Coverage

- `internal/fuzzseed`: 87.6%
- `internal/variable`: 97.6%
- `internal/parser`: 88.8%
- `internal/assertion`: 93.9%
- `internal/cel`: 94.7%

All above the 80% threshold (from this session's `./scripts/ci-local.sh --go` run).

## DoD Verification

| Item | Status | Notes |
|---|---|---|
| Fuzz targets for parser, interpolation, JSONPath and CEL | Done | All four exist, build, and pass in the gate. |
| Each seeded from real fixtures already in the repository | Done | Unaffected by this iteration; verified in iteration 1. |
| Extended fuzzing run, duration and findings recorded | **Partially overstated** | Numbers are accurate and independently re-derived in this review, but the report's characterization of what the post-fix `FuzzInterpolate` chunks demonstrated is broader than what was actually asserted during those chunks (Finding #2). |
| Every crasher fixed and its input committed to testdata/fuzz | Done, but its own audit trail is broken | The corpus entry now genuinely reproduces (independently re-verified, see above). The `improved.md` report documenting *how* that was verified does not reproduce as written (Finding #1). |
| Committed corpora run as ordinary tests in the gate | Done | `fuzz corpora (M27-002)` step confirmed running in this session's gate. |
| Circular-reference termination asserted, not assumed | Done | `TestScope_self_reference_terminates_with_ErrCircularReference`, unaffected by this iteration. |
| `./scripts/ci-local.sh --go` passes | Done | Ran twice in this session, both green. |
| CHANGELOG.md updated | Done | Unaffected by this iteration's diff. |

## Summary

The production fix from iteration 1 is real: the committed corpus entry now
correctly reaches the `randomBase64` handler, and the new 16 MiB output-size
assertion on `Interpolate` genuinely fails when the bound is disabled and
passes when it is restored — both independently reconfirmed in this review
through a compiling variant of the revert experiment. The other three fuzz
targets (`FuzzParseCollection`, `FuzzJSONPath`, `FuzzCEL`) were checked line
by line and are not toothless — each asserts real structural and
error-contract invariants on its primary surface, well beyond "did not
panic." The review fails on three items in the audit trail rather than the
underlying fix: the `improved.md` report's own "RED/GREEN Experiment"
transcript does not reproduce via the commands it documents (Finding #1,
High); the verification report's campaign narrative does not caveat that
~39% of `FuzzInterpolate`'s post-fix execs ran with zero output-bound
assertion, so "meaningful" and "zero crashers" language there is narrower
than it reads (Finding #2, Medium); and the fix itself only extends the new
assertion to one of the three functions `FuzzInterpolate`'s own doc comment
claims to cover together, though the residual risk is low given per-request
memoization (Finding #3, Low).
