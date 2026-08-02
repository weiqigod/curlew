# apitest — Parallel execution reference

Load this file when the user asks about parallel requests, waves, worker pools,
or ordering.

## Parallel waves

Group requests into waves using the `wave:` field. Requests in the same wave
run concurrently; waves execute in the order they appear.

```yaml
requests:
  - name: Create user
    wave: 1
    request:
      method: POST
      url: "{{base_url}}/users"
      body:
        name: Alice

  - name: Create product
    wave: 1
    request:
      method: POST
      url: "{{base_url}}/products"
      body:
        name: Widget

  - name: Place order
    wave: 2
    request:
      method: POST
      url: "{{base_url}}/orders"
      body:
        user_id: "{{create_user.id}}"
        product_id: "{{create_product.id}}"
```

Wave 1 fires both `POST /users` and `POST /products` concurrently. Wave 2
runs only after wave 1 finishes. Extracted values from wave 1 are available
in wave 2.

## Worker pool

Limit the number of concurrent requests within a wave with `workers:` in the
`parallel:` config block:

```yaml
# apitest.yaml
parallel:
  workers: 4     # at most 4 concurrent HTTP calls at once
```

The default is unbounded (one goroutine per in-wave request). Set a limit when
hitting rate-limited APIs or testing concurrency behaviour.

## Ordering within a wave

Within a single wave, request order is non-deterministic. Never depend on
within-wave ordering. Use separate waves when ordering matters.

## When not to parallelise

- **Dependent requests** — if request B uses an extract from request A, put A
  in wave 1 and B in wave 2.
- **Rate-limited APIs** — use `workers:` to cap concurrency and add `delay:`
  between waves if needed.
- **Stateful sequences** — auth flows, create-then-update patterns — always
  keep these sequential (different wave numbers or no `wave:` field).
- **Teardown** — cleanup requests should be sequential to ensure predictable
  resource release.

## Notes

- The NDJSON event stream groups `request.start` / `request.end` events by
  `wave`. The markdown run summary shows wave groupings with timing.
- Parallel requests each count against the guard rail (`MaxRequests`).
- Without `wave:`, all main-phase requests run sequentially in document order.
