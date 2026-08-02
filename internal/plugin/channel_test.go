package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// loopSpawner is a spawner that keeps the plugin loop running until EOF.
type loopSpawner struct {
	// handle is called for each request the fake plugin receives.
	handle func(rpcRequest) rpcResponse
}

func (s *loopSpawner) Spawn(_ context.Context, _ string) (io.WriteCloser, io.ReadCloser, func(), error) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = outW.Close() }()
		br := bufio.NewReader(inR)
		for {
			req, err := readRequestForTest(br)
			if err != nil {
				return
			}
			resp := s.handle(req)
			resp.JSONRPC = "2.0"
			resp.ID = req.ID
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

// makeChannel creates a Channel using loopSpawner with the given handler.
func makeChannel(t *testing.T, handle func(rpcRequest) rpcResponse) *Channel {
	t.Helper()
	sp := &loopSpawner{handle: handle}
	stdin, stdout, kill, err := sp.Spawn(context.Background(), "test")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	return &Channel{
		name:   "testplugin",
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		kill:   kill,
	}
}

func TestChannel_Call_RoundTrips(t *testing.T) {
	ch := makeChannel(t, func(req rpcRequest) rpcResponse {
		return rpcResponse{Result: req.Params}
	})
	defer ch.Close() //nolint:errcheck

	params, _ := json.Marshal(map[string]string{"key": "val"})
	result, err := ch.Call(context.Background(), "echo", params)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got["key"] != "val" {
		t.Errorf("expected val, got %q", got["key"])
	}
}

func TestChannel_Call_SecondCallOnSameChannel(t *testing.T) {
	count := 0
	ch := makeChannel(t, func(req rpcRequest) rpcResponse {
		count++
		return rpcResponse{Result: json.RawMessage(`{"count":` + string(rune('0'+count)) + `}`)}
	})
	defer ch.Close() //nolint:errcheck

	for i := 0; i < 3; i++ {
		_, err := ch.Call(context.Background(), "ping", json.RawMessage(`{}`))
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
}

func TestChannel_Call_PluginErrorResponse(t *testing.T) {
	ch := makeChannel(t, func(_ rpcRequest) rpcResponse {
		return rpcResponse{Error: &rpcError{Code: -1, Message: "plugin says no"}}
	})
	defer ch.Close() //nolint:errcheck

	_, err := ch.Call(context.Background(), "any", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrCallPluginError) {
		t.Errorf("expected ErrCallPluginError in chain, got: %v", err)
	}
	if !strings.Contains(err.Error(), "plugin says no") {
		t.Errorf("expected plugin message in error, got: %v", err)
	}
}

func TestChannel_Call_TimesOut(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	killed := make(chan struct{})
	kill := func() {
		_ = inR.Close()
		_ = outW.Close()
		close(killed)
	}
	// Plugin never responds — goroutine just holds outR open.
	_ = outR
	_ = inW

	ch := &Channel{
		name:   "stall",
		stdin:  inW,
		stdout: bufio.NewReader(outR),
		kill:   kill,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := ch.Call(ctx, "stall", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error from timeout, got nil")
	}
	if !errors.Is(err, ErrCallTimeout) {
		t.Errorf("expected ErrCallTimeout, got: %v", err)
	}

	// Verify kill was invoked.
	select {
	case <-killed:
	case <-time.After(500 * time.Millisecond):
		t.Error("kill was not invoked after timeout")
	}
}

func TestChannel_Call_ProcessExited(t *testing.T) {
	// Simulate a process that has already exited: close both stdin reader
	// and stdout writer so writes to stdin fail and reads from stdout return EOF.
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	// Close both ends to simulate process exit.
	_ = inR.Close()  // makes writes to inW fail immediately
	_ = outW.Close() // makes reads from outR return EOF
	kill := func() {}
	ch := &Channel{
		name:   "dead",
		stdin:  inW,
		stdout: bufio.NewReader(outR),
		kill:   kill,
	}

	_, err := ch.Call(context.Background(), "any", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error from EOF, got nil")
	}
	if !errors.Is(err, ErrChannelClosed) {
		t.Errorf("expected ErrChannelClosed, got: %v", err)
	}
}

func TestChannel_Close_Idempotent(t *testing.T) {
	ch := makeChannel(t, func(_ rpcRequest) rpcResponse {
		return rpcResponse{Result: json.RawMessage(`{}`)}
	})
	if err := ch.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	// Second Close must not panic or error.
	if err := ch.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestHost_LoadForRun_KeepsChannelsAlive(t *testing.T) {
	tmp := t.TempDir()
	p := writeExe(t, tmp, "loop")

	// Use loopSpawner instead of fakeSpawner (which only handles one request).
	callCount := 0
	lsp := &loopSpawner{handle: func(req rpcRequest) rpcResponse {
		callCount++
		switch req.Method {
		case "apitest/hello":
			return rpcResponse{
				Result: json.RawMessage(`{"name":"loopplugin","version":"1.0","hooks":["on_request"],"protocol_version":1}`),
			}
		default:
			return rpcResponse{Result: json.RawMessage(`{"ok":true}`)}
		}
	}}

	host := NewHostWithSpawner(io.Discard, lsp)
	plugins, channels, loadErrs, err := host.LoadForRun(context.Background(), p)
	if err != nil {
		t.Fatalf("LoadForRun: %v", err)
	}
	if len(loadErrs) > 0 {
		t.Errorf("unexpected loadErrs: %+v", loadErrs)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	defer channels[0].Close() //nolint:errcheck

	// Make a second call over the same channel to verify it's still alive.
	_, err = channels[0].Call(context.Background(), "apitest/on_request", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestHost_LoadForRun_ReturnsChannelsParallelToPlugins(t *testing.T) {
	tmp := t.TempDir()
	pathA := writeExe(t, tmp, "a")
	pathB := writeExe(t, tmp, "b")

	makeLoop := func(name string) *loopSpawner {
		return &loopSpawner{handle: func(req rpcRequest) rpcResponse {
			if req.Method == "apitest/hello" {
				return rpcResponse{
					Result: json.RawMessage(`{"name":"` + name + `","version":"1.0","hooks":[],"protocol_version":1}`),
				}
			}
			return rpcResponse{Result: json.RawMessage(`{}`)}
		}}
	}

	// We need separate spawners per path, but Host takes a single spawner.
	// Use a multi-spawner shim.
	ms := &multiLoopSpawner{
		paths: map[string]*loopSpawner{
			pathA: makeLoop("plug-a"),
			pathB: makeLoop("plug-b"),
		},
	}

	host := NewHostWithSpawner(io.Discard, ms)
	env := pathA + pathSep() + pathB
	plugins, channels, _, err := host.LoadForRun(context.Background(), env)
	if err != nil {
		t.Fatalf("LoadForRun: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(plugins))
	}
	if len(channels) != len(plugins) {
		t.Fatalf("channels len %d != plugins len %d", len(channels), len(plugins))
	}
	for i, p := range plugins {
		if channels[i] == nil {
			t.Errorf("channel[%d] is nil (plugin %s)", i, p.Name)
		}
	}
	for _, ch := range channels {
		_ = ch.Close()
	}
}

func TestHost_Close_TerminatesRunningChannels(t *testing.T) {
	tmp := t.TempDir()
	p := writeExe(t, tmp, "loop")

	closedCh := make(chan struct{}, 1)
	lsp := &loopSpawner{handle: func(req rpcRequest) rpcResponse {
		if req.Method == "apitest/hello" {
			return rpcResponse{
				Result: json.RawMessage(`{"name":"killme","version":"1.0","hooks":[],"protocol_version":1}`),
			}
		}
		return rpcResponse{Result: json.RawMessage(`{}`)}
	}}
	// Wrap to capture kill calls.
	origSpawn := lsp
	wrapped := &killTrackSpawner{inner: origSpawn, killed: closedCh}

	host := NewHostWithSpawner(io.Discard, wrapped)
	_, channels, _, err := host.LoadForRun(context.Background(), p)
	if err != nil {
		t.Fatalf("LoadForRun: %v", err)
	}
	if len(channels) == 0 {
		t.Fatal("expected at least one channel")
	}

	if err := host.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-closedCh:
	case <-time.After(500 * time.Millisecond):
		t.Error("kill was not invoked by host.Close()")
	}
}

// multiLoopSpawner routes Spawn calls to per-path loopSpawners.
type multiLoopSpawner struct {
	paths map[string]*loopSpawner
}

func (m *multiLoopSpawner) Spawn(ctx context.Context, path string) (io.WriteCloser, io.ReadCloser, func(), error) {
	lsp, ok := m.paths[path]
	if !ok {
		lsp = &loopSpawner{handle: func(req rpcRequest) rpcResponse {
			return rpcResponse{Result: json.RawMessage(`{"name":"default","version":"1.0","hooks":[],"protocol_version":1}`)}
		}}
	}
	return lsp.Spawn(ctx, path)
}

// killTrackSpawner wraps an inner spawner and sends to killed when kill() is
// invoked.
type killTrackSpawner struct {
	inner  spawner
	killed chan<- struct{}
}

func (k *killTrackSpawner) Spawn(ctx context.Context, path string) (io.WriteCloser, io.ReadCloser, func(), error) {
	stdin, stdout, origKill, err := k.inner.Spawn(ctx, path)
	if err != nil {
		return nil, nil, nil, err
	}
	wrappedKill := func() {
		origKill()
		select {
		case k.killed <- struct{}{}:
		default:
		}
	}
	return stdin, stdout, wrappedKill, nil
}

// pathSep returns the OS path list separator as a string for test env building.
func pathSep() string {
	return string(filepath.ListSeparator)
}
