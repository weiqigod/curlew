# Verification Report: M21-001

**Task:** Collection/project JSON Schema completeness: 9 parser fields missing, editors flag valid collections
**Verified by:** AI
**Date:** 2026-08-04
**Branch:** fix/M21-001-schema-completeness
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `./scripts/ci-local.sh` | PASS | exit 0. Auto-scope resolved to the Go gate — no `src/`, `web/` or stack files changed on this branch. |
| `go build ./cmd/curlew` | PASS | Clean, no warnings |
| `go test ./...` | PASS | All packages |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Smoke suite clean |
| Coverage | **85.0%** | Meets the ≥ 80% threshold |

## Observable Output

All four observables from the task YAML, run verbatim.

**1. Every parser field is present in the emitted schema**

```
MISSING: none
```

Expected `MISSING: none`. Result: **MATCH** (was previously all nine: `cel, config,
depends_on, graphql, if, protocol, signing, websocket`).

**2. A collection using those fields validates cleanly**

```
0 error(s)
```

Expected `0 error(s)`. Result: **MATCH**. Run with Python `jsonschema`
`Draft202012Validator` — an implementation independent of the Go library the test
suite uses, so this is not the schema agreeing with itself.

The same validator was then given six malformed documents to confirm the
completeness was not bought by weakening `additionalProperties: false`:

```
REJECTED  misspelled top-level key (assertiosn)
REJECTED  typo inside signing (parms)
REJECTED  typo inside graphql (querry)
REJECTED  unknown protocol (grpc)
REJECTED  unknown websocket action (yell)
REJECTED  typo in collection config (locail)
```

**3. Committed files match the binary's output**

```
collection in sync
project in sync
```

Result: **MATCH**. Noted as tautological — `curlew schema` writes the `go:embed`ed
bytes of the file being diffed. See "Honest caveats" below.

**4. Published `$id` points at the real repository**

```
schemas/collection-v1.json:"$id": ".../weiqigod/curlew/main/schemas/collection-v1.json"
schemas/project-v1.json:"$id":    ".../weiqigod/curlew/main/schemas/project-v1.json"
```

Result: **MATCH** for the org segment. The URL does **not** resolve — see "Honest
caveats".

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Collection using `if`, `depends_on`, `signing`, `protocol`, `graphql`, `websocket`, `cel`, `config` validates cleanly | `TestSchema_accepts` (7 new fixtures), `TestSchema_examples`, observable 2 | PASS |
| 2 | Every yaml-tagged parser field is described in the schema | `TestSchema_parser_fields_are_described`, `TestSchema_properties_have_parser_fields`, `TestSchema_parity_table_covers_every_parser_struct` | PASS |
| 3 | Committed schema files are byte-identical to `curlew schema` output, enforced by a test | `TestSchema_published_path_matches_embed`, `TestSchema_project_published_path_matches_embed` | PASS (pre-existing; tautological) |
| 4 | `$id` resolves rather than 404s; org matches the remote | `TestSchema_ids_match_module_path`, `scripts/check-schema-urls.sh` | PARTIAL — premise false, resolved in docs |
| 5 | MANUAL §1.5 no longer names `auth`, `retry`, `data_driven`, the section object form, or the variables object form as uncovered | Prose; verified by grep (`isn't covered yet` and `not yet schema-described` both absent) | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` clean | PASS |
| 2 | A collection exercising all nine fields validates cleanly | `gap_15_all_nine_fields.yaml` passes `TestSchema_accepts` **and** `TestSchema_dod_fixture_parses` (real parser round-trip) | PASS |
| 3 | Both schema files regenerated and committed | `schemas/collection-v1.json` +143, `schemas/project-v1.json` +13 | PASS |
| 4 | A test fails if committed files drift from `curlew schema` | `TestSchema_published_path_matches_embed` | PASS (tautological — see caveats) |
| 5 | `$id` values resolve against the real remote | Org corrected and pinned; resolution blocked by repo visibility | PARTIAL — see caveats |
| 6 | `docs/MANUAL.md` §1.5 caveat corrected | Rewritten as "What the schema covers" / "What it deliberately does not do" | PASS |
| 7 | CLI_SPECIFICATION Appendix A divergence note updated or removed | Replaced with a description of the guards; two spec claims corrected alongside | PASS |
| 8 | `./scripts/ci-local.sh --go` passes | exit 0 | PASS |
| 9 | CHANGELOG.md updated | Entry under `[Unreleased] → Fixed` | PASS |

## Guard Effectiveness (mutation testing)

A test that passes proves nothing about whether it *can* fail. Three regressions
were injected into `schemas/collection-v1.json` and the schema restored after each:

| Mutation | Caught by | Message |
|----------|-----------|---------|
| Removed `if` from `$defs/requestItem` | `TestSchema_parser_fields_are_described` | `parser accepts "if" but schema "$defs/requestItem" does not describe it` |
| Added `timeout_ms` to `$defs/request` | `TestSchema_properties_have_parser_fields` | `schema "$defs/request" describes "timeout_ms" but the parser binds no such key` |
| Dropped `websocket` from the `protocol` enum | `TestSchema_enums_match_parser` | `enum at "$defs/request/properties/protocol": "websocket" is expected but absent` |

`git diff --stat schemas/collection-v1.json` is empty afterwards; the full suite
is green.

## Code Review

Branch A — `management/reviews/M21-001-review.md` verdict **PASS** (second pass;
all four first-pass findings resolved in `99cf9e2`).

| Check | Status |
|-------|--------|
| Error handling | PASS — no new error paths; the four extracted comparisons preserve their `apierrors.Structured` returns, sentinels and categories exactly |
| Naming conventions | PASS — four new exported vars, each with a doc comment, following the existing `output.SupportedFormats` precedent |
| Code organization | PASS — no new package dependencies; `internal/parser` does not import `internal/schema`, so the parity test creates no cycle |
| Test quality | PASS — table-driven with `t.Run` subtests, explicit `error case -` rows, negative controls for every new definition, plus the mutation testing above |

Spot-checks performed beyond reading: the three mutations above, and the
independent-validator run in observable 2.

## Honest caveats

Two DoD items are signed off with qualifications rather than silently:

**DoD 4 is a tautology.** `curlew schema` writes the `go:embed`ed bytes of the file
being diffed, and `go test` recompiles the embed from that same file, so the check
cannot fail. It was already satisfied before this task began. No second tautology
was added to dress it up. The guard that actually prevents recurrence is
`internal/schema/parity_test.go`, demonstrated above.

**DoD 5 rests on a false premise.** The behaviour assumed the `$id` 404'd because
of the wrong org. The org *was* wrong and is fixed, but `gh repo view` reports
`"isPrivate": true`, and `raw.githubusercontent.com` returns 404 for a private
repository regardless of org — so the URL would have 404'd either way. Rather than
leave MANUAL §1.5 offering a dead link, it now directs out-of-tree users to emit
the schema from their own binary (`curlew schema > .curlew/schemas/collection-v1.json`),
which is better advice regardless — the schema they get is the one their installed
binary enforces — and states plainly that the published URL works only while the
repository is public. `scripts/check-schema-urls.sh` makes the claim checkable; it
is deliberately outside `ci-local.sh` because it needs network and a private repo
would redden the gate for a non-defect.

## Scope beyond the task, each verified

| Defect | How found | Fix |
|--------|-----------|-----|
| `$defs.request` required `method`; the parser defaults it three ways (GET, POST for graphql, WS for websocket) | Plan-phase exploration | `required` reduced to `["url"]`; `TestSchema_method_is_optional_url_is_not`. The WebSocket example in CLI_SPECIFICATION §12.3 was itself being flagged. |
| `project-v1.json` omitted the `config:` block `internal/config` binds | Plan-phase exploration | Root property + duplicated `$defs.config` pinned by `TestSchema_config_defs_match` |
| `ErrUnsupportedProtocol` hint offered `https`, which the parser rejects | Closed-set extraction | Hint derived from `SupportedProtocols`; `TestParser_protocol_hint_lists_only_accepted_values` |
| `graphql.error_handling` documented as two values in three places; the parser accepts three | Plan-phase exploration | Corrected in CLI_SPECIFICATION §12.2 and the `GraphQLConfig` struct comment; pinned by `TestSchema_enums_match_parser` |
| Published schema URL 404s | `scripts/check-schema-urls.sh` | MANUAL §1.5 rewritten (above) |

## Commits

| Hash | Message |
|------|---------|
| `0a933ef` | docs(plan): add implementation plan for M21-001 |
| `2cc78cc` | chore(task): mark M21-001 as planned |
| `9360421` | chore(task): mark M21-001 as in_progress |
| `6ebdfb7` | test(config): add failing tests for schema $id org segment |
| `befcf19` | fix(config): point schema $id at the real remote |
| `b8c4879` | test(config): add failing schema/parser parity tests |
| `5938084` | test(config): add failing tests for the nine undescribed schema fields |
| `1b87936` | fix(config): describe every parser field in the JSON Schemas |
| `f1cc15c` | test(config): add fixtures exercising the nine reinstated fields |
| `f3f8f80` | refactor(parser): pin the four closed-value sets to exported lists |
| `b0ac878` | docs: describe the completed schema and correct two spec claims |
| `694d4a5` | docs(plan): record execute-phase deviations for M21-001 |
| `8b0df24` | chore(task): mark M21-001 as review |
| `09623bc` | docs(review): add review with findings for M21-001 |
| `99cf9e2` | fix(config): address M21-001 review findings |
| `874fe4a` | docs(review): record passing second review for M21-001 |

TDD pattern is visible: every `fix(...)` is preceded by the `test(...)` commit
that made it necessary.

## Files Changed

26 files, +3,232 / −36.

| File | Action | Lines |
|------|--------|-------|
| `schemas/collection-v1.json` | modified | +143/−… (9 properties, 8 new `$defs`, `required` relaxed, `$id`) |
| `schemas/project-v1.json` | modified | +13 (`config` property + `$defs`, `$id`) |
| `internal/schema/parity_test.go` | created | +489 |
| `internal/schema/validate_coverage_test.go` | modified | +173 |
| `internal/parser/closedsets_test.go` | created | +162 |
| `internal/schema/astsource_test.go` | created | +160 |
| `internal/schema/validate_test.go` | modified | +55 |
| `internal/parser/parser.go` | modified | +45/−… (closed sets, derived hints) |
| `internal/schema/testdata/gap_9…gap_15*.yaml` | created | +202 across 7 fixtures |
| `scripts/check-schema-urls.sh` | created | +38 |
| `docs/CLI_SPECIFICATION.md` | modified | +30/−… |
| `docs/MANUAL.md` | modified | +23/−… |
| `CHANGELOG.md` | modified | +39 |
| `internal/parser/hints_init.go` | modified | +8/−… |
| `internal/parser/collection.go` | modified | +2/−2 |
| `internal/schema/project_schema_test.go` | modified | +2/−2 |
| `management/**` | plan, review, task, backlog | +1,684 |

## Issues Found

**Pre-existing, unrelated to this task.** `TestTAPOutput_ParallelSpeedup`
(`cmd/curlew/main_test.go:6892`) is flaky. Verified on `main` in a clean worktree
with none of this branch's changes: **2 failures in 8 consecutive runs**. It
compares `speedup_factor` between two separate wall-clock executions, so machine
load makes the values legitimately diverge. Filed separately; recorded here so an
intermittent red CI is not attributed to this branch.

No issues attributable to this task.

## Recommendation

**PASS** — ready for PR and merge.
