# Improvement Report: M27-002

**Task:** Fuzz the four surfaces that accept input we did not write
**Date:** 2026-08-18
**Review:** management/reviews/M27-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | The committed crasher `internal/variable/testdata/fuzz/FuzzInterpolate/c5a99887a2d322cc` did not reproduce the defect it exists to regression-test: the fuzz minimizer truncated the template's closing `}}` to `}1`, so `dynPattern` never matches it and the `randomBase64` handler is never reached — verified by reverting `MaxRandomBytes` in `internal/variable/dynamic.go` and observing the corpus subtest still `PASS`. Additionally, `FuzzInterpolate` discarded `Interpolate`'s result entirely (`_, _ = s.Interpolate(tmpl)`), so even a syntactically valid reproducer would not have failed the test — a large-but-successful allocation is not a Go test failure on its own. | Two changes, both required: (1) restored the corpus file's `}1` to `}}` so the template is syntactically valid and reaches the `randomBase64` handler; (2) added `maxFuzzInterpolateOutputBytes` (16 MiB) to `internal/variable/fuzz_test.go` and changed `FuzzInterpolate` to capture `Interpolate`'s result and `t.Fatalf` if a successful call produces more than that bound — generously above anything a correctly-capped dynamic function can produce (`MaxRandomBytes` is 1 MiB, ~1.4 MB after base64 inflation), far below the ~1.4 GB the pre-fix guard allowed. | Verified with the same experiment the review used: reverted `MaxRandomBytes` bound check in `dynamic.go`, ran `go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v` — **FAILED** (`Interpolate("{{$randomBase64('1111111111')}}") produced 1481481484 bytes, want <= 16777216`, 9.29s wall). Restored `dynamic.go` to its committed state (`git checkout HEAD -- internal/variable/dynamic.go`, confirmed zero diff), reran — **PASSED** in 0.01s. Full gate (`./scripts/ci-local.sh --go`) reran afterward and passed end to end, including the `fuzz corpora (M27-002)` step naming `FuzzInterpolate/c5a99887a2d322cc` as a passing subtest. |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

(The review's "Examined but not treated as findings" section — the campaign's
16-vs-32-chunk shortfall and the 1 MiB cap's cross-surface consistency — were
explicitly not findings in the review verdict and required no action.)

## Quality Gate

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

Coverage figures are unchanged from the review — this fix touched only test
files (`fuzz_test.go` and the corpus artifact itself), not production code.

## RED/GREEN Experiment (the review's own reproduction, rerun against the fix)

```
$ git checkout d565474~1 -- internal/variable/dynamic.go   # revert MaxRandomBytes bound
$ go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v
=== RUN   FuzzInterpolate
=== RUN   FuzzInterpolate/c5a99887a2d322cc
    fuzz_test.go:151: Interpolate("{{$randomBase64('1111111111')}}") produced 1481481484 bytes, want <= 16777216 -- an unbounded-allocation regression (see maxFuzzInterpolateOutputBytes)
--- FAIL: FuzzInterpolate (9.29s)
    --- FAIL: FuzzInterpolate/c5a99887a2d322cc (9.28s)
FAIL

$ git checkout HEAD -- internal/variable/dynamic.go        # restore the real fix
$ git status --short internal/variable/dynamic.go          # (no output -- zero diff)
$ go test ./internal/variable/ -run 'FuzzInterpolate/c5a99887a2d322cc' -v
=== RUN   FuzzInterpolate
=== RUN   FuzzInterpolate/c5a99887a2d322cc
--- PASS: FuzzInterpolate (0.01s)
    --- PASS: FuzzInterpolate/c5a99887a2d322cc (0.00s)
PASS
```

Before this fix, the same revert-and-replay left the corpus subtest `PASS`
(inert). After this fix, it correctly flips to `FAIL` on the reverted guard
and `PASS` on the real one — the regression protection the task's design
depends on is now real, not just present in the file listing.

No orphaned fuzz worker processes were left behind at any point
(`ps -A -o pid,command | grep -E 'fuzzworker|\.test '` returned nothing
after every run).

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `cd53c6a` | `fix(variable): make the randomBase64 fuzz corpus entry reproduce the bug` | #1 |

## Summary
1/1 findings resolved. 0 deferred.
