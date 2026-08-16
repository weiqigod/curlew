# curlew — Output formats reference

Load this file when the user asks about output format selection, report files,
the NDJSON event stream, or `output:` block configuration.

## Choosing a format

| Format | Best for | Artifact |
|---|---|---|
| `terminal` | Local iteration, human reading | stdout only |
| `markdown` | Agent-driven runs, per-request context | `responses/run.md` + per-request `.md` |
| `json` | Scripting, `jq` pipelines | stdout JSON or `report:` file |
| `tap` | Legacy CI systems expecting TAP | stdout TAP or `report:` file |
| `junit` | JUnit-compatible CI (Jenkins, Azure Pipelines) | `report:` XML file |
| `html` | Shareable standalone report | `report:` HTML file |

For agent-driven runs, use `markdown`. The per-request `.md` files are the
canonical artifacts you should read and narrate.

## The `output:` block

Declare output in `curlew.yaml` so every `curlew run` uses the same format
without flags:

```yaml
output:
  format: markdown
  report: responses/
  events: .curlew/run.ndjson
  verbosity: normal
```

`curlew init --skill agent` scaffolds this block automatically.

### Fields

| Field | Meaning | Default |
|---|---|---|
| `format` | One of the formats above | `terminal` |
| `report` | File or directory for the primary artifact | — (stdout) |
| `events` | Path for the NDJSON event stream | — (disabled) |
| `verbosity` | `silent` / `normal` / `verbose` | `normal` |

## Markdown output

Two artifact types:

- **`responses/run.md`** — summary: total / passed / failed / skipped. Links
  to per-request files. Read this first.
- **`responses/<slug>.md`** — per-request detail: status, headers, body,
  assertions (operator + expected + actual). Each file has a deterministic
  block delimited by `<!-- BEGIN curlew:response ... -->` sentinels so
  re-runs splice only that block.

## NDJSON event stream

Enable with `events: .curlew/run.ndjson`. One JSON object per line:

| Kind | When emitted |
|---|---|
| `run.start` | Before any request fires |
| `request.start` | Before each HTTP call |
| `assertion.result` | After each assertion check |
| `request.end` | After each HTTP response (or error) |
| `run.error` | On fatal error (undefined variable, circular ref, …) |
| `run.end` | After the last request; carries `exit_code` |

Use the event stream for `jq` pipelines, timing analysis, and programmatic
error extraction. Do not parse stdout — it is human-formatted.

## Per-format notes

**JSON** — stdout carries clean JSON when `report:` is omitted. Use
`stdout: clean` (implicit) and pipe to `jq`. With `report: results.json` the
summary JSON lands in the file; stdout shows progress on stderr.

**TAP** — line-oriented; compatible with `tap-parser`, `prove`, and most CI
TAP consumers.

**JUnit** — the XML `<testsuite>` wraps each request as a `<testcase>`.
Failures include the assertion message. Upload to Jenkins, Azure Pipelines, or
GitHub Actions test-results annotations.

**HTML** — a single self-contained `.html` file, suitable for emailing or
attaching to a ticket.
