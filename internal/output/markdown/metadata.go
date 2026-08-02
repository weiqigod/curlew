package markdown

import (
	"bytes"
	"cmp"
	"fmt"
	"net/http"
	"slices"
)

// volatileHeaders is the closed set of response headers that are filtered
// out of the request and response signal blocks and rendered exclusively
// inside the ### Response metadata subsection. Keys are stored in their
// http.CanonicalHeaderKey form.
//
// Adding to this set is a behaviour change; do not extend without a
// corresponding task and golden update.
var volatileHeaders = map[string]struct{}{
	"Date":         {},
	"X-Request-Id": {}, // http.CanonicalHeaderKey("X-Request-ID") => "X-Request-Id"
	"Set-Cookie":   {},
	"Etag":         {}, // http.CanonicalHeaderKey("ETag") => "Etag"
	"Server":       {},
	"Age":          {},
}

// IsVolatileHeader reports whether name (case-insensitive) is in the
// volatile-header set.
func IsVolatileHeader(name string) bool {
	_, ok := volatileHeaders[http.CanonicalHeaderKey(name)]
	return ok
}

// renderMetadata writes the body of the ### Response metadata subsection.
// Headers are emitted in alphabetical order. All header values are included.
//
// Empty headers map renders nothing.
func renderMetadata(w *bytes.Buffer, h http.Header, _ int) {
	if len(h) == 0 {
		return
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, cmp.Compare)
	for _, k := range keys {
		for _, v := range h[k] {
			fmt.Fprintf(w, "%s: %s\n", k, v)
		}
	}
}
