# curlew — Output formats reference

Load this file when the user asks about output format selection, report files,
the NDJSON event stream, or `output:` block configuration.

## Choosing a format

| Format | Best for | Artifact |
|---|---|---|
| `terminal` | Local iteration, human reading | stdout only |
| `markdown` | Agent-driven runs, per-request context | `responses/run.md` + per-request `.md` |
| `json` | Scripting, `jq` pipelines | stdout JSON (redirect to save) |
| `tap` | Legacy CI systems expecting TAP | stdout TAP (redirect to save) |
| `junit` | JUnit-compatible CI (Jenkins, Azure Pipelines) | stdout XML or `report:` XML file |
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
| `report` | Markdown directory, HTML file or optional JUnit file | Required for Markdown/HTML |
| `events` | Path for the NDJSON event stream | — (disabled) |
| `verbosity` | `quiet` / `normal` / `verbose` / `debug` | `normal` |

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
error extraction. With the default Markdown configuration, use artifacts instead of parsing terminal text. With JSON selected, parse the JSON artifact or stdout as configured.

## Per-format notes

**JSON** — stdout carries clean JSON. Save it with `--format json > results.json`.
The `report:` setting does not redirect JSON; it applies to Markdown, HTML and
JUnit. Keep stderr separate from stdout when parsing JSON.

**TAP** — line-oriented; compatible with `tap-parser`, `prove`, and most CI
TAP consumers.

**JUnit** — the XML `<testsuite>` wraps each request as a `<testcase>`.
Failures include the assertion message. Upload to Jenkins, Azure Pipelines, or
GitHub Actions test-results annotations.

**HTML** — a single self-contained `.html` file, suitable for emailing or
attaching to a ticket.
