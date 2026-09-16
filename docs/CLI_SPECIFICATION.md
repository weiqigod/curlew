# Curlew CLI Specification

## A File-Based HTTP Testing and Validation Tool

<!-- doc-check: prose-not-executable the document's own completeness claim about itself, not a claim about the binary's behaviour -->

**Version:** 1.0
**Date:** 2026-09-15
**Status:** Describes the CLI as shipped. Every behaviour in this document is implemented in `cmd/curlew` and `internal/`.
**Scope:** The `curlew` binary only. The `src/` .NET backend and `web/` dashboard are specified separately in [SPECIFICATION.md](SPECIFICATION.md) and are not part of this document.

> **Relationship to the other documents.** This specification states what the CLI
> must do — contracts, invariants, precedence, formats, algorithms, exit codes.
> [MANUAL.md](MANUAL.md) teaches a user how to operate it. Where a behaviour is
> testable, this document is the contract the test asserts against; where a
> behaviour is merely convenient, the manual explains it. The two are expected to
> agree. If they disagree, the binary is the tiebreaker and both documents are
> wrong until corrected.

---

## Table of Contents

1. [Scope and Non-Goals](#1-scope-and-non-goals)
2. [Design Principles](#2-design-principles)
3. [Execution Model](#3-execution-model)
4. [Project Layout and Discovery](#4-project-layout-and-discovery)
5. [File Formats](#5-file-formats)
6. [Variable System](#6-variable-system)
7. [Assertions](#7-assertions)
8. [Extraction](#8-extraction)
9. [Retry](#9-retry)
10. [Data-Driven Testing](#10-data-driven-testing)
11. [Parallel Execution](#11-parallel-execution)
12. [Protocols](#12-protocols)
13. [Authentication and Secrets](#13-authentication-and-secrets)
14. [Request Signing](#14-request-signing)
15. [Plugins](#15-plugins)
16. [Output and Reporting](#16-output-and-reporting)
17. [Exit Codes](#17-exit-codes)
18. [Command Reference](#18-command-reference)
19. [Environment Variables](#19-environment-variables)
20. [Local Web UI](#20-local-web-ui)
21. [Performance Testing](#21-performance-testing)
22. [Telemetry](#22-telemetry)
23. [CI Integration](#23-ci-integration)
24. [Limits and Guard Rails](#24-limits-and-guard-rails)
25. [Conformance](#25-conformance)
- [Appendix A — Collection JSON Schema](#appendix-a--collection-json-schema)
- [Appendix B — Deliberately Absent Surfaces](#appendix-b--deliberately-absent-surfaces)

---

## 1. Scope and Non-Goals

### 1.1 What Curlew Is

Curlew is a file-based API testing tool: a Postman replacement for developers who
prefer files, version control, and CLI workflows. Tests are YAML files, committed
alongside the code they exercise, reviewed in pull requests, and executed by a
single static binary in CI.

The binary is the whole product. It is distributed as one statically linked
executable with no runtime dependencies beyond the operating system, and no
install-time service configuration.

### 1.2 Locality Guarantee

**Curlew is entirely local.** This is a hard contract, not a default:

- There is no account, no registration, no login, and no license key.
- The binary makes no network request other than the HTTP requests a collection
  defines, the subprocess calls a `from_command` variable or vault provider
  makes, and the plugin processes the user launches.
- Every feature is unconditionally available. There are no tiers, no
  feature gates, no trials, and no upgrade prompts.
- Telemetry is opt-in, off by default, and writes to a local file. Nothing is
  transmitted.

Any code path that would contact a first-party service is a defect against this
specification.

### 1.3 Non-Goals

Curlew deliberately does not provide:

<!-- doc-check: table-not-executable a statement of what the CLI declines to be; there is nothing to run -->
| Non-goal | Rationale |
|---|---|
| A GUI for authoring | Files are the interface. The local UI (§20) reads and runs; it never writes collections. |
| A hosted service, account, or sync | See §1.2. Collections sync through the user's version control. |
| Exploratory API browsing | The tool validates known-good expectations; it is not a client for poking at unfamiliar APIs. |
| Mocking or stubbing servers | Out of scope. Use a dedicated mock server and point a collection at it. |
| Contract-test publication | Curlew asserts against live responses; it does not broker consumer/provider contracts. |

### 1.4 Terminology

- **Collection** — a YAML file naming a test suite with `setup`, `requests`, and
  `teardown` phases.
- **Request item** — one entry in a phase: a name, a request envelope, and
  optional assertions, extractions, retry, data-driven, and auth configuration.
- **Phase** — one of `setup`, `requests`, `teardown`.
- **Assertion** — a check against a response that contributes to a request
  item's pass/fail outcome.
- **Extraction** — pulling a value out of a response body by JSONPath into a
  named variable visible to later request items.
- **Environment** — a named file of variable overrides selected with `--env`.
- **Auth profile** — a named configuration that runs a collection to produce a
  token, cached with a TTL.
- **Wave** — a set of request items that may execute concurrently under
  `--parallel`, computed from the dependency graph.
- **Redaction** — replacement of a sensitive value with `[REDACTED]` in all
  output streams, in both directions: values curlew sent and values the server
  returned.

---

## 2. Design Principles

### 2.1 Tests as Code

Collections are plain YAML in the repository. They diff meaningfully, review in
pull requests, and carry no binary or machine-specific state. Nothing Curlew
needs to run a test lives outside the project directory except the user's own
secrets.

### 2.2 Progressive Sophistication

<!-- doc-check: prose-not-executable narrative description of the format's generality, not a specific checkable behaviour -->

A first test is a single file with an inline request and a status assertion. The
same file format scales to external request files, environments, composition via
`include`, data-driven iteration, retry policy, and parallel waves. No rewrite is
required to move along that path, and the execution engine does not distinguish
inline from external request items — externals are resolved at load time and
treated identically thereafter.

### 2.3 Always Runnable

The binary builds cleanly, starts without errors, and does what its help text
claims. Features that are not implemented are not advertised: there are no flags
or commands that parse but do nothing. Help text is part of the contract.

### 2.4 Machine-Readable by Default

Every command that produces a verdict can produce it as structured data.
`--format json` is available on `run`, `exec`, `validate`, and `info`; `run` also
emits TAP, JUnit, HTML, and Markdown. Exit codes are stable and documented
(§17). The `exec` command exists specifically so an AI agent or shell script can
issue one request and parse one JSON object.

### 2.5 Secrets Are Redacted Unless Proven Safe

Redaction is the default and the burden of proof runs the other way: a value is
shown in plain text only when it is neither tagged, nor vault-resolved, nor
name-matched by the heuristic, nor derived from a value that was. `--allow-sensitive`
is the single per-run escape hatch, and the local UI does not offer even that.

### 2.6 Determinism Where It Is Affordable

Given `--seed`, every random and faker function produces identical values across
runs, platforms, and binary versions. Dependency analysis, wave assignment, and
output ordering are deterministic for a given collection. Wall-clock functions
and network timing are the documented exceptions.

---

## 3. Execution Model

### 3.1 Run Lifecycle

```
Load project config (curlew.yaml, walking up from the collection directory)
  → Load environment file (--env) and .env
  → Parse and validate collection; resolve include: and external path: files
  → Resolve variables and build the sensitivity set
  → Execute setup phase
  → Execute requests phase (sequential, or waves under --parallel)
  → Execute teardown phase
  → Aggregate results, format output, determine exit code
```

Parse and validation errors abort before any HTTP traffic. Variable resolution
errors abort before the request they affect.

### 3.2 Phases

| Phase | Ordering | On failure |
|---|---|---|
| `setup` | Always sequential | A failed item marked `required: true` aborts the run. Otherwise the run continues and the failure is recorded. |
| `requests` | Sequential by default; waves under `--parallel` | Governed by `options.stop_on_failure`. |
| `teardown` | Always sequential | Executes even when the requests phase failed. |

Variables extracted in `setup` are visible to `requests` and `teardown`.
Variables extracted in `requests` are visible to `teardown`.

### 3.3 Per-Request Lifecycle

For each request item, in order:

1. Evaluate `if:` (§3.4). If false, the item is skipped.
2. Evaluate `depends_on:` skip propagation (§3.4). If any named item was skipped
   in the same phase, this item is skipped.
3. Interpolate variables into method, URL, headers, query, and body.
4. Apply the auth profile, if any.
5. Apply request signing, if any (§14).
6. Call plugin `on_request` hooks in declared order (§15).
7. Execute the request, applying retry policy (§9).
8. Call plugin `on_response` hooks.
9. Evaluate assertions (§7).
10. Perform extractions (§8).

At run completion, plugin `on_result` hooks are called once.

### 3.4 Conditional Execution

**`if:`** takes a CEL boolean expression. It is evaluated *before* templating and
may reference:

- `vars.<name>` — the resolved variable scope
- `env.<name>` — environment variables
- `previous.status`, `previous.headers`, `previous.body` — the last non-skipped
  response in the run

```yaml
- name: notify
  if: 'previous.body.status == "pending"'
  request: { method: POST, url: "{{base_url}}/notify" }
```

An item whose `if:` is false is reported as SKIPPED and does not contribute a
pass or a fail.

**`depends_on:`** serves two distinct purposes:

1. **Skip propagation** (all modes). If any named item was skipped in the same
   phase, this item is skipped too. Names must match an existing request item in
   that phase, case-sensitively; an unknown name is a parse error (exit 3).
2. **Wave ordering** (under `--parallel` only). See §11.

```yaml
- name: confirm-order
  if: 'vars.confirm == "yes"'
- name: notify
  depends_on: [confirm-order]     # skipped when confirm-order is skipped
```

### 3.5 Request Selection

`--only "<name>"` restricts the requests phase to the named item. The flag is
repeatable and the selections form a union. Setup and teardown still run in
full — the flag narrows the main sequence only, so an auth setup step still
provides its token. A name that matches nothing is an error (exit 3).

```bash
curlew run collections/users.yaml --only "Get user" --only "Update user"
```

### 3.6 Rate Limiting

`rate_limit_rps` at collection level throttles outbound requests with a token
bucket. `0` (the default) means unlimited. Under `--parallel` the limit applies
across all concurrent workers, not per worker. Data-driven items may set their
own `rate_limit_rps` (§10), which applies to that item's iterations.

---

## 4. Project Layout and Discovery

### 4.1 Canonical Layout

```
curlew.yaml              # project config; its directory is the project root
.env                     # local secrets
.curlewignore            # glob exclusions, .gitignore syntax
environments/
  dev.yaml
  staging.yaml
  prod.yaml
requests/                # external request files
  auth/login.yaml
  users/create.yaml
collections/
  smoke.yaml
  regression.yaml
graphql/
  queries/, mutations/, fragments/
schemas/                 # JSON Schema files for schema: assertions
data/                    # CSV/JSON/YAML files for data-driven tests
.curlew/ui/              # local UI run history (self-gitignored)
```

Only `curlew.yaml` and at least one collection are required. A single collection
file with no project config runs fine; the project root is then the collection's
own directory.

### 4.2 Project Root Resolution

The project root is the nearest ancestor directory of the collection file that
contains `curlew.yaml`. Resolution walks upward from the collection's directory.
If no `curlew.yaml` is found, the collection's own directory is the root and
project-level variables are empty.

Environment files are resolved as `environments/<name>.yaml`, first relative to
the collection's directory, then relative to the project root.

### 4.3 Glob Discovery

`curlew run` accepts a glob pattern instead of a file path. Supported syntax:

| Token | Meaning |
|---|---|
| `*` | Any run of characters within one path segment |
| `?` | Any single character |
| `[abc]` | Character class |
| `**` | Any number of path segments |

```bash
curlew run "tests/**/*_test.yaml"
```

Matched collections execute in lexicographic path order. `.curlewignore` in the
working directory excludes paths using `.gitignore` syntax. The run's exit code
is the worst outcome across all matched collections.

### 4.4 File Reference Resolution

| Reference | Resolved relative to |
|---|---|
| `path:` (external request file) | The referencing collection's directory |
| `include:` | The including collection's directory |
| `body_file:`, `body_binary_file:` | The collection file's directory |
| `assertions.schema:` | The collection file's directory |
| `data_driven.source:` | The collection file's directory |
| `graphql.query_file:`, `graphql.fragments:` | The collection file's directory |
| `environments/<name>.yaml` | Collection directory, then project root |

Path traversal outside the project root is rejected.
---

## 5. File Formats

All input files are YAML 1.2 unless stated otherwise. Unknown keys are rejected
where the schema declares `additionalProperties: false` — this is deliberate, so
that a typo in a field name fails loudly at parse time rather than being silently
ignored.

### 5.1 Collection

The collection is the primary test definition unit.

```yaml
name: string                      # required
description: string
variables: VariableMap
include: [string]                 # other collection files spliced in
retry: RetryConfig                # collection-wide default
rate_limit_rps: int               # 0 = unlimited
signing: SigningSpec              # collection-wide default
config:
  locale: string                  # faker locale for this collection
output: OutputConfig              # per-collection output override
setup:    [RequestItem]
requests: [RequestItem]           # required
teardown: [RequestItem]
options:
  stop_on_failure: bool           # default false
```

`include:` splices other collection files into this one: each included file's
`setup`, `requests`, and `teardown` are appended to the corresponding phase of
the including collection, in the order the includes are listed. Includes are
resolved relative to the including collection's directory. Cyclic includes are
rejected at parse time.

A phase may also carry its own `retry:` default, which sits between the
collection default and any per-item override.

#### 5.1.1 Request Item

```yaml
- name: string                    # required
  path: string                    # external request file; mutually exclusive with request:
  auth: string                    # auth profile name from curlew.yaml
  required: bool                  # default false; in setup, true aborts the run on failure
  if: string                      # CEL boolean guard (§3.4)
  depends_on: [string]            # skip propagation, and wave ordering under --parallel
  retry: RetryConfig              # per-item override
  signing: SigningSpec            # per-item override; `signing: null` disables an inherited default
  data_driven: DataDrivenConfig
  variables: VariableMap          # item-scoped variables
  request: Request
  assertions: Assertions
  extract:
    var_name: "$.jsonpath"
```

`path:` and `request:` are mutually exclusive. An item with `path:` loads an
external request file (§5.2) and may still override `variables:`.

Request item names must be unique within a phase; duplicates are a parse error
(exit 3).

#### 5.1.2 Request

```yaml
request:
  method: string                  # optional; defaults to GET, POST when protocol
                                  # is graphql, WS when protocol is websocket
  url: string                     # required
  headers: { Name: string }
  query:   { param: string }
  body: string | mapping          # mutually exclusive with body_file, body_binary_file
  body_file: string               # text file; {{vars}} interpolated
  body_binary_file: string        # raw bytes; never interpolated
  protocol: http | graphql | websocket    # default http
  graphql: GraphQLConfig          # protocol: graphql
  websocket: WebSocketConfig      # protocol: websocket
```

`body`, `body_file`, and `body_binary_file` are mutually exclusive; setting more
than one is a parse error.

**Body file semantics.** `body_file` reads a text file, interpolates `{{...}}`
placeholders at request time, and auto-detects `Content-Type` from the file
extension. `body_binary_file` ships the file's bytes verbatim with no
interpolation, auto-detecting `Content-Type` from the extension and falling back
to `application/octet-stream`. In both cases an explicit `Content-Type` header
always wins. Both are capped at 50 MB at parse time; there is no flag to raise
the cap.

**Content-Type auto-detection** delegates to the platform MIME database
(Go's `mime.TypeByExtension`), so the exact mapping is the host's, not a table
maintained here. The one exception is `.yaml`/`.yml`: `application/yaml` was
registered late (RFC 9512, 2024) and most hosts have no entry for it, so one is
added — but only where the host is silent, so a host that does know the
extension keeps its own answer. The two variants differ in what happens when
the extension is unknown:

| Variant | Unknown extension |
|---|---|
| `body_file` | Content-Type is left unset; the HTTP client applies its own default |
| `body_binary_file` | Falls back to `application/octet-stream` |

An explicit `Content-Type` header overrides detection in both cases.

#### 5.1.3 Assertions

```yaml
assertions:
  status: int | [int]
  headers:
    Name: { equals: ..., exists: ..., matches: ... }
  body:
    "$.jsonpath": { operator: value }
  timing:
    max_duration_ms: int
  schema: string                  # path to a JSON Schema file
  cel:                            # list of CEL boolean expressions
    - "response.status == 200"
    - cel: "response.body.items.size() > 0"
```

Operators are catalogued in §7. The `schema` file is compiled at parse time, so a
malformed schema fails the run before any request is sent.

### 5.2 External Request File

Same shape as a collection's request item, with the request envelope at the top
level:

```yaml
name: string
request: Request
assertions: Assertions
extract: { var_name: "$.jsonpath" }
```

An external request file runs standalone (`curlew run requests/users/create.yaml`)
or as a `path:` reference from a collection. Variables it references must be
supplied by the project, the environment, the referencing collection, or the CLI.

### 5.3 Environment

```yaml
variables:
  key: scalar                     # nested maps are flattened with underscores
config:
  locale: string
```

Nested maps flatten: `db: { host: x }` becomes `db_host`.

### 5.4 Project Config (`curlew.yaml`)

```yaml
project_name: string              # required
variables: { key: value }
secrets: SecretsConfig            # vault provider block (§13.3)
auth_profiles:                    # (§13.4)
  name:
    type: dynamic
    collection: string
    extract: string
    cache_ttl: int
    refresh_on_failure: bool
defaults:
  retry: RetryConfig
  graphql:
    error_handling:
      partial_success: fail | warn | ignore
output: OutputConfig
config:
  locale: string
ui:                               # (§20)
  port: int                       # 1024–65535
  host: string                    # loopback literals only
  open_browser: bool              # default true
  editor: string                  # command template for the UI's open action
  history:
    enabled: bool                 # default true
    max_runs: int                 # default 50, cap 500
```

`curlew schema --project` emits the JSON Schema for this file.

### 5.5 `.env`

```
# comment
KEY=value
KEY="quoted value"
!sensitive KEY=value
```

Loaded from the project root. Values are strings. The `!sensitive` prefix marks a
key as sensitive regardless of its name.

### 5.6 Data Files

Used by `data_driven.source` (§10):

- **CSV** — first row is the header; each subsequent row is one iteration.
- **JSON** — a top-level array of objects.
- **YAML** — a top-level list of maps.

Format is auto-detected from the file extension and may be forced with
`data_driven.format`.

### 5.7 GraphQL Files

- **Query files** (`graphql/**/*.graphql`) — one operation per file, using
  `$variable` placeholders bound from `graphql.variables`.
- **Fragment files** — one fragment per file (`fragment Name on Type { ... }`),
  concatenated onto the query when listed in `graphql.fragments`.

### 5.8 `.curlewignore`

`.gitignore` syntax. Consulted during glob discovery (§4.3) in the working
directory.

### 5.9 Output Config Block

Accepted in `curlew.yaml` (project default) and in a collection (override):

| Field | Values | Default |
|---|---|---|
| `format` | `terminal`, `json`, `tap`, `junit`, `html`, `markdown` | `terminal` |
| `report` | file path | none |
| `events` | file path | none |
| `verbosity` | `quiet`, `normal`, `verbose`, `debug` | `normal` |

Each field resolves independently:

> CLI flag > collection `output:` > project `output:` > built-in default

Resolution happens once, before any request runs, so watch mode and glob
expansion both honour it.
---

## 6. Variable System

### 6.1 Interpolation Syntax

`{{name}}` interpolates a variable. The scanning pattern is:

```
\{\{([a-zA-Z_][a-zA-Z0-9_\.]*(?:\|[^}]+)?)\}\}
```

| Form | Meaning |
|---|---|
| `{{user_id}}` | Simple reference |
| `{{user_id\|default:123}}` | Reference with fallback if undefined |
| `{{$uuid}}` | Dynamic function (§6.5) — `$` prefix |
| `{{secrets.NAME}}` | Shared vault template alias (§13.5) |

Note what does **not** match: `{user_id}` (single braces) and `{{ user_id }}`
(interior spaces). Both pass through as literal text. A plain reference name is
`[a-zA-Z_][a-zA-Z0-9_]*` — dots are not part of it, so `{{user.id}}` is literal
text too. The only dotted forms are the two above, `$`-prefixed functions and
`secrets.` aliases, each resolved by its own namespace.

Interpolation applies to method, URL, headers, query parameters, body, assertion
expected values, and `body_file` contents. It does not apply to
`body_binary_file`.

Nested references resolve to a depth of 10. A reference cycle
(`a` → `b` → `a`) is a variable resolution error (exit 5). An undefined
reference with no default is also exit 5.

### 6.2 Precedence

Ten sources, lowest to highest. When a name is defined more than once, the
higher-numbered source wins.

| # | Source | Where |
|---|---|---|
| 1 | Dynamic functions | `{{$uuid}}`, `{{$timestamp}}`, … |
| 2 | Project variables | `curlew.yaml` → `variables:` |
| 3 | Environment file | `environments/<name>.yaml` → `variables:`, selected by `--env` |
| 4 | `.env` | `.env` in the project root |
| 5 | `from_command` | `variables: { KEY: { from_command: "…" } }` |
| 6 | Vault secrets | `secrets:` block in `curlew.yaml` |
| 7 | Collection variables | `variables:` in the collection |
| 8 | Request-item variables | `variables:` on a request item |
| 9 | `--env-var` | CLI import of an OS environment variable |
| 10 | `--var` | CLI explicit value — beats everything |

Precedence is resolved once per run, before the first request.

### 6.3 CLI Variable Flags

```bash
curlew run tests.yaml --var base_url=https://local.test    # explicit value
curlew run tests.yaml --env-var API_KEY                    # import OS var of the same name
curlew run tests.yaml --env-var API_KEY=$CI_API_KEY        # import and rename
```

Both flags are repeatable.

### 6.4 `from_command` Variables

A variable may be produced by running a shell command:

```yaml
variables:
  session_token:
    from_command: "vault-helper --issue"
    sensitive: true
    cache: 300              # seconds; reuse within the run
```

The command runs once (or once per cache window) and its stdout, trimmed, becomes
the value. A non-zero exit is a variable resolution error (exit 5).

On POSIX the shell is `/bin/sh -c`. On Windows it is system Windows PowerShell
with profiles disabled and non-interactive encoded scripts; Bash syntax is not
translated. Both use a 30-second execution cap (an earlier caller deadline wins).
Commands must finish their work before returning: remaining descendants are
terminated. Windows uses a Job Object; POSIX uses a process group. A POSIX child
that deliberately creates another process group is outside that containment.

Captured stdout and stderr must be UTF-8. Windows PowerShell output is configured
as UTF-8; external programs must emit it themselves. Trailing LF is removed on
POSIX; trailing LF/CRLF sequences are removed on Windows. Spaces, interior line
endings, and a standalone trailing CR are preserved. Failure messages include
the failure kind/exit code, not command text or captured output that may contain
credentials. Resolved sensitive command values and all vault values are registered
before request/event output.

Vault providers invoke programs with argv and child-only environment overrides,
not user-shell strings. Windows `.exe` arguments support literal quotes and
metacharacters. `.cmd`/`.bat` support direct `%*` forwarding with delayed expansion
disabled: embedded double quotes and control characters other than tab are
rejected before launch. Invocation and environment-entry limits are 8000 UTF-16
units, including transport overhead for the invocation. Wrappers that reparse
arguments with `CALL` or enable delayed expansion are unsupported; use a native
executable or a compatible wrapper. See [Windows notes](WINDOWS.md) for verification
scope; Windows arm64 and the full repository gate remain separate checks.

### 6.5 Sensitivity and Redaction

A value is sensitive if any of the following holds:

1. It is tagged `!sensitive` in YAML or `.env`.
2. It is declared with `sensitive: true` in object form (variable, `from_command`,
   or extraction).
3. It was resolved from a vault provider or a shared vault template.
4. Its name matches the heuristic: the name contains `password`, `token`,
   `secret`, `key`, `auth`, or `credential`, case-insensitively.
5. It is derived by interpolation from a value that is sensitive
   (**propagation**).

```yaml
variables:
  base_url: "https://api.example.com"      # not sensitive
  admin_password:
    value: "Secret123!"
    sensitive: true
  api_key: !sensitive "sk_live_abc123"
```

**Propagation is a one-way ratchet.** Once a value is sensitive, it remains
sensitive at every higher precedence level and through every interpolation that
consumes it. Setting `sensitive: false` on a name that inherited sensitivity is
an error, not an override.

**Redaction** replaces the value with `[REDACTED]` in terminal output, JSON, TAP,
JUnit, HTML, Markdown, event streams, and JSONL logs. Auth-profile variables are
always sensitive with no opt-out.

**Redaction is direction-agnostic.** A sensitive value is replaced wherever it
appears, not only where curlew sent it. That covers the request URL — a token in
a query string appears in no body but in every surface that records the URL — the
request and response headers, both bodies, and each assertion's expected and
actual strings. A failure message exists to print the value that did not match,
which makes it the most likely place for a secret to surface:

```
✗ body $.authorization equals: expected …, got Bearer [REDACTED]
```

Response headers whose *names* are inherently sensitive — `Authorization`,
`Cookie`, `Set-Cookie`, `X-Api-Key`, `X-Auth-Token`, `Proxy-Authorization` — are
replaced whole, as are the actual values of assertions against them. Nothing
registered those values, because they came from the server, and matching part of
a credential does not make the rest of it safe to print. The *expected* string is
left readable: it is the collection author's own text, and hiding it would leave
a failure no one can act on.

`--allow-sensitive` disables the normal redaction pass for a single run.
Markdown remains redacted; the local UI (§20) does not accept the flag.

### 6.6 Dynamic Functions

Functions are referenced with a `$` prefix and evaluate at request time.

**Evaluation is per-request and memoised.** Two references to `{{$uuid}}` within
one request item yield the same UUID; the next request item yields a new one.

```yaml
body:
  id: "{{$uuid}}"              # abc-123
  request_id: "{{$uuid}}"      # SAME: abc-123
  created_at: "{{$timestamp}}" # 1730217600
```

**Type preservation.** A function returning a number interpolates as a JSON
number, not a string: `count: "{{$randomInt(1, 100)}}"` serialises as `42`.

Categories:

| Category | Functions |
|---|---|
| Timestamps | `$timestamp`, `$isoTimestamp`, `$dateAdd(n, unit)` |
| Identifiers | `$uuid`, `$guid` |
| Random numbers | `$randomInt(min, max)`, `$randomFloat(min, max, dp)`, `$randomBoolean` |
| Random strings | `$randomString(n)`, `$randomHex(n)`, `$randomPassword`, `$randomBase64` |
| Encoding | `$base64(s)`, `$base64Decode(s)`, `$urlEncode(s)`, `$jsonEncode(v)` |
| Hashing | `$md5(s)`, `$sha256(s)`, `$hmacSha256(data, key)` |
| Faker | `$faker.*` (§6.7) |

Functions whose names match the sensitivity heuristic are auto-sensitive:
`{{$randomPassword}}` and `{{$randomHex(32)}}` assigned to `api_key` are both
redacted.

### 6.7 Faker Functions

53 functions generating realistic test data, grouped by domain. All are
seed-reproducible (§6.9) and locale-aware (§6.8).

**Personal (10):** `firstName`, `lastName`, `fullName`, `username`, `email`,
`phone`, `phoneInternational`, `ssn`, `namePrefix`, `nameSuffix`

**Location (12):** `address`, `street`, `streetName`, `city`, `state`,
`stateAbbr`, `zipCode`, `country`, `countryCode`, `latitude`, `longitude`,
`timezone`

**Company (5):** `company`, `companySuffix`, `jobTitle`, `department`,
`catchPhrase`

**Internet (9):** `url`, `domain`, `domainSuffix`, `ip`, `ipv6`, `mac`,
`userAgent`, `color`, `hexColor`

**Content (5):** `word`, `words(n)`, `sentence(n)`, `paragraph(n)`, `text(n)`

**Financial (8):** `price(min, max)`, `currencyCode`, `currencyName`,
`currencySymbol`, `creditCard`, `creditCardCVV`, `iban`, `bic`

**File (4):** `fileName`, `fileExtension`, `mimeType`, `imageUrl(w, h)`

**Unconditionally sensitive:** `$faker.ssn`, `$faker.creditCard`,
`$faker.creditCardCVV`, `$faker.iban`. These are redacted regardless of the
variable name they are assigned to.

### 6.8 Locale

Fifteen locales are supported: `en-US` (default), `en-GB`, `de-DE`, `fr-FR`,
`es-ES`, `it-IT`, `pt-BR`, `ja-JP`, `zh-CN`, `ko-KR`, `nl-NL`, `pl-PL`, `ru-RU`,
`sv-SE`, `tr-TR`.

Precedence: built-in default < project `config.locale` < environment
`config.locale` < collection `config.locale` < `--locale`.

Fallback is by language then default: `en-GB` → `en` → `en-US`. Fallbacks are
reported in verbose mode.

### 6.9 Determinism

```bash
curlew run tests.yaml --seed 12345
```

The same seed produces identical values across runs, platforms, and binary
versions for `$randomInt`, `$randomFloat`, `$randomString`, `$randomHex`,
`$uuid`, and every `$faker.*` function. Without `--seed`, values are drawn from
the system entropy source.

Timestamp functions read the wall clock and are **not** affected by `--seed`.

---

## 7. Assertions

An assertion group targets `status`, `headers`, `body`, `timing`, `schema`, or
`cel`. A request item passes when every assertion in every group passes.

### 7.1 Status

A single code, or a list of acceptable ones:

```yaml
assertions:
  status: 200
```

```yaml
assertions:
  status: [200, 201, 204]
```

### 7.2 Headers

Header names are case-insensitive. Three operators:

| Operator | Meaning |
|---|---|
| `equals` | Exact string match |
| `exists` | Presence (`true`) or absence (`false`) |
| `matches` | Regular expression against the value |

### 7.3 Body

The response body is parsed as JSON and each assertion targets a JSONPath
expression. Fourteen operators:

| Operator | Applies to | Meaning |
|---|---|---|
| `equals` | any | Exact equality. Numerics compare as numbers, so `"42"` equals `42`. |
| `matches` | string | Regular expression |
| `exists` | any | Path resolves |
| `not_exists` | any | Path does not resolve |
| `type` | any | One of `null`, `string`, `number`, `boolean`, `array`, `object` |
| `contains` | array, string | Array membership, or substring |
| `contains_all` | array | Every listed item is present |
| `length` | array, string, object | Array length, string length, or object key count |
| `greater_than` | number | `>` |
| `less_than` | number | `<` |
| `greater_than_or_equal` | number | `>=` |
| `less_than_or_equal` | number | `<=` |
| `approximately` | number | `{ value, tolerance }` — passes when `|actual - value| <= tolerance` |
| `in_range` | number | `{ min, max }` — inclusive |

**Bodies that are not JSON.** A body that does not parse as JSON still has a
root, and that root is its text. At `$`, the text operators — `equals`,
`contains`, `matches`, `length`, `exists`, `not_exists` — evaluate against the
raw body, so an event stream, an HTML error page, a CSV export, XML, NDJSON or
plain text can be asserted on:

```yaml
assertions:
  body:
    $: { matches: "(?m)^id: 1$" }
```

Any other path reports that the body is not JSON: there is no `$.foo` in a
document with no structure, and answering "no match at path" would imply there
could have been one. Structural operators at `$` — `type`, `contains_all`, the
numeric comparisons — report the same, so a permissive fallback cannot quietly
turn every operator into a pass.

In `cel:`, `response.body` is the decoded document for a JSON body and the raw
string for anything else, which is what makes the escape hatch usable where the
operator catalogue runs out.

### 7.4 Timing

```yaml
assertions:
  timing:
    max_duration_ms: 500
```

Measures the full request/response cycle for the successful attempt. Under retry,
each attempt is measured independently.

### 7.5 Schema

```yaml
assertions:
  schema: "schemas/user.json"
```

Validates the response body against a JSON Schema file, resolved relative to the
collection's directory. The schema is compiled at parse time: a malformed schema
fails the run before any request is sent.

### 7.6 CEL Assertions

For checks the operator catalog cannot express, `cel:` accepts a list of boolean
CEL expressions:

```yaml
assertions:
  cel:
    - "response.status == 200"
    - cel: "response.body.items.size() > 0"
```

Each list entry is either a plain string (the expression) or a mapping with a
single `cel:` key. A mapping that carries `cel:` alongside any other operator key
is rejected at parse time. A failing expression reports its source text in the
failure message.

### 7.7 Failure Reporting

An assertion failure names the target, the expectation, and the actual value:

```
status | expected one of [201, 204] | got 200
```

A request item with any failed assertion is a FAIL and contributes exit code 1.

---

## 8. Extraction

```yaml
extract:
  user_id: "$.id"
  api_key:
    path: "$.key"
    sensitive: true
```

Extraction runs after assertions. The extracted value enters the variable scope
under the given name, visible to every later request item in the run, subject to
phase ordering (§3.2).

Rules:

- Extraction keys must be static. A key containing `{{` is a parse error — see
  §11.4.
- A JSONPath that does not resolve leaves the variable undefined. A later
  reference then fails with exit 5 unless it supplies a `|default:` fallback.
- Extracted values inherit sensitivity from the name heuristic, or may declare
  `sensitive: true` explicitly in object form.
- Under `--parallel`, two items that can run concurrently may not extract the
  same name — see §11.4.
---

## 9. Retry

### 9.1 Configuration

Retry may be declared at four levels; the nearest wins:

> request item `retry:` > phase `retry:` > collection `retry:` > `curlew.yaml` `defaults.retry:`

```yaml
retry:
  enabled: true
  max_attempts: 5                 # 1 initial attempt + 4 retries
  initial_delay_ms: 500
  max_delay_ms: 30000             # cap for exponential and linear growth
  backoff_strategy: exponential   # exponential | linear | constant
  jitter: true
  jitter_factor: 0.2
  respect_retry_after: true
  retry_on:
    status_codes: [429, 502, 503, 504]
    status_ranges: ["5xx"]        # "Nxx" shorthand or explicit "500-599"
    network_errors: true
    timeouts: true
    methods: [GET, PUT, DELETE]
  do_not_retry_on:
    status_codes: [401, 403]
    methods: [POST]
```

Enabling retry with no further configuration retries on `[429, 502, 503, 504]`
for idempotent methods (GET, HEAD, PUT, DELETE, OPTIONS), up to three attempts,
exponential from 500 ms.

**Evaluation order** for a given attempt's outcome:

1. **Trigger (OR).** The status matches `retry_on.status_codes` or
   `retry_on.status_ranges`, or the failure is a network error and
   `retry_on.network_errors` is true, or it is a timeout and `retry_on.timeouts`
   is true. If nothing triggers, do not retry.
2. **Method restriction (AND).** If `retry_on.methods` is set, the request's
   method must appear in it.
3. **Exclusions (AND NOT).** If the status matches `do_not_retry_on.status_codes`
   or `.status_ranges`, or the method appears in `do_not_retry_on.methods`, do
   not retry.
4. **Warn** if the method is non-idempotent (§9.5).

`do_not_retry_on` therefore always wins: a status or method listed in both is not
retried.

### 9.2 Status Ranges

`status_ranges` accepts the explicit `"min-max"` form (`"500-599"`) and the class
shorthand `"Nxx"` (`"5xx"`, case-insensitive). A range that parses as neither is
a validation error reported by `curlew validate`, not silently ignored.

### 9.3 Backoff Algorithms

| Strategy | Delay for retry *n* (0-based) |
|---|---|
| `exponential` (default) | `initial_delay_ms * 2^n` |
| `linear` | `initial_delay_ms * (n + 1)` |
| `constant` | `initial_delay_ms` |

Every result is clamped to `max_delay_ms`. With `jitter: true` the delay is then
multiplied by `1 + jitter_factor * random(-1, 1)`, so a computed 2000 ms with
`jitter_factor: 0.2` lands anywhere in `[1600, 2400]` ms. Jitter exists to break
up synchronised retry storms across concurrent workers.

Exponential from 500 ms capped at 30000 ms:

| Retry | Delay |
|---|---|
| 1 | 500 ms |
| 2 | 1000 ms |
| 3 | 2000 ms |
| 4 | 4000 ms |
| 5 | 8000 ms |
| 6 | 16000 ms |
| 7 | 30000 ms (capped) |

### 9.4 `Retry-After`

When `respect_retry_after` is true (the default) and the response carries a
`Retry-After` header — integer seconds or an HTTP-date — that value overrides the
computed backoff, subject to a 30-second cap. Set `respect_retry_after: false` to
ignore the header.

### 9.5 Idempotency

POST and PATCH are not idempotent: a retried POST whose first attempt succeeded
server-side but whose response was lost creates a duplicate. Curlew emits a
warning when retry is enabled for non-idempotent methods. Suppress it by
restricting `retry_on.methods`, by listing the method in
`do_not_retry_on.methods`, or by making the endpoint idempotent.

### 9.6 Interaction with Other Features

- Retries occur inside a single request item's execution. Assertions and
  extraction run once, against the final attempt.
- A request item that fails after exhausting its attempts fails once. Dependent
  items observe one failure, not the intermediate attempts.
- Plugin `on_request` and `on_response` hooks fire per attempt.
- Timing assertions (§7.4) measure each attempt independently.

---

## 10. Data-Driven Testing

A request item with `data_driven:` executes once per row of a data file. Every
column becomes a variable for that iteration.

```yaml
- name: Create user
  data_driven:
    source: "./data/users.csv"
    format: csv                     # csv | json | yaml; auto-detected from extension
    filter: "{{age}} >= 25"         # evaluated per row, after interpolation
    limit: 100                      # max rows after filtering
    start_row: 0                    # 0-based, inclusive
    end_row: 99                     # 0-based, inclusive
    fail_fast: false
    parallel: false
    rate_limit_rps: 5               # 0 or absent = unlimited
    store_results: all              # all | summary | failed_only
  request:
    method: POST
    url: "{{base_url}}/users"
    body:
      id: "{{user_id}}"
      email: "{{email}}"
  assertions:
    status: 201
```

### 10.1 Row Selection Order

`start_row`/`end_row` slice first, then `filter` runs, then `limit` truncates.

### 10.2 Iteration Variables

| Variable | Meaning |
|---|---|
| `{{_index}}` | Zero-based row index |
| `{{_count}}` | One-based row number |
| `{{_total}}` | Total rows that will execute |
| `{{_iteration}}` | One-based row number — alias of `{{_count}}` |
| `{{_row_number}}` | One-based row number — alias of `{{_count}}` |

### 10.3 Result Storage

| `store_results` | Retained |
|---|---|
| `all` (default) | Every iteration's full result |
| `summary` | Aggregate counts only |
| `failed_only` | Full results for failing iterations, counts for the rest |

### 10.4 Large Dataset Guard

A data file yielding more than 10,000 rows requires `--confirm-large-dataset`.
Without it the run exits with code 2 before issuing any request. This is a
safety interlock against pointing a 50,000-row file at a production API, not a
capacity limit.

### 10.5 Parallel Iterations

`parallel: true` runs iterations concurrently with up to 20 workers.
`rate_limit_rps` is enforced across all workers, not per worker. Extraction from
parallel iterations accumulates into an array whose order follows completion, not
row order.

---

## 11. Parallel Execution

`--parallel` executes the requests phase as a sequence of waves. Setup and
teardown remain sequential. Without the flag, requests run in file order
regardless of any declared dependencies — the flag is the opt-in.

### 11.1 Dependency Sources

An edge from item *j* to item *i* (meaning *i* must wait for *j*) is created when
either holds:

1. **Explicit** — *i* names *j* in `depends_on:`.
2. **Inferred** — *j* extracts a variable that *i* references, and *j* precedes
   *i* in file order.

Inference means a correct collection usually needs no `depends_on` at all;
declare it when the ordering constraint is real but invisible to the variable
scan (for example, a server-side side effect).

### 11.2 Analysis Algorithm

**Phase 1 — Load.** Parse the collection, resolve `include:` and every external
`path:` file, and flatten to a single ordered list of request items.

**Phase 2 — Variable analysis.** For each item compute:

- *produced* — the set of `extract:` keys. A key containing `{{` is rejected
  (§11.4).
- *referenced* — every `{{name}}` occurrence in the URL, headers, query, body,
  assertions, and extraction paths, excluding names that start with `$` (dynamic
  functions) and names already resolvable before execution (project,
  environment, `.env`, collection, and CLI variables).

Nested references resolve to depth 10; exceeding it produces a warning.

**Phase 3 — Build edges.** For each ordered pair (*j* < *i*), the intersection of
*j*'s produced set with *i*'s referenced set yields an edge labelled with those
variable names. Explicit `depends_on:` entries add edges directly.

**Phase 4 — Cycle detection.** Kahn's algorithm topologically sorts the graph.
Any node that never reaches in-degree zero participates in a cycle; a DFS then
recovers a concrete cycle path for the error message. A cycle is a parse-time
failure (exit 3), before any request is sent.

**Phase 5 — Collision detection.** If two items produce the same variable name
and no path connects them, they can run concurrently and the winner is
non-deterministic. This is an error (exit 3), not a warning.

Complexity is `O(N² + E)` in time and `O(N + E)` in space for *N* items and *E*
edges.

### 11.3 Waves

Items with in-degree zero form wave 1. Removing them and repeating yields wave 2,
and so on. All items in a wave execute concurrently; the next wave begins when
every item in the current wave has completed.

```
Wave 1: Create user, Create product      (concurrent)
Wave 2: Place order                      (waits for both)
```

Setup and teardown act as implicit barriers around the whole wave sequence.

`--show-dependencies` prints the graph in DOT format without executing;
adding `--dry-run` prints the wave assignment instead.

### 11.4 Rejected Constructions

| Construction | Why | Exit |
|---|---|---|
| Dynamic extract key: `"{{prefix}}_id": "$.id"` | The produced-variable set cannot be computed statically | 3 |
| Two concurrent items extracting the same name | Race on the variable's value | 3 |
| Cyclic `depends_on` or inferred cycle | No valid ordering exists | 3 |
| `depends_on:` naming an item not in the same phase | Unresolvable reference | 3 |

### 11.5 Failure and Skip Semantics

When an item fails, every item that depends on it — directly or transitively — is
skipped, and the skip reason names the variable and the producing item:

```
✗ Create User (404, 98ms)
   Assertion failed: expected status 200, got 404

⊘ Get User (skipped)
   Reason: depends on 'user_id' from 'Create User', which failed
```

A dependent whose reference supplies a `|default:` fallback is still skipped when
its producer *failed*. The default covers the narrower case where the producer
succeeded but its JSONPath did not resolve:

| Producer outcome | Variable extracted | Dependent has default | Dependent runs |
|---|---|---|---|
| Success | Yes | — | Yes, with the extracted value |
| Success | No | Yes | Yes, with the default |
| Success | No | No | Skipped |
| Failure | — | Yes | Skipped |
| Failure | — | No | Skipped |

### 11.6 Protocol Constraints

| Protocol | Parallelism |
|---|---|
| HTTP | Fully parallel; stateless |
| GraphQL | Fully parallel; identical to HTTP |
| WebSocket | Connections open in parallel, but steps within one connection are strictly ordered |

Mixed-protocol collections parallelise across protocols; each protocol's own
constraint still applies within an item.
---

## 12. Protocols

`request.protocol` selects the adapter: `http` (default), `graphql`, or
`websocket`. A URL with a `ws://` or `wss://` scheme auto-detects as `websocket`.

Every protocol shares the same surrounding machinery: variable interpolation,
auth profiles, assertions, extraction, retry, data-driven iteration, plugin
hooks, and dependency analysis.

### 12.1 HTTP

The default. Fields are the top-level `request` keys of §5.1.2.

### 12.2 GraphQL

```yaml
request:
  protocol: graphql
  url: "{{base_url}}/graphql"
  headers:
    Authorization: "Bearer {{token}}"
  graphql:
    query: |
      query GetUser($id: ID!) {
        user(id: $id) { id name email }
      }
    query_file: "graphql/queries/get-user.graphql"   # mutually exclusive with query
    fragments:
      - "graphql/fragments/user_fields.graphql"
    variables:
      id: "{{user_id}}"
    error_handling: fail                              # fail (default) | warn | ignore
```

`query` and `query_file` are mutually exclusive. Listed `fragments` are
concatenated onto the query text.

Assertions target the raw response document, so `$.data.*` and `$.errors` are
both addressable:

```yaml
assertions:
  body:
    $.data.user.id: { exists: true }
    $.errors:       { not_exists: true }
```

**Error outcomes.** GraphQL returns HTTP 200 with a populated `errors` array for
both partial and total failures. `error_handling` (per request) and
`defaults.graphql.error_handling.partial_success` (project-wide, accepting
`fail`, `warn`, or `ignore`) decide whether that condition fails the request
item, warns, or is disregarded. The per-request setting wins.

The mode applies to **both** error outcomes, not only to partial success:

| Outcome | When | `fail` | `warn` | `ignore` |
|---|---|---|---|---|
| success | `data` non-null, no errors | pass | pass | pass |
| partial-success | `data` non-null, errors present | fail | warn | pass |
| full-failure | `data` null, errors present | fail | warn | pass |
| empty | no `data` and no `errors` | pass | pass | pass |

This restates `docs/MANUAL.md` §7.1 rather than differing from it. The two
documents disagreeing — this one silent on full failure, the manual describing
the whole matrix — is what let the binary go on treating full failure as
unconditionally fatal for as long as it did.

`ignore` suppresses GraphQL-level error checking only. The request's own
assertions still run, so `$.data` and `$.errors` remain assertable.

**A response that is not a GraphQL document** — an HTML error page from a
gateway that never reached the service, say — fails that request with the parse
error as its actual value. It does not abort the run, and the other requests in
the collection still execute and report.

### 12.3 WebSocket

```yaml
request:
  protocol: websocket
  url: "wss://{{host}}/socket"
  websocket:
    steps:
      - action: send
        message: { type: "subscribe", channel: "orders" }
      - action: expect
        timeout_ms: 5000
        count: 1
        message:
          $.type: { equals: "subscribed" }
      - action: wait
        duration_ms: 1000
      - action: close
    reconnect:
      enabled: true
      max_attempts: 3
      initial_delay_ms: 1000
      backoff: exponential
    heartbeat:
      enabled: true
      interval_ms: 30000
```

Four step actions:

| Action | Purpose |
|---|---|
| `send` | Transmit a payload: `message:` (JSON object), `message_raw:` (literal string), or `message_template:` (external file, interpolated, with optional step-scoped `variables:`) |
| `expect` | Wait for matching messages. `message:` holds JSONPath assertions; `any_of:` accepts alternative assertion sets; `count:` collects N matches (default 1); `timeout_ms:` bounds the wait; `extract:` binds values from the matched messages |
| `wait` | Pause for `duration_ms` |
| `close` | Close the connection |

**Fields are checked against the action.** Each action reads only its own
fields, and a step carrying any other — `timeout_ms` on a `wait`, `count` on a
`close` — is rejected at parse time, naming the field, the action, and where
that field does belong. Unknown fields are rejected too.

| Action | Fields |
|---|---|
| `send` | `message`, `message_raw`, `message_template`, `variables` |
| `expect` | `message`, `any_of`, `timeout_ms`, `count`, `extract` |
| `wait` | `duration_ms` |
| `close` | `code`, `reason` |

<!-- doc-check: prose-not-executable rationale recounting a past mistake, not a claim about current behaviour -->

A field an action ignores is a mistake, not a no-op. The example above once gave
`wait` a `timeout_ms`, which decoded cleanly into a field the executor never
reads, so the documented step paused for zero milliseconds and said nothing —
and survived being written down twice.

**Extraction under `count`.** With `count: 1` — the default — `extract:` binds
the value from the matching message. With `count` greater than 1 it binds a
**JSON-encoded array** of the per-message values, in arrival order:

```yaml
- action: expect
  count: 3
  extract:
    order_ids: "$.order_id"      # -> ["ord_1","ord_2","ord_3"]
```

`{{order_ids}}` interpolates as that literal text, so a request built from it
sends the whole array rather than one element.

**Heartbeat.** With `message:` unset the adapter sends a protocol-level ping
frame and relies on the pong handler. With `message:` set it sends a data frame
and matches the reply against `expect:` assertions.

A heartbeat holds the connection whether or not a step is reading. Reads are
served by a single pump that stays inside the underlying `ReadMessage` for the
life of the connection, which is where control frames — and therefore pongs —
are dispatched; an idle `wait` is a supported way to keep a subscription alive.
Messages arriving during a `wait` are buffered and remain available to the next
`expect`. An orderly close (1000, 1001) during a `wait` ends the step
successfully; a broken connection fails it.

Steps within one connection are strictly ordered and cannot be parallelised.

---

## 13. Authentication and Secrets

### 13.1 Model

Curlew never stores a secret. It resolves secrets at run time from sources the
user already controls — the environment, a file, a subprocess, or a vault CLI —
marks the resolved values sensitive, and redacts them from every output stream.

### 13.2 Ordinary Sources

`.env`, `--env-var`, `--var`, and `from_command` (§6.4) cover most needs and
carry no external dependency.

### 13.3 Vault Providers

Five providers are supported. Curlew shells out to the provider's own CLI rather
than embedding an SDK, so authentication is whatever the user has already
established in their shell.

```yaml
# curlew.yaml
secrets:
  provider: aws-secrets-manager
  region: us-east-1
  keys:
    db_password: "prod/db#password"
    api_key: "prod/api-keys#main"
  cache_ttl: 300
  refresh_on_failure: true
```

| Provider | Key form | Requires |
|---|---|---|
| `aws-secrets-manager` | `path#field` | `aws` CLI, `secretsmanager:GetSecretValue` |
| `azure-key-vault` | secret name, with `vault_name:` | `az` CLI, `az login` |
| `hashicorp-vault` | `secret/data/path#field`, with `address:` and `auth:` | `vault`-reachable endpoint |
| `gcp-secret-manager` | secret name, with `project:` | `gcloud` CLI, `secretmanager.secretAccessor` |
| `1password` | `op://Vault/Item/field` | `op` CLI, signed in |

HashiCorp Vault accepts two auth methods:

```yaml
auth:
  method: approle          # approle | token
  role_id: "demo-api-role"
  secret_id: "${VAULT_SECRET_ID}"
```

**The `#field` suffix** extracts one field from a structured secret (AWS,
1Password, HashiCorp). Without it, the value is expected to be a plain string.

**Caching.** `cache_ttl` is a per-run cache: later requests in the same run reuse
the fetched value. `refresh_on_failure: true` re-fetches once on a retrieval
error instead of aborting.

All vault-resolved values are unconditionally sensitive.

`curlew vault list` prints the configured provider profiles.

### 13.4 Dynamic Auth Profiles

For suites that must log in before they can test, an auth profile runs a
collection and extracts a token:

```yaml
# curlew.yaml
auth_profiles:
  admin:
    type: dynamic
    collection: "auth/admin-login.yaml"
    extract: "access_token"
    cache_ttl: 600
    refresh_on_failure: true
```

A request item opts in by name:

```yaml
- name: List users
  auth: admin
  request: { method: GET, url: "{{base_url}}/users" }
```

The profile collection runs once per cache window. Its extracted value is always
sensitive. `refresh_on_failure: true` re-runs the profile once when a request
using it fails in a way that suggests an expired token. Under `--parallel`, a
profile completes before any item that references it — an implicit dependency.

### 13.5 Shared Vault Templates

A team can share one vault-coordinate mapping across a project. The template is a
local file; the CLI never downloads it.

```bash
export CURLEW_TEAM_CONFIG=~/team/curlew-team-config.yaml
curlew run collections/users.yaml --env staging
```

The template maps aliases to provider coordinates per environment. `--env`
selects which environment's aliases back the collection's `{{secrets.ALIAS}}`
references. Selecting an environment the template does not define is an error.

When `CURLEW_TEAM_CONFIG` is set, `--env` naming an environment with no
`environments/<name>.yaml` file is tolerated: the flag is understood as selecting
the template's environment. This tolerance applies only when the variable is set.

`CURLEW_VAULT_STUB=1` substitutes an in-memory stub provider, for local and CI
testing without real vault access.

`curlew validate` checks shared vault templates (`team_secrets.vault_configs`)
alongside collections.

---

## 14. Request Signing

`signing:` attaches a signature scheme to a request. It may be declared at
collection level as a default and overridden per request item. `signing: null`
on an item disables an inherited default — this is distinguished from an absent
field.

```yaml
signing:
  type: aws-sigv4
  params:
    region: us-east-1
    service: execute-api
```

Two schemes are registered:

| Type | Purpose |
|---|---|
| `aws-sigv4` | AWS Signature Version 4 |
| `oauth1` | OAuth 1.0a, with `consumer_key`, `token`, `method`, `nonce`, `timestamp` parameters |

Signing runs after interpolation and auth-profile application, and before the
request is dispatched, so the signature covers the final bytes. Signing
parameters follow the same sensitivity rules as any other variable.

---

## 15. Plugins

Plugins are external processes. Curlew launches them, speaks a line-oriented JSON
protocol over stdin/stdout, and never loads foreign code into its own address
space.

### 15.1 Discovery

`CURLEW_PLUGINS` holds a colon-separated (semicolon on Windows) list of plugin
executables or directories. Directories expand to their executable entries.

```bash
export CURLEW_PLUGINS=/usr/local/lib/curlew-plugins:/home/me/my-plugin
curlew plugins list
```

`curlew plugins list` performs the handshake with each discovered plugin and
prints its name, version, and registered hooks.

### 15.2 Handshake

Curlew sends a `curlew/hello` request; the plugin replies with its name,
version, and the hooks it registers. A plugin that fails to respond in time is
reported by `plugins list` and excluded from the run.

### 15.3 Hooks

Three lifecycle hooks, called in the order plugins were declared:

| Hook | When | May do |
|---|---|---|
| `on_request` | Before each request is sent | Mutate method, URL, headers, body, query parameters |
| `on_response` | After each response | Attach annotations. Status, headers, and body are **not** replaceable |
| `on_result` | Once at run completion | Observe pass/fail counts and per-test rows |

Retry attempts and data-driven iterations fire the per-request hooks once per
attempt and per iteration respectively.

### 15.4 Fault Behaviour

Each hook invocation has a 10-second timeout. A plugin that exceeds it is
terminated and the run continues as though the hook had never been registered. A
plugin crash is contained the same way: the run proceeds, and the failure is
reported. Plugins cannot fail a run that would otherwise pass.

The full wire format is documented in [plugins.md](plugins.md).

---

## 16. Output and Reporting

### 16.1 Formats

| Format | Destination | Purpose |
|---|---|---|
| `terminal` (default) | stdout | Human-readable, coloured, progressive |
| `json` | stdout | One structured document per run |
| `tap` | stdout | Test Anything Protocol |
| `junit` | stdout, or `--report <file>` | JUnit XML for CI test reporters |
| `html` | `--report <file>` (required) | Self-contained report |
| `markdown` | `--report <dir>` (required) | One file per request plus an index |

### 16.2 Stream Discipline

Results go to stdout. Diagnostics, warnings, and progress go to stderr. A
consumer may therefore pipe stdout into a parser without filtering. `--format
json` emits exactly one JSON document on stdout and nothing else.

Colour is governed by `--color={auto|always|never}`, defaulting to `auto`.
Under `auto` it is enabled for interactive terminals and disabled when the
`NO_COLOR` environment variable is set to a **non-empty** value, whatever that
value is — the no-color.org convention. An empty `NO_COLOR` expresses no
preference and leaves the TTY check to decide. An explicit `--color` takes
precedence over `NO_COLOR`. `--no-color` is `--color=never`.

`--color=always` cannot put escape codes into a machine format's payload:
`json`, `tap`, `junit`, `markdown` and `html` never construct a terminal
printer for stdout.

### 16.3 Verbosity

| Flag | Level | Shows |
|---|---|---|
| `-q`, `--quiet` | quiet | Summary line only |
| *(none)* | normal | Per-request pass/fail and the summary |
| `-v` | verbose | Adds request and response headers |
| `-vv` | debug | Full HTTP request/response dump |

### 16.4 Markdown Reports

`--format markdown --report <dir>` writes one file per main-phase request plus an
index, for reading in an editor pane or by an agent:

```
responses/
├── run.md                 index: environment, summary, links
├── hello-world.md         one file per main request
└── create-user/           data-driven requests get a directory
    ├── index.md           iteration manifest
    ├── iter-0.md
    └── iter-1.md
```

`run.md` carries the run summary and links to every per-request file; parallel
collections group those links under `## Wave <N>` headings in ascending order.
Slugs derive from the request name (lowercase, dashes, ASCII).

Each per-request file has a CLI-owned region delimited by HTML comment sentinels
and a free region outside it. Content outside the sentinels survives every
re-run, so notes written by a human or an agent are not overwritten.

Three correlation IDs — `run_id`, `request_id`, `request_slug` — link each file
to its event-stream counterpart from the same collection run.
For data-driven reports, remove the `-iter-<zero-based index>` suffix from
the sentinel ID before comparing it to the event request ID; the group index
uses a `-index` suffix on the first request ID. Match run ID and this normalised
request ID. Event slugs include the row name (`each-1-2`); Markdown uses the
group slug (`each/iter-0.md`).

### 16.5 Event Stream

`--events <file>` writes a structured event stream for the run, suitable for
progressive consumption by a supervising process. The same field is settable as
`output.events` in configuration.

### 16.6 JSONL Logging

`curlew exec --log <file>` appends one JSON object per one-shot request.
Redaction applies. `run` does not accept `--log`; use `--events` for collection runs.

---

## 17. Exit Codes

| Code | Meaning | CI treatment |
|---|---|---|
| `0` | Every assertion passed | Pass |
| `1` | Assertion failure; or a top-level usage error — unknown command, unknown flag, missing argument, invalid flag value | Fail — product regression, or fix the invocation |
| `2` | A safety guard tripped (a dataset over 10,000 rows without `--confirm-large-dataset`); or a usage error in `perf` or `pr-check` | Fail — fix the invocation |
| `3` | Parse or configuration error: file not found, invalid YAML, circular include, missing required field, duplicate request name, `--only` matched nothing, dependency cycle, variable collision | Fail — pipeline configuration problem |
| `4` | Execution error: network failure, connection refused, TLS error | Fail — infrastructure |
| `5` | Variable resolution error: undefined variable, circular reference, bad interpolation | Fail — configuration |
| `130` | SIGINT during a `perf` run | Conventional interrupt |

The distinction between `1` and `3` is load-bearing for CI. A `3` means the tests
never started and should be triaged as a pipeline problem, not a product
regression.

> **Known inconsistency.** Usage errors do not map to a single code. `curlew run`
> with no argument, an unknown command, an unknown flag, and an invalid
> `--format` value all exit `1`; `perf` and `pr-check` usage errors exit `2`.
> This is recorded here as observed behaviour, not endorsed as a design. A future
> change that unifies usage errors on `2` would be a deliberate breaking change
> to this table and must be treated as such.

Under glob discovery (§4.3) the run's exit code is the worst outcome across all
matched collections.
---

## 18. Command Reference

`curlew <command> --help` (or `-h`) prints help without loading a project or
executing a request. Nested `import openapi`, `vault list`, and `plugins list`
also accept help. AI integration uses this CLI and its local artifacts; no MCP
server is provided. See [AGENT_GUIDE.md](AGENT_GUIDE.md).

```
curlew <command> [arguments]
```

`--version` is a top-level flag. `--help` is accepted at top level and,
by itself after the command name, on every subcommand.

### 18.1 `run <file|pattern>`

Execute a collection, or every collection matching a glob (§4.3).

| Flag | Meaning |
|---|---|
| `--env <name>` | Load `environments/<name>.yaml`; also selects the shared vault template environment |
| `--env-var VAR` / `--env-var VAR=$OS` | Import an OS environment variable, optionally renaming it. Repeatable |
| `--var key=value` | Set a variable. Highest precedence. Repeatable |
| `--seed <n>` | Seed for deterministic random and faker functions |
| `--locale <code>` | Faker locale |
| `--format <type>` | `terminal`, `json`, `tap`, `junit`, `html`, `markdown` |
| `--report <path>` | Report destination. Required for `html` and `markdown` |
| `--events <file>` | Write the run's event stream |
| `--log <file>` | Append a JSONL record per request |
| `--parallel` | Execute the requests phase in waves (§11) |
| `--only "<name>"` | Restrict the requests phase to the named item. Repeatable, union |
| `--show-dependencies` | Print the dependency graph in DOT format without executing |
| `--dry-run` | Print text wave assignment without sending requests; also works alone |
| `--confirm-large-dataset` | Permit a data file over 10,000 rows |
| `--allow-sensitive` | Disable redaction for this run |
| `--color <when>` | `auto` (default), `always`, or `never`. `always` forces colour on a non-TTY |
| `--no-color` | Disable ANSI colour (same as `--color=never`) |
| `-v` / `-vv` / `-q` | Verbosity (§16.3) |

### 18.2 `exec <url>`

Execute a single request without a collection file. Built for agents and shell
scripts.

| Flag | Meaning |
|---|---|
| `--stdin` | Read the request as JSON from stdin (10 MB limit) |
| `-X`, `--method <M>` | HTTP method, default GET |
| `--dry-run` | Resolve and preview without sending |
| `--log <file>` | Append a JSONL record |
| `--non-interactive` | Suppress interactive prompts on error |
| `--format <type>` | `terminal` or `json` |
| `--var`, `--env-var`, `--seed`, `--locale`, `--color`, `--no-color`, `-v`/`-vv`/`-q` | As for `run` |

### 18.3 `validate <file|glob>`

Parse and validate without executing. Reports syntax errors, unknown fields,
unresolvable references, dependency cycles, malformed retry status ranges, and
invalid JSON Schema files, each pinned to a line. Also validates shared vault
configuration templates (`team_secrets.vault_configs`). `--format json` for
machine output.

### 18.4 `init [dir]`

Scaffold a project: `curlew.yaml`, `.gitignore`, `.env.example`,
`environments/dev.yaml`, `collections/sample.yaml`.

| Flag | Meaning |
|---|---|
| `--project-name <name>` | Override the project name; defaults to the directory basename |
| `--output <format>` | Scaffold an `output:` block for `terminal`, `json`, `tap`, `junit`, `html`, or `markdown` |
| `--skill <name>` | Scaffold an Agent Skill at `.claude/skills/curlew/SKILL.md`. One of `agent` (canonical) or `claude` (compatibility alias); both scaffold the identical payload. Implies `--output markdown`, appends `events: .curlew/run.ndjson`, and adds `.curlew/` to `.gitignore` |

### 18.4.1 `skill <install|update>`

Usage: `curlew skill <install|update> --agent <codex|claude|copilot> [dir]`.
The project directory defaults to the current directory. The explicit agent selects
`.agents/skills/curlew`, `.claude/skills/curlew`, or `.github/skills/curlew` respectively.
All targets receive the same embedded skill with the executable version substituted.

`install` adds missing files and adopts identical ones. `update` requires an
existing SKILL.md and can replace files whose current SHA-256 matches the prior
`.curlew-skill.json` manifest. Unknown files are preserved. Differing unmanaged or
locally edited files cause a conflict before writes. Symlinks and unexpected file
types inside the agent destination are rejected. Neither command modifies project
configuration. The manifest is versioned (schema 1) and committed with the skill.
Files are individually replaced; the manifest is written last. This is not a
transaction across filesystem I/O failures or concurrent external modification.

Exit codes: 0 success, 1 invalid command syntax, 3 installation/update error
(including unknown agent, manifest, path, conflict or filesystem errors). Top-level
and nested help need neither a project nor filesystem mutation.

### 18.5 `info`

Print project metadata: root, collections, environments. `--format json` for
machine output.

### 18.6 `schema`

Emit the JSON Schema for the collection format to stdout. `--project` emits the
schema for `curlew.yaml` instead.

### 18.7 `watch <file>`

Run the collection, then re-run on file changes. Accepts every `run` flag, plus
`--clear` to clear the terminal between runs. `--format json` suppresses terminal
decoration and emits one JSON document per run. Ctrl+C exits.

### 18.8 `import openapi <spec>`

Convert an OpenAPI 3.x specification into a collection, generating headers,
request bodies, and status assertions. `--output <file>` sets the destination.

**Self-contained output.** Every `{{variable}}` the import writes into a request
is also emitted in the collection's own `variables:` block, so the generated file
runs as generated. Header and query parameters default to empty; a path
parameter takes its value from the document — a parameter or schema `example`,
then an `enum` member, then `default`, then the schema's type, with a declared
`minimum` respected for numbers. Only `base_url` normally needs overriding, and
it defaults to the first entry in `servers:`.

**3.0 and 3.1.** A document declaring 3.1 is translated into the 3.0 spelling of
the same meaning before validation:

| 3.1 construct | Treatment |
|---|---|
| `type: ["string","null"]` | `type: string` with `nullable: true` |
| `type: ["string","integer"]` | Type dropped (3.0 cannot express a union) and a warning issued |
| `info.summary`, `info.license.identifier`, `jsonSchemaDialect` | Dropped; the import does not read them |
| `webhooks` | Dropped with a warning — a webhook is an inbound callback, so there is no request to generate |

A 3.0 document is passed through untouched.

### 18.9 `pr-check`

Turn a run's results file into a CI gate. Local only — nothing is transmitted.

| Flag | Meaning |
|---|---|
| `--results <file>` | Results JSON to read. Required. Accepts `curlew run --format json` output |
| `--summary <file>` | Write the verdict as JSON |
| `--dry-run` | Print the verdict instead of writing it |

Exit codes: `0` all passed, `1` the results file contains failures, `2` usage
error or unreadable results file.

### 18.10 `vault list`

List the configured vault provider profiles.

### 18.11 `plugins list`

Discover plugins from `CURLEW_PLUGINS` and print each plugin's name, version, and
registered hooks (§15).

### 18.12 `telemetry <subcommand>`

See §22.

### 18.13 `ui`

See §20.

### 18.14 `perf <request-file>`

See §21.

---

## 19. Environment Variables

| Variable | Consumed by | Meaning |
|---|---|---|
| `CURLEW_TEAM_CONFIG` | Shared vault templates | Path to a local shared vault configuration file (§13.5) |
| `CURLEW_VAULT_STUB` | Shared vault templates | `1` selects the in-memory stub provider |
| `CURLEW_CONFIG_DIR` | Telemetry | Overrides the config directory (`~/.config/curlew` on Linux, `~/Library/Application Support/curlew` on macOS) |
| `CURLEW_TELEMETRY_FILE` | Telemetry | Overrides the local events file |
| `CURLEW_PLUGINS` | Plugins | Colon-separated (semicolon on Windows) list of plugin paths |
| `NO_COLOR` | All terminal output | Any non-empty value disables ANSI colour |

Plus any OS environment variable the user imports with `--env-var`.

There are deliberately no variables configuring a server endpoint, credential, or
account. See §1.2.

---

## 20. Local Web UI

`curlew ui` starts a runner and inspector over the project's collections.

| Flag | Meaning |
|---|---|
| `--port <n>` | Listen port. Default `ui.port` from `curlew.yaml`, else 8765. `0` picks an ephemeral port. An explicitly chosen busy port fails; the default scans up to 19 ports upward |
| `--env <name>` | Environment preselected in the UI, validated against `environments/` |
| `--collection <file>` | Restrict the tree to one collection inside the project root |
| `--no-open` | Do not launch a browser. The URL is always printed |
| `--color <when>` | `auto` (default), `always`, or `never` |
| `--no-color` | Disable colour on stderr (same as `--color=never`) |

### 20.1 Invariants

- **Loopback only.** The server binds `127.0.0.1`. Remote access is the user's
  own SSH tunnel, never a bind-address flag.
- **A per-start session token** is minted on each launch and embedded in the
  printed URL.
- **Files are the source of truth.** The UI never edits a collection,
  environment, or config file. Its single write is the run-history store under
  `.curlew/ui/`, which is self-gitignored.
- **Redaction is unconditional.** The UI rejects `--allow-sensitive`. There is no
  configuration that reveals a sensitive value in the UI.

Exit codes: `0` clean shutdown on signal, `1` usage error, port bind failure, or
fatal server error, `3` invalid configuration (bad `ui:` block, unknown `--env`,
`--collection` outside the project root).

---

## 21. Performance Testing

`curlew perf <request-file>` load-tests a single request. The file defines
`{ name, request }` — the same shape as an external request file (§5.2).

| Flag | Required | Meaning |
|---|---|---|
| `--vus <n>` | Yes | Virtual users (concurrent workers), ≥ 1 |
| `--duration <d>` | Yes | Total run duration, e.g. `30s`, `2m` |
| `--ramp-up <d>` | No | Linear ramp from 1 to `--vus` over this window. Default 0 |
| `--rps <n>` | No | Target throughput in requests/sec. Default 0 = unbounded |
| `--output <dest>` | No | Extension selects the format: `.json` writes a time series plus metrics, `.html` writes a self-contained report, `stdout` prints a summary (default) |

Exit codes: `0` ok, `1` failures, `2` usage, `3` request-file error, `130`
SIGINT.

> **Note.** The generated `.html` report references Chart.js from a CDN, so
> rendering it in a browser requires network access. The CLI itself makes no such
> request; this is a property of the artifact, not of the run.

Load testing points real traffic at a real endpoint. Target only systems the
operator is authorised to load.

---

## 22. Telemetry

Telemetry is **opt-in, off by default, and local**. There is no telemetry
backend; nothing is transmitted. Events are appended as NDJSON, one object per
line, to a file on the user's own disk.

| Subcommand | Effect |
|---|---|
| `enable` | Generate a persistent `install_id` (UUIDv4) and enable recording. Writes `install_id` (mode 0600) and `telemetry.json` into the config directory |
| `disable` | Stop recording. The `install_id` is retained so recording can resume |
| `status` | Print the current state. Exit 1 if telemetry was never enabled |
| `reset-id` | Regenerate the `install_id`. The previous value is unrecoverable |
| `export` | Print the `install_id` and recent emission history as JSON |
| `delete` | Remove the `install_id`, `telemetry.json`, and the events file |

Default events file: `telemetry.ndjson` inside the config directory,
overridable with `CURLEW_TELEMETRY_FILE`. The config directory is
`os.UserConfigDir()/curlew` — `~/.config/curlew` on Linux,
`~/Library/Application Support/curlew` on macOS — and is overridable with
`CURLEW_CONFIG_DIR`. `curlew telemetry --help` prints the resolved path.

---

## 23. CI Integration

### 23.1 Shape of a CI Job

```bash
curlew run "collections/**/*.yaml" \
  --env staging \
  --env-var API_KEY \
  --format json > results.json
curlew pr-check --results results.json --summary verdict.json
```

`run` produces the machine-readable results; `pr-check` reduces them to a gate
and exits 1 on any failure. Both steps are local.

### 23.2 Requirements CI Places on the Tool

- **Stable exit codes** (§17), with `1` and `3` distinguishing a product
  regression from a broken pipeline.
- **Stream discipline** (§16.2): results on stdout, diagnostics on stderr.
- **Redaction on by default** (§6.5), so a CI log cannot leak a secret through
  ordinary output. `--allow-sensitive` in CI is a misuse.
- **No interactive prompts** when `--non-interactive` is set, and no prompt that
  blocks a non-TTY run.
- **`NO_COLOR` support** for log collectors that do not render ANSI.

### 23.3 Report Artifacts

`--format junit --report results.xml` suits CI test reporters.
`--format html --report report.html` produces a self-contained artifact.
`--format markdown --report responses/` suits review in an editor or by an agent.

---

## 24. Limits and Guard Rails

| Limit | Value | Behaviour on breach |
|---|---|---|
| `body_file` / `body_binary_file` size | 50 MB | Parse error, exit 3. No flag raises it |
| Collection file not found | — | Parse error, exit 3 |
| `exec --stdin` payload | 10 MB | Usage error |
| Data-driven rows without confirmation | 10,000 | Exit 2 unless `--confirm-large-dataset` |
| Data-driven parallel workers | 20 | Capped |
| Plugin hook invocation | 10 s | Hook abandoned, run continues |
| Nested variable resolution depth | 10 | Warning; deeper references unresolved |
| `Retry-After` honoured | 30 s | Longer values clamped |
| UI default port scan | 19 ports upward | Bind failure if none free |
| UI run history | `max_runs`, default 50, cap 500 | Oldest evicted |

These are safety interlocks, not commercial limits. Nothing about them changes
based on who is running the binary.

---

## 25. Conformance

An implementation conforms to this specification when all of the following hold.
Each is mechanically checkable.

**Locality**

1. A run of any collection issues no network request other than those the
   collection defines, plus subprocesses invoked by `from_command`, a vault
   provider, or a plugin.
2. No command requires, accepts, or stores an account credential or license key.
3. Every documented feature works without configuration beyond the project files.

**Contracts**

4. Exit codes match §17 for every listed condition.
5. Variable precedence matches §6.2 exactly, resolved once per run.
6. A value that is sensitive under any rule in §6.5 appears as `[REDACTED]` in
   every output format, whether curlew sent it or the server returned it, and
   in assertion failure messages as well as in bodies, headers and URLs —
   unless `--allow-sensitive` was passed to a command that accepts it.
7. `curlew ui` rejects `--allow-sensitive` and never emits an unredacted value.

**Determinism**

8. Two runs with the same `--seed` and the same collection produce identical
   values for every seeded function.
9. Dependency analysis produces the same graph, the same waves, and the same
   error for the same input.

**Failure handling**

10. Parse, cycle, and collision errors abort before any request is sent.
11. A plugin fault cannot change a run's verdict from pass to fail.
12. `stop_on_failure` and `required` behave as specified in §3.2.

**Documentation**

13. `--help` output for every command matches the flags this document lists.
14. Every flag that parses has an implemented effect.

---

## Appendix A — Collection JSON Schema

The authoritative machine-readable schema for the collection format is emitted by
the binary itself:

```bash
curlew schema             # collection format
curlew schema --project   # curlew.yaml format
```

The schema describes structure, not cross-field constraints: mutual exclusions
(`body` / `body_file` / `body_binary_file`, `query` / `query_file`,
`path` / `request`) are enforced by the parser and stated in §5.

> The schema is maintained by hand rather than generated, but it is kept honest:
> a reflection test walks the parser structs in `internal/parser/collection.go`
> and fails when a yaml-tagged field has no schema entry, or when the schema
> describes a key the parser would ignore. A second test pins every closed-set
> enum (`protocol`, WebSocket `action`, `reconnect.backoff`,
> `graphql.error_handling`, `output.format`) to the parser's own list.
>
> Both `requestItem` and `request` declare `additionalProperties: false`, so a
> misspelled key is reported by an editor wired up per MANUAL §1.5. Note that
> the parser itself ignores unknown keys — the schema is the only place a typo
> surfaces, which is why its completeness is a functional property rather than a
> documentation nicety.
>
> One cross-field constraint is expressed in the schema as an exception to the
> rule above: `$defs.variableEntry` forbids `from_command` and `value` together.
> Every other mutual exclusion is the parser's.

---

## Appendix B — Deliberately Absent Surfaces

<!-- doc-check: ignore-names -->

The following appear in [SPECIFICATION.md](SPECIFICATION.md), which describes the
platform as designed, and are **not** part of the CLI. They are listed so that a
reader arriving from that document knows the omission is intentional rather than
an oversight.

**Removed with the licensing system**

- Five-tier feature gating (Free, Solo, Professional, Team, Enterprise); every
  feature in this document is unconditional.
- `curlew license` and its subcommands.
- Exit code `6` (feature gate).
- Trials, upgrade prompts, and any tier annotation on a feature.

**Removed with the backend**

- `curlew login` and the device-code flow.
- `curlew worker`, distributed execution (`--workers`, `--coordinator-url`), and
  schedule pull.
- Report upload (`--report-upload`) and PR-check posting to GitHub or GitLab.
  `pr-check` is now a local results-file gate (§18.9).
- The backend team-vault cache and `--refresh-vault`. Shared vault templates load
  only from `CURLEW_TEAM_CONFIG` (§13.5).
- Telemetry ingest. `telemetry` writes to a local NDJSON file (§22).
- Every `CURLEW_BACKEND_*` and `CURLEW_COORDINATOR_URL` variable.
- Exit code `10` (worker unauthorized).

**Designed but not implemented in the CLI**

- Multipart file uploads (`body: { field: { file: ... } }`), `raw_file`, and
  `save_to` response downloads. The shipped equivalents are `body_file` and
  `body_binary_file` (§5.1.2).
- `--fixed-time` for freezing timestamp functions.
- The 1,000-request abuse-prevention ceiling. The shipped guard is the
  10,000-row data-driven interlock (§10.4).
- gRPC and Server-Sent Events protocol adapters.
