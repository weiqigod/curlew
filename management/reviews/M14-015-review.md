# Code Review: M14-015

**Task:** Backend: 6-template inventory (MJML + manifests) + CI upload job
**Reviewer:** AI
**Date:** 2026-05-05
**Branch:** feature/M14-015-email-template-inventory
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors returned, wrapped, or thrown appropriately. `LoadManifest` propagates `FileNotFoundException` and `InvalidDataException`; `DevCommands` catches per-slug and returns exit 1; no swallowed errors. |
| Input Validation | PASS | Unknown slug → `FileNotFoundException`; empty/null JSON → `InvalidDataException`; unknown template variable → `EmailTemplateVariableUnknownException` thrown before any HTTP call (asserted by `SendGridSmtpSender_rejects_unknown_variable`). |
| Naming | PASS | No stuttering; all exported types have doc comments; `NeverCalledSendGridClient` follows the test-double naming convention; `EmailTemplateInventory` is in the correct namespace. |
| Code Organization | PASS | `RunUploadTemplatesAsync` iterates `EmailTemplateInventory.Slugs` (the canonical single source of truth) — the previous finding (#2) is fixed. No filesystem globbing. |
| Correctness | PASS | All six MJML templates reference every declared variable from their manifest (verified by the new `Mjml_uses_all_declared_variables` [Theory] covering all six slugs). The previous finding (#1, `first_name` unused in `account_security_alert.mjml`) is fixed. `CancellationToken` is propagated. `HttpClient` is disposed via `using`. |
| Test Quality | PASS | 52 tests pass for `FullyQualifiedName~EmailTemplateInventory` (DoD threshold ≥ 12 met). 8 DevCommands tests pass. New `Mjml_uses_all_declared_variables` [Theory] closes the gap that allowed the previous finding to slip through. Assertions use `.Be(EmailTemplateInventory.Slugs.Count)` instead of the previous weak `>= 6`. |

## Test Coverage

- Go gate: PASS (all packages ≥ 80%, `ci-local.sh --go` green).
- Backend (.NET): 52 tests pass for `FullyQualifiedName~EmailTemplateInventory`; 8 for `FullyQualifiedName~DevCommands`.
- Observable verification: all six templates render non-empty HTML via `dev email-preview <slug>` (verified).
- No missing coverage identified.

## Summary

Both findings from the iteration-1 review are resolved. `account_security_alert.mjml` now references `{{first_name}}` in the body, and the new `Mjml_uses_all_declared_variables` parameterized test guards against this class of drift for all six slugs. `RunUploadTemplatesAsync` iterates `EmailTemplateInventory.Slugs` directly (not a filesystem glob), restoring the inventory class as the single source of truth. All 52 inventory tests and 8 DevCommands tests pass; the task observable passes end-to-end.
