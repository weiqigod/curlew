# M30-005 Local Windows Workflow Checks

Date: 2026-09-21. Scope: the installed application on the existing Windows
development machine. This is partial acceptance evidence, not task completion.
Task status and the clean-host/release acceptance requirements remain unchanged.

## Environment

- Windows 10.0.26200.0, native x64; PowerShell 7.6.6.
- Source base: `4aaef40`; checkout: `fix/M30-006-windows-verification-profiles`.
- Installed executable: `C:\tools\curlew\curlew.exe`, `0.1.0-dev`.
- SHA-256: `6EF1F6D776E0B45DBDFECC6093A059CB63881ABC95728E1D924540BDA7A5D32F`.
- Disposable project: `%TEMP%\Curlew Windows Acceptance 20260921\project with spaces-`
  followed by U+00E5. The directory tests both spaces and Unicode.
- Fixture: the existing `examples/local-server.py`, using the already-installed
  Python 3.14.7 and loopback port 62336. No runtime/tool installation was performed.

## Observed Results

| Check | Result | Evidence |
| --- | --- | --- |
| Installed `init` on a spaced/Unicode path | PASS | Generated a separate disposable project; exit 0 |
| Codex, Claude Code and Copilot skill installation | PASS | Install, repeated install and update each exit 0 for all three destinations |
| Skill update preserves local edits | PASS | Each edited `SKILL.md` produces exit 3 naming the conflict; edited bytes, another topic file and project configuration remain unchanged |
| Installed CLI request execution | PASS | One loopback GET and status assertion pass; JSON output, exit 0 |
| Native browser auto-opening | PASS | `ui --port 0` opens Edge; no browser had a Curlew title before launch, and Edge did afterward |
| Browser execution | PASS | Sample request passes; result details show HTTP 200 |
| Browser file watching | PASS | YAML request rename appears in the collection tree without navigation, reload or server restart |
| UI stop and restart | PASS | Ctrl+C stops the test UI; restarting with explicit port 62020 succeeds |
| History persists across restart | PASS | Both earlier browser runs appear after restart; another request then passes |
| CLI watch reruns edited YAML | PASS | Initial run passes; changing the request name triggers a second passing run with the new name |
| Owned process cleanup | PASS | Test UI and fixture ports no longer listen; watch/test UI processes are absent |
| Original application preserved | PASS | Original UI remains HTTP 200 on port 52150, PID 27880; it is the only remaining Curlew process |

Skill checks and machine-readable results are retained in the temporary root as
`check-skills.ps1` and `skill-results.json`. The fixture project and its local
history remain there as evidence; all test services are stopped.

Commands exercised against the installed executable:

```powershell
curlew init <temporary-project>
curlew skill install --agent <codex|claude|copilot> <temporary-project>
curlew skill update --agent <codex|claude|copilot> <temporary-project>
curlew run collections/sample.yaml --format json
curlew ui --port 0
curlew ui --port 62020 --no-open
curlew watch collections/sample.yaml --format json
```

The temporary project overrides `base_url` to the loopback fixture. Its generated
`dev` environment has no overrides. No public endpoint, business API, vault or
paid service was called. The user's project and installed executable were unchanged.

## Interpretation And Limits

- No application failure requiring an implementation change was found.
- The browser accessibility snapshot retained a throttled, hidden `0 of 0 done`
  announcement after a fast run. Inspection confirmed it is clipped to a 1px live
  region. The visible summary and progressbar both report 1/1, and the completion
  announcement correctly reports one passed and zero failed.
- Ctrl+C also interrupted the PowerShell command wrapper. Terminal cancellation
  exit 1 is not an observed Curlew exit code; clean application exit 0 was not
  independently measured here. Process termination, listener release and restart
  were verified separately. One restart command was misread by the terminal and
  retried successfully before any application result was claimed.
- Skill filesystem behavior passed. Actual discovery by each agent host was not
  tested, and installation success does not establish host discovery.
- Clean-host installation, extracted-ZIP end-to-end acceptance, native arm64,
  isolated Windows race verification and the authoritative full gate remain open.
  These local workflow checks do not replace those gates.

Earlier runtime, output and request-cache checks are recorded in
[M30-004 verification](M30-004-verified.md); compiler-free developer profiles and
toolchain cleanup are recorded in [M30-006 verification](M30-006-verified.md).