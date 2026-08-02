# Verification Report: M18-008

**Task:** CLI telemetry emitter: internal/telemetry package, persistent install_id, `curlew telemetry` subcommand
**Verified by:** AI
**Date:** 2026-05-19
**Branch:** feature/M18-008-cli-telemetry-emitter
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass, 0 failures |
| `go test -race ./...` (via ci-local) | PASS | No races detected |
| `golangci-lint run` (via ci-local) | PASS | No findings |
| `./smoke/run.sh` | PASS | M18-008 telemetry round-trip section passes |
| Coverage (`internal/telemetry`) | 82.0% | Meets >= 80% threshold |
| `internal/telemetry` test count | 33 tests | Meets >= 18 requirement |

## Observable Output

```
$ rm -rf ~/.config/curlew/install_id ~/.config/curlew/telemetry.json
$ ./curlew telemetry status
telemetry: disabled (no install_id; opt-in required)
# exit 1 ✓

$ ./curlew telemetry enable
telemetry: enabled; install_id=441572e4-df52-4652-b491-1389e7273c04; events will post to https://api.curlew.org/telemetry/events
# exit 0 ✓

# File created at ~/Library/Application Support/curlew/install_id (macOS config dir)
# stat -f '%Lp' → 600 ✓

$ ./curlew telemetry status
telemetry: enabled; install_id=441572e4-df52-4652-b491-1389e7273c04
# exit 0 ✓

$ ./curlew telemetry reset-id
telemetry: install_id regenerated; old id discarded
# exit 0 ✓

$ ./curlew telemetry disable
telemetry: disabled; install_id retained (use `reset-id` to regenerate or `delete-request` to remove)
# exit 0 ✓

$ ./curlew telemetry export
{
  "enabled": false,
  "endpoint": "https://api.curlew.org/telemetry/events",
  "install_id": "4523c1f0-10de-4ede-a775-5d2bed8a4b78",
  "recent_emissions": null
}
# exit 0 ✓

$ ./curlew telemetry delete-request
telemetry: local files removed; backend delete POST failed (offline). Re-run when online to send the delete request.
# exit 0 ✓ (offline — files still removed)
```

Expected: all verbs produce documented stdout/exit codes per task YAML observable
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `status` with no install_id → disabled, exit 1 | `TestTelemetryCmd_StatusOnFreshDir` | PASS |
| 2 | `enable` creates install_id mode 0600, telemetry.json enabled=true | `TestTelemetryCmd_EnableCreatesFiles`, `TestStoreEnable`, `TestStoreFilePermissions` | PASS |
| 3 | POST on `run` when enabled with install_id, event_type, payload, Idempotency-Key | `TestRunFiresTelemetryWhenEnabled`, `TestClientEmit_PostsCorrectBody` | PASS |
| 4 | No POST when telemetry disabled | `TestRunDoesNotFireWhenDisabled`, `TestEmitter_DisabledNoNetwork` | PASS |
| 5 | POST failure is silent, does not surface to user | `TestRunDoesNotFailOnTelemetryError`, `TestEmitter_BackendErrorSwallowed` | PASS |
| 6 | `reset-id` regenerates install_id, old id unrecoverable | `TestTelemetryCmd_ResetID`, `TestStoreResetID` | PASS |
| 7 | `delete-request` removes files and POSTs delete event | `TestTelemetryCmd_DeleteRequest_Online`, `TestTelemetryCmd_DeleteRequest_Offline` | PASS |
| 8 | `internal/telemetry` does NOT import `internal/backend` | CI grep guard in `scripts/ci-local.sh` | PASS |
| 9 | All events carry same install_id across runs; session_uuid nested in payload | `TestRunFiresTelemetryWhenEnabled`, `TestStoreEnableIdempotent` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | `go test ./internal/telemetry/... >= 18 tests` | 33 tests pass (33 counting subtests) | PASS |
| 2 | Real binary `curlew telemetry {enable,disable,status,reset-id,export,delete-request}` produces documented stdout/exit codes | Observable verification above | PASS |
| 3 | Test coverage >= 80% on internal/telemetry | 82.0% | PASS |
| 4 | go vet + staticcheck clean | `ci-local.sh` linting gate passed | PASS |
| 5 | `internal/telemetry` does NOT import `internal/backend` | CI grep guard at `scripts/ci-local.sh:115` | PASS |
| 6 | Help text for `curlew telemetry` updated; main help lists the subcommand | `./curlew --help` and `./curlew telemetry --help` verified via smoke | PASS |
| 7 | `docs/SPECIFICATION.md` Telemetry Phase 3 updated to reflect persistent install-ID model | Spec correction committed in `feat(telemetry): add spec correction` | PASS |
| 8 | Smoke test adds telemetry enable → status → disable round-trip | `smoke/run.sh` M18-008 section | PASS |
| 9 | CHANGELOG.md entry references v4-8, v4-11 | CHANGELOG.md updated in spec correction commit | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |
| `internal/backend` isolation | PASS |

Branch A: Review PASS trusted (iteration 3, post two improve passes). Spot-check clean:
- Error wrapping: all `fmt.Errorf("...: %w", err)` pattern confirmed in `store.go`
- Exported symbols: all have doc comments (`NewStore`, `Enable`, `Disable`, `Status`, `ResetID`, `DeleteLocal`, `RecordEmission`, `RecentEmissions`, `Emitter`, `Client`)
- Test quality: `store_test.go` tests are specific and exercise real I/O via `t.TempDir()`

## Commits

| Hash | Message |
|------|---------|
| `92d67e80` | docs(review): add passing review for M18-008 (iteration 3) |
| `2c3b4276` | docs(review): add improvement report for M18-008 (iteration 2) |
| `90bed1a3` | fix(telemetry): delete-request on fresh dir shows 'nothing to delete' |
| `8fa8a4bf` | fix(telemetry): Disable() is a no-op when telemetry.json absent |
| `89a60492` | docs(review): add review with findings for M18-008 (iteration 2) |
| `9c521f69` | docs(review): add improvement report for M18-008 |
| `66f86fb7` | fix(telemetry): align uuid_test.go uuidRE with stricter pattern |
| `fcddb8f9` | fix(telemetry): remove misleading time.Sleep calls |
| `e99fdc5e` | fix(telemetry): remove dead exported Store.InstallID() method |
| `2e8a569b` | fix(telemetry): guard delete-request POST on empty install_id |
| `32b4b7eb` | docs(review): add review with findings for M18-008 |
| `7dcd7dab` | chore(task): mark M18-008 as review |
| `2cefa92b` | test(telemetry): add coverage tests for NewSessionUUID + ResolvedEndpoint |
| `e95d8653` | feat(telemetry): add spec correction, CHANGELOG, CI guard, and smoke test |
| `5f6c442d` | feat(cli): wire telemetry run.completed emission into curlew run end-of-run |
| `0cf62b52` | test(cli): add failing integration tests for telemetry run.completed wiring |
| `0518413b` | feat(cli): implement curlew telemetry subcommand + help + sentinel registration |
| `ba573d42` | test(cli): add failing tests for curlew telemetry subcommand |
| `8b8c35c9` | feat(telemetry): implement Emitter (silent fire-and-forget facade) |
| `51faa7f3` | test(telemetry): add failing tests for Emitter facade |
| `83b5d38d` | feat(telemetry): implement HTTP Client with idempotency-key + timeout |
| `749a4f0f` | test(telemetry): add failing tests for HTTP Client |
| `29412c5a` | feat(telemetry): implement Store (install_id + telemetry.json persistence) |
| `6e5a99b7` | test(telemetry): add failing tests for Store persistence |
| `27e7f9e2` | feat(telemetry): implement newUUIDv4 + NewSessionUUID |
| `b3af24bf` | test(telemetry): add failing tests for newUUIDv4 |

TDD pattern confirmed: all `test(...)` commits precede corresponding `feat(...)` commits.

## Files Changed

| File | Action |
|------|--------|
| `internal/telemetry/uuid.go` | created |
| `internal/telemetry/uuid_test.go` | created |
| `internal/telemetry/store.go` | created |
| `internal/telemetry/store_test.go` | created |
| `internal/telemetry/client.go` | created |
| `internal/telemetry/client_test.go` | created |
| `internal/telemetry/emit.go` | created |
| `internal/telemetry/emit_test.go` | created |
| `internal/telemetry/hints_init.go` | created |
| `cmd/curlew/telemetry.go` | created |
| `cmd/curlew/telemetry_test.go` | created |
| `cmd/curlew/telemetry_run_test.go` | created |
| `cmd/curlew/main.go` | modified |
| `docs/SPECIFICATION.md` | modified |
| `CHANGELOG.md` | modified |
| `scripts/ci-local.sh` | modified |
| `smoke/run.sh` | modified |
| `internal/errors/coverage_test.go` | modified |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
