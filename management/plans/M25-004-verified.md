# Verification Report: M25-004

**Task:** A binary built from source knows which version it is
**Verified by:** AI
**Date:** 2026-08-18
**Branch:** feature/M25-004-buildinfo-version
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/curlew` (via ci-local.sh) | PASS | no output, no warnings |
| `go test ./...` | PASS | `cmd/curlew` 101.087s; all 40+ packages `ok` |
| `go test -race ./...` | PASS | `cmd/curlew` 103.439s; no races detected |
| `golangci-lint run` | PASS | `0 issues.` |
| `./smoke/run.sh` | PASS | ends `=== Smoke Test Complete ===` |
| Coverage | 86.2% total / 81.1% `cmd/curlew` | Meets >= 80% threshold |
| `./scripts/ci-local.sh` (full, not `--go`) | PASS | `rc=0`, ends `=== ci-local PASS ===` |

`ci-local.sh` auto-detected scope `go=1 backend=0 web=0 ui=0 e2e=0` — correct,
since this branch touches only `cmd/curlew/`, `management/`, `scripts/`,
`README.md` and `CHANGELOG.md`, none of `src/ApiTool.Backend/**`, `web/**`, or
the docker-compose stack. Per the coordinator's instructions the *full*
(non-`--go`) gate was run anyway, as the authoritative and only signal — this
also exercised the goreleaser snapshot build, the release-artifact tests, the
tagged `readme_install`/`release_artifacts` suites, and the full mudflat
dogfood/redaction/openapi/crosscheck harnesses, all of which passed. The
known-flaky WebSocket-heartbeat timing test (`a heartbeat survives while a
step is reading` / `...while a step is idle`) did **not** flake this run —
both passed on the only run performed; no retry was needed.

`git tag -l` printed exactly `v0.1.0` before the gate run and after every
subsequent command in this session, including the tests that build a tagged
throwaway fixture repo and the tests that probe worktree/shallow-clone
behavior. `git status --porcelain` was empty throughout.

## Observable Output

Command 1:
```
$ go test ./cmd/curlew/ -run TestVersion_falls_back_to_build_info -v
--- PASS: TestVersion_falls_back_to_build_info (0.00s)
    (17/17 subtests PASS: installed_at_a_tag, installed_at_a_pre-release_tag,
    built_at_a_local_tag, ldflag_wins_over_build_info,
    ldflag_wins_even_when_build_info_is_junk,
    goreleaser_form_is_passed_through_verbatim, devel_is_not_a_version,
    empty_is_not_a_version, absent_build_info,
    pseudo-version_from_a_commit_after_a_tag,
    pseudo-version_from_an_untagged_repo, pseudo-version_from_a_pre-release_base,
    dirty_pseudo-version, dirty_at_an_exact_tag, incompatible_major,
    module_path_is_not_a_version, empty_injection_is_treated_as_untouched)
PASS
ok  	github.com/weiqigod/curlew/cmd/curlew	0.462s
```

Command 2:
```
$ go test ./cmd/curlew/ -run TestVersion_is_injectable_at_link_time -v
--- PASS: TestVersion_is_injectable_at_link_time (0.89s)
PASS
ok  	github.com/weiqigod/curlew/cmd/curlew	1.201s
```

Command 3:
```
$ go test ./cmd/curlew/ -run TestVersion_ -v
--- PASS: TestVersion_is_injectable_at_link_time (0.87s)
--- PASS: TestVersion_injection_reaches_help (0.90s)
--- PASS: TestVersion_injection_reaches_info_json (0.92s)
--- PASS: TestVersion_default_when_not_injected (0.84s)
--- PASS: TestVersion_reported_matches_this_binarys_build_info (1.02s)
--- PASS: TestVersion_all_surfaces_agree (0.96s)
--- PASS: TestVersion_falls_back_to_build_info (0.00s)  [17 subtests]
--- PASS: TestVersion_default_is_the_constant (0.00s)
--- PASS: TestVersion_build_info_reader_reads_this_binary (0.00s)
--- PASS: TestVersion_build_info_form_matches_goreleaser (0.00s)
--- PASS: TestVersion_only_version_go_reads_the_raw_symbol (0.00s)
--- PASS: TestVersion_tagged_build_reports_the_tag (5.68s)  [2 subtests]
--- PASS: TestVersion_v0_1_0_predates_the_fallback (0.02s)
PASS
ok  	github.com/weiqigod/curlew/cmd/curlew	11.521s
```

All three observable commands from `management/tasks/M25-004.yaml` were run
verbatim, in the foreground, this session. Expected: all `TestVersion_*`
tests pass, with the ldflag-precedence test (command 2) unmodified and
passing on its own. Result: MATCH.

Additionally, three raw build behaviors were independently re-observed this
session (not merely relied upon from the coordinator's brief) to corroborate
the DoD's "not a real version" enumeration end to end:

```
$ go build -o curlew-plain ./cmd/curlew && ./curlew-plain --version
curlew 0.1.0-dev

$ go build -buildvcs=false -o curlew-novcs ./cmd/curlew && ./curlew-novcs --version
curlew 0.1.0-dev

$ go build -ldflags "-X main.version=9.9.9" -o curlew-ldflag ./cmd/curlew && ./curlew-ldflag --version
curlew 9.9.9
```

## Behaviors Verified

| # | Behavior | Test(s) | Status |
|---|----------|---------|--------|
| 1 | ldflag wins; fallback not consulted | `TestVersion_is_injectable_at_link_time`, `TestVersion_falls_back_to_build_info/ldflag_wins_over_build_info`, `.../ldflag_wins_even_when_build_info_is_junk` | PASS |
| 2 | `go install <module>@<tag>` reports the tag | `TestVersion_tagged_build_reports_the_tag` (documented faithful proxy — real `go install @<tag>` is unreachable today because the only tag, `v0.1.0`, predates this fix; plan D7) | PASS |
| 3 | Plain `go build` reports the default (pseudo-version/`(devel)` rejected) | `TestVersion_default_when_not_injected` (exact equality, `-buildvcs=false`), `TestVersion_falls_back_to_build_info/devel_is_not_a_version`, `.../pseudo-version_from_a_commit_after_a_tag`, `.../pseudo-version_from_an_untagged_repo`, `.../pseudo-version_from_a_pre-release_base` | PASS |
| 4 | Build-info form matches goreleaser's (`v` stripped) | `TestVersion_build_info_form_matches_goreleaser` | PASS |
| 5 | `--version`, `--help`, `info --format json` agree | `TestVersion_all_surfaces_agree`, `TestVersion_injection_reaches_help`, `TestVersion_injection_reaches_info_json` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | A test proves an installed binary reports its module version | `TestVersion_falls_back_to_build_info` subtests `installed_at_a_tag`/`installed_at_a_pre-release_tag`; `TestVersion_tagged_build_reports_the_tag` (real tag, real build, real `--version`) — all PASS | PASS |
| 2 | A test proves the ldflag still takes precedence over the fallback | `TestVersion_is_injectable_at_link_time` unmodified and passing; `ldflag_wins_over_build_info`/`ldflag_wins_even_when_build_info_is_junk` subtests PASS | PASS |
| 3 | A test proves pseudo-versions, `(devel)`, empty, and absent build info all keep the default | 10 of 17 `TestVersion_falls_back_to_build_info` subtests cover exactly this set, all PASS | PASS |
| 4 | A test proves the fallback and goreleaser forms are byte-identical | `TestVersion_build_info_form_matches_goreleaser` parses `.goreleaser.yaml`'s ldflags template and asserts `resolveVersion` reproduces it — PASS | PASS |
| 5 | `--version`, `--help`, `info --format json` verified to agree | `TestVersion_all_surfaces_agree` — PASS | PASS |
| 6 | `TestVersion_default_when_not_injected` no longer passes for `(devel)` | Read `cmd/curlew/version_test.go:142-154`: now builds with `-buildvcs=false` and asserts **exact equality** (`got != want`) against `"curlew "+defaultVersion`, not a prefix/non-empty check | PASS |
| 7 | `./scripts/ci-local.sh --go` passes | Full (non-`--go`) gate run this session passed, `rc=0` — a strict superset of `--go`'s steps, per coordinator instruction to treat the full run as authoritative | PASS |
| 8 | Coverage >= 80% | `go coverage` step: 86.2% total, 81.1% `cmd/curlew` | PASS |
| 9 | CHANGELOG.md updated | `## [Unreleased]` / `### Fixed` carries a full, specific M25-004 entry (version-fallback mechanism + the README correction found in review) — already present from `/execute`/`/improve`; no further edit needed at verify time | PASS |

## Code Review

Branch A: a PASS review already exists
(`management/reviews/M25-004-review.md`, iteration 3, commit `ff4b627`,
dated 2026-08-18). Trusted, with three spot-checks performed independently
this session rather than re-reading the review's own claims:

| Check | Status | Note |
|-------|--------|------|
| Error handling site | PASS | `buildFixtureBinary`/`buildTaggedFixture` (`version_test.go`) use `t.Fatalf` with contextual messages and distinguish "our resolver is broken" from "the toolchain didn't stamp" via explicit guard comments — idiomatic for test helpers. Production `resolveVersion` adds no new error-return paths; by construction it never panics and never returns `""`. |
| Exported-symbol doc comments | PASS | Every identifier in `cmd/curlew/version.go` (`defaultVersion`, `version`, `resolvedVersion`, `releaseVersionRE`, `pseudoVersionRE`, `buildInfoVersion`, `resolveVersion`, `usableBuildVersion`, `trimVersionPrefix`) carries a doc comment explaining intent, not just behavior (e.g. the pseudo-version regex comment enumerates Go's three canonical forms and why the char class matches all of them). |
| Test correctness (tests what it claims) | PASS | Read `TestVersion_all_surfaces_agree` in full: it builds a real binary, actually parses `--version`, the `--help` output's `Version:` line, and `info --format json`'s version field, and compares all three — not a vacuous check. |

Spot-check did not surface any issue; Branch A stands (no need to fall back
to a full Branch B review).

## Commits

22 commits on `feature/M25-004-buildinfo-version`, oldest first, all
referencing M25-004:

| Hash | Message |
|------|---------|
| ee18291 | chore(task): add M25-004 for build-info version derivation |
| 32d750f | docs(plan): add implementation plan for M25-004 |
| 80cf592 | chore(task): mark M25-004 as planned |
| bd3e60c | fix(task): M25-004's (devel) premise was wrong; go build stamps a pseudo-version |
| f55056c | chore(task): mark M25-004 as in_progress |
| 78289d9 | test(cli): add failing tests for the build-info version resolver |
| 6769454 | feat(cli): fall back to build-info version when unstamped |
| 68e85d4 | docs(plan): record two deviations found during /execute for M25-004 |
| 5092462 | test(cli): pin goreleaser's version template offline |
| 014f787 | test(cli): guard against an eighth surface reading the raw version symbol |
| e8a9b1b | test(cli): repoint the three test-side reads of the raw version symbol |
| b9c74a2 | test(cli): end-to-end fixture proves a tagged build reports the tag |
| 7b79675 | fix(cli): rewrite the vacuous default-version test and the prose it licensed |
| 718ec0d | docs(plan): record E3 - docs/MANUAL.md needed no change for M25-004 |
| 3059f3c | chore(task): mark M25-004 as review |
| 47a7133 | docs(review): add review with findings for M25-004 |
| 7652b24 | fix(cli): correct the README-exec test's false next-tag claim |
| ef058fe | docs(review): add improvement report for M25-004 |
| 6a8c730 | docs(review): add review with findings for M25-004 (iteration 2) |
| 912cf06 | fix(docs): correct README's false claim about go install at v0.1.0 |
| 86602d9 | docs(review): append iteration-2 improvement report for M25-004 |
| ff4b627 | docs(review): add passing review for M25-004 (iteration 3) |

TDD pattern visible: `78289d9 test(cli): add failing tests...` precedes
`6769454 feat(cli): fall back to build-info...` (RED before GREEN).
Conventional commit format (`type(scope): description`) used throughout.
Review/improve cycle visible: two rounds of findings (iterations 1 and 2)
each followed by a fix commit, then a clean PASS on iteration 3.

## Files Changed

| File | Action | Lines +/- |
|------|--------|-----------|
| `cmd/curlew/version.go` | created | +88 |
| `cmd/curlew/version_test.go` | modified | +575/-~ |
| `cmd/curlew/main.go` | modified | +23/-~ (7 call sites repointed) |
| `cmd/curlew/main_test.go` | modified | +5/- |
| `cmd/curlew/ui.go` | modified | +2/-1 |
| `cmd/curlew/release_artifacts_test.go` | modified | +11/- |
| `cmd/curlew/skill_playbook_test.go` | modified | +10/- |
| `cmd/curlew/readme_install_exec_test.go` | modified | +59/- |
| `cmd/curlew/readme_install_test.go` | modified | +19 |
| `README.md` | modified | +9/- |
| `CHANGELOG.md` | modified | +91 |
| `scripts/ci-local.sh` | modified | +11/- |
| `management/tasks/M25-004.yaml` | created | +109 |
| `management/plans/M25-004-plan.md` | created | +1151 |
| `management/plans/M25-004-improved.md` | created | +129 |
| `management/reviews/M25-004-review.md` | created | +336 |
| `management/backlog.yaml` | modified | +6 |

17 files changed, 2576 insertions(+), 58 deletions(-) (`git diff --stat main...HEAD`).

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
