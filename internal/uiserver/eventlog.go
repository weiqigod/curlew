package uiserver

import (
	"bytes"
	"encoding/json"
	"sync"
)

// EventLog retains every emitted NDJSON line of a run in memory, indexed by
// the emitter's strictly monotonic event id, and notifies an optional
// broadcast callback per line. It implements io.Writer so it can sit directly
// under an events.Emitter. Memory bound: event bodies are 2 KiB-truncated, so
// a 1,000-request run is a few MB; the log is released when the run leaves
// the memory ring (spec §5.2).
type EventLog struct {
	mu      sync.RWMutex
	lines   [][]byte // raw NDJSON lines, without trailing newline
	ids     []int64  // ids[i] = event id of lines[i]
	lastID  int64
	onEvent func(id int64, line []byte) // called outside the lock is NOT safe; see Write
}

// NewEventLog constructs an EventLog. onEvent (may be nil) fires under the
// log lock for every appended line — the hub uses this to broadcast and the
// subscribe path replays under the same lock, guaranteeing no gap and no
// duplicate between replay and live attach.
func NewEventLog(onEvent func(id int64, line []byte)) *EventLog {
	return &EventLog{onEvent: onEvent}
}

// Write implements io.Writer for the events.Emitter: each write is one
// complete NDJSON line (the emitter writes line-atomically under its mutex).
func (l *EventLog) Write(p []byte) (int, error) {
	line := bytes.TrimRight(p, "\n")
	stored := make([]byte, len(line))
	copy(stored, line)

	var header struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(stored, &header)

	l.mu.Lock()
	l.lines = append(l.lines, stored)
	l.ids = append(l.ids, header.ID)
	if header.ID > l.lastID {
		l.lastID = header.ID
	}
	cb := l.onEvent
	if cb != nil {
		cb(header.ID, stored)
	}
	l.mu.Unlock()
	return len(p), nil
}

// LastID returns the highest event id seen.
func (l *EventLog) LastID() int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.lastID
}

// ReplayAfter returns copies of all lines with id > fromID, in order.
func (l *EventLog) ReplayAfter(fromID int64) [][]byte {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var out [][]byte
	for i, id := range l.ids {
		if id > fromID {
			out = append(out, l.lines[i])
		}
	}
	return out
}

// ReplayAfterLocked runs fn under the log's write lock with all lines whose
// id > fromID; while fn runs no new line can be appended or broadcast. The
// subscribe path uses this to replay and attach atomically (spec §5.2).
func (l *EventLog) ReplayAfterLocked(fromID int64, fn func(lines [][]byte)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out [][]byte
	for i, id := range l.ids {
		if id > fromID {
			out = append(out, l.lines[i])
		}
	}
	fn(out)
}

// Lines returns copies of all stored lines in order (for persistence).
func (l *EventLog) Lines() [][]byte {
	return l.ReplayAfter(0)
}
