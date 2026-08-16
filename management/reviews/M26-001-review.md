# Code Review: M26-001

**Task:** The shipped agent skill stops describing a licensing system that was deleted
**Reviewer:** AI
**Date:** 2026-08-16
**Branch:** feature/M26-001-agent-skill-truth

## Verdict: PASS

## Findings

None. Every claim this task's plan, commits, and CHANGELOG entry make about its own
behavior was independently reproduced by direct measurement (not re-read from the
record) — see "Verification performed" below. Two inert, non-blocking observations
are logged under "Observations" for the record; neither is a standards violation,
a defect, or a false claim.

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| — | — | — | — | — | — | — |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Sentinel errors (`ErrRootNotFound`, `ErrNoSources`) wrapped with `fmt.Errorf("...: %w", err)`; no swallowed errors on any path a caller can observe; no panics on expected-failure paths. `record()`'s silent skip of a non-decimal integer literal is a documented, deliberate under-collection policy (safe-failure direction), not a swallowed error — the package's own doc comments say so and the depth/size guards exist precisely to catch under-collection. |
| Input Validation | PASS | `internal/exitcodes` is test/build tooling only — confirmed by grep that its only importers are `cmd/curlew/skill_exit_codes_test.go` and `internal/errors/coverage_test.go`; it is never linked into the shipped binary. Its two error sentinels cover the malformed-input cases that matter for that role (no sources, unknown root). CLAUDE.md's user-input-validation bar applies to code on the CLI's runtime request path; this code is not on it. |
| Naming | PASS | No stuttering (`exitcodes.Reachable`, not `exitcodes.ExitCodesReachable`); exported surface is minimal (`Code`, `Reachable`, `Set`, `Find`, `MaxDepth`, two sentinels) and every exported symbol has a doc comment explaining rationale, not just shape. |
| Code Organization | PASS | New package is single-responsibility; only cross-package dependency is `internal/errors` via the established hint-registration convention. No circular imports. `defer delete(w.visiting, key)` used correctly for the cycle-guard cleanup. |
| Correctness | PASS | See "Verification performed" — extractor output, depth guard, worktree exclusion, and all three DoD mutations were independently reproduced, not just re-read. `go test -race` clean. |
| Test Quality | PASS | Table-driven tests with descriptive `t.Run` names; fixtures pin exact expected sets, not just "non-empty"; vacuity guards (root-not-found, size, depth) are themselves tested for their trigger condition. Two minor, non-blocking coverage gaps noted below. |

## Test Coverage

- `internal/exitcodes`: 87.4% (independently re-measured; matches the CHANGELOG's claimed 79.7% → 87.4% exactly — see below)
- `cmd/curlew`: 81.1%
- Repository total: 86.2%
- Both exceed the 80% DoD bar.

**Coverage claim reproduced.** The CHANGELOG states commit `e876570` closed
`internal/exitcodes` coverage from 79.7% to 87.4% by adding two fixtures
(`testdata/multiresult`, `testdata/unresolvable`). I checked out the parent
commit (`303b9fb`) into an isolated `git worktree`, ran
`go test ./internal/exitcodes/... -cover` there, and measured **79.7%** exactly.
At `HEAD` the same command measures **87.4%** exactly. Worktree removed after
measurement; no trace left.

**Fixtures judged real, not padding.** `multiresult` exercises two branches no
prior fixture reached: `examineReturn`'s "single return-statement forwards an
entire multi-result call" shape, and `resultCount`'s named-grouped-result
counting branch. Both mirror real `cmd/curlew` code the walker must already
handle correctly — `runCmdWithWriters` at `cmd/curlew/main.go:1493`
(`code, _ := runCmdInner(...); return code`) is the exact multi-value-assignment
shape the fixture's `run`/`forward` pair drives at the unit level. `unresolvable`
exercises `followCall`'s two silent-skip paths (a selector call, a call to a
non-local identifier) that nothing else reached. I traced both fixtures against
the real source by hand and confirm the branches they close were genuinely
unexercised before this commit — not restated coverage of paths five other
fixtures already hit.

## Verification performed

The pipeline instructions called for measuring rather than trusting the written
record, given a prior task's CHANGELOG carried an unreproducible claim. Every
substantive claim in this task's plan and CHANGELOG was independently
reproduced by direct command execution, not by re-reading the record:

1. **Extractor correctness.** Dumped full provenance (`Value`/`File`/`Line`/`Fn`/`Depth`)
   for every code `exitcodes.Reachable(".", "runWithWriters")` finds in the real
   `cmd/curlew` source via a throwaway test (removed afterward; `git status --porcelain`
   confirmed clean). Result: `SET=[0 1 2 3 4 5 130] MAXDEPTH=3`, matching the CHANGELOG
   exactly. Confirmed by hand: `htmlResultStatusCode` and `countWaveResults`
   (main.go:2307, main.go:1894) are excluded because neither's result ever flows into a
   return statement (both are assigned to locals consumed by struct fields / loop logic,
   never returned) — traced their sole call sites to confirm. `runErrorExitCode`
   (main.go:368-376, values 2/3/5) is included at Depth 3, reached exactly via the chain
   the CHANGELOG describes: `runWithWriters` → `runCmdWithWriters` (return-position call,
   depth 1) → `code, _ := runCmdInner(...)` (multi-value-assignment resolution, depth 2)
   → `code := runErrorExitCode(varErr)` (single-assignment resolution, depth 3). The dead
   `exitCodeSeverity` map's `6: 6, // feature gate` (`cmd/curlew/discovery_run.go:25`,
   confirmed untouched by this task's diff) does not appear in the set, structurally —
   the walker only ever examines `*ast.ReturnStmt` nodes, never composite-literal values.

2. **Depth guard fires.** Mutated `internal/exitcodes/reachable.go`'s `followCall` to
   `return` unconditionally (disabling all call resolution), wrapped in
   `trap 'git checkout -- internal/exitcodes/reachable.go' EXIT INT TERM`. Result:
   `TestSkill_exit_codes_are_reachable` failed with exactly the predicted message —
   `every exit code came from runWithWriters itself ([0 1]); nothing was collected
   through a return-position call — the call-graph walk is broken, not the CLI` — rather
   than silently reporting `{0,1}` as a plausible complete answer. `git status --porcelain`
   confirmed clean after.

3. **All three DoD mutations reproduce, byte for byte.** Each wrapped in a `trap ... EXIT
   INT TERM` cleanup and confirmed `git status --porcelain` empty afterward:
   - **M1 (invents a code):** added `| 7 | ERR_FICTION | ... |` to `exit-codes.md`'s
     master table. Got both predicted failures: `exit-codes.md (master table) documents
     exit 7, which cmd/curlew cannot return (reachable: [0 1 2 3 4 5 130])` and the
     four-statement agreement failure.
   - **M2a (omits a code from one statement):** deleted only the `130` row from
     `exit-codes.md`. Got exactly the predicted agreement failure
     (`exit-codes.md (master table) lists [0 1 2 3 4 5]; SKILL.md (failure playbook)
     lists [0 1 2 3 4 5 130]`), with `TestSkill_exit_codes_are_reachable` passing (the
     other three statements still document 130, so the union-based reverse check is not
     the one that fires — confirms the two guards are independent, as designed).
   - **M2b (omits a code from all four statements):** deleted the 130 statement from
     SKILL.md, both places in exit-codes.md, and failure-playbook.md. Agreement passed;
     equality still failed with exactly `cmd/curlew can return exit 130 (perf.go:142, in
     perfCmdOut) but no skill file documents it`.
   - **M3 (binary gains a code):** added `case flags.vus < 0: return 7` before
     `case sum.Aborted: return 130` in `perfCmdOut` (`cmd/curlew/perf.go:140`) — guarded
     by a condition `parsePerfArgs` already rejects before this switch is reached, so
     dynamically unreachable. Ran the full `TestPerf*` suite (14 tests): all still pass,
     confirming no behavioral test outcome changed. `TestSkill_exit_codes_are_reachable`
     failed with exactly `cmd/curlew can return exit 7 (perf.go:142, in perfCmdOut) but no
     skill file documents it`. (Note: the plan's own pre-implementation estimate said
     `perf.go:141`; the CHANGELOG's post-implementation record says `:142`, matching my
     reproduction exactly — the plan's line-number guess being one off before the code
     existed at its final position is normal and not a discrepancy in the shipped work.)

4. **Task YAML observables run as written.** All three commands from
   `management/tasks/M26-001.yaml`'s `observable:` block, run exactly as written:
   `go test ./cmd/curlew/ -run TestSkill_exit_codes_are_reachable -v` (PASS),
   `go test ./cmd/curlew/ -run TestSkill_names_only_real_commands -v` (PASS), and
   `curlew init /tmp/probe --skill agent` followed by
   `! grep -ril "license\|feature gate\|tier" /tmp/probe/.claude/skills/` against a
   freshly built binary — scaffolded 11 files, grep found zero matches, so the negated
   check passes.

5. **Worktree exclusion.** `exitcodes.Reachable(".", skillRoot)` is called from
   `cmd/curlew/skill_exit_codes_test.go`, where Go sets the test binary's working
   directory to the package directory itself (`go list -f '{{.Dir}}' ./cmd/curlew`
   confirms this resolves to the real `cmd/curlew/`, not repo root). Independently,
   `.claude/worktrees/sleepy-hawking-c63960/cmd/curlew/perf.go` exists (confirmed) as a
   second checkout with its own exit-code source, but `parseDir` uses non-recursive
   `os.ReadDir` — even a hypothetical repo-root call could never descend into it.

6. **Regex word-anchoring.** `licensingSurface` correctly excludes `prettier` and
   `frontier` (verified: no `\b` boundary exists mid-word since both neighboring
   characters are word characters) and correctly matches every line the self-test
   (`TestSkill_licensingPattern_matchesWhatItIsFor`) asserts it should. `internal/schema/
   parity_test.go`'s `tierWords` (`strings.Contains`-based) was correctly NOT reused —
   confirmed its `"free"` entry would false-positive on `SKILL.md:160`'s "Edit it freely."

7. **Out-of-scope boundary held.** `cmd/curlew/discovery_run.go` (home of the dead
   `exitCodeSeverity` map, M26-002's territory) does not appear in this task's diff —
   confirmed via `git diff --name-only main...HEAD` and `git log -1 -- discovery_run.go`
   showing its last touch was an unrelated, pre-existing commit.

All mutation testing was performed with `trap ... EXIT INT TERM` cleanup as instructed;
`git status --porcelain` was confirmed empty after every mutation and again after the
full sequence. A final fresh `./scripts/ci-local.sh --go` run after all verification
mutations still passes (build, test, race, lint 0 issues, smoke, dogfood/gaps/redaction/
openapi/crosscheck harnesses all green). No tag, release, or push occurred.

## Observations (non-blocking)

Two items surfaced during the audit that are worth recording but do not rise to
standards violations, defects, or false claims — consistent with this being a
"real issues, not style nitpicks" pass:

1. **A redundant regex alternative.** `licensingSurface`
   (`cmd/curlew/skill_hygiene_test.go:23-31`) includes both a bare `tiers?` alternative
   and a separate `(?:free|solo|professional|enterprise)[ -]tier` alternative. Because
   `\b` boundaries treat a space or hyphen adjacent to "tier" the same way regardless of
   what precedes it, the second alternative is fully subsumed by the first — I confirmed
   this empirically with a differential test (both patterns evaluated against the file's
   own eight self-test cases plus six additional adjacency variants: identical results
   in every case). The pattern is still correctly word-anchored and the self-test still
   proves what it's meant to; this is a dead branch inside the regex, not a behavioral
   gap. Not worth an `/improve` cycle on its own.

2. **The cycle guard has no regression test.** `walker.walk`'s `visiting` map (guarding
   against infinite recursion if the walked source contains a call cycle reachable in
   return position) shows 0% coverage on its early-return branch — no fixture creates a
   genuine cycle. I built a throwaway mutually-recursive fixture
   (`run` → `helper` → `run`) and confirmed the guard works correctly today: the walk
   terminates immediately with `{0}` and `MaxDepth 0` rather than hanging or overflowing
   the stack (fixture and test removed after; `git status --porcelain` confirmed clean).
   A regression fixture for this would be a small, worthwhile future addition, but the
   mechanism is verified correct now and the package already clears the 80% coverage bar
   comfortably (87.4%).

## Summary

This task is unusually well self-verified: nearly every design decision in
`internal/exitcodes` carries a doc comment explaining why, the plan records measured
ground truth rather than assumptions, and the CHANGELOG's specific quoted output
(exact reachable sets, exact line numbers, exact coverage percentages, exact mutation
failure messages) all reproduced byte-for-byte under independent, adversarial
re-execution — including the one figure I did not expect to match exactly
(`MaxDepth 3`) and the one line number where the plan's pre-implementation estimate
and the CHANGELOG's post-implementation measurement legitimately differ by one line
(141 vs. 142), which I confirmed resolves in the CHANGELOG's favor. The three DoD
mutations are not merely described — I re-ran all three from scratch and they fire
exactly as documented, including the two-guard independence property (M2a vs. M2b)
that is easy to get wrong. Coverage added late (`e876570`) to close a real gap under
the 80% bar is genuine new coverage of previously-unexercised branches tied to actual
production code shapes, not padding. The task's own stated out-of-scope boundary
(`discovery_run.go`'s dead map) is untouched and structurally excluded, not carved out
by exception list. No basis found for the deviation notes about a mismatched
finding-count of 16 or ordering issues — I searched the plan, task YAML, and CHANGELOG
for any such claim specific to this task and found none; nothing here contradicts or
confirms it, so it is left unaddressed as unlocatable rather than asserted either way.
