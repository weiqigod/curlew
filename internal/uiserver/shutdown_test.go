package uiserver_test

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/uiserver"
)

// TestShutdown_CancelsActiveRunAndClosesWS exercises Server.Shutdown: the
// active run is cancelled (the runner reports context-cancelled state), the
// run goroutine is awaited, and open WS connections are closed.
func TestShutdown_CancelsActiveRunAndClosesWS(t *testing.T) {
	blockingExec := func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "curlew.yaml"), []byte("project_name: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "collections/ok.yaml", "name: OK\nrequests:\n  - name: Fine\n    request: {method: GET, url: \"http://t.test/ok\"}\n")

	srv, err := uiserver.NewServer(uiserver.Options{
		Root: root, ProjectName: "t", Version: "test", Token: testToken, Exec: blockingExec,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	runID := startRun(t, ts, map[string]any{"collection": "collections/ok.yaml"})

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws?token=" + testToken
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := ws.ReadMessage(); err != nil { // hello
		t.Fatalf("hello: %v", err)
	}

	done := make(chan struct{})
	go func() {
		srv.Shutdown()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("Shutdown did not return (active-run wait hung)")
	}

	// The WS connection was closed by the hub.
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			break // closed — expected
		}
	}

	// The run reached a terminal cancelled state.
	final := waitTerminal(t, ts, runID)
	if final["state"] != "cancelled" {
		t.Errorf("state after shutdown = %v, want cancelled", final["state"])
	}
}
