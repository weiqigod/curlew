// Package hooks dispatches lifecycle events to loaded plugins over the
// persistent JSON-RPC channels established by plugin.Host.LoadForRun.
// Three hook points are supported: on_request (before HTTP send, may mutate),
// on_response (after HTTP receive, may annotate), and on_result (run end, read-only).
package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/peterlindqvist/apitest/internal/plugin"
)

// HookTimeout is the hard per-hook wall-clock limit.
// Tests that need a shorter timeout should construct the Dispatcher via
// NewDispatcher and call its internal timeout field through export_test.go.
const HookTimeout = 10 * time.Second

// ErrHookAborted is returned from OnRequest when a plugin's hook returns a
// JSON-RPC error. The caller should mark the request as error (not fail).
var ErrHookAborted = errors.New("plugin hook aborted request")

// Registration associates a plugin with its live channel and the hooks it declared.
type Registration struct {
	Plugin  plugin.Plugin
	Channel *plugin.Channel
}

// RequestPayload is the serialised input to on_request and the output that
// may mutate the request before it is sent.
type RequestPayload struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        any               `json:"body,omitempty"`
	QueryParams map[string]string `json:"query_params,omitempty"`
}

// ResponsePayload is the serialised input to on_response. Status, headers, and
// body are read-only; plugins may append to Annotations.
type ResponsePayload struct {
	StatusCode  int               `json:"status_code"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        json.RawMessage   `json:"body,omitempty"`
	DurationMs  int64             `json:"duration_ms"`
	Annotations []string          `json:"annotations,omitempty"`
}

// ResultPayload is the serialised input to on_result, fired once at run end.
type ResultPayload struct {
	PassCount  int             `json:"pass_count"`
	FailCount  int             `json:"fail_count"`
	SkipCount  int             `json:"skip_count"`
	DurationMs int64           `json:"duration_ms"`
	Tests      []ResultTestRow `json:"tests,omitempty"`
}

// ResultTestRow holds the per-request outcome in ResultPayload.Tests.
type ResultTestRow struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // "pass" | "fail" | "skip" | "error"
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// Dispatcher runs the registered hooks for each lifecycle event.
// Construct via NewDispatcher. Safe for concurrent use across goroutines.
type Dispatcher struct {
	stderr      io.Writer
	regs        []Registration
	hookTimeout time.Duration // per-call timeout; defaults to HookTimeout

	mu         sync.Mutex
	onRequest  []*Registration // live registrations for on_request
	onResponse []*Registration // live registrations for on_response
	onResult   []*Registration // live registrations for on_result
}

// DispatcherOption configures optional behaviour on a Dispatcher.
type DispatcherOption func(*Dispatcher)

// WithHookTimeout overrides the default per-hook timeout (HookTimeout).
// Primarily used in tests to avoid waiting the full 10 seconds.
func WithHookTimeout(d time.Duration) DispatcherOption {
	return func(disp *Dispatcher) {
		disp.hookTimeout = d
	}
}

// NewDispatcher builds a Dispatcher from the successfully loaded plugins.
// Plugins are invoked in slice order; stderr receives timeout warnings.
// The per-hook timeout defaults to HookTimeout (10 seconds).
func NewDispatcher(stderr io.Writer, regs []Registration, opts ...DispatcherOption) *Dispatcher {
	d := &Dispatcher{
		stderr:      stderr,
		regs:        regs,
		hookTimeout: HookTimeout,
	}
	for _, o := range opts {
		o(d)
	}
	for i := range regs {
		r := &regs[i]
		for _, h := range r.Plugin.Hooks {
			switch h {
			case "on_request":
				d.onRequest = append(d.onRequest, r)
			case "on_response":
				d.onResponse = append(d.onResponse, r)
			case "on_result":
				d.onResult = append(d.onResult, r)
			}
		}
	}
	return d
}

// OnRequest is called before Execute. Each plugin may mutate the request;
// the returned RequestPayload is chained into the next plugin. A JSON-RPC
// error from any plugin is returned with ErrHookAborted in the error chain;
// timeouts drop the plugin and continue.
func (d *Dispatcher) OnRequest(ctx context.Context, req RequestPayload) (RequestPayload, error) {
	regs := d.liveRegistrations("on_request")
	cur := req
	for _, r := range regs {
		out, err := d.callOne(ctx, r, "apitest/on_request", cur)
		if err != nil {
			if errors.Is(err, plugin.ErrCallTimeout) {
				d.dropPlugin(r, "on_request")
				_, _ = fmt.Fprintf(d.stderr, "warning: plugin %s timed out on on_request\n", r.Plugin.Name)
				continue
			}
			if errors.Is(err, plugin.ErrCallPluginError) {
				return cur, fmt.Errorf("%w: %w", ErrHookAborted, err)
			}
			// Channel closed or other error — drop and continue.
			d.dropPlugin(r, "on_request")
			_, _ = fmt.Fprintf(d.stderr, "warning: plugin %s error on on_request: %v\n", r.Plugin.Name, err)
			continue
		}
		var next RequestPayload
		if err := json.Unmarshal(out, &next); err != nil {
			// Malformed response — keep current request.
			continue
		}
		// Merge: only replace fields that the plugin set to non-zero values.
		// This allows identity responses (empty {}) to pass through unchanged.
		cur = mergeRequestPayload(cur, next)
	}
	return cur, nil
}

// OnResponse is called after Execute. Plugins may append to Annotations;
// status/body/headers are never replaced. Timeouts drop the plugin.
// Never returns an abort error.
func (d *Dispatcher) OnResponse(ctx context.Context, resp ResponsePayload) (ResponsePayload, error) {
	regs := d.liveRegistrations("on_response")
	orig := resp // preserve immutable fields
	cur := resp
	for _, r := range regs {
		out, err := d.callOne(ctx, r, "apitest/on_response", cur)
		if err != nil {
			if errors.Is(err, plugin.ErrCallTimeout) {
				d.dropPlugin(r, "on_response")
				_, _ = fmt.Fprintf(d.stderr, "warning: plugin %s timed out on on_response\n", r.Plugin.Name)
				continue
			}
			d.dropPlugin(r, "on_response")
			_, _ = fmt.Fprintf(d.stderr, "warning: plugin %s error on on_response: %v\n", r.Plugin.Name, err)
			continue
		}
		var pluginResp ResponsePayload
		if err := json.Unmarshal(out, &pluginResp); err != nil {
			continue
		}
		// Replace annotations with the plugin's returned list (the plugin
		// receives existing annotations and returns the cumulative set).
		cur.Annotations = pluginResp.Annotations
	}
	// Restore immutable fields: status code, headers, body.
	cur.StatusCode = orig.StatusCode
	cur.Headers = orig.Headers
	cur.Body = orig.Body
	return cur, nil
}

// OnResult fires once at run completion. Errors are best-effort (logged to
// stderr); the run's exit code is unaffected.
func (d *Dispatcher) OnResult(ctx context.Context, res ResultPayload) error {
	regs := d.liveRegistrations("on_result")
	for _, r := range regs {
		out, err := d.callOne(ctx, r, "apitest/on_result", res)
		if err != nil {
			if errors.Is(err, plugin.ErrCallTimeout) {
				d.dropPlugin(r, "on_result")
			}
			_, _ = fmt.Fprintf(d.stderr, "warning: plugin %s error on on_result: %v\n", r.Plugin.Name, err)
			continue
		}
		_ = out // on_result return value is ignored
	}
	return nil
}

// Close terminates all plugin processes. Safe to call multiple times.
func (d *Dispatcher) Close() error {
	d.mu.Lock()
	regs := make([]*Registration, len(d.regs))
	for i := range d.regs {
		regs[i] = &d.regs[i]
	}
	d.mu.Unlock()
	for _, r := range regs {
		_ = r.Channel.Close()
	}
	return nil
}

// callOne invokes a single plugin hook with an independent HookTimeout.
// The per-plugin timeout is derived from the caller's context so that
// run cancellation (e.g. Ctrl+C, test deadline) also cancels in-flight hooks.
// If the plugin does not respond within HookTimeout, ErrCallTimeout is
// returned and the plugin process is killed.
func (d *Dispatcher) callOne(ctx context.Context, r *Registration, method string, payload any) (json.RawMessage, error) {
	params, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal hook params: %w", err)
	}
	// Derive timeout from the caller's context so cancellation propagates.
	callCtx, cancel := context.WithTimeout(ctx, d.hookTimeout)
	defer cancel()
	return r.Channel.Call(callCtx, method, params)
}

// liveRegistrations returns a snapshot of registrations for the given hook.
func (d *Dispatcher) liveRegistrations(hook string) []*Registration {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch hook {
	case "on_request":
		out := make([]*Registration, len(d.onRequest))
		copy(out, d.onRequest)
		return out
	case "on_response":
		out := make([]*Registration, len(d.onResponse))
		copy(out, d.onResponse)
		return out
	case "on_result":
		out := make([]*Registration, len(d.onResult))
		copy(out, d.onResult)
		return out
	}
	return nil
}

// dropPlugin removes r from all live hook lists.
func (d *Dispatcher) dropPlugin(r *Registration, hook string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onRequest = removeReg(d.onRequest, r)
	d.onResponse = removeReg(d.onResponse, r)
	d.onResult = removeReg(d.onResult, r)
	_ = hook // hook name is for the warning message, not needed here
}

func removeReg(slice []*Registration, r *Registration) []*Registration {
	// Use a fresh slice to avoid holding pointers in the backing array,
	// which would prevent GC of dropped registrations and their channels.
	out := make([]*Registration, 0, len(slice))
	for _, v := range slice {
		if v != r {
			out = append(out, v)
		}
	}
	return out
}

// mergeRequestPayload applies non-zero fields from next onto cur.
// This preserves the original request when a plugin returns an identity
// response (empty {}).
func mergeRequestPayload(cur, next RequestPayload) RequestPayload {
	if next.Method != "" {
		cur.Method = next.Method
	}
	if next.URL != "" {
		cur.URL = next.URL
	}
	if next.Headers != nil {
		cur.Headers = next.Headers
	}
	if next.Body != nil {
		cur.Body = next.Body
	}
	if next.QueryParams != nil {
		cur.QueryParams = next.QueryParams
	}
	return cur
}
