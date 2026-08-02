# curlew — Retry reference

Load this file when the user asks about retrying requests, backoff policies,
flaky tests, or idempotency guards.

## Retry block

Add a `retry:` block to any request to automatically retry on assertion failure
or network error:

```yaml
requests:
  - name: Wait for job
    request:
      method: GET
      url: "{{base_url}}/jobs/{{job_id}}"
    assertions:
      body:
        $.status:
          equals: "complete"
    retry:
      attempts: 5
      delay: 2s
      backoff: linear
```

## Fields

| Field | Meaning | Default |
|---|---|---|
| `attempts` | Maximum number of total attempts (initial + retries) | `1` (no retry) |
| `delay` | Wait between attempts | `1s` |
| `backoff` | Delay growth strategy | `constant` |
| `on` | Retry trigger: `assertion_failure`, `network_error`, or `any` | `any` |

## Backoff policies

| Policy | Delay between attempts |
|---|---|
| `constant` | Always `delay` |
| `linear` | `delay × attempt_number` (1×, 2×, 3×, …) |
| `exponential` | `delay × 2^(attempt_number-1)` (1×, 2×, 4×, …) |

## Idempotency guard

Retry is safe only for idempotent requests. Mark non-idempotent requests
explicitly to suppress auto-retry suggestions:

```yaml
    retry:
      idempotent: false   # disables retry even if attempts > 1
```

Curlew emits a warning if `idempotent: false` and `attempts > 1`; it does
not prevent the configuration but records the warning in the event stream.

## Retry and the event stream

Each attempt emits a `request.start` / `request.end` pair. The final
`request.end` event carries `retries: N` (number of extra attempts used).
Read `.curlew/run.ndjson` to see retry history.

## Notes

- Retries count against the request guard rail (`MaxRequests`). Each attempt
  is a separate HTTP call.
- Setup and teardown phases do not retry by default; add `retry:` explicitly
  if needed.
- For polling patterns (wait until a job completes), prefer `retry:` over
  sleep scripts — the delay is measured from the end of the previous response,
  not wall-clock.
