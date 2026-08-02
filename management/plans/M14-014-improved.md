# Improvement Report: M14-014 (Iteration 2)

**Task:** Backend: SendGridSmtpSender + EmailQueueProcessor + MJML compile pipeline
**Date:** 2026-05-05
**Review:** management/reviews/M14-014-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `OperationCanceledException` raised during `SendTemplateAsync` (in-flight HTTP send) fell through to the generic `catch (Exception ex)` handler in `EmailQueueProcessor.ProcessOneAsync`, triggering `DeadLetterAsync` on server shutdown and producing false dead-letter alerts on every rolling deployment | Added `catch (OperationCanceledException) when (ct.IsCancellationRequested) { return; }` between the `EmailTemplateVariableUnknownException` catch and the transient `IsTransient()` catch. TDD: added `Cancellation_during_send_does_not_dead_letter` test using `CancelOnSendSmtpSender` that blocks on `Task.Delay(Infinite, ct)` until `StopAsync` cancels the stoppingToken; asserts `deadLetters.Entries` is empty after shutdown | ✓ tests pass |

## Previous Findings (Iteration 1) — Already Resolved

| # | Original Finding | Status |
|---|-----------------|--------|
| 1 | Double-logging per dead-letter | **Fixed in iteration 1** |
| 2 | Missing ±20% jitter on retry delays | **Fixed in iteration 1** |
| 3 | `Happy_path` only asserted count, not `(to, slug, variables)` values | **Fixed in iteration 1** |
| 4 | Processor permanent 4xx path not tested | **Fixed in iteration 1** |
| 5 | `DevCommandsTests` only tested arg-parsing | **Fixed in iteration 1** |
| 6 | `RunUploadTemplates` blocked with `.GetAwaiter().GetResult()` | **Fixed in iteration 1** |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS |
| Task filter (SendGridSmtpSender, EmailQueueProcessor, Mjml) | PASS — 22 passed (1 new test added) |
| Full backend test suite | PASS — 850 passed, 0 failed, 8 skipped |
| `golangci-lint run` | PASS — 0 issues |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `c5267d6c` | fix(email): handle OperationCanceledException in EmailQueueProcessor | #1 |

## Summary

1/1 findings resolved. 0 deferred.
