package output

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestIsTerminal(t *testing.T) {
	tests := []struct {
		name string
		w    io.Writer
		want bool
	}{
		{"bytes.Buffer is not terminal", &bytes.Buffer{}, false},
		{"strings.Builder is not terminal", &strings.Builder{}, false},
		{"nil-type writer is not terminal", (*bytes.Buffer)(nil), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTerminal(tt.w)
			if got != tt.want {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.want)
			}
		})
	}

	// Regular file is *os.File, Stat() succeeds, but ModeCharDevice is not set → false
	t.Run("regular *os.File is not a char device", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "isterm")
		if err != nil {
			t.Fatalf("os.CreateTemp: %v", err)
		}
		t.Cleanup(func() { _ = f.Close() })
		got := IsTerminal(f)
		if got {
			t.Error("IsTerminal(regular file) = true, want false")
		}
	})

	// Closed file triggers f.Stat() error path → false
	t.Run("closed *os.File returns false (Stat error)", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "isterm")
		if err != nil {
			t.Fatalf("os.CreateTemp: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("f.Close: %v", err)
		}
		got := IsTerminal(f)
		if got {
			t.Error("IsTerminal(closed file) = true, want false")
		}
	})
}

func TestColorize(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		code    string
		enabled bool
		want    string
	}{
		{"enabled wraps with code and reset", "hello", ansiGreen, true, "\033[32mhello\033[0m"},
		{"disabled returns unchanged", "hello", ansiGreen, false, "hello"},
		{"empty string with enabled returns empty", "", ansiGreen, true, ""},
		{"empty string with disabled returns empty", "", ansiGreen, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := colorize(tt.s, tt.code, tt.enabled)
			if got != tt.want {
				t.Errorf("colorize() = %q, want %q", got, tt.want)
			}
		})
	}
}
