# Verification Report: M30-004

Date: 2026-09-16. Branch: feature/M30-004-windows-commands.
Verdict: **BLOCKED / NOT COMPLETE**. Task remains `in_progress`.

## Summary

Native command execution and all five CLI-backed vault providers are implemented
and pass scoped Windows and Linux tests. The established workflow cannot reach
review PASS or done: the authoritative gate fails in pre-existing Windows CLI
tests. A separate fully equipped Linux gate attempt reached full tests and found
one task documentation error (now fixed) plus live-network test dependencies.
The initial missing batch-quote support has been implemented and verified below.
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
| Quotes/spaces/Unicode/metacharacters | PASS for native and batch provider launchers, including embedded quotes, backslashes, literal expansion syntax and injection-shaped arguments |
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
2. [x] Implement missing batch quote/control handling and verify supported provider
  launcher patterns, including delayed-expansion bootstrap and real Python argv.
3. [ ] Run the full gate, race, and linter; repeat the official review/verify cycle.
4. [x] Verify actual Windows console Ctrl+C delivery and child cleanup; startup
  failure and missing-thread tests also pass. Exhaustive API fault injection is
  additional hardening, not claimed as performed.

Task completion and release readiness are not inferred from scoped coverage.
See [pre-audit result](../reviews/M30-004-review.md).

## Resumed Windows Verification

The quote-rejection RED commit is `36a3bd5`; `2829eaa` fixes encoding across both
CMD parsing passes. Provider quote-rejection assertions were replaced with exact
argv and exact secret-value assertions. No requirement was removed from the task.

| Check | Result | Evidence |
| --- | --- | --- |
| Final complete native variable/vault suites after batch and console changes | PASS | variable 551.573s, 95.9%; vault 379.089s, 97.8%; combined command exit 0 |
| Batch .cmd/.bat/.CMD and all provider integration cases | PASS | variable 263.812s; vault 336.938s |
| Expanded quote/backslash/collision matrix | PASS | variable 125.404s |
| Azure MSI/ZIP and Google launcher patterns with real Python | PASS | vault 22.402s; subsequent linked-doc run 37.643s |
| Real Windows isolated Ctrl+C | PASS twice | 21.45s and 19.79s; CLI exit 5, context canceled, all four processes exited before fallback cleanup |
| Failed process start and missing primary-thread error | PASS | 6.895s, no process/output from failed start |
| Go vet on affected packages | PASS | Exit 0 |
| Prose inventory gate previously failing at MANUAL batch contract | PASS after repair | 1.237s; contract linked to actual launcher tests |
| Complete UI plus native CLI build/startup after quote fix | PASS | Vite build followed by go build; curlew 0.1.0-dev |
| Pinned golangci-lint on complete task-changed files | PASS | v2.11.2 with Go 1.26.8; --new-from-rev=fbde6b5 --whole-files; zero issues |
| Final focused rerun after lint fixes | PASS | variable 29.467s; vault 120.016s; real CLI, Python launchers, Windows Ctrl+C and helper compiler lookup |

The real Windows console test retains handles for broker, CLI, PowerShell and a
Ctrl+C-immune native child. It deliberately presents a foreign-process allowlist
first and asserts that the broker refuses to signal. Only an exact match to its
private console permits CTRL_C_EVENT. Both command children must be gone before
closing the test's fallback Job Object; no request is sent before resolution.

Launcher fixture provenance:
- Azure CLI 2.90.0 build_scripts/windows/scripts/az_msi.cmd and az_zip.cmd:
  exact IF/interpreter forwarding bodies, with local fake azure.cli modules in an
  offline venv. No Azure CLI service calls or credentials.
- Google Cloud CLI 585.0.0: reduced bootstrap retaining interpreter probe,
  delayed-expansion enable/disable, final %* forwarding and exit propagation.
  Source archive google-cloud-sdk-585.0.0-windows-x86_64-bundled-python.zip,
  SHA256 `42ab5eb7ccc4c217f96afcc17f427f177b58af5334854e358d2b909115c2fd79`.
  This tests the relevant launcher pattern, not every SDK bootstrap branch.

The earlier blanket warnings about quotes and delayed expansion are superseded.
Actual provider launchers do not CALL-reparse user arguments; Google disables
delayed expansion before forwarding. Tests compare both Go and Python argv rather
than assuming their parsers are identical. CR/LF and NUL validation and the CMD
length budget remain explicit platform input constraints.

## Additional Full Gate Attempt

The unmodified scripts/ci-local.sh --go ran on a detached LF Ubuntu clone of
`2829eaa`, with its origin pointing only at the local checkout. It exited 1 after
11m27s at full go test, not at a missing-tool step. Checksum-verified temporary
tools: Go 1.27.1, Node 22.23.2, npm 10.9.8, golangci-lint 2.11.2, GoReleaser 2.17.1,
jq 1.8.1 and Zig 0.15.2 for Cgo. All disposable tools/checkouts were removed.

Build, fuzz corpora, backlog, layout, front-door, README quickstart and stream
discipline passed. POSIX variable and vault tests passed within the full run.
The M30-004 prose-inventory error is now fixed. Remaining failures were:

- TestRunDoesNotFailOnTelemetryError depends on GET https://example.com and failed
  with that live request blocked. No request to the live endpoint was authorized.
- DNS classification initially saw the refusing proxy; a focused rerun exempting
  only the reserved nonexistent.invalid host passed all three classification cases.

Race, coverage, lint, smoke, release and authenticated README installation were
not reached. The full gate has not been rerun to PASS. Its smoke fixture also has
a public HTTPBin dependency; the install stage requires authenticated access.
Raw logs were retained under the local TEMP directory
`curlew-M30-004-2829eaa-validation`, outside the source checkout.

## Lint Toolchain Check

The pinned v2.11.2 prebuilt linter targets Go 1.26 and cannot read Go 1.27 export
data; rebuilding the same linter with Go 1.27 did not update its internal reader.
It was run with a checksum-verified temporary Go 1.26.8 toolchain instead. This
did not change the installed Go or repository module/linter versions.

New test/helper errcheck, deprecated runtime.GOROOT lookup and formatting findings
were fixed. Helper builds now locate the active compiler on PATH. Complete changed
files in variable/vault/consolehelper pass lint. An unrestricted run also reports
two pre-existing gofumpt findings in internal/variable/dynamic.go and
dynamic_helpers.go; those files were not reformatted as part of this task.

## YAML Variable Resolution Follow-up (2026-09-18)

The installed CLI left dynamic functions inside named project variables literal.
A focused regression reproduced the invoice ID/date failure, including nested
date aliases and quoted values. Interpolation now evaluates the original named
expression at request time with bounded recursion and the existing function cache.
Request-scope copies preserve the expression. No invoice generation belongs in
the external PowerShell runner; that workaround was removed.

| Check | Result |
| --- | --- |
| ID/date/escaping regression before implementation | FAIL as expected, literal functions reached interpolation output |
| Dynamic variable, override, recursion, sensitivity, and literal-return tests | PASS |
| Existing interpolation/function/redaction and fuzz-seed checks | PASS |
| Native Windows race check for interpolation/function scope | PASS |
| Real CLI with project YAML, JSON body file and explicit overrides | PASS |
| Pinned linter for this fix, complete changed files | PASS, 0 issues |
| Installed executable with the external Ingestion Test project | PASS against loopback only; YAML generated IDs and UTC dates without runtime value overrides |

The external project retains its existing customer and numeric invoice data.
No Azure secret or business endpoint was accessed. This focused repair does not
close the outstanding full-gate requirements above. The user subsequently requested
local-main consolidation; see [the handoff](windows-repair-plan.md#consolidation-handoff-2026-09-18).

Consolidation rerun: variable regressions and backlog integrity PASS. The final
CLI regression now FAILS at its subsequently added `docs.Prose` assertion because
the specification text is not recognized as a prose claim. Its earlier runtime
PASS remains historical evidence, not a PASS for this final test revision.

## Editor Diagnostics Follow-up (2026-09-18)

Fixed the 14 current-source entries from the user's Problems screenshot: four
`fmt.Appendf` suggestions, four buffer-write suggestions, one error-comparison
warning, and five unused parameters. Error identity is checked separately from
the other result fields. Removed private-helper arguments were updated at every
caller, including the Unix-only quickstart test. No diagnostic settings changed.

| Check | Result |
| --- | --- |
| Focused binary-events, Markdown, raw HTTP golden bytes, WebSocket, session, retention, extraction, GraphQL and agent-guide tests | PASS |
| `gopls check -severity=info` on the changed native files | PASS, no information, warnings or errors |
| Native `go build ./...` | PASS, Go 1.27.1 |
| Full uncapped `golangci-lint run` | PASS, 0 issues; v2.11.2 with Go 1.26.8 |
| CLI test cross-compilation for linux/amd64 and darwin/amd64 | PASS; compilation only, not runtime evidence |

Optional hint-level modernization suggestions are outside this cleanup. The
known documentation-claim test failure and full-gate requirements above remain
open. The user requested committing this cleanup and merging it into local main.
No push or task completion is included.

## Combined Verification Follow-up (2026-09-18)

Base: `39d14ad`, with the following repairs, requested for commit on 2026-09-21. The earlier
documentation-claim failure is resolved by expressing the existing contract as
"must be evaluated"; the prose assertion is retained. Windows precedence fixtures
now use PowerShell and a local `.cmd` provider stub. Fast-loopback timing tests
accept zero at the host clock resolution, with new exact phase-arithmetic cases
and the existing delayed-body regression retained. One Unix test cleanup now
explicitly discards its close error. Go sources now check out with LF under
`core.autocrlf=true`; the new regression failed before the attribute rule and
passes after it, with raw golden transcripts still excluded from conversion.
The watch integration test no longer limits reruns based on OS event-delivery
speed; exact coalescing remains covered by the direct debounce-burst test.
The editor-launch test now waits for its helper to report the exact file/line
argument after leaving the temporary working directory, avoiding Windows cleanup
failure while retaining asynchronous editor launch.
No production implementation changed.

| Check | Result |
| --- | --- |
| Real CLI YAML dynamic values and overrides | PASS, native Windows and Linux |
| Native UI check, lint, unit tests, build | PASS; svelte-check 0 errors/warnings, 13 test files |
| Native full ordinary tests, first run | FAIL on POSIX-only precedence fixtures, strictly positive fast-loopback timing assertions, and a filesystem debounce-count assumption; all failing groups pass on focused reruns |
| Native full tests with coverage, final run | PASS, 87.5%; no failed test events |
| Native race | All packages passed across the full run and a complete UI-server package rerun; the initial run failed only editor-helper directory cleanup, with no data race reported |
| Native loopback smoke | PASS |
| Windows amd64 snapshot ZIP on Windows | PASS: SHA-256, version, init, validate, embedded JavaScript/CSS, authenticated collection discovery; not clean-host acceptance |
| LF checkout policy and native lint | PASS: RED/GREEN checkout regression, uncapped lint 0 issues |
| Linux full ordinary tests and race | PASS |
| Linux total coverage | PASS, 86.6% |
| Linux full lint and smoke | PASS after Unix-only errcheck repair |
| Linux release config, host snapshot, embedded UI, six archives/version/checksums | PASS |
| README remote installation stage | BLOCKED: release not found; source install and clone cannot authenticate to GitHub |
| Local Mudflat collections, parallel rendezvous, gaps, redaction, OpenAPI, curl cross-check and ledger | PASS, unchanged stages run separately after the install blocker |
| Final test-only repairs on Linux | PASS for YAML/precedence, timing arithmetic, process containment and uncapped lint |
| Final watch/timing race checks on Linux | PASS, complete packages |
| Final editor-helper cleanup | PASS: native regression three times, complete UI-server race suites on Windows and Linux, final Linux lint 0 issues |

Evidence: `%TEMP%/curlew-windows-verification-20260918-152628.log`,
`%TEMP%/curlew-windows-smoke-20260918.log`,
`%TEMP%/curlew-native-final-checks-20260918.log`,
`%TEMP%/curlew-native-coverage-final-20260918.jsonl`,
`%TEMP%/curlew-native-final-coverage-20260918.out`, and in WSL
`~/.cache/curlew-ci-39d14ad-repaired.log` plus
`~/.cache/curlew-dogfood-39d14ad.log`.

The authoritative gate remains blocked, not passed with a skipped stage.
The native verifier's original invocation also failed; the repaired native test,
race, coverage, lint and smoke checks were verified separately, not represented
as one uninterrupted script PASS.
GitHub authentication and release visibility must be resolved before rerunning
the README install checks and full gate. Clean-host Windows acceptance remains
M30-005 work. No paid service, real vault, or business API was called. The user
authorized a local commit only; no merge, push, task closure, or release
publication is included in this follow-up.

## Installed Windows Runtime Follow-up (2026-09-21)

Scope: make the existing app usable on this Windows amd64 machine. No GitHub,
real vault, business API, or paid service calls. Local repair branch:
`fix/M30-004-dynamic-assertion-cache`, based on `1f03b70`. After runtime verification,
the user requested a local commit and merge into `main`; no push is authorized.

The installed binary reproduced a request-cache bug: invoice ID interpolation
was consistent in the sent header and JSON body, but response assertions generated
new values. The real-binary invoice regression failed with both dynamic and
overridden invoice IDs (UUID header assertions make the failure independent of
clock resolution). `InterpolateRequest` now retains the function cache through
response handling; the next request resets it. Unit coverage checks fresh values
and independent snapshot scopes. A nearby date fixture now freezes its timestamp
function as well as its date-add clock; previously it only passed on September 18.

| Check | Result |
| --- | --- |
| Existing API project validation | PASS before and after replacement; no project configuration changed |
| Installed command discovery | Saved user PATH already correct; refreshed this terminal's stale process PATH |
| Real-binary invoice regression | RED before repair, GREEN for both override cases afterward |
| Request-cache lifecycle and snapshot isolation | PASS |
| Complete requtil, runner, parallel and runservice packages | PASS |
| Dynamic scope and registry tests | PASS after fixing the frozen-clock fixture |
| Whole variable package attempt | Not a full pass: exceeded the five-minute package budget in Windows batch tests; interrupted `.cmd/long_argument` case passes separately |
| Native affected-package lint | PASS, zero issues with Go 1.26.8 and ten-minute limit; first attempt exhausted five-minute limit |
| Frontend build and native executable build | PASS |
| Copied invoice YAML/JSON on loopback | PASS, two requests in sequential and parallel modes; stable ID, dates, numeric amounts |
| JSON, NDJSON events and Markdown reports | PASS |
| Browser run, request details and history | PASS, two requests and zero failures; JavaScript and CSS return HTTP 200 |
| Actual project UI discovery | PASS, eight collections and 36 request entries; no real requests executed |
| Disposable server cleanup | PASS, both test listeners stopped; actual project UI left running |

Installed `C:\tools\curlew\curlew.exe` matches the verified candidate SHA-256:
`6EF1F6D776E0B45DBDFECC6093A059CB63881ABC95728E1D924540BDA7A5D32F`.
The previous binary is retained as `C:\tools\curlew\curlew-before-20260921.exe`.
Both report `0.1.0-dev`; the hash distinguishes this local build. The candidate
contains the complete embedded UI. Only the generated checkout index was restored
after building; no generated assets are included in the source patch.

Loopback evidence remains under `%LOCALAPPDATA%\Temp\Curlew Local Check 20260921`
(events, Markdown reports, copied configuration and candidate executable).
The Python fixture logged connection resets after completed HTTP responses when
clients closed keep-alive sockets; all asserted requests passed.

This is development-host runtime evidence, not clean-host, arm64, full race-suite,
or uninterrupted authoritative-gate acceptance. Existing task statuses are unchanged.
The real vault mapping and live API authentication remain unverified.

### Local Merge Checks (2026-09-21)

The pre-commit native build passed. Whole-repository lint passed with zero issues
using Go 1.26.8 and golangci-lint 2.11.2. The full native Go test run used
`go test -json -p 4 -timeout=30m ./...`; its retained log is
`%TEMP%\curlew-premerge-tests-20260921-105054.jsonl`.
All other packages completed successfully, but the CLI package exhausted the
30-minute budget during the dirty-tag build fixture. No individual test failed.
Its first 490 top-level tests had completed; the remaining 13 were selected from
the test list and rerun without repeating completed tests. All 13 passed in
335 seconds, including both tagged-build cases and the Windows end-to-end smoke.
The completion log is
`%TEMP%\curlew-premerge-cli-remainder-20260921-112622.jsonl`.
This is complete ordinary-test evidence across two runs, not an uninterrupted
`go test ./...` pass. The original timeout remains recorded.
These checks do not replace the broader authoritative gate or close the tasks.