# curlew — CEL expressions reference

Load this file when the user asks about `if:` conditions, `cel:` assertions,
CEL syntax, the standard activation bindings, or CEL error codes.

## What CEL is for

Curlew embeds the Common Expression Language (CEL) for two purposes:

1. **`if:` field on requests** — a boolean gate that skips the request when the
   expression evaluates to `false`.
2. **`assertions: - cel: <expr>`** — a boolean assertion that fails when the
   expression evaluates to `false`.

CEL is intentionally NOT available at `extract:` — use JSONPath there.

## `if:` field

Skip a request conditionally based on a previous response or a variable:

```yaml
requests:
  - name: Create user
    request:
      method: POST
      url: "{{base_url}}/users"
    extract:
      created_id: "$.id"

  - name: Send welcome email
    if: "previous.status == 201"
    request:
      method: POST
      url: "{{base_url}}/emails/welcome"
      body:
        user_id: "{{created_id}}"
```

When `if:` evaluates to `false`, the request is skipped (outcome: `skipped`
in the event stream; not counted as a failure).

## `assertions: - cel: <expr>` shape

Add a CEL assertion to the list form of `assertions:`:

```yaml
assertions:
  - cel: "response.status == 200"
  - cel: "response.body.total > 0"
  - cel: "response.duration_ms < 500"
```

A CEL assertion fails when the expression is `false`. The failure message
includes the expression, the resolved sub-values, and (if the expression
references a sensitive variable) the value is redacted.

CEL assertions cannot be mixed with operator assertions in the same `assertions:`
map. Use the list form when you need both:

```yaml
assertions:
  - status: 200                            # operator assertion (list form)
  - cel: "response.body.count > vars.min_count"  # CEL assertion
```

## Standard activation

Every CEL expression has access to these bindings:

| Binding | Type | Contents |
|---|---|---|
| `response` | map | `status` (int), `headers` (map), `body` (dyn), `duration_ms` (double) |
| `previous` | map | Same shape as `response`; the immediately preceding request's result |
| `vars` | map | All resolved variables at the time the expression is evaluated |
| `env` | map | OS environment variables (from `env_import:` list) |

`previous` is `null` for the first request; guard with `previous != null`.

`response.body` is typed as `dyn` — CEL's dynamic type. Field access works
without casting for map-shaped JSON. For array elements, use index notation:
`response.body.items[0].id`.

## Disabled functions

The following functions are disabled to ensure deterministic test results:

| Function | Reason |
|---|---|
| `now()` | Returns current time — non-deterministic across runs |
| `timestamp()` (zero-arg) | Same reason as `now()` |

All other standard CEL library functions are available (string operations,
math, type conversions, list operations, etc.).

## Decision table: CEL vs operator assertions

| Scenario | Recommendation |
|---|---|
| Assert a specific HTTP status code | Operator: `status: 200` |
| Assert a JSON field value | Operator: `body: {$.field: {equals: "value"}}` |
| Assert across two fields (ratio, comparison) | CEL: `response.body.a > response.body.b` |
| Assert based on a previous response's value | CEL: `response.body.id == previous.body.created_id` |
| Skip a request based on a condition | CEL: `if: "previous.status == 201"` |
| Assert a timing bound | Operator: `timing: {less_than: 500ms}` or CEL: `response.duration_ms < 500` |

## Error codes

| Code | Meaning | Diagnostic |
|---|---|---|
| `ERR_CEL_PARSE` | Expression syntactically invalid or uses a disabled function | Run `curlew validate` — message names field path and shows up to 200 chars of source |
| `ERR_CEL_TYPE` | Expression compiles but returns a non-bool type | Run `curlew validate` — message names the actual type |

Both error codes surface at `curlew validate` time, not at run time, when the
collection is syntactically valid YAML but contains an invalid CEL expression.
