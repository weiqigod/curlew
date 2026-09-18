package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeSpawner creates a fakeSpawner with the given paths mapped to plugins.
func makeSpawner(plugins map[string]*fakePlugin) *fakeSpawner {
	if plugins == nil {
		plugins = map[string]*fakePlugin{}
	}
	return &fakeSpawner{plugins: plugins}
}

func TestHost_Load(t *testing.T) {
	type tc struct {
		name        string
		setup       func(t *testing.T, tmp string) (env string, sp *fakeSpawner)
		wantNames   []string
		wantErrLike []string
		wantFatal   bool
	}

	tests := []tc{
		{
			name: "zero plugins when env is empty",
			setup: func(t *testing.T, tmp string) (string, *fakeSpawner) {
				return "", makeSpawner(nil)
			},
			wantNames: nil,
		},
		{
			name: "single plugin happy path",
			setup: func(t *testing.T, tmp string) (string, *fakeSpawner) {
				p := writeExe(t, tmp, "hello")
				sp := makeSpawner(map[string]*fakePlugin{
					p: {handler: echoHello},
				})
				return p, sp
			},
			wantNames: []string{"hello-plugin"},
		},
		{
			name: "unknown hook ignored with warning",
			setup: func(t *testing.T, tmp string) (string, *fakeSpawner) {
				p := writeExe(t, tmp, "plug")
				handler := func(_ rpcRequest) rpcResponse {
					return rpcResponse{
						Result: json.RawMessage(
							`{"name":"plug","version":"1.0","hooks":["on_request","on_magic"],"protocol_version":1}`,
						),
					}
				}
				sp := makeSpawner(map[string]*fakePlugin{p: {handler: handler}})
				return p, sp
			},
			wantNames:   []string{"plug"},
			wantErrLike: []string{"unknown hook on_magic ignored"},
		},
		{
			name: "duplicate plugin name is fatal",
			setup: func(t *testing.T, tmp string) (string, *fakeSpawner) {
				a := writeExe(t, tmp, "a")
				b := writeExe(t, tmp, "b")
				handler := func(_ rpcRequest) rpcResponse {
					return rpcResponse{
						Result: json.RawMessage(
							`{"name":"same-name","version":"1.0","hooks":[],"protocol_version":1}`,
						),
					}
				}
				sp := makeSpawner(map[string]*fakePlugin{
					a: {handler: handler},
					b: {handler: handler},
				})
				env := a + string(filepath.ListSeparator) + b
				return env, sp
			},
			wantNames:   []string{"same-name"},
			wantErrLike: []string{"duplicate plugin name"},
			wantFatal:   true,
		},
		{
			name: "protocol error: missing name",
			setup: func(t *testing.T, tmp string) (string, *fakeSpawner) {
				p := writeExe(t, tmp, "bad")
				handler := func(_ rpcRequest) rpcResponse {
					return rpcResponse{
						Result: json.RawMessage(`{"name":"","version":"1.0","hooks":[]}`),
					}
				}
				sp := makeSpawner(map[string]*fakePlugin{p: {handler: handler}})
				return p, sp
			},
			wantNames:   nil,
			wantErrLike: []string{"missing name"},
		},
		{
			name: "jsonrpc error response",
			setup: func(t *testing.T, tmp string) (string, *fakeSpawner) {
				p := writeExe(t, tmp, "err")
				handler := func(_ rpcRequest) rpcResponse {
					return rpcResponse{
						Error: &rpcError{Code: -32601, Message: "method not found"},
					}
				}
				sp := makeSpawner(map[string]*fakePlugin{p: {handler: handler}})
				return p, sp
			},
			wantNames:   nil,
			wantErrLike: []string{"handshake error: method not found"},
		},
		{
			name: "protocol_version=2 warning but plugin still loaded",
			setup: func(t *testing.T, tmp string) (string, *fakeSpawner) {
				p := writeExe(t, tmp, "future")
				handler := func(_ rpcRequest) rpcResponse {
					return rpcResponse{
						Result: json.RawMessage(
							`{"name":"future-plug","version":"2.0","hooks":[],"protocol_version":2}`,
						),
					}
				}
				sp := makeSpawner(map[string]*fakePlugin{p: {handler: handler}})
				return p, sp
			},
			wantNames:   []string{"future-plug"},
			wantErrLike: []string{"unsupported protocol_version 2"},
		},
		{
			name: "missing path is fatal",
			setup: func(t *testing.T, _ string) (string, *fakeSpawner) {
				return "/does/not/exist/plugin", makeSpawner(nil)
			},
			wantNames:   nil,
			wantErrLike: []string{"not found"},
			wantFatal:   true,
		},
		{
			name: "stderr nil returns error",
			setup: func(t *testing.T, _ string) (string, *fakeSpawner) {
				return "", makeSpawner(nil)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			env, sp := tc.setup(t, tmp)

			var stderrBuf bytes.Buffer
			var host *Host
			if tc.name == "stderr nil returns error" {
				host = NewHostWithSpawner(nil, sp)
				_, _, err := host.Load(context.Background(), env)
				if err == nil {
					t.Error("expected error for nil stderr")
				}
				return
			}
			host = NewHostWithSpawner(&stderrBuf, sp)
			loaded, loadErrs, err := host.Load(context.Background(), env)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify loaded names.
			gotNames := make([]string, len(loaded))
			for i, p := range loaded {
				gotNames[i] = p.Name
			}
			if len(tc.wantNames) == 0 && len(gotNames) == 0 {
				// both empty — ok
			} else if !stringSliceContainsAll(gotNames, tc.wantNames) {
				t.Errorf("loaded names: got %v, want %v", gotNames, tc.wantNames)
			}

			// Verify error messages.
			for i, substr := range tc.wantErrLike {
				found := false
				for _, le := range loadErrs {
					if strings.Contains(le.Message, substr) {
						found = true
						if tc.wantFatal && i == len(tc.wantErrLike)-1 {
							if !le.Fatal {
								t.Errorf("expected fatal error for %q", substr)
							}
						}
						break
					}
				}
				if !found {
					t.Errorf("expected loadErr containing %q, got errs: %+v", substr, loadErrs)
				}
			}
		})
	}
}

func TestHost_Close_NoOp(t *testing.T) {
	host := NewHost(io.Discard)
	if err := host.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

// TestHost_Load_OnResultHook verifies that on_result is a known hook and that
// plugins declaring it are loaded without warnings.
func TestHost_Load_OnResultHook(t *testing.T) {
	tests := []struct {
		name        string
		hooks       []string
		wantHooks   []string
		wantWarnStr string
	}{
		{
			name:      "all three known hooks",
			hooks:     []string{"on_request", "on_response", "on_result"},
			wantHooks: []string{"on_request", "on_response", "on_result"},
		},
		{
			name:      "on_result alone",
			hooks:     []string{"on_result"},
			wantHooks: []string{"on_result"},
		},
		{
			name:        "unknown mixed with on_result",
			hooks:       []string{"on_result", "on_magic"},
			wantHooks:   []string{"on_result"},
			wantWarnStr: "unknown hook on_magic ignored",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			p := writeExe(t, tmp, "plug")
			hooksJSON, _ := json.Marshal(tc.hooks)
			resp := json.RawMessage(
				`{"name":"testplug","version":"1.0","hooks":` + string(hooksJSON) + `,"protocol_version":1}`,
			)
			sp := makeSpawner(map[string]*fakePlugin{
				p: {handler: func(_ rpcRequest) rpcResponse {
					return rpcResponse{Result: resp}
				}},
			})
			var stderrBuf bytes.Buffer
			host := NewHostWithSpawner(&stderrBuf, sp)
			loaded, loadErrs, err := host.Load(context.Background(), p)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(loaded) != 1 {
				t.Fatalf("expected 1 loaded plugin, got %d", len(loaded))
			}
			// Verify the exact hook list.
			if len(loaded[0].Hooks) != len(tc.wantHooks) {
				t.Errorf("hooks: got %v, want %v", loaded[0].Hooks, tc.wantHooks)
			}
			for i, h := range tc.wantHooks {
				if i >= len(loaded[0].Hooks) || loaded[0].Hooks[i] != h {
					t.Errorf("hook[%d]: got %v, want %s", i, loaded[0].Hooks, h)
				}
			}
			// Verify warning presence/absence.
			if tc.wantWarnStr != "" {
				found := false
				for _, le := range loadErrs {
					if strings.Contains(le.Message, tc.wantWarnStr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected warning containing %q; loadErrs=%+v", tc.wantWarnStr, loadErrs)
				}
			} else if len(loadErrs) > 0 {
				t.Errorf("expected no warnings; got %+v", loadErrs)
			}
		})
	}
}

func TestHost_Load_HandshakeTimeout(t *testing.T) {
	tmp := t.TempDir()
	p := writeExe(t, tmp, "stall")
	sp := makeSpawner(map[string]*fakePlugin{
		p: {stall: true},
	})
	host := NewHostWithSpawner(io.Discard, sp)

	// Use a pre-cancelled context so the test does not actually wait 5s.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled

	loaded, loadErrs, err := host.Load(ctx, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected 0 loaded, got %v", loaded)
	}
	// Should have exactly one non-fatal error containing "timeout" or "canceled".
	if len(loadErrs) != 1 {
		t.Fatalf("expected 1 loadErr, got %+v", loadErrs)
	}
	if loadErrs[0].Fatal {
		t.Error("timeout/canceled error should be non-fatal")
	}
	if !strings.Contains(loadErrs[0].Message, "timeout") &&
		!strings.Contains(loadErrs[0].Message, "canceled") {
		t.Errorf("unexpected message: %s", loadErrs[0].Message)
	}
}

// writeExe writes a minimal executable file into dir with the given base name
// and returns its full path.
func writeExe(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, discoveryExecutableName(name))
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// stringSliceContainsAll checks that all want elements appear in got.
func stringSliceContainsAll(got, want []string) bool {
	gotSet := make(map[string]struct{}, len(got))
	for _, g := range got {
		gotSet[g] = struct{}{}
	}
	for _, w := range want {
		if _, ok := gotSet[w]; !ok {
			return false
		}
	}
	return true
}
