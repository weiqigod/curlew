package hooks_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/plugin"
	"github.com/weiqigod/curlew/internal/plugin/hooks"
)

// fakeHookPlugin is a scripted in-process plugin for hooks tests.
// It answers the methods it's configured for; unknown methods get an error.
type fakeHookPlugin struct {
	name     string
	hookList []string   // hooks declared in hello
	onReq    reqHandler // nil = pass-through
	onResp   respHandler
	onResult resultHandler
	stall    string // if non-empty, stall this method name
}

type (
	reqHandler    func(hooks.RequestPayload) (hooks.RequestPayload, error)
	respHandler   func(hooks.ResponsePayload) (hooks.ResponsePayload, error)
	resultHandler func(hooks.ResultPayload) error
)

// loopSpawner is a spawner that loops until EOF.
type loopSpawner struct {
	handle func(method string, params json.RawMessage) (json.RawMessage, error)
	stall  string // method name to stall on
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
			line, err := br.ReadBytes('\n')
			if err != nil {
				return
			}
			var req struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      int             `json:"id"`
				Method  string          `json:"method"`
				Params  json.RawMessage `json:"params,omitempty"`
			}
			if err := json.Unmarshal(line, &req); err != nil {
				return
			}
			// Stall requested method forever (until pipe is closed).
			if s.stall != "" && req.Method == s.stall {
				// Block indefinitely by reading from a channel that never fires.
				// The goroutine will unblock when inR is closed (kill()).
				buf := make([]byte, 1)
				_, _ = inR.Read(buf)
				return
			}
			result, callErr := s.handle(req.Method, req.Params)
			var resp map[string]any
			if callErr != nil {
				resp = map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"error":   map[string]any{"code": -1, "message": callErr.Error()},
				}
			} else {
				resp = map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"result":  json.RawMessage(result),
				}
			}
			out, _ := json.Marshal(resp)
			if _, err := outW.Write(append(out, '\n')); err != nil {
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

// buildPlugin creates a plugin.Plugin + plugin.Channel pair using fakeHookPlugin.
func buildPlugin(fp fakeHookPlugin) (plugin.Plugin, *plugin.Channel) {
	sp := &loopSpawner{
		stall: fp.stall,
		handle: func(method string, params json.RawMessage) (json.RawMessage, error) {
			switch method {
			case "curlew/hello":
				return json.Marshal(map[string]any{
					"name":             fp.name,
					"version":          "0.1.0",
					"hooks":            fp.hookList,
					"protocol_version": 1,
				})
			case "curlew/on_request":
				if fp.onReq == nil {
					// Identity pass-through — return the input unchanged.
					return params, nil
				}
				var p hooks.RequestPayload
				if err := json.Unmarshal(params, &p); err != nil {
					return nil, err
				}
				out, err := fp.onReq(p)
				if err != nil {
					return nil, err
				}
				return json.Marshal(out)
			case "curlew/on_response":
				if fp.onResp == nil {
					return params, nil
				}
				var p hooks.ResponsePayload
				if err := json.Unmarshal(params, &p); err != nil {
					return nil, err
				}
				out, err := fp.onResp(p)
				if err != nil {
					return nil, err
				}
				return json.Marshal(out)
			case "curlew/on_result":
				if fp.onResult == nil {
					return json.RawMessage(`{}`), nil
				}
				var p hooks.ResultPayload
				if err := json.Unmarshal(params, &p); err != nil {
					return nil, err
				}
				return json.RawMessage(`{}`), fp.onResult(p)
			default:
				return json.RawMessage(`{}`), nil
			}
		},
	}
	stdin, stdout, kill, _ := sp.Spawn(context.Background(), fp.name)
	ch := plugin.NewChannel(fp.name, stdin, bufio.NewReader(stdout), kill)
	p := plugin.Plugin{
		Name:    fp.name,
		Version: "0.1.0",
		Hooks:   fp.hookList,
		Path:    "/fake/" + fp.name,
	}
	return p, ch
}

// buildDispatcher creates a Dispatcher from a slice of fakeHookPlugins.
func buildDispatcher(t *testing.T, fps []fakeHookPlugin) (*hooks.Dispatcher, *strings.Builder) {
	t.Helper()
	var regs []hooks.Registration
	for _, fp := range fps {
		p, ch := buildPlugin(fp)
		regs = append(regs, hooks.Registration{Plugin: p, Channel: ch})
	}
	var sb strings.Builder
	d := hooks.NewDispatcher(&sb, regs)
	t.Cleanup(func() { _ = d.Close() })
	return d, &sb
}

func TestDispatcher_OnRequest_NoPlugins(t *testing.T) {
	d, _ := buildDispatcher(t, nil)
	in := hooks.RequestPayload{Method: "GET", URL: "https://example.com"}
	out, err := d.OnRequest(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Method != "GET" || out.URL != "https://example.com" {
		t.Errorf("expected pass-through, got %+v", out)
	}
}

func TestDispatcher_OnRequest_SinglePluginMutatesHeaders(t *testing.T) {
	fp := fakeHookPlugin{
		name:     "mutator",
		hookList: []string{"on_request"},
		onReq: func(p hooks.RequestPayload) (hooks.RequestPayload, error) {
			if p.Headers == nil {
				p.Headers = make(map[string]string)
			}
			p.Headers["X-Plugin"] = "mutator"
			return p, nil
		},
	}
	d, _ := buildDispatcher(t, []fakeHookPlugin{fp})
	in := hooks.RequestPayload{Method: "POST", URL: "https://api.test/v1"}
	out, err := d.OnRequest(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Headers["X-Plugin"] != "mutator" {
		t.Errorf("expected mutator header, got: %v", out.Headers)
	}
}

func TestDispatcher_OnRequest_TwoPluginsChainInOrder(t *testing.T) {
	fp1 := fakeHookPlugin{
		name:     "first",
		hookList: []string{"on_request"},
		onReq: func(p hooks.RequestPayload) (hooks.RequestPayload, error) {
			if p.Headers == nil {
				p.Headers = make(map[string]string)
			}
			p.Headers["X-Order"] = "first"
			return p, nil
		},
	}
	fp2 := fakeHookPlugin{
		name:     "second",
		hookList: []string{"on_request"},
		onReq: func(p hooks.RequestPayload) (hooks.RequestPayload, error) {
			// Must receive header from first.
			if p.Headers["X-Order"] != "first" {
				return p, errors.New("first plugin header not received")
			}
			p.Headers["X-Order"] = "second"
			return p, nil
		},
	}
	d, _ := buildDispatcher(t, []fakeHookPlugin{fp1, fp2})
	in := hooks.RequestPayload{Method: "GET", URL: "https://x.test"}
	out, err := d.OnRequest(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Headers["X-Order"] != "second" {
		t.Errorf("expected second, got %q", out.Headers["X-Order"])
	}
}

func TestDispatcher_OnRequest_PluginNotDeclaredOnRequestIsSkipped(t *testing.T) {
	fp := fakeHookPlugin{
		name:     "observer",
		hookList: []string{"on_result"}, // no on_request
	}
	d, _ := buildDispatcher(t, []fakeHookPlugin{fp})
	in := hooks.RequestPayload{Method: "GET", URL: "https://skip.test"}
	out, err := d.OnRequest(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.URL != "https://skip.test" {
		t.Errorf("expected pass-through, got %+v", out)
	}
}

func TestDispatcher_OnRequest_PluginRPCErrorAbortsWithErrHookAborted(t *testing.T) {
	fp := fakeHookPlugin{
		name:     "aborter",
		hookList: []string{"on_request"},
		onReq: func(_ hooks.RequestPayload) (hooks.RequestPayload, error) {
			return hooks.RequestPayload{}, errors.New("plugin says no")
		},
	}
	d, _ := buildDispatcher(t, []fakeHookPlugin{fp})
	_, err := d.OnRequest(context.Background(), hooks.RequestPayload{Method: "GET", URL: "https://x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, hooks.ErrHookAborted) {
		t.Errorf("expected ErrHookAborted in chain, got: %v", err)
	}
}

func TestDispatcher_OnRequest_TimeoutDropsPlugin_WarningEmitted(t *testing.T) {
	fp := fakeHookPlugin{
		name:     "staller",
		hookList: []string{"on_request"},
		stall:    "curlew/on_request",
	}
	d, sb := buildDispatcher(t, []fakeHookPlugin{fp})
	// Use a very short timeout so the test does not wait 10 seconds.
	hooks.SetHookTimeoutForTesting(d, 20*time.Millisecond)
	_, err := d.OnRequest(context.Background(), hooks.RequestPayload{Method: "GET", URL: "https://stall"})
	// Timeout should NOT abort the run — no error.
	if err != nil {
		t.Errorf("timeout should not abort; got err: %v", err)
	}
	if !strings.Contains(sb.String(), "timed out") {
		t.Errorf("expected timeout warning, got stderr: %q", sb.String())
	}
}

func TestDispatcher_OnRequest_SecondPluginRunsAfterFirstTimesOut(t *testing.T) {
	fp1 := fakeHookPlugin{
		name:     "slow",
		hookList: []string{"on_request"},
		stall:    "curlew/on_request",
	}
	fp2 := fakeHookPlugin{
		name:     "fast",
		hookList: []string{"on_request"},
		onReq: func(p hooks.RequestPayload) (hooks.RequestPayload, error) {
			if p.Headers == nil {
				p.Headers = make(map[string]string)
			}
			p.Headers["X-Fast"] = "yes"
			return p, nil
		},
	}
	d, _ := buildDispatcher(t, []fakeHookPlugin{fp1, fp2})
	hooks.SetHookTimeoutForTesting(d, 20*time.Millisecond)
	out, err := d.OnRequest(context.Background(), hooks.RequestPayload{Method: "GET", URL: "https://x"})
	if err != nil {
		t.Errorf("expected no error after timeout, got: %v", err)
	}
	if out.Headers["X-Fast"] != "yes" {
		t.Errorf("expected fast plugin to run, headers: %v", out.Headers)
	}
}

func TestDispatcher_OnResponse_AnnotationsAccumulate(t *testing.T) {
	fp1 := fakeHookPlugin{
		name:     "annotator1",
		hookList: []string{"on_response"},
		onResp: func(p hooks.ResponsePayload) (hooks.ResponsePayload, error) {
			p.Annotations = append(p.Annotations, "note-from-1")
			return p, nil
		},
	}
	fp2 := fakeHookPlugin{
		name:     "annotator2",
		hookList: []string{"on_response"},
		onResp: func(p hooks.ResponsePayload) (hooks.ResponsePayload, error) {
			p.Annotations = append(p.Annotations, "note-from-2")
			return p, nil
		},
	}
	d, _ := buildDispatcher(t, []fakeHookPlugin{fp1, fp2})
	in := hooks.ResponsePayload{StatusCode: 200, DurationMs: 50}
	out, err := d.OnResponse(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Annotations) != 2 {
		t.Errorf("expected 2 annotations, got %v", out.Annotations)
	}
}

func TestDispatcher_OnResponse_StatusCodeNotReplaced(t *testing.T) {
	fp := fakeHookPlugin{
		name:     "rewriter",
		hookList: []string{"on_response"},
		onResp: func(p hooks.ResponsePayload) (hooks.ResponsePayload, error) {
			// Plugin tries to change status code — should be ignored.
			p.StatusCode = 999
			return p, nil
		},
	}
	d, _ := buildDispatcher(t, []fakeHookPlugin{fp})
	in := hooks.ResponsePayload{StatusCode: 200, DurationMs: 30}
	out, err := d.OnResponse(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.StatusCode != 200 {
		t.Errorf("status code should be immutable; got %d", out.StatusCode)
	}
}

func TestDispatcher_OnResult_PassCountsForwarded(t *testing.T) {
	var captured hooks.ResultPayload
	fp := fakeHookPlugin{
		name:     "observer",
		hookList: []string{"on_result"},
		onResult: func(p hooks.ResultPayload) error {
			captured = p
			return nil
		},
	}
	d, _ := buildDispatcher(t, []fakeHookPlugin{fp})
	in := hooks.ResultPayload{PassCount: 5, FailCount: 2, DurationMs: 100}
	if err := d.OnResult(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.PassCount != 5 || captured.FailCount != 2 {
		t.Errorf("expected pass=5 fail=2, got %+v", captured)
	}
}

func TestDispatcher_OnResult_PerTestRowsForwarded(t *testing.T) {
	var captured hooks.ResultPayload
	fp := fakeHookPlugin{
		name:     "rowcheck",
		hookList: []string{"on_result"},
		onResult: func(p hooks.ResultPayload) error {
			captured = p
			return nil
		},
	}
	d, _ := buildDispatcher(t, []fakeHookPlugin{fp})
	in := hooks.ResultPayload{
		PassCount: 1,
		Tests: []hooks.ResultTestRow{
			{Name: "req-1", Status: "pass", DurationMs: 42},
		},
	}
	if err := d.OnResult(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(captured.Tests) != 1 || captured.Tests[0].Name != "req-1" {
		t.Errorf("expected rows forwarded, got %+v", captured.Tests)
	}
}

func TestDispatcher_OnResult_PluginErrorDoesNotPropagate(t *testing.T) {
	fp := fakeHookPlugin{
		name:     "errorer",
		hookList: []string{"on_result"},
		onResult: func(_ hooks.ResultPayload) error {
			return errors.New("on_result failed internally")
		},
	}
	d, _ := buildDispatcher(t, []fakeHookPlugin{fp})
	// Best-effort: no error propagation.
	if err := d.OnResult(context.Background(), hooks.ResultPayload{PassCount: 1}); err != nil {
		t.Errorf("expected no error propagation from on_result, got: %v", err)
	}
}

func TestDispatcher_HookTimeout_DropsPluginGlobally(t *testing.T) {
	fp := fakeHookPlugin{
		name:     "slow",
		hookList: []string{"on_request", "on_response"},
		stall:    "curlew/on_request",
	}
	d, _ := buildDispatcher(t, []fakeHookPlugin{fp})
	hooks.SetHookTimeoutForTesting(d, 20*time.Millisecond)
	// Trigger timeout on on_request.
	_, _ = d.OnRequest(context.Background(), hooks.RequestPayload{Method: "GET", URL: "https://x"})

	// Subsequent on_response should skip the timed-out plugin.
	resp := hooks.ResponsePayload{StatusCode: 200}
	out, err := d.OnResponse(context.Background(), resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No annotations added (plugin was dropped).
	if len(out.Annotations) != 0 {
		t.Errorf("expected no annotations from dropped plugin, got %v", out.Annotations)
	}
}

func TestDispatcher_Close_KillsAllChannels(t *testing.T) {
	fp := fakeHookPlugin{
		name:     "killme",
		hookList: []string{"on_request"},
	}
	d, _ := buildDispatcher(t, []fakeHookPlugin{fp})
	if err := d.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// A second Close should also be safe.
	if err := d.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestHookTimeout_ConstantIs10Seconds(t *testing.T) {
	if hooks.HookTimeout != 10*time.Second {
		t.Errorf("HookTimeout should be 10s, got %v", hooks.HookTimeout)
	}
}
