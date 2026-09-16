# Plugin examples

Each plugin is a separate Go module with a self-contained test suite.

| Plugin | Description | Hooks |
|--------|-------------|-------|
| [datadog-metrics](datadog-metrics/README.md) | Sends a request-duration gauge for each HTTP response | `on_response`, `on_result` |

## Quick start

Follow the [complete local walkthrough](datadog-metrics/README.md#prerequisites).
It builds the plugin from the repository root, starts a loopback fixture and
runs a shipped collection. Both the API request and metric submission stay
local; no Datadog account is required. Commands preserve the working directory.

## Writing your own plugin

Plugins are executables communicating with Curlew over JSON-RPC 2.0 on
stdin/stdout. This protocol is not MCP. See the
[plugin developer guide](../../docs/plugins.md) for the handshake, hooks,
packaging and debugging. The example implementation is Go; another language
can implement the same protocol.
