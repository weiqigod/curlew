# Gap collections

Requests in this directory **are expected to fail** against curlew as it stands.
Each one documents a defect or a missing capability found by running the dogfood
suite against mudflat.

They are kept as executable requests rather than as prose in a backlog because a
prose entry cannot tell you when it stops being true. A gap that closes should
close visibly.

## Status in Phase 1

Phase 1 ships these as documentation only — they are **not run** by
`ci-local.sh`. The harness that runs them and fails on an *unexpected pass*
(specification §12.3) is Phase 2.

Until then, run them by hand:

```bash
mudflat serve &
curlew run 'testapi/gaps/*.yaml' --env local --var run=manual
```

Every request in `curlew-defects.yaml` should fail. If one passes, the defect it
describes has been fixed: move the request into the matching collection under
`testapi/collections/` and delete the entry here.

## Why they still count for the parity test

`testapi/parity_test.go` scans this directory as well as `collections/`. An
endpoint exercised only by a gap request is still doing its job — it is the
curlew side of the exchange that is not — so it does not count as an orphan
under specification §16.

## The three findings from the first run

Full detail lives in the request descriptions in `curlew-defects.yaml`. In short:

| # | Defect | Where |
|---|---|---|
| 1 | Assertion expected values are never interpolated: `equals: "{{var}}"` compares against the literal template | `internal/assertion`, all operators |
| 2 | Header `exists: false` is not honoured, so header *absence* cannot be asserted at all | `internal/assertion` header path |
| 3 | A body that fails to decode is classified as a **network error**, so `retry_on.network_errors` retries something that can never succeed | `internal/httpexec` error classification |

Findings 1 and 2 are both cases where `docs/CLI_SPECIFICATION.md` documents
behaviour the binary does not have — §7.2 states plainly that header `exists`
takes "Presence (`true`) or absence (`false`)".
