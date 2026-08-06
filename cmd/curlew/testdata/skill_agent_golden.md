---
name: curlew
description: Run Curlew collections and interpret results. Use when the user asks to run, test, hit, exercise, or check an HTTP API; or when the user mentions a collection file (e.g. "run the users collection", "hit the staging API", "test the auth flow"). The CLI never calls an LLM — the agent reads deterministic artifacts (markdown reports + NDJSON event stream) and narrates them.
---

<!-- curlew-skill: agent v1.0 (curlew v0.0.0-test) -->

# curlew skill

You are driving the `curlew` CLI on behalf of a developer. Curlew is a
file-based HTTP testing tool. Collections live in YAML; results land as
markdown files and an NDJSON event stream. Your job: invoke the CLI, read the
artifacts, and narrate. Never paraphrase response bodies — point at the file.

## When invoked

Trigger on natural-language phrasings that describe HTTP calls or test runs:

- "run the test", "run the collection", "run the users collection"
- "hit the staging API", "exercise the checkout flow"
- "did the auth assertion pass?", "what does GET /users return?"
- Direct mentions: "use curlew to ...", "curlew run ..."

Before invoking, look at the project layout. Curlew projects have an
`curlew.yaml` at the root, a `collections/` directory with one or more
`*.yaml` files, optional `environments/` for per-environment variables, and
optional `responses/` (created on first run) for markdown artifacts.

## Invocation

The default invocation has zero flags beyond a collection path:

```bash
curlew run collections/users.yaml
```

The `curlew.yaml` `output:` block declares the output format and artifact
paths — that is why the agent does not need to pass `--format markdown
--report responses/ --events .curlew/run.ndjson`. If the user names an
environment, append `--env <name>`. If the user wants to iterate on a single
request, append `--only "<request name>"`.

```bash
curlew run collections/users.yaml --env staging
curlew run collections/users.yaml --only "Get user"
```

For a one-shot URL with no collection, use `curlew exec`:

```bash
curlew exec https://api.example.com/health
```

## Where results land

After every `curlew run`, three artifact streams exist:

- `responses/run.md` — the canonical landing file. Read this first. It links
  to per-request markdown files and shows the summary (total / passed /
  failed / skipped, plus parallel-wave grouping when present).
- `responses/<slug>.md` — one file per main-phase request. Slug is derived
  from the request name (URL-safe). Each file has a fixed 10-section block
  delimited by `<!-- BEGIN curlew:response id=<request_id> slug=<slug>
  run=<run_id> --> ... <!-- END ... -->` sentinels. Re-runs splice only the
  bytes between the sentinels; agent or human notes outside the block
  survive verbatim.
- `.curlew/run.ndjson` — the structured event stream. One JSON object per
  line: `run.start`, `request.start`, `assertion.result`, `request.end`,
  `run.error`, `run.end`. Use this for programmatic introspection.

Three correlation IDs span all three streams:

- `run_id` (32-char lowercase hex) — one per `curlew run` invocation.
- `request_id` (`req-N`) — stable per request within a run.
- `request_slug` (URL-safe) — matches the per-request markdown filename.

## How to narrate

File-path narration is mandatory. Do not paraphrase response bodies; the
markdown file is the canonical view, and copying its contents into chat
risks staleness as the user iterates.

A good narration pattern:

> Three requests ran, one failed (see `responses/run.md`). The failing one
> is `Get user` — the assertion expected `email` to be present, but the
> response had `email: null`. Full context: `responses/get-user.md`.

If a request failed, point at the per-request markdown for the rendered
context, then at `.curlew/run.ndjson` for structured error details. If the
run itself errored before any HTTP request was sent (parse error, undefined
variable, etc.), `responses/run.md` may not exist yet — read stderr or the
`run.error` event in `.curlew/run.ndjson`.

## Failure playbook

| Exit | Meaning | Read this artifact | What to tell the user |
|---|---|---|---|
| 0 | All assertions passed | `responses/run.md` | Summarise the run; link the report. |
| 1 | A main-phase assertion failed | `responses/<slug>.md` (failing request's `## Assertions` section) | Name the failed assertion (operator + expected + actual); link the file. |
| 2 | Guard rail tripped (request limit exceeded) | `.curlew/run.ndjson` (`run.end` event has `exit_code: 2`; skipped requests show `outcome: skipped`) | The collection ran past the safety cap. Suggest splitting into smaller collections. |
| 3 | Configuration error before HTTP fired | stderr (parse error names the YAML line; config error names the missing key; `--only` no-match lists available request names) | Echo the stderr line; tell the user which file/line/option to fix. |
| 4 | Non-assertion runtime error | `.curlew/run.ndjson` (`request.end` event with `outcome: error`, `error.category: network`) | Connection refused / DNS failure / TLS error. Suggest checking the URL and environment variables. |
| 5 | Undefined or circular variable | `.curlew/run.ndjson` (`run.error` event names the variable) | Tell the user which variable is missing and the most likely producer (env file, `--var`, an upstream extract). |
| 6 | Feature gate denied | stderr (`feature_gated` line; json/tap/junit emit a structured gate envelope) | Name the feature, the required tier, and the workaround the CLI suggests. |
| 9 | License grace period expired | stderr (`feature_gated: grace period expired ...`) | Tell the user to run `curlew license --validate` and re-authenticate. |

When the run produced multiple failures, walk the user through them one at
a time, file-by-file.

## Topic files

This skill ships a flat set of reference files alongside `SKILL.md`. Load the
one you need; do not load all of them by default.

- `variables.md` — variable types, the precedence ladder, when each level wins.
- `output-formats.md` — terminal / JSON / TAP / JUnit / HTML / markdown, and when to pick each.
- `assertions.md` — operator assertion catalogue (status, headers, body, JSONPath, timing).
- `retry.md` — retry block, backoff policies, idempotency guard.
- `parallel.md` — parallel waves and worker pool semantics.
- `vault.md` — secret providers and the redaction contract.
- `signing.md` — `aws-sigv4` and `oauth1` request signing.
- `expressions.md` — CEL: `if:`, `assertions: - cel:`, standard activation, disabled functions.
- `exit-codes.md` — master table of every exit code curlew can return.
- `failure-playbook.md` — per-exit-code remediation (and CEL validate errors).

## What to edit

- Always edit the YAML collection. Never hand-roll a `curl` command or
  generate a one-off shell script — the agent's edits should be
  reproducible by the developer.
- When adding a new request, copy the closest existing one as a template
  rather than authoring from scratch. The schema is forgiving but
  hand-authored requests miss conventions (assertion blocks, extract IDs).
- Use `--only "<name>"` to iterate fast on a single request. Setup and
  teardown still run; the rest of the main phase does not. Pair with
  `curlew watch` for tight inner-loop iteration.
- If a request needs a new variable, prefer setting it in
  `environments/<env>.yaml` (commit-safe) over inline literals.

## Notes

- This skill was scaffolded for curlew **v0.0.0-test**. If your
  CLI binary is much newer or older, regenerate it with
  `curlew init --skill agent` in a fresh directory and diff.
- For a human-friendly walk-through of the same workflow, see
  `docs/MANUAL.md` §4.9 ("Driving curlew with an AI agent").
- The skill is checked in. Edit it freely for your team's conventions —
  the next `curlew init --skill agent` will not overwrite a
  pre-existing `.claude/skills/curlew/SKILL.md`.
