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

## Windows Collection Filter Fix (2026-09-22)

A local project exposed multiple similarly named invoice collections using
different environments. The reported missing `invoice_base_url` belonged to an
APIM collection selected with the Ingestion environment; the failed run contained
zero executed HTTP requests. The documented startup now scopes the UI to the
configured collection instead of suggesting URL aliases for incompatible routes.

Exercising `ui --collection` then exposed a Windows defect: `filepath.Rel` supplied
backslashes but discovery compared against slash-separated paths. The server now
normalizes the filter at construction, including the metadata it exposes.

| Check | Result |
| --- | --- |
| Existing filter regression extended with native paths | RED: empty tree, unnormalized metadata and rejected batch start on Windows |
| Same regression after normalization | PASS: slash and native paths both select one collection and one passing fake request |
| Entire UI-server package | PASS, native Windows, cgo disabled |
| UI-server package lint | PASS, zero issues |
| Frontend and complete executable build | PASS |
| Installed executable with `--collection .\collections\ingestion.yaml` | PASS: metadata and tree contain only `collections/ingestion.yaml` |
| Actual project browser view | PASS: Ingestion TEST, configured TEST base URL, mutations enabled, invoice action available; not executed |

Installed executable SHA-256:
`07217F8AD98DA43F28F473EF34EDAD582399B7F3A9C0627CCE051C8A69724D41`.
Previous executable retained as
`C:\tools\curlew\curlew-before-collection-filter-20260922.exe`.
An extra unfiltered Curlew instance on port 8766 held the executable open; after
checking its exact executable/command and absence of child processes or external
connections, it was stopped for replacement. One filtered UI remains on port 8765.

The tests use fake request execution and localhost metadata reads. No secret was
read, invoice sent, or business endpoint called by this repair. The unrelated
APIM collections and their credentials were not changed. The generated tracked
asset index was restored after building. This is scoped fix evidence, not another
full-suite, race, clean-host or packaged-release acceptance run.

## Persistent Request Workspace (2026-09-22)

User requirement: run, inspect and run again without navigating away from the
selected request. The definition view now retains the Run control and renders
the existing inspector inline. Tab selection is local to that view. Sidebar
request clicks consistently select the workspace rather than redirecting to a
past result. The contextual toolbar and `r` key select the same main request;
batch execution remains an explicit menu action. Setup/teardown cannot be run
individually. No mutation is automatically retried.

| Check | Result |
| --- | --- |
| Persistent-view component regressions | RED before implementation; GREEN for unchanged route intent, repeated results, inline network/config errors and busy guards |
| All UI unit/component tests | PASS, 134 tests in 13 files |
| Svelte/TypeScript and ESLint | PASS, zero errors/warnings |
| Contextual toolbar browser regression | RED: toolbar submitted a batch; GREEN after selecting the open request |
| Full embedded-app Playwright suite | PASS, 23 tests in installed Edge with loopback fixtures |
| Manual run loop | Request button, toolbar and keyboard each start a new run of exactly one selected main request plus setup/teardown; URL remains unchanged |
| Failure and cancellation behavior | Network error remains inline; cancel and subsequent successful run stay in the same workspace |
| Screenshots | Reviewed at 1280px and 960px; existing sub-960px guard remains |
| Complete executable and installed UI | PASS; actual Ingestion TEST workspace opened without executing a request |

One intermediate browser run displayed an empty Assertions tab. It was not
reproduced by the same four focused tests with browser exceptions enabled or by
the final complete browser suite. This observation is retained, not claimed to
have a diagnosed cause. No assertions were removed to get the final pass.

Installed SHA-256:
`7AF6A01E338A8E0A1366712455F0A44D9AD774F90F743C8269F199A5F40588FF`.
Backup: `C:\tools\curlew\curlew-before-request-workspace-20260922.exe`.
The filtered user session was restarted on port 8765. The extra unfiltered
instance on port 8766 was stopped only after checking its exact executable,
command line, and absence of child processes or external connections.

Read-only inspection of the user's latest run showed OAuth HTTP 200 followed by
an invoice POST ending with EOF before an HTTP response. This does not establish
whether the remote service accepted the invoice. No live token, invoice, health
or other business request was made during this work. Prior filter-fix changes
were preserved; this workspace change remains uncommitted. No full Go suite,
race or release acceptance was rerun for these frontend-only additions.

## Manual Auth And Visible Setup (2026-09-22)

This supersedes the earlier restriction on individual setup actions. The UI now
supports `mode: setup` with one exact setup name. The server runs setup through
that step, preserving prerequisites and redaction, without later setup, main or
teardown requests. Panel, toolbar and keyboard use the same mode. Normal main
runs still execute fresh setup; tokens are not cached across runs.

The request workspace shows setup/main/teardown outcomes and HTTP codes together.
Selecting a result changes only the inline inspector. Final details refresh
without resetting the selected tab. Browser testing also exposed a null source
snippet crash and premature summary reconciliation; both now have deterministic
RED/GREEN regressions and fixes.

| Check | Result |
| --- | --- |
| Real loopback token capture | PASS; two successive UI runs send the exact newly extracted fake bearer token; inspector remains redacted |
| Setup-only API regression | RED unknown mode, then PASS; prerequisite ordering, external fragment, exclusion of same-name main/later setup/teardown, validation and redaction |
| Affected Go packages | PASS, complete runservice and uiserver suites with CGO disabled |
| Scoped Go lint | PASS, zero issues |
| UI tests | PASS, 137 tests in 14 files |
| Svelte/TypeScript and ESLint | PASS, zero errors/warnings |
| Full real-binary browser suite | PASS, 23 tests using installed Edge and loopback fixtures |
| Auth controls | Panel, toolbar and keyboard each execute setup only at 1280px and 960px, without navigation |
| Installed UI | PASS; current auth controls enabled and final JS asset served on port 8765; no live request executed |

Installed SHA-256:
`5305F4A5702093F5AF5808C678BC1014E3DB9A56784728184A40530A2CDEE56C`.
Backup: `C:\tools\curlew\curlew-before-manual-auth-20260922.exe`.
Restarted only the verified idle filtered session. The second, user-started
process was left running on its old loaded executable. No commit, merge or push.

Read-only inspection of saved run `1f4ba266563b8c33131efd4529d950d4` showed
auth HTTP 200, an outbound redacted Authorization header and invoice HTTP 401
with `WWW-Authenticate: Bearer error="invalid_token"`. The saved redacted token
cannot establish which validation failed. The loopback check proves the token
wiring, not the original live token's exact bytes or validity. Before restart,
the latest visible run instead showed a 1Password retrieval timeout; no run was
active. No live credential retrieval or invoice retry was performed. Full Go,
race, hosted CI and clean-host/release acceptance were not rerun for this change.

### Pre-commit gate (2026-09-23)

| Check | Result |
| --- | --- |
| `go build ./cmd/curlew` (CGO disabled) | PASS |
| Whole-repository `golangci-lint run` (Go 1.26.8) | PASS, 0 issues |
| `go test -json -p 4 -timeout 60m ./...` | 54 of 55 packages PASS; `internal/plugin` `TestHost_Close_RealProcessTree/cooperative_exit` failed with a plugin handshake timeout under parallel load |
| That test alone, `-count=3` | PASS; package unchanged by this work, so treated as a load-dependent flake, not repaired |

Log: `%TEMP%\curlew-precommit-auth-20260923.jsonl`. Race, hosted CI and
clean-host acceptance were not run.

The live 401 was later traced to the user's API project, not Curlew: the token
request used a retired token server and omitted `Accept: application/jwt`, so
Curity returned an opaque token the JWT-validating APIs reject. Both were fixed
in that project's configuration; the invoice call is not yet live-verified.