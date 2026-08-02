// Package telemetry provides the CLI-side telemetry emitter: persistent
// install_id management, fire-and-forget event posting, and the
// `curlew telemetry` subcommand handlers.
//
// Per v4-11, this package has its own HTTP client and MUST NOT import
// internal/backend.
package telemetry

import (
	"crypto/rand"
	"fmt"
)

// newUUIDv4 returns a freshly-generated UUID v4 string in canonical 8-4-4-4-12
// form. Mirrors internal/variable/dynamic.go:makeUUID; lives here so
// internal/telemetry does not depend on internal/variable.
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("telemetry: read random bytes: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// NewSessionUUID returns a freshly-generated UUID v4 for use as a per-execution
// session identifier. It is exported for use from cmd/curlew.
func NewSessionUUID() (string, error) {
	return newUUIDv4()
}
