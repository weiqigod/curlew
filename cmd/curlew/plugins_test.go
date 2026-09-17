package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/plugin"
)

// testSpawner is an in-process fake spawner for CLI-level tests.
type testSpawner struct {
	plugins map[string]*testPluginDef
}

type testPluginDef struct {
	handler func(req testRPCRequest) testRPCResponse
	stall   bool
}

type testRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type testRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *testRPCError   `json:"error,omitempty"`
}

type testRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *testSpawner) Spawn(ctx context.Context, path string) (io.WriteCloser, io.ReadCloser, func(), error) {
	def, ok := s.plugins[path]
	if !ok {
		def = &testPluginDef{handler: defaultHelloHandler}
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = outW.Close() }()
		if def.stall {
			<-ctx.Done()
			return
		}
		buf := make([]byte, 4096)
		n, err := readLine(inR, buf)
		if err != nil {
			return
		}
		var req testRPCRequest
		if err := json.Unmarshal(buf[:n], &req); err != nil {
			return
		}
		resp := def.handler(req)
		resp.JSONRPC = "2.0"
		resp.ID = req.ID
		b, _ := json.Marshal(resp)
		_, _ = outW.Write(append(b, '\n'))
	}()
	kill := func() {
		_ = inR.Close()
		_ = outR.Close()
		<-done
	}
	return inW, outR, kill, nil
}

func readLine(r io.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total : total+1])
		total += n
		if err != nil {
			return total, err
		}
		if buf[total-1] == '\n' {
			return total, nil
		}
	}
	return total, nil
}

func defaultHelloHandler(_ testRPCRequest) testRPCResponse {
	return testRPCResponse{
		Result: json.RawMessage(
			`{"name":"hello-plugin","version":"0.1.0","hooks":["on_request","on_response"],"protocol_version":1}`,
		),
	}
}

func TestPluginsList_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	pluginPath := testExecutablePath(tmp, "hello")
	if err := os.WriteFile(pluginPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sp := &testSpawner{plugins: map[string]*testPluginDef{
		pluginPath: {handler: defaultHelloHandler},
	}}

	prev := pluginsHostFactory
	defer func() { pluginsHostFactory = prev }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, sp)
	}
	t.Setenv("CURLEW_PLUGINS", pluginPath)

	var code int
	stdout, stderr := capturePluginsOutput(t, func(out, errOut io.Writer) int {
		code = runWithWriters([]string{"plugins", "list"}, out, errOut)
		return code
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d (stderr: %s)", code, stderr)
	}

	if !strings.Contains(stdout, "NAME") {
		t.Errorf("expected header, got: %s", stdout)
	}
	if !strings.Contains(stdout, "hello-plugin") {
		t.Errorf("expected hello-plugin in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "0.1.0") {
		t.Errorf("expected version, got: %s", stdout)
	}
	if !strings.Contains(stdout, "on_request,on_response") {
		t.Errorf("expected hooks, got: %s", stdout)
	}
}

func TestPluginsList_EmptyEnv(t *testing.T) {
	prev := pluginsHostFactory
	defer func() { pluginsHostFactory = prev }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, &testSpawner{})
	}
	t.Setenv("CURLEW_PLUGINS", "")

	var code int
	stdout, _ := capturePluginsOutput(t, func(out, errOut io.Writer) int {
		code = runWithWriters([]string{"plugins", "list"}, out, errOut)
		return code
	})
	if code != 0 {
		t.Errorf("expected exit 0 for empty env, got %d", code)
	}
	// Header still printed.
	if !strings.Contains(stdout, "NAME") {
		t.Errorf("expected header even with zero plugins, got: %s", stdout)
	}
}

func TestPluginsList_UnknownHookWarning(t *testing.T) {
	tmp := t.TempDir()
	pluginPath := testExecutablePath(tmp, "plug")
	if err := os.WriteFile(pluginPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sp := &testSpawner{plugins: map[string]*testPluginDef{
		pluginPath: {handler: func(_ testRPCRequest) testRPCResponse {
			return testRPCResponse{
				Result: json.RawMessage(`{"name":"myplug","version":"1.0","hooks":["on_request","on_magic"],"protocol_version":1}`),
			}
		}},
	}}
	prev := pluginsHostFactory
	defer func() { pluginsHostFactory = prev }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, sp)
	}
	t.Setenv("CURLEW_PLUGINS", pluginPath)

	var code int
	capturePluginsOutput(t, func(out, errOut io.Writer) int {
		code = runWithWriters([]string{"plugins", "list"}, out, errOut)
		return code
	})
	// Unknown hooks produce a warning (non-fatal); exit 0.
	if code != 0 {
		t.Errorf("expected exit 0 for unknown hook warning, got %d", code)
	}
}

func TestPluginsList_DuplicateNameExits2(t *testing.T) {
	tmp := t.TempDir()
	pathA := filepath.Join(tmp, "a")
	pathB := filepath.Join(tmp, "b")
	for _, p := range []string{pathA, pathB} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	handler := func(_ testRPCRequest) testRPCResponse {
		return testRPCResponse{
			Result: json.RawMessage(`{"name":"same","version":"1.0","hooks":[],"protocol_version":1}`),
		}
	}
	sp := &testSpawner{plugins: map[string]*testPluginDef{
		pathA: {handler: handler},
		pathB: {handler: handler},
	}}
	prev := pluginsHostFactory
	defer func() { pluginsHostFactory = prev }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, sp)
	}
	env := pathA + string(filepath.ListSeparator) + pathB
	t.Setenv("CURLEW_PLUGINS", env)

	var code int
	capturePluginsOutput(t, func(out, errOut io.Writer) int {
		code = runWithWriters([]string{"plugins", "list"}, out, errOut)
		return code
	})
	if code != 2 {
		t.Errorf("expected exit 2 for duplicate name, got %d", code)
	}
}

func TestPluginsList_MissingPathExits2(t *testing.T) {
	prev := pluginsHostFactory
	defer func() { pluginsHostFactory = prev }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, &testSpawner{})
	}
	t.Setenv("CURLEW_PLUGINS", "/does/not/exist/plugin")

	var code int
	capturePluginsOutput(t, func(out, errOut io.Writer) int {
		code = runWithWriters([]string{"plugins", "list"}, out, errOut)
		return code
	})
	if code != 2 {
		t.Errorf("expected exit 2 for missing path, got %d", code)
	}
}

func TestPluginsList_Help(t *testing.T) {
	stdout, _ := capturePluginsOutput(t, func(out, errOut io.Writer) int {
		return runWithWriters([]string{"plugins", "--help"}, out, errOut)
	})
	if !strings.Contains(stdout, "Subcommands") {
		t.Errorf("expected help output to contain 'Subcommands', got: %s", stdout)
	}
	if !strings.Contains(stdout, "CURLEW_PLUGINS") {
		t.Errorf("expected help output to mention CURLEW_PLUGINS, got: %s", stdout)
	}
}

func TestPluginsCmd_UnknownSubcmd(t *testing.T) {
	var code int
	capturePluginsOutput(t, func(out, errOut io.Writer) int {
		code = runWithWriters([]string{"plugins", "unknown"}, out, errOut)
		return code
	})
	if code != 1 {
		t.Errorf("expected exit 1 for unknown subcommand, got %d", code)
	}
}

func TestPluginsList_HelpArg(t *testing.T) {
	stdout, _ := capturePluginsOutput(t, func(out, errOut io.Writer) int {
		return runWithWriters([]string{"plugins", "list", "--help"}, out, errOut)
	})
	if !strings.Contains(stdout, "Subcommands") {
		t.Errorf("expected help output to contain 'Subcommands', got: %s", stdout)
	}
}

func TestPluginsList_UnknownFlag(t *testing.T) {
	var code int
	_, stderr := capturePluginsOutput(t, func(out, errOut io.Writer) int {
		code = runWithWriters([]string{"plugins", "list", "--unknown-flag"}, out, errOut)
		return code
	})
	if code != 2 {
		t.Errorf("expected exit 2 for unknown flag, got %d", code)
	}
	if !strings.Contains(stderr, "Unknown flag") {
		t.Errorf("expected 'Unknown flag' in stderr, got: %s", stderr)
	}
}

// TestPluginsList_TimeoutWarning verifies that a plugin handshake timeout
// produces a non-fatal warning on stderr and exits 0 (behavior 2).
func TestPluginsList_TimeoutWarning(t *testing.T) {
	tmp := t.TempDir()
	pluginPath := testExecutablePath(tmp, "stall")
	if err := os.WriteFile(pluginPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sp := &testSpawner{plugins: map[string]*testPluginDef{
		pluginPath: {stall: true},
	}}

	prevHost := pluginsHostFactory
	defer func() { pluginsHostFactory = prevHost }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, sp)
	}

	// Inject a pre-cancelled context so the handshake times out immediately
	// without waiting the full 5-second timeout constant.
	prevCtx := pluginsCtxFactory
	defer func() { pluginsCtxFactory = prevCtx }()
	pluginsCtxFactory = func() context.Context {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		_ = cancel // cancel will be called when context times out; leak is benign in test
		return ctx
	}

	t.Setenv("CURLEW_PLUGINS", pluginPath)

	var code int
	_, stderr := capturePluginsOutput(t, func(out, errOut io.Writer) int {
		code = runWithWriters([]string{"plugins", "list"}, out, errOut)
		return code
	})
	if code != 0 {
		t.Errorf("expected exit 0 for timeout (non-fatal), got %d", code)
	}
	if !strings.Contains(stderr, "warning:") {
		t.Errorf("expected warning message in stderr, got: %s", stderr)
	}
}

// TestPluginsList_Directory verifies that CURLEW_PLUGINS pointing to a
// directory expands to all executable files inside, showing both in the table
// (behavior 3).
func TestPluginsList_Directory(t *testing.T) {
	tmp := t.TempDir()
	pathA := testExecutablePath(tmp, "aplug")
	pathB := testExecutablePath(tmp, "zplug")
	for _, p := range []string{pathA, pathB} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	handlerA := func(_ testRPCRequest) testRPCResponse {
		return testRPCResponse{
			Result: json.RawMessage(`{"name":"aplug","version":"1.0","hooks":["on_request"],"protocol_version":1}`),
		}
	}
	handlerB := func(_ testRPCRequest) testRPCResponse {
		return testRPCResponse{
			Result: json.RawMessage(`{"name":"zplug","version":"2.0","hooks":["on_response"],"protocol_version":1}`),
		}
	}
	sp := &testSpawner{plugins: map[string]*testPluginDef{
		pathA: {handler: handlerA},
		pathB: {handler: handlerB},
	}}

	prevHost := pluginsHostFactory
	defer func() { pluginsHostFactory = prevHost }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, sp)
	}

	// Point CURLEW_PLUGINS at the directory.
	t.Setenv("CURLEW_PLUGINS", tmp)

	var code int
	stdout, stderr := capturePluginsOutput(t, func(out, errOut io.Writer) int {
		code = runWithWriters([]string{"plugins", "list"}, out, errOut)
		return code
	})
	if code != 0 {
		t.Errorf("expected exit 0 for directory listing, got %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "aplug") {
		t.Errorf("expected aplug in table output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "zplug") {
		t.Errorf("expected zplug in table output, got: %s", stdout)
	}
}

// TestPluginsList_ListsConfiguredPlugin verifies that `plugins list` with
// CURLEW_PLUGINS set lists the configured plugin.
func TestPluginsList_ListsConfiguredPlugin(t *testing.T) {
	tmp := t.TempDir()
	pluginPath := testExecutablePath(tmp, "hello")
	if err := os.WriteFile(pluginPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sp := &testSpawner{plugins: map[string]*testPluginDef{
		pluginPath: {handler: defaultHelloHandler},
	}}
	prev := pluginsHostFactory
	defer func() { pluginsHostFactory = prev }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, sp)
	}

	t.Setenv("CURLEW_PLUGINS", pluginPath)

	var code int
	stdout, _ := capturePluginsOutput(t, func(out, errOut io.Writer) int {
		code = runWithWriters([]string{"plugins", "list"}, out, errOut)
		return code
	})
	if code != 0 {
		t.Errorf("exit=%d, want 0", code)
	}
	if !strings.Contains(stdout, "hello-plugin") {
		t.Errorf("expected hello-plugin in table, got: %s", stdout)
	}
}

// TestPluginsList_EmptyTableWhenEnvUnset verifies that `plugins list` with
// CURLEW_PLUGINS unset prints an empty table.
func TestPluginsList_EmptyTableWhenEnvUnset(t *testing.T) {
	prev := pluginsHostFactory
	defer func() { pluginsHostFactory = prev }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, &testSpawner{})
	}
	t.Setenv("CURLEW_PLUGINS", "")

	var code int
	stdout, _ := capturePluginsOutput(t, func(out, errOut io.Writer) int {
		code = runWithWriters([]string{"plugins", "list"}, out, errOut)
		return code
	})
	if code != 0 {
		t.Errorf("with unset env: exit=%d, want 0", code)
	}
	if !strings.Contains(stdout, "NAME") {
		t.Error("with unset env: header missing")
	}
}

// TestRun_Help_IncludesPluginHooksSection verifies that --help mentions the
// three lifecycle hooks.
func TestRun_Help_IncludesPluginHooksSection(t *testing.T) {
	stdout, _, exitCode := captureRun(t, "--help")
	if exitCode != 0 {
		t.Errorf("expected exit 0 for --help, got %d", exitCode)
	}
	for _, want := range []string{"Plugin hooks", "on_request", "on_response", "on_result"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help text missing %q\n---\n%s", want, stdout)
		}
	}
}

// loopTestSpawner is a loop-capable in-process spawner used by hook dispatch
// tests. Unlike testSpawner it reads requests in a loop until EOF, allowing
// multiple hook calls per plugin process (on_request, on_response, on_result).
type loopTestSpawner struct {
	plugins map[string]*loopTestPluginDef
}

type loopTestPluginDef struct {
	// helloResult is the JSON result for curlew/hello.
	helloResult json.RawMessage
	// handler is called for every method after hello.
	// Return (result, nil) for success or (nil, error) for a JSON-RPC error.
	handler func(method string, params json.RawMessage) (json.RawMessage, error)
	// stallMethod stalls forever when a request for this method arrives.
	stallMethod string
}

func (s *loopTestSpawner) Spawn(_ context.Context, path string) (io.WriteCloser, io.ReadCloser, func(), error) {
	def, ok := s.plugins[path]
	if !ok {
		def = &loopTestPluginDef{
			helloResult: json.RawMessage(`{"name":"test-plugin","version":"0.1.0","hooks":["on_request","on_response","on_result"],"protocol_version":1}`),
			handler: func(_ string, _ json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(`{}`), nil
			},
		}
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = outW.Close() }()
		buf := make([]byte, 65536)
		total := 0
		for {
			total = 0
			for {
				n, err := inR.Read(buf[total : total+1])
				total += n
				if err != nil {
					return
				}
				if total > 0 && buf[total-1] == '\n' {
					break
				}
				if total >= len(buf) {
					return
				}
			}
			var req testRPCRequest
			if err := json.Unmarshal(buf[:total], &req); err != nil {
				return
			}
			// Stall requested method until pipe close.
			if def.stallMethod != "" && req.Method == def.stallMethod {
				tmp := make([]byte, 1)
				_, _ = inR.Read(tmp)
				return
			}
			var result json.RawMessage
			var rpcErr *testRPCError
			if req.Method == "curlew/hello" {
				result = def.helloResult
			} else {
				r, err := def.handler(req.Method, req.Params)
				if err != nil {
					rpcErr = &testRPCError{Code: -1, Message: err.Error()}
				} else {
					result = r
				}
			}
			resp := testRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  result,
			}
			if rpcErr != nil {
				resp.Result = nil
				resp.Error = rpcErr
			}
			b, _ := json.Marshal(resp)
			if _, err := outW.Write(append(b, '\n')); err != nil {
				return
			}
		}
	}()
	kill := func() {
		_ = inR.Close()
		_ = outR.Close()
		<-done
	}
	return inW, outR, kill, nil
}

// writeTestCollection writes a minimal YAML collection to dir and returns its path.
func writeTestCollection(t *testing.T, dir, url string) string {
	t.Helper()
	content := `name: Hook Test Collection
requests:
  - name: ping
    request:
      method: GET
      url: ` + url + `
    assertions:
      status: 200
`
	path := filepath.Join(dir, "col.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}
	return path
}

// TestRun_HookPlugin_OnRequestReceivesRequest verifies that a plugin registered
// for on_request receives the method and URL before the HTTP call is made.
func TestRun_HookPlugin_OnRequestReceivesRequest(t *testing.T) {
	var gotMethod, gotURL string
	var callCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	pluginPath := testExecutablePath(tmp, "hookplugin")
	if err := os.WriteFile(pluginPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	sp := &loopTestSpawner{plugins: map[string]*loopTestPluginDef{
		pluginPath: {
			helloResult: json.RawMessage(`{"name":"req-hook","version":"0.1.0","hooks":["on_request","on_response","on_result"],"protocol_version":1}`),
			handler: func(method string, params json.RawMessage) (json.RawMessage, error) {
				if method == "curlew/on_request" {
					callCount++
					var p struct {
						Method string `json:"method"`
						URL    string `json:"url"`
					}
					_ = json.Unmarshal(params, &p)
					gotMethod = p.Method
					gotURL = p.URL
				}
				return json.RawMessage(`{}`), nil
			},
		},
	}}

	prev := pluginsHostFactory
	defer func() { pluginsHostFactory = prev }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, sp)
	}
	t.Setenv("CURLEW_PLUGINS", pluginPath)

	colPath := writeTestCollection(t, tmp, srv.URL+"/ping")
	_, _, exitCode := captureRun(t, "run", colPath)
	if exitCode != 0 {
		t.Errorf("expected exit 0, got %d", exitCode)
	}
	if callCount != 1 {
		t.Errorf("expected on_request called once, got %d", callCount)
	}
	if gotMethod != "GET" {
		t.Errorf("expected method GET, got %q", gotMethod)
	}
	if gotURL != srv.URL+"/ping" {
		t.Errorf("expected URL %s, got %q", srv.URL+"/ping", gotURL)
	}
}

// TestRun_HookPlugin_OnResponseReceivesResponse verifies that a plugin registered
// for on_response receives the status code after the HTTP response arrives.
func TestRun_HookPlugin_OnResponseReceivesResponse(t *testing.T) {
	var gotStatus int
	var callCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(201)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	pluginPath := testExecutablePath(tmp, "respplugin")
	if err := os.WriteFile(pluginPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	sp := &loopTestSpawner{plugins: map[string]*loopTestPluginDef{
		pluginPath: {
			helloResult: json.RawMessage(`{"name":"resp-hook","version":"0.1.0","hooks":["on_response"],"protocol_version":1}`),
			handler: func(method string, params json.RawMessage) (json.RawMessage, error) {
				if method == "curlew/on_response" {
					callCount++
					var p struct {
						StatusCode int `json:"status_code"`
					}
					_ = json.Unmarshal(params, &p)
					gotStatus = p.StatusCode
				}
				return json.RawMessage(`{}`), nil
			},
		},
	}}

	prev := pluginsHostFactory
	defer func() { pluginsHostFactory = prev }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, sp)
	}
	t.Setenv("CURLEW_PLUGINS", pluginPath)

	// Collection expects 201 so the run passes.
	content := `name: Resp Hook Test
requests:
  - name: created
    request:
      method: GET
      url: ` + srv.URL + `/created
    assertions:
      status: 201
`
	colPath := filepath.Join(tmp, "col.yaml")
	if err := os.WriteFile(colPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, exitCode := captureRun(t, "run", colPath)
	if exitCode != 0 {
		t.Errorf("expected exit 0, got %d", exitCode)
	}
	if callCount != 1 {
		t.Errorf("expected on_response called once, got %d", callCount)
	}
	if gotStatus != 201 {
		t.Errorf("expected status 201, got %d", gotStatus)
	}
}

// TestRun_HookPlugin_OnResultReceivesSummary verifies that a plugin registered
// for on_result receives pass/fail counts after the run completes.
func TestRun_HookPlugin_OnResultReceivesSummary(t *testing.T) {
	var gotPass, gotFail int
	var resultCallCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	pluginPath := testExecutablePath(tmp, "resultplugin")
	if err := os.WriteFile(pluginPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	sp := &loopTestSpawner{plugins: map[string]*loopTestPluginDef{
		pluginPath: {
			helloResult: json.RawMessage(`{"name":"result-hook","version":"0.1.0","hooks":["on_result"],"protocol_version":1}`),
			handler: func(method string, params json.RawMessage) (json.RawMessage, error) {
				if method == "curlew/on_result" {
					resultCallCount++
					var p struct {
						PassCount int `json:"pass_count"`
						FailCount int `json:"fail_count"`
					}
					_ = json.Unmarshal(params, &p)
					gotPass = p.PassCount
					gotFail = p.FailCount
				}
				return json.RawMessage(`{}`), nil
			},
		},
	}}

	prev := pluginsHostFactory
	defer func() { pluginsHostFactory = prev }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, sp)
	}
	t.Setenv("CURLEW_PLUGINS", pluginPath)

	colPath := writeTestCollection(t, tmp, srv.URL+"/ok")
	_, _, exitCode := captureRun(t, "run", colPath)
	if exitCode != 0 {
		t.Errorf("expected exit 0, got %d", exitCode)
	}
	if resultCallCount != 1 {
		t.Errorf("expected on_result called once, got %d", resultCallCount)
	}
	if gotPass != 1 {
		t.Errorf("expected pass_count=1, got %d", gotPass)
	}
	if gotFail != 0 {
		t.Errorf("expected fail_count=0, got %d", gotFail)
	}
}

// TestRun_HookPlugin_TimeoutEmitsWarning verifies that a plugin whose hook
// stalls emits a timeout warning on stderr and the run completes successfully.
func TestRun_HookPlugin_TimeoutEmitsWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	pluginPath := testExecutablePath(tmp, "stallplugin")
	if err := os.WriteFile(pluginPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	sp := &loopTestSpawner{plugins: map[string]*loopTestPluginDef{
		pluginPath: {
			helloResult: json.RawMessage(`{"name":"stall-hook","version":"0.1.0","hooks":["on_request"],"protocol_version":1}`),
			stallMethod: "curlew/on_request",
			handler: func(_ string, _ json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(`{}`), nil
			},
		},
	}}

	prev := pluginsHostFactory
	defer func() { pluginsHostFactory = prev }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, sp)
	}
	t.Setenv("CURLEW_PLUGINS", pluginPath)

	// Shorten the hook timeout so the test does not take 10 seconds.
	hookTimeoutOverride = 50 * time.Millisecond
	defer func() { hookTimeoutOverride = 0 }()

	colPath := writeTestCollection(t, tmp, srv.URL+"/ok")
	_, stderr, exitCode := captureRun(t, "run", colPath)
	if exitCode != 0 {
		t.Errorf("expected exit 0 (timeout does not abort run), got %d", exitCode)
	}
	if !strings.Contains(stderr, "timed out") {
		t.Errorf("expected 'timed out' warning in stderr, got: %s", stderr)
	}
}

// TestRun_HookPlugin_TwoPluginsChainInOrder verifies that two plugins both
// declaring on_request are invoked in declaration order, and the second plugin
// sees the request mutated by the first.
func TestRun_HookPlugin_TwoPluginsChainInOrder(t *testing.T) {
	var receivedByFirst, receivedBySecond string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	pathA := testExecutablePath(tmp, "aplug")
	pathB := testExecutablePath(tmp, "bplug")
	for _, p := range []string{pathA, pathB} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	sp := &loopTestSpawner{plugins: map[string]*loopTestPluginDef{
		pathA: {
			helloResult: json.RawMessage(`{"name":"plug-a","version":"0.1.0","hooks":["on_request"],"protocol_version":1}`),
			handler: func(method string, params json.RawMessage) (json.RawMessage, error) {
				if method == "curlew/on_request" {
					var p struct {
						URL     string            `json:"url"`
						Headers map[string]string `json:"headers"`
					}
					_ = json.Unmarshal(params, &p)
					receivedByFirst = p.URL
					// Mutate: add a header so plug-b can see it.
					if p.Headers == nil {
						p.Headers = make(map[string]string)
					}
					p.Headers["X-Chain"] = "from-plug-a"
					out, _ := json.Marshal(map[string]any{
						"method":  "GET",
						"url":     p.URL,
						"headers": p.Headers,
					})
					return out, nil
				}
				return json.RawMessage(`{}`), nil
			},
		},
		pathB: {
			helloResult: json.RawMessage(`{"name":"plug-b","version":"0.1.0","hooks":["on_request"],"protocol_version":1}`),
			handler: func(method string, params json.RawMessage) (json.RawMessage, error) {
				if method == "curlew/on_request" {
					var p struct {
						URL     string            `json:"url"`
						Headers map[string]string `json:"headers"`
					}
					_ = json.Unmarshal(params, &p)
					receivedBySecond = p.Headers["X-Chain"]
				}
				return json.RawMessage(`{}`), nil
			},
		},
	}}

	prev := pluginsHostFactory
	defer func() { pluginsHostFactory = prev }()
	pluginsHostFactory = func() *plugin.Host {
		return plugin.NewHostWithSpawner(os.Stderr, sp)
	}
	// A is declared first so it runs first.
	t.Setenv("CURLEW_PLUGINS", pathA+string(filepath.ListSeparator)+pathB)

	colPath := writeTestCollection(t, tmp, srv.URL+"/chain")
	_, _, exitCode := captureRun(t, "run", colPath)
	if exitCode != 0 {
		t.Errorf("expected exit 0, got %d", exitCode)
	}
	if receivedByFirst == "" {
		t.Error("plug-a on_request was not called")
	}
	if receivedBySecond != "from-plug-a" {
		t.Errorf("expected plug-b to see X-Chain=from-plug-a, got %q", receivedBySecond)
	}
}

// TestRun_PluginsEnvUnset verifies that `curlew run` with CURLEW_PLUGINS
// unset runs normally.
func TestRun_PluginsEnvUnset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	colPath := writeTestCollection(t, tmp, srv.URL+"/ok")

	t.Setenv("CURLEW_PLUGINS", "")

	_, _, code := captureRun(t, "run", colPath)
	if code != 0 {
		t.Errorf("with unset env: exit=%d, want 0", code)
	}
}

// capturePluginsOutput captures stdout and stderr during fn execution.
// fn receives the injected writers; call the relevant Cmd function with them.
func capturePluginsOutput(t *testing.T, fn func(stdout, stderr io.Writer) int) (stdout, stderr string) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	fn(&outBuf, &errBuf)
	return outBuf.String(), errBuf.String()
}
