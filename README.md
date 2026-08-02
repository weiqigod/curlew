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

- **Plain-file collections** — YAML in, exit codes out. `{{variable}}` interpolation with a clear precedence chain (project config, environments, `.env`, shell-outs, CLI flags).
- **Rich assertions** — status, headers, JSONPath body checks, numeric proximity, JSON Schema validation.
- **Request chaining** — extract values from one response, use them in the next; `depends_on` with automatic skip on failure.
- **Composition** — `include:` shared requests across collections; setup/teardown phases.
- **Data-driven testing** — run a request once per CSV/JSON row, sequentially or in parallel.
- **Parallel execution** — dependency-aware waves (`--parallel`, `--show-dependencies` prints the graph).
- **Protocols** — HTTP/REST, GraphQL (inline or `.graphql` files with fragments), WebSocket step scripts.
- **Auth** — dynamic auth profiles (login → extract → cache), request signing (AWS SigV4, OAuth 1.0), vault providers for secrets.
- **Output for humans and machines** — colored terminal, JSON, JUnit, TAP, HTML reports, markdown artifacts, NDJSON event streams (`--events`), deterministic runs with `--seed` and localized fake data.
- **Local web UI** — `curlew ui` starts a localhost runner/inspector over your project. Files stay the source of truth; the UI never edits them.
- **Watch mode, glob discovery, OpenAPI import, plugins, load testing** (`curlew perf`), and an `exec` command built for one-shot use by scripts and AI agents.

Everything is available unconditionally — this repository contains no feature gating.

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

`curlew --help` lists all commands; every command supports `--format json` and `--non-interactive` for scripting.

## Documentation

- **[docs/MANUAL.md](docs/MANUAL.md)** — the complete tutorial-style manual, from first request to distributed execution. Start here.
- **[docs/UI_SPECIFICATION.md](docs/UI_SPECIFICATION.md)** — the local web UI.
- **[docs/EVENTS_SCHEMA_v1.3.md](docs/EVENTS_SCHEMA_v1.3.md)** — the NDJSON event stream contract.
- **[site/](site/)** — a static examples cookbook.

## Repository layout

| Path | What it is |
|---|---|
| `cmd/curlew/` | CLI entry point (the product — a single static Go binary) |
| `internal/` | CLI implementation packages |
| `ui/` | Svelte single-page app served by `curlew ui` (embedded at build time) |
| `src/` | Optional C#/.NET backend + web dashboard for team features (shared vault templates, scheduled runs, PR status checks) |
| `smoke/` | Hermetic end-to-end smoke suite (local fixture server, no public internet) |
| `docs/` | Manual, specifications, and design history |

The CLI is fully standalone — the backend is only needed for the optional team-oriented features reached via `curlew login`.

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

Apache License 2.0 — see [LICENSE](LICENSE).
