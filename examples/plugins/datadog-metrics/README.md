# datadog-metrics plugin

Submits a `curlew.request.duration` gauge metric to Datadog for every HTTP
response observed by Curlew via the `on_response` hook.

## Configuration

| Environment variable | Required | Description |
|----------------------|----------|-------------|
| `DATADOG_API_KEY` | yes | Datadog API key. When absent or blank, the plugin starts in disabled mode: it still participates in the handshake but never submits metrics. |
| `DD_API_URL` | no | Override the full Datadog API base URL (no trailing slash). Use this to point the plugin at a local mock server during development or testing. Example: `http://127.0.0.1:8888`. Overrides `DD_SITE`. |
| `DD_SITE` | no | Datadog site. Defaults to `datadoghq.com`. Common values: `datadoghq.eu`, `us3.datadoghq.com`. Ignored when `DD_API_URL` is set. |

> **Security note:** Use a Datadog API key scoped to a sandbox organisation when
> experimenting. The plugin submits live metrics when `DATADOG_API_KEY` and
> network access are both present.

## Prerequisites

Run the following commands from the repository root. Install Go 1.24+, Python 3,
and put `curlew` on `PATH` (see [installation](../../../README.md#install)).
The plugin is a separate Go module. Build/test commands below use a subshell so
your terminal stays at the repository root.

## Start the fixture

In a separate terminal, from the repository root:

```bash
python3 examples/local-server.py
```

This server handles the test GET request locally and accepts metric POSTs with
HTTP 202. It prints `http://127.0.0.1:18081`. Stop it with Ctrl-C when finished.
For an available ephemeral port, use `--port 0` and set `EXAMPLE_URL` in the
other terminal to the printed address.

## Run locally

This builds the plugin, shows its metadata, and runs the shipped collection.
Both the tested API and metric destination use the local fixture. The dummy key
only enables the plugin; no Datadog account is needed.

```bash
example_bin=$(mktemp -d)
trap 'rm -rf "$example_bin"' EXIT
(cd examples/plugins/datadog-metrics && go build -o "$example_bin/datadog-metrics" .)
"$example_bin/datadog-metrics" --help
DD_API_URL="${EXAMPLE_URL:-http://127.0.0.1:18081}" \
DATADOG_API_KEY=local-example-key \
CURLEW_PLUGINS="$example_bin/datadog-metrics" \
  curlew run examples/plugins/datadog-metrics/collection.yaml \
    --var "base_url=${EXAMPLE_URL:-http://127.0.0.1:18081}"
```

The collection reports one passing request, and stderr includes
`[plugin:datadog-metrics] submitted 1 metric`.

The fixture's `/observations` endpoint returns the received GET count and
submitted JSON payloads. The regression test checks those payloads, so a
passing request alone cannot hide failed metric delivery. This mock does not
validate Datadog's production API contract.

## Test

The plugin tests use local `httptest.Server` fixtures; they need loopback sockets
but no external service or credentials.

```bash
(cd examples/plugins/datadog-metrics && go test ./...)
```

## Real Datadog (optional)

Build a persistent plugin binary from the repository root:

```bash
(cd examples/plugins/datadog-metrics && go build -o /tmp/curlew-dd-plugin .)
```

For a real service, set `DATADOG_API_KEY` in your environment and point
`CURLEW_PLUGINS` at that binary when running your own collection. Unset
`DD_API_URL` to use `DD_SITE` (default `datadoghq.com`). This submits live metrics;
it requires your account and can affect your usage charges. The local recipe
above is the tested example; no live Datadog account is used in verification.

## Metric shape

Each `on_response` fires a single POST to `/api/v2/series` with:

```json
{
  "series": [{
    "metric": "curlew.request.duration",
    "type": 3,
    "points": [{"timestamp": <unix>, "value": <duration_ms>}],
    "tags": ["status:<status_code>"]
  }]
}
```

## Caveats

- **Rate limits:** High-QPS test suites may hit Datadog's
  [Metrics API rate limits](https://docs.datadoghq.com/api/latest/metrics/).
  Pre-aggregation or increasing your Datadog plan limits are your options; this
  example intentionally keeps the code simple and does not implement retry.
- **Tag cardinality:** Submitting per-request tags (e.g. full URL) can balloon
  custom metric counts. The example uses `status:<code>` only.
- **Timeout:** The plugin uses a 5-second HTTP timeout per submission. If
  Datadog is unreachable, the submit failure is logged as a warning and the
  run continues.
