package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
)

// fakePlugin is a scripted in-process "plugin" used by tests. The handler
// receives a decoded request and returns a response to emit.
// If stall is true, the plugin never responds (drives timeout tests).
type fakePlugin struct {
	handler func(rpcRequest) rpcResponse
	stall   bool
}

// fakeSpawner implements the spawner interface using in-process pipes.
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
		defer func() { _ = outW.Close() }()
		if p.stall {
			<-ctx.Done()
			return
		}
		br := bufio.NewReader(inR)
		req, err := readRequestForTest(br)
		if err != nil {
			return
		}
		resp := p.handler(req)
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
	s.mu.Lock()
	s.started = append(s.started, kill)
	s.mu.Unlock()
	return inW, outR, kill, nil
}

// readRequestForTest reads a single newline-framed JSON-RPC request from r.
func readRequestForTest(r *bufio.Reader) (rpcRequest, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return rpcRequest{}, err
	}
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return req, err
	}
	return req, nil
}

// echoHello returns the canonical hello-plugin handshake response.
func echoHello(_ rpcRequest) rpcResponse {
	return rpcResponse{
		Result: json.RawMessage(
			`{"name":"hello-plugin","version":"0.1.0","hooks":["on_request","on_response"],"protocol_version":1}`,
		),
	}
}
