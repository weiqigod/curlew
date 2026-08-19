# Code Review: M27-002

**Task:** Fuzz the four surfaces that accept input we did not write
**Reviewer:** AI
**Date:** 2026-08-18
**Branch:** feature/M27-002-fuzz-input-surfaces
**Iteration:** 3

## Verdict: PASS

## Pre-audit Gate

`./scripts/ci-local.sh --go` was run twice in this session (once streamed to
the terminal, once captured to a log and grepped for stage markers). Both
runs passed end to end and ended `=== ci-local PASS ===`: `go build`,
`go test`, `go test -race`, per-package coverage (`internal/variable` 97.6%,
`internal/parser` 88.8%, `internal/assertion` 93.9%, `internal/cel` 94.7%,
`internal/fuzzseed` 87.6%, all >= 80%), `golangci-lint` (0 issues),
`./smoke/run.sh`, and the `fuzz corpora (M27-002)` step. Static audit and
independent reproduction proceeded.

## Scope of this iteration

Iteration 2 found three findings, all addressed by commits `d759796`
(bound `InterpolateMap`/`InterpolateBody` output in `FuzzInterpolate`),
`fefbc2f` (caveat `verified.md`'s campaign summary), and `52490fa` (correct
the RED/GREEN transcript in `improved.md`). This iteration's job was to
independently re-verify each fix rather than trust the improve pass's own
account of it — the prior finding was itself "a report claims something
that doesn't reproduce," so the bar for this iteration's own verification
is to actually run the things being claimed, not read them.

## Independent Verification Performed This Iteration

**1. Re-ran the corrected RED/GREEN/Erratum transcript in `improved.md` exactly
as written, byte for byte.**

Starting from a clean tree (`git status --short` empty), applied the exact
surgical edit the transcript documents (`git diff` reproduced verbatim
against the transcript's own diff block: `if n < 1 || n > MaxRandomBytes {`
→ `if n < 1 {`, and the parallel edit for `randomPassword`'s `n < 4`
guard), then ran the documented command:

```
$ go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v
=== RUN   FuzzInterpolate
=== RUN   FuzzInterpolate/c5a99887a2d322cc
    fuzz_test.go:157: Interpolate("{{$randomBase64('1111111111')}}") produced 1481481484 bytes, want <= 16777216 -- an unbounded-allocation regression (see maxFuzzInterpolateOutputBytes)
--- FAIL: FuzzInterpolate (5.13s)
    --- FAIL: FuzzInterpolate/c5a99887a2d322cc (5.12s)
FAIL
```

Matches the transcript's claimed output exactly, including the file:line
(`fuzz_test.go:157`), the exact byte count (`1481481484`), and the bound
(`16777216`). Only wall-clock time differs (5.13s here vs 7.29s in the
report), which is expected machine-to-machine/run-to-run variance and not
part of the reproducibility claim.

Restored `dynamic.go` (`git checkout HEAD -- internal/variable/dynamic.go`,
confirmed zero diff via `git diff`) and reran the same subtest:

```
$ go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v
=== RUN   FuzzInterpolate
=== RUN   FuzzInterpolate/c5a99887a2d322cc
--- PASS: FuzzInterpolate (0.01s)
    --- PASS: FuzzInterpolate/c5a99887a2d322cc (0.00s)
PASS
```

Matches the GREEN half exactly. Then independently ran the Erratum's
claim — that reverting *both* `dynamic.go` and `dynamic_test.go` to
`d565474~1` still fails to compile, not just the single-file revert:

```
$ git checkout d565474~1 -- internal/variable/dynamic.go internal/variable/dynamic_test.go
$ go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v
# github.com/weiqigod/curlew/internal/variable [github.com/weiqigod/curlew/internal/variable.test]
internal/variable/dynamic_test.go:1996:60: undefined: MaxRandomBytes
internal/variable/dynamic_test.go:1997:66: undefined: MaxRandomBytes
internal/variable/dynamic_test.go:2000:64: undefined: MaxRandomBytes
internal/variable/dynamic_test.go:2001:70: undefined: MaxRandomBytes
FAIL	github.com/weiqigod/curlew/internal/variable [build failed]
```

Matches exactly, including the four specific line numbers. Restored both
files (`git checkout HEAD -- internal/variable/dynamic.go
internal/variable/dynamic_test.go`); `git status --short` returned empty.
**All three halves of the corrected transcript reproduce precisely as
documented.** This was the single most important check for this iteration
and it passed without qualification.

**2. Verified the finding-3 fix (`InterpolateMap`/`InterpolateBody` bounds)
fires independently, not merely riding on the pre-existing `Interpolate`
check that runs first in the same fuzz body.**

Backed up `fuzz_test.go`. Neutered only the `Interpolate` check (changed its
guard to `err == nil && false && len(out) > ...` so it can never fire),
applied the same surgical `dynamic.go` revert, and ran the corpus subtest:

```
    fuzz_test.go:161: InterpolateMap("{{$randomBase64('1111111111')}}") produced 1481481484 bytes, want <= 16777216 -- an unbounded-allocation regression (see maxFuzzInterpolateOutputBytes)
--- FAIL: FuzzInterpolate (5.16s)
```

`InterpolateMap`'s own check fired on its own. Then additionally neutered
the `InterpolateMap` check the same way (leaving only `InterpolateBody`'s
check live) and reran:

```
    fuzz_test.go:166: InterpolateBody("{{$randomBase64('1111111111')}}") produced a 1481481484-byte string, want <= 16777216 -- an unbounded-allocation regression (see maxFuzzInterpolateOutputBytes)
--- FAIL: FuzzInterpolate (5.31s)
```

`InterpolateBody`'s check (via the new `fuzzBodyExceedsOutputBound` helper)
also fired on its own. Restored `dynamic.go` and `fuzz_test.go` to HEAD;
`diff` against the pre-experiment backup and `git status --short` both
confirmed a byte-identical, clean tree. Finding #3 from iteration 2 is
genuinely fixed — each of the three checks is load-bearing independently,
not decorative.

**3. Verified the finding-2 caveat's numbers against the campaign log.**

Recomputed the exec-count arithmetic directly from the campaign table in
`verified.md` (not copied from its prose): chunks 6+7+8 =
50,939,807 + 49,060,978 + 42,458,162 = **142,458,947**, against a table-wide
total of **361,385,417** execs (summed across all 15 completed chunks) —
**39.42%**, which the caveat rounds to "39.4%," both matching exactly.
Checked the caveat's factual claim about what those chunks' target body
actually did by reading the pre-fix source directly:

```
$ git show cd53c6a~1:internal/variable/fuzz_test.go | sed -n '130,170p'
...
		_, _ = s.Interpolate(tmpl)
		_, _ = s.InterpolateMap(map[string]string{"k": tmpl})
		_, _ = s.InterpolateBody(map[string]any{"k": []any{tmpl, 1, true}})
```

Confirms the caveat's claim that all three calls' results and errors were
discarded during the campaign is accurate (`cd53c6a~1` is the code state at
the time the original campaign ran — `fuzz_test.go` was untouched between
`FuzzInterpolate`'s creation, `6b52b3e`, and the iteration-1 fix, `cd53c6a`).
The caveat is accurate, not just plausible.

**4. Scope check on what changed since the iteration-2 review commit
(`5c16003`).**

```
$ git diff --name-only 5c16003..HEAD
internal/variable/fuzz_test.go
management/plans/M27-002-improved.md
management/plans/M27-002-verified.md
```

Only the three files the three findings targeted changed. `CHANGELOG.md`
and every other file are untouched, consistent with the DoD item for
`CHANGELOG.md` having already been satisfied in an earlier iteration and
not needing to change again.

**5. General standards pass on the one code diff this iteration
(`d759796`, `internal/variable/fuzz_test.go`).** Read the full diff: the new
`fuzzBodyExceedsOutputBound` helper is a straightforward recursive walk over
`InterpolateBody`'s `string`/`map[string]any`/`[]any` return shape, correctly
falls through to `(0, false)` for non-string leaves (`int`, `bool`), and is
exercised by every corpus/seed subtest. `go vet ./internal/variable/...` and
`golangci-lint run ./internal/variable/...` both reported clean. No new
exported symbols were added (`fuzzBodyExceedsOutputBound` and the extended
checks are unexported, test-only), so no naming/doc-comment obligations were
introduced.

**No orphaned processes at any point.** `ps -A -o pid,command | grep -E
'fuzzworker|\.test '` (filtered `grep -v grep`) returned nothing after every
command in this session, including the neutered-check experiments.
**Working tree confirmed clean** (`git status --short` empty) both mid-review
after each restoration and at the end of the session.

## Findings

None.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No production error-handling paths changed this iteration; `d759796` is fuzz-harness test code, `fefbc2f`/`52490fa` are docs-only. |
| Input Validation | PASS | Unaffected by this iteration's diff. |
| Naming | PASS | `fuzzBodyExceedsOutputBound` is descriptive, unexported, matches the existing `maxFuzzInterpolateOutputBytes` naming; doc comment present and accurate. |
| Code Organization | PASS | No new imports, no package-boundary changes. |
| Correctness | PASS | Both the original production fix and this iteration's harness extension independently reconfirmed by direct reproduction (see Verification 1 and 2 above). |
| Test Quality | PASS | The three checks inside `FuzzInterpolate` (`Interpolate`, `InterpolateMap`, `InterpolateBody`) are each independently load-bearing, confirmed by neutering experiments rather than by reading the code and assuming so. |

## Test Coverage

- `internal/variable`: 97.6%
- `internal/parser`: 88.8%
- `internal/assertion`: 93.9%
- `internal/cel`: 94.7%
- `internal/fuzzseed`: 87.6%

All above the 80% threshold (from this session's `./scripts/ci-local.sh --go` run).

## DoD Verification

| Item | Status | Notes |
|---|---|---|
| Fuzz targets for parser, interpolation, JSONPath and CEL | Done | All four exist, build, and pass in the gate (confirmed again this session). |
| Each seeded from real fixtures already in the repository | Done | Unaffected by this iteration; verified in iteration 1. |
| Extended fuzzing run, duration and findings recorded | Done | Numbers independently re-derived and confirmed exact in this review (Verification 3); the campaign narrative now correctly scopes what "zero crashers" and "meaningful" cover. |
| Every crasher fixed and its input committed to testdata/fuzz | Done | The corpus entry reproduces the defect (independently reconfirmed, Verification 1), and the audit trail documenting that fact now reproduces as written — the exact gap iteration 2 flagged is closed. |
| Committed corpora run as ordinary tests in the gate | Done | `fuzz corpora (M27-002)` step confirmed running in this session's gate. |
| Circular-reference termination asserted, not assumed | Done | `TestScope_self_reference_terminates_with_ErrCircularReference`, unaffected by this iteration. |
| `./scripts/ci-local.sh --go` passes | Done | Ran twice in this session, both green, ending `=== ci-local PASS ===`. |
| CHANGELOG.md updated | Done | Unaffected by this iteration's diff (already satisfied in an earlier iteration; confirmed no new gap opened). |

## Summary

All three iteration-2 findings are genuinely resolved, and this iteration
independently reproduced the evidence rather than trusting the improve
pass's account of it. The corrected RED/GREEN/Erratum transcript in
`improved.md` (finding #1, previously the highest-severity issue precisely
because a transcript *didn't* reproduce) now reproduces exactly, command for
command, output for output, down to specific byte counts and line numbers —
verified independently in this session, not re-read from the report. The
`InterpolateMap`/`InterpolateBody` bound checks added for finding #3 are
each independently load-bearing: neutering the checks ahead of them in the
fuzz body still surfaces the regression, confirming they are not decorative.
The campaign-summary caveat added for finding #2 is factually accurate
against both the campaign log's own numbers (recomputed independently: exact
match) and the actual pre-fix source code from that point in history (also
confirmed by direct inspection). No new findings surfaced in this iteration's
audit of the diff since the iteration-2 review. The task is ready for
`/verify`.
