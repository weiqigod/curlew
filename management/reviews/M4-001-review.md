# Code Review: M4-001

**Task:** Shared vault configuration template format
**Reviewer:** AI
**Date:** 2026-04-14
**Branch:** feature/M4-001-shared-vault-template
**Iteration:** 2 (post-improvement)

## Verdict: PASS

## Findings

No findings. All 5 issues from the iteration-1 review have been resolved:

| # | Original Severity | Finding | Resolution |
|---|----------|---------|------------|
| 1 | Medium | `parse.go:22` — `%v` for inner error breaking chain | Fixed: now `%w: %w` |
| 2 | Medium | `parse.go:65` — `%v` for inner error breaking chain | Fixed: now `%w: %w` |
| 3 | Low | `ParseReader` exported with no callers or tests | Fixed: removed entirely |
| 4 | Low | `validate.go` non-deterministic map iteration over key aliases | Fixed: aliases sorted before iteration |
| 5 | Low | `validateCmd` doc comment omitted exit code 2 | Fixed: comment updated at line 1517 |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All `fmt.Errorf` chains use `%w`. Sentinel errors (`ErrInvalidTemplate`, `ErrUnknownProvider`, `ErrMissingField`, `ErrDuplicateAlias`, `ErrInvalidKeyRef`) defined and used correctly. No swallowed errors. |
| Input Validation | PASS | Empty document returns empty template (not panic). Malformed YAML returns wrapped `ErrInvalidTemplate`. Unknown providers, missing fields, empty keys blocks all produce deterministic error messages with key paths. `sniffTeamTemplate` falls through on read/parse failure. |
| Naming | PASS | No stuttering. All exported types and functions have doc comments. Package name is lowercase single-word. `ResultKind` constants (`KindCollection`, `KindTeamTemplate`) are clean. |
| Code Organization | PASS | `internal/` boundaries respected. No circular dependencies. `internal/vault/teamtemplate` does not import `internal/runner`, `internal/variable`, or `internal/config`. Exported surface is minimal (`Parse`, `Validate`, `Resolve`, `Summary`, types, sentinels). |
| Correctness | PASS | Duplicate alias detection via `yaml.Node` walk (not Go map decode) — correctly detects YAML-level duplicates. Deterministic output via sorted alias iteration. `context.Context` not needed (no I/O, no network). No goroutines. Race detector passes. |
| Test Quality | PASS | All 8 task behaviors covered by table-driven tests with descriptive `t.Run` names. `TestTeamTemplate_Resolve` and `TestTeamTemplate_Summary` cover structural behaviours. Integration tests in `cmd/apitest/validate_team_test.go` exercise real dispatch via `captureRun`. `testdata/` fixtures checked in. |

## Test Coverage
- `internal/vault/teamtemplate`: **87.8%** (exceeds >= 80% gate)
  - `parseBytes`: 72.4% — uncovered branches are defensive error paths (non-mapping root, `team_secrets` not a mapping, malformed environment decode) that require crafted YAML not exercised by the 8 behavior tests; acceptable gap
  - All other functions: 91.7%–100%
- `internal/validator`: **91.8%**
  - `sniffTeamTemplate`: 71.4% — uncovered: `os.ReadFile` error path (requires filesystem failure, not practical without mocking)
  - `validateTeamTemplate`: 68.4% — uncovered: `os.ReadFile` error path; `teamtemplate.Parse` error path (structurally malformed YAML after sniff passes, extremely rare)
  - Both gaps are in defensive/error paths, not in any behavior exercised by the task specification

## Observable Verification

```
$ ./apitest validate testdata/team/shared-vault-template.yaml
OK: shared vault template valid (2 environments, 4 secrets)   ← exit 0 ✓

$ ./apitest validate testdata/team/shared-vault-template.invalid.yaml; echo "Exit: $?"
FAIL testdata/team/shared-vault-template.invalid.yaml is invalid
  [ERROR]   line 3: team_secrets.vault_configs.production.provider: unknown provider 'foo'
Exit: 2   ← exit 2 ✓

$ go test ./internal/vault/teamtemplate/... -run TestTeamTemplate -count=1
ok  (10 subtests pass including all 8 from the task DoD)   ✓
```

All observable targets from the task YAML pass exactly as specified.

## Summary

The iteration-2 code is clean, correct, and complete. All 5 findings from the previous review have been resolved correctly — error chains now preserve full unwrappability via `%w`, the non-deterministic iteration was fixed with `sort.Strings`, the dead exported function was removed, and the doc comment was updated. All 8 task behaviors are covered by tests, the build is lint-clean, and all tests pass under the race detector with 87.8% coverage in the new package. No new issues were found.
