# curlew — Assertions reference

Use an `assertions:` **map**. `status` accepts an integer or list of integers.
`headers` accepts `equals`, `exists`, and `matches`. Body operators target JSONPath
expressions; they are not raw-body substring checks. Timing uses
`timing: {max_duration_ms: 5000}`. Put CEL expressions in a `cel:` list alongside
operator assertions in the same map; see `expressions.md`.

Body operators: `equals`, `exists`, `not_exists`, `type`, `contains`, `contains_all`,
`matches`, `greater_than`, `less_than`, `greater_than_or_equal`,
`less_than_or_equal`, `length`, `approximately`, `in_range`.
Use `curlew schema` for the accepted keys. A failed assertion exits with code 1; invalid
YAML/configuration exits with code 3. Read the linked report's `### Assertions` section.

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

<!-- agent-source: examples/agent/assertions.yaml -->
```yaml
name: Assertion example
requests:
- name: status any-of form
  request:
    method: GET
    url: '{{mud}}/status/418'
  assertions:
    status:
    - 200
    - 418
    - 503
    body:
      $.status:
        equals: 418
      $.status_text:
        equals: I'm a teapot
- name: bodiless 204 carries nothing
  request:
    method: GET
    url: '{{mud}}/status/204'
  assertions:
    status: 204
- name: integers beyond float64 survive intact
  request:
    method: GET
    url: '{{mud}}/json/bignum'
  assertions:
    status: 200
    body:
      $.beyond_float64:
        equals: 9007199254740993
      $.negative:
        less_than: 0
      $.very_large:
        greater_than: 1000000
    cel:
    - response.body.beyond_float64 == 9007199254740993
```
