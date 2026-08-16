# Code Review: M25-002

**Task:** v0.1.0 is something a person can download and run
**Reviewer:** AI
**Date:** 2026-08-16
**Branch:** feature/M25-002-release-artifact-verification

## Verdict: PASS

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| — | — | — | — | — | None | — |

No findings at any severity. Every claim in the plan, the CHANGELOG, and the code comments that was checkable by direct measurement was independently reproduced during this review, live against this tree, not accepted from the written record. See "Independent verification performed" below for what was run and what came back.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All returned errors wrapped with `fmt.Errorf("...: %w", err)`; three sentinel errors (`ErrGoreleaserNotFound`, `ErrNoArchives`, `ErrEmptyArchive`) used with `errors.Is` semantics — confirmed directly (see Vacuity below). `defer`-based `Close()` handling never overwrites a real error with a close error. No panics for expected failures. |
| Input Validation | PASS | Regex non-match, empty `builds`/`archives`/`files` lists, an `ignore:` key, an absent `-X` flag, an empty flag value, and a malformed `checksums.txt` line are all explicit errors, never a silent skip or a panic. |
| Naming | PASS | Package `main`, no exported surface beyond the required `Test*` functions. New identifiers are deliberately prefixed `release`/`archive`/`checksum` to avoid colliding with `buildBinary`/`runBinary*`/`testVersion` already declared in untagged sibling files compiled into the same binary — confirmed no redeclaration by the fact that every `-tags release_artifacts` build in this review compiled cleanly. Every exported-by-necessity `Test*` function and every significant helper carries a doc comment. |
| Code Organization | PASS | Single file, single responsibility, `defer` for every `Close()`, `context.WithTimeout` propagated into `exec.CommandContext`. `golangci-lint` (with `run.build-tags: [release_artifacts]` applied — confirmed via `Using build tags: [release_artifacts]` in verbose output) reports 0 issues; `golangci-lint fmt --diff` reports no gofumpt drift. |
| Correctness | PASS | No `t.Parallel()` anywhere in the file, so the shared `sync.Once`-cached release result is never accessed concurrently. `runReleaseSnapshot` and everything it transitively calls (`findGoreleaser`, `releaseFindRepoRoot`, `releaseFindArchivePaths`, `readReleaseArchive`, `parseArchiveName`, `readTarGzEntries`, `readZipEntries`, `readZipEntry`, `loadChecksums`) take no `*testing.T` at all — structurally incapable of calling `t.Fatal`/`t.Skip`/Goexiting, not merely disciplined not to. |
| Test Quality | PASS | Table-driven where there are repeated cases, descriptive `t.Run` names throughout, assertions check specific message substrings rather than bare `err != nil`, and the test drives a real `goreleaser` subprocess and executes a real cross-compiled binary via `os/exec` — as real an integration test as this claim admits. All five behaviors in the task YAML map to a named test (see Spec Compliance). |

## Independent verification performed

Per the pipeline's instructions, the plan, CHANGELOG, and code comments were treated as claims to reproduce, not facts to accept. Every mutation below was applied to the working tree, run, and restored; `.goreleaser.yaml`/`NOTICE` were confirmed byte-identical to `main` after each one via `git diff main --exit-code`, and `git status --porcelain` was confirmed empty after every experiment, including the two that used temporary scratch files (both removed).

| # | Check | Method | Result |
|---|---|---|---|
| 1a | Both observables genuinely select and run tests | Ran both `go test -tags release_artifacts ./cmd/curlew/ -run <name> -v` commands as literally written in the task YAML | `TestRelease_every_archive_is_complete`: 42 subtests, PASS, 4.71s. `TestRelease_version_matches_the_tag`: PASS, 3.73s. Confirmed (again) that the same command **without** `-tags` reports `no tests to run`, exit 0 — the exact vacuity D9 exists to close. |
| 1b | The `-list` count guard fails on a typo | Ran the guard's exact shell logic with the tag misspelled (`release_artifactz`) and with the `-run`/`-list` pattern misspelled (`^TestReleaseXYZ`) | Both produced a count of `0` (not `3`), which the guard's `if [ "$release_tests" != "3" ]` correctly turns into a failure. |
| 2 | The Apache-2.0 floor is independent of `.goreleaser.yaml` | Deleted `- NOTICE` from `archives[].files`, ran `TestRelease_every_archive_is_complete` | **FAILED**, on all 6 archives, each with `missing NOTICE — required regardless of current .goreleaser.yaml content (Apache-2.0 section 4(d)...)`. This is the mutation that matters, and it fails exactly as claimed. |
| 3a | A zero-entry archive fails loudly | Deleted the `NOTICE` file itself, ran `goreleaser release --snapshot --clean` directly, reproducing the 32-byte `.tar.gz` / 22-byte `.zip` stubs. Added a temporary test file calling the project's own `readTarGzEntries` and `readReleaseArchive` against those stub bytes, then deleted the temp file. | Confirmed Go's stdlib reads the stub as **0 entries, nil error** (the underlying hazard). Confirmed the project's own `readReleaseArchive` **does** reject it — `archive contains no entries: curlew_...tar.gz`, wrapping `ErrEmptyArchive` (`errors.Is` succeeds). |
| 3b | Checksum cardinality checked before set-difference | Ran the shipped fabricated subtest and read `releaseChecksumProblems` | `both_sets_empty_is_not_silently_clean` PASSES; source confirms both cardinality checks run unconditionally before either set-difference loop, so two empty sets still produce 2 problems, not 0. |
| 4a | buildinfo leg covers all six targets | Read the per-archive loop; independently confirmed via the case-sensitivity mutation below, whose failure output names all 6 archives by exact filename | All six: darwin_amd64, darwin_arm64, linux_amd64, linux_arm64, windows_amd64, windows_arm64. |
| 4b | Exec leg fails rather than skips when no target matches the host | Temporarily changed the native-match condition to `a.GOOS == "no-such-os" && ...` so `nativeCount` could never reach 1, ran `TestRelease_version_matches_the_tag` | **FAILED** with `expected exactly 1 archive matching this host's runtime.GOOS/GOARCH (darwin/arm64), found 0` — a named `t.Fatal`, not a skip. |
| 4c | `-X main.version=` match is literal and case-sensitive | Mutated `.goreleaser.yaml`'s ldflags to `-X main.Version=` (capital V) | **FAILED** on all 6 archives: `-ldflags "-s -w -X main.Version=0.0.1-snapshot" does not contain "-X main.version="`. Confirms the linker silently dropped the flag (buildinfo faithfully echoed the literal text) and the case-sensitive match caught it rather than loose-matching. |
| 5a | golangci-lint genuinely lints the tagged file | Planted a discarded `os.Setenv(...)` return (errcheck, deliberately not govet/printf) inside `TestRelease_every_archive_is_complete` | `golangci-lint run` reported exactly `cmd/curlew/release_artifacts_test.go:658:11: Error return value of \`os.Setenv\` is not checked (errcheck)`. Removed; lint returned to `0 issues`. |
| 5b | Attribution isolation (not a `go vet` catch) | Ran the same planted violation through `go test -tags release_artifacts` (no golangci-lint) | Exit 0 — `go test`'s built-in vet subset does not flag a discarded `os.Setenv`, confirming the earlier catch is attributable to golangci-lint's `errcheck` linter picking up the build-tagged file, not to `go vet`. |
| 6 | `sync.Once` + `t.Fatal` hazard | Read `runReleaseSnapshot`'s full call graph; grepped the file for every `t.Fatal`/`t.Skip`/`runtime.Goexit` occurrence and its enclosing function | `runReleaseSnapshot` and everything it calls take **no** `*testing.T` parameter — structurally cannot reach a `t.Fatal` call. Every `t.Fatal(f)` in the file lives in `releaseSnapshot`, `assertArchiveMachine`, or a `Test*`/`t.Run` closure, all of which do have `t`. The belt-and-braces `rel.Err == nil && len(rel.Archives) == 0` guard in `releaseSnapshot` is present. |
| N2 (bonus) | Target floor is also independent of the config | Deleted `- windows` from `builds[].goos` | **FAILED** at the top vacuity guard: `got 4 archives, want exactly 6`, matching the plan's own recorded execution evidence exactly. |

### Assessment on the merits

- **Gate cost.** Timed the exact new step (the `-list` guard plus `go test -tags release_artifacts -run '^TestRelease' ./cmd/curlew/ -count=1 -v`) standalone with a warm build cache: ~2s + ~6s ≈ 8s. Consistent with `ci-local.sh`'s own comment ("~9s standalone, warm cache") and in the same range as the CHANGELOG's "6.5s" figure — the small spread across separate measurement sessions is ordinary wall-clock variance for a step that shells out to `go mod download` and a real compiler toolchain six times, not a discrepancy worth flagging. Placement (unconditional step, build-tagged file, after M25-001's `release_bin_count` guard) is the right call: it is cheap on a warm cache, and every `-list`/typo experiment above confirms it is a real, wired-in, always-run step rather than an escape hatch.
- **Ordering.** Confirmed via `grep`: `release_bin_count` guard at `ci-local.sh:295-303`, new step begins at line 322, `# --- Dogfood gate` comment at line 359. The only occurrences of `dist` after line 322 are inside the new step's own explanatory comments (339, 341, 350) — nothing after it reads `dist/`.
- **The CHANGELOG's `[0.1.0]`-heading reasoning.** Confirmed no `## [0.1.0]` heading exists anywhere in `CHANGELOG.md`; the entry lives under `## [Unreleased]` → `### Added`. The reasoning holds: `CHANGELOG.md` ships inside every archive including snapshot ones, and a `[0.1.0]` heading written before the tag exists would have shipped inside `0.0.1-snapshot`-labelled archives, announcing a release the binary next to it does not agree happened. Recorded explicitly as Deviation 7 in the plan rather than silently — properly recorded.
- **`releaseChecksumProblems` extraction.** Sound, not a weakening. It is the same function backing both the fabricated unit tests (N5, the empty/empty vacuity case) and the live end-to-end assertion against a real release's `archiveNames`/`rel.Checksums` — not a parallel reimplementation that could quietly drift from what the live path does. Given D2's own design (the test always calls `--clean` and rebuilds, and `rel.Checksums` is parsed into memory once rather than re-read from disk at assertion time), there is no way to inject a live post-build mutation without weakening D2's "never read a `dist/` this run did not produce" guarantee — the plan states this limitation honestly rather than omitting the N5 case, and the extraction is what makes the fabricated coverage cheap and exactly-once-implemented.

## Test Coverage

- Overall repository statement coverage: 83.7% (`go test -coverprofile`, untagged — meets the project's 80% floor).
- `cmd/curlew` with `-tags release_artifacts`: 81.1% of statements (this package's ordinary, non-test source, exercised while the tagged tests also run).
- The new logic itself lives entirely inside `cmd/curlew/release_artifacts_test.go`, and Go's coverage tooling does not instrument `_test.go` files (confirmed: the profile contains zero entries for this file) — there is no meaningful line-coverage percentage to report for it, because it *is* the test, not code under test. Its correctness was instead verified behaviourally, via the eleven independent mutation/inspection experiments in the table above, all reproduced live during this review rather than taken from the written record.
- Missing coverage: none identified. All five `behaviors` in the task YAML map to a named test (target floor, required-files floor, version-not-default, windows zip/`curlew.exe`, checksums bidirectional).

## Pre-audit gate

`./scripts/ci-local.sh --go` — **PASS** (exit 0), full output ending `=== ci-local PASS ===`, including the dogfood/redaction/openapi/crosscheck harnesses.

## Scope boundary respected

No git tag was created or pushed at any point in this review — `git tag -l` was empty before, during (checked after each mutation), and after. `.goreleaser.yaml` and `NOTICE` are byte-identical to `main` (`git diff main --exit-code -- .goreleaser.yaml NOTICE`, exit 0). `git status --porcelain` is empty. The task YAML's deferred DoD item (`v0.1.0 tagged, artifacts published, release notes reviewed by a human`) remains correctly un-actioned; this is expected per the plan's own scope boundary and is not a review failure.

## Summary

Every checkable claim in the plan, the CHANGELOG entry, and the code's own comments was independently reproduced against this tree during this review — the Apache-2.0 floor failing exactly as described when `NOTICE` is un-declared, the zero-entry stub hazard and its guard both reproduced against real bytes, the case-sensitive version-flag match, the exec-leg's fail-not-skip behaviour, the golangci-lint build-tag wiring (with attribution isolated from `go vet`), and the `sync.Once`/`t.Fatal` hazard closed by construction (no `*testing.T` in scope for the once-function's call graph). No discrepancy between what was written and what was measured turned up anywhere. Zero findings at any severity.
