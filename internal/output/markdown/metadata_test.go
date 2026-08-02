package markdown

import (
	"bytes"
	"net/http"
	"testing"
)

// TestMarkdown_VolatileHeaderSet verifies that IsVolatileHeader returns true
// for the closed set of volatile headers (case-insensitive) and false for others.
func TestMarkdown_VolatileHeaderSet(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"Date", "Date", true},
		{"date lowercase", "date", true},
		{"DATE upper", "DATE", true},
		{"X-Request-ID", "X-Request-ID", true},
		{"x-request-id lowercase", "x-request-id", true},
		{"Set-Cookie", "Set-Cookie", true},
		{"set-cookie lowercase", "set-cookie", true},
		{"ETag canonical", "ETag", true},
		{"etag lowercase", "etag", true},
		{"Server", "Server", true},
		{"Age", "Age", true},
		{"Content-Type not volatile", "Content-Type", false},
		{"Content-Length not volatile", "Content-Length", false},
		{"Authorization not volatile", "Authorization", false},
		{"X-Custom not volatile", "X-Custom-Header", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsVolatileHeader(tt.in); got != tt.want {
				t.Errorf("IsVolatileHeader(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestMarkdown_RenderMetadata verifies that renderMetadata emits headers in
// alphabetical order.
func TestMarkdown_RenderMetadata(t *testing.T) {
	h := http.Header{
		"Content-Type":   {"application/json"},
		"Content-Length": {"42"},
		"Cache-Control":  {"no-cache"},
		"X-Trace-Id":     {"abc"},
	}
	var buf bytes.Buffer
	renderMetadata(&buf, h, 42)
	got := buf.String()
	// Sorted: Cache-Control, Content-Length, Content-Type, X-Trace-Id
	want := "Cache-Control: no-cache\nContent-Length: 42\nContent-Type: application/json\nX-Trace-Id: abc\n"
	if got != want {
		t.Errorf("renderMetadata mismatch:\n got: %q\nwant: %q", got, want)
	}
}
