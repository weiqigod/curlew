# Curlew User Manual

*A tutorial-style guide for developers and testers, from first request to enterprise deployment.*

---

## Table of Contents

**Part 0 — Preface**
- [P.1 What Curlew is](#p1-what-curlew-is)
- [P.2 Who this manual is for](#p2-who-this-manual-is-for)
- [P.3 How to read this manual](#p3-how-to-read-this-manual)
- [P.4 Conventions](#p4-conventions)

**Part 1 — Getting Started**
- [1.1 Installing curlew](#11-installing-curlew)
- [1.2 Your first test in five minutes](#12-your-first-test-in-five-minutes)
- [1.3 Anatomy of a collection](#13-anatomy-of-a-collection)
- [1.4 When a test fails](#14-when-a-test-fails)
- [1.5 Editor setup (VS Code)](#15-editor-setup-vs-code)

**Part 2 — Writing Tests**
- [2.1 HTTP methods, headers, query, and body](#21-http-methods-headers-query-and-body)
- [2.2 Assertions — the full operator catalog](#22-assertions--the-full-operator-catalog)
- [2.3 JSONPath crash course](#23-jsonpath-crash-course)
- [2.4 Extracting values into variables](#24-extracting-values-into-variables)
- [2.5 Required vs. optional requests](#25-required-vs-optional-requests)

**Part 3 — Organizing a Project**
- [3.1 Variables and interpolation](#31-variables-and-interpolation)
- [3.2 The precedence ladder](#32-the-precedence-ladder)
- [3.3 Environments](#33-environments)
- [3.4 `.env` files](#34-env-files)
- [3.5 CLI variable flags](#35-cli-variable-flags)
- [3.6 Project-wide config (`curlew.yaml`)](#36-project-wide-config-curlewyaml)
- [3.6.1 The `output:` block](#361-the-output-block)
- [3.7 Dynamic and faker functions](#37-dynamic-and-faker-functions)
- [3.8 Determinism with `--seed`](#38-determinism-with---seed)
- [3.9 External request files](#39-external-request-files)
- [3.10 Expression Language (CEL)](#310-expression-language-cel)

**Part 4 — Running Tests in CI**
- [4.1 Output formats](#41-output-formats)
- [4.1a Markdown response files](#41a-markdown-response-files)
- [4.2 Streams, verbosity, and color](#42-streams-verbosity-and-color)
- [4.2b Running a Single Request (--only)](#42b-running-a-single-request---only)
- [4.3 Exit codes — master table](#43-exit-codes--master-table)
- [4.4 Redaction of secrets](#44-redaction-of-secrets)
- [4.5 JSONL logging](#45-jsonl-logging)
- [4.5a Event stream (`--events`)](#45a-event-stream---events)
- [4.6 CI recipes](#46-ci-recipes)
- [4.7 Project utility commands](#47-project-utility-commands)
- [4.8 AI-agent commands](#48-ai-agent-commands)
- [4.9 Driving curlew with an AI agent](#49-driving-curlew-with-an-ai-agent)

**Part 5 — Test Authoring at Scale**
- [5.1 Setup and teardown phases](#51-setup-and-teardown-phases)
- [5.2 Composition with `include:`](#52-composition-with-include)
- [5.3 Collection-level defaults](#53-collection-level-defaults)
- [5.4 Retry logic](#54-retry-logic)
- [5.5 Data-driven testing](#55-data-driven-testing)
- [5.6 Parallel execution](#56-parallel-execution)
- [5.7 Watch mode](#57-watch-mode)
- [5.8 Dry run](#58-dry-run)

**Part 6 — Authentication and Secrets**
- [6.1 Mental model](#61-mental-model)
- [6.2 `.env`, CLI, and OS imports (recap)](#62-env-cli-and-os-imports-recap)
- [6.3 `from_command` variables](#63-from_command-variables)
- [6.4 Vault providers](#64-vault-providers)
- [6.5 Dynamic auth profiles](#65-dynamic-auth-profiles)
- [6.6 Shared vault templates](#66-shared-vault-templates)
- [6.7 The redaction contract](#67-the-redaction-contract)
- [6.8 Request signing](#68-request-signing)
- [6.9 No account, no backend](#69-no-account-no-backend)

**Part 7 — Beyond REST**
- [7.1 GraphQL](#71-graphql)
- [7.2 WebSocket](#72-websocket)
- [7.3 OpenAPI import](#73-openapi-import)

**Part 8 — CI Integration**
- [8.1 Gating CI on a results file](#81-gating-ci-on-a-results-file)
- [8.2 Local telemetry](#82-local-telemetry)

**Part 9 — Scale Features**
- [9.1 Concurrency](#91-concurrency)
- [9.2 Performance testing](#92-performance-testing)

**Part 10 — Plugins**
- [10.1 What plugins can do](#101-what-plugins-can-do)
- [10.2 Discovering plugins](#102-discovering-plugins)
- [10.3 Inspecting installed plugins](#103-inspecting-installed-plugins)
- [10.4 The JSON-RPC handshake](#104-the-json-rpc-handshake)
- [10.5 Per-hook payload shapes](#105-per-hook-payload-shapes)
- [10.6 Timeouts and termination](#106-timeouts-and-termination)

**Part 11 — Reference**
- [A. CLI reference](#a-cli-reference)
- [B. Environment variables](#b-environment-variables)
- [C. File format schemas](#c-file-format-schemas)
- [D. Exit codes](#d-exit-codes)
- [E. Glossary](#e-glossary)
- [F. Troubleshooting](#f-troubleshooting)

---

## Part 0 — Preface

### P.1 What Curlew is

Curlew is a file-based API testing tool. You write test collections as YAML files, commit them to version control alongside your application code, and execute them with a single static binary called `curlew`.

It is the tool you reach for when Postman's clickable UI starts fighting you: when you want diffs instead of workspaces, pull requests instead of shared accounts, text editors instead of drag-and-drop, and the same tests running in your terminal and in CI without re-exporting anything. It also runs well as a tool invoked by AI coding agents — every command has a `--format json` and a `--non-interactive` mode, and there is an `exec` command built for one-shot execution from stdin.

### P.2 Who this manual is for

Developers and QA testers who:

- Understand HTTP (methods, headers, status codes, JSON bodies).
- Can read and write YAML.
- Are comfortable in a terminal.

You do **not** need to know Go or the Curlew source code to work through this manual.

### P.3 How to read this manual

The manual is written to be read straight through, but it is also structured to be sliced:

| If you are … | Read |
|---|---|
| A developer kicking the tires | Parts 1–3 |
| A QA engineer wiring this into CI | Parts 1–5 |
| Managing secrets or dynamic auth | Parts 1–3, then 6 |
| Testing GraphQL or WebSocket APIs | Parts 1–3, then 7 |
| Running at enterprise scale | Parts 1–5, then 8–10 |
| Looking something up | Part 11 (Reference) |

Each part introduces new machinery and then uses it. Example YAML snippets accumulate — a project started in Part 1 is extended, not replaced, in Parts 3, 5, and 6.

### P.4 Conventions

**Command style.** Commands are shown as if `curlew` is on your `$PATH`:

```bash
curlew run collections/sample.yaml
```

If you installed to a non-PATH location and are running out of a build directory, mentally substitute `./curlew`.

**Placeholders.** Beginner examples use `https://httpbin.org`, which is a real public HTTP echo service — the examples run as-is. Business-logic examples use `https://api.example.com` as a stand-in for your own API.

**Variable casing.** Variable names are `snake_case` throughout (`base_url`, `admin_token`, `user_id`) — this matches the scaffold generated by `curlew init`.

**Exit codes.** Numerical exit codes are mentioned often; the full table lives in §4.3.

---

## Part 1 — Getting Started

### 1.1 Installing curlew

Curlew is a single static Go binary. Two install paths:

**From source** (current, pre-release):

```bash
git clone https://github.com/weiqigod/curlew
cd curlew
go build -o curlew ./cmd/curlew
# Then copy to your PATH:
sudo mv curlew /usr/local/bin/
```

**Verify the install:**

```bash
curlew --version
# curlew 0.1.0-dev

curlew --help
# curlew — a file-based API testing tool
# ...
```

If `curlew --help` prints the usage summary with the full command list (`run`, `exec`, `validate`, `init`, `info`, `schema`, `watch`, `vault`, `import`, `pr-check`, `ui`, `perf`, `plugins`, `telemetry`), you are ready.

### 1.2 Your first test in five minutes

From an empty directory you want to use for tests:

```bash
mkdir demo-api && cd demo-api
curlew init
```

`curlew init` scaffolds a project. Inspect what it created:

```
demo-api/
├── curlew.yaml              # project config
├── .env.example              # template for your local secrets
├── .gitignore                # excludes .env
├── collections/
│   └── sample.yaml           # your first collection
└── environments/
    └── dev.yaml              # environment variables for dev
```

The generated `collections/sample.yaml` looks like this:

```yaml
name: Sample Collection
description: A sample collection generated by curlew init

requests:
  - name: Hello World
    request:
      method: GET
      url: "{{base_url}}/get"
    assertions:
      status: 200
```

And `curlew.yaml`:

```yaml
project_name: demo-api
variables:
  base_url: "https://httpbin.org"
```

Run it:

```bash
curlew run collections/sample.yaml
```

You should see something like:

```
Sample Collection
  ✓ Hello World (234ms)

1 passed, 0 failed, 0 skipped — 234ms
```

That is end-to-end Curlew. One command, one file, real HTTP request, assertion checked, exit code 0.

### 1.3 Anatomy of a collection

Re-read `collections/sample.yaml` with the structure named:

```yaml
name: Sample Collection                   # (1) collection name, shown in output
description: A sample collection ...      # (2) optional description

requests:                                 # (3) list of request items
  - name: Hello World                     # (4) each item has a name ...
    request:                              # (5) ... and a `request:` block
      method: GET                         # (6)
      url: "{{base_url}}/get"             # (7) `{{...}}` interpolates variables
    assertions:                           # (8) checks to run on the response
      status: 200                         # (9)
```

Every collection has at minimum a `name` and a `requests` array. Every request item has at minimum a `name`, a `request.method`, and a `request.url`. Everything else is optional.

The `{{base_url}}` reference resolves to `https://httpbin.org` because that value is defined in `curlew.yaml`'s `variables:` block. Curlew finds `curlew.yaml` by walking up from the collection file's directory — you can keep your tests in any subdirectory of the project and the config will be picked up automatically.

### 1.4 When a test fails

Edit `collections/sample.yaml` and change the assertion:

```yaml
    assertions:
      status: 404           # was 200
```

Run again:

```bash
curlew run collections/sample.yaml
```

You now see a failure:

```
Sample Collection
  ✗ Hello World (218ms)
      status | expected 404 | got 200

0 passed, 1 failed, 0 skipped — 218ms
```

And:

```bash
echo $?
# 1
```

Exit code 1 means "assertion failure or general error." Your CI system will notice this immediately.

Put the assertion back to `200` before continuing.

### 1.5 Editor setup (VS Code)

Curlew publishes its collection JSON Schema at a stable in-repo path so VS Code (with the `redhat.vscode-yaml` extension) can give you autocomplete, hover documentation, and inline validation while you edit collection YAML files.

**Prerequisite.** Install the [YAML extension by Red Hat](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml).

**Wire it up.** Add to your workspace `.vscode/settings.json` (this file is user-local — keep it out of version control):

```json
{
  "yaml.schemas": {
    "./schemas/collection-v1.json": "collections/*.yaml",
    "./schemas/project-v1.json":    "curlew.yaml"
  }
}
```

If you are using Curlew from outside its source tree and don't have the schema files on disk, ask the binary for them. This is the reliable option, and it has an advantage over any published copy: the schema you get is the one your installed binary actually enforces.

```bash
mkdir -p .curlew/schemas
curlew schema           > .curlew/schemas/collection-v1.json
curlew schema --project > .curlew/schemas/project-v1.json
```

```json
{
  "yaml.schemas": {
    "./.curlew/schemas/collection-v1.json": "collections/*.yaml",
    "./.curlew/schemas/project-v1.json":    "curlew.yaml"
  }
}
```

Each schema also carries an `$id` pointing at its published location
(`https://raw.githubusercontent.com/weiqigod/curlew/main/schemas/…`). That URL is
an identifier, not a promise: it resolves only while the repository is public,
and the repository is currently private, so pointing `yaml.schemas` at it gets
you a 404 rather than autocomplete. `./scripts/check-schema-urls.sh` reports
whether the URLs currently resolve.

**What you get.** Typing `req<Ctrl-Space>` inside a collection file suggests `requests:`. Hovering `rate_limit_rps` shows its description. Removing a required field like `name:` gets a squiggle. Misspelling a key (`assertiosn:`) is flagged.

**What the schema covers.** Every field the parser binds, including `if:`, `depends_on:`, `signing:` (at both collection and request-item level), `protocol:` / `graphql:` / `websocket:`, `cel:` assertions, `retry:` at all three levels, `data_driven:`, the object form of `setup` / `teardown` (`{ retry, items }`), and the object form of `variables` (`from_command` / `sensitive` / `cache`). A test walks the parser structs and fails if a field is ever added without a matching schema entry, so this list cannot quietly fall behind the binary.

**What it deliberately does not do.** The schema describes structure, not cross-field constraints. Mutual exclusions — `body` / `body_file` / `body_binary_file`, `graphql.query` / `graphql.query_file`, `path` / `request`, and a WebSocket step's `message` / `any_of` — are enforced by the parser and reported by `curlew validate`, not by your editor. Conditional requirements are the parser's too: `graphql.query` is required only when `protocol: graphql`, and `websocket.heartbeat.interval_ms` must be positive only when the heartbeat is enabled. External request files (§3.9) have no published schema of their own, so map `yaml.schemas` at `collections/*.yaml` and `curlew.yaml` only.

---

## Part 2 — Writing Tests

This part widens what a single request can do: different HTTP methods, every assertion operator, extracting data from responses, and controlling the failure model.

### 2.1 HTTP methods, headers, query, and body

**All seven HTTP verbs are supported:** `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`. Method names are case-insensitive in YAML; Curlew normalizes to uppercase internally.

**A fully-loaded request:**

```yaml
name: Collection Basics
requests:
  - name: Full request example
    request:
      method: POST
      url: "https://httpbin.org/anything"
      headers:
        Accept: "application/json"
        X-Correlation-Id: "demo-123"
      query:
        source: curlew
        debug: "true"
      body:
        action: "create"
        item:
          id: 42
          name: "Widget"
    assertions:
      status: 200
```

Under the hood:

- `headers` is a map. Values are always strings; Curlew sets the `Content-Type` to `application/json` automatically when `body` is a YAML map.
- `query` is a map, appended to the URL as `?source=curlew&debug=true`. Order is not guaranteed.
- `body` can be either a YAML map (serialized as JSON) **or** a raw string:

```yaml
    request:
      method: POST
      url: "https://httpbin.org/post"
      headers:
        Content-Type: "text/plain"
      body: "Hello, raw string body"
```

When `body` is a string, Curlew sends it as-is — it does not re-encode it as JSON.

#### Loading the body from a file

Inline bodies work fine up to maybe a screenful. Past that, `body_file:` (text) and `body_binary_file:` (raw bytes) point to an external file. The loaded contents become the request body.

**Text variant — `body_file:`.** Resolved relative to the collection file; variables inside the file are interpolated; `Content-Type` is auto-detected from the file extension unless you set the header explicitly.

```yaml
# collections/import.yaml
requests:
  - name: Bulk import
    request:
      method: POST
      url: "{{base_url}}/import"
      body_file: "../payloads/users.json"
```

```json
// payloads/users.json
{
  "batch_id": "{{batch_id}}",
  "records": [
    { "id": 1, "name": "Alice" },
    { "id": 2, "name": "Bob" }
  ]
}
```

Run it with the batch id supplied as a variable:

```bash
curlew run collections/import.yaml --var batch_id=B-2026-04-21
```

Curlew auto-detects `Content-Type: application/json` from the `.json` extension. An explicit `headers.Content-Type` always wins:

```yaml
    request:
      method: POST
      url: "{{base_url}}/import"
      headers:
        Content-Type: "application/vnd.acme.v2+json"
      body_file: "../payloads/users.json"     # still interpolated; CT stays vnd.acme.v2+json
```

For unknown extensions (e.g. `.tmpl`, `.dat`) Curlew leaves `Content-Type` unset — set it yourself if the server requires it.

**Binary variant — `body_binary_file:`.** Raw bytes, no interpolation. Auto-detected `Content-Type` falls back to `application/octet-stream` when the extension is unknown. Use this for file uploads: PDF, PNG, protobuf, compressed blobs.

```yaml
requests:
  - name: Upload report
    request:
      method: POST
      url: "{{base_url}}/reports"
      body_binary_file: "../fixtures/report.pdf"
```

No `{{...}}` inside the file is touched. The payload is sent byte-for-byte.

**Rules and limits.**

- `body`, `body_file`, and `body_binary_file` are **mutually exclusive** on the same request. Using more than one fails at parse time with exit 3.
- Paths are resolved relative to the collection file's directory. Absolute paths also work.
- Max file size is **50 MB**. Larger files fail at parse time with a structured error — if you legitimately need larger uploads, Curlew is the wrong tool.
- Missing files fail at parse time, so `curlew validate` catches them before any HTTP traffic.
- Watch mode (`curlew watch`) reruns automatically when either the collection *or* the body file changes.

**Auto-detected Content-Types** (via `mime.TypeByExtension` — OS-dependent MIME database):

| Extension | Detected Content-Type (typical) |
|---|---|
| `.json` | `application/json` |
| `.xml` | `application/xml` or `text/xml` |
| `.yaml`, `.yml` | `application/yaml` or `text/yaml` |
| `.txt` | `text/plain; charset=utf-8` |
| `.html` | `text/html; charset=utf-8` |
| `.csv` | `text/csv; charset=utf-8` |
| `.pdf` | `application/pdf` |
| `.png`, `.jpg`, `.gif` | `image/png`, `image/jpeg`, `image/gif` |
| unknown (text variant) | *none — user sets header* |
| unknown (binary variant) | `application/octet-stream` |

**Multi-request collections** run sequentially by default:

```yaml
name: Sample multi-request
requests:
  - name: Get
    request:
      method: GET
      url: "https://httpbin.org/get"
    assertions:
      status: 200

  - name: Post
    request:
      method: POST
      url: "https://httpbin.org/post"
      body:
        message: "Hello"
    assertions:
      status: 200
```

### 2.2 Assertions — the full operator catalog

Assertions are grouped by what they target: `status`, `headers`, `body`, or `timing`. Each group accepts a specific set of operators.

#### Status

The simplest assertion. Scalar or list form.

```yaml
assertions:
  status: 200                 # exact match
  status: [200, 201, 204]     # any of these
```

Failure:

```
status | expected one of [201, 204] | got 200
```

#### Headers

Three operators: `equals` (exact), `exists` (presence), `matches` (regex). Header names are case-insensitive.

```yaml
assertions:
  headers:
    Content-Type:
      matches: "^application/json"
    X-Request-Id:
      exists: true
    Server:
      equals: "nginx"
```

#### Body

The body is always parsed as JSON, and every body assertion targets a JSONPath expression (see §2.3). Thirteen operators are available.

**Value operators:**

```yaml
assertions:
  body:
    $.id:
      equals: 42                # exact equality (numeric types compared as numbers: "42" == 42)
    $.name:
      matches: "^Widget"        # regex on string value
```

**Presence operators:**

```yaml
assertions:
  body:
    $.created_at:
      exists: true
    $.deleted_at:
      not_exists: true
```

**Type operator:**

```yaml
assertions:
  body:
    $.count:
      type: "number"            # null | string | number | boolean | array | object
```

**Containment:**

```yaml
assertions:
  body:
    $.tags:
      contains: "urgent"        # for arrays: is "urgent" in the array? for strings: substring.
    $.tags:
      contains_all: ["urgent", "review"]   # all listed items present in the array
```

**Size:**

```yaml
assertions:
  body:
    $.items:
      length: 3                 # array length, string length, or object key count
```

**Numeric ordering:**

```yaml
assertions:
  body:
    $.stock:
      greater_than: 0
    $.price:
      less_than: 100
    $.rating:
      greater_than_or_equal: 4.5
    $.discount:
      less_than_or_equal: 0.20
```

**Numeric proximity:**

```yaml
assertions:
  body:
    $.average:
      approximately:
        value: 100
        tolerance: 0.5          # passes if |actual - 100| <= 0.5
    $.latency_ms:
      in_range:
        min: 50
        max: 200
```

#### Timing

```yaml
assertions:
  timing:
    max_duration_ms: 5000       # request must complete within 5s
```

Timing covers the full request/response cycle including TLS handshake.

#### Putting it together

A realistic request with every assertion category exercised:

```yaml
- name: Create widget
  request:
    method: POST
    url: "https://api.example.com/widgets"
    body:
      name: "Demo"
      price: 19.99
  assertions:
    status: [200, 201]
    headers:
      Content-Type:
        matches: "^application/json"
      Location:
        exists: true
    body:
      $.id:
        type: "string"
      $.name:
        equals: "Demo"
      $.price:
        approximately:
          value: 19.99
          tolerance: 0.01
      $.created_at:
        matches: "^\\d{4}-\\d{2}-\\d{2}T"
    timing:
      max_duration_ms: 3000
```

### 2.3 JSONPath crash course

Curlew body assertions target response fields via JSONPath. If you already know JSONPath, skip ahead. If you don't, this section is all you need.

Given this response body:

```json
{
  "id": "widget-42",
  "name": "Demo",
  "tags": ["urgent", "review"],
  "owner": { "email": "alice@example.com" },
  "items": [
    { "sku": "A", "qty": 3 },
    { "sku": "B", "qty": 7 }
  ]
}
```

The expressions you will use most:

| Expression | Result |
|---|---|
| `$` | the whole document |
| `$.id` | `"widget-42"` |
| `$.owner.email` | `"alice@example.com"` |
| `$.tags` | `["urgent", "review"]` |
| `$.tags[0]` | `"urgent"` |
| `$.items[0].sku` | `"A"` |
| `$.items[*].sku` | `["A", "B"]` (all skus) |
| `$..sku` | `["A", "B"]` (recursive — any `sku` at any depth) |
| `$.items.length` | use the `length:` operator instead |

Three worked assertions against the response above:

```yaml
assertions:
  body:
    $.id:
      equals: "widget-42"
    $.owner.email:
      matches: "@example\\.com$"
    $.items[*].sku:
      contains_all: ["A", "B"]
```

### 2.4 Extracting values into variables

You often need the output of one request as the input to another: create a resource, then fetch it by id.

Use `extract:` on the first request and `{{...}}` on the second:

```yaml
name: Create then fetch
requests:
  - name: Create widget
    request:
      method: POST
      url: "https://api.example.com/widgets"
      body:
        name: "Demo"
    assertions:
      status: 201
    extract:
      widget_id: "$.id"                 # pulls .id out of the JSON body
      created_at: "$.metadata.created_at"
      signing_ref:                      # object form: same thing, plus a flag
        path: "$.metadata.ref"
        sensitive: true

  - name: Fetch widget by id
    request:
      method: GET
      url: "https://api.example.com/widgets/{{widget_id}}"
    assertions:
      status: 200
      body:
        $.id:
          equals: "{{widget_id}}"
```

Extracted values are available to every request that runs after the extraction. They are strings (Curlew converts numbers and booleans via their canonical string form).

Each entry takes either form: a JSONPath string, or an object with `path:` and an optional `sensitive:`. An extracted value is redacted automatically when its *name* matches the heuristic in §4.4 — `widget_id` is not a secret, `auth_token` is. Use the object form when it does not: the field is named by the API you are testing, and `signing_ref` above is as much a credential as `auth_token` without looking like one. Once marked, the value is redacted everywhere it appears, including in the response that produced it.

Extraction only runs against JSON bodies. If the response is not JSON, extraction fails the request. Extraction runs even when assertions fail, so you can still inspect the extracted values when debugging — they simply won't be used because the run stops.

### 2.5 Required vs. optional requests

By default, a failing request does **not** stop the rest of the collection. The run continues; the final exit code is 1 if any request failed.

To stop on a specific request's failure, mark it `required: true`:

```yaml
requests:
  - name: Login (must succeed)
    required: true
    request:
      method: POST
      url: "{{base_url}}/login"
      body:
        username: "{{user}}"
        password: "{{pass}}"
    assertions:
      status: 200
    extract:
      auth_token: "$.token"

  - name: Fetch profile
    request:
      method: GET
      url: "{{base_url}}/me"
      headers:
        Authorization: "Bearer {{auth_token}}"
    assertions:
      status: 200
```

If login fails, the profile request is skipped (shown with the ⊘ marker) rather than running with an empty token.

A related but broader control, `options.stop_on_failure`, stops on the first failure regardless of `required`:

```yaml
options:
  stop_on_failure: true
```

Use `required` for the few requests where continuing makes no sense; use `stop_on_failure` for strict fail-fast runs.

---

## Part 3 — Organizing a Project

One file with a few requests grows into many files, shared variables, several environments, and machine-generated values. Part 3 is the toolkit for that.

### 3.1 Variables and interpolation

Curlew has one interpolation syntax: double curly braces.

```yaml
url: "{{base_url}}/users/{{user_id}}"
```

Any string value in YAML — URL, header value, query value, body string, assertion expected value — is interpolated before the request is sent. Variables not found cause a hard error before any HTTP traffic starts, which is deliberate: you find out about typos immediately.

Three special forms:

```yaml
url: "{{base_url}}/ping"              # regular variable
id: "{{$uuid}}"                       # dynamic/faker function (§3.7)
token: "{{secrets.admin_jwt}}"        # shared vault template (§6.6)
```

Variable names must match `[a-zA-Z_][a-zA-Z0-9_]*` — letters, digits, underscores; must start with a letter or underscore.

### 3.2 The precedence ladder

Variables can be set in many places. When the same name is defined twice, the higher-precedence source wins. There are ten sources, numbered lowest to highest precedence:

| # | Source | Where it lives |
|---|---|---|
| 1 | Dynamic functions | `{{$uuid}}`, `{{$timestamp}}`, … (§3.7) |
| 2 | Project variables | `curlew.yaml` → `variables:` |
| 3 | Environment file | `environments/<name>.yaml` → `variables:` (selected with `--env <name>`) |
| 4 | `.env` file | `./.env` in project root |
| 5 | `from_command` | `variables: { KEY: { from_command: "..." } }` in collection |
| 6 | Vault secrets | `secrets:` block in `curlew.yaml` |
| 7 | Collection variables | `variables:` in the collection YAML |
| 8 | Request-scoped variables | `variables:` on a request item |
| 9 | `--env-var` | CLI: import OS env var |
| 10 | `--var` | CLI: explicit value — **wins against all** |

If you set `base_url` in `curlew.yaml` (precedence 2), override it in `environments/staging.yaml` (precedence 3), and then pass `--var base_url=https://local.test` on the command line (precedence 10), the CLI wins.

A worked example. Three files:

```yaml
# curlew.yaml
project_name: demo-api
variables:
  base_url: "https://api.example.com"
  region: "us-east-1"
```

```yaml
# environments/staging.yaml
variables:
  base_url: "https://staging.api.example.com"
  log_level: "debug"
```

Run:

```bash
curlew run collections/users.yaml --env staging --var region=eu-west-1
```

The effective scope for the run is:

| Variable | Value | Came from |
|---|---|---|
| `base_url` | `https://staging.api.example.com` | env file (beats project) |
| `region` | `eu-west-1` | `--var` (beats everything) |
| `log_level` | `debug` | env file |

### 3.3 Environments

An environment is a named set of variables, stored in `environments/<name>.yaml`. Load one with `--env`:

```bash
curlew run collections/users.yaml --env staging
```

Typical files:

```yaml
# environments/dev.yaml
variables:
  base_url: "http://localhost:3000"
  request_timeout_ms: "5000"
  log_level: "debug"
```

```yaml
# environments/staging.yaml
variables:
  base_url: "https://staging.api.example.com"
  request_timeout_ms: "10000"
  log_level: "info"
```

```yaml
# environments/prod.yaml
variables:
  base_url: "https://api.example.com"
  request_timeout_ms: "15000"
  log_level: "warn"
```

Environment files may contain nested maps; nested keys are flattened with underscores (`database.host` becomes `database_host`), and all values are converted to strings. Keep environment values simple — use collections or the project config for structure.

### 3.4 `.env` files

`.env` files are for **local** secrets and config that should never be committed. The scaffold from `curlew init` already adds `.env` to `.gitignore`.

Format:

```bash
# Comments start with #
base_url=http://localhost:3000
api_key=sk_live_abc123
!sensitive db_password=correcthorsebatterystaple
```

- One `KEY=VALUE` pair per line.
- Quotes are optional: `KEY="value"` and `KEY=value` are equivalent.
- Prefix with `!sensitive` to mark the value as secret (see §4.4 and §6.7).
- `.env` is loaded automatically if present in the project root.

`.env.example` is the canonical way to document what variables a collaborator needs without leaking real values:

```bash
# .env.example (committed)
# API_KEY=your-api-key-here
# DB_PASSWORD=your-db-password
```

### 3.5 CLI variable flags

Two flags override everything else. Both are repeatable.

**`--var key=value`** — explicit value. Wins against all file-based sources:

```bash
curlew run collections/users.yaml \
  --var base_url=http://localhost:8080 \
  --var user_id=42
```

**`--env-var VAR_NAME`** — import an OS environment variable of the same name:

```bash
export API_KEY=sk_live_abc123
curlew run collections/users.yaml --env-var API_KEY
```

This form is ideal in CI, where secrets are typically set as environment variables on the pipeline.

**`--env-var NEW_NAME=$OS_NAME`** — import and rename:

```bash
export CI_API_TOKEN=abc
curlew run collections/users.yaml --env-var api_token='$CI_API_TOKEN'
```

Quote the value in your shell so the shell doesn't expand `$CI_API_TOKEN` before Curlew sees it.

### 3.6 Project-wide config (`curlew.yaml`)

`curlew.yaml` lives at the project root. Every command that loads a collection walks up from the collection file's directory looking for `curlew.yaml` or `curlew.yml` and loads it automatically.

Full schema (with Part 6 additions shown as teasers):

```yaml
project_name: demo-api                    # required, non-empty

variables:                                # project-level vars (precedence 2)
  base_url: "https://httpbin.org"
  api_version: "v2"

secrets:                                  # vault config — see §6.4
  provider: aws-secrets-manager
  region: us-east-1
  keys:
    db_password: "prod/db#password"

auth_profiles:                            # dynamic auth — see §6.5
  admin_token:
    type: dynamic
    collection: "auth/admin-login.yaml"
    extract: "access_token"
    cache_ttl: 600

defaults:                                 # project-level defaults
  retry:                                  # inherited by collections (§5.4)
    enabled: true
    max_attempts: 3
  graphql:
    error_handling:
      partial_success: warn               # default for GraphQL requests (§7.1)
```

For now, `project_name` and `variables` are enough; `secrets`, `auth_profiles`, and `defaults` are introduced in later parts.

### 3.6.1 The `output:` block

Curlew accepts an optional `output:` block at two levels: in `curlew.yaml` (project default) and in the collection YAML (per-collection override). The block supports four optional fields:

| Field       | YAML type | Values                                     | Default    |
|-------------|-----------|---------------------------------------------|------------|
| `format`    | string    | `terminal`, `json`, `tap`, `junit`, `html`  | `terminal` |
| `report`    | string    | file path (non-empty)                       | _(none)_   |
| `events`    | string    | file path (non-empty)                       | _(none)_   |
| `verbosity` | string    | `quiet`, `normal`, `verbose`, `debug`       | `normal`   |

Example `curlew.yaml` with project-wide output defaults:

```yaml
project_name: my-api
output:
  format: json
  verbosity: normal
```

Example collection that overrides only the format:

```yaml
name: Smoke Tests
output:
  format: tap
requests:
  - name: health
    request: { method: GET, url: "{{base_url}}/health" }
```

**Precedence (each field resolved independently):**

> CLI flag > collection `output:` > project `output:` > built-in default

This means you can set a project-wide `format: json` and override it per-collection or per-invocation with `--format terminal`. The resolution happens once before any HTTP request runs, so watch mode and glob expansion both honour it correctly.

**`curlew schema --project`** emits the JSON Schema for `curlew.yaml` so you can validate your project config programmatically.

### 3.7 Dynamic and faker functions

Dynamic functions generate values at request time. Syntax: `{{$functionName}}`.

**The fifteen no-argument built-in functions:**

| Function | Returns | Example |
|---|---|---|
| `{{$uuid}}` | UUID v4 | `550e8400-e29b-41d4-a716-446655440000` |
| `{{$guid}}` | Alias for `$uuid` | same |
| `{{$timestamp}}` | Unix seconds | `1713701401` |
| `{{$timestampMs}}` | Unix milliseconds | `1713701401234` |
| `{{$isoTimestamp}}` | ISO 8601 UTC | `2026-04-21T15:10:01Z` |
| `{{$randomInt}}` | Integer 0–1000 | `427` |
| `{{$randomFloat}}` | Float 0.0–1000.0 (2 decimals) | `523.47` |
| `{{$randomBoolean}}` | `true` or `false` | `true` |
| `{{$randomString}}` | 16 alphanumeric chars | `aBc2dEf3gHi4jKlM` |
| `{{$randomHex}}` | 32 hex digits | `f4a3c1e2b8d0a7f2…` |
| `{{$randomEmail}}` | `firstname.NNNN@example.com` | `alice.3847@example.com` |
| `{{$randomName}}` | First Last | `Alice Smith` |
| `{{$randomFirstName}}` | First name | `Alice` |
| `{{$randomLastName}}` | Last name | `Smith` |
| `{{$randomColor}}` | `#RRGGBB` hex | `#f4a3c1` |

**Argument-bearing helpers (M12+):**

| Function | Arguments | Returns | Example |
|---|---|---|---|
| `{{$base64('s')}}` | 1 string | Standard base64 encoding (RFC 4648 §4) of UTF-8 bytes | `{{$base64('user:pw')}}` → `dXNlcjpwdw==` |
| `{{$base64Decode('s')}}` | 1 string | Decoded UTF-8 string from standard base64 | `{{$base64Decode('aGVsbG8=')}}` → `hello` |
| `{{$urlEncode('s')}}` | 1 string | Query-component percent-encoding (`net/url.QueryEscape`); space → `+`, `&` → `%26`, `/` → `%2F` | `{{$urlEncode('hello world & co')}}` → `hello+world+%26+co` |
| `{{$jsonEncode('s')}}` | 1 string | RFC 8259 JSON-string literal of the input, **including the surrounding double quotes**, with `"`, `\`, and control bytes escaped | `{{$jsonEncode('he said "hi"')}}` → `"he said \"hi\""` |
| `{{$sha256('s')}}` | 1 string | Lowercase hex of `crypto/sha256.Sum256` over UTF-8 bytes (64 chars) | `{{$sha256('hello')}}` → `2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824` |
| `{{$md5('s')}}` | 1 string | Lowercase hex of `crypto/md5.Sum` over UTF-8 bytes (32 chars) — **legacy parity only** | `{{$md5('hello')}}` → `5d41402abc4b2a76b9719d911017c592` |
| `{{$hmacSha256('payload', 'key')}}` | 2 strings | Lowercase hex of `h := hmac.New(sha256.New, []byte(key)); h.Write([]byte(payload)); hex(h.Sum(nil))` (64 chars) — RFC 2104 HMAC-SHA-256 | `{{$hmacSha256('The quick brown fox jumps over the lazy dog', 'key')}}` → `f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8` |
| `{{$dateAdd('amount', 'unit')}}` | 2 strings | ISO-8601 UTC of `now + amount × unit`. Units: `second`, `minute`, `hour`, `day`, `week`, `month`, `year`. Negative amounts allowed. | `{{$dateAdd('7', 'day')}}` → `2026-05-05T12:00:00Z` |
| `{{$dateSubtract('amount', 'unit')}}` | 2 strings | Equivalent to `$dateAdd` with a negated amount. | `{{$dateSubtract('30', 'minute')}}` → `2026-04-28T11:30:00Z` |
| `{{$formatDate('input', 'layout')}}` | 2 strings | Parses `input` as either a Unix-seconds integer string or an RFC3339 string, then formats with `layout` interpreted as a Go reference-time layout. | `{{$formatDate('1713701401', '2006-01-02')}}` → `2024-04-21` |
| `{{$parseDate('s', 'layout')}}` | 2 strings | Parses `s` against `layout` (Go reference-time), converts to UTC, returns canonical ISO-8601 (`2006-01-02T15:04:05Z`). | `{{$parseDate('21/04/2024 15:10:01', '02/01/2006 15:04:05')}}` → `2024-04-21T15:10:01Z` |
| `{{$randomPassword('n')}}` | 1 integer string (n >= 4) | An n-character password guaranteed to contain at least one upper, one lower, one digit, and one symbol from `!@#$%^&*()-_=+[]{}<>?,.`. Seed-deterministic when `--seed` is set. | `{{$randomPassword('16')}}` → a 16-char mixed-class string |
| `{{$randomBase64('byteLength')}}` | 1 integer string (>= 1) | Standard-base64 encoding (RFC 4648 §4) of `byteLength` random bytes. Seed-deterministic when `--seed` is set; otherwise sourced from `crypto/rand`. | `{{$randomBase64('32')}}` → a 44-char base64 string |

**`$faker.*` personal-data functions (M13-002):**

| Function | Returns | Example (seed 42) |
|---|---|---|
| `{{$faker.firstName}}` | First name from a 50-name en-US pool | `Carol` |
| `{{$faker.lastName}}` | Last name from a 49-name en-US pool | `Johnson` |
| `{{$faker.fullName}}` | `<firstName> <lastName>` joined by a single ASCII space | `Carol Johnson` |
| `{{$faker.username}}` | `<lower(first)>.<lower(last)><0-99>` — matches `^[a-z0-9._]+$` | `carol.johnson42` |
| `{{$faker.email}}` | `<lower(first)>.<lower(last)>@example.com` | `carol.johnson@example.com` |
| `{{$faker.phone}}` | US format `(NNN) NNN-NNNN` | `(642) 837-5291` |
| `{{$faker.phoneInternational}}` | E.164-shape `+1-NNN-NNN-NNNN` (en-US only in M13) | `+1-642-837-5291` |
| `{{$faker.ssn}}` | US SSN `NNN-NN-NNNN` — **auto-sensitive**, redacted as `[REDACTED]` in JSON/markdown output | `347-82-1924` |
| `{{$faker.namePrefix}}` | One of `Mr.`, `Mrs.`, `Ms.`, `Dr.`, `Prof.` | `Dr.` |
| `{{$faker.nameSuffix}}` | One of `Jr.`, `Sr.`, `II`, `III`, `IV`, `PhD`, `MD`, `Esq.` | `PhD` |

Name and phone pools are locale-aware as of M20. Use `--locale` (or
`config.locale:` in the collection) to select any of the 15 supported
locales; see the [locale reference table](#faker-locales-m20) below.

**`$faker.*` location-data functions (M13-003):**

| Function | Returns | Example |
|---|---|---|
| `{{$faker.address}}` | US-format full address `<num> <street>, <city>, <stateAbbr> <zip>` | `8960 Meadow Rd, Manchester, CT 88423` |
| `{{$faker.street}}` | Street with building number `<num> <streetName> <suffix>` | `5458 Spring Ln` |
| `{{$faker.streetName}}` | Street name only — `<name> <suffix>` (no leading number) | `Walnut Pl` |
| `{{$faker.city}}` | US city name from a 30-name pool | `Riverside` |
| `{{$faker.state}}` | Full US state name | `Pennsylvania` |
| `{{$faker.stateAbbr}}` | 2-letter US state code (ISO 3166-2:US) | `MD` |
| `{{$faker.zipCode}}` | 5-digit US ZIP code (uniform random; no per-state realism) | `29342` |
| `{{$faker.country}}` | Country name from a 30-name pool | `Canada` |
| `{{$faker.countryCode}}` | ISO 3166-1 alpha-2 country code | `DE` |
| `{{$faker.latitude}}` | Float in `[-90.0000, 90.0000]` with 4 decimals — string-encoded; renders as a JSON number when placed in a numeric position (`{"lat":{{$faker.latitude}}}`) | `39.3388` |
| `{{$faker.longitude}}` | Float in `[-180.0000, 180.0000]` with 4 decimals — same JSON-numeric convention | `-173.8983` |
| `{{$faker.timezone}}` | IANA timezone identifier (loadable via `time.LoadLocation`) | `UTC` |

State and country pools are 1:1 aligned (so `$faker.state` and
`$faker.stateAbbr` render the matching full name and abbreviation for the
same pool index — e.g. Illinois/IL). City pools are locale-aware as of
M20; `$faker.city` draws from the active locale's city pool. Address,
street, state, zip, and country pools remain en-US format — use `--locale`
to select locale-specific name and city output; see the
[locale reference table](#faker-locales-m20) below.

**`$faker.*` company-data functions (M13-004):**

| Function | Returns | Example |
|---|---|---|
| `{{$faker.company}}` | Company name from a 30-entry curated pool | `Acme Corporation` |
| `{{$faker.companySuffix}}` | One of `Inc.`, `LLC`, `Corp.`, `Ltd.`, `Co.` (closed canonical set) | `Inc.` |
| `{{$faker.jobTitle}}` | Job title from a 30-entry pool | `Senior Developer` |
| `{{$faker.department}}` | Department name from a 30-entry pool | `Engineering` |
| `{{$faker.catchPhrase}}` | Three-word business buzzword phrase: `<adjective> <noun> <gerund>` (three independent pool draws joined by single spaces) | `Synergized leverage scaling` |

Company-name and job-title pools are locale-neutral en-US data by design;
`--locale` does not localize this family. The personal-data and location
families (`$faker.firstName`, `$faker.city`, `$faker.phone`) are
locale-aware — see the [locale reference table](#faker-locales-m20).

**`$faker.*` internet-data functions (M13-005):**

| Function | Returns | Example |
|---|---|---|
| `{{$faker.url}}` | Full URL with scheme, host, and path — `https://<root>.<tld>/<segment>` | `https://example.com/home` |
| `{{$faker.domain}}` | Hostname with TLD only — `<root>.<tld>` (no scheme, no path) | `acme.io` |
| `{{$faker.domainSuffix}}` | TLD without a leading dot, drawn from a 12-entry pool (`com`, `org`, `net`, `io`, `dev`, `app`, `co`, `uk`, `de`, `jp`, `eu`, `tech`) | `com` |
| `{{$faker.ip}}` | IPv4 dotted-quad — each octet in `[0, 255]` | `192.0.2.123` |
| `{{$faker.ipv6}}` | IPv6 full-form (eight 4-digit hex groups joined by `:`); parseable by `net.ParseIP` | `2001:0db8:85a3:0000:0000:8a2e:0370:7334` |
| `{{$faker.mac}}` | MAC-48 with **uppercase** hex (`^([0-9A-F]{2}:){5}[0-9A-F]{2}$`) | `00:1B:63:84:45:E6` |
| `{{$faker.userAgent}}` | Browser user-agent string from a 6-entry pool (Chrome / Firefox / Safari / Edge on Windows / macOS / Linux); every entry begins with `Mozilla/5.0` | `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36` |
| `{{$faker.color}}` | CSS color **name** keyword (e.g. `red`, `blue`, `crimson`) drawn from a 24-entry pool | `blue` |
| `{{$faker.hexColor}}` | `#RRGGBB` lowercase hex color code | `#3498db` |

TLD pools and user-agent strings are locale-neutral by design; `--locale`
does not localize this family. The personal-data and location families
are locale-aware — see the [locale reference table](#faker-locales-m20).

**`$faker.color` vs legacy `$randomColor` — both ship.** The
existing `$randomColor` (M11) returns a `#RRGGBB` hex string; the
new `$faker.color` (M13-005) returns a
CSS color keyword (`"blue"`, `"crimson"`, etc.). Both registrations
co-exist as distinct functions — no aliasing, no deprecation. Pick
whichever matches your test fixture: hex for CSS inline styles, name
for accessibility labels and stylesheet keywords. `$faker.hexColor`
is also available if you want the hex form under the `$faker.*`
namespace specifically.

**`$faker.*` content-data functions (M13-006):**

These functions emit lorem-ipsum prose. Four of the five accept an
optional integer-string count argument that controls the size of
the output. The argument is parsed by the M12-001 dynamic-argument
parser (single-quoted string); each function `strconv.Atoi`-converts
it internally, exactly like `$randomBase64('32')`.

| Function | Returns | Example |
|---|---|---|
| `{{$faker.word}}` | A single lowercase word from the lorem-ipsum pool | `lorem` |
| `{{$faker.words}}` or `{{$faker.words('count')}}` | `count` space-joined lowercase words (default: 3) | `lorem ipsum dolor` |
| `{{$faker.sentence}}` or `{{$faker.sentence('wordCount')}}` | A complete sentence with `wordCount` words (default: 6–10), first word capitalised, period-terminated | `Lorem ipsum dolor sit amet.` |
| `{{$faker.paragraph}}` or `{{$faker.paragraph('sentenceCount')}}` | `sentenceCount` sentences (each 6–10 words) joined by single spaces (default: 3–5 sentences) | `Lorem ipsum dolor sit amet. Consectetur adipiscing elit.` |
| `{{$faker.text}}` or `{{$faker.text('charCount')}}` | Approximately `charCount` characters of prose; the generator stops at the first sentence boundary at-or-after `charCount` to keep output well-formed (default: 200) | `Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor.` |

The lorem-ipsum pool is locale-neutral Latin; `--locale` does not change
content output for this family. This is by design — lorem-ipsum text is
conventionally locale-agnostic.

**Bad-input handling.** Passing a non-integer count (e.g.
`{{$faker.words('abc')}}`) returns a structured `CategoryInput`
error with code `DYNFN_FAKER_CONTENT_BAD_INPUT`. Passing a count
below 1 returns `DYNFN_FAKER_CONTENT_BAD_LENGTH`. Both shapes mirror
the existing `$randomBase64` and `$randomPassword` error contracts.

**`$faker.*` financial-data functions (M13-007):**

These functions emit financial values. Three (`creditCard`,
`creditCardCVV`, `iban`) are **auto-sensitive** — the generated
value is registered as a redaction trigger for the run, so it
appears as `[REDACTED]` wherever the request body is rendered: in
the `-vv` terminal body dump, the markdown report body section, the
events stream, and `--log` output. `$faker.bic` is NOT
auto-sensitive — BIC/SWIFT codes identify a bank, not an account.

`$faker.price` is the only argument-bearing function — it accepts
an optional `(min, max)` pair as two single-quoted decimal strings,
parsed by the M12-001 dynamic-argument parser. Both values are
`strconv.ParseFloat`-converted; passing `min > max` returns a
structured `DYNFN_FAKER_PRICE_INVERTED_RANGE` error.

| Function | Returns | Example |
|---|---|---|
| `{{$faker.price}}` or `{{$faker.price('min','max')}}` | A 2-decimal float in `[min, max]` (default: `[1.00, 1000.00]`) | `99.99` |
| `{{$faker.currencyCode}}` | A 3-letter ISO 4217 currency code | `USD` |
| `{{$faker.currencyName}}` | A full currency name | `US Dollar` |
| `{{$faker.currencySymbol}}` | A currency symbol | `$` |
| `{{$faker.creditCard}}` | A 16-digit Visa-like number passing the Luhn checksum — **auto-sensitive**, redacted as `[REDACTED]` in body output | `4111111111111111` |
| `{{$faker.creditCardCVV}}` | A 3-or-4 digit CVV — **auto-sensitive**, redacted as `[REDACTED]` in body output | `123` |
| `{{$faker.iban}}` | A valid IBAN (mod-97 checksum) for a randomly chosen country (DE/GB/FR/ES/IT/NL/BE/CH) — **auto-sensitive**, redacted as `[REDACTED]` in body output | `DE89370400440532013000` |
| `{{$faker.bic}}` | An 8-or-11-character BIC/SWIFT code | `COBADEFFXXX` |

The three currency pools (codes + names + symbols) are aligned 1:1:1
by index but the three functions draw independently when called
separately. Index-aligned access to the pools is reserved for
future "matched currency triple" use cases.

Currency and financial pools are locale-neutral by design; `--locale`
does not localize this family.

**Bad-input handling.** Passing a non-numeric arg to `$faker.price`
(e.g. `{{$faker.price('abc','50')}}`) returns a structured
`CategoryInput` error with code `DYNFN_FAKER_PRICE_BAD_INPUT`.
Passing `min > max` returns `DYNFN_FAKER_PRICE_INVERTED_RANGE`.
Passing 1 or 3+ args returns `DYNFN_ARITY` ("expected 0 or 2
arguments"). Both shapes mirror the existing `$randomBase64` and
`$randomPassword` error contracts.

**`$faker.*` file-data functions (M13-008):**

These functions emit file-related metadata: filenames, extensions,
MIME types, and a placeholder image URL. The extension and MIME-type
pools are aligned 1:1 by index — `fileExtensions[i]` and
`fileMimeTypes[i]` refer to the same format — so the test suite can
spot-check canonical pairings, but the three RNG-bearing functions
draw independently when called separately.

`$faker.imageUrl` is the family's only argument-bearing function —
it accepts an optional `(width, height)` pair as two single-quoted
positive-integer strings, parsed by the M12-001 dynamic-argument
parser. The no-arg form returns the constant default URL
`https://picsum.photos/640/480` — that default is intentionally
deterministic across runs (it's a fixed string), so the no-seed
entropy property does not apply to it.

| Function | Returns | Example |
|---|---|---|
| `{{$faker.fileName}}` | A lowercase filename with a dotted extension matching `^[a-z0-9_-]+\.[a-z0-9]+$` | `document.pdf` |
| `{{$faker.fileExtension}}` | A 2-to-4 character lowercase extension with no leading dot | `pdf` |
| `{{$faker.mimeType}}` | A MIME type matching `^[a-z]+/[a-z0-9.+-]+$` | `application/pdf` |
| `{{$faker.imageUrl}}` or `{{$faker.imageUrl('width','height')}}` | A picsum.photos URL — `https://picsum.photos/640/480` (default) or `https://picsum.photos/<w>/<h>` for the arg-bearing form | `https://picsum.photos/640/480` |

File name, extension, MIME type, and image URL pools are locale-neutral
by design; `--locale` does not localize this family.

**Bad-input handling.** Passing a non-integer arg to
`$faker.imageUrl` (e.g. `{{$faker.imageUrl('abc','480')}}`) returns
a structured `CategoryInput` error with code
`DYNFN_FAKER_IMAGEURL_BAD_INPUT`. Passing a non-positive integer
(e.g. `{{$faker.imageUrl('-100','480')}}` or
`{{$faker.imageUrl('0','480')}}`) returns
`DYNFN_FAKER_IMAGEURL_BAD_DIMENSION`. Passing 1 or 3+ args returns
`DYNFN_ARITY` ("expected 0 or 2 arguments"). All shapes mirror the
existing `$randomBase64` and `$faker.price` error contracts.

`$faker.latitude` and `$faker.longitude` follow the same
rendered-string-into-JSON convention as `$randomInt` and `$randomFloat`:
the registry returns a string, but the placeholder lands unquoted in
JSON if the surrounding context is a JSON-numeric position
(`{"lat":{{$faker.latitude}}}` → `{"lat":40.7128}`) and quoted if it
is inside a JSON-string context.

#### `$faker.*` locales (M20)

The personal-data and city/location families are locale-aware: use
`--locale` or `config.locale:` to select any of the 15 supported locales.
Company, internet, content, financial, and file-data families are
locale-neutral by design.

**Supported locales:**

| Locale | Language / Region | Name format | Phone format |
|--------|-------------------|-------------|--------------|
| `en-US` | US English (default) | John Smith | (555) 123-4567 |
| `en-GB` | British English | John Smith | +44 20 7946 0958 |
| `de-DE` | German | Johann Schmidt | +49 30 12345678 |
| `fr-FR` | French | Jean Dupont | +33 1 23 45 67 89 |
| `es-ES` | Spanish | Juan García | +34 91 123 45 67 |
| `it-IT` | Italian | Giovanni Rossi | +39 06 1234 5678 |
| `pt-BR` | Brazilian Portuguese | João Silva | +55 11 91234-5678 |
| `ja-JP` | Japanese | 山田 太郎 | +81 3-1234-5678 |
| `zh-CN` | Simplified Chinese | 张伟 | +86 10 1234 5678 |
| `ko-KR` | Korean | 김민준 | +82 2-1234-5678 |
| `nl-NL` | Dutch | Jan de Vries | +31 20 123 4567 |
| `pl-PL` | Polish | Jan Kowalski | +48 22 123 45 67 |
| `ru-RU` | Russian | Иван Иванов | +7 495 123-45-67 |
| `sv-SE` | Swedish | Erik Svensson | +46 8 123 45 67 |
| `tr-TR` | Turkish | Ahmet Yılmaz | +90 212 123 45 67 |

**Precedence chain (SPEC:1006):**
Default (`en-US`) < Project < Environment < Collection < CLI flag (`--locale`).

- **Default** — `en-US` when no locale is configured anywhere.
- **Project** — `config.locale:` in `curlew.yaml` (project-level default).
- **Environment** — `config.locale:` in an environment YAML file (overrides project).
- **Collection** — `config.locale:` in the collection YAML file.
- **CLI flag** — `--locale de-DE` (highest priority, overrides all).

(The environment-config locale seam is reserved in the precedence chain
but environment-file parsing for locale is not yet wired; in current builds
collection config is the effective level above project.)

**Fallback chain (SPEC:989):** An unsupported regional variant falls back
through its language root to `en-US`. For example, `en-GB` → `en` →
`en-US`. In practice all 15 locale codes have shipped pools so no
fallback occurs for supported codes. Fallback warnings surface in verbose
(`-v`) mode.

**`ERR_LOCALE_UNKNOWN`:** An unsupported `--locale` value aborts the run
with a structured `CategoryInput` error and lists the supported locales:

```
[INPUT] ERR_LOCALE_UNKNOWN: unknown locale "xx-YY"
  hint: Supported locales: en-US, en-GB, de-DE, fr-FR, es-ES, it-IT,
        pt-BR, ja-JP, zh-CN, ko-KR, nl-NL, pl-PL, ru-RU, sv-SE, tr-TR
```

**Seed and locale are independent (SPEC:1023-1026):** A fixed `--seed`
selects the same draw position in every locale's pool. Switching locale
changes the *language* of the output, not *which* entry is chosen. Run
the worked example to see it end-to-end:

```bash
./curlew run testdata/locale/reproducibility-matrix.yaml --seed 12345
```

The exhaustive all-15-locale proof is in `TestLocale_ReproducibilityMatrix`
(`internal/variable/locale_matrix_test.go`).

**SSN auto-redaction.** `$faker.ssn` is the only auto-sensitive function
in the personal-data category. Each generated SSN is registered as a
redaction trigger for the run, so it appears as `[REDACTED]` wherever
the request body is rendered: in the `-vv` terminal body dump, the
markdown report body section, the events stream, and `--log` output.
Example:

```yaml
body:
  name: "{{$faker.fullName}}"
  ssn:  "{{$faker.ssn}}"
```

With `-vv`, `name` resolves to a real name string while `ssn` appears
as `[REDACTED]` in the terminal body dump. The `--format json` output
does not include a request-body field, so the SSN simply does not
appear there at all (positive-redaction proof is in `-vv` and the
markdown report).

`$base64Decode` rejects malformed input with a structured `[INPUT]`
error that includes a 32-character snippet of the offending value.
`$urlEncode` mirrors `net/url.QueryEscape` exactly — note the `+`
for space, which is the query-component convention; use it in
URL query strings, not in path segments. `$jsonEncode` returns
the literal JSON-string form **with** surrounding `"…"`, so the
result drops directly into a JSON body (`{"name": {{$jsonEncode('{{name}}')}}}`)
without further quoting. All seven helpers honour per-request
memoisation keyed by their resolved argument(s).

**Sensitive-key auto-redaction (`$hmacSha256`).** When the *key*
argument (the second positional arg) of `$hmacSha256` resolves from
a variable whose name matches the sensitive-name heuristic
(`secret`, `token`, `password`, `api_key`, `apikey`, `credential`,
`authorization`) — e.g. `STRIPE_SIGNING_SECRET` — or from a
`{{secrets.X}}` token, the resolved key string is automatically
registered as a redaction trigger for the run. It will be replaced
with `[REDACTED]` wherever it appears in serialised request and
response bodies in `--format json`, `--log`, the events stream, and
all other output sinks.

**The literal-key footgun.** A literal key passed inline — e.g.
`{{$hmacSha256('payload', 'sk_live_abc123')}}` — *cannot* be
auto-redacted, because there is no source variable to inspect.
Inline keys leak into output. Always source HMAC keys from a
sensitive-named variable, an env file, `{{secrets.X}}`, or a
`from_command:` block.

The payload argument (the first positional arg) is **not**
auto-marked, even when it itself resolves from a sensitive-named
variable. HMAC payloads are typically the data being signed, not
the secret; if your payload contains other sensitive material, mark
*that* variable as sensitive at its own definition site.

**Webhook-signature helpers (`$webhookSign.stripe`, `$webhookSign.github`, `$webhookSign.slack`) — M17-004.**

These three dynamic functions emit the **value** of the provider-specific
webhook-signature header (not the header name). Each is a thin shaping
layer over HMAC-SHA-256 under the second positional argument (the
secret). The signed-payload construction follows each provider's
published spec.

| Function | Arguments | Returns |
|---|---|---|
| `{{$webhookSign.stripe('body','secret','timestamp')}}` | 3 strings (or 2 — clock seam supplies timestamp) | `t=<timestamp>,v1=<hex>` — `<hex>` = HMAC-SHA-256(`<timestamp>.<body>`, `secret`) |
| `{{$webhookSign.github('body','secret')}}` | 2 strings | `sha256=<hex>` — `<hex>` = HMAC-SHA-256(`body`, `secret`) |
| `{{$webhookSign.slack('body','secret','timestamp')}}` | 3 strings (or 2 — clock seam supplies timestamp) | `v0=<hex>` — `<hex>` = HMAC-SHA-256(`v0:<timestamp>:<body>`, `secret`) |

Examples:

```yaml
headers:
  # Stripe: pin the timestamp; the 2-arg form uses the clock seam
  X-Stripe-Signature: "{{$webhookSign.stripe('{{payload}}', '{{stripe_signing_secret}}', '{{stripe_ts}}')}}"
  # GitHub x-hub-signature-256
  X-Hub-Signature-256: "{{$webhookSign.github('{{payload}}', '{{github_webhook_secret}}')}}"
  # Slack X-Slack-Signature; 2-arg form uses current time
  X-Slack-Signature: "{{$webhookSign.slack('{{payload}}', '{{slack_signing_secret}}', '{{slack_ts}}')}}"
```

The two-arg forms of `$webhookSign.stripe` and `$webhookSign.slack`
default the timestamp to the current Unix-seconds time (the same clock
seam used by `$timestamp`, `$dateAdd`, `$dateSubtract`); use the three-arg
form when you need to pin the timestamp deterministically (e.g. against
a recorded webhook fixture).

GitHub's legacy SHA-1 form (`x-hub-signature`) is intentionally not
shipped — only the v2 SHA-256 form (`x-hub-signature-256`). New
GitHub integrations must use the SHA-256 header.

**Sensitive-secret auto-redaction.** Like `$hmacSha256`, when the *secret*
argument (the second positional arg) of any of the three functions
resolves from a sensitive-named variable (`secret`, `token`,
`password`, `api_key`, `apikey`, `credential`, `authorization`) or
from a `{{secrets.X}}` token, the resolved secret string is
automatically registered as a redaction trigger for the run. It will
appear as `[REDACTED]` wherever it surfaces in serialised output.

**The literal-secret footgun.** A literal secret passed inline — e.g.
`{{$webhookSign.github('payload', 'whsec_inline_value')}}` — *cannot*
be auto-redacted because there is no source variable to inspect. Always
source webhook secrets from a sensitive-named variable, an env file,
`{{secrets.X}}`, or a `from_command:` block. Mirrors the `$hmacSha256`
documentation above.

**JWT decode helpers (`$jwtDecodeHeader`, `$jwtDecodeClaims`) — M17-005.**

These two dynamic functions split a JWT on `.` and base64-url-decode the
requested segment, returning the decoded JSON as a canonical compact
string. The third segment (the signature) is **never examined** — these
helpers are decode-only.

| Function | Arguments | Returns | Example |
|---|---|---|---|
| `{{$jwtDecodeHeader('token')}}` | 1 string (the JWT) | The header segment as compact JSON | `{{$jwtDecodeHeader('eyJ...JWT...')}}` → `{"alg":"HS256","typ":"JWT"}` |
| `{{$jwtDecodeClaims('token')}}` | 1 string (the JWT) | The claims segment as compact JSON | `{{$jwtDecodeClaims('eyJ...JWT...')}}` → `{"iat":1516239022,"name":"John Doe","sub":"1234567890"}` |

The output is always **canonical compact JSON** with map keys sorted
alphabetically — this is the standard `encoding/json` round-trip
guarantee. The original key order in the source token is **not**
preserved; downstream `$jsonpath`, `$regex`, and equality assertions
should not rely on input ordering. Values are byte-exact (UTF-8 strings
round-trip unchanged, including multi-byte sequences).

Example — decode a JWT stored in a variable and echo the fields back as
custom request headers (useful for debugging or audit logging):

```yaml
- name: inspect-token
  request:
    method: GET
    url: "{{base_url}}/me"
    headers:
      Authorization: "Bearer {{access_token}}"
      # These headers carry the decoded JSON strings — e.g.:
      #   X-Token-Header:  {"alg":"HS256","typ":"JWT"}
      #   X-Token-Claims:  {"iat":1516239022,"name":"John Doe","sub":"1234567890"}
      # The values are canonical compact JSON (keys sorted alphabetically),
      # ready for a downstream body assertion using JSONPath.
      X-Token-Header: "{{$jwtDecodeHeader('{{access_token}}')}}"
      X-Token-Claims: "{{$jwtDecodeClaims('{{access_token}}')}}"
```

Example — assert a field in the JWT claims against an expected value.
Because `$jwtDecodeClaims` returns a JSON string, you can reference it
in any string-valued field and let a body assertion (or a variable
compare with `$regex`) inspect the value:

```yaml
- name: login
  request:
    method: POST
    url: "{{base_url}}/login"
    body: { username: "alice", password: "{{password}}" }
  assertions:
    status: 200
  extract:
    access_token: "$.token"   # extract the raw JWT from the response

- name: assert-claims-sub
  request:
    method: GET
    url: "{{base_url}}/whoami"
    headers:
      # Embed the decoded claims JSON as a custom header so the
      # rendered value appears in --format json output for inspection.
      X-Claims: "{{$jwtDecodeClaims('{{access_token}}')}}"
  assertions:
    status: 200
```

**Errors.** Three structured `[INPUT]` codes distinguish the failure modes:

- `DYNFN_JWT_DECODE_BAD_FORMAT` — token does not split into exactly three
  `.`-separated segments.
- `DYNFN_JWT_DECODE_BAD_BASE64` — the target segment is not valid
  URL-safe base64 (RFC 4648 §5, no padding).
- `DYNFN_JWT_DECODE_BAD_JSON` — the segment base64-url-decodes
  successfully but the bytes are not valid JSON.

Each error message includes the offending input truncated to 32 chars
(with `...` marker) so long tokens cannot flood logs or leak in full.

**Security caveat — no signature verification.** Both functions decode
**without verifying the signature**. Calling `$jwtDecodeHeader` or
`$jwtDecodeClaims` does **not** prove the token was issued by anyone
trusted; an attacker can forge an arbitrary header and claims and the
decode will succeed. If your test asserts on a claim value as a
*security boundary* — for example, "the token is for user 42" — the
correctness of that assertion depends on the token's signature having
been verified by the API server (or a separate mechanism) before it was
returned to you. Curlew does not ship a `$jwtVerify` helper; if you
need cryptographic verification within a test, generate the expected
signature with `$hmacSha256` over the canonicalised header.claims
prefix and compare with `equals`.

**MD5 caveat.** `$md5` is provided for parity with legacy webhook
signature schemes (Mailgun, older Stripe variants, certain
on-prem systems) only. Do not use it for new integrations —
prefer `$sha256` for content hashes and `$hmacSha256` for signed
payloads. MD5 is collision-broken and unsuitable for any
security-relevant comparison.

**Date arithmetic (`$dateAdd`, `$dateSubtract`).** Both functions take
two string arguments — an integer `amount` and one of seven canonical
`unit` values (`second`, `minute`, `hour`, `day`, `week`, `month`,
`year`). The base instant is the wall-clock `now` at evaluation time,
formatted UTC ISO-8601 (`2026-04-21T15:10:01Z`) to match
`$isoTimestamp`. Negative amounts are accepted, so
`$dateAdd('-3', 'hour')` and `$dateSubtract('3', 'hour')` produce the
same value. `amount` must be an integer string — `'1.5'` is rejected
with a structured `[INPUT]` error. To express decimal hours, use a
smaller unit: `$dateAdd('90', 'minute')` instead of `$dateAdd('1.5',
'hour')`. Month and year offsets follow Go `time.AddDate` semantics —
adding 13 months to 2026-01-31 rolls forward to 2027-03-03 because
Feb 31 doesn't exist. Use day-based arithmetic when end-of-month
stability matters.

**Go reference-time layouts (`$formatDate`, `$parseDate`).** Both
functions interpret their `layout` argument using Go's reference-time
convention — the layout *is itself a literal date* representing
`Mon Jan 2 15:04:05 MST 2006` (Unix time `1136239445`). Each component
of the layout that matches one of these reference values is replaced
by the corresponding component of the actual time; everything else
passes through verbatim. The full grammar is on the stdlib
[`pkg.go.dev/time#pkg-constants`](https://pkg.go.dev/time#pkg-constants)
page. Common layouts:

| Layout | Renders | Example output |
|---|---|---|
| `2006-01-02` | ISO date | `2024-04-21` |
| `02 Jan 2006` | day-month-year | `21 Apr 2024` |
| `Mon Jan _2 15:04:05 2006` | ANSI C | `Sun Apr 21 15:10:01 2024` |
| `2006-01-02T15:04:05Z07:00` | RFC3339 (`time.RFC3339`) | `2024-04-21T15:10:01Z` |
| `02/01/2006 15:04:05` | EU date+time | `21/04/2024 15:10:01` |

`$formatDate` accepts two input shapes for the *first* argument: a
Unix-seconds integer string (e.g. `1713701401`) or an RFC3339 string
(e.g. `2024-04-21T15:10:01Z`). Anything else returns a structured
`[INPUT]` error. To format the current wall-clock time, nest
`{{$timestamp}}`: `{{$formatDate('{{$timestamp}}', '2006-01-02')}}`.

`$parseDate` is the inverse: it parses any layout-formatted string
and returns canonical ISO-8601 UTC. Non-UTC zone-tagged inputs are
converted to UTC before formatting, so `$parseDate('2024-04-21T17:10:01+02:00', time.RFC3339)`
yields `2024-04-21T15:10:01Z`.

**The layout-as-template footgun.** Go's `time.Format` returns the
layout verbatim if it contains no recognised reference-time tokens.
That means `$formatDate('1713701401', 'YYYY-MM-DD')` returns the
literal string `YYYY-MM-DD` — *not* a date. There is no validation
step. Always sanity-check a layout by passing it through
`time.Parse(layout, "2006-01-02T15:04:05Z")` (or a quick `go run`)
before relying on it in production. The valid spelling for ISO date
is `2006-01-02`.

**Random secrets (`$randomPassword`, `$randomBase64`).** `$randomPassword(n)`
returns an `n`-character password (`n >= 4`) sampled across four classes —
ASCII upper, lower, digit, and the symbol set
`!@#$%^&*()-_=+[]{}<>?,.` — with at least one character from each class
guaranteed (the remainder is filled uniformly across all classes, then
Fisher-Yates shuffled). The symbol set is fixed; configurable symbol sets
are not yet supported. `$randomBase64(byteLength)` returns the standard-base64
encoding (RFC 4648 §4) of `byteLength` random bytes; the encoded string
is roughly `4 × ceil(byteLength / 3)` characters. Both functions honour
`--seed` for reproducibility — under a fixed seed, the same length always
produces the same output. Without a seed, both draw from `crypto/rand`.

`$randomPassword('3')` (or any `n < 4`) returns a structured `[INPUT]`
error — at least four characters are needed to satisfy the four-class
guarantee. Non-integer arguments (`{{$randomPassword('xyz')}}`,
`{{$randomBase64('1.5')}}`) return `[INPUT]` errors quoting the bad
input. `$randomBase64('0')` and negative byte lengths also error.

Within a single request, multiple references to the same function return the same value (per-request memoization):

```yaml
- name: Create user
  request:
    method: POST
    url: "{{base_url}}/users"
    body:
      id: "{{$uuid}}"
      external_id: "{{$uuid}}"       # different from id — different reference
    headers:
      X-Idempotency-Key: "{{$uuid}}" # same value as body.id? No — each occurrence is memoized independently
```

Actually — important clarification: memoization is keyed by function name, not by occurrence. So within a single request, **every `{{$uuid}}` resolves to the same UUID.** If you want distinct UUIDs in the same request, use different field layouts or an `extract` round trip.

Between requests, values are regenerated — the memo cache is scoped to the request.

**Parenthesized arguments (M12+).**

Dynamic functions can accept arguments in parentheses: `{{$func('arg1', 'arg2')}}`.
Each argument is a single-quoted string literal; `\'` and `\\` are honoured as
escapes. Arguments may contain other variable references — those are resolved
*before* the literal is passed to the function:

```yaml
headers:
  Authorization: "Basic {{$base64('{{user}}:{{pass}}')}}"
  X-Signature:   "{{$hmacSha256('payload', '{{secret}}')}}"
```

The 15 no-argument built-in functions (from the first table in §3.7 above) take
no arguments — call them with `{{$timestamp}}` (legacy form) or equivalently
`{{$timestamp()}}`. Calling a zero-arg helper with arguments produces a
structured input error. Per-request memoization keys include the resolved
arguments, so `{{$base64('a')}}` and `{{$base64('b')}}` do not collide in the
same request.

(All M12 argument-bearing helpers — `$base64`, `$base64Decode`,
`$urlEncode`, `$jsonEncode`, `$sha256`, `$md5`, `$hmacSha256`,
`$dateAdd`, `$dateSubtract`, `$formatDate`, `$parseDate`,
`$randomPassword`, `$randomBase64` — are now available. M13-002
added the ten `$faker.*` personal-data functions, M13-003 added the
twelve `$faker.*` location-data functions, M13-004 added the five
`$faker.*` company-data functions, M13-005 added the nine `$faker.*`
internet-data functions, M13-006 added the five `$faker.*`
content-data functions, and M13-007 adds the eight `$faker.*`
financial-data functions documented above.)

**Dotted-namespace function names (M13+).**

Dynamic-function names may contain dot-separated namespace segments —
each segment is `[a-zA-Z][a-zA-Z0-9_]*` and consecutive dots, leading
dots, and trailing dots are rejected at the parse step. The `$faker.*`
personal-data family (M13-002 — `$faker.firstName`, `$faker.email`,
`$faker.ssn`, etc.), the `$faker.*` location-data family (M13-003 —
`$faker.address`, `$faker.city`, `$faker.timezone`, etc.), the
`$faker.*` company-data family (M13-004 — `$faker.company`,
`$faker.jobTitle`, `$faker.catchPhrase`, etc.), the `$faker.*`
internet-data family (M13-005 — `$faker.url`, `$faker.ip`,
`$faker.userAgent`, etc.), the `$faker.*` content-data family
(M13-006 — `$faker.word`, `$faker.sentence`, `$faker.paragraph`,
etc.), and the `$faker.*` financial-data family (M13-007 —
`$faker.price`, `$faker.creditCard`, `$faker.iban`, etc.) use this
syntax, as do the `$faker.*` file-data functions (M13-008 —
`$faker.fileName`, `$faker.fileExtension`, `$faker.mimeType`,
`$faker.imageUrl`). Arbitrary third-party namespaces
are not yet supported. The captured name — including its dots — is
used verbatim as the registry lookup key, so `{{$faker.firstName}}`
and `{{$firstName}}` are independent functions.

### 3.8 Determinism with `--seed`

Random functions default to crypto-strength randomness. For tests you want to reproduce — bug reports, CI diffs, regression snapshots — pass a numeric seed:

```bash
curlew run collections/users.yaml --seed 42
```

With a seed, every random function (`$uuid`, `$randomInt`, `$randomEmail`, etc.) returns the same value every run. Timestamp functions (`$timestamp`, `$timestampMs`, `$isoTimestamp`) are **never** seeded — they always reflect the real wall clock.

### 3.9 External request files

As collections grow, inline request definitions become unwieldy. You can extract a single request to its own file and reference it by path:

```yaml
# collections/users.yaml
name: User flow
requests:
  - path: ../requests/login.yaml
  - path: ../requests/fetch-profile.yaml
```

```yaml
# requests/login.yaml
name: Login
request:
  method: POST
  url: "{{base_url}}/login"
  body:
    username: "{{user}}"
    password: "{{pass}}"
assertions:
  status: 200
extract:
  auth_token: "$.token"
```

Paths are relative to the collection file's directory. You cannot set both `path:` and `request:` on the same item — pick one.

Circular path references (request A includes B includes A) are detected and rejected at parse time.

**When to extract:** when a request is long (>15 lines), when it is reused across multiple collections, or when it belongs to a distinct domain (auth, user flows, billing). Small one-off requests are cleaner inline.

---

### 3.10 Expression Language (CEL)

Curlew embeds Google's [Common Expression Language](https://github.com/google/cel-spec) as a strategic escape hatch for predicates and invariants that outgrow the operator catalogue. CEL is parse-checked and type-checked at `curlew validate` time so authoring errors fail before any HTTP request is sent.

#### Where CEL appears

CEL appears at two sites:

| Site                   | Type    | Example                                                                                         |
| ---------------------- | ------- | ----------------------------------------------------------------------------------------------- |
| `if: <expr>`           | `bool`  | `if: 'vars.active && previous.body.status == 200'`                                              |
| `assertions: - cel:`   | `bool`  | `assertions:`<br>`  cel:`<br>`    - 'response.body.items.size() == response.body.total'`        |

`extract:` intentionally remains JSONPath-only — there is exactly one extraction language.

#### Standard activation

Every CEL expression evaluates against the same four bindings:

| Binding    | Type                          | Description                                                                             |
| ---------- | ----------------------------- | --------------------------------------------------------------------------------------- |
| `response` | `map(string, dyn)`            | Current request's response; fields `status` (int), `headers` (map), `body` (dyn).      |
| `previous` | `map(string, dyn)`            | The previous request's response, same shape as `response`.                              |
| `vars`     | `map(string, dyn)`            | All variables in scope at the request site (collection, env, CLI).                      |
| `env`      | `map(string, string)`         | Process environment variables.                                                          |

#### When to use CEL vs operators

CEL is the second rung of the scripting ladder — reach for it only when operator assertions cannot express what you mean.

| Use operator assertions when…                         | Use CEL when…                                                          |
| ----------------------------------------------------- | ---------------------------------------------------------------------- |
| Comparing a single JSONPath to a literal              | Comparing two JSONPaths to each other                                  |
| Checking a status code or header                      | Asserting a cross-field invariant (`total == items.size()`)            |
| Checking a value's type or existence                  | Aggregating over a list (`items.all(i, i.price > 0)`)                  |
| Bounded numeric ranges                                | Predicates spanning `response`, `previous`, `vars` simultaneously      |

If you find yourself writing the same CEL expression in more than three collections, it belongs in a [plugin](#part-10--plugins) (rung 3 of the ladder) rather than inline.

#### Disabled functions

These cel-go functions are blocked at parse time because they make tests non-deterministic:

| Function                 | Why blocked                                                            |
| ------------------------ | ---------------------------------------------------------------------- |
| `now()`                  | Wall-clock time at evaluation; would break replay determinism.         |
| `timestamp()` (zero arg) | Same as `now()` — returns the current instant.                        |

Use `vars.now` (set explicitly by your test or by `--seed`) when an expression needs a fixed time reference.

#### Validation error codes

Both error codes are surfaced by `curlew validate` with the precise field path and a source excerpt (truncated to 200 runes followed by `…`):

| Code             | Meaning                                                                                        |
| ---------------- | ---------------------------------------------------------------------------------------------- |
| `ERR_CEL_PARSE`  | The expression is syntactically invalid or references a disabled function.                     |
| `ERR_CEL_TYPE`   | The expression compiles but its result type is not `bool`; the message names the actual type.  |

Example output for a non-bool `if:` and an invalid `assertions: - cel:`:

```
requests[0].if: ERR_CEL_TYPE: got int, expected bool (source: 1 + 2)
requests[0].assertions[0].cel: ERR_CEL_PARSE: <inner cel-go message> (source: response.status ==)
```

`curlew validate` performs no HTTP requests; CEL is parse-checked and type-checked offline.

---

## Part 4 — Running Tests in CI

Once your tests pass locally, you want them running in CI on every pull request. This part covers output formats, exit codes, logging, and the paste-ready recipes for the most common CI systems.

### 4.1 Output formats

Five output formats for the `run` command, selected with `--format`:

| Format | Intended for | Flag | Notes |
|---|---|---|---|
| `terminal` | humans | (default) | Colored, symbols, summary |
| `json` | machines, post-processing | `--format json` | Stable schema |
| `tap` | TAP consumers (prove, tappy) | `--format tap` | TAP version 13 |
| `junit` | CI test reporters | `--format junit` | TAP-compatible JUnit XML |
| `html` | shareable reports | `--format html --report file.html` | Self-contained single file |

**Side-by-side on the same collection:**

Terminal (default):

```
Sample Collection
  ✓ Hello World (234ms)
  ✗ Create Widget (512ms)
      status | expected 201 | got 400
  ⊘ Delete Widget (skipped: required request "Create Widget" failed)

1 passed, 1 failed, 1 skipped — 746ms
```

JSON (abbreviated):

```json
{
  "name": "Sample Collection",
  "status": "failed",
  "duration_ms": 746,
  "summary": {"total": 3, "passed": 1, "failed": 1, "skipped": 1},
  "requests": [
    {"name": "Hello World", "status": "passed", "status_code": 200, "duration_ms": 234, "assertions": [...]},
    {"name": "Create Widget", "status": "failed", "status_code": 400, "duration_ms": 512, "assertions": [...]},
    {"name": "Delete Widget", "status": "skipped", "skip_reason": "required request \"Create Widget\" failed"}
  ]
}
```

The root `summary` block is always present. Quick pass/fail check without traversing `requests[]`:

```bash
curlew run collections/api.yaml --format json | jq '.summary.failed == 0'
```

**Data-driven JSON:**

```json
{
  "data_driven": [
    {
      "type": "data_driven",
      "name": "Create User",
      "total_iterations": 3,
      "passed_iterations": 3,
      "failed_iterations": 0,
      "total_duration_ms": 600,
      "average_duration_ms": 200,
      "iterations": [
        {"name": "Create User [1/3]", "status": "passed", "duration_ms": 100, "data_columns": {"name": "alice"}},
        {"name": "Create User [2/3]", "status": "passed", "duration_ms": 200, "data_columns": {"name": "bob"}},
        {"name": "Create User [3/3]", "status": "passed", "duration_ms": 300, "data_columns": {"name": "charlie"}}
      ]
    }
  ]
}
```

`iterations[].status` is `"passed"`, `"failed"`, or `"skipped"`. `data_columns` is omitted when the row has no column data. Invariant: `len(iterations) == total_iterations`.

TAP:

```
TAP version 13
1..3
ok 1 - Hello World (234ms)
not ok 2 - Create Widget
  ---
  failures:
    - type: "status"
      expected: "201"
      actual: "400"
  ...
ok 3 - Delete Widget # SKIP required request "Create Widget" failed
# Summary: 1 passed, 1 failed
```

**Data-driven TAP:**

```
TAP version 13
1..3
# Data-Driven: Create User (3 iterations)
ok 1 - Create User [1/3] (50ms)
ok 2 - Create User [2/3] (60ms)
ok 3 - Create User [3/3] (55ms)
# Summary: 3 passed, 0 failed
```

The plan count reflects the total iteration test-point count. Each
iteration's test-point name uses the `BaseName [i/N]` convention shared
with the events stream and JSON output.

**Parallel TAP** (with `--parallel`):

```
TAP version 13
1..3
# Wave 1
ok 1 - First (40ms)
ok 2 - Second (35ms)
# Wave 2
ok 3 - Third (60ms)
# Parallel execution:
  ---
  speedup_factor: 1.5
  wave_count: 2
  max_parallelism: 2
  ...
# Summary: 3 passed, 0 failed
```

`speedup_factor` matches `jq '.parallel_execution.speedup_factor'` from
the equivalent `--format json` invocation. The block is omitted on
sequential and single-wave runs. (M11-003)

JUnit XML (trimmed):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="Sample Collection" tests="3" failures="1" errors="0" skipped="1" time="0.746">
    <testcase name="Hello World" classname="Sample Collection" time="0.234"/>
    <testcase name="Create Widget" classname="Sample Collection" time="0.512">
      <failure message="status assertion failed" type="assertion">
        Expected: 201
        Actual: 400
      </failure>
    </testcase>
    <testcase name="Delete Widget" classname="Sample Collection" time="0.0">
      <skipped message="required request Create Widget failed"/>
    </testcase>
  </testsuite>
</testsuites>
```

HTML reports are single-file, self-contained HTML you can open in a browser or attach to a build artifact.

### 4.1a Markdown response files

`--format markdown --report <dir>` is a sixth output format aimed at humans
reading per-request results in a VS Code split pane and at AI agents that
need to narrate runs. It writes one markdown file per main-phase request
plus an index — large response bodies render properly, files diff cleanly
between runs, and three correlation IDs (`run_id`, `request_id`,
`request_slug`) link each file back to its events-stream and JSONL
counterparts.

**Layout.**

```
responses/
├── run.md                 ← index: env, summary, links to per-request files
├── hello-world.md         ← one file per main request
├── create-widget.md
└── create-user/           ← data-driven requests get a subdirectory
    ├── index.md           ← iteration manifest with summary counts
    ├── iter-1.md
    ├── iter-2.md
    └── iter-3.md
```

`run.md` lists the run summary and links to every per-request file.
Parallel collections group those links under `## Wave <N>` headers in
ascending wave order. Slugs come from the request name with a slug
sanitizer (lowercase, dashes, ASCII).

**Per-request file structure.**

Each file has two regions: an agent- or human-owned region (free notes, a
title you wrote, observations) and a CLI-owned region delimited by HTML
comment sentinels:

```markdown
# Get user

## Notes
<!-- write whatever you want here; it survives every re-run -->

<!-- BEGIN curlew:response id=req-3 slug=get-user run=abc123def456... -->
... ten-section CLI-owned block ...
<!-- END curlew:response id=req-3 slug=get-user run=abc123def456... -->

## Analysis
<!-- write here too -->
```

The opening and closing sentinels carry all three IDs. On every re-run,
Curlew rewrites only the bytes between the sentinels — your notes
survive byte-for-byte.

**Ten sections (always present, fixed order).** Title, Request, Request
body, Response, Response metadata, Response body, Timing, Assertions,
Errors, Trailer. No conditional sections; empty sections render as a
heading plus a placeholder line so diffs stay structural.

**Splice rules.** Six cases:

1. New file: write full content.
2. Existing file with matching sentinel pair: replace bytes between sentinels.
3. Existing file with mismatched IDs: write to `<slug>.md.new`, warn on stderr, leave the original untouched.
4. Existing file with malformed (unbalanced) sentinels: write to `.md.new`, warn.
5. Existing file with no sentinels (entirely user-owned): write to `.md.new`, warn.
6. Atomic write: `O_EXCL` temp-file + rename, no partial writes on disk full or `Ctrl+C`.

**Content-type matrix.**

| Body type | Rendering |
|---|---|
| `application/json`, `*/json` | Pretty-printed in a `json` fenced block (2-space indent). |
| `application/yaml`, `*/yaml` | Re-serialised with stable key order in a `yaml` block. |
| `application/xml`, `text/xml` | Indented in an `xml` block. |
| `text/html` | Raw HTML preserved verbatim in a `html` fenced block — never executed. |
| `text/*` (plain) | Verbatim in a `text` block. |
| `application/octet-stream`, `image/*`, etc. | First 256 bytes as `hex.Dump` plus total byte count. |
| HEAD response | `(no body)`. |
| Empty body | `(empty)`. |

**1 MiB body cap.** Response bodies larger than 1,048,576 bytes are
truncated *after* redaction; a truncation marker line records the
original size. The post-redaction order is deliberate — secrets cannot
leak through size-based corner cases.

**Redaction is always on.** `--allow-sensitive` does *not* affect markdown
output. That flag only disables redaction for `-vv` terminal dumps. The
markdown formatter is the canonical record you check into git or share
with a teammate; it is always safe.

**Determinism.** Given the same inputs, the CLI-owned block is
byte-identical across runs except for explicitly volatile lines
(`duration_ms`, `wave_index`, `started_at`). Volatile lines have a fixed
prefix so a `git diff` mask is one regex.

**See also:** §5.7 for the watch-mode workflow and §4.9 for using
markdown files with an AI agent; `docs/CLI_SPECIFICATION.md` §16 for
the formal output contract.

### 4.2 Streams, verbosity, and color

**Stream discipline.** stdout and stderr carry disjoint payloads. stdout
carries only the `--format` payload — the bytes you would pipe into `jq`,
a TAP harness, a JUnit aggregator, or a markdown renderer. stderr
carries diagnostics, progress indicators, warnings, the help synopsis
on flag-parse errors, and one-line notices such as the `--only`
minimal-setup fallback message. The split is enforced regardless of
TTY state, so this works exactly as you expect:

```bash
# stdout: clean JSON for jq.
# stderr: progress + summary, visible on the terminal.
curlew run collections/users.yaml --format json | jq '.summary.failed'
```

If something appears on the wrong stream, that's a bug — file an issue.

**Verbosity.**

| Flag | Shows |
|---|---|
| `-q`, `--quiet` | Summary line only |
| (default) | Pass/fail per request |
| `-v` | Request headers + response headers + status |
| `-vv` | Full request and response dump (including bodies) |

Verbosity affects the terminal format only. Machine formats
(`json`/`tap`/`junit`/`markdown`/`html`) have their own well-defined
shapes that do not change with `-v` / `-vv`.

**Color control.** `--color={auto|always|never}` governs ANSI escape
sequences. The default `auto` honours the [NO_COLOR](https://no-color.org)
environment variable and disables colour when stderr is not a TTY:

```bash
curlew run collections/users.yaml                    # auto: colour if stderr is a TTY
curlew run collections/users.yaml --color=never      # plain text
curlew run collections/users.yaml --color=always     # force on (pipe to less -R)
NO_COLOR=1 curlew run collections/users.yaml        # any non-empty value disables
curlew run collections/users.yaml --no-color        # alias for --color=never
```

Colour is **never** emitted on stdout when `--format` is non-terminal
(`json`/`tap`/`junit`/`markdown`/`html`), regardless of TTY or `--color`
value. A piped consumer never sees escape codes in its payload. Colour
on stderr follows the TTY/`NO_COLOR` rules independently of stdout's
format.

### 4.2b Running a Single Request (--only)

Use `--only "<name>"` to run exactly one main request by name. Setup and teardown phases still run in full, ensuring auth and cleanup happen as usual.

```bash
# Run only the "Get user" request
curlew run collections/users.yaml --only "Get user"

# Run two specific requests (union)
curlew run collections/users.yaml --only "Get user" --only "Update user"

# Combine with --events to record the selection in the stream
curlew run collections/users.yaml --only "Get user" --events run.ndjson
```

The match is exact and case-sensitive. Surrounding whitespace in the value is trimmed. Repeating `--only` forms a union — all named requests run, in their original collection order.

**No match → exit 3 before any HTTP.** When no main request matches the provided names, `curlew run` prints the available names to stderr and exits immediately:

```
Error: no request named "Nope"; available: "Get user", "Update user", "Delete user"
```

This is deliberately reported as exit 3 (config error) because it means the caller's argument was wrong — not that a test failed.

**Variable-cliff diagnostic.** If a selected request depends on a variable extracted by a request you did not include in `--only`, the error message names the filtered producer and explains what to do:

```
Error: variable {{user_id}} is not defined; normally extracted from "Get user"
       which was not included by --only
Hint:  Add the producer to --only (e.g. --only "Get user" --only "Update user")
       or pass the variable explicitly via --var user_id=<value>
```

**Setup names do not work with --only.** `--only` targets main-phase requests only. Passing a setup or teardown name as the value yields the no-match error.

**Minimal setup.** By default `--only` runs only the setup items whose extracted
variables are transitively referenced by the selected request. Setup items with
no `extract:` block are treated as pure seeders and always run. This speeds up
the inner loop when your setup phase is large but the request under test only
needs a subset.

Example: given setup `[Login (extract token), Seed users (extract user_id), Seed posts (uses {{user_id}}, extract post_id), Warm cache (no extract)]` and `--only "Get user"` (which uses `{{token}}`), only `Login` and `Warm cache` are executed during setup — `Seed users` and `Seed posts` are skipped.

If curlew cannot analyse the dependency graph (e.g. two setup items extract the
same variable in parallel), it falls back to running the full setup and prints a
one-line warning on stderr:

```
curlew: --only minimal-setup analysis failed (<reason>); running full setup
```

**In watch mode.** `curlew watch collections/users.yaml --only "Get user"` works identically — the minimal-setup pruning applies on every file-change trigger.

### 4.3 Exit codes — master table

This is the single source. Every inline exit-code reference elsewhere in the manual points here.

| Code | Meaning |
|---|---|
| `0` | Every assertion passed |
| `1` | Assertion failure; or a top-level usage error — unknown command, unknown flag, missing argument, invalid flag value |
| `2` | A safety guard tripped (a data set over 10,000 rows without `--confirm-large-dataset`); or a usage error in `perf` or `pr-check` |
| `3` | Parse or config error (file not found, YAML invalid, circular references, missing required fields, `--only` name not found, duplicate request names) |
| `4` | Execution error (network failure, connection refused, TLS error) |
| `5` | Variable resolution error (undefined variable, circular variable, bad interpolation) |
| `130` | SIGINT — Ctrl+C during `perf` run |

The distinction between `1` (assertions) and `3` (setup) matters for CI: a `3` means your tests didn't even start, usually a broken YAML or missing file, and should be treated as a pipeline configuration problem, not a product regression.

**A note on usage errors.** They do not all map to one code. `curlew run` with no argument, an unknown command, an unknown flag, and an invalid `--format` value all exit `1`; `perf` and `pr-check` usage errors exit `2`. That split is recorded here as the binary's observed behaviour rather than as a design worth copying — see `docs/CLI_SPECIFICATION.md` §17. If you are gating a pipeline, treat both as "fix the invocation".

### 4.4 Redaction of secrets

By default, Curlew aggressively hides values that look sensitive:

- Variables marked `!sensitive` in `.env` or in YAML (`!sensitive token: "abc"`).
- `from_command` variables with `sensitive: true`.
- Extracted values declared `sensitive: true` in the object form of `extract:` (§2.4).
- Vault-resolved values.
- Values for variables whose names match the heuristic — any of `password`, `token`, `secret`, `key`, `auth`, or `credential` in the name.

Redaction shows up as `[REDACTED]` in terminal output, JSON output, TAP output, JUnit, HTML, Markdown, event streams, and JSONL logs.

**It does not matter which direction the value travelled.** A secret is hidden where you sent it *and* where the server sent it back — in the request URL, in request and response headers, in either body, and in the failure message of an assertion:

```
✗ body $.authorization equals: expected …, got Bearer [REDACTED]
```

That last one matters most: an API that echoes your token, a `Set-Cookie` carrying a session, or a redirect with a token in its query would otherwise put the secret straight into a CI log. Response headers that are credentials by nature — `Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, `X-Auth-Token`, `Proxy-Authorization` — are hidden whole, since nothing on your side registered their values.

If you need to see plain values (debugging a local run only — never in CI):

```bash
curlew run collections/users.yaml --allow-sensitive
```

The flag is per-run; there is no global disable. If you set it in a CI pipeline, the CI logs will contain your secrets. Don't.

### 4.5 JSONL logging

For post-hoc inspection or central log aggregation, write one JSON object per request to a file:

```bash
curlew run collections/users.yaml --log runs.jsonl
```

Each line has this shape:

```json
{"timestamp":"2026-04-21T14:30:45Z","method":"POST","url":"https://api.example.com/users","status_code":201,"duration_ms":234,"run_id":"0123456789abcdef0123456789abcdef","request_id":"req-1"}
```

`run_id` (32-char lowercase hex, minted per `curlew exec` invocation) and
`request_id` (`"req-1"` for the single-request exec command) correlate the
JSONL entry with the same identifiers in the `--events` NDJSON stream and the
`--format markdown` sentinel block. Both fields are emitted only by `curlew exec`
today; older log files written before this version omit them.

JSONL is easy to grep and feed into `jq`:

```bash
# Find slow requests
jq 'select(.duration_ms > 1000)' runs.jsonl

# Tally status codes
jq -r .status_code runs.jsonl | sort | uniq -c

# Errors only
jq 'select(.error != "")' runs.jsonl
```

### 4.5a Event stream (`--events`)

`--events <file>` writes a structured NDJSON stream alongside whatever
`--format` produces. One JSON object per line, one line per event,
schema-versioned. The audience is AI agents and tooling that needs
machine-readable, real-time visibility into what happened during a run —
which assertion failed, which file/line declared it, what the response
status was, what got redacted.

**Event kinds.** `run.start`, `request.start`, `assertion.result`,
`request.end`, `run.error`, `run.end`. Every event carries
`schema_version: "1.3"` and `run_id`. Per-request events also carry
`request_id` (matches `run --only` selection IDs and the `--log` JSONL
`request_id`) and `request_slug` (matches the `<slug>.md` filename
written by `--format markdown`). The `run.start` event carries an
optional `selection` field listing the `--only` values for the run.

A request.end line looks like:

```json
{"schema_version":"1.3","kind":"request.end","run_id":"abc123def456...","request_id":"req-3","request_slug":"get-user","status":200,"duration_ms":234,"wave_index":1}
```

Failed assertions surface a structured error block with
`category`/`code`/`file`/`line`/`hint`. Bodies are subject to the same
redaction the markdown and JSONL formats apply, and CLI args sent in
the `run.start` event are redacted likewise.

**Cross-format correlation.** `run_id` is the same hex value across
`--events`, `--log`, and the markdown sentinel block; `request_id` and
`request_slug` link a specific event line to a specific markdown file
and a specific JSONL entry, so an agent can fan out from any fragment
back to the whole run.

For the full schema — every field, every enum, every ordering guarantee,
the v1.x stability policy — see [`docs/EVENTS_SCHEMA_v1.6.md`](EVENTS_SCHEMA_v1.6.md).

### 4.6 CI recipes

**GitHub Actions:**

```yaml
# .github/workflows/api-tests.yml
name: API tests
on: [push, pull_request]
jobs:
  curlew:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - run: go install github.com/weiqigod/curlew/cmd/curlew@latest
      - name: Run API tests
        env:
          API_KEY: ${{ secrets.API_KEY }}
        run: |
          curlew run 'collections/**/*.yaml' \
            --env staging \
            --env-var API_KEY \
            --format junit \
            --report test-results.xml
      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: test-results
          path: test-results.xml
```

**GitLab CI:**

```yaml
# .gitlab-ci.yml
api-tests:
  image: golang:1.24
  script:
    - go install github.com/weiqigod/curlew/cmd/curlew@latest
    - curlew run 'collections/**/*.yaml' --env staging --format junit --report test-results.xml
  artifacts:
    when: always
    reports:
      junit: test-results.xml
  variables:
    API_KEY: $API_KEY
```

**Jenkins declarative:**

```groovy
pipeline {
  agent any
  stages {
    stage('API tests') {
      steps {
        sh '''
          curlew run 'collections/**/*.yaml' \
            --env staging \
            --format junit \
            --report test-results.xml
        '''
      }
      post {
        always {
          junit 'test-results.xml'
        }
      }
    }
  }
}
```

All three rely on the same pattern: run Curlew with `--format junit --report <file>`, then let the CI's built-in JUnit integration handle the reporting.

### 4.7 Project utility commands

**`curlew validate <file|glob>`** — parse and validate collections without executing HTTP:

```bash
curlew validate collections/users.yaml
# collections/users.yaml: OK

curlew validate 'collections/**/*.yaml'
# collections/users.yaml: OK
# collections/billing.yaml: line 12: assertions.body.$.total: unknown operator "eq" (did you mean "equals"?)
```

Validate checks YAML syntax, required fields, operator names, circular references, and `include:` paths. It never makes an HTTP request.

It also warns about `{{variable}}` references it cannot account for statically. Names defined in the collection's own `variables:` block, in `curlew.yaml` (found by walking up from the collection's directory, as `curlew run` does), or passed via `--var`/`--env-var` are considered defined; a name that appears in none of those is reported as a warning, not an error, since environments and `.env` supply values at run time.

**`curlew info`** — project metadata:

```bash
curlew info
# Project: demo-api
# Root:    /Users/alice/demo-api
# Collections: 4 (users.yaml, billing.yaml, admin.yaml, flows.yaml)
# Environments: 3 (dev, staging, prod)

curlew info --format json
```

Useful in AI-agent scripting — feed the JSON output to the agent for it to orient itself.

**`curlew schema`** — emits the JSON Schema describing the collection YAML format. Pipe it into IDE tools that provide autocompletion and inline validation:

```bash
curlew schema > .curlew-schema.json
# Then point your editor's YAML schema config at it.
```

### 4.8 AI-agent commands

Curlew was designed for agents as well as humans. The relevant affordances:

**`curlew exec <url>`** — one-shot request, no collection file:

```bash
curlew exec https://api.example.com/health
# HTTP 200 — 45ms
```

With method, body, headers — or read the whole request from stdin as JSON:

```bash
echo '{"method":"POST","url":"https://api.example.com/users","body":{"name":"Demo"}}' | \
  curlew exec --stdin --format json
```

The stdin payload limit is 10 MB.

**`--non-interactive`** — suppresses any interactive prompts (of which there are currently few, but future vault auth flows may prompt).

**`--dry-run`** — parse and resolve, but do not send any HTTP request. For `run`, this is paired with `--show-dependencies` to get the parallel execution plan; for `exec`, it prints what would be sent.

**`--format json`** — every command that produces output has a machine-readable alternative. Pair with `--non-interactive` for fully automated use.

A typical agent pattern:

```bash
curlew info --format json                          # 1. orient
curlew validate 'collections/*.yaml' --format json # 2. lint
curlew run collections/users.yaml --format json \
  --non-interactive --dry-run --show-dependencies   # 3. preview
curlew run collections/users.yaml --format json \
  --non-interactive                                 # 4. execute
```

---

### 4.9 Driving curlew with an AI agent

Curlew ships a default Agent Skill that teaches a coding agent how to
invoke the CLI, where artifacts land, and how to narrate results. The
skill is opt-in via `curlew init --skill agent`.

The scaffolded file is a standard Agent Skill — a `SKILL.md` with `name`
and `description` frontmatter plus per-topic reference files — and it
lands in `.claude/skills/`, which is a project skill directory for both
Claude Code and GitHub Copilot. One file serves both; there is no
per-vendor variant to choose between. `--skill claude` is accepted as a
compatibility alias and scaffolds exactly the same project.

**Worked example.** Scaffold a new project with the skill enabled, open it
in your agent, and ask it to run a collection in natural language.

```bash
mkdir users-api && cd users-api
curlew init --skill agent
```

The following are created or modified in addition to the standard scaffold:

- `.claude/skills/curlew/SKILL.md` — the agent's playbook (new file)
- `curlew.yaml` — `output:` block defaults to `format: markdown` with
  `events: .curlew/run.ndjson` appended (modified)
- `.gitignore` — gains `.curlew/` (modified)

Open the directory in your agent — Claude Code (`claude` CLI or the
desktop app), or GitHub Copilot (coding agent, Copilot CLI, or VS Code
agent mode). Ask:

> Run the sample collection and tell me what happened.

The agent reads `.claude/skills/curlew/SKILL.md`, recognises the trigger
phrasing, and invokes:

```bash
curlew run collections/sample.yaml
```

No extra flags. The `output:` block in `curlew.yaml` declares everything:
markdown reports land in `responses/`, the event stream lands in
`.curlew/run.ndjson`. After the run, the agent reads `responses/run.md`
first (summary), then drills into `responses/<slug>.md` for any failing
request, then `.curlew/run.ndjson` for structured error details. It
narrates with file paths, never paraphrasing response bodies.

If you ask it to fix a failing assertion, it edits the YAML — never a
one-off curl command. Re-run with `curlew watch collections/sample.yaml
--only "<request name>"` for tight inner-loop iteration; the agent reads
the re-spliced markdown on every save.

**Customise the skill.** The skill is a checked-in file. Edit
`.claude/skills/curlew/SKILL.md` for your team's conventions — preferred
environments, project-specific triggers, narration tone, additional
playbook entries. Subsequent `curlew init --skill agent` runs do not
overwrite a pre-existing skill file.

**See also.** §3.6.1 (the `output:` block precedence ladder), §4.3 (exit
codes), §4.5 (the events stream), and `docs/history/IMPROVEMENT.md` §3.2
(archived design rationale for the human-agent-CLI workflow).

---

## Part 5 — Test Authoring at Scale

Larger projects bring their own problems: shared setup, cross-file composition, flaky networks, lots of rows of test data, independent requests that could be parallel, and tight inner-loop iteration. Part 5 covers the full machinery.

### 5.1 Setup and teardown phases

A collection can have three phases: `setup`, `requests`, and `teardown`. Each is a list of request items. They run in order: setup, then main, then teardown.

```yaml
name: Order flow
description: Create an account, run the flow, clean up.

setup:
  - name: Create test account
    request:
      method: POST
      url: "{{base_url}}/admin/accounts"
      body: { email: "test@example.com" }
    assertions:
      status: 201
    extract:
      account_id: "$.id"

requests:
  - name: Place order
    request:
      method: POST
      url: "{{base_url}}/accounts/{{account_id}}/orders"
      body: { sku: "WIDGET-1" }
    assertions:
      status: 201

teardown:
  - name: Delete test account
    request:
      method: DELETE
      url: "{{base_url}}/admin/accounts/{{account_id}}"
    assertions:
      status: 204
```

Semantics:

- **Failure of a setup request fails the collection** and skips the main phase (but teardown still runs, so you can clean up).
- Failure of a main request only affects other main requests (per `required:` semantics, §2.5).
- Failure of a teardown request is reported but does not change the collection's pass/fail outcome.
- Variables extracted in setup flow into main and teardown.
- Variables extracted in main flow into teardown.

### 5.2 Composition with `include:`

Share request blocks across collections by splicing in other YAML files:

```yaml
# collections/smoke.yaml
name: Smoke tests
include:
  - shared/auth-flow.yaml
  - shared/health-checks.yaml

requests:
  - name: My specific smoke request
    request: { method: GET, url: "{{base_url}}/smoke" }
    assertions: { status: 200 }
```

The included files are appended (setup to setup, requests to requests, teardown to teardown) in order.

### 5.3 Collection-level defaults

A few knobs let you set behavior once per collection:

```yaml
name: API tests

rate_limit_rps: 10                  # global pacing

retry:                              # default retry for every request
  enabled: true
  max_attempts: 3
  initial_delay_ms: 500
  backoff_strategy: exponential

options:
  stop_on_failure: false            # stop immediately on any failure (vs. the default: continue)

requests:
  - name: ...
```

`rate_limit_rps` applies a token bucket across the whole collection. `retry` defaults can be overridden per-request.

Project-level defaults live in `curlew.yaml` under `defaults:` and inherit into every collection:

```yaml
# curlew.yaml
defaults:
  retry:
    enabled: true
    max_attempts: 2
    initial_delay_ms: 300
```

### 5.4 Retry logic

Networks and rate limiters fail in predictable, retryable ways. Curlew's retry logic targets those: transient 5xx, 429 throttling, connection resets, timeouts.

**Default behavior.** When you enable retry without further config, Curlew retries on `[429, 502, 503, 504]` for idempotent methods (GET, HEAD, PUT, DELETE, OPTIONS) up to three attempts with exponential backoff starting at 500ms.

**Full config, annotated:**

```yaml
- name: Fetch user
  request:
    method: GET
    url: "{{base_url}}/users/{{user_id}}"
  retry:
    enabled: true
    max_attempts: 5                 # 1 initial + 4 retries
    initial_delay_ms: 500           # first retry waits 500ms
    max_delay_ms: 30000             # cap for exponential/linear growth
    backoff_strategy: exponential   # exponential | linear | constant
    jitter: true
    jitter_factor: 0.2              # delay *= 1 + jitter_factor * random(-1, 1)
    respect_retry_after: true       # honor server's Retry-After header when present
    retry_on:
      status_codes: [429, 502, 503, 504]
      status_ranges: ["5xx"]        # "Nxx" shorthand or explicit "500-599"
      network_errors: true          # connection refused, DNS failure, reset
      timeouts: true
      methods: [GET, PUT, DELETE]   # only retry for these methods
    do_not_retry_on:
      status_codes: [401, 403]      # never retry — these are config errors
      methods: [POST]               # extra safety: don't retry POST
  assertions:
    status: 200
```

**Status ranges** accept the explicit `"min-max"` form (`"500-599"`) and the class shorthand `"Nxx"` (`"5xx"`, case-insensitive). A range that parses as neither is a validation error — `curlew validate` reports it rather than silently skipping it.

**Delay sequence, exponential, initial 500ms, cap 30000ms:**

| Attempt | Computed delay (no jitter) |
|---|---|
| 1 (first retry) | 500 ms |
| 2 | 1000 ms |
| 3 | 2000 ms |
| 4 | 4000 ms |
| 5 | 8000 ms |
| 6 | 16000 ms |
| 7 | 30000 ms (capped) |

With jitter (`jitter_factor: 0.2`), each delay is multiplied by `1 + 0.2 * random(-1, 1)` — so attempt 3 (2000 ms) runs anywhere in `[1600, 2400]` ms. Jitter reduces thundering-herd retry storms.

**Retry-After.** When the server sends a `Retry-After` header (either integer seconds `120` or HTTP-date `Wed, 21 Oct 2024 07:28:00 GMT`), Curlew honors it — up to a 30-second cap — overriding the computed backoff. Set `respect_retry_after: false` to ignore it.

**Backoff strategies:**

- `exponential` (default): `initial_delay_ms * 2^attempt`
- `linear`: `initial_delay_ms * (attempt + 1)`
- `constant`: always `initial_delay_ms`

**Idempotency warning.** POST and PATCH are not idempotent. Retrying a POST that succeeded server-side but failed client-side (response dropped) can create duplicates. Curlew warns when you enable retry for non-idempotent methods; either restrict retries with `methods:`, or put `POST` in `do_not_retry_on.methods`, or ensure your endpoint is idempotent (e.g., via an idempotency key).

### 5.5 Data-driven testing

Run the same request once per row of a CSV/JSON/YAML file. Columns become variables.

**CSV data file:**

```csv
user_id,email,role,age
123,alice@example.com,admin,30
456,bob@example.com,user,25
789,carol@example.com,viewer,35
```

**Collection:**

```yaml
requests:
  - name: Create user
    data_driven:
      source: "./data/users.csv"
      filter: "{{age}} >= 25"           # expression evaluated per row (after interpolation)
      limit: 100                        # max rows after filter
      start_row: 0                      # 0-based inclusive
      end_row: 99                       # 0-based inclusive
      fail_fast: false                  # stop at first failure? default false
      parallel: false                   # run iterations concurrently? default false
      rate_limit_rps: 5                 # pace iterations (0 = unlimited)
      store_results: all                # all | summary | failed_only
    request:
      method: POST
      url: "{{base_url}}/users"
      body:
        id: "{{user_id}}"
        email: "{{email}}"
        role: "{{role}}"
    assertions:
      status: 201
```

**JSON data file:**

```json
[
  {"sku": "ABC-001", "name": "Widget", "price": "29.99"},
  {"sku": "ABC-002", "name": "Gadget", "price": "49.99"}
]
```

**YAML data file:**

```yaml
- sku: "ABC-001"
  name: "Widget"
  price: "29.99"
- sku: "ABC-002"
  name: "Gadget"
  price: "49.99"
```

**Built-in iteration variables** are available inside the request for the current row:

| Variable | Meaning |
|---|---|
| `{{_index}}` | zero-based row index |
| `{{_count}}` | one-based row number |
| `{{_total}}` | total rows that will run |

Plus every column name in the data file.

**Large dataset guard.** To protect you from accidentally blasting a production API with 50k iterations, data files with more than 10,000 rows require an explicit confirmation flag:

```bash
curlew run collections/users.yaml --confirm-large-dataset
```

Without it, the run exits with code 2.

**Parallel data-driven.** Setting `parallel: true` runs iterations concurrently with up to 20 workers. `rate_limit_rps` still applies and is enforced across workers.

### 5.6 Parallel execution

For collections where requests are independent of each other, Curlew can run them concurrently. You declare dependencies; Curlew computes the execution waves.

**Declaring dependencies:**

```yaml
requests:
  - name: Create user
    request: { method: POST, url: "{{base_url}}/users", body: { name: "A" } }
    extract:
      user_id: "$.id"

  - name: Create product
    request: { method: POST, url: "{{base_url}}/products", body: { name: "Widget" } }
    extract:
      product_id: "$.id"

  - name: Place order
    depends_on: [Create user, Create product]
    request:
      method: POST
      url: "{{base_url}}/orders"
      body:
        user_id: "{{user_id}}"
        product_id: "{{product_id}}"
```

Run with `--parallel`:

```bash
curlew run collections/order-flow.yaml --parallel
```

Curlew computes waves by topological sort:

```
Wave 1: Create user  |  Create product      (both run concurrently)
Wave 2: Place order                         (waits for both of Wave 1)
```

Variables extracted in Wave 1 propagate to Wave 2. Cycles (A depends on B, B depends on A) are detected during dependency analysis and rejected with a clear error before any request executes.

**Preview without executing:**

```bash
curlew run collections/order-flow.yaml --show-dependencies
# Wave 1: Create user, Create product
# Wave 2: Place order
```

Or combined with `--dry-run` for the full plan without any HTTP:

```bash
curlew run collections/order-flow.yaml --show-dependencies --dry-run
```

**Without `--parallel`**, requests run sequentially regardless of `depends_on`. The flag is the opt-in.

### 5.7 Watch mode

For TDD-style inner loops where you're editing a collection and running it repeatedly:

```bash
curlew watch collections/users.yaml
```

Curlew runs the collection once, then re-runs it every time the file changes. Pass `--clear` to clear the terminal between runs:

```bash
curlew watch collections/users.yaml --clear
```

All of the run flags (`--env`, `--var`, `--format`, verbosity) work with watch. Press Ctrl+C to exit.

**Worked example: VS Code split-pane workflow.**

The markdown output format (§4.1a; `--format markdown --report responses/`)
pairs naturally with watch mode for tight inner-loop iteration. Open
your collection YAML in VS Code, then open the per-request markdown file
in a right-side split:

1. Scaffold a markdown-ready project: `curlew init --output markdown`
2. Open `collections/sample.yaml` on the left. Hit `Ctrl+\` (or `Cmd+\`)
   to split the editor, then open `responses/hello-world.md` on the right.
3. Run `curlew watch collections/sample.yaml --only "Hello World"`.

On every save of the YAML, Curlew re-runs that one request and rewrites
the bytes between the `BEGIN/END curlew:response` sentinels in the
markdown file. VS Code's markdown preview (`Ctrl+K V`) updates in place,
showing the formatted request, response (pretty-printed JSON, YAML, XML,
or hex preview for binary), assertion outcomes, and timing. Notes you add
above or below the sentinel block survive each re-run — useful for
recording observations as you iterate.

This works equally well when an AI agent (Claude Code, Copilot, etc.)
drives the loop: the agent edits the YAML, you watch the markdown updates
in real time and intervene when needed.

### 5.8 Dry run

`--dry-run` parses the collection, resolves variables, but skips HTTP execution:

```bash
curlew run collections/users.yaml --dry-run
```

Useful for:

- Confirming variable resolution — you'll see a variable error (exit 5) before any network traffic.
- Previewing URLs and bodies that would be sent.
- Combining with `--show-dependencies` to get the full parallel plan.

Plugins (§10) still fire their `on_request` hook in dry run, since that's when mutations happen; `on_response` and `on_result` are skipped because no response occurred.

---

## Part 6 — Authentication and Secrets

Testing real APIs means handling real credentials. Curlew's stance: never store secrets; read them from your environment or an external vault on every run.

### 6.1 Mental model

Curlew never writes a secret to disk under any user-visible path. It supports four ways to supply secrets, in roughly increasing sophistication:

1. **OS environment** — you set env vars in your shell or CI, Curlew imports them (§3.5).
2. **`.env` file** — you commit `.env.example`, not `.env`; your own `.env` stays local (§3.4).
3. **`from_command`** — Curlew shells out to any command and captures its stdout as a variable (§6.3).
4. **Vault providers** — Curlew talks to AWS / Azure / HashiCorp / GCP / 1Password for you (§6.4).

You mix and match. A typical setup is: test data in a committed YAML, low-sensitivity tokens in `.env`, real production secrets in a vault.

### 6.2 `.env`, CLI, and OS imports (recap)

Already covered in Part 3. The 30-second recap:

```bash
# 1. .env file — local only
echo 'api_key=sk_test_abc' >> .env

# 2. CLI override — ephemeral
curlew run collections/users.yaml --var api_key=sk_test_abc

# 3. OS env import — CI-friendly
export API_KEY=sk_live_abc
curlew run collections/users.yaml --env-var API_KEY
```

Whichever form you use, mark the variable `!sensitive` or name it with a heuristic-matching prefix (`*_secret`, `*_token`, `*_password`) so Curlew redacts it in output (§4.4, §6.7).

### 6.3 `from_command` variables

When you want Curlew to pull a value from any command at run time:

```yaml
# curlew.yaml
variables:
  admin_token:
    from_command: "cat ~/.config/demo-api/admin-token"
    sensitive: true
    cache: 3600                     # cache result for 3600 seconds
```

Or in a collection:

```yaml
name: Admin tests
variables:
  admin_token:
    from_command: "op read 'op://Employee/demo-api/admin token'"
    sensitive: true
    cache: 300
```

Semantics:

- The command runs via `/bin/sh -c`, so shell features work (pipes, subshells, `&&`).
- Trailing newline is stripped from stdout.
- Non-zero exit terminates the run and includes stderr in the error.
- `sensitive: true` redacts the value in all output.
- `cache: N` — memoize for N seconds within the run scope.

The command runs through the same redaction and `sensitive: true` machinery as every other variable source.

### 6.4 Vault providers

For each supported vault, Curlew expects the provider's CLI to be installed and authenticated. Curlew does not embed SDKs; it shells out.

**AWS Secrets Manager:**

```yaml
# curlew.yaml
secrets:
  provider: aws-secrets-manager
  region: us-east-1
  keys:
    db_password: "prod/db#password"        # #password extracts the "password" field
    api_key: "prod/api-keys#main"
  cache_ttl: 300
  refresh_on_failure: true
```

Requires: `aws` CLI on PATH, authenticated with permission to `secretsmanager:GetSecretValue`.

**Azure Key Vault:**

```yaml
secrets:
  provider: azure-key-vault
  vault_name: "demo-api-kv"
  keys:
    db_password: "db-password"
    api_key: "api-key"
```

Requires: `az` CLI, `az login` completed.

**HashiCorp Vault:**

```yaml
secrets:
  provider: hashicorp-vault
  address: "https://vault.example.com:8200"
  auth:
    method: approle                      # approle | token
    role_id: "demo-api-role"
    secret_id: "${VAULT_SECRET_ID}"      # use OS env in your shell
  keys:
    db_password: "secret/data/prod/db#password"
  cache_ttl: 600
```

Or token auth:

```yaml
  auth:
    method: token
    token: "${VAULT_TOKEN}"
```

**GCP Secret Manager:**

```yaml
secrets:
  provider: gcp-secret-manager
  project: "demo-api-123456"
  keys:
    db_password: "prod-db-password"
    api_key: "prod-api-key"
```

Requires: `gcloud` CLI, authenticated with `secretmanager.secretAccessor` role.

**1Password:**

```yaml
secrets:
  provider: 1password
  keys:
    db_password: "op://Employee/demo-api/db-password"
    api_key: "op://Employee/demo-api/api-key#credential"
```

Requires: `op` CLI, signed in (`eval $(op signin)`).

**The `#field` suffix.** For providers whose secrets are structured (AWS, 1Password, HashiCorp), `path#field` extracts a single field; without `#field`, Curlew expects the value to be a plain string.

**Caching.** `cache_ttl` is a per-run cache. Subsequent requests in the same run reuse the fetched value. Set `refresh_on_failure: true` to re-fetch once on retrieval errors rather than aborting the run.

### 6.5 Dynamic auth profiles

For token-based auth flows where the test suite itself needs to log in, Curlew supports named auth profiles that each run a collection to produce an auth token.

**Define profiles in `curlew.yaml`:**

```yaml
auth_profiles:
  admin:
    type: dynamic
    collection: "auth/admin-login.yaml"
    extract: "access_token"
    cache_ttl: 600
    refresh_on_failure: true

  user:
    type: dynamic
    collection: "auth/user-login.yaml"
    extract: "token"
    cache_ttl: 300
```

**The auth collection** looks like any other collection. Its last request should extract the token:

```yaml
# auth/admin-login.yaml
name: Admin login
requests:
  - name: Login
    request:
      method: POST
      url: "{{base_url}}/auth/login"
      body:
        username: "{{admin_user}}"
        password: "{{admin_pass}}"
    assertions:
      status: 200
    extract:
      access_token: "$.access_token"
```

**Use the profile on any request:**

```yaml
# collections/admin-things.yaml
requests:
  - name: List users
    auth: admin                          # runs auth_profiles.admin first, then this request
    request:
      method: GET
      url: "{{base_url}}/admin/users"
      headers:
        Authorization: "Bearer {{access_token}}"
    assertions:
      status: 200
```

The auth collection runs before any request that declares `auth: admin`. The extracted value is cached for `cache_ttl` seconds in `.curlew/cache/` (obfuscated, never plain text). `refresh_on_failure: true` retries once on failure — useful for token expiry.

### 6.6 Shared vault templates

For teams with shared vault configurations, point Curlew at a team config file via `CURLEW_TEAM_CONFIG`:

```bash
export CURLEW_TEAM_CONFIG=~/team/curlew-team-config.yaml
curlew run collections/users.yaml --env staging
```

The team config declares named vault entries; collections reference them via `{{secrets.<name>}}`:

```yaml
# collections/users.yaml
requests:
  - name: Call admin endpoint
    request:
      method: GET
      url: "{{base_url}}/admin/metrics"
      headers:
        Authorization: "Bearer {{secrets.admin_jwt}}"
    assertions:
      status: 200
```

Team templates let one team own the vault configuration while individual collections stay clean of provider-specific detail.

For local testing and CI where you don't want to actually hit a vault, set `CURLEW_VAULT_STUB=1` and Curlew uses an in-memory stub.

### 6.7 The redaction contract

Restated here for anyone who landed in Part 6 via search. A value is redacted (shown as `[REDACTED]` in terminal, JSON, TAP, JSONL log output) when:

- It is declared `!sensitive` in `.env` or YAML.
- Its `from_command` entry has `sensitive: true`.
- It is a vault-resolved value.
- Its variable name contains one of `password`, `token`, `secret`, `key`, `auth`, or `credential`.

The only way to see a plain value is `--allow-sensitive` per run. That flag is meant for local debugging. Never set it in CI.

---

### 6.8 Request signing

The `signing:` field on a request (or as a collection-level default) applies a built-in signer to the outgoing request between variable templating and HTTP dispatch. Built-in signers require no plugin binary.

| Type        | Required params                                  | Optional params   | Summary |
|-------------|--------------------------------------------------|-------------------|---------|
| `aws-sigv4` | `region`, `service`, `access_key`, `secret_key`  | `session_token`                            | AWS Signature Version 4. Supports STS-issued credentials via `session_token`. |
| `oauth1`    | `consumer_key`, `consumer_secret`                | `token`, `token_secret`, `method`, `realm` | OAuth 1.0a. `method` defaults to `HMAC-SHA1`; `HMAC-SHA256` is also supported. RSA-SHA1 and PLAINTEXT are not supported. |

Example:

```yaml
requests:
  - name: list-buckets
    request:
      method: GET
      url: "https://s3.us-east-1.amazonaws.com/"
    signing:
      type: aws-sigv4
      params:
        region: us-east-1
        service: s3
        access_key: "{{aws_key}}"
        secret_key: "{{aws_secret}}"   # auto-redacted when aws_secret is sensitive
```

Example (OAuth 1.0a):

```yaml
requests:
  - name: oauth1-initiate
    request:
      method: POST
      url: "https://photos.example.net/initiate"
    signing:
      type: oauth1
      params:
        consumer_key: "{{consumer_key}}"
        consumer_secret: "{{consumer_secret}}"   # auto-redacted when consumer_secret is sensitive
        token: "{{access_token}}"
        token_secret: "{{access_token_secret}}"
        method: HMAC-SHA1                         # default; HMAC-SHA256 also supported
```

**Sensitive-secret footgun:** when `secret_key` is a literal string (no `{{var}}` reference), the resolved value is **not** added to the sensitive set — there is no source variable to back-trace. Always prefer variable references for credential params. This mirrors the `$hmacSha256` documentation in §3.7.

### 6.9 No account, no backend

Curlew is entirely local. There is no `curlew login`, no account, and no service it
reports to. The only network traffic it generates is the HTTP requests your collections
define.

Features that used to require an account are now local:

| Was | Now |
|---|---|
| Team vault fetched from the backend | `CURLEW_TEAM_CONFIG=<path>` reads a local template file (§6.10) |
| `curlew run --report-upload --org …` | `curlew run --format json --report <file>`, then `curlew pr-check --results <file>` (§8.1) |
| `curlew worker` / `run --workers` (distributed) | Removed. Use `--parallel` for concurrency within one process. |
| Telemetry posted to an ingest endpoint | `curlew telemetry` appends to a local NDJSON file, never transmitted (§8.2) |

---

## Part 7 — Beyond REST

Curlew is not only for JSON REST APIs. It speaks GraphQL and WebSocket as first-class protocols, and can import OpenAPI specs.

### 7.1 GraphQL

GraphQL requests share the same request envelope as REST, with a `protocol: graphql` marker and a `graphql:` block.

**Inline query:**

```yaml
requests:
  - name: List users
    request:
      protocol: graphql
      url: "{{base_url}}/graphql"
      graphql:
        query: |
          query ListUsers($limit: Int!) {
            users(first: $limit) {
              id
              name
              email
            }
          }
        variables:
          limit: 10
        error_handling: fail            # fail | warn | ignore
    assertions:
      status: 200
      body:
        $.data.users:
          type: array
        $.errors:
          not_exists: true
```

The method is automatically `POST`, and `Content-Type: application/json` is set. The body becomes `{"query": "...", "variables": {...}}`.

**External query file plus fragments:**

```yaml
    request:
      protocol: graphql
      url: "{{base_url}}/graphql"
      graphql:
        query_file: "graphql/list-users.graphql"
        fragments:
          - "graphql/fragments/user-fields.graphql"
          - "graphql/fragments/timestamps.graphql"
        variables:
          limit: 10
```

```graphql
# graphql/list-users.graphql
query ListUsers($limit: Int!) {
  users(first: $limit) {
    ...UserFields
    ...Timestamps
  }
}
```

```graphql
# graphql/fragments/user-fields.graphql
fragment UserFields on User {
  id
  name
  email
}
```

```graphql
# graphql/fragments/timestamps.graphql
fragment Timestamps on Node {
  createdAt
  updatedAt
}
```

Fragments are topologically ordered before being concatenated with the query, so fragments can reference other fragments. Circular fragment references and duplicate fragment names are rejected.

`query:` and `query_file:` are mutually exclusive on the same request.

**Error handling.** GraphQL servers can return a 200 with `errors` populated. Four outcome classes:

| Outcome | When | `fail` | `warn` | `ignore` |
|---|---|---|---|---|
| success | `data` non-null, no errors | pass | pass | pass |
| partial-success | `data` non-null, errors present | fail | warn | pass |
| full-failure | `data` null, errors present | fail | warn | pass |
| empty | no `data` and no `errors` | pass | pass | pass |

The default is `fail`. Set a project-wide default under `curlew.yaml`:

```yaml
defaults:
  graphql:
    error_handling:
      partial_success: warn
```

### 7.2 WebSocket

WebSocket testing is a sequence of steps — send a message, expect a response, wait, close — with assertions and extractions woven in.

**Minimal subscription flow:**

```yaml
requests:
  - name: Order updates
    request:
      protocol: websocket
      url: "wss://{{ws_host}}/stream"
      headers:
        Authorization: "Bearer {{auth_token}}"
    websocket:
      steps:
        - action: send
          message:
            type: "subscribe"
            channel: "orders"
        - action: expect
          timeout_ms: 5000
          message:
            $.type:
              equals: "subscribed"
        - action: expect
          timeout_ms: 10000
          count: 3                      # wait for 3 matching messages
          message:
            $.event:
              equals: "order_placed"
          extract:
            last_order_id: "$.order_id"
        - action: wait
          duration_ms: 500
        - action: close
          code: 1000                    # normal closure
          reason: "done"
```

**Step actions:**

| Action | Purpose | Key fields |
|---|---|---|
| `send` | Send one message | `message` (YAML map → JSON), `message_raw` (literal string), `message_template` (file with `{{vars}}`) |
| `expect` | Read N messages and assert | `timeout_ms`, `count`, `message` (JSONPath assertions), `any_of` (alternative assertion sets), `extract` |
| `wait` | Pause | `duration_ms` |
| `close` | Close the connection | `code` (default 1000), `reason` |

**Heartbeats and reconnect:**

```yaml
    websocket:
      heartbeat:
        enabled: true
        interval_ms: 30000
        message: { type: "ping" }
      reconnect:
        enabled: true
        max_attempts: 3
        initial_delay_ms: 1000
      steps:
        - ...
```

Heartbeat sends periodic keep-alive messages. Reconnect transparently reconnects on dropped connections and resumes the step sequence. Both are useful for long-running subscriptions.

### 7.3 OpenAPI import

If your API already has an OpenAPI 3.x spec, convert it to a starting-point collection:

```bash
curlew import openapi docs/openapi.yaml
# Wrote collections/imported.yaml (32 operations)
```

Or with an output path:

```bash
curlew import openapi docs/openapi.yaml --output collections/imported.yaml
```

The import emits:

- One request per OpenAPI operation (or per operation ID).
- Methods, paths, headers declared in the spec.
- Request body templates derived from request schemas.
- Default status assertion (expected response code from the spec).

The import does **not** fabricate:

- Auth credentials.
- Realistic request body values (you get placeholders).
- Environment-specific hostnames (the spec's `servers[0].url` is used as `base_url` and you'll likely want to move it to `curlew.yaml`).

Think of it as a faster starting point, not a finished test suite.

---

## Part 8 — CI Integration

Curlew's CI story is file-based: a run writes its results, and a second command turns
those results into a pass/fail verdict. Nothing is uploaded.

### 8.1 Gating CI on a results file

Write the results, then gate on them:

```bash
curlew run 'collections/**/*.yaml' --env staging --format json > test-results.json

curlew pr-check --results test-results.json --summary verdict.json
```

`pr-check` reads the output of `curlew run --format json` directly. (It also accepts the
flatter `collection_name` / `pass_count` payload shape, for results produced by other
tooling.)

`pr-check` exits 0 when every test passed, 1 when the results contain failures, and 2 on
a usage error or an unreadable results file — so it drops straight into a pipeline step.
`--summary <file>` writes the verdict as JSON:

```json
{
  "collection_name": "checkout-api",
  "state": "failure",
  "pass_count": 41,
  "fail_count": 2,
  "skipped_count": 0,
  "git_sha": "abc1234"
}
```

`--dry-run` prints that summary instead of writing it.

Publishing the verdict — a PR comment, a commit status, a dashboard — is your CI
system's job. `curlew` produces the artefacts; `gh`, `curl`, or your CI provider's
native step consumes them.

### 8.2 Local telemetry

`curlew telemetry` records anonymous usage events to a local NDJSON file. It is opt-in,
disabled by default, and has no transmission path — there is no endpoint to configure.

```bash
curlew telemetry enable       # generates an install_id, starts recording
curlew telemetry status       # shows state and the events file path
curlew telemetry export       # prints install_id + recent emissions as JSON
curlew telemetry delete       # removes install_id, state, and collected events
```

Events land in `telemetry.ndjson` inside curlew's config directory —
`~/.config/curlew` on Linux, `~/Library/Application Support/curlew` on macOS
(Go's `os.UserConfigDir`). `curlew telemetry --help` prints the resolved path
for your machine. Override the directory with `CURLEW_CONFIG_DIR`, or the file
itself with `CURLEW_TELEMETRY_FILE`. One JSON object per line:

```json
{"at":"2026-08-03T09:12:44Z","install_id":"…","event_type":"run.completed","event_payload":{"exit_code":0,"collection_size":12}}
```

Useful if you want to chart your own suite's duration or failure rate over time.

---

## Part 9 — Scale Features

### 9.1 Concurrency

Curlew runs in a single process. For large suites, `--parallel` executes independent
requests concurrently within the run, respecting the dependency graph:

```bash
curlew run 'collections/**/*.yaml' --parallel
```

Inspect the plan before committing to it:

```bash
curlew run 'collections/**/*.yaml' --show-dependencies --dry-run
```

Multi-host distributed execution was removed along with the backend; shard across CI
jobs by globbing different subsets of collections instead.

### 9.2 Performance testing

The `perf` command turns a single request file into a load test:

```bash
curlew perf requests/hot-path.yaml \
  --vus 50 \
  --duration 2m \
  --ramp-up 30s \
  --rps 500 \
  --output stdout
```

Flags:

| Flag | Meaning |
|---|---|
| `--vus <n>` | Virtual users (concurrent workers) |
| `--duration <d>` | Total run duration (`30s`, `2m`, `1h`) |
| `--ramp-up <d>` | Linearly ramp 1→N VUs over this window |
| `--rps <n>` | Target RPS (0 = unbounded, let VUs sprint) |
| `--output` | Output destination (`stdout` supported today) |

Ctrl+C exits with code 130 and writes the partial summary.

> **Safety reminder.** Load testing generates real HTTP traffic. Before pointing `perf` at any endpoint you do not own, verify that load testing is authorized. Curlew is a tool, not a policy — the responsibility for where you send requests is yours. For production systems, hitting anything beyond a staging environment without approval is a good way to trigger a billing or capacity incident.

---

## Part 10 — Plugins

Plugins are external processes that extend Curlew with custom logic at three points in the request lifecycle. They are discovered via the `CURLEW_PLUGINS` env var. Built-in signers (the `signing:` field — see §6.8, shipping in aws-sigv4 and oauth1 forms) and built-in dynamic functions (`{{$...}}` — see §3.7) require no plugin binary; the external-process plugin path described below is for custom hooks beyond what the built-ins cover.

> **Tip:** To sign requests with AWS SigV4 or OAuth1, use the built-in `signing:` field directly — no plugin binary required. See [Authentication and Secrets §6.8] for the full signing reference. The plugin system below is for custom organisation-specific hooks that go beyond what built-in signers cover.

### 10.1 What plugins can do

Three hooks, each with a narrow contract:

| Hook | When it fires | Can mutate |
|---|---|---|
| `on_request` | Before each HTTP request is sent | method, url, headers, body, query_params |
| `on_response` | After each response is received | annotations (status/headers/body are **not** replaceable) |
| `on_result` | Once at run completion | nothing (informational) |

Typical uses:

- `on_request`: inject signing headers, rewrite URLs, add correlation IDs.
- `on_response`: tag responses with custom metadata, emit metrics.
- `on_result`: post run summaries to Slack/Teams, upload to an internal dashboard.

Plugins fire **per attempt and per iteration** — a request that retries three times will fire `on_request` three times; a data-driven iteration fires for every row.

### 10.2 Discovering plugins

Curlew looks up plugins via `CURLEW_PLUGINS`:

```bash
# macOS / Linux — colon-separated
export CURLEW_PLUGINS=/usr/local/bin/curlew-myhook:/opt/curlew-plugins

# Windows — semicolon-separated
set CURLEW_PLUGINS=C:\Tools\curlew-myhook.exe;C:\curlew-plugins
```

Paths can be:

- A specific executable.
- A directory (scanned for executables named `curlew-*`).

All listed plugins are launched at run start.

### 10.3 Inspecting installed plugins

List discovered plugins and their registered hooks:

```bash
curlew plugins list
# NAME              VERSION  HOOKS
# curlew-myhook    0.3.1    on_request
# curlew-slack     1.0.0    on_result
```

Use this to confirm that the right plugins are loaded before a run.

### 10.4 The JSON-RPC handshake

Plugins are external processes speaking JSON-RPC 2.0 over stdin/stdout (newline-delimited). At startup, Curlew sends a handshake:

Request (curlew → plugin):

```json
{"jsonrpc":"2.0","id":1,"method":"curlew/hello","params":{}}
```

Response (plugin → curlew):

```json
{"jsonrpc":"2.0","id":1,"result":{
  "name": "curlew-myhook",
  "version": "0.3.1",
  "protocol_version": 1,
  "hooks": ["on_request"]
}}
```

Required response fields: `name`, `version`, `hooks`. `protocol_version` is optional (defaults to 1). Duplicate plugin names (post-handshake) are rejected.

### 10.5 Per-hook payload shapes

Sketched, not exhaustive:

**`on_request`** — curlew sends the request envelope; plugin may return changes:

```json
// curlew → plugin
{"jsonrpc":"2.0","id":42,"method":"on_request","params":{
  "method":"GET",
  "url":"https://api.example.com/users/123",
  "headers":{"Accept":"application/json"},
  "body":null,
  "query_params":{}
}}

// plugin → curlew — only non-zero fields override
{"jsonrpc":"2.0","id":42,"result":{
  "headers":{"Accept":"application/json","Authorization":"AWS4-HMAC-SHA256 ..."}
}}
```

**`on_response`** — plugin sees the full response but cannot replace it; it may attach annotations for later consumption:

```json
{"jsonrpc":"2.0","id":43,"method":"on_response","params":{
  "status_code":200,
  "headers":{"Content-Type":"application/json"},
  "body":"...",
  "duration_ms":234
}}
```

**`on_result`** — a summary and per-request rows:

```json
{"jsonrpc":"2.0","id":99,"method":"on_result","params":{
  "summary":{"total":10,"passed":9,"failed":1,"duration_ms":5120},
  "requests":[ ... ]
}}
```

### 10.6 Timeouts and termination

- Per-hook timeout: **10 seconds**. A hook that exceeds the timeout is terminated; the run continues as if the hook were not registered.
- At run end (or Ctrl+C), plugins receive SIGINT and are given **500 ms** to exit cleanly before SIGKILL.
- Plugin stderr is forwarded to Curlew's stderr — your plugin can log without any extra wiring.

For a worked example, see the shipped go-cli plugin under `examples/plugins/` in the repository.

---

## Part 11 — Reference

### A. CLI reference

Alphabetical, every command with one-line purpose and flags. Details are in the body of the manual.

**`curlew --help`** / **`curlew --version`** — help or version, respectively.

**`curlew exec <url>`** — execute a single request. Flags:

| Flag | Meaning |
|---|---|
| `--stdin` | Read request JSON from stdin (10 MB limit) |
| `-X, --method <M>` | HTTP method (default GET) |
| `--dry-run` | Resolve and preview, don't send |
| `--log <file>` | Append JSONL log line |
| `--non-interactive` | Suppress prompts |
| `--format <type>` | `terminal` or `json` |
| `--var k=v` | Set a variable (repeatable) |
| `--env-var VAR` | Import OS env var (repeatable) |
| `--no-color` | Disable ANSI color |
| `-v` / `-vv` / `-q` | Verbosity |

**`curlew import openapi <spec>`** — convert an OpenAPI 3.x spec to a collection. `--output <file>` sets the destination.

**`curlew info`** — show project metadata. `--format json` for machine output.

**`curlew init [dir]`** — scaffold a new project (curlew.yaml, .env.example, .gitignore, environments/dev.yaml, collections/sample.yaml).

**`curlew perf <request-file>`** — load test. Flags: `--vus`, `--duration`, `--ramp-up`, `--rps`, `--output`.

**`curlew plugins list`** — list discovered plugins and their hooks.

**`curlew pr-check`** — gate CI on a results file written by `curlew run --format json`. Exits 0 when every test passed, 1 when the results contain failures, 2 on a usage error or an unreadable file. Flags: `--results`, `--summary`, `--dry-run`. Nothing is uploaded (§8.1).

**`curlew run <file|pattern>`** — execute a collection (or glob-matched collections). Full flag list in §4.1, §5.6, §8.1, §9.1.

**`curlew schema`** — emit the JSON Schema for the collection format to stdout.

**`curlew ui`** — start the local web UI (runner & inspector) on 127.0.0.1. Flags:

| Flag | Meaning |
|---|---|
| `--port <n>` | Listen port (default `ui.port` from curlew.yaml, else 8765; `0` = ephemeral; defaulted busy ports scan +1…+19) |
| `--env <name>` | Environment preselected in the UI |
| `--collection <file>` | Restrict the tree to one collection |
| `--no-open` | Don't launch the browser (the tokened URL is always printed) |
| `--no-color` | Disable ANSI color on stderr |

The server binds loopback only and mints a per-start session token (printed in the URL). Files are the only source of truth — the UI never edits them; its single write is the run-history store under `.curlew/ui/` (self-gitignored). Sensitive values are always redacted; there is deliberately no `--allow-sensitive`. Full specification: `docs/UI_SPECIFICATION.md`.

**`curlew validate <file|glob>`** — parse and validate collections without executing. `--format json` for machine output.

**`curlew vault list`** — list configured vault provider profiles.

**`curlew watch <file>`** — run and re-run on file changes. Accepts all `run` flags plus `--clear`.

### B. Environment variables

| Variable | Used by | Meaning |
|---|---|---|
| `CURLEW_TEAM_CONFIG` | shared vault templates | Path to a local shared vault config file. |
| `CURLEW_VAULT_STUB` | shared vault templates | `1` enables in-memory stub provider. |
| `CURLEW_CONFIG_DIR` | telemetry | Override the config directory (`~/.config/curlew` on Linux, `~/Library/Application Support/curlew` on macOS). |
| `CURLEW_TELEMETRY_FILE` | telemetry | Override the local events file. |
| `CURLEW_PLUGINS` | plugins | Colon/semicolon-separated list of plugin paths. |
| `NO_COLOR` | all terminal output | Any non-empty value disables ANSI colors. |

Plus any OS environment variable a user imports via `--env-var`.

### C. File format schemas

Compact schema for each supported input file. Refer back to the tutorial sections for examples.

#### C.1 Collection (`collections/*.yaml`)

```yaml
name: string                            # required
description: string
variables:
  key: scalar
  key: !sensitive value
  key: { from_command: "...", sensitive: bool, cache: int }
include: [string]
retry: RetryConfig                      # collection-wide default
rate_limit_rps: int
setup:    [RequestItem]
requests: [RequestItem]
teardown: [RequestItem]
options:
  stop_on_failure: bool

# RequestItem:
# - name: string                        # required
#   path: string                        # external request file (mutually exclusive with `request:`)
#   auth: string                        # auth profile name
#   required: bool
#   depends_on: [string]                # parallel: names of requests that must run first
#   retry: RetryConfig                  # per-request override
#   data_driven: DataDrivenConfig
#   variables: { ... }
#   request:
#     method: string                    # default GET
#     url: string                       # required (if no external `path:`)
#     headers: { Name: string }
#     query:   { param: string }
#     body:    string | {...}                    # inline body (mutex with body_file, body_binary_file)
#     body_file: string                          # text file; {{vars}} interpolated; CT auto-detected
#     body_binary_file: string                   # raw bytes; no interpolation; CT auto-detected
#     protocol: http | graphql | websocket   # default http
#     graphql: GraphQLConfig
#     websocket: WebSocketConfig
#   assertions:
#     status: int | [int]
#     headers: { Name: { equals|exists|matches } }
#     body:    { $.jsonpath: { operator: value } }   # see §2.2 for operator list
#     timing:  { max_duration_ms: int }
#     schema:  string                   # JSON Schema file
#   extract:
#     var_name: $.jsonpath
```

#### C.2 External request (`requests/**/*.yaml`)

Same shape as a collection's `RequestItem`. Top level: `name`, `request`, `assertions`, `extract`.

#### C.3 Environment (`environments/*.yaml`)

```yaml
variables:
  key: scalar                           # nested maps are flattened with underscores
```

#### C.4 Project config (`curlew.yaml`)

```yaml
project_name: string                    # required
variables:      { key: value }
secrets:        SecretsConfig
auth_profiles:  { name: AuthProfile }
defaults:
  retry:   RetryConfig
  graphql:
    error_handling:
      partial_success: fail | warn | ignore
```

#### C.5 `.env`

```
# comment
KEY=value
KEY="quoted value"
!sensitive KEY=value
```

#### C.6 Data file (`data/*.csv|.json|.yaml`)

- CSV: first row header, rest data.
- JSON: top-level array of objects.
- YAML: top-level list of maps.

#### C.7 GraphQL file (`graphql/**/*.graphql`)

Query files: one query per file, using `$variable` placeholders that map to `graphql.variables`. Fragment files: one fragment per file (`fragment Name on Type { ... }`).

### D. Exit codes

(Identical to §4.3 — listed once more for lookup.)

| Code | Meaning |
|---|---|
| `0` | Every assertion passed |
| `1` | Assertion failure; or a top-level usage error |
| `2` | Safety guard tripped; or a `perf` / `pr-check` usage error |
| `3` | Parse or config error |
| `4` | Execution error (network/TLS) |
| `5` | Variable resolution error |
| `130` | SIGINT during `perf` |

### E. Glossary

- **Collection** — A YAML file containing a named test suite with `setup`, `requests`, and `teardown` phases.
- **Request item** — A single entry in one of those phases: a `name`, a `request` envelope, optional assertions, extractions, and retry/data-driven/auth config.
- **Assertion** — A check applied to a response (status, headers, body, timing) that contributes to the request's pass/fail outcome.
- **Variable scope** — The merged map of all defined variables at run time. See precedence in §3.2.
- **Extraction** — Pulling a value out of a JSON response body via JSONPath into a named variable for use by later requests.
- **Environment** — A named file of variable overrides (`environments/<name>.yaml`) selected with `--env`.
- **Auth profile** — A named configuration that runs an auth collection to produce a token, cached with TTL.
- **Wave** — A set of requests that can be executed concurrently in parallel mode, computed from `depends_on` graph.
- **Data-driven request** — A request that iterates over rows of a data file, one iteration per row.
- **Hook** — A point in the request lifecycle (`on_request`, `on_response`, `on_result`) at which plugins may run custom code.
- **Redaction** — Replacement of sensitive values with `[REDACTED]` in output, triggered by `!sensitive`, vault resolution, `sensitive: true`, or heuristic name matching.

### F. Troubleshooting

**1. `undefined variable: base_url` (exit 5).** The variable is referenced but never defined. Check `curlew.yaml`, your environment file, `.env`, and the CLI flags. Run with `--dry-run` to surface the error without any HTTP traffic.

**2. `yaml: line X: ...` parse errors (exit 3).** Usually indentation or a missing colon. Run `curlew validate <file>` for a line-pinpointed message.

**3. `circular variable reference: a -> b -> a` (exit 5).** Two variables interpolate each other. Break the cycle or inline one side.

**4. `cycle detected in parallel graph` (exit 3).** Your `depends_on` declarations form a cycle. Use `--show-dependencies` to inspect the graph.

**5. Many 429s all at once.** Your collection is hammering a rate-limited endpoint. Add `retry:` with `respect_retry_after: true`, or set `rate_limit_rps:` at collection level.

**6. `plugin handshake timeout`.** The plugin executable did not respond to `curlew/hello` in time. Run the plugin binary directly and verify it responds to a hello request on stdin. Check with `curlew plugins list`.

**7. A value you expected to be secret is showing in logs.** Mark it `!sensitive` in `.env`, or rename it to match the redaction heuristic (`*_token`, `*_secret`, `*_password`, `*_key`, `*_auth`, `*_credential`). Do **not** use `--allow-sensitive` in CI.

**8. JSONPath assertion fails but the value looks right.** Check types. `equals: 200` matches both `200` (number) and `"200"` (string), but `equals: "200"` only matches the string. Use `type: number` to be explicit.

**9. WebSocket test fails with code 1006.** Abnormal closure — the peer didn't send a close frame. Add `reconnect:` config if this is a flaky network, or increase `timeout_ms` on `expect` steps if the server is slow.

**10. `body_file: ...` loads but variables stay as literal `{{...}}` in the request.** Confirm the file you pointed at ends in a text extension (`.json`, `.xml`, `.txt`, `.yaml`) and that you used `body_file:` and not `body_binary_file:`. The binary variant never interpolates — it ships bytes verbatim.

**11. `body file too large` (exit 3).** Your `body_file`/`body_binary_file` exceeds 50 MB. This is a parse-time hard cap; there is no flag to raise it. If you genuinely need larger payloads, invoke `curl` from a `from_command` wrapper or split the upload.

**12. `body, body_file, and body_binary_file are mutually exclusive`.** You set more than one on the same request. Pick one. For a big inline JSON that also uses variables, use `body_file:` and put the JSON in its own file.

---

## Conclusion

If you made it here, you can now do everything Curlew does:

- Author collections from trivial smoke tests to parallel, data-driven, retry-aware suites across multiple environments.
- Handle secrets responsibly with `.env`, `from_command`, vault providers, and dynamic auth profiles.
- Test REST, GraphQL, and WebSocket endpoints from the same file format.
- Wire test runs into any CI system via JUnit, TAP, JSON, or HTML output.
- Scale a run out across cores with `--parallel`, and plug in custom process-level extensions.

The two things worth knowing about going forward:

- `docs/CLI_SPECIFICATION.md` — the CLI's specification, and the source of truth for subtler behaviour edge cases. (`docs/SPECIFICATION.md` describes the separate backend and web dashboard, which the CLI does not talk to.)
- `curlew schema` — the JSON Schema of the collection format, for IDE autocompletion.

Tests ship with your code. Make them boring, make them fast, and keep them in the diff.
