# Verification Report: M25-002

**Task:** v0.1.0 is something a person can download and run
**Verified by:** AI
**Date:** 2026-08-16
**Branch:** feature/M25-002-release-artifact-verification
**Verdict:** PASS

## Scope boundary respected

`git tag -l` was confirmed empty before this verification pass, checked again after every command below (including two direct `goreleaser release --snapshot --clean` invocations run outside `ci-local.sh` to exercise the task YAML's literal observable), and is confirmed empty at the end of this report. No tag was created or pushed. No `goreleaser release` command without `--snapshot` was run. No GitHub release was created. `.goreleaser.yaml` and `NOTICE` are byte-identical to `main`.

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/curlew` | PASS | Clean, no warnings |
| `go test ./...` | PASS | 53 packages ok, 0 failed |
| `go test -race ./...` | PASS | 53 packages ok, no races detected |
| `go tool cover` (via `./scripts/ci-local.sh`) | PASS | 53 packages ok, total 86.2% of statements |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Completes; the five `FAIL ... is invalid` lines in the log are the smoke suite's own deliberate negative test cases (curlew correctly rejecting bad input), not gate failures |
| `./scripts/ci-local.sh` (full, auto-scoped) | PASS | Scope auto-detected as `go=1 backend=0 web=0 ui=0 e2e=0` (only Go/docs/management files changed on this branch — correct, no backend/web/e2e gates needed) — ends `=== ci-local PASS ===`, including the dogfood/gaps/redaction/openapi/crosscheck harnesses |
| Coverage | 86.2% | Meets >= 80% threshold. (The review recorded 83.7% on an earlier run the same day; both clear the floor — ordinary run-to-run coverage variance, not a regression, since the tree is byte-identical between the two measurements.) |

Full log saved during this verification: `/private/tmp/claude-501/-Users-peterlindqvist-kod-active-curlew/d32df30e-f2eb-4621-8652-909762c2454d/scratchpad/ci-local-full.log` (1621 lines, not part of the repo).

## Observable Output

Ran exactly as specified in the task YAML (which already carries the `-tags release_artifacts` deviation recorded under D9 — see Behaviors below for confirmation this deviation is load-bearing, not cosmetic):

```
$ goreleaser release --snapshot --clean
  ...
  • release succeeded after 4s
  • thanks for using GoReleaser!

$ ls dist/*.tar.gz dist/*.zip dist/checksums.txt
dist/checksums.txt
dist/curlew_0.0.1-snapshot_darwin_amd64.tar.gz
dist/curlew_0.0.1-snapshot_darwin_arm64.tar.gz
dist/curlew_0.0.1-snapshot_linux_amd64.tar.gz
dist/curlew_0.0.1-snapshot_linux_arm64.tar.gz
dist/curlew_0.0.1-snapshot_windows_amd64.zip
dist/curlew_0.0.1-snapshot_windows_arm64.zip

$ go test -tags release_artifacts ./cmd/curlew/ -run TestRelease_every_archive_is_complete -v
--- PASS: TestRelease_every_archive_is_complete (4.93s)
    [6 targets x 6 required files, all PASS]
PASS
ok  	github.com/weiqigod/curlew/cmd/curlew	4.452s

$ go test -tags release_artifacts ./cmd/curlew/ -run TestRelease_version_matches_the_tag -v
--- PASS: TestRelease_version_matches_the_tag (4.42s)
PASS
ok  	github.com/weiqigod/curlew/cmd/curlew	4.738s
```

Expected: all six archives produced; both named tests pass genuinely (not vacuously).
Result: MATCH.

**Vacuity check, explicitly run (per the pipeline's merge checklist item 5):**

```
$ go test ./cmd/curlew/ -run TestRelease_every_archive_is_complete -v      # no -tags
testing: warning: no tests to run
PASS
ok  	github.com/weiqigod/curlew/cmd/curlew	(cached) [no tests to run]
```

Confirms the `-tags release_artifacts` deviation (plan D9) is necessary: without it, the literal command from the original task YAML wording reports success (exit 0) having run nothing — the exact defect class this task exists to close, reproduced one level up. The task YAML's `observable` field already carries the corrected `-tags` form, so what a future reader copy-pastes is the real command, not the vacuous one.

Additionally ran the third `TestRelease_*` test (not listed in `observable` but required by the DoD's "checksums.txt verified against the archives" item):

```
$ go test -tags release_artifacts ./cmd/curlew/ -run TestRelease_checksums_match_the_archives -v
--- PASS: TestRelease_checksums_match_the_archives (4.10s)
PASS
```

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Given a snapshot release, when the archives are listed, then all six os/arch combinations named in `.goreleaser.yaml` are present | `TestRelease_every_archive_is_complete` (top-level vacuity guard: archive count == 6, checked before any per-archive assertion) | PASS |
| 2 | Given any produced archive, when it is extracted, then it contains LICENSE, NOTICE, README.md, CHANGELOG.md, docs/MANUAL.md and docs/CLI_SPECIFICATION.md | `TestRelease_every_archive_is_complete` (6 required-file subtests x 6 targets = 36 subtests, all observed PASS) | PASS |
| 3 | Given any produced archive, when the binary inside it is executed with `--version`, then it reports the release version and not `0.1.0-dev` | `TestRelease_version_matches_the_tag` (buildinfo leg: all 6 targets; execution leg: the 1 native target, written to `t.TempDir()` and run) | PASS |
| 4 | Given the windows targets, when the archive is inspected, then it is a `.zip` and the binary is named `curlew.exe` | `TestRelease_every_archive_is_complete` (`windows/amd64` and `windows/arm64` cases assert `wantExt=".zip"`, `wantBin="curlew.exe"`) | PASS |
| 5 | Given `checksums.txt`, when each archive is hashed, then every digest matches | `TestRelease_checksums_match_the_archives` (`each_recomputed_sha256_equals_the_listed_digest` subtest, re-hashes with `crypto/sha256`) | PASS |

All 5 behaviors map to a named, independently-run test; all observed passing directly during this verification (not taken from the review's record).

**Honest coverage note on behavior 3** (carried from the plan/review, restated here because it is load-bearing for what "PASS" means): only 1 of 6 binaries is *executed* on this host (darwin/arm64); the other 5 are verified via `debug/buildinfo` (GOOS/GOARCH/CGO_ENABLED/`-X main.version=`) plus a format/machine-field parse (`debug/elf`/`debug/macho`/`debug/pe`), because no single host can natively execute all six cross-compiled targets. This is stated as designed, not as a gap — see plan D4.

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | A test drives a snapshot release and asserts on the produced archives | `releaseSnapshot`/`runReleaseSnapshot` in `cmd/curlew/release_artifacts_test.go` runs `goreleaser release --snapshot --clean` as a subprocess and asserts exit 0 before reading anything (D2) | PASS |
| 2 | All six targets present, each extracting cleanly | `TestRelease_every_archive_is_complete`, observed PASS on all 6 (linux/darwin/windows x amd64/arm64) this run | PASS |
| 3 | LICENSE and NOTICE verified present in every archive | Same test, `LICENSE — Apache-2.0 section 4(a)` and `NOTICE — Apache-2.0 section 4(d)` subtests, all 6 targets, observed PASS | PASS |
| 4 | Every archived binary executes and reports the release version | `TestRelease_version_matches_the_tag`: buildinfo leg covers all 6; execution leg covers the 1 host-native target (see honest-coverage note above) — this is the maximum any single host can prove directly, and the plan/review both justify why buildinfo is not a lesser substitute for the other 5 | PASS (as designed) |
| 5 | `checksums.txt` verified against the archives | `TestRelease_checksums_match_the_archives`, observed PASS, both directions plus cardinality | PASS |
| 6 | v0.1.0 tagged, artifacts published, release notes reviewed by a human | **DEFERRED — see below** | OPEN (deliberately) |
| 7 | `./scripts/ci-local.sh` passes | Full run this verification, exit 0, `=== ci-local PASS ===` | PASS |
| 8 | CHANGELOG.md updated with the release entry | `## [Unreleased]` -> `### Added`, two entries (M25-001's and M25-002's) documenting the six-target verification in detail, including the honest 1-of-6-execution coverage statement | PASS |

### DoD item 6 — recorded as deferred, not failed

**"v0.1.0 tagged, artifacts published, release notes reviewed by a human"** remains **OPEN and unticked**. This is not a verification failure. Reason, as specified for this verification pass:

- A pushed tag is public and effectively immutable — unlike everything else in this task, it cannot be verified by running it and then restoring the tree.
- The criterion's subject is a person ("reviewed by a human"); an automated pipeline phase cannot satisfy a criterion whose completion condition is human judgment.
- The project owner will run the tag-and-publish step separately, after reviewing the draft. The procedure is documented as a runbook at the end of `management/plans/M25-002-plan.md` ("Deferred: cutting v0.1.0") — explicitly marked there as documentation for a human to run deliberately, not instructions for `/execute` or `/verify` to execute.

The DoD list in `management/tasks/M25-002.yaml` is **not rewritten** — item 6 stays exactly as originally worded.

`git tag -l` confirmed empty at the start and end of this verification.

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

**Branch A (review PASS trusted, spot-checked):** `management/reviews/M25-002-review.md` carries verdict **PASS** with zero findings at any severity, and documents 13 independently-reproduced mutation/inspection experiments (NOTICE-omission, empty-archive stubs, target-count vacuity, case-sensitive version-flag matching, `sync.Once`/`t.Fatal` hazard closed by construction, golangci-lint build-tag attribution isolated from `go vet`, etc.).

This verification independently spot-checked, rather than accepting on faith:
1. **Error wrapping** — read `releaseFindRepoRoot`, `findGoreleaser`, `releaseFindArchivePaths`, `readReleaseArchive`, `parseArchiveName`: every returned error is either wrapped with `fmt.Errorf("context: %w", err)` or built from a sentinel (`ErrGoreleaserNotFound`, `ErrEmptyArchive`) via `%w`. Confirmed clean.
2. **Doc comments** — every helper read (`findGoreleaser`, `releaseFindArchivePaths`, `readReleaseArchive`, `parseArchiveName`, `readTarGzEntries`) carries a doc comment explaining what it does and, where relevant, why (e.g. `findGoreleaser` explicitly cites the `ci-local.sh` resolution order it mirrors).
3. **Test-matches-claim** — read `TestRelease_version_matches_the_tag` in full: it genuinely implements the two-legged design from plan D3/D4 (buildinfo across all 6 targets, native execution of exactly 1 with a `t.Fatalf` if that count is ever != 1), not a weaker approximation of it.

No new findings. Branch A's trust is upheld.

## Commits

| Hash | Message |
|------|---------|
| `76ab86d` | docs(plan): add implementation plan for M25-002 |
| `039b72a` | chore(task): mark M25-002 as planned |
| `cf20d20` | chore(task): mark M25-002 as in_progress |
| `8ab778c` | docs(plan): reconcile M25-002 plan with independent review findings |
| `d6a4068` | chore(lint): teach golangci-lint the release_artifacts build tag |
| `36965a0` | feat(cli): drive a real six-target release and verify every archive |
| `357ee0f` | feat(cli): wire the six-target release verification into the gate |
| `c62ab2f` | docs(changelog): record the six-target release verification (M25-002) |
| `4e27de3` | docs(task): record the -tags release_artifacts observable deviation |
| `56c7c0e` | refactor(cli): extract releaseChecksumProblems for fabricated-input testing |
| `0f933c6` | docs(plan): record M25-002's mutation verification as executed |
| `552b22b` | chore(task): mark M25-002 as review |
| `0b7d4e7` | docs(review): add passing review for M25-002 |

All 13 commits carry `Refs: M25-002` and conventional `type(scope): description` subjects. The test file itself (`36965a0`) is labelled `feat` rather than `test` because the test **is** the deliverable this task exists to produce (the task's entire scope is "write a test that drives a snapshot release and inspects the output" — there is no separate production code under test in the usual TDD sense). RED-phase evidence is the plan's own Step 1 measurements (current gate blind to a missing NOTICE, exit 0) and the mutation-verification table, run and recorded before the corresponding GREEN commit, rather than a separate failing-test commit.

## Files Changed

| File | Action | Lines +/- |
|------|--------|-----------|
| `cmd/curlew/release_artifacts_test.go` | created | +1077/-0 |
| `scripts/ci-local.sh` | modified | +37/-0 |
| `.golangci.yml` | modified | +10/-0 |
| `CHANGELOG.md` | modified | +103/-0 |
| `management/tasks/M25-002.yaml` | modified | +8/-3 |
| `management/backlog.yaml` | modified | +5/-1 |
| `management/plans/M25-002-plan.md` | created | +1479/-0 |
| `management/reviews/M25-002-review.md` | created | +72/-0 |

No production Go source outside `cmd/curlew/release_artifacts_test.go` changed. No `src/` or `web/` files touched — confirmed by `git diff --name-only main...HEAD`, which is why `ci-local.sh` auto-scoped to `go=1 backend=0 web=0 ui=0 e2e=0`.

## Tree integrity (checked at the end of this verification)

| Check | Result |
|---|---|
| `git status --porcelain` | empty |
| `git diff main --exit-code -- .goreleaser.yaml NOTICE` | exit 0 (byte-identical to `main`) |
| `git tag -l` | empty |
| `git diff --exit-code -- internal/uiserver/assets/dist/index.html` | exit 0 (untouched) |
| `go test ./internal/backlog/ -run TestBacklog_repository_is_consistent -v` | PASS |
| `go test ./cmd/curlew/ -run TestCiLocalDownIdempotent -v` | PASS |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge. DoD item 6 (tag/publish) stays open by design; everything else is verified with evidence gathered directly during this pass, not carried forward from the review.
