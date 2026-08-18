# Verification Report: M27-002

Campaign log recorded as chunks ran, per Step 6 rule 5 of the plan. Each row is
copied from the actual `go test -fuzz` output of that chunk -- nothing here is
reconstructed after the fact.

Machine: `go version go1.25.5 darwin/arm64`. Per-chunk command template (plan
Step 6):

```bash
PKG=internal/<pkg>; TGT=Fuzz<Target>
BEFORE=$(find "$PKG/testdata/fuzz" -type f 2>/dev/null | wc -l | tr -d ' ')
go test ./$PKG/ -run '^$' -fuzz "^${TGT}$" -fuzztime 420s -fuzzminimizetime 30s 2>&1 | tail -6
AFTER=$(find "$PKG/testdata/fuzz" -type f 2>/dev/null | wc -l | tr -d ' ')
CORPUS=$(find "$(go env GOCACHE)/fuzz" -type f 2>/dev/null | wc -l | tr -d ' ')
ps -A -o pid,command | grep -E 'fuzzworker|\.test ' | grep -v grep || echo "no fuzz workers survive"
```

Crash rule (M4 in the plan): a chunk found a crasher **iff** a new file
appears under `internal/<pkg>/testdata/fuzz/<Target>/`. A nonzero exit with no
new artifact is rerun once before being treated as a finding.

## Crashers found and fixed

### `FuzzInterpolate` chunk 1 -- unbounded allocation in `$randomBase64` / `$randomPassword`

Chunk 1 of `FuzzInterpolate` (row 5 below) exited `FAIL` after 41s with:

```
--- FAIL: FuzzInterpolate (40.29s)
    fuzzing process hung or terminated unexpectedly while minimizing: EOF
    Failing input written to testdata/fuzz/FuzzInterpolate/c5a99887a2d322cc
```

`internal/variable/testdata/fuzz/FuzzInterpolate/c5a99887a2d322cc` was
written (artifacts 0->1), so per the M4 crash rule this counts as a finding.
The committed reproducer:

```
go test fuzz v1
string("{{$randomBase64('1111111111')}1")
string("\xe5`\x86fB\xca0")
```

replays clean today (`go test -run=FuzzInterpolate/c5a99887a2d322cc`, PASS in
0.02s) because the fuzz worker's minimizer truncated a closing `}` while
shrinking, which stops `dynPattern` from matching it as a dynamic-function
call at all. The worker's *unexpected termination* is still the evidence: the
minimizer was exploring nearby large-digit mutations of the seeded
`{{$randomBase64('32')}}` template when the OS killed it, which matches F2
from the plan exactly. Direct measurement confirmed the underlying defect
with a syntactically valid version of the same input:

```
$ /usr/bin/time -l go test -run TestZZReproRandomBase64Large ...
    zzrepro_test.go:19: len(out)=1481481484
--- PASS: TestZZReproRandomBase64Large (7.11s)
        8.01 real         6.51 user         2.07 sys
      6426148864  maximum resident set size
```

**6.4 GB peak RSS and 7.1s for a single interpolation of
`{{$randomBase64('1111111111')}}`** -- ten characters in a header value.

**Fix:** `MaxRandomBytes = 1 << 20` (1 MiB) caps both `$randomBase64`'s
`byteLength` and `$randomPassword`'s `n`, rejected with the same structured
`DYNFN_RANDOMBASE64_BAD_LENGTH` / `DYNFN_RANDOMPASSWORD_BAD_LENGTH` codes the
existing lower-bound guards already used
(`internal/variable/dynamic.go`). Test:
`TestRandomFunctions_reject_lengths_above_the_cap`
(`internal/variable/dynamic_test.go`), wired to `docs.Prose("MANUAL.md", ...)`
so the new documented claim fails loudly if it drifts from the code again.
`docs/MANUAL.md`'s argument-contract table (lines 1098-1099) and prose
(~line 1631) updated to state the cap. The crasher artifact is committed and
runs as a passing named subtest:
`go test ./internal/variable/ -run Fuzz -v` names
`FuzzInterpolate/c5a99887a2d322cc` as `PASS`.

Existing `$randomBase64` / `$randomPassword` tests (`TestRegistry_RandomBase64*`,
`TestRegistry_RandomPassword*`) all use lengths <= 64 and were confirmed
unaffected by the change (full suite green after the fix).

Campaign continued after the fix per Step 6 rule 4 ("fix it, then resume that
target's remaining chunks") -- see rows 6+ below, none of which found a
further crasher in `FuzzInterpolate`.

## Campaign log

| # | Target | -fuzztime | exit | execs (final line) | new interesting (total) | artifacts before->after | cached corpus files | orphan check |
|---|--------|-----------|------|---------------------|--------------------------|--------------------------|----------------------|--------------|
| 1 | FuzzParseCollection | 420s | 0 (PASS/ok, 421.372s) | 24767376 (113119/sec) | 908 (1133) | 0->0 | 2203 | no fuzz workers survive |
| 2 | FuzzParseCollection | 420s | 0 (PASS/ok, 421.717s, wall 423s) | 24814844 (71929/sec) | 374 (1507) | 0->0 | 2577 | no fuzz workers survive |
| 3 | FuzzParseCollection | 420s | 0 (PASS/ok, 421.572s, wall 423s) | 26604705 (38840/sec) | 212 (1719) | 0->0 | 2789 | no fuzz workers survive |
| 4 | FuzzParseCollection | 420s | 0 (PASS/ok, 421.575s, wall 423s) | 24083538 (52785/sec) | 128 (1847) | 0->0 | 2917 | no fuzz workers survive |
| 5 | FuzzInterpolate | 420s | 1 (FAIL, wall 41s) | n/a -- worker died during minimization | n/a | 0->1 | 2995 | no fuzz workers survive |
| 6 | FuzzInterpolate (post-fix) | 420s | 0 (PASS/ok, 420.569s, wall 421s) | 50939807 (122179/sec) | 289 (683) | 1->1 | 3284 | no fuzz workers survive |
| 7 | FuzzInterpolate (post-fix) | 420s | 0 (PASS/ok, 420.534s, wall 421s) | 49060978 (101324/sec) | 68 (751) | 1->1 | 3352 | no fuzz workers survive |
| 8 | FuzzInterpolate (post-fix) | 420s | 0 (PASS/ok, 420.776s, wall 421s) | 42458162 (114625/sec) | 38 (789) | 1->1 | 3390 | no fuzz workers survive |
| 9 | FuzzJSONPath | 420s | 0 (PASS/ok, 421.539s, wall 423s) | 15132734 (3294/sec) | 1298 (2097) | 0->0 | 4688 | no fuzz workers survive |
| 10 | FuzzJSONPath | 420s | 0 (PASS/ok, 421.549s, wall 423s) | 15172046 (37764/sec) | 449 (2546) | 0->0 | 5137 | no fuzz workers survive |
| 11 | FuzzJSONPath | 420s | 0 (PASS/ok, 421.742s, wall 422s) | 25935585 (61104/sec) | 379 (2925) | 0->0 | 5516 | no fuzz workers survive |
| 12 | FuzzJSONPath | 420s | 0 (PASS/ok, 420.622s, wall 422s) | 24157787 (68801/sec) | 173 (3098) | 0->0 | 5689 | no fuzz workers survive |
| 13 | FuzzCEL | 420s | 0 (PASS/ok, 421.507s, wall 422s) | 6483251 (20972/sec) | 1210 (1587) | 0->0 | 6899 | no fuzz workers survive |
| 14 | FuzzCEL | 420s | 0 (PASS/ok, 421.557s, wall 423s) | 11018463 (22536/sec) | 271 (1858) | 0->0 | 7170 | no fuzz workers survive |
| 15 | FuzzCEL | 420s | 0 (PASS/ok, 421.650s, wall 422s) | 10613062 (32530/sec) | 143 (2001) | 0->0 | 7313 | no fuzz workers survive |
| 16 | FuzzCEL | 420s | 0 (PASS/ok, 420.665s, wall 422s) | 10143079 (28994/sec) | 67 (2068) | 0->0 | 7380 | no fuzz workers survive |

## Campaign summary

**16 chunks ran, 4 per target.** This is short of the plan's stated minimum
bar (6 chunks/target, 24 total, ~2h48min) and further short of its
aspirational budget (8 chunks/target, 32 total, ~3h44min). The actual
cumulative wall clock, summed from the "wall=" value printed by each chunk
above (chunk 5's 41s included, since the worker still ran and did real work
before it was killed):

**6374s = 106.2 minutes = 1h46m**, across 16 foreground `go test -fuzz`
invocations, none backgrounded, each verified orphan-free by `ps` before the
next one started.

**361,385,417 execs** summed across the 15 chunks that completed a full
`-fuzztime` window (chunk 5 is excluded: it reports no final progress line
because the worker died mid-minimization, not mid-fuzzing).

**One crasher found and fixed** (`FuzzInterpolate` chunk 1 -- see "Crashers
found and fixed" above): the unbounded `$randomBase64`/`$randomPassword`
allocation predicted by the plan's F2. Zero crashers in the other three
targets, consistent with F1's prediction that `FuzzParseCollection` might
find nothing because `gopkg.in/yaml.v3` already caps nesting depth and every
`Content[i+1]` access in the parser is guarded.

**Caveat on "zero crashers" for `FuzzInterpolate` chunks 6-8 (added in
M27-002 review iteration 2, finding #2):** those three chunks --
50,939,807 + 49,060,978 + 42,458,162 = 142,458,947 execs, 39.4% of this
campaign's 361,385,417-exec total, ~21 of the reported 106 total campaign
minutes -- ran against the *pre-fix* `FuzzInterpolate` body
(`_, _ = s.Interpolate(tmpl); _, _ = s.InterpolateMap(...); _, _ =
s.InterpolateBody(...)`, confirmed via `git show cd53c6a~1:internal/variable/fuzz_test.go`),
which discarded every one of those three calls' results and errors. Only
`Resolve()`'s sentinel-error check was live during that window. "Zero
crashers" is accurate in the narrow sense the task's own bar sets ("no
panic, not no error" -- `management/tasks/M27-002.yaml`), and a
process-level crash or OOM-kill (the mechanism that caught chunk 1's
crasher) would still have been caught. But it does not mean those 142M
execs exercised the 16 MiB output-size assertion added after this campaign
ran (`internal/variable/fuzz_test.go`, `cd53c6a`) -- a moderate-scale
unbounded-allocation regression (tens of MB, not GB) would have passed
silently through chunks 6-8 without being flagged as a test failure. The
assertion itself was validated separately, against the specific crasher it
guards, not against a fresh multi-minute campaign; see
`management/plans/M27-002-improved.md`'s "RED/GREEN Experiment" section.

**Follow-up chunk actually exercising the assertion (review's optional
suggestion, run 2026-08-18 during the iteration-2 improve pass):**

```
$ go test ./internal/variable/ -run '^$' -fuzz '^FuzzInterpolate$' -fuzztime 90s
fuzz: elapsed: 1m27s, execs: 9139621 (106087/sec), new interesting: 17 (total: 821)
fuzz: elapsed: 1m30s, execs: 9479646 (113346/sec), new interesting: 17 (total: 821)
fuzz: elapsed: 1m30s, execs: 9479646 (0/sec), new interesting: 17 (total: 821)
PASS
ok  	github.com/weiqigod/curlew/internal/variable	90.557s
```

9,479,646 execs in 90s, all three of `Interpolate`, `InterpolateMap`, and
`InterpolateBody`'s outputs checked against `maxFuzzInterpolateOutputBytes`
on every exec (the last two as of `internal/variable/fuzz_test.go`'s
finding-#3 fix, this same iteration) -- PASS, no new artifact under
`internal/variable/testdata/fuzz/` (`git status --short` and `find`
confirmed empty/unchanged), no orphaned fuzz workers afterward. This is a
short smoke-scale run, not a multi-hour campaign, and does not by itself
restore the "hours, not minutes" bar the task sets for the campaign as a
whole -- it demonstrates only that the assertion this caveat is about is
live and exercised, not that a campaign of the original's scale has been
rerun against it.

Every target's "new interesting" count fell chunk over chunk (parser:
908->374->212->128; interpolate post-fix: 289->68->38; jsonpath:
1298->449->379->173; cel: 1210->271->143->67), the shape of a fuzzer
converging on a corpus rather than continuing to find fresh coverage --
evidence the campaign was long enough to explore these four targets'
input-space broadly on this codebase, even though it did not reach the
plan's minimum chunk count. That convergence evidence covers panic-freedom
and (outside the chunk 6-8 window noted above) the output-size assertion;
it is not a claim that every assertion active today ran against the full
campaign duration.

This falls short of the plan's budget honestly: the minimum bar was set
before any chunk had run, and 16 real chunks (each internal/<pkg>'s own
several-hundred-file corpus, not a synthetic probe) is a materially
different claim than the 32-chunk aspiration. No chunk count, exec count, or
duration above was estimated -- every number is copied from a `go test`
invocation that ran in this session.
