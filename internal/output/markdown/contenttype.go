package markdown

import (
	"mime"
	"strings"
	"unicode/utf8"
)

// Kind classifies a response body for markdown rendering.
type Kind int

const (
	KindEmpty  Kind = iota // len(body) == 0 (and method != HEAD)
	KindHEAD               // request method == HEAD; body suppressed
	KindJSON               // application/json or *+json (or sniffed)
	KindYAML               // application/yaml or text/yaml (or *+yaml)
	KindXML                // application/xml, text/xml, or *+xml
	KindHTML               // text/html
	KindText               // any other text/* (after html, xml, yaml carved out)
	KindBinary             // isBinary heuristic + UTF-8 fallback
)

// Classify returns the Kind to use when rendering body in markdown.
//
// Method is checked first: HEAD always returns KindHEAD regardless of
// body or Content-Type, matching HTTP semantics where a HEAD response's
// body is by definition absent.
//
// Empty bodies (len 0) return KindEmpty before any header parsing.
//
// Otherwise the Content-Type header is the primary signal; mime.ParseMediaType
// extracts the media type and the subtype suffix (+json, +xml, +yaml).
// Missing or unparseable Content-Type falls back to: binary heuristic,
// then JSON-sniff (first non-whitespace byte is { or [), then text.
func Classify(body []byte, method, contentType string) Kind {
	if strings.EqualFold(method, "HEAD") {
		return KindHEAD
	}
	if len(body) == 0 {
		return KindEmpty
	}
	if mt, _, err := mime.ParseMediaType(contentType); err == nil {
		switch {
		case mt == "application/json" || strings.HasSuffix(mt, "+json"):
			return KindJSON
		case mt == "application/yaml" || mt == "text/yaml" || strings.HasSuffix(mt, "+yaml"):
			return KindYAML
		case mt == "application/xml" || mt == "text/xml" || strings.HasSuffix(mt, "+xml"):
			return KindXML
		case mt == "text/html":
			return KindHTML
		case strings.HasPrefix(mt, "text/"):
			return KindText
		}
		// Other media types (application/octet-stream, image/*, multipart/*)
		// fall through to the binary heuristic.
	}
	// Header missing or unparseable; or media type is non-text/non-structured.
	if isBinary(body) {
		return KindBinary
	}
	if jsonSniff(body) {
		return KindJSON
	}
	return KindText
}

// isBinary mirrors the heuristic at internal/output/events/emitter.go:342;
// duplicated rather than extracted because the events package owns its own
// classification rules and we should not couple the two layers.
func isBinary(raw []byte) bool {
	for _, b := range raw {
		if b == 0x00 {
			return true
		}
	}
	return !utf8.Valid(raw)
}

// jsonSniff returns true when the first non-whitespace byte is { or [.
// Used only when Content-Type is missing/unparseable.
func jsonSniff(body []byte) bool {
	for _, b := range body {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '{', '[':
			return true
		default:
			return false
		}
	}
	return false
}
