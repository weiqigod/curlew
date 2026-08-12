# Golden wire transcripts

Byte-exact captures of what the raw layer (`testapi/mudflat/raw.go`) writes to a
socket. One file per endpoint, produced and compared by
`TestRaw_MatchesGoldenTranscripts`.

## What these are, and what they are not

They are **drift detection**. The correctness argument for these bytes is the
one-time hand review recorded below, against the RFC text each response is meant
to violate or satisfy. What the test adds is that the server cannot change what
it emits without a reviewer seeing the diff.

A golden that is regenerated and committed without reading the diff provides
nothing. `MUDFLAT_UPDATE_GOLDEN=1` exists because hand-typing 8 KB of header is
absurd, not because regeneration is a substitute for review.

```bash
MUDFLAT_UPDATE_GOLDEN=1 go test ./testapi/mudflat/ -run TestRaw_MatchesGolden
git diff testapi/golden/    # then actually read it
```

## Line endings

`.gitattributes` marks this directory `-text -diff`. Git must not normalise
these files: a CRLF→LF conversion on checkout would rewrite the framing of every
response and fail every comparison on a machine configured differently from the
author's. `-diff` keeps the NUL-byte file from being treated as binary noise in
a diff while still leaving the bytes untouched.

## Review record

Reviewed 2026-08-11 against RFC 9110 (HTTP Semantics) and RFC 9112 (HTTP/1.1).

| File | Intended property | Authority |
|---|---|---|
| `content-length-over.txt` | `Content-Length: 1000`, 29 bytes sent, then close | RFC 9112 §6.3 — an incomplete message must not be treated as complete |
| `content-length-under.txt` | `Content-Length: 5`, far more sent | RFC 9112 §6.3 — the recipient reads exactly the declared length |
| `content-length-duplicate.txt` | Two `Content-Length` fields, 10 and 20 | RFC 9112 §6.3 — differing values are an unrecoverable error |
| `content-length-with-chunked.txt` | Both `Content-Length` and `Transfer-Encoding: chunked` | RFC 9112 §6.1 — `Transfer-Encoding` overrides; the combination signals smuggling |
| `chunked-bad-size.txt` | Chunk size line `ZZZZ`, not hex | RFC 9112 §7.1 — chunk-size is HEXDIG |
| `chunked-no-terminator.txt` | Two chunks, no final `0\r\n\r\n` | RFC 9112 §7.1 — the last chunk terminates the body |
| `chunked-trailers.txt` | **Well-formed.** Announced `Trailer:`, final chunk, trailer field, terminator. The checksum value is the MD5 of `hello` | RFC 9112 §7.1.2 |
| `no-status-line.txt` | Header block and body with no status line | RFC 9112 §4 — a response begins with a status-line |
| `bare-lf.txt` | LF line endings throughout, no CR anywhere | RFC 9112 §2.2 — a recipient MAY recognise a bare LF |
| `header-huge.txt` | One 8 KB field value, above the common 8 KB proxy limit | RFC 9110 §5.4 — no length limit is specified; recipients impose their own |
| `header-many.txt` | 1000 distinct field lines | RFC 9110 §5.4 |
| `header-nul.txt` | NUL byte inside a field value | RFC 9110 §5.5 — field values must not contain NUL |
| `header-no-colon.txt` | A field line with no `:` separator | RFC 9112 §5 — field-line is `field-name ":" OWS field-value OWS` |
| `trailing-garbage.txt` | Complete 9-byte response, then junk on the same connection | RFC 9112 §6.3 — the recipient stops at the declared length |
| `http09.txt` | Bare body, no status line, no headers | The pre-1.0 shape; modern recipients reject it |

Two properties are checked by test rather than by eye, because eyes miss them:

- `TestRaw_OnlyTheIntendedMalformationIsPresent` verifies that every endpoint
  *outside* the content-length family has a `Content-Length` matching its body.
  An accidental mismatch would make an endpoint test two things at once and
  attribute the failure to the wrong cause.
- `chunked-trailers.txt` is the family's control case. Without a well-formed
  member, the family would only ever prove a client is strict, never that it is
  correct.

## Not covered here

`/raw/reset-after/{n}` has no golden. Its response is cut mid-flight, so what
arrives depends on TCP timing rather than on what the server wrote;
`TestRaw_ResetAfterNBytes` asserts the reset instead.
