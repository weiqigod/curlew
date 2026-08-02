# datadog-metrics plugin

Submits an `curlew.request.duration` gauge metric to Datadog for every HTTP
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

## Build

```bash
# From the repository root
cd examples/plugins/datadog-metrics
go build -o /tmp/curlew-dd-plugin .
```

## Run

### Against a local mock server (recommended for demos, no Datadog account needed)

Start a minimal HTTP server in one terminal that accepts POST requests and
returns 202:

```bash
# Python 3 one-liner — accepts all requests and responds 200
python3 -c "
import http.server, json

class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        self.send_response(202); self.end_headers()
    def log_message(self, *a): pass

http.server.HTTPServer(('127.0.0.1', 8888), H).serve_forever()
"
```

In a second terminal, run curlew with the plugin:

```bash
DD_API_URL=http://127.0.0.1:8888 \
CURLEW_PLUGINS=/tmp/curlew-dd-plugin \
DATADOG_API_KEY=test-key \
  ./curlew run testdata/plugins/one-request.yaml
```

Expected output (stderr tail):

```
[plugin:datadog-metrics] submitted 1 metric
```

### Against real Datadog

```bash
CURLEW_PLUGINS=/tmp/curlew-dd-plugin \
DATADOG_API_KEY=<your-key> \
  ./curlew run your-collection.yaml
```

## Test

The test suite runs entirely against an `httptest.Server` — no Datadog account
or network access needed:

```bash
cd examples/plugins/datadog-metrics
go test ./...
```

## Standalone invocation

Run the binary with `--help` to print plugin metadata without entering the
JSON-RPC loop:

```bash
/tmp/curlew-dd-plugin --help
```

Output:

```
Plugin:   datadog-metrics
Version:  0.1.0
Hooks:    on_response, on_result
Protocol: 1
...
```

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
