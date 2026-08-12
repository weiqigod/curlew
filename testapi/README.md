# testapi — Mudflat

A deliberately difficult HTTP server, and the dogfood suite that runs curlew
against it.

Full design: [`docs/TESTAPI_SPECIFICATION.md`](../docs/TESTAPI_SPECIFICATION.md).
**Phase 1 is implemented.** Phases 2 and 3 are specified but not built; `GET
/capabilities` reports what is absent and why.

## Why this exists

curlew has 1,963 tests and 88.3% statement coverage, and before this directory
every one of them that executed a request terminated at a server curlew's own
suite had written — 23 files construct `httptest.NewServer`, and the smoke suite
runs a local echo fixture. A Go test server and a Go HTTP client agree by
construction about header canonicalisation, body framing, chunking and HTTP/2,
so every bug living in that shared reading was invisible to the whole suite.

Mudflat is a server curlew did not write.

## Run it

```bash
go build -o mudflat ./testapi/cmd/mudflat
./mudflat serve
```

Then, in another shell:

```bash
go build -o curlew ./cmd/curlew
./curlew run 'testapi/collections/*.yaml' --env local --var run=$(date +%s)
```

`./scripts/ci-local.sh` does all of this on port 18080 as a gate step, so the
dogfood run happens on every change rather than when someone remembers.

Two other commands:

```bash
./mudflat index          # the endpoint registry as JSON, no listener
./mudflat serve --help   # flags
```

## The `run` variable

Every session is namespaced by `--var run=<id>`. That makes two concurrent runs
independent (specification §6.2) and gives each run a clean namespace, while
keeping the session visible in terminal output, the `--events` stream and any
HTML report — so a failing request names its own session.

## Layout

```
testapi/
  cmd/mudflat/       the binary
  mudflat/           the server: session store, envelope, capture, endpoints
  collections/       the dogfood suite — these pass
  gaps/              requests that MUST fail — the server is deliberately broken
  environments/      local.yaml, pointing at 127.0.0.1:8080
  parity_test.go     §16: every endpoint is exercised, every URL resolves
```

## Two rules that keep this honest

**Nothing under `testapi/` may import curlew.** If curlew's own packages were
what proved mudflat correct, the closed loop this directory exists to break
would simply re-form from the other side. Enforced by
`TestParity_NoCurlewImportsUnderTestapi`.

**Every endpoint must be exercised by a collection, and every collection URL
must resolve to an endpoint.** Enforced by the other parity tests. An endpoint
nobody calls is deleted; a URL that hits nothing is a typo that would otherwise
surface as a 404 looking like a curlew bug. Both sides are derived from the
artefacts themselves rather than from a hand-maintained list.

## What Phase 1 covers

| Family | Endpoints | What it tests |
|---|---|---|
| A — echo | `/echo`, `/echo/status/{code}`, `/echo/delay/{ms}`, `/anything/…` | what curlew actually sent, with original header casing and order |
| B — status | `/status/{code}` | status assertions; 204/304 carry no body |
| C — bodies | `/encoding/…`, `/charset/…`, `/content-type/…`, `/json/…`, `/ndjson/{n}`, `/bytes/{n}`, `/empty` | the 13 body operators against documents that break naive parsing |
| H — resources | `/s/{sid}/resources…`, `/etag`, `/idempotency` | extraction, chaining, pagination, conditional requests |
| I — failure injection | `/s/{sid}/flaky/…`, `/retry-after/…` | every retry trigger, deterministically |
| meta | `/`, `/capabilities`, `DELETE /s/{sid}` | self-description |

The echo envelope is the piece worth understanding: it reports headers as
**ordered pairs with the client's original casing**, duplicates as separate
entries, and the body as **base64 that is never decoded**. Each of those refuses
a convenience that would hide the class of bug the endpoint exists to find. See
specification §8.

## What it found on the first run

Three defects, all reproducible, **all now fixed**. The requests that reproduced
them live in the passing collections; the write-ups are in specification §11A.

1. **Assertion expected values were never interpolated.** `equals: "{{var}}"`
   compared against the literal template. The same variable interpolated
   correctly in a URL, which is what made it confusing in practice — a request
   could fetch exactly the right resource and then fail to assert anything about
   it. Fixed in `internal/requtil`; covered by `10-assertions.yaml` and
   `20-extraction.yaml`.
2. **Header `exists: false` was not honoured**, so header *absence* could not be
   asserted at all, although `docs/CLI_SPECIFICATION.md` §7.2 documents it and
   the body path had a working `not_exists`. Fixed on both paths in
   `internal/assertion`; covered by `10-assertions.yaml`.
3. **Body-read failures were labelled and classified wrongly, in opposite
   directions.** A lying `Content-Encoding` was called a network error though no
   retry can fix it — *and* nothing read off a body was classified as a
   `*errors.NetworkError` at all, so a connection dying mid-response never
   triggered `retry_on.network_errors` either. Fixed in `internal/httpexec`;
   covered by `gaps/expected-failures.yaml` and the package's own tests.

The third is the one worth dwelling on: measuring it contradicted the first
write-up, which had assumed from the "network error" label that the response was
being retried. It was not, and finding out why turned up the second half of the
defect.

## Safety

Mudflat binds loopback. `--bind-unsafe` is required to bind anything else and
prints a warning: later phases serve deliberate protocol violations and ship
fixed credentials, and none of that belongs on a network.
