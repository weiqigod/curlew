package websocket

import "sync"

// pumpFrameBuffer is how many frames the pump may run ahead of the step loop.
// Beyond this the pump blocks, which applies backpressure to a chatty server
// rather than growing without bound; unmatched frames that the step loop does
// take are held by messageBuffer, which has its own warning threshold.
const pumpFrameBuffer = 32

// pumpFrame is one result from the connection's sole reader: either a message
// or the error that ended the stream.
type pumpFrame struct {
	data []byte
	err  error
}

// readPump owns every read on a WebSocket connection.
//
// It exists because of two gorilla properties that combine badly. Control
// frames — including the pongs a heartbeat depends on — are dispatched only
// from inside ReadMessage, so nothing answers a ping while no one is reading.
// And a read error is permanent: once a read fails, the connection is marked
// failed and further reads return the same error, panicking with "repeated read
// on failed websocket connection" if a caller keeps trying.
//
// Together those made read deadlines unusable for keeping a connection alive. A
// step that read with a deadline in order to dispatch pongs destroyed the
// connection the moment that deadline fired, poisoning every later step; a step
// that slept instead never dispatched anything and the heartbeat declared a
// healthy peer dead (§11C.5).
//
// So the pump never sets a deadline. It blocks in ReadMessage for the life of
// the connection — which is exactly where control frames get dispatched — and
// steps take frames from a channel, bounding their own waits with timers. The
// pump is the only reader, so the frames never interleave.
//
// The pump is unblocked by closing the connection, which the executor does on
// return.
type readPump struct {
	frames   chan pumpFrame
	done     chan struct{}
	stopOnce sync.Once
}

// startReadPump begins reading conn on its own goroutine. Every read on conn
// must go through the returned pump from this point on.
func startReadPump(conn Conn) *readPump {
	p := &readPump{
		frames: make(chan pumpFrame, pumpFrameBuffer),
		done:   make(chan struct{}),
	}
	go func() {
		defer close(p.frames)
		for {
			_, data, err := conn.ReadMessage()
			select {
			case p.frames <- pumpFrame{data: data, err: err}:
			case <-p.done:
				return
			}
			if err != nil {
				// A gorilla read error is terminal; reading again would return
				// the same error forever and eventually panic.
				return
			}
		}
	}()
	return p
}

// stop releases the pump goroutine. A read already in flight is not
// interrupted — only closing the connection does that — but the goroutine will
// exit rather than block forever trying to deliver its result. Safe to call
// more than once.
func (p *readPump) stop() {
	p.stopOnce.Do(func() { close(p.done) })
}
