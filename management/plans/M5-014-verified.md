# Verification Report: M5-014

**Task:** go-cli: license export bundle for air-gapped use
**Verified by:** AI
**Date:** 2026-04-20
**Branch:** feature/M5-014-license-export-bundle
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | M5-014 smoke section PASS |
| Coverage — `internal/license/export` | 80.6% | Meets >= 80% threshold |
| Coverage — `internal/license` | 88.0% | Above threshold |
| Coverage — `cmd/apitest` | 81.2% | Above threshold |
| Coverage — total | 86.7% | Well above threshold |

## Observable Output

```
Exporting license bundle...
Included: license token (valid until 2030-01-01), JWKS (1 keys)
Wrote /tmp/bundle_test.tar.gz (1.6 KB)
---
license.json
jwks.json
README.txt
```

Expected: stdout shows "Exporting license bundle...", "Included: license token (valid until ...)", "Wrote ... (... KB)" and tar listing shows license.json, jwks.json, README.txt
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Valid license → export → tarball with license.json/jwks.json/README.txt | `TestLicenseExport_HappyPath` + smoke | PASS |
| 2 | Extract bundle + APITEST_LICENSE_BUNDLE → validate succeeds offline | `TestLicenseExportThenValidate_RoundTrip` + smoke | PASS |
| 3 | Bundle older than 30 days → "Bundle grace period expires in N days" warning | `TestLicenseBundleGraceWarning` (day25 + day31 sub-tests) | PASS |
| 4 | No license → exit 2 + "no license to export; run apitest login first" | `TestLicenseExport_NoLicense` | PASS |
| 5 | --output parent dir missing → exit 2 + "output directory does not exist" | `TestLicenseExport_OutputParentMissing` | PASS |
| 6 | Tampered bundle signature → exit 6 + "signature_invalid" | `TestLicenseExport_TamperedBundle` | PASS |
| 7 | --help documents export --output and APITEST_LICENSE_BUNDLE env var | `TestLicenseHelp_DocumentsExport` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | 7/7 behavior tests pass | PASS |
| 2 | Observable output works as specified | Command output matches expected format | PASS |
| 3 | Test coverage >= 80% | export: 80.6%, license: 88.0%, cmd/apitest: 81.2% | PASS |
| 4 | No build warnings or lint errors | `golangci-lint run` → 0 issues | PASS |
| 5 | Help text documents license export and APITEST_LICENSE_BUNDLE env var | `TestLicenseHelp_DocumentsExport` passes, confirmed in binary output | PASS |
| 6 | Smoke test covers export → import → validate cycle | Smoke M5-014 section: PASS | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling (`%w` wrapping) | PASS |
| Sentinel errors | PASS |
| Naming conventions | PASS |
| Doc comments on exports | PASS |
| Code organization | PASS |
| Test quality (table-driven, TDD) | PASS |
| `context.Context` as first parameter | PASS |
| `defer` for cleanup | PASS |
| No goroutine leaks | PASS |

(Branch A: Review iteration 2 PASS trusted — spot-check confirmed clean error wrapping with `%w`, doc comments on `Report`, `Write`, `Bundle`, `ParseMembers`, and test functions exercise expected behavior)

## Commits

| Hash | Message |
|------|---------|
| 663abd3 | docs(review): add passing review for M5-014 |
| 220057a | docs(review): add improvement report for M5-014 |
| 7c4c736 | test(license): resolve Low/Medium test review findings |
| 6877f05 | fix(license/export): resolve High/Medium review findings |
| aa1bc3b | docs(review): add review with findings for M5-014 |
| d520769 | chore(task): mark M5-014 as review |
| 67ecf89 | feat(smoke): add M5-014 license bundle export/validate smoke test |
| 01ff886 | feat(cli): implement license export subcommand and bundle grace warning |
| 3d57a19 | test(cli): add failing tests for license export CLI and round-trip validation |
| 11d9d42 | feat(license): add ResolveConfigDir, ReadOnlyStore, bundle JWKS resolver, and FromBundle flag |
| ba9eaa9 | test(license): add failing tests for ResolveConfigDir, ReadOnlyStore, bundle JWKS resolver |
| 1e5c128 | feat(export): implement Write function — gather license+JWKS, write tarball |
| 39eddbf | test(export): add failing tests for Write function (exporter) |
| 45d2e39 | feat(export): implement Bundle type with SerializeMembers and ParseMembers |
| b8e36a2 | test(export): add failing tests for Bundle type serialize/parse |
| 79b8571 | feat(export): implement tar/gzip archive writer and reader |
| fa5e819 | test(export): add failing tests for tar/gzip archive round-trip |
| 2af38ae | chore(task): mark M5-014 as in_progress |
| 8f5d320 | chore(task): mark M5-014 as planned |
| 5d66669 | docs(plan): add implementation plan for M5-014 |

## Files Changed

| File | Action |
|------|--------|
| `cmd/apitest/license.go` | modified — added `licenseExport`, `humanBytes`, updated help text |
| `cmd/apitest/license_test.go` | modified — added 7 new tests, bundle env isolation |
| `internal/license/export/archive.go` | created — tar/gzip writer and reader |
| `internal/license/export/archive_test.go` | created — round-trip tests |
| `internal/license/export/bundle.go` | created — Bundle type, sentinels |
| `internal/license/export/bundle_test.go` | created — serialize/parse tests |
| `internal/license/export/export.go` | created — Write orchestrator |
| `internal/license/export/export_test.go` | created — Write tests |
| `internal/license/paths.go` | created — ResolveConfigDir, BundleEnv, ConfigEnv |
| `internal/license/paths_test.go` | created — env precedence tests |
| `internal/license/resolver.go` | modified — bundlePath, WithBundleJWKS, SourceBundle |
| `internal/license/store.go` | modified — ReadOnlyStore, ErrReadOnlyBundle |
| `internal/license/validator.go` | modified — FromBundle wiring |
| `smoke/run.sh` | modified — M5-014 export/extract/validate section |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
