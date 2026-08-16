# Improvement Report: M25-001

**Task:** Exercise the release build before the day it is needed
**Date:** 2026-08-16
**Review:** management/reviews/M25-001-review.md (verdict: FAIL, 3 findings)

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `CHANGELOG.md:36-42` repeated a false, already-retracted measurement claiming `goreleaser build --snapshot` "accepts [the undefined template variable] silently and links a binary carrying the same `0.1.0-dev` default instead." `management/plans/M25-001-plan.md` had retracted the identical claim in `05ce503`, but `CHANGELOG.md` was never updated to match. | Rewrote the paragraph to state the true, repeatedly-measured behavior: the mutation fails at the **build** step itself (exit 1, `map has no entry for key "NoSuchVar"`, `dist/` left with zero files — no binary is ever linked). Added an explicit **"Correction to an earlier revision of this entry"** paragraph naming the false claim, stating it does not reproduce, cross-referencing the plan file's retraction, and noting `CHANGELOG.md` is deliberately outside this project's doc-prose/doc-table checks (`cmd/curlew/doc_prose_test.go:35-39`, `internal/docs/inventory.go:10-13`) so nothing automated would have caught the drift. Does not silently swap one claim for another — the retraction is stated in the text itself. | Independently re-verified the true behavior by re-running the mutation directly (`goreleaser build --snapshot --clean --single-target` on `{{ .Version }}` → `{{ .NoSuchVar }}`) before writing the correction. Full gate re-run below confirms the corrected file ships. |
| 2 | Low | `scripts/ci-local.sh:286` — under `set -o pipefail`, `find dist -type f -name curlew` failing on a missing `dist/` aborted the assignment before the intended "expected exactly one built curlew under dist/, found N" message could print, so the operator saw `find`'s raw stderr instead. | Added a `[ -d dist ]` guard before the `find`, short-circuiting to `release_bin_count=0` when `dist/` doesn't exist, so the existing `if [ "$release_bin_count" != "1" ]` branch (and its message) fires in that case too. Deliberately **not** the review's literal suggestion (`find ... 2>/dev/null \|\| release_bin_count=0`), which would fold *any* `find` failure (e.g. a permission error) into "found 0" — a count `find` never produced, which is the same false-clear class this step exists to prevent. The `-d` guard narrows the swallow to the one case where `0` is actually true. | RED: extracted the pre-fix 1-line assignment and ran it against a missing `dist/` — printed only `find: dist: No such file or directory`, exit 1, custom message never shown. GREEN: extracted the real patched lines 294-302 from the file post-fix and ran them the same way — printed `expected exactly one built curlew under dist/, found 0`, exit 1. Regression check: same patched lines against a `dist/` containing exactly one `curlew` file gave `count=[1]`, byte-identical to the pre-fix output. Full gate (which exercises the real, populated `dist/` case) re-run below, exit 0. |
| 3 | Low | `.github/workflows/go.yml:45-46` — `go install github.com/goreleaser/goreleaser/v2@v2.17.1` runs under Go 1.24 while goreleaser v2.17.1 requires Go >= 1.26.5. This works (`GOTOOLCHAIN=auto` transparently fetches the newer toolchain), but the comment said nothing about it, while `release.yml`'s comment for the identical mismatch explains it in detail and deliberately avoids `go install` for that reason. | Added a comment block explaining that `GOTOOLCHAIN=auto` (the unmodified default in this repo) is why `go install` works despite the version gap, what was measured (dev host at go1.25.5 fetched go1.26.6, exit 0), and why `release.yml` makes a different choice for the same underlying gap (installs a prebuilt binary via `goreleaser-action` instead, to avoid paying a toolchain download on release day). | Re-confirmed the underlying facts before writing the comment: `go.mod` declares `go 1.24.0` with no `toolchain` directive; goreleaser v2.17.1's own `go.mod` header reads `go 1.26.5`; `GOTOOLCHAIN` is not overridden anywhere in this repo (grepped). Comment-only change to a `workflow_dispatch`-only workflow; no execution surface. |

## Out of Scope (Deferred)

No findings deferred. All 3 findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS (all packages ok) |
| `go test -race $(go list ./... \| grep -v /node_modules/)` | PASS (run inside `ci-local.sh --go` below) |
| `golangci-lint run` | PASS (0 issues) |
| Coverage | 83.7% (statements, `go tool cover -func=coverage.out`) |
| `go test ./internal/backlog/ -run TestBacklog_repository_is_consistent -v` | PASS — 243 tasks across 85 capabilities; M25-001 still `review` (task status untouched, per pipeline rules) |
| `./scripts/ci-local.sh --go` (full gate, foreground) | **PASS** — exit 0, `=== ci-local PASS ===`. Release gate steps confirmed in output: `goreleaser 2.17.1`, `check` validated, snapshot build succeeded, artifact reported `curlew 0.0.1-snapshot` |
| `git diff main --exit-code -- .goreleaser.yaml` | exit 0 — byte-identical to `main` |
| `git diff --exit-code -- internal/uiserver/assets/dist/index.html` | exit 0 — unaffected by the release build |
| `git status --porcelain` (post-gate) | empty |
| Tags / publishes | none — `git tag -l` empty; no `goreleaser release` invoked; nothing pushed |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `f86eee9` | fix(ci): report the artifact-count diagnostic when dist/ is absent | #2 |
| `866ec50` | docs(ci): explain go.yml's go install despite the goreleaser toolchain gap | #3 |
| `516f6c8` | docs(changelog): correct a false mutation measurement in the M25-001 entry | #1 |

Ordering: code/config fixes (#2, #3) committed before the CHANGELOG correction (#1) so the CHANGELOG's account of "what the release gate now does" describes a tree that already matches — deliberately mirroring the plan's own Step 6 rationale ("Documentation last, describing what the previous steps actually did rather than what they intended to").

## Summary
3/3 findings resolved. 0 deferred. Full gate (`./scripts/ci-local.sh --go`) passes end to end, `.goreleaser.yaml` remains byte-identical to `main`, and nothing was tagged, released, or published during this pass.
