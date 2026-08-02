package uiserver

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

const (
	wsWriteWait    = 10 * time.Second
	wsPingInterval = 30 * time.Second
	wsReadDeadline = 90 * time.Second
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// Accept when Origin is absent (non-browser clients) or parses to
	// http/https with a loopback hostname, port ignored (spec §5).
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return false
		}
		return loopbackHostname(u.Hostname())
	},
}

// clientFrame is the client→server message shape (spec §5.1).
type clientFrame struct {
	Type   string `json:"type"`
	RunID  string `json:"run_id"`
	FromID int64  `json:"from_id"`
}

// handleWS upgrades GET /api/v1/ws and runs the read/write pumps.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the error
	}
	conn := s.hub.Register()
	defer func() {
		s.hub.Unregister(conn)
		_ = ws.Close()
	}()

	// hello: server identity, the active run (if any), and the tree etag.
	var helloRun any
	if active := s.orch.Current(); active != nil {
		helloRun = map[string]any{
			"run_id":        active.RunID,
			"state":         active.State,
			"last_event_id": active.Log.LastID(),
		}
	}
	hello := marshalFrame("hello", "", map[string]any{
		"proto":          1,
		"server_version": s.opts.Version,
		"run":            helloRun,
		"tree_etag":      s.treeEtag(s.collectionPaths()),
	})
	if !conn.trySend(hello) {
		return
	}

	// Write pump: drains the hub channel; control pings keep the peer alive.
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		ticker := time.NewTicker(wsPingInterval)
		defer ticker.Stop()
		for {
			select {
			case frame, ok := <-conn.send:
				_ = ws.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if !ok {
					_ = ws.WriteMessage(websocket.CloseMessage, nil)
					return
				}
				if err := ws.WriteMessage(websocket.TextMessage, frame); err != nil {
					return
				}
			case <-ticker.C:
				_ = ws.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Read pump: subscribe + app-level ping.
	_ = ws.SetReadDeadline(time.Now().Add(wsReadDeadline))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(wsReadDeadline))
	})
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			break
		}
		_ = ws.SetReadDeadline(time.Now().Add(wsReadDeadline))
		var frame clientFrame
		if err := json.Unmarshal(msg, &frame); err != nil {
			conn.trySendOrDrop(marshalFrame("error", "", map[string]string{
				"code": "bad_subscribe", "message": "malformed client frame",
			}))
			continue
		}
		switch frame.Type {
		case "ping":
			conn.trySendOrDrop(marshalFrame("pong", "", map[string]any{}))
		case "subscribe":
			s.handleSubscribe(conn, frame)
		default:
			conn.trySendOrDrop(marshalFrame("error", "", map[string]string{
				"code": "bad_subscribe", "message": "unknown frame type " + frame.Type,
			}))
		}
	}
	<-writeDone
}

// trySendOrDrop sends without slow-consumer bookkeeping (read-pump replies).
func (c *hubConn) trySendOrDrop(frame []byte) { _ = c.trySend(frame) }

// handleSubscribe implements replay-from-event-id (spec §5.2): under the
// event-log lock, replay all entries with id > from_id directly into this
// connection, then atomically mark it subscribed so live broadcasts attach
// with no gap and no duplicate. Subscribing to a completed run replays the
// whole stored stream and then sends a terminal run.state.
func (s *Server) handleSubscribe(conn *hubConn, frame clientFrame) {
	if active := s.orch.Current(); active != nil && active.RunID == frame.RunID {
		active.Log.ReplayAfterLocked(frame.FromID, func(lines [][]byte) {
			for _, line := range lines {
				conn.trySendOrDrop(marshalRunEventFrame(frame.RunID, line))
			}
			conn.subscribed.Store(true)
		})
		return
	}
	// Completed: ring first, then the persisted store.
	if c := s.orch.FromRing(frame.RunID); c != nil {
		for _, line := range c.Log.ReplayAfter(frame.FromID) {
			conn.trySendOrDrop(marshalRunEventFrame(frame.RunID, line))
		}
		conn.subscribed.Store(true)
		conn.trySendOrDrop(marshalFrame("run.state", frame.RunID, map[string]any{
			"state": c.State, "exit_status": c.Meta.ExitStatus,
		}))
		return
	}
	if s.store != nil {
		if events, err := s.store.Events(frame.RunID); err == nil {
			meta, _ := s.store.Meta(frame.RunID)
			for _, line := range splitNDJSON(events) {
				conn.trySendOrDrop(marshalRunEventFrame(frame.RunID, line))
			}
			conn.subscribed.Store(true)
			state := "completed"
			exitStatus := ""
			if meta != nil {
				exitStatus = meta.ExitStatus
				if exitStatus == "cancelled" || exitStatus == "error" {
					state = exitStatus
				}
			}
			conn.trySendOrDrop(marshalFrame("run.state", frame.RunID, map[string]any{
				"state": state, "exit_status": exitStatus,
			}))
			return
		}
	}
	conn.trySendOrDrop(marshalFrame("error", "", map[string]string{
		"code": "bad_subscribe", "message": "unknown run " + frame.RunID,
	}))
}

// splitNDJSON splits stored NDJSON bytes into lines, skipping blanks.
func splitNDJSON(data []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				out = append(out, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}
