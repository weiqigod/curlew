# Verification Report: M8-002

**Task:** Schema completeness: auth, retry, data_driven, section/variables object forms, status union
**Verified by:** AI
**Date:** 2026-04-24
**Branch:** feature/M8-002-schema-completeness
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 findings |
| `./smoke/run.sh` | PASS | All smoke assertions pass |
| Coverage | `[no statements]` | Package is a single `var` alias — no executable statements; DoD threshold not violated |
| `./scripts/ci-local.sh --go` | PASS | All gates passed cleanly |

## Observable Output

```
=== RUN   TestSchema_accepts
=== RUN   TestSchema_accepts/request_auth_string
=== RUN   TestSchema_accepts/status_integer
=== RUN   TestSchema_accepts/status_array
=== RUN   TestSchema_accepts/section_object_form
=== RUN   TestSchema_accepts/requests_object_form
=== RUN   TestSchema_accepts/collection_retry
=== RUN   TestSchema_accepts/section_retry
=== RUN   TestSchema_accepts/request_retry
=== RUN   TestSchema_accepts/data_driven_request
=== RUN   TestSchema_accepts/variables_object_form
--- PASS: TestSchema_accepts (0.00s)
    --- PASS: TestSchema_accepts/request_auth_string (0.00s)
    --- PASS: TestSchema_accepts/status_integer (0.00s)
    --- PASS: TestSchema_accepts/status_array (0.00s)
    --- PASS: TestSchema_accepts/section_object_form (0.00s)
    --- PASS: TestSchema_accepts/requests_object_form (0.00s)
    --- PASS: TestSchema_accepts/collection_retry (0.00s)
    --- PASS: TestSchema_accepts/section_retry (0.00s)
    --- PASS: TestSchema_accepts/request_retry (0.00s)
    --- PASS: TestSchema_accepts/data_driven_request (0.00s)
    --- PASS: TestSchema_accepts/variables_object_form (0.00s)
PASS
ok  	github.com/weiqigod/curlew/internal/schema	0.190s
```

```
=== RUN   TestSchema_examples
=== RUN   TestSchema_examples/internal/schema/testdata/gap_1_request_auth.yaml
=== RUN   TestSchema_examples/internal/schema/testdata/gap_2_collection_retry.yaml
=== RUN   TestSchema_examples/internal/schema/testdata/gap_3_section_retry.yaml
=== RUN   TestSchema_examples/internal/schema/testdata/gap_4_request_retry.yaml
=== RUN   TestSchema_examples/internal/schema/testdata/gap_5_data_driven.yaml
=== RUN   TestSchema_examples/internal/schema/testdata/gap_6_section_object_form.yaml
=== RUN   TestSchema_examples/internal/schema/testdata/gap_6b_requests_object_form.yaml
=== RUN   TestSchema_examples/internal/schema/testdata/gap_7_variables_object.yaml
=== RUN   TestSchema_examples/internal/schema/testdata/gap_8_status_array.yaml
=== RUN   TestSchema_examples/internal/schema/testdata/gap_8_status_integer.yaml
=== RUN   TestSchema_examples/sample/hello.yaml
--- PASS: TestSchema_examples (0.00s)
PASS
ok  	github.com/weiqigod/curlew/internal/schema	0.183s
```

Expected: PASS for both observables.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `requestItem.auth` as string ref to auth profile | `TestSchema_accepts/request_auth_string`, `gap_1_request_auth.yaml` | PASS |
| 2 | `retry` at collection scope | `TestSchema_accepts/collection_retry`, `gap_2_collection_retry.yaml` | PASS |
| 3 | `retry` at section scope | `TestSchema_accepts/section_retry`, `gap_3_section_retry.yaml` | PASS |
| 4 | `retry` at request scope | `TestSchema_accepts/request_retry`, `gap_4_request_retry.yaml` | PASS |
| 5 | `data_driven` at request scope | `TestSchema_accepts/data_driven_request`, `gap_5_data_driven.yaml` | PASS |
| 6 | `setup`/`teardown`/`requests` accept object form `{retry, items}` | `TestSchema_accepts/section_object_form`, `requests_object_form` | PASS |
| 7 | `variables` accept object form with `from_command`/`value`/`sensitive`/`cache` | `TestSchema_accepts/variables_object_form`, `gap_7_variables_object.yaml` | PASS |
| 8 | `assertions.status` constrained to `oneOf[integer, array[integer]]` | `TestSchema_accepts/status_integer`, `status_array` | PASS |
| 9 | Every fixture in examples/ validates | `TestSchema_examples` (11 files: 10 gap fixtures + `sample/hello.yaml`) | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All eight gap-closing sub-tests pass | `TestSchema_accepts` — 10 sub-tests (gaps 2–9, gap 6 + 6b, gap 8 × 2) all PASS | PASS |
| 2 | Every collection fixture under examples/ still validates | `TestSchema_examples` — 11 files, all PASS | PASS |
| 3 | CHANGELOG.md updated with M8-002 entry | Entry present under `[Unreleased] › Added` at line 26 of CHANGELOG.md | PASS |
| 4 | `go test -cover ./internal/schema/... >= 80%` | Package reports `[no statements]` — single `var` alias with no executable statements; threshold not applicable and not violated | PASS |
| 5 | `./scripts/ci-local.sh` passes | `=== ci-local PASS ===` confirmed | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS (dated 2026-04-24, final review after all improvements applied). Spot-check clean:
- `filepath.Rel` error handled explicitly with fallback (not silently discarded)
- `compileCollectionSchema` has doc comment: "compileCollectionSchema compiles the embedded collection JSON Schema."
- `TestSchema_rejects_malformed_status` tests what it claims: validates that a status map and status string both fail schema validation (expects non-nil error).

## Commits

| Hash | Message |
|------|---------|
| 3c19519 | docs(review): add passing review for M8-002 |
| a078bd1 | docs(review): add improvement report for M8-002 |
| e4edba3 | fix(smoke): guard SEED_FILE with EXIT trap to prevent stale temp files |
| 86bc777 | fix(schema): remove duplicate helpers and add requests object-form test |
| f532b8c | docs(review): add review with findings for M8-002 |
| e08f40d | chore(task): mark M8-002 as review |
| 9017f45 | refactor(schema): align struct literal map keys with gofumpt |
| 8a56fa7 | docs(changelog): add M8-002 schema completeness entry |
| f421423 | feat(schema): Gap 7 - variables accept object form {from_command|value, sensitive, cache} |
| 20623dc | test(schema): RED - variables object form rejected by string-only additionalProperties |
| 6d24f33 | feat(schema): Gap 5 - add requestItem.data_driven and $defs/dataDriven |
| 30bd2dc | test(schema): RED - requestItem.data_driven not in schema |
| 34b1904 | feat(schema): Gaps 2/3/4 - add retry at collection and request scopes |
| 4442a52 | test(schema): RED - retry not in collection-level or requestItem properties |
| 72f6368 | feat(schema): Gap 6 - section phases accept object form {retry, items} |
| 802a0b6 | test(schema): RED - setup/teardown reject object form {items} |
| 88aeebb | feat(schema): Gap 8 - constrain assertions.status to oneOf[integer, array[integer]] |
| 2764b10 | test(schema): RED - assertions.status unconstrained, accepts map/string |
| 982da2f | feat(schema): Gap 1 - add requestItem.auth string property |
| 4c3afbd | test(schema): RED - requestItem.auth string not yet in schema |
| 6e300e4 | test(schema): add TestSchema_examples and TestSchema_accepts harness stubs |
| 0ae7421 | chore(task): mark M8-002 as in_progress |
| 08534d2 | chore(task): mark M8-002 as planned |
| 1337e25 | docs(plan): add implementation plan for M8-002 |

TDD pattern visible: `test(schema): RED` commits precede each `feat(schema)` GREEN commit. All commits include `Refs: M8-002`.

## Files Changed

| File | Action |
|------|--------|
| `schemas/collection-v1.json` | modified — all 8 schema gaps closed |
| `internal/schema/validate_coverage_test.go` | created — `TestSchema_accepts`, `TestSchema_examples`, negative controls |
| `internal/schema/validate_test.go` | modified — `publishedSchemaPath` delegates to `repoRoot`; `repoRoot` import consolidated |
| `internal/schema/testdata/gap_1_request_auth.yaml` | created |
| `internal/schema/testdata/gap_2_collection_retry.yaml` | created |
| `internal/schema/testdata/gap_3_section_retry.yaml` | created |
| `internal/schema/testdata/gap_4_request_retry.yaml` | created |
| `internal/schema/testdata/gap_5_data_driven.yaml` | created |
| `internal/schema/testdata/gap_6_section_object_form.yaml` | created |
| `internal/schema/testdata/gap_6b_requests_object_form.yaml` | created |
| `internal/schema/testdata/gap_7_variables_object.yaml` | created |
| `internal/schema/testdata/gap_8_status_array.yaml` | created |
| `internal/schema/testdata/gap_8_status_integer.yaml` | created |
| `smoke/run.sh` | modified — `SEED_FILE` guard + `trap` for stale temp file cleanup |
| `CHANGELOG.md` | modified — M8-002 entry added |
| `management/backlog.yaml` | modified — status: review |
| `management/plans/M8-002-plan.md` | created |
| `management/plans/M8-002-improved.md` | created |
| `management/reviews/M8-002-review.md` | created |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge. All 8 schema gaps closed, 10 acceptance sub-tests pass, 11-file regression suite passes, CI gate clean, review PASS with all 5 findings resolved.
