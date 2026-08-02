package markdown

import (
	"testing"
)

// pngHeader is the 8-byte PNG magic number used in classifier tests.
var pngHeader = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

// TestMarkdown_ContentType verifies that Classify returns the correct Kind
// when the Content-Type header is present and parseable.
func TestMarkdown_ContentType(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		method      string
		contentType string
		want        Kind
	}{
		{"head wins over body", []byte("ignored"), "HEAD", "application/json", KindHEAD},
		{"head case-insensitive", []byte("ignored"), "head", "", KindHEAD},
		{"empty body", []byte{}, "GET", "application/json", KindEmpty},
		{"application/json", []byte(`{"a":1}`), "GET", "application/json", KindJSON},
		{"vendor +json", []byte(`{}`), "GET", "application/vnd.github.v3+json", KindJSON},
		{"json with charset", []byte(`{}`), "GET", "application/json; charset=utf-8", KindJSON},
		{"application/yaml", []byte("a: 1\n"), "GET", "application/yaml", KindYAML},
		{"text/yaml", []byte("a: 1\n"), "GET", "text/yaml", KindYAML},
		{"application/xml", []byte("<root/>"), "GET", "application/xml", KindXML},
		{"text/xml", []byte("<root/>"), "GET", "text/xml", KindXML},
		{"vendor +xml", []byte("<x/>"), "GET", "application/atom+xml", KindXML},
		{"text/html", []byte("<html></html>"), "GET", "text/html", KindHTML},
		{"text/plain", []byte("hello"), "GET", "text/plain", KindText},
		{"text/markdown", []byte("# hi"), "GET", "text/markdown", KindText},
		{"image/png", pngHeader, "GET", "image/png", KindBinary},
		{"octet-stream nul", []byte{0x00, 0x01}, "GET", "application/octet-stream", KindBinary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.body, tt.method, tt.contentType)
			if got != tt.want {
				t.Errorf("Classify() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMarkdown_ContentType_FallbackSniff verifies that Classify falls back to
// binary detection + JSON-sniff when Content-Type is missing or unparseable.
func TestMarkdown_ContentType_FallbackSniff(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		contentType string
		want        Kind
	}{
		{"no header json object", []byte(`{"a":1}`), "", KindJSON},
		{"no header json array", []byte(`[1,2,3]`), "", KindJSON},
		{"no header json with leading whitespace", []byte("  {\"a\":1}"), "", KindJSON},
		{"no header binary nul", []byte{0x00, 0xff}, "", KindBinary},
		{"no header invalid utf8", []byte{0xff, 0xfe}, "", KindBinary},
		{"no header plain text", []byte("hello world"), "", KindText},
		{"unparseable header text", []byte("hi"), "???", KindText},
		{"unparseable header json", []byte("{}"), "!!!", KindJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.body, "GET", tt.contentType)
			if got != tt.want {
				t.Errorf("Classify(%q, %q) = %v, want %v", tt.body, tt.contentType, got, tt.want)
			}
		})
	}
}
