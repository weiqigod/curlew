# Implementation Plan: M6-001

## Overview

Lay the classification plumbing the agent event stream depends on: extend the existing error taxonomy, add a process-wide sentinel hint registry, and implement `ClassifyError` that walks any error chain and produces a `*Structured` carrying `Category`, `Code`, `Hint`, and `Inner`. No behavioural change in `cmd/apitest`; this task is foundation. A source-parsing coverage test enforces that every exported `Err*` sentinel in the `internal/` tree is registered with a `Category` and `Code`, and that every non-internal classification carries a concrete-action `Hint`.

## Task Details

- **ID:** M6-001
- **Title:** Error taxonomy extension and sentinel hint registry
- **Phase:** M6: AI Agent Integration
- **Priority:** 3
- **Complexity:** medium

## Dependencies

None. This is the first slice of M6.

## Architectural Decisions

1. **Distributed registration via `init()` per owning package.** `internal/errors` is already imported by packages that own sentinels (e.g. `internal/parser` uses `apierrors.Structured` in parser.go). The reverse import — `internal/errors` referencing those packages to register their sentinels — would create an import cycle. The cleanest workaround is to let each owning package register its own sentinels via an `init()` hook, calling into `apierrors.RegisterPackage`. This keeps the hint definitions colocated with the error declarations they classify.

2. **Registry keyed by error value with a Name field for coverage introspection.** The map is `map[error]ClassifiedHint` so `LookupHint` can use `errors.Is` against wrapped chains. Each `RegisteredError` also carries a `Name` field matching the var name (e.g. `"ErrInvalidYAML"`), which the coverage test cross-references against source-parsed declarations. Without the `Name` field, there is no robust way to confirm "every source-level `Err*` has been registered" because Go reflection cannot enumerate package-level vars.

3. **`ClassifyError` at the serialization boundary, not at error sites.** Error sites keep using sentinels and `fmt.Errorf("%w", ...)` exactly as today. Classification is a one-way projection performed by consumers (the events package in M6-004, the terminal formatter already via `apierrors.Format`). This decouples error taxonomy churn from the producers of errors.

4. **Four-case precedence in `ClassifyError`:** (a) chain already carries a `*Structured` — preserve `FilePath`/`Line` and enrich missing `Code`/`Hint` from the registry; (b) chain carries a `*NetworkError` — translate its `Kind` to a stable `Code` and wrap into `Structured` with `Category=network`; (c) chain contains a registered sentinel — build a `Structured` from the registry; (d) fallback — `Category=internal`, no `Code`, no `Hint`.

5. **Coverage test discovers sentinels by source parsing, not reflection.** Go does not expose package-level vars via reflection. The test uses `go/parser` to walk `internal/` for `var Err*` declarations, then cross-references against `RegisteredNames()`. Blank imports force each owning package's `init()` to run.

6. **Hint quality bar enforced by test, not convention.** `TestCoverage_ClassifiedEntriesHaveHints` fails if any registration with `Category != CategoryInternal` has an empty `Hint`. `TestCoverage_CodesAreUnique` fails on duplicate codes. These are structural guarantees — future additions cannot drift from the contract.

7. **`CategoryInternal` without hint is a valid allowlist.** Sentinels that are platform/internal (cache expiry, handshake timeouts, context cancels) are registered with `Category=CategoryInternal` and an empty `Hint`. They appear in the coverage map, pass the completeness check, and pass the hint-quality check.

## Implementation Steps

### Step 1: Extend `errors.go` with new Categories and Structured.Code

**Rationale:** Additive change; cannot break existing callers because existing sites populate by field name.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/errors/errors.go` | edit | Add CategoryAssertion, CategoryAuth, CategoryInput, CategoryInternal; add Code field to Structured |

### Step 2: Add the registry and classifier

**Rationale:** Single new file keeps the registration API, lookup functions, and the classifier together. Mutex-guarded state is init-time-populated and read-only afterward.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/errors/classify.go` | create | ClassifiedHint, RegisteredError, RegisterPackage, LookupHint, IsRegistered, RegisteredPackages, PackageRegistrations, RegisteredNames, ClassifyError, networkCodeFor helper |
| `internal/errors/classify_test.go` | create | Unit tests for every ClassifyError case, LookupHint, RegisterPackage idempotence, PackageRegistrations return shape |

### Step 3: Register every sentinel via per-package `init()` hooks

**Rationale:** Distributes the registration workload to the packages that own each sentinel (avoids import cycles; keeps hints colocated with error declarations).

#### Files to Modify

One new `hints_init.go` in each owning package:

| Package | Sentinels registered | Category breakdown |
|---------|----------------------|--------------------|
| parser | 12 | parse |
| variable | 10 | config(2), input(5), auth(1), assertion(2) |
| assertion | 2 | assertion |
| auth | 5 | auth(3), internal(2) |
| vault | 7 | auth(4), config(3) |
| vault/teamtemplate | 9 | config(7), input(2) |
| config | 4 | config(3), input(1) |
| httpbody | 2 | parse |
| httpexec | 1 | network |
| datadriven | 6 | input |
| discovery | 3 | input |
| graphql | 3 | parse |
| graphql/files | 6 | parse |
| openapi | 2 | input |
| websocket | 11 | network(6), parse(2), assertion(2), internal(1) |
| websocket/templates | 1 | parse |
| runner | 1 | auth |
| runner/distributed | 1 | input |
| plugin | 7 | internal(5), config(2) |
| plugin/hooks | 1 | internal |
| scaffold | 1 | input |
| jsonpath | 2 | assertion |
| loadgen | 5 | input |
| loadgen/report | 1 | input |
| worker | 4 | input(2), auth(1), network(1) |
| prcheck | 3 | input(1), auth(1), network(1) |
| license | 7 | auth(5), internal(2) |
| license/export | 2 | internal(1), input(1) |
| license/jwks | 1 | internal |

### Step 4: Coverage test

**Rationale:** Future additions to the codebase must automatically be caught. The test is a regression fence: add a new `Err*` sentinel, forget to register it, CI fails with a pointer to the exact (package, name).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/errors/coverage_test.go` | create | Source-parses internal/ for Err* declarations; asserts registration; asserts Category+Code completeness; asserts classified entries have hints; asserts Codes are unique |

## Observable Output

```bash
go test ./internal/errors/...
# ok  github.com/peterlindqvist/apitest/internal/errors (~0.4s)

go test -cover ./internal/errors/...
# Coverage meets >= 80% gate
```

## Risks and Mitigations

- **Import cycle accident.** Any attempt to add `import "internal/parser"` to `internal/errors` will fail at compile. Distributed registration is the mitigation.
- **New sentinel added without registration.** Coverage test catches this at `go test` time with a diff listing missing `(package, name)` pairs.
- **Hint drift over time.** The hint quality bar test (`TestCoverage_ClassifiedEntriesHaveHints`) fails CI if any non-internal registration has an empty `Hint`. Reviewers see hints in diffs like any other code.

## Test Strategy

- Unit tests for every `ClassifyError` precedence case against synthetic sentinels scoped to the test file (to avoid polluting the real registry).
- Unit tests for `LookupHint` wrapped vs direct, `IsRegistered`, `RegisterPackage` idempotence, introspection accessors.
- Source-walking coverage test as described above.
- Full `go test ./...` to confirm no regression.
- `golangci-lint run` across the whole tree.
