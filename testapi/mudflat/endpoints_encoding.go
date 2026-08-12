package mudflat

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
)

// registerEncoding mounts family C (§9.C): everything about how a body is
// framed, encoded, typed, and shaped.
func (s *Server) registerEncoding() {
	s.register(Endpoint{
		Pattern:   "/encoding/{enc}",
		Methods:   []string{http.MethodGet},
		Family:    "C",
		Summary:   "Body correctly encoded with gzip, deflate, or identity.",
		Exercises: "Response decoding. Go's client transparently handles gzip only when it set Accept-Encoding itself, so deflate is where a decoding gap becomes visible.",
		Handler:   s.handleEncoding,
	})

	s.register(Endpoint{
		Pattern:   "/encoding/lying/{enc}",
		Methods:   []string{http.MethodGet},
		Family:    "C",
		Summary:   "Declares a Content-Encoding it did not apply; the body is plain.",
		Exercises: "Error legibility. A client that trusts the header fails to decode; the question is whether the resulting message names the request and the cause.",
		Handler:   s.handleEncodingLying,
	})

	s.register(Endpoint{
		Pattern:   "/charset/{name}",
		Methods:   []string{http.MethodGet},
		Family:    "C",
		Summary:   "Non-ASCII payload in utf-8, iso-8859-1, shift_jis, or utf-16le-bom.",
		Exercises: "Body handling for bodies that are not UTF-8. The base64 envelope field exists because a decode-and-re-encode round trip would hide exactly this.",
		Handler:   s.handleCharset,
	})

	s.register(Endpoint{
		Pattern:   "/content-type/{variant}",
		Methods:   []string{http.MethodGet},
		Family:    "C",
		Summary:   "JSON body under varying, absent, or lying Content-Type headers.",
		Exercises: "Whether body assertions run at all. A parser keyed on an exact application/json match silently skips a vendor type, turning every body assertion into a path-not-found rather than a failure.",
		Handler:   s.handleContentType,
	})

	s.register(Endpoint{
		Pattern:   "/json/{shape}",
		Methods:   []string{http.MethodGet},
		Family:    "C",
		Summary:   "JSON edge cases: deep, bignum, dupkeys, unicode-escapes, empty, toplevel-array, toplevel-string, toplevel-null, nan.",
		Exercises: "The 13 body operators and JSONPath extraction against values that break naive parsing — notably bignum, where a float64 round trip changes the number.",
		Handler:   s.handleJSONShape,
	})

	s.register(Endpoint{
		Pattern:   "/ndjson/{n}",
		Methods:   []string{http.MethodGet},
		Family:    "C",
		Summary:   "n newline-delimited JSON objects.",
		Exercises: "Bodies that are not a single JSON document, and the events stream's own NDJSON shape.",
		Handler:   s.handleNDJSON,
	})

	s.register(Endpoint{
		Pattern:   "/bytes/{n}",
		Methods:   []string{http.MethodGet},
		Family:    "C",
		Summary:   "n deterministic pseudorandom bytes, seeded by the seed query parameter.",
		Exercises: "Binary bodies, Content-Length handling, and the 50 MB body guard rail.",
		Handler:   s.handleBytes,
	})

	s.register(Endpoint{
		Pattern:   "/empty",
		Methods:   []string{http.MethodGet},
		Family:    "C",
		Summary:   "200 with a zero-length body and no Content-Type.",
		Exercises: "Assertions against an absent body: exists/not_exists must distinguish 'no body' from 'path missing'.",
		Handler:   s.handleEmpty,
	})
}

func (s *Server) handleEncoding(w http.ResponseWriter, r *http.Request) {
	enc := r.PathValue("enc")
	payload := fmt.Appendf(nil,
		`{"encoding":%q,"note":"this body really is %s-encoded","filler":%q}`+"\n",
		enc, enc, strings.Repeat("compressible ", 20))

	var body []byte
	switch enc {
	case "identity":
		body = payload
	case "gzip":
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(payload); err != nil {
			writeProblem(w, http.StatusInternalServerError, "gzip write: "+err.Error())
			return
		}
		if err := zw.Close(); err != nil {
			writeProblem(w, http.StatusInternalServerError, "gzip close: "+err.Error())
			return
		}
		body = buf.Bytes()
	case "deflate":
		var buf bytes.Buffer
		fw, err := flate.NewWriter(&buf, flate.DefaultCompression)
		if err != nil {
			writeProblem(w, http.StatusInternalServerError, "deflate init: "+err.Error())
			return
		}
		if _, err := fw.Write(payload); err != nil {
			writeProblem(w, http.StatusInternalServerError, "deflate write: "+err.Error())
			return
		}
		if err := fw.Close(); err != nil {
			writeProblem(w, http.StatusInternalServerError, "deflate close: "+err.Error())
			return
		}
		body = buf.Bytes()
	default:
		// br and zstd are not in the standard library. Adding a dependency to a
		// test server is a real cost (CLAUDE.md: evaluate every go get), and the
		// lying variant below already covers the high-value case with none.
		writeProblem(w, http.StatusBadRequest, fmt.Sprintf(
			"unsupported encoding %q: gzip, deflate and identity are available; br and zstd need a dependency and are Phase 3", enc,
		))
		return
	}

	if enc != "identity" {
		w.Header().Set("Content-Encoding", enc)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *Server) handleEncodingLying(w http.ResponseWriter, r *http.Request) {
	enc := r.PathValue("enc")
	body := fmt.Appendf(nil,
		`{"declared":%q,"actual":"identity","note":"not actually compressed"}`+"\n", enc)

	w.Header().Set("Content-Encoding", enc)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// charsetFixtures hold literal bytes rather than transcoded strings. Encoding
// tables would be another dependency, and the bytes are the contract anyway.
var charsetFixtures = map[string]struct {
	contentType string
	body        []byte
}{
	// "Grüße, 世界" in UTF-8.
	"utf-8": {
		contentType: "text/plain; charset=utf-8",
		body:        []byte("Grüße, 世界\n"),
	},
	// "Grüße" in ISO-8859-1: ü is 0xFC, ß is 0xDF. Not valid UTF-8.
	"iso-8859-1": {
		contentType: "text/plain; charset=iso-8859-1",
		body:        []byte{'G', 'r', 0xFC, 0xDF, 'e', '\n'},
	},
	// "世界" in Shift_JIS.
	"shift_jis": {
		contentType: "text/plain; charset=shift_jis",
		body:        []byte{0x90, 0xA2, 0x8A, 0x45, '\n'},
	},
	// BOM followed by "Hi" in UTF-16LE.
	"utf-16le-bom": {
		contentType: "text/plain; charset=utf-16le",
		body:        []byte{0xFF, 0xFE, 'H', 0x00, 'i', 0x00},
	},
}

func (s *Server) handleCharset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fixture, ok := charsetFixtures[name]
	if !ok {
		writeProblem(w, http.StatusBadRequest, invalidParam("charset", name,
			"one of utf-8, iso-8859-1, shift_jis, utf-16le-bom").Error())
		return
	}

	w.Header().Set("Content-Type", fixture.contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(fixture.body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(fixture.body)
}

var contentTypeVariants = map[string]struct {
	contentType string
	body        string
}{
	"json":         {"application/json", `{"variant":"json","ok":true}` + "\n"},
	"json-charset": {"application/json; charset=utf-8", `{"variant":"json-charset","ok":true}` + "\n"},
	"vendor":       {"application/vnd.api+json", `{"variant":"vendor","ok":true}` + "\n"},
	"text-json":    {"text/json", `{"variant":"text-json","ok":true}` + "\n"},
	"plain":        {"text/plain", `{"variant":"plain","ok":true}` + "\n"},
	"absent":       {"", `{"variant":"absent","ok":true}` + "\n"},
	"lying":        {"application/json", "this is not JSON, despite the header\n"},
}

func (s *Server) handleContentType(w http.ResponseWriter, r *http.Request) {
	variant := r.PathValue("variant")
	fixture, ok := contentTypeVariants[variant]
	if !ok {
		writeProblem(w, http.StatusBadRequest, invalidParam("variant", variant,
			"one of json, json-charset, vendor, text-json, plain, absent, lying").Error())
		return
	}

	if fixture.contentType != "" {
		w.Header().Set("Content-Type", fixture.contentType)
	} else {
		// Go sniffs a Content-Type on first Write unless one is already set.
		// The absent variant only means something if nothing is sent, so the
		// sniffer is suppressed explicitly.
		w.Header()["Content-Type"] = nil
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(fixture.body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fixture.body))
}

// jsonShapes are literal documents. Marshalling them from Go values would
// normalise away the very properties under test — encoding/json cannot emit a
// duplicate key, and it would round-trip the big integer through float64.
var jsonShapes = map[string]string{
	"bignum": `{"beyond_float64":9007199254740993,` +
		`"high_precision":0.1234567890123456789,` +
		`"very_large":1e308,` +
		`"negative":-9007199254740993}` + "\n",
	"dupkeys": `{"a":1,"a":2,"b":"first","b":"second"}` + "\n",
	// Escape sequences rather than the characters themselves: a parser that
	// leaves the escape undecoded and one that decodes it both produce valid
	// JSON, and an equals assertion tells them apart only if the wire form is
	// the escaped one. The surrogate pair is the case naive decoders get wrong.
	"unicode-escapes": `{"latin":"\u00e9","cjk":"\u4e16\u754c",` +
		`"surrogate_pair":"\ud83d\ude00","control":"\u0000","quote":"\""}` + "\n",
	"empty":           "{}\n",
	"toplevel-array":  `[1,2,3,{"nested":true}]` + "\n",
	"toplevel-string": `"a bare top-level string"` + "\n",
	"toplevel-null":   "null\n",
	// Bare NaN is not valid JSON. Real servers emit it; a parser that accepts it
	// silently and one that rejects it loudly are both defensible, and the
	// difference should be visible rather than assumed.
	"nan": `{"value":NaN,"also":Infinity}` + "\n",
}

func (s *Server) handleJSONShape(w http.ResponseWriter, r *http.Request) {
	shape := r.PathValue("shape")

	if shape == "deep" {
		w.Header().Set("Content-Type", "application/json")
		body := deepJSON(40)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}

	body, ok := jsonShapes[shape]
	if !ok {
		names := make([]string, 0, len(jsonShapes)+1)
		names = append(names, "deep")
		for name := range jsonShapes {
			names = append(names, name)
		}
		writeProblem(w, http.StatusBadRequest, invalidParam("shape", shape,
			"one of "+strings.Join(names, ", ")).Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

// deepJSON builds a document nested depth levels, with a leaf a JSONPath
// expression can reach.
func deepJSON(depth int) []byte {
	var b strings.Builder
	for range depth {
		b.WriteString(`{"a":`)
	}
	b.WriteString(`{"leaf":"bottom"}`)
	for range depth {
		b.WriteString(`}`)
	}
	b.WriteString("\n")
	return []byte(b.String())
}

func (s *Server) handleNDJSON(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 0 {
		writeProblem(w, http.StatusBadRequest,
			invalidParam("n", r.PathValue("n"), "expected a non-negative whole number").Error())
		return
	}
	if n > 10000 {
		writeProblem(w, http.StatusBadRequest,
			invalidParam("n", r.PathValue("n"), "maximum 10000 lines").Error())
		return
	}

	var b bytes.Buffer
	for i := range n {
		fmt.Fprintf(&b, `{"i":%d,"name":"row-%d","even":%t}`+"\n", i, i, i%2 == 0)
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Length", strconv.Itoa(b.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b.Bytes())
}

// bytesStreamKey is a fixed odd constant used as the second PCG word, so the
// default (unseeded) stream is still stable across runs and processes.
const bytesStreamKey = 0x9E3779B97F4A7C15

func (s *Server) handleBytes(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 0 {
		writeProblem(w, http.StatusBadRequest,
			invalidParam("n", r.PathValue("n"), "expected a non-negative whole number").Error())
		return
	}
	if n > maxRequestBody {
		writeProblem(w, http.StatusBadRequest,
			invalidParam("n", r.PathValue("n"), fmt.Sprintf("maximum %d bytes", maxRequestBody)).Error())
		return
	}

	seed := uint64(0)
	if raw := r.URL.Query().Get("seed"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			writeProblem(w, http.StatusBadRequest,
				invalidParam("seed", raw, "expected a non-negative whole number").Error())
			return
		}
		seed = parsed
	}

	// crypto/rand would make this endpoint non-reproducible, which §6.1 forbids.
	gen := rand.New(rand.NewPCG(seed, bytesStreamKey))
	body := make([]byte, n)
	for i := range body {
		body[i] = byte(gen.UintN(256))
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *Server) handleEmpty(w http.ResponseWriter, _ *http.Request) {
	w.Header()["Content-Type"] = nil
	w.Header().Set("Content-Length", "0")
	w.WriteHeader(http.StatusOK)
}
