# curlew — Exit codes reference

Load this file when the user asks what an exit code means, how to interpret a
non-zero exit, or which artifact to read for a given code.

## Exit code table

These are collection-run meanings unless a command is named. For other commands,
read its help and stderr (for example, ui uses 5 for a missing project).

| Code | Meaning | Primary artifact |
|---|---|---|
| 0 | All assertions passed (or dry run with no failures) | `responses/run.md` |
| 1 | One or more assertions failed, or CLI usage error | stderr first; `responses/<slug>.md` (`### Assertions` section) |
| 2 | Safety guard; also usage/input error for perf or pr-check | `.curlew/run.ndjson` (`run.end` event, `exit_code: 2`) |
| 3 | Configuration error before any HTTP fired | stderr |
| 4 | Non-assertion runtime error (network, TLS, DNS) | `.curlew/run.ndjson` (`request.end` with `error.category: network`) |
| 5 | Undefined or circular variable reference | `.curlew/run.ndjson` (`run.error` event names the variable) |
| 130 | SIGINT during a `curlew perf` run | stdout (partial summary) |

## CEL-specific codes (surfaced by `curlew validate`)

| Code | Symbolic name | Meaning | Diagnostic |
|---|---|---|---|
| — | `ERR_CEL_PARSE` | CEL expression is syntactically invalid or uses a disabled function | Run `curlew validate`; message names field path and includes up to 200 chars of source |
| — | `ERR_CEL_TYPE` | CEL expression compiles but returns a non-bool type | Run `curlew validate`; message names the actual type returned |

`ERR_CEL_PARSE` and `ERR_CEL_TYPE` surface as exit code 3 at runtime
(configuration error). Use `curlew validate` before running to catch them
early.

## How to read the right artifact

1. **Exit 0** — read `responses/run.md` to confirm and summarise.
2. **Exit 1** — check stderr for an unknown option or missing argument first. If the collection actually ran, open `responses/run.md` for the summary; then open each
   failing `responses/<slug>.md` for operator + expected + actual.
3. **Exit 2** — read `.curlew/run.ndjson`; filter `kind: "run.end"` for the
   guard-rail event and `kind: "request.end"` with `outcome: "skipped"`.
4. **Exit 3** — read stderr. The message names the YAML file and line, the
   missing config key, or the `--only` mismatch list.
5. **Exit 4** — read `.curlew/run.ndjson`; filter `kind: "request.end"` with
   `error.category: "network"`.
6. **Exit 5** — read `.curlew/run.ndjson`; filter `kind: "run.error"` for the
   variable name.
7. **Exit 130** — the user pressed Ctrl+C during `curlew perf`. Report the
   partial summary already written to stdout; do not re-run without asking.

For CEL errors (exit 3 with `ERR_CEL_PARSE` or `ERR_CEL_TYPE`): run
`curlew validate` against the collection file to see the detailed message.
