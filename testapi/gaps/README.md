# Expected-failure collections

Requests in this directory **are expected to fail**, and a failure here is the
correct outcome: mudflat sends something no client can succeed against. What
each one actually checks is whether the resulting error is *legible* — does it
name the cause, and is it classified so retry rules treat it correctly?

They are kept as executable requests rather than as prose because a prose entry
cannot tell you when it stops being true.

## How they run

`testapi/harness/gaps.sh` runs every collection here and asserts each request
fails. **An unexpected pass fails the harness** — that is the design, and it is
the only thing that makes this directory shrink. It runs in `ci-local.sh`.

```bash
./mudflat serve &
testapi/harness/gaps.sh --url http://127.0.0.1:8080
```

If a request passes, whatever it documented has been fixed: move it into
`testapi/collections/` and delete the entry here.

## Why they still count for the parity test

`testapi/parity_test.go` scans this directory as well as `collections/`. An
endpoint whose only job is to break a client cannot appear in a passing
collection, so without this it would look like an orphan under specification
§16.

## History: the three defects the first run found

All three are **fixed**. The requests that reproduced them now live in the
passing collections, which is where a closed gap belongs:

| # | Defect | Fixed in | Now covered by |
|---|---|---|---|
| 1 | Assertion expected values were never interpolated | `internal/requtil` | `10-assertions.yaml`, `20-extraction.yaml` |
| 2 | Header `exists: false` was not honoured | `internal/assertion` | `10-assertions.yaml` |
| 3 | Body-read failures were mislabelled and misclassified | `internal/httpexec` | this directory, plus `internal/httpexec` tests |

Defect 3 is the one that keeps an entry here: a lying Content-Encoding can never
produce a passing request, so only an expected-failure request can exercise it.
