# Code Review: M9-001

**Task:** request_slug: derive per-request identifier and emit in events schema v1.2
**Reviewer:** AI
**Date:** 2026-04-25
**Branch:** feature/M9-001-request-slug
**Iteration:** 2 (re-review after /improve)

## Verdict: PASS

## Findings

No findings. All four issues from iteration 1 have been correctly resolved.

## Previous Findings — Resolved

| # | Severity | Finding | Resolution | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `TestEvents_v12_RequestSlug` did not exist | Implemented in `internal/output/events/emitter_test.go` with 5 sub-tests covering schema_version 1.2, slug on start, slug on end, omitempty on both events, and multiple distinct slugs | `go test -run TestEvents_v12_RequestSlug ./internal/output/events/...` passes |
| 2 | High | Golden test helpers (`TestEmitter_GoldenRunHappy`, `TestEmitter_GoldenRunFailedAssertion`) set no `RequestSlug` on `RequestEndInput`; golden files lacked `request_slug` on `request.end` | Added `RequestSlug: "create-user"` and `RequestSlug: "get-user"` to both helpers; golden files regenerated and verified | Both golden files now have `request_slug` on both `request.start` and `request.end` lines |
| 3 | High | `TestRunner_RequestSlugAllEmitSites` was missing "parallel main waves", "data-driven parallel", and "websocket main" sub-tests | Three sub-tests added; parallel waves assert slug present and matching on all starts/ends; data-driven parallel checks slug prefix and start/end pairing; websocket checks slug on both events | `go test -run TestRunner_RequestSlugAllEmitSites ./internal/runner/...` passes all 5 sub-tests |
| 4 | Low | Two doc-comments in `schema_test.go` said "v1.1 JSON Schema" after migration to v1.2 | Both comments updated to "v1.2 JSON Schema" at lines 94 and 161 | Confirmed in source |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `fmt.Errorf("context: %w", err)`. `ErrSlugEmpty` sentinel added and registered in `hints_init.go` with code `PARSE_SLUG_EMPTY`. `populateSlugs` returns structured `apierrors.Structured` with file/line attribution. No swallowed errors. Data-driven `iterSlug, _ := parser.Slug(iterName)` correctly ignores an error that is provably impossible (iter names always have `[N/M]` with digits). |
| Input Validation | PASS | `Slug("")` returns `ErrSlugEmpty`. Parser rejects empty-after-slugify names at load time across all three phases (setup, main, teardown). Items with empty `Name` are skipped in `populateSlugs` (delegated to other validators). |
| Naming | PASS | No stuttering found. All exported symbols (`Slug`, `ErrSlugEmpty`, `RequestSlug`, `RequestEndInput.RequestSlug`) have doc comments. Package names conform to convention. `Slug string \`yaml:"-"\`` on `RequestItem` is not user-configurable. |
| Code Organization | PASS | `slug.go` correctly scoped to `internal/parser/`. `parallel.EventSink` widened in lock-step with `runner.EventSink`. No import cycles. `defer` not applicable to new code paths. |
| Correctness | PASS | Slug derivation algorithm matches spec (NFKD + strip Mn + lowercase + collapse + trim). Data-driven iteration slugs re-derived from iteration name for 1:1 match with emitted `name`. `omitempty` on JSON tags ensures forward-compat. v1.0 and v1.1 schema files are byte-unchanged (confirmed via `git diff main...HEAD`). Golden files have `schema_version: "1.2"` and `request_slug` on both request events. |
| Test Quality | PASS | `TestEvents_v12_RequestSlug` covers all required emit paths. `TestRunner_RequestSlugAllEmitSites` covers sequential, parallel, data-driven sequential, data-driven parallel, and websocket. `TestSlug_Derive` is fully table-driven with 33 cases. `TestParser_SlugEmptyRejection` covers all three phases. Historical regression tests `TestSchema_v10_validates` and `TestSchema_v11_validates` use hand-crafted lines and load explicit schemas. |

## Test Coverage

- `internal/parser`: 90.1% — above 80% threshold
- `internal/runner`: 85.1% — above 80% threshold
- `internal/output/events`: 96.9% — above 80% threshold

## DoD Verification

| DoD Item | Status |
|----------|--------|
| All behavior tests pass | PASS |
| `TestSlug_Derive` passes (ASCII, punctuation, unicode, case-fold, empty rejected) | PASS |
| `TestRunner_RequestSlugAllEmitSites` passes (sequential, parallel, data-driven, websocket) | PASS |
| `TestEvents_v12_RequestSlug` passes (schema_version 1.2, request_slug present, derivation matches) | PASS |
| `TestSchema_v12_validates` passes (v1.2 schema validates representative goldens) | PASS |
| `TestSchema_v11_validates` and `TestSchema_v10_validates` still pass (regression) | PASS |
| `TestParser_DuplicateNameRejection` still passes (M8-004 regression) | PASS |
| `docs/events-schema/v1.2.json` exists; v1.0.json and v1.1.json byte-unchanged | PASS |
| `docs/EVENTS_SCHEMA_v1.2.md` documents additive `request_slug` field and derivation rule | PASS |
| `go test ./...` passes with no regressions | PASS |
| Coverage `internal/parser`, `internal/runner`, `internal/output/events` >= 80% | PASS |
| `golangci-lint run` passes with 0 issues | PASS |
| `./smoke/run.sh` passes | PASS |
| `./scripts/ci-local.sh --go` passes | PASS |

## Summary

The implementation is complete and correct. All four findings from the iteration 1 review have been addressed: `TestEvents_v12_RequestSlug` now exists with comprehensive coverage, both golden test helpers correctly set `RequestSlug` on `RequestEndInput` (and the golden files reflect this), `TestRunner_RequestSlugAllEmitSites` now covers all five emit paths including parallel waves and websocket, and the stale doc-comments in `schema_test.go` have been corrected. The slug derivation helper, parse-time validation, event schema v1.2, and runner wiring are all well-executed with no remaining quality issues.
