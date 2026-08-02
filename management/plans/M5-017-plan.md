# Implementation Plan: M5-017

## Overview

Introduce a new `internal/plugin` package and a top-level `curlew plugins list` subcommand that discovers, spawns, and handshakes with external-process plugins over JSON-RPC 2.0 on stdin/stdout. Deliver a sample `testdata/plugins/hello-plugin` fixture that is the canonical example of the plugin contract, plus `docs/plugins.md` documenting the handshake schema and the `CURLEW_PLUGINS` env-var surface. M5-017 is the pure discovery/handshake slice; actual hook invocation (`on_request`, `on_response`) is explicitly out of scope — it lands in M5-018 and M5-019.

## Task Details

- **ID:** M5-017
- **Title:** go-cli: plugin interface + loader (external-process model)
- **Phase:** M5: Enterprise Tier
- **Priority:** 3
- **Complexity:** high
- **Branch:** `feature/M5-017-plugin-interface`

## Dependencies

None. (Standalone slice; M5-018 and M5-019 build on this.)

## Architectural Decisions

Several decisions fall outside the task YAML; documented here so the executor follows them unless a hard constraint forces a deviation.

1. **External-process + JSON-RPC 2.0 on stdin/stdout.** Locked by the task YAML scope. Go's `plugin` package is Linux/Mac only and ABI-brittle; WASM via wazero would add ~3 MB of runtime and a sandbox story the spec does not request. External-process keeps plugins portable, language-agnostic, and OS-sandboxable. The newline-delimited JSON-RPC 2.0 framing (one JSON object per line, UTF-8, `\n`-terminated) is chosen over the LSP-style `Content-Length`-framed variant because it is simpler to emit from scripting languages and matches what popular tools like `jq`, `git-credential-helper`, and `dap-mode` plugins already do. The handshake is a synchronous request/response over the same pipe pair; later slices (M5-018/019) can multiplex additional `curlew/hookCall` requests on the same channel.

2. **JSON-RPC handshake method name: `curlew/hello`.** Chosen for two reasons: (a) namespacing with `curlew/` prevents collisions if plugins also host their own methods; (b) `hello` matches the scope's "Hello handshake" wording. Request params are empty (`{}`); the response shape is:
   ```json
   {
     "name": "hello-plugin",
     "version": "0.1.0",
     "hooks": ["on_request", "on_response"],
     "protocol_version": 1
   }
   ```
   `protocol_version: 1` is fixed for M5-017. Unknown fields in the response are ignored (forward-compat); the host does not send unknown request fields.

3. **Known hooks.** For M5-017 the complete set of recognized hook names is `on_request` and `on_response` (matching the observable table). A plugin that declares `on_error`, `post_assertion`, or anything else prints `warning: plugin <name>: unknown hook <hook> ignored` to stderr and loading continues with the unknown hook stripped from the registered capability list. (Behavior 4 requirement.)

4. **Discovery semantics for `CURLEW_PLUGINS`.** Rules:
   - Unset or empty → no plugins loaded, list shows only a `No plugins configured.` line and exits 0.
   - Value is a `:`-separated list of entries on Unix, `;`-separated on Windows (matches `PATH` semantics; uses `filepath.ListSeparator`).
   - Each entry is `os.Stat`'d:
     - Regular file with execute bit → single plugin candidate.
     - Directory → every **regular file** directly inside (non-recursive) with the execute bit set, in sorted alphabetical order, becomes a candidate. Subdirectories are skipped silently.
     - Missing path → `error: plugin <path> not found` on stderr; other entries continue; exit code 2 at end.
     - Not-a-regular-file and not-a-directory (e.g. symlink to nothing, device file) → `error: plugin <path> is not executable` on stderr; exit 2.
   - A regular file without the execute bit → `error: plugin <path> is not executable`, exit 2 at end (behavior 6).
   - Exit-2 errors from any entry are aggregated: the CLI still attempts to load every other candidate, prints all handshake results in order, and returns the worst exit code seen (2 > 0).

5. **Duplicate name detection.** Plugins are canonicalized by the `name` field they report in their hello response. If two loaded plugins report the same `name`, the CLI prints `error: duplicate plugin name <name>` to stderr and exits 2 (behavior 5). Detection happens **after** all handshakes complete so the error wins over other non-fatal warnings. If a plugin times out or fails the handshake, it does not participate in the duplicate check (it was never successfully loaded).

6. **Handshake timeout: 5 seconds.** Hard-coded per behavior 2. Implemented via `context.WithTimeout` around the blocking read of the response line. On timeout, the host sends `SIGKILL` to the plugin process (after a 500 ms grace for SIGTERM), prints `error: plugin <path> handshake timeout`, and continues with other plugins. The command still exits 0 when the only failure is a timeout on a non-duplicate plugin — behavior 2 says "skips it (exit 0 for the list command)". This is different from exit-2 errors (missing file / not executable / duplicate name), which are fatal.

7. **Plugin lifecycle for `plugins list`.** After the handshake response is received (or times out), the plugin is told to shut down: the host closes stdin, waits up to 2 seconds for the process to exit, then sends SIGKILL. Plugins that do not exit after stdin-close are not an error condition for the list command (some plugins legitimately run servers); they are just killed. For M5-018+ the plugin stays alive for the duration of the run — the API surface is designed so the same `Host` object can be reused for hook calls later, but M5-017 spawns-and-shuts-down per `plugins list`.

8. **Output format: fixed-width table.** The observable pins the format exactly:
   ```
   NAME         VERSION   HOOKS
   hello-plugin 0.1.0     on_request,on_response
   ```
   Columns are tab-padded to the longest value per column with a two-space minimum gap between columns; header is always printed even with zero plugins. `HOOKS` is the comma-separated list in the plugin's declaration order (with unknown hooks filtered). No `--format json` variant in M5-017 — that is future scope.

9. **Package layout.** Single package `internal/plugin` to keep the M5-017 surface compact; later slices can split out `internal/plugin/host`, `internal/plugin/jsonrpc`, etc. if the file grows past ~800 LOC. Public API:
   ```go
   package plugin
   type Plugin struct {
       Name    string
       Version string
       Hooks   []string
       Path    string
   }
   type Host struct { /* unexported fields */ }
   func NewHost(stderr io.Writer) *Host
   func (h *Host) Load(ctx context.Context, pluginsEnv string) ([]Plugin, []LoadError, error)
   func (h *Host) Close() error // Shutdown all running plugin processes
   type LoadError struct { Path, Message string; Fatal bool }
   var (
       ErrHandshakeTimeout  = errors.New("handshake timeout")
       ErrDuplicateName     = errors.New("duplicate plugin name")
       ErrNotExecutable     = errors.New("not executable")
       ErrHandshakeProtocol = errors.New("handshake protocol error")
   )
   ```
   Rationale: `LoadError` carries per-plugin non-fatal warnings (timeouts, unknown hooks) so the CLI can print them and compute the aggregate exit code. The top-level `error` return is only used for programmer errors (e.g., `stderr` is nil) — never for plugin failures.

10. **Process-spawn abstraction for testability.** Expose `Host` fields behind an interface so tests can substitute an in-process JSON-RPC endpoint without spawning subprocesses:
    ```go
    type spawner interface {
        Spawn(ctx context.Context, path string) (stdin io.WriteCloser, stdout io.ReadCloser, kill func(), err error)
    }
    func NewHostWithSpawner(stderr io.Writer, s spawner) *Host
    ```
    Default spawner uses `exec.CommandContext`. Tests use an `io.Pipe`-backed spawner that runs the fake plugin as a goroutine. This lets the test suite run without compiling the fixture binary, which keeps `go test ./internal/plugin/...` fast (<1 s).

11. **Sample plugin fixture: `testdata/plugins/hello-plugin/main.go`.** A standalone `package main` that reads one line from stdin, decodes it as a JSON-RPC request, and writes one line with the canonical response (name=hello-plugin, version=0.1.0, hooks=[on_request, on_response]). It uses the standard library only — no `encoding/gob`, no reflect. To prevent the Go test runner from trying to test the fixture, the directory uses `//go:build pluginfixture` on the file, OR the fixture lives in a directory that does not match `*_test.go` discovery (simpler). Decision: use a distinct `package main` with no special build tag so `go build -o /tmp/... ./testdata/plugins/hello-plugin` works without flags. The fixture's tests are not in that directory — they are part of `internal/plugin` suite which spawns the built binary in one integration test.

12. **Integration test strategy.** Two tiers:
    - **Unit tests in `internal/plugin`** — use the in-process spawner to drive ~15 table cases without building the fixture.
    - **One integration test** (`TestHost_Load_WithRealFixture`) that `go build`s `testdata/plugins/hello-plugin` into `t.TempDir()`, sets up `CURLEW_PLUGINS`, calls `Host.Load`, and asserts the round-trip. Gated on `testing.Short()` → skip in `-short` mode (so `go test -short` stays <500 ms). Gated on the presence of a working `go` binary on PATH — if missing, test is skipped with a diagnostic.

13. **CLI-level test strategy.** `cmd/curlew/plugins_test.go` drives `run([]string{"plugins", "list"})` end-to-end through the in-process spawner (injected via a package-level hook `pluginsHostFactory = plugin.NewHost`). 6 table cases covering: happy path with one plugin, zero plugins (empty env), two plugins directory, timeout plugin, unknown hook warning, duplicate name error. These replace any need for `exec.Command`-level e2e in the CLI package.

14. **Smoke test addition.** Append a `=== Plugins (M5-017) ===` block to `smoke/run.sh` that:
    1. Builds the fixture: `go build -o "$PLUGIN_DIR/hello-plugin" ./testdata/plugins/hello-plugin`.
    2. Runs `CURLEW_PLUGINS="$PLUGIN_DIR/hello-plugin" ./curlew plugins list` and greps for `hello-plugin 0.1.0`.
    3. Runs with a directory value and asserts same output.
    4. Runs with `CURLEW_PLUGINS="/nonexistent"` and asserts exit 2 with stderr match.
    5. Cleans up temp directory.

15. **Help text.** `printHelp()` in `main.go` gets a new `Commands` entry: `plugins         Manage external-process plugins` and a new `Plugins Options:` section documenting `CURLEW_PLUGINS`. Behavior 7 is satisfied by the root `--help`. The subcommand `curlew plugins --help` prints a focused help block (mirrors `printVaultHelp()` pattern).

16. **No tier gate in M5-017.** The observable and behaviors do not include any tier-gating check, and exit codes listed in behaviors (0, 2) do not include the `6` that `auth.CheckFeature` would return. The plugin system is documented as Enterprise in `docs/SPECIFICATION.md` but the gate is a concern for M5-018/M5-019 when real hook execution lands (that is where the value unlock happens). M5-017 just lists registered plugins — no billing-relevant behavior. Noted in `docs/plugins.md`.

17. **Security posture documented, not enforced.** M5-017 does not sandbox plugin processes. It spawns them with the host's full environment and working directory. `docs/plugins.md` explicitly calls out that plugin executables run with the invoking user's privileges and that users should only point `CURLEW_PLUGINS` at trusted binaries. A sandboxing story (seccomp on Linux, `sandbox-exec` on macOS) is future work. This is consistent with the task YAML (no mention of sandboxing) and with how other CLI tools handle plugin trust (git, kubectl).

18. **`protocol_version` mismatch handling.** If a plugin responds with `protocol_version: 2` (or any non-1 integer) the host prints `warning: plugin <name>: unsupported protocol_version N, using 1` to stderr and continues with version-1 semantics. If the field is missing or zero, default to 1. This is forward-compat so today's CLI can at least see a future plugin's name/version rather than refusing to list it.

19. **JSON-RPC error responses in handshake.** If the plugin responds with `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}` the host prints `error: plugin <path>: handshake error: <message>` and skips the plugin (non-fatal; exit 0). If the response is unparseable as JSON-RPC 2.0 at all (missing `jsonrpc` field, malformed JSON), the host prints `error: plugin <path>: invalid handshake response` and skips the plugin (non-fatal).

20. **Documentation file.** `docs/plugins.md` is mandatory per DoD. It includes:
    - Overview of the external-process model
    - `CURLEW_PLUGINS` syntax (file, directory, list-separator)
    - Handshake wire format (request shape, response shape, example)
    - Current supported hooks (`on_request`, `on_response`) with a note that M5-017 only lists them — invocation is M5-018/019
    - Exit codes from `plugins list`
    - Minimum Go example (~25 lines)
    - Security warning (per decision 17)

## Implementation Steps

Step ordering is smallest blast radius first — pure JSON-RPC types and codec, then Host, then CLI wiring, then fixture + docs + smoke.

---

### Step 1: Package skeleton + handshake types + JSON-RPC codec

**Rationale:** Pure types and I/O helpers with no process-spawn concerns. Unit-testable with `strings.Reader` / `bytes.Buffer`. Unblocks Step 2 without touching anything outside `internal/plugin/`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/plugin/plugin.go` | create | Package doc, `Plugin` struct, `LoadError`, sentinel errors |
| `internal/plugin/jsonrpc.go` | create | Minimal JSON-RPC 2.0 request/response codec (newline-framed) |
| `internal/plugin/jsonrpc_test.go` | create | Table-driven round-trip + malformed-input tests |
| `internal/plugin/handshake.go` | create | `helloRequest`, `helloResponse`, `validateHello` |
| `internal/plugin/handshake_test.go` | create | Response validation tests |

#### New Code

```go
// internal/plugin/plugin.go
package plugin

import "errors"

// Plugin describes a successfully-loaded external plugin.
type Plugin struct {
    Name    string
    Version string
    Hooks   []string
    Path    string
}

// LoadError carries a per-plugin load failure or warning.
// Fatal=true ⇒ CLI exit code 2; Fatal=false ⇒ stderr message, exit still 0.
type LoadError struct {
    Path    string
    Message string
    Fatal   bool
}

// Sentinel errors returned via LoadError.Message context or wrapped upstream.
var (
    ErrHandshakeTimeout  = errors.New("handshake timeout")
    ErrDuplicateName     = errors.New("duplicate plugin name")
    ErrNotExecutable     = errors.New("not executable")
    ErrHandshakeProtocol = errors.New("handshake protocol error")
)

// Known hooks understood by M5-017. Unknown hooks in a plugin's response are
// dropped with a warning; see Host.Load.
var knownHooks = map[string]struct{}{
    "on_request":  {},
    "on_response": {},
}
```

```go
// internal/plugin/jsonrpc.go
package plugin

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
)

type rpcRequest struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      int             `json:"id"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

type rpcResponse struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      int             `json:"id"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *rpcError       `json:"error,omitempty"`
}

// writeRequest writes one newline-framed JSON-RPC request.
func writeRequest(w io.Writer, req rpcRequest) error {
    req.JSONRPC = "2.0"
    data, err := json.Marshal(req)
    if err != nil {
        return fmt.Errorf("encode request: %w", err)
    }
    if _, err := w.Write(append(data, '\n')); err != nil {
        return fmt.Errorf("write request: %w", err)
    }
    return nil
}

// readResponse reads one newline-framed JSON-RPC response from r.
// Returns ErrHandshakeProtocol-wrapped errors for malformed responses.
func readResponse(r *bufio.Reader) (rpcResponse, error) {
    line, err := r.ReadBytes('\n')
    if err != nil {
        return rpcResponse{}, fmt.Errorf("read response: %w", err)
    }
    var resp rpcResponse
    if err := json.Unmarshal(line, &resp); err != nil {
        return rpcResponse{}, fmt.Errorf("%w: %v", ErrHandshakeProtocol, err)
    }
    if resp.JSONRPC != "2.0" {
        return rpcResponse{}, fmt.Errorf("%w: missing jsonrpc field", ErrHandshakeProtocol)
    }
    return resp, nil
}
```

```go
// internal/plugin/handshake.go
package plugin

import (
    "encoding/json"
    "fmt"
)

type helloResponse struct {
    Name            string   `json:"name"`
    Version         string   `json:"version"`
    Hooks           []string `json:"hooks"`
    ProtocolVersion int      `json:"protocol_version"`
}

// parseHello decodes a rpcResponse.Result into a helloResponse, validating
// required fields.
func parseHello(raw json.RawMessage) (helloResponse, error) {
    var h helloResponse
    if err := json.Unmarshal(raw, &h); err != nil {
        return h, fmt.Errorf("%w: decode hello: %v", ErrHandshakeProtocol, err)
    }
    if h.Name == "" {
        return h, fmt.Errorf("%w: missing name", ErrHandshakeProtocol)
    }
    if h.Version == "" {
        return h, fmt.Errorf("%w: missing version", ErrHandshakeProtocol)
    }
    if h.ProtocolVersion == 0 {
        h.ProtocolVersion = 1
    }
    return h, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/plugin/jsonrpc_test.go
func TestWriteRequest_RoundTrip(t *testing.T) {
    var buf bytes.Buffer
    if err := writeRequest(&buf, rpcRequest{ID: 1, Method: "curlew/hello"}); err != nil {
        t.Fatal(err)
    }
    got := buf.String()
    if !strings.HasSuffix(got, "\n") {
        t.Errorf("missing trailing newline: %q", got)
    }
    var back rpcRequest
    if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &back); err != nil {
        t.Fatal(err)
    }
    if back.JSONRPC != "2.0" || back.Method != "curlew/hello" {
        t.Errorf("round-trip mismatch: %+v", back)
    }
}

func TestReadResponse(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr error // sentinel or nil
        check   func(*testing.T, rpcResponse)
    }{
        {"valid hello result", `{"jsonrpc":"2.0","id":1,"result":{"name":"x","version":"1","hooks":[],"protocol_version":1}}` + "\n", nil, func(t *testing.T, r rpcResponse) {
            if r.ID != 1 { t.Error("id") }
        }},
        {"missing jsonrpc field", `{"id":1,"result":{}}` + "\n", ErrHandshakeProtocol, nil},
        {"malformed json", "not-json\n", ErrHandshakeProtocol, nil},
        {"eof before newline", "", io.EOF, nil},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            r := bufio.NewReader(strings.NewReader(tc.input))
            resp, err := readResponse(r)
            if tc.wantErr != nil {
                if !errors.Is(err, tc.wantErr) {
                    t.Fatalf("want %v, got %v", tc.wantErr, err)
                }
                return
            }
            if err != nil { t.Fatal(err) }
            if tc.check != nil { tc.check(t, resp) }
        })
    }
}
```

```go
// internal/plugin/handshake_test.go
func TestParseHello(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantErr  error
        wantName string
        wantPV   int
    }{
        {"full response", `{"name":"hello","version":"0.1","hooks":["on_request"],"protocol_version":1}`, nil, "hello", 1},
        {"missing protocol_version defaults to 1", `{"name":"hello","version":"0.1","hooks":[]}`, nil, "hello", 1},
        {"empty name", `{"name":"","version":"0.1","hooks":[]}`, ErrHandshakeProtocol, "", 0},
        {"empty version", `{"name":"x","version":"","hooks":[]}`, ErrHandshakeProtocol, "", 0},
        {"malformed json", `{"name":`, ErrHandshakeProtocol, "", 0},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            h, err := parseHello(json.RawMessage(tc.input))
            if tc.wantErr != nil {
                if !errors.Is(err, tc.wantErr) { t.Fatalf("want %v, got %v", tc.wantErr, err) }
                return
            }
            if err != nil { t.Fatal(err) }
            if h.Name != tc.wantName || h.ProtocolVersion != tc.wantPV {
                t.Errorf("got %+v", h)
            }
        })
    }
}
```

#### Impact on Existing Tests
No existing tests affected (new package).

---

### Step 2: Discovery (`CURLEW_PLUGINS` parsing) and executable checks

**Rationale:** Pure filesystem logic, no process spawning. Covers half of the error paths (missing file, not executable, directory expansion) with a filesystem-only test.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/plugin/discover.go` | create | `discover(env string) (candidates []string, errs []LoadError)` |
| `internal/plugin/discover_test.go` | create | Table-driven discovery test using `t.TempDir()` fixtures |

#### New Code

```go
// internal/plugin/discover.go
package plugin

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "strings"
)

// discover returns an alphabetical list of candidate executable paths plus any
// per-entry errors. It does not spawn anything.
func discover(env string) (candidates []string, errs []LoadError) {
    if env == "" {
        return nil, nil
    }
    sep := string(filepath.ListSeparator)
    for _, entry := range strings.Split(env, sep) {
        if entry == "" {
            continue
        }
        info, err := os.Stat(entry)
        if err != nil {
            if errors.Is(err, os.ErrNotExist) {
                errs = append(errs, LoadError{Path: entry,
                    Message: fmt.Sprintf("plugin %s not found", entry), Fatal: true})
            } else {
                errs = append(errs, LoadError{Path: entry,
                    Message: fmt.Sprintf("plugin %s: %v", entry, err), Fatal: true})
            }
            continue
        }
        if info.IsDir() {
            dirEntries, readErr := os.ReadDir(entry)
            if readErr != nil {
                errs = append(errs, LoadError{Path: entry,
                    Message: fmt.Sprintf("plugin %s: %v", entry, readErr), Fatal: true})
                continue
            }
            var found []string
            for _, de := range dirEntries {
                if de.IsDir() {
                    continue
                }
                full := filepath.Join(entry, de.Name())
                de2, statErr := os.Stat(full)
                if statErr != nil || !de2.Mode().IsRegular() {
                    continue
                }
                if !isExecutable(de2.Mode()) {
                    errs = append(errs, LoadError{Path: full,
                        Message: fmt.Sprintf("plugin %s is not executable", full), Fatal: true})
                    continue
                }
                found = append(found, full)
            }
            sort.Strings(found)
            candidates = append(candidates, found...)
            continue
        }
        if !info.Mode().IsRegular() {
            errs = append(errs, LoadError{Path: entry,
                Message: fmt.Sprintf("plugin %s is not executable", entry), Fatal: true})
            continue
        }
        if !isExecutable(info.Mode()) {
            errs = append(errs, LoadError{Path: entry,
                Message: fmt.Sprintf("plugin %s is not executable", entry), Fatal: true})
            continue
        }
        candidates = append(candidates, entry)
    }
    return candidates, errs
}

// isExecutable tests any-user execute bit. On Windows the concept differs;
// file is considered executable if the extension is in PATHEXT. For M5-017
// behavior 6 is tested on Unix only (fixture + smoke) so we use Unix semantics.
func isExecutable(m os.FileMode) bool {
    return m&0o111 != 0
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/plugin/discover_test.go
func TestDiscover(t *testing.T) {
    // Helper: make a temp file with mode.
    mk := func(t *testing.T, dir, name string, mode os.FileMode) string {
        t.Helper()
        p := filepath.Join(dir, name)
        if err := os.WriteFile(p, []byte("#!/bin/sh\n"), mode); err != nil { t.Fatal(err) }
        return p
    }

    t.Run("empty env returns nothing", func(t *testing.T) {
        cands, errs := discover("")
        if len(cands) != 0 || len(errs) != 0 {
            t.Errorf("got cands=%v errs=%v", cands, errs)
        }
    })

    t.Run("single executable file", func(t *testing.T) {
        dir := t.TempDir()
        p := mk(t, dir, "plug", 0o755)
        cands, errs := discover(p)
        if len(cands) != 1 || cands[0] != p { t.Errorf("cands=%v", cands) }
        if len(errs) != 0 { t.Errorf("errs=%v", errs) }
    })

    t.Run("non-executable file produces fatal error", func(t *testing.T) {
        dir := t.TempDir()
        p := mk(t, dir, "plug", 0o644)
        cands, errs := discover(p)
        if len(cands) != 0 || len(errs) != 1 || !errs[0].Fatal ||
            !strings.Contains(errs[0].Message, "is not executable") {
            t.Errorf("cands=%v errs=%+v", cands, errs)
        }
    })

    t.Run("missing path produces fatal error", func(t *testing.T) {
        _, errs := discover("/does/not/exist/xyz")
        if len(errs) != 1 || !errs[0].Fatal ||
            !strings.Contains(errs[0].Message, "not found") {
            t.Errorf("errs=%+v", errs)
        }
    })

    t.Run("directory expands to sorted executables", func(t *testing.T) {
        dir := t.TempDir()
        _ = mk(t, dir, "zplug", 0o755)
        _ = mk(t, dir, "aplug", 0o755)
        _ = mk(t, dir, "readme.txt", 0o644) // ignored
        cands, errs := discover(dir)
        if len(cands) != 2 { t.Fatalf("cands=%v", cands) }
        if filepath.Base(cands[0]) != "aplug" || filepath.Base(cands[1]) != "zplug" {
            t.Errorf("not sorted: %v", cands)
        }
        if len(errs) != 0 { t.Errorf("errs=%+v", errs) }
    })

    t.Run("directory with non-executable file produces fatal error", func(t *testing.T) {
        dir := t.TempDir()
        _ = mk(t, dir, "good", 0o755)
        _ = mk(t, dir, "bad", 0o644)
        cands, errs := discover(dir)
        if len(cands) != 1 { t.Errorf("cands=%v", cands) }
        if len(errs) != 1 || !errs[0].Fatal { t.Errorf("errs=%+v", errs) }
    })

    t.Run("list separator splits entries", func(t *testing.T) {
        dir := t.TempDir()
        a := mk(t, dir, "a", 0o755)
        b := mk(t, dir, "b", 0o755)
        env := a + string(filepath.ListSeparator) + b
        cands, _ := discover(env)
        if len(cands) != 2 { t.Errorf("cands=%v", cands) }
    })
}
```

#### Impact on Existing Tests
None.

---

### Step 3: Host — spawner seam + handshake orchestration

**Rationale:** Introduces the `Host` type and the in-process spawner used by all downstream tests. Depends on Steps 1 and 2.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/plugin/host.go` | create | `Host`, `spawner`, `execSpawner`, `NewHost`, `NewHostWithSpawner`, `Load`, `Close` |
| `internal/plugin/host_test.go` | create | In-process spawner tests for handshake round-trip, timeout, unknown hook warning, duplicate name, protocol error |
| `internal/plugin/fakeplugin_test.go` | create | Helper: in-process `fakePlugin` that implements spawner for tests |

#### New Code

```go
// internal/plugin/host.go
package plugin

import (
    "bufio"
    "context"
    "errors"
    "fmt"
    "io"
    "os/exec"
    "sync"
    "time"
)

const (
    handshakeTimeout = 5 * time.Second
    shutdownTimeout  = 2 * time.Second
)

// spawner abstracts process launching so tests can substitute in-process pipes.
type spawner interface {
    Spawn(ctx context.Context, path string) (stdin io.WriteCloser, stdout io.ReadCloser, kill func(), err error)
}

type execSpawner struct{}

func (execSpawner) Spawn(ctx context.Context, path string) (io.WriteCloser, io.ReadCloser, func(), error) {
    cmd := exec.CommandContext(ctx, path)
    stdin, err := cmd.StdinPipe()
    if err != nil {
        return nil, nil, nil, fmt.Errorf("stdin pipe: %w", err)
    }
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return nil, nil, nil, fmt.Errorf("stdout pipe: %w", err)
    }
    if err := cmd.Start(); err != nil {
        return nil, nil, nil, fmt.Errorf("start: %w", err)
    }
    kill := func() {
        // Best-effort SIGTERM, then SIGKILL.
        _ = cmd.Process.Signal(interruptSignal())
        done := make(chan struct{})
        go func() { _ = cmd.Wait(); close(done) }()
        select {
        case <-done:
        case <-time.After(500 * time.Millisecond):
            _ = cmd.Process.Kill()
            <-done
        }
    }
    return stdin, stdout, kill, nil
}

// Host is the plugin loader/manager.
type Host struct {
    stderr  io.Writer
    spawner spawner
    mu      sync.Mutex
    running []func() // kill funcs for running plugins (empty after Close)
}

func NewHost(stderr io.Writer) *Host {
    return &Host{stderr: stderr, spawner: execSpawner{}}
}

func NewHostWithSpawner(stderr io.Writer, s spawner) *Host {
    return &Host{stderr: stderr, spawner: s}
}

// Load discovers plugins from env, performs handshakes, and returns the
// loaded set. Non-fatal failures go in loadErrs with Fatal=false; fatal
// failures (missing file, not-executable, duplicate name) get Fatal=true.
// The returned plain `error` is only for programmer errors.
func (h *Host) Load(ctx context.Context, pluginsEnv string) ([]Plugin, []LoadError, error) {
    if h.stderr == nil {
        return nil, nil, errors.New("plugin: stderr is nil")
    }
    candidates, loadErrs := discover(pluginsEnv)

    var loaded []Plugin
    for _, path := range candidates {
        p, lerrs := h.handshakeOne(ctx, path)
        loadErrs = append(loadErrs, lerrs...)
        if p != nil {
            loaded = append(loaded, *p)
        }
    }

    // Duplicate name detection (post-handshake).
    seen := make(map[string]string)
    var deduped []Plugin
    for _, p := range loaded {
        if prev, ok := seen[p.Name]; ok {
            loadErrs = append(loadErrs, LoadError{Path: p.Path,
                Message: fmt.Sprintf("duplicate plugin name %s (also at %s)", p.Name, prev),
                Fatal:   true})
            continue
        }
        seen[p.Name] = p.Path
        deduped = append(deduped, p)
    }
    return deduped, loadErrs, nil
}

func (h *Host) handshakeOne(ctx context.Context, path string) (*Plugin, []LoadError) {
    spawnCtx, cancel := context.WithTimeout(ctx, handshakeTimeout)
    defer cancel()

    stdin, stdout, kill, err := h.spawner.Spawn(spawnCtx, path)
    if err != nil {
        return nil, []LoadError{{Path: path,
            Message: fmt.Sprintf("plugin %s spawn failed: %v", path, err), Fatal: false}}
    }
    defer kill()
    defer stdin.Close()

    if err := writeRequest(stdin, rpcRequest{ID: 1, Method: "curlew/hello",
        Params: []byte(`{}`)}); err != nil {
        return nil, []LoadError{{Path: path,
            Message: fmt.Sprintf("plugin %s: write handshake: %v", path, err), Fatal: false}}
    }

    // Read the response on a goroutine to compose with the deadline.
    type readResult struct {
        resp rpcResponse
        err  error
    }
    ch := make(chan readResult, 1)
    br := bufio.NewReader(stdout)
    go func() { r, e := readResponse(br); ch <- readResult{r, e} }()

    select {
    case <-spawnCtx.Done():
        if errors.Is(spawnCtx.Err(), context.DeadlineExceeded) {
            return nil, []LoadError{{Path: path,
                Message: fmt.Sprintf("plugin %s handshake timeout", path), Fatal: false}}
        }
        return nil, []LoadError{{Path: path,
            Message: fmt.Sprintf("plugin %s canceled", path), Fatal: false}}
    case rr := <-ch:
        if rr.err != nil {
            return nil, []LoadError{{Path: path,
                Message: fmt.Sprintf("plugin %s: invalid handshake response: %v", path, rr.err), Fatal: false}}
        }
        if rr.resp.Error != nil {
            return nil, []LoadError{{Path: path,
                Message: fmt.Sprintf("plugin %s: handshake error: %s", path, rr.resp.Error.Message), Fatal: false}}
        }
        hello, err := parseHello(rr.resp.Result)
        if err != nil {
            return nil, []LoadError{{Path: path,
                Message: fmt.Sprintf("plugin %s: %v", path, err), Fatal: false}}
        }
        // Protocol version warning.
        var warns []LoadError
        if hello.ProtocolVersion != 1 {
            warns = append(warns, LoadError{Path: path,
                Message: fmt.Sprintf("plugin %s: unsupported protocol_version %d, using 1",
                    hello.Name, hello.ProtocolVersion), Fatal: false})
        }
        // Hook filtering.
        filtered := make([]string, 0, len(hello.Hooks))
        for _, hk := range hello.Hooks {
            if _, ok := knownHooks[hk]; !ok {
                warns = append(warns, LoadError{Path: path,
                    Message: fmt.Sprintf("plugin %s: unknown hook %s ignored", hello.Name, hk),
                    Fatal:   false})
                continue
            }
            filtered = append(filtered, hk)
        }
        return &Plugin{Name: hello.Name, Version: hello.Version, Hooks: filtered, Path: path}, warns
    }
}

// Close signals all running plugin processes to terminate. M5-017 spawns them
// per handshake and kills on return, so this is a no-op today; M5-018 uses it.
func (h *Host) Close() error {
    h.mu.Lock()
    defer h.mu.Unlock()
    for _, k := range h.running {
        k()
    }
    h.running = nil
    return nil
}
```

```go
// internal/plugin/signal_unix.go (build-tag: !windows)
package plugin

import (
    "os"
    "syscall"
)

func interruptSignal() os.Signal { return syscall.SIGTERM }
```

```go
// internal/plugin/signal_windows.go (build-tag: windows)
package plugin

import "os"

func interruptSignal() os.Signal { return os.Interrupt }
```

```go
// internal/plugin/fakeplugin_test.go
package plugin

import (
    "bufio"
    "context"
    "encoding/json"
    "io"
    "sync"
)

// fakePlugin is a scripted in-process "plugin" used by tests. The handler
// receives a decoded request and returns a response to emit (or empty to
// block forever).
type fakePlugin struct {
    handler func(rpcRequest) rpcResponse
    stall   bool // if true, never respond (drives timeout tests)
}

type fakeSpawner struct {
    plugins map[string]*fakePlugin
    mu      sync.Mutex
    started []func()
}

func (s *fakeSpawner) Spawn(ctx context.Context, path string) (io.WriteCloser, io.ReadCloser, func(), error) {
    p, ok := s.plugins[path]
    if !ok {
        p = &fakePlugin{handler: echoHello}
    }
    inR, inW := io.Pipe()
    outR, outW := io.Pipe()
    done := make(chan struct{})
    go func() {
        defer close(done)
        defer outW.Close()
        if p.stall {
            <-ctx.Done()
            return
        }
        br := bufio.NewReader(inR)
        req, err := readRequestForTest(br)
        if err != nil { return }
        resp := p.handler(req)
        resp.JSONRPC = "2.0"
        resp.ID = req.ID
        b, _ := json.Marshal(resp)
        _, _ = outW.Write(append(b, '\n'))
    }()
    kill := func() { _ = inR.Close(); _ = outR.Close(); <-done }
    s.mu.Lock()
    s.started = append(s.started, kill)
    s.mu.Unlock()
    return inW, outR, kill, nil
}

func readRequestForTest(r *bufio.Reader) (rpcRequest, error) {
    line, err := r.ReadBytes('\n')
    if err != nil { return rpcRequest{}, err }
    var req rpcRequest
    if err := json.Unmarshal(line, &req); err != nil { return req, err }
    return req, nil
}

func echoHello(req rpcRequest) rpcResponse {
    return rpcResponse{Result: json.RawMessage(
        `{"name":"hello-plugin","version":"0.1.0","hooks":["on_request","on_response"],"protocol_version":1}`)}
}
```

#### Tests to Write FIRST (RED phase)

Table-driven, covers all handshake paths:

```go
// internal/plugin/host_test.go
func TestHost_Load(t *testing.T) {
    type tc struct {
        name          string
        env           func(tmp string) (env string, sp *fakeSpawner)
        wantLoaded    []string // names
        wantHooks     map[string][]string
        wantErrsLike  []string // substrings in LoadError.Message, order-preserving
        wantFatalIdx  []int    // indices of wantErrsLike that are Fatal
    }
    // Tests:
    //   "single plugin happy path"
    //   "zero plugins when env is empty"
    //   "directory with two plugins"
    //   "handshake timeout" (stall=true) — non-fatal, message contains "handshake timeout"
    //   "unknown hook ignored" — non-fatal warning; returned Hooks filtered
    //   "duplicate name" — two plugin binaries both report name=hello — fatal
    //   "protocol error: missing name" — non-fatal
    //   "jsonrpc error response" — non-fatal; message contains "handshake error"
    //   "protocol_version=2 warning" — non-fatal; plugin still loaded
    //   "stderr nil returns error"
    //   "missing path is fatal"
    //   "not executable is fatal"
}

func TestHost_Close_NoOpAfterLoad(t *testing.T) { /* ... */ }
```

#### Impact on Existing Tests
None (new package).

---

### Step 4: CLI wiring — `curlew plugins list`

**Rationale:** Single-entry integration with `main.go`. Depends on Step 3. Introduces the rendering + exit-code logic.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/plugins.go` | create | `pluginsCmd`, `pluginsListCmd`, `printPluginsHelp`, `renderPluginsTable`, `pluginsHostFactory` |
| `cmd/curlew/plugins_test.go` | create | End-to-end `run()` tests with in-process spawner |
| `cmd/curlew/main.go` | modify | Add `case "plugins"` in `run()`; add `plugins` to `printHelp()` |

#### Current Code (main.go)

```go
case "perf":
    return perfCmd(args[1:])
default:
    _, _ = fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
    printHelp()
    return 1
}
```

#### New Code (main.go dispatcher delta)

```go
case "perf":
    return perfCmd(args[1:])
case "plugins":
    return pluginsCmd(args[1:])
default:
    ...
```

#### New Code (plugins.go)

```go
package main

import (
    "context"
    "fmt"
    "os"
    "strings"

    "github.com/weiqigod/curlew/internal/plugin"
)

// pluginsHostFactory is a test seam: tests replace it with an in-process
// spawner-backed host.
var pluginsHostFactory = func() *plugin.Host { return plugin.NewHost(os.Stderr) }

// pluginsCmd dispatches `curlew plugins <subcommand>`.
func pluginsCmd(args []string) int {
    if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
        printPluginsHelp()
        return 0
    }
    switch args[0] {
    case "list":
        return pluginsListCmd(args[1:])
    default:
        _, _ = fmt.Fprintf(os.Stderr, "Unknown plugins subcommand: %s\n", args[0])
        printPluginsHelp()
        return 1
    }
}

// pluginsListCmd loads all plugins from CURLEW_PLUGINS and prints a table.
// Exit codes: 0 ok (warnings still produce 0); 2 any fatal load error.
func pluginsListCmd(args []string) int {
    for _, a := range args {
        if a == "--help" || a == "-h" {
            printPluginsHelp()
            return 0
        }
        _, _ = fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", a)
        return 2
    }
    env := os.Getenv("CURLEW_PLUGINS")
    host := pluginsHostFactory()
    defer func() { _ = host.Close() }()

    loaded, loadErrs, err := host.Load(context.Background(), env)
    if err != nil {
        _, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
        return 1
    }

    // Emit stderr messages in order; track exit code.
    exit := 0
    for _, le := range loadErrs {
        prefix := "warning: "
        if le.Fatal {
            prefix = "error: "
            if exit < 2 {
                exit = 2
            }
        }
        _, _ = fmt.Fprintln(os.Stderr, prefix+le.Message)
    }

    renderPluginsTable(os.Stdout, loaded)
    return exit
}

// renderPluginsTable prints the fixed-width table. Columns: NAME VERSION HOOKS.
// Always prints the header even with zero rows.
func renderPluginsTable(w *os.File, plugins []plugin.Plugin) {
    const minGap = "  "
    nameW, verW := len("NAME"), len("VERSION")
    for _, p := range plugins {
        if len(p.Name) > nameW {
            nameW = len(p.Name)
        }
        if len(p.Version) > verW {
            verW = len(p.Version)
        }
    }
    _, _ = fmt.Fprintf(w, "%-*s%s%-*s%s%s\n",
        nameW, "NAME", minGap, verW, "VERSION", minGap, "HOOKS")
    for _, p := range plugins {
        hooks := strings.Join(p.Hooks, ",")
        _, _ = fmt.Fprintf(w, "%-*s%s%-*s%s%s\n",
            nameW, p.Name, minGap, verW, p.Version, minGap, hooks)
    }
    if len(plugins) == 0 {
        // Header only. No-op: users can distinguish empty by absence of rows.
    }
}

func printPluginsHelp() {
    fmt.Println("Usage: curlew plugins <subcommand>")
    fmt.Println()
    fmt.Println("Subcommands:")
    fmt.Println("  list    Discover plugins, handshake, and print the registered capabilities")
    fmt.Println()
    fmt.Println("Environment:")
    fmt.Println("  CURLEW_PLUGINS   Colon-separated (";"-separated on Windows) list of plugin")
    fmt.Println("                    executables or directories. Directory entries load every")
    fmt.Println("                    executable file inside (non-recursive, alphabetical).")
    fmt.Println()
    fmt.Println("Exit codes:")
    fmt.Println("  0    ok (warnings printed to stderr are non-fatal)")
    fmt.Println("  2    fatal load error (missing file, not executable, duplicate name)")
    fmt.Println()
    fmt.Println("See docs/plugins.md for the handshake protocol.")
}
```

#### New Code (main.go `printHelp` additions)

Insert under the existing `Commands:` list, near the other advanced commands:

```go
fmt.Println("  plugins         Manage external-process plugins")
fmt.Println("  plugins list    Discover plugins and print registered capabilities")
```

And add a new section after `Perf Options`:

```go
fmt.Println()
fmt.Println("Plugins:")
fmt.Println("  curlew plugins list    Discover plugins from CURLEW_PLUGINS and show their")
fmt.Println("                          name, version, and registered hooks.")
fmt.Println("  Env vars:  CURLEW_PLUGINS   Colon-separated list of plugin executables or")
fmt.Println("                               directories (see docs/plugins.md)")
```

#### Tests to Write FIRST (RED phase)

```go
// cmd/curlew/plugins_test.go
func TestPluginsList_HappyPath(t *testing.T) {
    // Install a fake factory that returns a host wired to a spawner returning
    // the canonical hello-plugin handshake.
    prev := pluginsHostFactory
    defer func() { pluginsHostFactory = prev }()
    pluginsHostFactory = func() *plugin.Host { /* ... */ }

    t.Setenv("CURLEW_PLUGINS", "/fake/hello")
    // Capture stdout + stderr via os.Pipe, run, assert output contains
    // "NAME         VERSION   HOOKS" and "hello-plugin 0.1.0     on_request,on_response".
}

func TestPluginsList_Scenarios(t *testing.T) {
    // table: empty env → zero plugins; unknown hook warns; duplicate name exits 2;
    // timeout warns but exit 0; missing path exits 2; not executable exits 2.
}

func TestPluginsList_Help(t *testing.T) { /* asserts "Subcommands" block printed */ }
```

Asserting stdout/stderr cleanly: use the existing `captureStdout(t, func())` helper if one exists, else introduce a local one. Check via `Grep` during execution.

#### Impact on Existing Tests

- `TestRun_NoArgs` / `TestRun_Help` in `main_test.go` — expected output contains the command list. If those tests assert exact equality of help text, they will break. **Mitigation**: use `Grep` to find them in Step 4 and update expected substrings to include the new line, or assert via `strings.Contains` (the prevailing pattern in this repo).

---

### Step 5: Sample plugin fixture (`testdata/plugins/hello-plugin`)

**Rationale:** Required by the observable. Small (~35 LOC). Drives one integration test + the smoke script. Depends only on the plugin wire format established in Steps 1-3.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `testdata/plugins/hello-plugin/main.go` | create | Minimal Go `package main` that handshakes |
| `internal/plugin/integration_test.go` | create | One test that `go build`s the fixture and drives the real spawner |

#### New Code

```go
// testdata/plugins/hello-plugin/main.go
// Package main is the sample plugin fixture for M5-017. It reads one line of
// JSON-RPC 2.0 request on stdin and writes one line of JSON-RPC response on
// stdout, declaring the hooks on_request and on_response.
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
            Name:            "hello-plugin",
            Version:         "0.1.0",
            Hooks:           []string{"on_request", "on_response"},
            ProtocolVersion: 1,
        },
    }
    out, _ := json.Marshal(resp)
    _, _ = os.Stdout.Write(append(out, '\n'))
}
```

```go
// internal/plugin/integration_test.go
//go:build !short

package plugin

func TestHost_Load_WithRealFixture(t *testing.T) {
    if testing.Short() { t.Skip("skip in -short") }
    if _, err := exec.LookPath("go"); err != nil { t.Skip("go toolchain not on PATH") }

    tmp := t.TempDir()
    binPath := filepath.Join(tmp, "hello-plugin")
    cmd := exec.Command("go", "build", "-o", binPath,
        "../../testdata/plugins/hello-plugin")
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil { t.Fatalf("go build: %v", err) }

    host := NewHost(io.Discard)
    loaded, errs, err := host.Load(context.Background(), binPath)
    if err != nil { t.Fatal(err) }
    if len(loaded) != 1 || loaded[0].Name != "hello-plugin" ||
       loaded[0].Version != "0.1.0" ||
       !reflect.DeepEqual(loaded[0].Hooks, []string{"on_request", "on_response"}) {
        t.Errorf("unexpected plugin: %+v, errs=%+v", loaded, errs)
    }
}
```

#### Impact on Existing Tests

None. The fixture directory is ignored by `go test ./...` because Go's test runner does not recurse into `testdata/`.

---

### Step 6: Documentation — `docs/plugins.md`

**Rationale:** DoD item. Required by behavior 8. No code depends on it.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `docs/plugins.md` | create | Wire format, env-var, exit codes, minimum example, security note |

#### Outline

```
# Curlew Plugins

## Overview
Curlew loads plugins as external processes and talks to them over JSON-RPC
2.0 on stdin/stdout. Plugins can be written in any language.

## Discovery: `CURLEW_PLUGINS`
(colon-separated list; directory expansion; executable bit; examples)

## Handshake
On load, curlew sends one line:
  {"jsonrpc":"2.0","id":1,"method":"curlew/hello","params":{}}
The plugin replies with one line:
  {"jsonrpc":"2.0","id":1,"result":{"name":"...","version":"...",
   "hooks":["on_request"],"protocol_version":1}}
Timeout: 5 seconds.

## Supported hooks (M5-017)
`on_request`, `on_response` — declared here, invoked in M5-018+.

## Exit codes
0 ok; 2 fatal load error; timeouts are non-fatal (warning printed).

## Minimum Go example
(~25 lines, identical to testdata/plugins/hello-plugin/main.go)

## Security
Plugins run with the invoking user's privileges. Only add trusted binaries
to CURLEW_PLUGINS.
```

#### Impact on Existing Tests

None.

---

### Step 7: Smoke test + CHANGELOG

**Rationale:** DoD items. Depends on all prior steps.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Append `=== Plugins (M5-017) ===` section |
| `CHANGELOG.md` | modify | Add Unreleased → Added bullet for M5-017 |

#### New Code (smoke/run.sh)

Append after the license export block:

```bash
echo "=== Plugins (M5-017) ==="

PLUGIN_DIR=$(mktemp -d /tmp/curlew_plugins_XXXXXX)
go build -o "$PLUGIN_DIR/hello-plugin" ./testdata/plugins/hello-plugin

echo "--- Single plugin ---"
OUT=$(CURLEW_PLUGINS="$PLUGIN_DIR/hello-plugin" ./curlew plugins list)
echo "$OUT" | grep -q "hello-plugin 0.1.0" \
  && echo "PASS: single plugin listed" || { echo "FAIL: single plugin — $OUT"; exit 1; }

echo "--- Directory of plugins ---"
OUT=$(CURLEW_PLUGINS="$PLUGIN_DIR" ./curlew plugins list)
echo "$OUT" | grep -q "hello-plugin 0.1.0" \
  && echo "PASS: directory expanded" || { echo "FAIL: directory — $OUT"; exit 1; }

echo "--- Missing path exits 2 ---"
CURLEW_PLUGINS="/does/not/exist" ./curlew plugins list \
  && { echo "FAIL: should have exited 2"; exit 1; } \
  || echo "PASS: exit $?"

rm -rf "$PLUGIN_DIR"
echo
```

#### CHANGELOG delta

Under `## [Unreleased]` → `### Added`, prepend:

```md
- CLI: `curlew plugins list` subcommand and `internal/plugin` external-process plugin loader (M5-017): `CURLEW_PLUGINS` env var (colon-separated files or directories) discovers plugin executables; JSON-RPC 2.0 `curlew/hello` handshake over stdin/stdout with 5s timeout negotiates `{name, version, hooks, protocol_version}`; known hooks `on_request`, `on_response` are kept; unknown hooks print a warning and are filtered; duplicate names and missing/non-executable binaries exit 2; timeouts are non-fatal (exit 0, warning to stderr); `testdata/plugins/hello-plugin` sample fixture; `docs/plugins.md` documents the handshake schema, exit codes, and minimum Go example; test coverage ≥80%; one integration test builds the fixture under `go test -short=false` (M5-017)
```

#### Impact on Existing Tests

- The smoke script now calls `go build ./testdata/plugins/hello-plugin` before the plugin tests; if Go is missing from PATH the smoke test fails. This is already the case for `go build ./cmd/curlew` earlier in the script, so no new dependency.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|-----------------|
| `cmd/curlew/main_test.go` | `TestRun_Help` / `TestPrintHelp` | may break | check exact-match vs. substring; update expected substring if needed |
| `internal/plugin/*_test.go` | — | new | all new tests — write first |
| `cmd/curlew/plugins_test.go` | — | new | new end-to-end tests via `pluginsHostFactory` seam |
| `internal/plugin/integration_test.go` | `TestHost_Load_WithRealFixture` | new | gated on `testing.Short()` and `go` on PATH |

Count: **15+ unit tests** in `internal/plugin` + **6 CLI-level tests** + **1 integration test** = **22+ tests** total. Coverage target ≥80% is well within reach (the package is I/O-pure except for the `execSpawner`, which is exercised by the integration test).

## Risks and Edge Cases

- **Risk:** Windows CI. The repo does not appear to enforce Windows runs (smoke is bash). `isExecutable` uses Unix mode bits, which on Windows `os.Stat` will always return 0 for the x-bits. **Mitigation:** on Windows, treat any regular file as executable (the OS layer decides at `exec.Command` time). Guard `isExecutable` with `runtime.GOOS == "windows"` returning true unconditionally for regular files.
- **Risk:** Pipe deadlock when plugin writes to stderr and doesn't drain stdin. **Mitigation:** use `exec.CommandContext`, which sets up unbuffered pipes; host closes stdin after writing the handshake so the plugin's read loop completes even if we don't drain stderr. Do not attach `cmd.Stderr`; let the plugin's stderr go to the parent's stderr (Go default when `cmd.Stderr` is nil is to discard; we accept that — plugins can log to a file).
- **Risk:** Plugin writes multiple lines before we read one. **Mitigation:** `bufio.Reader.ReadBytes('\n')` reads one frame at a time; extra lines are left in the buffer. Since we kill the plugin after handshake in M5-017, no data is lost that matters.
- **Edge case:** Plugin exits cleanly before responding. **Handling:** `readResponse` sees EOF; we wrap with `ErrHandshakeProtocol`. Loader prints `plugin <path>: invalid handshake response: EOF`, non-fatal.
- **Edge case:** Plugin writes an extremely long line (multi-MB). **Handling:** `bufio.Reader` default 4KB buffer is insufficient. Use `bufio.NewReaderSize(stdout, 1<<20)` (1 MiB). Document the 1 MiB limit in `docs/plugins.md`.
- **Edge case:** Plugin binary has the exec bit but is not a valid executable (e.g., a shell script without a shebang). **Handling:** `exec.Command.Start()` will fail; surfaces as "spawn failed" non-fatal error. Already covered.
- **Edge case:** Plugin name contains tabs or newlines. **Handling:** Accept it as a warning: `warning: plugin <path>: name contains control characters, using <name-redacted>` — but this is gold-plating for M5-017. Instead, leave the raw string in the table and let the terminal handle it; document that names must be ASCII-printable in `docs/plugins.md`.
- **Edge case:** `CURLEW_PLUGINS` with a single empty-string element (e.g., `CURLEW_PLUGINS=` or `CURLEW_PLUGINS=foo::bar`). **Handling:** `strings.Split` on `":"` yields empty strings for leading/trailing/consecutive separators; the loop skips empty entries (decision 4).
- **Risk:** lint rules for `internal/plugin`. `golangci-lint` v2 with `staticcheck` may flag the channel pattern in `handshakeOne`. **Mitigation:** keep the select-on-context pattern — it's idiomatic. If `bodyclose` or `errcheck` complain about `stdin.Close()`, use `//nolint:errcheck` with explanation.
- **Risk:** `testdata/plugins/hello-plugin/main.go` is discovered by `go vet ./...`. **Mitigation:** Go's `./...` pattern explicitly excludes `testdata/`. Verified by Go docs.

## Verification

```bash
go build ./cmd/curlew
go test ./internal/plugin/...
go test ./cmd/curlew/...
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
./scripts/ci-local.sh --go
```

Coverage check:

```bash
go test -coverprofile=coverage.out ./internal/plugin/...
~/go/bin/go tool cover -func=coverage.out | tail -1
# expect: total >= 80%
```

Observable verification (from task YAML):

```bash
go build ./cmd/curlew
go test ./internal/plugin/...
# Expected: ok  internal/plugin  (>=10 tests passing)
go build -o /tmp/curlew-hello-plugin ./testdata/plugins/hello-plugin
CURLEW_PLUGINS=/tmp/curlew-hello-plugin ./curlew plugins list
# Expected stdout:
#   NAME         VERSION   HOOKS
#   hello-plugin 0.1.0     on_request,on_response
# exit 0
```

## Open Questions (resolved in Decisions section)

1. External-process vs. Go plugin vs. WASM → **external-process + JSON-RPC 2.0** (decision 1).
2. Single package vs. sub-packages → **single `internal/plugin` for M5-017** (decision 9).
3. Tier gate on `plugins list` → **no gate in M5-017** (decision 16).
4. `--format json` → **not in M5-017** (decision 8).
5. Sandboxing → **documented, not enforced** (decision 17).

All other ambiguities documented inline with decisions and rationale.
