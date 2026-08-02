# Code Review: M11-001

**Task:** JUnit XML output ungating (remove Professional-tier feature gate)
**Reviewer:** AI
**Date:** 2026-04-25
**Branch:** feature/M11-001-junit-ungating
**Iteration:** 3

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new error-returning code introduced; all changes are deletions of gate blocks. No swallowed errors; no `%v` instead of `%w`; no new panic paths. |
| Input Validation | PASS | No changes to input validation logic. |
| Naming | PASS | No new exported symbols. `TestRun_JUnitFormat_FreeTier` follows naming conventions. |
| Code Organization | PASS | Package boundaries respected; auth registry and main.go call sites cleanly deleted. Retained `writeJUnitError` correctly since it is still used for non-gate error paths. |
| Correctness | PASS | Registry entry deleted (`grep -c '"junit_xml"' internal/auth/registry.go` = 0); both call sites removed (CLI-set path at ~line 645 and YAML-resolved path at ~line 1018); comment at line 1018 notes the historical ungating; `writeJUnitError` and formatter helpers correctly retained; `TestRunCmd_Events_GateError_EmitsRunError` migrated to `--parallel 2`; agent-harness `feature-gate-denied` fixture correctly migrated. |
| Test Quality | PASS | `TestRun_JUnitFormat_FreeTier` added with explicit `APITEST_TIER=free` pin, real httptest.Server, XML unmarshaling assertion, and 1-suite×1-testcase structural check; all four gating tests removed; events test migrated to `--parallel 2` with identical assertions; tier elevations cleaned from eight existing JUnit tests; `run_junit_happy` stream discipline cell cleaned of redundant `APITEST_TIER=professional` env; `env.txt` comment updated (iteration-2 fix confirmed). |

## Behaviors Verified

| Behavior | Status | Evidence |
|----------|--------|----------|
| `internal/auth/registry.go` no longer registers `junit_xml` | PASS | `grep -c '"junit_xml"' internal/auth/registry.go` returns 0 |
| `cmd/apitest/main.go` no longer calls `auth.CheckFeature(reg, "junit_xml", ...)` | PASS | Only a historical comment at line 1018 mentions `junit_xml`; both gate blocks deleted |
| `TestRun_JUnitFormat_FreeTier` drives exit 0 + valid JUnit XML on free tier | PASS | Test exists at `cmd/apitest/main_test.go` line 6283; passes with `APITEST_TIER=free` explicitly set |
| Existing JUnit gating tests removed or rewritten | PASS | `TestRunCmd_format_junit_recognized`, `_feature_gate_free_tier`, `_feature_gate_before_parse`, `_feature_gate_professional_tier`, `TestCheckFeature_junitXML` all absent |
| `docs/SPECIFICATION.md` and `docs/MANUAL.md` drop tier-gating language | PASS | No "Professional-tier gated" or similar remains for JUnit in either doc; MANUAL.md format table row for `junit` reads "TAP-compatible JUnit XML" (no tier restriction) |
| `CHANGELOG.md [Unreleased]` gains a Changed entry | PASS | Line 10 of CHANGELOG.md contains the full entry referencing M11-001 |
| `IMPROVEMENT.md §2.4 bullet 3` annotated with "Shipped (M11-001)" | PASS | Line 59 has the annotation; line 34 format table notes "Free tier (ungated in M11-001)" |
| `testdata/agent-harness/feature-gate-denied/env.txt` comment updated | PASS | Comment now reads "Force the free tier so the parallel_execution gate fires (--parallel 2). Migrated from --format junit in M11-001 when JUnit was ungated." |

## Test Coverage

- Coverage: 81.6% (via `go test -cover ./cmd/apitest/...`)
- Threshold: 80% required
- Missing coverage: none identified; exceeds the threshold.

## Pre-audit Gate

`./scripts/ci-local.sh --go` passed all steps: go build, go test, go test -race, coverage (81.6%), golangci-lint, and smoke test.

## Summary

All findings from iterations 1 and 2 are resolved. The implementation is complete across all seven behaviors: registry deletion, call-site removal, new free-tier test, stale gate-test cleanup, documentation cleanup, CHANGELOG entry, and IMPROVEMENT.md annotation. The iteration-2 finding (stale `env.txt` comment referencing `junit_xml`) has been correctly fixed. No new issues were found in this audit. Code, tests, and documentation are consistent and correct.
