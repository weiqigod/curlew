# Historical UI design

This preserves the original design, including removed tier gates and obsolete code sketches.
For the implemented contract use [the current specification](../UI_SPECIFICATION.md).

# curlew ui — Specification

**Version:** 1.0
**Date:** 2026-06-11
**Status:** Draft — pre-milestone design, complete enough to implement
> **Note on tier gating (2026-08):** The licensing/tier system has since been removed from the CLI. All tier-gating statements in this spec are historical and no longer apply: there is no `/meta` tier block, no 403 `feature_gated` response, no `ui_run_history`/`parallel_execution`/`test_discovery`/`data_driven` entitlement checks, and no `CURLEW_TIER` seam. The UI exposes every feature unconditionally; run-history persistence is controlled solely by the `ui.history.enabled` config flag.

**Relationship to other documents:** This is a standalone specification for the `curlew ui` feature. It is written against SPECIFICATION.md v4.4 and EVENTS_SCHEMA_v1.2.md; it defines the v1.3 events-schema delta (§7). It will be folded into a SPECIFICATION.md v4.5 bump when the milestone is scheduled. The interactive design prototype that this spec formalizes (a design-tool export) is committed at `docs/design/curlew-ui/` — open `curlew UI.html` for the interactive app or `curlew Canvas.html` for the artboards; `at-tokens.css` is the design-token ground truth, adopted verbatim into `ui/src/styles/tokens.css` (§10.5).

**Terminology used consistently throughout (normative):**

| Term | Values |
|---|---|
| Request **outcome** (server) | `passed` \| `failed` \| `skipped` \| `error` |
| Request **live status** (client-only superset) | `pending` \| `running` \| outcome |
| Request **phase** | `setup` \| `main` \| `teardown` |
| Run **state** | `running` \| `cancelling` \| `completed` \| `cancelled` \| `error` |
| Run **exit_status** | `passed` \| `failed` \| `error` \| `cancelled` |
| Body size limits | WS events: 2 KiB (`events.DefaultBodyLimit`) · REST inline: 256 KiB · persisted store: 1 MiB per body |
| Retention | persisted history: 50 runs (configurable) · in-memory ring: 5 runs |
| Default port | 8765 |
| Redaction marker | `[REDACTED]` (the `variable.Redacted` constant — what `RedactBody`/`RedactHeaders`/`RedactValue` substitute) |
| Error categories | `parse` \| `network` \| `config` \| `assertion` \| `auth` \| `input` \| `internal` (the stable taxonomy from `internal/errors`, as documented in the events schema) |

---

## Table of contents

1. [Overview & product principles](#1-overview--product-principles)
2. [Command surface](#2-command-surface)
3. [Architecture](#3-architecture)
4. [HTTP API reference](#4-http-api-reference)
5. [WebSocket protocol](#5-websocket-protocol)
6. [Run orchestration](#6-run-orchestration)
7. [Events schema v1.3 delta](#7-events-schema-v13-delta)
8. [Run history store](#8-run-history-store)
9. [Security model](#9-security-model)
10. [Frontend specification](#10-frontend-specification)
11. [Configuration](#11-configuration)
12. [Tier gating summary](#12-tier-gating-summary)
13. [Testing strategy](#13-testing-strategy)
14. [Milestone decomposition](#14-milestone-decomposition)
15. [Appendix: deferred follow-ups](#15-appendix-deferred-follow-ups)

---

## 1. Overview & product principles

`curlew ui` starts a localhost HTTP server embedded in the existing single Go binary, serving an embedded single-page application. The UI is a **runner and inspector** over the user's YAML files:

- **Files are the only source of truth.** The UI reads collections, environments, and project config from disk; it never edits them. There is deliberately no in-app editor and no fix-it form — every file reference carries an "open in editor" affordance instead.
- **One write, clearly scoped.** The only thing the UI ever writes is its own run-history store under `.curlew/ui/` (§8), which is self-gitignored.
- **The UI is a lens over existing contracts.** Live runs stream over WebSocket as verbatim events-schema objects (§5); project structure comes from the same parser/validator the CLI uses; redaction uses the same `SensitiveSet` machinery. The UI adds a thin HTTP facade — not a second implementation of anything.
- **Redaction is always on.** There is no `--allow-sensitive` for the UI and no reveal toggle in the frontend. Redacted values render as a `[REDACTED]` chip, full stop (§9.4).
- **Everything is available.** Starting runs, inspecting results (including the in-memory ring of the last 5 runs), persisted history, run comparison, and parallel execution are all unconditionally available; the UI passes choices through to the runner and never gates them. (Historical tier-gating text elsewhere in this spec is superseded — see the note at the top.)

### 1.1 Screens (v1)

1. **Live run view** — compact CI-log list (default), with waves-as-columns and swimlane layouts as toggles for parallel runs; streaming status transitions; summary strip.
2. **Response inspector** — Body / Headers / Assertions / Timing / Request tabs, plus an Error tab for `error` outcomes and a skip panel for `skipped`.
3. **Run comparison** — history rail, server-aligned request pairs, client-computed body diff.
4. **Empty-project onboarding** and **validation-error panel** for invalid collections.

### 1.2 Out of scope for v1 (explicit non-goals)

- Editing, creating, or deleting files or requests from the UI (product stance, not a deferral).
- Ad-hoc "send one request" console; request authoring.
- Multi-project switching; the server is rooted in one project per process.
- Team-vault shared templates, plugin hooks, distributed workers, report upload, and perf/load testing in UI-initiated runs (§6.5).
- `--allow-sensitive` passthrough; trend charts over history; saved/exported diff reports; mobile layouts (minimum supported width 960 px — below that a "viewport too narrow" notice renders).
- Themes beyond dark/light; accent color variants stay dormant in the token file.

---

## 2. Command surface

New file `cmd/curlew/ui.go` (precedent: `worker.go`, `perf.go`, `license.go` as sibling command files in package main), following the established triad: `uiCmdOut(args []string, stdout, stderr io.Writer) int`, `parseUIArgs(args []string) (uiFlags, error)`, `printUIHelpTo(w io.Writer)`.

Wiring (all three asserted in sync by the existing `TestUsageSynopsis_MatchesPrintHelpFirstLine` pattern):

- `case "ui":` in the command switch in `runWithWriters` (cmd/curlew/main.go) dispatches directly to `uiCmdOut(args[1:], stdout, stderr)`.
- Entry in the `usageSynopses` map.
- Line in `printHelpTo`: `  ui              Start the local web UI (runner & inspector)`.

### 2.1 Synopsis

```
Usage: curlew ui [--port <n>] [--env <name>] [--collection <file>] [--no-open] [--no-color]
```

### 2.2 Flags

| Flag | Type | Default | Meaning |
|---|---|---|---|
| `--port <n>` | int | config `ui.port`, else **8765** | Listen port. `0` = ephemeral port chosen by the OS (printed to stdout; used by e2e tests). When set explicitly (non-zero) and busy → exit 1. When defaulted and busy → scan ports +1…+19, then exit 1. |
| `--env <name>` | string | empty | Default environment preselected in the UI (surfaced via `/meta`). Each run request carries its own `env`; this only sets the initial selection. Validated against `config.ListAvailableEnvironments`; unknown → exit 3. |
| `--collection <file>` | string | empty = all | Restrict the tree (and watcher) to one collection file. Must resolve inside the project root → else exit 3. |
| `--no-open` | bool | false | Do not launch the browser. The URL (with token) is always printed to stdout. |
| `--no-color` | bool | false | Matches repo convention for stderr output. |
| `--help` / `-h` | | | `printUIHelpTo(stdout)`, exit 0. |

**Deliberately absent flags:**

- `--host` — the server binds loopback only, by construction (§9.1). Remote access is the user's own SSH tunnel.
- `--allow-sensitive` — rejected with exit 1 and the message `the UI always redacts sensitive values`.
- `--parallel` — parallelism is a per-run choice made in the UI, gated server-side per request (§4.7).

**Hidden environment variable:** `CURLEW_UI_DEV_PROXY=http://localhost:5173` — dev-mode reverse proxy to the Vite dev server (§3.4). Precedent for env-gated hidden behavior: `CURLEW_INTERNAL`.

### 2.3 Startup sequence

1. `os.Getwd()` → `config.FindProjectRoot(wd)`. Not found → stderr `no curlew project found (no curlew.yaml in current or parent directories)` → exit 5 (identical to `curlew info`).
2. `config.LoadProjectConfig(root)`; parse failure → exit 3.
3. Resolve port/host per precedence: flag > `ui:` config block > built-in default (§11). Non-loopback host in config → exit 3.
4. `net.Listen("tcp", "127.0.0.1:"+port)`; busy handling per the flags table → exit 1 with a hint naming the busy port.
5. Mint the session token (§9.3), print `curlew ui listening on http://127.0.0.1:<port>/?token=<t>` to stdout, open the browser unless `--no-open` (darwin `open`, linux `xdg-open`, windows `cmd /c start`, via `exec.Command`; best-effort — failure is a stderr warning only).
6. Serve until SIGINT/SIGTERM (`signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` — the watch command handles Interrupt only; `ui` adds SIGTERM for service managers). On signal: cancel any active run, flush the history store, `http.Server.Shutdown` with a 5 s timeout → exit 0.

### 2.4 Exit codes

| Exit | Meaning |
|---|---|
| 0 | clean shutdown (signal) |
| 1 | usage error; port bind failure; fatal server error after start |
| 3 | invalid config (bad `ui:` block, unknown `--env`, `--collection` outside root) |
| 5 | no project found |

Note: the MANUAL's master exit-code table lists 2 for usage errors, but every `*CmdOut` in main.go returns 1 for usage/flag errors today; `ui` follows the de-facto convention (1). Reconciling the MANUAL table is a deferred follow-up (§15).

---

## 3. Architecture

### 3.1 Package layout

```
cmd/curlew/ui.go                 command: flags, help, lifecycle, browser-open
internal/runservice/              extracted run pipeline (§6.1–6.2)
    runservice.go                 Execute(), Request/Result types
    sensitive.go                  BuildPreRunSensitive / BuildPostRunSensitive (extracted from main.go)
    sink.go                       EmitterSink (the events adapter moved out of package main)
internal/uiserver/
    server.go                     Server struct, NewServer(Options), Handler() http.Handler,
                                  middleware (token, host/origin, JSON errors)
    api_meta.go                   /meta, /tree, /environments, /validate, /files, /open
    api_runs.go                   /runs (start/list/detail/cancel), /requests, /body, /events download
    api_compare.go                /compare
    orchestrator.go               single-flight run lifecycle, event log, detail collector
    ws.go                         hub, per-connection pump, replay
    watcher.go                    fsnotify tree watcher
    store.go                      .curlew/ui history store, retention, pruning
    gitinfo.go                    .git/HEAD reader (no shell-out)
    assets/assets.go              //go:embed all:dist
    assets/dist/index.html        committed placeholder (§3.3)
ui/                               Vite + Svelte SPA (§10)
docs/EVENTS_SCHEMA_v1.3.md        new schema doc (§7)
docs/events-schema/v1.3.json      new JSON schema
```

**Routing:** stdlib `http.ServeMux` with Go 1.22 method+wildcard patterns (`mux.HandleFunc("GET /api/v1/runs/{run_id}/requests/{request_id}", …)`). The repo's minimal-dependency policy holds; the only non-stdlib pieces are `github.com/gorilla/websocket` (already a dependency, used by `internal/websocket` for the WebSocket test protocol) and `github.com/fsnotify/fsnotify` (already a dependency, used by `internal/watch`).

### 3.2 Data-flow split (normative)

```
                       ┌────────────────────────────────────────────┐
                       │                Go binary                   │
 browser ── WS ────────┤ orchestrator ── EventLog ── hub            │
   live status only    │      │                                     │
                       │      ├── runservice.Execute ── runner.Run  │
 browser ── REST ──────┤      │                                     │
   everything else     │ DetailCollector (full redacted results)    │
                       │ Store (.curlew/ui, Solo+)                 │
                       └────────────────────────────────────────────┘
```

- **WebSocket carries lifecycle, not payloads.** WS frames wrap verbatim events-schema v1.3 objects, whose bodies are truncated at 2 KiB and redacted — they drive live status transitions and the summary strip only. The frontend never renders event-frame bodies.
- **REST is authoritative for everything else**: full redacted bodies (inline ≤ 256 KiB; raw streaming via the `/body` endpoint beyond that), headers, assertion detail, timing, source snippets — and **wave structure**. The `wave_index` field in events has an `omitempty` quirk (wave 0 is omitted; sequential runs emit −1), so clients must never derive wave grouping from events; `GET /runs/{id}/requests` is the authority.
- **Reconciliation**: after a run reaches a terminal state, the client refetches `GET /runs/{id}` and `GET /runs/{id}/requests` and replaces its live-derived state wholesale (§10.3.4).

### 3.3 Asset embedding

`internal/uiserver/assets/assets.go`:

```go
package assets

import ("embed"; "io/fs")

//go:embed all:dist
var distFS embed.FS

func Dist() fs.FS { sub, _ := fs.Sub(distFS, "dist"); return sub }
```

Served via `http.FileServerFS` with an SPA fallback: any non-`/api` path that doesn't match a file serves `index.html` (the SPA routes via URL hash, so the fallback is only ever hit for `/`). Cache headers: hashed assets `Cache-Control: public, max-age=31536000, immutable`; `index.html` `no-cache`.

**`go build` without a frontend build — committed placeholder.** A minimal `internal/uiserver/assets/dist/index.html` is committed: a static page reading "curlew ui assets are not built into this binary. The API is live; build the UI with `scripts/build-ui.sh`." The root `.gitignore` gains:

```
internal/uiserver/assets/dist/*
!internal/uiserver/assets/dist/index.html
```

Real builds (`cd ui && vite build` → `../internal/uiserver/assets/dist`, wrapped in `scripts/build-ui.sh`) overwrite locally; release/CI pipelines run the UI build before `go build`. *Rejected alternative:* build tags (`//go:build ui_embed`) — they bifurcate the binary matrix and a tag forgotten in a release script ships a UI-less binary silently. The placeholder keeps `go build ./...` always green and the API curl-able without node — every vertical slice stays observable.

### 3.4 Dev mode

`CURLEW_UI_DEV_PROXY=http://localhost:5173 curlew ui` → all non-`/api` paths are reverse-proxied to the Vite dev server via `httputil.ReverseProxy` instead of the embedded FS. Chosen over a Vite-side proxy so the token flow, Host checks, and WS path are identical to production. The complementary frontend dev workflow (Vite proxying `/api` the other way) is described in §10.2.4; both work, the choice is the developer's.

### 3.5 Memory model

- **Active run** (≤1): full event log + growing detail map, in memory.
- **Ring**: the last **5** completed runs stay fully in memory (event log + details), regardless of tier. This is what makes free-tier run+inspect real — including inspecting the run you just made, and re-rendering a finished run's live view from replay.
- **Store** (Solo+): persisted runs beyond the ring are served from `.curlew/ui/` (§8).

---

## 4. HTTP API reference

All endpoints under `/api/v1/`. All responses `application/json; charset=utf-8` unless noted. Every `/api/*` request requires the session token (§9.3). The SPA itself and its static assets are served without the token.

### 4.1 Error envelope

```json
{"error": {"code": "<machine code>", "message": "<human>", "hint": "<optional>", "details": { }}}
```

| HTTP | `code` | When |
|---|---|---|
| 400 | `bad_request` | malformed body/params; empty `rerun_failed` result; path outside project root |
| 401 | `unauthorized` | missing/wrong token |
| 403 | `forbidden_origin` | Host/Origin validation failed (§9.2) |
| 403 | `feature_gated` | tier gate; `details` = the `auth.GateResult` JSON verbatim (`feature`, `required_tier`, `current_tier`, `message`, `upgrade_url`, `trial_available`, `register_for_trial`, `workaround`), mirroring the CLI's gate JSON output |
| 404 | `not_found` | unknown run/request/path |
| 409 | `run_active` | run start while a run is active; `details: {"run_id": "..."}` |
| 409 | `no_editor` | `/open` with no editor configured (§4.13) |
| 422 | `collection_invalid` | parse/validation failure on run start; `details: {"file": "...", "issues": [{"severity","line","message","hint"}]}` (`validator.Issue` shape) |
| 422 | `env_not_found` | `config.ErrEnvironmentNotFound` |
| 500 | `internal` | everything else |

### 4.2 `GET /api/v1/meta`

```json
{
  "server":  {"version": "0.1.0-dev", "api_version": 1, "events_schema_version": "1.3",
              "started_at": "2026-06-11T09:00:00Z"},
  "project": {"root": "/abs/path", "name": "My Project", "default_env": "dev",
              "collection_filter": null},
  "tier": {
    "current": "free",
    "features": {
      "ui_run_history":     {"allowed": false, "required_tier": "solo",
                             "message": "Run history and comparison in curlew ui require Solo tier",
                             "workaround": "Runs remain inspectable for the current session; use --events to persist NDJSON streams yourself"},
      "parallel_execution": {"allowed": false, "required_tier": "professional",
                             "message": "…", "workaround": "…"},
      "test_discovery":     {"allowed": false, "required_tier": "professional",
                             "message": "…", "workaround": "…"},
      "data_driven":        {"allowed": false, "required_tier": "professional",
                             "message": "…", "workaround": "…"}
    }
  },
  "history": {"enabled": false, "max_runs": 50},
  "limits":  {"inline_body_bytes": 262144, "stored_body_bytes": 1048576,
              "event_body_bytes": 2048, "memory_runs": 5}
}
```

- `tier.features` is computed via `auth.CheckFeature(auth.DefaultRegistry(), name, currentTier())` per listed feature; `message`/`workaround` come from the registry's `FeatureDefinition`. The client uses these to render gate chips and panels and to hide the parallel toggle; **the server re-enforces every gate regardless**.
- `history.enabled` = tier allows `ui_run_history` AND config `ui.history.enabled` is not false (§11). The client distinguishes "gated" (feature not allowed) from "disabled by config" (allowed but `enabled:false`) — see §10.6.1.3.
- `project.default_env` comes from `--env`; `collection_filter` from `--collection` (root-relative) or null.
- Never included: environment variable *values*, `os.Environ`, license claims, absolute paths outside the project root.

### 4.3 `GET /api/v1/tree`

Built from `config.ListCollections(root)`, filtered by `--collection` when set. Each file is parsed with `parser.ParseFileWithOptions(path, parser.ParseOptions{})` — **without Include/Schema tier gates** for display purposes (rationale: read-only rendering of the user's own files must never gate-block; gates are enforced at run time exactly as the CLI does). Parse failure → entry with `valid:false` and diagnostics from `validator.ValidateAuto(path, nil)`.

```json
{
  "etag": "9f2c…",
  "collections": [
    {
      "path": "collections/users.yaml",
      "name": "Users API",
      "valid": true,
      "counts": {"setup": 1, "main": 7, "teardown": 1},
      "requests": [
        {"name": "Create user", "slug": "create-user", "phase": "main", "method": "POST",
         "url": "{{base_url}}/users", "source_line": 12, "data_driven": false, "required": false}
      ],
      "issues": []
    },
    {
      "path": "collections/broken.yaml",
      "name": null, "valid": false, "counts": null, "requests": [],
      "issues": [{"severity": "error", "line": 5, "message": "…", "hint": "…"}]
    }
  ]
}
```

- `url` is the **raw uninterpolated template** (`parser` request URL field) — never resolved values.
- `etag` = SHA-256 over sorted (path, mtime, size) tuples; also broadcast in `files.changed` WS frames so clients know when to refetch.

### 4.4 `GET /api/v1/environments`

From `config.ListAvailableEnvironments(root)` + per-env parse:

```json
{"environments": [
  {"name": "dev", "file": "environments/dev.yaml",
   "variables": [{"name": "base_url", "value": "http://localhost:3000", "sensitive": false},
                 {"name": "api_token", "value": "[REDACTED]", "sensitive": true}]}
]}
```

Values pass through the redaction layer with a `SensitiveSet` built from heuristic name patterns over the env map plus the project's configured secret names. **`.env` file contents are never served** — the runner consumes them; the UI does not display them.

### 4.5 `GET /api/v1/validate?path=<root-relative>`

Runs `validator.ValidateAuto` for the named file, or all tree files when `path` is omitted. Response = the exact shape of the CLI's validation JSON output:

```json
{"valid": false, "files": [{"file": "collections/broken.yaml", "valid": false,
  "issues": [{"severity": "error", "line": 5, "message": "…", "hint": "…"}]}]}
```

### 4.6 `GET /api/v1/files?path=<root-relative>`

Read-only file fetch for the inspector's "view source" affordance. Jailed to the project root (`filepath.Clean` + prefix check), `.yaml`/`.yml` only, 1 MiB cap. Returns `text/plain` raw bytes.

### 4.7 `POST /api/v1/runs`

```json
{"collection": "collections/users.yaml", "env": "dev", "parallel": false,
 "mode": "all", "selection": null, "rerun_of": null}
```

- `collection`: root-relative path, or **`null` = run every valid collection** (a *batch run*, §6.4). When more than one valid collection exists, `collection:null` is gated by the existing `test_discovery` feature (Professional) — the same capability the CLI gates for glob runs. With exactly one valid collection, `null` is equivalent to naming it and is not gated.
- `mode`:
  - `"all"` — every request in the target collection(s).
  - `"selection"` — requires non-empty `selection`: exact case-sensitive main-phase request names within **one** collection (`collection` must be non-null), mapped to the runner's `VarSources.Selection` semantics.
  - `"rerun_failed"` — requires `rerun_of` = a known run_id; the server computes the selection as the names of main-phase results with outcome `failed` or `error` in that run (per collection, for batch sources). Empty computed selection → 400.
- `parallel: true` → `auth.CheckFeature(reg, "parallel_execution", tier)`; gated → 403 `feature_gated`.
- A run already active → 409 `run_active` (one run at a time, no queue in v1).
- Success → **202**: `{"run_id": "<32-hex>", "state": "running"}` (id minted via `runner.NewRunID()`).
- Other failures: 422 `collection_invalid` (with validator issues in `details`), 422 `env_not_found`, 400 `bad_request`.

### 4.8 Run status & results

**`GET /api/v1/runs/current`** → `{"run": null}` or:

```json
{"run": {"run_id": "…", "state": "running",
         "params": {"collection": "collections/users.yaml", "env": "dev", "parallel": true,
                    "mode": "all", "selection": null},
         "started_at": "2026-06-11T09:30:00Z", "last_event_id": 17,
         "progress": {"total": 12, "passed": 4, "failed": 1, "skipped": 0, "error": 0, "completed": 5}}}
```

`state` here ∈ `running | cancelling`. Progress counts derive from the detail collector (request.end events seen so far); `total` = planned items after selection filtering, matching `runner.Summary.Total` semantics.

**`GET /api/v1/runs/{run_id}`** → meta + summary for an active, ring, or persisted run:

```json
{"run_id": "…", "state": "completed", "exit_status": "failed", "source": "memory",
 "meta": { …meta.json shape, §8.2… },
 "summary": {"total": 12, "passed": 10, "failed": 1, "skipped": 1, "error": 0,
             "duration_ms": 4810, "parallel": true,
             "wave_count": 3, "max_parallelism": 4, "wave_durations_ms": [1200, 2400, 1210]}}
```

- Summary maps 1:1 from `runner.Summary` (`WaveCount`, `MaxParallelism`, `WaveDurations` → ms ints, `IsParallel`). The `error` count is the subset of failures that were execution errors (computed from results; the runner's summary counts errors within `Failed` — the server splits them when building this payload).
- For **batch runs** (§6.4) the wave fields are omitted (`wave_count`/`max_parallelism`/`wave_durations_ms` absent): waves are per-collection; clients group by `(source_file, wave_index)` from the request list instead.
- `source` ∈ `active | memory | store` — where the data came from (informational).
- `exit_status` ∈ `passed | failed | error | cancelled` (§6.3).

**`GET /api/v1/runs/{run_id}/requests`** → the light list driving the run view:

```json
{"requests": [
  {"request_id": "req-1", "slug": "create-user", "name": "Create user", "phase": "main",
   "method": "POST",
   "outcome": "passed", "status_code": 201, "duration_ms": 184, "wave_index": -1,
   "retry_count": 0, "skip_reason": null, "fail_message": null, "error": null,
   "iteration": null,
   "source_file": "collections/users.yaml", "source_line": 12}
]}
```

- Returned for the **active run too**, from the planner: as soon as a run starts, this returns every planned request with `outcome: null` (pending) — this is how the client seeds wave/phase grouping before the first event arrives (§10.3.3). Entries gain outcomes as request.end events land.
- `wave_index` comes from `RequestResult.WaveIndex` and is authoritative (−1 for sequential runs).
- `skip_reason` is the verbatim runner string (e.g. `dependency "Get Token" failed` (parallel cascade), `parent skipped: Get Token` (sequential `depends_on` cascade), `if: false`, `context cancelled`) — present so the run view renders reasons inline without per-request detail fetches.
- `fail_message` (failed outcomes): the server-rendered first failing assertion, `"<type>: expected <expected>, got <actual>"` — the same compact form the client derives live from `assertion.result` events; this makes the post-run list self-sufficient.
- `error` (error outcomes): the compact form of §4.9's error object — `{"category", "code", "message"}` (no hint).
- `iteration` = `{"index": 1, "total": 3, "base_name": "Create user", "base_slug": "create-user"}` for data-driven expansions, else null. `base_slug` is the slug of the un-expanded request (matches the tree entry); per-iteration `slug` values are iteration-suffixed.
- `source_file`/`source_line` are root-relative; the run view renders the file basename per row and batch runs group by it.

**`POST /api/v1/runs/{run_id}/cancel`** → 202 `{"state": "cancelling"}`. 404 if not the active run; idempotent while cancelling.

### 4.9 `GET /api/v1/runs/{run_id}/requests/{request_id}` — full detail (the inspector)

```json
{
  "request_id": "req-3", "slug": "create-user", "name": "Create user", "phase": "main",
  "method": "POST", "url": "https://api.example.com/users",
  "outcome": "failed", "status_code": 422, "duration_ms": 184, "wave_index": 1,
  "timing": {"dns_us": 1234, "connect_us": 2100, "tls_us": 15400, "ttfb_us": 48200,
             "download_us": 900, "total_us": 68100, "connection_reused": false, "attempts": 1},
  "retry": {"count": 2, "warnings": ["…"],
            "attempts": [{"number": 1, "status_code": 503, "duration_ms": 120, "delay_ms": 0, "error": null}]},
  "skipped": false, "skip_reason": null, "warnings": [],
  "iteration": null,
  "source": {"file": "collections/users.yaml", "line": 12,
             "snippet": ["  - name: Create user", "    request:", "      method: POST"],
             "snippet_start_line": 12},
  "request": {"headers": {"Authorization": "[REDACTED]", "Content-Type": "application/json"},
              "body": {"content": "{\"name\":\"x\"}", "encoding": "", "size": 12,
                       "truncated": false, "content_type": "application/json"}},
  "response": {"headers": {"Content-Type": ["application/json"]},
               "body": {"content": "…", "encoding": "", "size": 48211,
                        "truncated": false, "content_type": "application/json"}},
  "assertions": {"passed": false,
    "items": [{"type": "status", "expected": "201", "actual": "422", "passed": false}]},
  "error": null
}
```

Field sources (all existing runner types): `runner.RequestResult` — `RequestHeaders`, `RequestBody`, `Result.Headers`/`Result.Body`/`Result.StatusCode`/`Result.Duration`, `AssertionResults.Items` (`assertion.Result{Type, Expected, Actual, Passed}`), `RetryCount`, `RetryWarnings`, `AttemptDetails` (`retry.AttemptDetail{Number, StatusCode, Duration, Delay, Err}`), `WaveIndex`, `SkipReason`, `Warnings`, data-driven fields, `SourceFile`/`SourceLine`.

- `url` is the **resolved, redacted** as-sent URL; the raw template is available from the tree.
- `timing` per §7; null for skipped requests and for protocols without phase capture (WebSocket/GraphQL).
- `error` (for `outcome: "error"`): `{"category": "network", "code": "NETWORK_CONNECTION_REFUSED", "message": "<verbatim>", "hint": "<optional>"}` — `category` is one of the stable error taxonomy values `parse|network|config|assertion|auth|input|internal` (from `internal/errors`, per the events schema's error documentation); `code` is the machine code when one exists (e.g. `NETWORK_DNS`, `NETWORK_TIMEOUT`), else absent.
- Request headers redacted via `variable.RedactHeaders`; response header *values* pass value-based redaction; bodies via `variable.RedactBody` — always with `allowSensitive=false`.
- **Mid-run availability:** the detail collector populates entries from `RequestEndEvent` as requests finish, so the inspector works while the run is still going (§6.2).

**Body size policy (normative):**

| Path | Limit | Behavior over limit |
|---|---|---|
| WS event bodies | 2 KiB (`events.DefaultBodyLimit`) | truncated by the emitter (`*_truncated`, `*_size`); never rendered by the UI |
| REST detail inline | 256 KiB per body | `content` omitted, `truncated: true`, `size` = real size; client fetches via `/body` |
| `/body` raw endpoint | full in-memory body (active/ring); 1 MiB for persisted runs | persisted bodies beyond 1 MiB were truncated at store time (§8.3) |
| Binary bodies | same limits | inline as `encoding: "base64"`; raw endpoint streams original bytes |

### 4.10 `GET /api/v1/runs/{run_id}/requests/{request_id}/body?which=request|response`

Streams the full **redacted** body bytes with the original `Content-Type` and `Content-Disposition: attachment; filename="<slug>-<which>.<ext>"`. Used by the frontend for >256 KiB bodies and the Download button.

### 4.11 History (Solo) — list, delete, raw events

New feature registry entry in `internal/auth/registry.go` `DefaultRegistry()`:

```go
r.Register(FeatureDefinition{
    Name:         "ui_run_history",
    RequiredTier: TierSolo,
    Description:  "Run history and comparison in curlew ui require Solo tier",
    Workaround:   "Runs remain inspectable in the UI for the current session; use --events to persist NDJSON streams yourself",
})
```

Solo is the right tier by registry convention: Solo houses individual-productivity persistence/convenience features (`vault_provider_profiles`, `dynamic_auth_profiles`, `retry`); Professional gates protocol/scale features. One feature covers both history and comparison — comparison is meaningless without history.

- **`GET /api/v1/runs?limit=50&offset=0`** → 403 `feature_gated` when not entitled; else `{"runs": [ …meta.json shapes, newest first… ], "total": 37}` from the store.
- **`DELETE /api/v1/runs/{run_id}`** → delete a persisted run directory (Solo). 404 for memory-only ids.
- **`GET /api/v1/runs/{run_id}/events`** → raw `application/x-ndjson` stream of the run's stored or in-memory event log. Free for ring runs; persisted runs are implicitly Solo.

The free-tier surface is unaffected: `GET /runs/{id}`, `/requests`, `/requests/{rid}`, `/body` all work for the active run and the 5-run memory ring at any tier. Only the persisted *list*, *delete*, and *compare* are gated. Below Solo, the store is never constructed and **nothing is written to `.curlew/ui/`**.

### 4.12 `GET /api/v1/compare?base=<run_id>&target=<run_id>` (Solo)

```json
{
 "base": { …run summary as §4.8… }, "target": { … },
 "pairs": [
   {"slug": "create-user", "iteration": null, "name": "Create user", "method": "POST",
    "base":   {"request_id": "req-3", "outcome": "failed", "status_code": 422,
               "duration_ms": 184, "timing": { … },
               "fail_message": "body: expected 100, got 95", "error": null},
    "target": {"request_id": "req-2", "outcome": "passed", "status_code": 201,
               "duration_ms": 142, "timing": { … }, "fail_message": null, "error": null},
    "delta":  {"duration_ms": -42, "outcome_changed": true, "status_changed": true,
               "timing_us": {"dns": -3, "connect": 0, "tls": 0, "ttfb": -3800,
                             "download": -12, "total": -42000}}}
 ],
 "only_in_base":   [{"slug": "old-test", "name": "Old test", "method": "GET", "request_id": "req-9"}],
 "only_in_target": []
}
```

Each pair carries `name`/`method` (and each side `fail_message`/`error` per the §4.8 compact forms) so the picker and run-header cards render without backfilling from the tree — which would fail for `only_in_base` entries, whose requests no longer exist in the current files.

**Division of labor (normative): the server computes alignment and numeric deltas; the client computes body text diffs.** Pairing key = `(slug, iteration index)` — slugs are canonical, stable, and frozen by the events schema's slug-derivation rule. Rationale: alignment requires both runs' full result sets (server-side data), is cheap, and must be consistent everywhere; body *text* diffing is presentation-specific, the Go stdlib has no diff implementation, and the minimal-dependency policy argues against adding one. The client fetches both bodies through §4.9/§4.10 on demand and diffs locally (§10.6.4.5).

### 4.13 `POST /api/v1/open`

```json
{"file": "collections/users.yaml", "line": 12}
```

Launches the user's editor at file:line. Editor resolution order: `$CURLEW_EDITOR` → config `ui.editor` (§11) → `code --goto <abs>:<line>` as a last-resort attempt. `$VISUAL`/`$EDITOR` are deliberately **not** consulted — they typically name terminal editors, which cannot be sensibly spawned from a detached server process. The editor value is executed as a command template: `{file}`/`{line}` placeholders substituted when present, else `<cmd> <abs-path>:<line>` appended.

- `204` — launched (fire-and-forget; non-zero editor exit is not detected).
- `409 no_editor` — nothing configured and `code` not on PATH; `hint: "set ui.editor in curlew.yaml or $CURLEW_EDITOR"`.
- `file` is jailed to the project root; outside → 400.

Token-guarded like every `/api` route. This endpoint exists because client-side `vscode://` URL schemes cover only some editors and fail silently when unhandled.

---

## 5. WebSocket protocol

Endpoint: `GET /api/v1/ws?token=<t>` (token via query param — browsers cannot set headers on WS upgrades). Upgrader: gorilla `websocket.Upgrader` with a custom `CheckOrigin`: accept when `Origin` is absent (non-browser clients) or parses to scheme http/https with hostname ∈ {`localhost`, `127.0.0.1`, `::1`} (port ignored).

### 5.1 Frame envelopes

Server → client (one JSON object per text frame):

```json
{"type": "hello", "data": {"proto": 1, "server_version": "0.1.0-dev",
  "run": {"run_id": "…", "state": "running", "last_event_id": 17},
  "tree_etag": "9f2c…"}}

{"type": "run.event", "run_id": "…", "data": { …verbatim events-schema v1.3 object… }}

{"type": "run.state", "run_id": "…",
 "data": {"state": "running|cancelling|completed|cancelled|error", "exit_status": "failed"}}

{"type": "files.changed", "data": {"paths": ["collections/users.yaml"], "tree_etag": "a01b…"}}

{"type": "error", "data": {"code": "bad_subscribe", "message": "…"}}

{"type": "pong", "data": {}}
```

Client → server:

```json
{"type": "subscribe", "run_id": "…", "from_id": 17}
{"type": "ping"}
```

`run.event.data` is the **unmodified events-schema object** — the same bytes the NDJSON emitter produced — so any v1.x consumer logic works identically on `--events` files and the WS stream. `run.state` frames are UI-protocol additions outside the events schema (the schema has no "cancelling" notion).

### 5.2 Lifecycle, replay, reconnection, multiple tabs

- On connect the server immediately sends `hello`. If a run is active, the client sends `subscribe` with `from_id` (0 for "everything", or its last-seen event `id` after a reconnect).
- **Replay-from-event-id (normative):** the orchestrator's `EventLog` retains every emitted NDJSON line of the active run in memory, indexed by the emitter's strictly monotonic `id`. On `subscribe`, under the hub lock, the server replays all log entries with `id > from_id`, then atomically attaches the connection to the live broadcast — no gap, no duplicate. Memory bound: event bodies are 2 KiB-truncated, so a 1,000-request run is ≈ 3–6 MB; the log is released when the run leaves the memory ring.
- Subscribing to a **completed** ring or persisted run replays the whole stored stream and then sends a terminal `run.state` — this is how a freshly opened tab renders a finished run's live-view timeline.
- **Multiple tabs:** the hub broadcasts every frame to all connections; each connection has a 256-frame buffered send channel; a full buffer marks the connection slow and closes it (the client auto-reconnects and catches up via replay). No per-tab server state beyond the connection itself.
- **Keepalive:** server-side control pings every 30 s, read deadline 90 s. The JSON `ping`/`pong` is an app-level liveness aid for the SPA.
- `files.changed` frames go to all connections regardless of subscriptions.

---

## 6. Run orchestration

### 6.1 The extraction from main.go (scope-honest)

The CLI's `runCmdInner` is a ~1,300-line monolith interleaving flag parsing, events-file management, per-format error rendering, glob discovery, dry-run, team vault, plugins, telemetry, and the actual pipeline. Rewriting it in this milestone is high-risk and unnecessary. **The minimal extraction is the load-and-run pipeline plus two helpers that are currently inline:**

1. **`internal/runservice.Execute`** — the happy-path pipeline: `parser.ParseFileWithOptions` (with `IncludeGate`/`SchemaGate` built from tier — the two small gate-constructor funcs move into runservice) → `config.LoadEnvironment` → `config.LoadProjectConfig` → `config.LoadDotenv` → pre-run `SensitiveSet` → `runner.Run`.
2. **`internal/runservice/sensitive.go`** — `BuildPreRunSensitive(...)` and `BuildPostRunSensitive(...)`, extracted verbatim from the two inline blocks in main.go (post-run includes `summary.AuthSensitive`/`RuntimeSensitive`).
3. **`internal/runservice/sink.go`** — `EmitterSink`, the events adapter moved out of package main unchanged in behavior (including the assertion-failure sentinel injection). `cmd/curlew` switches to the moved type — a mechanical edit; everything else in `runCmdInner` stays untouched this milestone. Converging `runCmdInner` itself onto `runservice.Execute` is an explicitly deferred follow-up (§15).

### 6.2 `internal/runservice` API

```go
type Request struct {
    CollectionPath string           // absolute (caller resolves against project root)
    EnvName        string
    Selection      []string         // nil = all (VarSources.Selection semantics)
    Parallel       bool
    Seed           *int64           // nil in UI v1
    RunID          string           // required; caller mints via runner.NewRunID()
    RequestIDPrefix string          // "" for single runs; "c<i>-" per collection in batch runs (§6.4)
    Tier           auth.Tier
    Sink           runner.EventSink // may be nil
    Diagnostics    io.Writer
}

type Result struct {
    Collection   *parser.Collection
    ProjectCfg   *config.ProjectConfig
    Results      []runner.RequestResult
    Summary      *runner.Summary
    PreSensitive *variable.SensitiveSet // what the sink used mid-run
    Sensitive    *variable.SensitiveSet // post-run full set (incl. Auth/RuntimeSensitive)
}

func Execute(ctx context.Context, req Request, exec runner.ExecuteFunc) (*Result, error)
```

`exec` is injectable (`httpexec.Execute` in production) so orchestrator tests run against fakes, exactly as runner tests do. Error contract: `*auth.GateError` passes through unwrapped (→ 403); parse errors → 422 `collection_invalid` (the handler runs `validator.ValidateAuto` to enrich `details`); `config.ErrEnvironmentNotFound` → 422; no-matching-requests → 400.

**Required additive change to `runner.RequestEndEvent`** (for mid-run inspection):

```go
RequestHeaders  map[string]string // interpolated request headers (same data as RequestResult.RequestHeaders)
ResponseHeaders http.Header       // from httpexec.Result.Headers; nil on error/skip
Timing          *httpexec.Timing  // §7; nil when unavailable
Attempts        int               // retry.Outcome.Attempts; 1 when no retry
```

Populated at every `OnEvent.RequestEnd` emission site in the runner and in the parallel-executor sink path. Additive struct fields; the existing CLI emitter ignores the header fields — **headers stay out of the NDJSON events schema in v1.3** (they would bloat the stream; the REST detail covers them). The emitter maps `Timing`/`Attempts` into the new v1.3 `timing` event field.

### 6.3 Orchestrator

```go
type Orchestrator struct {
    mu     sync.Mutex
    active *ActiveRun        // nil when idle
    ring   []*CompletedRun   // last 5, newest first
    store  *Store            // nil when history not entitled
    hub    *Hub
    exec   runner.ExecuteFunc
    tier   auth.Tier
    root   string
}

type ActiveRun struct {
    RunID     string
    Cancel    context.CancelFunc
    State     string           // running | cancelling
    Params    StartParams
    StartedAt time.Time
    Log       *EventLog        // ordered raw event lines + last id
    Details   *DetailCollector // request_id → *RequestDetail (built from sink)
}
```

**Start** (`POST /runs`): under `mu`, reject if `active != nil` (409). Mint run id. Build the **sink fan-out** (implements `runner.EventSink`):

1. `runservice.EmitterSink` over an `events.Emitter` whose `io.Writer` is the `EventLog` — each write parses the line's `id`, appends, and broadcasts a `run.event` frame. Emitter options: `{CurlewVersion, RunID}` as the CLI sets them.
2. `DetailCollector` — on `RequestStart` records identity/phase/source; on `AssertionResult` appends assertion items; on `RequestEnd` stores redacted full bodies (via `variable.RedactBody` + the `PreSensitive` set), redacted headers, timing, outcome. The emitter's redaction and the collector's redaction share the **same** `PreSensitive` set built by runservice before `runner.Run`.

The run executes on a goroutine: `runservice.Execute(runCtx, …)`. On return: build a `CompletedRun` from the **authoritative** `[]RequestResult`/`Summary` (this overwrites collector entries — picking up the post-run `Sensitive` set including `AuthSensitive`/`RuntimeSensitive` for the stored copy, plus `WaveIndex`, retry details, and skip cascades), persist via `store` if non-nil, push into the ring (evict beyond 5), broadcast the terminal `run.state`.

`exit_status` derivation: `error` when Execute returned a pre-execution error; `cancelled` when `ctx.Err() != nil`; else `failed` if `Summary.Failed > 0`; else `passed`.

**Cancel**: matches the active id → `Cancel()`, state `cancelling`, broadcast. The runner honors context cancellation (cancelled items come back `Skipped` with reason `context cancelled`).

**Server shutdown**: cancel the active run, wait ≤ 5 s for the goroutine, flush the store, `http.Server.Shutdown`.

### 6.4 Batch runs (`collection: null`)

A batch run executes every valid collection sequentially in tree order under **one** run_id, one event stream, and one aggregated summary. This is schema-compatible by design: `run.start`/`run.end` are command-level boundaries (they carry `cli_args`) and `collection_file` is optional, so a multi-collection stream is valid. (Note: CLI glob runs do **not** do this today — `runDiscoveredCollections` invokes the run pipeline once per matched file, each with its own run_id and emitter. Batch runs are a new composition the schema permits, not a reuse of glob behavior.)

- The orchestrator plays the "command" role: it emits `run.start` once (with `collection_file` omitted, as the schema allows), invokes `runservice.Execute` per collection with the shared sink (request/assertion events flow through; per-collection run boundaries are not emitted), then emits one aggregated `run.end`.
- The shared emitter keeps event `id`s monotonic across collections. `request_id`s, however, are minted inside `runner.Run` with a per-invocation counter (each collection would restart at `req-1` and collide in the event stream and the detail map). To prevent this, `runner.VarSources` gains an **additive** `RequestIDPrefix string` field: when non-empty, request ids are `<prefix>req-N`. The orchestrator passes `c1-`, `c2-`, … per collection in batch runs (empty for single-collection runs — existing id format unchanged). `request_id` is an opaque pairing string per the events schema, so prefixed ids are schema-legal.
- **Gate:** when more than one valid collection exists, batch requires `test_discovery` (Professional) — CLI parity with glob runs. With exactly one valid collection, no gate.
- **Summary:** totals are summed; the per-run wave fields (`wave_count`, `max_parallelism`, `wave_durations_ms`) are omitted — wave indices are collection-scoped, so clients group by `(source_file, wave_index)` from the request list (§4.8).
- Invalid collections are excluded from the batch (they are visible as invalid in the tree); if *all* collections are invalid, the start returns 422 for the first.
- `parallel` applies within each collection (waves are computed per collection, as the runner does today).

### 6.5 Excluded from UI-initiated runs in v1 (documented, not silent)

Team-vault shared templates (Team tier; requires backend-cache plumbing from the cmd layer), plugin hooks (Enterprise, `CURLEW_PLUGINS`), report upload (`pr-check`), distributed workers, and the interactive large-dataset confirmation prompt (`ConfirmLargeDataset: false` → oversized data-driven sets fail with the runner's guard error, surfaced as a run error). `VarSources` is populated exactly as the CLI run path does minus team/hooks/telemetry: `Project, EnvFile, DotEnv, Seed, Tier, Secrets, AuthProfiles, ProjectRoot, GlobalRetry, GlobalGraphQL, Parallel, CollectionDir, OnEvent, Selection, RunID, Diagnostics`.

---

## 7. Events schema v1.3 delta

A backward-compatible, additive release following the documented evolution conventions (additive optional fields, changelog section, version-constant bump — the same pattern as v1.1 → v1.2). Ships as its own vertical slice (§14, slice 2): it benefits plain `curlew run --events` users independent of the UI.

### 7.1 httptrace capture in `internal/httpexec`

```go
// Timing captures connection-phase durations for a single HTTP exchange,
// measured via net/http/httptrace. Zero-value phase fields with Reused=true
// mean the phase did not occur (pooled connection).
type Timing struct {
    DNS      time.Duration // DNSStart → DNSDone
    Connect  time.Duration // ConnectStart → ConnectDone (final address)
    TLS      time.Duration // TLSHandshakeStart → TLSHandshakeDone
    TTFB     time.Duration // WroteRequest → GotFirstResponseByte
    Download time.Duration // GotFirstResponseByte → response body fully read
    Total    time.Duration // Do() entry → body fully read (note: Result.Duration
                           // is measured around Do() only and excludes body read;
                           // its semantics are unchanged — golden output depends on it)
    Reused   bool          // GotConnInfo.Reused
}
```

`httpexec.Result` gains `Timing *Timing`. Implementation: build an `&httptrace.ClientTrace{…}` capturing timestamps (DNS callbacks may fire on other goroutines — guard with a mutex), wrap the context via `httptrace.WithClientTrace` before `http.NewRequestWithContext`. WebSocket-protocol and GraphQL requests get `Timing == nil` — the field is optional everywhere downstream.

**Retry interaction:** `retry.Outcome.Result` is the final attempt's result, so `Timing` reflects the **final attempt only**. The `Attempts` count (1 = no retry) rides along. Per-attempt durations already exist in `AttemptDetails` and are exposed via REST detail; per-attempt phase traces would multiply event size for marginal value.

### 7.2 Schema changes

- `internal/output/events/events.go`: `SchemaVersion = "1.3"`; package comment gains a `v1.2 → v1.3: additive change` paragraph.
- New optional object on `request.end`:

```go
// TimingInfo is the optional request.end connection-phase breakdown. Added in v1.3.
// All duration fields are integer microseconds. Phase fields are omitted when the
// phase did not occur (e.g. pooled connection: no dns/connect/tls).
type TimingInfo struct {
    DNSUs      *int64 `json:"dns_us,omitempty"`
    ConnectUs  *int64 `json:"connect_us,omitempty"`
    TLSUs      *int64 `json:"tls_us,omitempty"`
    TTFBUs     *int64 `json:"ttfb_us,omitempty"`
    DownloadUs *int64 `json:"download_us,omitempty"`
    TotalUs    int64  `json:"total_us"`
    Reused     bool   `json:"connection_reused,omitempty"`
    Attempts   int    `json:"attempts,omitempty"` // >1 only when retries occurred
}

// on RequestEnd:
Timing *TimingInfo `json:"timing,omitempty"` // v1.3 additive
```

**Units decision:** integer **microseconds** (`_us`) rather than the stream's `_ms` convention — millisecond resolution is useless for localhost APIs where DNS/connect take tens of µs; integers avoid float-comparison noise in golden tests; the suffix makes the unit unambiguous. Pointer + `omitempty` distinguishes "0 µs" from "did not occur".

- `RequestEndInput` gains `Timing *TimingInfo`; `EmitRequestEnd` copies it. The emitter sink converts `httpexec.Timing` + `Attempts` → `TimingInfo`.
- New `docs/EVENTS_SCHEMA_v1.3.md` (full document; changelog section mirroring the v1.1→v1.2 style: "backward-compatible, additive release … new optional `request.end.timing` object … `schema_version` changes to \"1.3\"") and `docs/events-schema/v1.3.json` (v1.2 schema + the `timing` object definition). v1.2 files are retained as historical anchors per convention. The events schema test updates to validate against v1.3.

### 7.3 Quirks documented in v1.3 (no emission changes)

- **`wave_index` omitempty:** `request.end.wave_index` is omitted when the value is 0 (first wave) while sequential runs emit −1. Events alone cannot distinguish "first wave" from "absent"; consumers needing wave structure must use a result-level source (for the UI: the REST request list). v1.3 documents the quirk; emission is unchanged (golden-test stability).
- **Phase strings:** the v1.2 document describes `phase` as `setup|test|teardown`, but the runner emits `setup|main|teardown` (`runner.PhaseMain = "main"`). v1.3 corrects the documentation to match the emitter. (No code change.)

---

## 8. Run history store (`.curlew/ui/`)

### 8.1 Layout

```
<projectRoot>/.curlew/ui/
  .gitignore                 # single line: *
  runs/
    <run_id>/
      meta.json
      events.ndjson          # full v1.3 stream (redacted, 2 KiB-truncated bodies)
      detail.json            # array of request-detail objects (§4.9 shape), bodies ≤ 1 MiB each
```

Written **only** by the UI server — plain `curlew run` remains persistence-free. Writes are atomic: write `<run_id>.tmp/`, then `os.Rename`. `.curlew/` already exists as a tool-owned directory (auth cache); the UI nests under `.curlew/ui/` to avoid collisions.

**gitignore handling:** on first creation of `.curlew/ui/`, the store writes `.curlew/ui/.gitignore` containing `*` (the self-ignoring pattern used by terraform and package-manager caches). This is required, not belt-and-braces: the project scaffolder only adds `.curlew/` to the project `.gitignore` when `--skill` was used, so many existing projects don't ignore it. Making `init` always ignore `.curlew/` is a deferred follow-up (§15).

### 8.2 `meta.json` (store schema v1)

```json
{
  "schema_version": 1,
  "run_id": "a1b2…(32-hex)",
  "created_at": "2026-06-11T09:30:00Z",
  "curlew_version": "0.1.0-dev",
  "events_schema_version": "1.3",
  "collection_file": "collections/users.yaml",
  "collection_name": "Users API",
  "env_name": "dev",
  "selection": null,
  "parallel": true,
  "exit_status": "failed",
  "git": {"branch": "main", "commit": "0f3a…(40-hex) or null"},
  "summary": {"total": 12, "passed": 10, "failed": 1, "skipped": 1, "error": 0,
              "duration_ms": 4810,
              "wave_count": 3, "max_parallelism": 4, "wave_durations_ms": [1200, 2400, 1210]}
}
```

For batch runs, `collection_file`/`collection_name` are null and the wave fields are omitted (§6.4). `schema_version` is an integer, store-local: readers skip directories with an unknown major version; additive fields don't bump it.

**Git metadata — read `.git/HEAD` directly, no shell-out.** Rationale: minimal-dependency policy, works without a git binary, deterministic, no per-run process spawn. Algorithm (`gitinfo.go`): walk up from the project root for `.git`; if it is a file, parse `gitdir: <path>` (worktrees); read `HEAD`: `ref: refs/heads/<branch>` → branch, then best-effort commit from `.git/<ref>` or `packed-refs`; detached HEAD → `commit` = the content, `branch` = null. Any failure → `"git": null`. No `dirty` flag (would require status computation).

### 8.3 Caps & retention

- Per-body storage cap **1 MiB** (larger bodies stored truncated with `truncated: true, size: <real>` — the inspector shows the truncation panel for such historical runs). Rationale: 50 runs × dozens of requests must stay in the tens-of-MB range; inspecting >1 MiB historical payloads is an edge case.
- Retention: after each persist, list `runs/*/meta.json` by `created_at` (fallback: directory mtime) and delete the oldest beyond `max_runs` (default **50**, config `ui.history.max_runs`, §11). Unreadable/corrupt run directories are skipped for serving and counted for pruning.
- `events.ndjson` is naturally bounded by the 2 KiB event body limit.

### 8.4 Gating

The store is constructed at server start only if `auth.CheckFeature(reg, "ui_run_history", tier)` passes and config `ui.history.enabled` is not false. Otherwise `store == nil`: zero filesystem writes, and the history/compare endpoints return 403 `feature_gated` (or, when config-disabled on an entitled tier, an empty list with `history.enabled:false` already visible in `/meta`).

---

## 9. Security model

1. **Loopback-only bind, by construction.** The listener is literally `net.Listen("tcp", "127.0.0.1:"+port)`. Config `ui.host` accepts only `127.0.0.1`, `localhost` (resolved to 127.0.0.1), or `::1`; anything else is a startup error (exit 3). No flag can widen it. Remote use is the user's own SSH tunnel.
2. **Host-header validation** middleware on every request: the hostname part of `Host` must be `localhost`, `127.0.0.1`, or `[::1]`, else 403 `forbidden_origin`. This is the DNS-rebinding defense — a rebound hostname arrives carrying the attacker's Host. WS upgrades additionally validate `Origin` (§5).
3. **Session token (v1 ships it).** 32 hex chars from `crypto/rand`, minted per server start. Required on every `/api/*` request — `Authorization: Bearer <t>` (or `X-Curlew-UI-Token`); WS upgrade via `?token=`. The SPA boots from `http://127.0.0.1:<port>/?token=<t>` (printed and auto-opened), moves the token to `sessionStorage`, and strips it from the URL via `history.replaceState`. Static assets are served without the token (they contain nothing sensitive). Justification: loopback + Host checks do not protect against *other local users* on shared machines, and the token also closes CSRF completely; the cost is ~50 lines.
4. **Redaction always on.** No `--allow-sensitive` for the UI. Every body/header/value leaving the server passes `variable.RedactBody`/`RedactHeaders`/value redaction with `allow=false`; persisted artifacts are redacted *before* hitting disk. The frontend's redacted chips intentionally have no reveal — a reveal would require persisting or transporting unredacted data, both rejected.
5. **CSRF posture:** the token requirement on all endpoints defeats cross-origin form/fetch attacks (an attacker cannot read the token). Additionally, `Content-Type: application/json` is enforced on POST/DELETE, and Origin (when present) is validated.
6. **Never exposed:** `os.Environ`; `.env` file contents; `secrets:`/vault configuration values; license JWTs/claims; absolute paths outside the project root (all file params jailed via `filepath.Clean` + root-prefix check; `/api/v1/files` additionally restricted to `.yaml`/`.yml`).
7. WS frames carry only what REST already allows.

---

## 10. Frontend specification

The SPA lives in a new top-level `ui/` directory (the existing `web/` is the team-tier cloud dashboard — a different product surface; do not merge them).

### 10.1 Stack decisions

**Framework: Svelte 4 + Vite (plain SPA, no SvelteKit), TypeScript.** The repo's `web/` is SvelteKit 2 / Svelte 4 / TS, so the team already knows Svelte idioms, ESLint config, and `svelte-check`. SvelteKit itself is wrong here: there is no server runtime (the Go binary is the server), no SSR, and no file-based routing need — plain Vite produces exactly one `index.html` + hashed assets, ideal for `go:embed`. Svelte stores fit the WS-event-driven state model: a `writable` map updated by the event reducer gives O(changed-row) DOM updates. Rejected: React/Preact (new ecosystem for this repo), SolidJS, vanilla (the JSON tree, diff view, and live run view are too stateful).

**Styling: plain CSS with the design-token file — no Tailwind.** The prototype's `at-tokens.css` is adopted verbatim as `ui/src/styles/tokens.css` (plus §10.5.2 additions): it is already a complete design system (density variables, dark/light themes, status colors, primitives `.at-btn`, `.at-chip`, `.at-tabs`, `.at-menu`, `.at-input`, `.at-dot`, `.at-redacted`, `.at-spin`). Component styles are Svelte-scoped `<style>` blocks. The consistency cost with `web/` (Tailwind) is acceptable: `ui/` is a devtools-dense product surface with its own design language.

**Runtime dependencies: none beyond Svelte.** The diff (§10.6.4.5), JSON tree, waterfall, and router are hand-rolled — the prototype already proves the JSON tree in ~150 lines. Dev dependencies: `svelte`, `@sveltejs/vite-plugin-svelte`, `vite`, `typescript`, `svelte-check`, `vitest`, `jsdom`, `@testing-library/svelte`, `@playwright/test`, `eslint` (flat config, conventions from `web/eslint.config.js`). Versions align with `web/package.json` (Svelte 4, Vite 5, Vitest 2, Playwright 1.4x).

### 10.2 Package layout, types, build

```
ui/
  package.json  vite.config.ts  tsconfig.json  svelte.config.js  playwright.config.ts
  index.html
  public/fonts/                       # self-hosted IBM Plex woff2 (§10.8.3)
  src/
    main.ts                           # token strip, boot sequence (§10.4.3)
    App.svelte                        # shell: TopBar + Sidebar + route outlet
    styles/tokens.css                 # at-tokens.css verbatim + §10.5.2 additions
    styles/fonts.css
    lib/
      router.ts                       # hash router (~80 lines, store-backed)
      run-machine.ts                  # §10.3.2 pure FSM
      event-reducer.ts                # §10.3.3
      diff.ts                         # Myers line diff (§10.6.4.5)
      format.ts                       # fmtMs, fmtBytes, fmtUs
      api/
        client.ts                     # fetch wrapper: bearer auth, ApiError
                                      # (mirrors web/'s api-error pattern: code/status/details)
        meta.ts tree.ts runs.ts environments.ts compare.ts validate.ts open.ts
      ws.ts
      types/
        meta.ts tree.ts run.ts events.ts compare.ts   # 1:1 mirrors of server JSON, snake_case kept
      stores/                          # §10.3.1
      components/
        atoms/    Dot.svelte Method.svelte Code.svelte Chip.svelte Redacted.svelte
                  Tabs.svelte Menu.svelte CopyBtn.svelte Btn.svelte Icon.svelte
                  OpenInEditor.svelte PhaseBadge.svelte RetryBadge.svelte TemplateUrl.svelte
        chrome/   TopBar.svelte Sidebar.svelte EnvMenu.svelte RunSplitButton.svelte
                  WatchIndicator.svelte WsBanner.svelte HelpOverlay.svelte
        run/      RunView.svelte SummaryStrip.svelte RequestRow.svelte RequestCard.svelte
                  WaveHeader.svelte PhaseHeader.svelte CollectionHeader.svelte
                  LayoutToggle.svelte RunErrorBanner.svelte
        inspector/ Inspector.svelte BodyTab.svelte JsonTree.svelte HeadersTab.svelte
                   AssertionsTab.svelte TimingTab.svelte RequestTab.svelte ErrorTab.svelte
                   BinaryPanel.svelte SourceSnippet.svelte
        compare/  Compare.svelte HistoryRail.svelte RunHeaderCard.svelte DiffView.svelte
                  RequestPicker.svelte
        states/   EmptyProject.svelte ValidationPanel.svelte GatePanel.svelte
                  Disconnected.svelte NotFoundPanel.svelte
  tests/e2e/                           # Playwright (§13.3)
  tests/fixtures/project/              # fixture curlew project for e2e
```

Types stay snake_case matching the wire format (no mapping layer); `types/events.ts` defines a discriminated union on the WS frame `type` and on the v1.3 event `type`.

**Build:** `vite build` with `outDir: '../internal/uiserver/assets/dist'`, `emptyOutDir: true`, `base: './'` (assets resolve regardless of mount path). `scripts/build-ui.sh` runs `npm ci && npm run build` before `go build` in release/CI pipelines.

**Dev mode:** `npm run dev` starts Vite on :5173 with `server.proxy` for `/api` → `http://127.0.0.1:8765` (`ws: true` for `/api/v1/ws`). Workflow: run `curlew ui --no-open` in a fixture project, copy the token, open `http://localhost:5173/?token=…`. (The server-side `CURLEW_UI_DEV_PROXY` mode, §3.4, is the inverse arrangement; both work.)

### 10.3 App state model

#### 10.3.1 Store layout (`ui/src/lib/stores/`)

| File | Store(s) | Contents |
|---|---|---|
| `meta.ts` | `meta` | `/meta` payload; derived `feature(name)` helper → `{allowed, required_tier, message, workaround}` |
| `tree.ts` | `tree`, `treeFilter` | `/tree` payload + etag; `refreshTree()` guarded by etag comparison |
| `environments.ts` | `environments`, `selectedEnv` | `/environments` payload; selected env name (sessionStorage) |
| `run.ts` | `runState`, `requests`, `summary`, `runMeta` | run lifecycle machine + per-request map (§10.3.2–10.3.4) |
| `history.ts` | `runs`, `historyTotal` | Solo-only; holds the 403 gate marker when not allowed |
| `compare.ts` | `compareSelection`, `comparePairs` | base/target ids, `/compare` result, fetched bodies, computed diffs |
| `connection.ts` | `wsStatus`, `serverReachable` | `'connecting'\|'open'\|'reconnecting'\|'dead'`; drives banners (§10.6.7) |
| `ui.ts` | `prefs`, `helpOpen`, `focusZone` | theme/density/layout prefs, chrome state |
| `toast.ts` | `toasts` | transient notices (copy confirmations, 409, open-in-editor results) |

Persisted client prefs in `localStorage['curlew.prefs']` (JSON): `theme`, `density`, `runLayout` (`compact|columns|lanes`), `changesOnly`, `parallel`. Environment selection is per-session (`sessionStorage`), defaulting to `meta.project.default_env`.

#### 10.3.2 Run lifecycle state machine (`run-machine.ts`)

```
idle ──Run click──► starting ──202──► running ──run.state:cancelling──► cancelling
  ▲                    │                  │                                  │
  │                  409/403/422          ├─run.state:completed──► completed │
  │                    ▼                  ├─run.state:error─────► error ◄────┤
  └──── new Run ◄── idle(+toast/panel)    └─run.state:cancelled─► cancelled ◄┘
```

- **idle**: no run this session, or viewing a historical run. Run button enabled.
- **starting** (optimistic UI): entered synchronously on Run click. Run button shows a spinner + "Starting…" and is disabled; tree dots in the selection flip to `pending`; the summary strip renders 0/N. `POST /runs` is in flight.
  - `202 {run_id}` → `running`; navigate to `#/`; immediately `GET /runs/{id}/requests` to seed the map (§10.3.3) and WS `subscribe {run_id, from_id: 0}`.
  - `409 run_active` → previous state + toast "A run is already in progress" with a **View** action that adopts the run from `GET /runs/current` (covers a second browser tab racing).
  - `403 feature_gated` (defensive; gated toggles shouldn't be visible) → idle + GatePanel as a modal.
  - `422 collection_invalid` → idle + navigate to `#/file/<path>` of the offending collection. `env_not_found` → idle + toast naming the env.
  - Network failure → idle + toast; `serverReachable=false` flow (§10.6.7.1).
- **running**: WS events mutate the request map. The Run button becomes **Cancel**; clicking POSTs cancel → `cancelling`.
- **cancelling**: Cancel disabled, label "Cancelling…"; rows still `running` keep spinning until their `request.end` arrives.
- **completed / cancelled / error**: terminal → reconciliation (§10.3.4). `error` means the *run infrastructure* errored (`run.error` event): render `RunErrorBanner` above the list with the event's message.

Implemented as a pure `transition(state, action): state` function, unit-tested exhaustively.

#### 10.3.3 Event-application reducer (`event-reducer.ts`)

The `requests` store is `Map<request_id, LiveRequest>`:

```ts
type LiveRequest = {
  request_id: string; slug: string; name: string; method: string;
  phase: 'setup' | 'main' | 'teardown';
  status: 'pending' | 'running' | 'passed' | 'failed' | 'skipped' | 'error';
  status_code?: number; duration_ms?: number;
  fail_message?: string;        // live: derived from assertion.result; authoritative from the light list at reconcile
  error?: { category: string; code?: string; message: string };  // error outcomes; from the light list
  skip_reason?: string;         // from the REST light list (events don't carry it)
  source_file: string; source_line: number;
  wave_index: number;           // ALWAYS from REST, never events (omitempty quirk)
  retry_count: number;
  iteration?: { index: number; total: number; base_name: string; base_slug: string };
  at_ms?: number;               // request.start at_ms, for per-row live elapsed
};
```

Pure reducer `applyEvent(map, frame): map` over verbatim v1.3 events:

- `run.start` → reset the map and seed it from the **REST pre-fetch** (`GET /runs/{id}/requests` issued on the 202): every planned request becomes a `pending` row with structural fields (slug, name, method, phase, wave_index, iteration, source_file). This is what makes wave/phase/collection grouping correct from second zero — structure is **never** derived from events (binding rule, §3.2).
- `request.start` → status `running`, store `at_ms`. If the `request_id` wasn't seeded (e.g. a data-driven expansion the planner didn't predict), insert it from the event's structural fields with `wave_index: -1` until reconcile.
- `assertion.result` → if `!passed` and the row has no `fail_message`, set it to `"<type>: expected <expected>, got <actual>"`. (The full assertion list always comes from REST detail.)
- `request.end` → status ← `outcome`; set `status_code`/`duration_ms`. **Ignore** the frame's `wave_index`, `timing`, and 2 KiB-truncated bodies entirely — REST is authoritative for those.
- `run.end` → freeze the live summary; trigger reconcile.
- `run.error` → store on `runMeta.error`.

The summary strip derives counts from the map (a `derived` store), not a separate counter — replay-safe and idempotent. The reducer tolerates `request.end` for an already-terminal row (overwrite, never double count); replays after reconnect re-apply cleanly. The WS layer tracks `lastEventId` (highest event `id` seen) for re-subscription.

#### 10.3.4 Reconciliation after run end

On terminal `run.state` or `run.end`:

1. `GET /runs/{id}` → authoritative `summary` (incl. `wave_count`, `max_parallelism`, `wave_durations_ms`, `parallel`) replaces the live-derived summary.
2. `GET /runs/{id}/requests` → replace the map wholesale. The light list carries everything row rendering needs — `skip_reason`, `fail_message`, the compact `error` object (§4.8) — so no per-row detail fetches are ever needed, and historical runs render identically to just-finished ones. During the live run, skip rows show a generic "skipped" and fail messages come from `assertion.result` events until reconcile replaces them with the authoritative values.
3. If history is available (`meta.history.enabled` and tier allows), refresh `/runs` so the compare rail is current.

#### 10.3.5 WS client (`ws.ts`)

- Connect `ws://127.0.0.1:<port>/api/v1/ws?token=<t>` (token from sessionStorage).
- On `hello`: compare `tree_etag` with the store; refetch `/tree` if different. If `run` is non-null and not subscribed, `subscribe {from_id: 0}` (or `lastEventId` if the same run_id).
- `files.changed` → `refreshTree()`; if the open validation panel's file is in `paths`, refetch `/validate?path=` for it; pulse the watching indicator (§10.6.1.2).
- Reconnect with exponential backoff 0.5 s → 1 → 2 → 4 → 8 s cap, jittered. After 3 failed attempts, `wsStatus='reconnecting'` (banner); after 30 s of failure, probe `GET /meta` — if that also fails, `serverReachable=false` → Disconnected screen.
- On reconnect: re-`subscribe {run_id, from_id: lastEventId}` — the server replays gaplessly (§5.2).

### 10.4 Information architecture & routing

#### 10.4.1 Hash router

Hand-rolled hash routing (`#/…`, ~80-line store-backed router). Rationale: deep links survive being pasted anywhere without server involvement; the token-strip `history.replaceState` interaction stays trivial (only the search string is replaced, never the hash); no risk of colliding with a future server route; zero server code. A router library is unjustified for 6 routes.

#### 10.4.2 URL scheme

```
#/                                        run view (current/last run, no filter)
#/tree/<collectionPath>                   run view filtered to a collection (URL-encoded)
#/runs/<run_id>                           run view pinned to a specific run
#/runs/<run_id>/requests/<request_id>     inspector
    ?tab=error|body|headers|assertions|timing|request
#/compare                                 compare screen (baseline unset)
#/compare?base=<run_id>&target=<run_id>&slug=<slug>&iter=<n>&changes=1
#/file/<collectionPath>                   validation panel for an invalid collection
```

- `#/` always shows "the run in focus": the live run if active, else the most recent completed run of this server session, else the no-run-yet state.
- `#/runs/<id>` pins a specific run. On the free tier, only runs still in the 5-run memory ring are addressable; expired or unknown ids render the inline `NotFoundPanel` ("run no longer available") with a link to `#/`.
- Inspector `tab` defaults to `body`, except `outcome=error` defaults to `error` (§10.6.3.6).
- `#/compare` query params make comparisons shareable; `changes=1` encodes the changes-only toggle.

#### 10.4.3 Boot sequence & state on reload (`main.ts`)

1. If `location.search` contains `token`: store in `sessionStorage['curlew.token']`, then `history.replaceState(null, '', location.pathname + location.hash)` — the deep-link hash survives.
2. No token in URL or sessionStorage → Disconnected screen (§10.6.7.1) with "restart `curlew ui` and reopen the printed URL". There is no login form.
3. Fetch `/meta`, `/tree`, `/environments` in parallel; open the WS.
4. The `hello` frame carries the active run, if any → `subscribe {from_id: 0}`; full replay rebuilds live state, so a mid-run reload recovers completely.
5. Resolve the route from the hash; 404s render `NotFoundPanel` inline.

### 10.5 Visual system

#### 10.5.1 Tokens

`at-tokens.css` adopted as-is: pure-neutral grays (`--bg0..4`, `--fg0..3`), dark default + light via `data-theme="light"`, three densities via `data-density`, IBM Plex Sans/Mono stacks, status colors as the only color (`--ok` green, `--err` red, `--warn` amber), primitives (`.at-btn`, `.at-input`, `.at-chip`, `.at-method`, `.at-dot`, `.at-spin`, `.at-redacted`, `.at-tabs`, `.at-menu`). Accent variants in the file stay dormant — **v1 ships mono accent only**.

#### 10.5.2 Token/primitive additions (complete list)

```css
/* NEW: error outcome — same red family as fail, hue-shifted; distinction is
   carried primarily by glyph, not hue */
--err2: oklch(0.62 0.19 12);             /* dark */
[data-theme="light"] { --err2: oklch(0.50 0.21 12); }

.at-dot.error { background: transparent; border: 1.5px solid var(--err2); }
/* the Dot component renders a small "!" glyph inside */
```

Plus: `--fs-lg: 15px` (panel titles); `.at-gate` (gate panel), `.at-banner` (WS reconnect bar), `.at-kbd` (help-overlay keycaps), `.at-group-h` (shared wave/phase/collection group-header style); a global `:focus-visible` rule (`outline: 1px solid var(--acc); outline-offset: 1px`); the prototype's inspector-inline styles (`.jt-*` JSON-tree, `.at-kv`, `.at-seg`) promoted into `tokens.css`.

**Error-color decision:** same red *family* as `--err` but a distinct token, and — critically — a different glyph everywhere: failed = filled red dot; error = hollow red-ring dot containing a small `!`. Color alone never disambiguates (also the a11y rule, §10.8.2). Amber is taken by `running`/warnings; a fourth hue would break the "color = status only" discipline. "Failed" and "error" are both bad outcomes; kinship in hue is honest — the glyph and the word do the work.

#### 10.5.3 Component inventory (atoms)

| Component | From prototype | Spec |
|---|---|---|
| `Dot` | `AtDot` | states: `pending` (hollow gray), `running` (spinner, `--warn`), `passed` (filled `--ok`), `failed` (filled `--err`), `skipped` (hollow `--fg3` ring), **`error`** (hollow `--err2` ring + 7 px `!` glyph), `mixed` (diagonal ok/err split — covers "any bad"; tooltips enumerate counts). Every Dot carries `title` = the state word. |
| `Method` | `AtMethod` | uppercase mono 10 px; weight-only differentiation, no color. |
| `Code` | `AtCode` | renders `status_code` + reason text ("201 Created") from a client-side status-text table; the server sends only the number. |
| `Chip` | `.at-chip` | unchanged. |
| `Redacted` | `AtRedacted` | renders for any value equal to `[REDACTED]` (the `variable.Redacted` marker): dot mask + REDACTED tag. **No reveal affordance, no tooltip — the value does not exist client-side.** |
| `Tabs` | `AtTabs` | adds keyboard: Left/Right move tabs when the tablist is focused; `1–6` global shortcuts (§10.7). |
| `PhaseBadge` | new | tiny uppercase chip `SETUP` / `TEARDOWN` (`main` renders nothing — it is the default). |
| `RetryBadge` | new | chip `↻2` when `retry_count > 0`; tooltip "2 retries — see Timing tab". |
| `TemplateUrl` | new | renders raw template URLs; `{{…}}` spans styled `--fg2` italic on `--acc-dim`, rest mono `--fg1`. Used in tree tooltips and the Request tab definition line. |
| `OpenInEditor` | `AtOpenInEditor` | wired to `POST /open` (§10.6.3.8). |
| `CopyBtn` | `AtCopyBtn` | clipboard API + 1.1 s check feedback, unchanged. |

### 10.6 Screen-by-screen behavioral spec

#### 10.6.1 Chrome

**10.6.1.1 Top bar** (`TopBar.svelte`, 44 px). Left→right: project name (`meta.project.name`); watching indicator; spacer; History button; env switcher; theme/density popover (`⋯` ghost button); Run split-button. The prototype's git-branch chip is dropped in v1 (no server field for the live project; branch renders only where run meta provides it — history rail and compare cards).

**10.6.1.2 Watching indicator** (`WatchIndicator.svelte`).
- Idle: 7 px `--fg3` dot + mono `--fg3` "watching files".
- On `files.changed`: dot turns `--warn` with the `at-pulse` animation (2 iterations); label becomes `"<basename of paths[0]>[ +n] changed · tree reloaded"` for 2.6 s, then reverts. The label is an `aria-live="polite"` region.
- While the WS is reconnecting, the indicator is replaced by the WS banner state — never show "watching files" while the socket is down.

**10.6.1.3 History button.** Always visible (binding decision). Three states:
- **Allowed** (`meta.tier.features['ui_run_history'].allowed`): ghost button; toggles `#/compare`.
- **Gated**: same button with a 9 px lock glyph. Click navigates to `#/compare`, which renders the GatePanel. Never hidden, never disabled.
- **Config-disabled** (`meta.history.enabled === false` on an entitled tier): tooltip "history is disabled (ui.history.enabled: false)"; the compare screen shows a config-hint empty state instead of the gate.

**10.6.1.4 Env switcher** (`EnvMenu.svelte`). Button: `env <name> ▾`. Menu from `GET /environments`:
- One row per environment: check mark when selected, mono name, hint = the env's `base_url` variable value if present (else its `file`).
- **Variables popover** (new vs prototype): hovering/focusing a row 300 ms — or pressing `→` — opens a side flyout listing that env's `variables` as a `.at-kv` grid; `sensitive` values render the `Redacted` chip (the server already sent `[REDACTED]`). Footer: mono `--fg3` file path. Max-height 320 px, scrolls.
- Selecting an env stores `selectedEnv` (session) — it triggers nothing; it is the `env` field of the next `POST /runs`.
- The summary strip echoes the env a run actually used (`env: staging`, from run meta), which may differ from the switcher if changed mid-run.

**10.6.1.5 Run split-button** (`RunSplitButton.svelte`). Primary segment + caret segment.
- **Parallel toggle**: a small `⇉` segment inside the button group, left of the primary label, rendered **only when** `meta.tier.features['parallel_execution'].allowed`. Sticky pref (localStorage); tooltip "run requests in parallel waves". Not rendered at all on lower tiers — the Run button never advertises parallel.
- Menu items and enablement (all also require `runState` ∉ {starting, running, cancelling}):

| Item | Hint | Enabled when | POST body |
|---|---|---|---|
| Run all | `<N> requests` (sum of valid collections' `counts.main`) | ≥1 valid collection. With >1 valid collection, requires `test_discovery` — otherwise the item shows a lock glyph and opens the GatePanel (§6.4) | `{collection: null, mode: "all"}` (or the single collection's path) |
| Run current collection | collection name | a collection is focused (route `#/tree/…`, or the inspector's request belongs to one) | `{collection: <path>, mode: "all"}` |
| Run selection | `<n> selected` | ≥1 request checked in the sidebar (§10.6.1.6); else disabled with hint "select requests in the sidebar" | `{collection: <path>, mode: "selection", selection: [names]}` |
| Re-run failed | `<n> failed` | the last completed run has ≥1 `failed` or `error` outcome | `{mode: "rerun_failed", rerun_of: <last run_id>}` |

- `parallel` is included in every body; always `false` when the toggle isn't visible.
- While running, the primary segment becomes **Cancel** (`--err` outlined, not primary-filled); the caret menu is disabled.

**10.6.1.6 Sidebar** (`Sidebar.svelte`, 264 px).
- Top: filter input (global `/` focuses it). Filters request `name`, `slug`, and collection `path` (case-insensitive substring; no fuzzy matching in v1). Collections with zero matches collapse out.
- **Collection row**: chevron, mono short path, request count, aggregate Dot. Aggregate logic (extends the prototype's to 4 outcomes): any `running` → running; all `pending` → no dot; any bad (`failed`/`error`) and any `passed` → `mixed`; all bad → `failed` dot (`error`-ring variant when *all* bad ones are errors); else `passed`. Tooltip enumerates: "3 passed · 1 failed · 1 error".
- **Invalid collection** (`valid: false`): warn-colored `⚠ invalid` affix; click navigates to `#/file/<path>`; tooltip = first issue's `message`. No chevron, no children.
- **Phase grouping**: when a collection has `counts.setup > 0` or `counts.teardown > 0`, its requests render under 10 px uppercase `--fg3` separators `setup` / `teardown` (main has no header). Tree rows carry no per-row phase badge — the separator suffices.
- **Request row**: Dot (live status from the `requests` store, matched by slug — data-driven iterations match via `iteration.base_slug`, aggregating all iterations into the tree entry's dot; dotless when no run), name, `Method`. Tooltip: `TemplateUrl` rendering of the raw `url` + `file:line` — **template form, never resolved**. `data_driven: true` rows append a stack glyph `⛁` ("data-driven — expands at run time"). `required: true` is tooltip-only.
- Click request → inspector if it has a result in the focused run; else focus the row + footer hint "no result yet — run to inspect". Double-click collection row → `#/tree/<path>` filter.
- **Selection mode**: a checkbox appears on row hover; checking any row enters selection mode (checkboxes always visible, count in footer, `Esc` clears). Selection is scoped to **one collection**: checking a request in a different collection moves the selection there (clears the previous, with a toast "selection moved to <file>"). Feeds "Run selection".
- **Footer**: `N collections · M requests`, spacer, `read-only` label, title "files are the source of truth — curlew never edits them". In selection mode: `n selected · esc to clear`.

#### 10.6.2 Run view (`RunView.svelte`)

Composition top→bottom: `SummaryStrip`; optional filter chip row (`filtered: <file> ✕`); optional `RunErrorBanner`; layout toggle row; scrollable result area.

**10.6.2.1 Layout toggle.** Right-aligned `.at-seg`: `compact | columns | lanes`. **Default `compact`** (binding decision — overrides the prototype's columns-default). Persisted pref. When the focused run is sequential (`summary.parallel === false`): `columns` and `lanes` are disabled with tooltip "this run was sequential — waves apply to parallel runs only"; the control stays visible for discoverability. The toggle reflects the *focused* run (flipping between a parallel historical run and a sequential one re-enables/disables accordingly). Batch runs (§6.4) support all layouts; wave grouping nests inside collection sections.

**10.6.2.2 Summary strip** (`SummaryStrip.svelte`). Per prototype, extended:
- Counts: `passed` (`--ok`), `failed` (`--err`), `error` (`--err2`, rendered only when > 0), `skipped` (neutral), `running` (`--warn`, only while running). Each count is `aria-label`ed.
- Elapsed/total: while running, elapsed = `performance.now() − t0` where `t0` is set when `run.start` arrives (not at click), ticking at 100 ms; event `at_ms` values are per-row offsets, not the global clock. On terminal state, swap to authoritative `summary.duration_ms` ("3.4s total"). On mid-run reconnect, re-derive `t0 = performance.now() − latest at_ms seen` (±network skew, acceptable).
- Right: `done/total · streaming` while running; after: `· run finished`, plus for parallel single-collection runs `· 3 waves · max 4∥` from `summary.wave_count`/`max_parallelism` (omitted for batch runs), plus `env: <name>`.
- Progress bar: 3 px segments pass/fail/error/skip/running in that order, widths = count/total (`error` segment uses `--err2`).
- Viewing a non-current run (`#/runs/<id>`): a left chip `viewing past run · <relative time>` with ✕ returning to `#/`.

**10.6.2.3 Compact list (default) — grouping and row anatomy** (`RequestRow.svelte`). Bordered `--bg1` container. Grouping, outermost first:
1. **Collection sections** (batch runs only): `CollectionHeader` rows with the file basename, when the run spans >1 `source_file`.
2. **Phase sections**: `PhaseHeader` rows (`setup` / `teardown`; main unlabeled) when the run includes setup/teardown requests.
3. **Wave groups** (parallel runs): `WaveHeader` rows `WAVE <i+1> · n req`, right-aligned mono wave duration (`summary.wave_durations_ms[i]`, after completion; per-collection waves in batch runs show no duration). `wave_index` ALWAYS from the REST-seeded map.
4. Sequential runs without phases: flat list in REST order.
- **Iteration rows**: data-driven requests render one row per iteration, named `"<base_name> <index+1>/<total>"`, consecutive, with a 2 px left inset bar grouping them. No collapse in v1.

Row (height `--row-h`), left→right:
1. `Dot` (live status).
2. `PhaseBadge` (setup/teardown only; omitted under a phase header to avoid stutter — shown in mixed contexts like filtered views).
3. `Method`.
4. Name (sm, `--fg0`).
5. Collection basename (xs mono `--fg3`).
6. `RetryBadge` when `retry_count > 0`.
7. Status-dependent middle (flex-1, ellipsized, xs mono):
   - `failed`: `fail_message` in `--err`.
   - `error`: `error.category` + first message line in `--err2` (e.g. `network: dial tcp 10.0.0.5:443: connect: refused`).
   - `skipped`: verbatim `skip_reason` in `--fg3` (e.g. `dependency "Get Token" failed`, `parent skipped: Get Token`, `if: false`, `context cancelled`); placeholder "skipped" until reconcile fills it.
   - `running`: live per-row elapsed in `--warn`, derived from the row's `at_ms`.
   - `pending`: nothing; row at 0.55 opacity.
8. Right: `duration_ms` (mono xs `--fg2`) + `Code` for passed/failed; for `error`, `Code` only if `status_code` present; nothing for skipped.

Interactivity: rows with terminal outcomes (`passed/failed/skipped/error`) are clickable → inspector (`Esc` returns; scroll position and row cursor preserved). Skipped rows ARE clickable (delta from prototype) → the skip panel (§10.6.3.7). `pending`/`running` rows are not clickable. Hover `--bg2`. Live transitions animate via `at-fadein` on the metadata cluster (suppressed under reduced motion). Rows never reorder — order is fixed by the REST seed.

**10.6.2.4 Columns / lanes layouts** (`RequestCard.svelte`). As the prototype: columns = one 252 px column per wave with chevron separators; lanes = wave-label gutter + responsive card grid per wave. Cards gain the 4-outcome treatment (error message box uses `--err2`), verbatim skip reasons, and iteration/retry chips under the name line. Parallel runs only (§10.6.2.1); in batch runs, columns/lanes render per collection section.

**10.6.2.5 No-run-yet state.** `#/` with no run this session: centered hint — "No run yet" headline; "Press **r** or click Run all to execute `<N>` requests against `<env>`"; if history is available, a link "view previous runs →" to `#/compare`.

#### 10.6.3 Inspector (`Inspector.svelte`)

Route `#/runs/<id>/requests/<rid>?tab=…`. Loads `GET /runs/{id}/requests/{rid}` on mount (spinner while loading; the header renders immediately from list-store data).

**Header bar**: back button (`←`, also `Esc`), `Dot`, name (+ iteration suffix), mono method + **raw template url** (from the tree, by slug; tooltip shows the resolved URL from detail), spacer, `Code` (when present), duration, `RetryBadge`, `OpenInEditor`.

**Tabs**: `Body · Headers (n) · Assertions (n|n✗) · Timing · Request`; when `outcome === 'error'`, a leading red-tinted **`Error`** tab is the default; when `outcome === 'skipped'`, the tab strip is replaced by the skip panel (§10.6.3.7). Active tab in the URL via `?tab=`.

**10.6.3.1 Body tab** (`BodyTab.svelte` + `JsonTree.svelte`). Decision tree on `response.body`:
1. `truncated: true` (> 256 KiB inline): fetch-on-demand panel — formatted `size`, `content_type`, button "Load body (1.4 MB)" → `GET …/body?which=response`. After load, continue at 2. Bodies > 5 MB after load render raw-only with virtualized lines (no JSON tree — guards against tree blowups); a Download button always accompanies.
2. `encoding === 'base64'` (binary): `BinaryPanel` — content type, size, "binary content", Download button (base64 → Blob). Image content types render an inline preview (object URL, max-height 480 px, checkerboard background). Nothing else.
3. JSON: `content_type` matches JSON AND `JSON.parse` succeeds → `JsonTree`. Parse failure silently falls through to 4.
4. Everything else: raw `<pre>` (mono sm, pre-wrap); the search box degrades to plain-text find with highlight + count.

**`JsonTree` (normative port of the prototype):**
- Expansion defaults: depth < 2 auto-expanded, except arrays with > 10 items start collapsed. `Expand all` / `Collapse all` reset per-node overrides; chevrons toggle per node.
- Collapsed preview: `{…} 12 keys` / `[…] 50 items`; click expands.
- Arrays > 20 items show the first 20 + a dashed `… show N more items` button (per node; no further pagination in v1).
- Search: matches keys and stringified leaf values, case-insensitive; hits get a `--warn` 14% background; ancestors force-expand; counter "n matches"; Enter/Shift+Enter cycle with scroll-into-view. Focused via `Ctrl/Cmd+F` inside the inspector (§10.7).
- Copy-path: hover-visible per-row button; copies **`body.$` + JSONPath** (`body.$.data[3].amount`) — matching curlew's assertion syntax so paths paste directly into `assert:` blocks. Root copies `body.$`. Toolbar `Copy body` copies pretty-printed JSON.
- Pretty/Raw `.at-seg` toggle; Raw = 2-space re-serialized text.
- Redacted leaves: string values equal to `[REDACTED]` render the `Redacted` chip.
- The Body tab shows the **response**; the request body lives in the Request tab.

**10.6.3.2 Headers tab.** `.at-kv` grid of response headers, order as received; multi-value headers render one row per value. `[REDACTED]` values → `Redacted` chip. Toolbar: copy-all (`k: v` lines).

**10.6.3.3 Assertions tab.** From `assertions.items`. Each card: pass/fail icon, **type badge** (uppercase chip `STATUS|BODY|HEADER|SCHEMA` — the server gives `type/expected/actual`; the prototype's free-text expr is gone), a client-rendered expectation line (`"<type> · expected <expected>"`), and the PASSED/FAILED affix. Failed cards expand expected/actual rows (`expected` in `--fg0`, `actual` in `--err`); values > 120 chars get expand/collapse + per-value copy. The tab count shows `n✗` red when failures exist, else the total. Empty state: "no assertions for this request — only transport success was checked".

**10.6.3.4 Timing tab** (`TimingTab.svelte`). From `timing` (§4.9). All µs; format `< 1000 µs → "412 µs"`, else ms with one decimal.
- **Full waterfall** (fresh connection): rows DNS lookup, TCP connect, TLS handshake, Waiting (TTFB), Content download — sequential offsets as the prototype; any individually absent field's row is simply omitted (plain http → no TLS row). An unaccounted remainder (`total_us − Σ phases`) > 5% of total renders as a hatched "other" segment. Total row pinned below.
- **Reused connection** (`connection_reused: true`): waterfall shows only TTFB + download, with an info line "connection reused — no DNS/connect/TLS phases". No zero-width ghost rows.
- **No phase data** (`timing` null — WebSocket/GraphQL): single total bar + caption "phase timing not available for this request type".
- **Attempts section**: when `retry.count > 0`: header `Attempts (n)`, one row per `retry.attempts[]` — `#<number>`, `Code` or `--err2` error text, `duration_ms`, `+<delay_ms> backoff` chip between attempts. A caption notes the waterfall above describes the **final** attempt.

**10.6.3.5 Request tab.** Line 1: method + **resolved** URL (as sent, redacted); beneath it, smaller: `template:` + `TemplateUrl` raw form. Request headers `.at-kv` (redacted chips). Request body block: same decision logic as the Body tab (truncated → fetch `?which=request`; base64 → BinaryPanel), caption "(variables resolved, secrets redacted)". **Source** section: `source.file:line` + `OpenInEditor`; snippet from `source.snippet[]` with line numbers from `snippet_start_line`, the request's own line highlighted (`--acc-dim`).

**10.6.3.6 Error tab** (only when `outcome === 'error'`; the default tab then).
- Banner block (`--err2`-tinted): bold `error.category` (`network`, `auth`, `config`, …) with the machine `error.code` as a chip when present (`NETWORK_CONNECTION_REFUSED`), then the full `error.message` verbatim mono.
- `hint` when present: lighter block, `--fg1`, prefixed `hint:`.
- "Request as sent": method + resolved URL + request headers/body (same components as the Request tab) — for errors there is no response; the first question is "what did we actually send".
- If a partial response exists (`status_code` present), a note links to Body/Headers, which stay enabled; otherwise Body/Headers/Assertions render empty states ("no response — the request errored before/while receiving"); tabs stay clickable, never hidden. Timing renders whatever phases were measured (often dns/connect only — exactly where it died).

**10.6.3.7 Skipped outcome.** A single centered panel: hollow skip dot, "Skipped", verbatim `skip_reason` mono. The Request tab remains available (definition + source are still useful); Body/Headers/Assertions/Timing are disabled with tooltip "request was not executed".

**10.6.3.8 Open in editor** (`OpenInEditor.svelte`). On click, `POST /api/v1/open {file, line}` (§4.13). On 204: flash `→ editor` 1.4 s. On 409 `no_editor`: toast with the server hint ("set ui.editor in curlew.yaml or $CURLEW_EDITOR"). If the endpoint 404s (older binary), degrade to copying `file:line` to the clipboard with a "copied path" flash.

#### 10.6.4 Compare screen (`Compare.svelte`, `#/compare`)

**10.6.4.1 Gated state.** `meta.tier.features['ui_run_history'].allowed === false` → the whole screen is the GatePanel (§10.6.7.3), with the rail+diff layout rendered behind it as a blurred/dimmed pure-CSS static illustration (no data) — making the value visible. No data is fetched.

**10.6.4.2 History rail** (`HistoryRail.svelte`, 224 px). `GET /runs?limit=50&offset=0`, infinite scroll appends, `total` caps. Row per run meta: start time + relative day, branch chip when `git.branch` present, counts `n✓ n✗ n! n–` (`!` = error, `--err2`), duration. The newest run is **B** (target) by default; clicking another row sets **A** (base); a second click on A swaps A↔B (hover swap icon). Explicit A/B labels per the prototype. Runs still `running` are listed but not pickable (dimmed, "in progress"). Empty state: "no recorded runs yet — runs are saved automatically (last `<max_runs>`)".

**10.6.4.3 Pair list & request picker.** On A+B: `GET /compare?base=&target=` → server-aligned `pairs` by `(slug, iteration)`. The picker lists pairs grouped **changed first** (`outcome_changed || status_changed || |duration_ms delta| > 20%`), then unchanged, then `only in base (n)` / `only in target (n)` (selectable; they render a single-sided view with an explanatory note instead of a diff). Each item: Method, name (+`#i` iteration suffix), right-aligned delta glyph (`✓→✗`, `+212ms`). Default selection = first changed pair, else first.

**10.6.4.4 Run header cards + delta.** Two `RunHeaderCard`s (A → B) from `pairs[i].base/target`: time, branch chip, outcome dot+word, code, duration; B shows the signed `delta.duration_ms` chip and the fail/error message when B is bad. A micro-row shows `delta.timing_us` entries exceeding 10% change as compact chips (`ttfb +38ms`).

**10.6.4.5 Body diff — client-side, hand-rolled Myers line diff** (`ui/src/lib/diff.ts`). Pipeline:
1. Fetch both bodies (inline from detail or via `/body` when truncated; both are server-redacted, so the diff never sees secrets).
2. If both parse as JSON: canonicalize — `JSON.stringify(sortKeysDeep(v), null, 2)`, arrays NOT sorted — so key-order churn doesn't pollute the diff. Otherwise diff raw text lines.
3. Myers O(ND) diff on line arrays → hunks of context/`-`/`+`.
4. Intra-line highlight for paired `-`/`+` lines: common-prefix/suffix trim marks the changed span. No word-level LCS in v1.

Why not `jsdiff`: the need is exactly line diff + tiny intra-line refinement; Myers is ~120 LOC, dependency-free (a stated goal for an embedded asset bundle), trivially unit-tested. Guard: inputs > 1.5 MB canonicalized → "bodies too large to diff — download A / download B".

Rendering (`DiffView.svelte`): unified view per the prototype — line numbers, sign gutter, `--err`/`--ok` 10% line backgrounds, 28% intra-line marks; header `Response body diff · <k> keys changed` (count of changed top-level paths via a cheap structural walk; omitted for non-JSON) + `−n +m` totals. **Changes-only toggle** (persisted): filters to hunks, keeping 2 context lines per hunk with `⋯` separators. **Identical state**: dashed panel "Response bodies are identical between these runs", with a sub-line itemizing any non-body deltas (`status unchanged · duration +12 ms`).

#### 10.6.5 Empty project state (`EmptyProject.svelte`)

Shown when `/tree` returns zero collections. Verbatim from the prototype: headline "No collections in this repo yet", explanation referencing `collections/` (or `meta.project.collection_filter` when set), the `curlew init` terminal mock, and the "this view refreshes automatically — curlew is watching the repo" footer (true: `files.changed` → tree refetch swaps this screen out live). Sidebar hidden; Run button disabled with tooltip "no collections".

#### 10.6.6 Validation panel (`ValidationPanel.svelte`, `#/file/<path>`)

Prototype layout with corrections:
- Data: the tree entry's `issues[]`, refreshed via `GET /validate?path=` on mount and whenever `files.changed` includes the path.
- Renders **all** issues, not just the first: each issue = a severity-colored diagnostic banner (`error` → `--err`, `warning` → `--warn`) with `Line <n>:` + `message` verbatim; the **`hint` field renders as its own line** beneath (`hint: did you mean 'equals'?`, `--fg2` italic) — the prototype's hardcoded inline `←` arrow annotation is dropped; never fabricate annotations.
- Body copy: "This file was skipped — its requests won't run until the file parses. Fix it in your editor; the tree reloads the moment you save."
- Footer kept verbatim: "curlew never edits your files — there is deliberately no fix-it form here." `OpenInEditor` wired to the first issue's line.

#### 10.6.7 Global states

**10.6.7.1 Server unreachable** (`Disconnected.svelte`). Full-screen replacement (chrome hidden) when boot fails or `serverReachable` flips false: centered terminal-styled panel — "curlew ui is not running"; body "the server at 127.0.0.1:<port> stopped or this tab's session expired. Restart it and reopen the printed URL:"; mock `$ curlew ui` line. A Retry ghost button re-probes `/meta`; auto-retry every 5 s with a subtle countdown. On success: full re-boot (§10.4.3) without losing the hash. A `401` on any API call (token rotated — the server restarted) lands here too: the old token is dead; the new URL carries the new one.

**10.6.7.2 WS reconnecting banner** (`WsBanner.svelte`). Slim bar under the top bar, `--warn` 8% background: spinner + "reconnecting to curlew…"; if a run was live, append "run continues — events will catch up". On reconnect: flash `--ok` "reconnected" 1.5 s, then remove (replay fills gaps silently). `aria-live="polite"`.

**10.6.7.3 Gate panel** (`GatePanel.svelte`). Reusable for any 403 `feature_gated` (history/compare, defensive run gates). Content from the 403 details or `/meta` features: lock glyph; title "`<feature>` requires `<required_tier>`"; the server's `message` verbatim; `workaround` (when present) as a terminal-styled line; a single ghost link "see plans →". Modal only for the defensive 403-on-run case; otherwise rendered in place, never blocking unrelated UI.

### 10.7 Keyboard model

Global listener with a **focus-zone** model (`focusZone: 'sidebar' | 'list' | 'inspector' | 'compare' | 'none'`). Shortcuts are suppressed while a text input has focus (except `Esc`).

| Key | Context | Action |
|---|---|---|
| `r` | global | Run all (respects parallel pref); disabled states no-op with a shake on the Run button |
| `Shift+R` | global | Re-run failed (when enabled) |
| `c` | running | Cancel run — hold-to-confirm: keyup before 200 ms shows a "hold c to cancel" hint |
| `/` | global | focus sidebar filter (`Esc` blurs + clears) |
| `Ctrl/Cmd+F` | inspector Body | focus body search |
| `j` / `k` (and `↓`/`↑`) | list zone | move row cursor (skips group headers); cursor = 1 px `--acc` left bar + `--bg2`; auto-scrolls |
| `Enter` / `o` | list zone | open focused row in inspector |
| `j` / `k` | inspector | next/previous request **with a result**, run order, preserving the active tab |
| `1`–`6` | inspector | tabs in displayed order (the Error tab, when present, is `1`) |
| `Esc` | inspector | back to run view (restores cursor + scroll) |
| `Esc` | menus/overlays/selection | close / clear, innermost first |
| `g` then `h` | global | go to History/compare (gated → gate panel) |
| `g` then `r` | global | go to run view `#/` |
| `e` | inspector / focused row | open in editor |
| `y` | inspector Body | copy the focused JSON path |
| `[` / `]` | compare | previous/next pair (changed-first order) |
| `?` | global | help overlay |

**Help overlay** (`HelpOverlay.svelte`): centered `.at-menu`-styled sheet; two-column table of the above with `.at-kbd` keycaps, grouped Run / Navigate / Inspector / Compare; `Esc` or `?` closes; focus-trapped.

**Focus management rules**: route changes move focus to the new screen's heading (`tabindex="-1"` + programmatic focus) so screen readers announce context; the run-view list follows the j/k cursor via `aria-activedescendant`; opening the inspector focuses the back button; closing restores focus to the originating row; menus are focus-trapped and restore trigger focus on close; `:focus-visible` outlines per §10.5.2.

### 10.8 Theme plumbing, accessibility, fonts

**10.8.1 Theme/density switching.** The top-bar `⋯` popover: theme dark/light/system (system = `prefers-color-scheme` listener), density dense/comfortable/compact. Sets `data-theme`/`data-density` on the root `.at-root` element; persisted prefs. Accent stays mono in v1.

**10.8.2 Accessibility baseline.**
- `prefers-reduced-motion`: the spinner already degrades in the token file; additionally disables `at-fadein`, `at-pulse`, progress-bar width transitions (instant), and the gate-panel blur backdrop.
- Color is never the sole channel: every status pairs hue with a glyph (check/✕/!/–/spinner) or word; diff lines carry the sign gutter; assertion cards carry icons + PASSED/FAILED text.
- Live regions: summary-strip counts wrapped in `aria-live="polite"` with throttled announcements (at most every 2 s: "8 of 14 done, 1 failed"); the terminal announcement is assertive: "Run finished: 11 passed, 2 failed, 1 error, 1 skipped". Watch indicator and WS banner polite.
- Contrast: `--fg2` on `--bg0` is the floor (≈ 4.6:1 dark); `--fg3` is decorative only — never information without a redundant channel.
- All icon-only buttons have `aria-label`; tabs use `role="tablist"` semantics; the JSON tree uses `role="tree"`/`aria-expanded`.

**10.8.3 Font bundling.** Self-hosted IBM Plex Sans (400/500/600) + IBM Plex Mono (400/600), latin subset, woff2 only, in `ui/public/fonts/` (~6 files, ~180 KB), declared in `styles/fonts.css` with `font-display: swap` and the token stacks as fallback. **No Google Fonts CDN** — the UI must work fully offline on localhost. Files vendored from the official IBM Plex release with the OFL license file committed alongside.

### 10.9 Deltas from the design prototype (complete)

1. **Default run layout is `compact`**, not columns; columns/lanes are toggles, disabled for sequential runs.
2. **Fourth outcome `error`**: `--err2` token, hollow-ring `!` Dot, Error inspector tab, error bucket in summary strip / progress bar / history counts. The prototype had pass/fail/skip only.
3. **Phases**: setup/teardown separators in the sidebar, phase section headers in the run view, `PhaseBadge` atom. The prototype ignored phases.
4. **Retries**: `RetryBadge` on rows and the inspector header; Attempts section in the Timing tab. Absent from the prototype.
5. **Data-driven iterations**: per-iteration rows "`base_name` i/N" with inset grouping; the tree shows a single entry + stack glyph. Absent from the prototype.
6. **Skip rows are clickable and show the verbatim `skip_reason`** (the prototype showed synthetic strings and non-clickable rows); the skipped inspector panel is new.
7. **Tree and inspector-header URLs are raw templates** rendered with `TemplateUrl`; the prototype resolved them in places. (The Request tab's line 1 stays resolved per server detail, with the template shown beneath.)
8. **Body handling**: > 256 KiB fetch-on-demand, base64/binary download panel + image preview, JSON tree only when content-type and parsing agree, raw fallback. The prototype assumed always-JSON inline. WS 2 KiB event bodies are never displayed.
9. **Validation panel**: renders all `issues[]` with severity coloring and the **`hint` field as text** — the prototype's hardcoded inline `←` annotation is dropped.
10. **Env menu gains a variables popover** with redacted sensitive values; the prototype showed only name + base URL.
11. **Branch chip in the top bar dropped for v1** (no server field); branch renders only where run meta provides it (history rail, compare cards).
12. **Timing waterfall**: µs precision, absent-phase omission, reused-connection and nil-timing degraded views; the prototype always drew five phases in ms.
13. **Assertions**: type badge from the server's `type` enum + client-rendered expectation line; the prototype had hand-written expr strings.
14. **Parallel toggle on the Run button** (Professional only) and Cancel-while-running behavior — absent from the prototype.
15. **History button gated-not-hidden** with lock affix + GatePanel; the prototype assumed history always available.
16. **Compare**: server-driven `(slug, iteration)` pairs with changed-first grouping and only-in-base/target sections; diff computed client-side (Myers) instead of mock data; the identical-state panel itemizes non-body deltas.
17. **Summary strip**: wave-count/max-parallelism affordances, env echo, past-run chip, authoritative-duration swap at run end; elapsed-timer semantics defined.
18. **Open in editor is real**: `POST /api/v1/open` (§4.13) with clipboard fallback; the prototype faked it.
19. **Sidebar aggregate dots extended for `error`**; mixed-state tooltips enumerate all four outcome buckets.
20. **Added screens with no prototype counterpart**: Disconnected, WS-reconnect banner, GatePanel, no-run-yet hint, narrow-viewport notice, help overlay.
21. **"Run all" spans collections as a batch run** (§6.4), with collection section headers in the run view and a `test_discovery` gate when more than one valid collection exists; the prototype implied multi-file runs without defining them.

---

## 11. Configuration

New top-level `ui:` block in `curlew.yaml`:

```yaml
ui:
  port: 8765            # 1024–65535 (0 reserved for the --port flag); default 8765
  host: 127.0.0.1       # loopback literals only: 127.0.0.1 | localhost | ::1
  open_browser: true    # default true
  editor: "code --goto {file}:{line}"   # optional; see §4.13
  history:
    enabled: true       # default true; false disables persistence even on Solo+
    max_runs: 50        # default 50; cap 500
```

Go side (`internal/config/project.go`):

```go
type UIConfig struct {
    Port        int              `yaml:"port,omitempty"`
    Host        string           `yaml:"host,omitempty"`
    OpenBrowser *bool            `yaml:"open_browser,omitempty"`
    Editor      string           `yaml:"editor,omitempty"`
    History     *UIHistoryConfig `yaml:"history,omitempty"`
}

type UIHistoryConfig struct {
    Enabled *bool `yaml:"enabled,omitempty"`
    MaxRuns int   `yaml:"max_runs,omitempty"` // 0 = default
}

// on ProjectConfig:
UI *UIConfig
```

- **Precedence per field: CLI flag > `ui:` block > built-in default** — the same model as the existing output-config precedence, minus the collection level (collections have no say over the UI).
- The embedded project JSON schema (`schemas/project-v1.json`) gains the `ui:` object so `curlew validate` and `curlew schema --project` stay accurate.
- Editor resolution order at `/open` time: `$CURLEW_EDITOR` env var > `ui.editor` > `code --goto` fallback (§4.13).

---

## 12. Tier gating summary (historical — superseded)

> Tier gating has been removed. This section is retained for historical context only; none of the gates below exist any more.

| Feature | Registry key | Tier | Enforced at |
|---|---|---|---|
| Start runs, live view, inspector, 5-run memory ring | — (ungated) | Free | — |
| Persisted run history, history list, delete, compare | `ui_run_history` (new) | Solo | store construction (§8.4); `GET /runs`, `DELETE /runs/{id}`, `GET /compare`, `GET /runs/{id}/events` for persisted runs |
| Parallel runs (waves) | `parallel_execution` (existing) | Professional | `POST /runs` with `parallel: true` |
| Batch run across >1 collection ("Run all") | `test_discovery` (existing) | Professional | `POST /runs` with `collection: null` (§6.4) |
| Data-driven expansion within runs | `data_driven` (existing) | Professional | inside the runner during execution — below Professional, a data-driven request gate-errors at run time (surfaces as an `error` outcome / run error, exactly as the CLI behaves) |

Mechanics:

- Gates are checked server-side with `auth.CheckFeature(auth.DefaultRegistry(), key, currentTier())`; gated endpoints return 403 `feature_gated` with the `auth.GateResult` fields in `details` (§4.1). The UI uses plain `CheckFeature` (not the trial-claims variant) for consistency with the runner-internal gates, which do not receive trial claims from the CLI run path today; wiring trial claims is a deferred follow-up (§15).
- The client learns entitlements from `/meta.tier.features` and renders **gated-not-hidden** UX (§10.6.1.3, §10.6.4.1): locked buttons and GatePanels with the registry's `message` and `workaround` verbatim. The server re-enforces regardless of what the client renders.
- Tier source: the CLI's `currentTier()` as-is — release binaries derive it from the cryptographically verified cached License JWT and fall back to Free when no usable license exists. Repository test/smoke builds enable an `CURLEW_TIER` linker seam; release binaries ignore that environment variable.

---

## 13. Testing strategy

### 13.1 Go (`internal/uiserver`, `internal/runservice`, `internal/httpexec`)

- **Handler tests**: `httptest.NewServer(srv.Handler())` for every REST endpoint (precedent: `internal/prcheck`, `internal/backend` client tests). Token/Host middleware tested explicitly (401/403 paths). The asset handler test asserts the committed placeholder serves.
- **WS tests**: gorilla dialer against the httptest server (precedent: `internal/websocket/integration_test.go`) — hello/subscribe/replay-from-id (gapless, no duplicates), slow-consumer disconnect, origin rejection.
- **Orchestrator tests**: inject a fake `runner.ExecuteFunc` with testdata collections (exactly as runner tests do) — single-flight 409, cancellation → `cancelled`, ring eviction at 5, batch-run aggregation and gating, sink fan-out producing both an event log and collected details with shared redaction.
- **Store tests**: `t.TempDir()` — atomic write, retention pruning at `max_runs`, corrupt-dir tolerance, the self-ignoring `.gitignore`, the 1 MiB body cap, no-write below Solo.
- **runservice extraction**: behavior-lock tests — existing `curlew run` golden outputs stay green after the `EmitterSink`/sensitive-builder moves.
- **httptrace**: unit tests against `httptest.NewServer`/`httptest.NewTLSServer` asserting phase presence (fresh vs reused connections), nil for non-HTTP protocols, and unchanged `Result.Duration` semantics; events golden tests updated for schema 1.3.

### 13.2 Frontend unit & component (Vitest, jsdom; `@testing-library/svelte`)

- `event-reducer.test.ts`: golden NDJSON fixtures (recorded from a real run, committed under `ui/src/lib/__fixtures__/`) replayed through `applyEvent` — status maps at checkpoints; idempotency under duplicate replay; the `wave_index`-omitted-when-0 trap (events with and without the field never affect grouping); out-of-order `assertion.result` after `request.end`.
- `run-machine.test.ts`: the full transition table including 409/403/422/network on `starting`, cancel paths, terminal re-entry.
- `diff.test.ts`: Myers correctness vs a brute-force LCS on randomized small inputs; canonicalization (key-order invariance); intra-line marking; changes-only hunking; the > 1.5 MB guard.
- `router.test.ts`, `format.test.ts`, `api/client.test.ts` (ApiError mapping, 401 → disconnect signal, bearer header).
- Component tests: `JsonTree` (expansion defaults, > 20-array show-more, search force-expand, copy-path string `body.$.data[3].amount`), `RequestRow` (all six live statuses incl. verbatim skip reason and error rendering), `TimingTab` (full / reused / nil-timing / attempts), `SummaryStrip` (error bucket, parallel affordances), `GatePanel`, `EnvMenu` redaction.

### 13.3 Playwright e2e — against the real binary

**Decision: real `curlew ui` binary + fixture project + tiny local echo API.** The e2e job builds the Go binary (with built UI assets), starts a small deterministic echo server (endpoints: JSON, one slow, one 500, one connection-refused port for the `error` outcome, one > 256 KiB body, one binary body), and launches `curlew ui --port 0 --no-open` in `ui/tests/fixtures/project/`. Playwright `webServer` wires both. Rationale: mocking the WS protocol would test our own mock; the contract risk lives exactly at the Go↔SPA seam.

**Tier matrix (required — several runner features are tier-gated):** the suite launches the binary with `CURLEW_TIER` per scenario group. The fixture project keeps gated features in **separate collections** so lower-tier scenarios never trip runner gates:

| Tier (`CURLEW_TIER`) | Fixture collections exercised | Scenarios |
|---|---|---|
| free | `basic.yaml` (happy path, a failing assertion, an error endpoint, setup/teardown, a sensitive env var), `broken.yaml` (invalid) | boot + token strip; run happy path; error row + Error tab; skip reason; big-body; binary panel; validation panel; redacted chip; gated History (gate panel); mid-run reload |
| solo | + `retried.yaml` (a retried request — `retry` is Solo-gated in the runner; a free-tier run of it would abort with a gate error) | history rail populates; compare diff; identical-state panel; delete run; retry badge |
| professional | + `iterations.yaml` (data-driven — gated at Professional by the runner's `data_driven` feature), `parallel.yaml` (`depends_on` chain for waves) | iteration rows i/N; parallel toggle + wave headers; columns/lanes layouts; batch "Run all" across collections |

Cross-cutting scenarios: boot + token strip (URL carries no token after load; sessionStorage does); mid-run page reload recovering via replay; keyboard smoke (`r`, `j/k`, `Enter`, `1–6`, `Esc`, `?`).

### 13.4 CI gates

- Go-side uiserver/runservice/httpexec tests run in the normal Go gate (they don't need built assets — handlers are tested directly).
- `scripts/ci-local.sh` gains a `ui/**` + `internal/uiserver/assets/**` change-detection glob running, in `ui/`: `npm ci`, `npm run check` (svelte-check), `npm run lint`, `vitest run`, `npm run build`.
- Playwright e2e on PRs touching `ui/` or `internal/uiserver/` (chromium only) and nightly (chromium + webkit).

---

## 14. Milestone decomposition (vertical slices)

Each slice is TDD-able and observable via curl/browser; slice 2 is a pure-core change shippable independently of the UI.

| # | Slice | Observable |
|---|---|---|
| 1 | **`ui` command + server skeleton** — switch case, flags/help/usage triad, project-root discovery, loopback bind + port scan, token + Host middleware, embedded placeholder, `GET /api/v1/meta`, clean shutdown | `curlew ui --no-open`, then `curl -H "Authorization: Bearer $T" 127.0.0.1:8765/api/v1/meta` |
| 2 | **httptrace timing + events v1.3** — `httpexec.Timing`, runner event plumbing (timing/attempts only), emitter `timing` field, `SchemaVersion = "1.3"`, docs + JSON schema, golden updates | `curlew run x.yaml --events /tmp/e.ndjson && jq 'select(.type=="request.end").timing' /tmp/e.ndjson` (note: `--events` takes a file path; there is no stdout mode) |
| 3 | **Read-only project API** — `/tree`, `/environments`, `/validate`, `/files`, env-value redaction | curl against the repo's `examples/` project |
| 4 | **runservice extraction** — `Execute`, sensitive-set builders, `EmitterSink` move; cmd/curlew switched to the moved sink; behavior-lock tests | existing `curlew run` goldens stay green |
| 5 | **Run orchestration + WS streaming** — `POST /runs` (single-flight, gates, batch), orchestrator, EventLog, hub, hello/subscribe/replay, `/runs/current`, `GET /runs/{id}` (state + summary), cancel | `websocat` or a browser console streaming a live run |
| 6 | **Inspector REST** — DetailCollector + `RequestEndEvent` additive fields, `/requests`, `/requests/{id}`, `/body`, source snippets, memory ring of 5 | curl full detail incl. timing + redacted bodies mid-run |
| 7 | **History store (Solo gate)** — `ui_run_history` registry entry, store + meta.json + gitinfo + retention + self-ignoring gitignore, `/runs` list, DELETE, `/events` download, 403 gate shape | `CURLEW_TIER=solo` vs free behavior |
| 8 | **Comparison** — `/compare` alignment + deltas; `/open` endpoint | two runs, curl the diff JSON; `curl -X POST …/open` |
| 9 | **Watcher + SPA shell** — `files.changed` frames, tree etag, browser-open, dev proxy, `ui:` config + JSON schema, CI globs; SPA boot/chrome/tree | browser shows the live tree, watching indicator pulses on save |
| 10 | **SPA run view + inspector** — state machine, reducer, compact/columns/lanes, all inspector tabs | full run-and-inspect in the browser |
| 11 | **SPA history + compare + polish** — history rail, diff view, gate panels, keyboard model, help overlay, a11y pass, Playwright suite | the §13.3 e2e scenarios pass |

(Slices 1–8 are Go-only and individually curl-observable; 9–11 build the SPA on top. The original backend design's slice list is preserved in 1–9; 10–11 split the frontend delivery into reviewable pieces.)

---

## 15. Appendix: deferred follow-ups

Explicitly out of this milestone, recorded so they aren't lost:

1. **Converge `runCmdInner` onto `runservice.Execute`** — the CLI run path keeps its monolith this milestone; the extraction (§6.1) is sized for the UI's needs. A later refactor milestone can migrate the CLI onto the same pipeline.
2. **`curlew init` should always add `.curlew/` to the project `.gitignore`** — today it does so only with `--skill`. The store's self-ignoring `.gitignore` (§8.1) covers the gap meanwhile.
3. **Events-schema doc phase-string fix** — v1.2's `setup|test|teardown` documentation vs the emitted `setup|main|teardown`; corrected in the v1.3 document (§7.3), no emission change.
4. **MANUAL exit-code table** — says 2 for usage errors; every command returns 1. Reconcile the table (or the commands) repo-wide.
5. **CLI-run persistence opt-in** — letting plain `curlew run` write to `.curlew/ui/runs/` (config-gated) so CLI runs appear in UI history. Deliberately excluded from v1 to keep the CLI persistence-free.
6. **Trial-claims wiring for runner-internal gates** — `VarSources.TrialClaims` is never set by the CLI run path today; the UI follows suit (§12). Wiring trials through both paths is one follow-up.
7. **Team-vault, plugins, report upload in UI runs** (§6.5) — each needs cmd-layer plumbing extracted before the UI can offer it.
8. **Wave-0 `omitempty` emission fix** — a v2.0 (breaking) candidate: emit `wave_index` unconditionally for parallel runs. Documented quirk until then (§7.3).
9. **Word-level diff refinement, diff export, trend charts over history** — compare-screen enhancements beyond v1.
