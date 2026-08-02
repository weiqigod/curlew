package uiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/peterlindqvist/apitest/internal/httpexec"
	"github.com/peterlindqvist/apitest/internal/uiserver"
)

type frame struct {
	Type  string          `json:"type"`
	RunID string          `json:"run_id"`
	Data  json.RawMessage `json:"data"`
}

func dialWS(t *testing.T, ts *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws?token=" + testToken
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	return ws
}

func readFrame(t *testing.T, ws *websocket.Conn) frame {
	t.Helper()
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var f frame
	if err := json.Unmarshal(msg, &f); err != nil {
		t.Fatalf("unmarshal %s: %v", msg, err)
	}
	return f
}

func TestWS_HelloAndPing(t *testing.T) {
	ts, _ := newTestServer(t)
	ws := dialWS(t, ts)

	hello := readFrame(t, ws)
	if hello.Type != "hello" {
		t.Fatalf("first frame = %s, want hello", hello.Type)
	}
	var data struct {
		Proto         int    `json:"proto"`
		ServerVersion string `json:"server_version"`
		Run           *any   `json:"run"`
		TreeEtag      string `json:"tree_etag"`
	}
	if err := json.Unmarshal(hello.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Proto != 1 || data.TreeEtag == "" {
		t.Errorf("hello data = %+v", data)
	}
	if data.Run != nil {
		t.Errorf("hello run = %v, want null when idle", data.Run)
	}

	if err := ws.WriteJSON(map[string]string{"type": "ping"}); err != nil {
		t.Fatal(err)
	}
	pong := readFrame(t, ws)
	if pong.Type != "pong" {
		t.Errorf("reply = %s, want pong", pong.Type)
	}
}

func TestWS_TokenRequired(t *testing.T) {
	ts, _ := newTestServer(t)
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
	_, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		t.Fatal("dial succeeded without token")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %v, want 401", resp)
	}
}

func TestWS_OriginRejected(t *testing.T) {
	ts, _ := newTestServer(t)
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws?token=" + testToken
	header := http.Header{"Origin": []string{"https://evil.example.com"}}
	_, resp, err := websocket.DefaultDialer.Dial(url, header)
	if err == nil {
		t.Fatal("dial succeeded with evil origin")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %v, want 403", resp)
	}
}

// TestWS_LiveRunStreamAndReplay drives a full run over WS: subscribe from 0,
// receive verbatim events, then a terminal run.state; a second client
// replays the completed stream gaplessly.
func TestWS_LiveRunStreamAndReplay(t *testing.T) {
	release := make(chan struct{})
	gateExec := func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		<-release
		return &httpexec.Result{StatusCode: 200, Body: []byte(`{"ok":true}`)}, nil
	}
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = gateExec })
	writeFile(t, root, "collections/ok.yaml", `name: OK
requests:
  - name: One
    request: {method: GET, url: "http://t.test/1"}
  - name: Two
    request: {method: GET, url: "http://t.test/2"}
`)

	ws := dialWS(t, ts)
	if f := readFrame(t, ws); f.Type != "hello" {
		t.Fatalf("want hello, got %s", f.Type)
	}

	runID := startRun(t, ts, map[string]any{"collection": "collections/ok.yaml"})
	if err := ws.WriteJSON(map[string]any{"type": "subscribe", "run_id": runID, "from_id": 0}); err != nil {
		t.Fatal(err)
	}

	var kinds []string
	var lastEventID int64
	released := false
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		f := readFrame(t, ws)
		switch f.Type {
		case "run.event":
			var ev struct {
				Kind string `json:"kind"`
				ID   int64  `json:"id"`
			}
			_ = json.Unmarshal(f.Data, &ev)
			kinds = append(kinds, ev.Kind)
			if ev.ID != lastEventID+1 {
				t.Fatalf("event id gap: %d after %d", ev.ID, lastEventID)
			}
			lastEventID = ev.ID
			if f.RunID != runID {
				t.Errorf("frame run_id = %s", f.RunID)
			}
			// Release the gated exec only once the subscription is live
			// (run.start replayed) — pins the replay→live attach as gapless.
			if !released {
				released = true
				close(release)
			}
		case "run.state":
			var st struct {
				State      string `json:"state"`
				ExitStatus string `json:"exit_status"`
			}
			_ = json.Unmarshal(f.Data, &st)
			if st.State != "completed" || st.ExitStatus != "passed" {
				t.Errorf("terminal state = %+v", st)
			}
			goto done
		}
	}
	t.Fatal("no terminal run.state within deadline")
done:
	joined := strings.Join(kinds, ",")
	if !strings.HasPrefix(joined, "run.start") || !strings.HasSuffix(joined, "run.end") {
		t.Errorf("event kinds = %v", kinds)
	}
	if !strings.Contains(joined, "request.start") || !strings.Contains(joined, "request.end") {
		t.Errorf("missing request events: %v", kinds)
	}

	// A fresh tab subscribing to the completed ring run replays the whole
	// stream then sends a terminal run.state (spec §5.2).
	ws2 := dialWS(t, ts)
	if f := readFrame(t, ws2); f.Type != "hello" {
		t.Fatalf("want hello, got %s", f.Type)
	}
	if err := ws2.WriteJSON(map[string]any{"type": "subscribe", "run_id": runID, "from_id": 0}); err != nil {
		t.Fatal(err)
	}
	var replayCount int
	for {
		f := readFrame(t, ws2)
		if f.Type == "run.event" {
			replayCount++
			continue
		}
		if f.Type == "run.state" {
			break
		}
		t.Fatalf("unexpected frame %s", f.Type)
	}
	if int64(replayCount) != lastEventID {
		t.Errorf("replayed %d events, want %d", replayCount, lastEventID)
	}

	// Partial replay from a mid-stream id yields only the tail.
	ws3 := dialWS(t, ts)
	_ = readFrame(t, ws3) // hello
	if err := ws3.WriteJSON(map[string]any{"type": "subscribe", "run_id": runID, "from_id": lastEventID - 2}); err != nil {
		t.Fatal(err)
	}
	tail := 0
	for {
		f := readFrame(t, ws3)
		if f.Type == "run.event" {
			tail++
			continue
		}
		break
	}
	if tail != 2 {
		t.Errorf("partial replay = %d events, want 2", tail)
	}
}

func TestWS_SubscribeUnknownRun(t *testing.T) {
	ts, _ := newTestServer(t)
	ws := dialWS(t, ts)
	_ = readFrame(t, ws) // hello
	if err := ws.WriteJSON(map[string]any{"type": "subscribe", "run_id": "deadbeef", "from_id": 0}); err != nil {
		t.Fatal(err)
	}
	f := readFrame(t, ws)
	if f.Type != "error" {
		t.Fatalf("frame = %s, want error", f.Type)
	}
	var data struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(f.Data, &data)
	if data.Code != "bad_subscribe" {
		t.Errorf("code = %s", data.Code)
	}
}
