package mudflat

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Version is reported in every envelope so a stale binary is identifiable from
// a captured response alone.
const Version = "0.1.0"

// Envelope is the response shape of the echo family (§8).
//
// Every field choice here exists to preserve information a more convenient
// representation would destroy. Headers are ordered pairs rather than a map,
// because a map loses order and collapses duplicates — and header order is how
// a signing bug becomes visible. The body is base64 and is never decoded,
// because decoding and re-encoding launders exactly the encoding bugs worth
// finding. There is no timestamp, because §6.1 requires that two identical
// requests produce identical bytes.
type Envelope struct {
	Request    RequestInfo    `json:"request"`
	Connection ConnectionInfo `json:"connection"`
	Server     ServerInfo     `json:"server"`
}

type RequestInfo struct {
	Method      string `json:"method"`
	Target      string `json:"target"`
	HTTPVersion string `json:"http_version"`

	// Headers are [name, value] pairs in the order received, with the casing
	// the client used. Duplicates appear as separate entries.
	Headers [][2]string `json:"headers"`

	// HeaderNamesInOrder duplicates the names from Headers. It is redundant, and
	// it is here because JSONPath over an array of arrays is awkward to write —
	// ergonomics for the assertion author is a feature of a test API.
	HeaderNamesInOrder []string `json:"header_names_in_order"`

	// HeadersFromParser reports that the raw preamble was unavailable and the
	// headers below came from net/http's canonicalised map instead. When true,
	// casing and order in Headers are not the client's.
	HeadersFromParser bool `json:"headers_from_parser,omitempty"`

	Query [][2]string `json:"query"`

	BodyBase64 string `json:"body_base64"`
	BodySHA256 string `json:"body_sha256"`
	BodyLen    int    `json:"body_len"`

	Trailers [][2]string `json:"trailers"`
}

type ConnectionInfo struct {
	ID             string `json:"id"`
	RequestsOnConn int    `json:"requests_on_conn"`
	// TLS is nil on a cleartext connection. Populated from Phase 3 onward.
	TLS *TLSInfo `json:"tls"`
}

type TLSInfo struct {
	Version string `json:"version"`
	Cipher  string `json:"cipher"`
	ALPN    string `json:"alpn"`
}

type ServerInfo struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Endpoint string `json:"endpoint"`
	Session  string `json:"session,omitempty"`
}

// buildEnvelope assembles the response for one request. body must already have
// been read by the middleware, which is also what guarantees the capture state
// machine can safely re-arm.
func buildEnvelope(r *http.Request, body []byte, endpoint, session string) Envelope {
	env := Envelope{
		Request: RequestInfo{
			Method:      r.Method,
			Target:      r.RequestURI,
			HTTPVersion: r.Proto,
			Query:       parseQueryPairs(r.URL.RawQuery),
			BodyBase64:  base64.StdEncoding.EncodeToString(body),
			BodyLen:     len(body),
			Trailers:    [][2]string{},
		},
		Server: ServerInfo{
			Name:     "mudflat",
			Version:  Version,
			Endpoint: endpoint,
			Session:  session,
		},
	}

	sum := sha256.Sum256(body)
	env.Request.BodySHA256 = hex.EncodeToString(sum[:])

	if conn, ok := connFromContext(r.Context()); ok {
		env.Connection.ID = conn.id
		env.Connection.RequestsOnConn = requestNumberFromContext(r.Context())

		if raw, ok := conn.takePreamble(); ok {
			_, headers := parsePreamble(raw)
			env.Request.Headers = headers
		}
	}

	if env.Request.Headers == nil {
		env.Request.Headers = headersFromParser(r)
		env.Request.HeadersFromParser = true
	}

	env.Request.HeaderNamesInOrder = make([]string, 0, len(env.Request.Headers))
	for _, pair := range env.Request.Headers {
		env.Request.HeaderNamesInOrder = append(env.Request.HeaderNamesInOrder, pair[0])
	}

	return env
}

// headersFromParser is the degraded path, used when no raw preamble is
// available — HTTP/2, or a preamble larger than the capture bound. Sorted so
// that even the degraded output is deterministic.
func headersFromParser(r *http.Request) [][2]string {
	names := make([]string, 0, len(r.Header))
	for name := range r.Header {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([][2]string, 0, len(names))
	for _, name := range names {
		for _, value := range r.Header[name] {
			out = append(out, [2]string{name, value})
		}
	}
	return out
}

// parseQueryPairs preserves order and repeated keys, both of which
// url.Values discards.
func parseQueryPairs(raw string) [][2]string {
	out := [][2]string{}
	if raw == "" {
		return out
	}
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		decodedName, err := url.QueryUnescape(name)
		if err != nil {
			decodedName = name
		}
		decodedValue, err := url.QueryUnescape(value)
		if err != nil {
			decodedValue = value
		}
		out = append(out, [2]string{decodedName, decodedValue})
	}
	return out
}
