// Package ids — white-box tests for the crypto/rand failure fallback path.
// These tests live in package ids (not ids_test) so they can inject randRead.
package ids

import (
	"errors"
	"strings"
	"testing"
)

// TestNewRunID_FallbackOnRandError exercises the never-hit production path
// where crypto/rand fails, ensuring the fallback return value is non-empty
// and has the expected "fallback" prefix shape.
func TestNewRunID_FallbackOnRandError(t *testing.T) {
	// Inject a failing reader.
	orig := randRead
	randRead = func(b []byte) (int, error) {
		return 0, errors.New("injected rand failure")
	}
	t.Cleanup(func() { randRead = orig })

	id := NewRunID()
	if id == "" {
		t.Fatal("fallback id must not be empty")
	}
	if !strings.HasPrefix(id, "fallback") {
		t.Errorf("fallback id %q should start with \"fallback\"", id)
	}
}
