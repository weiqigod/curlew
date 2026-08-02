# Code Review: M5-014

**Task:** go-cli: license export bundle for air-gapped use
**Reviewer:** AI
**Date:** 2026-04-20
**Branch:** feature/M5-014-license-export-bundle
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All 7 issues from the previous review have been resolved.

## Resolved Since Last Review

| # | Severity | Finding | Status |
|---|----------|---------|--------|
| 1 | High | `ErrReadOnlyBundle` in `export/bundle.go` duplicated sentinel from `internal/license/store.go` with inconsistent error string | Removed from `export/bundle.go` — canonical sentinel in `store.go` only |
| 2 | High | `ErrOutputParentMissing` exported but never returned — dead exported identifier | Removed from `export/bundle.go` |
| 3 | Medium | Missing `TestLicenseExport_TamperedBundle` for Behavior 6 | Added — exports bundle, tampers JWT signature, validates via `APITEST_LICENSE_BUNDLE`, asserts rc=6 + `"signature_invalid"` |
| 4 | Medium | `gz.Close()` not called when `tw.Close()` fails in `writeTarGz` | Fixed — `gz.Close()` called on all early-return paths; normal path joins both errors via `errors.Join` |
| 5 | Medium | Duplicate `// 3.` step comment in `Resolve()` after adding bundle lookup step | Fixed — online-fetch step is now correctly numbered `// 4.` |
| 6 | Low | Pre-existing license tests missing `t.Setenv("APITEST_LICENSE_BUNDLE", "")` | Fixed — all 10+ pre-existing tests in `cmd/apitest/license_test.go` now explicitly clear the bundle env var |
| 7 | Low | `TestLicenseBundleGraceWarning` only tested day-25 (GRACE_PERIOD), not post-30-day regime | Fixed — split into two sub-tests: `day25_grace_warning` (StateGracePeriod) and `day31_grace_expired_still_warns` (StateGraceExpired) |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`. Sentinel errors used for well-known failures (`ErrNoLicenseToExport`, `ErrBundleIncomplete`, `ErrReadOnlyBundle`). No panics for expected failures. `ErrNoLicenseToExport` correctly translated to exit 2 in CLI. |
| Input Validation | PASS | nil/empty/missing cases handled. Context cancellation checked on entry to `Write`. Missing `--output` flag returns clear exit-2 error. Parent dir existence verified. `--output ""` treated as missing. |
| Naming | PASS | No stuttering. Short names in tight scopes. All exported symbols have doc comments. Package names are lowercase, single-word, no underscores. |
| Code Organization | PASS | Dead exported sentinels removed. `ErrReadOnlyBundle` in exactly one place (`store.go`). Clean separation: `archive.go` (tar/gzip I/O), `bundle.go` (type + serialization), `export.go` (orchestration). `internal/` boundaries respected. |
| Correctness | PASS | `gz.Close()` called on all exit paths in `writeTarGz`. Step numbering in `resolver.go` is sequential 1 through 4. `ResolveConfigDir` precedence correct (bundle > config > default). `ReadOnlyStore` refuses mutating operations with `ErrReadOnlyBundle`. |
| Test Quality | PASS | All 7 behaviors from the task YAML have dedicated tests. Pre-existing tests have bundle env var isolation. Bundle grace warning tested at day-25 (GRACE_PERIOD) and day-31 (GRACE_EXPIRED). Tampered-bundle test covers Behavior 6 end-to-end. |

## Test Coverage
- `internal/license/export`: 80.6% — meets the >=80% bar
- `internal/license`: 88.0% — above bar
- `internal/license/jwks`: 92.0% — above bar
- `cmd/apitest`: 81.2% — above bar

## Behavior Coverage

| Behavior | Test | Status |
|----------|------|--------|
| B1: valid license -> export -> tarball with license.json/jwks.json/README.txt | `TestLicenseExport_HappyPath` + smoke | PASS |
| B2: extract bundle + APITEST_LICENSE_BUNDLE -> validate succeeds offline | `TestLicenseExportThenValidate_RoundTrip` + smoke | PASS |
| B3: bundle older than 30 days -> "Bundle grace period expires in N days" warning | `TestLicenseBundleGraceWarning` (day25 + day31 sub-tests) | PASS |
| B4: no license -> exit 2 + "no license to export; run apitest login first" | `TestLicenseExport_NoLicense` | PASS |
| B5: --output parent dir missing -> exit 2 + "output directory does not exist" | `TestLicenseExport_OutputParentMissing` | PASS |
| B6: tampered bundle signature -> exit 6 + "signature_invalid" | `TestLicenseExport_TamperedBundle` | PASS |
| B7: --help documents export --output and APITEST_LICENSE_BUNDLE env var | `TestLicenseHelp_DocumentsExport` | PASS |

## Summary

All 7 findings from the previous review iteration were correctly resolved. The implementation delivers the full `apitest license export --output` command with gzip-tar bundle creation, offline import via `APITEST_LICENSE_BUNDLE`, read-only store semantics, key-resolver bundle JWKS lookup, bundle grace warning, and comprehensive test coverage for all specified behaviors. The gate is green, coverage exceeds 80% in every package, and lint is clean.
