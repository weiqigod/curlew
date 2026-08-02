# Improvement Report: M4-004

**Task:** Backend: test results ingestion API
**Date:** 2026-04-15
**Review:** management/reviews/M4-004-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `Post_results_returns_413_when_body_exceeds_5mb` only asserted HTTP status 413 but never verified the response body `code` field equals `"payload_too_large"` | Added `response.Content.ReadAsStringAsync()` + JSON parse + `.Should().Be("payload_too_large")` assertion after the status check | ✓ tests pass |
| 2 | Medium | `Get_results_returns_newest_first_for_member` used `>= 0` (vacuously true) to check ordering; required fields `fail_count`, `duration_ms`, `run_at` were never asserted | Replaced `>= 0` with `.Should().Be(2)` (newest payload has pass_count=2); added `TryGetProperty` presence checks for `fail_count`, `duration_ms`, and `run_at` | ✓ tests pass |
| 3 | Low | `RateLimiter_policy_results_ingest_is_registered` had a dead `options` variable and a trivially-true `_factory.Should().NotBeNull()` assertion — never verified the policy actually works | Rewrote test as a full integration test: creates org, POSTs a valid payload to the results endpoint, asserts 202 Accepted — proves the policy is registered and the endpoint is reachable | ✓ tests pass |
| 4 | Low | `Swagger_json_lists_results_endpoints` only checked for path strings in raw JSON; did not verify `post`/`get` method verbs as required by task behavior #8 | Replaced `json.Should().Contain(...)` with proper `JsonDocument` parse; asserts `post` verb on `/api/v1/organizations/{orgId}/results`, `get` verb on both org-results and `/api/v1/results/{resultId}` paths | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build` (backend) | PASS — 0 warnings |
| `dotnet test ./...` (91 tests) | PASS — 91/91 pass |
| `go build ./cmd/curlew` | PASS |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| ad6843b | fix(results): strengthen test assertions for review findings #1–#4 | #1, #2, #3, #4 |

## Summary

4/4 findings resolved. 0 deferred. All findings were in test quality (not production code). The implementation was already correct; the fixes tightened the tests to properly verify the behaviors specified in the task YAML.
