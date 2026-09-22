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

## Setup Request Action Fix (2026-09-22)

The definition panel offered **Run this request** for setup and teardown rows,
but the execution contract accepts only exact main-request names. Selecting the
token setup therefore failed with `no request named`. The panel now omits that
action for non-main phases and its handler rejects them defensively. This matches
the existing sidebar selection rules without broadening execution to other requests.

| Check | Result |
| --- | --- |
| Component regression before implementation | RED for setup and teardown; also covers duplicate names across phases |
| Focused definition-panel tests after implementation | PASS, nine tests including main selection and busy states |
| Whole UI unit/component suite | PASS, 132 tests in 13 files |
| Svelte/TypeScript and ESLint | PASS, zero reported errors or warnings |
| Real embedded-app Playwright regressions | PASS, two tests using installed Edge and loopback fixtures |
| Main-request execution | PASS, setup + selected main + teardown all return HTTP 200; no other main requests execute |
| Definition views | PASS at 1280px and 960px; 390px shows the existing minimum-width guard, not a mobile application claim |
| Installed application | PASS, correct phase actions verified in the user's API project without executing live requests |

Playwright's bundled Chromium was absent, so an untracked temporary configuration
selected the already-installed Edge browser. No browser was downloaded. An initial
new assertion matched both a sidebar badge and the summary; it was scoped to the
summary, then passed. The temporary configuration was removed after verification.
Screenshots remain under `ui/test-results/` (ignored build/test output).

Installed executable: `C:\tools\curlew\curlew.exe`.
SHA-256: `976147A60E34448B6F3CD6B5CA1EC36B4B6CD9DCA0FCF94EDDB09A4DE177AC61`.
Previous executable retained as
`C:\tools\curlew\curlew-before-setup-action-20260922.exe`.
The installed binary embeds the rebuilt frontend; the tracked fallback index was
restored after building, so generated assets are not part of the source patch.

The existing user UI was restarted on port 8765 with `ingestion-test` selected.
The setup panel was inspected without running it; the main-request action remains
enabled. No 1Password retrieval, token request, invoice or business API request was
made during this fix. Formal Windows acceptance remains open.

### Pre-commit Checks

The user requested a local commit only, not a merge or push.

- Native Go build passed with Go 1.27.1 and cgo disabled.
- Whole-repository golangci-lint 2.11.2 passed with zero issues using Go 1.26.8.
- `go test -json -p 4 -timeout=60m ./...` passed with exit 0: 55 test packages,
  including 504 top-level CLI tests. Go reused valid cached package results;
  the CLI package completed in 681.606 seconds.
- Structured test log: `%TEMP%/curlew-ui-precommit-20260922-132607.jsonl`.

These checks supplement the UI and browser results above. They do not claim a
full native profile, race or clean-host acceptance pass.