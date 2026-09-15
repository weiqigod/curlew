package mudflat

import (
	"bytes"
	"fmt"
	"net"
	"runtime"
	"sync"
	"testing"
)

// Yield between socket writes to expose a control pump inserting a frame
// between a foreground frame's header and payload. The recording connection
// itself is safe for concurrent calls, just like net.Conn.
type yieldingFrameConn struct {
	net.Conn
	mu   sync.Mutex
	wire bytes.Buffer
}

func (c *yieldingFrameConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	n, err := c.wire.Write(p)
	c.mu.Unlock()
	runtime.Gosched()
	return n, err
}

func TestWS_ConcurrentFramesRemainWhole(t *testing.T) {
	socket := &yieldingFrameConn{}
	conn := &wsConn{conn: socket}
	var wg sync.WaitGroup
	const count = 64
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := conn.WriteText(fmt.Sprintf("%032d", i)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	wire := socket.wire.Bytes()
	if len(wire) != count*34 {
		t.Fatalf("wire length = %d", len(wire))
	}
	seen := map[string]bool{}
	for offset := 0; offset < len(wire); offset += 34 {
		frame := wire[offset : offset+34]
		if frame[0] != 0x81 || frame[1] != 32 {
			t.Fatalf("frame boundary corrupted at %d: %x", offset, frame)
		}
		payload := string(frame[2:])
		var index int
		if _, err := fmt.Sscanf(payload, "%d", &index); err != nil || index < 0 || index >= count || payload != fmt.Sprintf("%032d", index) || seen[payload] {
			t.Fatalf("interleaved or duplicate frame payload at %d: %q", offset, payload)
		}
		seen[payload] = true
	}
}
