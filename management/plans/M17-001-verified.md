# Verification Report: M17-001

**Task:** Signer registry foundation and `signing:` request field
**Verified by:** AI
**Date:** 2026-04-29
**Branch:** feature/M17-001-signer-registry-foundation
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | (run via ci-local.sh) |
| `golangci-lint run` | PASS | 0 findings |
| `./smoke/run.sh` | PASS | All smoke checks pass (junit soft-fail is pre-existing, does not exit non-zero) |
| Coverage `internal/signer` | 100% | Exceeds >= 80% threshold |
| Coverage `internal/parser` | 90.1% | Exceeds >= 80% threshold, no regression |
| Coverage `internal/runner` | 85.0% | Exceeds >= 80% threshold, no regression |
| `./scripts/ci-local.sh --go` | PASS | Exit code 0 |

## Observable Output

```
$ go test -run 'TestSignerRegistry_RegisterAndLookup' -v ./internal/signer/...
=== RUN   TestSignerRegistry_RegisterAndLookup
--- PASS: TestSignerRegistry_RegisterAndLookup (0.00s)
PASS

$ go test -run 'TestSignerRegistry_BuiltinTypesEmpty' -v ./internal/signer/...
=== RUN   TestSignerRegistry_BuiltinTypesEmpty
--- PASS: TestSignerRegistry_BuiltinTypesEmpty (0.00s)
PASS

$ go test -run 'TestParser_SigningField_RequestLevel' -v ./internal/parser/...
=== RUN   TestParser_SigningField_RequestLevel
--- PASS: TestParser_SigningField_RequestLevel (0.00s)
PASS

$ go test -run 'TestParser_SigningField_CollectionDefault' -v ./internal/parser/...
=== RUN   TestParser_SigningField_CollectionDefault
--- PASS: TestParser_SigningField_CollectionDefault (0.00s)
PASS

$ go test -run 'TestRunner_SignerStep_InvokedBetweenTemplatingAndExec' -v ./internal/runner/...
=== RUN   TestRunner_SignerStep_InvokedBetweenTemplatingAndExec
--- PASS: TestRunner_SignerStep_InvokedBetweenTemplatingAndExec (0.00s)
PASS

$ go test -run 'TestRunner_SignerStep_NoSigning_FastPath' -v ./internal/runner/...
=== RUN   TestRunner_SignerStep_NoSigning_FastPath
--- PASS: TestRunner_SignerStep_NoSigning_FastPath (0.00s)
PASS

$ grep -nF 'apitest-sigv4' docs/MANUAL.md | grep -v '8\.[0-9]' || echo "doc-fix landed"
doc-fix landed

$ grep -nE 'plugin loading.*Enterprise|Enterprise.*plugin' docs/MANUAL.md | head -5
3135:Plugins are external processes ... **Plugin loading is an Enterprise-tier feature.** ... Built-in signers ... universally available across all tiers...

$ git diff internal/auth/profile.go
(empty — no lines changed)
```

Expected: All tests PASS, doc-fix landed, Enterprise clarification present, auth/profile.go unchanged.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Register + Lookup returns registered factory | `TestSignerRegistry_RegisterAndLookup` | PASS |
| 2 | Lookup unknown → ErrUnknownSignerType with sorted available list | `TestSignerRegistry_Lookup_UnknownType` | PASS |
| 3 | YAML `signing:` round-trips to `*SigningSpec` with Type and Params | `TestParser_SigningField_RequestLevel` | PASS |
| 4 | Collection default + per-request override + explicit null disables | `TestParser_SigningField_CollectionDefault` + `TestRunner_SignerStep_CollectionDefault_RequestNullDisables` | PASS |
| 5 | Unknown signer type → ErrUnknownSignerType on RequestResult.Err | `TestRunner_SignerStep_UnknownTypeProducesError` | PASS |
| 6 | Signer invoked between templating and exec; injected header present in exec | `TestRunner_SignerStep_InvokedBetweenTemplatingAndExec` + `TestRunner_Signing_EndToEnd_Noop` | PASS |
| 7 | No signing → exec pointer unchanged (fast path) | `TestRunner_SignerStep_NoSigning_FastPath` | PASS |
| 8 | sensitives.AddValue inside Sign registered on run's SensitiveSet | `TestRunner_SignerStep_SensitiveValueRegistered` | PASS |
| 9 | auth/profile.go unchanged | `git diff internal/auth/profile.go` produces 0 lines | PASS |
| 10 | Signer interface does NOT carry clock; package doc states no-clock contract | `internal/signer/signer.go` package doc | PASS |
| 11 | docs/MANUAL.md apitest-sigv4 removed; Enterprise clarification present | grep observables | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | 11/11 behaviors verified above | PASS |
| 2 | `go test ./...` passes | All packages pass (no failures) | PASS |
| 3 | `go test -cover ./internal/signer/... >= 80%` | 100% | PASS |
| 4 | `go test -cover ./internal/parser/... >= 80%` | 90.1% (no regression) | PASS |
| 5 | `go test -cover ./internal/runner/... >= 80%` | 85.0% (no regression) | PASS |
| 6 | `golangci-lint run` passes with 0 issues | 0 issues | PASS |
| 7 | `./smoke/run.sh` passes | All smoke checks pass | PASS |
| 8 | `./scripts/ci-local.sh` passes | Exit code 0 | PASS |
| 9 | `internal/auth/profile.go` unchanged | `git diff` produces 0 lines | PASS |
| 10 | `signer.go` package doc states no-clock contract | Present in package doc | PASS |
| 11 | `docs/MANUAL.md` no longer cites apitest-sigv4 | grep confirms "doc-fix landed" | PASS |
| 12 | `docs/MANUAL.md` Enterprise plugin tier-gate clarification present | grep confirms sibling paragraph | PASS |
| 13 | Smoke/integration fixture exercising end-to-end wiring | `TestRunner_Signing_EndToEnd_Noop` | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — all errors wrapped with `%w`, sentinel `ErrUnknownSignerType` tested |
| Naming conventions | PASS — no stuttering, doc comments on all exports, `-er` convention on `Signer` |
| Code organization | PASS — `internal/signer` independent package, signing logic in `runner/signing.go` |
| Test quality | PASS — TDD pattern visible in commits, table-driven tests, atomic counters |
| No test-only helpers in production code | PASS — finding from review iteration 1 resolved |

(Branch A: Review PASS trusted from iteration 2, spot-check clean — error wrapping uses `%w`, exported symbols have doc comments, tests exercise real behaviors)

## Commits

| Hash | Message |
|------|---------|
| 920d219 | docs(review): add passing review for M17-001 |
| 82911b1 | docs(review): add improvement report for M17-001 |
| 1cc3f51 | fix(parser,runner): remove test-only helper from production code and improve test safety |
| e7fd411 | docs(review): add review with findings for M17-001 |
| 953e5f0 | chore(task): mark M17-001 as review |
| 56bac47 | test(runner): add end-to-end signing integration test with noop signer |
| f19ba8f | docs(manual): replace apitest-sigv4 plugin example with built-in signing |
| 41934fd | feat(parallel): add PreExec per-item context hook to Config |
| 0648efa | test(parallel): add failing tests for PreExec per-item context hook |
| 87fcb3c | feat(runner): add signer exec-wrap step between templating and HTTP dispatch |
| 78bf3e2 | test(runner): add failing tests for signer exec-wrap step |
| 955f492 | feat(parser): add signing: field to Collection and RequestItem |
| 54bb49e | test(parser): add failing tests for signing: field on RequestItem and Collection |
| adaa08d | feat(signer): implement signer registry, Signer interface, and context key helpers |
| c7df017 | test(signer): add failing tests for signer registry and context seam |
| 88c4363 | chore(task): mark M17-001 as in_progress |
| 731857e | chore(task): mark M17-001 as planned |
| 2cae596 | docs(plan): add implementation plan for M17-001 |

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/signer/signer.go` | created | Signer interface, Registry, ErrUnknownSignerType, context key helpers |
| `internal/signer/signer_test.go` | created | Registry unit tests, fake signer |
| `internal/signer/hints_init.go` | created | Init-time error-hint placeholder |
| `internal/parser/collection.go` | modified | Added `Signing *SigningSpec` to Collection and RequestItem, SigningSpec type with UnmarshalYAML |
| `internal/parser/collection_test.go` | created | Parser signing field tests |
| `internal/parser/testdata/with_signing_request.yaml` | created | Test fixture |
| `internal/parser/testdata/with_signing_collection_default.yaml` | created | Test fixture |
| `internal/parser/testdata/signing_missing_type.yaml` | created | Negative test fixture |
| `internal/parser/testdata/signing_non_mapping.yaml` | created | Negative test fixture |
| `internal/runner/signing.go` | created | resolveSigningSpec + collectionUsesSigning helpers |
| `internal/runner/runner.go` | modified | Signer exec-wrap step, per-item ctx threading, VarSources.Signer field |
| `internal/runner/runner_test.go` | modified | TestRunner_SignerStep_* tests |
| `internal/runner/signing_integration_test.go` | created | End-to-end noop signer integration test |
| `internal/parallel/executor.go` | modified | Added PreExec hook to Config |
| `internal/parallel/executor_test.go` | modified | TestExecuteWaves_PreExec_* tests |
| `docs/MANUAL.md` | modified | Replaced apitest-sigv4 example; added Enterprise clarification |
| `management/backlog.yaml` | modified | Status updated |
| `management/plans/M17-001-plan.md` | created | Implementation plan |
| `management/reviews/M17-001-review.md` | created | Review report (PASS on iteration 2) |
| `management/plans/M17-001-improved.md` | created | Improvement report |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
