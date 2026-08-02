# curlew — Assertions reference

Load this file when the user asks about assertion operators, how to assert
status codes, headers, response bodies, JSONPath, or timing.

For CEL expressions (`assertions: - cel: <expr>`), see `expressions.md`.

## Assertion block shape

Assertions live under the `assertions:` key of each request. You can use the
map form (shorthand for single-value operators) or the list form:

```yaml
assertions:
  status: 200
  headers:
    Content-Type: "application/json"
  body:
    $.user.id:
      exists: true
      equals: 42
```

## Status assertions

```yaml
assertions:
  status: 200          # exact match
  status:
    in: [200, 201]     # any of the listed codes
```

## Header assertions

```yaml
assertions:
  headers:
    Content-Type: "application/json"          # substring match
    X-Request-Id:
      exists: true                            # header must be present
    Cache-Control:
      equals: "no-store"                      # exact match
```

## Body assertions

### JSONPath operators

Use `$.path` syntax to select a value in the JSON response body:

| Operator | Meaning | Example |
|---|---|---|
| `exists: true/false` | Field presence | `$.data: {exists: true}` |
| `equals: <value>` | Exact equality | `$.status: {equals: "active"}` |
| `contains: <str>` | Substring or array contains | `$.message: {contains: "OK"}` |
| `matches: <regex>` | Regex match | `$.email: {matches: ".*@example\\.com"}` |
| `greater_than: <n>` | Numeric > | `$.count: {greater_than: 0}` |
| `less_than: <n>` | Numeric < | `$.latency_ms: {less_than: 500}` |
| `length: <n>` | Array or string length | `$.items: {length: 3}` |
| `type: <t>` | JSON type (`string`, `number`, `boolean`, `array`, `object`, `null`) | `$.id: {type: "number"}` |

```yaml
assertions:
  body:
    $.user.email:
      exists: true
      matches: ".*@acme\\.com"
    $.items:
      length: 3
    $.total:
      greater_than: 0
```

### Raw body assertions

```yaml
assertions:
  body:
    raw:
      contains: "pong"
      equals: "pong\n"
```

## Timing assertions

```yaml
assertions:
  timing:
    less_than: 500ms     # total round-trip under 500 ms
```

## Failure model

- `assertions:` with any failing operator → exit code 1.
- The per-request markdown file (`responses/<slug>.md`) shows operator,
  expected value, and actual value for every assertion.
- For CEL expressions as assertions, see `expressions.md`.

## Decision table: operator vs CEL

| Use case | Use |
|---|---|
| Status code, header, JSONPath field | Operator assertion |
| Arithmetic, cross-field logic, regex patterns, response-time-dependent logic | `assertions: - cel: <expr>` |

Operator assertions are checked before CEL expressions. They cannot be mixed
in the same `assertions:` map; use the list form when combining.
