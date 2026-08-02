# Code Review: M17-004

**Task:** Webhook-signature dynamic functions: $webhookSign.stripe / .github / .slack
**Reviewer:** AI
**Date:** 2026-04-29
**Branch:** feature/M17-004-webhook-sign-stripe-github-slack

## Verdict: PASS

## Findings

No findings.

## Previous Findings (Iteration 1) — Both Resolved

| # | Severity | Finding | Resolution |
|---|----------|---------|------------|
| 1 | Low | `management/tasks/M17-004.yaml` had `status: planned` | Fixed: updated to `status: review` |
| 2 | Low | `TestRegistry_WebhookSign_ArityErrors` did not assert `err.Error()` contained `$funcName` | Fixed: `strings.Contains(err.Error(), "$"+tt.funcName)` check added at line 4707 |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All three providers return structured `DYNFN_ARITY` errors with correct `Code`, `Message`, `Hint`; `Evaluate` wraps with `fmt.Errorf("$%s: %w", name, err)`; no swallowed errors |
| Input Validation | PASS | Arity errors for all invalid arg counts (stripe/slack: <2 or >3; github: ≠2); empty-body and empty-secret cases handled correctly via `crypto/hmac` accepting empty byte slices per RFC 2104 |
| Naming | PASS | `twoOrThreeArgs`, `stripeWebhookSig`, `githubWebhookSig`, `slackWebhookSig`, `hmacHexLower` — all lowercase unexported, matching package conventions; no stuttering; doc comments on all helpers |
| Code Organization | PASS | New pure helpers isolated in `dynamic_helpers.go`; `dynamic.go` focused on registration; no circular deps; `sensitiveArgIdx` plumbing reused without changes to `variable.go` |
| Correctness | PASS | HMAC-SHA256 byte-exact against independent reference (`independentHMACHex`); clock-seam tests with `frozenClock`; sensitive-arg index 1 registered for all three providers; literal-secret no-op behaviour verified; `err.Error()` function-name propagation asserted in arity tests |
| Test Quality | PASS | Coverage 97.4% (well above 80%); all 8 task behaviors covered; table-driven arity tests; clock-seam-pinned two-arg tests; sensitive-propagation integration tests; literal-secret no-op test; smoke fixture proves end-to-end secret redaction |

## All Behaviors Verified

| # | Behavior | Test(s) |
|---|---------|---------|
| 1 | `$webhookSign.stripe(body, secret, timestamp)` returns `t=<ts>,v1=<hex>` | `TestRegistry_WebhookSign_Stripe` (3-arg), `TestWebhookSig_Helpers_StripeVector` |
| 2 | `$webhookSign.stripe(body, secret)` uses clock seam for timestamp | `TestRegistry_WebhookSign_Stripe` (2-arg + `frozenClock`) |
| 3 | `$webhookSign.github(body, secret)` returns `sha256=<hex>` | `TestRegistry_WebhookSign_GitHub`, `TestWebhookSig_Helpers_GitHubVector` |
| 4 | `$webhookSign.slack(body, secret, timestamp)` and 2-arg form | `TestRegistry_WebhookSign_Slack`, `TestWebhookSig_Helpers_SlackVector` |
| 5 | Wrong arity returns structured `DYNFN_ARITY` error | `TestRegistry_WebhookSign_ArityErrors` (9 sub-cases) |
| 6 | Secret from sensitive-named variable propagated to `SensitiveSet` | `TestRegistry_WebhookSign_SecretSensitivePropagation` (all 3 providers) |
| 7 | Literal secret does not mutate `SensitiveSet` (documented footgun) | `TestRegistry_WebhookSign_LiteralSecret_NotMarked` |
| 8 | Non-dotted name (`webhookSignStripe` etc.) returns unknown-function error | `TestRegistry_WebhookSign_DottedNamespaceRequired` |

## Test Coverage
- Coverage: **97.4%** (`internal/variable/...`)
- `dynamic_helpers.go`: 100% on all four helpers
- `twoOrThreeArgs`: fully exercised by arity error tests and happy-path tests
- Missing coverage: pre-existing gaps in unrelated helpers (`SensitiveArgIndex`, `generateSentence`, `isAllDigits`, `isLuhnValid`, `isIBANValid`) — none introduced by this slice

## Summary

The implementation is correct, complete, and meets all project standards. Both Low-severity findings from the first review iteration were resolved: the task YAML status is now `review` and the arity error tests assert function-name propagation through `Evaluate`'s wrapper. All three webhook-sign functions produce byte-exact HMAC-SHA256 signatures per provider spec, sensitive-secret propagation is correctly wired via the M12-005 `sensitiveArgIdx` mechanism, the smoke fixture proves end-to-end secret redaction, and MANUAL.md §3.7 and CHANGELOG.md are fully updated. No new findings.
