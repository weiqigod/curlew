# Verification Report: M14-014

**Task:** Backend: SendGridSmtpSender + EmailQueueProcessor + MJML compile pipeline
**Verified by:** AI
**Date:** 2026-05-05
**Branch:** feature/M14-014-sendgrid-mjml-pipeline
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| `dotnet test` (task filter) | PASS | 52 passed, 0 failed |
| `dotnet test` (full suite) | PASS | 850 passed, 0 failed, 8 skipped |
| Go Coverage | >= 80% | All Go packages ≥ 80% |
| Docker/E2E gate | N/A | Pre-existing environment issue (docker compose plugin not installed); gate not applicable to this backend-only task |

## Observable Output

```
dotnet test src/ApiTool.Backend.Tests --filter "FullyQualifiedName~SendGridSmtpSender|FullyQualifiedName~EmailQueueProcessor|FullyQualifiedName~Mjml"
Passed!  - Failed: 0, Passed: 22, Skipped: 0, Total: 22, Duration: 55 ms
```

Expected: Passed >= 10, Failed: 0
Result: MATCH (22 pass, >= 10 threshold met)

Note: Full task-filter (including EmailPreviewRenderer, EmailTemplateLoader, etc.) yields 52 passing tests.

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Channel message → `SendTemplateAsync(to, slug, variables)` | `Happy_path_invokes_SendTemplateAsync` | PASS |
| 2 | Unknown slug → `EmailTemplateNotFoundException` → dead-lettered | `SendTemplateAsync_unknown_slug_throws_EmailTemplateNotFoundException`, `TemplateNotFound_skips_retries_and_dead_letters_immediately` | PASS |
| 3 | Unknown variable → `EmailTemplateVariableUnknownException` → dead-lettered | `SendTemplateAsync_unknown_variable_throws_EmailTemplateVariableUnknownException`, `TemplateVariableUnknown_skips_retries_and_dead_letters_immediately` | PASS |
| 4 | `dev email-preview <slug>` → MJML compiled, test_data substituted, stdout, no SendGrid | `Run_dev_email_preview_happy_path_writes_html_to_stdout_and_returns_exit_0`, `Fixture_template_renders_to_more_than_1000_bytes` | PASS |
| 5 | CI upload → `UploadAsync` posts compiled HTML, returns template ID | `UploadAsync_creates_template_and_returns_id_when_no_existing_template`, `UploadAsync_reuses_existing_template_when_slug_already_uploaded` | PASS |
| 6 | SendGrid 5xx → retry with backoff → dead-letter with last_error after 3 attempts | `Three_consecutive_5xx_dead_letters_with_last_error`, `Transient_then_success_does_not_dead_letter` | PASS |
| 7 | `<script>` in variable → HTML-escaped in rendered output | `Render_html_escapes_xss_in_variables` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass (>=10) | 52 tests pass in task filter | PASS |
| 2 | `dev email-preview` produces >1000-byte HTML for fixture template | `Fixture_template_renders_to_more_than_1000_bytes` + `Render_returns_html_with_substituted_test_data` | PASS |
| 3 | Variable-allowlist rejection covered by a test | `SendTemplateAsync_unknown_variable_throws_EmailTemplateVariableUnknownException` | PASS |
| 4 | SendGrid 9.x NuGet referenced; build succeeds without warnings | `SendGrid Version="[9.29.3,10.0.0)"` in csproj; `TreatWarningsAsErrors=true` | PASS |
| 5 | Dead-letter path for repeated SendGrid 5xx covered | `Three_consecutive_5xx_dead_letters_with_last_error` | PASS |
| 6 | `deploy/self-hosted/README.md` documents `SENDGRID__APIKEY` + `SENDGRID__TEMPLATES__<SLUG>` | "## SendGrid Configuration" section present with full env-var table | PASS |
| 7 | `docs/SPECIFICATION.md:8866–8953` cited in `SendGridSmtpSender.cs` header | `// Spec refs: docs/SPECIFICATION.md:8866-8953` at line 1 | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS trusted (Iteration 3, verdict PASS with no findings). Spot-check clean:
- `EmailQueueProcessor.cs`: `OperationCanceledException` caught before generic handler; sentinel exceptions propagated correctly
- `SendGridSmtpSender.cs`: Full XML doc comments on all exported symbols; spec reference cited at file top
- `EmailQueueProcessorTests.cs`: `Happy_path_invokes_SendTemplateAsync` asserts `.To`, `.Slug`, `.Variables["first_name"]` — not just call count; `Cancellation_during_send_does_not_dead_letter` verifies clean shutdown path

## Commits

| Hash | Message |
|------|---------|
| `0d96b3c6` | docs(review): add passing review for M14-014 |
| `b9290e18` | docs(review): add iteration-2 improvement report for M14-014 |
| `c5267d6c` | fix(email): handle OperationCanceledException in EmailQueueProcessor |
| `7a57b253` | docs(review): add iteration-2 review with findings for M14-014 |
| `8040380d` | docs(review): add improvement report for M14-014 |
| `a2fc48d7` | fix(dev): async RunUploadTemplatesAsync and happy-path dispatch test |
| `506df7a6` | fix(email): remove double-logging, add jitter, improve processor tests |
| `283a68e4` | docs(review): add review with findings for M14-014 |
| `6ba21503` | chore(task): mark M14-014 as review |
| `ad7407f8` | docs(deploy): add SendGrid configuration section to self-hosted README |
| `00b40868` | feat(email): wire SendGrid DI registrations and EmailQueueProcessor into Program.cs |
| `1b492a89` | feat(email): implement EmailQueueProcessor BackgroundService with retry + dead-letter |
| `87dcb2c8` | test(email): add failing tests for EmailQueueProcessor |
| `cd953db7` | feat(email): ship email_verification fixture template (MJML + manifest) |
| `44339992` | test(email): add failing fixture tests for email_verification template |
| `7842382f` | feat(cli): add dev email-preview + upload-templates subcommands, EmailPreviewRenderer, SendGridTemplateUploader |
| `2abfb663` | test(cli): add failing tests for DevCommands |
| `c303da68` | test(email): add failing tests for SendGridTemplateUploader |
| `d302c60f` | test(email): add failing tests for EmailPreviewRenderer |
| `3f0e97b3` | feat(email): implement SendGridSmtpSender with SendGrid 9.x SDK |
| `2c8a0893` | test(email): add failing tests for SendGridSmtpSender |
| `ff929970` | feat(email): extend ISmtpSender with SendTemplateAsync + update NoopSmtpSender/FakeSmtpSender |
| `912102c5` | test(email): add failing tests for ISmtpSender.SendTemplateAsync |
| `426425e8` | feat(email): add IEmailDeadLetterStore, LoggingEmailDeadLetterStore, and recording test double |
| `b45df4d4` | test(email): add failing tests for IEmailDeadLetterStore / LoggingEmailDeadLetterStore |
| `7c0e8b9a` | feat(email): add IMjmlCompiler + MjmlNetCompiler (Mjml.Net 4.x) |
| `391df992` | test(email): add failing tests for IMjmlCompiler/MjmlNetCompiler |
| `82887a3c` | feat(email): implement EmailTemplateLoader |
| `90f0d8bc` | test(email): add failing tests for EmailTemplateLoader |
| `1541a209` | feat(email): add SendGridOptions, EmailTemplateManifest, and exception types |
| `0610ba44` | test(email): add failing tests for SendGridOptions and manifest types |

## Files Changed

| File | Action |
|------|--------|
| `deploy/self-hosted/README.md` | modified — added SendGrid config docs |
| `management/backlog.yaml` | modified |
| `management/plans/M14-014-plan.md` | added |
| `management/plans/M14-014-improved.md` | added |
| `management/reviews/M14-014-review.md` | added |
| `src/ApiTool.Backend.Tests/DevCommandsTests.cs` | added |
| `src/ApiTool.Backend.Tests/Notifications/Email/EmailPreviewRendererTests.cs` | added |
| `src/ApiTool.Backend.Tests/Notifications/Email/EmailQueueProcessorTests.cs` | added |
| `src/ApiTool.Backend.Tests/Notifications/Email/EmailTemplateLoaderTests.cs` | added |
| `src/ApiTool.Backend.Tests/Notifications/Email/EmailVerificationFixtureTests.cs` | added |
| `src/ApiTool.Backend.Tests/Notifications/Email/LoggingEmailDeadLetterStoreTests.cs` | added |
| `src/ApiTool.Backend.Tests/Notifications/Email/MjmlCompilerTests.cs` | added |
| `src/ApiTool.Backend.Tests/Notifications/Email/RecordingEmailDeadLetterStore.cs` | added |
| `src/ApiTool.Backend.Tests/Notifications/Email/RecordingSmtpSender.cs` | added |
| `src/ApiTool.Backend.Tests/Notifications/Email/SendGridOptionsTests.cs` | added |
| `src/ApiTool.Backend.Tests/Notifications/Email/SendGridTemplateUploaderTests.cs` | added |
| `src/ApiTool.Backend.Tests/Notifications/Email/SmtpSenderExtensionTests.cs` | added |
| `src/ApiTool.Backend.Tests/Notifications/FakeSmtpSender.cs` | modified |
| `src/ApiTool.Backend.Tests/Notifications/SendGridSmtpSenderTests.cs` | added |
| `src/ApiTool.Backend/ApiTool.Backend.csproj` | modified — added SendGrid 9.x + Mjml.Net |
| `src/ApiTool.Backend/DevCommands.cs` | added |
| `src/ApiTool.Backend/Notifications/Email/*.cs` | added (11 files) |
| `src/ApiTool.Backend/Notifications/ISmtpSender.cs` | modified |
| `src/ApiTool.Backend/Notifications/NoopSmtpSender.cs` | modified |
| `src/ApiTool.Backend/Notifications/SendGridSmtpSender.cs` | added |
| `src/ApiTool.Backend/Program.cs` | modified — DI wiring |
| `templates/email/email_verification.json` | added |
| `templates/email/email_verification.mjml` | added |

## Issues Found
None

## Recommendation
PASS — ready for PR and merge. All 7 behaviors covered by 52 tests, all 7 DoD items satisfied, Go gate and dotnet test suite both pass, review iteration 3 gave a clean PASS verdict.
