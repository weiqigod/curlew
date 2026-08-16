# Verification Report: M25-003

**Task:** Every install path in the README is one that was executed
**Verified by:** AI (pipeline verify phase)
**Date:** 2026-08-16
**Branch:** feature/M25-003-readme-install-paths
**Verdict:** PASS

All commands below were executed live in this session against this tree. Nothing is carried over from the plan or review without being independently re-run.

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `./scripts/ci-local.sh` (full, foreground) | PASS | rc=0, ends `=== ci-local PASS ===`. Auto-scoped to `Gates: go=1 backend=0 web=0 ui=0 e2e=0` (no backend/web/e2e files on this branch) — equivalent in coverage to `--go`. |
| `go test ./...` | PASS | all packages `ok`; `cmd/curlew` 90.817s |
| `go test -race ./...` | PASS | no races detected; `cmd/curlew` 92.593s |
| `golangci-lint run` | PASS | 0 issues — `.golangci.yml`'s `run.build-tags` lists both `release_artifacts` and `readme_install`, so both tagged files were compiled and linted together (the symbol-collision-sensitive configuration) |
| `./smoke/run.sh` | PASS | ends "Smoke Test Complete"; the only `FAIL`-prefixed lines in the whole run are expected negative-path smoke assertions (e.g. `FAIL nonexistent_validate_test.yaml is invalid`), confirmed by grep — zero `--- FAIL` Go test failures anywhere in the log |
| Coverage | 86.2% total, `cmd/curlew` 81.1% | Meets >= 80% threshold |
| M25-003 gate step: "install: every README install command is executed" | PASS | Ran inside `ci-local.sh`: `TestReadme_install_commands_execute` (5.93s), `TestReadme_install_blocks_extraction` (8 subtests), `TestReadme_documents_a_binary_download` (7 subtests) — all green |

No known-flaky WebSocket-heartbeat timing issue occurred in this run — both `a heartbeat survives while a step is reading` (758ms) and `a heartbeat survives while a step is idle` (403ms) passed cleanly on the first attempt; no re-run was needed.

## Observable Output

Command 1 (task YAML, corrected observable — requires `-tags readme_install`):
```
$ go test -tags readme_install ./cmd/curlew/ -run TestReadme_install_commands_execute -v
=== RUN   TestReadme_install_commands_execute
--- PASS: TestReadme_install_commands_execute (5.74s)
PASS
ok  	github.com/weiqigod/curlew/cmd/curlew	6.076s
rc=0
```

Command 2 (task YAML, hermetic observable — no tag, no network):
```
$ go test ./cmd/curlew/ -run TestReadme_documents_a_binary_download -v
=== RUN   TestReadme_documents_a_binary_download
    (7 subtests, all PASS)
--- PASS: TestReadme_documents_a_binary_download (0.00s)
PASS
ok  	github.com/weiqigod/curlew/cmd/curlew	0.397s
rc=0
```

The tag boundary itself was independently re-confirmed live: `go test ./cmd/curlew/ -list 'TestReadme.*'` lists 3 tests untagged; `go test -tags readme_install ./cmd/curlew/ -list 'TestReadme.*'` lists 4 (adds `TestReadme_install_commands_execute`).

Expected: both commands exit 0 with the named test passing.
Result: MATCH.

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Given the Install section of README.md, when a command block in it is extracted, then that command is executed in a temporary directory and must succeed. | `TestReadme_install_commands_execute` | PASS — run directly, 5.74s, real `gh`/git/network |
| 2 | Given the go install line, when it runs against the module path in go.mod, then the module path in the README matches go.mod rather than being a stale copy. | `TestReadme_documents_a_binary_download/go_install_line_matches_the_go.mod_module_path` | PASS |
| 3 | Given a published release, when the README describes downloading an archive, then the described URL shape matches the name_template in .goreleaser.yaml. | `TestReadme_documents_a_binary_download/documented_archive_name_matches_name_template` (+ `documented_download_URL_matches_the_go.mod_module_path`) | PASS |
| 4 | Given a new install method added to the README without a matching check, when the test suite runs, then it fails. | `TestReadme_install_commands_execute` (structural — no test code changes) | PASS — reproduced live in this session (see below), not merely trusted from the review |

**Live reproduction of behavior 4 (this session):** added a new `### A deliberately broken install method` subsection under `## Install` with a `bash` fence containing `exit 7`, touching no test code. Reran `TestReadme_install_commands_execute`: `README.md:146: install block failed: exit status 7`, test **FAILED** (rc=1) after 6.04s. `git checkout -- README.md` afterward; `git status --porcelain` confirmed empty.

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | README Install commands extracted and executed by a test | `TestReadme_install_commands_execute` runs `readmeSectionBlocks(doc, "Install")` and executes each block; PASS live | PASS |
| 2 | An empty extraction fails rather than passing vacuously | Live mutation this session: renamed `## Install` → `## Install-renamed-for-verify`, reran the exec test — failed in 0.00s (no network cost) with `no fenced code blocks under README.md '## Install' -- the extraction is broken, not the README`. Reverted; tree confirmed clean. | PASS |
| 3 | Documented archive URL shape asserted against `.goreleaser.yaml` | `documented_archive_name_matches_name_template` and `documented_extension_matches_archives[0].formats` subtests, PASS | PASS |
| 4 | Module path in README asserted against `go.mod` | `go_install_line_matches_the_go.mod_module_path` and `documented_download_URL_matches_the_go.mod_module_path` subtests, PASS | PASS |
| 5 | A download-and-run path is documented and works | `sawReleasedVersion` check inside `TestReadme_install_commands_execute` (fails the test if no block prints `curlew X.Y.Z` without a `-dev`/`-snapshot` suffix) — test passed, so this held | PASS |
| 6 | Package-manager decision recorded in `docs/TECH_CHOICES.md` | `### Distribution` section present at `docs/TECH_CHOICES.md:54-77`, records "no Homebrew tap yet" with the 404-on-private-repo reasoning and a revisit trigger; read directly | PASS |
| 7 | `./scripts/ci-local.sh --go` passes | Full `./scripts/ci-local.sh` run this session auto-scoped to `go=1` only (no backend/web/e2e files changed) — functionally the same step set `--go` forces — rc=0, `=== ci-local PASS ===` | PASS |
| 8 | CHANGELOG.md updated | `## [Unreleased]` gained `### Fixed` (README's non-working install command, Homebrew-tap false claims) and `### Added` (the two new tests, the TECH_CHOICES.md decision) entries; read directly at `CHANGELOG.md:7-98` | PASS |

## Code Review

Branch A (review PASS exists): `management/reviews/M25-003-review.md`, verdict PASS, zero findings, with an extensive independently-reproduced verification table of its own (17 checks).

Spot-check performed this session, independent of trusting the review's own claims:
1. **Error handling site** — `loadReadmeArchiveConfig` (`cmd/curlew/readme_install_test.go:171-193`): `fmt.Errorf("read %s: %w", path, err)` and `fmt.Errorf("parse %s: %w", path, err)` both wrap correctly; the remaining `fmt.Errorf` calls in the same function (empty `project_name`/`archives`/`name_template`/`formats`) construct new validation errors with no underlying `err` to wrap, which is correct as written.
2. **Doc comments** — every type/func/var in `readme_install_test.go` (`readmeBlock`, `readmeSectionBounds`, `readmeSectionText`, `readmeSectionBlocks`, `readmeRepoRoot`, `readmeReadFileOrFatal`, `readmeModulePath`, `readmeArchiveConfig`, `loadReadmeArchiveConfig`, `renderNameTemplate`) carries a doc comment, read directly — confirmed, despite `package main` meaning none of them are cross-package exported.
3. **Test correctness** — `windows override is documented` subtest (`readme_install_test.go:334-348`) derives the Windows extension from `.goreleaser.yaml`'s actual `format_overrides` (not a hardcoded `.zip`) and asserts the README section contains it — genuinely tests what it claims.

Spot-check found no issues; Branch A trust stands.

Also independently confirmed: `.golangci.yml`'s `run.build-tags` extends (not replaces) the existing `release_artifacts` entry with `readme_install`; `scripts/ci-local.sh` names the new step with a `gh`-presence probe and a `-list` count guard (`!= "3"`) using an anchored alternation, not a bare prefix. Read directly at `scripts/ci-local.sh:359-392`.

## Plan Deviations Cross-Check

The plan's "Deviations (recorded during /execute)" section names two corrections; both verified against the shipped code in this session:
1. `ErrReadmeSectionMissing`/`ErrReadmeNoBlocks` sentinels dropped — confirmed absent from both test files; the corresponding failure modes are surfaced via descriptive `t.Fatal`/`t.Fatalf` instead, exactly as recorded.
2. `readFileOrFatal`→`readmeReadFileOrFatal` and `releasedVersionRE`→`readmeReleasedVersionRE` renamed to carry the `readme` prefix — confirmed present under the corrected names in `readme_install_test.go:126` and `readme_install_exec_test.go:28` respectively.

## Commits

| Hash | Message |
|------|---------|
| `8f6f63b` | docs(plan): add implementation plan for M25-003 |
| `a9543f1` | chore(task): mark M25-003 as planned |
| `64f3db9` | chore(task): mark M25-003 as in_progress |
| `714c4b6` | docs(readme): rewrite Install section with download, source and clone paths |
| `4284660` | test(cli): add failing tests for README install-section extraction (RED) |
| `f57e8c5` | feat(cli): implement README install-section extractor and download-path check (GREEN) |
| `2ec0b0c` | refactor(cli): gofumpt-format readme_install_test.go |
| `b3d429c` | feat(cli): execute README install commands under a readme_install tag |
| `a5f0746` | feat(ci): wire the readme_install tag into lint and the gate |
| `6ef5d79` | docs(tech-choices): record the Homebrew decision and correct two false claims |
| `964531c` | docs(task): correct M25-003's observable and record the CHANGELOG entry |
| `8789c87` | docs(plan): record two small deviations for M25-003 |
| `44a9294` | chore(task): mark M25-003 as review |
| `9c22854` | docs(review): add passing review for M25-003 |

All 14 commits carry `Refs: M25-003`, conventional-commit type(scope) prefixes, and a visible RED (`4284660`) before GREEN (`f57e8c5`, `b3d429c`) TDD pattern.

## Files Changed

| File | Action | Lines +/- |
|------|--------|-----------|
| `.golangci.yml` | modified | +7 |
| `CHANGELOG.md` | modified | +91 |
| `README.md` | modified | +33/-11 (net, includes rewritten Install section) |
| `cmd/curlew/readme_install_exec_test.go` | created | +112 |
| `cmd/curlew/readme_install_test.go` | created | +366 |
| `docs/TECH_CHOICES.md` | modified | +29/- |
| `docs/security/info-sec-policy.md` | modified | +5/- |
| `docs/security/pentest-2026-Q2.md` | modified | +3/- |
| `management/backlog.yaml` | modified | +6/- |
| `management/plans/M25-003-plan.md` | created | +882 |
| `management/reviews/M25-003-review.md` | created | +73 |
| `management/tasks/M25-003.yaml` | modified | +9/- |
| `scripts/ci-local.sh` | modified | +35 |

13 files changed, 1640 insertions(+), 11 deletions(-) (`git diff --stat main...HEAD`).

## Issues Found

None.

## Out-of-scope items noted, not held against this diff

- `docs/MANUAL.md:2340` and `:2364` still carry the bare `go install github.com/weiqigod/curlew/cmd/curlew@latest` line — found by the reviewer, explicitly named out of scope for M25-003 (title/behaviors/scope all name `README.md` only), and already spawned as its own follow-up task per the pipeline context. Not touched here.
- One known-flaky, pre-existing WebSocket-heartbeat timing test exists in the mudflat dogfood suite (unrelated to this diff) — it did not flake in this run, so there is nothing to report beyond confirming it passed cleanly.

## Recommendation

PASS — ready for PR and merge.
