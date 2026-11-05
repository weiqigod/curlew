# Native Windows Investigation

Date: 2026-09-16. Reviewed commit: `b485e27ba71e6bd01cce6a27d4e9090add93994b`.
Investigation only. No implementation, test, configuration, or task-status changes.

## Findings

**Curlew is cross-built for Windows, but is not ready to claim native Windows support.**
The work extends beyond the known command/vault gap: plugin discovery, editor
launch, test compilation, executable naming, checkout bytes, and the development
gate need attention. This does not establish that ordinary HTTP runs are broken.

Priority: P1 blocks a supported feature or meaningful native verification;
P2 breaks a narrower workflow; P3 is a presentation issue.
"Source-confirmed" means the owning code and platform contract establish the
problem, not that the Curlew executable was run. Native probes are listed separately.

| ID | Priority | Finding | Evidence |
| --- | --- | --- | --- |
| W1 | P1 | Command-backed variables and all CLI-backed vault providers depend on `/bin/sh`. | Source-confirmed; native process probe returns `ENOENT`. |
| W2 | P1 | Plugin discovery rejects normal Windows executables because it requires Unix execute bits. | Source-confirmed against Go's Windows file-mode implementation. |
| W3 | P1 | The ordinary CLI test package cannot compile for Windows: an unguarded test calls `syscall.Kill`. | Source-confirmed; no native compiler available to capture the diagnostic. |
| W4 | P1 | Test helpers build extensionless executables, then try to launch them. | Source-confirmed; native extensionless launch fails, `.exe` control succeeds. |
| W5 | P1 | Windows checkout conversion changes byte-exact Markdown goldens. | Measured CRLF checkout; comparator uses `bytes.Equal` against LF output. |
| W6 | P1 | The authoritative development gate is not a native Windows gate. | Bash/tool dependencies, further Unix-specific tests, Ubuntu-only workflows. |
| W7 | P2 | UI "open in editor" cannot directly launch the normal `code.cmd` fallback; quoted executable paths are also split incorrectly. | Source-confirmed; native direct wrapper launch returns `EINVAL`. |
| W8 | P3 | UI file-label code assumes `/`, but discovery returns native Windows separators. | Source-confirmed producer/consumer mismatch. |

### W1: Command And Vault Execution

[internal/variable/command.go](../../../internal/variable/command.go#L65)
unconditionally executes `/bin/sh -c`. Installing Git Bash does not create that
path for a native Go process. On this host Git Bash exists under Program Files,
but `/bin/sh` does not.

The providers build shell strings using POSIX escaping in
[internal/vault/aws.go](../../../internal/vault/aws.go#L22).
[internal/vault/hashicorp.go](../../../internal/vault/hashicorp.go#L63) also uses
`VAULT_ADDR=... VAULT_TOKEN=... command` syntax. Neither convention can simply be
carried over to `cmd.exe`; PowerShell is not POSIX-compatible either.

The executor also trims only `\n`, leaving a trailing `\r` from CRLF output.
It has no output-encoding contract, process-tree cleanup, or `WaitDelay` bound
for descendants retaining output handles. These need explicit tests during the
port; they are not all uniquely Windows defects.

Recommendation: separate user-authored shell commands from provider executable,
argument, and environment invocation. Define the Windows shell contract, handle
`.cmd` provider wrappers deliberately, normalize line endings without stripping
meaningful secret whitespace, and retain cancellation and redaction behavior.
Use local provider stubs, never real accounts, for portability tests.

### W2: Plugin Discovery

[internal/plugin/discover.go](../../../internal/plugin/discover.go#L104) says
Windows regular files are accepted, but `isExecutable` actually returns
`mode & 0111 != 0` on every platform. Go reports ordinary Windows files as
`0666` or `0444`, even when their extension is `.exe`.

Consequences: directory discovery silently skips plugins; an explicitly named
plugin produces a fatal "not executable" error. `chmod +x` is not a Windows fix.
The list separator is already platform-aware and should be retained.

Recommendation: implement an explicit Windows executable policy and test an
actual compiled plugin, including a directory containing non-executable files.
Also exercise shutdown: [internal/plugin/signal_windows.go](../../../internal/plugin/signal_windows.go#L8)
returns `os.Interrupt`, but Go does not implement sending that signal to another
Windows process. The [host fallback](../../../internal/plugin/host.go#L52)
eventually kills the direct process; that alone does not prove graceful shutdown
or descendant cleanup.

### W3-W4: Native Test Compilation And Launch

[cmd/curlew/perf_test.go](../../../cmd/curlew/perf_test.go#L256) calls
`syscall.Kill(syscall.Getpid(), syscall.SIGINT)` without a Windows build exclusion.
That API is unavailable on Windows. A narrow `-run` filter cannot avoid this:
Go compiles all included test files before selecting test functions.

After that is fixed, [cmd/curlew/main_test.go](../../../cmd/curlew/main_test.go#L58)
builds a file named `curlew`, without `.exe`. Go's Windows executable lookup
tries executable extensions rather than launching that extensionless file.
The same assumption appears in
[cmd/curlew/version_test.go](../../../cmd/curlew/version_test.go#L33),
[internal/plugin/integration_test.go](../../../internal/plugin/integration_test.go),
and [ui/tests/e2e/global-setup.ts](../../../ui/tests/e2e/global-setup.ts#L55).

Recommendation: platform-correct executable names across all real-binary helpers;
native console-cancellation tests instead of an unavailable Unix syscall.
Do not disable the whole integration suite to obtain a green Windows result.

The UI setup also uses `JSON.stringify(bin)` as shell quoting. A native probe
showed doubled backslashes reaching the child argument unchanged. This is an
argument-preservation defect, but not independently established as a failed Go
build: path normalization may tolerate repeated separators. Prefer an argument
array and no shell for the Go build. Test paths with spaces and metacharacters.

### W5: Checkout Bytes

[.gitattributes](../../../.gitattributes) protects only the binary wire captures
under the test API golden directory. This host has system-level
`core.autocrlf=true`. Git reports `i/lf w/crlf` for all four Markdown goldens,
the shell scripts, the fallback UI HTML, and embedded agent Markdown templates.

[internal/output/markdown/golden_test.go](../../../internal/output/markdown/golden_test.go#L57)
compares raw bytes. The renderer deliberately emits LF; a representative golden
is 593 bytes in this checkout versus 556 after removing CR from CRLF.
These tests will report mismatches unrelated to report behavior.

Recommendation: establish repository-controlled LF for deterministic text inputs,
goldens, scripts, and embedded templates while preserving the existing binary
wire-capture exception. Do not regenerate goldens to mask the conversion.
Template hashes are byte-based, so also verify skill install/update across a
Windows-built binary and a release-built binary.

Git Bash's syntax check and the isolated CRLF `set -euo pipefail` command both
passed here. CRLF is therefore **not** reported as an observed Bash failure.

### W6: Development Gate

[scripts/ci-local.sh](../../../scripts/ci-local.sh#L173) always builds UI assets,
then runs Go build/tests/race/coverage/lint and additional harnesses. Its entry
point and [scripts/build-ui.sh](../../../scripts/build-ui.sh) require Bash.
[smoke/run.sh](../../../smoke/run.sh#L37) assumes Unix tools, `/tmp`, `python3`,
fixed-port cleanup with `lsof`/`kill`, and extensionless binaries. It is not a
PowerShell smoke command. Its existing cleanup also makes it unsuitable for
blind execution on a developer machine during this investigation.

Further test obstacles remain beyond W3-W5:

| Surface | Windows assumption to remove or explicitly scope |
| --- | --- |
| [internal/variable/command_test.go](../../../internal/variable/command_test.go#L17) | `printf`, `tr`, `true`, shell syntax, and `sleep` are treated as universal. |
| [cmd/curlew/ci_local_test.go](../../../cmd/curlew/ci_local_test.go#L16) | Tests invoke Bash; the teardown test invokes a script with Git fetch and Docker teardown paths. |
| [cmd/curlew/doc_precedence_test.go](../../../cmd/curlew/doc_precedence_test.go#L154) | Provider stubs are POSIX shebang scripts. |
| [internal/scaffold/scaffold_test.go](../../../internal/scaffold/scaffold_test.go#L287) | `chmod` on a directory is expected to remove write access; Windows read-only attributes do not implement Unix directory permissions. |
| [internal/output/markdown/writer_test.go](../../../internal/output/markdown/writer_test.go#L141) | Same Unix permission assumption in an error-path test. |
| [cmd/curlew/main_test.go](../../../cmd/curlew/main_test.go#L8092) | `/dev/stdout` test is not guarded by OS. |
| [ui/tests/e2e/global-teardown.ts](../../../ui/tests/e2e/global-teardown.ts#L13) | Node's Windows signal termination does not demonstrate delivery of a graceful Unix SIGTERM handler. |

The build's `CGO_ENABLED=0` recipe is valid in principle; it is not a recipe for
the race gate, which requires a supported native C toolchain. Windows arm64
race availability must be checked separately from executable cross-build support.

[.github/workflows/go.yml](../../../.github/workflows/go.yml#L19) and the release
workflow run on Ubuntu. Re-enabling existing triggers would not add native
Windows coverage. No workflow was triggered; billing remains a separate decision.

Recommendation: a small PowerShell app smoke gate plus an explicit native
developer test entry point, with POSIX script-contract tests remaining clearly
identified. Missing mandatory dependencies must fail, not silently skip into a
Windows PASS. Reuse existing loopback fixtures and test assertions where practical.

### W7-W8: Local UI Integration

[internal/uiserver/api_open.go](../../../internal/uiserver/api_open.go#L36)
finds `code`, then directly launches it with `exec.Command`. Windows Go lookup
finds the installed `code.cmd`, but batch files require a command interpreter.
Native direct launch of this host's wrapper failed. Separately,
[buildEditorArgs](../../../internal/uiserver/api_open.go#L66) uses
`strings.Fields`, so a quoted executable path under Program Files becomes
multiple arguments. Placeholder substitution itself does preserve spaces in
the substituted file argument; that is not the broken part.

Recommendation: robust editor argument parsing and deliberate Windows wrapper
handling. Test default VS Code discovery and a quoted absolute editor executable
without actually launching an interactive editor in automated tests.

[internal/config/discovery.go](../../../internal/config/discovery.go#L27)
returns `collections\\sample.yaml` on Windows, while
[ui/src/lib/components/chrome/RunSplitButton.svelte](../../../ui/src/lib/components/chrome/RunSplitButton.svelte#L38)
and other labels use `split('/')`. They display the whole relative path instead
of its basename. Normalize API/display path representation consistently. This
does not establish that collection execution or routing fails.

## Scope And Baseline

| Item | Observed state |
| --- | --- |
| Host | Windows 11 Enterprise, x64; PowerShell 7.6.6 |
| Frontend tools | Node v24.18.1; npm 11.7.0 |
| Go | Not on PATH; absent from the three common install locations checked |
| Other tooling | Git and Python available; Git Bash installed but not on PATH; GCC, golangci-lint, and Docker not found on PATH |
| Checkout | Clean `main` before reporting; no existing executable or UI dependencies |
| Embedded UI | Tracked fallback page, not a built SPA |
| Product scope | Go CLI, embedded UI, skills, plugins, local state, and their build/test/release paths |
| Retained platform | Backend and web dashboard are frozen, separate applications; no native runtime/build verification performed for them |

No SDKs or packages were installed, binaries downloaded, cloud providers called,
live API tests run, branches switched, or shared systems changed. Temporary probe
artifacts were removed. The only retained repository addition is this report.

### Existing Foundations

- [.goreleaser.yaml](../../../.goreleaser.yaml) already targets Windows amd64 and
  arm64, disables CGO, and produces ZIP archives. Native execution remains unproven.
- [ui/vite.config.ts](../../../ui/vite.config.ts) places built assets directly in
  the Go embed directory. The npm build command itself has no POSIX shell recipe.
- [docs/WINDOWS.md](../../../docs/WINDOWS.md) correctly separates building UI first
  from building Go, uses `npm.cmd`, and avoids claiming native verification.
- Discovery generally uses `filepath`; glob matching normalizes separators.
  [internal/appdir/appdir.go](../../../internal/appdir/appdir.go#L29) delegates the
  default state location to `os.UserConfigDir`, which supports Windows.
- [internal/skillinstall/install.go](../../../internal/skillinstall/install.go)
  uses portable manifest paths and preflights conflicts. This is a good starting
  point, not evidence of actual Windows agent discovery.
- Browser launch already has a Windows branch in
  [cmd/curlew/ui.go](../../../cmd/curlew/ui.go#L118). The HTTP engine, assertions,
  and collection parser have no demonstrated blanket Windows runtime blocker.

## Recommended Work

The existing [M30-004](../../tasks/M30-004.yaml) covers W1 well.
[M30-005](../../tasks/M30-005.yaml) is the right home for native verification,
but its implementation work needs to account explicitly for W2-W8.
Neither task was updated or marked complete.

1. Provision an approved native Go toolchain and build the complete app using the
   existing PowerShell procedure. Capture an initial loopback-only baseline.
2. Unblock native test compilation, executable naming, and deterministic checkout
   bytes. Inventory remaining POSIX-only tests without hiding missing coverage.
3. Implement the shell/provider contract with failing native tests first; retain
   POSIX regressions, encoding, timeout, process-tree, and redaction checks.
4. Fix plugin discovery and editor launch with actual Windows executable stubs;
   correct UI path labels and verify file-backed operations.
5. Add the repeatable native smoke gate and run the acceptance matrix below with
   both a source build and an extracted release ZIP outside the checkout.

No broad rewrite, backend deployment, installer, or hosted CI purchase is needed
to start. Support should initially name the Windows versions and architectures
actually tested; do not promote an arm64 cross-build to an arm64 native PASS.

### Acceptance Checklist

All items below are **NOT RUN against Curlew** in this investigation.

1. [ ] Build full SPA and native executable; verify embedded JS/CSS, help/version,
   and startup outside the repository without Go, Node, Docker, or a backend.
2. [ ] Run and validate loopback collections; verify environments, overrides,
   bodies/files, local JSON Schema and OpenAPI inputs, and documented exit codes.
3. [ ] Exercise JSON, Markdown, events, HTML/JUnit output, repeated report writes,
   history reload/pruning, auth cache refresh, and concurrent file access.
4. [ ] Run UI discovery, execution, cancellation, browser launch, editor launch,
   history, and watch save/restart/shutdown checks, including actual console Ctrl+C.
5. [ ] Install/update each agent skill, preserve a local edit, and verify discovery
   in each available agent host. Record unavailable hosts and skipped symlink
   tests rather than treating them as passes.
6. [ ] Use local shell/provider/plugin stubs for spaces, Unicode, metacharacters,
   `.exe`/`.cmd`, CRLF, encoding, missing programs, failures, timeouts, cleanup,
   and secret redaction.
7. [ ] Test short/long paths, spaces, Unicode, drive-letter case, alternate drives,
   UNC paths where supported, CRLF files, junctions, and reserved report names.
8. [ ] Verify local secret/cache ACL expectations. POSIX `0600` is not a Windows
   ACL guarantee; no actual cross-user disclosure was established here.
9. [ ] Verify local TLS trust/proxy behavior, WebSocket use, and performance-mode
   cancellation against local fixtures only. Run applicable native tests and
   existing POSIX gates; declare race-toolchain limits explicitly.
10. [ ] Repeat core checks from release ZIPs on matching amd64/arm64 hosts, retain
    command outputs and exit codes, and update Windows support documentation.

## Evidence Appendix

These are OS/Node/Git probes, **not substituted Go unit tests or Curlew smoke tests**.

| Probe | Result |
| --- | --- |
| `git status --short --branch`, before report | `## main...origin/main`; clean |
| `git rev-parse HEAD` | Commit recorded at the top of this report |
| Tool lookup and common Go-path checks | No available Go compiler or Curlew executable; product build/tests NOT RUN |
| `git config --show-origin --get core.autocrlf` | `file:C:/Program Files/Git/etc/gitconfig true` |
| `git ls-files --eol` on scripts, goldens, embedded templates | `i/lf w/crlf attr/` for the inspected text files |
| Byte comparison of [pass_json.md](../../../internal/output/markdown/testdata/golden/pass_json.md) | 593 checkout bytes; 556 LF-normalized bytes; unequal |
| Node `spawnSync('/bin/sh', ['-c', 'echo curlew-probe'])` | `ENOENT`; no child started |
| Node direct `code.cmd --version` | `EINVAL`; no editor started |
| Existing Windows `where.exe` copied to disposable `curlew`, launched with `/?` | `ENOENT`; no child started |
| Same executable copied as `curlew.exe`, same arguments | Exit 0 |
| Git Bash `-n` on the gate | Exit 0; syntax check only |
| Git Bash executes the gate's exact CRLF `set` line in isolation | Exit 0 |
| Git Bash negative control `printf probe-ok; exit 7` | `probe-ok`, exit 7, for both installed Bash paths |
| UI-style JSON shell quoting of a spaced Windows path | Doubled backslashes reached the child; no Go build failure claimed from this alone |
| Node creation of `con`, `nul`, `aux`, `com1` directories | Succeeded; not proof of Go or Explorer compatibility |
| Follow-up `cmd mkdir` reserved-name probe | Inconclusive: ordinary-name control also failed due to invocation quoting; excluded from findings |
| Probe cleanup | All disposable directories removed; repository still clean before adding report |

### Risks Not Promoted To Findings

- [internal/parser/slug.go](../../../internal/parser/slug.go) allows device-name
  slugs, which [Markdown output](../../../internal/output/markdown/formatter.go#L153)
  uses in filenames and [iteration directories](../../../internal/output/markdown/datadriven.go#L20).
  Test with Go on Windows; the Node directory probe did not reproduce a failure.
- Atomic replacement code is not automatically wrong just because it uses
  `os.Rename`. No native Go replacement failure was observed. Test open handles,
  read-only targets, security scanners, and network shares before changing it.
- Case-sensitive string comparisons in UI filtering/watch bookkeeping need native
  cases, but no missed event or false path rejection was reproduced here.
- The UI setup's freshness check uses mtimes, not evidence that the fallback page
  was replaced by actual assets. Include an explicit JS/CSS assertion in smoke.
- Windows setup documentation is not included in the current release archive's
  explicit documentation list. Include it when publishing verified instructions.

### Platform References

- [Go 1.24 Windows file modes](https://github.com/golang/go/blob/go1.24.0/src/os/types_windows.go): regular files lack Unix execute bits.
- [Go 1.24 Windows executable lookup](https://github.com/golang/go/blob/go1.24.0/src/os/exec/lp_windows.go): extension and PATHEXT handling.
- [Go 1.24 Windows syscalls](https://github.com/golang/go/blob/go1.24.0/src/syscall/syscall_windows.go): Windows process API, not Unix `Kill`.
- [Windows CreateProcessW](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createprocessw): batch files require a command interpreter.
- [Windows filename rules](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file): device names, case, path, and namespace limitations.