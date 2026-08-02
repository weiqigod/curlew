# apitest — Exit codes reference

Load this file when the user asks what an exit code means, how to interpret a
non-zero exit, or which artifact to read for a given code.

## Exit code table

| Code | Symbolic name | Meaning | Primary artifact |
|---|---|---|---|
| 0 | — | All assertions passed (or dry run with no failures) | `responses/run.md` |
| 1 | `ERR_ASSERTION` | One or more assertions failed | `responses/<slug>.md` (`## Assertions` section) |
| 2 | `ERR_GUARD_RAIL` | Request count exceeded `MaxRequests` safety cap | `.apitest/run.ndjson` (`run.end` event, `exit_code: 2`) |
| 3 | `ERR_CONFIG` | Configuration error before any HTTP fired | stderr |
| 4 | `ERR_RUNTIME` | Non-assertion runtime error (network, TLS, DNS) | `.apitest/run.ndjson` (`request.end` with `error.category: network`) |
| 5 | `ERR_VARIABLE` | Undefined or circular variable reference | `.apitest/run.ndjson` (`run.error` event names the variable) |
| 6 | `ERR_FEATURE_GATE` | Feature requires a higher subscription tier | stderr (`feature_gated` line) |
| 9 | `ERR_LICENSE` | License grace period expired | stderr (`feature_gated: grace period expired ...`) |

## CEL-specific codes (surfaced by `apitest validate`)

| Code | Symbolic name | Meaning | Diagnostic |
|---|---|---|---|
| — | `ERR_CEL_PARSE` | CEL expression is syntactically invalid or uses a disabled function | Run `apitest validate`; message names field path and includes up to 200 chars of source |
| — | `ERR_CEL_TYPE` | CEL expression compiles but returns a non-bool type | Run `apitest validate`; message names the actual type returned |

`ERR_CEL_PARSE` and `ERR_CEL_TYPE` surface as exit code 3 at runtime
(configuration error). Use `apitest validate` before running to catch them
early.

## How to read the right artifact

1. **Exit 0** — read `responses/run.md` to confirm and summarise.
2. **Exit 1** — open `responses/run.md` for the summary; then open each
   failing `responses/<slug>.md` for operator + expected + actual.
3. **Exit 2** — read `.apitest/run.ndjson`; filter `kind: "run.end"` for the
   guard-rail event and `kind: "request.end"` with `outcome: "skipped"`.
4. **Exit 3** — read stderr. The message names the YAML file and line, the
   missing config key, or the `--only` mismatch list.
5. **Exit 4** — read `.apitest/run.ndjson`; filter `kind: "request.end"` with
   `error.category: "network"`.
6. **Exit 5** — read `.apitest/run.ndjson`; filter `kind: "run.error"` for the
   variable name.
7. **Exit 6** — read stderr for the feature name and required tier.
8. **Exit 9** — read stderr; tell the user to run `apitest license --validate`.

For CEL errors (exit 3 with `ERR_CEL_PARSE` or `ERR_CEL_TYPE`): run
`apitest validate` against the collection file to see the detailed message.
