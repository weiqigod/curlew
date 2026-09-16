# Verification Report: M30-004

Date: 2026-09-16. Branch: feature/M30-004-windows-commands.
Verdict: **BLOCKED / NOT COMPLETE**. Task remains `in_progress`.

## Summary

Native command execution and all five CLI-backed vault providers are implemented
and pass scoped Windows and Linux tests. The established workflow cannot reach
review PASS or done: the authoritative gate fails in pre-existing Windows CLI
tests, required lint/race tooling is absent, and batch support has explicit limits.
No push, PR, or task completion was performed.

## Environment

| Host | Toolchain | Role |
| --- | --- | --- |
| Windows 11 Enterprise amd64 | Go 1.27.1; PowerShell host 7.6.6; system Windows PowerShell for command scripts | Native Windows execution |
| Existing Ubuntu WSL2, Linux 6.6.87.2 amd64 | Official Go 1.27.1 Linux archive in a disposable directory | POSIX regressions, not Windows evidence |

Linux archive SHA256:
`63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445`.
POSIX module lookups used `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off` and the
existing cached modules. No provider accounts, public test APIs, or paid services
were used. HTTP requests in integration tests target httptest loopback servers.

## Results

| Check | Result | Evidence |
| --- | --- | --- |
| Windows complete internal/variable suite | PASS | 300.836s; 95.8% coverage |
| Windows complete internal/vault suite | PASS | 399.864s; 97.8% coverage |
| Linux complete internal/variable suite | PASS | 101.285s; 96.6% coverage |
| Linux complete internal/vault suite | PASS | 15.103s; 97.8% coverage, after cold CLI build was warmed |
| Adjacent runner/service provider and sensitive tests | PASS | Focused patterns below |
| Early-error runtime-sensitive retention | PASS | Supplied and internally allocated sets; no HTTP on scope failure |
| POSIX real CLI interrupt cleanup | PASS | Exit 5; child/process group gone; zero HTTP requests |
| Native Windows build/version | PASS | curlew 0.1.0-dev |
| Linux CLI build/version | PASS | curlew 0.1.0-dev |
| Go vet on variable/vault/runner/runservice | PASS | Exit 0 |
| Backlog integrity | PASS | Planned/in-progress lifecycle consistent |
| Documentation front-door/layout checks | PASS | Named tests passed |
| Module integrity | PASS | All modules verified |
| macOS cross-compilation | PASS before final POSIX drain refinement | Not runtime evidence; final macOS execution NOT RUN |
| Authoritative scripts/ci-local.sh --go | FAIL | README quickstart discovery cannot compile syscall.Kill on Windows |
| Full go test ./..., race, golangci-lint | NOT PASS | Compiler blocker; C compiler/linter unavailable |
| Windows arm64 / extracted release ZIP / browser UI | NOT RUN | M30-005 scope, not established here |

Commands used (native Windows selected the installed Go executable explicitly):

```text
go test ./internal/variable/ ./internal/vault/ -count=1 -coverprofile=<temp> -timeout=600s
go test ./internal/runservice/ ./internal/runner/ -run '^(TestRun_VaultResolution|TestRealTeamProviderFactoryConstructsConfiguredProvider|TestRealTeamProviderFactoryRejectsUnknownProvider|TestRun_TeamSecrets|TestRunFromCommand|Test.*Sensitive.*|Test.*Redact.*)$' -count=1
go test ./internal/runner/ -run '^TestResolvedSecretsSurviveScopeFailure$' -count=1
go vet ./internal/variable/ ./internal/vault/ ./internal/runner/ ./internal/runservice/
go test ./internal/backlog/ -run '^TestBacklog_repository_is_consistent$' -count=1
go build -o <temp>/curlew.exe ./cmd/curlew
bash ./scripts/ci-local.sh --go
```

Linux used its own Go compiler for full suites. Earlier cross-built test binaries
successfully exercised shell/process behavior but could not build helpers using a
Windows GOROOT. Initial full Linux attempts timed out while compiling helpers or
the cold CLI; these were NOT passes. Removing fixture VCS stamping and completing
the initial CLI build allowed the final full affected-package runs above to pass.
An earlier variable-suite random collision failed once and passed ten isolated
reruns; the final complete suite also passed. No unrelated random test was changed.

## Observable

[TestProviderNativeCommands](../../internal/vault/command_integration_test.go)
builds the real CLI and local provider executables, initializes disposable projects,
and runs a collection containing a sensitive command value plus a vault value.

- Project HashiCorp and shared AWS/Azure cases exit 0 and reach loopback with exact
  values, including Unicode and whitespace. Exact provider argv are asserted.
- JSON, verbose stderr, and request/response events remain valid and redact both
  values, including values echoed by the server. Nested JSON strings are inspected.
- All five providers also execute local .exe and compatible .cmd stubs, covering
  Fetch, BulkFetch, ValidateConfig, AppRole reuse, and both 1Password forms.
- No real provider executable can be found through the isolated test PATH.

## Behavior And DoD Status

| Requirement | Status |
| --- | --- |
| Native command shell and explicit syntax | PASS: system PowerShell on Windows, /bin/sh on POSIX |
| Quotes/spaces/Unicode/metacharacters | PASS for native executables and documented batch subset; arbitrary batch quoting NOT COMPLETE |
| Output encoding and CRLF | PASS: UTF-8 validation, platform newline rules, spaces and standalone CR retained |
| Nonzero/missing/cancel/deadline | PASS: typed causes and safe public diagnostics; 30-second cap |
| Descendant cleanup | PASS on Windows and Linux tests; POSIX children deliberately leaving the group are outside containment |
| Secret redaction | PASS: native failure diagnostics, provider JSON errors, early summary, real CLI JSON/events |
| Failing tests precede implementation | PASS: local RED/GREEN commits retained |
| POSIX regressions | PASS for changed packages on Linux; macOS execution NOT RUN |
| Required authoritative gate | FAIL: see pre-audit report |
| Documentation | Updated with actual contract and limits |

## Remaining Work

1. [ ] Resolve the pre-existing native Windows gate prerequisites under M30-005
   or establish an explicitly approved equivalent; do not silently skip them.
2. [ ] Resolve or explicitly accept the task's batch compatibility limits: embedded
   double quotes/controls, CALL reparsing, delayed expansion, and length limits.
3. [ ] Run the full gate, race, and linter; repeat the official review/verify cycle.
4. [ ] Add fault-injection evidence for Windows job/thread setup errors and actual
   Windows console Ctrl+C delivery; existing tests prove context cancellation.

Task completion and release readiness are not inferred from scoped coverage.
See [pre-audit result](../reviews/M30-004-review.md).