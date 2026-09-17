# Running Curlew on Windows

**Status checked: 2026-09-17.** This is the setup procedure and the remaining
Windows work, not a claim of completed native Windows testing.

## What is ready, and what is not

| Area | Evidence / status |
|---|---|
| Windows amd64 and arm64 executables and ZIP archives | Cross-built and archive/checksum checks passed in the local gate. This does not prove native execution. |
| Basic CLI | Native amd64 build, validation, loopback execution, documented failure and cleanup pass through `smoke/run.ps1`. Full browser acceptance remains open. |
| Agent skill installation | Codex, Claude Code and Copilot destinations implemented; native Windows filesystem/discovery verification remains open. |
| Command-backed variables (`from_command`) | Known portability gap: the executor hardcodes `/bin/sh -c`. Not supported on ordinary native Windows yet. |
| CLI-backed vault providers | Share that executor and need a Windows-safe invocation/quoting strategy and native tests. |
| Plugins and editor launch | Native `.exe` plugin discovery, process-tree cleanup, quoted editor commands and `.cmd`/`.bat` forwarding pass local tests. |
| Development verification | `scripts/verify-windows.ps1` runs native checks; `scripts/ci-local.sh` remains the authoritative repository gate. |

Open work is tracked in [M30-004: command/vault portability](../management/tasks/M30-004.yaml)
and [M30-005: native Windows verification](../management/tasks/M30-005.yaml).
Windows foundation fixes are tracked in [M30-006](../management/tasks/M30-006.yaml).
Neither task is complete. WSL success does not count as native Windows evidence;
installing Git Bash alone is not a demonstrated fix for the hardcoded `/bin/sh` path.

## Prerequisites

For building a checkout, install Git, Go 1.24+ and Node.js 22+ with npm, and ensure
they are on PATH. The private repository requires working GitHub credentials.
Use PowerShell and a normal writable directory. The complete local app needs no
Docker, .NET backend, database or account. No C compiler is needed for the build
below. Python 3 is optional, used only for the local fixture in the smoke checklist.

The finished executable embeds the UI; Go and Node.js are build dependencies,
not runtime dependencies. Build from current main to include the recent skill
installation and agent usability improvements; an older release may lack them.

## Clone and build in PowerShell

These commands are derived from `scripts/build-ui.sh` and `ui/vite.config.ts`.
They still need execution on a native Windows host under M30-005. The explicit
exit checks prevent a failed frontend build from looking like a complete app.

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
6. **Shell/vault features:** after M30-004 is implemented, test command-backed
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

The broader native entry point additionally requires Node.js 22+, npm,
golangci-lint 2.11.2, GoReleaser 2.17.1 and a GCC-compatible Windows C compiler
for `go test -race`. Check prerequisites without running the suites:

```powershell
pwsh -NoProfile -File scripts/verify-windows.ps1 -PreflightOnly
```

Then run native build, UI, tests, race, coverage, lint, release-config and smoke
checks:

```powershell
pwsh -NoProfile -File scripts/verify-windows.ps1
```

The pinned golangci-lint 2.11.2 binary was built with Go 1.26 and cannot analyze
Go 1.27 export data. Keep the active product compiler and pass a separate Go
1.26.x executable through `-LintGoCommand` when using Go 1.27. This native script
does not replace the authoritative auto-scoped `./scripts/ci-local.sh` POSIX run.
A missing required tool fails preflight; it is not reported as a skipped pass.

## Remaining engineering work

- **M30-004:** replace the POSIX-only command execution assumption, define the
  Windows shell/quoting contract, verify CLI-backed vault invocation, and retain
  timeout, error and secret-redaction behavior. A shell-name substitution alone
  is insufficient evidence that provider commands are quoted correctly.
- **M30-005:** run the complete checklist on clean and packaged Windows installs,
   resolve findings, and update these instructions from observed results.
  Separate the lightweight app smoke test from development tests needing Bash or
  other tools. Automatic hosted CI remains subject to the separate M29-001 billing
  decision; local Windows verification can proceed independently.
