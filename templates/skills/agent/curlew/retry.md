# curlew — Retry reference

Retry is opt-in with `retry.enabled: true`. Use `max_attempts` (including the
first request), `initial_delay_ms`, `max_delay_ms`, and `backoff_strategy`
(`constant`, `linear`, `exponential`). `retry_on` selects status codes/ranges,
network errors, timeouts and methods; `do_not_retry_on` exclusions take priority.
Do not use `attempts`, `delay`, `backoff`, or `idempotent` as retry keys.

Enabling retry with defaults allows three attempts, starting at 500 ms with
exponential backoff, for transient statuses 429/502/503/504 and idempotent methods.
Retry is not general assertion polling. A successful HTTP response with a wrong
body does not become retryable just because its assertion fails. For POST/PATCH,
review idempotency and specify allowed methods deliberately.

This fixture fails twice with 503, then succeeds. The assertion checks the server's
attempt counter, not an invented timing expectation.

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

<!-- agent-source: examples/agent/retry.yaml -->
```yaml
name: Retry example
requests:
- name: recovers after two 503s
  request:
    method: GET
    url: '{{mud}}/s/{{run}}-retry/flaky/fail-then-succeed?times=2&status=503'
  retry:
    enabled: true
    max_attempts: 4
    initial_delay_ms: 10
    backoff_strategy: constant
    retry_on:
      status_codes:
      - 503
  assertions:
    status: 200
    body:
      $.attempt:
        equals: 3
      $.failing:
        equals: false
```
