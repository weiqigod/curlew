# Code Review: M8-005

**Task:** run --only: prune setup to transitive closure of {{variable}} references
**Reviewer:** AI
**Date:** 2026-04-24
**Branch:** feature/M8-005-only-prune-setup

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `fmt.Errorf("context: %w", err)`. `fmt.Fprintf` return values discarded intentionally (`_, _ =`) for diagnostic-writer path — acceptable pattern for optional diagnostic output. No swallowed errors on code paths. |
| Input Validation | PASS | `AncestorClosure` guards nil graph, empty nodes, empty starts, and out-of-range indices. Runner pruning block guards `len(col.Setup.Items) == 0` before calling Analyze. |
| Naming | PASS | `AncestorClosure` follows Go exported-symbol conventions; doc comment present. `buildPreExecVarSet`, `fullSetupForCliff`, `Diagnostics` all named clearly. No stuttering. `enrichSelectionCliff` signature extended cleanly. |
| Code Organization | PASS | `AncestorClosure` co-located with `hasTransitiveDep` in `waves.go` as planned — single file for graph traversal primitives. `buildPreExecVarSet` is an unexported helper in runner.go above the function that originally inlined it. `fullSetupForCliff` and `Diagnostics` added to `VarSources` with doc comments. Package boundaries respected throughout. |
| Correctness | PASS | Pruning logic is correct: `prunedSetup` initialized to full setup before the `IsValid` check, so the fallback path requires no extra assignment. Shallow-copy re-threads `Retry` pointer from the original section (mirrors M8-004 pattern). `emptySummary.Total` uses `len(col.Teardown.Items)` from the shallow-copied `col` — teardown is shared and unchanged. Four call sites of `enrichSelectionCliff` all pass `vars.fullSetupForCliff`. |
| Test Quality | PASS | All DoD tests present and passing. Table-driven `TestParallel_AncestorClosure` covers linear-chain, diamond, disconnected, empty-starts, out-of-range, duplicate, and nil cases. Runner integration tests cover happy path (two sub-tests), chained producers, no-extract seeder, analyzer fallback, regression without --only, cliff safety-net path, and cmd-layer stderr diagnostic. |

## Test Coverage
- Coverage: `internal/parallel` 90.1% — above 80% threshold
- Coverage: `internal/runner` 84.1% — above 80% threshold
- Missing coverage: None materially — the safety-net cliff path (`TestRunner_OnlyCliff_SetupProducer`) is tested as a unit directly on `enrichSelectionCliff` since it is unreachable in normal operation.

## Behavior Coverage

| Behavior | Test(s) | Status |
|----------|---------|--------|
| Setup pruned to transitive closure on --only | `TestRunner_OnlyPrunesSetup` (2 sub-tests) | ✓ |
| Analyzer seeded with preExecVars from scope.Resolved() | `TestRunner_BuildPreExecVarSet` + integration tests | ✓ |
| No-extract setup items always included | `TestRunner_OnlyPrunesSetup_NoExtractSeeder` | ✓ |
| Single Analyze call + AncestorClosure primitive | `TestParallel_AncestorClosure` (12 cases) | ✓ |
| IsValid==false fallback to full setup + stderr diagnostic | `TestRunner_OnlyPrunesSetup_AnalyzerFallback` + `TestRun_OnlyAnalyzerFallback_StderrDiagnostic` | ✓ |
| Teardown unaffected | `TestRunner_OnlyFilter` (existing) | ✓ |
| No pruning without --only (regression) | `TestRunner_OnlyPrunesSetup_NoPruningWithoutOnly` | ✓ |
| enrichSelectionCliff scans fullSetupItems | `TestRunner_OnlyCliff_SetupProducer` | ✓ |
| Data-driven parent item contributes vars | Deferred per task scope | N/A |
| watch propagates transparently | `TestWatch_OnlyPropagates` (existing) | ✓ |
| AncestorClosure exported primitive | `TestParallel_AncestorClosure` | ✓ |
| No events schema change | No new test needed (additive) | ✓ |
| CHANGELOG.md updated | Verified in diff | ✓ |
| IMPROVEMENT.md W3 + §8 Q3 updated | Verified in diff | ✓ |

## Summary

The implementation is correct, complete, and well-tested. The three-component design — `parallel.AncestorClosure` primitive, runner pruning block, cliff diagnostic extension — follows the plan faithfully. The analyzer-invalid fallback correctly preserves the full setup in `prunedSetup` via initialization-before-branch, avoiding a missed-assignment bug. All 14 DoD items are met, including the explicitly deferred data-driven case. Coverage for both modified packages exceeds 80%. The pre-audit gate (`ci-local.sh --go`) passed cleanly including smoke tests.
