# Curlew Agent Event Stream — v1.4

## Audience and scope

This document is the authoritative reference for agents and tools that consume
the NDJSON event stream emitted by `curlew run --events <file>`. The stream
provides a machine-readable, schema-versioned record of every run, request,
assertion, and failure. This document defines every event kind, every field's
semantics, the ordering and timing guarantees, the body-truncation and encoding
rules, and the v1.x stability policy. You do not need to read the source code to
write a reliable consumer of this stream.

## Table of contents

- [v1.3 → v1.4 changelog](#v13--v14-changelog)
- [Stream invariants](#stream-invariants)
- [Event kinds](#event-kinds)
  - [run.start](#runstart)
  - [run.error](#runerror)
  - [request.start](#requeststart)
  - [request.end](#requestend)
  - [assertion.result](#assertionresult)
  - [run.end](#runend)
- [Error taxonomy](#error-taxonomy)
  - [Categories](#categories)
  - [Error codes by category](#error-codes-by-category)
- [Ordering guarantees](#ordering-guarantees)
- [Body truncation and encoding](#body-truncation-and-encoding)
- [run.error vs request.end with error](#runerror-vs-requestend-with-error)
- [Stability policy](#stability-policy)
- [Consumer guidance for agents](#consumer-guidance-for-agents)

---

## v1.3 → v1.4 changelog

v1.4 changes `assertion.result`. Two of the three changes are additive; the
third is a **conformance fix** that changes an emitted value.

**New optional fields:**

- `assertion.result.target` (string, omitempty) — what the assertion addressed:
  a JSONPath for `body`, a header name for `header`, a schema file path or
  failing instance path for `schema`, and `assertions[N]` for `cel`. Absent for
  `status` and `timing`, which address the response as a whole.
- `assertion.result.operator` (string, omitempty) — the comparison applied
  (`equals`, `matches`, `exists`, `contains_all`, ...). Absent where the type
  implies the comparison.

**New required field:**

- `assertion.result.label` (string) — the assembled human-readable phrase, e.g.
  `body $.user.name equals`. This is exactly the value `type` carried through
  v1.3, so a consumer that string-matched the composite migrates by reading
  this field instead.

**Conformance fix to `assertion.result.type`:**

Every published schema since v1.0 has declared `type` as an enum. The emitter
never honoured it. For every kind except `status` the field was built as a
composite —

```go
fmt.Sprintf("body %s %s", path, operator)   // "body $.user.name equals"
```

— so `body`, `header`, `schema` and `cel` assertions never matched the enum,
and `timing` was not in the enum at all. Of the four declared members exactly
one (`status`) was ever emitted verbatim, which meant a consumer following the
documented contract and switching on `type == "body"` matched nothing.

From v1.4, `type` emits the discriminator alone and the enum is widened to the
seven kinds that can actually be produced — the six from the assertion
evaluators plus `graphql_error`, which the runner appends when a GraphQL
response carries errors under a failing `error_handling` mode:

| | v1.3 emitted | v1.4 emitted |
|---|---|---|
| `type` | `body $.user.name equals` | `body` |
| `target` | — | `$.user.name` |
| `operator` | — | `equals` |
| `label` | — | `body $.user.name equals` |

**Why this is not a v2.0 bump.** The stability policy lists "changing the
semantic meaning of an existing field or value" as breaking. That protects a
contract consumers could rely on; here no published schema ever permitted the
composite, so the emission was non-conformant rather than contractual — the
same reasoning applied in v1.3 to the `wave_index` `minimum` correction. The
migration is mechanical and lossless: read `label` wherever you read `type`.

**`schema_version`** changes from `"1.3"` to `"1.4"` in every emitted event.

---

## Stream invariants

Every event in the stream satisfies the following invariants unconditionally:

- **Schema version** — every event carries `"schema_version": "1.4"`. Consumers
  MUST reject or warn on unknown schema versions; they MUST NOT silently discard
  the version field.
- **Run ID** — `run_id` is a 32-character lowercase hexadecimal string. It is
  identical for every event in one run. Use `(run_id, id)` as the globally unique
  event identity when merging streams from multiple parallel runs.
- **Monotonic ID** — `id` is a 64-bit integer counter, starts at 1 for the first
  event (`run.start`), and increments by 1 for each subsequent event within a
  run. There are no gaps. `run.start` is always `id=1`.
- **Millisecond timestamp** — `at_ms` is the number of milliseconds elapsed since
  the run started, measured from a single monotonic origin. `run.start` always
  has `at_ms=0`. In parallel runs, events from different requests may interleave
  in `at_ms` order; `id` remains strictly monotonic regardless.
- **NDJSON encoding** — each event is a single complete JSON object, terminated
  by a newline (`\n`). There is no trailing newline after the last event. The
  encoding is UTF-8. Do not buffer the whole stream.
- **Kind discriminator** — every event has a `kind` field whose value determines
  which fields are present. Dispatch on `kind` before reading other fields.

---

## Event kinds

Each sub-section below shows the field table for that kind (required fields in
**bold**), a minimal example using only required fields, and a maximal example
using all defined fields.

### run.start

Emitted as the first event of every run. Always has `id=1` and `at_ms=0`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| **schema_version** | string | yes | Always `"1.4"` |
| **run_id** | string | yes | 32-char lowercase hex run identifier |
| **id** | integer | yes | Monotonic event counter; always 1 for this event |
| **at_ms** | integer | yes | Milliseconds since run start; always 0 for this event |
| **kind** | string | yes | Always `"run.start"` |
| **started_at** | string | yes | RFC3339Nano UTC timestamp when the run began |
| **curlew_version** | string | yes | `curlew` binary version string |
| **cli_args** | array of string | yes | CLI arguments passed to the run command |
| collection_file | string | no | Path to the collection file, if provided |
| env_name | string | no | Environment name selected for this run |
| selection | array of string | no | Names passed via `--only`; omitted when `--only` was not supplied. Added in v1.1. |

Minimal example (required fields only):

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":1,"at_ms":0,"kind":"run.start","started_at":"2026-04-25T10:00:00Z","curlew_version":"1.0.0","cli_args":["run","tests.yaml"]}
```

Maximal example (all fields, including v1.1 selection):

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":1,"at_ms":0,"kind":"run.start","started_at":"2026-04-25T10:00:00Z","curlew_version":"1.0.0","cli_args":["run","tests.yaml","--env","staging","--only","Get user"],"collection_file":"tests.yaml","env_name":"staging","selection":["Get user"]}
```

### run.error

Emitted when the run fails before any requests are executed — for example,
because the collection file cannot be parsed, a required variable is missing, or
a pre-flight auth check fails. See [run.error vs request.end with error](#runerror-vs-requestend-with-error)
for guidance on when each applies.

`run.error` is always followed by `run.end` (with `exit_code != 0`). No
`request.start` or `request.end` events precede it.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| **schema_version** | string | yes | Always `"1.4"` |
| **run_id** | string | yes | Run identifier |
| **id** | integer | yes | Monotonic event counter |
| **at_ms** | integer | yes | Milliseconds since run start |
| **kind** | string | yes | Always `"run.error"` |
| **error** | object | yes | Error payload; see [Error taxonomy](#error-taxonomy) |
| error.category | string | yes | Error category (one of the values in the taxonomy) |
| error.message | string | yes | Human-readable error message |
| error.code | string | no | Stable machine-readable error code |
| error.hint | string | no | Suggested remediation |
| error.file | string | no | Source file where the error originated |
| error.line | integer | no | Line in the source file |

Minimal example:

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":2,"at_ms":0,"kind":"run.error","error":{"category":"parse","message":"unexpected token at line 5"}}
```

Maximal example:

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":2,"at_ms":3,"kind":"run.error","error":{"category":"parse","code":"PARSE_INVALID_YAML","message":"invalid YAML syntax","hint":"Fix the YAML syntax at the line indicated. Check indentation and quoting.","file":"tests.yaml","line":5}}
```

### request.start

Emitted immediately before a request is executed. The `request_id` pairs with
the corresponding `request.end` and with any `assertion.result` events for the
same request. The `request_slug` (v1.2) provides a URL-safe identifier derived
from the request name.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| **schema_version** | string | yes | Always `"1.4"` |
| **run_id** | string | yes | Run identifier |
| **id** | integer | yes | Monotonic event counter |
| **at_ms** | integer | yes | Milliseconds since run start |
| **kind** | string | yes | Always `"request.start"` |
| **request_id** | string | yes | Opaque request identifier; paired with `request.end` |
| **method** | string | yes | HTTP method (`GET`, `POST`, etc.) |
| **url** | string | yes | Resolved URL for this request |
| request_slug | string | no | URL-safe slug derived from `name`; see slug derivation rule. Added in v1.2. |
| name | string | no | Human-readable request name, if defined in the collection |
| phase | string | no | Run phase (`setup`, `main`, `teardown`) if applicable |
| source_file | string | no | Collection file where this request is defined |
| source_line | integer | no | Line in `source_file` where this request is defined |

Minimal example:

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":2,"at_ms":1,"kind":"request.start","request_id":"req-1","method":"GET","url":"https://api.example.com/users/1"}
```

Maximal example (including v1.2 request_slug):

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":2,"at_ms":1,"kind":"request.start","request_id":"req-1","request_slug":"get-user-by-id","name":"Get user by ID","method":"GET","url":"https://api.example.com/users/1","phase":"main","source_file":"tests.yaml","source_line":12}
```

### request.end

Emitted after the request completes and all assertions have been evaluated.
Contains the outcome, the HTTP status code (when available), the round-trip
duration, and optional request/response body snapshots (subject to truncation;
see [Body truncation and encoding](#body-truncation-and-encoding)). The
`request_slug` (v1.2) matches the paired `request.start`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| **schema_version** | string | yes | Always `"1.4"` |
| **run_id** | string | yes | Run identifier |
| **id** | integer | yes | Monotonic event counter |
| **at_ms** | integer | yes | Milliseconds since run start |
| **kind** | string | yes | Always `"request.end"` |
| **request_id** | string | yes | Matches the paired `request.start` |
| **outcome** | string | yes | `passed`, `failed`, `skipped`, or `error` |
| **duration_ms** | integer | yes | Round-trip duration in milliseconds |
| request_slug | string | no | Matches `request.start.request_slug`. Added in v1.2. |
| status_code | integer | no | HTTP response status code; absent when `outcome=skipped` or `outcome=error` |
| wave_index | integer | no | Zero-based wave index when running in parallel waves |
| request_body | string | no | Request body content (UTF-8 text, or base64 if `request_body_encoding=base64`) |
| request_body_size | integer | no | Original request body size in bytes; present only when truncated |
| request_body_truncated | boolean | no | `true` when request body was truncated to the body limit |
| request_body_encoding | string | no | `"base64"` when request body content is base64-encoded binary |
| response_body | string | no | Response body content (UTF-8 text, or base64 if `response_body_encoding=base64`) |
| response_body_size | integer | no | Original response body size in bytes; present only when truncated |
| response_body_truncated | boolean | no | `true` when response body was truncated to the body limit |
| response_body_encoding | string | no | `"base64"` when response body content is base64-encoded binary |
| error | object | no | Error payload when `outcome=error` or `outcome=failed`; same structure as `run.error.error` |
| timing | object | no | Connection-phase breakdown in integer microseconds; see [docs/EVENTS_SCHEMA_v1.3.md](EVENTS_SCHEMA_v1.3.md). Added in v1.3. |

Minimal example (`outcome=passed`):

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":4,"at_ms":52,"kind":"request.end","request_id":"req-1","outcome":"passed","duration_ms":42}
```

Example with request_slug and status code:

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":4,"at_ms":52,"kind":"request.end","request_id":"req-1","request_slug":"get-user","outcome":"passed","status_code":200,"duration_ms":42,"response_body":"{\"id\":1,\"name\":\"Alice\"}"}
```

Example with the v1.3 `timing` object (fresh connection, one retry):

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":4,"at_ms":52,"kind":"request.end","request_id":"req-1","request_slug":"get-user","outcome":"passed","status_code":200,"duration_ms":42,"timing":{"dns_us":1234,"connect_us":2100,"tls_us":15400,"ttfb_us":21200,"download_us":900,"total_us":41100,"attempts":2}}
```

Example with a reused connection (no dns/connect/tls phases):

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":6,"at_ms":61,"kind":"request.end","request_id":"req-2","outcome":"passed","status_code":200,"duration_ms":4,"timing":{"ttfb_us":3100,"download_us":210,"total_us":3500,"connection_reused":true}}
```

### assertion.result

Emitted once per assertion evaluated for a request, after the response is
received but before `request.end`. When a request has no assertions, no
`assertion.result` events are emitted.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| **schema_version** | string | yes | Always `"1.4"` |
| **run_id** | string | yes | Run identifier |
| **id** | integer | yes | Monotonic event counter |
| **at_ms** | integer | yes | Milliseconds since run start |
| **kind** | string | yes | Always `"assertion.result"` |
| **request_id** | string | yes | Matches the surrounding `request.start` / `request.end` |
| **type** | string | yes | Assertion kind discriminator: `status`, `body`, `header`, `schema`, `timing`, `cel`, or `graphql_error` |
| target | string | no | What was asserted on — JSONPath, header name, schema path, or `assertions[N]`. Absent for `status` and `timing` |
| operator | string | no | Comparison applied (`equals`, `matches`, `exists`, ...). Absent where the type implies it |
| **label** | string | yes | Assembled human-readable phrase, e.g. `body $.user.name equals` |
| **passed** | boolean | yes | `true` if the assertion passed |
| expected | string | no | Expected value (stringified) |
| actual | string | no | Actual value observed (stringified) |

Minimal example (assertion passed):

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":3,"at_ms":45,"kind":"assertion.result","request_id":"req-1","type":"status","label":"status","passed":true}
```

Example with expected and actual (assertion failed):

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":3,"at_ms":45,"kind":"assertion.result","request_id":"req-1","type":"status","label":"status","passed":false,"expected":"200","actual":"404"}
```

### run.end

The terminal event for every run. Always the last event in the stream. Its `id`
field equals the total number of events emitted. The `event_count` field
duplicates this value for convenient stream-length verification.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| **schema_version** | string | yes | Always `"1.4"` |
| **run_id** | string | yes | Run identifier |
| **id** | integer | yes | Monotonic event counter; equals `event_count` |
| **at_ms** | integer | yes | Milliseconds since run start |
| **kind** | string | yes | Always `"run.end"` |
| **duration_ms** | integer | yes | Total run duration in milliseconds |
| **total** | integer | yes | Total number of requests executed |
| **passed** | integer | yes | Number of requests that passed |
| **failed** | integer | yes | Number of requests that failed (assertion failure) |
| **skipped** | integer | yes | Number of requests that were skipped |
| **exit_code** | integer | yes | Process exit code: `0` = success, `1` = failure |
| **event_count** | integer | yes | Total events emitted, including `run.start` and `run.end` |

Example (successful run, 2 requests):

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":7,"at_ms":123,"kind":"run.end","duration_ms":123,"total":2,"passed":2,"failed":0,"skipped":0,"exit_code":0,"event_count":7}
```

Example (failed run, 1 request with assertion failure):

```json
{"schema_version":"1.4","run_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4","id":5,"at_ms":88,"kind":"run.end","duration_ms":88,"total":1,"passed":0,"failed":1,"skipped":0,"exit_code":1,"event_count":5}
```

---

## Error taxonomy

### Categories

Error events (`run.error.error` and `request.end.error`) always include a
`category` field. The category tells the consumer what type of failure occurred
and guides automated triage:

| Category | When used | Recommended agent action |
|----------|-----------|--------------------------|
| `parse` | Collection file or environment file has invalid syntax | Report file path and line number; show hint |
| `network` | TCP/DNS/TLS failure or timeout | Check connectivity; inspect `code` for specifics |
| `config` | Invalid configuration value or missing required config | Correct the collection or environment file |
| `assertion` | An assertion in the collection failed | Compare `expected` vs `actual` in `assertion.result` |
| `auth` | Authentication pre-flight failed (token unavailable, expired, etc.) | Check credentials and auth profile config |
| `input` | Invalid CLI argument, missing required flag, or unresolvable variable reference | Re-read help text; check invocation and variable definitions |
| `internal` | Unexpected internal error (always a bug) | Report with the full event stream |

### Error codes by category

Error codes are stable, SCREAMING_SNAKE_CASE identifiers. New codes may be
added in v1.x without a version bump, but existing codes will not be renamed or
removed until v2.0. The following table lists known codes per category.

**parse** — produced by `internal/parser`

| Code | Short description | Typical hint | Producing package |
|------|-------------------|--------------|-------------------|
| `PARSE_INVALID_YAML` | YAML syntax error in the collection file | "Fix the YAML syntax at the line indicated. Check indentation and quoting." | `internal/parser` |
| `PARSE_MISSING_REQUIRED_FIELD` | Required field absent from a request definition | "Add the missing field named in the message to the request definition." | `internal/parser` |
| `PARSE_FILE_NOT_FOUND` | Collection file not found at the given path | "Verify the path is correct and readable. Run from the project root or pass an absolute path." | `internal/parser` |
| `PARSE_CIRCULAR_INCLUDE` | A collection's `include:` chain is circular | "Remove the circular include: a collection may not transitively include itself. Check the include: chain and break the cycle." | `internal/parser` |
| `PARSE_CIRCULAR_FILE_REFERENCE` | A `request_file:` chain is circular | "Remove the circular reference: a request_file: must not transitively reference itself. Check the request_file: chain." | `internal/parser` |
| `PARSE_DUPLICATE_REQUEST_NAME` | Two or more main requests share the same name | "Rename one of the duplicated requests. Main request names must be unique because --only, per-request markdown files, and CI report rows all address requests by name." | `internal/parser` |
| `PARSE_SLUG_EMPTY` | Request name slugifies to empty (no alphanumeric runes after normalization) | "Rename the request to include at least one ASCII letter or digit. Slugs are used as filenames for per-request markdown reports." | `internal/parser` |

**network** — produced by `internal/errors` (via `ClassifyNetworkError`)

| Code | Short description | Typical hint | Producing package |
|------|-------------------|--------------|-------------------|
| `NETWORK_DNS` | DNS lookup failed | "Check that the hostname is correct and DNS is configured" | `internal/errors` |
| `NETWORK_TIMEOUT` | Connection or read timed out | "Consider increasing the timeout duration" | `internal/errors` |
| `NETWORK_TLS` | TLS handshake failed | "Verify the server's certificate is valid and trusted" | `internal/errors` |
| `NETWORK_CONNECTION_REFUSED` | TCP connection refused | "Check that the server is running and listening on this port" | `internal/errors` |
| `NETWORK_OTHER` | Other network error | Check network connectivity to the target host | `internal/httpexec` |

**config** — produced by `internal/config`

| Code | Short description | Typical hint | Producing package |
|------|-------------------|--------------|-------------------|
| `CONFIG_INVALID_PROJECT` | Invalid `curlew.yaml` project config | "Validate curlew.yaml against the documented project config schema." | `internal/config` |
| `CONFIG_INVALID_ENV` | Malformed environment file | "Ensure variables: is a map of string values." | `internal/config` |
| `VAR_CIRCULAR_REFERENCE` | A variable value transitively references itself | "Break the cycle: a variable value must not transitively reference itself via {{...}}." | `internal/variable` |
| `VAR_DEPTH_EXCEEDED` | Nested variable interpolation depth exceeded | "Reduce nested variable interpolation depth or refactor to fewer levels." | `internal/variable` |

**input** — produced by `internal/variable` and `internal/runner`

| Code | Short description | Typical hint | Producing package |
|------|-------------------|--------------|-------------------|
| `VAR_UNDEFINED` | A `{{variable}}` reference could not be resolved | "Define the variable in the environment file, pass it via --var NAME=VALUE, or add a default in the collection." | `internal/variable` |
| `ONLY_NO_MATCH` | `--only` was passed but no main request matched the given name | "Pass --only with a name that matches one of the available main requests. --only does not target setup or teardown items." | `internal/runner` |

**assertion** — produced by `internal/assertion`

| Code | Short description | Typical hint | Producing package |
|------|-------------------|--------------|-------------------|
| `ASSERTION_FAILED` | A request completed but one or more assertions did not pass | "Inspect the assertion.result events for this request and adjust either the assertion or the request to make them agree." | `internal/assertion` |

**auth** — produced by `internal/auth`

| Code | Short description | Typical hint | Producing package |
|------|-------------------|--------------|-------------------|
| `AUTH_PROFILE_FAILED` | Auth profile login request returned non-2xx | "Check credentials and the profile's login endpoint." | `internal/auth` |
| `AUTH_PROFILE_NO_EXTRACT` | Auth profile extract path did not match response | "Verify the path against the actual response shape." | `internal/auth` |
| `AUTH_CACHE_CORRUPTED` | Auth cache directory is corrupted | "Delete the auth cache directory and re-run to force re-authentication." | `internal/auth` |
| `RUNNER_AUTH_PROFILE_NOT_FOUND` | An auth profile referenced in the collection does not exist | "Define the auth profile in curlew.yaml or Check the profile name in the collection." | `internal/runner` |

**internal** — produced by `internal/output/events`

| Code | Short description | Typical hint | Producing package |
|------|-------------------|--------------|-------------------|
| `EVENTS_EMITTER_CLOSED` | Emit called after `Close` | Always a bug; report with stream | `internal/output/events` |

---

## Ordering guarantees

Within a single run, the following ordering guarantees hold:

1. `run.start` is always the first event and always has `id=1`, `at_ms=0`.
2. `run.end` is always the last event. Its `id` equals `event_count`.
3. Event `id` values are strictly monotonically increasing with no gaps.
4. For a given `request_id`, the `request.start` event precedes all
   `assertion.result` events, which precede the `request.end` event.
5. When `run.error` is emitted, no `request.start` events appear in the stream.
   `run.end` always follows `run.error`.
6. In parallel execution waves, `at_ms` values across different `request_id`
   values may interleave in any order. Two events with different `request_id`
   values in the same wave may carry overlapping `at_ms` ranges. The `id` field
   remains strictly monotonic regardless, because it is assigned under a single
   atomic counter.
7. `at_ms` is measured from a single monotonic clock origin (the moment
   `run.start` was created). It never decreases within the stream of events from
   one producer. When streams from different producers are merged, `at_ms` values
   MUST be re-calibrated by the merger.

---

## Body truncation and encoding

The `request_body` and `response_body` fields in `request.end` are subject to
the following rules:

- **Default limit** — 2048 bytes. Configurable via `--body-limit` (future flag).
- **Text body ≤ limit** — body emitted inline as a UTF-8 string. No truncation
  fields are set.
- **Text body > limit** — truncated at a valid UTF-8 code-point boundary to the
  limit. `*_truncated=true`, `*_size` set to the original byte length.
- **Binary body** — detected by the presence of any byte value outside the valid
  UTF-8 text range (including NUL). Encoded as base64. `*_encoding="base64"`.
- **Binary body > limit** — base64 encoding of the first N bytes (where N is the
  limit). `*_truncated=true`, `*_size` set to the original byte length.
  `*_encoding="base64"`.
- **Sensitive redaction** — values matching redaction rules (configured via
  `--redact` or the collection's `sensitive` list) are replaced with
  `***REDACTED***` before truncation is evaluated. Truncation operates on the
  redacted bytes.
- **Absent field** — if a body is not present (the request had no body, or the
  response had no body), the field is omitted entirely from the event.

---

## run.error vs request.end with error

Two distinct error surfaces exist in the stream:

**`run.error`** signals that the run cannot proceed at all. It is emitted after
`run.start` and before any request execution begins. Causes include:

- Collection file has invalid YAML or missing required fields
- A request name slugifies to empty (`PARSE_SLUG_EMPTY`) — added in v1.2
- An include file or variable reference is circular
- Pre-flight authentication failed
- An unsupported feature was invoked
- A required CLI flag was missing
- An undefined variable is referenced in the collection
- `--only` was passed but no main request name matched (`ONLY_NO_MATCH`)

After `run.error`, the stream always ends with `run.end` (exit code ≠ 0). No
`request.start` events appear.

**`request.end` with `outcome=error`** signals that one specific request failed
during execution (network failure, plugin error, extraction error). Other requests
in the same run are unaffected. They emit their own `request.start`,
`assertion.result`, and `request.end` events normally. The run may still succeed
overall (exit code 0) if the error request was configured to be non-fatal.

---

## Stability policy

### Additive changes (allowed in v1.x without a version bump)

The following changes may be made to the stream format in any v1.x release. A
consumer that handles unknown values gracefully will continue to work without
modification:

- New `kind` values (new event types)
- New optional fields on existing event kinds (consumers must tolerate unknown
  fields)
- New values in non-discriminator enum fields (`category`, network code values)
- New error codes

### Breaking changes (require a v2.0 bump)

The following changes break consumers and require bumping the major version:

- Renaming or removing any existing field
- Changing a field's type or units (e.g. changing `duration_ms` from integer to
  float, or from milliseconds to seconds)
- Narrowing an enum by removing a previously valid value
- Changing a field from optional to required
- Changing the semantic meaning of an existing field or value
- Changing the `schema_version` constant format
- Changing the slug derivation algorithm (the canonical algorithm is defined in
  this document; any change would invalidate existing filenames and sentinel tags)

---

## Consumer guidance for agents

When building an agent that consumes this stream:

1. **Parse one line at a time.** Do not buffer the whole stream before
   processing. Each line is a complete, self-contained JSON object.

2. **Dispatch on `kind` first.** Before reading any other field, read `kind` and
   branch on it. This ensures you handle new kinds gracefully (by skipping with a
   warning) without breaking on unexpected fields.

3. **Tolerate unknown optional fields.** The stability policy allows additive
   optional fields in v1.x. A conforming consumer must not error on unknown
   field names.

4. **Tolerate unknown `kind` values.** A conforming consumer must not error on
   unknown kind values; skip them with a warning and continue consuming.

5. **Use `(run_id, id)` as the unique event identity.** When merging streams from
   multiple parallel runs, this pair is globally unique.

6. **Check `schema_version` on the first event.** If the version is unknown,
   emit a warning and proceed with best-effort parsing, or halt — depending on
   your tolerance for forward-compatibility risk.

7. **`run.end.exit_code`** is the authoritative success/failure signal. A run is
   successful if and only if `exit_code=0`.

8. **Reconnect on `run.error`.** If you see `run.error`, do not expect any more
   `request.start` events. The next event will be `run.end`.

9. **On `request.end` with `outcome=failed`**, check the `error` field for
   `category=assertion` and `code=ASSERTION_FAILED`. The paired
   `assertion.result` events carry the `expected`/`actual` detail.

10. **On `run.start`, check the `selection` field (v1.1+).** When `selection` is
    present, only the named main requests were executed in this run. Setup and
    teardown still ran. Use this to correlate which requests were intentionally
    included when diagnosing variable-cliff errors.

11. **On `request.start`, use `request_slug` (v1.2+) for filename-safe
    correlation.** The `request_slug` is guaranteed to be a valid filename
    component. It matches the per-request markdown file name under `--report`.
    Use `(run_id, request_slug)` to identify a specific request's markdown file.
