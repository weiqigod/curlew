# Curlew local UI specification

**Updated:** 2026-09-15. Scope: `curlew ui`, `internal/uiserver`, and `ui/`.
The [original design](history/UI_SPECIFICATION-v1-design.md) is retained as history;
its tier gates and code sketches are not the current contract.

## 1. Product boundary

The UI is a runner and inspector over YAML files in one local project. It uses
the same Go execution services as the CLI. Files remain the source of truth;
there is no in-app collection editor, account, license, billing, backend upload,
MCP service, or distributed worker. History, comparison, parallel runs and batch
runs are available without feature gating. `web/` is a different, retained platform.

See [the manual walkthrough](MANUAL.md#16-local-browser-ui) for normal use and
[the agent guide](AGENT_GUIDE.md) for shell-based automation.

## 2. Launch and configuration

### 2.1 Invocation

```bash
curlew ui
curlew ui --no-open --port 8766
curlew ui --env dev --collection collections/sample.yaml
```

Run in a directory containing `curlew.yaml` or a child of it. The command finds
that project root, loads configuration, validates the selected environment,
binds loopback, prints a session URL and optionally opens the browser.
`--collection` must resolve inside the project.

### 2.2 Network and lifecycle

Default port: 8765. The default scans up to 19 higher ports if busy. An explicit
busy port fails; `--port 0` requests an ephemeral port. The bind address is loopback
only, normally `127.0.0.1`. The full printed URL carries a fresh session token.
Ctrl+C/SIGTERM cancels an active run, flushes history and shuts down the server.
Do not share a token-bearing URL as a public link.

### 2.3 Project configuration

```yaml
ui:
  port: 8765
  host: 127.0.0.1
  open_browser: true
  editor: "code --goto {file}:{line}"
  history:
    enabled: true
    max_runs: 50
```

Flags override the corresponding configuration. `CURLEW_EDITOR` overrides
`ui.editor`; the fallback is `code --goto`. Editor templates are split into
arguments, not evaluated as shell scripts. In `ui.editor`, single and double
quotes must preserve spaces and Windows backslashes in one argument. When
CURLEW_EDITOR names a Windows batch wrapper, Curlew must pass literal arguments
through the system command interpreter; `.cmd` and `.bat` wrappers receive spaces,
quotes and shell metacharacters as data, not commands. NUL, CR,
LF and batch invocations above the documented CMD length budget are rejected.
A missing or invalid editor produces a diagnostic; it does not prevent running
collections.

### 2.4 Exit codes

| Exit | Meaning |
|---|---|
| 0 | Clean shutdown or help |
| 1 | Usage error, bind failure, or fatal server error |
| 3 | Invalid configuration, unknown environment, or collection outside the project |
| 5 | No project found |

## 3. Build and assets

The SPA is compiled into `internal/uiserver/assets/dist` and embedded by Go.
Build the frontend before the executable:

```bash
./scripts/build-ui.sh
go build -o curlew ./cmd/curlew
```

A release contains the resulting static assets, so Node.js is a build dependency
only. Plain Go builds without the frontend display an explicit build-required
page. Release verification must request the HTML and each referenced JS/CSS asset;
HTTP 200 by itself is insufficient because the SPA fallback can return HTML for
a missing asset.

For development use `ui`'s Vite server and `CURLEW_UI_DEV_PROXY` as described by
`ui/vite.config.ts`; this proxy is not needed for normal use.

## 4. HTTP contract

Routes are registered in `internal/uiserver/server.go`; request and response
structures live beside each `api_*.go` handler and in `ui/src/lib/types/`.
The API version is 1, with prefix `/api/v1`.

| Route | Purpose |
|---|---|
| `GET /meta` | Version, event schema version, project, history settings and body limits |
| `GET /tree` | Collections, request metadata and validation issues |
| `GET /environments` | Environment choices and redacted values |
| `GET /validate` | Validation diagnostics |
| `GET /files` | Project YAML source with path and size restrictions |
| `POST /runs` | Start a run; returns 202 and run ID |
| `GET /runs` | Retained run metadata |
| `GET /runs/current` | Active run and progress, or null |
| `GET /runs/{run_id}` | Run state and summary |
| `DELETE /runs/{run_id}` | Delete retained history |
| `POST /runs/{run_id}/cancel` | Cancel the active run |
| `GET /runs/{run_id}/events` | Run event stream |
| `GET /runs/{run_id}/requests` | Request results |
| `GET /runs/{run_id}/requests/{request_id}` | Request detail |
| `GET /runs/{run_id}/requests/{request_id}/body` | Body content |
| `GET /compare?base=…&target=…` | Align results and calculate differences |
| `POST /open` | Open project source in the configured editor |
| `GET /ws` | WebSocket live events, replay and file-change notifications |

API requests require `Authorization: Bearer <token>` or `X-Curlew-UI-Token`.
The WebSocket upgrade also accepts the token query parameter. JSON writes require
JSON content type. Host and Origin checks reject nonlocal browser access.
Sensitive values are redacted; the UI has no `--allow-sensitive` option.

## 5. Execution and events

Only one run executes at a time; another start receives 409 `run_active`.
The `POST /runs` payload is `StartParams` in `orchestrator.go`:

```json
{"collection":"collections/sample.yaml","env":"dev","parallel":false,"mode":"all"}
```

A null collection requests a batch. Modes are `all`, `selection`, and
`rerun_failed`; selection uses exact main-request names from one collection.
Setup and teardown remain part of the execution. The UI renders pending, running,
passed, failed, skipped and error outcomes. Assertion failures and transport
errors are different outcomes; a skipped request is not a pass.

Run events use [event schema v1.6](EVENTS_SCHEMA_v1.6.md). The WebSocket hub
supports subscription/replay and file-change notifications. Refresh/reconnect
must reconcile current run state, not count replayed events twice. Request
source links use assertion `source_file` and `source_line` where present.

## 6. History and comparison

Five completed runs remain in memory. Persisted history defaults to 50 runs under
`.curlew/ui/`, with a generated local ignore file. Disable persistence with
`ui.history.enabled: false`; this does not disable current-run inspection.
Inline body content is capped at 256 KiB and stored bodies at 1 MiB; responses
carry truncation metadata. Compare aligns by request slug and iteration index,
not by the per-run request ID. The server calculates outcome/status/timing deltas;
the client renders body differences. Plain `curlew run` does not populate UI history.

## 7. Browser workflow

The sidebar discovers collections and marks invalid files. Environment selection,
Run all, selection, rerun failed, cancellation and optional parallel execution
feed the same orchestration endpoint. The inspector exposes body, headers,
assertions, timing, request and error details. Editing opens an external editor.
History and comparison use retained UI runs.

Press `?` for the current keyboard map: `r` runs, Shift+R reruns failures, `j`/`k`
move through results, Enter opens the inspector, and `g` then `h` opens history.
The help overlay traps focus and closes with Escape. Disconnection and validation
errors remain visible rather than appearing as successful runs.

## 8. Verification

Go tests exercise authentication, paths, APIs, execution, cancellation, replay,
history and comparison. UI component tests cover rendering and stores.
`cd ui && npm run test:e2e` launches the real executable and local fixture API
through `tests/e2e/global-setup.ts`. `scripts/check-ui-artifact.py` checks the actual
release executable's embedded resources and collection discovery.
