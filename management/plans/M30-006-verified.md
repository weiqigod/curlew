# M30-006 Windows Setup Follow-up

Date: 2026-09-21. Base: `4aaef40`. Task remains in progress.
Scope: user-approved removal of the vulnerable WinLibs development bundle and
separation of everyday verification from native race and release checks.
Earlier foundation evidence remains in [M30-004 verification](M30-004-verified.md).

## Machine Cleanup

- Removed `%LOCALAPPDATA%\Programs\CurlewTools\mingw64`, identified by its
  WinLibs GCC 16.2.0 manifest. No processes were using it before removal.
- Removed only its exact `bin` entry from user PATH and this terminal's PATH.
  No machine PATH entry existed. Existing VS Code/terminal processes may retain
  a stale PATH until restarted.
- The bundle contained Python 3.9.7 launcher templates and a Python 3.9.7 DLL
  imported by GDB. The entire bundle was removed, not just the flagged launchers.
- The installed Curlew executable is unchanged and starts successfully.
  No replacement compiler, debugger, VM or hosted runner was installed.
- Azure CLI, Codex, standalone Python, Go, Node, lint tools and API project
  configuration were left unchanged. Their separate security findings are not
  remediated by this cleanup.

## Verification Profiles

| Profile | Scope |
| --- | --- |
| Local (default) | Existing UI, ordinary Go, coverage, lint and smoke checks with cgo disabled; no compiler or GoReleaser dependency |
| Race | Native Windows race tests only; requires an approved compatible compiler |
| Release | Existing GoReleaser configuration check only; no archive build or publication |
| Full | All previous verification stages, including race and release checks |

Only Full emits `WINDOWS_VERIFY_PASS`. Each narrower profile reports its own
scope. Required tools fail closed. Process environment and directory are restored
on success, preflight return and failure.

## Evidence

| Check | Result |
| --- | --- |
| New native profile regression before implementation | RED: default requires release tool; profiles unsupported |
| Nine offline profile cases after implementation | PASS: command routing, cgo modes, missing tools, error propagation and environment restoration |
| Environment-restoration regression | RED/GREEN: absent GOROOT must remain absent, not become an empty variable |
| Existing Windows verifier and LF checkout regressions | PASS, native Windows with cgo disabled |
| Actual Local preflight with nonexistent compiler and release-tool names | PASS; caller environment restored |
| Compiler-free build, validate, HTTP assertions and invalid-YAML exit | PASS through `smoke/run.ps1` with `CGO_ENABLED=0` and an invalid `CC` path |
| Installed CLI and existing localhost UI after removal | PASS; executable SHA-256 unchanged, UI returns HTTP 200 with embedded asset references |
| Final focused verifier tests and changed-package lint | PASS; lint reports zero issues with cgo disabled |
| Editor diagnostics for script and tests | No reported errors |

The profile command tests use offline tool stubs; they prove orchestration, not
that the full underlying suites passed. No full native verifier, WSL race suite,
isolated Windows race suite or authoritative gate was rerun for this change.
Windows race coverage remains required for platform-specific concurrency claims;
WSL/Linux evidence cannot substitute for it. No cloud or business API was called.

## Pre-commit Verification (2026-09-22)

Scope: commit the six pending Windows setup/test/documentation files and merge
them into local main. No push, hosted CI, tool installation or API-project change.

| Check | Result |
| --- | --- |
| Native `go build ./cmd/curlew`, Go 1.27.1, cgo disabled | PASS |
| Whole-repository golangci-lint 2.11.2 with Go 1.26.8, uncapped findings | PASS, zero issues |
| Final `go test -json -p 4 -timeout=60m ./...`, cgo disabled | PASS, exit 0; 55 test packages, including 504 top-level CLI tests |
| Editor diagnostics and `git diff --check` | PASS |

The first full test attempt failed only
`TestCookbookRecipes/websocket-order-stream.svx`: the heartbeat-while-reading
request reported `websocket: close sent`. The other 54 test packages passed.
The unchanged failing subtest passed three consecutive focused repeats, followed
by the successful full command above. Successful package results were reused by
Go's test cache; the CLI package reran and passed in 995.657 seconds. No assertion
was weakened or test skipped to obtain the pass. The intermittent WebSocket
failure was not repaired by this setup change.

Retained logs:

- `%TEMP%/curlew-precommit-tests-20260922-074440.jsonl`: initial failure.
- `%TEMP%/curlew-precommit-confirm-20260922-080744.jsonl`: final full-command pass.

These results satisfy the local build/test/lint checks for this commit. They do
not establish a full Windows profile, race, clean-host or packaged-app acceptance
pass; the existing task statuses remain unchanged.