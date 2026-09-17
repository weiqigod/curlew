# Windows Repair Plan

Date: 2026-09-17. Status: proposed execution plan; implementation not started here.

## Goal

Finish M30-004 with a passing required gate, then complete M30-005 with native
Windows evidence. Passing selected packages is not a full gate pass. A Windows
cross-build or a WSL test run is not native Windows verification.

This is the repair queue, not a replacement for the task lifecycle. No task status,
dependency, source code or support claim is changed by writing this plan.

## Baseline

| Area | Recorded result | Treatment |
| --- | --- | --- |
| Command variables and five vault providers | Native Windows and Linux scoped suites pass, including batch quotes and redaction | Keep as regressions; do not reopen the repaired quote limitation |
| Windows command interruption | Isolated real Ctrl+C test passes; children exit | Reuse the isolation pattern for other cancellation tests |
| Native CLI test package | Compilation fails at Unix-only `syscall.Kill` | First repair |
| Full Linux gate | Stops in tests; telemetry test calls live example.com | Replace public test dependencies, then rerun |
| Full race, coverage, lint, smoke and release checks | No complete passing gate recorded | Required work, not assumed failures or passes |
| Windows plugins, editor integration and wider acceptance | Original investigation findings or unverified behavior | Reproduce and repair under M30-005 |

Baseline branch: `feature/M30-004-windows-commands`, last recorded commit
`f78238a`. Recheck HEAD, worktree and current source before executing each item.
Older report statements about missing Go and unsupported batch quotes are historical.

## Task Ordering

1. [ ] Register an independent Windows foundation task for R1-R6, depending on
   M30-003, not on M30-004 or M30-005. Assign an unused ID and record the blocker
   against M30-004 using the task index's existing format. Move the known W2-W8
   repairs into that scope; M30-005 retains end-to-end acceptance and any new
   findings. Product repairs must precede the full native gate, not depend on it.
2. [ ] Run that task through `/plan`, `/execute`, `/review`, `/improve` if needed,
   and `/verify`. Obtain approval before changing task scope or the checkout.
3. [ ] Complete R7: repeat M30-004 review and verification against the repaired gate.
4. [ ] Plan and execute M30-005 for R8-R9 after its dependency is done. Link the
   foundation evidence rather than duplicating completed repairs.

Do not reset M30-004 to backlog or mark M30-005 planned while its dependency is
unfinished. This companion plan does not claim the formal `/plan` stage ran.

## Execution Rules

1. [ ] Work one item at a time. Read its owning code and nearby tests, capture RED,
   make the smallest repair, and rerun the same check before expanding scope.
2. [ ] Use loopback servers, offline provider/plugin stubs and disposable directories.
   No cloud calls, public load tests, paid APIs or hosted CI without explicit approval.
3. [ ] Never broadcast console signals to VS Code, kill an unknown port owner, change
   a user's global Git settings, or modify their real secrets or agent installation.
4. [ ] Record commit, OS/architecture, tool versions, exact command, exit code and
   durable evidence in the owning task's verification report. Use PASS, FAIL,
   BLOCKED or NOT RUN; keep existing completed checkboxes and append new evidence.
5. [ ] A missing required tool or an unavailable required check blocks the relevant
   claim. Do not obtain green by weakening assertions, adding blanket skips or
   removing gate stages. Get explicit agreement for any change to gate policy.
6. [ ] Local implementation follows repository TDD and task conventions. Pushes,
   PRs, shared-system changes and any real-world cost require separate approval.

## Repair Queue

### R1: Compile And Launch Native Tests

**Owner:** Windows foundation. **Depends on:** task registration. **Covers:** W3-W4.

1. [ ] Capture `go test ./cmd/curlew/ -run '^$'` failing on native Windows.
2. [ ] Separate platform-specific perf interruption tests from common tests in
   [perf_test.go](../../cmd/curlew/perf_test.go). Preserve actual signal delivery:
   POSIX SIGINT and an isolated Windows console, never a signal to the test host.
   Reuse the safety approach in [interrupt_windows_test.go](../../internal/vault/interrupt_windows_test.go)
   and [consolehelper](../../testdata/consolehelper/main.go) where suitable.
3. [ ] Add native regressions for executable suffixes in [main_test.go](../../cmd/curlew/main_test.go),
   [version_test.go](../../cmd/curlew/version_test.go), [plugin integration tests](../../internal/plugin/integration_test.go)
   and [UI E2E setup](../../ui/tests/e2e/global-setup.ts). Use `.exe` on Windows;
   preserve other platforms. Replace shell-string Go builds with argument arrays.
4. [ ] Check compilation and real fixture startup from spaced and Unicode paths.
   Verify perf receives a loopback request, interrupt cancels in-flight work,
   returns 130 and leaves no owned processes. Ordinary command-resolution exit 5
   remains a separate contract. Run matching POSIX regressions.

**Done when:** the native CLI test package compiles and the named real-binary and
perf cancellation tests pass without skipping Windows coverage.

### R2: Remove Public Test Dependencies

**Owner:** Windows foundation. **Depends on:** R1 for native CLI execution.

1. [ ] Capture the telemetry failure with external requests blocked. In
   [telemetry_run_test.go](../../cmd/curlew/telemetry_run_test.go), use a disposable
   collection targeting `httptest` for all three run tests. Preserve the shared
   [minimal fixture](../../cmd/curlew/testdata/minimal.yaml) until its other callers
   have been reviewed.
2. [ ] First require exact successful exit 0, an observed loopback request and no
   telemetry noise when the sink's parent is a regular file. Keep enabled-event
   and disabled-event assertions. Do not merely broaden accepted exit codes.
3. [ ] Replace the known public HTTPBin dependency in [smoke/run.sh](../../smoke/run.sh)
   with the existing local fixture or a minimal local equivalent. Inspect the
   gate's reachable network calls before running it; separate dependency downloads
   and authenticated artifact retrieval from application test traffic.
4. [ ] Keep DNS error classification distinct from proxy refusal. Isolate relevant
   proxy variables for the reserved invalid-domain case and restore them afterward.
   Run the affected checks on Windows and POSIX with external test traffic blocked.

**Done when:** telemetry and smoke assertions pass against controlled local
responses without reaching example.com, HTTPBin or any provider account.

### R3: Fix Checkout And Test Assumptions

**Owner:** Windows foundation. **Depends on:** R1; execute after R2. **Covers:** W5 and test parts of W6.

1. [ ] Reproduce Markdown golden byte differences in a disposable checkout with
   Windows checkout conversion enabled. Add targeted LF rules to
   [.gitattributes](../../.gitattributes) for deterministic inputs, shell scripts,
   goldens and embedded templates. Preserve binary wire captures. Do not regenerate
   goldens or normalize the whole worktree to conceal the mismatch.
2. [ ] Verify fresh checkouts produce identical embedded template hashes and golden
   bytes on Windows and POSIX, independent of global `core.autocrlf` settings.
3. [ ] Run and repair the remaining known assumptions: shebang provider stubs in
   [doc_precedence_test.go](../../cmd/curlew/doc_precedence_test.go), directory-mode
   failure tests in [scaffold_test.go](../../internal/scaffold/scaffold_test.go)
   and [writer_test.go](../../internal/output/markdown/writer_test.go), and the
   `/dev/stdout` case in [main_test.go](../../cmd/curlew/main_test.go).
4. [ ] Use deterministic portable failures where the behavior is portable; retain
   separate platform tests for genuinely OS-specific contracts. Label Bash/Docker
   script-contract tests in [ci_local_test.go](../../cmd/curlew/ci_local_test.go)
   explicitly and run them in the appropriate gate, not against a developer stack.
   Check [UI teardown](../../ui/tests/e2e/global-teardown.ts) separately for owned
   process cleanup versus actual graceful signal delivery.

**Done when:** relevant tests pass in fresh native Windows and POSIX checkouts;
every platform exclusion has a reason and an identified replacement or gate.

### R4: Restore Native Plugin Behavior

**Owner:** Windows foundation. **Depends on:** R1; execute after R3. **Covers:** W2.

1. [ ] Write RED tests showing [plugin discovery](../../internal/plugin/discover.go)
   rejects a real Windows executable. Cover directory and explicit-path discovery,
   non-executable files, spaced/Unicode paths and the platform list separator.
2. [ ] Implement an explicit Windows executable policy without weakening POSIX
   execute-bit checks. Decide supported launcher types from the plugin contract;
   do not accidentally accept every regular file.
3. [ ] Exercise startup, protocol failures, shutdown and descendant cleanup with
   local compiled plugins. Investigate [Windows signaling](../../internal/plugin/signal_windows.go)
   and [host cleanup](../../internal/plugin/host.go); a fallback direct-process kill
   alone is not proof of graceful shutdown or descendant cleanup.

**Done when:** real plugins are discovered and run natively with verified failure
and lifecycle behavior; POSIX plugin tests still pass.

### R5: Repair Editor And UI Paths

**Owner:** Windows foundation. **Depends on:** R1; execute after R4. **Covers:** W7-W8.

1. [ ] Reproduce `code.cmd` fallback and quoted executable paths in
   [api_open.go](../../internal/uiserver/api_open.go). Test exact argv through local
   stubs, including quotes and metacharacters, without opening an interactive editor.
   Reuse proven Windows launch handling only where its lifecycle fits editor launch.
2. [ ] Fix argument parsing and deliberate batch launch. Do not attach a persistent
   editor to a short request's cancellation or kill it after a successful launcher exit.
3. [ ] Define display-path normalization at the appropriate UI/API boundary. Add
   native-separator cases to [RunSplitButton.svelte](../../ui/src/lib/components/chrome/RunSplitButton.svelte)
   and other affected labels. Preserve real filesystem paths for execution/watch.
4. [ ] Run component/API tests and native browser checks for discovery, execution,
   history, editor launch, browser launch and shutdown, including spaced paths.

**Done when:** labels show correct basenames, selected files execute correctly and
default/custom editor launch preserves arguments on Windows and POSIX.

### R6: Make Verification Repeatable

**Owner:** Windows foundation. **Depends on:** R1-R5. **Covers:** gate parts of W6.

1. [ ] Establish a supported Go/linter combination. Installed Go 1.27.1 works for
   build/tests; pinned golangci-lint 2.11.2 cannot read its export data. A temporary
   Go 1.26.8 worked for changed-file lint. Choose and document a reproducible
   compatible combination before whole-repository lint; do not silently replace
   the installed Go or claim changed-file lint equals whole-repository lint.
2. [ ] Provision or document required native C/race tools and Windows shell/test
   prerequisites, including Python for launcher tests. Check prerequisites up
   front with actionable errors. Keep these separate from runtime requirements.
3. [ ] Add a PowerShell developer-test entry point and a loopback-only app smoke
   entry point. Map every stage of [ci-local.sh](../../scripts/ci-local.sh) to
   native, POSIX or artifact verification. Preserve the authoritative gate; do not
   declare the new entry point equivalent without explicit agreement. Regression
   tests must prove failed commands and missing required tools fail the entry point.
4. [ ] Make smoke use temporary paths, dynamic ports, exact exit assertions and
   cleanup of only its own processes, including on failure. Repair the existing
   POSIX smoke's fixed-port kill and shared temporary-file cleanup too; making
   only the new PowerShell entry point safe does not make the full gate safe.
5. [ ] Reproduce and fix the recorded whole-file formatting findings in
   [dynamic.go](../../internal/variable/dynamic.go) and
   [dynamic_helpers.go](../../internal/variable/dynamic_helpers.go) under the agreed
   linter. Handle other newly exposed failures as separate RED/GREEN items.
6. [ ] Run full native tests, applicable race/coverage/lint and native smoke.
   Run the POSIX gate in auto-scope mode on the final committed revision: UI changes
   require UI checks that `--go` omits. Preserve the intended comparison base in
   any disposable checkout; preflight the script's Git fetch and artifact access.
   Never fast-forward the user's main or switch their checkout without approval.

Record all of these gate groups, not just `go test`:

| Gate group | Required evidence |
| --- | --- |
| Build and repository guards | Embedded UI, Go build, backlog, fuzz corpora, layout, front-door files, executable README quickstart and stream discipline |
| Tests and analysis | Full tests, race, coverage >= 80%, whole-repository lint, telemetry import guard, smoke-failure guard and stubbed signing-key tests |
| App smoke | Loopback-only behavior and owned-process cleanup; POSIX and native results separate |
| Release artifacts | GoReleaser version/check, snapshot, injected version, embedded assets/discovery and all six archive checks; no publish |
| README installation | Actual install-command checks; `gh` and authenticated repository access where required |
| Mudflat dogfood | Local build/run, parallel rendezvous, expected failures, redaction, OpenAPI, curl crosscheck and ledger |
| Changed UI | npm install, Svelte check, lint, unit tests and build; native local-UI browser acceptance recorded separately |
| Frozen backend/web/stack | Scope detection recorded; run only when those paths change, not merely because Windows work exists |

Enter any required credentials directly in the terminal, never chat. Artifact
downloads are not application test traffic; keep access and cost approval explicit.

**Done when:** required gates pass with retained stage-by-stage results. Missing
tools, private-artifact access or unsupported race targets are explicit blockers,
not green skips. A new product failure discovered here joins the foundation queue
before completion; do not defer a gate blocker to dependent M30-005.

### R7: Close M30-004 Properly

**Owner:** M30-004. **Depends on:** R6 and completed foundation workflow.

1. [ ] Rerun full native variable/vault suites, POSIX regressions, runner redaction
   tests, provider launcher argv cases and isolated Windows interruption tests.
2. [ ] Repeat the official review after the full pre-audit gate passes. Repair all
   findings and repeat review; do not replace the existing FAIL with a scoped-test PASS.
3. [ ] Execute `/verify`, update the changelog and task evidence, and transition
   to done only when every requirement passes. Pause before any push, PR or merge.

**Done when:** M30-004 has an actual review PASS and complete verification report.

### R8: Complete Windows Acceptance

**Owner:** M30-005. **Depends on:** R7 and approved M30-005 plan; reuse R1-R6 evidence.

1. [ ] Turn the investigation's acceptance checklist into a per-feature evidence
   matrix in the M30-005 verification report. Separate confirmed failures from
   risk probes; add a regression before repairing any newly reproduced defect.
2. [ ] Cover run/validate, environments and overrides, request bodies/files, local
   JSON Schema/OpenAPI, documented exit codes, TLS/proxy and WebSocket behavior.
3. [ ] Cover JSON/Markdown/events/HTML/JUnit output, repeated writes, history
   reload/pruning, auth cache refresh and concurrent file access using local data.
4. [ ] Cover UI/watch save/restart/cancel/shutdown and perf cancellation with real
   native evidence. Check short/long/spaced/Unicode paths, drive-letter case,
   CRLF, reserved report names, junctions and available alternate-drive/UNC cases.
   Do not rewrite atomic replacement or models without a demonstrated failure.
5. [ ] Verify local secret/cache ACL expectations; POSIX `0600` is not a Windows
   ACL guarantee. Record unavailable cross-user or filesystem scenarios explicitly.
6. [ ] Install/update skills in disposable homes, preserve local edits and compare
   source-built/release-built template hashes. Verify discovery in each available
   agent host; record unavailable hosts and symlink privileges as NOT RUN/BLOCKED.

**Done when:** every applicable acceptance row has native evidence or an explicitly
agreed support limitation. Unknown results cannot become PASS through documentation.

### R9: Verify Packaged Delivery

**Owner:** M30-005. **Depends on:** R8.

1. [ ] Build the full UI before Go and assert actual embedded JavaScript/CSS, not
   just the fallback HTML. Verify [release configuration](../../.goreleaser.yaml)
   includes the Windows instructions and correct archive contents.
2. [ ] Follow [docs/WINDOWS.md](../../docs/WINDOWS.md) on a clean native environment.
   Repeat core CLI/UI/skill smoke from an extracted ZIP outside the checkout with
   Go, Node, Docker and backend services unavailable at runtime.
3. [ ] Record source-build and ZIP results separately by Windows version and CPU
   architecture. Obtain matching arm64 hardware before claiming native arm64 PASS;
   otherwise explicitly leave arm64 unverified and limit the published claim.
4. [ ] Rerun all applicable gates on the final revision, perform official review
   and verification, update documentation/changelog, then close M30-005 only when
   its DoD is satisfied. Keep M29-001 billing/CI trigger decisions separate.

**Done when:** the documented support claim matches clean-host and packaged
native evidence, with no failed required gates concealed as limitations.

## Evidence And Handoff

For each R-item, record this row in its owning task's verification report:

| Item | Commit | Host / architecture / tools | Command | Exit | Result | Evidence / next action |
| --- | --- | --- | --- | --- | --- | --- |
| R1-R9 | Pending | Pending | Pending | Pending | NOT RUN under this plan | Link retained output and next unchecked step |

After a failed gate, stop at the failing stage, record the exact test/error,
create the next bounded repair item and rerun that stage. Only rerun the full gate
after focused checks pass. Keep later stages NOT RUN until actually executed.

## References

- [Windows investigation and W1-W8 evidence](../reviews/M30-005/windows-investigation.md)
- [M30-004 verification, including final focused passes and full-gate blockers](M30-004-verified.md)
- [M30-004 pre-audit review](../reviews/M30-004-review.md)
- [M30-004 task](../tasks/M30-004.yaml) and [M30-005 task](../tasks/M30-005.yaml)
- [Current task index](../backlog.yaml)