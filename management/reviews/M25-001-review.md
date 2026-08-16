# Code Review: M25-001

**Task:** Exercise the release build before the day it is needed
**Reviewer:** AI
**Date:** 2026-08-16
**Branch:** feature/M25-001-release-build-gate

## Verdict: FAIL

## Pre-audit Gate

`./scripts/ci-local.sh --go` — **PASS** (exit 0, `=== ci-local PASS ===`; re-run
in full by this review, not just trusted from the branch state). Continuing to
the static audit.

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| 1 | Critical | Documentation Accuracy | `CHANGELOG.md` | 36–42 | Repeats a false, already-retracted measurement: "measurement showed `goreleaser build --snapshot` accepts it silently and links a binary carrying the same `0.1.0-dev` default instead — so the `--version` assertion catches that case too." This is the exact claim `management/plans/M25-001-plan.md` retracted in commit `05ce503` after it failed to reproduce four times. It still does not reproduce: independently re-run for this review, `goreleaser build --snapshot --clean --single-target` on the `{{ .Version }}` → `{{ .NoSuchVar }}` mutation exits **1** with `build failed: template: failed to apply "-s -w -X main.version={{ .NoSuchVar }}": map has no entry for key "NoSuchVar"`, and leaves zero files named `curlew` under `dist/` — it does not link a binary at all, silently or otherwise. `05ce503` corrected `management/plans/M25-001-plan.md` but never touched `CHANGELOG.md`, which was last written in `bd9b254`, before the retraction. `git log main..HEAD -- CHANGELOG.md` shows only that one commit. | Rewrite CHANGELOG.md lines 36–42 to match the corrected plan record: the undefined-template-variable mutation fails at the **build** step itself (`goreleaser build` exits 1, `dist/` stays empty), not at the `--version` assertion. Only the ldflag-*deletion* mutation (drop `-X main.version=...` entirely) passes both `check` and `build` and is caught exclusively by the `--version` assertion — that is the one true "silently ships 0.1.0-dev" case, and the CHANGELOG's own preceding paragraph (lines 31–35) already describes it correctly. |
| 2 | Low | Error Handling | `scripts/ci-local.sh` | 286 | The `release_bin_count` vacuity guard's clear diagnostic ("expected exactly one built curlew under dist/, found N") is bypassed in the one case it seems designed to cover most explicitly: `dist/` not existing at all. `find dist -type f -name curlew` exits non-zero when `dist` is missing; under `pipefail`, that failure propagates through `| wc -l | tr -d ' '`; and because the whole pipeline is the right-hand side of a bare assignment (`release_bin_count="$(...)"`), `set -e` aborts the script at that line — before the `if [ "$release_bin_count" != "1" ]` check runs. Verified directly: running the assignment and `if` in a subshell against a nonexistent `dist/` prints only `find: dist: No such file or directory` and exits 1; "expected exactly one built curlew..." is never printed. The script still fails closed (never a false PASS/no-op, which is the property the comment at :283–286 actually promises), but the friendlier message the code appears to have been written to guarantee does not fire in this one path. In the real gate flow this is very hard to reach — the preceding `goreleaser_cmd build` step must itself have exited 0 for control to reach this line, and goreleaser's own `--verbose` output shows it creates `dist/` ("ensuring distribution directory") unconditionally as an early pipeline step, before any binary is built — so this is closer to a documented edge case than a live risk. | Either note in the comment that a missing `dist/` falls through to `find`'s own stderr rather than the custom message (harmless, since the exit code is still 1), or make the guard resilient with something like `release_bin_count="$(find dist -type f -name curlew 2>/dev/null \| wc -l \| tr -d ' ')" || release_bin_count=0`. |
| 3 | Low | CI Configuration / Documentation | `.github/workflows/go.yml` | 45–46 | `go install github.com/goreleaser/goreleaser/v2@v2.17.1` runs under the toolchain from `go-version-file: go.mod` (`go 1.24.0`), while goreleaser v2.17.1's own `go.mod` declares `go 1.26.5` (confirmed from the cached module source: `go.mod` header reads `go 1.26.5`). This works — Go's toolchain management (`GOTOOLCHAIN=auto`, the default; confirmed not overridden anywhere in this repo) transparently downloads and uses go1.26.5+ for that one `go install` invocation. I verified this empirically: the local toolchain cache at `$(go env GOPATH)/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.5...` and `...go1.26.6...` exists, produced by exactly this auto-download mechanism, and `go version -m` on the resulting `goreleaser` binary confirms it was built with go1.26.6. So the step is not broken. But `go.yml`'s comment says nothing about this version gap, while `release.yml`'s comment for the *identical* underlying mismatch explains it in detail and deliberately avoids `go install` for that reason (`release.yml:47-51`). A future reader comparing the two files has no way to tell the go.yml choice was deliberate rather than an oversight, and the step silently costs an extra toolchain download (network + ~untracked seconds) on every gate run that nothing in the diff accounts for. | Add a one-line comment to `go.yml`'s "Install goreleaser" step noting that `go install` auto-fetches the go1.26.5+ toolchain goreleaser requires (via `GOTOOLCHAIN=auto`), which is why the same Go-version gap that made `release.yml` avoid `go install` doesn't block it here. |

## Verified claims (not findings — recorded because the brief asked for independent re-measurement)

- **M3 mutation** (delete `-X main.version={{ .Version }}` from ldflags, leaving `- -s -w`): reproduced myself. `goreleaser check` exits 0, `goreleaser build --snapshot --clean --single-target` exits 0, and the resulting artifact reports `curlew 0.1.0-dev` — the hardcoded source default. Confirmed the `--version` assertion (`ci-local.sh:295-300` and the regex backstop at `:304-308`) is genuinely the only one of the three release steps that catches this; `check` and `build` are both blind to it.
- **Missing-goreleaser path**: reproduced in isolation with `goreleaser_cmd` extracted verbatim, `PATH` stripped of both lookup locations and `GOPATH` redirected to an empty directory. Fails immediately with exit 1 and the two-line install instruction; never reaches a `PASS`. `goreleaser_cmd`'s shape (probe `$(go env GOPATH)/bin`, then `PATH`, then `return 1` with an install command) is structurally identical to `lint_cmd`'s, as claimed.
- **`GitVersion:`-anchored version parse**: traced through the empty-match, non-numeric-major, and too-old-major cases by hand against the actual `case`/`if` logic; all three fail closed with a specific message and `exit 1`, never falling through to an unguarded arithmetic comparison.
- **`.goreleaser.yaml` byte-identity**: confirmed unchanged from `main` both before and after every mutation performed during this review, via `git diff main --exit-code -- .goreleaser.yaml`, using a `trap 'git checkout -- .goreleaser.yaml' EXIT INT TERM` around each mutation so a killed command could not leave it broken. Working tree is clean at the end of this review (`git status --short` empty), no stray `dist/`, `curlew`, or `mudflat` artifacts, no stray processes.
- **`release.yml`'s `~> v2` choice**: matches the pre-existing (unmodified by this diff) `Run goreleaser` publish step at `release.yml:63-67`, which already used `~> v2` before this task. The new gate-install step intentionally mirrors it so the gate and the real publish cannot resolve to different goreleaser builds within that workflow — this is correctly reasoned, not an inconsistency with `go.yml`'s separate choice to pin an exact version for its own, differently-motivated reasons.
- **Gate runtime cost**: measured `goreleaser check` at ~0.16s and `goreleaser build --snapshot --clean --single-target` at ~1.35s on this host (~1.5s combined) — same order of magnitude as the ~2.1s claimed in the comment and CHANGELOG; not overstated.
- **`dist/` gitignored**: `.gitignore:42` has `dist/`; zero tracked files under `dist/`; the *tracked* `internal/uiserver/assets/dist/index.html` (a different, nested `dist/` path) was confirmed unchanged by every `goreleaser build --clean` run performed during this review.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | FAIL | Finding #2. Fails closed in all cases (never a false PASS), but one path's diagnostic message is less clear than the code's own comment implies. |
| Input Validation | PASS | Tool-presence and version-string parsing both fail closed on empty/malformed input (verified by hand-tracing and by isolated reproduction of the missing-tool path). |
| Naming | PASS | `goreleaser_cmd`, `goreleaser_version`, `goreleaser_major`, `release_bin_count`, `release_bin`, `release_version` — consistent with the file's existing `lint_cmd`/`go_pkgs`/`mudflat_pid` snake_case convention. |
| Code Organization | FAIL | Finding #3. Placement within `ci-local.sh` (D4 in the plan: after `smoke`, before the dogfood gate) and the `goreleaser_cmd` resolver's placement next to `lint_cmd` are both correct and well-reasoned; the finding is specifically the undocumented asymmetry between the two workflow files' comments for the same underlying issue. |
| Correctness | FAIL | Finding #1. The shipped shell logic and workflow YAML are correct — every behavior in the task YAML was independently reproduced. The defect is in the prose record (CHANGELOG.md), which is itself a definition-of-done item and part of this task's deliverable. |
| Test Quality | PASS | No new Go test, and that is a considered, documented decision (plan D7/D8: a test asserting the script *contains* a goreleaser step would restate the script rather than exercise it). Verification is by direct execution and mutation instead; every mutation this review re-ran (M3, missing-tool, and — because of finding #1 — M2/`NoSuchVar`) reproduced exactly as the corrected plan record states. `cmd/curlew/ci_local_test.go`'s `TestCiLocalDownIdempotent` is correctly unaffected (`--down` returns at line 76, before `goreleaser_cmd` is even defined). |

## Behaviour Coverage

| # | Behaviour | Verified by | Status |
|---|-----------|--------------|--------|
| 1 | `check` runs and a failure stops the gate | M1′ (bad `goos`) in the plan's record; consistent with `goreleaser_cmd check` being a bare command under `set -e` | PASS |
| 2 | Valid config builds and executes a single-target snapshot, `--version` asserted | Full `ci-local.sh --go` run (this review), plus direct `goreleaser build` + `--version` | PASS |
| 3 | Missing goreleaser fails with an install instruction, never a skip | Reproduced in isolation this review (PATH/GOPATH stripped) | PASS |
| 4 | A build-breaking config change fails the gate naming the release step | Reproduced this review: `build` itself exits 1 on the undefined-template-variable mutation, so the last `step` header printed is `=== release: snapshot build for this host ===` | PASS (see finding #1 — the *behavior* is satisfied; the CHANGELOG's prose about *which* step catches it is wrong) |
| 5 | Snapshot binary reports the snapshot version, not the hardcoded default | Full gate run: `curlew 0.0.1-snapshot` | PASS |

All five behaviors are genuinely implemented and independently reproducible. Every finding in this review is about the accuracy of the surrounding record (CHANGELOG, a comment, an edge-case message), not about the shell logic failing to do what the task asked.

## Test Coverage

Not applicable in the usual Go-coverage sense — no `.go` files changed in this
diff (confirmed via `git diff --name-only main...HEAD`: only `scripts/ci-local.sh`,
two workflow YAMLs, `CHANGELOG.md`, and three `management/` files changed).
`go test -coverprofile` from the pre-audit gate run is unaffected by this task.

## Summary

The shell and CI-config implementation is sound: every one of the five task
behaviors reproduces exactly as claimed, the `goreleaser_cmd` resolver fails
closed and mirrors `lint_cmd`'s shape, the version parse fails closed on every
malformed input traced, and `.goreleaser.yaml` was left byte-identical to
`main` throughout. The blocking problem is not the code — it is that
`CHANGELOG.md` still asserts a specific, falsifiable technical claim
(`goreleaser build` "accepts [an undefined template variable] silently and
links a binary") that this same branch already proved false and retracted
in the plan file, in a commit that never touched the changelog. That the
correction happened in one document and not the other is exactly the kind
of drift this project has built dedicated tooling to catch elsewhere
(M21 post-strip drift, M22 false-clears); here it survived past the plan
retraction into the artifact most likely to be read as the permanent record.
Two Low findings round out the review: a documented-but-not-quite-true
guarantee in the `release_bin_count` guard's error path, and an undocumented
(but verified-working) reliance on Go's automatic toolchain download in
`go.yml` that `release.yml` explicitly reasons about but `go.yml` does not.
