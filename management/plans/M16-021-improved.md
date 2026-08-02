# Improvement Report: M16-021

**Task:** End-to-end happy-path workflow scenario across all M16 capabilities
**Date:** 2026-05-12
**Review:** management/reviews/M16-021-review.md

## Resolved Findings (Iteration 1 — 7 findings from first review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `{{ BACKEND_URL }}` template variable in `testdata/m16/e2e-collection.yaml` is unresolvable at runtime — the schedule worker always provides an empty EnvVars map on claim, causing the worker run to fail with "undefined variable" | Replaced `{{ BACKEND_URL }}/healthz` with hardcoded `http://localhost:5000/healthz`. Updated description comment to explain why the URL is hardcoded (not templated). | ✓ tests pass |
| 2 | High | `Returns_200_and_ticked_true` in `InternalTrialTickEndpointTests.cs` is misleadingly named — the test only verifies the endpoint is reachable (not 404); it cannot test the 200 path because `TrialExpiryNotifier` is not registered in Testing | Renamed to `Returns_not_404_when_registered_in_Testing`. Simplified assertion body to match the actual intent: `res.StatusCode.Should().NotBe(HttpStatusCode.NotFound)`. | ✓ tests pass |
| 3 | High | Step 13 in `m16-e2e.sh` is guarded by `if [ -n "$RESET_TOKEN" ]`, silently skipping the RFC 9700 §4.14 revocation assertion when the audit email is absent | Removed the conditional guard. If the token is absent after 10s polling, `fail()` is called with exit code 13. The revocation assertion is now always exercised. | ✓ tests pass |
| 4 | High | Step 11 uses `|| true`, ignoring all failure modes including genuine errors (not just 409). Post-activation JWT feature verification (Behavior 6) is absent | Replaced `|| true` with explicit exit code handling: 0 (activated) and 5 (already consumed per CLI taxonomy) are acceptable; any other code fails the script. Added post-activation token refresh and JWT payload check asserting `shared_vault_templates` in `features[]` or `trial_state=active`. | ✓ tests pass |
| 5 | Medium | Step 9 only asserts `totals.runs >= 1` — the `trend[today].runs >= 1` assertion (Behavior 9) is absent | Added trend array assertion: finds the entry whose date matches `$(date -u +%Y-%m-%d)` and asserts `runs >= 1`. Uses `>= 1` rather than `== 1` per Open Decision 9 (shared stacks may have pre-existing runs). | ✓ tests pass |
| 6 | Medium | `using System.Security.Cryptography;` unused import in `InternalTrialTickEndpointTests.cs` | Removed the unused import. | ✓ tests pass |
| 7 | Low | Step numbers jump from 6 to 8 in `m16-e2e.sh`, making the observable harder to reconcile against the spec | Added `step 7 "worker claimed and executed the scheduled run"` echo immediately after step 6 exits, confirming step 7 = step 6's completion. Step count is now contiguous 0–14. | ✓ tests pass |

## Resolved Findings (Iteration 2 — 1 finding from second review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | Exit code 11 (license trial start / post-activation JWT feature check) is used in three `fail()` calls (lines 233, 247, 263) in `scripts/m16-e2e.sh` but is absent from the header exit-code documentation table. | Added `#   11 — license trial start / post-activation JWT feature check failed (Step 11)` to the header exit-code table, in sequence between codes 10 and 12. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 87.0% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 5b92ee42 | fix(e2e): hardcode healthz URL in m16 e2e collection fixture | iter1 #1 |
| 13fb91c2 | fix(test): rename misleading test and remove unused import in InternalTrialTickEndpointTests | iter1 #2, #6 |
| cc3012fa | fix(e2e): resolve three high/medium findings in m16-e2e.sh orchestration script | iter1 #3, #4, #5, #7 |
| 3aa297ad | fix(e2e): add exit code 11 to m16-e2e.sh header exit-code table | iter2 #1 |

## Summary

8/8 findings resolved across 2 iterations. 0 deferred.
