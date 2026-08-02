# Improvement Report: M8-003

**Task:** `output:` block in project + collection YAML with CLI>collection>project precedence
**Date:** 2026-04-24
**Review:** management/reviews/M8-003-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | Events-emitter opened at line 569 before `resolveOutputPrecedence`; YAML-declared `output.events` paths silently ignored | Implemented two-phase approach: CLI-provided `--events` (flags.eventsSet) opened early (preserving run.start/run.end on parse errors); YAML-declared events path opened in late-open block after precedence resolution (only fires when eventsEmitter==nil) | ✓ tests pass; `TestOutputPrecedence_Events` sub-tests verify all three cases |
| 2 | High | Feature-gate checks for `--format junit`/`html` read pre-resolution format; YAML-declared format bypasses tier gates | Two-phase approach: CLI-set format (flags.formatSet) gate-checked early (before parse), YAML-resolved format (!flags.formatSet) gate-checked after precedence resolution | ✓ tests pass; existing `TestRunCmd_format_junit_feature_gate_before_parse` continues to pass |
| 3 | High | `ErrEmptyReportPath` and `ErrEmptyEventsPath` declared, hint-registered, but never returned by any code path (dead code) | Removed both sentinel vars from `internal/output/config.go` and their registrations from `hints_init.go`; path emptiness enforced by JSON Schema `minLength: 1` gate | ✓ `TestCoverage_EverySentinelIsRegistered` passes (no unregistered sentinels) |
| 4 | Medium | `TestOutputPrecedence` only covered format field; no tests for events or verbosity YAML origin | Added `TestOutputPrecedence_Events` (3 sub-tests: project_sets_events, collection_overrides_project_events, cli_events_wins_over_yaml) and `TestOutputPrecedence_Verbosity` | ✓ all 4 new sub-tests pass |
| 5 | Low | Misleading comment at line 957-958 claiming flags.events "consumed below" when open block was above | Removed inaccurate comment; replaced with accurate phase-separation descriptions in both early and late open blocks | ✓ code review verified |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage `cmd/curlew` | 81.2% |
| Coverage `internal/config` | 96.0% |
| Coverage `internal/parser` | 89.8% |
| Coverage `internal/output` | 93.9% |

All packages >= 80% threshold.

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| ce8f92e | fix(cmd,output): resolve all M8-003 review findings | #1, #2, #3, #4, #5 |

## Summary
5/5 findings resolved. 0 deferred.
