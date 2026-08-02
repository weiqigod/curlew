# Improvement Report: M5-015

**Task:** Backend: self-hosted docker-compose bundle
**Date:** 2026-04-20
**Review:** management/reviews/M5-015-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `management/tasks/M5-015.yaml` had `status: backlog` instead of `status: review` | Updated `status: backlog` → `status: review` in the YAML | ✓ tests pass |
| 2 | Medium | `Health_returns_503_when_db_down` only asserted status code, not response body | Added `body.Should().Contain(...)` assertions for `"status":"unhealthy"`, `"db":"disconnected"`, `"redis":"connected"` | ✓ tests pass |
| 3 | Medium | No endpoint-level test for the redis-down → 503 path | Added `Health_returns_503_when_redis_down` test calling `NewClientWith(true, false, true)` and asserting 503 with body containing `"status":"unhealthy"`, `"db":"connected"`, `"redis":"disconnected"` | ✓ tests pass |
| 4 | Low | Bare `catch` in `TcpRedisHealthProbe.IsConnectedAsync` swallowed `OperationCanceledException` | Replaced bare `catch` with `catch (OperationCanceledException) { throw; } catch { return false; }` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| `dotnet test` (all 578 tests) | PASS |
| Coverage (Go) | 86.7% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 7fe208f | fix(hygiene): update M5-015 task status to review | #1 |
| 50e8429 | fix(health): propagate OperationCanceledException in TcpRedisHealthProbe | #4 |
| 4201879 | test(health): add body assertions and redis-down 503 endpoint test | #2, #3 |

## Summary

4/4 findings resolved. 0 deferred.
