# Mudflat — Test API Specification

## A Deliberately Difficult HTTP Server for Exercising Curlew

**Version:** 0.3
**Status:** Phases 1, 2 and 3 implemented (`testapi/`), with §9.N reduced — see §14.3
**Date:** 2026-08-12
**Applies to:** curlew 0.1.0-dev and later

> *A curlew feeds by probing soft ground for what it cannot see. This is the
> ground.*

The name is a placeholder chosen for memorability and namespace clarity;
substitute freely. Everything below is independent of it.

---

## Table of Contents

1. [Purpose](#1-purpose)
2. [Non-Goals](#2-non-goals)
3. [The Closed-Loop Problem](#3-the-closed-loop-problem)
4. [Design Principles](#4-design-principles)
5. [Architecture](#5-architecture)
6. [Invariants](#6-invariants)
7. [Session Model](#7-session-model)
8. [The Echo Envelope](#8-the-echo-envelope)
9. [Endpoint Families](#9-endpoint-families)
10. [Coverage Matrix](#10-coverage-matrix)
11. [Gaps This API Will Expose](#11-gaps-this-api-will-expose)
11A. [What Phase 1 Actually Found](#11a-what-phase-1-actually-found)
11B. [What Phase 2 Found](#11b-what-phase-2-found)
11C. [What Phase 3 Found](#11c-what-phase-3-found)
12. [The Dogfood Suite](#12-the-dogfood-suite)
13. [Testing the Tester](#13-testing-the-tester)
14. [Operations](#14-operations)
15. [Integration](#15-integration)
16. [The Anti-Bloat Rule](#16-the-anti-bloat-rule)
17. [Implementation Decisions](#17-implementation-decisions)
18. [Delivery Phases](#18-delivery-phases)
19. [Conformance](#19-conformance)
- [Appendix A — Port Map](#appendix-a--port-map)
- [Appendix B — Fixed Test Credentials](#appendix-b--fixed-test-credentials)
- [Appendix C — Deliberately Absent](#appendix-c--deliberately-absent)

---

## 1. Purpose

Curlew has 1,963 test functions and 88.3% statement coverage. Every one of those
tests that executes an HTTP request terminates at a server curlew's own test
suite wrote — 23 files construct `httptest.NewServer`, and the smoke suite runs a
local Python echo server on `127.0.0.1:9190`. The handful of `httpbin.org` URLs
in `testdata/` sit in parse and validate paths that never dial; one of them is
named `Should Not Run`.

Mudflat exists to give curlew a server it did not write.

Its job is not to be a realistic API. It is to be a **hostile, exhaustive, and
perfectly reproducible** one: an HTTP surface that produces every response shape,
encoding, timing profile, failure mode, and protocol violation that curlew claims
to handle, plus a substantial number it does not, so that the difference becomes
visible.

Three concrete outcomes define success:

1. **Dogfooding becomes a gate, not an intention.** `curlew run` against Mudflat
   runs in `ci-local.sh` on every change.
2. **Claims become verified.** `internal/signer` currently emits AWS SigV4 and
   OAuth 1.0a signatures that no verifier has ever checked. Mudflat verifies them
   and reports *where* a mismatch occurs.
3. **Missing capability becomes visible and tracked.** Endpoints curlew cannot
   yet handle are shipped as executable, expected-to-fail entries (§12.3) rather
   than as prose in a backlog.

---

## 2. Non-Goals

Mudflat is **not**:

- **A production API, or a model of one.** It has no domain, no business logic,
  and no user-facing purpose. Resource endpoints (§9.G) exist only because
  variable chaining needs an ID that came from somewhere.
- **A general-purpose httpbin replacement.** Endpoints are justified by a curlew
  behavior they exercise. The rule in §16 deletes any that are not.
- **A load-test target for capacity work.** It supports perf runs (§9.J) to prove
  curlew's `perf` command measures what it claims, not to benchmark the server.
- **A public service.** It binds loopback by default, ships fixed test
  credentials (Appendix B), and must never be exposed to a network. See §14.5.
- **A replacement for unit tests.** `httptest` remains correct for testing a
  function in isolation. Mudflat tests the binary against a socket.

---

## 3. The Closed-Loop Problem

This section is the reason the rest of the document is shaped as it is.

A test suite proves that the code behaves correctly against the inputs the suite
supplies. When the suite authors those inputs using the same assumptions the code
holds, the assumptions themselves become untestable. Curlew's current suite is in
exactly that position: `httptest.NewServer` is Go's `net/http` server, and curlew
is Go's `net/http` client. They agree, by construction, about:

- header name canonicalization (`content-type` → `Content-Type`)
- when to use `Content-Length` versus chunked transfer encoding
- how to frame a body, and what a well-formed chunk looks like
- HTTP/2 negotiation, connection pooling, and idle-connection lifetime
- what constitutes a valid status line

A Go server will never emit a response a Go client mishandles, because the same
maintainers wrote both to the same reading of the same RFCs. Every bug that lives
in that shared reading is invisible to the entire existing suite.

**Writing Mudflat in Go re-imports this problem.** The specification addresses it
in three ways, in descending order of importance:

1. **The adversarial layer bypasses `net/http` entirely** (§5.2). Malformed
   responses are written to a raw `net.Conn`. No HTTP library is involved on the
   server side, so no library's opinions constrain the bytes. This is where
   language choice stops mattering — you are writing bytes.
2. **Golden byte transcripts** (§13.2). Every endpoint's wire output is captured
   and version-controlled as literal bytes, reviewed once by hand against RFC
   text, and thereafter asserted by a raw-socket client. The server cannot drift
   without a reviewer seeing the diff.
3. **An independent cross-check** (§13.3). A defined subset of well-behaved
   responses is also served by stock nginx from static fixtures. Curlew must
   produce identical results against both. A behavior that only works against the
   Go implementation fails here.

**Residual risk, stated plainly.** After all three mitigations, the well-formed
responses from the structured layer are still generated by Go's `net/http`. The
cross-check in §13.3 covers a subset, not the whole surface. This is a real
limitation of the design and it is accepted deliberately, because the alternative
— a second language toolchain in a repository that already carries Go, .NET, and
Node — costs more than it returns. Anyone extending Mudflat should know that the
structured layer is the weakest part of the independence argument, and should
prefer to add a new case to the raw layer when the choice is available.

---

## 4. Design Principles

**P1 — Reproducibility is a hard invariant, not a goal.**
Two runs of the dogfood suite against a fresh server produce byte-identical
results, modulo a named and enumerated set of time-varying fields (§6.1). Every
non-deterministic behavior — flakiness, latency, failure — is *requested by URL
and seeded*, never ambient. A test API that is itself flaky teaches nothing;
it just relocates the doubt.

**P2 — The response reports what was received, not what the server understood.**
Header names appear in their original casing and original order. Duplicates
appear as separate entries. Bodies are returned base64-encoded, never decoded and
re-encoded. Every lossy convenience — a header map, a parsed JSON body — hides
exactly the class of bug the server exists to find. See §8.

**P3 — Every endpoint cites a curlew behavior.**
An endpoint's documentation names the feature it exercises and what a wrong
implementation would look like when it hits that endpoint. An endpoint that
cannot answer "what would break" is deleted (§16).

**P4 — Failure modes are first-class, not edge cases.**
Roughly half the surface exists to fail: malformed framing, lying headers, resets
mid-body, expired certificates, servers that never answer. These are the
endpoints with the highest yield per line, because they are the ones no
`httptest` handler has ever produced.

**P5 — State is scoped or absent.**
Stateless endpoints work without ceremony. Stateful endpoints live under a
session prefix and share nothing. There is no global mutable state, so parallel
runs cannot contaminate each other (§7).

**P6 — The server is hermetic.**
No network access at run time, no external services, no clock dependency beyond
explicitly requested delays, no filesystem writes outside a temp dir. It starts
in under 200 ms and can run in the smoke suite.

**P7 — Gaps are executable.**
When curlew cannot handle an endpoint, the dogfood suite still contains it,
marked `gap`. An unexpected pass is a failure (§12.3), so a capability that
arrives cannot arrive silently.

---

## 5. Architecture

Three layers, distinguished by how much machinery sits between the endpoint logic
and the socket.

### 5.1 Structured layer

Standard `net/http` handlers. Serves everything well-formed: status codes,
redirects, content encodings, JSON bodies, auth challenges, resource CRUD,
rate limiting, GraphQL, WebSocket, SSE.

Two capabilities must be threaded down from the connection, because
`http.Request` has already destroyed them by the time a handler runs:

- **Original header casing and order.** Captured via a `net.Conn` wrapper that
  tees the request preamble before it reaches the parser, keyed by connection and
  request index.
- **Connection identity.** Assigned in `ConnContext`, exposed as
  `X-Mudflat-Conn` and `X-Mudflat-Conn-Reqs` so connection reuse is observable
  from the client side.

### 5.2 Raw layer

A `net.Listener` accept loop that reads the request preamble far enough to route,
then writes a response as literal bytes. **No HTTP library.** This layer produces
what `net/http` structurally cannot:

- `Content-Length` that disagrees with the body, in both directions
- `Content-Length` and `Transfer-Encoding: chunked` together
- malformed chunked framing — bad size lines, absent terminator, oversized chunk
- bare-LF line endings, absent status line, HTTP/0.9-style bodies
- duplicate `Content-Length` with conflicting values
- header names containing control characters or NUL
- 8,000-byte header values; 1,000 headers
- a connection reset after N bytes of body
- trailing garbage after a complete response

### 5.3 Protocol layer

Endpoints that are not request/response: WebSocket (§9.L), Server-Sent Events and
chunked streaming (§9.M). Separated because their lifecycle and failure modes do
not fit the other two layers' shapes.

### 5.4 TLS variants

One listener per certificate posture, on distinct ports (Appendix A). Ports
rather than SNI because SNI selection requires hosts-file entries, and curlew
exposes no `--resolve` equivalent. See §14.3 for the trust-anchor mechanism.

---

## 6. Invariants

Each is mechanically checkable and belongs in the server's own test suite.

### 6.1 Determinism

> A response is a pure function of `(method, path, query, request headers,
> request body, session state, seed)`.

Wall-clock time influences a response only where the endpoint's contract is about
time, and then only through a duration the caller supplied. Randomness requires
an explicit `seed` query parameter; the same seed yields the same sequence
forever.

The complete set of fields permitted to vary between two otherwise-identical runs:

| Field | Where | Why |
|---|---|---|
| `Date` | response header | HTTP requires it |
| `connection.id` | echo envelope | assignment order under concurrency |
| `connection.requests_on_conn` | echo envelope | pooling is the client's choice |
| `server.session` | echo envelope | session ids are caller-supplied or minted |
| `X-Mudflat-Conn`, `X-Mudflat-Conn-Reqs` | response header | as above |

Anything else varying between runs is a bug in Mudflat. A test asserts this by
running the dogfood suite twice and diffing the event streams with these fields
masked.

### 6.2 Isolation

No handler mutates state visible to another session. Two dogfood runs against the
same server instance, concurrently, produce identical results.

### 6.3 Hermeticity

Zero outbound connections. Zero filesystem writes outside `$TMPDIR`. No
dependency on a system clock beyond monotonic durations. Startup to first
accepted connection under 200 ms.

### 6.4 Boundedness

Every endpoint that can delay, stall, or hang is bounded server-side by a hard
ceiling of **120 seconds**, after which the connection is closed. Without this, a
curlew regression that ignores a timeout wedges CI indefinitely rather than
failing. The ceiling is deliberately far above any legitimate test duration.

### 6.5 Self-description

- `GET /` — index of every endpoint with its contract and the curlew feature it
  exercises, as JSON.
- `GET /openapi.json` — an OpenAPI 3.1 document describing the structured layer.
- `GET /capabilities` — which families are enabled in this instance, and why any
  are not (missing certs, no h2 support, etc.).

---

## 7. Session Model

Stateless endpoints are reachable bare:

```
GET /echo
GET /status/404
GET /encoding/gzip
```

Stateful endpoints require a session prefix:

```
GET  /s/{sid}/counter
POST /s/{sid}/resources
GET  /s/{sid}/flaky/fail-then-succeed?times=2
```

Rules:

- `{sid}` is any string of 1–64 characters from `[A-Za-z0-9_-]`. It is created
  implicitly on first reference. There is no registration call.
- All counters, resources, rate-limit buckets, token state, and barrier state are
  scoped to `{sid}`. Nothing is shared.
- `DELETE /s/{sid}` resets the session to empty and returns 204. Idempotent.
- Sessions expire 1 hour after last use. At most 1,000 concurrently; the
  least-recently-used is evicted beyond that, and eviction is reported in
  `/capabilities` so a suite that trips it can find out.
- `X-Mudflat-Session: {sid}` is accepted as an alternative to the path prefix,
  for cases where the path is fixed — notably the OpenAPI round-trip (§9.P),
  where the imported collection's paths come from the served document.

**Why path-first.** A session in the path is visible in terminal output, in the
`--events` NDJSON stream, and in an HTML report. When a dogfood test fails, the
failing line names its own session. A header would work and would be invisible.

Collections mint a session per run:

```yaml
variables:
  sid: "{{$faker.uuid}}"
  base: "{{mudflat_url}}/s/{{sid}}"
```

---

## 8. The Echo Envelope

The single most important response shape. Every echo-family endpoint returns it.

```json
{
  "request": {
    "method": "POST",
    "target": "/s/ab12/echo?x=1&x=2",
    "http_version": "HTTP/1.1",
    "headers": [
      ["User-Agent", "curlew/0.1.0"],
      ["content-type", "application/json"],
      ["X-Dup", "a"],
      ["X-Dup", "b"]
    ],
    "header_names_in_order": ["User-Agent", "content-type", "X-Dup", "X-Dup"],
    "query": [["x", "1"], ["x", "2"]],
    "body_base64": "eyJhIjoxfQ==",
    "body_sha256": "9d4e1e23bd5b727046a9e3b4b7db57bd8d6ee6841e4a6c04a0f0a4d3d6e2b7c15",
    "body_len": 8,
    "trailers": []
  },
  "connection": {
    "id": "c17",
    "requests_on_conn": 3,
    "tls": { "version": "1.3", "cipher": "TLS_AES_128_GCM_SHA256", "alpn": "h2" }
  },
  "server": {
    "name": "mudflat",
    "version": "0.1.0",
    "endpoint": "/echo",
    "session": "ab12"
  }
}
```

Four decisions, each of which exists to preserve information a convenient
representation would destroy:

**Headers are an array of pairs, not a map.** A map loses order and collapses
duplicates. Both matter: header order is how you detect that a signing
implementation canonicalized correctly, and duplicate handling is where
`Set-Cookie` and `X-Forwarded-For` bugs live. `header_names_in_order` is
redundant with `headers` but exists because JSONPath over an array of arrays is
awkward, and this is a test API — ergonomics for the assertion author is a
feature.

**The body is base64, never decoded.** Decoding and re-encoding launders exactly
the bugs worth finding: a body sent as UTF-16, a body with a stray BOM, a body
whose declared charset disagrees with its bytes. `body_sha256` and `body_len`
exist so an assertion can check the body without the collection author handling
base64.

**Query parameters are pairs too**, for the same reason as headers — repeated
keys are a real and commonly mishandled case.

**No timestamps.** The envelope carries no wall-clock field, which is what makes
byte-identical reruns possible (§6.1).

---

## 9. Endpoint Families

Sixteen families. Each entry gives the path, the behavior, the curlew feature it
exercises, and — the part that justifies its existence — what a wrong
implementation looks like when it arrives.

### A. Echo and identity

| Endpoint | Behavior |
|---|---|
| `/echo` | Envelope (§8) for any method |
| `/echo/status/{code}` | Envelope, returned with `{code}` |
| `/echo/delay/{ms}` | Envelope after a delay |
| `/anything/**` | Envelope regardless of path depth |

**Exercises:** request construction end to end — method, URL building, query
merging, header assembly, body serialization, variable interpolation, faker
output, request signing (as the signed bytes are visible), `exec --stdin`.

**Wrong looks like:** a header curlew claims to send that does not appear;
`{{var}}` unresolved in the body; query parameters from `query:` colliding with
ones already in the URL rather than merging; a `Content-Type` set when the user
set one explicitly.

### B. Status and redirection

| Endpoint | Behavior |
|---|---|
| `/status/{code}` | Any code 100–599, including 1xx informational and 418 |
| `/status/204`, `/status/304` | Bodiless by protocol rule |
| `/redirect/{n}` | `n` chained 302s, then 200 |
| `/redirect-to?url=&code=` | Single redirect with a chosen code |
| `/redirect/relative/{n}` | Relative `Location` values |
| `/redirect/loop` | Infinite self-redirect |
| `/redirect/method/{code}` | Redirects a POST; 303 must become GET, 307/308 must preserve method and body |
| `/redirect/cross-origin` | Redirects to a different port — `Authorization` must be dropped |
| `/redirect/downgrade` | HTTPS → HTTP |

**Exercises:** status assertions including the `[200, 201, 204]` any-of form;
redirect handling; the interaction between redirects and auth headers.

**Wrong looks like:** a 307 that arrives as a GET; an `Authorization` header
following a cross-origin redirect (a credential leak, not a correctness nit); a
redirect loop that hangs instead of erroring.

### C. Body encodings and content types

| Endpoint | Behavior |
|---|---|
| `/encoding/{gzip,deflate,br,zstd,identity}` | Correctly encoded |
| `/encoding/lying/{enc}` | Declares `Content-Encoding: {enc}`, body is plain |
| `/encoding/double` | `Content-Encoding: gzip, gzip` |
| `/charset/{utf-8,iso-8859-1,shift_jis,utf-16le-bom}` | Non-ASCII payload in each |
| `/content-type/{variant}` | `application/json`, `…;charset=utf-8`, `application/vnd.api+json`, `text/json`, `text/plain`, absent, and a lying `application/json` over non-JSON |
| `/json/{deep,bignum,dupkeys,unicode-escapes,empty,toplevel-array,toplevel-string,toplevel-null,nan}` | JSON edge cases |
| `/ndjson/{n}` | `n` NDJSON lines |
| `/bytes/{n}` | `n` deterministic pseudorandom bytes |
| `/empty` | 200 with zero-length body and no `Content-Type` |

`/json/bignum` returns integers beyond float64's exact range
(`9007199254740993`) and high-precision decimals. `/json/dupkeys` returns
`{"a":1,"a":2}` — legal JSON with implementation-defined resolution.

**Exercises:** every one of the 13 body operators, JSONPath extraction, the
`schema:` assertion, CEL expressions over the body, `type` assertions.

**Wrong looks like:** a large integer silently losing precision through a float64
round-trip, so `equals: 9007199254740993` passes against a different number; a
`Content-Type` that is not exactly `application/json` causing the body parser to
skip, so every body assertion silently reports "path not found" rather than
failing loudly; a lying `Content-Encoding` producing a confusing decode error
instead of a clear one.

### D. Timing and connection behavior

| Endpoint | Behavior |
|---|---|
| `/delay/{ms}` | Full response after a delay |
| `/slow-headers/{ms}` | Delay before the status line |
| `/drip?bytes=&ms=&chunks=` | Body trickled over time |
| `/stall/{ms}` | Headers, partial body, pause, then completion |
| `/stall-reset/{ms}` | Headers, partial body, pause, then RST |
| `/hang` | Accepts, never responds (bounded at 120 s per §6.4) |
| `/close-idle` | Responds, then closes the connection during the client's idle window |
| `/expect-100` | Requires a `100-continue` handshake |
| `/keepalive/{n}` | Closes after `n` requests on the connection |

**Exercises:** `timing.max_duration_ms`, retry on network errors and timeouts,
connection reuse under `perf` and `--parallel`, TTFB versus total duration in the
events stream.

**Wrong looks like:** a timing assertion that measures parse time rather than
wire time; a retry that does not fire on a mid-body reset because the error is
classified as a body error rather than a network error; and — see §11.1 —
`/hang` wedging the run forever, which is the current behavior.

### E. Adversarial framing (raw layer)

| Endpoint | Behavior |
|---|---|
| `/raw/content-length/over` | Declares more bytes than it sends |
| `/raw/content-length/under` | Declares fewer |
| `/raw/content-length/duplicate` | Two conflicting `Content-Length` headers |
| `/raw/content-length/with-chunked` | Both framing mechanisms at once |
| `/raw/chunked/bad-size` | Non-hex chunk size |
| `/raw/chunked/no-terminator` | Omits the final `0\r\n\r\n` |
| `/raw/chunked/trailers` | Legitimate trailers after the last chunk |
| `/raw/no-status-line` | Body with no status line |
| `/raw/bare-lf` | LF-only line endings throughout |
| `/raw/header/huge` | An 8 KB header value |
| `/raw/header/many` | 1,000 headers |
| `/raw/header/nul` | NUL byte in a header value |
| `/raw/header/no-colon` | Malformed header line |
| `/raw/trailing-garbage` | Valid response followed by junk |
| `/raw/reset-after/{n}` | RST after `n` bytes |
| `/raw/http09` | Bare body, HTTP/0.9 style |

**Exercises:** error classification and message quality. Nothing here should
produce a correct parse; the question is whether the failure is *legible*.

**Wrong looks like:** a panic; a hang; an error message that says `unexpected EOF`
without naming the request, the URL, or what was malformed. Per curlew's own
error-handling standard — wrap with `fmt.Errorf("context: %w", err)` — every one
of these should produce a traceable chain ending in a sentinel the user can act
on.

### F. Authentication

| Endpoint | Behavior |
|---|---|
| `/auth/basic` | HTTP Basic; 401 with `WWW-Authenticate` on miss |
| `/auth/bearer` | Bearer token |
| `/auth/apikey/{header,query,cookie}` | API key in three positions |
| `/auth/digest` | RFC 7616 digest, two-round-trip |
| `/s/{sid}/auth/token` | Issues a token with a **10-second TTL** |
| `/s/{sid}/auth/protected` | 401 once the token expires |
| `/s/{sid}/auth/refresh` | Exchanges a refresh token |
| `/auth/401-vs-403` | Distinguishes unauthenticated from unauthorized |
| `/s/{sid}/cookies/set` | `Set-Cookie` with domain, path, expiry, `SameSite`, `HttpOnly` |
| `/s/{sid}/cookies/require` | 403 unless the cookie is returned |

**Exercises:** dynamic auth profiles, `cache_ttl`, `refresh_on_failure`, vault
resolution (with `CURLEW_VAULT_STUB=1`), sensitive-value redaction.

**Wrong looks like:** `refresh_on_failure` not firing because the 401 arrives
with a body curlew classifies differently; a cached token reused past its TTL
because `cache_ttl` is measured from the wrong instant; a token appearing
unredacted in an HTML report. The 10-second TTL is chosen so expiry happens
*within* a normal suite run rather than needing a contrived wait.

### G. Signature verification

| Endpoint | Behavior |
|---|---|
| `/verify/sigv4` | Recomputes AWS SigV4 with the credentials in Appendix B |
| `/verify/oauth1` | Recomputes OAuth 1.0a |

On success: 200 with the canonical request the server derived. On failure: 401
with a structured diff —

```json
{
  "ok": false,
  "stage": "string_to_sign",
  "expected": "AWS4-HMAC-SHA256\n20260810T120000Z\n…",
  "received": "AWS4-HMAC-SHA256\n20260810T120000Z\n…",
  "first_difference_at": 47,
  "hint": "canonical_headers: signed header list does not match Authorization header"
}
```

**Exercises:** `internal/signer`, whose output nothing has ever checked.

**Wrong looks like:** anything. This family has no prior verification at all, so
its first run is the highest-information event in this specification. The staged
diff matters because a SigV4 mismatch is otherwise nearly undebuggable — knowing
whether the canonical request, the string-to-sign, or the signing key derivation
diverged is the difference between a ten-minute fix and an afternoon.

### H. Stateful resources

| Endpoint | Behavior |
|---|---|
| `POST /s/{sid}/resources` | Creates; returns a deterministic id derived from `(sid, sequence)` |
| `GET /s/{sid}/resources/{id}` | Fetches; 404 after delete |
| `PUT`, `PATCH`, `DELETE` | Update and remove |
| `GET /s/{sid}/resources?cursor=&limit=` | Cursor pagination with a `Link` header |
| `GET /s/{sid}/resources?offset=&limit=` | Offset pagination |
| `/s/{sid}/etag` | `ETag`; `If-None-Match` → 304, `If-Match` → 412 |
| `/s/{sid}/idempotency` | `Idempotency-Key` returns the first response |

Ids are deterministic — `res_{sid}_{n}` — so a failing assertion quotes a stable
value and a rerun reproduces it.

**Exercises:** extraction, variable chaining across request items, dependency
analysis under `--parallel`, phase ordering.

**Wrong looks like:** an extracted id visible to an item that should not yet see
it; two parallel items extracting the same name without the documented error;
`--parallel` reordering a create/fetch pair.

### I. Deterministic failure injection

| Endpoint | Behavior |
|---|---|
| `/s/{sid}/flaky/fail-then-succeed?times=N&status=503` | Fails `N` times, then 200 |
| `/s/{sid}/flaky/fail-on?attempts=1,3` | Fails on listed attempt numbers |
| `/s/{sid}/flaky/reset-then-succeed?times=N` | Network reset, then 200 |
| `/s/{sid}/flaky/timeout-then-succeed?times=N&ms=` | Hangs, then 200 |
| `/retry-after/seconds?n=` | 429 with `Retry-After: n` |
| `/retry-after/date?ms=` | 429 with an HTTP-date `Retry-After` |
| `/retry-after/huge` | `Retry-After: 3600` — must clamp to 30 s |
| `/retry-after/malformed` | `Retry-After: soon` |

Attempt counters are per `(session, endpoint, query)`. A fresh session restarts
the sequence, so a rerun is identical.

**Exercises:** the whole retry surface — `max_attempts`, all three backoff
strategies, `jitter_factor`, `respect_retry_after` and its 30-second cap,
`retry_on.status_ranges` in both `5xx` and `500-599` forms, `retry_on.methods`,
`do_not_retry_on` precedence, and the non-idempotent-method warning.

**Wrong looks like:** `do_not_retry_on` losing to `retry_on` when a status
appears in both — the spec says exclusion always wins; the `Retry-After: 3600`
case waiting an hour instead of clamping; a malformed `Retry-After` falling back
to no delay rather than to computed backoff.

Because backoff timing is observable through the events stream, this family also
verifies the backoff *table* in CLI_SPECIFICATION.md §9.3 rather than trusting it.

### J. Rate limiting and concurrency

| Endpoint | Behavior |
|---|---|
| `/s/{sid}/ratelimit?rate=&burst=` | Real token bucket with `X-RateLimit-*` headers; 429 on exhaustion |
| `/s/{sid}/barrier/{n}` | Blocks until `n` requests arrive concurrently, then releases all with arrival order |
| `/s/{sid}/concurrency` | Reports current and maximum observed concurrency |
| `/s/{sid}/serialize` | Rejects with 409 if two requests overlap |

The barrier is the important one. It converts "is `--parallel` actually
parallel?" from a timing heuristic into a positive proof: with `n` set to the
expected worker count, a serial implementation **times out**. A wall-clock
speedup comparison — which this repository has already had to remove once as
flaky (M21-003) — cannot make that claim.

**Exercises:** `--parallel` worker counts, the 20-worker data-driven cap,
`internal/ratelimit`, `perf --vus` and `--rps`.

**Wrong looks like:** `--parallel 8` producing a maximum observed concurrency of
1; `perf --vus 10` never reaching 10 concurrent; a client-side rate limiter that
does not actually throttle, visible as 429s the server should never have needed
to send.

### K. GraphQL

| Endpoint | Behavior |
|---|---|
| `/graphql` | Queries, mutations, variables, aliases, fragments |
| `/graphql` (partial) | **HTTP 200** with both `data` and `errors` populated |
| `/graphql` (errors only) | 200 with `data: null` and `errors` |
| `/graphql/introspection` | Full introspection response |
| `/graphql/http-error` | 500 with a non-GraphQL body |

**Exercises:** the `graphql` protocol adapter, `query_file`, `fragments`
concatenation, and `error_handling: fail|warn|ignore` against the partial-success
case.

**Wrong looks like:** a partial success treated as a pass because the HTTP status
was 200 — the trap this family exists for. Curlew's spec says `$.errors:
{ not_exists: true }` should be addressable; this proves it.

### L. WebSocket

| Endpoint | Behavior |
|---|---|
| `/ws/echo` | Echoes text and binary frames |
| `/ws/push?n=&ms=` | Server-initiated messages on a schedule |
| `/ws/ping?ms=` | Protocol pings; expects pongs |
| `/ws/no-pong` | Never answers a ping |
| `/ws/close/{code}` | Closes with a chosen code |
| `/ws/fragmented` | Multi-frame messages |
| `/ws/subprotocol` | Negotiates, and rejects an unoffered subprotocol |
| `/ws/reject` | Refuses the upgrade with 426 |
| `/ws/slow-accept?ms=` | Delays the handshake |

**Exercises:** all four step actions (`send`, `expect`, `wait`, `close`),
`any_of`, `count`, `timeout_ms`, reconnect with backoff, and both heartbeat modes
— protocol ping and data-frame.

**Wrong looks like:** `count: 3` satisfied by one message matching three times;
`/ws/no-pong` not triggering the reconnect path; a close code not surfaced.

### M. Streaming

| Endpoint | Behavior |
|---|---|
| `/sse?events=&ms=` | Server-Sent Events with ids and retry hints |
| `/sse/reconnect` | Drops, expects `Last-Event-ID` |
| `/stream/{n}` | `n` chunked JSON objects |
| `/stream/infinite` | Never ends (bounded at 120 s) |

**Exercises:** body handling under chunked transfer, memory behavior on unbounded
responses, whether a streaming body is buffered whole.

### N. TLS and protocol matrix

| Port | Posture |
|---|---|
| 8443 | Valid chain from Mudflat's own CA |
| 8444 | Expired leaf |
| 8445 | Self-signed |
| 8446 | Valid chain, wrong hostname |
| 8447 | Missing intermediate |
| 8448 | Client certificate required |
| 8449 | TLS 1.2 maximum |
| 8080 | Cleartext HTTP/1.1 |
| 8090 | h2c (cleartext HTTP/2) |

`/protocol` on any port returns the negotiated version, cipher, and ALPN.

**Exercises:** TLS handling, HTTP/2 support, and — see §11.3 — the absence of any
TLS configuration surface in curlew.

**Wrong looks like:** an expired certificate producing `connection refused`
rather than a certificate error; a hostname mismatch accepted.

### O. Redaction bait

| Endpoint | Behavior |
|---|---|
| `/leak/token` | Returns a realistic bearer token |
| `/leak/set-cookie` | Session cookie with a secret value |
| `/leak/pan` | A Luhn-valid test card number |
| `/leak/in-url` | Redirects to a URL with a token in the query string |
| `/leak/nested` | A secret at depth 6 in a JSON body |
| `/leak/header-echo` | Echoes the request's `Authorization` back |

**Exercises:** redaction across every output format — terminal, json, tap, junit,
html, markdown — and the `--events` NDJSON stream.

**Wrong looks like:** a secret redacted in terminal output but present in the
HTML report, or in `--events`, or in a `--report` file. Each format is a separate
code path and each is a separate opportunity to leak.

**Harness note.** This family is exercised by a shell harness that greps curlew's
output artifacts, not by a collection — the assertion is about curlew's output,
not about the response. See §12.4.

### P. OpenAPI round-trip

`GET /openapi.json` serves an OpenAPI 3.1 document describing the structured
layer, hand-written and version-controlled — **not generated from the handlers**,
because a generated document would make the round-trip a tautology.

The acceptance criterion:

```bash
curlew import openapi docs/mudflat-openapi.json -o /tmp/imported.yaml
curlew run /tmp/imported.yaml          # must pass
```

**Exercises:** `import openapi` — header generation, request body synthesis from
schemas, status assertions — verified by execution rather than by golden-file
comparison.

**Wrong looks like:** an import that produces a syntactically valid collection
which fails on contact with the server it was generated from. A golden-file test
cannot detect this; only running it can.

---

## 10. Coverage Matrix

Curlew feature → families that exercise it. Features with **no real-server
coverage today** are marked ●.

| Feature | Families | Today |
|---|---|---|
| Status assertions | B | httptest |
| Header assertions (3 operators) | A, C, F | httptest |
| Body assertions (13 operators) | C, H | httptest |
| Timing assertions | D | httptest |
| Schema assertions | C | httptest |
| CEL assertions | C, K | httptest |
| JSONPath extraction | H, K | httptest |
| Variable chaining | H | httptest |
| Faker + `--seed` + `--locale` | A | unit only |
| Retry — all strategies | I | httptest |
| `Retry-After` + 30 s clamp | I | ● |
| `do_not_retry_on` precedence | I | ● |
| Rate limiting | J | ● |
| Parallel execution | H, J | timing heuristic |
| Data-driven | H, I | httptest |
| Dynamic auth profiles | F | httptest |
| Token expiry / `refresh_on_failure` | F | ● |
| Vault resolution | F | stub only |
| **Request signing (SigV4, OAuth1)** | **G** | **● none** |
| GraphQL partial success | K | httptest |
| WebSocket, all step actions | L | httptest |
| WebSocket reconnect + heartbeat | L | ● |
| Redirects | B | ● |
| Content encodings | C | ● |
| Charsets / non-UTF-8 | C | ● |
| Malformed framing | E | ● |
| TLS postures | N | one listener; the rest blocked (§14.3) |
| HTTP/2 | N | negotiated over TLS; h2c unreachable (§14.3) |
| Connection reuse | D, J | ● |
| Streaming / SSE | M | mudflat |
| Redaction across 6 formats | O | partial |
| `import openapi` | P | round trip against mudflat |
| `perf` VU/RPS accuracy | J | ● |
| Events stream v1.6 fidelity | all | httptest |
| Error message quality | E | ● |

Twenty of thirty-six rows have no coverage against a server curlew did not write.

---

## 11. Gaps This API Will Expose

These are not speculative. Each was verified in the source while writing this
document, and each will surface the moment the corresponding endpoint is run.
They are listed here so the first dogfood run is not mistaken for a set of new
regressions.

### 11.1 No request timeout

`internal/httpexec/executor.go:83` calls `http.DefaultClient.Do`. Go's
`DefaultClient` has `Timeout: 0` — no timeout at any layer. Against `/hang`,
curlew waits forever.

`timing.max_duration_ms` does not help: it is an assertion evaluated after a
response arrives, so it never runs. There is no flag, no collection field, and no
project setting that bounds a request.

The 120-second server-side ceiling (§6.4) exists precisely so this gap does not
wedge CI while it is open.

**Endpoints:** `/hang`, `/slow-headers/{ms}`, `/stall/{ms}`.
**Status:** `gap`.

### 11.2 No redirect policy control

`http.DefaultClient` follows up to 10 redirects and drops `Authorization` on a
cross-host hop. Both are Go's defaults, neither is curlew's decision, and neither
is configurable or documented. `/redirect/loop` produces Go's
`stopped after 10 redirects` error, which names no request.

**Endpoints:** the whole B family.
**Status:** `gap` for configurability; `pass` expected for the default behaviors.

### 11.3 No TLS configuration

No `--insecure`, no custom CA option, no client-certificate support, no minimum
version. Curlew uses the system trust store. This makes ports 8443–8449 reachable
only via the `SSL_CERT_FILE` mechanism in §14.3, and makes port 8448 (mTLS)
untestable until a capability exists.

**Endpoints:** N family.
**Status:** `gap` except 8444/8445/8446, which should correctly fail closed.

### 11.4 No proxy support and no HTTP version selection

Neither `HTTP_PROXY` handling nor a way to force HTTP/1.1 versus HTTP/2 is
exposed. Port 8090 (h2c) is unreachable without prior knowledge.

**Status:** `gap`, low priority — noted for completeness.

### 11.5 Unverified signatures

§9.G. Not a known defect — a known absence of evidence. The signer may be
perfectly correct; nothing has ever checked.

---

## 11A. What Phase 1 Actually Found

The four gaps above were predicted from reading the source. These three were
not: they came out of the first run of the dogfood suite, which is the return on
building it.

**All three are now fixed.** The requests that reproduced them have moved into
the passing collections, which is where a closed gap belongs — §11A.1 and §11A.2
are asserted by `10-assertions.yaml` and `20-extraction.yaml`, and §11A.3 keeps
an entry in `testapi/gaps/expected-failures.yaml` because a lying
Content-Encoding can never produce a passing request.

Two of them were places where `docs/CLI_SPECIFICATION.md` documented behaviour
the binary did not have — the more interesting category, because a specification
that is wrong about the shipped tool is worse than one that is silent.

### 11A.1 Assertion expected values are never interpolated

`equals: "{{var}}"` compares the response against the literal template text, for
collection variables and for extracted values alike, on both the header and body
paths.

What makes this hard to diagnose from the outside is that the *same* variable
interpolates correctly everywhere else. A request can extract an id, use it to
build a URL, fetch exactly the right resource — and then fail to assert anything
about it:

```
✓ create                       201
✗ fetch it by the extracted id 200
    ✗ body $.id equals: expected {{rid}}, got res_r9-interp_1
```

The URL used `{{rid}}` and hit the right resource. The assertion did not.

**Impact.** Every assertion written against a configured or extracted value
silently compares against a template string. The failure reads as a server
problem.
**Endpoints:** any; `/content-type/{variant}` and the resource family reproduce
it in two lines.
**Fixed.** `requtil.ToHeaderInputs` and `ToBodyInputs` now take the scope and
interpolate. Body values go through `InterpolateBody`, which walks strings
wherever they appear — including inside the map an operator like `in_range`
takes — and leaves every other type alone, so an expected integer is never
routed through a string round trip. An unresolvable reference is an error, the
same as at every other interpolation site.

### 11A.2 Header absence cannot be asserted

`headers: { X-Thing: { exists: false } }` behaves identically to `exists: true`:
an absent header reports `expected exists, got header not present`, which is the
condition the assertion asked for.

§7.2 of the CLI specification documents the operator as "Presence (`true`) or
absence (`false`)". The body path has a working `not_exists` operator, so the
capability exists on one path and is missing on the other.

**Impact.** "This response must not carry `Set-Cookie`" is unexpressible. That
is a security-relevant assertion, and the redaction work in family O will want
it.
**Endpoint:** `/content-type/plain`.
**Fixed.** `exists` now reads its boolean on both the header and the body path,
so `exists: false` and `not_exists: true` are two spellings of one intent
instead of disagreeing. A value that is not a recognisable boolean keeps the
historical meaning — assert presence — so no collection that was passing can
start failing on a value nobody intended as a boolean.

### 11A.3 A body that fails to decode is reported as a network error

`/encoding/lying/gzip` declares `Content-Encoding: gzip` and sends plain bytes.
curlew reports:

```
network error: reading response body: gzip: invalid header
```

The message is legible. The classification is wrong, and the difference is
load-bearing: with `retry_on.network_errors: true` curlew retries a response
that can never decode, spending every attempt before failing with the same
error. A network error may succeed on retry; a lying content-encoding header
will not.

**Impact.** A misleading error class in the events stream — and, once measured,
something worse in the other direction.

**Correction to the original finding.** The first write-up of this said curlew
retried the undecodable response and burned every attempt. It does not, and the
reason it does not is a second defect: the body-read path returned a plain
wrapped sentinel, not a `*errors.NetworkError`, and `ClassifyForRetry` keys on
the latter. So *nothing* read off a response body was ever classified for retry.
A connection that dies mid-body — a genuine network failure that a retry may
well fix — did not trigger `retry_on.network_errors` either.

The label was wrong in one direction and the classification in the other.

**Fixed.** `httpexec.classifyBodyError` splits the two cases. A decode failure
(`gzip.ErrHeader`, `gzip.ErrChecksum`, `flate.CorruptInputError`) returns the
new `ErrDecode` sentinel with a message naming the header responsible, and no
retry rule matches it. Everything else is classified as a `*errors.NetworkError`
so `retry_on.network_errors` fires, which it never did before.
`io.ErrUnexpectedEOF` is deliberately on the network side: a compressed stream
that stops early almost always means the connection died.
**Endpoint:** `/encoding/lying/{enc}`.

---

## 11B. What Phase 2 Found

Two more, both the same shape as §11A: the specification documents behaviour the
binary does not have.

**Both are now fixed.** They are described below in the present tense of the
defect, because that is the record of what dogfooding bought; each subsection
ends with what closed it and what holds it closed.

### 11B.1 The object form of `extract:` does not parse

§8 of `docs/CLI_SPECIFICATION.md` opens with this example:

```yaml
extract:
  user_id: "$.id"
  api_key:
    path: "$.key"
    sensitive: true
```

The parser rejects it — `cannot unmarshal !!map into string` — because
`parser.RequestItem` declares `Extract` as `map[string]string`. Only the string
form exists.

**Impact.** The object form is the only way to declare sensitivity explicitly.
Without it a value is sensitive only if its *name* happens to match the §6.5
heuristic, which is not something a collection author can always arrange: the
field is named by the API being tested.

**Fixed** in `internal/parser`. `ExtractSpec` accepts both forms and rejects an
object with no `path` or an unknown key — a misspelled `sensitiv: true` that
parsed silently would leave a value unredacted while its author believed the
opposite. The published JSON Schema accepts both forms too, so an editor no
longer flags a valid collection.

**Now covered by** `testapi/collections/70-redaction.yaml`, which uses the object
form for the two values whose names the heuristic cannot reach, and
`internal/schema/validate_test.go`. The parse-fail reproduction is gone: it
parses.

### 11B.2 Redaction covers the request but not the response

A value curlew has marked sensitive is replaced where curlew *sent* it and
printed verbatim where the server *returned* it:

```
> Authorization: [REDACTED]
✗ body $.authorization equals: expected …, got Bearer SENTINELVALUE123
```

Both lines are from the same run, and `my_secret_token` matches the §6.5 name
heuristic twice over.

§6.5 says the value is replaced in terminal output, JSON, TAP, JUnit, HTML,
Markdown, event streams and JSONL logs. It does not restrict that to
request-side occurrences.

**Impact.** An API that echoes a token, a `Set-Cookie` carrying a session, or a
redirect with a token in its query puts the secret straight into a CI log. This
is the security-relevant one.

**Measured surfaces.** `testapi/harness/redaction-known-leaks.txt` recorded 13
concrete leaks across the JSON, Markdown and event-stream outputs. The harness
gates on that baseline: a leak outside it fails, and a baseline entry that stops
leaking *also* fails, so a fix forces the line out rather than leaving a
permanent excuse. It did exactly that — all thirteen went in one commit, and the
file is now empty.

The other surfaces were absent from the baseline because the assertions in
`70-redaction.yaml` all pass, so no `actual` value was ever printed — not because
they were safe. That was a hole in the harness, not evidence about curlew, and
`testapi/harness/redaction-actual.yaml` closes it: every assertion in it is wrong
on purpose, which is the only way terminal, TAP, JUnit and JSONL are handed a
response value at all. It runs only under `redaction.sh`, because the dogfood
gate requires `collections/*.yaml` to pass.

**Fixed** in four places, because it was four defects wearing one coat:

| What was wrong | Where |
|---|---|
| An extracted value was never registered as a sensitive *value*, so a token pulled out of a response was redacted nowhere | `internal/runner`, `internal/parallel` — `variable.MarkExtractedSensitive` at every extraction site |
| An assertion's expected and actual strings were never redacted at all — the one surface whose whole job is to print the value that did not match | `internal/runservice/redact.go`, applied by `cmd/curlew` and the events sink |
| Response headers were never redacted, and `Set-Cookie` was not inherently sensitive although `Cookie` was | `internal/variable` |
| A secret that lived only in a URL query string survived, because no body carried it | `internal/runservice/redact.go` |

The runner now takes its runtime sensitive set from the caller when one is
watching the run — the `--events` sink redacts each event as it is emitted, so a
token extracted at request 1 has to be known before request 1's own response body
reaches the stream. A set handed back at the end is too late.

**Now covered by** `testapi/harness/redaction.sh` across nine artefact
directories, `cmd/curlew/redaction_response_test.go`, and the inverted request in
`testapi/gaps/expected-failures.yaml` — whose assertion still fails on purpose,
but whose actual value must now read `Bearer [REDACTED]`.

---

## 11C. What Phase 3 Found

Ten more, and five affirmative results. The pattern from §11A and §11B holds:
most of these are places where a document describes behaviour the binary does
not have, which stays the more interesting category, because a specification
that is wrong about the shipped tool is worse than one that is silent.

**All ten are now fixed** (2026-08-13). Each entry below keeps the finding as it
was written, because the reproduction is the record, and adds what changed. An
eleventh defect surfaced during the work and is recorded as §11C.11.

Every fix moved its reproduction rather than deleting it. A gap that closes
leaves behind the test that proves it stayed closed:

| Finding | Where the evidence lives now |
|---|---|
| §11C.1 | `expected-failures.yaml` — the request still fails, and the run no longer dies with it |
| §11C.2 | `90-graphql.yaml` — full failure under `ignore` and `warn`, both passing |
| §11C.3 | `internal/parser/manual_examples_test.go` — every manual example must parse |
| §11C.4 | `expected-failures.yaml`, inverted: the message must now name the status |
| §11C.5 | `91-websocket.yaml` — the B case, beside the A it used to contradict |
| §11C.6 | `expected-failures.yaml` — a 50ms ceiling against a 1s stream must FAIL |
| §11C.7 | `92-streaming.yaml` passing; a structural operator still failing in gaps |
| §11C.8 | `internal/websocket` unit tests, array-under-count vs plain-under-default |
| §11C.9, §11C.10 | `testapi/harness/openapi.sh`, inverted from "must be rejected" |

Three were originally recorded by harnesses rather than by gap requests. A gap
request has to *fail*, and §11C.6, §11C.9 and §11C.10 were about a wrong thing
**passing** or about a command rather than a response — so there was no failing
request to record, and the claim was about curlew's own output. §11C.6's harness
is gone, promoted into a collection exactly as its own header instructed;
§11C.9 and §11C.10 keep theirs, inverted.

### 11C.1 A non-JSON GraphQL response aborts the whole run

`/graphql/http-error` answers 500 with an HTML error page — a gateway that never
reached the GraphQL service, which is the ordinary real-world case and the exact
shape §9.K predicted.

Failing that request is right. Taking the rest of the run with it is not:

- The run **aborts**. `--format json` reports `"requests": []` beside
  `"summary": {"total": 8, "passed": 6}` — the two contradict each other, and
  six passing requests vanish from the report.
- The **exit code is 5**, which `CLI_SPECIFICATION` §17 assigns to "variable
  resolution error: undefined variable, circular reference, bad interpolation".
  CI reading that triages a product problem as a pipeline misconfiguration.
- The **message is doubled**: `parsing graphql response: parsing graphql
  response: invalid character '<'`.

**Reproduction:** `testapi/gaps/graphql-http-error.run-abort.yaml`, which needed
a new harness category — the defect destroys the per-request output `gaps.sh`
reads, so an unmodified check reports "nothing evaluated" and blames itself.

**Fixed.** The parse failure now fails that request and the run continues:
`internal/runner` records it as a `graphql_error` assertion instead of
returning. Against the same reproduction, 2 of 2 requests are reported, the
summary agrees with them, and the exit code is 1. The doubled prefix is gone —
`internal/graphql` already named the operation, and the runner no longer names
it again. The `*.run-abort.yaml` harness category existed only for this defect
and is removed with it.

### 11C.2 `error_handling` is inert for the full-failure outcome

`docs/MANUAL.md` §7.1 documents a matrix in which `warn` warns and `ignore`
passes, for partial success **and** full failure. Partial success honours it —
proven in `90-graphql.yaml`, which passes under both. Full failure fails
identically under `fail`, `warn` and `ignore`.

`internal/runner` handles `OutcomeFullFailure` before the mode is read, under a
comment claiming "per spec". `CLI_SPECIFICATION` §12.2 describes the setting
only in terms of partial success and says no such thing.

**Fixed.** The two branches were near-identical and are now one, so the mode
cannot govern one outcome and not the other. `ignore` suppresses GraphQL-level
error checking only — the request's own assertions still run. The manual was
right; `CLI_SPECIFICATION` §12.2 now states the matrix rather than being silent
about full failure, since that silence is what let the drift persist.

### 11C.3 The manual's WebSocket example does not parse

`docs/MANUAL.md` §7.2 puts `websocket:` at the request-item level, as a sibling
of `request:`. The parser wants it **inside** `request:`, as
`CLI_SPECIFICATION` §12.3 correctly shows. A collection copied from the manual
is rejected with "must have websocket.steps with at least one action". Both the
step example and the heartbeat/reconnect example have it wrong.

**Fixed.** Both examples now nest `websocket:` inside `request:`, and
`internal/parser/manual_examples_test.go` holds the manual to it: every complete
collection example in `MANUAL.md` must parse, and every websocket example must
yield steps. It is structural rather than a copy of the examples into Go
literals, which would go stale the moment the manual is edited, and it fails
rather than passing vacuously if it finds nothing to check.

### 11C.4 A refused WebSocket upgrade loses its status and its body

`/ws/reject` answers 426 with a JSON body explaining itself. curlew reports
`websocket dial failed: websocket: bad handshake` — no status, no body, no
headers, so 426, 401, 403 and 500 are indistinguishable. §9.L predicted the
shape; the dialer has the response and discards it before building the message.

**Fixed.** gorilla returns the response alongside `ErrBadHandshake`, having
already read up to 1024 bytes of the body into it; the dialer was discarding it
with `conn, _, err :=`. The message now reads

    websocket dial failed: server refused the upgrade with 426 Upgrade
    Required; body: {…"a client should report the 426 and this body, not a bare
    dial failure"} (websocket: bad handshake)

which is the endpoint's own acceptance criterion, quoted back from the body it
sends. A dial that failed with no response is unchanged and does not invent a
status.

### 11C.5 A heartbeat reports a healthy peer as dead whenever a step is idle

Settled by an A/B pair against one server at one interval:

| | step | outcome |
|---|---|---|
| A | `expect`, reading for 750ms | passes |
| B | `wait`, idle for 400ms | **heartbeat timeout after 100ms** |

`internal/websocket/heartbeat.go` registers a pong handler with gorilla and then
polls a flag on a ticker. gorilla dispatches control frames from inside
`ReadMessage`, so while nothing is reading, the pong lands on the socket and the
handler never runs.

That inverts the feature: a heartbeat exists to hold an **idle** connection open
and to notice a peer that stopped answering, and this one fails precisely when
idle, against a server that answered every ping. Detection of a genuinely dead
peer works — `/ws/no-pong` produces a timeout from inside an `expect` step — so
the mechanism is right and only the dispatch is missing.

That mudflat answers pings is pinned server-side by
`TestWS_EchoAnswersAClientPing`, without which this finding would not be
attributable.

**Fixed**, and the fix is larger than the symptom. Reading during the wait is
not enough on its own: a gorilla read error is permanent, so ending a wait with
a read timeout poisons the connection and every later step inherits the stale
error — measured, as `expect timed out … collected 0/1` on a connection that was
fine.

Reads therefore move to a single pump (`internal/websocket/pump.go`) that never
sets a deadline and stays inside `ReadMessage` for the life of the connection,
which is where control frames are dispatched. Steps take frames from a channel
and bound their own waits with timers. The pump is the sole reader, so frames
cannot interleave.

Consequences, each covered: messages arriving during a wait are buffered rather
than dropped; an orderly close (1000, 1001) ends a wait successfully while a
broken connection fails it, where a sleeping wait passed on a dead one; and
detection of a genuinely dead peer is unchanged.

### 11C.6 A request's reported duration excludes the body read

`internal/httpexec` measures `time.Since(start)` around
`http.DefaultClient.Do`, which returns when the **headers** arrive. `io.ReadAll`
comes afterwards and is never counted.

Against `/sse?events=5&ms=200` — one second of body, no headers to speak of:

```
duration_ms reported : 0
timing.total_us      : 1010867   (1011 ms)
timing.download_us   : 1009959
```

All three from the same `request.end` event. curlew measures the right number
and reports the wrong one, and `timing.max_duration_ms: 50` **passes** — a
documented assertion that cannot fail on a slow body. For an ordinary small
response the download is negligible, which is why nothing noticed.

**Recorded by** `testapi/harness/timing.sh`, which asserts the defect and fails
when it closes.

**Fixed.** `Duration` is measured after `io.ReadAll`, so it and
`Timing.Total` share a start and an end and agree. The same reproduction now
reports 1006ms against a 1006ms request, and `max_duration_ms: 50` is refused.
`timing.sh` is deleted, following its own promotion instructions.

### 11C.7 A response body that is not JSON cannot be asserted on at all

`CLI_SPECIFICATION` §7.3 says the body is parsed as JSON and every assertion
targets a JSONPath, so a `text/event-stream` yields "response body is not valid
JSON" for every operator. That much is documented.

What is not: `cel:`, offered as the escape hatch for what the operator catalogue
cannot express, does not help either. `buildCelResponse` decodes best-effort and
leaves the body `nil`, so `response.body` is null and
`response.body.contains("id: 1")` fails with "no such overload".

HTML, CSV, XML, plain text, NDJSON and SSE are assertable only by status and
headers. §9.K hit the same wall from the other side with its HTML error page.

**Fixed.** A body that is not JSON still has a root, and that root is its
text. At `$`, the text operators — `equals`, `contains`, `matches`, `length`,
`exists`, `not_exists` — evaluate against the raw body, and `response.body` in
CEL is the raw string rather than nil.

Deliberately not permissive: a deeper path stays an error, because there is no
`$.foo` in a document with no structure and "no match at path" would imply there
could have been one; structural operators at `$` stay errors too. Both messages
now name the one thing that does work. A gaps entry asserts that `type: array`
against a stream still fails, so the fallback cannot drift into "every operator
succeeds somehow".

### 11C.8 A counted WebSocket extraction yields an array, undocumented

With `count: > 1`, an `expect` step's `extract:` produces a **JSON-encoded array
of the per-message values**, not the value from the last message. Neither
document says so, and the manual's own example names the variable
`last_order_id`, implying the opposite. An author following it sends
`["ord_1","ord_2","ord_3"]` to a URL and finds out then.
`91-websocket.yaml` now states the real contract executably.

**Fixed by documenting it.** The behaviour is defensible and is kept; what
was missing was anything stating it. Both documents now describe it, the
manual's example is renamed from `last_order_id` to `order_ids`, and two unit
tests pin array-under-count against plain-value-under-default.

### 11C.9 The OpenAPI importer rejects ordinary 3.1 documents

Both `CLI_SPECIFICATION` §18.8 and `docs/MANUAL.md` promise "OpenAPI 3.x". Three
constructs added or changed by 3.1 are refused:

| Construct | 3.1 status | Error |
|---|---|---|
| `info.summary` | added in 3.1 | `invalid info: extra sibling fields: [summary]` |
| `webhooks` | added in 3.1, a headline feature | `extra sibling fields: [webhooks]` |
| `type: ["string","null"]` | JSON Schema 2020-12 alignment | `unsupported 'type' value "null"` |

The importer accepts a document *declaring* `openapi: 3.1.0` and then validates
it against 3.0 rules.

**Fixed.** kin-openapi implements 3.0, and replacing it with a 3.1-native
library is a large dependency for a small gap — the import reads only paths,
parameters, bodies and response codes, and 3.1 changed none of those in ways
that matter. `internal/openapi/relax31.go` translates a 3.1 document into the
3.0 spelling of the same meaning before validation: type arrays become `type` +
`nullable`, 3.1-only descriptive fields are dropped, and `webhooks` are dropped
**with a warning**, since a webhook is an inbound callback with no request to
generate. A union type 3.0 cannot express drops the constraint and says so
rather than silently picking a member. A 3.0 document is untouched.

`testdata/petstore_31.yaml` declared 3.1 and used nothing the version added,
which is why nothing caught this; `testdata/petstore_31_constructs.yaml` uses
all of it. mudflat's own document now carries `info.summary`, so the round trip
exercises a 3.1 construct end to end.

### 11C.10 An imported collection with a path parameter cannot run

§9.P's acceptance criterion is literal: `curlew run /tmp/imported.yaml` must
pass. A path parameter becomes `{{code}}`, and the import emits no `variables:`
entry and no default for it, so the generated collection exits 5 with "undefined
variable" — at run time rather than at import time. Everything else round-trips.

**Fixed.** Path parameters now emit a variable whose default comes from the
document — a parameter or schema `example`, then an `enum` member, then
`default`, then the schema's type, with a declared `minimum` respected so a
status-code parameter cannot default to 0. The variable keeps the parameter's
own name, because `interpolatePath` has already written that name into the URL
and the two have to agree.

`openapi.sh` now asserts that the import defines every variable it references,
and that a run supplying **only** `base_url` passes 8 of 8 — which is what makes
the default real rather than a placeholder that happens to parse. mudflat's
document carries an explicit `example: 200` for its path parameter, as a
document that expects to be imported should.

### 11C.11 A documented `wait` step pauses for no time at all

Found while fixing §11C.3, and the same class as it: `CLI_SPECIFICATION` §12.3
gave its `wait` step a `timeout_ms`, and its step table said "Pause for
`timeout_ms`". The parser reads `duration_ms` for a wait; `timeout_ms` sets a
field `runWait` never looks at, so the documented example paused for zero
milliseconds — a silent no-op rather than an error, which is why it survived
being written down twice.

**Fixed** in the example and the step table, which now names `duration_ms` and
says explicitly that `wait` ignores `timeout_ms`.

It was found by reading §12.3 while checking what §11C.3 should say, not by any
test — the manual-example test added for §11C.3 covered `MANUAL.md` only, and
the specification's own snippets were executed by nothing.

**That gap is now closed too, and closing it took two changes rather than one.**
Extending the example test to `CLI_SPECIFICATION.md` was not sufficient by
itself: `timeout_ms` *parsed* perfectly well, so an example containing it would
have passed a parse test. The parser had to start rejecting a field its action
ignores before the mistake became visible at all —
`internal/parser` now checks each step's fields against its action and names
where a misplaced field does belong.

The two together are verified by a canary: reintroducing the defect into §12.3
fails the test with

    docs/CLI_SPECIFICATION.md:1300 — example does not parse: "timeout_ms" is
    not a field of a "wait" step (it applies to: expect); a "wait" step reads:
    duration_ms

The specification is written in **fragments** — a bare `request:` mapping, a
bare `assertions:` mapping — because it is a reference rather than a tutorial,
and a checker that only accepted whole collections found nothing in it at all.
Fragments are now wrapped into a collection before parsing, which is the point:
a snippet the reader is expected to paste under a request must be valid there.

Checking the specification immediately found two more of its own: a duplicate
`status:` key used to show two alternatives in one block — invalid YAML that a
reader would copy — in both documents.

### 11C.12 The documents' tables were checked by nothing

The last shape of the same problem, and the one §11C.2 actually was. A document
makes three kinds of statement about the binary: **examples**, **tables** and
**prose**. Examples are now parsed on every build. A table is not a snippet —
no parser will ever reject one — so a table can promise behaviour the binary
does not have and the build stays green. §11C.2 was precisely that: a matrix of
four outcomes against three modes, half of which the binary ignored, in two
documents, for as long as it existed.

**Fixed** by making the tables executable rather than decorative.
`internal/docs` reads a markdown table, and the tests run what it says:

| Table | Held to | Direction |
|---|---|---|
| GraphQL outcome × mode matrix (§7.1, §12.2) | the runner, every cell | both documents must agree first |
| Body operators (§7.3) | `evalBodyAssertion` | documented ⇄ implemented |
| Header operators (§7.2) | `evalHeaderAssertion` | documented ⇄ implemented |
| WebSocket step fields (§12.3) | `stepFieldsByAction` | documented ⇄ accepted |

The operator sets are read out of the source with `go/ast` rather than restated
in the test, because a list restated in a test is a second thing to forget to
update. Each check was verified by a canary: reverting the §11C.2 fix fails
twelve matrix cells by name, adding an undocumented operator fails the parity
test, and drifting the step-field table fails with both lists printed.

Turning the checks on found one more: both documents said "Thirteen operators"
above a table of **fourteen** — a prose claim contradicting the very list it
introduces. The count is now checked against the table it precedes.

**What this section claimed, and what was true.** "Tables are executed" was
true of the three tables wired here, out of **77**. The other 70 were in exactly
the state the heading describes, and one of them — the environment-variable row
saying `NO_COLOR` disables on a non-empty value — was right while the binary was
wrong, unread, for the whole life of the variable (§11C.14).

The lesson is about the shape of the check rather than the tables. Counting what
*is* executed can never find what is not; only an inventory can. `internal/docs`
now takes one, and `docs/table-execution-baseline.txt` carries what is still
owed — a register that fails the build when it grows and fails again when a paid
entry is left in it.

### 11C.13 Prose named things that no longer existed

The last of the three surfaces, and the one this project has actually been
burned by. A sentence cannot be executed — but almost every prose claim worth
making **names** something concrete, and a name is checkable even when the
sentence around it is not.

The drift is not hypothetical. The licensing strip removed `curlew license`;
the backend strip removed `curlew login`, `curlew worker`, `--workers`,
`--report-upload` and every `CURLEW_BACKEND_*` variable; M21-002 was four
`MANUAL.md` surfaces still describing the removed backend, found by reading,
months later. Every one left prose naming something that no longer existed, and
nothing failed.

**Fixed** by checking the names: every `curlew <command>`, every `--flag` on a
line that names curlew, and every `CURLEW_*` variable in `MANUAL.md` and
`CLI_SPECIFICATION.md` must exist in the binary or be read by the source.
`CHANGELOG.md` and this document are excluded, because both discuss removed and
unbuilt things deliberately.

Sections that name removed things *on purpose* — the specification's
"Deliberately Absent Surfaces" appendix, the manual's "No account, no backend"
— carry an explicit `<!-- doc-check: ignore-names -->` marker. A marker cannot
outlive the next heading, so none can blanket a document, and their total is
capped so the checks cannot be hollowed out a section at a time.

Turning it on found a live defect immediately: the manual documented
`--color={auto|always|never}` with three worked examples and called `--no-color`
"an alias for `--color=never`". **No `--color` flag exists** — the binary
answers `unknown flag: --color=never`. Only `--no-color` and `NO_COLOR` ship,
and `NO_COLOR` disables on presence even when empty, which is stricter than
no-color.org specifies. The section was first corrected to describe what ships.

Both halves have since been closed the other way, by building what the document
described: `--color={auto|always|never}` ships, and `NO_COLOR` now takes effect
on a non-empty value only. The second is worth noting for what it says about
the limits of name-checking — the false sentence named `NO_COLOR`, a variable
that exists and is read, so checking the names could never reach the claim. It
was the *table* row two sections away, "Any non-empty value disables ANSI
colour", that had been right all along and was contradicting the binary in
silence. That row is now executed (`cmd/curlew/no_color_env_test.go`).

It also exposed a hole in an existing test: `help_parity_test.go` derived the
accepted flag set from `case "--flag":` clauses only, so `--clear` — accepted by
an `if` on `curlew watch` — was invisible to it. The extractor now reads both.

What remains unguarded is a sentence that names nothing: a behavioural claim
with no command, flag or variable in it. The three mechanisms — parse the
examples, execute the tables, check the names — do not reach it, and nothing
here pretends otherwise.

### 11C.14 Executing the rest of the tables

Three live defects, each a table that had been stating something false or
incomplete for as long as it existed.

**The large-dataset guard exited 5, not 2.** Three tables document a tripped
safety guard as exit 2; the specification's CI column says "Fail — fix the
invocation". The guard refused correctly and exited 5, the code meaning the run
could not resolve its variables, because it returned a bare error and every bare
error ending a run became a 5. A pipeline branching on that goes hunting for a
missing variable when the fix is a flag. Found by *producing* all six documented
codes rather than reading about them: 2 was documented three times and produced
by nothing.

**`--seed 42` did not produce the manual's seeded examples.** The faker table's
"Example (seed 42)" column is a reproducibility promise and all ten rows were
wrong — `{{$faker.firstName}}` under that seed is `Tom`, not `Carol` — under no
reading: not per-function, not read across the table in order. Regenerated from
the binary, and the section now says what the seed actually guarantees.

**The manual's output-format table omitted `markdown`**, and announced "Five
output formats" above a binary that supports six. The specification listed all
six and §4.1a documents markdown reports at length; the one place it was missing
was the table a reader consults to choose a `--format`.

What came out affirmative is worth as much: every one of the 85 flag mentions
across eleven tables is accepted by a parser, and the dynamic-function tables
are complete in both directions — no documented function is missing from the
registry, and no registered function is missing a row.

**Paying the register down** found two more, both in the same family: a table
describing a feature that was documented, referenced elsewhere as though it
worked, and never finished.

**`{{name|default:value}}` never substituted anything.** §6.1 lists it as an
interpolation form and §11.5 builds a whole row of its skip matrix on it — "the
default covers the narrower case where the producer succeeded but its JSONPath
did not resolve". The scanner in `internal/parallel` recognised the form, in
order to read the dependency name out of it; the runner used it to decide
skip-versus-run. Nothing put the value in. The dependent ran, as documented, and
sent `http://host/{{user_id|default:FALLBACK}}` to the server. A reference that
cannot resolve is supposed to exit 5; this one reached the network looking like
a success.

**§6.1's dotted reference does not exist.** A plain reference name is
`[a-zA-Z_][a-zA-Z0-9_]*` and dots are not in it, so `{{user.id}}` was literal
text with no error. Removed, with the prose beneath now naming the dotted forms
that are real.

Both are the shape §11C.12 was about, one level deeper: not a table nobody
checked, but a table nobody could check, because the feature it described was
finished everywhere except where it mattered.

### What came out affirmative

Worth as much as the defects, because each replaces an assumption with evidence:

- **Close codes are surfaced.** §9.L listed "a close code not surfaced" as the
  failure mode; curlew reports `websocket: close 4000: mudflat closing with
  4000`.
- **curlew answers protocol pings.** `/ws/ping` counts the pongs and reports 2
  of 2 in a data message.
- **A genuinely dead peer is detected**, from inside a reading step.
- **The TLS failure path is correct**: the message names the cause and the run
  exits 4, which §17 assigns to TLS errors.
- **The OpenAPI round trip passes 8 of 8** against the server that served the
  document — with the path parameter supplied.

### What mudflat got wrong, and how

Three of its own, fixed rather than blamed on curlew:

- **Write-only WebSocket endpoints never answered a ping**, so they looked
  exactly like a dead peer and would have made §11C.5 unattributable. RFC 6455
  §5.5.2 requires a pong, which means a server that only writes still has to
  read.
- **`/ws/no-pong` consulted the request context after hijacking**, which
  `net/http` cancels; it exited after one read cycle instead of holding, so it
  simulated a peer that DROPS the socket rather than one that keeps it open and
  stops answering. Different failures, and only the second is what that endpoint
  is for.
- **`gaps.sh` read a `passed` field that does not exist** in the JSON output —
  the per-request outcome is a status string. Every request therefore looked
  failed, and the one thing the harness exists to catch, an unexpected PASS,
  could never fire. It had been reporting a vacuous pass since Phase 2, and was
  verified fixed with a canary request that passes on purpose.

---

## 12. The Dogfood Suite

The suite is the deliverable. Mudflat without it is a server nobody calls.

### 12.1 Layout

```
testapi/
  collections/
    00-smoke.yaml           # server is up, echo round-trips
    10-assertions.yaml      # every operator against C and H
    20-extraction.yaml      # chaining through H
    30-retry.yaml           # I, all strategies
    40-auth.yaml            # F, including expiry
    50-signing.yaml         # G
    60-parallel.yaml        # J barrier
    70-graphql.yaml         # K
    80-websocket.yaml       # L
    90-encodings.yaml       # C
    95-adversarial.yaml     # E — all expected to fail legibly
    99-openapi.yaml         # P round-trip
  curlew.yaml
  environments/
    local.yaml
  schemas/
  openapi/
    mudflat-openapi.json    # P — hand-written, served at /openapi.json
  harness/
    redaction.sh            # O — asserts on curlew's output artifacts
    redaction-actual.yaml   # assertions wrong on purpose, so `actual` is printed
    redaction-known-leaks.txt
    gaps.sh                 # runs `gap` entries, fails on unexpected pass
    crosscheck.sh           # curl reads the raw layer the same way
    openapi.sh              # P — import the served document, run what comes out
                            #     (also: 3.1 accepted, import self-contained)
```

### 12.2 Execution

```bash
mudflat serve &
curlew run 'testapi/collections/*.yaml' --env local --events /tmp/dogfood.ndjson
```

Exit 0 required. The event stream is retained and diffed against the previous
run's, with the §6.1 fields masked, to enforce reproducibility.

### 12.3 Gap entries

A request item that documents missing capability carries a marker in its
description and lives in a collection the harness runs separately:

```yaml
- name: "GAP: request against /hang should time out"
  description: |
    gap: curlew has no request timeout (executor.go:83, http.DefaultClient).
    Expected: the run fails with a timeout error within 10s.
    Actual today: the run hangs until the server's 120s ceiling.
  request: { method: GET, url: "{{base}}/hang" }
```

`harness/gaps.sh` runs these under an external `timeout(1)` bound and asserts the
**expected failure**. An entry that starts passing fails the harness with
`unexpected pass — promote this entry`. A gap that closes cannot close silently,
and a gap that persists cannot be forgotten.

### 12.4 The redaction harness

Redaction is asserted on curlew's own output, so it is a shell test, not a
collection. `harness/redaction.sh` runs the O-family collections in each output
format plus `--events`, then greps every artifact for the known secret values
from Appendix B. Any hit is a failure naming the format.

It runs **two** collections, because they cover different halves of §6.5.
`collections/70-redaction.yaml` passes end to end, which is what lets it also run
in the dogfood gate — and which means it never prints an `actual` value.
`harness/redaction-actual.yaml` has every assertion wrong on purpose, which is
the only way terminal, TAP, JUnit and JSONL are handed a response value at all.
Scanning only the first made those four surfaces look clean because nothing had
been put in front of them; a harness that reports zero exposure as safety is the
same vacuous pass §12.3 exists to prevent.

Each collection extracts every published value it will encounter. A value nothing
extracts is not sensitive under any rule in §6.5, so omitting one would produce a
leak that is the harness's fault rather than curlew's.

The baseline file `harness/redaction-known-leaks.txt` is currently **empty**,
which is the state to keep it in. A leak outside it fails; a listed leak that
stops leaking *also* fails, so a fix forces the line out rather than leaving a
permanent excuse. That rule is what emptied it.

---

## 13. Testing the Tester

If curlew's tests are what prove Mudflat correct, the loop closes again from the
other side. Mudflat's own suite must not involve curlew.

### 13.1 Independence rule

> No test in `testapi/` may import any `github.com/weiqigod/curlew/internal/...`
> package, and none may invoke the curlew binary.

Enforced by a grep-based build step, in the manner of the existing
`internal/telemetry` / `internal/backend` import guard in `ci-local.sh`.

### 13.2 Golden byte transcripts

For every endpoint, the exact response bytes are captured to
`testapi/golden/<endpoint>.txt` with non-deterministic fields masked. A raw-socket
client — `net.Dial`, write a request, read until close — asserts the server emits
them.

These files are reviewed **once, by hand, against RFC text** at the time they are
introduced. That review is the actual correctness argument for the raw layer;
everything after is drift detection.

### 13.3 Independent cross-check

Two mechanisms, neither of which shares Go's `net/http` opinions:

1. **`curl`.** A script runs `curl -sv --raw` against a defined subset and diffs
   the observed wire bytes against the goldens. `curl` is present on every
   developer machine and in CI, and is an independent implementation with a
   different reading of the same RFCs.
2. **nginx.** A container in `docker-compose.test.yml` serves the same
   well-behaved response subset from static fixtures. Curlew must produce
   identical results against both. This is the only mechanism that tests the
   structured layer's Go-ness, and it is the reason §3's residual risk is
   *reduced* rather than merely acknowledged.

### 13.4 Determinism test

Run the full endpoint surface twice through the raw client, mask the §6.1 fields,
require byte equality.

---

## 14. Operations

### 14.1 Run modes

```bash
mudflat serve                    # all families, default ports
mudflat serve --port 8080        # relocate the base port; others derive by offset
mudflat serve --only echo,retry  # subset, for a fast smoke run
mudflat serve --no-tls           # skip cert generation
mudflat certs --out testapi/certs   # generate the CA and leaf certificates
mudflat index                    # print the endpoint index as JSON, no listener
```

### 14.2 Startup

Startup to first accepted connection: **under 200 ms** with `--no-tls`. Certificate
generation is a separate command whose output is cached on disk and regenerated
only when absent or expired, so it never sits in the hot path.

### 14.3 Trust anchors

> **MEASURED, AND THE ANSWER IS NO.** The risk this section flagged was
> confirmed on the real toolchain before the N family was built, exactly as it
> asked. `SSL_CERT_FILE` does **not** override the platform verifier on
> go1.25.5 darwin/arm64:
>
> ```
> CONTROL (explicit RootCAs):                     TRUSTED — 200 OK
> SSL_CERT_FILE:                                  FAILED  — x509: certificate signed by unknown authority
> SSL_CERT_DIR:                                   FAILED  — same
> SSL_CERT_FILE + GODEBUG=x509usefallbackroots=1: FAILED  — same
> ```
>
> The control passing is what makes it attributable: a client that sets
> `RootCAs` explicitly trusts the same certificate served by the same listener,
> so the certificates are sound and the trust mechanism is not. Reproduced
> through curlew itself, which fails identically with `SSL_CERT_FILE` set.

The original plan was for `mudflat certs` to write a CA that clients trust via
`SSL_CERT_FILE`. With that route closed and curlew exposing no CA option
(§11.3), a client cannot trust mudflat's CA by any means — and chain building
fails before expiry, hostname or intermediate are ever examined, so an expired
leaf, a self-signed leaf and a wrong-hostname leaf all produce one identical
error.

**What was built instead.** One TLS listener on `--port + 2`, with a CA and leaf
generated per process and no `certs` subcommand: there is nothing to cache when
nothing can trust it. `--ca-out <path>` writes the CA for clients that *can* be
told about it, and `--no-tls` skips generation entirely.

- The listener is verified by the package's own tests, which set `RootCAs`, and
  by `curl --cacert`, which reports HTTP/2 over TLS 1.3.
- curlew's half is one entry in `testapi/gaps/expected-failures.yaml`, and it is
  half affirmative: the failure message names the cause exactly and the run
  exits 4, which §17 assigns to TLS errors. Nothing had checked that against a
  real TLS server before.
- The other eight postures are **not built**. Eight endpoints that a client
  cannot tell apart are what §16 deletes. They are blocked rather than
  abandoned: when curlew gains a CA option they become distinguishable and worth
  writing, and the note at the top of `testapi/mudflat/tls.go` says so.

**On HTTP/2**, the measurement sharpened §11.4 rather than confirming it. Go's
default transport offers `h2` in ALPN, so a client that reaches the TLS listener
negotiates HTTP/2 without asking and `/protocol` reports it — verified by curl.
What curlew cannot reach is **h2c** specifically, cleartext HTTP/2, which has no
ALPN to negotiate with and needs an upgrade the client never sends. The gap is
narrower than "no HTTP/2".

### 14.4 Configuration

None beyond the flags above. Behavior is selected by URL, never by server
configuration, so a failing test's URL is the complete reproduction recipe.

### 14.5 Safety

Binds `127.0.0.1` by default. Binding a non-loopback interface requires
`--bind-unsafe` and prints a warning naming the fixed credentials it ships. The
`/leak/*` family returns realistic secrets and the `/verify/*` family holds fixed
signing keys; neither belongs on a network.

---

## 15. Integration

### 15.1 `ci-local.sh`

A new step in the Go gate, after `smoke`:

```bash
step "dogfood: curlew against mudflat"
./mudflat serve --no-tls &
MUDFLAT_PID=$!
trap 'kill $MUDFLAT_PID 2>/dev/null' EXIT
./scripts/wait-for-port.sh 8080
./curlew run 'testapi/collections/*.yaml' --env local --events "$TMP/dogfood.ndjson"
./testapi/harness/gaps.sh
./testapi/harness/redaction.sh
```

TLS families run under `--full` only, since certificate generation costs more
than the Go gate's budget.

### 15.2 Smoke suite

`smoke/fixtures/httpbin_server.py` is replaced by `mudflat serve --only echo`.
This removes a Python dependency from the smoke path and unifies the stub.

### 15.3 Docker

A `mudflat` service in `docker-compose.test.yml`, plus the nginx cross-check
service from §13.3.

### 15.4 CI

No change to trigger policy. `.github/workflows/go.yml` runs
`./scripts/ci-local.sh --go`, so the dogfood step is inherited automatically —
which is the property that workflow was built for.

---

## 16. The Anti-Bloat Rule

A test API grows endpoints nobody calls. The rule:

> **Every endpoint must be exercised by at least one dogfood collection or one
> golden transcript, and every URL in a dogfood collection must resolve to a
> documented endpoint.**

Enforced by a parity test in the manner of `internal/schema/parity_test.go`: walk
`mudflat index` output and the `testapi/collections/` tree, and fail the build on
either direction of mismatch — an orphan endpoint or an undocumented URL.

This repository has a working pattern for this.
`internal/output/events/assertion_type_parity_test.go` walks the parser's AST to
derive the assertion-type list, and caught struct/schema drift unprompted in both
PR #20 and PR #21; `internal/backlog` caught three unparseable task files.
Derive the truth from the code, and the drift fails the build instead of reaching
a user.

---

## 17. Implementation Decisions

### 17.1 Language: Go, with a stated boundary

**Decision:** Go.

**Reasoning:** one toolchain in a repository that already carries three; a single
static binary matching curlew's own distribution model; `go run` in the smoke
path with no container; and — decisively — the layer where language actually
matters is the raw layer, which uses `net.Listen` and writes bytes, where Go
carries no opinions at all.

**The cost, stated:** the structured layer's well-formed responses come from Go's
`net/http` and therefore share curlew's assumptions. §3 describes the three
mitigations and their limits. The rule that follows from this: **when a behavior
can be expressed in either layer, put it in the raw layer.**

**Rejected:** Rust or Python for full independence — better isolation, but a
fourth toolchain, slower startup or a heavier build, and a maintenance surface
that would decay. The nginx cross-check (§13.3) buys most of the independence for
one container.

### 17.2 Not a fork of httpbin

httpbin's response shapes are lossy in exactly the ways §8 rejects — headers as a
map, bodies decoded. Its endpoint set is also aimed at demonstrating HTTP to
humans, not at breaking a client. Some path names are borrowed for familiarity;
nothing else is.

### 17.3 Where it lives

`testapi/` in this repository, not a separate one. It versions with curlew, so
the parity test in §16 can see both sides. It builds and runs independently:
`go build ./testapi/cmd/mudflat`.

### 17.4 Deterministic pseudorandom data

`/bytes/{n}` and faker-adjacent fixtures derive from a fixed-seed
`math/rand/v2` PCG stream keyed by `(endpoint, n, seed)`. Never
`crypto/rand`, never the clock.

---

## 18. Delivery Phases

Each phase ends with something runnable, per the repository's vertical-slice
commitment.

### Phase 1 — Foundation ✅ implemented

Session model, echo envelope, status, body encodings and content types, failure
injection, resources. `00-smoke`, `10-assertions`, `20-extraction`, `30-retry`
collections. Parity test. Wired into `ci-local.sh`.

**Observable:** `curlew run testapi/collections/*.yaml` passes against a server
curlew did not write. Dogfooding exists.

**Delivered.** 31 endpoints across six families, 53 dogfood assertions passing,
82.3% coverage on `testapi/mudflat`. Three previously unknown defects found on
the first run (§11A). The `--only` flag and `mudflat certs` from §14.1 are not
implemented, because the families they would gate do not exist yet; a flag that
accepts a value and does nothing is exactly the kind of false clear this
specification exists to avoid.

### Phase 2 — Adversarial and verification ✅ implemented

Raw layer, golden transcripts, `curl` cross-check, signature verification,
concurrency barrier, rate limiting, redaction harness, gap harness.

**Observable:** `internal/signer` is verified for the first time; `--parallel` is
proven parallel rather than inferred from timing.

**Delivered.** 16 raw endpoints on a second listener that uses no HTTP library,
15 golden transcripts hand-reviewed against RFC 9110/9112, and three harnesses
in `ci-local.sh`. 74 dogfood assertions.

Both observables came out affirmative:

- curlew's SigV4 and OAuth 1.0a signatures are **correct**. mudflat's verifier
  is written from the AWS documentation and RFC 5849 rather than from
  `internal/signer`, and its SigV4 chain is pinned against AWS's published
  `get-vanilla` vector, so this is evidence rather than two copies of one
  misreading agreeing.
- `--parallel` is **genuinely concurrent**. With the flag, four requests
  rendezvous and the run reports "Waves: 1, Max parallelism: 4"; without it the
  barrier times out reporting `arrived: 1`.

The `curl` cross-check confirms the malformations are real rather than Go being
strict: curl independently rejects the duplicate `Content-Length`, the non-hex
chunk size, the missing status line, the NUL in a field value and the header
line without a colon — and accepts the well-formed control case.

Two further defects surfaced (§11B).

### Phase 3 — Protocols and matrix ✅ implemented (§9.N reduced)

WebSocket, GraphQL, SSE and streaming, TLS, HTTP/2, OpenAPI round-trip.

**Observable:** every protocol curlew claims is exercised against a real server.

**Delivered.** 21 endpoints across five families — K GraphQL (5), L WebSocket
(9), M streaming (5), N TLS (1), P OpenAPI (1) — 102 dogfood assertions, and
three more harnesses in `ci-local.sh` — the timing gap, the OpenAPI round trip,
and the existing gap/redaction/crosscheck set.

The WebSocket frame layer is written from RFC 6455 rather than taken from
gorilla, which is what curlew's client uses: a mudflat built on gorilla would
agree with curlew by construction about masking, fragmentation, control frames
and close codes, which is §3's closed-loop problem wearing a different hat. The
package's own tests read those frames back **with** gorilla, so a second
implementation checks the bytes — the role curl plays for the raw layer. The
handshake accept value is pinned against RFC 6455 §1.3's published example.

Writing the frames is also what buys the endpoints: a library will not send an
empty continuation frame, decline to answer a ping, or close with a code it
dislikes, and those are exactly what `/ws/fragmented`, `/ws/no-pong` and
`/ws/close/{code}` exist to produce.

**Two things were deliberately not built:**

- **The nine-port TLS matrix**, reduced to one listener. §14.3 has the
  measurement and the reasoning; the short version is that no client-side route
  to trusting mudflat's CA exists on this toolchain, so all eight remaining
  postures produce one identical error, and §16 deletes endpoints a client
  cannot tell apart.
- **The nginx cross-check.** curl already plays the independent-reader role for
  the raw layer, and gorilla plays it for the WebSocket layer, so nginx would
  add a container and a second config surface to re-answer a question two
  cheaper checks already answer. Worth revisiting only if a finding turns out to
  hinge on Go's server behaviour specifically.

Ten more defects surfaced, and five results came out affirmative (§11C).

**Phase 1 is where the value concentrates.** It is roughly a quarter of the
endpoint surface and it closes the dogfooding gap outright; Phases 2 and 3 deepen
what Phase 1 makes possible. If only one phase is ever built, build that one.

---

## 19. Conformance

An implementation conforms when all of the following hold. Each is mechanically
checkable.

1. `mudflat serve --no-tls` accepts connections within 200 ms. (`--no-tls`
   exists for that reason: certificate generation is the only startup cost that
   is not constant.)
2. `GET /` lists every implemented endpoint with its contract and cited curlew
   feature.
3. Running the full endpoint surface twice yields byte-identical output with only
   the §6.1 fields differing.
4. Every endpoint appears in a dogfood collection or a golden transcript, and
   every dogfood URL resolves to a documented endpoint (§16).
5. No file under `testapi/` imports a curlew package or invokes the binary
   (§13.1).
6. Goldens for the raw layer are asserted by a raw-socket client, not an HTTP
   library.
7. The `curl` cross-check passes for its defined subset.
8. Every delaying, stalling, or hanging endpoint terminates within 120 s.
9. No outbound network connection is made during a full run.
10. Sessions are isolated: two concurrent dogfood runs produce identical results.
11. `harness/gaps.sh` fails on an unexpected pass as well as on an unexpected
    failure.
12. `harness/redaction.sh` finds no Appendix B value in any output artifact
    across all six formats and the events stream — including in the artifacts of
    a run whose assertions fail, which is the only way several of those formats
    print a response value at all — and its baseline file is empty.
13. `curlew import openapi` on the served document produces a collection that
    passes against the server (§9.P). Asserted by `harness/openapi.sh`, which
    also checks the served document is byte-identical to the version-controlled
    one — a server that rewrote it on the way out would be describing something
    else.
14. The server binds loopback unless `--bind-unsafe` is given.

---

## Appendix A — Port Map

| Port | Layer | Purpose |
|---|---|---|
| 8080 | structured | HTTP/1.1 cleartext — the default target |
| 8081 | raw | Adversarial framing |
| 8090 | structured | h2c (cleartext HTTP/2) |
| 8443 | structured + TLS | Valid chain from Mudflat's CA |
| 8444 | structured + TLS | Expired leaf |
| 8445 | structured + TLS | Self-signed |
| 8446 | structured + TLS | Wrong hostname |
| 8447 | structured + TLS | Missing intermediate |
| 8448 | structured + TLS | Client certificate required |
| 8449 | structured + TLS | TLS 1.2 maximum |

`--port N` relocates the base; the rest derive by the same offsets.

---

## Appendix B — Fixed Test Credentials

Published, fixed, and non-secret by design. They exist to be leaked in tests so
redaction can be verified, and they must never appear on a network host.

| Purpose | Value |
|---|---|
| Basic auth | `mudflat` / `probe` |
| Bearer token | `mud_tok_7f3a9c2e5b1d4680` |
| API key | `mud_key_a1b2c3d4e5f60718` |
| SigV4 access key | `AKIAMUDFLATTEST0000` |
| SigV4 secret key | `wJalrMudflatEXAMPLEKEY/K7MDENG/bPxRfi` |
| OAuth1 consumer key | `mud_consumer_0001` |
| OAuth1 consumer secret | `mud_consumer_secret_0001` |
| Leak-bait card number | `4242424242424242` |
| Leak-bait cookie value | `mud_sess_c9e4f1a7b2d80356` |

`harness/redaction.sh` greps output artifacts for exactly this list.

---

## Appendix C — Deliberately Absent

| Absent | Why |
|---|---|
| A database | State is in memory, scoped to a session, and disposable. Persistence would add a failure mode that teaches nothing about curlew. |
| Authentication on Mudflat itself | It binds loopback. Auth would obstruct the tests. |
| Configurable behavior via config file | Behavior is selected by URL, so a failing test's URL is the whole reproduction recipe (§14.4). |
| Realistic domain modelling | §2. Resources exist so chaining has an id. |
| Response recording / proxy mode | A different tool. Mudflat generates; it does not capture. |
| A hosted instance | §14.5. Fixed credentials and deliberate protocol violations do not belong on a network. |
| Endpoints without a cited curlew behavior | §16 deletes them. |
