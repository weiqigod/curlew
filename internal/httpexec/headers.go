package httpexec

import (
	"net/http"
	"sort"
	"strings"
)

// goManagedHeaders are headers Go's HTTP client reads or writes itself under
// their canonical names. They are set in canonical form so the client sees
// them and does not send its own copy as well.
var goManagedHeaders = map[string]bool{
	"Host":              true,
	"Content-Length":    true,
	"Transfer-Encoding": true,
	"Trailer":           true,
	"Connection":        true,
	"Accept-Encoding":   true,
	"Range":             true,
	"Expect":            true,
}

// setRequestHeaders copies the request's headers into h with their names
// spelled as written, rather than canonicalised by Header.Set: `x-tenant` is
// sent as x-tenant. Names that differ only in case are one header; when the
// map holds more than one spelling, the last in sorted order is sent.
func setRequestHeaders(h http.Header, headers map[string]string) {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := headers[name]
		canonical := http.CanonicalHeaderKey(name)
		if goManagedHeaders[canonical] {
			h.Set(canonical, value)
			continue
		}
		for existing := range h {
			if strings.EqualFold(existing, name) {
				delete(h, existing)
			}
		}
		h[name] = []string{value}
		if canonical == "User-Agent" && name != canonical {
			// Go's client sends its own User-Agent unless the canonical key is
			// present; an empty value there suppresses it, and the request's
			// own spelling is written instead.
			h[canonical] = []string{""}
		}
	}
}

// HasHeader reports whether headers sets name, compared ignoring case.
func HasHeader(headers map[string]string, name string) bool {
	for k := range headers {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}

// SetHeader sets name to value in headers. When headers already holds the
// name in another case, that entry is updated and keeps its spelling, so a
// header Curlew adds never goes out twice.
func SetHeader(headers map[string]string, name, value string) {
	for k := range headers {
		if strings.EqualFold(k, name) {
			headers[k] = value
			return
		}
	}
	headers[name] = value
}
