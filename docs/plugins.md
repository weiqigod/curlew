# Curlew Plugins

Curlew loads plugins as **external processes** and communicates with them over
JSON-RPC 2.0 on stdin/stdout. Plugins can be written in any language.

## Overview

The external-process model means:

- Plugins are external programs written in any language that implements the wire protocol.
- No shared library ABI concerns.
- OS-level sandboxing is possible (future work; see Security below).
- Language-agnostic: any runtime that can read stdin and write stdout works.

When `curlew plugins list` runs, it discovers plugin executables, spawns each
one, sends a JSON-RPC 2.0 `curlew/hello` handshake request, reads the
response, and prints the plugin's name/version/hooks as a table.

## Quickstart: your first plugin in 5 minutes

The fastest way to see plugins in action is the `hello-plugin` fixture that
ships in the repository:

```bash
# Build the fixture
cd testdata/plugins/hello-plugin && go build -o /tmp/hello-plugin .

# Load it
CURLEW_PLUGINS=/tmp/hello-plugin ./curlew plugins list
```

Expected output:

```
NAME           VERSION  HOOKS
hello-plugin   0.1.0    on_request, on_response
```

For a production-shaped example that submits metrics to Datadog, see
[examples/plugins/datadog-metrics/](../examples/plugins/datadog-metrics/).
That example runs entirely against a local fake server — no Datadog account
needed.

## Discovery: `CURLEW_PLUGINS`

Set `CURLEW_PLUGINS` to a list of plugin executables or directories separated
by `:` (or `;` on Windows — `filepath.ListSeparator`):

```
# Single executable
CURLEW_PLUGINS=/usr/local/lib/curlew-plugins/my-plugin

# Multiple executables
CURLEW_PLUGINS=/path/to/plugin-a:/path/to/plugin-b

# Directory (every executable file inside, alphabetical order)
CURLEW_PLUGINS=/usr/local/lib/curlew-plugins

# Mixed
CURLEW_PLUGINS=/path/to/specific-plugin:/path/to/plugin-dir
```

### Directory expansion

On POSIX, a candidate must be a regular file with an execute bit set. On Windows,
plugin candidates must be regular `.exe` files; extensionless files and script wrappers
such as `.cmd`, `.bat`, and `.ps1` are not plugin executables. Directory expansion
is non-recursive and alphabetical. Unsupported files and subdirectories found in
a directory are silently skipped.

### Error handling

| Situation | Behaviour |
|-----------|-----------|
| Entry not found | `error: plugin <path> not found` to stderr; exit 2 |
| Explicit path is not executable on the current platform | `error: plugin <path> is not executable` to stderr; exit 2 |
| Handshake timeout (> 5 s) | `warning: plugin <path> handshake timeout` to stderr; skipped; exit 0 |
| Two plugins with same `name` | `error: duplicate plugin name <name>` to stderr; exit 2 |
| Unknown hook in response | `warning: plugin <name>: unknown hook <hook> ignored` to stderr; loading continues |

Fatal errors (exit 2) are aggregated: the CLI still tries to load every other
candidate and prints all results, returning the worst exit code (2 > 0).

## Handshake Wire Format

### Request (curlew → plugin)

On load, curlew writes exactly one newline-terminated JSON-RPC 2.0 line to the
plugin's stdin:

```json
{"jsonrpc":"2.0","id":1,"method":"curlew/hello","params":{}}
```

### Response (plugin → curlew)

The plugin must write exactly one newline-terminated JSON-RPC 2.0 line to
stdout **within 5 seconds**:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "name": "my-plugin",
    "version": "1.0.0",
    "hooks": ["on_request", "on_response"],
    "protocol_version": 1
  }
}
```

#### Response fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Unique plugin identifier. ASCII-printable recommended. |
| `version` | string | yes | Plugin version string (semver recommended). |
| `hooks` | array of string | yes | Hook names this plugin handles. |
| `protocol_version` | int | no | Protocol version. Omit or set to `1` for M5-017+. |

Unknown fields in the response are ignored (forward-compatible).

### Supported hooks

| Hook | Description |
|------|-------------|
| `on_request` | Called before each HTTP request is sent; may mutate method/url/headers/body/query_params |
| `on_response` | Called after each HTTP response is received; may attach annotations |
| `on_result` | Called once at run completion with aggregate pass/fail counts and per-test rows |

Unknown hook names in the hello response are ignored with a warning.

## Hook Invocation Protocol (M5-018)

When `curlew run` executes a collection with `CURLEW_PLUGINS` set, the plugin
processes remain alive for the duration of the run. Hooks are invoked over the
same stdin/stdout JSON-RPC 2.0 channel used for the handshake.

### on_request

Called **before** each HTTP request is sent. The plugin receives the current
request and may return a modified version.

**Request from curlew:**

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "curlew/on_request",
  "params": {
    "method": "GET",
    "url": "https://api.example.com/users",
    "headers": {"Authorization": "Bearer token"},
    "body": null,
    "query_params": {"page": "1"}
  }
}
```

**Response from plugin (identity — no mutation):**

```json
{"jsonrpc":"2.0","id":2,"result":{}}
```

**Response from plugin (with mutation):**

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "headers": {"Authorization": "Bearer new-token", "X-Trace": "abc123"}
  }
}
```

#### Mutation rules

- Only **present and non-zero fields** in the result replace the original.
  An empty object `{}` leaves the request unchanged.
- `headers` is a **full replacement** (not a merge) when present.
- Returning a JSON-RPC **error object** aborts the request; the run marks
  the request as `error` (not `fail`) with the plugin's error message.

#### Chaining

When multiple plugins declare `on_request`, they are invoked in the order
listed in `CURLEW_PLUGINS`. Each plugin receives the output of the previous
plugin.

### on_response

Called **after** each HTTP response is received. The plugin may append
annotations; status code, headers, and body are read-only.

**Request from curlew:**

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "curlew/on_response",
  "params": {
    "status_code": 200,
    "headers": {"Content-Type": "application/json"},
    "body": {"id": 1, "name": "Alice"},
    "duration_ms": 142,
    "annotations": []
  }
}
```

**Response from plugin:**

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "annotations": ["latency=142ms", "cache=miss"]
  }
}
```

#### Rules

- The plugin may only append to `annotations`; `status_code`, `headers`,
  and `body` in the response are ignored.
- A JSON-RPC error is logged as a warning; the run is not aborted.

### on_result

Called **once** at run completion, after all phases (setup/main/teardown).

**Request from curlew:**

```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "curlew/on_result",
  "params": {
    "pass_count": 5,
    "fail_count": 1,
    "skip_count": 0,
    "duration_ms": 3200,
    "tests": [
      {"name": "create user", "status": "pass", "duration_ms": 142},
      {"name": "delete user", "status": "fail", "duration_ms": 88, "error": "assertion failed: expected 204, got 200"}
    ]
  }
}
```

**Response from plugin:** any JSON object (ignored by curlew).

#### Rules

- Errors from `on_result` are logged as warnings; the run's exit code is
  unaffected.
- `status` is one of: `pass`, `fail`, `skip`, `error`.

## Timeout and fault behaviour

| Situation | Behaviour |
|-----------|-----------|
| Plugin hook exceeds 10 seconds | Plugin process is killed; warning `plugin <name> timed out on <hook>` printed; run continues without that plugin for all subsequent hooks |
| Plugin process exits unexpectedly | Treated like a timeout; plugin is dropped from all subsequent hooks |
| Plugin returns JSON-RPC error on `on_request` | Request is aborted with status `error`; run continues with remaining requests |
| Plugin returns JSON-RPC error on `on_response` / `on_result` | Warning logged; run continues |

## Known limitations (M5-018)

- Hook timeout (10 s) is hard-coded.
- WebSocket requests do not fire hooks.
- `on_response` annotations are informational (printed by the plugin's
  stderr); they are not yet threaded into request result output.
- Parallel execution (`--parallel`) serialises hooks per plugin: concurrent
  requests queue at the plugin's channel boundary.

### Response length limit

The handshake response must fit within **1 MiB** (1,048,576 bytes). Responses
longer than that will be truncated and treated as a protocol error.

## Exit codes for `curlew plugins list`

| Code | Meaning |
|------|---------|
| 0 | All plugins loaded (warnings may be printed to stderr) |
| 2 | At least one fatal error (missing file, not executable, duplicate name) |

## Full example: datadog-metrics

The [datadog-metrics walkthrough](../examples/plugins/datadog-metrics/README.md)
builds the plugin from the repository root and runs a shipped collection against
a Python loopback fixture. Both the API request and metric submission stay local;
no Datadog account is required. The walkthrough includes prerequisites, exact
commands, tests and optional real-provider configuration.

The executable documentation regression checks the submitted metric payload as
well as the CLI result. The mock does not verify Datadog's production contract.

## Packaging tips

- **Single static binary.** Use `go build -o my-plugin .` from the plugin
  module directory. The resulting binary has no runtime dependencies.
- **Cross-compilation.**
  ```bash
  GOOS=linux GOARCH=amd64 go build -o my-plugin-linux-amd64 .
  GOOS=darwin GOARCH=arm64 go build -o my-plugin-darwin-arm64 .
  ```
- **Separate Go module.** Keep the plugin in its own `go.mod` (like the
  `datadog-metrics` example). This keeps your dependencies isolated from
  Curlew's own dependency graph and signals to readers that the plugin ships
  independently.
- **Naming.** Use `curlew-<name>` as the binary name (e.g.
  `curlew-datadog-metrics`) so it is easy to spot in process lists.
- **Directory layout.** Store plugins in a directory and set
  `CURLEW_PLUGINS=/path/to/plugin-dir`. Curlew will discover every executable
  file inside alphabetically.

## Debugging plugins

### Run the plugin standalone

Plugins are ordinary executables. Run them directly to check they start
without errors:

```bash
./my-plugin --help   # should print metadata and exit 0 (if implemented)
```

### Simulate Curlew's handshake

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"curlew/hello","params":{}}' | ./my-plugin
```

The plugin should respond with a JSON-RPC 2.0 result containing `name`,
`version`, `hooks`, and `protocol_version` within 5 seconds.

### Simulate a hook call

```bash
printf '{"jsonrpc":"2.0","id":1,"method":"curlew/hello","params":{}}\n{"jsonrpc":"2.0","id":2,"method":"curlew/on_response","params":{"status_code":200,"duration_ms":42}}\n' \
  | ./my-plugin
```

### Plugin stderr is shown in curlew output

Anything the plugin writes to stderr appears in `curlew`'s stderr (prefixed
by the plugin name). Use `fmt.Fprintf(os.Stderr, ...)` for diagnostic logging.

### Common failure modes

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Plugin not loaded | Binary not listed in `CURLEW_PLUGINS` or missing execute bit | `chmod +x ./my-plugin`; verify the path in `CURLEW_PLUGINS` |
| Handshake timeout (5 s) | Plugin does expensive work before writing the hello response | Defer initialisation; write the hello response immediately |
| Hook timeout (10 s) | Hook handler blocks on I/O | Add a timeout to any network call |
| Duplicate plugin name | Two binaries return the same `name` in the hello response | Ensure each plugin returns a unique name |
| Hook silently skipped | Plugin hello response lists a hook with an unknown name | Check the exact hook name spelling (`on_request`, `on_response`, `on_result`) |

## Minimum Go example

This is a complete plugin that handles the `curlew/hello` handshake. It is
identical to `testdata/plugins/hello-plugin/main.go` in the repository.

```go
package main

import (
    "bufio"
    "encoding/json"
    "os"
)

type request struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      int             `json:"id"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
    JSONRPC string      `json:"jsonrpc"`
    ID      int         `json:"id"`
    Result  interface{} `json:"result,omitempty"`
}

type hello struct {
    Name            string   `json:"name"`
    Version         string   `json:"version"`
    Hooks           []string `json:"hooks"`
    ProtocolVersion int      `json:"protocol_version"`
}

func main() {
    r := bufio.NewReader(os.Stdin)
    line, err := r.ReadBytes('\n')
    if err != nil {
        os.Exit(1)
    }
    var req request
    if err := json.Unmarshal(line, &req); err != nil {
        os.Exit(1)
    }
    resp := response{
        JSONRPC: "2.0",
        ID:      req.ID,
        Result: hello{
            Name:            "my-plugin",
            Version:         "1.0.0",
            Hooks:           []string{"on_request", "on_response"},
            ProtocolVersion: 1,
        },
    }
    out, _ := json.Marshal(resp)
    _, _ = os.Stdout.Write(append(out, '\n'))
}
```

Build and register:

```bash
go build -o my-plugin .
CURLEW_PLUGINS=./my-plugin curlew plugins list
```

## Troubleshooting checklist

Use this table when a plugin is not behaving as expected.

| Symptom | Likely cause | Resolution |
|---------|--------------|------------|
| `error: plugin <path> not found` | Path in `CURLEW_PLUGINS` is wrong or the binary was not built | Build the binary; verify the path |
| `error: plugin <path> is not executable` | Execute bit not set | `chmod +x <path>` |
| `warning: plugin <path> handshake timeout` | Plugin is slow to start or does not flush stdout | Write the hello response before any slow initialisation; flush stdout explicitly |
| `error: duplicate plugin name <name>` | Two plugins return the same `name` field | Rename one; each loaded plugin must have a unique name |
| `warning: plugin <name>: unknown hook <hook> ignored` | Typo in the hook name returned by hello | Check spelling: `on_request`, `on_response`, `on_result` |
| `warning: plugin <name> timed out on <hook>` | Hook handler takes > 10 s | Add a context-aware timeout to any network or blocking call |
| Plugin loaded but hooks never fire | Plugin declared hooks in hello but curlew was not invoked with `curlew run` | `plugins list` only tests the handshake; hooks fire during `curlew run` |
| `fatal: ...` in plugin stderr | Plugin crashed after the handshake | Run the plugin standalone (see Debugging section) to reproduce |

## Security

**Plugins run with the invoking user's full OS privileges.**

The external-process model does not add any sandboxing. A plugin executable
has access to all files, environment variables, and network resources available
to the `curlew` process.

**Only point `CURLEW_PLUGINS` at plugin executables you trust.** Treat plugin
binaries with the same care you would apply to other executables you run on
your machine.

A sandboxing story (Linux seccomp, macOS `sandbox-exec`) is planned as future
work and is not part of M5-017.
