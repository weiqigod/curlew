# Code Review: M19-002

**Task:** internal/cel/ foundation: Evaluator + standard activation
**Reviewer:** AI
**Date:** 2026-05-16
**Branch:** feature/M19-002-cel-foundation

## Verdict: PASS

## Findings

No findings. All four findings from the first review iteration were resolved by the improve pass:

| # | Previous Finding | Resolution |
|---|-----------------|------------|
| 1 | Medium: No CHANGELOG.md entry | CHANGELOG entry added under `[Unreleased] → Added` with full package description |
| 2 | Low: `TestEvaluator_RejectsZeroArgTimestamp` missing "timestamp" assertion | `errors.As` + `strings.Contains(ce.Error(), "timestamp")` added |
| 3 | Low: No test for `response.headers` CEL field access | `response.headers access` sub-case added to `TestEvaluator_StandardActivationBindsResponsePreviousVarsEnv` |
| 4 | Low: Redundant `CelError.Is()` method | Confirmed non-existent in implementation; finding was not applicable |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped via `newParseError`/`newTypeError` constructors or `fmt.Errorf("context: %w", err)`. Sentinels exposed correctly via `Unwrap`. No swallowed errors. No panic for expected failures. Runtime CEL errors surfaced as `ErrCelParse` (documented trade-off deferred to M19-004). |
| Input Validation | PASS | Nil `Response`/`Previous` handled in `responseToMap` (returns empty map). Nil `Vars` and `Env` normalised to empty maps in `activationMap`. Nil `SensitiveObserver` guarded before invocation. Nil map read at `activation.Vars[name]` is safe in Go (returns zero value). |
| Naming | PASS | No stuttering. Doc comments on all exported types, functions, methods, and constants. Package name `cel` is lowercase single-word. Interface names `Evaluator` and `Program` are clean. |
| Code Organization | PASS | `internal/` boundary respected. Single responsibility per file: `cel.go` (core types + env + compile/eval), `errors.go` (error construction + truncation), `forbid.go` (AST walk for time-of-day), `observe.go` (vars ref collection), `hints_init.go` (sentinel registration), `doc.go` (package-level godoc). No circular dependencies. |
| Correctness | PASS | AST walk runs between Parse and Check (correct order). `collectVarsRefs` deduplicates via map before sorting. Observer fires exactly once per sensitive name per Eval call. `truncateSource` operates on runes not bytes. `out.Value()` unwraps cel-go ref.Val to native Go values. CEL `prog.Eval` error is wrapped as `ErrCelParse` and returned before observer fires (consistent). |
| Test Quality | PASS | All 8 behaviors from task YAML covered by named tests. Table-driven tests used throughout. Subtests use `t.Run()` with descriptive names. `errors.As` assertions check structured fields (not just sentinel). Observer tested for: fires once, not called for non-sensitive, deduplication, nil-observer no-panic. Edge cases: nil response/previous, empty expression. |

## Test Coverage
- **Coverage: 92.6%** (exceeds 80% threshold)
- Uncovered paths (acceptable): `NewEvaluator` error branch (cel-go env construction — only reachable with invalid internal configuration), `env.Program(checked)` error branch after successful `Check` (not reachable in practice), one visitor short-circuit branch in `rejectTimeOfDay` (`firstErr != nil` early return — `PostOrderVisit` does not short-circuit the visitor).

## Definition of Done Verification

| DoD Item | Status |
|----------|--------|
| All behavior tests pass | PASS — all 8 behaviors in task YAML have passing named tests |
| Test coverage >= 80% for `internal/cel/` | PASS — 92.6% |
| No build warnings or lint errors | PASS — `golangci-lint run ./internal/cel/...` reports 0 issues |
| `./scripts/ci-local.sh --go` passes | PASS — gate passed cleanly including race detector |
| `go.mod` and `go.sum` updated with `cel-go` pinned to a tagged release | PASS — `github.com/google/cel-go v0.28.1` |
| Public package documentation (doc.go) | PASS — explains standard activation, disabled time-of-day functions, and sensitive-observer contract |
| `ErrCelParse` and `ErrCelType` exported and reachable via `errors.Is` | PASS — both sentinels registered in `internal/errors`, reachable via `Unwrap` chain |

## Summary

The implementation is complete and sound. The CEL foundation package correctly wraps `cel-go` with the project's deterministic-only constraint (time-of-day rejection via AST walk), a fully typed `StandardActivation`, structured error sentinels with 200-rune source truncation, and a sensitive-observer hook that fires exactly once per referenced sensitive variable per eval call. All four findings from iteration 1 were resolved. Coverage is 92.6%. The pre-audit gate passes cleanly.
