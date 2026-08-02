# Verification Report: M8-003

**Task:** `output:` block in project + collection YAML with CLI>collection>project precedence
**Verified by:** AI
**Date:** 2026-04-24
**Branch:** feature/M8-003-output-block
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass, no failures |
| `go test -race ./...` | PASS | No races detected (run via ci-local.sh) |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage `cmd/apitest` | 81.2% | Meets >= 80% threshold |
| Coverage `internal/config` | 96.0% | Meets >= 80% threshold |
| Coverage `internal/parser` | 89.8% | Meets >= 80% threshold |
| Coverage `internal/output` | 93.9% | Meets >= 80% threshold |
| Coverage `internal/schema` | [no statements] | Test-only package |
| `./scripts/ci-local.sh --go` | PASS | All gates green |

## Observable Output

```
$ go test -run TestSchema_accepts_output ./internal/schema/...
ok  github.com/peterlindqvist/apitest/internal/schema (cached)

$ ls -la schemas/project-v1.json
-rw-r--r--@ 1 peterlindqvist  staff  1230 Apr 24 12:30 schemas/project-v1.json

$ ./apitest schema --project | jq -r .title
ApiTest Project v1

$ ./apitest schema --project | jq -r '.["$id"]'
https://raw.githubusercontent.com/peterlindqvist/apitest/main/schemas/project-v1.json

$ ./apitest schema | jq -r .title
ApiTest Collection v1

$ ./apitest run examples/output-block/collection.yaml | jq '.' | head -5
{
  "name": "Output Block Demo",
  "status": "passed",
  "duration_ms": 1139,
  ...
}
(valid JSON output from project-level format: json default)

$ ./apitest run examples/output-block/bad-format.yaml; echo "Exit: $?"
[ERROR] unknown output format "markdown" (supported: terminal, json, tap, junit, html)
Exit: 3

$ cd "$(mktemp -d)" && /path/to/apitest init --project-name demo && grep -A3 '^output:' apitest.yaml
output:
  format: terminal
  verbosity: normal
```

Expected: all observables match task YAML specification.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `schemas/project-v1.json` exists with correct `$id`, `title`, `additionalProperties: false` | `TestSchema_project_file_exists_at_published_path`, `TestProjectSchema` | PASS |
| 2 | Both schemas accept optional `output:` with `format`, `report`, `events`, `verbosity` (`additionalProperties: false`) | `TestSchema_accepts_output` (11 sub-tests) | PASS |
| 3 | `output.format` enum is `[terminal, json, tap, junit, html]`; markdown excluded | `TestSchema_rejects_unknown_output_format` | PASS |
| 4 | `output.verbosity` enum is `[quiet, normal, verbose, debug]` | `TestSchema_accepts_output` (verbosity sub-tests), `TestParseVerbosity` | PASS |
| 5 | `Collection.Output` and `ProjectConfig.Output` are `*output.Config` fields that round-trip | `TestCollection_Output_Roundtrip`, `TestParseProjectConfig_Output` | PASS |
| 6 | Runtime precedence: CLI > collection > project > built-in | `TestOutputPrecedence` (4 sub-tests) | PASS |
| 7 | `runFlags` has `formatSet/reportSet/eventsSet/verbositySet` parallel bools | `TestOutputPrecedence*` (all sub-tests exercise this) | PASS |
| 8 | Unknown `output.format` fails with exit 3 before any HTTP request | `TestSchema_rejects_unknown_output_format`, observable `bad-format.yaml` | PASS |
| 9 | `schema.ProjectSchema` is a byte-slice alias mirroring `schema.CollectionSchema` | `TestSchema_project_published_path_matches_embed` | PASS |
| 10 | `apitest schema --project` emits project schema; no flag emits collection schema | `TestSchemaCmd` (2 rows) | PASS |
| 11 | `scaffold.Init` emits active `output:` block and validates against project schema | `TestInit`, `TestSchema_scaffolded_apitest_yaml_validates` | PASS |
| 12 | `docs/MANUAL.md` §1.5 documents second `yaml.schemas` + §3.6.1 documents output block | Verified in MANUAL.md | PASS |
| 13 | `CHANGELOG.md` updated with M8-003 bullet | Verified in CHANGELOG.md | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — all packages pass | PASS |
| 2 | `schemas/project-v1.json` exists with correct `$id` and title | `ls schemas/project-v1.json` + jq checks | PASS |
| 3 | `schemas/collection-v1.json` accepts `output:` block | `TestSchema_accepts_output` collection sub-tests | PASS |
| 4 | `$defs.output` subschema byte-identical across both schema files | `TestSchema_output_defs_match` PASS | PASS |
| 5 | `schema.ProjectSchema` public API added and exported | `internal/schema/schema.go` declares `var ProjectSchema = schemas.ProjectV1` | PASS |
| 6 | `TestSchema_accepts_output` passes (11 sub-tests) | All pass | PASS |
| 7 | `TestSchema_rejects_unknown_output_format` passes | PASS | PASS |
| 8 | `TestSchema_rejects_empty_output_path` passes | PASS | PASS |
| 9 | `TestSchema_project_published_path_matches_embed` passes | PASS | PASS |
| 10 | `TestSchema_project_file_exists_at_published_path` passes | PASS | PASS |
| 11 | `TestSchema_scaffolded_apitest_yaml_validates` passes | PASS | PASS |
| 12 | `TestOutputPrecedence` passes with 4 sub-tests | PASS: builtin, project-wins, collection-wins, cli-wins | PASS |
| 13 | `parser.Collection.Output` and `config.ProjectConfig.Output` round-trip | `TestCollection_Output_Roundtrip`, `TestParseProjectConfig_Output` | PASS |
| 14 | `scaffold.Init` emits active output block | Observable + `TestInit` | PASS |
| 15 | `apitest schema --project` / no-flag backward compat | Observable verified | PASS |
| 16 | `docs/MANUAL.md` §1.5 documents project schema mapping | Verified | PASS |
| 17 | `docs/MANUAL.md` has output: block section (fields + precedence) | §3.6.1 present | PASS |
| 18 | `CHANGELOG.md` updated with M8-003 entry | Verified | PASS |
| 19 | `go test ./...` passes with no regressions | All packages pass | PASS |
| 20 | Coverage >= 80% in all listed packages | min 81.2% (`cmd/apitest`) | PASS |
| 21 | `golangci-lint run` passes with 0 issues | ci-local.sh PASS | PASS |
| 22 | `./smoke/run.sh` passes | ci-local.sh PASS | PASS |
| 23 | `./scripts/ci-local.sh` passes | PASS | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — `%w` wrapping used; `ErrUnknownFormat` sentinel; no panics |
| Naming conventions | PASS — no stuttering; `IsSupportedFormat`/`FormatList` exported correctly; doc comments on all exports |
| Code organization | PASS — `output.Config` in `internal/output/`; no import cycles; `schemas/` owns embeds; `internal/schema` aliases |
| Test quality | PASS — table-driven tests throughout; `TestOutputPrecedence*` verifies all four fields |

Branch A: "Review PASS trusted (iteration 2 verdict: PASS), spot-check clean (error wrapping, doc comments, test quality all verified)."

## Commits

| Hash | Message |
|------|---------|
| f90fac1 | docs(review): add passing review for M8-003 (iteration 2) |
| e33edb8 | docs(review): add improvement report for M8-003 |
| ce8f92e | fix(cmd,output): resolve all M8-003 review findings |
| afe5fea | docs(review): add review with findings for M8-003 |
| 79a827c | chore(task): mark M8-003 as review |
| 8ab003c | refactor(parser): fix gofumpt struct field alignment in Collection |
| 6c6143 | docs(manual,changelog): document output: block, project schema, and precedence table |
| 671c6cc | chore(examples): add examples/output-block fixtures |
| 730c7ff | feat(scaffold,cmd): emit active output block and accept --project-name flag |
| ae59e1f | test(cmd): add failing init --project-name test |
| 0214235 | feat(cmd): apitest schema --project emits project schema |
| 47c5d7e | test(cmd): add failing schema --project flag tests |
| c211e5b | feat(cmd): resolve output precedence once before formatter construction |
| 3c76c2e | test(cmd): add failing TestOutputPrecedence four-case table |
| 002ca6a | feat(parser,config): thread Output field through YAML unmarshal |
| f028a3b | test(parser,config): add failing round-trip tests for Collection.Output and ProjectConfig.Output |
| 1190208 | feat(schemas): publish project-v1.json + extend collection-v1.json with output block |
| 33446c1 | test(schemas): add failing tests for project schema drift, output defs match, accept/reject output |
| dc2d57b | feat(output): introduce output.Config with format/verbosity parsing |
| 2ee1731 | test(output): add failing tests for Config unmarshal, Validate, ParseVerbosity |

## Files Changed

Key files added/modified on this branch (summary):
| File | Action |
|------|--------|
| `internal/output/config.go` | created — `Config` struct, `Validate`, `ParseVerbosity`, `SupportedFormats`, `ErrUnknownFormat` |
| `internal/output/config_test.go` | created — table-driven tests |
| `schemas/project-v1.json` | created — Draft 2020-12 project schema |
| `schemas/schemas.go` | modified — added `ProjectV1` embed |
| `schemas/collection-v1.json` | modified — added `output` property + `$defs.output` |
| `internal/schema/schema.go` | modified — added `var ProjectSchema` |
| `internal/schema/project_schema_test.go` | created |
| `internal/schema/validate_test.go` | modified — drift guard tests |
| `internal/schema/validate_coverage_test.go` | modified — accept/reject tests |
| `internal/schema/testdata/output_*.yaml` | created — 11 test fixtures |
| `internal/parser/collection.go` | modified — added `Output *output.Config` field |
| `internal/parser/collection_test.go` | modified — `TestCollection_Output_Roundtrip` |
| `internal/config/project.go` | modified — added `Output *output.Config` to both structs |
| `internal/config/project_test.go` | modified — `TestParseProjectConfig_Output` |
| `cmd/apitest/main.go` | modified — `runFlags` parallel bools, `resolveOutputPrecedence`, `--project` flag, `--project-name` flag, two-phase events/gate |
| `cmd/apitest/output_precedence_test.go` | created — `TestOutputPrecedence*` |
| `cmd/apitest/main_test.go` | modified — `TestSchemaCmd` rows, `TestInitCmd` row |
| `internal/scaffold/scaffold.go` | modified — `apitestYAML` emits `output:` block |
| `internal/scaffold/scaffold_test.go` | modified — output block assertion |
| `examples/output-block/` | created — demo and bad-format fixtures |
| `docs/MANUAL.md` | modified — §1.5 second yaml.schemas, §3.6.1 output block section |
| `CHANGELOG.md` | modified — M8-003 bullet |
| `management/tasks/M8-003.yaml` | modified — status: review |
| `management/plans/M8-003-plan.md` | created |
| `management/reviews/M8-003-review.md` | created (iterations 1 and 2) |
| `management/plans/M8-003-improved.md` | created |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
