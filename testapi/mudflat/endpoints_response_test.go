package mudflat

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// startServer returns a running server and a client that does not interfere
// with what is under test: transparent gzip is disabled so Content-Encoding
// behaviour is observable, and redirects are not followed.
func startServer(t *testing.T) (string, *http.Client) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := New(Options{})
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{DisableCompression: true},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return "http://" + ln.Addr().String(), client
}

func get(t *testing.T, client *http.Client, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body of %s: %v", url, err)
	}
	return resp, body
}

func TestStatus_ReturnsRequestedCode(t *testing.T) {
	base, client := startServer(t)

	for _, code := range []int{200, 201, 202, 204, 301, 400, 401, 403, 404, 418, 429, 500, 502, 503} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			resp, _ := get(t, client, fmt.Sprintf("%s/status/%d", base, code))
			if resp.StatusCode != code {
				t.Errorf("status = %d, want %d", resp.StatusCode, code)
			}
		})
	}
}

func TestStatus_BodilessCodesCarryNoBody(t *testing.T) {
	// 204 and 304 must not carry a body. A test API that sends one teaches a
	// client the wrong lesson and can desynchronise a keep-alive connection.
	base, client := startServer(t)

	for _, code := range []int{204, 304} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			resp, body := get(t, client, fmt.Sprintf("%s/status/%d", base, code))
			if resp.StatusCode != code {
				t.Fatalf("status = %d, want %d", resp.StatusCode, code)
			}
			if len(body) != 0 {
				t.Errorf("body = %q, want empty for %d", body, code)
			}
		})
	}
}

func TestStatus_RejectsOutOfRange(t *testing.T) {
	base, client := startServer(t)

	for _, raw := range []string{"99", "600", "abc", "-1"} {
		t.Run(raw, func(t *testing.T) {
			resp, body := get(t, client, base+"/status/"+raw)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
			if !strings.Contains(string(body), raw) {
				t.Errorf("error body does not quote the offending value %q: %s", raw, body)
			}
		})
	}
}

func TestEncoding_GzipDecompressesToTheDeclaredPayload(t *testing.T) {
	base, client := startServer(t)

	resp, body := get(t, client, base+"/encoding/gzip")
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}

	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("body is not valid gzip: %v", err)
	}
	defer func() { _ = zr.Close() }()

	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(plain, &payload); err != nil {
		t.Fatalf("decompressed body is not JSON: %v (%q)", err, plain)
	}
	if payload["encoding"] != "gzip" {
		t.Errorf("payload.encoding = %v, want gzip", payload["encoding"])
	}
}

func TestEncoding_DeflateDecompresses(t *testing.T) {
	base, client := startServer(t)

	resp, body := get(t, client, base+"/encoding/deflate")
	if got := resp.Header.Get("Content-Encoding"); got != "deflate" {
		t.Fatalf("Content-Encoding = %q, want deflate", got)
	}

	plain, err := io.ReadAll(flate.NewReader(bytes.NewReader(body)))
	if err != nil {
		t.Fatalf("body is not valid deflate: %v", err)
	}
	if !bytes.Contains(plain, []byte("deflate")) {
		t.Errorf("decompressed body = %q, want it to name the encoding", plain)
	}
}

func TestEncoding_IdentityIsPlain(t *testing.T) {
	base, client := startServer(t)

	_, body := get(t, client, base+"/encoding/identity")
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("identity body is not plain JSON: %v (%q)", err, body)
	}
}

func TestEncoding_LyingContentEncodingSendsPlainBytes(t *testing.T) {
	// The highest-yield endpoint in the family and the one that needs no
	// compression library: declare gzip, send plain. A client that trusts the
	// header produces a confusing decode error; the question this asks is
	// whether the error is legible.
	base, client := startServer(t)

	resp, body := get(t, client, base+"/encoding/lying/gzip")
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if _, err := gzip.NewReader(bytes.NewReader(body)); err == nil {
		t.Error("body decoded as gzip; this endpoint must send plain bytes under a gzip header")
	}
	if !bytes.Contains(body, []byte("not actually compressed")) {
		t.Errorf("body = %q, want the plain marker text", body)
	}
}

func TestEncoding_UnknownEncodingRejected(t *testing.T) {
	base, client := startServer(t)

	resp, _ := get(t, client, base+"/encoding/br")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 — br needs a dependency and is Phase 3", resp.StatusCode)
	}
}

func TestCharset_ProducesTheDeclaredBytes(t *testing.T) {
	base, client := startServer(t)

	tests := []struct {
		name        string
		wantCT      string
		wantPrefix  []byte
		wantNotUTF8 bool
	}{
		{name: "utf-8", wantCT: "text/plain; charset=utf-8"},
		{name: "iso-8859-1", wantCT: "text/plain; charset=iso-8859-1", wantNotUTF8: true},
		{name: "shift_jis", wantCT: "text/plain; charset=shift_jis", wantNotUTF8: true},
		{name: "utf-16le-bom", wantCT: "text/plain; charset=utf-16le", wantPrefix: []byte{0xff, 0xfe}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := get(t, client, base+"/charset/"+tc.name)
			if got := resp.Header.Get("Content-Type"); got != tc.wantCT {
				t.Errorf("Content-Type = %q, want %q", got, tc.wantCT)
			}
			if len(body) == 0 {
				t.Fatal("empty body")
			}
			if tc.wantPrefix != nil && !bytes.HasPrefix(body, tc.wantPrefix) {
				t.Errorf("body prefix = % x, want % x (BOM)", body[:2], tc.wantPrefix)
			}
			if tc.wantNotUTF8 && json.Valid(body) {
				t.Errorf("body for %s should not be valid UTF-8 JSON", tc.name)
			}
		})
	}
}

func TestContentType_Variants(t *testing.T) {
	base, client := startServer(t)

	tests := []struct{ variant, want string }{
		{"json", "application/json"},
		{"json-charset", "application/json; charset=utf-8"},
		{"vendor", "application/vnd.api+json"},
		{"text-json", "text/json"},
		{"plain", "text/plain"},
		{"absent", ""},
		{"lying", "application/json"},
	}

	for _, tc := range tests {
		t.Run(tc.variant, func(t *testing.T) {
			resp, body := get(t, client, base+"/content-type/"+tc.variant)
			if got := resp.Header.Get("Content-Type"); got != tc.want {
				t.Errorf("Content-Type = %q, want %q", got, tc.want)
			}
			if tc.variant == "lying" && json.Valid(body) {
				t.Error("the lying variant must not return valid JSON under an application/json header")
			}
			if tc.variant == "json" && !json.Valid(body) {
				t.Errorf("body is not valid JSON: %q", body)
			}
		})
	}
}

func TestJSON_EdgeCases(t *testing.T) {
	base, client := startServer(t)

	tests := []struct {
		name      string
		checkBody func(t *testing.T, body []byte)
	}{
		{"deep", func(t *testing.T, b []byte) {
			if !json.Valid(b) {
				t.Errorf("not valid JSON: %q", b)
			}
			if bytes.Count(b, []byte("{")) < 30 {
				t.Errorf("expected deep nesting, got %d levels", bytes.Count(b, []byte("{")))
			}
		}},
		{"bignum", func(t *testing.T, b []byte) {
			// The point of this endpoint: a float64 round trip turns
			// 9007199254740993 into ...92, so an equals assertion against the
			// original would pass against a different number.
			if !bytes.Contains(b, []byte("9007199254740993")) {
				t.Errorf("body does not contain the beyond-float64 integer: %s", b)
			}
		}},
		{"dupkeys", func(t *testing.T, b []byte) {
			if bytes.Count(b, []byte(`"a"`)) < 2 {
				t.Errorf("expected a duplicated key, got %s", b)
			}
		}},
		{"unicode-escapes", func(t *testing.T, b []byte) {
			if !bytes.Contains(b, []byte(`\u`)) {
				t.Errorf("expected escape sequences, got %s", b)
			}
		}},
		{"empty", func(t *testing.T, b []byte) {
			if strings.TrimSpace(string(b)) != "{}" {
				t.Errorf("body = %q, want {}", b)
			}
		}},
		{"toplevel-array", func(t *testing.T, b []byte) {
			if !bytes.HasPrefix(bytes.TrimSpace(b), []byte("[")) {
				t.Errorf("body = %q, want a top-level array", b)
			}
		}},
		{"toplevel-string", func(t *testing.T, b []byte) {
			if !bytes.HasPrefix(bytes.TrimSpace(b), []byte(`"`)) {
				t.Errorf("body = %q, want a top-level string", b)
			}
		}},
		{"toplevel-null", func(t *testing.T, b []byte) {
			if strings.TrimSpace(string(b)) != "null" {
				t.Errorf("body = %q, want null", b)
			}
		}},
		{"nan", func(t *testing.T, b []byte) {
			if json.Valid(b) {
				t.Errorf("body %q is valid JSON; this endpoint must emit bare NaN", b)
			}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := get(t, client, base+"/json/"+tc.name)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			tc.checkBody(t, body)
		})
	}
}

func TestNDJSON_ReturnsOneObjectPerLine(t *testing.T) {
	base, client := startServer(t)

	resp, body := get(t, client, base+"/ndjson/5")
	if got := resp.Header.Get("Content-Type"); got != "application/x-ndjson" {
		t.Errorf("Content-Type = %q, want application/x-ndjson", got)
	}

	lines := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("got %d lines, want 5", len(lines))
	}
	for i, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Errorf("line %d is not valid JSON: %q", i, line)
		}
	}
}

func TestBytes_IsDeterministicForAGivenSeed(t *testing.T) {
	base, client := startServer(t)

	_, first := get(t, client, base+"/bytes/64?seed=7")
	_, second := get(t, client, base+"/bytes/64?seed=7")
	_, other := get(t, client, base+"/bytes/64?seed=8")

	if len(first) != 64 {
		t.Fatalf("length = %d, want 64", len(first))
	}
	if !bytes.Equal(first, second) {
		t.Error("same seed produced different bytes; §6.1 requires determinism")
	}
	if bytes.Equal(first, other) {
		t.Error("different seeds produced identical bytes")
	}
}

func TestBytes_DefaultSeedIsStable(t *testing.T) {
	base, client := startServer(t)

	_, a := get(t, client, base+"/bytes/32")
	_, b := get(t, client, base+"/bytes/32")
	if !bytes.Equal(a, b) {
		t.Error("omitting the seed must still be deterministic")
	}
}

func TestEmpty_ReturnsZeroLengthBodyWithNoContentType(t *testing.T) {
	base, client := startServer(t)

	resp, body := get(t, client, base+"/empty")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if len(body) != 0 {
		t.Errorf("body = %q, want empty", body)
	}
	if got := resp.Header.Get("Content-Type"); got != "" {
		t.Errorf("Content-Type = %q, want absent", got)
	}
}

func TestIndex_ListsEveryEndpointWithACitation(t *testing.T) {
	base, client := startServer(t)

	_, body := get(t, client, base+"/")

	var doc struct {
		Name      string       `json:"name"`
		Endpoints []IndexEntry `json:"endpoints"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("index is not valid JSON: %v", err)
	}
	if doc.Name != "mudflat" {
		t.Errorf("name = %q, want mudflat", doc.Name)
	}
	if len(doc.Endpoints) == 0 {
		t.Fatal("index lists no endpoints")
	}
	for _, e := range doc.Endpoints {
		if e.Exercises == "" {
			t.Errorf("endpoint %q has no Exercises citation (§P3)", e.Pattern)
		}
		if e.Summary == "" {
			t.Errorf("endpoint %q has no summary", e.Pattern)
		}
	}
}

func TestIndex_IsStablyOrdered(t *testing.T) {
	base, client := startServer(t)

	_, first := get(t, client, base+"/")
	_, second := get(t, client, base+"/")
	if !bytes.Equal(first, second) {
		t.Error("index bytes differ between two calls; §6.1 requires a stable order")
	}
}

func TestUnknownPath_Returns404(t *testing.T) {
	base, client := startServer(t)

	resp, _ := get(t, client, base+"/no-such-endpoint")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}
