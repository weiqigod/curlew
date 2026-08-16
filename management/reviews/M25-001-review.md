# Code Review: M25-001

**Task:** Exercise the release build before the day it is needed
**Reviewer:** AI
**Date:** 2026-08-16
**Branch:** feature/M25-001-release-build-gate
**Iteration:** 2 (re-review after `/improve`)

## Verdict: PASS

## Pre-audit Gate

`./scripts/ci-local.sh --go` — **PASS** (exit 0, `=== ci-local PASS ===`; run in
full, in the foreground, by this review — not trusted from the improvement
report). Continuing to the static audit.

## Iteration 1 Findings: Verification

Iteration 1 (`management/plans/M25-001-improved.md`, commits `f86eee9`,
`866ec50`, `516f6c8`) reported all 3 findings resolved. Each is re-verified
below by independent measurement, not by reading the improvement report's
claims.

### #1 (Critical) — CHANGELOG.md false measurement — RESOLVED, confirmed

The prior entry claimed `goreleaser build --snapshot` "accepts [an undefined
ldflags template variable] silently and links a binary carrying the same
`0.1.0-dev` default instead." That claim was already known false at iteration
1 (it does not reproduce). Current `CHANGELOG.md:36-43` now reads:

> A third mutation (an undefined template variable in `ldflags`) fails at the
> **build** step itself, as originally expected: `goreleaser build --snapshot
> --clean --single-target` exits 1 with `map has no entry for key
> "NoSuchVar"` and leaves `dist/` with zero files — it does not link a
> binary, silently or otherwise.

Independently reproduced an eighth time by this review: mutated
`.goreleaser.yaml` (`{{ .Version }}` → `{{ .NoSuchVar }}` in the `ldflags`
line), ran `goreleaser check` (exit 0) then
`goreleaser build --snapshot --clean --single-target` (exit **1**,
`⨯ build failed after 0s error=build failed: template: failed to apply
"-s -w -X main.version={{ .NoSuchVar }}": ... map has no entry for key
"NoSuchVar"`), then confirmed `find dist -type f -name curlew` returns
**zero** results. Matches the CHANGELOG's corrected claim exactly.

`CHANGELOG.md:45-56` also carries an explicit **"Correction to an earlier
revision of this entry"** paragraph: it states what the earlier text claimed,
says plainly that the claim is false and does not reproduce, cross-references
`management/plans/M25-001-plan.md`'s prior retraction, and explains why
nothing automated caught the drift (`CHANGELOG.md` is excluded from the
doc-prose/doc-table checks). This is a correction recorded in place, not a
silent rewrite — satisfies the review brief's requirement directly.

### #2 (Low) — `ci-local.sh` dist/-missing diagnostic — RESOLVED, confirmed; reasoning for the alternate fix holds

`scripts/ci-local.sh:294-298` now reads:

```sh
if [ -d dist ]; then
  release_bin_count="$(find dist -type f -name curlew | wc -l | tr -d ' ')"
else
  release_bin_count=0
fi
```

Traced the `set -euo pipefail` interaction by hand and confirmed the fix
closes the exact gap: with `dist/` absent, `[ -d dist ]` is false,
`release_bin_count=0` is a plain assignment (cannot trip `pipefail`), and
execution reaches `if [ "$release_bin_count" != "1" ]` and prints "expected
exactly one built curlew under dist/, found 0" before exiting 1 — the
diagnostic iteration 1 found missing.

The improve phase deliberately did **not** take the review's literal
suggestion (`find ... 2>/dev/null || release_bin_count=0`). I judged this
reasoning and it holds: that alternative treats the exit status of the whole
`find | wc -l | tr -d ' '` pipeline as one signal, so *any* `find` failure —
including a permission error hit partway through a `dist/` subtree that had
already produced legitimate matches — would be swallowed and reported as
"found 0," a count `find` never actually produced. The chosen guard only
special-cases the one condition it can prove in advance (`dist/` does not
exist), and leaves every other `find` failure to abort the script with
`find`'s own stderr, exactly as documented in the new comment at
`ci-local.sh:287-293`. That is a narrower, more honest fix than the one
originally suggested.

New-problem check (task 2 of this review's brief): the guard fails closed in
every reachable state —
- `dist/` absent → count forced to `0` → `!= "1"` → exit 1 with message.
- `dist/` present, wrong count (0 or ≥2) → same exit 1 path.
- `dist/` present with exactly one `curlew` → falls through to
  `release_bin="$(find dist -type f -name curlew)"` (line 303), which
  re-resolves the same single match, and the binary is actually executed and
  its `--version` output asserted against both the hardcoded-default check
  and the `X.Y.Z-snapshot` regex.

There is no path where the version assertion becomes a silent no-op: the only
way past the count check is a real, singular `curlew` binary, which is then
genuinely invoked. Also confirmed by reproducing the *only* realistic trigger
for the missing-`dist/`-adjacent code (the `NoSuchVar` mutation above): `goreleaser
build` itself exits 1 and `set -e` aborts the whole script at the "release:
snapshot build for this host" step, before the dist-count step is ever
reached — consistent with iteration 1's own characterization of finding #2 as
"very hard to reach" in the live gate, and confirming the fix is scoped to a
real (if rare) edge case rather than a live risk.

### #3 (Low) — `go.yml` undocumented toolchain gap — RESOLVED, confirmed

`.github/workflows/go.yml:46-53` now carries a comment explaining that
`GOTOOLCHAIN=auto` is why `go install github.com/goreleaser/goreleaser/v2@v2.17.1`
works under a Go 1.24 base despite goreleaser v2.17.1 requiring Go ≥ 1.26.5,
and why `release.yml` makes the opposite choice (prebuilt binary via
`goreleaser-action`, `install-only: true`) for release-day cost reasons.

Independently verified every factual claim in the new comment:
- `grep -rn GOTOOLCHAIN` across the repo returns only the new comment line
  itself — not overridden anywhere. `go env GOTOOLCHAIN` on this host reports
  `auto` (the unmodified default).
- `go.mod` declares `go 1.24.0` with no `toolchain` line.
- Goreleaser v2.17.1's own cached module source
  (`$GOPATH/pkg/mod/github.com/goreleaser/goreleaser/v2@v2.17.1/go.mod`)
  declares `go 1.26.5`, matching the comment.
- `go version -m "$(go env GOPATH)/bin/goreleaser"` on this host reports
  `go1.26.6` — matching the comment's specific measured claim ("fetched
  go1.26.6") exactly, and `$GOPATH/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.6...`
  is present in the module cache as corroborating evidence of a real
  auto-download, not an assertion taken on faith.
- `release.yml:47-51`'s existing comment (untouched by this diff, confirmed
  via `git diff 97ae89a..HEAD --name-only`) independently gives the same
  "toolchain download on every release run" rationale for its own opposite
  choice — the two files are now consistent and cross-referential rather than
  silently divergent.

## Task 3: Independent re-verification of the core claim

The task's central claim — that a config change silently dropping the
`-X main.version` ldflag passes `check` and `build` and is caught only by the
`--version` assertion — was re-measured directly by this review, not read
from any report, because the written record for this task (the CHANGELOG
entry above) has already been wrong once.

Procedure: `trap 'git checkout -- .goreleaser.yaml' EXIT INT TERM`, then
edited `.goreleaser.yaml` to change
`- -s -w -X main.version={{ .Version }}` → `- -s -w` (ldflags, line 47).

Results:
- `goreleaser check` → exit **0** (`1 configuration file(s) validated`).
- `goreleaser build --snapshot --clean --single-target` → exit **0**
  (`build succeeded after 0s`).
- Built artifact: `dist/curlew_darwin_arm64_v8.0/curlew --version` →
  `curlew 0.1.0-dev` — the hardcoded `cmd/curlew/main.go` default, silently
  shipped by both `check` and `build`. Only the artifact/version-assertion
  step (`ci-local.sh:299-320`) would catch this.

This is exactly the task's claimed behavior, reproduced independently.
Cleanup: `git diff main --exit-code -- .goreleaser.yaml` → exit 0
(byte-identical to `main`); `git status --short` → empty; no stray `dist/`,
`curlew`, or `mudflat` left on disk.

## New Findings

None. No new problems were introduced by the three fix commits.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Finding #2 resolved; the `[ -d dist ]` guard fails closed in every reachable state, and the choice to leave non-missing-`dist` `find` failures aborting via `set -e` (rather than folding them into a fabricated count) is deliberate and documented. |
| Input Validation | PASS | Unchanged from iteration 1 (still PASS) — re-confirmed tool-presence and version-string parsing both fail closed. |
| Naming | PASS | Unchanged; no new identifiers introduced by the fix commits beyond the existing `release_bin_count` variable, reused correctly. |
| Code Organization | PASS | Finding #3 resolved; `go.yml` and `release.yml` now carry matching, cross-referential rationale for their differing `goreleaser`-install strategies. |
| Correctness | PASS | Finding #1 resolved; `CHANGELOG.md` now states the independently-reproduced true behavior and explicitly marks the correction. |
| Test Quality | PASS | Unchanged from iteration 1 — no new Go test, a considered decision; verification remains by direct execution and mutation, which this review repeated independently rather than trusting the record. |

## Behaviour Coverage

All 5 task behaviors were independently reproduced in iteration 1 and remain
unaffected by the 3 fix commits (none of which touch the shell logic that
implements the behaviors, only its diagnostic message, a comment, and prose).
This review additionally re-reproduced behavior #4 (build-breaking config
change) via the `NoSuchVar` mutation and the task's core claim (behavior #2's
version assertion) via the `-X main.version` deletion, both independently,
both matching the documented record.

## Test Coverage

Not applicable in the Go-coverage sense — no `.go` files changed in this
diff or in the improve-phase diff (`git diff 97ae89a..HEAD --name-only`:
`.github/workflows/go.yml`, `CHANGELOG.md`,
`management/plans/M25-001-improved.md`, `scripts/ci-local.sh` only).
`go test -coverprofile` from the pre-audit gate run is unaffected.

`go test ./internal/backlog/ -run TestBacklog_repository_is_consistent -v`
re-run by this review: PASS, 243 tasks across 85 capabilities, M25-001 still
`review` (task status correctly untouched pending `/verify`).

## Summary

All 3 iteration-1 findings are genuinely resolved, each verified by this
review's own independent measurement rather than by trusting the improvement
report: the CHANGELOG now states the true, reproduced build-step failure and
explicitly marks the correction rather than silently rewriting it; the
`ci-local.sh` diagnostic now fires on a missing `dist/` while deliberately
declining to fold unrelated `find` failures into a fabricated count, a
narrower and more honest fix than the one originally suggested; and
`go.yml`'s toolchain comment is accurate down to the specific Go patch
version measured. No new problems were introduced — the `[ -d dist ]` guard
fails closed in every reachable state and never turns the version assertion
into a no-op. This review also independently re-ran the task's core mutation
(deleting `-X main.version` from `.goreleaser.yaml`) and confirmed `check`
and `build` both pass while only the artifact assertion catches the
regression, exactly as claimed. `.goreleaser.yaml` is confirmed
byte-identical to `main` after every mutation performed during this review.
Nothing was tagged, released, or published.
