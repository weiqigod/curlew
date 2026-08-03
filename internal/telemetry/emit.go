package telemetry

import (
	"time"
)

// Emitter is the high-level entry point. Wraps Store and FileSink to provide
// the "silent failure + record in ring buffer" emit policy.
type Emitter struct {
	store *Store
	sink  *FileSink
}

// NewEmitter builds an Emitter from a pre-constructed Store and FileSink.
// This is the preferred constructor for cmd/curlew which resolves the events
// file before building the sink.
func NewEmitter(store *Store, sink *FileSink) *Emitter {
	return &Emitter{store: store, sink: sink}
}

// Emit records a single event when telemetry is enabled. When disabled or when
// install_id is absent, it returns immediately without touching the file.
// All errors are recorded to the ring buffer; callers MUST NOT surface them.
func (e *Emitter) Emit(eventType string, payload map[string]any) {
	enabled, installID, _, err := e.store.Status()
	if err != nil || !enabled {
		_ = e.store.RecordEmission(Emission{
			At:        time.Now().UTC().Format(time.RFC3339),
			EventType: eventType,
			Status:    "disabled",
		})
		return
	}
	if emitErr := e.sink.Emit(installID, eventType, payload); emitErr != nil {
		_ = e.store.RecordEmission(Emission{
			At:        time.Now().UTC().Format(time.RFC3339),
			EventType: eventType,
			Status:    "error",
			Error:     emitErr.Error(),
		})
		return
	}
	_ = e.store.RecordEmission(Emission{
		At:        time.Now().UTC().Format(time.RFC3339),
		EventType: eventType,
		Status:    "ok",
	})
}
