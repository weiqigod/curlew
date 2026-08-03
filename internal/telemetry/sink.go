package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileSink appends telemetry events to a local NDJSON file — one JSON object
// per line. Curlew has no telemetry backend: events never leave the machine,
// and the file is the whole delivery mechanism.
type FileSink struct {
	path string
}

// NewFileSink returns a sink that appends events to path. The file and its
// parent directory are created on first write.
func NewFileSink(path string) *FileSink {
	return &FileSink{path: path}
}

// Path returns the file events are appended to.
func (s *FileSink) Path() string {
	return s.path
}

// Emit appends one event. A nil payload is written as an empty object so every
// record has the same shape. Errors are returned so callers can record them in
// the ring buffer; callers MUST NOT surface them to the user.
func (s *FileSink) Emit(installID, eventType string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	line, err := json.Marshal(map[string]any{
		"at":            time.Now().UTC().Format(time.RFC3339),
		"install_id":    installID,
		"event_type":    eventType,
		"event_payload": payload,
	})
	if err != nil {
		return fmt.Errorf("telemetry: marshal: %w", err)
	}
	if mkErr := os.MkdirAll(filepath.Dir(s.path), 0o700); mkErr != nil {
		return fmt.Errorf("telemetry: create events dir: %w", mkErr)
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // path is operator-controlled
	if err != nil {
		return fmt.Errorf("telemetry: open events file: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("telemetry: append event: %w", err)
	}
	return nil
}
