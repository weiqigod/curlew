package telemetry

import (
	"regexp"
	"testing"
)

// uuidRE matches the canonical 8-4-4-4-12 hex form of a UUID v4.
// The version nibble (position 14) is fixed to '4' and the variant nibble
// (position 19) is restricted to [89ab], matching the pattern used in
// cmd/apitest/telemetry_test.go.
var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUUIDv4(t *testing.T) {
	t.Run("format matches 8-4-4-4-12 hex", func(t *testing.T) {
		got, err := newUUIDv4()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !uuidRE.MatchString(got) {
			t.Errorf("UUID %q does not match expected format", got)
		}
	})

	t.Run("version nibble is 4", func(t *testing.T) {
		got, err := newUUIDv4()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Position 14 (0-indexed, after removing dashes) is the version nibble.
		// In the canonical form "xxxxxxxx-xxxx-Mxxx-..." M is at index 14.
		if len(got) < 19 {
			t.Fatalf("UUID too short: %q", got)
		}
		if got[14] != '4' {
			t.Errorf("version nibble at index 14 = %q, want '4'; UUID=%q", got[14], got)
		}
	})

	t.Run("variant bits are 10xx", func(t *testing.T) {
		got, err := newUUIDv4()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// First char of the 4th group (index 19, after "xxxxxxxx-xxxx-xxxx-") is variant nibble.
		// RFC-4122 variant: high two bits must be 10xx → hex digit is 8, 9, a, or b.
		if len(got) < 24 {
			t.Fatalf("UUID too short: %q", got)
		}
		variantNibble := got[19]
		switch variantNibble {
		case '8', '9', 'a', 'b':
			// correct
		default:
			t.Errorf("variant nibble at index 19 = %q, want one of 8/9/a/b; UUID=%q", variantNibble, got)
		}
	})

	t.Run("two consecutive calls differ", func(t *testing.T) {
		a, err := newUUIDv4()
		if err != nil {
			t.Fatalf("unexpected error on first call: %v", err)
		}
		b, err := newUUIDv4()
		if err != nil {
			t.Fatalf("unexpected error on second call: %v", err)
		}
		if a == b {
			t.Errorf("expected distinct UUIDs, got %q twice", a)
		}
	})
}

func TestNewSessionUUID(t *testing.T) {
	got, err := NewSessionUUID()
	if err != nil {
		t.Fatalf("NewSessionUUID: %v", err)
	}
	if !uuidRE.MatchString(got) {
		t.Errorf("NewSessionUUID %q does not match UUID format", got)
	}
}
