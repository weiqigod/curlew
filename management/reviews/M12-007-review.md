# Code Review: M12-007

**Task:** $formatDate and $parseDate using Go reference-time layouts
**Reviewer:** AI
**Date:** 2026-04-28
**Branch:** feature/M12-007-format-parse-date

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors use `apierrors.Structured` with meaningful `Code`, `Message`, `Hint` fields. `parseDateToUTC` correctly populates `Inner` with the raw `*time.ParseError` for `errors.Is/As` traversal. `formatDate` does not set `Inner` on bad-input (intentional — there is no inner error to chain when input shape is simply unrecognised). The `Evaluate` wrapper adds `fmt.Errorf("$%s: %w", name, err)` context. No `%v` wrapping found. No swallowed errors. |
| Input Validation | PASS | Empty-input short-circuits before reaching `time.Parse` in both functions (dedicated `DYNFN_FORMATDATE_EMPTY_INPUT` / `DYNFN_PARSEDATE_EMPTY_INPUT` codes). Arity-0/1/3 arguments rejected via `twoArgs` wrapper with `DYNFN_ARITY`. int64-overflow digit strings (>19 digits) fall through to `DYNFN_FORMATDATE_BAD_INPUT`. Negative-integer strings, whitespace-prefixed digits, and bare date strings (no time component) are all correctly rejected. |
| Naming | PASS | `isAllDigits`, `formatDate`, `parseDateToUTC` are all unexported — correct since they are implementation helpers. No stuttering, no all-caps, no underscore names. All three have doc comments. |
| Code Organization | PASS | Both helpers are placed as file-level functions adjacent to `applyDateOffset`. Registrations live in `register()` next to the existing date-arithmetic block. No circular dependencies. No unused imports. `twoArgs` wrapper reused correctly — no bespoke arity logic. |
| Correctness | PASS | `time.Unix(n, 0).UTC().Format(layout)` for digit-path ensures consistent zone regardless of host locale. `parseDateToUTC` calls `.UTC()` before formatting, correctly converting non-UTC zone-tagged inputs. `formatDate` RFC3339 branch deliberately preserves source zone (documented in plan Decision 6 and MANUAL.md). `isAllDigits` guards `""` before the loop (the guard is unreachable from `formatDate`'s pre-check but is a valid defensive design). All `time.Parse` / `strconv.ParseInt` error paths handled. |
| Test Quality | PASS | All 8 spec behaviors covered by at least one test. Table-driven tests used for multi-case happy paths. Error-path tests verify `Code`, `Message` content, `Hint` MANUAL.md reference, `Inner` error, and truncation. `TestRegistry_FormatDate_Now` uses a shape-based regex assertion (not a frozen value) — correct since `$timestamp` is not seeded. `TestRegistry_ParseDate_NestedVar` exercises nested variable resolution through `Scope.Interpolate`. `TestRegistry_available_sorted` updated to 26. |

## Test Coverage

- Coverage: 96.6% (internal/variable package overall)
- `formatDate`: 100%
- `parseDateToUTC`: 100%
- `isAllDigits`: 85.7% — the `return false` on the empty-string guard (line 350) is unreachable in production: `formatDate` already rejects `""` before calling `isAllDigits`. This is intentional defensive programming, not a test gap.

## Spec Compliance

All 8 behaviors from `management/tasks/M12-007.yaml` are covered:

| # | Behavior | Test |
|---|----------|------|
| 1 | `$formatDate` with digit string parses as Unix seconds | `TestRegistry_FormatDate_FromUnix` |
| 2 | `$formatDate` with RFC3339 string reformats with layout | `TestRegistry_FormatDate_FromIso` |
| 3 | Unrecognisable input returns structured error naming input shape | `TestRegistry_FormatDate_BadInput` |
| 4 | `$parseDate` returns `t.UTC().Format("2006-01-02T15:04:05Z")` | `TestRegistry_ParseDate` |
| 5 | Bogus layout returns layout verbatim (layout-as-template) | `TestRegistry_FormatDate_LayoutAsTemplate` |
| 6 | Non-UTC zone-tagged input to `$parseDate` converted to UTC | `TestRegistry_ParseDate_NonUTCZone` |
| 7 | Arity 0/1/3 returns `DYNFN_ARITY` error naming arity 2 | `TestRegistry_FormatDate_ParseDate_arity_errors` |
| 8 | Empty input returns structured error naming empty input | `TestRegistry_FormatDate_EmptyInput`, `TestRegistry_ParseDate_EmptyInput` |

## MANUAL.md Compliance

Definition of Done item: "docs/MANUAL.md §3.7 documents `$formatDate`, `$parseDate`, the Go reference-time layout convention, and the layout-as-template footgun" — VERIFIED:

- Two rows added to the argument-bearing helpers table (lines 1062–1063).
- "Go reference-time layouts" subsection added with common-layouts table, usage guidance, and the layout-as-template footgun callout (lines 1121–1157).
- `$formatDate` removed from the "land in later M12 tasks" parenthetical (now mentions `$randomPassword`, `$randomBase64`).

## Summary

Implementation is clean, correct, and complete. Both `$formatDate` and `$parseDate` follow established patterns (`twoArgs` wrapper, `apierrors.Structured` codes, 32-char snippet truncation, `Inner` error chaining). All 8 spec behaviors are covered by targeted tests. MANUAL.md §3.7 is fully updated. Coverage remains at 96.6% with the only sub-100% function (`isAllDigits` at 85.7%) being a legitimate defensive guard that is structurally unreachable from the production call path.
