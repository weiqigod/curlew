# Verification Report: M9-002

**Task:** markdown formatter: --format markdown with sentinel splice, JSON body, run.md index
**Verified by:** AI
**Date:** 2026-04-25
**Branch:** feature/M9-002-markdown-formatter
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | No races detected (included in ci-local.sh) |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage `internal/output/markdown` | 87.7% | Above >= 80% threshold |
| Coverage `internal/runner` | 85.1% | Above >= 80% threshold |
| Coverage `cmd/curlew` | 80.9% | At 80% threshold |

## Observable Output

```
# Missing --report with --format markdown exits 3 at load time
./curlew run collections/sample.yaml --format markdown 2>err.log; echo $?
[ERROR] format: markdown requires --report <dir>
3

# Schema accepts format: markdown
./curlew schema | jq '."$defs".output.properties.format.enum'
["terminal","json","tap","junit","html","markdown"]

./curlew schema --project | jq '."$defs".output.properties.format.enum'
["terminal","json","tap","junit","html","markdown"]
```

Expected: exit 3 with error message; both schemas include "markdown" in format enum.
Result: MATCH

Note: Full observable (file generation, splice, run.md links) verified via
`TestRun_MarkdownFormat_HappyPath`, `TestRun_MarkdownFormat_SpliceOnRerun`,
`TestRun_MarkdownFormat_NoSentinelWritesDotNew`, and
`TestRun_MarkdownFormat_MalformedSentinelWritesDotNew` — these exercise
real binary behavior via `runCmdInner` with an httptest server.

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | --format markdown requires --report; exits 3 at load time | `TestRun_MarkdownFormat_RequiresReport`, `TestConfig_RejectsMarkdownWithoutReport` | PASS |
| 2 | SupportedFormats includes "markdown"; schemas accept markdown format enum | `TestSchema_AcceptsMarkdownFormat` | PASS |
| 3 | `ensureReportDir(path, subpath...)` creates dir via MkdirAll 0755 | `TestMarkdown_EnsureReportDir` | PASS |
| 4 | Each main-phase request produces `<slug>.md` with 10-section order | `TestRun_MarkdownFormat_HappyPath`, `TestMarkdown_Render_PassJSON` | PASS |
| 5 | Sentinel opening/closing tags carry all three correlation IDs | `TestMarkdown_Splice_MismatchedRunStillSplices`, `TestRun_MarkdownFormat_HappyPath` | PASS |
| 6 | Re-running with matching sentinel rewrites only between sentinels | `TestMarkdown_Splice_MatchRewritesRegion`, `TestRun_MarkdownFormat_SpliceOnRerun` | PASS |
| 7 | Mismatched slug appends new block below, warns on stderr | `TestMarkdown_Splice_RenameAppends` | PASS |
| 8 | Malformed sentinel (BEGIN without END) writes .md.new; original untouched | `TestMarkdown_Splice_MalformedWritesDotNew`, `TestRun_MarkdownFormat_MalformedSentinelWritesDotNew` | PASS |
| 9 | No-sentinel file writes .md.new; original untouched; stderr warning | `TestMarkdown_Splice_NoSentinelWritesDotNew`, `TestRun_MarkdownFormat_NoSentinelWritesDotNew` | PASS |
| 10 | Atomic writes via O_EXCL temp + rename; concurrent runs produce no corrupt sentinels | `TestMarkdown_Concurrent` | PASS |
| 11 | LF inside sentinels; CRLF outside preserved verbatim | `TestMarkdown_Newlines` | PASS |
| 12 | Assertions section: `_No assertions declared._` when empty; checklist otherwise | `TestMarkdown_EmptyAssertions`, `TestMarkdown_Render_FailJSON` | PASS |
| 13 | JSON bodies pretty-printed in ```json fenced block | `TestMarkdown_Render_PassJSON`, `TestMarkdown_Render_FailJSON` | PASS |
| 14 | Timing subsection: three fixed-prefix lines (duration_ms, wave_index, started_at) | `TestMarkdown_Render_PassJSON` (golden) | PASS |
| 15 | run.md lists per-request files in execution order | `TestMarkdown_RunMD`, `TestMarkdown_RunMD_WithEnvName` | PASS |
| 16 | Two runs produce byte-identical sentinel regions except volatile lines | `TestMarkdown_DeterminismRegression` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` passes | PASS |
| 2 | TestMarkdown_Render passes | Both pass_json and fail_json golden tests pass | PASS |
| 3 | TestMarkdown_EmptyAssertions passes | `_No assertions declared._` rendered | PASS |
| 4 | TestMarkdown_Splice_MatchRewritesRegion passes | Content above/below preserved | PASS |
| 5 | TestMarkdown_Splice_RenameAppends passes | Orphan block preserved; stderr warning | PASS |
| 6 | TestMarkdown_Splice_MalformedWritesDotNew passes | .md.new written; original untouched | PASS |
| 7 | TestMarkdown_Splice_NoSentinelWritesDotNew passes | .md.new written; original untouched | PASS |
| 8 | TestMarkdown_Splice_MismatchedRunStillSplices passes | run= volatile, slug identity matches | PASS |
| 9 | TestMarkdown_Concurrent passes | No corrupt sentinels under goroutine concurrency | PASS |
| 10 | TestMarkdown_Newlines passes | LF inside; CRLF outside preserved | PASS |
| 11 | TestMarkdown_DeterminismRegression passes | Byte-identical after masking volatile lines | PASS |
| 12 | TestMarkdown_RunMD passes | Index links all per-request files | PASS |
| 13 | TestConfig_RejectsMarkdownWithoutReport passes | Exit 3 for YAML-resolved path | PASS |
| 14 | TestSchema_AcceptsMarkdownFormat passes | Both collection and project schemas valid | PASS |
| 15 | M9-001 and M8 tests pass unmodified | `go test ./...` passes; events golden unchanged | PASS |
| 16 | go test ./... passes with no regressions | ci-local.sh PASS | PASS |
| 17 | Coverage internal/output/markdown >= 80% | 87.7% | PASS |
| 18 | golangci-lint run passes 0 issues | ci-local.sh lint gate PASS | PASS |
| 19 | ./smoke/run.sh passes | ci-local.sh smoke gate PASS | PASS |
| 20 | ./scripts/ci-local.sh passes | === ci-local PASS === | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS trusted (iteration 8, dated 2026-04-25). Spot-check:
1. Error handling — `writer.go` uses `fmt.Errorf("context: %w", err)` at all error sites.
2. Exported symbols — `formatter.go` has doc comments on all exported constants, types, functions.
3. Test quality — `TestMarkdown_Splice_MatchRewritesRegion` tests actual splice behavior (verifies agent content above/below is preserved, not just that no error occurs).

## Commits

| Hash | Message |
|------|---------|
| c42d058 | docs(review): add passing review for M9-002 |
| 7a5155e | docs(review): add improvement report for M9-002 iteration 7 |
| ee0fafc | fix(runner): propagate RequestID/RequestSlug through parallel data-driven path |
| 11432ac | docs(review): add review with findings for M9-002 |
| 0678009 | docs(review): add improvement report for M9-002 iteration 6 |
| da66372 | test(markdown): cover environment name branch in run.md |
| 62a168d | fix(markdown): handle multi-pair sentinel files after orphan-append |
| 443aaec | docs(review): add review with findings for M9-002 |
| 96c5413 | docs(review): add improvement report for M9-002 iteration 5 |
| 91874c7 | fix(markdown): remove dead code and fix deprecated os.IsNotExist usage |
| de34db5 | fix(runner): set RequestSlug on all skipped RequestResult literals |
| ... (39 total commits from b632359 to c42d058) | |

## Files Changed

| File | Action |
|------|--------|
| `cmd/curlew/main.go` | modified — --format markdown dispatch, buildMarkdownReport |
| `cmd/curlew/run_test.go` | modified — TestRun_MarkdownFormat_* tests |
| `internal/datadriven/parallel.go` | modified — RequestID/RequestSlug in IterationResult |
| `internal/output/config.go` | modified — add "markdown" to SupportedFormats |
| `internal/output/config_test.go` | modified — swap invalid_format test from "markdown" to "yaml" |
| `internal/output/markdown/formatter.go` | created |
| `internal/output/markdown/formatter_test.go` | created |
| `internal/output/markdown/golden_test.go` | created |
| `internal/output/markdown/regression_test.go` | created |
| `internal/output/markdown/run_md.go` | created |
| `internal/output/markdown/run_md_test.go` | created |
| `internal/output/markdown/splice.go` | created |
| `internal/output/markdown/splice_test.go` | created |
| `internal/output/markdown/testdata/golden/*.md` | created (4 golden files) |
| `internal/output/markdown/writer.go` | created |
| `internal/output/markdown/writer_test.go` | created |
| `internal/runner/runner.go` | modified — RequestID/RequestSlug/RunID fields |
| `internal/runner/runner_test.go` | modified — new runner ID tests |
| `internal/schema/validate_coverage_test.go` | modified — TestSchema_AcceptsMarkdownFormat |
| `schemas/collection-v1.json` | modified — add "markdown" to format enum |
| `schemas/project-v1.json` | modified — add "markdown" to format enum |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
