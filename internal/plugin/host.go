package plugin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	handshakeTimeout = 5 * time.Second
	shutdownTimeout  = 2 * time.Second

	// handshakeBufSize is the read-buffer size for the plugin's stdout.
	// 1 MiB is generous enough for any realistic handshake response.
	handshakeBufSize = 1 << 20
)

// spawner abstracts process launching so tests can substitute in-process pipes.
type spawner interface {
	Spawn(ctx context.Context, path string) (stdin io.WriteCloser, stdout io.ReadCloser, kill func(), err error)
}

// execSpawner is the production spawner backed by os/exec.
// stderr receives the subprocess's stderr output so plugin log lines are visible.
type execSpawner struct {
	stderr io.Writer
}

func (s execSpawner) Spawn(_ context.Context, path string) (io.WriteCloser, io.ReadCloser, func(), error) {
	return spawnPlugin(path, s.stderr)
}

// Host is the plugin loader and lifecycle manager.
type Host struct {
	stderr  io.Writer
	sp      spawner
	mu      sync.Mutex
	running []func() // kill funcs for long-lived plugin processes (M5-018+)
}

// NewHost returns a Host that spawns real subprocesses and writes diagnostics
// to stderr. The plugin subprocesses' stderr output is forwarded to the same
// writer so hook log lines are visible in the terminal.
func NewHost(stderr io.Writer) *Host {
	return &Host{stderr: stderr, sp: execSpawner{stderr: stderr}}
}

// NewHostWithSpawner returns a Host using the provided spawner (intended for tests).
func NewHostWithSpawner(stderr io.Writer, s spawner) *Host {
	return &Host{stderr: stderr, sp: s}
}

// Load discovers plugins from pluginsEnv, performs handshakes, and returns the
// successfully-loaded set. Non-fatal failures appear in loadErrs with Fatal=false;
// fatal failures (missing file, not-executable, duplicate name) have Fatal=true.
// The returned error is only non-nil for programmer errors (e.g., nil stderr).
func (h *Host) Load(ctx context.Context, pluginsEnv string) ([]Plugin, []LoadError, error) {
	if h.stderr == nil {
		return nil, nil, errors.New("plugin: Host.stderr is nil")
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
	seen := make(map[string]string, len(loaded))
	deduped := make([]Plugin, 0, len(loaded))
	for _, p := range loaded {
		if prev, ok := seen[p.Name]; ok {
			loadErrs = append(loadErrs, LoadError{
				Path:    p.Path,
				Message: fmt.Sprintf("duplicate plugin name %s (also at %s)", p.Name, prev),
				Fatal:   true,
			})
			continue
		}
		seen[p.Name] = p.Path
		deduped = append(deduped, p)
	}
	return deduped, loadErrs, nil
}

// handshakeOne spawns one plugin, sends the hello request, reads the response,
// and returns the parsed Plugin plus any per-plugin warnings.
func (h *Host) handshakeOne(ctx context.Context, path string) (*Plugin, []LoadError) {
	spawnCtx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	stdin, stdout, kill, err := h.sp.Spawn(spawnCtx, path)
	if err != nil {
		return nil, []LoadError{{
			Path:    path,
			Message: fmt.Sprintf("plugin %s spawn failed: %v", path, err),
			Fatal:   false,
		}}
	}
	defer kill()
	defer stdin.Close() //nolint:errcheck // best-effort close

	type readResult struct {
		resp rpcResponse
		err  error
	}
	ch := make(chan readResult, 1)
	br := bufio.NewReaderSize(stdout, handshakeBufSize)
	go func() {
		if err := writeRequest(stdin, rpcRequest{
			ID: 1, Method: "curlew/hello",
			Params: []byte(`{}`),
		}); err != nil {
			ch <- readResult{err: fmt.Errorf("write handshake: %w", err)}
			return
		}
		r, e := readResponse(br)
		ch <- readResult{r, e}
	}()

	select {
	case <-spawnCtx.Done():
		if errors.Is(spawnCtx.Err(), context.DeadlineExceeded) {
			return nil, []LoadError{{
				Path:    path,
				Message: fmt.Sprintf("plugin %s handshake timeout", path),
				Fatal:   false,
			}}
		}
		return nil, []LoadError{{
			Path:    path,
			Message: fmt.Sprintf("plugin %s handshake canceled", path),
			Fatal:   false,
		}}

	case rr := <-ch:
		if rr.err != nil {
			return nil, []LoadError{{
				Path:    path,
				Message: fmt.Sprintf("plugin %s: invalid handshake response: %v", path, rr.err),
				Fatal:   false,
			}}
		}
		if rr.resp.Error != nil {
			return nil, []LoadError{{
				Path:    path,
				Message: fmt.Sprintf("plugin %s: handshake error: %s", path, rr.resp.Error.Message),
				Fatal:   false,
			}}
		}
		hello, err := parseHello(rr.resp.Result)
		if err != nil {
			return nil, []LoadError{{
				Path:    path,
				Message: fmt.Sprintf("plugin %s: %v", path, err),
				Fatal:   false,
			}}
		}

		var warns []LoadError

		// Protocol version warning.
		if hello.ProtocolVersion != 1 {
			warns = append(warns, LoadError{
				Path: path,
				Message: fmt.Sprintf("plugin %s: unsupported protocol_version %d, using 1",
					hello.Name, hello.ProtocolVersion),
				Fatal: false,
			})
		}

		// Hook filtering: drop unknown hooks with a warning.
		filtered := make([]string, 0, len(hello.Hooks))
		for _, hk := range hello.Hooks {
			if _, ok := knownHooks[hk]; !ok {
				warns = append(warns, LoadError{
					Path:    path,
					Message: fmt.Sprintf("plugin %s: unknown hook %s ignored", hello.Name, hk),
					Fatal:   false,
				})
				continue
			}
			filtered = append(filtered, hk)
		}

		return &Plugin{
			Name:    hello.Name,
			Version: hello.Version,
			Hooks:   filtered,
			Path:    path,
		}, warns
	}
}

// LoadForRun performs the same handshake as Load but keeps each plugin's stdio
// channel alive after the handshake so callers can dispatch hook calls. The
// returned channels slice is parallel to the plugins slice (same index, same
// order). Call host.Close() to terminate all processes when the run ends.
//
// Non-fatal failures appear in loadErrs with Fatal=false; fatal failures have
// Fatal=true. The returned error is only non-nil for programmer errors.
func (h *Host) LoadForRun(ctx context.Context, pluginsEnv string) ([]Plugin, []*Channel, []LoadError, error) {
	if h.stderr == nil {
		return nil, nil, nil, errors.New("plugin: Host.stderr is nil")
	}
	candidates, loadErrs := discover(pluginsEnv)

	var loaded []Plugin
	var channels []*Channel

	for _, path := range candidates {
		p, ch, lerrs := h.handshakeOneForRun(ctx, path)
		loadErrs = append(loadErrs, lerrs...)
		if p != nil {
			loaded = append(loaded, *p)
			channels = append(channels, ch)
		}
	}

	// Duplicate name detection (post-handshake). Close duplicate channels.
	seen := make(map[string]string, len(loaded))
	deduped := make([]Plugin, 0, len(loaded))
	dedupedCh := make([]*Channel, 0, len(channels))
	for i, p := range loaded {
		if prev, ok := seen[p.Name]; ok {
			loadErrs = append(loadErrs, LoadError{
				Path:    p.Path,
				Message: fmt.Sprintf("duplicate plugin name %s (also at %s)", p.Name, prev),
				Fatal:   true,
			})
			_ = channels[i].Close() // terminate duplicate process
			continue
		}
		seen[p.Name] = p.Path
		deduped = append(deduped, p)
		dedupedCh = append(dedupedCh, channels[i])
	}

	// Register surviving channels so host.Close() can terminate them.
	h.mu.Lock()
	for _, ch := range dedupedCh {
		ch := ch // capture
		h.running = append(h.running, func() { _ = ch.Close() })
	}
	h.mu.Unlock()

	return deduped, dedupedCh, loadErrs, nil
}

// handshakeOneForRun spawns one plugin, performs the handshake, and keeps the
// stdio channel alive. It returns the parsed Plugin, the live Channel, and any
// per-plugin warnings.
func (h *Host) handshakeOneForRun(ctx context.Context, path string) (*Plugin, *Channel, []LoadError) {
	spawnCtx, cancel := context.WithTimeout(ctx, handshakeTimeout)

	stdin, stdout, kill, err := h.sp.Spawn(spawnCtx, path)
	if err != nil {
		cancel()
		return nil, nil, []LoadError{{
			Path:    path,
			Message: fmt.Sprintf("plugin %s spawn failed: %v", path, err),
			Fatal:   false,
		}}
	}

	// Channel owns the kill func; cancel the spawnCtx after handshake completes.
	br := bufio.NewReaderSize(stdout, handshakeBufSize)

	type readResult struct {
		resp rpcResponse
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		if err := writeRequest(stdin, rpcRequest{
			ID: 1, Method: "curlew/hello",
			Params: []byte(`{}`),
		}); err != nil {
			ch <- readResult{err: fmt.Errorf("write handshake: %w", err)}
			return
		}
		r, e := readResponse(br)
		ch <- readResult{r, e}
	}()

	select {
	case <-spawnCtx.Done():
		cancel()
		kill()
		if errors.Is(spawnCtx.Err(), context.DeadlineExceeded) {
			return nil, nil, []LoadError{{
				Path:    path,
				Message: fmt.Sprintf("plugin %s handshake timeout", path),
				Fatal:   false,
			}}
		}
		return nil, nil, []LoadError{{
			Path:    path,
			Message: fmt.Sprintf("plugin %s handshake canceled", path),
			Fatal:   false,
		}}

	case rr := <-ch:
		cancel() // handshake done; release spawnCtx
		if rr.err != nil {
			kill()
			return nil, nil, []LoadError{{
				Path:    path,
				Message: fmt.Sprintf("plugin %s: invalid handshake response: %v", path, rr.err),
				Fatal:   false,
			}}
		}
		if rr.resp.Error != nil {
			kill()
			return nil, nil, []LoadError{{
				Path:    path,
				Message: fmt.Sprintf("plugin %s: handshake error: %s", path, rr.resp.Error.Message),
				Fatal:   false,
			}}
		}
		hello, err := parseHello(rr.resp.Result)
		if err != nil {
			kill()
			return nil, nil, []LoadError{{
				Path:    path,
				Message: fmt.Sprintf("plugin %s: %v", path, err),
				Fatal:   false,
			}}
		}

		var warns []LoadError

		// Protocol version warning.
		if hello.ProtocolVersion != 1 {
			warns = append(warns, LoadError{
				Path: path,
				Message: fmt.Sprintf("plugin %s: unsupported protocol_version %d, using 1",
					hello.Name, hello.ProtocolVersion),
				Fatal: false,
			})
		}

		// Hook filtering: drop unknown hooks with a warning.
		filtered := make([]string, 0, len(hello.Hooks))
		for _, hk := range hello.Hooks {
			if _, ok := knownHooks[hk]; !ok {
				warns = append(warns, LoadError{
					Path:    path,
					Message: fmt.Sprintf("plugin %s: unknown hook %s ignored", hello.Name, hk),
					Fatal:   false,
				})
				continue
			}
			filtered = append(filtered, hk)
		}

		liveCh := &Channel{
			name:   hello.Name,
			stdin:  stdin,
			stdout: br,
			kill:   kill,
		}

		return &Plugin{
			Name:    hello.Name,
			Version: hello.Version,
			Hooks:   filtered,
			Path:    path,
		}, liveCh, warns
	}
}

// Close signals all long-lived plugin processes to terminate.
func (h *Host) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, k := range h.running {
		k()
	}
	h.running = nil
	return nil
}
