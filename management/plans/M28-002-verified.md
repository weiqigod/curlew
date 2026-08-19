# Verification Report: M28-002

**Task:** The front-door files, and a quickstart that was executed
**Verified by:** AI
**Date:** 2026-08-19
**Branch:** feature/M28-002-front-door-files-and-quickstart
**Verdict:** PASS

## Test Results

`./scripts/ci-local.sh` was run at verification time (auto-scope selected the Go
gate, since the branch touches no `src/`, `web/`, or stack file — the same set
`--go` forces). It exited 0 and printed `=== ci-local PASS ===`.

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/curlew` | PASS | Clean, no warnings |
| `go test ./...` | PASS | Exit 0, no failures |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | `0 issues.` |
| `./smoke/run.sh` | PASS | `=== Smoke Test Complete ===` |
| Release checks (goreleaser, six archives) | PASS | Snapshot built and version-checked |
| README install commands (M25-003) | PASS | Executed |
| Dogfood suite vs mudflat (6 harnesses) | PASS | Ledger, redaction, OpenAPI, crosscheck, expected-failures, `--parallel` |
| Coverage | 86.6% total | `internal/docs` 87.5%, `cmd/curlew` 81.1% — all above the 80% floor |

Both task-specific gate steps ran as named steps and passed:
`=== front-door files (M28-002) ===` and
`=== README quickstart, executed (M28-002) ===`.

## Observable Output

The task names two observables. Both were run verbatim.

```
$ go test ./cmd/curlew/ -run TestReadme_quickstart_actually_works -v
=== RUN   TestReadme_quickstart_actually_works
--- PASS: TestReadme_quickstart_actually_works (0.90s)
PASS
ok  	github.com/weiqigod/curlew/cmd/curlew	1.226s

$ go test ./internal/docs/ -run TestRepo_front_door_files_are_present -v
=== RUN   TestRepo_front_door_files_are_present
--- PASS: TestRepo_front_door_files_are_present (0.00s)
PASS
ok  	github.com/weiqigod/curlew/internal/docs	0.322s
```

Expected: both observables pass, by the exact test names the task YAML states.
Result: MATCH.

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Quickstart commands run in a temp dir against a local test server; each succeeds and final output matches the README | `TestReadme_quickstart_actually_works` | PASS |
| 2 | CONTRIBUTING.md's named commands are the ones `ci-local.sh` actually runs | `TestContributing_names_the_gate_that_ci_local_runs`, `TestAuditGate` (6 cases incl. 2 MUTATION, failing in both directions) | PASS |
| 3 | A quickstart yielding no commands fails rather than passing vacuously | `TestReadme_quickstart_extraction` — 12 cases, 7 labelled MUTATION, covering empty section, comments-only block, blank block, missing output block, ambiguous double block | PASS |
| 4 | SECURITY.md names a reporting route that exists | `TestSecurity_names_a_reporting_route_that_exists` | PASS |

Behavior 1 was inspected, not merely run. The test asserts output **byte for
byte** after duration normalisation (`cmd/curlew/readme_quickstart_test.go:422`),
executes the README's own block under `bash -euo pipefail` in `t.TempDir()`, and
independently refuses to let the quickstart reach the network: it fails if the
block names `http://`, `https://`, `example.com` or `httpbin.org`, and fails if
the block stops naming `$BASE_URL` — the seam the harness redirects through.
The server is in-process mudflat on `127.0.0.1:0`.

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | CONTRIBUTING.md, SECURITY.md, issue templates and a PR template written | All six exist: `CONTRIBUTING.md`, `SECURITY.md`, `.github/ISSUE_TEMPLATE/{bug_report,feature_request,config}.yml`, `.github/pull_request_template.md` | PASS |
| 2 | CONTRIBUTING command list derived from ci-local.sh, not restated | `GateModes` parses the live `case "$MODE" in`; `AuditGate` fails in both directions (undocumented mode / unknown mode) | PASS |
| 3 | README quickstart extracted and executed against a local server | `TestReadme_quickstart_actually_works` runs it against in-process mudflat | PASS |
| 4 | Output asserted against what the README shows | Byte-for-byte comparison, durations normalised (`TestReadme_quickstart_normalization`, 6 cases incl. `0ms`) | PASS |
| 5 | An empty extraction fails rather than passing | Sentinels `errNoQuickstartSection` / `errNoQuickstartCommands` / `errNoQuickstartOutput`, plus three independent floors in the observable (>= 3 commands, >= 10 non-blank output lines, must contain `passed`), plus `-list` vacuity guards on both new gate steps | PASS |
| 6 | `./scripts/ci-local.sh --go` passes | Full gate run exited 0 at verification time | PASS |
| 7 | CHANGELOG.md updated | Two `[Unreleased]` entries referencing M28-002 (a *fix* for the fabricated quickstart, an *add* for the front-door files) | PASS |

## Plan Completion

All 12 plan steps are present and complete. One deviation is documented in the
plan file: the `^TestReadme` prefix count in `ci-local.sh`'s comment was
predicted as 5 and measured as 7 during execution; the comment was corrected and
the guard itself (an anchored alternation) was unaffected.

## Code Review

Branch A — `management/reviews/M28-002-review.md` carries verdict PASS
(iteration 2, after one improve cycle resolving 4 findings). Spot-check clean:

| Check | Status |
|-------|--------|
| Error handling — `fmt.Errorf(... %w)` at all five wrap sites, sentinels for each failure mode | PASS |
| Doc comments — all 11 exported symbols in `frontdoor.go` documented | PASS |
| Test honesty — the observable asserts real bytes and forbids network access, rather than checking for absence of error | PASS |
| Commits — 17 commits, all `Refs: M28-002`, conventional format, `test(...)` before `feat(...)` at each RED/GREEN pair | PASS |

## Files Changed

19 files, +3309/−21. Production/test code: `internal/docs/frontdoor.go` (+478),
`internal/docs/frontdoor_test.go` (+667), `cmd/curlew/readme_quickstart_test.go`
(+439), `cmd/curlew/bug_report_exit_codes_test.go` (+120),
`internal/docs/hints_init.go` (+45), `scripts/ci-local.sh` (+27/−4).
Documentation: `README.md` (+61/−21, hero and Quickstart rewritten),
`CONTRIBUTING.md`, `SECURITY.md`, four `.github/` templates, `CHANGELOG.md`.

## Issues Found

None blocking. One operational note, outside this repository's files:

- `SECURITY.md` points at
  `https://github.com/weiqigod/curlew/security/advisories/new`. That URL is the
  correct route for this module path (derived from `go.mod`, not hand-typed, and
  held there by test), but it only accepts reports once **private vulnerability
  reporting is enabled** in the repository's Settings → Security. That is a
  repository-settings change, not a code change, and was not made as part of
  this task. The test deliberately rejects any `@users.noreply.github.com`
  address, so the dead mail route cannot be substituted later by accident.

## Recommendation

PASS — ready for PR and merge.
