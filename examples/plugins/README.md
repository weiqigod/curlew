# Plugin Examples

This directory contains example Curlew plugins. Each plugin lives in its own
subdirectory with a separate Go module and a self-contained test suite that
requires no real external service accounts.

## Examples

| Plugin | Description | Hooks |
|--------|-------------|-------|
| [datadog-metrics](datadog-metrics/) | Submits `curlew.request.duration` gauge metrics to Datadog for every HTTP response | `on_response`, `on_result` |

## Quick start

1. Pick an example and read its `README.md`.
2. Build the plugin binary:
   ```bash
   cd datadog-metrics
   go build -o /tmp/my-plugin .
   ```
3. Run Curlew with the plugin enabled:
   ```bash
   CURLEW_PLUGINS=/tmp/my-plugin ./curlew run your-collection.yaml
   ```

For the full plugin developer guide — handshake protocol, hook reference,
packaging tips, debugging, and security — see
[docs/plugins.md](../../docs/plugins.md) in the repository root.

## Writing your own plugin

Plugins are ordinary executables that communicate with Curlew over JSON-RPC 2.0
on stdin/stdout. You can write them in Go, Python, Bash, Rust, or any other
language that can read from stdin and write to stdout.

See `docs/plugins.md` → **Quickstart** for a step-by-step guide and
`docs/plugins.md` → **Full example: datadog-metrics** for a tour of the code
in this directory.
