package websocket

import "fmt"

const bufferWarnThreshold = 100

// messageBuffer is an unbounded FIFO queue of frames received on a WebSocket
// connection that are awaiting consumption by an expect step. It is not safe
// for concurrent use; the executor owns it on the step-loop goroutine.
type messageBuffer struct {
	frames  [][]byte
	warned  bool   // true once the warn threshold has been crossed
	warning string // pending one-shot warning, cleared by drainWarning
}

// push appends a copy of data to the buffer. If this push crosses the warn
// threshold for the first time, it records a one-shot warning string.
func (b *messageBuffer) push(data []byte) {
	cpy := make([]byte, len(data))
	copy(cpy, data)
	b.frames = append(b.frames, cpy)
	if !b.warned && len(b.frames) > bufferWarnThreshold {
		b.warned = true
		b.warning = fmt.Sprintf(
			"websocket message buffer exceeded %d frames (current %d); "+
				"unmatched server messages are accumulating",
			bufferWarnThreshold, len(b.frames))
	}
}

// takeMatch removes and returns the first buffered frame that satisfies match.
// Returns (-1, nil) if no buffered frame matches.
func (b *messageBuffer) takeMatch(match func([]byte) bool) (int, []byte) {
	for i, f := range b.frames {
		if match(f) {
			b.frames = append(b.frames[:i], b.frames[i+1:]...)
			return i, f
		}
	}
	return -1, nil
}

// len returns the number of buffered frames.
func (b *messageBuffer) len() int { return len(b.frames) }

// drainWarning returns and clears the pending one-shot warning string.
// Returns an empty string if no warning is pending.
func (b *messageBuffer) drainWarning() string {
	w := b.warning
	b.warning = ""
	return w
}
