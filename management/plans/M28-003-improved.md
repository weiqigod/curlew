# Improvement Report: M28-003

**Task:** The usage synopsis, and seven test stubs whose reason expired
**Date:** 2026-08-20
**Review:** management/reviews/M28-003-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `cmd/curlew/help_parity_test.go:218`'s `flagSurface` doc comment pointed to `docs/PRODUCT_ROADMAP.md` for detail on the deferred watch-synopsis follow-up (twelve missing flags), but that file contains no such mention — the note actually lives in `CHANGELOG.md`. | Repointed the comment to `CHANGELOG.md` (the file that already carries the full explanation, including the exact flag count and why the watch synopsis was left out of scope) and inlined the flag count so the pointer only needs to supply the *why*. `docs/PRODUCT_ROADMAP.md` was deliberately left untouched — per `CLAUDE.md`, it is "the open work" tracker and its M28-003 bullet is pruned when the milestone closes, so it is the less durable home for a permanent cross-reference; `CHANGELOG.md` is append-only history that keeps the entry (and the file name cited) even after `[Unreleased]` becomes a version heading. | ✓ `gofmt -l` clean, `go build ./cmd/curlew` OK, `go test ./...` all pass, `golangci-lint run ./cmd/curlew/...` 0 issues, `./scripts/ci-local.sh --go` PASS |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run ./cmd/curlew/...` | PASS (0 issues) |
| `./scripts/ci-local.sh --go` | PASS |
| Coverage (`cmd/curlew`) | 81.2% |
| Coverage (`internal/variable`) | 97.6% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `bfe946a` | fix(cmd/curlew): point flagSurface comment at where the note lives | #1 |

## Summary
1/1 findings resolved. 0 deferred.
