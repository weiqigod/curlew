# Running Curlew on Windows

**Setup updated: 2026-09-21.** This is the setup procedure and the remaining
Windows work, not a claim of completed native Windows testing.

## What is ready, and what is not

| Area | Evidence / status |
|---|---|
| Windows amd64 and arm64 executables and ZIP archives | Cross-built and archive/checksum checks passed in the local gate. This does not prove native execution. |
| Basic CLI and embedded browser UI | Native amd64 build, validation, loopback execution, documented failure and cleanup pass through `smoke/run.ps1`. Installed-app browser opening, live file refresh, CLI watch, stop/restart and history persistence also pass on this host. Packaged browser acceptance remains open. |
| Agent skill installation | Installed-app install/reinstall/update and local-edit preservation pass for Codex, Claude Code and Copilot destinations on a spaced/Unicode Windows path. Actual agent-host discovery remains unverified. |
| Command-backed variables (`from_command`) | Native amd64 PowerShell/output/cancellation tests pass; Linux regression tests also pass. Full repository gate remains open. |
| CLI-backed vault providers | Native `.exe` and compatible `.cmd` stubs pass for all five providers; real CLI loopback and redaction cases pass. Batch limits below apply. |
| Plugins and editor launch | Native `.exe` plugin discovery, process-tree cleanup, quoted editor commands and `.cmd`/`.bat` forwarding pass local tests. |
| Development verification | `scripts/verify-windows.ps1` defaults to compiler-free `Local` checks; `Race`, `Release` and `Full` are explicit profiles. `scripts/ci-local.sh` remains the authoritative repository gate. |

Open work is tracked in [M30-004: command/vault portability](../management/tasks/M30-004.yaml)
and [M30-005: native Windows verification](../management/tasks/M30-005.yaml).
Windows foundation fixes are tracked in [M30-006](../management/tasks/M30-006.yaml).
M30-004 and M30-006 still need final workflow closure. WSL success counts only as
POSIX evidence, not native Windows evidence. The command implementation does not
require Git Bash. See the
[M30-004 verification record](../management/plans/M30-004-verified.md) for actual
checks and remaining blockers; packaged-app and arm64 checks are still open.
The [installed-app workflow checks](../management/plans/M30-005-verified.md) record
native development-host results, not clean-install or full-gate acceptance.
The proposed [Windows repair plan](../management/plans/windows-repair-plan.md)
orders the remaining fixes, task dependencies and completion checks.

## Prerequisites

For building a checkout, install Git, Go 1.24+ and Node.js 22+ with npm, and ensure
they are on PATH. The private repository requires working GitHub credentials.
Use PowerShell and a normal writable directory. The complete local app needs no
Docker, .NET backend, database or account. No C compiler is needed for the build
below. Python 3 is optional, used only for the local fixture in the smoke checklist.

The finished executable embeds the UI; Go and Node.js are build dependencies,
not runtime dependencies. Build from current main to include the recent skill
installation and agent usability improvements; an older release may lack them.

Installing or running Curlew does not require MinGW, GCC, GDB or Python. External
tools are needed only for features configured to call them, such as Azure CLI for
the Azure vault provider. Keep developer test tools separate from the app install.

## Clone and build in PowerShell

These commands are derived from `scripts/build-ui.sh` and `ui/vite.config.ts`.
The UI build and native amd64 build/startup were exercised on this host; a clean
clone and packaged-app smoke test remain under M30-005. The explicit exit checks
prevent a failed frontend build from looking like a complete app.

```powershell
git clone https://github.com/weiqigod/curlew
if ($LASTEXITCODE -ne 0) { throw "Clone failed" }
Set-Location curlew

Push-Location ui
try {
    npm.cmd ci
    if ($LASTEXITCODE -ne 0) { throw "Frontend dependency installation failed" }
    npm.cmd run build
    if ($LASTEXITCODE -ne 0) { throw "Frontend build failed" }
} finally {
    Pop-Location
}

$env:CGO_ENABLED = "0"
go build -o curlew.exe ./cmd/curlew
if ($LASTEXITCODE -ne 0) { throw "Curlew build failed" }
.\curlew.exe --version
if ($LASTEXITCODE -ne 0) { throw "Curlew did not start" }
```

Use `npm.cmd` explicitly so PowerShell does not select the npm.ps1 wrapper.
Build the frontend **before** Go: a Go-only build includes the tracked fallback
page rather than the full UI. Once built, `curlew.exe` can be copied to a directory
on PATH; that is optional when using the explicit paths below.

## Create a project and start the UI

Run from the checkout root, using a new `demo` directory:

```powershell
.\curlew.exe init demo
if ($LASTEXITCODE -ne 0) { throw "Project initialization failed" }
.\curlew.exe skill install --agent codex demo
if ($LASTEXITCODE -ne 0) { throw "Skill installation failed" }

Set-Location demo
..\curlew.exe ui --no-open
```

Open the complete local URL printed by the command, including its session token.
Keep the process running; Ctrl+C stops it. This is a browser UI served by the
local executable. `--agent claude` and `--agent copilot` select their project skill
directories instead; see the [agent guide](AGENT_GUIDE.md).

The new project uses default output settings. Installing a skill does not change
those settings. Configure Markdown/events explicitly if desired, as documented
in the agent guide. The scaffold's sample points at an external API by default;
use the local override below for offline-service smoke checks.

## What to verify on the Windows machine

Record the commit (`git rev-parse HEAD`), Windows version and architecture,
PowerShell version (`$PSVersionTable.PSVersion`), `go version`, `node --version`,
and `npm.cmd --version`. Save actual command output and exit codes. An unrun item
is **not verified**, not PASS. Report any failure with the command and diagnostic.

1. **Build and start:** execute the PowerShell build above. Confirm `curlew.exe`
   starts and help works from a directory with spaces in its path.
2. **Exercise actual local HTTP:** if Python 3 is available through the Windows
   launcher, start the fixture from the checkout root in another terminal:

   ```powershell
   py -3 examples/local-server.py --port 18081
   ```

   From `demo`, run:

   ```powershell
   ..\curlew.exe validate collections/sample.yaml --format json
   ..\curlew.exe run collections/sample.yaml --var base_url=http://127.0.0.1:18081 --format json
   ```

   Both should exit 0; inspect the executed request and assertion results, not
   just the exit status. If port 18081 is busy, select another port and use it
   consistently. Keep the fixture running during UI execution checks.
3. **Browser UI:** confirm the full interface loads, collections are discovered,
   and a collection runs against the fixture. For UI runs, change `base_url` in
   the disposable demo's configuration to the fixture origin first. Verify assets
   load, reports/history work, and Ctrl+C shuts the server down. Also check browser
   auto-opening separately by running `ui` without `--no-open`.
4. **Agent installation:** inspect the selected skill path, run `skill update`,
   then introduce a local edit in a disposable skill copy. Confirm an update
   reports a conflict and preserves it. Check actual discovery in each supported
   agent host that is available; record unavailable hosts as untested.
5. **Diagnostics and paths:** verify assertion failure, invalid YAML, a missing
   variable and an unreachable local port produce their documented outcomes.
   Check environment selection, JSON/Markdown/events, Unicode and spaced paths,
   watch restart/shutdown, and child-process cleanup. Use the [manual](MANUAL.md)
   for syntax; a Bash recipe must be translated or explicitly marked unverified.
6. **Shell/vault features:** repeat the M30-004 native tests for command-backed
   variables and provider CLI stubs on Windows, including quotes, spaces, output
   encoding, CRLF, errors and timeouts. Do not test real cloud accounts merely
   to verify portability.
7. **Packaged app:** repeat the core smoke checks with the Windows release ZIP's
   extracted executable, outside the source checkout. Verify amd64 and arm64 on
   matching hosts before claiming both architectures natively verified.

Stop fixture and UI processes when done. Store the evidence in the M30-005
verification report, with a per-platform/per-feature PASS, FAIL or NOT RUN matrix.

## Native developer verification

The lightweight product smoke needs Go and Python 3 and uses only dynamic
loopback ports. It creates one temporary root and terminates only its own fixture:

```powershell
pwsh -NoProfile -File smoke/run.ps1
```

The native verifier has four profiles. `Local` is the default and sets
`CGO_ENABLED=0`; it never resolves or invokes a C compiler or GoReleaser.

| Profile | Checks | Required tools |
| --- | --- | --- |
| `Local` | UI check/lint/test/build, Go build/tests/coverage, Go lint, loopback smoke | Go, Node.js 22+, npm, Python 3 for fixtures, golangci-lint 2.11.2 and Go 1.26.x for lint |
| `Race` | Native `go test -race` only | Go and an approved compatible Windows C compiler |
| `Release` | `goreleaser check` only; does not build or publish archives | GoReleaser 2.17.1 |
| `Full` | All three profiles' checks | All of the above |

Check local prerequisites without running suites, then run the local checks:

```powershell
pwsh -NoProfile -File scripts/verify-windows.ps1 -PreflightOnly
pwsh -NoProfile -File scripts/verify-windows.ps1
```

Successful local checks print `WINDOWS_LOCAL_VERIFY_PASS`, not a full-gate pass.
`Race` and `Release` have their own success markers. Only `-Profile Full` prints
`WINDOWS_VERIFY_PASS`. An explicitly selected profile fails if one of its tools
is missing. The verifier restores its process environment and working directory.

Do not install a broad compiler/debugger bundle just to run the app or local
checks. The WinLibs bundle used during early Windows verification included an old
Python runtime for GDB and was removed from the development machine. A recent GCC
version alone does not establish that bundled runtimes are maintained.

Run shared race coverage inside an existing WSL/Linux development environment,
from a checkout of the revision being verified:

```bash
go test -race ./...
```

That requires a Linux C compiler and is Linux evidence, not Windows coverage.
Keep Windows-specific race checks in an isolated Windows test environment with
an explicitly approved compiler and inspected bundled dependencies. Go's
[Windows race detector requirements](https://go.dev/doc/articles/race_detector#Requirements)
include MinGW-w64 runtime libraries; the full WinLibs bundle and GDB are not
required. In that environment, use:

```powershell
pwsh -NoProfile -File scripts/verify-windows.ps1 -Profile Race
pwsh -NoProfile -File scripts/verify-windows.ps1 -Profile Release
pwsh -NoProfile -File scripts/verify-windows.ps1 -Profile Full
```

The isolated environment is a setup requirement, not something these scripts
provision or install. No hosted CI or cloud spending is enabled by choosing a profile.

The pinned golangci-lint 2.11.2 binary was built with Go 1.26 and cannot analyze
Go 1.27 export data. Keep the active product compiler and pass a separate Go
1.26.x executable through `-LintGoCommand` when using Go 1.27. This native script
does not replace the authoritative auto-scoped `./scripts/ci-local.sh` POSIX run.
A missing required tool fails preflight; it is not reported as a skipped pass.
See the [profile and toolchain cleanup evidence](../management/plans/M30-006-verified.md).

## Remaining engineering work

- **M30-004:** complete the authoritative full gate and resolve any remaining
   review findings. Native scoped success is not final task completion.
- **M30-006:** record the completed native/Posix verification evidence and close
   the foundation task after the authoritative gate passes.
- **M30-005:** run the complete checklist on clean and packaged Windows installs,
   resolve findings, and update these instructions from observed results.
  Separate the lightweight app smoke test from development tests needing Bash or
  other tools. Automatic hosted CI remains subject to the separate M29-001 billing
  decision; local Windows verification can proceed independently.

## Windows command contract

- `from_command` uses system Windows PowerShell (not whichever `pwsh` is on PATH),
   with no profile, non-interactive encoded scripts, and UTF-8 output. Use `&` for
   a quoted executable path; existing Bash commands need platform-specific syntax.
- Native failures and script errors fail variable resolution with exit 5. Public
   errors omit script text, arguments, credentials, and raw stderr. A successful
   script can handle an earlier native error; this is not a shell-wide `set -e`.
- Commands/provider calls are capped at 30 seconds or an earlier caller deadline.
   Windows children start suspended, join a kill-on-close Job Object, then resume.
   Remaining descendants are terminated on completion/cancellation; commands must
   not rely on detached background work surviving.
- stdout/stderr must be UTF-8. Trailing LF/CRLF is stripped, but spaces, internal
   newlines, and a standalone CR are not. Other encodings fail rather than silently
   changing a secret.
- Vault `.exe` calls receive literal argv and child-only environment overrides.
   `.cmd`/`.bat` calls preserve double quotes, backslashes, spaces, Unicode, `%`,
   `!`, `&`, `|`, `^`, `<`, and `>` as literal arguments. Native tests exercise
   Azure CLI 2.90.0 MSI/ZIP forwarding and Google Cloud CLI 585.0.0 forwarding
   with real Python, including Google's enable/disable delayed-expansion sequence.
- Batch line terminators CR/LF and OS string terminator NUL are rejected before
   execution. Encoded invocations and each environment entry must fit 8000 UTF-16
   units below CMD's line limit. Custom scripts run with their own semantics;
   explicit additional expansion inside script code can change argument values.
- Windows launcher compatibility tests require Python 3 with the standard-library
   venv module. They create offline temporary modules, not cloud SDK installations,
   and use no provider accounts. The CLI does not otherwise require Python.

Native test commands (loopback/stubs only):

```powershell
go test ./internal/variable/ ./internal/vault/ -count=1 -cover -timeout=600s
if ($LASTEXITCODE -ne 0) { throw "Command/vault verification failed" }
```
