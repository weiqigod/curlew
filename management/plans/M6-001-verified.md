# Verification Report: M6-001

**Task:** Error taxonomy extension and sentinel hint registry
**Verified by:** AI
**Date:** 2026-04-21
**Branch:** feature/M6-001-error-taxonomy
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | Every internal package passes, no regressions |
| `go test ./internal/errors/...` | PASS | Classifier and coverage tests green |
| `go test -cover ./internal/errors/...` | PASS | 93.2% coverage, above the 80% gate |
| `golangci-lint run ./...` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Smoke test clean |
| `go build ./cmd/apitest` | PASS | Binary builds |

## Observable Output

```
$ go test ./internal/errors/...
ok  	github.com/peterlindqvist/apitest/internal/errors	0.409s	coverage: 93.2% of statements

$ go test ./... 2>&1 | tail
ok  	github.com/peterlindqvist/apitest/internal/websocket/templates	(cached)
ok  	github.com/peterlindqvist/apitest/internal/worker	(cached)

$ golangci-lint run ./...
0 issues.
```

End-to-end sanity check — a wrapped `parser.ErrInvalidYAML` classifies correctly:

```go
s := apierrors.ClassifyError(fmt.Errorf("outer: %w", parser.ErrInvalidYAML))
// s = &Structured{
//   Category: "parse",
//   Code:     "PARSE_INVALID_YAML",
//   Hint:     "Fix the YAML syntax at the line indicated. Check indentation and quoting.",
//   Message:  "outer: invalid YAML syntax",
//   Inner:    <wrapped error chain>,
// }
```

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Given a registered sentinel (parser.ErrInvalidYAML), when ClassifyError is called with a wrapped form, then it returns Category=parse, Code=PARSE_INVALID_YAML, non-empty Hint | `TestClassifyError_WrappedSentinel` + coverage_test (runtime lookup) | PASS |
| 2 | Given a network error identified by ClassifyNetworkError, when ClassifyError is called, then it returns Category=network with a stable Code | `TestClassifyError_NetworkErrorClassified`, `TestClassifyError_WrappedNetworkError`, `TestClassifyError_ClassifyNetworkErrorCompatibility`, `TestClassifyError_DeadlineExceededRoundTrip` | PASS |
| 3 | Given an error chain that already contains a *Structured with FilePath/Line, when ClassifyError is called, then location is preserved; missing Code/Hint enriched from registry | `TestClassifyError_ExistingStructuredPreserved`, `TestClassifyError_ExistingStructuredEnrichedFromRegistry` | PASS |
| 4 | Given an unregistered error, when ClassifyError is called, then it returns Category=internal with empty Code and Hint | `TestClassifyError_UnregisteredFallsBackToInternal` | PASS |
| 5 | Given coverage_test runs, when it walks internal/ source, then every Err* var in a non-test .go file is registered | `TestCoverage_EverySentinelIsRegistered` | PASS |
| 6 | Given any registered sentinel, when the registry is introspected, then it has non-empty Category and Code; non-internal entries have non-empty Hint | `TestCoverage_EveryRegisteredHasCategoryAndCode`, `TestCoverage_ClassifiedEntriesHaveHints` | PASS |
| 7 | Given all registered sentinels, when their Codes are enumerated, then every Code is unique | `TestCoverage_CodesAreUnique` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | 16 tests under `internal/errors/classify_test.go` + 4 coverage tests all green | PASS |
| 2 | `go test ./internal/errors/...` passes | Output above | PASS |
| 3 | `go test ./...` passes (no regressions) | Full suite green | PASS |
| 4 | `golangci-lint run` passes with 0 issues | Output above | PASS |
| 5 | Test coverage >= 80% for internal/errors | 93.2% | PASS |
| 6 | Every exported Err* sentinel in internal/ is registered | `TestCoverage_EverySentinelIsRegistered` passes; 29 packages, ~100 sentinels covered | PASS |
| 7 | Every registered sentinel has Category + Code; classified entries have concrete-action Hint | `TestCoverage_EveryRegisteredHasCategoryAndCode` + `TestCoverage_ClassifiedEntriesHaveHints` both green | PASS |
| 8 | All Codes unique | `TestCoverage_CodesAreUnique` green | PASS |

## Notes

- No behavioural change in `cmd/apitest` as designed. This is plumbing consumed by M6-004 (events package) and, for free via the existing `apierrors.Format()` call in `terminal.go:12`, by the terminal formatter when errors eventually carry the new Code/Hint enrichment.
- The coverage test uses `go/parser` to walk the source tree rather than reflection, because Go does not expose package-level `var` declarations via reflection. Blank imports in `coverage_test.go` force each owning package's `init()` to populate the registry before the source walk cross-references.
- `internal/errors` itself has no `Err*` sentinels and is skipped by the source walk.
- Distributed registration (per-package `hints_init.go`) is the only workable pattern given that `internal/errors` is already imported by sentinel-owning packages (e.g. `parser`). The reverse import would cycle.

## Ready for next slice

M6-002 (source-location plumbing) is the next dependency for the events stream and is now unblocked.
