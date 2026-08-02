package websocket

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	gws "github.com/gorilla/websocket"

	"github.com/weiqigod/curlew/internal/parser"
)

// startHeartbeat launches a goroutine that sends ping control frames every
// cfg.IntervalMs milliseconds. If no pong is received within one interval,
// the goroutine sends ErrHeartbeatTimeout on the returned channel.
//
// The returned stop function cancels the heartbeat loop. Call it with defer
// immediately after startHeartbeat returns. The returned errCh receives at
// most one error and is closed when the goroutine exits.
//
// When cfg is nil or cfg.Enabled is false, the goroutine is not started and
// the returned errCh is immediately closed.
func startHeartbeat(ctx context.Context, conn Conn, cfg *parser.HeartbeatConfig) (func(), <-chan error) {
	errCh := make(chan error, 1)
	if cfg == nil || !cfg.Enabled {
		close(errCh)
		return func() {}, errCh
	}

	interval := time.Duration(cfg.IntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = 30 * time.Second
	}

	// pongReceived is set to true by the pong handler and cleared to false
	// before each ping. It is primed to true so the first interval does not
	// spuriously report a timeout before the first ping is sent.
	var pongReceived atomic.Bool
	pongReceived.Store(true)

	conn.SetPongHandler(func(string) error {
		pongReceived.Store(true)
		return nil
	})

	hbCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(errCh)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
				// Check pong from previous cycle.
				if !pongReceived.Swap(false) {
					select {
					case errCh <- fmt.Errorf("%w after %v", ErrHeartbeatTimeout, interval):
					default:
					}
					return
				}
				// Send a ping control frame.
				if err := conn.WriteControl(gws.PingMessage, nil, time.Now().Add(interval)); err != nil {
					select {
					case errCh <- fmt.Errorf("%w: %w", ErrHeartbeatFailed, err):
					default:
					}
					return
				}
			}
		}
	}()
	return cancel, errCh
}
