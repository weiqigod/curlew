package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// Sentinel errors for Channel operations.
var (
	// ErrCallTimeout is returned by Channel.Call when the context deadline
	// expires before the plugin responds. The plugin process is killed.
	ErrCallTimeout = errors.New("plugin call timeout")

	// ErrCallPluginError is returned by Channel.Call when the plugin returns
	// a JSON-RPC error object. The plugin error message is wrapped inside.
	ErrCallPluginError = errors.New("plugin call error")

	// ErrChannelClosed is returned by Channel.Call when the plugin process has
	// exited (stdout EOF) or the channel was already closed.
	ErrChannelClosed = errors.New("plugin channel closed")
)

// NewChannel creates a Channel for the given plugin. Intended for testing and
// hook dispatcher construction; production code uses Host.LoadForRun.
func NewChannel(name string, stdin io.WriteCloser, stdout *bufio.Reader, kill func()) *Channel {
	return &Channel{name: name, stdin: stdin, stdout: stdout, kill: kill}
}

// Channel is a live JSON-RPC duplex to a single running plugin.
// Call serializes requests; the wire protocol is request-response and plugins
// handle one call at a time. Safe for concurrent use — callers queue at the
// internal call mutex.
type Channel struct {
	name   string // plugin name (for diagnostics)
	stdin  io.WriteCloser
	stdout *bufio.Reader
	kill   func()
	callMu sync.Mutex // serialises concurrent Call invocations
	seq    int        // next request id (protected by callMu)
	closed atomic.Bool
	once   sync.Once // ensures kill is called at most once
}

// Call sends a JSON-RPC request to the plugin and blocks until the response
// or ctx.Done. If ctx expires first the channel is closed (kill() invoked)
// and ErrCallTimeout is returned. When the plugin returns a JSON-RPC error
// the error message is wrapped in ErrCallPluginError.
// Returns ErrChannelClosed when the channel is already closed or stdout EOF.
//
// Call serialises concurrent callers via callMu so the plugin sees one request
// at a time. kill() and the closed flag are managed via atomic and sync.Once
// so Close() can interrupt a blocked Call without any mutex deadlock.
func (c *Channel) Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("%w: channel already closed for plugin %s", ErrChannelClosed, c.name)
	}

	// Serialise calls: one request/response pair at a time on the wire.
	c.callMu.Lock()
	defer c.callMu.Unlock()

	// Re-check after acquiring the lock (another goroutine may have closed it).
	if c.closed.Load() {
		return nil, fmt.Errorf("%w: channel already closed for plugin %s", ErrChannelClosed, c.name)
	}

	c.seq++
	id := c.seq

	type readResult struct {
		resp rpcResponse
		err  error
	}
	resultCh := make(chan readResult, 1)

	// Run write+read in a goroutine so we can race against ctx.Done.
	// When Close() is called it invokes kill(), which closes the pipes and
	// causes any blocked read/write to return an error, unblocking this goroutine.
	go func() {
		if err := writeRequest(c.stdin, rpcRequest{
			ID:     id,
			Method: method,
			Params: params,
		}); err != nil {
			if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, io.EOF) {
				resultCh <- readResult{err: fmt.Errorf("%w: plugin %s stdin closed", ErrChannelClosed, c.name)}
				return
			}
			resultCh <- readResult{err: fmt.Errorf("write to plugin %s: %w", c.name, err)}
			return
		}
		resp, err := readResponse(c.stdout)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
				resultCh <- readResult{err: fmt.Errorf("%w: plugin %s stdout EOF", ErrChannelClosed, c.name)}
				return
			}
			resultCh <- readResult{err: fmt.Errorf("read from plugin %s: %w", c.name, err)}
			return
		}
		resultCh <- readResult{resp: resp}
	}()

	select {
	case <-ctx.Done():
		// Kill the process so the goroutine unblocks; mark channel closed.
		c.closed.Store(true)
		c.once.Do(c.kill)
		// Drain the result channel so the goroutine can exit.
		go func() { <-resultCh }()
		return nil, fmt.Errorf("%w: plugin %s method %s", ErrCallTimeout, c.name, method)
	case rr := <-resultCh:
		if rr.err != nil {
			if errors.Is(rr.err, ErrChannelClosed) {
				c.closed.Store(true)
			}
			return nil, rr.err
		}
		if rr.resp.Error != nil {
			return nil, fmt.Errorf("%w: plugin %s: %s", ErrCallPluginError, c.name, rr.resp.Error.Message)
		}
		return rr.resp.Result, nil
	}
}

// Close terminates the plugin process. Safe to call multiple times.
func (c *Channel) Close() error {
	c.closed.Store(true)
	c.once.Do(c.kill)
	return nil
}
