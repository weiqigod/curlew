# testapi — Mudflat

A deliberately difficult HTTP server, and the dogfood suite that runs curlew
against it.

Full design: [`docs/TESTAPI_SPECIFICATION.md`](../docs/TESTAPI_SPECIFICATION.md).
**Phases 1 and 2 are implemented.** Phase 3 is specified but not built; `GET
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
  cmd/mudflat/           the binary — two listeners, structured and raw
  mudflat/               the server: sessions, envelope, capture, endpoints,
                         the raw byte layer, signature verification
  collections/           the dogfood suite — these pass
  collections/parallel/  requires --parallel; the barrier cannot pass serially
  gaps/                  requests that MUST fail; *.parse-fail.yaml must not parse
  golden/                byte-exact raw transcripts, hand-reviewed against the RFCs
  harness/               assertions no collection can express (see below)
  environments/          local.yaml, pointing at 127.0.0.1:8080 and :8081
  parity_test.go         §16: every endpoint is exercised, every URL resolves
```

## The two listeners

`mudflat serve` opens the structured layer on `--port` and the **raw adversarial
layer** on `--port + 1`. They cannot share a server: the raw layer's responses
are things `net/http` will not emit — a `Content-Length` that disagrees with its
body, a chunk size that is not hex, a NUL inside a header value — so they are
written as literal bytes to a socket with no HTTP library involved.

That is also the one place where the language mudflat is written in stops
mattering (specification §3).

## The harnesses

Each asserts something a collection cannot, and all three run in `ci-local.sh`:

| Harness | Asserts |
|---|---|
| `gaps.sh` | Every request under `gaps/` still fails. An **unexpected pass** fails the harness — that is what makes the directory shrink. |
| `redaction.sh` | No published secret reached any output artefact. Gates on a baseline of known leaks; a new leak fails, and a *fixed* leak fails too. |
| `crosscheck.sh` | curl reads the raw layer the way the specification intends — independent evidence that the malformations are real rather than Go being strict. |

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

## What Phase 2 adds

| Family | Endpoints | What it tests |
|---|---|---|
| E — raw framing | `/raw/…` on port +1 | malformed framing, chunking and header syntax that no HTTP library can produce |
| G — signatures | `/verify/sigv4`, `/verify/oauth1` | `internal/signer`, recomputed and returned as a staged diff |
| J — concurrency | `/s/{sid}/barrier/{n}`, `/concurrency`, `/serialize`, `/ratelimit` | `--parallel` as a rendezvous proof, and a limiter that actually enforces |
| O — redaction bait | `/leak/…` | realistic secrets for the redaction harness to hunt for |

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

## What Phase 2 found

Two more, both reproducible and **both now fixed** — write-ups in
specification §11B:

1. **The object form of `extract:` did not parse.** `CLI_SPECIFICATION` §8 opens
   with it. `Extract` was `map[string]string`, so it could not. That form is the
   only way to declare sensitivity explicitly, and the name of an extracted field
   is whatever the API under test calls it. Fixed in `internal/parser`; covered
   by `70-redaction.yaml`, which now uses it for the two values the name
   heuristic cannot reach.
2. **Redaction covered the request but not the response.** The same sensitive
   value was `[REDACTED]` on the way out and verbatim on the way back —
   13 measured leaks. The security-relevant one: an API that echoes a token puts
   it in a CI log. Four separate holes, fixed across `internal/runner`,
   `internal/variable` and a new `internal/runservice/redact.go`.

   `harness/redaction-known-leaks.txt` is now empty, and that is the interesting
   part. The harness fails when a *listed* leak stops leaking, which is what
   forced all thirteen lines out in one go instead of leaving a baseline nobody
   prunes. Every surface is enforced.

   Fixing it also exposed a hole in the harness: terminal, TAP, JUnit and JSONL
   had looked clean only because every assertion in `70-redaction.yaml` passes,
   so no `actual` value was ever printed to them. `harness/redaction-actual.yaml`
   — every assertion wrong on purpose — closes that.

And two affirmative results, which are worth as much: curlew's **SigV4 and
OAuth 1.0a signatures are correct**, and **`--parallel` really is parallel** —
both now backed by evidence rather than by absence of evidence.

## What Phase 3 found

Ten more, **none fixed** — each is an executable reproduction, and five results
came out affirmative. Write-ups in specification §11C.

The ones that would change how you use curlew today:

1. **A non-JSON GraphQL response aborts the whole run.** A gateway's HTML error
   page discards every result in the collection, reports `"requests": []` beside
   a summary claiming six passed, and exits 5 — the code §17 assigns to variable
   resolution errors.
2. **`timing.max_duration_ms` cannot fail on a slow body.** The reported
   duration stops when the headers arrive, so a one-second stream reports 0ms
   and a 50ms ceiling passes. curlew measures the right number in the same
   request — `timing.total_us` is there in the event stream — and reports the
   wrong one.
3. **A response body that is not JSON cannot be asserted on at all.** Not by
   `body:`, which is documented, and not by `cel:` either, which is not: the
   body decode is best-effort and leaves `response.body` null.
4. **A WebSocket heartbeat reports a healthy peer as dead** whenever no step is
   reading — which is when a heartbeat is for.
5. **The OpenAPI importer rejects ordinary 3.1 documents** (`info.summary`,
   `webhooks`, `type: ["string","null"]`) although both documents promise 3.x.

And the affirmatives: close codes are surfaced, curlew answers protocol pings, a
genuinely dead peer is detected, a TLS failure is legible and exits 4, and the
OpenAPI round trip passes 8 of 8 against the server that served the document.

## What is deliberately not built

- **Eight of the nine TLS postures.** `SSL_CERT_FILE` was measured — not assumed
  — not to override the platform verifier on macOS, and curlew has no CA option,
  so every posture produces one identical "unknown authority". Eight endpoints a
  client cannot tell apart are what §16 deletes. See §14.3.
- **The nginx cross-check.** curl already reads the raw layer independently and
  gorilla reads the WebSocket layer independently; nginx would add a container to
  re-answer a question two cheaper checks already answer.

## Safety

Mudflat binds loopback. `--bind-unsafe` is required to bind anything else and
prints a warning: later phases serve deliberate protocol violations and ship
fixed credentials, and none of that belongs on a network.
