# Code Review: M14-014 (Iteration 3)

**Task:** Backend: SendGridSmtpSender + EmailQueueProcessor + MJML compile pipeline
**Reviewer:** AI
**Date:** 2026-05-05
**Branch:** feature/M14-014-sendgrid-email-pipeline

## Verdict: PASS

## Findings

No findings.

## Previous Findings — Resolution Status

All findings from iterations 1 and 2 are confirmed resolved:

| Iteration | Finding | Status |
|-----------|---------|--------|
| 1 | Double-logging per dead-letter | **Fixed** — `DeadLetterAsync` uses `LogWarning`; only `LoggingEmailDeadLetterStore` emits `LogError` |
| 1 | Missing ±20% jitter on retry delays | **Fixed** — `jitteredMs = baseDelay * (0.8 + Random.Shared.NextDouble() * 0.4)` |
| 1 | `Happy_path` only asserted count, not `(to, slug, variables)` values | **Fixed** — test asserts `.To`, `.Slug`, and `.Variables["first_name"]` |
| 1 | Processor permanent 4xx path not tested | **Fixed** — `Permanent_4xx_dead_letters_immediately_without_retry` added |
| 1 | `DevCommandsTests` only tested arg-parsing; dispatch happy path untested | **Fixed** — `Run_dev_email_preview_happy_path_writes_html_to_stdout_and_returns_exit_0` added |
| 1 | `RunUploadTemplates` blocked with `.GetAwaiter().GetResult()` inside async code | **Fixed** — `RunUploadTemplatesAsync` is `async Task<int>`; top-level sync `Run()` correctly blocks via `.GetAwaiter().GetResult()` (acceptable for CLI tools mode) |
| 2 | `OperationCanceledException` during `SendTemplateAsync` fell to generic handler, producing false dead-letters on shutdown | **Fixed** — `catch (OperationCanceledException) when (ct.IsCancellationRequested) { return; }` added; `Cancellation_during_send_does_not_dead_letter` test verifies the fix |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Sentinel exceptions (`EmailTemplateNotFoundException`, `EmailTemplateVariableUnknownException`) defined and thrown before any HTTP call. `OperationCanceledException` correctly handled in `EmailQueueProcessor` — clean shutdown does not dead-letter. `EnsureSuccessStatusCode()` used in `SendGridTemplateUploader`. All error paths propagate with context. No swallowed errors. |
| Input Validation | PASS | Variable allowlist validated before any HTTP call. Slug lookup rejects missing template IDs. Manifest null/empty JSON check present. Empty variables correctly accepted (no missing-required-field enforcement, consistent with SendGrid's Dynamic Templates model). `EmailTemplateLoader` rejects missing `.mjml` and `.json` files with `FileNotFoundException`. |
| Naming | PASS | No stuttering. All exported types, methods, and interfaces have doc comments. `IMjmlCompiler` follows `-er`-suffix convention. Package structure follows C# naming conventions. |
| Code Organization | PASS | Package boundaries respected. Single responsibility per class. `ConfigureAwait(false)` used throughout async code. `using` for `JsonDocument` disposal in `SendGridTemplateUploader`. `BackgroundService` pattern mirrors existing `SchedulerHost`. `DevCommands` correctly intercepts args before web host builder. |
| Correctness | PASS | Retry logic correct: 3 attempts, exponential backoff with ±20% jitter, correct delay indexing (`attempt - 1`). Cancellation during send handled — no false dead-letters on shutdown. Dead-letter path exhaustion uses `lastError ?? new Exception(...)` defensive guard. Case-insensitive slug lookup via `StringComparer.OrdinalIgnoreCase` on `SendGridOptions.Templates`. Variable allowlist is case-sensitive per spec :8941-8942. XSS escaping via HandlebarsDotNet default HTML-encoding confirmed by test. |
| Test Quality | PASS | All 7 behaviors from the task YAML covered by tests. 52 tests in the task filter, 850 total, 0 failures. Behavior #6 retry exhaustion and retry-then-success both tested. Cancellation path tested with `CancelOnSendSmtpSender`. CLI dispatch tested end-to-end. `SendGridTemplateUploader` both create and reuse paths tested. XSS escaping tested against real Handlebars rendering. |

## Behavior Coverage

| # | Behavior | Test(s) |
|---|----------|---------|
| 1 | Channel message → `SendTemplateAsync(to, slug, variables)` | `Happy_path_invokes_SendTemplateAsync` |
| 2 | Unknown slug → `EmailTemplateNotFoundException` → dead-lettered | `SendTemplateAsync_unknown_slug_throws_EmailTemplateNotFoundException`, `TemplateNotFound_skips_retries_and_dead_letters_immediately` |
| 3 | Unknown variable → `EmailTemplateVariableUnknownException` → dead-lettered | `SendTemplateAsync_unknown_variable_throws_EmailTemplateVariableUnknownException`, `TemplateVariableUnknown_skips_retries_and_dead_letters_immediately` |
| 4 | `dev email-preview <slug>` → MJML compiled, test_data substituted, stdout, no SendGrid | `Run_dev_email_preview_happy_path_writes_html_to_stdout_and_returns_exit_0`, `Fixture_template_renders_to_more_than_1000_bytes` |
| 5 | CI upload → `UploadAsync` posts compiled HTML, returns template ID | `UploadAsync_creates_template_and_returns_id_when_no_existing_template`, `UploadAsync_reuses_existing_template_when_slug_already_uploaded` |
| 6 | SendGrid 5xx → retry with backoff → dead-letter with last_error after 3 attempts | `Three_consecutive_5xx_dead_letters_with_last_error`, `Transient_then_success_does_not_dead_letter` |
| 7 | `<script>` in variable → HTML-escaped in rendered output | `Render_html_escapes_xss_in_variables` |

## Definition of Done Verification

| DoD Item | Status |
|----------|--------|
| All behavior tests pass (≥10) | PASS — 52 tests pass in task filter |
| `dev email-preview` produces >1000-byte HTML for fixture template | PASS — `Fixture_template_renders_to_more_than_1000_bytes` + `Render_returns_html_with_substituted_test_data` |
| Variable-allowlist rejection covered by a test | PASS — `SendTemplateAsync_unknown_variable_throws_EmailTemplateVariableUnknownException` |
| SendGrid 9.x NuGet referenced; build succeeds without warnings | PASS — `SendGrid Version="[9.29.3,10.0.0)"` in csproj; `TreatWarningsAsErrors=true`; CI gate passes |
| Dead-letter path for repeated SendGrid 5xx covered | PASS — `Three_consecutive_5xx_dead_letters_with_last_error` |
| `deploy/self-hosted/README.md` documents `SENDGRID__APIKEY` + `SENDGRID__TEMPLATES__<SLUG>` | PASS — "## SendGrid Configuration" section present with full env-var table |
| `docs/SPECIFICATION.md:8866–8953` cited in `SendGridSmtpSender.cs` header | PASS — `// Spec refs: docs/SPECIFICATION.md:8866-8953` at line 1 |

## Test Coverage

- Task filter (`SendGridSmtpSender|EmailQueueProcessor|Mjml|EmailPreviewRenderer|EmailTemplateLoader|LoggingEmailDeadLetterStore|SendGridTemplateUploader|SendGridOptions|SmtpSenderExtension|EmailVerificationFixture|DevCommands`): **52 passed, 0 failed**.
- Full backend test suite: **850 passed, 0 failed, 8 skipped** (skipped tests are pre-existing Stripe live-mock tests unrelated to this task).

## Summary

Iteration 3 is clean. The iteration-2 finding (false dead-lettering of in-flight sends on server shutdown) is correctly resolved by the `catch (OperationCanceledException) when (ct.IsCancellationRequested)` guard, verified by the new `Cancellation_during_send_does_not_dead_letter` test. All 7 behaviors are covered by tests, all 7 definition-of-done items are satisfied, and the CI gate passes. The architecture — SendGrid 9.x sender with pre-HTTP validation, channel-backed `EmailQueueProcessor` with exponential-backoff retry, MJML/Handlebars pipeline, per-slug variable allowlist, `IEmailDeadLetterStore` seam, and `dev email-preview` CLI — is sound and complete.
