package markdown

import (
	"bytes"
	"testing"
)

// TestMarkdown_BodyCap verifies truncateForMarkdown boundary conditions.
func TestMarkdown_BodyCap(t *testing.T) {
	tests := []struct {
		name          string
		in            []byte
		cap           int
		wantOut       []byte
		wantTruncated bool
		wantOriginal  int
	}{
		{"empty", []byte{}, 1024, []byte{}, false, 0},
		{"under cap", []byte("hello"), 1024, []byte("hello"), false, 5},
		{"at cap", bytes.Repeat([]byte("x"), 1024), 1024, bytes.Repeat([]byte("x"), 1024), false, 1024},
		{"over cap", bytes.Repeat([]byte("x"), 1025), 1024, bytes.Repeat([]byte("x"), 1024), true, 1025},
		{"1 MiB cap with 2 MiB body", bytes.Repeat([]byte("y"), 2<<20), 1 << 20, bytes.Repeat([]byte("y"), 1<<20), true, 2 << 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, truncated, original := truncateForMarkdown(tt.in, tt.cap)
			if !bytes.Equal(out, tt.wantOut) {
				t.Errorf("out mismatch: got len=%d, want len=%d", len(out), len(tt.wantOut))
			}
			if truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", truncated, tt.wantTruncated)
			}
			if original != tt.wantOriginal {
				t.Errorf("original = %d, want %d", original, tt.wantOriginal)
			}
		})
	}
}

// TestMarkdown_BodyCap_PostRedaction proves that redaction shrinking a body
// below the cap does not trigger truncation — only post-redaction size matters.
func TestMarkdown_BodyCap_PostRedaction(t *testing.T) {
	// 2 MiB raw input, redacted to a small slice by the time it reaches the cap helper.
	redactedSmall := []byte("[REDACTED] secret content here, originally 2 MiB on the wire")
	out, trunc, _ := truncateForMarkdown(redactedSmall, BodyCapBytes)
	if trunc {
		t.Error("redaction-shrunk body should not be truncated")
	}
	if !bytes.Equal(out, redactedSmall) {
		t.Error("expected unchanged bytes for sub-cap input")
	}

	// 2 MiB redacted body still over cap: truncate.
	big := bytes.Repeat([]byte("x"), 2<<20)
	out2, trunc2, orig := truncateForMarkdown(big, BodyCapBytes)
	if !trunc2 {
		t.Error("over-cap body should be truncated")
	}
	if len(out2) != 1<<20 || orig != 2<<20 {
		t.Errorf("unexpected sizes: out=%d original=%d", len(out2), orig)
	}
}
