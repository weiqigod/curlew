# Improvement Report: M14-013

**Task:** Backend: Stripe invoice + payment-method event handlers
**Date:** 2026-05-06
**Review:** management/reviews/M14-013-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Operator precedence bug in `FormatBillingPeriod` at line 270: `&&` binds tighter than `\|\|`, causing the December→January cross-year branch to escape the `start.Date == startMonthFirst && end.Date == endMonthFirst` boundary guard. Any mid-month invoice spanning December→January returned "December yyyy" instead of the date-range format. | Wrapped the `\|\|` operands in explicit parentheses so the boundary guard applies to both the normal-month and December→January sub-expressions. See `InvoiceAndPaymentHandlers.cs` line 270. | ✓ failing test added first (RED), then fix applied (GREEN), 990 tests pass |
| 2 | Medium | Tests only asserted key presence in `Variables` dict (`ContainKey`), not formatted values. The December→January boundary case was entirely untested, making finding #1 uncatchable. | Added `FormatBillingPeriod_formats_correctly` table-driven theory with 7 cases (full May, full January, full November, full December→January cross-year, mid-month same-month, non-full cross-month, mid-December→mid-January cross-year). Added `FormatAmount_formats_correctly` theory with 5 cases. Added `PaymentSucceeded_email_variables_have_correct_formatted_values` and `PaymentFailed_email_variables_have_correct_formatted_values` that assert formatted values not just key presence. Total: 14 new test cases. | ✓ all 14 new tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS (990 passed, 8 skipped) |
| `go build ./cmd/apitest` | PASS |
| `~/go/bin/golangci-lint run` | PASS (0 issues) |
| Coverage | 93.9% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 84c926b1 | fix(webhooks): fix operator precedence bug in FormatBillingPeriod + add value assertions | #1, #2 |

## Summary

2/2 findings resolved. 0 deferred.

Both the production bug (operator precedence in `FormatBillingPeriod`) and the test quality gap (no value assertions, no December→January boundary coverage) were resolved in a single commit following TDD: the failing test for the December→January mid-month case was written first and confirmed RED, then the production fix was applied to bring it GREEN.
