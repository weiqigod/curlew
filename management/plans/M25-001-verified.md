# Verification Report: M25-001

**Task:** Exercise the release build before the day it is needed
**Verified by:** AI
**Date:** 2026-08-16
**Branch:** feature/M25-001-release-build-gate
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `./scripts/ci-local.sh` (auto-scope) | PASS | Exit 0, `=== ci-local PASS ===`. Run twice; scope auto-detected Go-only (diff touches no `src/`, `web/`, or stack files) |
| `./scripts/ci-local.sh --go` | PASS | Exit 0, run twice more (once as the literal observable command, once as the post-mutation clean-baseline re-run) |
| `go test ./...` (inside gate) | PASS | All packages ok |
| `go test -race` (inside gate) | PASS | No races detected |
| `golangci-lint run` (inside gate) | PASS | 0 issues |
| `./smoke/run.sh` (inside gate) | PASS | Smoke test clean |
| `go build -o ./curlew ./cmd/curlew` | PASS | Clean, no warnings; plain build reports `curlew 0.1.0-dev` (correct baseline — only the goreleaser-injected build carries a real version) |
| `go test -v -run TestCiLocalDownIdempotent ./cmd/curlew/...` | PASS | 2.04s — confirms the plan's Step 4 impact note: `--down` exits before the release section, unaffected |
| `go test ./internal/backlog/ -run TestBacklog_repository_is_consistent -v` | PASS | 243 tasks / 85 capabilities; M25-001 correctly still `review` at time of check |
| Coverage | 86.1% | `total: (statements) 86.1%`, from the gate's own `go tool cover -func` step. Meets >= 80% threshold |

## Observable Output

Ran the exact three-part observable from the task YAML, standalone (not just inside the gate):

```
$ goreleaser check
  • checking                                  path=.goreleaser.yaml
  • 1 configuration file(s) validated
  • thanks for using GoReleaser!
exit=0

$ goreleaser build --snapshot --clean --single-target
  ... building snapshot...  version=0.0.1-snapshot ...
  • build succeeded after 0s
exit=0

$ ./dist/curlew_darwin_arm64_v8.0/curlew --version
curlew 0.0.1-snapshot
exit=0

$ ./scripts/ci-local.sh --go
=== ci-local PASS ===
exit=0
```

Expected: `goreleaser check` validates; a real single-target binary is produced and reports a real version; the gate passes with the release steps included.
Result: MATCH

## Behaviors Verified

| # | Behavior | Verification | Status |
|---|----------|--------------|--------|
| 1 | `goreleaser check` runs against `.goreleaser.yaml` and a failure stops the gate | Plan's M1′ (invalid `goos`) and review's independent re-run: `check` exit 1, gate stops at `=== release: goreleaser check ===`. Not re-executed in this pass (trusted per Branch A — see Code Review below); structurally cross-checked that the referenced config lines (`.goreleaser.yaml:19,35,47`) exist as described | PASS |
| 2 | Valid config -> single-target snapshot built and executed, `--version` asserted | Independently reproduced by this verification directly (standalone observable run) and via 4 full/`--go` gate runs, all printing `dist/curlew_darwin_arm64_v8.0/curlew: curlew 0.0.1-snapshot` | PASS |
| 3 | Missing `goreleaser` fails the gate with an install instruction, not a skip | **Independently reproduced by this verification.** Moved the binary aside, stripped `GOPATH/bin` from `PATH`, ran `./scripts/ci-local.sh --go` under a trap-guarded subshell: exit 1, last header `=== release: goreleaser is installed and new enough ===`, stderr carried `go install github.com/goreleaser/goreleaser/v2@latest`, `ci-local PASS` printed 0 times. Binary restored automatically by trap; confirmed present and working afterward | PASS |
| 4 | A build-breaking config change fails the gate, naming the release step | Plan's M2 (undefined ldflags template var, fails at `build`) and M3 (dropped `-X main.version`, passes `check`+`build`, caught only by the version assertion) — both independently re-measured by the review (verdict PASS) with exact transcripts. Not re-executed in this pass; trusted per Branch A | PASS |
| 5 | `--version` reports the snapshot version, not the hardcoded default | Independently reproduced: `curlew 0.0.1-snapshot`, matches `^curlew [0-9]+\.[0-9]+\.[0-9]+-snapshot$`, and is not `curlew 0.1.0-dev` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | `goreleaser check` and a single-target snapshot build both run in `ci-local.sh --go` | Observed in every gate run: `=== release: goreleaser check ===` then `=== release: snapshot build for this host ===`, both green | PASS |
| 2 | Snapshot binary executed, `--version` asserted | `=== release: the built artifact reports its injected version ===` → `curlew 0.0.1-snapshot`, printed in every run | PASS |
| 3 | Missing goreleaser fails the gate with an install instruction, never a skip | Independently reproduced this pass (see Behavior 3 above) | PASS |
| 4 | Verified by mutation — break the config, confirm the gate fails, restore | Plan documents M1′/M2/M3 with real transcripts; review independently re-ran the core claim; `.goreleaser.yaml` confirmed byte-identical to `main` after all of it (`git diff main --exit-code -- .goreleaser.yaml` → exit 0, re-confirmed by this verification) | PASS |
| 5 | `dist/` is gitignored | `.gitignore:42:dist/`; `git check-ignore -v dist/artifacts.json` → `.gitignore:42:dist/ dist/artifacts.json`; `git status --porcelain` empty with `dist/` present on disk | PASS |
| 6 | `./scripts/ci-local.sh --go` passes | 4 green runs during this verification pass alone (2 unflagged auto-scope, 2 explicit `--go`), plus the missing-tool run's expected failure and the clean re-run immediately after | PASS |
| 7 | CHANGELOG.md updated | `CHANGELOG.md` `## [Unreleased]` → `### Added` carries a detailed entry plus an explicit self-correction paragraph (see Code Review) | PASS |

## Code Review

**Branch A: Review PASS exists** (`management/reviews/M25-001-review.md`, iteration 2, commit `b8f0339`).

Trusted the review, with spot-checks beyond the minimum 2-3 items given this diff has no `.go` source changes (the standard Go-idiom checklist — `%w` wrapping, doc comments, stuttering — doesn't directly apply to a shell/YAML/docs diff):

1. **Full read of the `scripts/ci-local.sh` diff** (both `goreleaser_cmd()` and the four-step release section). Confirmed: the resolver mirrors `lint_cmd()`'s probe-then-fail shape exactly (probes `$(go env GOPATH)/bin` before `PATH`, `return 1` on absence — never a skip); the version-major gate fails closed on an unparseable version; the `[ -d dist ]` guard (review finding #2's fix) correctly short-circuits to `release_bin_count=0` without swallowing unrelated `find` failures.
2. **Workflow placement check** (`go.yml`, `release.yml`): confirmed by direct `grep -n` that `Install goreleaser` precedes `Run the Go gate` / `Go gate` in both files, and that `release.yml` uses `install-only: true` (not `go install`) for its stated reason (pinned Go 1.24 vs. goreleaser's `go >= 1.26.5`).
3. **CHANGELOG cross-check**: read the diff in full; the corrected mutation claims match what's independently observable in `.goreleaser.yaml` (line 19 `version: 2`, line 35 `goos:`, line 47 the `-X main.version` ldflag) and match this verification's own directly-reproduced output (`curlew 0.0.1-snapshot`).

No issues found. Spot-check clean — no escalation to Branch B needed.

| Check | Status |
|-------|--------|
| Error handling (`goreleaser_cmd` fails closed, never skips) | PASS |
| Placement (release gate before dogfood, installs before gate in workflows) | PASS |
| Documentation accuracy (CHANGELOG, inline comments match measured behavior) | PASS |
| No new Go test (D7) — reasoning re-examined | PASS — sound; verification is by execution/mutation of the gate script itself, consistent with this project's own M22-001 lesson about checkers that don't read what they claim to check |

## Commits

| Hash | Message |
|------|---------|
| `14748a0` | docs(plan): add implementation plan for M25-001 |
| `90ba52e` | docs(plan): fold three verified findings into the M25-001 plan |
| `d5c3c8a` | chore(task): mark M25-001 as planned |
| `b90c72a` | chore(task): mark M25-001 as in_progress |
| `03d6e26` | chore(task): sync M25-001 task file status to in_progress |
| `cb1d2df` | feat(cli): add goreleaser_cmd resolver to ci-local.sh |
| `5d95077` | feat(cli): wire the release build into the Go gate |
| `3a95128` | docs(plan): M1 mutation revised after measurement contradicts it |
| `6c254f9` | docs(plan): record actual M2/M3 mutation measurements from /execute |
| `c9935ae` | docs(plan): record missing-tool verification against implemented gate |
| `c4598c3` | feat(cli): install goreleaser in the two workflows that run the gate |
| `bd9b254` | docs(changelog): record the release build gate (M25-001) |
| `e36f68a` | chore(task): mark M25-001 as review |
| `05ce503` | docs(plan): retract a false M2 measurement in the M25-001 record |
| `97ae89a` | docs(review): add review with findings for M25-001 |
| `f86eee9` | fix(ci): report the artifact-count diagnostic when dist/ is absent |
| `866ec50` | docs(ci): explain go.yml's go install despite the goreleaser toolchain gap |
| `516f6c8` | docs(changelog): correct a false mutation measurement in the M25-001 entry |
| `22cb51e` | docs(review): add improvement report for M25-001 |
| `b8f0339` | docs(review): add passing review for M25-001 |

All 19 commits carry `Refs: M25-001`, use conventional commit format, and show the TDD-for-infrastructure pattern this task actually needed: plan → RED (resolver added but unwired) → GREEN (wired) → measurement corrections found during `/execute` retracted in place rather than silently fixed → review → improve → re-review.

## Files Changed

| File | Action | Lines +/- |
|------|--------|-----------|
| `scripts/ci-local.sh` | modified | +124/-0 (approx., two hunks: resolver + release section) |
| `.github/workflows/go.yml` | modified | +17/-0 |
| `.github/workflows/release.yml` | modified | +16/-0 |
| `CHANGELOG.md` | modified | +57/-0 |
| `management/tasks/M25-001.yaml` | modified | status transitions |
| `management/backlog.yaml` | modified | status transitions |
| `management/plans/M25-001-plan.md` | added/modified | plan + deviations |
| `management/plans/M25-001-improved.md` | added | improvement report |
| `management/reviews/M25-001-review.md` | added | review report |

No `.go` source files changed — confirmed via `git diff main...HEAD --name-only`.

## Issues Found

None.

## Additional Invariant Checks (pipeline-specific)

| Invariant | Check | Result |
|---|---|---|
| `.goreleaser.yaml` untouched by this task | `git diff main --exit-code -- .goreleaser.yaml` | exit 0 — byte-identical to `main` |
| No tag ever cut | `git tag -l` | empty |
| Nothing published | No `goreleaser release` invoked at any point in this verification; `dist/` never committed | confirmed |
| Working tree clean after every gate run | `git status --porcelain` | empty after each of the 5 gate invocations run during this verification |
| No orphaned processes from the missing-tool mutation test | `ps aux \| grep -iE "ci-local\|mudflat\|curlew\|goreleaser"` | empty after an initial 5-minute-timeout scare on the first (undersized-timeout) attempt — goreleaser binary was restored immediately by hand, then the test was redone cleanly with a 600s timeout and a shell `trap` as a second safety net |

## Recommendation

PASS — ready for PR and merge.
