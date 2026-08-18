# Code Review: M27-002

**Task:** Fuzz the four surfaces that accept input we did not write
**Reviewer:** AI
**Date:** 2026-08-18
**Branch:** feature/M27-002-fuzz-input-surfaces
**Iteration:** 1

## Verdict: FAIL

## Pre-audit Gate

`./scripts/ci-local.sh --go` was run in full (captured output, ~2100 lines) and
passed end to end: `go build`, `go test`, `go test -race`, per-package
coverage, `golangci-lint`, `./smoke/run.sh`, and the new `fuzz corpora
(M27-002)` step. The `FAIL` lines visible in the smoke output are the
suite's own expected-failure fixtures (`nonexistent_validate_test.yaml`,
malformed YAML, etc.), not gate failures. Static audit proceeded.

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| 1 | High | Test Quality | `internal/variable/testdata/fuzz/FuzzInterpolate/c5a99887a2d322cc` | 1-3 | The committed crasher artifact does not reproduce the defect it exists to regression-test. Its template is `{{$randomBase64('1111111111')}1` — the fuzz minimizer truncated the trailing `}}` to `}1` while shrinking. Verified directly in this review: `internal/variable/dynamic.go` was temporarily reverted to its pre-fix guard (`n < 1` / `n < 4`, no upper bound) and `go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v` still reported `--- PASS`. A standalone check of `dynPattern.MatchString` on the exact committed string also returns `false`. So the artifact never reaches the `randomBase64` handler at all, on either side of the fix — it is inert. The report itself discloses this ("replays clean today ... because the minimizer truncated a closing `}`"), which is honest, but the underlying gap stands: the fuzz-corpus mechanism this task is built around ("a crasher found once is regression-tested forever without anyone maintaining a list") provides zero protection for this specific defect. The only real regression guard is the hand-written `TestRandomFunctions_reject_lengths_above_the_cap` — exactly the kind of maintained-list dependency the corpus mechanism was supposed to make unnecessary. | Replace (or add alongside) the committed corpus file with a `go test fuzz v1` entry whose template actually triggers the pre-fix defect — e.g. a syntactically valid `{{$randomBase64('1111111111')}}` with a matching second `string(...)` arg for `blob`. The report's own `TestZZReproRandomBase64Large` measurement already confirms that exact shape reproduces (6.4 GB RSS / 7.1s) pre-fix and returns a clean bounded error post-fix, so it is a drop-in replacement, not new investigation. |

## Examined but not treated as findings

**Campaign shortfall (16 chunks / 4 per target / 1h46m vs. the plan's stated
minimum of 6 chunks/target, 2h48m).** Verified as genuine, not padded:
recomputed the campaign log's exec-count column by hand and it sums to
exactly the reported 361,385,417; recomputed wall-clock from the per-row
`wall=` values and it sums to ~6373s against the reported 6374s. The task's
own DoD line is "Extended fuzzing run, duration and findings recorded in the
verification report" — satisfied literally, and the shortfall against the
plan's self-imposed minimum is disclosed three times (verified.md,
CHANGELOG.md, commit `a0ecde1`), not hidden or rounded up to look complete.
The plan's 6-chunk floor was computed from a synthetic single-function probe
module before any real per-package corpus existed; the actual per-target
"new interesting" counts (parser 908→374→212→128; jsonpath
1298→449→379→173; cel 1210→271→143→67; interpolate post-fix 289→68→38) show
real convergence, which is the substantive signal the chunk-count floor was
a proxy for. No explanation is given for *why* the run stopped at 4/target
rather than continuing toward 6, which is a minor transparency gap, but
given the DoD wording is met and the shortfall is prominently and honestly
recorded rather than concealed, this is not scored as a review finding.

**The 1 MiB cap's cross-surface consistency.** Traced independently across
every register the plan named: `internal/variable/dynamic.go` (both
`randomBase64` and `randomPassword` guards use `MaxRandomBytes`),
`internal/variable/dynamic_test.go`
(`TestRandomFunctions_reject_lengths_above_the_cap`, boundary-tested at
exactly the cap and one over), `docs/MANUAL.md`'s argument-contract table
(lines 1098-1099, both rows now state `1048576`) and its prose (~line
1649), and `docs/CLI_SPECIFICATION.md` (lists the function names only, no
argument contracts to drift). `docs.Prose("MANUAL.md", "reject a length
above 1048576 (1 MiB)")` was confirmed to match the exact sentence added at
`docs/MANUAL.md:1649` and only that sentence, and the phrase does not also
appear in `docs/prose-claim-baseline.txt` (which would indicate stale
debt). No schema file references either function's argument shape. This
register is fully consistent — no finding.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `fuzzseed.ErrNoSeeds` used as a sentinel with `errors.Is` in tests; every reader wraps with `%w` and named context (`reading %s: %w`). The one pre-existing double-wrap (`fmt.Errorf("internal: %w", fmt.Errorf("not a directory"))`) was cleaned up in a separate commit (`4869016`). `dynamic.go`'s new upper-bound check reuses the existing `apierrors.Structured` shape and codes. |
| Input Validation | PASS | `MaxRandomBytes` bounds both functions above as well as below; `strconv.Atoi` failures, arity mismatches, and empty seed sources (`ErrNoSeeds`) are all handled as errors, never panics. `FuzzParseCollection`'s harness correctly separates fuzzed input from real filesystem access (empty `f.TempDir()`, no include resolution). |
| Naming | PASS | No stuttering (`fuzzseed.Seed`, not `fuzzseed.FuzzseedSeed`). Every exported symbol in `internal/fuzzseed` has a doc comment. `operatorsFromSwitch`'s widening to `testing.TB` is a minimal, well-justified signature change with no behavior change for existing callers. |
| Code Organization | PASS | `internal/fuzzseed` is a leaf package (no dependency on `internal/parser`, avoiding the import cycle the plan called out as D4's reason for walking `yaml.Node` directly). No circular imports found. `internal/errors/coverage_test.go` correctly registers the new package's blank import. |
| Correctness | PASS | Verified independently: `gofmt -l` clean on all changed files; `go vet` clean; all four fuzz targets build and run (`FuzzCEL` smoke-run for 5s in this review, no crash, no orphaned worker); `TestScope_self_reference_terminates_with_ErrCircularReference`'s seven cases all pass, including the `default:`-fallback asymmetry case; `TestFuzzParseCollection_matches_ParseFile` parity guard passes. Measured seed counts (168/110/18/119/6) reproduced exactly against the numbers recorded in `fuzzseed_test.go` and `CHANGELOG.md`. |
| Test Quality | **FAIL** | See Finding #1 — the one committed crasher artifact does not exercise the code path it is meant to regression-test. Everything else in this category is solid: table-driven tests throughout, `t.Run` with descriptive names, a 5s-deadline goroutine pattern for termination assertions, and the `docs.Prose` wiring that ties the doc claim to the code so the two cannot silently drift apart again. |

## Test Coverage

- `internal/fuzzseed`: 87.6%
- `internal/variable`: 97.6%
- `internal/parser`: 88.8%
- `internal/assertion`: 93.9%
- `internal/cel`: 94.7%

All above the 80% threshold. (Figures from the full `./scripts/ci-local.sh
--go` coverage step run in this review.)

## DoD Verification

| Item | Status | Notes |
|---|---|---|
| Fuzz targets for parser, interpolation, JSONPath and CEL | Done | All four exist, build, and were smoke-run in this review |
| Each seeded from real fixtures already in the repository | Done | `internal/fuzzseed`, counts reproduced independently |
| Extended fuzzing run, duration and findings recorded | Done | 16 chunks, 1h46m, 361,385,417 execs — arithmetic independently verified |
| Every crasher fixed and its input committed to testdata/fuzz | **Not met** | Fix is real and correctly applied; the committed input does not reproduce the crash it documents (Finding #1) |
| Committed corpora run as ordinary tests in the gate | Done | `fuzz corpora (M27-002)` step added to `ci-local.sh`, confirmed running in the pre-audit gate |
| Circular-reference termination asserted, not assumed | Done | `TestScope_self_reference_terminates_with_ErrCircularReference`, 5s-deadline goroutine pattern |
| `./scripts/ci-local.sh --go` passes | Done | Full run in this review |
| CHANGELOG.md updated | Done | Both `Fixed` and `Added` entries, consistent with the code and the verification report |

## Summary

The four fuzz targets, the `fuzzseed` package, the circular-reference
termination test, and the gate wiring are all well-built: careful design
decisions (compile-only CEL, operator matrix from the switch's own source of
truth, a `ParseFile` parity guard against harness drift) are followed
faithfully in the code, every seed count and campaign number checked in this
review reproduced exactly, and the campaign's honestly-disclosed shortfall
against its own planning estimate does not by itself sink the task. The
review fails on one concrete, verified defect: the single crasher this
campaign found has a committed reproducer that no longer reproduces
anything, on either side of the fix, which undercuts the specific DoD item
("every crasher fixed and its input committed") and the task's core premise
that a fuzz corpus, once committed, needs no further maintenance to keep
protecting against a regression.
