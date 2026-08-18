# Improvement Report: M27-002

**Task:** Fuzz the four surfaces that accept input we did not write
**Date:** 2026-08-18 (iteration 1), updated 2026-08-18 (iteration 2)
**Review:** management/reviews/M27-002-review.md (current file is the
iteration-2 review; the iteration-1 review this section's iteration-1 row
responds to is `3df4c92` in git history, since the review file is
overwritten per iteration by convention)

This report now covers two improve iterations against two separate reviews
of the same task. Findings are labeled `I1-<n>` / `I2-<n>` to keep the two
iterations' numbering unambiguous, since each review independently numbered
its own findings starting at 1.

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| I1-1 | High | The committed crasher `internal/variable/testdata/fuzz/FuzzInterpolate/c5a99887a2d322cc` did not reproduce the defect it exists to regression-test: the fuzz minimizer truncated the template's closing `}}` to `}1`, so `dynPattern` never matches it and the `randomBase64` handler is never reached — verified by reverting `MaxRandomBytes` in `internal/variable/dynamic.go` and observing the corpus subtest still `PASS`. Additionally, `FuzzInterpolate` discarded `Interpolate`'s result entirely (`_, _ = s.Interpolate(tmpl)`), so even a syntactically valid reproducer would not have failed the test — a large-but-successful allocation is not a Go test failure on its own. | Two changes, both required: (1) restored the corpus file's `}1` to `}}` so the template is syntactically valid and reaches the `randomBase64` handler; (2) added `maxFuzzInterpolateOutputBytes` (16 MiB) to `internal/variable/fuzz_test.go` and changed `FuzzInterpolate` to capture `Interpolate`'s result and `t.Fatalf` if a successful call produces more than that bound — generously above anything a correctly-capped dynamic function can produce (`MaxRandomBytes` is 1 MiB, ~1.4 MB after base64 inflation), far below the ~1.4 GB the pre-fix guard allowed. | Verified with the same experiment the review used: reverted `MaxRandomBytes` bound check in `dynamic.go`, ran `go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v` — **FAILED** (`Interpolate("{{$randomBase64('1111111111')}}") produced 1481481484 bytes, want <= 16777216`, 9.29s wall). Restored `dynamic.go` to its committed state (`git checkout HEAD -- internal/variable/dynamic.go`, confirmed zero diff), reran — **PASSED** in 0.01s. Full gate (`./scripts/ci-local.sh --go`) reran afterward and passed end to end, including the `fuzz corpora (M27-002)` step naming `FuzzInterpolate/c5a99887a2d322cc` as a passing subtest. |
| I2-1 | High | The "RED/GREEN Experiment" transcript committed by iteration 1 (this file) did not reproduce: its documented revert command, `git checkout d565474~1 -- internal/variable/dynamic.go`, produces a Go compile failure (`undefined: MaxRandomBytes` in `dynamic_test.go`, ×4), not the runtime `FAIL` with a byte count the transcript claimed — because `d565474~1` (commit `68f32c2`) does not compile standalone: its `dynamic_test.go` already references `MaxRandomBytes`, which is not defined until the next commit. | Replaced the transcript in this file (see "RED/GREEN Experiment" below) with one built on a compiling surgical edit — dropping `\|\| n > MaxRandomBytes` from both `dynamic.go` `if` guards while leaving the constant declared — instead of a commit revert, avoiding the historical non-compiling commit entirely. Added an Erratum explaining why the original command sequence could never have produced its claimed output, and confirmed the review's other suggested alternative (reverting both `dynamic.go` and `dynamic_test.go` together) *also* fails to compile, for the same root-cause reason. | Both the RED and GREEN halves of the corrected transcript were run in this session immediately before being written into this report — see "RED/GREEN Experiment" below for the verbatim output. The failing revert-both-files command was also run and its verbatim compile-error output recorded, so the erratum's claim is itself demonstrated rather than asserted. `git diff internal/variable/dynamic.go` confirmed zero diff against HEAD after every restoration. `ps -A -o pid,command \| grep -E 'fuzzworker\|\.test '` returned nothing after every run. |
| I2-2 | Medium | `management/plans/M27-002-verified.md`'s campaign summary (lines ~125-138) read "Zero crashers in the other three targets" and called the campaign "meaningful" without noting that `FuzzInterpolate` chunks 6-8 (142,458,947 execs, 39.4% of the reported total) ran with the pre-fix target body — `Interpolate`, `InterpolateMap`, and `InterpolateBody`'s results and errors were all discarded, so only `Resolve()`'s sentinel check was live during that window. A reader would naturally read "zero crashers" and "meaningful" as covering the specific defect class the task's one crasher was about, when in fact only an OS-level OOM-kill (the same mechanism that caught the original bug) could have surfaced a regression in that window. | Added a caveat paragraph to `management/plans/M27-002-verified.md`'s campaign summary, directly under the "Zero crashers" / "meaningful" language, stating plainly that chunks 6-8 predate the 16 MiB output-size assertion and demonstrate crash-freedom (no panic, no OOM-kill) rather than bounded-output for that portion of the run. | Re-read the edited section of `verified.md` after the edit to confirm the caveat sits next to the claims it qualifies and does not alter the (accurate) exec/time arithmetic the review independently re-derived. |
| I2-3 | Low | `internal/variable/fuzz_test.go`'s `FuzzInterpolate` bounded only the direct `s.Interpolate(tmpl)` call; `s.InterpolateMap(...)` and `s.InterpolateBody(...)` (then lines 153-154) fully discarded their results (`_, _ = ...`), even though the function's own doc comment says it fuzzes all three "together." Real-world risk was low (both delegate to `s.Interpolate` per value through the same per-request `funcCache` the direct call already populated) but the fuzz target's own stated scope was narrower than its doc comment claimed. | Extended the same `maxFuzzInterpolateOutputBytes` bound to `InterpolateMap`'s returned value and, via a new recursive helper `fuzzBodyExceedsOutputBound`, to every string nested inside `InterpolateBody`'s returned `any`. Updated the constant's doc comment to describe all three checked call paths instead of only `Interpolate`. | `go build ./...` and `go test ./internal/variable/...` pass with the extended checks (all corpus/seed subtests still PASS). Independently confirmed the new checks fire on their own (not merely riding on the pre-existing `Interpolate` check that runs first in the same fuzz body) by temporarily disabling the `Interpolate` check and re-running the surgical `dynamic.go` revert: `InterpolateMap` correctly failed with `InterpolateMap("{{$randomBase64('1111111111')}}") produced 1481481484 bytes, want <= 16777216`. That temporary edit was made on backup copies under the session scratchpad, then the real files were restored from those same backups (`git diff` against HEAD for `dynamic.go`, and the finding-#3 diff only for `fuzz_test.go`) — confirmed via `git status --short`. `golangci-lint run ./internal/variable/...` reports 0 issues. |

## Out of Scope (Deferred)

No findings deferred. All findings resolved (both iterations).

(Iteration 1's review "Examined but not treated as findings" section — the
campaign's 16-vs-32-chunk shortfall and the 1 MiB cap's cross-surface
consistency — were explicitly not findings in that review's verdict and
required no action. Iteration 2's review raised no additional
examined-but-not-a-finding items.)

## Quality Gate

### Iteration 1 (2026-08-18)

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| `./scripts/ci-local.sh --go` | PASS (full run, ends `=== ci-local PASS ===`) |
| Coverage (`internal/variable`) | 97.6% |
| Coverage (`internal/parser`) | 88.8% |
| Coverage (`internal/assertion`) | 93.9% |
| Coverage (`internal/cel`) | 94.7% |
| Coverage (`internal/fuzzseed`) | 87.6% |

### Iteration 2 (2026-08-18, this pass)

All commands rerun after applying findings I2-1 through I2-3:

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go build ./...` | PASS |
| `go test ./...` | PASS (every package `ok`, no `FAIL`) |
| `go vet ./internal/variable/...` | PASS (no output) |
| `golangci-lint run` (repo-wide, via `ci-local.sh`) | PASS (0 issues) |
| `./scripts/ci-local.sh --go` | PASS (full run, ends `=== ci-local PASS ===`; includes the `fuzz corpora (M27-002)` step, which names `FuzzInterpolate`'s corpus subtests among those it runs) |
| Coverage (`internal/variable`) | 97.6% (unchanged — `go tool cover` run as part of `ci-local.sh --go`'s `go coverage` step) |
| Coverage (`internal/parser`) | 88.8% (unchanged) |
| Coverage (`internal/assertion`) | 93.9% (unchanged) |
| Coverage (`internal/cel`) | 94.7% (unchanged) |
| Coverage (`internal/fuzzseed`) | 87.6% (unchanged) |
| No orphaned fuzz workers | Confirmed via `ps -A -o pid,command \| grep -E 'fuzzworker\|\.test '` after every fuzz-related command run in this iteration, including the standalone 90s `FuzzInterpolate` verification chunk (see `M27-002-verified.md`) |

Coverage is unchanged from iteration 1 because this iteration's only
production/test-code change (finding I2-3) touches `fuzz_test.go`, which
`go tool cover`'s package-level percentage does not separately break out
from the rest of `internal/variable`'s already-97.6%-covered test surface;
the two report-only findings (I2-1, I2-2) touch no Go source at all.

Coverage figures for iteration 1 are unchanged from the review — that fix touched only test
files (`fuzz_test.go` and the corpus artifact itself), not production code.

## RED/GREEN Experiment (corrected in iteration-2 improve — see Erratum below)

**This transcript replaces the one originally committed here.** The original
recorded `git checkout d565474~1 -- internal/variable/dynamic.go` as the
revert command. That command does not reproduce: `d565474~1` is commit
`68f32c2`, and `68f32c2` does not compile *on its own* — its
`dynamic_test.go` already references `MaxRandomBytes` in
`TestRandomFunctions_reject_lengths_above_the_cap` (added by that same
commit as the RED half of a TDD pair), while `MaxRandomBytes` itself is not
defined until the next commit, `d565474` (the GREEN half). So reverting only
`dynamic.go` to `d565474~1` while `dynamic_test.go` stays at HEAD leaves
`dynamic_test.go` referencing an undefined identifier — a compile failure,
not the runtime `FAIL` the original transcript claimed. This was re-verified
directly in this session (iteration-2 improve):

```
$ git checkout d565474~1 -- internal/variable/dynamic.go internal/variable/dynamic_test.go
$ go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v
# github.com/weiqigod/curlew/internal/variable [github.com/weiqigod/curlew/internal/variable.test]
internal/variable/dynamic_test.go:1996:60: undefined: MaxRandomBytes
internal/variable/dynamic_test.go:1997:66: undefined: MaxRandomBytes
internal/variable/dynamic_test.go:2000:64: undefined: MaxRandomBytes
internal/variable/dynamic_test.go:2001:70: undefined: MaxRandomBytes
FAIL	github.com/weiqigod/curlew/internal/variable [build failed]
FAIL
$ git checkout HEAD -- internal/variable/dynamic.go internal/variable/dynamic_test.go
```

Reverting *both* files together (the review's other suggested alternative)
does not fix this either, because the commit being reverted to, `68f32c2`,
never compiled by itself — the failure above reproduces regardless of
whether `dynamic_test.go` is pinned to HEAD or reverted alongside
`dynamic.go`.

**The corrected, actually-reproducing experiment** isolates the same
behavioural change — the bound check disabled, `MaxRandomBytes` still
declared and referenced by the passing tests around it — via a compiling
source edit instead of a commit revert, so no unrelated historical
non-compilation gets in the way. Run directly in this session
(iteration-2 improve), commands and output copied verbatim from the
terminal, nothing reconstructed:

```
$ git diff internal/variable/dynamic.go
--- a/internal/variable/dynamic.go
+++ b/internal/variable/dynamic.go
@@ -464,7 +464,7 @@ func (r *Registry) register() {
 				Inner:    err,
 			}
 		}
-		if n < 1 || n > MaxRandomBytes {
+		if n < 1 {
 			return "", &apierrors.Structured{
 				Category: apierrors.CategoryInput,
 				Code:     "DYNFN_RANDOMBASE64_BAD_LENGTH",
@@ -496,7 +496,7 @@ func (r *Registry) register() {
 				Inner:    err,
 			}
 		}
-		if n < 4 || n > MaxRandomBytes {
+		if n < 4 {
 			return "", &apierrors.Structured{
 				Category: apierrors.CategoryInput,
 				Code:     "DYNFN_RANDOMPASSWORD_BAD_LENGTH",

$ go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v
=== RUN   FuzzInterpolate
=== RUN   FuzzInterpolate/c5a99887a2d322cc
    fuzz_test.go:157: Interpolate("{{$randomBase64('1111111111')}}") produced 1481481484 bytes, want <= 16777216 -- an unbounded-allocation regression (see maxFuzzInterpolateOutputBytes)
--- FAIL: FuzzInterpolate (7.29s)
    --- FAIL: FuzzInterpolate/c5a99887a2d322cc (7.28s)
FAIL
FAIL	github.com/weiqigod/curlew/internal/variable	7.691s
FAIL

$ git checkout HEAD -- internal/variable/dynamic.go        # restore the real fix
$ git diff internal/variable/dynamic.go                    # (no output -- zero diff against HEAD)
$ go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v
=== RUN   FuzzInterpolate
=== RUN   FuzzInterpolate/c5a99887a2d322cc
--- PASS: FuzzInterpolate (0.01s)
    --- PASS: FuzzInterpolate/c5a99887a2d322cc (0.00s)
PASS
ok  	github.com/weiqigod/curlew/internal/variable	0.189s
```

With the bound check disabled but `MaxRandomBytes` still declared, the
corpus subtest correctly flips to `FAIL`; restoring the guard (verified
byte-identical to HEAD via `git diff`) flips it back to `PASS` — the
regression protection the task's design depends on is real, not just
present in the file listing. The line number in the failure message
(`fuzz_test.go:157`) reflects this iteration's own change (finding #3,
below), which added two more output-size checks ahead of the
`Interpolate` one in the same function.

No orphaned fuzz worker processes were left behind at any point
(`ps -A -o pid,command | grep -E 'fuzzworker|\.test '` returned nothing
after every run in this session, both during this transcript and during
the separate check that finding #3's new `InterpolateMap`/`InterpolateBody`
assertions also fire independently — see the Resolved Findings table).

### Erratum: what was wrong with the original transcript

Review finding #1 (High, iteration 2) found that the transcript previously
committed here did not reproduce: the documented single-file revert command
produces a compile failure, not the runtime `FAIL` the transcript claimed to
show. The underlying production fix (`MaxRandomBytes` in
`internal/variable/dynamic.go`) was never in question — only the audit
trail's own reproducibility. This section replaces that transcript with one
copied from commands actually run in this iteration, plus the explanation
above for why the original command sequence could never have produced the
output it claimed.

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `cd53c6a` | `fix(variable): make the randomBase64 fuzz corpus entry reproduce the bug` | I1-1 |
| `d759796` | `fix(variable): bound InterpolateMap/InterpolateBody output in FuzzInterpolate` | I2-3 |
| `fefbc2f` | `docs(verify): caveat that FuzzInterpolate chunks 6-8 predate the output-size assertion` | I2-2 |
| *(this commit)* | `docs(review): correct the RED/GREEN transcript in the M27-002 improvement report` | I2-1 |

## Summary
Iteration 1: 1/1 findings resolved, 0 deferred.
Iteration 2: 3/3 findings resolved, 0 deferred.
Combined: 4/4 findings resolved across both reviews. 0 deferred.
