# Improvement Report: M5-014

**Task:** go-cli: license export bundle for air-gapped use
**Date:** 2026-04-20
**Review:** management/reviews/M5-014-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `ErrReadOnlyBundle` in `export/bundle.go` duplicates the authoritative sentinel in `internal/license/store.go` with a different error string, confusing `errors.Is` callers | Removed `ErrReadOnlyBundle` from `internal/license/export/bundle.go`; the canonical sentinel with message `"license: bundle is read-only"` remains in `store.go` | ✓ tests pass |
| 2 | High | `ErrOutputParentMissing` exported from `export/bundle.go` but never returned by any function — dead exported identifier | Removed `ErrOutputParentMissing` from `internal/license/export/bundle.go`; the CLI checks the parent directory inline and surfaces the error as a plain message | ✓ tests pass |
| 3 | Medium | Missing `TestLicenseExport_TamperedBundle` for Behavior 6: tamper bundle after export, validate via `CURLEW_LICENSE_BUNDLE`, expect rc=6 + `signature_invalid` | Added `TestLicenseExport_TamperedBundle` in `cmd/curlew/license_test.go` — exports a bundle, extracts it, corrupts the JWT signature in `license.json`, sets `CURLEW_LICENSE_BUNDLE`, runs `license --validate`, asserts rc=6 and `"signature_invalid"` in stderr | ✓ tests pass |
| 4 | Medium | `gz.Close()` not called when `tw.Close()` returns an error in `writeTarGz`, leaving the gzip footer unwritten and memory unreleased | Rewrote close path in `archive.go`: call `gz.Close()` on every early-return error path; in the normal path join both `tw.Close()` and `gz.Close()` errors via `errors.Join` | ✓ tests pass |
| 5 | Medium | Duplicate `// 3.` step comment in `resolver.go` `Resolve()` — the online-fetch step was numbered 3 instead of 4 | Changed comment on the online-fetch branch from `// 3.` to `// 4. Online fetch (only when not offline).` | ✓ lint clean |
| 6 | Low | Pre-existing license tests (6 tests in `cmd/curlew/license_test.go`) do not call `t.Setenv("CURLEW_LICENSE_BUNDLE", "")`, risking cross-contamination if the env var is set in a developer's shell | Added `t.Setenv("CURLEW_LICENSE_BUNDLE", "")` to all pre-existing tests that set `CURLEW_CONFIG_DIR` but did not clear the bundle env var | ✓ tests pass |
| 7 | Low | `TestLicenseBundleGraceWarning` only tested the day-25 (within grace) regime; Behavior 3 requires a post-30-day assertion | Split into two sub-tests: `day25_grace_warning` (StateGracePeriod, exit 0, bundle warning appears) and `day31_grace_expired_still_warns` (StateGraceExpired, exit 0, bundle warning still appears and `State: GRACE_EXPIRED` in stdout) | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage — `internal/license/export` | 80.6% |
| Coverage — `internal/license` | 88.0% |
| Coverage — `cmd/curlew` | 81.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 6877f05 | fix(license/export): resolve High/Medium review findings | #1, #2, #4, #5 |
| 7c4c736 | test(license): resolve Low/Medium test review findings | #3, #6, #7 |

## Summary

7/7 findings resolved. 0 deferred.
