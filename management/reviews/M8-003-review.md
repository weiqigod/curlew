# Code Review: M8-003

**Task:** `output:` block in project + collection YAML with CLI>collection>project precedence
**Reviewer:** AI
**Date:** 2026-04-24
**Branch:** feature/M8-003-output-block
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All five issues from the iteration 1 review were resolved by the `/improve` phase.

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|

_(No findings.)_

## Pre-audit Gate

`./scripts/ci-local.sh --go` passed in full: go build, go test, go test -race, coverage, golangci-lint, smoke tests all green.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; sentinel `ErrUnknownFormat` used correctly; formerly dead sentinels (`ErrEmptyReportPath`, `ErrEmptyEventsPath`) were removed. No swallowed errors; no panics for expected failures. |
| Input Validation | PASS | `Config.Validate()` validates `format`; empty-path enforcement delegated to JSON Schema `minLength: 1`. Collection `output.format` double-validated via `resolveOutputPrecedence` defensive check before any HTTP request runs. |
| Naming | PASS | No stuttering; `IsSupportedFormat` and `FormatList` correctly exported for reuse; `resolveOutputPrecedence` correctly unexported; doc comments on all exported symbols. |
| Code Organization | PASS | `output.Config` alongside `Verbosity`; no import cycles; `schemas/` owns embeds; `internal/schema` aliases; all `internal/` boundaries respected. |
| Correctness | PASS | Two-phase events-emitter opening correctly separates CLI-provided vs YAML-declared paths. Feature-gate checks for `junit`/`html` are two-phase: early for CLI-set format, late (after precedence resolution) for YAML-origin format. Precedence resolution is a single point in `runCmdInner` before any HTTP request. The `bad-format.yaml` observable produces exit 3 via the defensive format check in `resolveOutputPrecedence`. |
| Test Quality | PASS | All DoD tests present and passing: `TestOutputPrecedence` (4 sub-tests), `TestOutputPrecedence_Events` (3 sub-tests), `TestOutputPrecedence_Verbosity`, `TestSchema_accepts_output` (11 sub-tests), `TestSchema_rejects_unknown_output_format`, `TestSchema_rejects_empty_output_path`, `TestSchema_project_published_path_matches_embed`, `TestSchema_project_file_exists_at_published_path`, `TestSchema_output_defs_match`, `TestSchema_scaffolded_apitest_yaml_validates`, `TestProjectSchema`, `TestCollection_Output_Roundtrip`, `TestParseProjectConfig_Output`, `TestInitCmd_project_name_flag`, `TestSchemaCmd` (with `project_flag_emits_project_schema` and `no_flag_emits_collection_schema` rows). |

## Prior Findings Resolution (Iteration 1 → Iteration 2)

| # | Severity | Finding | Resolution |
|---|----------|---------|-----------|
| 1 | Critical | Events-emitter opened before `resolveOutputPrecedence`; YAML-declared `output.events` paths silently ignored | Fixed: two-phase open — CLI `--events` opened early (preserving run.start/run.end on pre-parse errors); YAML-declared path opened late after precedence resolution (`eventsEmitter == nil && flags.events != ""`). Verified by `TestOutputPrecedence_Events`. |
| 2 | High | Feature-gate checks for `junit`/`html` bypassed for YAML-origin format | Fixed: two-phase gate — `flags.formatSet` check at early gate block; `!flags.formatSet` check at late gate block after `resolveOutputPrecedence`. |
| 3 | High | `ErrEmptyReportPath`/`ErrEmptyEventsPath` declared, hint-registered, but never returned (dead code) | Fixed: both sentinels removed from `internal/output/config.go` and `hints_init.go`. Path emptiness enforced by JSON Schema `minLength: 1`. Coverage sentinel test passes. |
| 4 | Medium | `TestOutputPrecedence` lacked coverage for `events` and `verbosity` YAML-origin fields | Fixed: `TestOutputPrecedence_Events` (3 sub-tests) and `TestOutputPrecedence_Verbosity` added. |
| 5 | Low | Misleading comment claiming `flags.events` "consumed below" when open block was above | Fixed: comment removed; both early and late open blocks have accurate phase-separation descriptions. |

## Test Coverage

- `cmd/apitest`: 81.2% (above 80% gate)
- `internal/config`: 96.0%
- `internal/parser`: 89.8%
- `internal/output`: 93.9%
- `internal/schema`: no statements (test-only package, compile-time schema tests)

All packages meet the >= 80% threshold.

## Behavior Coverage (from task YAML)

| Behavior | Test(s) |
|----------|---------|
| `schemas/project-v1.json` exists with correct `$id`, `title`, `additionalProperties: false` | `TestSchema_project_file_exists_at_published_path`, `TestProjectSchema` |
| Both schemas accept optional `output:` with `format`, `report`, `events`, `verbosity` | `TestSchema_accepts_output` (11 sub-tests) |
| `output.format` enum is `[terminal, json, tap, junit, html]`; markdown excluded | `TestSchema_rejects_unknown_output_format` |
| `output.verbosity` enum is `[quiet, normal, verbose, debug]` | `TestSchema_accepts_output` (verbosity sub-tests) |
| `Collection.Output` and `ProjectConfig.Output` are `*output.Config` fields that round-trip | `TestCollection_Output_Roundtrip`, `TestParseProjectConfig_Output` |
| Runtime precedence: CLI > collection > project > built-in | `TestOutputPrecedence` (4 sub-tests) |
| `runFlags` has `formatSet/reportSet/eventsSet/verbositySet` parallel bools | Exercised throughout `TestOutputPrecedence*` |
| Unknown `output.format` or empty path fails with exit 3 before any HTTP request | Defensive check in `resolveOutputPrecedence`; `TestSchema_rejects_unknown_output_format`, `TestSchema_rejects_empty_output_path` |
| `schema.ProjectSchema` is a byte-slice alias mirroring `schema.CollectionSchema` | `TestSchema_project_published_path_matches_embed` |
| `apitest schema --project` emits project schema; no flag emits collection schema | `TestSchemaCmd` (`project_flag_emits_project_schema`, `no_flag_emits_collection_schema`) |
| `scaffold.Init` emits active `output:` block and validates against project schema | `TestInit` ("apitest.yaml contains active output block"), `TestSchema_scaffolded_apitest_yaml_validates` |
| `docs/MANUAL.md` §1.5 documents second `yaml.schemas` entry and §3.6.1 documents output block | Verified in MANUAL.md (§1.5 and §3.6.1 present) |
| `CHANGELOG.md` updated with M8-003 bullet | Verified in CHANGELOG.md |

## Summary

The implementation is complete and correct after the improvement pass. All five findings from iteration 1 — including the critical events-emitter ordering bug, the feature-gate bypass for YAML-declared junit/html format, and the dead-code sentinels — have been properly resolved. The two-phase approach for both events-emitter opening and feature-gate checking is clean and well-commented. All 21 DoD items are verified, all behavior tests pass, and coverage is above the 80% gate in every package. The `$defs.output` subschema is byte-identical across both schema files (enforced by `TestSchema_output_defs_match`), MANUAL.md §1.5 and §3.6.1 are in place, and the CHANGELOG carries the M8-003 bullet.
