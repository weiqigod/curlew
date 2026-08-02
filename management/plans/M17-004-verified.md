# Verification Report: M17-004

**Task:** Webhook-signature dynamic functions: $webhookSign.stripe / .github / .slack
**Verified by:** AI
**Date:** 2026-04-29
**Branch:** feature/M17-004-webhook-sign-stripe-github-slack
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Webhook-sign Stripe section: t=...,v1=... header rendered, secret not leaked |
| Coverage (`internal/variable/...`) | 97.4% | Exceeds >= 80% threshold |

## Observable Output

```
go build ./cmd/curlew   → BUILD OK

TestRegistry_WebhookSign_Stripe              → PASS
TestRegistry_WebhookSign_GitHub              → PASS
TestRegistry_WebhookSign_Slack               → PASS
TestRegistry_WebhookSign_DottedNamespaceRequired  → PASS (3 sub-cases)
TestRegistry_WebhookSign_SecretSensitivePropagation → PASS (3 sub-cases)
TestRegistry_WebhookSign_ArityErrors          → PASS (9 sub-cases)
TestWebhookSig_Helpers_StripeVector          → PASS
TestWebhookSig_Helpers_GitHubVector          → PASS
TestWebhookSig_Helpers_SlackVector           → PASS

Smoke: PASS: Stripe-Signature t=...,v1=... header rendered
Smoke: PASS: secret not visible in serialised output
```

Expected: All tests PASS, smoke shows `t=...,v1=...` in header, secret not in JSON output.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `$webhookSign.stripe(body, secret, timestamp)` returns `t=<ts>,v1=<hex>` (HMAC-SHA256 over `<ts>.<body>`) | `TestRegistry_WebhookSign_Stripe` (3-arg), `TestWebhookSig_Helpers_StripeVector` | PASS |
| 2 | `$webhookSign.stripe(body, secret)` uses clock seam for timestamp | `TestRegistry_WebhookSign_Stripe` (2-arg + `frozenClock`) | PASS |
| 3 | `$webhookSign.github(body, secret)` returns `sha256=<hex>` (HMAC-SHA256 over body) | `TestRegistry_WebhookSign_GitHub`, `TestWebhookSig_Helpers_GitHubVector` | PASS |
| 4 | `$webhookSign.slack(body, secret, timestamp)` returns `v0=<hex>` and 2-arg form uses clock seam | `TestRegistry_WebhookSign_Slack`, `TestWebhookSig_Helpers_SlackVector` | PASS |
| 5 | Wrong arity returns structured `DYNFN_ARITY` error (9 combinations) | `TestRegistry_WebhookSign_ArityErrors` | PASS |
| 6 | Secret from sensitive-named variable propagated to `SensitiveSet` | `TestRegistry_WebhookSign_SecretSensitivePropagation` (3 providers) | PASS |
| 7 | Literal secret does not mutate `SensitiveSet` (documented footgun) | `TestRegistry_WebhookSign_LiteralSecret_NotMarked` | PASS |
| 8 | Non-dotted name (`webhookSignStripe` etc.) returns unknown-function error | `TestRegistry_WebhookSign_DottedNamespaceRequired` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | All 10 webhook-sign tests PASS | PASS |
| 2 | `go test ./...` passes | All packages pass in ci-local.sh | PASS |
| 3 | Coverage `internal/variable/...` >= 80% | 97.4% | PASS |
| 4 | `golangci-lint run` passes with 0 issues | ci-local.sh lint gate: PASS | PASS |
| 5 | `./smoke/run.sh` passes | Webhook-sign Stripe section passes | PASS |
| 6 | `./scripts/ci-local.sh` passes | `=== ci-local PASS ===` | PASS |
| 7 | Published-spec test vectors for all 3 providers | `TestWebhookSig_Helpers_*Vector` — byte-exact via `independentHMACHex` | PASS |
| 8 | `docs/MANUAL.md §3.7` updated with rows + examples + footgun callout | Added subsection with table + per-provider examples | PASS |
| 9 | Smoke fixture proves Stripe-style header end-to-end with secret never in output | `smoke/run.sh` webhook-sign Stripe block passes both checks | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS trusted (management/reviews/M17-004-review.md verdict: PASS). Spot-check clean:
- `hmacHexLower`, `stripeWebhookSig`, `githubWebhookSig`, `slackWebhookSig` — all have doc comments, no stuttering, correct package-level naming.
- `twoOrThreeArgs` returns `&apierrors.Structured{Code: "DYNFN_ARITY"}` — structured error, not panic.
- `sensitiveArgIdx["webhookSign.*"] = 1` wiring correctly reuses M12-005 plumbing; no changes to `variable.go`.

## Commits

| Hash | Message |
|------|---------|
| 3a481b76 | docs(review): add passing review for M17-004 |
| 7e695d53 | docs(review): add improvement report for M17-004 |
| 62b8d554 | fix(task): resolve review findings for M17-004 |
| a129a383 | docs(review): add review with findings for M17-004 |
| 57a616ce | chore(task): mark M17-004 as review |
| 844ef458 | docs(variable): update CHANGELOG.md for M17-004 webhook-sign functions |
| 49894c0a | feat(variable): add webhook-sign Stripe smoke fixture to smoke/run.sh |
| a01f0b8d | docs(variable): add $webhookSign.stripe/.github/.slack to MANUAL.md §3.7 |
| 6d264c98 | test(variable): add sensitive-secret propagation tests for webhook-sign (Step 4) |
| 53b4c840 | test(variable): add registry tests for webhook-sign stripe/github/slack (Step 3) |
| 32e92594 | feat(variable): add twoOrThreeArgs helper and register webhook-sign functions |
| fdba9f45 | test(variable): add failing arity tests for webhook-sign functions (Step 2) |
| 3963cbcf | feat(variable): add webhook-sig helper functions |
| d3acdd3b | test(variable): add failing tests for webhook-sig helper functions (Step 1) |

TDD pattern confirmed: `test(...)` commits precede their `feat(...)` counterparts.

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/variable/dynamic.go` | modified | `twoOrThreeArgs` helper + 3 new registrations with `sensitiveArgIdx` |
| `internal/variable/dynamic_helpers.go` | created | `hmacHexLower`, `stripeWebhookSig`, `githubWebhookSig`, `slackWebhookSig` |
| `internal/variable/dynamic_test.go` | modified | 10 new tests across all 8 behaviors |
| `docs/MANUAL.md` | modified | §3.7 webhook-sign subsection with table, examples, footgun callout |
| `smoke/run.sh` | modified | Webhook-sign Stripe block (dry-run, secret-redaction check) |
| `CHANGELOG.md` | modified | M17-004 entry under [Unreleased] |
| `management/backlog.yaml` | modified | M17-004 status → review |
| `management/tasks/M17-004.yaml` | modified | status → review |
| `management/plans/M17-004-plan.md` | created | Implementation plan |
| `management/reviews/M17-004-review.md` | created | Code review (PASS) |
| `management/plans/M17-004-improved.md` | created | Improvement report |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
