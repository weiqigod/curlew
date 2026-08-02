# apitest — Failure playbook

Load this file when a run exits non-zero and you need the step-by-step
remediation for a specific exit code.

## Per-exit-code remediation

### Exit 0 — all passed

Nothing to fix. Read `responses/run.md` and summarise the run for the user.

---

### Exit 1 — assertion failed

1. Open `responses/run.md`. Find the failing request(s) in the summary table.
2. Open `responses/<slug>.md` for each failing request.
3. Read the `## Assertions` section: it shows operator, expected, and actual
   for every checked assertion.
4. Tell the user which assertion failed, what value was expected, and what the
   API returned.
5. If the failure looks like an API bug, point at the full response body in the
   same file. If it looks like a wrong expected value, suggest editing the
   collection.

---

### Exit 2 — guard rail (request limit)

1. Read `.apitest/run.ndjson`. Filter `kind: "run.end"` — the event has
   `exit_code: 2`.
2. Filter `kind: "request.end"` with `outcome: "skipped"` to see which
   requests did not fire.
3. Tell the user the collection exceeded the request safety cap. Suggest
   splitting the collection into smaller files or raising `MaxRequests` if
   the guard rail is intentionally conservative.

---

### Exit 3 — configuration error

1. Read stderr. It names the file and line (parse error), the missing config
   key (config error), or the available request names (`--only` no-match).
2. Echo the relevant stderr line to the user.
3. Tell the user which file and line to fix, or which `--only` name to use.

For CEL-specific configuration errors (`ERR_CEL_PARSE`, `ERR_CEL_TYPE`),
see the CEL section below.

---

### Exit 4 — runtime error (network)

1. Read `.apitest/run.ndjson`. Filter `kind: "request.end"` with
   `error.category: "network"`.
2. The event's `error.message` has the underlying cause (connection refused,
   DNS resolution failed, TLS handshake error, timeout).
3. Suggest: check the URL is correct, the server is reachable, and any required
   environment variables (`{{base_url}}`) are set.

---

### Exit 5 — undefined or circular variable

1. Read `.apitest/run.ndjson`. Filter `kind: "run.error"`.
2. The event's `error.message` names the undefined or circular variable.
3. Tell the user which variable is missing. Suggest checking:
   - Is it defined in `apitest.yaml` or `environments/<env>.yaml`?
   - Did the upstream `extract:` run before this request?
   - Is `--env <name>` missing from the command?

---

### Exit 6 — feature gate denied

1. Read stderr. Look for the `feature_gated` line.
2. It names the feature, the current tier, and the required tier.
3. Tell the user the feature requires the named tier and suggest the
   workaround the CLI prints (e.g. use a different output format).

---

### Exit 9 — license grace period expired

1. Read stderr. Look for `feature_gated: grace period expired`.
2. Tell the user their license validation has lapsed.
3. Suggest running `apitest license --validate` to refresh the license.

---

## CEL expression errors (ERR_CEL_PARSE, ERR_CEL_TYPE)

These surface as exit code 3 (configuration error). The `apitest validate`
command provides detailed diagnostics.

| Code | Meaning | Diagnostic action |
|---|---|---|
| `ERR_CEL_PARSE` | CEL expression syntactically invalid, or uses a disabled function (`now()`, zero-arg `timestamp()`). | Run `apitest validate` — message names the field path and includes up to 200 chars of the source expression. Fix the expression or replace the disabled function with a variable. |
| `ERR_CEL_TYPE` | CEL expression compiles but its result type is not `bool`. | Run `apitest validate` — message names the actual type returned. Ensure the expression ends in a boolean comparison (e.g. `response.status == 200`, not `response.status`). |

### CEL remediation steps

1. Run `apitest validate collections/<file>.yaml`.
2. Read the error output: field path (e.g. `requests[2].if`) and source
   excerpt.
3. For `ERR_CEL_PARSE`:
   - Check parentheses, operator spelling, and string quoting.
   - Replace `now()` or `timestamp()` with a variable bound at run time.
4. For `ERR_CEL_TYPE`:
   - Ensure the expression evaluates to `true` or `false`.
   - Common mistake: `response.status` alone is an int, not a bool. Use
     `response.status == 200`.
5. Re-run `apitest validate` until it reports no errors, then `apitest run`.
