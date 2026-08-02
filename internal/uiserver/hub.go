package uiserver

import (
	"encoding/json"
	"sync"
	"sync/atomic"
)

// wsFrame is the server→client frame envelope (spec §5.1).
type wsFrame struct {
	Type  string          `json:"type"`
	RunID string          `json:"run_id,omitempty"`
	Data  json.RawMessage `json:"data"`
}

// marshalFrame builds a frame with data marshalled from v.
func marshalFrame(frameType, runID string, v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		data = []byte("{}")
	}
	out, _ := json.Marshal(wsFrame{Type: frameType, RunID: runID, Data: data})
	return out
}

// hubConn is one WebSocket connection's send side. A full buffer marks the
// connection slow; the hub closes it and the client reconnects and catches up
// via replay (spec §5.2). run.event frames flow only after the connection
// subscribes — the subscribe path replays missed events under the event-log
// lock and flips subscribed atomically, so there is no gap and no duplicate.
type hubConn struct {
	send       chan []byte
	once       sync.Once
	subscribed atomic.Bool
}

const connSendBuffer = 256

func (c *hubConn) trySend(frame []byte) bool {
	select {
	case c.send <- frame:
		return true
	default:
		return false
	}
}

func (c *hubConn) close() {
	c.once.Do(func() { close(c.send) })
}

// Hub broadcasts frames to all registered connections. No per-tab server
// state exists beyond the connection itself.
type Hub struct {
	mu    sync.Mutex
	conns map[*hubConn]struct{}
}

func newHub() *Hub {
	return &Hub{conns: make(map[*hubConn]struct{})}
}

// Register adds a connection and returns it.
func (h *Hub) Register() *hubConn {
	c := &hubConn{send: make(chan []byte, connSendBuffer)}
	h.mu.Lock()
	h.conns[c] = struct{}{}
	h.mu.Unlock()
	return c
}

// Unregister removes a connection and closes its send channel.
func (h *Hub) Unregister(c *hubConn) {
	h.mu.Lock()
	if _, ok := h.conns[c]; ok {
		delete(h.conns, c)
		c.close()
	}
	h.mu.Unlock()
}

// Broadcast sends frame to every connection; slow consumers are dropped.
func (h *Hub) Broadcast(frame []byte) {
	h.broadcast(frame, false)
}

// BroadcastRunEvent sends a run.event frame to subscribed connections only.
func (h *Hub) BroadcastRunEvent(frame []byte) {
	h.broadcast(frame, true)
}

func (h *Hub) broadcast(frame []byte, subscribedOnly bool) {
	h.mu.Lock()
	var slow []*hubConn
	for c := range h.conns {
		if subscribedOnly && !c.subscribed.Load() {
			continue
		}
		if !c.trySend(frame) {
			slow = append(slow, c)
		}
	}
	for _, c := range slow {
		delete(h.conns, c)
		c.close()
	}
	h.mu.Unlock()
}

// marshalRunEventFrame wraps a verbatim events-schema NDJSON line in a
// run.event frame without re-encoding the payload.
func marshalRunEventFrame(runID string, line []byte) []byte {
	out, _ := json.Marshal(wsFrame{Type: "run.event", RunID: runID, Data: json.RawMessage(line)})
	return out
}

// CloseAll closes every connection (server shutdown).
func (h *Hub) CloseAll() {
	h.mu.Lock()
	for c := range h.conns {
		delete(h.conns, c)
		c.close()
	}
	h.mu.Unlock()
}
