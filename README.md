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

`curlew init` writes exactly that file. [Quickstart](#quickstart) has the
commands, the bytes they print, and the test that runs them.

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
- **Redaction on by default** — values marked sensitive print as `[REDACTED]` in every format, including reports and logs. `--allow-sensitive` opts back in for supported run outputs; Markdown remains redacted.

**Scale**
- **Data-driven testing** — run a request once per CSV or JSON row, sequentially or in parallel.
- **Parallel execution** — dependency-aware waves via `--parallel`; `--show-dependencies` prints the graph as DOT, and adding `--dry-run` prints the execution waves.
- **Load testing** — `curlew perf` with `--vus`, `--duration`, `--ramp-up` and `--rps`.

**Output**
- **Formats** — colored terminal, `json`, `tap`, `junit`, `html`, and `markdown` artifacts (`--format`; `--report` targets Markdown, HTML and JUnit, while JSON/TAP use stdout).
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
- **Agent-friendly** — `curlew exec` for one-shot requests (`--stdin`, `--dry-run`, `--log`, `--non-interactive`), and `curlew init --skill agent` scaffolds an Agent Skill at `.claude/skills/curlew/` — a project skill directory read by both Claude Code and GitHub Copilot.

Local CLI features are available without account or tier gating. The retained
platform directories have a separate scope; see the [documentation map](docs/README.md).

## Commands

| Command | What it does |
|---|---|
| `run <file\|glob>` | Execute a collection, or everything matching a pattern |
| `exec <url>` | Execute a single request — built for scripts and AI agents |
| `validate <file>` | Check collections (and shared vault templates) without sending requests |
| `skill install/update` | Install or safely update the agent skill; requires `--agent codex`, `claude` or `copilot` |
| `init [dir]` | Scaffold a project; `--output <fmt>`, `--skill agent` |
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

The repository is private, so every path below needs GitHub access — an
authenticated `gh`, or git credentials for `github.com/weiqigod`. There is no
anonymous install: a plain `curl` against a release asset returns 404, and
`go install` without `GOPRIVATE` fails because the public checksum database
cannot read the module.

### Download a release binary

No Go toolchain required. Release archives are named
`curlew_<version>_<os>_<arch>.tar.gz` — `.zip` on Windows — and are served from
`https://github.com/weiqigod/curlew/releases/download/v<version>/curlew_<version>_<os>_<arch>.tar.gz`.

```bash
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
gh release download --repo weiqigod/curlew --pattern "curlew_*_${os}_${arch}.tar.gz"
tar -xzf curlew_*_"${os}"_"${arch}".tar.gz
./curlew --version
```

A binary reports a real version when it was built from a release tag — the
release build injects it at link time. A source build or `go install` at
that tag reports the same version too, but only once that tag's own source
already carries the fallback in `cmd/curlew/version.go`. `v0.1.0` predates
it, so a source build or `go install` at `v0.1.0` reports `0.1.0-dev`
either way. Built from an untagged commit, every path reports `0.1.0-dev`.

### Install from source (requires Go 1.24+)

This installs the CLI only: `go install` does not build frontend assets. For the
browser UI, download a release or use the complete checkout build below.

```bash
GOPRIVATE='github.com/weiqigod/*' go install github.com/weiqigod/curlew/cmd/curlew@latest
```

### Clone and build

The complete app requires Go 1.24+ and Node.js 22+ at build time. Build the frontend
first so the executable includes its JavaScript and CSS. No Node.js is needed to
run the finished binary.

```bash
git clone https://github.com/weiqigod/curlew
cd curlew
./scripts/build-ui.sh
go build ./cmd/curlew
```

Every command block above is executed by
`TestReadme_install_commands_execute` — see `cmd/curlew/readme_install_exec_test.go`.

## Quickstart

`curlew init` points `base_url` at `https://httpbin.org`. Set `BASE_URL` to the
server you want to test — a public API, or a service on your own machine — and
pass it with `--var`, the highest-precedence variable source (see
[CLI_SPECIFICATION §6.2](docs/CLI_SPECIFICATION.md)).

```bash
mkdir demo-api && cd demo-api
curlew init
curlew run collections/sample.yaml --var base_url="$BASE_URL"
```

That prints:

```
Project initialized successfully!

Created:
  curlew.yaml
  .gitignore
  .env.example
  environments/dev.yaml
  collections/sample.yaml

Next steps:
  curlew run collections/sample.yaml
Collection: Sample Collection
  ✓ Hello World  200  12ms

────────────────────────────────
  1 request(s): 1 passed, 0 failed (12ms)
```

The `12ms` figures are illustrative — a real run's durations vary and are the
only thing the test below normalises before comparing.

`curlew run "collections/**/*.yaml"` runs everything matching a glob. `curlew ui`
starts the local web UI and runs until you stop it, so it is not part of the
block above. `curlew --help` lists all commands; `run`, `exec`, `validate` and
`info` support `--format json`, and `exec` additionally takes
`--non-interactive`.

Every command in the block above is executed by
`TestReadme_quickstart_actually_works` (`cmd/curlew/readme_quickstart_test.go`)
against a local [mudflat](docs/TESTAPI_SPECIFICATION.md) server, and the output
above is what it asserts — durations normalised, everything else byte for
byte.

## Use with an AI agent

The current source includes help, dry-run, skill-reference and skill installation/update improvements added after
v0.1.0. Build this checkout to use them until the next release.

Curlew integrates through shell commands, files, JSON results and NDJSON events.
**There is no MCP server.** The JSON-RPC plugin API extends request processing; it
is not an MCP connection.

In a new project, run `curlew init --skill agent`. This installs an agent playbook
and configures Markdown reports plus structured events. For an existing project,
use `curlew skill install --agent codex` (or `claude` / `copilot`). Update with
`curlew skill update --agent codex`; conflicting team edits are preserved. Start with the
[agent guide](docs/AGENT_GUIDE.md) for discovery, validation, execution and failure
handling. Use `curlew <command> --help` to inspect one command without executing it.

## Documentation

- **[docs/README.md](docs/README.md)** — documentation map: current CLI guides, platform material, and historical decisions.
- **[docs/AGENT_GUIDE.md](docs/AGENT_GUIDE.md)** — agent setup and tested command recipes.
- **[docs/MANUAL.md](docs/MANUAL.md)** — the complete tutorial-style manual, from your first request to parallel, data-driven suites. Start here.
- **[docs/CLI_SPECIFICATION.md](docs/CLI_SPECIFICATION.md)** — what the CLI must do: contracts, file formats, precedence, exit codes, conformance.
- **[docs/UI_SPECIFICATION.md](docs/UI_SPECIFICATION.md)** — the local web UI.
- **[docs/EVENTS_SCHEMA_v1.6.md](docs/EVENTS_SCHEMA_v1.6.md)** — the NDJSON event stream contract.
- **[site/](site/)** — a static examples cookbook.

## Repository layout

Every top-level directory is accounted for below, and the accounting is enforced: `go test
./internal/docs/ -run TestReadme_accounts_for_every_top_level_directory` fails if a directory
is added with no row, or a row survives after its directory is gone.

| Directory | What it is | Relationship to the CLI |
|---|---|---|
| `.claude/` | 9 slash commands, 2 skills, `launch.json` | none — workflow only |
| `.github/` | 8 workflows, all `workflow_dispatch`-gated (`go.yml` delegates to `ci-local.sh --go`); issue templates and a PR template | none at build or test time |
| `cmd/` | `cmd/curlew` (entry point) + `cmd/curlew-agent-harness` (test-only) | **build time — is the binary** |
| `deploy/` | self-hosted docker-compose for the platform; own proprietary LICENSE | none |
| `docs/` | manual, both specifications, event-schema versions, the debt registers | test time — `internal/docs` executes the MANUAL/CLI_SPECIFICATION tables and prose |
| `examples/` | a real Go plugin (`datadog-metrics`), output-block samples | test time — built by `smoke/run.sh`; a seed source for `internal/fuzzseed` |
| `internal/` | CLI implementation packages | **build time** |
| `management/` | backlog, tasks, plans, reviews | test time — `internal/backlog` reconciles it as a named gate step |
| `sample/` | `sample/hello.yaml` | test time — `smoke/run.sh`; `internal/schema` walks it |
| `schemas/` | canonical JSON Schemas + a Go package that embeds them | **build time — `//go:embed`**; served by `curlew schema` |
| `scripts/` | `ci-local.sh` (the gate) plus stack/seed/signing helpers | build and gate tooling; `build-ui.sh` produces the embedded UI assets |
| `site/` | standalone static examples cookbook (SvelteKit, prerendered) | none — `site/README.md` states it is separate from `web/`; no gate builds it |
| `smoke/` | hermetic smoke suite + fixtures | test time — runs the built binary |
| `src/` | `ApiTool.Backend` + tests; own proprietary LICENSE | **none — platform, frozen** |
| `templates/` | split: `templates/skills/` is embedded into the binary; `templates/email/` is MJML for the .NET backend | **partly build time**, partly platform |
| `testapi/` | Mudflat, the API the CLI is dogfooded against; forbids importing curlew | test time — 6 dogfood gate steps; never linked into the binary |
| `testdata/` | repository-root fixtures | test fixtures only |
| `ui/` | the Svelte SPA served by `curlew ui` | **build time, indirectly** — `scripts/build-ui.sh` builds it into the embedded `internal/uiserver/assets/dist` |
| `web/` | SvelteKit platform dashboard; own proprietary LICENSE; holds the 5 convergence specs | **none — platform, frozen**; the specs seed the backend over HTTP, no `curlew` binary involved |

The `src/` backend and `web/` dashboard stay in this repository, frozen: they build and pass their tests in `./scripts/ci-local.sh --full`, no new feature work is planned, and the `curlew` CLI does not call them. See [docs/TECH_CHOICES.md](docs/TECH_CHOICES.md#repository-shape) for the reasoning behind that decision.

The CLI is entirely local. It has no account, no login, and makes no network calls
other than the HTTP requests your collections define. Shared vault templates come from a
local file (`CURLEW_TEAM_CONFIG`), `pr-check` reads a local results file, and `telemetry`
(opt-in) appends to a local NDJSON file that is never transmitted.

## Development

```bash
./scripts/build-ui.sh          # build embedded browser assets
go build ./cmd/curlew          # build executable
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
