# Improvement Report: M14-015

**Task:** Backend: 6-template inventory (MJML + manifests) + CI upload job
**Date:** 2026-05-05
**Review:** management/reviews/M14-015-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `account_security_alert.mjml` declared `first_name` in the manifest (spec-prescribed) but never referenced it in the MJML body — personalisation was silently dropped. No test caught this gap. | Added `Hi {{first_name}},` greeting paragraph to `account_security_alert.mjml`. Added new `[Theory]` test `Mjml_uses_all_declared_variables(string slug)` that asserts every key in `manifest.Variables` appears as `{{key}}` in the raw MJML for all six slugs (6 new tests). | Tests pass (52 total for `~EmailTemplateInventory`) |
| 2 | Medium | `RunUploadTemplatesAsync` used `Directory.GetFiles(root, "*.json")` instead of `EmailTemplateInventory.Slugs`, bypassing the canonical inventory and making any non-manifest JSON file in `templates/email/` a runtime failure, and any slug added to the directory-but-not-inventory a silent over-count. | Replaced the filesystem glob with `EmailTemplateInventory.Slugs` iteration. Updated `DevCommandsTests.UploadTemplates_uses_SENDGRID_API_BASE_env_var_when_set` assertion from `>= 6` to `== EmailTemplateInventory.Slugs.Count`. | Tests pass (8 DevCommands tests) |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (Go) | 87.3% |
| `dotnet test --filter "FullyQualifiedName~EmailTemplateInventory"` | PASS (52 tests) |
| `dotnet test --filter "FullyQualifiedName~DevCommands"` | PASS (8 tests) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `14150b23` | fix(email): add {{first_name}} to account_security_alert.mjml + test all variables used | #1 |
| `5a4bd028` | fix(email): use EmailTemplateInventory.Slugs in RunUploadTemplatesAsync | #2 |

## Summary

2/2 findings resolved. 0 deferred.
