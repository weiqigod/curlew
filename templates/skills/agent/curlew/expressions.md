# curlew — Expressions reference

Use `if: <boolean expression>` to skip a request conditionally. Use
`assertions: {cel: [<boolean expression>]}` for invariants, alongside `status`,
`headers`, `body`, or `timing`. Assertions are a map, not a list of objects.
`extract:` uses JSONPath, not CEL.

Bindings are `response` and `previous` (status, headers, parsed JSON body), `vars`
(resolved variables), and `env` (process environment). Do not assume a response
exists before its request executes. The example uses `previous` only after a
successful first request. Use `timing.max_duration_ms` for duration assertions;
`response.duration_ms` is not a documented activation field.

`curlew validate` catches CEL parse/type errors as `ERR_CEL_PARSE` or
`ERR_CEL_TYPE` (exit code 3). Expressions must return bool. `now()` and zero-argument
`timestamp()` are unavailable; supply time explicitly as a variable when needed.

Variables are exposed as strings; convert with `int(vars.minimum)` for numeric comparisons.

## Decision table

| Need | Use |
|---|---|
| A status, header, JSONPath value or timing bound | Operator assertion |
| A cross-field invariant | `assertions.cel` |
| Conditional execution after a prior response | `if:` |

## Run the example

Start Mudflat using the repository's `site/README.md` setup. Save the complete
collection below as `example.yaml` in a scratch directory. `MUDFLAT_URL` defaults
to that setup's local port; override it if your fixture uses another port.

```bash
export MUDFLAT_URL="${MUDFLAT_URL:-http://127.0.0.1:18080}"
export RUN_ID="agent-$(date +%s)-$$"
curlew validate example.yaml --format json
curlew run example.yaml --var mud="$MUDFLAT_URL" --var run="$RUN_ID" --format json
```

## Complete collection

<!-- agent-source: examples/agent/expressions.yaml -->
```yaml
name: CEL example
variables:
  minimum: 199
requests:
- name: First response
  request:
    method: GET
    url: '{{mud}}/status/200'
  assertions:
    status: 200
- name: Conditional response
  if: previous.status == 200
  request:
    method: GET
    url: '{{mud}}/status/201'
  assertions:
    status:
    - 200
    - 201
    cel:
    - response.status == 201
    - response.body.status > int(vars.minimum)
```
