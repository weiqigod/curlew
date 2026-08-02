// Package ids mints 32-char lowercase hex identifiers used as run IDs across
// apitest's output surfaces (events stream, exec --log, markdown sentinels).
//
// The events emitter, runner, and exec command all delegate to NewRunID so a
// single invocation produces correlatable identifiers in every artifact.
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// randRead is the random byte source used by NewRunID. It is a package-level
// variable so tests can substitute a failing reader to exercise the fallback
// branch without requiring crypto/rand to actually fail.
var randRead = func(b []byte) (int, error) {
	return rand.Read(b)
}

// NewRunID returns a 32-char lowercase hex identifier (128 bits of crypto/rand
// entropy). On the never-hit failure path where crypto/rand cannot be read,
// it falls back to a time-based hex string of the form "fallback%016x" so the
// returned value is still safely matchable as identifier-shaped text.
func NewRunID() string {
	var b [16]byte
	if _, err := randRead(b[:]); err != nil {
		return fmt.Sprintf("fallback%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
