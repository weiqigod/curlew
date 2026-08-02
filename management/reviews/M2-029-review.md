# Code Review: M2-029 (Iteration 2)

**Task:** GraphQL protocol adapter (queries and mutations)
**Reviewer:** AI
**Date:** 2026-04-09
**Branch:** feature/M2-029-dashboard-export-sharing

## Verdict: PASS

## Previous Findings Resolution

All 5 substantive findings from the first review have been properly fixed:

| # | Original Finding | Resolution | Verified |
|---|-----------------|------------|----------|
| 1 | Shallow copy mutates caller's Headers map | Deep-copy headers map in BuildRequest + regression test | Yes |
| 2 | ParseErrorHandling error silently discarded in runner | Error now returned as request-scoped error | Yes |
| 3 | Parser does not validate error_handling values | Added validation + new sentinel ErrInvalidFieldValue | Yes |
| 4 | ErrGraphQLErrors defined but never used | Removed unused sentinel | Yes |
| 5 | No unit tests for requtil GraphQL interpolation | Added 8 unit tests in requtil_test.go | Yes |
| 6 | Branch name mismatch (Low) | Deferred -- cosmetic, does not affect functionality | Accepted |

## Findings

No new findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; sentinel errors properly defined and used; no swallowed errors |
| Input Validation | PASS | Parser validates protocol values, graphql.query presence, and error_handling values; BuildRequest validates nil config and empty query |
| Naming | PASS | All exported symbols have doc comments; no stuttering at import sites; package names correct |
| Code Organization | PASS | Clean package boundaries; internal/graphql/ owns its domain; minimal exported surface |
| Correctness | PASS | Headers map deep-copied; shallow copy for Request struct is safe; edge cases handled (nil, empty, invalid JSON) |
| Test Quality | PASS | Table-driven tests with descriptive names; error paths covered; edge cases tested; all 7 behaviors verified |

## Test Coverage
- `internal/graphql/`: 97.3% -- excellent
- `internal/parser/`: 90.7% -- well above threshold
- `internal/runner/`: 86.5% -- above threshold
- `internal/requtil/`: 85.7% for InterpolateRequest (the modified function); 58.5% overall due to pre-existing uncovered helper functions not modified by this task
- `internal/auth/`: 88.4% -- above threshold

## Spec Compliance

All 7 behaviors from the task YAML are covered by tests:

1. GraphQL query sent as POST with application/json -- TestRun_graphql_basic_query, TestBuildRequest
2. Variables included in request body -- TestRun_graphql_with_variables, TestBuildRequest
3. Mutations sent and $.data assertions work -- TestRun_graphql_mutation
4. $.errors with fail mode causes test failure -- TestRun_graphql_errors_fail_default
5. $.errors with warn mode does not fail -- TestRun_graphql_errors_warn
6. Free tier feature gate returns GateError -- TestRun_graphql_feature_gate_free_tier, TestCheckFeature_graphql, smoke test
7. JSONPath assertions work identically to HTTP -- TestRun_graphql_jsonpath_assertions

## Summary

All findings from the first review have been properly resolved. The code is well-structured, follows project standards, and has thorough test coverage. The GraphQL adapter correctly transforms requests into HTTP POST with JSON body, validates inputs at parse time, enforces feature gating, and handles error semantics (fail/warn) as specified.
