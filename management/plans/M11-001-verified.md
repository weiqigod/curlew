# Verification Report: M11-001

**Task:** JUnit XML output ungating (remove Professional-tier feature gate)
**Verified by:** AI
**Date:** 2026-04-25
**Branch:** feature/M11-001-junit-ungating
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass, 0 failures |
| `go test -race ./...` | PASS | No races detected (via ci-local.sh) |
| `golangci-lint run` | PASS | No findings (via ci-local.sh) |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage | 81.6% | Meets >= 80% threshold (`cmd/curlew`) |

## Observable Output

```
go build ./cmd/curlew                              → Build OK
go test -run 'TestRun_JUnitFormat_FreeTier' -v ./cmd/curlew/...
  === RUN   TestRun_JUnitFormat_FreeTier
  --- PASS: TestRun_JUnitFormat_FreeTier (0.00s)
  PASS
grep -c '"junit_xml"' internal/auth/registry.go   → 0
go test ./...                                       → all packages PASS
```

Expected: exit 0, JUnit XML produced on free tier, 0 gate references in registry
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `internal/auth/registry.go` no longer registers `junit_xml` | `grep -c '"junit_xml"' internal/auth/registry.go` = 0 | PASS |
| 2 | `cmd/curlew/main.go` no longer calls `auth.CheckFeature(reg, "junit_xml", ...)` | No matches in grep | PASS |
| 3 | `TestRun_JUnitFormat_FreeTier` drives exit 0 + valid JUnit XML on free tier | `TestRun_JUnitFormat_FreeTier` | PASS |
| 4 | Existing JUnit gating tests removed or rewritten | All 4 old gate tests absent; events test migrated to `--parallel 2` | PASS |
| 5 | `docs/SPECIFICATION.md` and `docs/MANUAL.md` drop tier-gating language | No "Professional-tier gated" for JUnit in either doc | PASS |
| 6 | `CHANGELOG.md [Unreleased]` gains a Changed entry | Line 10 references M11-001 | PASS |
| 7 | `IMPROVEMENT.md §2.4 bullet 3` annotated with "Shipped (M11-001)" | Line 59 has annotation; line 34 table row updated | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` all packages pass | PASS |
| 2 | JUnit gate registration removed | `grep -c '"junit_xml"' internal/auth/registry.go` = 0 | PASS |
| 3 | `TestRun_JUnitFormat_FreeTier` passes | Verified above, explicit PASS | PASS |
| 4 | `go test ./...` passes | All packages pass, 0 failures | PASS |
| 5 | `go test -cover ./cmd/curlew/...` >= 80% | 81.6% | PASS |
| 6 | `golangci-lint run` passes with 0 issues | Clean (via ci-local.sh) | PASS |
| 7 | `./smoke/run.sh` passes | All smoke checks passed | PASS |
| 8 | `./scripts/ci-local.sh` passes | Exit 0 — "=== ci-local PASS ===" | PASS |
| 9 | `IMPROVEMENT.md §2.4 bullet 3` marked Shipped | "Shipped (M11-001)" annotation present | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS trusted (iteration 3), spot-check clean — `grep -c '"junit_xml"' internal/auth/registry.go` = 0; `TestRun_JUnitFormat_FreeTier` has explicit `CURLEW_TIER=free` pin, real httptest.Server, and XML unmarshaling assertion; no new exported symbols requiring doc comments.

## Commits

| Hash | Message |
|------|---------|
| a5bee86 | docs(review): add passing review for M11-001 |
| cd17dcd | docs(review): update improvement report for M11-001 (iteration 2) |
| 7862996 | fix(testdata): update stale junit_xml comment in feature-gate-denied env.txt |
| c3e02ab | docs(review): add review with findings for M11-001 (iteration 2) |
| 1060074 | docs(review): add improvement report for M11-001 |
| 84691c0 | fix(docs): update IMPROVEMENT.md JUnit table row to reflect ungating |
| 65a0d82 | docs(review): add review with findings for M11-001 |
| 2348c50 | chore(task): mark M11-001 as review |
| 485ba92 | docs: strip Professional-tier framing for JUnit XML output (M11-001) |
| 5a39b82 | feat(cli,auth): remove junit_xml feature gate (M11-001) |
| 70d5dc3 | test(cli): add failing TestRun_JUnitFormat_FreeTier |
| 346a7df | chore(task): mark M11-001 as in_progress |
| 666ddae | chore(task): mark M11-001 as planned |
| 5345e79 | docs(plan): add implementation plan for M11-001 |

## Files Changed

| File | Action |
|------|--------|
| `CHANGELOG.md` | modified — Added Changed entry |
| `IMPROVEMENT.md` | modified — §2.4 bullet 3 + format table row updated |
| `cmd/curlew/main.go` | modified — Removed two `CheckFeature("junit_xml", ...)` call sites |
| `cmd/curlew/main_test.go` | modified — Added `TestRun_JUnitFormat_FreeTier`; removed 4 old gate tests |
| `cmd/curlew/run_test.go` | modified — Removed tier elevation in JUnit tests |
| `cmd/curlew/stream_discipline_matrix_test.go` | modified — JUnit gate cell migrated |
| `docs/MANUAL.md` | modified — JUnit tier-gating language removed |
| `docs/SPECIFICATION.md` | modified — JUnit tier-gating language removed |
| `internal/auth/gate_test.go` | modified — Removed `TestCheckFeature_junitXML` |
| `internal/auth/registry.go` | modified — Removed `junit_xml` FeatureDefinition block |
| `testdata/agent-harness/feature-gate-denied/args.txt` | modified — Migrated to `--parallel 2` |
| `testdata/agent-harness/feature-gate-denied/env.txt` | modified — Comment updated |
| `testdata/agent-harness/feature-gate-denied/expect.yaml` | modified — Updated expectations |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
