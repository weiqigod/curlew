package websocket

import (
	"context"
	"net/http"
	"time"

	gws "github.com/gorilla/websocket"
)

// Conn is the minimal connection interface required by the step executor.
// It is implemented by gorillaConn (wrapping *gws.Conn) in production and
// by fakes in tests.
type Conn interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, data []byte, err error)
	SetReadDeadline(t time.Time) error
	Close() error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	// SetPongHandler registers a callback invoked whenever the peer sends a pong
	// control frame. Gorilla/websocket calls the handler synchronously during ReadMessage.
	SetPongHandler(h func(appData string) error)
}

// Dialer abstracts WebSocket connection establishment so tests can inject a
// fake dialer that never touches the network.
type Dialer interface {
	Dial(ctx context.Context, url string, headers http.Header) (Conn, *http.Response, error)
}

// DefaultDialer is the package-level Dialer backed by gorilla/websocket.
var DefaultDialer Dialer = gorillaDialer{}

type gorillaDialer struct{}

func (gorillaDialer) Dial(ctx context.Context, url string, headers http.Header) (Conn, *http.Response, error) {
	d := *gws.DefaultDialer
	c, resp, err := d.DialContext(ctx, url, headers)
	if err != nil {
		return nil, resp, err
	}
	return &gorillaConn{c: c}, resp, nil
}

// gorillaConn adapts *gws.Conn to the Conn interface.
type gorillaConn struct {
	c *gws.Conn
}

func (g *gorillaConn) WriteMessage(messageType int, data []byte) error {
	return g.c.WriteMessage(messageType, data)
}

func (g *gorillaConn) ReadMessage() (int, []byte, error) {
	return g.c.ReadMessage()
}

func (g *gorillaConn) SetReadDeadline(t time.Time) error {
	return g.c.SetReadDeadline(t)
}

func (g *gorillaConn) Close() error {
	return g.c.Close()
}

func (g *gorillaConn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	return g.c.WriteControl(messageType, data, deadline)
}

// SetPongHandler registers h to be called when the peer sends a pong control frame.
func (g *gorillaConn) SetPongHandler(h func(string) error) {
	g.c.SetPongHandler(h)
}
