# Code Review: M9-005

**Task:** markdown shipment: init --output flag, docs, CHANGELOG, IMPROVEMENT.md W4 status
**Reviewer:** AI
**Date:** 2026-04-25
**Branch:** feature/M9-005-markdown-shipment
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All five findings from iteration 1 are verified resolved:

| Original # | Severity | Finding | Resolution Status |
|------------|----------|---------|------------------|
| 1 | Medium | Behavior 2 in task YAML said "no output: block" but implementation kept M8-003 baseline | Fixed: task YAML behavior 2 now reads "produces the same scaffold as the M8-003 baseline", consistent with implementation |
| 2 | Medium | `TestInit_OutputAllFormats` did string-substring checks only; no schema validation; no markdown fixtures | Fixed: `TestSchema_scaffolded_all_output_formats_validate` added in `internal/schema/validate_test.go`; `output_collection_markdown.yaml` and `output_project_markdown.yaml` fixtures added; both registered in `TestSchema_accepts_output` |
| 3 | Low | `outputBlock` default branch untested | Fixed: `TestOutputBlock_DefaultFallback` added in `internal/scaffold/scaffold_test.go`; `outputBlock` coverage now 100% |
| 4 | Low | CHANGELOG observable awk range `/^## \[/` terminated on the opening line | Fixed: range changed to `/^## \[0/` in `management/tasks/M9-005.yaml` |
| 5 | Low | W4 status observable `grep -A 1` did not reach the `**Status:**` line | Fixed: changed to `grep -A 2 … \| head -3` |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All error returns wrapped with `fmt.Errorf("context: %w", err)`. `initCmdOut` uses exit code 3 for invalid --output, exit 1 for I/O errors. No swallowed errors. Sentinel `ErrProjectExists` used correctly. |
| Input Validation | PASS | Unknown `--output` values validated before any file is written. `--output` without a value produces an error. Empty string treated as no-flag-passed (documented). |
| Naming | PASS | No stuttering. All exported symbols have doc comments (`Options`, `Init`, `ErrProjectExists`, `outputBlock`, `printInitHelpTo`). Package names are lowercase single-word. |
| Code Organization | PASS | `internal/scaffold` remains leaf-level. Validation stays in caller (`cmd/apitest`). `outputBlock()` is a clean pure function. `internal/schema` test-package imports `internal/scaffold` — no circular import (scaffold does not import schema). |
| Correctness | PASS | All six supported formats produce the correct scaffold content validated against the project schema. Exit codes match conventions. Defensive default branch in `outputBlock` falls back to terminal and is now tested. Spec and implementation are aligned on bare-init behavior. |
| Test Quality | PASS | All five task behaviors are covered: `TestInit_OutputMarkdownFlag`, `TestInit_OutputAllFormats`, `TestInit_DefaultUnchanged`, `TestInit_OutputUnknownFormat`, `TestInit_Help_DocumentsOutputFlag`, `TestInit_OutputMarkdown_FullPipeline`, `TestOutputBlock_DefaultFallback`, `TestSchema_scaffolded_all_output_formats_validate`, `TestSchema_accepts_output` (with markdown fixture), `TestSchema_AcceptsMarkdownFormat`. Table-driven tests used throughout. |

## Test Coverage
- Coverage `internal/scaffold`: 83.9% (above 80% threshold)
- Coverage `cmd/apitest`: 81.3% (above 80% threshold)
- `outputBlock` default branch: 100% (finding #3 resolved)

## DoD Verification

| DoD Item | Status |
|----------|--------|
| All behavior tests pass | PASS |
| TestInit_OutputMarkdownFlag passes | PASS |
| TestInit_OutputAllFormats passes | PASS |
| TestInit_OutputUnknownFormat passes | PASS |
| TestInit_DefaultUnchanged passes | PASS |
| TestInit_Help_DocumentsOutputFlag passes | PASS |
| docs/SPECIFICATION.md contains Markdown Output Format subsection | PASS (line 3059) |
| docs/MANUAL.md contains VS Code split-pane worked example | PASS (line 1807) |
| CHANGELOG.md [Unreleased] has Added (--format markdown) and Changed (events schema v1.2) | PASS (lines 28, 10) |
| IMPROVEMENT.md §5 W4 status block flipped to Shipped with M9-001..M9-005 + PR references | PASS (line 205, PR #TBD placeholder correct pre-merge) |
| IMPROVEMENT.md §6 request_slug paragraph references v1.2 | PASS (line 351) |
| IMPROVEMENT.md §8.1 extended with v1.2 note | PASS (line 385) |
| go test ./... passes with no regressions | PASS |
| go test -cover ./internal/scaffold/... ./cmd/apitest/... >= 80% | PASS (83.9%, 81.3%) |
| golangci-lint run passes with 0 issues | PASS |
| ./smoke/run.sh passes | PASS |
| ./scripts/ci-local.sh passes | PASS |

## Summary

All five findings from iteration 1 are correctly resolved. The `--output` flag is fully wired through CLI parsing to scaffold generation, with proper validation (exit 3 for unknown values), complete help text, and all six formats tested at both the unit level and the schema-validation level. Documentation (SPECIFICATION.md §Markdown Output Format, MANUAL.md split-pane example), CHANGELOG, and IMPROVEMENT.md surgical edits are all correct and verified. Coverage exceeds 80% in all changed packages. No new findings identified in iteration 2.
