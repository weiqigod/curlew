# Curlew

**File-based API testing.** Write your API tests as YAML files, keep them in version control next to your code, and run them with a single static binary — in your terminal, in CI, or from a local web UI.

Curlew is for developers who like Postman's job but not its workflow: diffs instead of workspaces, pull requests instead of shared accounts, text editors instead of drag-and-drop, and the same tests running locally and in CI without exporting anything.

```yaml
# collections/sample.yaml
name: Sample Collection

requests:
  - name: Hello World
    request:
      method: GET
      url: "{{base_url}}/get"
    assertions:
      status: 200
```

```bash
curlew run collections/sample.yaml
```

```
Sample Collection
  ✓ Hello World (234ms)

1 passed, 0 failed, 0 skipped — 234ms
```

## Features

**Collections and variables**
- **Plain-file collections** — YAML in, exit codes out. `{{variable}}` interpolation with a clear precedence chain (project config, environments, `.env`, shell-outs, CLI flags).
- **Composition** — `include:` shared requests across collections; `setup:` and `teardown:` phases that run around the main body.
- **Deterministic fake data** — `$faker.*` functions, reproducible with `--seed`, localized with `--locale` (15 locales from `en-US` to `ja-JP`).

**Assertions and chaining**
- **Rich assertions** — status, headers, JSONPath body checks, numeric proximity, JSON Schema validation.
- **Request chaining** — `extract:` values from one response and use them in the next.
- **Conditional execution** — `if:` takes a CEL expression over `vars`, `env`, and `previous.{status,headers,body}`, so a request can skip itself based on what the last one returned.
- **Dependencies** — `depends_on:` skips a request when anything it needs was skipped in the same phase; in `setup:`, marking a request `required: true` skips everything after it if it fails.

**Resilience**
- **Retry with backoff** — `retry:` at collection level or per request: exponential, linear or constant backoff, optional jitter, triggered by specific status codes, status ranges, or network errors.
- **Rate limiting** — `rate_limit_rps:` applies a token-bucket throttle so a suite can't stampede a staging environment.
- **Failure policy** — `options.stop_on_failure` halts the main phase at the first failure instead of running the rest; `teardown:` still runs.

**Protocols**
- **HTTP/REST** — full method, header, query and body control, including `body_file:` and `body_binary_file:` for uploads.
- **GraphQL** — inline queries or external `.graphql` files with fragment resolution and variables.
- **WebSocket** — scripted `steps:` with `expect:` matchers, heartbeats, and configurable reconnect.

**Auth and secrets**
- **Auth profiles** — dynamic login → extract → cache, referenced by name from any request.
- **Request signing** — AWS SigV4 and OAuth 1.0.
- **Vault providers** — HashiCorp Vault, AWS, Azure, GCP and 1Password, with local caching. `curlew vault list` shows configured profiles.
- **Redaction on by default** — values marked sensitive print as `[REDACTED]` in every format, including reports and logs. `--allow-sensitive` opts back in.

**Scale**
- **Data-driven testing** — run a request once per CSV or JSON row, sequentially or in parallel.
- **Parallel execution** — dependency-aware waves via `--parallel`; `--show-dependencies` prints the graph as DOT, and adding `--dry-run` prints the execution waves.
- **Load testing** — `curlew perf` with `--vus`, `--duration`, `--ramp-up` and `--rps`.

**Output**
- **Formats** — colored terminal, `json`, `tap`, `junit`, `html`, and `markdown` artifacts (`--format`, with `--report` for file/directory targets).
- **Event stream** — `--events <file>` writes one NDJSON object per line against a versioned schema, for CI systems and agents that want structured progress rather than parsed text.
- **Selection** — `--only "<name>"` runs just the named requests (repeatable); setup and teardown still run in full.
- **Verbosity** — `-v`, `-vv`, `-q`, and `NO_COLOR` support.

**Workflow**
- **Local web UI** — `curlew ui` starts a localhost runner/inspector over your project. Files stay the source of truth; the UI never edits them.
- **Watch mode** — `curlew watch` re-runs on file changes, with `--clear` between runs.
- **Glob discovery** — `**` patterns honoring `.curlewignore`.
- **OpenAPI import** — turn a 3.x spec into a collection with headers, request bodies and status assertions.
- **Plugins** — external processes registering `on_request`, `on_response` and `on_result` hooks, each with a 10-second timeout.
- **CI gate** — `curlew pr-check` turns a results file into a pass/fail exit code with a summary.
- **Agent-friendly** — `curlew exec` for one-shot requests (`--stdin`, `--dry-run`, `--log`, `--non-interactive`), and `curlew init --skill claude` scaffolds an agent skill for the project.

Everything is available unconditionally — this repository contains no feature gating.

## Commands

| Command | What it does |
|---|---|
| `run <file\|glob>` | Execute a collection, or everything matching a pattern |
| `exec <url>` | Execute a single request — built for scripts and AI agents |
| `validate <file>` | Check collections (and shared vault templates) without sending requests |
| `init [dir]` | Scaffold a project; `--output <fmt>`, `--skill claude` |
| `info` | Show project metadata — collections, environments, root |
| `schema` | Print the collection JSON Schema; `--project` for `curlew.yaml` |
| `watch <file>` | Re-run on file changes |
| `vault list` | List configured vault provider profiles |
| `import openapi` | Import an OpenAPI 3.x spec into a collection |
| `pr-check` | Gate CI on a run's results file |
| `ui` | Start the local web UI |
| `perf <file>` | Load-test a single request |
| `plugins list` | Discover plugins and show their registered hooks |
| `telemetry` | Opt-in local usage recording (never transmitted) |

## Install

From source (requires Go 1.24+):

```bash
go install github.com/weiqigod/curlew/cmd/curlew@latest
```

Or clone and build:

```bash
git clone https://github.com/weiqigod/curlew
cd curlew
go build ./cmd/curlew
```

## Quickstart

```bash
mkdir demo-api && cd demo-api
curlew init                          # scaffolds curlew.yaml, collections/, environments/
curlew run collections/sample.yaml   # run one collection
curlew run "collections/**/*.yaml"   # run everything matching a glob
curlew ui                            # open the local web UI
```

`curlew --help` lists all commands. `run`, `exec`, `validate`, and `info` support `--format json` for scripting; `exec` additionally takes `--non-interactive`.

## Documentation

- **[docs/MANUAL.md](docs/MANUAL.md)** — the complete tutorial-style manual, from your first request to parallel, data-driven suites. Start here.
- **[docs/CLI_SPECIFICATION.md](docs/CLI_SPECIFICATION.md)** — what the CLI must do: contracts, file formats, precedence, exit codes, conformance.
- **[docs/UI_SPECIFICATION.md](docs/UI_SPECIFICATION.md)** — the local web UI.
- **[docs/EVENTS_SCHEMA_v1.3.md](docs/EVENTS_SCHEMA_v1.3.md)** — the NDJSON event stream contract.
- **[site/](site/)** — a static examples cookbook.

## Repository layout

| Path | What it is |
|---|---|
| `cmd/curlew/` | CLI entry point (the product — a single static Go binary) |
| `internal/` | CLI implementation packages |
| `ui/` | Svelte single-page app served by `curlew ui` (embedded at build time) |
| `src/` | C#/.NET backend + web dashboard, kept for reference. The CLI does not talk to it. |
| `smoke/` | Hermetic end-to-end smoke suite (local fixture server, no public internet) |
| `docs/` | Manual, specifications, and design history |

The CLI is entirely local. It has no account, no login, and makes no network calls
other than the HTTP requests your collections define. Shared vault templates come from a
local file (`CURLEW_TEAM_CONFIG`), `pr-check` reads a local results file, and `telemetry`
(opt-in) appends to a local NDJSON file that is never transmitted.

## Development

```bash
go build ./cmd/curlew          # build
go test ./...                   # unit + integration tests
golangci-lint run               # lint
./smoke/run.sh                  # hermetic smoke suite
./scripts/ci-local.sh           # full local CI gate
```

The UI lives in `ui/` (`npm ci && npm run build` rebuilds the embedded assets; `npm test` and `npm run test:e2e` cover it). Development follows TDD with vertical slices — see [docs/DEVELOPMENT_PHILOSOPHY.md](docs/DEVELOPMENT_PHILOSOPHY.md).

## License

Apache License 2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE).

This covers the `curlew` CLI and everything you need to build, run and extend it.
The `src/` (.NET backend), `web/` (dashboard) and `deploy/` directories are proprietary
and carry their own LICENSE files. The CLI does not depend on any of them: a binary built
from this repository contains no proprietary code.
