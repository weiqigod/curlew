# Code Review: M25-003

**Task:** Every install path in the README is one that was executed
**Reviewer:** AI
**Date:** 2026-08-16
**Branch:** feature/M25-003-readme-install-paths

## Verdict: PASS

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| — | — | — | — | — | None | — |

No findings at any severity. Every checkable claim in the plan and the CHANGELOG entry was independently reproduced during this review — live against this tree, not accepted from the written record. See "Independent verification performed" below for what was run and what came back.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `loadReadmeArchiveConfig` wraps with `fmt.Errorf("...: %w", err)`, never `%v`. All other new call sites (`os.ReadFile`, `os.WriteFile`, `filepath.Abs`, `cmd.CombinedOutput`) check `err` and route to `t.Fatal[f]`/`t.Error[f]`, never a swallowed error or a `panic`. The plan's originally-declared sentinels (`ErrReadmeSectionMissing`, `ErrReadmeNoBlocks`) were correctly dropped as a recorded deviation: the concrete signatures actually implemented (`readmeSectionBlocks`/`readmeSectionText`) return zero-values, not errors, on a missing heading, so the sentinels would have had no call site and would fail the `unused` linter — confirmed by reading the shipped code, which surfaces those cases via descriptive `t.Fatal[f]` text at the point of use instead. |
| Input Validation | PASS | Missing heading, zero blocks, a non-bash/sh fence, an empty `.goreleaser.yaml` field (`project_name`, `archives`, `name_template`, `formats`), and an unrecognised template action are all explicit failures — verified live via mutation, not merely read (see table below). |
| Naming | PASS | `package main`, no new exported symbols. Every new identifier is `readme`-prefixed per the plan's D2 (avoiding collision with `release*` symbols from the sibling `release_artifacts`-tagged file compiled into the same binary under golangci-lint) — confirmed by running `golangci-lint run --build-tags release_artifacts,readme_install` directly: 0 issues, no redeclaration. Every new type/func/var carries a doc comment, exceeding the requirement for unexported symbols. |
| Code Organization | PASS | Two files split exactly on cost/network boundary (D1): extractor + hermetic test untagged, the network-touching exec test behind `//go:build readme_install`. `.golangci.yml`'s `run.build-tags` list is *extended* (the pre-existing `release_artifacts` entry is untouched), matching the file's own header comment's stated requirement. `defer cancel()` present for the one `context.WithTimeout`. No unused imports/vars (confirmed by golangci-lint with both tags active). |
| Correctness | PASS | `readmeSectionBounds` correctly distinguishes `## ` from `### ` (verified: `"### One"` does not have prefix `"## "` — confirmed both by code inspection and by the passing `### subheadings do not end the section` subtest). An unterminated fence degrades safely into "fewer blocks found," which the floor guard catches rather than silently under-counting into a pass. `cmd.Env` correctly overrides any ambient `GOBIN` (Go's documented last-wins duplicate-key behavior). `go test -race` passes (no goroutines introduced by this diff). |
| Test Quality | PASS | Table-driven extractor test with 8 cases including both edge cases named in the task's `behaviors` (missing heading, heading with no blocks). Each `TestReadme_documents_a_binary_download` sub-assertion runs in its own `t.Run` and was shown live to fail independently of its siblings. `TestReadme_install_commands_execute` is a real `os/exec` integration test against the real `curlew` binary and a real GitHub release. All four `behaviors` in the task YAML map to a test, including behavior 4 ("a new install method added without a matching check must fail"), which was reproduced live rather than taken on the plan's word (see table below). |

## Independent verification performed

Per the pipeline's instructions, the plan and CHANGELOG were treated as claims to reproduce, not facts to accept. Every mutation below was applied to the working tree with `trap 'git checkout -- <file>' EXIT INT TERM`, and `git status --porcelain` was confirmed empty after each one.

| # | Check | Method | Result |
|---|---|---|---|
| 1 | The tag boundary is real | `go test -list '^TestReadme' ./cmd/curlew/` vs. the same with `-tags readme_install` | Untagged: 3 (`_lists_every_command_...`, `_install_blocks_extraction`, `_documents_a_binary_download`). Tagged: 4 (adds `_install_commands_execute`). Matches the pipeline's own pre-confirmed boundary. |
| 2 | The `-list` guard's anchored alternation | Ran the exact regex from `ci-local.sh` against the tagged package | Exactly 3 matches (excludes the pre-existing `_lists_every_command_...`), confirming the alternation — not a bare prefix — is what is wired into the gate. |
| 3 | The `\|\| true` vacuity-guard plumbing | Ran the same `-list \| grep -c` pipeline with a pattern matching zero tests | Outputs `0`, exit 0 (via `\|\| true`) — the count correctly reaches the `!= "3"` check rather than aborting the script early under `set -e`. |
| 4 | golangci-lint with both build tags | `golangci-lint run --build-tags release_artifacts,readme_install ./cmd/curlew/...` | 0 issues — no symbol collision between the two tagged files compiled together. |
| 5 | Repository is private / assets 404 anonymously | `gh repo view weiqigod/curlew --json isPrivate`; `curl` the v0.1.0 darwin/arm64 asset with no auth | `{"isPrivate":true}`; HTTP `404`. Matches the README's stated claim exactly. |
| 6 | `go.mod` / `.goreleaser.yaml` are the real source of the assertions | Read `go.mod` (`module github.com/weiqigod/curlew`) and `.goreleaser.yaml` (`project_name: curlew`, `name_template: {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}`, `formats: [tar.gz]`, windows override `[zip]`) directly | Renders to exactly `curlew_<version>_<os>_<arch>.tar.gz`, matching the README's documented shape byte for byte — a real cross-file check, not a tautology. |
| 7 | The three hermetic/extraction tests actually run and pass | `go test ./cmd/curlew/ -run TestReadme_documents_a_binary_download -v` and `...TestReadme_install_blocks_extraction -v` | Both PASS; all 7 + 8 subtests green. |
| 8 | The exec test actually runs and passes | `go test -tags readme_install ./cmd/curlew/ -run TestReadme_install_commands_execute -v` | PASS, 6.29s — in the plan's measured range, hitting real network/`gh`/git. |
| 9 | Floor guard (`< 3`) fires on a deleted install path | Removed the "Clone and build" subsection from `README.md`, reran the exec test | **FAILED** fast (0.00s, no network): `found 2 command block(s) under '## Install'; measured 3 on this tree`. Restored; tree clean. |
| 10 | Module-path assertion is independent of its siblings | Changed the `go install` line's module to `github.com/wrong/curlew` | Only `go_install_line_matches_the_go.mod_module_path` failed; the other 6 subtests (including `documented_download_URL_matches...`, which shares the same derived `ownerRepo`) stayed green. Restored; tree clean. |
| 11 | `name_template` assertion is independent of its siblings | Changed `.goreleaser.yaml`'s separator to `-` | Only `documented_archive_name_matches_name_template` failed; all other subtests stayed green. Restored; tree clean. |
| 12 | Anti-vacuity: an unrecognised template action fails loudly | Changed `{{ .Version }}` to `{{ .Tag }}` in `.goreleaser.yaml` | `name_template_renders_without_unknown_actions` failed with `uses action(s) [{{ .Tag }}] that renderNameTemplate does not know how to substitute` — not a silent half-rendered comparison. Restored; tree clean. |
| 13 | Fence-language guard is a hard failure, not a skip | Retagged the "Install from source" fence from `` ```bash `` to `` ```text `` | **FAILED**, 4.31s: `fence language "text" under '## Install'` reported, and `executed 2 of 3 blocks` — the other two blocks still ran (network cost still paid for those), proving it is a per-block hard failure, not an exemption. Restored; tree clean. |
| 14 | Behavior 4: a new, unchecked install method is caught structurally | Added a 4th `## Install` subsection with a deliberately failing `exit 7` command — no test code touched | **FAILED**, 8.25s: `install block failed: exit status 7`, with the offending block and its output quoted. Confirms the extraction is structural (iterates whatever it finds), not a hand-maintained list. Restored; tree clean. |
| 15 | Misspelled build tag reproduces the guard's failure mode | Ran the `-list` guard with a typo'd tag (`readme_instal_typo`) | Count `2` (only the two untagged tests survive), correctly `!= "3"` — the guard fires before any test executes. |
| 16 | Backlog/task-file consistency | `go test ./internal/backlog/ -run TestBacklog_repository_is_consistent -v` | PASS; M25-003 correctly listed as the sole `review`-status task, consistent between `backlog.yaml` and the task file. |
| 17 | No shipped document still claims a Homebrew tap exists | `grep -rni homebrew` across the whole repo (excluding `.git` and an unrelated stray worktree under `.claude/worktrees/`, confirmed via `git worktree list` to be a separate, untracked checkout on a different branch, not part of this repo's tree) | Both corrected files (`docs/security/info-sec-policy.md:18`, `docs/security/pentest-2026-Q2.md:45`) now read "No Homebrew tap exists...". The only remaining hit outside this task's own plan/CHANGELOG/task files is `management/plans/M18-011-plan.md:435`, a frozen planning record for an already-completed milestone (last touched 2026-08-02, pre-dating this task) — not a live/shipped claim, and correcting it would be revisionism of a historical record rather than a fix. Not treated as a finding. |

## A pre-existing, out-of-scope issue found in passing

`docs/MANUAL.md:2340` and `:2364` (the "§4.6 CI recipes" GitHub Actions / GitLab CI examples) still contain the exact same unqualified `go install github.com/weiqigod/curlew/cmd/curlew@latest` line the README used to have — it would fail today for anyone who copies it, for the identical reason this task fixed in the README (private repo, no `GOPRIVATE`, `sum.golang.org` 404). This is real and verified (grepped directly, read the surrounding recipe blocks), but it is outside M25-003's stated scope (title, `behaviors`, and `scope` all name `README.md` specifically; the plan never lists `docs/MANUAL.md` as a file to modify), so it is not held against this diff. The repo's own `doc_prose_test.go` explains why nothing else already caught it: `MANUAL.md` is checked only for *names* that must still exist (commands/flags/env vars), not for whether an example shell recipe actually executes — this defect is a different class from what that harness covers. Flagged separately via `spawn_task` rather than blocking this review.

## Test Coverage

- `cmd/curlew` package, untagged (`go test -coverprofile`): 81.1% of statements — meets the project's 80% floor, and is effectively unchanged by this diff since no non-test source was added.
- The new logic lives entirely in `_test.go` files (one untagged, one behind `readme_install`); Go's coverage tool does not instrument test files, so there is no meaningful line-coverage percentage to attribute to it. Correctness was instead verified behaviorally: 15 extractor subtests + 7 hermetic-assertion subtests + 1 real exec test, plus the 8 live mutation/anti-vacuity experiments in the table above (floor guard, independence of each subtest, unknown-template-action detection, fence-language hard-failure, and a from-scratch unchecked-install-method addition).
- Missing coverage: none identified. All four `behaviors` in the task YAML map to a named, passing test.

## Pre-audit gate

`./scripts/ci-local.sh --go` — **PASS** (exit 0), full output ending `=== ci-local PASS ===`, including the new "install: every README install command is executed (M25-003)" step and the dogfood/redaction/openapi/crosscheck harnesses.

## Scope boundary respected

`git tag -l` printed exactly `v0.1.0` before, during (checked after every mutation), and after this review. No `goreleaser release` (snapshot or otherwise) was run. `git status --porcelain` was empty at the end of every mutation cycle and is empty now. No repository-visibility or account-setting change was made or requested.

## Summary

Every checkable claim in the plan and the CHANGELOG — the tag boundary, the `-list` guard's exact count, the private-repo/404 facts, the `go.mod`/`.goreleaser.yaml`-derived assertions, the independence of each subtest, the anti-vacuity template-action guard, the fence-language hard failure, and behavior 4's structural (not hand-listed) coverage — was independently reproduced against this tree during this review, including one check (behavior 4, a from-scratch unchecked install method) the written record described but that this review still exercised live rather than trusting. Nothing was found to be merely plausible rather than true. One real but out-of-scope defect (`docs/MANUAL.md`'s CI recipes) was found in passing and flagged separately rather than held against this diff. Zero findings at any severity.
