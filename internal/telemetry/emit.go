package telemetry

import (
	"context"
	"time"
)

// Emitter is the high-level entry point. Wraps Store and Client to provide
// the "silent failure + record in ring buffer" emit policy.
type Emitter struct {
	store  *Store
	client *Client
}

// NewEmitter builds an Emitter from a pre-constructed Store and Client.
// This is the preferred constructor for cmd/curlew which resolves the
// endpoint before building the Client.
func NewEmitter(store *Store, client *Client) *Emitter {
	return &Emitter{store: store, client: client}
}

// Emit fires a single event when telemetry is enabled. When disabled or when
// install_id is absent, returns immediately without contacting the network.
// All errors are recorded to the ring buffer; callers MUST NOT surface them.
func (e *Emitter) Emit(ctx context.Context, eventType string, payload map[string]any) {
	enabled, installID, _, err := e.store.Status()
	if err != nil || !enabled {
		_ = e.store.RecordEmission(Emission{
			At:        time.Now().UTC().Format(time.RFC3339),
			EventType: eventType,
			Status:    "disabled",
		})
		return
	}
	if emitErr := e.client.Emit(ctx, installID, eventType, payload); emitErr != nil {
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
