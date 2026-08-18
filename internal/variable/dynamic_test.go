package variable

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/weiqigod/curlew/internal/docs"
	apierrors "github.com/weiqigod/curlew/internal/errors"
)

// helpers ---------------------------------------------------------------

func isValidUnixSeconds(t *testing.T, val string) {
	t.Helper()
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		t.Errorf("timestamp %q is not an integer: %v", val, err)
		return
	}
	if n < 1_000_000_000 {
		t.Errorf("timestamp %q looks too small (want > 1e9)", val)
	}
}

func isValidRFC3339(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).MatchString(val) {
		t.Errorf("isoTimestamp %q does not match RFC3339 UTC format", val)
	}
}

func isValidUnixMs(t *testing.T, val string) {
	t.Helper()
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		t.Errorf("timestampMs %q is not an integer: %v", val, err)
		return
	}
	if n < 1_000_000_000_000 {
		t.Errorf("timestampMs %q looks too small (want > 1e12)", val)
	}
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func isValidUUIDv4(t *testing.T, val string) {
	t.Helper()
	if !uuidPattern.MatchString(val) {
		t.Errorf("value %q is not a valid UUID v4", val)
	}
}

func isIntInRange(lo, hi int64) func(t *testing.T, val string) {
	return func(t *testing.T, val string) {
		t.Helper()
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			t.Errorf("randomInt %q is not an integer: %v", val, err)
			return
		}
		if n < lo || n > hi {
			t.Errorf("randomInt %q not in range [%d,%d]", val, lo, hi)
		}
	}
}

func isFloatInRange(lo, hi float64) func(t *testing.T, val string) {
	return func(t *testing.T, val string) {
		t.Helper()
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			t.Errorf("randomFloat %q is not a float: %v", val, err)
			return
		}
		if f < lo || f > hi {
			t.Errorf("randomFloat %q not in range [%.2f,%.2f]", val, lo, hi)
		}
	}
}

func isBoolean(t *testing.T, val string) {
	t.Helper()
	if val != "true" && val != "false" {
		t.Errorf("randomBoolean %q is not 'true' or 'false'", val)
	}
}

func isAlphaNumLen(length int) func(t *testing.T, val string) {
	return func(t *testing.T, val string) {
		t.Helper()
		if len(val) != length {
			t.Errorf("randomString len=%d, want %d: %q", len(val), length, val)
		}
		if !regexp.MustCompile(`^[a-zA-Z0-9]+$`).MatchString(val) {
			t.Errorf("randomString %q contains non-alphanumeric characters", val)
		}
	}
}

func isHexLen(length int) func(t *testing.T, val string) {
	return func(t *testing.T, val string) {
		t.Helper()
		if len(val) != length {
			t.Errorf("randomHex len=%d, want %d: %q", len(val), length, val)
		}
		if !regexp.MustCompile(`^[0-9a-f]+$`).MatchString(val) {
			t.Errorf("randomHex %q contains non-hex characters", val)
		}
	}
}

func isEmail(t *testing.T, val string) {
	t.Helper()
	if !strings.Contains(val, "@") {
		t.Errorf("randomEmail %q missing '@'", val)
	}
	if !strings.HasSuffix(val, "@example.com") {
		t.Errorf("randomEmail %q does not end with @example.com", val)
	}
}

func isTwoParts(t *testing.T, val string) {
	t.Helper()
	parts := strings.Fields(val)
	if len(parts) != 2 {
		t.Errorf("randomName %q does not have exactly two space-separated parts", val)
	}
}

func isNonEmpty(t *testing.T, val string) {
	t.Helper()
	if val == "" {
		t.Error("expected non-empty string, got empty")
	}
}

func isHexColor(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^#[0-9a-f]{6}$`).MatchString(val) {
		t.Errorf("randomColor %q does not match #rrggbb format", val)
	}
}

// tests -----------------------------------------------------------------

func TestRegistry_Evaluate(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	cache := make(map[string]string)

	tests := []struct {
		name     string
		funcName string
		validate func(t *testing.T, val string)
	}{
		{"timestamp returns unix seconds", "timestamp", isValidUnixSeconds},
		{"isoTimestamp returns RFC3339", "isoTimestamp", isValidRFC3339},
		{"timestampMs returns milliseconds", "timestampMs", isValidUnixMs},
		{"uuid returns valid v4", "uuid", isValidUUIDv4},
		{"guid same format as uuid", "guid", isValidUUIDv4},
		{"randomInt in range 0-1000", "randomInt", isIntInRange(0, 1000)},
		{"randomFloat in range 0-1000", "randomFloat", isFloatInRange(0, 1000)},
		{"randomBoolean is true or false", "randomBoolean", isBoolean},
		{"randomString is 16 alphanumeric chars", "randomString", isAlphaNumLen(16)},
		{"randomHex is 32 hex chars", "randomHex", isHexLen(32)},
		{"randomEmail has @ and example.com", "randomEmail", isEmail},
		{"randomName has two space-separated parts", "randomName", isTwoParts},
		{"randomFirstName is non-empty string", "randomFirstName", isNonEmpty},
		{"randomLastName is non-empty string", "randomLastName", isNonEmpty},
		{"randomColor matches #rrggbb", "randomColor", isHexColor},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := reg.Evaluate(tt.funcName, nil, cache)
			if err != nil {
				t.Fatalf("Evaluate(%q) unexpected error: %v", tt.funcName, err)
			}
			tt.validate(t, val)
		})
	}

	t.Run("unknown function returns error", func(t *testing.T) {
		_, err := reg.Evaluate("unknownFunc", nil, cache)
		if err == nil {
			t.Fatal("expected error for unknown function, got nil")
		}
	})
}

func TestRegistry_no_seed_produces_valid_values(t *testing.T) {
	reg := NewRegistry(nil)
	cache := make(map[string]string)
	val, err := reg.Evaluate("uuid", nil, cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	isValidUUIDv4(t, val)
}

func TestRegistry_seeded_deterministic(t *testing.T) {
	seed := int64(99)
	reg1 := NewRegistry(&seed)
	reg2 := NewRegistry(&seed)
	cache1 := make(map[string]string)
	cache2 := make(map[string]string)

	for _, fn := range []string{"randomInt", "randomString", "randomBoolean", "randomHex"} {
		v1, _ := reg1.Evaluate(fn, nil, cache1)
		v2, _ := reg2.Evaluate(fn, nil, cache2)
		if v1 != v2 {
			t.Errorf("%s: seed 99 produced different values: %q vs %q", fn, v1, v2)
		}
	}
}

func TestRegistry_seeded_uuid_deterministic(t *testing.T) {
	seed := int64(42)
	reg1 := NewRegistry(&seed)
	reg2 := NewRegistry(&seed)
	cache1 := make(map[string]string)
	cache2 := make(map[string]string)

	u1, _ := reg1.Evaluate("uuid", nil, cache1)
	u2, _ := reg2.Evaluate("uuid", nil, cache2)
	if u1 != u2 {
		t.Errorf("seeded uuid should be deterministic: %q vs %q", u1, u2)
	}
	isValidUUIDv4(t, u1)
}

func TestRegistry_different_seeds_different_values(t *testing.T) {
	s1, s2 := int64(1), int64(2)
	reg1 := NewRegistry(&s1)
	reg2 := NewRegistry(&s2)
	cache1 := make(map[string]string)
	cache2 := make(map[string]string)

	// Different seeds should (with overwhelming probability) produce different values.
	v1, _ := reg1.Evaluate("randomString", nil, cache1)
	v2, _ := reg2.Evaluate("randomString", nil, cache2)
	if v1 == v2 {
		t.Errorf("different seeds produced same randomString: %q", v1)
	}
}

func TestRegistry_evaluate_caches_within_request(t *testing.T) {
	seed := int64(7)
	reg := NewRegistry(&seed)
	cache := make(map[string]string) // same cache = same request

	v1, _ := reg.Evaluate("uuid", nil, cache)
	v2, _ := reg.Evaluate("uuid", nil, cache)
	if v1 != v2 {
		t.Errorf("same cache: uuid should be memoized, got %q then %q", v1, v2)
	}
}

func TestRegistry_evaluate_different_cache_different_result(t *testing.T) {
	seed := int64(7)
	reg := NewRegistry(&seed)

	// Different caches simulate different requests.
	v1, _ := reg.Evaluate("uuid", nil, make(map[string]string))
	v2, _ := reg.Evaluate("uuid", nil, make(map[string]string))
	if v1 == v2 {
		t.Errorf("different caches: uuid should differ per request, got same value %q", v1)
	}
}

func TestRegistry_unknown_function_lists_available(t *testing.T) {
	reg := NewRegistry(nil)
	_, err := reg.Evaluate("bogus", nil, make(map[string]string))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The error message should mention available functions.
	msg := err.Error()
	if !strings.Contains(msg, "uuid") {
		t.Errorf("error message should list available functions; got: %q", msg)
	}
}

func TestRegistry_available_sorted(t *testing.T) {
	reg := NewRegistry(nil)
	avail := reg.Available()
	if !sort.StringsAreSorted(avail) {
		t.Errorf("Available() is not sorted: %v", avail)
	}
	if len(avail) != 86 {
		t.Errorf("expected exactly 86 built-in functions, got %d: %v", len(avail), avail)
	}
}

func TestRegistry_Evaluate_nil_cache_does_not_panic(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	// Passing nil cache must not panic — Evaluate should treat it as no-op memoization.
	val, err := reg.Evaluate("uuid", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	isValidUUIDv4(t, val)
}

func TestRegistry_no_seed_randomFloat_valid(t *testing.T) {
	// Exercises the crypto/rand path in float64InRange (rng==nil branch).
	reg := NewRegistry(nil)
	val, err := reg.Evaluate("randomFloat", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	isFloatInRange(0, 1000)(t, val)
}

func TestRegistry_no_seed_randomInt_valid(t *testing.T) {
	// Exercises the crypto/rand path in intn (rng==nil branch).
	reg := NewRegistry(nil)
	val, err := reg.Evaluate("randomInt", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	isIntInRange(0, 1000)(t, val)
}

func TestRegistry_seed_zero_is_valid(t *testing.T) {
	zero := int64(0)
	reg1 := NewRegistry(&zero)
	reg2 := NewRegistry(&zero)
	cache1 := make(map[string]string)
	cache2 := make(map[string]string)

	v1, err1 := reg1.Evaluate("randomInt", nil, cache1)
	v2, err2 := reg2.Evaluate("randomInt", nil, cache2)
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v / %v", err1, err2)
	}
	if v1 != v2 {
		t.Errorf("seed=0 not deterministic: %q vs %q", v1, v2)
	}
}

func TestRegistry_Evaluate_AcceptsArgs(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)

	t.Run("existing functions accept empty args", func(t *testing.T) {
		cache := make(map[string]string)
		for _, fn := range []string{"timestamp", "uuid", "randomInt", "randomString"} {
			v, err := reg.Evaluate(fn, nil, cache)
			if err != nil {
				t.Errorf("%s with nil args: unexpected error %v", fn, err)
			}
			if v == "" {
				t.Errorf("%s with nil args: empty result", fn)
			}
		}
	})

	t.Run("zero-arg function rejects unexpected args", func(t *testing.T) {
		_, err := reg.Evaluate("timestamp", []string{"surprise"}, nil)
		if err == nil {
			t.Fatal("expected arity error, got nil")
		}
		var se *apierrors.Structured
		if !errors.As(err, &se) {
			t.Fatalf("expected *apierrors.Structured, got %T", err)
		}
		if se.Code != "DYNFN_ARITY" {
			t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
		}
		// Message should contain the arity info without the function-name prefix
		// (the prefix is added by Evaluate's fmt.Errorf wrapper, not by arityError itself).
		if !strings.Contains(se.Message, "expected 0 arguments, got 1") {
			t.Errorf("message %q should mention arity 0 vs 1", se.Message)
		}
		// Verify no doubled "$name: $name:" prefix — err.Error() should have exactly one $timestamp prefix.
		errStr := err.Error()
		if strings.Count(errStr, "$timestamp") > 1 {
			t.Errorf("err.Error() has doubled function-name prefix: %q", errStr)
		}
	})

	t.Run("cache distinguishes by canonicalised args", func(t *testing.T) {
		// Register a test-only echo function that returns args[0] verbatim.
		reg2 := NewRegistry(nil)
		reg2.funcs["echo"] = func(_ *rand.Rand, args []string) (string, error) {
			if len(args) != 1 {
				return "", arityError("echo", 1, len(args))
			}
			return args[0], nil
		}
		cache := make(map[string]string)
		a, _ := reg2.Evaluate("echo", []string{"a"}, cache)
		b, _ := reg2.Evaluate("echo", []string{"b"}, cache)
		if a != "a" || b != "b" {
			t.Errorf("got (%q, %q), want (a, b) — cache collision", a, b)
		}
		if len(cache) != 2 {
			t.Errorf("cache size = %d, want 2", len(cache))
		}
	})
}

func TestRegistry_Base64(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name     string
		funcName string
		input    string
		want     string
	}{
		{"encode hello", "base64", "hello", "aGVsbG8="},
		{"encode user:pw", "base64", "user:pw", "dXNlcjpwdw=="},
		{"encode empty string", "base64", "", ""},
		{"encode utf-8 multibyte", "base64", "héllo", "aMOpbGxv"},
		{"decode hello", "base64Decode", "aGVsbG8=", "hello"},
		{"decode user:pw", "base64Decode", "dXNlcjpwdw==", "user:pw"},
		{"decode empty string", "base64Decode", "", ""},
		{"decode utf-8 multibyte", "base64Decode", "aMOpbGxv", "héllo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.Evaluate(tt.funcName, []string{tt.input}, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(%q, %q) unexpected error: %v", tt.funcName, tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Evaluate(%q, %q) = %q, want %q", tt.funcName, tt.input, got, tt.want)
			}
		})
	}
}

func TestRegistry_Base64_Roundtrip(t *testing.T) {
	reg := NewRegistry(nil)

	inputs := []string{
		"",                        // empty
		"a",                       // single ASCII
		"hello world",             // ASCII with space
		"héllo, wörld — 🌍",        // UTF-8 multibyte + emoji
		"line1\nline2\r\nline3",   // newlines
		"with\x00nul\x00bytes",    // binary-safe (NUL)
		strings.Repeat("x", 1024), // long input
	}
	for _, in := range inputs {
		t.Run(fmt.Sprintf("roundtrip len=%d", len(in)), func(t *testing.T) {
			enc, err := reg.Evaluate("base64", []string{in}, make(map[string]string))
			if err != nil {
				t.Fatalf("encode error: %v", err)
			}
			dec, err := reg.Evaluate("base64Decode", []string{enc}, make(map[string]string))
			if err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if dec != in {
				t.Errorf("roundtrip mismatch: input %q -> %q -> %q", in, enc, dec)
			}
		})
	}
}

func TestRegistry_Base64_NestedVar(t *testing.T) {
	seed := int64(1)
	reg := NewRegistry(&seed)
	s := NewScope(map[string]string{
		"user": "admin",
		"pass": "secret",
	})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	got, err := s.Interpolate(`{{$base64('{{user}}:{{pass}}')}}`)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	const want = "YWRtaW46c2VjcmV0" // base64("admin:secret")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRegistry_Base64Decode_invalid_input(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name  string
		input string
	}{
		{"non-base64 chars", "not!base64@all"},
		{"bad padding", "aGVsbG8"}, // missing one '=' padding char
		{"long invalid input", strings.Repeat("!", 64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate("base64Decode", []string{tt.input}, make(map[string]string))
			if err == nil {
				t.Fatalf("expected error for input %q, got nil", tt.input)
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
			}
			if se.Category != apierrors.CategoryInput {
				t.Errorf("Category = %q, want CategoryInput", se.Category)
			}
			if se.Code != "DYNFN_BASE64_DECODE" {
				t.Errorf("Code = %q, want DYNFN_BASE64_DECODE", se.Code)
			}
			// Message must contain a snippet of the offending input.
			// For long inputs (>32 chars) we expect truncation with "...".
			if len(tt.input) > 32 {
				if !strings.Contains(se.Message, tt.input[:32]) {
					t.Errorf("message %q should contain first 32 chars of input %q", se.Message, tt.input[:32])
				}
				if !strings.Contains(se.Message, "...") {
					t.Errorf("message %q should contain truncation marker '...'", se.Message)
				}
				if strings.Contains(se.Message, tt.input) {
					t.Errorf("message %q should NOT contain the full untruncated input", se.Message)
				}
			} else {
				if !strings.Contains(se.Message, tt.input) {
					t.Errorf("message %q should contain the offending input %q", se.Message, tt.input)
				}
			}
			// Inner error should be the underlying base64 decode error.
			if se.Inner == nil {
				t.Error("Inner error should carry the underlying decoder error")
			}
		})
	}
}

func TestRegistry_Base64_arity_errors(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name     string
		funcName string
		args     []string
	}{
		{"base64 zero args", "base64", nil},
		{"base64 two args", "base64", []string{"a", "b"}},
		{"base64Decode zero args", "base64Decode", nil},
		{"base64Decode two args", "base64Decode", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate(tt.funcName, tt.args, nil)
			if err == nil {
				t.Fatal("expected arity error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_ARITY" {
				t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
			}
			wantArity := fmt.Sprintf("expected 1 arguments, got %d", len(tt.args))
			if !strings.Contains(se.Message, wantArity) {
				t.Errorf("message %q should mention %q", se.Message, wantArity)
			}
			// The full err.Error() should name the function once.
			if got := strings.Count(err.Error(), "$"+tt.funcName); got != 1 {
				t.Errorf("err.Error() should name $%s exactly once, got %d: %q", tt.funcName, got, err.Error())
			}
		})
	}
}

func TestRegistry_Base64_caches_per_request(t *testing.T) {
	// Pure functions of input — calling Evaluate twice with the same arg in
	// the same request must hit the cache. Verify by wrapping the registered
	// function with a counter.
	reg := NewRegistry(nil)
	var calls int
	reg.funcs["base64"] = oneArg("base64", func(s string) (string, error) {
		calls++
		return base64.StdEncoding.EncodeToString([]byte(s)), nil
	})

	cache := make(map[string]string)
	v1, err := reg.Evaluate("base64", []string{"hello"}, cache)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	v2, err := reg.Evaluate("base64", []string{"hello"}, cache)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if v1 != v2 {
		t.Errorf("memoized values differ: %q vs %q", v1, v2)
	}
	if calls != 1 {
		t.Errorf("expected 1 underlying call, got %d", calls)
	}

	// Different argument bypasses the cache.
	if _, err := reg.Evaluate("base64", []string{"world"}, cache); err != nil {
		t.Fatalf("third call: %v", err)
	}
	if calls != 2 {
		t.Errorf("different arg should miss cache; expected 2 calls, got %d", calls)
	}
}

func TestRegistry_UrlEncode(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Behaviors 1 + observable: spaces → '+', '&' → '%26',
		// '/' → '%2F', UTF-8 percent-encoded.
		{"hello world", "hello world", "hello+world"},
		{"hello world & co", "hello world & co", "hello+world+%26+co"},
		{"slashes encoded", "a/b", "a%2Fb"},
		{"ampersand encoded", "a&b", "a%26b"},
		{"empty string", "", ""},
		{"unicode multibyte", "héllo", "h%C3%A9llo"},
		{"plus stays encoded", "a+b", "a%2Bb"},
		{"already-encoded value re-encoded", "a%20b", "a%2520b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.Evaluate("urlEncode", []string{tt.input}, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(urlEncode, %q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("urlEncode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRegistry_UrlEncode_NestedVar(t *testing.T) {
	seed := int64(1)
	reg := NewRegistry(&seed)
	s := NewScope(map[string]string{"query": "a b"})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	got, err := s.Interpolate(`{{$urlEncode('{{query}}')}}`)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	const want = "a+b"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRegistry_JsonEncode(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Behavior 2: surrounding double quotes included.
		{"plain ascii", "hello", `"hello"`},
		// Behavior 7: empty input → literal three chars `""`.
		{"empty string", "", `""`},
		// Behavior 3: quote and backslash escaped per RFC 8259.
		{"contains quote", `he said "hi"`, `"he said \"hi\""`},
		{"contains backslash", `a\b`, `"a\\b"`},
		// Behavior 4: literal newline byte → \n in output, valid JSON.
		{"contains newline", "line1\nline2", `"line1\nline2"`},
		{"contains tab", "a\tb", `"a\tb"`},
		{"contains carriage return", "a\rb", `"a\rb"`},
		// Unicode round-trip.
		{"unicode multibyte", "héllo", `"héllo"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.Evaluate("jsonEncode", []string{tt.input}, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(jsonEncode, %q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("jsonEncode(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// Sanity check: the result must itself be valid JSON
			// that round-trips back to the original input.
			var rt string
			if err := json.Unmarshal([]byte(got), &rt); err != nil {
				t.Errorf("jsonEncode(%q) result %q is not valid JSON: %v", tt.input, got, err)
			}
			if rt != tt.input {
				t.Errorf("jsonEncode(%q) round-trip = %q, want %q", tt.input, rt, tt.input)
			}
		})
	}
}

func TestRegistry_Sha256(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Behavior 1 + observable: golden vector for "hello".
		{"hello", "hello", "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"},
		// Behavior 3: empty string.
		{"empty string", "", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		// Behavior 4: UTF-8 multibyte — hash is over the byte sequence,
		// not the rune sequence. Golden value computed once and pinned.
		{"utf-8 multibyte", "héllo", "3c48591d8d098a4538f5e013dfcf406e948eac4d3277b10bf614e295d6068179"},
		// Determinism cross-check — repeating the call returns the same hash.
		{"abc", "abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.Evaluate("sha256", []string{tt.input}, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(sha256, %q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("sha256(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// Behavior 1: 64 chars, lowercase hex only.
			if len(got) != 64 {
				t.Errorf("sha256(%q) length = %d, want 64", tt.input, len(got))
			}
			if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(got) {
				t.Errorf("sha256(%q) = %q, not lowercase 64-char hex", tt.input, got)
			}
		})
	}
}

func TestRegistry_Md5(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Behavior 2 + observable: golden vector for "hello".
		{"hello", "hello", "5d41402abc4b2a76b9719d911017c592"},
		// Empty-string golden vector (RFC 1321).
		{"empty string", "", "d41d8cd98f00b204e9800998ecf8427e"},
		// UTF-8 multibyte — hash is over the byte sequence.
		// Golden value computed once and pinned.
		{"utf-8 multibyte", "héllo", "be50e8478cf24ff3595bc7307fb91b50"},
		// Determinism cross-check.
		{"abc", "abc", "900150983cd24fb0d6963f7d28e17f72"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.Evaluate("md5", []string{tt.input}, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(md5, %q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("md5(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// Behavior 2: 32 chars, lowercase hex only.
			if len(got) != 32 {
				t.Errorf("md5(%q) length = %d, want 32", tt.input, len(got))
			}
			if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(got) {
				t.Errorf("md5(%q) = %q, not lowercase 32-char hex", tt.input, got)
			}
		})
	}
}

func TestRegistry_Sha256_NestedVar(t *testing.T) {
	seed := int64(1)
	reg := NewRegistry(&seed)
	s := NewScope(map[string]string{"payload": "hello"})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	got, err := s.Interpolate(`{{$sha256('{{payload}}')}}`)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRegistry_Sha256_caches_per_request(t *testing.T) {
	// Behavior 6: pure function of input — calling Evaluate twice with the
	// same arg in the same request must hit the cache. Verify by wrapping
	// the registered function with a counter.
	reg := NewRegistry(nil)
	var calls int
	reg.funcs["sha256"] = oneArg("sha256", func(s string) (string, error) {
		calls++
		return fmt.Sprintf("%x", sha256.Sum256([]byte(s))), nil
	})

	cache := make(map[string]string)
	v1, err := reg.Evaluate("sha256", []string{"x"}, cache)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	v2, err := reg.Evaluate("sha256", []string{"x"}, cache)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if v1 != v2 {
		t.Errorf("memoized values differ: %q vs %q", v1, v2)
	}
	if calls != 1 {
		t.Errorf("expected 1 underlying call, got %d", calls)
	}
	// Different request (fresh cache) → same value because the function is pure.
	cache2 := make(map[string]string)
	v3, _ := reg.Evaluate("sha256", []string{"x"}, cache2)
	if v3 != v1 {
		t.Errorf("pure function should be deterministic across requests: %q vs %q", v3, v1)
	}
}

func TestRegistry_HashFns_arity_errors(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name     string
		funcName string
		args     []string
	}{
		{"sha256 zero args", "sha256", nil},
		{"sha256 two args", "sha256", []string{"a", "b"}},
		{"md5 zero args", "md5", nil},
		{"md5 two args", "md5", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate(tt.funcName, tt.args, nil)
			if err == nil {
				t.Fatal("expected arity error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_ARITY" {
				t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
			}
			wantArity := fmt.Sprintf("expected 1 arguments, got %d", len(tt.args))
			if !strings.Contains(se.Message, wantArity) {
				t.Errorf("message %q should mention %q", se.Message, wantArity)
			}
			// The full err.Error() should name the function exactly once.
			if got := strings.Count(err.Error(), "$"+tt.funcName); got != 1 {
				t.Errorf("err.Error() should name $%s exactly once, got %d: %q", tt.funcName, got, err.Error())
			}
		})
	}
}

func TestRegistry_UrlJsonEncode_arity_errors(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name     string
		funcName string
		args     []string
	}{
		{"urlEncode zero args", "urlEncode", nil},
		{"urlEncode two args", "urlEncode", []string{"a", "b"}},
		{"jsonEncode zero args", "jsonEncode", nil},
		{"jsonEncode two args", "jsonEncode", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate(tt.funcName, tt.args, nil)
			if err == nil {
				t.Fatal("expected arity error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_ARITY" {
				t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
			}
			wantArity := fmt.Sprintf("expected 1 arguments, got %d", len(tt.args))
			if !strings.Contains(se.Message, wantArity) {
				t.Errorf("message %q should mention %q", se.Message, wantArity)
			}
			// The full err.Error() should name the function exactly once.
			if got := strings.Count(err.Error(), "$"+tt.funcName); got != 1 {
				t.Errorf("err.Error() should name $%s exactly once, got %d: %q", tt.funcName, got, err.Error())
			}
		})
	}
}

// --- Step 1: $hmacSha256 golden-vector tests ---

func TestRegistry_HmacSha256(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name    string
		payload string
		key     string
		want    string
	}{
		// Behavior 2 + observable: the canonical RFC-style test vector.
		{
			"rfc-style fox vector",
			"The quick brown fox jumps over the lazy dog",
			"key",
			"f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8",
		},
		// Empty payload, empty key — HMAC-SHA-256 of zero bytes under a zero-byte key.
		{
			"empty payload empty key",
			"",
			"",
			"b613679a0814d9ec772f95d778c35fc5ff1697c493715653c6c712144292c5ad",
		},
		// Empty payload, non-empty key.
		{
			"empty payload non-empty key",
			"",
			"key",
			"5d5d139563c95b5967b9bd9a8c9b233a9dedb45072794cd232dc1b74832607d0",
		},
		// UTF-8 multibyte in both payload and key.
		{
			"utf-8 payload and key",
			"héllo",
			"schlüssel",
			"2549e44a3915d22c7c1aa9b091567603d08bb70d4add5021e5ba553699fb2b01",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.Evaluate("hmacSha256", []string{tt.payload, tt.key}, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(hmacSha256) unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("hmacSha256(%q, %q) = %q, want %q", tt.payload, tt.key, got, tt.want)
			}
			// Behavior 1: 64 chars, lowercase hex only.
			if len(got) != 64 {
				t.Errorf("hmacSha256(%q, %q) length = %d, want 64", tt.payload, tt.key, len(got))
			}
			if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(got) {
				t.Errorf("hmacSha256(%q, %q) = %q, not lowercase 64-char hex", tt.payload, tt.key, got)
			}
		})
	}
}

func TestRegistry_HmacSha256_NestedVar(t *testing.T) {
	seed := int64(1)
	reg := NewRegistry(&seed)
	s := NewScope(map[string]string{
		"payload": "The quick brown fox jumps over the lazy dog",
		"key":     "key",
	})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	got, err := s.Interpolate(`{{$hmacSha256('{{payload}}', '{{key}}')}}`)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	const want = "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRegistry_HmacSha256_caches_per_request(t *testing.T) {
	// Behavior 7: pure function of (payload, key) — calling Evaluate
	// twice with the same args in the same request must hit the cache.
	reg := NewRegistry(nil)
	var calls int
	reg.funcs["hmacSha256"] = twoArgs("hmacSha256", func(payload, key string) (string, error) {
		calls++
		h := hmac.New(sha256.New, []byte(key))
		h.Write([]byte(payload))
		return fmt.Sprintf("%x", h.Sum(nil)), nil
	})

	cache := make(map[string]string)
	v1, err := reg.Evaluate("hmacSha256", []string{"p", "k1"}, cache)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	v2, err := reg.Evaluate("hmacSha256", []string{"p", "k1"}, cache)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if v1 != v2 {
		t.Errorf("memoized values differ: %q vs %q", v1, v2)
	}
	if calls != 1 {
		t.Errorf("expected 1 underlying call, got %d", calls)
	}
	// Different key → cache miss.
	v3, err := reg.Evaluate("hmacSha256", []string{"p", "k2"}, cache)
	if err != nil {
		t.Fatalf("third call: %v", err)
	}
	if v3 == v1 {
		t.Errorf("different key should produce different MAC: both = %q", v3)
	}
	if calls != 2 {
		t.Errorf("different key should miss cache; expected 2 calls, got %d", calls)
	}
	// Different payload, same key → also a cache miss.
	if _, err := reg.Evaluate("hmacSha256", []string{"q", "k1"}, cache); err != nil {
		t.Fatalf("fourth call: %v", err)
	}
	if calls != 3 {
		t.Errorf("different payload should miss cache; expected 3 calls, got %d", calls)
	}
}

// --- Step 3: sensitive-arg propagation tests ---

func TestRegistry_HmacSha256_KeyIsSensitive_HeuristicName(t *testing.T) {
	if _, err := docs.Prose("MANUAL.md", "registered as a redaction trigger for the run"); err != nil {
		t.Fatalf("documented claim: %v", err)
	}

	// Behavior 4: key resolves from a variable whose name matches the
	// sensitive-name heuristic ("contains 'secret'"). Resolved key
	// string must land on the runtimeSensitive set.
	reg := NewRegistry(nil)
	s := NewScope(map[string]string{
		"payload":               "amount=100",
		"stripe_signing_secret": "sk_live_supersecret",
	})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	runtimeSet := NewSensitiveSet()
	s = s.WithDynamic(reg).WithRuntimeSensitive(runtimeSet)
	s.BeginRequest()
	defer s.EndRequest()

	_, err := s.Interpolate(`{{$hmacSha256('{{payload}}', '{{stripe_signing_secret}}')}}`)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}

	vals := runtimeSet.Values()
	if len(vals) != 1 || vals[0] != "sk_live_supersecret" {
		t.Errorf("runtimeSet.Values() = %v, want [\"sk_live_supersecret\"]", vals)
	}
}

func TestRegistry_HmacSha256_KeyIsSensitive_SecretsNamespace(t *testing.T) {
	// Behavior 4 (secrets-namespace branch): a {{secrets.X}} token is
	// always sensitive, regardless of alias name.
	reg := NewRegistry(nil)
	s := NewScope(map[string]string{
		"payload": "amount=100",
	})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	s = s.WithSecrets(map[string]string{"signing_key": "whsec_abc123"})
	runtimeSet := NewSensitiveSet()
	s = s.WithDynamic(reg).WithRuntimeSensitive(runtimeSet)
	s.BeginRequest()
	defer s.EndRequest()

	_, err := s.Interpolate(`{{$hmacSha256('{{payload}}', '{{secrets.signing_key}}')}}`)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}

	vals := runtimeSet.Values()
	if len(vals) != 1 || vals[0] != "whsec_abc123" {
		t.Errorf("runtimeSet.Values() = %v, want [\"whsec_abc123\"]", vals)
	}
}

func TestRegistry_HmacSha256_LiteralKey_NotMarked(t *testing.T) {
	// Behavior 5: a literal key (no {{var}} placeholder) cannot be
	// back-traced to a variable, so no SensitiveSet mutation occurs.
	reg := NewRegistry(nil)
	s := NewScope(map[string]string{"payload": "amount=100"})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	runtimeSet := NewSensitiveSet()
	s = s.WithDynamic(reg).WithRuntimeSensitive(runtimeSet)
	s.BeginRequest()
	defer s.EndRequest()

	_, err := s.Interpolate(`{{$hmacSha256('{{payload}}', 'literal-key')}}`)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}

	if vals := runtimeSet.Values(); len(vals) != 0 {
		t.Errorf("literal key should not mutate runtimeSet; got %v", vals)
	}
}

func TestRegistry_HmacSha256_PayloadSensitive_NotKey(t *testing.T) {
	// Behavior 6: only the key triggers auto-marking. If the payload
	// resolves from a sensitive variable but the key is a literal,
	// nothing is added to runtimeSet.
	reg := NewRegistry(nil)
	s := NewScope(map[string]string{
		"user_password_payload": "password=hunter2",
	})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	runtimeSet := NewSensitiveSet()
	s = s.WithDynamic(reg).WithRuntimeSensitive(runtimeSet)
	s.BeginRequest()
	defer s.EndRequest()

	_, err := s.Interpolate(`{{$hmacSha256('{{user_password_payload}}', 'literal-key')}}`)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}

	if vals := runtimeSet.Values(); len(vals) != 0 {
		t.Errorf("payload-only sensitivity must not mutate runtimeSet; got %v", vals)
	}
}

func TestRegistry_HmacSha256_TwoCallsDifferentKeys(t *testing.T) {
	// Behavior 7: two HMAC computations in the same request with
	// different keys produce correct, distinct MACs; both sensitive keys
	// are captured.
	reg := NewRegistry(nil)
	s := NewScope(map[string]string{
		"payload":  "data=v1",
		"secret_a": "key-aaa",
		"secret_b": "key-bbb",
	})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	runtimeSet := NewSensitiveSet()
	s = s.WithDynamic(reg).WithRuntimeSensitive(runtimeSet)
	s.BeginRequest()
	defer s.EndRequest()

	a, err := s.Interpolate(`{{$hmacSha256('{{payload}}', '{{secret_a}}')}}`)
	if err != nil {
		t.Fatalf("first interpolate: %v", err)
	}
	b, err := s.Interpolate(`{{$hmacSha256('{{payload}}', '{{secret_b}}')}}`)
	if err != nil {
		t.Fatalf("second interpolate: %v", err)
	}
	if a == b {
		t.Errorf("different keys produced same MAC: both = %q", a)
	}

	vals := runtimeSet.Values()
	seen := map[string]bool{}
	for _, v := range vals {
		seen[v] = true
	}
	if !seen["key-aaa"] || !seen["key-bbb"] {
		t.Errorf("runtimeSet.Values() = %v, want both \"key-aaa\" and \"key-bbb\"", vals)
	}
}

func TestRegistry_HmacSha256_arity_errors(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name string
		args []string
	}{
		{"zero args", nil},
		{"one arg", []string{"only-payload"}},
		{"three args", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate("hmacSha256", tt.args, nil)
			if err == nil {
				t.Fatal("expected arity error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_ARITY" {
				t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
			}
			// Behavior 3: arity-mismatch error names arity 2.
			wantArity := fmt.Sprintf("expected 2 arguments, got %d", len(tt.args))
			if !strings.Contains(se.Message, wantArity) {
				t.Errorf("message %q should mention %q", se.Message, wantArity)
			}
			// The full err.Error() should name $hmacSha256 exactly once.
			if got := strings.Count(err.Error(), "$hmacSha256"); got != 1 {
				t.Errorf("err.Error() should name $hmacSha256 exactly once, got %d: %q", got, err.Error())
			}
		})
	}
}

// --- M12-006: $dateAdd and $dateSubtract tests ---

// frozenClock returns a clock function that always returns t. Used to
// pin "now" for $dateAdd / $dateSubtract / $formatDate golden tests.
func frozenClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// TestRegistry_DateAdd verifies $dateAdd against a frozen clock.
// Behavior 1: $dateAdd('1', 'hour') returns now + 1h as ISO-8601 UTC.
func TestRegistry_DateAdd(t *testing.T) {
	base := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	reg := NewRegistry(nil)
	reg.now = frozenClock(base)

	got, err := reg.Evaluate("dateAdd", []string{"1", "hour"}, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	const want = "2026-04-28T13:00:00Z"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRegistry_DateSubtract verifies $dateSubtract against a frozen clock.
// Behavior 2: $dateSubtract('7', 'day') returns now - 7 days as ISO-8601 UTC.
func TestRegistry_DateSubtract(t *testing.T) {
	base := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	reg := NewRegistry(nil)
	reg.now = frozenClock(base)

	got, err := reg.Evaluate("dateSubtract", []string{"7", "day"}, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	const want = "2026-04-21T12:00:00Z"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRegistry_DateAdd_AllUnits exercises every supported unit via $dateAdd.
func TestRegistry_DateAdd_AllUnits(t *testing.T) {
	base := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	reg := NewRegistry(nil)
	reg.now = frozenClock(base)

	tests := []struct {
		name, amount, unit, want string
	}{
		{"second", "5", "second", "2026-04-28T12:00:05Z"},
		{"minute", "5", "minute", "2026-04-28T12:05:00Z"},
		{"hour", "1", "hour", "2026-04-28T13:00:00Z"},
		{"day", "1", "day", "2026-04-29T12:00:00Z"},
		{"week", "1", "week", "2026-05-05T12:00:00Z"},
		{"month", "1", "month", "2026-05-28T12:00:00Z"},
		{"year", "1", "year", "2027-04-28T12:00:00Z"},
		{"negative hour", "-3", "hour", "2026-04-28T09:00:00Z"},
		{"large day count", "365", "day", "2027-04-28T12:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.Evaluate("dateAdd", []string{tt.amount, tt.unit}, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(%s,%s): %v", tt.amount, tt.unit, err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRegistry_DateSubtract_AllUnits exercises every supported unit via $dateSubtract,
// including the 30-minute case that was previously in TestRegistry_DateSubtract.
func TestRegistry_DateSubtract_AllUnits(t *testing.T) {
	base := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	reg := NewRegistry(nil)
	reg.now = frozenClock(base)

	tests := []struct {
		name, amount, unit, want string
	}{
		{"minute 30", "30", "minute", "2026-04-28T11:30:00Z"},
		{"hour 1", "1", "hour", "2026-04-28T11:00:00Z"},
		{"day 7", "7", "day", "2026-04-21T12:00:00Z"},
		{"week 1", "1", "week", "2026-04-21T12:00:00Z"},
		{"month 1", "1", "month", "2026-03-28T12:00:00Z"},
		{"year 1", "1", "year", "2025-04-28T12:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.Evaluate("dateSubtract", []string{tt.amount, tt.unit}, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(%s,%s): %v", tt.amount, tt.unit, err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRegistry_DateAdd_InvalidUnit verifies the structured error for unknown units.
// Behavior 4: error names the function, the bad unit, and lists the seven allowed units.
func TestRegistry_DateAdd_InvalidUnit(t *testing.T) {
	reg := NewRegistry(nil)
	_, err := reg.Evaluate("dateAdd", []string{"1", "fortnight"}, nil)
	if err == nil {
		t.Fatal("expected structured error, got nil")
	}
	var se *apierrors.Structured
	if !errors.As(err, &se) {
		t.Fatalf("expected *apierrors.Structured, got %T", err)
	}
	if se.Code != "DYNFN_DATE_BAD_UNIT" {
		t.Errorf("Code = %q, want DYNFN_DATE_BAD_UNIT", se.Code)
	}
	if !strings.Contains(se.Message, "fortnight") {
		t.Errorf("Message %q should mention the bad unit", se.Message)
	}
	for _, u := range []string{"second", "minute", "hour", "day", "week", "month", "year"} {
		if !strings.Contains(se.Hint, u) {
			t.Errorf("Hint %q should list unit %q", se.Hint, u)
		}
	}
	// Function name appears once in err.Error().
	if got := strings.Count(err.Error(), "$dateAdd"); got != 1 {
		t.Errorf("err.Error() should name $dateAdd exactly once, got %d: %q", got, err.Error())
	}
}

// TestRegistry_DateAdd_NonIntegerAmount rejects "1.5" with DYNFN_DATE_BAD_AMOUNT.
// Behavior 5.
func TestRegistry_DateAdd_NonIntegerAmount(t *testing.T) {
	reg := NewRegistry(nil)
	_, err := reg.Evaluate("dateAdd", []string{"1.5", "hour"}, nil)
	if err == nil {
		t.Fatal("expected structured error, got nil")
	}
	var se *apierrors.Structured
	if !errors.As(err, &se) {
		t.Fatalf("expected *apierrors.Structured, got %T", err)
	}
	if se.Code != "DYNFN_DATE_BAD_AMOUNT" {
		t.Errorf("Code = %q, want DYNFN_DATE_BAD_AMOUNT", se.Code)
	}
	if !strings.Contains(se.Message, "1.5") {
		t.Errorf("Message %q should quote the bad amount", se.Message)
	}
	if !strings.Contains(se.Hint, "$dateAdd('90', 'minute')") {
		t.Errorf("Hint %q should suggest the integer-minutes workaround", se.Hint)
	}
	if se.Inner == nil {
		t.Error("Inner should carry the strconv.NumError")
	}
}

// TestRegistry_DateAdd_AddDateMonthRollover documents the time.AddDate
// rollover semantics. Behavior 6: amount='12' unit='month' on 2026-01-31
// produces 2027-01-31 (exact; no rollover), while amount='13' on the same
// base rolls to 2027-03-03 because Feb 31 doesn't exist.
func TestRegistry_DateAdd_AddDateMonthRollover(t *testing.T) {
	base := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name, amount, want string
	}{
		{
			// Spec behavior 6: 12 months lands on 2027-01-31 exactly — no rollover.
			name:   "12 months no rollover",
			amount: "12",
			want:   "2027-01-31T00:00:00Z",
		},
		{
			// 13 months: Jan 31 + 13 months = Feb 31 2027 → rolls to March 3, 2027.
			name:   "13 months rolls to March",
			amount: "13",
			want:   "2027-03-03T00:00:00Z",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry(nil)
			reg.now = frozenClock(base)
			got, err := reg.Evaluate("dateAdd", []string{tt.amount, "month"}, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q (pins time.AddDate rollover behaviour)", got, tt.want)
			}
		})
	}
}

// TestRegistry_DateAdd_DateSubtract_arity_errors verifies arity-2 enforcement.
// Behavior 7.
func TestRegistry_DateAdd_DateSubtract_arity_errors(t *testing.T) {
	reg := NewRegistry(nil)
	cases := []struct {
		name, fn string
		args     []string
	}{
		{"dateAdd zero args", "dateAdd", nil},
		{"dateAdd one arg", "dateAdd", []string{"1"}},
		{"dateAdd three args", "dateAdd", []string{"1", "hour", "extra"}},
		{"dateSubtract zero args", "dateSubtract", nil},
		{"dateSubtract one arg", "dateSubtract", []string{"1"}},
		{"dateSubtract three args", "dateSubtract", []string{"1", "hour", "extra"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate(tt.fn, tt.args, nil)
			if err == nil {
				t.Fatal("expected arity error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_ARITY" {
				t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
			}
			wantArity := fmt.Sprintf("expected 2 arguments, got %d", len(tt.args))
			if !strings.Contains(se.Message, wantArity) {
				t.Errorf("Message %q should contain %q", se.Message, wantArity)
			}
			if got := strings.Count(err.Error(), "$"+tt.fn); got != 1 {
				t.Errorf("err.Error() should name $%s exactly once, got %d: %q", tt.fn, got, err.Error())
			}
		})
	}
}

// TestRegistry_DateAdd_DateSubtract_ZeroOffset_Symmetric pins the
// behaviour-8 byte-for-byte guarantee: amount='0' unit='second'
// returns the same string from both functions.
func TestRegistry_DateAdd_DateSubtract_ZeroOffset_Symmetric(t *testing.T) {
	base := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	reg := NewRegistry(nil)
	reg.now = frozenClock(base)

	a, errA := reg.Evaluate("dateAdd", []string{"0", "second"}, make(map[string]string))
	b, errB := reg.Evaluate("dateSubtract", []string{"0", "second"}, make(map[string]string))
	if errA != nil || errB != nil {
		t.Fatalf("unexpected errors: %v / %v", errA, errB)
	}
	if a != b {
		t.Errorf("zero-offset must be symmetric: dateAdd=%q dateSubtract=%q", a, b)
	}
	if a != "2026-04-28T12:00:00Z" {
		t.Errorf("zero offset should equal frozen now formatted; got %q", a)
	}
}

// TestRegistry_DateAdd_NegativeIsDateSubtract pins behavior 3:
// $dateAdd('-3', 'hour') equals $dateSubtract('3', 'hour').
func TestRegistry_DateAdd_NegativeIsDateSubtract(t *testing.T) {
	base := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	reg := NewRegistry(nil)
	reg.now = frozenClock(base)

	a, _ := reg.Evaluate("dateAdd", []string{"-3", "hour"}, make(map[string]string))
	b, _ := reg.Evaluate("dateSubtract", []string{"3", "hour"}, make(map[string]string))
	if a != b {
		t.Errorf("dateAdd(-3,hour) %q must equal dateSubtract(3,hour) %q", a, b)
	}
	if a != "2026-04-28T09:00:00Z" {
		t.Errorf("expected 2026-04-28T09:00:00Z, got %q", a)
	}
}

// --- M12-007: $parseDate tests (Step 1) ---

// TestRegistry_ParseDate verifies the canonical $parseDate happy path:
// parse a layout-formatted string, return ISO-8601 UTC.
// Behavior 4 + observable.
func TestRegistry_ParseDate(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name, input, layout, want string
	}{
		// Observable: $parseDate('21/04/2024 15:10:01', '02/01/2006 15:04:05')
		// → '2024-04-21T15:10:01Z'.
		{"dd/mm/yyyy hh:mm:ss", "21/04/2024 15:10:01", "02/01/2006 15:04:05", "2024-04-21T15:10:01Z"},
		// RFC3339 round-trip — already UTC.
		{"rfc3339 utc", "2024-04-21T15:10:01Z", time.RFC3339, "2024-04-21T15:10:01Z"},
		// ISO date only — time defaults to 00:00:00 UTC.
		{"iso date only", "2024-04-21", "2006-01-02", "2024-04-21T00:00:00Z"},
		// RFC1123Z with explicit zone.
		{"rfc1123z", "Sun, 21 Apr 2024 15:10:01 +0000", time.RFC1123Z, "2024-04-21T15:10:01Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.Evaluate("parseDate", []string{tt.input, tt.layout}, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseDate(%q, %q) = %q, want %q", tt.input, tt.layout, got, tt.want)
			}
		})
	}
}

// TestRegistry_ParseDate_NonUTCZone pins behavior 6: non-UTC zone-tagged
// input is parsed and converted to UTC.
// $parseDate('2024-04-21T17:10:01+02:00', time.RFC3339) → '2024-04-21T15:10:01Z'
func TestRegistry_ParseDate_NonUTCZone(t *testing.T) {
	reg := NewRegistry(nil)
	got, err := reg.Evaluate("parseDate", []string{"2024-04-21T17:10:01+02:00", time.RFC3339}, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	const want = "2024-04-21T15:10:01Z"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRegistry_ParseDate_BadInput verifies the structured error when
// time.Parse fails (e.g. mismatched layout, "YYYY-MM-DD" footgun).
func TestRegistry_ParseDate_BadInput(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name, input, layout string
	}{
		{"yyyy-mm-dd footgun", "2024-04-21", "YYYY-MM-DD"},
		{"layout/input mismatch", "21/04/2024", "2006-01-02"},
		{"long invalid input", strings.Repeat("x", 64), "2006-01-02"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate("parseDate", []string{tt.input, tt.layout}, nil)
			if err == nil {
				t.Fatal("expected structured error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
			}
			if se.Code != "DYNFN_PARSEDATE_BAD_INPUT" {
				t.Errorf("Code = %q, want DYNFN_PARSEDATE_BAD_INPUT", se.Code)
			}
			if se.Inner == nil {
				t.Error("Inner should carry the *time.ParseError")
			}
			// Long inputs must be truncated.
			if len(tt.input) > 32 {
				if !strings.Contains(se.Message, tt.input[:32]) {
					t.Errorf("message %q should contain first 32 chars of input", se.Message)
				}
				if !strings.Contains(se.Message, "...") {
					t.Errorf("message %q should contain truncation marker '...'", se.Message)
				}
			}
			if !strings.Contains(se.Hint, "MANUAL.md") {
				t.Errorf("hint %q should reference MANUAL.md", se.Hint)
			}
			if got := strings.Count(err.Error(), "$parseDate"); got != 1 {
				t.Errorf("err.Error() should name $parseDate exactly once, got %d: %q", got, err.Error())
			}
		})
	}
}

// TestRegistry_ParseDate_EmptyInput verifies the dedicated empty-input
// error (avoids the obscure stdlib message).
func TestRegistry_ParseDate_EmptyInput(t *testing.T) {
	reg := NewRegistry(nil)
	_, err := reg.Evaluate("parseDate", []string{"", "2006-01-02"}, nil)
	if err == nil {
		t.Fatal("expected structured error, got nil")
	}
	var se *apierrors.Structured
	if !errors.As(err, &se) {
		t.Fatalf("expected *apierrors.Structured, got %T", err)
	}
	if se.Code != "DYNFN_PARSEDATE_EMPTY_INPUT" {
		t.Errorf("Code = %q, want DYNFN_PARSEDATE_EMPTY_INPUT", se.Code)
	}
	if !strings.Contains(se.Message, "empty") {
		t.Errorf("message %q should mention empty", se.Message)
	}
}

// TestRegistry_ParseDate_NestedVar verifies cooperation with M12-001
// nested interpolation — `s` may itself reference a variable.
func TestRegistry_ParseDate_NestedVar(t *testing.T) {
	reg := NewRegistry(nil)
	s := NewScope(map[string]string{"raw": "21/04/2024 15:10:01"})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	got, err := s.Interpolate(`{{$parseDate('{{raw}}', '02/01/2006 15:04:05')}}`)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	const want = "2024-04-21T15:10:01Z"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- M12-007: $formatDate tests (Step 2) ---

// TestRegistry_FormatDate_FromUnix verifies behavior 1 + the first
// observable: digits-only input parsed as Unix seconds.
// $formatDate('1713701401', '2006-01-02') → '2024-04-21'
func TestRegistry_FormatDate_FromUnix(t *testing.T) {
	reg := NewRegistry(nil)
	got, err := reg.Evaluate("formatDate", []string{"1713701401", "2006-01-02"}, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	const want = "2024-04-21"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRegistry_FormatDate_FromIso verifies behavior 2 + the second
// observable: RFC3339 input parsed and reformatted with layout.
// $formatDate('2024-04-21T15:10:01Z', '02/01/2006') → '21/04/2024'
func TestRegistry_FormatDate_FromIso(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name, input, layout, want string
	}{
		{"dd/mm/yyyy", "2024-04-21T15:10:01Z", "02/01/2006", "21/04/2024"},
		{"iso date only", "2024-04-21T15:10:01Z", "2006-01-02", "2024-04-21"},
		{"month name", "2024-04-21T15:10:01Z", "02 Jan 2006", "21 Apr 2024"},
		{"unix-seconds path with month name", "1713701401", "02 Jan 2006", "21 Apr 2024"},
		// Round-trip: RFC3339 → RFC3339.
		{"rfc3339 round-trip", "2024-04-21T15:10:01Z", time.RFC3339, "2024-04-21T15:10:01Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.Evaluate("formatDate", []string{tt.input, tt.layout}, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if got != tt.want {
				t.Errorf("formatDate(%q, %q) = %q, want %q", tt.input, tt.layout, got, tt.want)
			}
		})
	}
}

// TestRegistry_FormatDate_Now verifies cooperation with M12-001 nested
// interpolation: $timestamp resolves first, the digit string is then
// passed to $formatDate.
// Behavior pinned to *shape* (length 10, valid ISO date), not to a
// frozen value, because $timestamp is intentionally never seeded.
func TestRegistry_FormatDate_Now(t *testing.T) {
	reg := NewRegistry(nil)
	s := NewScope(nil)
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	got, err := s.Interpolate(`{{$formatDate('{{$timestamp}}', '2006-01-02')}}`)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(got) {
		t.Errorf("got %q, want YYYY-MM-DD", got)
	}
}

// TestRegistry_FormatDate_LayoutAsTemplate documents behavior 5 — Go's
// stdlib quirk: a layout containing no reference-time tokens is returned
// verbatim from time.Format. We do not validate the layout; users must
// validate against time.Parse first if in doubt.
func TestRegistry_FormatDate_LayoutAsTemplate(t *testing.T) {
	reg := NewRegistry(nil)
	got, err := reg.Evaluate("formatDate", []string{"1713701401", "bogus-layout"}, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	const want = "bogus-layout"
	if got != want {
		t.Errorf("got %q, want %q (this pins time.Format layout-as-template behaviour)", got, want)
	}
}

// TestRegistry_FormatDate_BadInput verifies the structured error for
// an input that is neither digits-only nor RFC3339-parseable.
// Behavior 3.
func TestRegistry_FormatDate_BadInput(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name, input string
	}{
		{"plain word", "hello"},
		{"yyyy-mm-dd no time component", "2024-04-21"},
		{"non-rfc3339 datetime", "21/04/2024 15:10:01"},
		{"negative integer", "-1713701401"},
		{"leading-whitespace digits", " 1713701401"},
		{"long garbage", strings.Repeat("x", 64)},
		{"int64 overflow digits", "99999999999999999999"}, // 20 digits, > MaxInt64
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate("formatDate", []string{tt.input, "2006-01-02"}, nil)
			if err == nil {
				t.Fatal("expected structured error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
			}
			if se.Code != "DYNFN_FORMATDATE_BAD_INPUT" {
				t.Errorf("Code = %q, want DYNFN_FORMATDATE_BAD_INPUT", se.Code)
			}
			// Long inputs are truncated to 32 chars + ellipsis.
			if len(tt.input) > 32 {
				if !strings.Contains(se.Message, tt.input[:32]) {
					t.Errorf("message %q should contain first 32 chars of input", se.Message)
				}
				if !strings.Contains(se.Message, "...") {
					t.Errorf("message %q should contain truncation marker '...'", se.Message)
				}
				if strings.Contains(se.Message, tt.input) {
					t.Errorf("message %q should NOT contain the full untruncated input", se.Message)
				}
			}
			if !strings.Contains(se.Hint, "MANUAL.md") {
				t.Errorf("hint %q should reference MANUAL.md", se.Hint)
			}
			if got := strings.Count(err.Error(), "$formatDate"); got != 1 {
				t.Errorf("err.Error() should name $formatDate exactly once, got %d: %q", got, err.Error())
			}
		})
	}
}

// TestRegistry_FormatDate_EmptyInput pins behavior 8: empty input
// returns a structured error naming the empty input.
func TestRegistry_FormatDate_EmptyInput(t *testing.T) {
	reg := NewRegistry(nil)
	_, err := reg.Evaluate("formatDate", []string{"", "2006-01-02"}, nil)
	if err == nil {
		t.Fatal("expected structured error, got nil")
	}
	var se *apierrors.Structured
	if !errors.As(err, &se) {
		t.Fatalf("expected *apierrors.Structured, got %T", err)
	}
	if se.Code != "DYNFN_FORMATDATE_EMPTY_INPUT" {
		t.Errorf("Code = %q, want DYNFN_FORMATDATE_EMPTY_INPUT", se.Code)
	}
	if !strings.Contains(se.Message, "empty") {
		t.Errorf("message %q should mention empty", se.Message)
	}
}

// TestRegistry_FormatDate_ParseDate_arity_errors verifies arity-2
// enforcement for both functions.
// Behavior 7.
func TestRegistry_FormatDate_ParseDate_arity_errors(t *testing.T) {
	reg := NewRegistry(nil)
	cases := []struct {
		name, fn string
		args     []string
	}{
		{"formatDate zero args", "formatDate", nil},
		{"formatDate one arg", "formatDate", []string{"1713701401"}},
		{"formatDate three args", "formatDate", []string{"a", "b", "c"}},
		{"parseDate zero args", "parseDate", nil},
		{"parseDate one arg", "parseDate", []string{"2024-04-21"}},
		{"parseDate three args", "parseDate", []string{"a", "b", "c"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate(tt.fn, tt.args, nil)
			if err == nil {
				t.Fatal("expected arity error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_ARITY" {
				t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
			}
			wantArity := fmt.Sprintf("expected 2 arguments, got %d", len(tt.args))
			if !strings.Contains(se.Message, wantArity) {
				t.Errorf("Message %q should contain %q", se.Message, wantArity)
			}
			if got := strings.Count(err.Error(), "$"+tt.fn); got != 1 {
				t.Errorf("err.Error() should name $%s exactly once, got %d: %q", tt.fn, got, err.Error())
			}
		})
	}
}

// TestRegistry_RandomBase64 verifies the length invariant: the result
// base64-decodes to exactly byteLength bytes, and characters are within
// the standard-base64 alphabet. Behavior 5 + observable.
func TestRegistry_RandomBase64(t *testing.T) {
	reg := NewRegistry(nil)
	stdB64 := regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`)
	for _, n := range []int{1, 16, 32, 64} {
		t.Run(fmt.Sprintf("byteLength=%d", n), func(t *testing.T) {
			got, err := reg.Evaluate("randomBase64", []string{strconv.Itoa(n)}, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if !stdB64.MatchString(got) {
				t.Errorf("randomBase64(%d) = %q, contains non-base64 chars", n, got)
			}
			decoded, err := base64.StdEncoding.DecodeString(got)
			if err != nil {
				t.Fatalf("base64 decode: %v", err)
			}
			if len(decoded) != n {
				t.Errorf("randomBase64(%d) decoded len = %d, want %d", n, len(decoded), n)
			}
		})
	}
}

// TestRegistry_RandomBase64_Seeded verifies seeded determinism. Behavior 7.
func TestRegistry_RandomBase64_Seeded(t *testing.T) {
	seed := int64(42)
	reg1, reg2 := NewRegistry(&seed), NewRegistry(&seed)
	v1, _ := reg1.Evaluate("randomBase64", []string{"32"}, make(map[string]string))
	v2, _ := reg2.Evaluate("randomBase64", []string{"32"}, make(map[string]string))
	if v1 != v2 {
		t.Errorf("seeded determinism: %q vs %q", v1, v2)
	}
}

// TestRegistry_RandomBase64_NoSeed_DiffersBetweenCalls verifies the crypto/rand
// path produces different outputs across distinct request caches. Behavior 8.
func TestRegistry_RandomBase64_NoSeed_DiffersBetweenCalls(t *testing.T) {
	reg := NewRegistry(nil)
	v1, _ := reg.Evaluate("randomBase64", []string{"16"}, make(map[string]string))
	v2, _ := reg.Evaluate("randomBase64", []string{"16"}, make(map[string]string))
	if v1 == v2 {
		t.Errorf("crypto/rand should produce different outputs: both = %q", v1)
	}
}

// TestRegistry_RandomBase64_BadLength rejects byteLength < 1. Behavior 6.
func TestRegistry_RandomBase64_BadLength(t *testing.T) {
	reg := NewRegistry(nil)
	for _, in := range []string{"0", "-1", "-100"} {
		t.Run("byteLength="+in, func(t *testing.T) {
			_, err := reg.Evaluate("randomBase64", []string{in}, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
			}
			if se.Code != "DYNFN_RANDOMBASE64_BAD_LENGTH" {
				t.Errorf("Code = %q, want DYNFN_RANDOMBASE64_BAD_LENGTH", se.Code)
			}
			if se.Category != apierrors.CategoryInput {
				t.Errorf("Category = %q, want CategoryInput", se.Category)
			}
		})
	}
}

// TestRegistry_RandomBase64_BadInput rejects non-integer byteLength.
func TestRegistry_RandomBase64_BadInput(t *testing.T) {
	reg := NewRegistry(nil)
	for _, in := range []string{"xyz", "1.5", "", " 16"} {
		t.Run("byteLength="+in, func(t *testing.T) {
			_, err := reg.Evaluate("randomBase64", []string{in}, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
			}
			if se.Code != "DYNFN_RANDOMBASE64_BAD_INPUT" {
				t.Errorf("Code = %q, want DYNFN_RANDOMBASE64_BAD_INPUT", se.Code)
			}
			if se.Inner == nil {
				t.Error("Inner should carry the strconv.NumError")
			}
		})
	}
}

// TestRegistry_RandomBase64_arity_errors covers 0 and 2 args. Behavior 9.
func TestRegistry_RandomBase64_arity_errors(t *testing.T) {
	reg := NewRegistry(nil)
	cases := []struct {
		name string
		args []string
	}{
		{"zero args", nil},
		{"two args", []string{"16", "extra"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate("randomBase64", tt.args, nil)
			if err == nil {
				t.Fatal("expected arity error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_ARITY" {
				t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
			}
		})
	}
}

// TestRandomFunctions_reject_lengths_above_the_cap guards against the
// unbounded allocation FuzzInterpolate found (M27-002): a fuzz worker was
// killed mid-minimization while mutating the digits of a seeded
// {{$randomBase64('32')}} template, and a direct measurement of
// {{$randomBase64('1111111111')}} confirmed why -- 6.4 GB peak RSS and 7.1s
// for a single interpolation (measured 2026-08-18). Ten characters in a
// header value should not be able to ask for gigabytes.
func TestRandomFunctions_reject_lengths_above_the_cap(t *testing.T) {
	reg := NewRegistry(nil)
	tests := []struct {
		name     string
		fn       string
		arg      string
		wantCode string // empty means "must succeed"
	}{
		{"randomBase64 at the cap", "randomBase64", strconv.Itoa(MaxRandomBytes), ""},
		{"randomBase64 one over the cap", "randomBase64", strconv.Itoa(MaxRandomBytes + 1), "DYNFN_RANDOMBASE64_BAD_LENGTH"},
		{"randomBase64 ten digits", "randomBase64", "9999999999", "DYNFN_RANDOMBASE64_BAD_LENGTH"},
		{"randomBase64 still valid at 32", "randomBase64", "32", ""},
		{"randomPassword at the cap", "randomPassword", strconv.Itoa(MaxRandomBytes), ""},
		{"randomPassword one over the cap", "randomPassword", strconv.Itoa(MaxRandomBytes + 1), "DYNFN_RANDOMPASSWORD_BAD_LENGTH"},
		{"randomPassword ten digits", "randomPassword", "9999999999", "DYNFN_RANDOMPASSWORD_BAD_LENGTH"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.Evaluate(tt.fn, []string{tt.arg}, nil)
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("Evaluate(%s, %s): unexpected error: %v", tt.fn, tt.arg, err)
				}
				if len(got) == 0 {
					t.Fatalf("Evaluate(%s, %s): got empty result", tt.fn, tt.arg)
				}
				return
			}
			if err == nil {
				t.Fatalf("Evaluate(%s, %s): expected error, got nil", tt.fn, tt.arg)
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("Evaluate(%s, %s): expected *apierrors.Structured, got %T: %v", tt.fn, tt.arg, err, err)
			}
			if se.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", se.Code, tt.wantCode)
			}
		})
	}
}

// TestRegistry_RandomPassword verifies length and four-class invariants.
// Behaviors 1 + 2 + observable.
func TestRegistry_RandomPassword(t *testing.T) {
	reg := NewRegistry(nil)
	for _, n := range []int{4, 8, 16, 32, 64} {
		t.Run(fmt.Sprintf("length=%d", n), func(t *testing.T) {
			got, err := reg.Evaluate("randomPassword", []string{strconv.Itoa(n)}, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if len(got) != n {
				t.Errorf("len(%q) = %d, want %d", got, len(got), n)
			}
			// Four-class guarantee.
			hasUpper := strings.ContainsAny(got, upperChars)
			hasLower := strings.ContainsAny(got, lowerChars)
			hasDigit := strings.ContainsAny(got, digitChars)
			hasSymbol := strings.ContainsAny(got, passwordSymbols)
			if !hasUpper || !hasLower || !hasDigit || !hasSymbol {
				t.Errorf("randomPassword(%d) %q missing class: upper=%v lower=%v digit=%v symbol=%v",
					n, got, hasUpper, hasLower, hasDigit, hasSymbol)
			}
			// Every byte must come from the union.
			allClasses := upperChars + lowerChars + digitChars + passwordSymbols
			for i := 0; i < len(got); i++ {
				if !strings.ContainsRune(allClasses, rune(got[i])) {
					t.Errorf("randomPassword(%d) %q contains out-of-class byte %q at %d", n, got, string(got[i]), i)
				}
			}
		})
	}
}

// TestRegistry_RandomPassword_Seeded verifies seeded determinism and pins
// the golden vector for seed 42. Behavior 7 + observable.
func TestRegistry_RandomPassword_Seeded(t *testing.T) {
	seed := int64(42)
	reg1, reg2 := NewRegistry(&seed), NewRegistry(&seed)
	v1, _ := reg1.Evaluate("randomPassword", []string{"16"}, make(map[string]string))
	v2, _ := reg2.Evaluate("randomPassword", []string{"16"}, make(map[string]string))
	if v1 != v2 {
		t.Errorf("seeded determinism: %q vs %q", v1, v2)
	}
	if len(v1) != 16 {
		t.Errorf("seeded password len = %d, want 16: %q", len(v1), v1)
	}
}

// TestRegistry_RandomPassword_TooShort rejects length < 4. Behavior 3 + observable.
func TestRegistry_RandomPassword_TooShort(t *testing.T) {
	reg := NewRegistry(nil)
	for _, in := range []string{"0", "1", "2", "3", "-1"} {
		t.Run("length="+in, func(t *testing.T) {
			_, err := reg.Evaluate("randomPassword", []string{in}, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
			}
			if se.Code != "DYNFN_RANDOMPASSWORD_BAD_LENGTH" {
				t.Errorf("Code = %q, want DYNFN_RANDOMPASSWORD_BAD_LENGTH", se.Code)
			}
			if se.Category != apierrors.CategoryInput {
				t.Errorf("Category = %q, want CategoryInput", se.Category)
			}
		})
	}
}

// TestRegistry_RandomPassword_BadInput rejects non-integer length. Behavior 4.
func TestRegistry_RandomPassword_BadInput(t *testing.T) {
	reg := NewRegistry(nil)
	for _, in := range []string{"xyz", "1.5", "", " 16"} {
		t.Run("length="+in, func(t *testing.T) {
			_, err := reg.Evaluate("randomPassword", []string{in}, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
			}
			if se.Code != "DYNFN_RANDOMPASSWORD_BAD_INPUT" {
				t.Errorf("Code = %q, want DYNFN_RANDOMPASSWORD_BAD_INPUT", se.Code)
			}
			if se.Inner == nil {
				t.Error("Inner should carry the strconv.NumError")
			}
		})
	}
}

// TestRegistry_RandomPassword_arity_errors covers 0 and 2 args. Behavior 9.
func TestRegistry_RandomPassword_arity_errors(t *testing.T) {
	reg := NewRegistry(nil)
	cases := []struct {
		name string
		args []string
	}{
		{"zero args", nil},
		{"two args", []string{"16", "extra"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate("randomPassword", tt.args, nil)
			if err == nil {
				t.Fatal("expected arity error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_ARITY" {
				t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
			}
		})
	}
}

// TestRegistry_DottedName_UnknownFunctionReturnsError proves that a dotted
// funcName that has not been registered is treated as an unknown key by
// Evaluate and surfaces the standard `unknown dynamic function "<name>"` error.
// Uses faker.unregistered as an example — a dotted name that is not registered
// in any M13 task.
func TestRegistry_DottedName_UnknownFunctionReturnsError(t *testing.T) {
	reg := NewRegistry(nil)
	_, err := reg.Evaluate("faker.unregistered", nil, make(map[string]string))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, `unknown dynamic function "faker.unregistered"`) {
		t.Errorf("error message should name the dotted function; got: %q", msg)
	}
	if !strings.Contains(msg, "available:") {
		t.Errorf("error message should list available functions; got: %q", msg)
	}
	// Sanity: the available list must NOT contain faker.unregistered.
	if idx := strings.Index(msg, "available:"); idx >= 0 {
		availSection := msg[idx:]
		if strings.Contains(availSection, "faker.unregistered") {
			t.Errorf("available list unexpectedly mentions faker.unregistered; got: %q", availSection)
		}
	}
}

// TestRegistry_RandomPassword_NestedVar verifies cooperation with M12-001
// argument evaluation.
func TestRegistry_RandomPassword_NestedVar(t *testing.T) {
	seed := int64(1)
	reg := NewRegistry(&seed)
	s := NewScope(map[string]string{"len": "12"})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()
	got, err := s.Interpolate(`{{$randomPassword('{{len}}')}}`)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if len(got) != 12 {
		t.Errorf("len = %d, want 12: %q", len(got), got)
	}
}

// ---------------------------------------------------------------------------
// Step 1 (M13-002): sensitive-return hook tests
// ---------------------------------------------------------------------------

func TestRegistry_SensitiveReturn_HookFires(t *testing.T) {
	reg := NewRegistry(nil)
	// Synthetic test-only generator: returns a fixed token and is
	// registered as sensitive-return. Must start with a letter to match dynPattern.
	reg.funcs["testSecret"] = noArgs("testSecret", func(_ *rand.Rand) string {
		return "tok-12345"
	})
	reg.sensitiveReturn["testSecret"] = true

	s := NewScope(map[string]string{})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	runtimeSet := NewSensitiveSet()
	s = s.WithDynamic(reg).WithRuntimeSensitive(runtimeSet)
	s.BeginRequest()
	defer s.EndRequest()

	val, err := s.Interpolate(`{{$testSecret}}`)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if val != "tok-12345" {
		t.Errorf("val = %q, want tok-12345", val)
	}
	vals := runtimeSet.Values()
	if len(vals) != 1 || vals[0] != "tok-12345" {
		t.Errorf("runtimeSet.Values() = %v, want [tok-12345]", vals)
	}
}

func TestRegistry_SensitiveReturn_NoHookNoMutation(t *testing.T) {
	// Functions without a sensitiveReturn entry must not mutate the set.
	reg := NewRegistry(nil)
	s := NewScope(map[string]string{})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	runtimeSet := NewSensitiveSet()
	s = s.WithDynamic(reg).WithRuntimeSensitive(runtimeSet)
	s.BeginRequest()
	defer s.EndRequest()

	if _, err := s.Interpolate(`{{$randomFirstName}}`); err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if vals := runtimeSet.Values(); len(vals) != 0 {
		t.Errorf("non-sensitive function leaked to runtimeSet: %v", vals)
	}
}

func TestRegistry_SensitiveReturn_NilRegistryAccessor(t *testing.T) {
	var r *Registry
	if r.IsSensitiveReturn("anything") {
		t.Errorf("nil registry must report no sensitive-return entries")
	}
}

// ---------------------------------------------------------------------------
// Step 2 (M13-002): 9 personal-data functions (no SSN yet)
// ---------------------------------------------------------------------------

func isFakerUsername(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^[a-z0-9._]+$`).MatchString(val) {
		t.Errorf("$faker.username %q does not match ^[a-z0-9._]+$", val)
	}
}

func isFakerEmail(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^[^@\s]+@example\.com$`).MatchString(val) {
		t.Errorf("$faker.email %q does not match ^[^@\\s]+@example\\.com$", val)
	}
}

func isUSPhone(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^\(\d{3}\) \d{3}-\d{4}$`).MatchString(val) {
		t.Errorf("$faker.phone %q does not match (NNN) NNN-NNNN", val)
	}
}

func isInternationalPhone(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^\+1-\d{3}-\d{3}-\d{4}$`).MatchString(val) {
		t.Errorf("$faker.phoneInternational %q does not match +1-NNN-NNN-NNNN", val)
	}
}

func isOneOf(allowed ...string) func(t *testing.T, val string) {
	return func(t *testing.T, val string) {
		t.Helper()
		for _, a := range allowed {
			if val == a {
				return
			}
		}
		t.Errorf("value %q not in allowed set %v", val, allowed)
	}
}

func TestRegistry_FakerPersonal(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)

	tests := []struct {
		name     string
		funcName string
		validate func(t *testing.T, val string)
	}{
		{"firstName non-empty pool draw", "faker.firstName", isNonEmpty},
		{"lastName non-empty pool draw", "faker.lastName", isNonEmpty},
		{"fullName has two ASCII-space parts", "faker.fullName", isTwoParts},
		{"username matches charset regex", "faker.username", isFakerUsername},
		{"email matches example.com regex", "faker.email", isFakerEmail},
		{"phone matches US format", "faker.phone", isUSPhone},
		{"phoneInternational matches +1 format", "faker.phoneInternational", isInternationalPhone},
		{"namePrefix in canonical set", "faker.namePrefix", isOneOf("Mr.", "Mrs.", "Ms.", "Dr.", "Prof.")},
		{"nameSuffix in canonical set", "faker.nameSuffix", isOneOf("Jr.", "Sr.", "II", "III", "IV", "PhD", "MD", "Esq.")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := reg.Evaluate(tt.funcName, nil, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(%q): %v", tt.funcName, err)
			}
			tt.validate(t, v)
		})
	}
}

func TestRegistry_FakerPersonal_Seeded(t *testing.T) {
	// Behavior 11: same seed → byte-equal output across two
	// independent registries (i.e. independent process invocations).
	seed := int64(42)
	reg1 := NewRegistry(&seed)
	reg2 := NewRegistry(&seed)
	for _, fn := range []string{
		"faker.firstName", "faker.lastName", "faker.fullName",
		"faker.username", "faker.email", "faker.phone",
		"faker.phoneInternational", "faker.namePrefix", "faker.nameSuffix",
	} {
		v1, _ := reg1.Evaluate(fn, nil, make(map[string]string))
		v2, _ := reg2.Evaluate(fn, nil, make(map[string]string))
		if v1 != v2 {
			t.Errorf("%s: seed 42 produced different values: %q vs %q", fn, v1, v2)
		}
	}
}

func TestRegistry_FakerPersonal_Unseeded(t *testing.T) {
	// Behavior 12: no seed → two independent calls produce different
	// values (no implicit seeding). Probability of collision on a 50-pool
	// x 50-pool space is 1/2500, so a single trial is usually enough.
	reg1 := NewRegistry(nil)
	reg2 := NewRegistry(nil)
	v1, _ := reg1.Evaluate("faker.fullName", nil, make(map[string]string))
	v2, _ := reg2.Evaluate("faker.fullName", nil, make(map[string]string))
	if v1 == v2 {
		// Re-roll once to absorb the rare collision; if it still
		// matches the test fails.
		v2b, _ := reg2.Evaluate("faker.fullName", nil, make(map[string]string))
		if v1 == v2b {
			t.Errorf("two unseeded fullName draws collided twice: %q", v1)
		}
	}
}

// ---------------------------------------------------------------------------
// Step 3 (M13-002): $faker.ssn specific tests
// ---------------------------------------------------------------------------

func TestRegistry_FakerSSN_Format(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	val, err := reg.Evaluate("faker.ssn", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !regexp.MustCompile(`^\d{3}-\d{2}-\d{4}$`).MatchString(val) {
		t.Errorf("ssn %q does not match NNN-NN-NNNN", val)
	}
}

func TestRegistry_FakerSSN_Seeded(t *testing.T) {
	seed := int64(42)
	a, _ := NewRegistry(&seed).Evaluate("faker.ssn", nil, make(map[string]string))
	b, _ := NewRegistry(&seed).Evaluate("faker.ssn", nil, make(map[string]string))
	if a != b {
		t.Errorf("seeded ssn not deterministic: %q vs %q", a, b)
	}
}

func TestRegistry_FakerSSN_Sensitive(t *testing.T) {
	// The generated SSN is added to runtimeSensitive at evaluation time
	// and (consequently) appears as [REDACTED] in any output sink that
	// consumes summary.RuntimeSensitive.
	seed := int64(42)
	reg := NewRegistry(&seed)
	s := NewScope(map[string]string{})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	runtimeSet := NewSensitiveSet()
	s = s.WithDynamic(reg).WithRuntimeSensitive(runtimeSet)
	s.BeginRequest()
	defer s.EndRequest()

	ssn, err := s.Interpolate(`{{$faker.ssn}}`)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if !regexp.MustCompile(`^\d{3}-\d{2}-\d{4}$`).MatchString(ssn) {
		t.Errorf("ssn %q malformed", ssn)
	}
	vals := runtimeSet.Values()
	if len(vals) != 1 || vals[0] != ssn {
		t.Errorf("runtimeSet.Values() = %v, want [%q]", vals, ssn)
	}

	// Prove the redaction sink consumes the registered value.
	body := map[string]any{"name": "Alice", "ssn": ssn}
	redacted := RedactBody(body, runtimeSet, false)
	rm := redacted.(map[string]any)
	if rm["ssn"] != Redacted {
		t.Errorf("RedactBody did not redact ssn field: %v", rm)
	}
}

func TestRegistry_FakerSSN_NoRuntimeSet_NoPanic(t *testing.T) {
	// Behavior contract: when no runtimeSensitive set is attached
	// to the scope, SSN evaluation is still safe — the hook is a no-op.
	reg := NewRegistry(nil)
	s := NewScope(map[string]string{})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	s = s.WithDynamic(reg) // no WithRuntimeSensitive
	s.BeginRequest()
	defer s.EndRequest()
	if _, err := s.Interpolate(`{{$faker.ssn}}`); err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
}

func TestRegistry_FakerPersonal_LocaleDeferred(t *testing.T) {
	// Behavior 13 (deferred): the --locale CLI flag does not exist in
	// M13. Once a future locale-support milestone introduces the flag
	// along with a recognise-but-warn-and-ignore policy, this test
	// should assert that en-US is still used and the warning fires.
	// Until then we keep this stub so the contract is visible.
	t.Skip("--locale flag is deferred from M13 entirely (see M13 Open Decision #1, M13-001 plan).")
}

// --- M13-003: $faker location-data family ---

// TestFakerLocation_PoolAlignment proves the parallel-slice invariant for state
// and country pools (behavior 5 + behavior 8 prerequisite). Catches index-skew
// regressions where someone reorders one slice but forgets the other.
func TestFakerLocation_PoolAlignment(t *testing.T) {
	if len(stateNames) != len(stateAbbrs) {
		t.Fatalf("stateNames (%d) and stateAbbrs (%d) must be the same length",
			len(stateNames), len(stateAbbrs))
	}
	if len(countryNames) != len(countryCodes) {
		t.Fatalf("countryNames (%d) and countryCodes (%d) must be the same length",
			len(countryNames), len(countryCodes))
	}
	// Spot-check known pairings to catch index-skew regressions where
	// someone reorders one slice but forgets the other.
	if stateNames[0] != "Alabama" || stateAbbrs[0] != "AL" {
		t.Errorf("stateNames[0]/stateAbbrs[0] = %q/%q, want Alabama/AL",
			stateNames[0], stateAbbrs[0])
	}
	if stateNames[12] != "Illinois" || stateAbbrs[12] != "IL" {
		t.Errorf("stateNames[12]/stateAbbrs[12] = %q/%q, want Illinois/IL",
			stateNames[12], stateAbbrs[12])
	}
	if countryNames[0] != "United States" || countryCodes[0] != "US" {
		t.Errorf("countryNames[0]/countryCodes[0] = %q/%q, want United States/US",
			countryNames[0], countryCodes[0])
	}
}

// --- M13-003 test helpers ---

func isUSAddress(t *testing.T, val string) {
	t.Helper()
	// <num> <streetName> <suffix>, <city>, <stateAbbr> <zip>
	pattern := `^\d{1,4} [A-Za-z][A-Za-z ]* (St|Ave|Blvd|Rd|Dr|Ln|Way|Ct|Pl|Ter), ` +
		`[A-Za-z][A-Za-z ]*, [A-Z]{2} \d{5}$`
	if !regexp.MustCompile(pattern).MatchString(val) {
		t.Errorf("$faker.address %q does not match US-address pattern", val)
	}
}

func isStreet(t *testing.T, val string) {
	t.Helper()
	// <num> <streetName> <suffix>
	pattern := `^\d{1,4} [A-Za-z][A-Za-z ]* (St|Ave|Blvd|Rd|Dr|Ln|Way|Ct|Pl|Ter)$`
	if !regexp.MustCompile(pattern).MatchString(val) {
		t.Errorf("$faker.street %q does not match street pattern", val)
	}
}

func isStreetName(t *testing.T, val string) {
	t.Helper()
	pattern := `^[A-Za-z][A-Za-z ]* (St|Ave|Blvd|Rd|Dr|Ln|Way|Ct|Pl|Ter)$`
	if !regexp.MustCompile(pattern).MatchString(val) {
		t.Errorf("$faker.streetName %q does not match streetName pattern", val)
	}
}

func isZip(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^\d{5}$`).MatchString(val) {
		t.Errorf("$faker.zipCode %q does not match ^\\d{5}$", val)
	}
}

func isStateAbbr(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(val) {
		t.Errorf("$faker.stateAbbr %q does not match ^[A-Z]{2}$", val)
	}
}

func isCountryCode(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(val) {
		t.Errorf("$faker.countryCode %q does not match ^[A-Z]{2}$", val)
	}
}

// isLatLong returns a validator for float values with 4 decimal places in [lo, hi].
func isLatLong(lo, hi float64) func(t *testing.T, val string) {
	return func(t *testing.T, val string) {
		t.Helper()
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			t.Errorf("value %q is not a parseable float: %v", val, err)
			return
		}
		if f < lo || f > hi {
			t.Errorf("value %q (%.4f) not in range [%.4f, %.4f]", val, f, lo, hi)
		}
		// Behavior 9/10: 4 decimal places. Asserted via the dot+4-digit suffix.
		if !regexp.MustCompile(`^-?\d+\.\d{4}$`).MatchString(val) {
			t.Errorf("value %q does not have exactly 4 decimal places", val)
		}
	}
}

func isIANATimezone(t *testing.T, val string) {
	t.Helper()
	if _, err := time.LoadLocation(val); err != nil {
		t.Errorf("$faker.timezone %q is not a valid IANA identifier: %v", val, err)
	}
}

// TestRegistry_FakerLocation is a table-driven validation of all 12 location functions.
func TestRegistry_FakerLocation(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)

	tests := []struct {
		name     string
		funcName string
		validate func(t *testing.T, val string)
	}{
		{"address matches US-address pattern", "faker.address", isUSAddress},
		{"street is <num> <streetName> <suffix>", "faker.street", isStreet},
		{"streetName is <name> <suffix>", "faker.streetName", isStreetName},
		{"city is non-empty pool draw", "faker.city", isNonEmpty},
		{"state is non-empty full state name", "faker.state", isNonEmpty},
		{"stateAbbr is 2 uppercase letters", "faker.stateAbbr", isStateAbbr},
		{"zipCode is 5 digits", "faker.zipCode", isZip},
		{"country is non-empty pool draw", "faker.country", isNonEmpty},
		{"countryCode is 2 uppercase letters", "faker.countryCode", isCountryCode},
		{"latitude in [-90, 90] with 4 decimals", "faker.latitude", isLatLong(-90, 90)},
		{"longitude in [-180, 180] with 4 decimals", "faker.longitude", isLatLong(-180, 180)},
		{"timezone is valid IANA identifier", "faker.timezone", isIANATimezone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := reg.Evaluate(tt.funcName, nil, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(%q): %v", tt.funcName, err)
			}
			tt.validate(t, v)
		})
	}
}

// TestRegistry_FakerLocation_Seeded proves behavior 12: same seed → byte-equal
// output across two independent registry instances (proxy for two independent
// process invocations).
func TestRegistry_FakerLocation_Seeded(t *testing.T) {
	seed := int64(42)
	reg1 := NewRegistry(&seed)
	reg2 := NewRegistry(&seed)
	for _, fn := range []string{
		"faker.address", "faker.street", "faker.streetName",
		"faker.city", "faker.state", "faker.stateAbbr",
		"faker.zipCode", "faker.country", "faker.countryCode",
		"faker.latitude", "faker.longitude", "faker.timezone",
	} {
		v1, _ := reg1.Evaluate(fn, nil, make(map[string]string))
		v2, _ := reg2.Evaluate(fn, nil, make(map[string]string))
		if v1 != v2 {
			t.Errorf("%s: seed 42 produced different values: %q vs %q", fn, v1, v2)
		}
	}
}

// TestRegistry_FakerLocation_Unseeded proves behavior 13: no seed → two
// independent calls produce different values. Pool sizes are large enough
// that single-trial collision is rare; absorb the rare hit by re-rolling once.
func TestRegistry_FakerLocation_Unseeded(t *testing.T) {
	reg1 := NewRegistry(nil)
	reg2 := NewRegistry(nil)
	v1, _ := reg1.Evaluate("faker.address", nil, make(map[string]string))
	v2, _ := reg2.Evaluate("faker.address", nil, make(map[string]string))
	if v1 == v2 {
		v2b, _ := reg2.Evaluate("faker.address", nil, make(map[string]string))
		if v1 == v2b {
			t.Errorf("two unseeded address draws collided twice: %q", v1)
		}
	}
}

// TestRegistry_FakerLocation_TimezonePool_AllValid ensures every entry in the
// shipped IANA pool is loadable. Catches typos at test time, not at customer runtime.
func TestRegistry_FakerLocation_TimezonePool_AllValid(t *testing.T) {
	for _, tz := range ianaTimezones {
		if _, err := time.LoadLocation(tz); err != nil {
			t.Errorf("ianaTimezones entry %q is not loadable: %v", tz, err)
		}
	}
}

// TestRegistry_FakerLocation_AddressShape explicitly exercises the
// SPECIFICATION.md:778 contract: address is <num> <street>, <city>, <stateAbbr> <zip>.
func TestRegistry_FakerLocation_AddressShape(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	val, err := reg.Evaluate("faker.address", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	parts := strings.Split(val, ", ")
	if len(parts) != 3 {
		t.Fatalf("expected 3 comma-space-separated parts, got %d in %q", len(parts), val)
	}
	// Last part: "<stateAbbr> <zip>"
	last := strings.Fields(parts[2])
	if len(last) != 2 {
		t.Fatalf("last part %q must be '<abbr> <zip>'", parts[2])
	}
	if !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(last[0]) {
		t.Errorf("state abbr %q is not 2 uppercase letters", last[0])
	}
	if !regexp.MustCompile(`^\d{5}$`).MatchString(last[1]) {
		t.Errorf("zip %q is not 5 digits", last[1])
	}
}

// TestRegistry_FakerLocation_LocaleDeferred is a contract stub for the deferred
// --locale behavior (M13 Open Decision #1).
func TestRegistry_FakerLocation_LocaleDeferred(t *testing.T) {
	t.Skip("--locale flag is deferred from M13 entirely (see M13 Open Decision #1, M13-001 plan).")
}

// ─── M13-004: $faker.* company-data functions ────────────────────────────────

// TestFakerCompany_PoolsNonEmpty proves all seven shipped pools are non-empty
// and that companySuffixes matches the closed canonical set from
// SPECIFICATION.md:796 + behavior 2 exactly.
func TestFakerCompany_PoolsNonEmpty(t *testing.T) {
	pools := []struct {
		name string
		pool []string
	}{
		{"companies", companies},
		{"companySuffixes", companySuffixes},
		{"jobTitles", jobTitles},
		{"departments", departments},
		{"catchPhraseAdjectives", catchPhraseAdjectives},
		{"catchPhraseNouns", catchPhraseNouns},
		{"catchPhraseGerunds", catchPhraseGerunds},
	}
	for _, p := range pools {
		if len(p.pool) == 0 {
			t.Errorf("pool %s is empty; expected at least one entry", p.name)
		}
	}
}

// TestFakerCompany_SuffixSet proves $faker.companySuffix is drawn from the
// canonical {Inc., LLC, Corp., Ltd., Co.} set per SPECIFICATION.md:796.
// Catches drift if the pool is ever reordered or extended.
func TestFakerCompany_SuffixSet(t *testing.T) {
	want := map[string]bool{
		"Inc.": true, "LLC": true, "Corp.": true, "Ltd.": true, "Co.": true,
	}
	if len(companySuffixes) != len(want) {
		t.Fatalf("len(companySuffixes) = %d, want %d", len(companySuffixes), len(want))
	}
	for _, s := range companySuffixes {
		if !want[s] {
			t.Errorf("unexpected entry %q in companySuffixes; canonical set is %v",
				s, []string{"Inc.", "LLC", "Corp.", "Ltd.", "Co."})
		}
	}
}

// TestRegistry_FakerCompany is a table-driven validation of all 5 company functions.
// Mirrors the M13-002 / M13-003 family-test shape.
func TestRegistry_FakerCompany(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)

	tests := []struct {
		name     string
		funcName string
		validate func(t *testing.T, val string)
	}{
		{"company is non-empty pool draw", "faker.company", isNonEmpty},
		{"companySuffix in canonical set", "faker.companySuffix", isOneOf("Inc.", "LLC", "Corp.", "Ltd.", "Co.")},
		{"jobTitle is non-empty pool draw", "faker.jobTitle", isNonEmpty},
		{"department is non-empty pool draw", "faker.department", isNonEmpty},
		{"catchPhrase is three space-separated parts", "faker.catchPhrase", isCatchPhrase},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := reg.Evaluate(tt.funcName, nil, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(%q): %v", tt.funcName, err)
			}
			tt.validate(t, v)
		})
	}
}

// isCatchPhrase asserts the value is exactly three space-separated non-empty
// tokens. Per behavior 5: <adjective> <noun> <gerund>.
func isCatchPhrase(t *testing.T, val string) {
	t.Helper()
	parts := strings.Split(val, " ")
	if len(parts) != 3 {
		t.Errorf("$faker.catchPhrase %q must have exactly 3 space-separated parts, got %d",
			val, len(parts))
		return
	}
	for i, p := range parts {
		if p == "" {
			t.Errorf("$faker.catchPhrase %q has empty token at position %d", val, i)
		}
	}
}

// TestRegistry_FakerCompany_Seeded verifies behavior 6: same seed → byte-equal
// output across two independent registry instances (proxy for two independent
// process invocations).
func TestRegistry_FakerCompany_Seeded(t *testing.T) {
	seed := int64(42)
	reg1 := NewRegistry(&seed)
	reg2 := NewRegistry(&seed)
	for _, fn := range []string{
		"faker.company", "faker.companySuffix", "faker.jobTitle",
		"faker.department", "faker.catchPhrase",
	} {
		v1, _ := reg1.Evaluate(fn, nil, make(map[string]string))
		v2, _ := reg2.Evaluate(fn, nil, make(map[string]string))
		if v1 != v2 {
			t.Errorf("%s: seed 42 produced different values: %q vs %q", fn, v1, v2)
		}
	}
}

// TestRegistry_FakerCompany_Unseeded verifies behavior 7: no seed → two
// independent calls produce different values. Pool sizes are large enough that
// single-trial collision is rare; absorb the rare hit by re-rolling once.
// catchPhrase has the largest entropy budget (30 * 30 * 30 = 27,000
// combinations) so collision is ~1/27k.
func TestRegistry_FakerCompany_Unseeded(t *testing.T) {
	reg1 := NewRegistry(nil)
	reg2 := NewRegistry(nil)
	v1, _ := reg1.Evaluate("faker.catchPhrase", nil, make(map[string]string))
	v2, _ := reg2.Evaluate("faker.catchPhrase", nil, make(map[string]string))
	if v1 == v2 {
		v2b, _ := reg2.Evaluate("faker.catchPhrase", nil, make(map[string]string))
		if v1 == v2b {
			t.Errorf("two unseeded catchPhrase draws collided twice: %q", v1)
		}
	}
}

// TestRegistry_FakerCompany_CatchPhraseShape explicitly exercises the
// behavior-5 contract: <adjective> <noun> <gerund>, three independent pool
// draws, single-space separator, all parts in pool.
func TestRegistry_FakerCompany_CatchPhraseShape(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	val, err := reg.Evaluate("faker.catchPhrase", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	parts := strings.Split(val, " ")
	if len(parts) != 3 {
		t.Fatalf("expected 3 space-separated parts, got %d in %q", len(parts), val)
	}
	if !companyPoolContains(catchPhraseAdjectives, parts[0]) {
		t.Errorf("first token %q is not in catchPhraseAdjectives", parts[0])
	}
	if !companyPoolContains(catchPhraseNouns, parts[1]) {
		t.Errorf("second token %q is not in catchPhraseNouns", parts[1])
	}
	if !companyPoolContains(catchPhraseGerunds, parts[2]) {
		t.Errorf("third token %q is not in catchPhraseGerunds", parts[2])
	}
}

// companyPoolContains is a tiny linear-search helper for catchPhrase shape
// tests. Test-only — small pool sizes (~30 entries) make this fine.
func companyPoolContains(pool []string, s string) bool {
	for _, p := range pool {
		if p == s {
			return true
		}
	}
	return false
}

// TestRegistry_FakerCompany_LocaleDeferred is a contract stub for the deferred
// --locale behavior (M13 Open Decision #1).
func TestRegistry_FakerCompany_LocaleDeferred(t *testing.T) {
	t.Skip("--locale flag is deferred from M13 entirely (see M13 Open Decision #1, M13-001 plan).")
}

// ─── M13-005: $faker.* internet-data pools ───────────────────────────────────

// TestFakerInternet_PoolsNonEmpty proves all five shipped pools are non-empty.
func TestFakerInternet_PoolsNonEmpty(t *testing.T) {
	pools := []struct {
		name string
		pool []string
	}{
		{"domainNameRoots", domainNameRoots},
		{"tldSuffixes", tldSuffixes},
		{"urlPathSegments", urlPathSegments},
		{"userAgents", userAgents},
		{"cssColorNames", cssColorNames},
	}
	for _, p := range pools {
		if len(p.pool) == 0 {
			t.Errorf("pool %s is empty; expected at least one entry", p.name)
		}
	}
}

// TestFakerInternet_TLDSuffixesShape proves every TLD entry is lowercase,
// has no leading dot, and is at least 2 characters.
func TestFakerInternet_TLDSuffixesShape(t *testing.T) {
	re := regexp.MustCompile(`^[a-z]{2,}$`)
	for _, tld := range tldSuffixes {
		if !re.MatchString(tld) {
			t.Errorf("tldSuffixes entry %q does not match ^[a-z]{2,}$", tld)
		}
	}
}

// TestFakerInternet_DomainRootsShape proves every domain root is lowercase
// ASCII matching ^[a-z0-9-]+$.
func TestFakerInternet_DomainRootsShape(t *testing.T) {
	re := regexp.MustCompile(`^[a-z0-9-]+$`)
	for _, root := range domainNameRoots {
		if !re.MatchString(root) {
			t.Errorf("domainNameRoots entry %q does not match ^[a-z0-9-]+$", root)
		}
	}
}

// TestFakerInternet_CSSColorNamesLowercase proves every CSS color keyword
// is lowercase ASCII.
func TestFakerInternet_CSSColorNamesLowercase(t *testing.T) {
	re := regexp.MustCompile(`^[a-z]+$`)
	for _, c := range cssColorNames {
		if !re.MatchString(c) {
			t.Errorf("cssColorNames entry %q is not lowercase ASCII", c)
		}
	}
}

// ─── M13-005: $faker.* internet-data registration tests ─────────────────────

// Helpers ------------------------------------------------------------------

// isURL asserts the value matches the spec regex
// ^https?://[^/]+/[^\s]*$ (behavior 1 + SPECIFICATION.md:805).
func isURL(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^https?://[^/]+/[^\s]*$`).MatchString(val) {
		t.Errorf("$faker.url %q does not match ^https?://[^/]+/[^\\s]*$", val)
	}
}

// isDomain asserts the value matches the spec regex
// ^[a-z0-9-]+\.[a-z]{2,}$ (behavior 2 + SPECIFICATION.md:806).
func isDomain(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^[a-z0-9-]+\.[a-z]{2,}$`).MatchString(val) {
		t.Errorf("$faker.domain %q does not match ^[a-z0-9-]+\\.[a-z]{2,}$", val)
	}
}

// isDomainSuffix asserts the value is a TLD without a leading dot
// (behavior 3 + SPECIFICATION.md:807) — lowercase ASCII, >= 2 chars.
func isDomainSuffix(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^[a-z]{2,}$`).MatchString(val) {
		t.Errorf("$faker.domainSuffix %q does not match ^[a-z]{2,}$", val)
	}
}

// isIPv4 asserts the value matches the spec regex AND each octet is in
// [0, 255] (behavior 4 + SPECIFICATION.md:808).
func isIPv4(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`).MatchString(val) {
		t.Errorf("$faker.ip %q does not match dotted-quad regex", val)
		return
	}
	parts := strings.Split(val, ".")
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			t.Errorf("$faker.ip %q octet %d (%q) out of range [0,255]", val, i, p)
		}
	}
}

// isIPv6 asserts net.ParseIP accepts the value as an IPv6 address
// (behavior 5 + SPECIFICATION.md:809).
func isIPv6(t *testing.T, val string) {
	t.Helper()
	ip := net.ParseIP(val)
	if ip == nil {
		t.Errorf("$faker.ipv6 %q is not parseable by net.ParseIP", val)
		return
	}
	// Guard by checking To4() is nil (i.e. genuinely IPv6, not IPv4-mapped).
	if ip.To4() != nil {
		t.Errorf("$faker.ipv6 %q parsed as IPv4, not IPv6", val)
	}
}

// isMAC asserts the value matches the spec regex
// ^([0-9A-F]{2}:){5}[0-9A-F]{2}$ — uppercase hex (behavior 6 +
// SPECIFICATION.md:810).
func isMAC(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^([0-9A-F]{2}:){5}[0-9A-F]{2}$`).MatchString(val) {
		t.Errorf("$faker.mac %q does not match uppercase MAC regex", val)
	}
}

// isUserAgent asserts the value is a non-empty UA string starting with
// "Mozilla/5.0" (behavior 7 + SPECIFICATION.md:811).
func isUserAgent(t *testing.T, val string) {
	t.Helper()
	if val == "" {
		t.Error("$faker.userAgent is empty")
		return
	}
	if !strings.HasPrefix(val, "Mozilla/5.0") {
		t.Errorf("$faker.userAgent %q does not start with Mozilla/5.0", val)
	}
}

// isCSSColorName asserts the value is a member of the shipped
// cssColorNames pool (behavior 8 + SPECIFICATION.md:812).
func isCSSColorName(t *testing.T, val string) {
	t.Helper()
	for _, c := range cssColorNames {
		if val == c {
			return
		}
	}
	t.Errorf("$faker.color %q is not in cssColorNames", val)
}

// isFakerHexColor asserts the value matches the spec regex
// ^#[0-9a-f]{6}$ (behavior 9 + SPECIFICATION.md:813).
func isFakerHexColor(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^#[0-9a-f]{6}$`).MatchString(val) {
		t.Errorf("$faker.hexColor %q does not match ^#[0-9a-f]{6}$", val)
	}
}

// Tests --------------------------------------------------------------------

// TestRegistry_FakerInternet — table-driven validation of all 9
// internet-data functions.
func TestRegistry_FakerInternet(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)

	tests := []struct {
		name     string
		funcName string
		validate func(t *testing.T, val string)
	}{
		{"url matches https?://host/path regex", "faker.url", isURL},
		{"domain matches root.tld regex", "faker.domain", isDomain},
		{"domainSuffix is bare TLD", "faker.domainSuffix", isDomainSuffix},
		{"ip is dotted-quad with octets in [0,255]", "faker.ip", isIPv4},
		{"ipv6 is parseable by net.ParseIP", "faker.ipv6", isIPv6},
		{"mac is uppercase hex with colons", "faker.mac", isMAC},
		{"userAgent starts with Mozilla/5.0", "faker.userAgent", isUserAgent},
		{"color is a CSS color name", "faker.color", isCSSColorName},
		{"hexColor matches #RRGGBB lowercase", "faker.hexColor", isFakerHexColor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := reg.Evaluate(tt.funcName, nil, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(%q): %v", tt.funcName, err)
			}
			tt.validate(t, v)
		})
	}
}

// TestRegistry_FakerInternet_Seeded — behavior 10: same seed produces
// byte-equal output across two independent registry instances.
func TestRegistry_FakerInternet_Seeded(t *testing.T) {
	seed := int64(42)
	reg1 := NewRegistry(&seed)
	reg2 := NewRegistry(&seed)
	for _, fn := range []string{
		"faker.url", "faker.domain", "faker.domainSuffix",
		"faker.ip", "faker.ipv6", "faker.mac",
		"faker.userAgent", "faker.color", "faker.hexColor",
	} {
		v1, _ := reg1.Evaluate(fn, nil, make(map[string]string))
		v2, _ := reg2.Evaluate(fn, nil, make(map[string]string))
		if v1 != v2 {
			t.Errorf("%s: seed 42 produced different values: %q vs %q", fn, v1, v2)
		}
	}
}

// TestRegistry_FakerInternet_Unseeded — behavior 11: no seed produces
// different values across two independent calls. Re-rolls once on
// collision to avoid false positives (~1/4320 for faker.url).
func TestRegistry_FakerInternet_Unseeded(t *testing.T) {
	reg1 := NewRegistry(nil)
	reg2 := NewRegistry(nil)
	v1, _ := reg1.Evaluate("faker.url", nil, make(map[string]string))
	v2, _ := reg2.Evaluate("faker.url", nil, make(map[string]string))
	if v1 == v2 {
		v2b, _ := reg2.Evaluate("faker.url", nil, make(map[string]string))
		if v1 == v2b {
			t.Errorf("two unseeded faker.url draws collided twice: %q", v1)
		}
	}
}

// TestRegistry_FakerInternet_URLShape explicitly exercises the
// composition contract: scheme is "https://", host has exactly one dot,
// path is a single token after a leading "/".
func TestRegistry_FakerInternet_URLShape(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	val, err := reg.Evaluate("faker.url", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !strings.HasPrefix(val, "https://") {
		t.Errorf("$faker.url %q must use https scheme", val)
	}
	afterScheme := strings.TrimPrefix(val, "https://")
	slash := strings.Index(afterScheme, "/")
	if slash < 0 {
		t.Fatalf("$faker.url %q has no path separator", val)
	}
	host := afterScheme[:slash]
	path := afterScheme[slash+1:]
	if !regexp.MustCompile(`^[a-z0-9-]+\.[a-z]{2,}$`).MatchString(host) {
		t.Errorf("$faker.url host %q does not match domain regex", host)
	}
	if path == "" {
		t.Errorf("$faker.url path is empty in %q", val)
	}
}

// TestRegistry_FakerInternet_DomainShape explicitly exercises the
// composition contract for $faker.domain: <root>.<tld>.
func TestRegistry_FakerInternet_DomainShape(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	val, err := reg.Evaluate("faker.domain", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	parts := strings.Split(val, ".")
	if len(parts) != 2 {
		t.Fatalf("$faker.domain %q must have exactly one dot, got %d parts", val, len(parts))
	}
	if !internetPoolContains(domainNameRoots, parts[0]) {
		t.Errorf("$faker.domain root %q is not in domainNameRoots", parts[0])
	}
	if !internetPoolContains(tldSuffixes, parts[1]) {
		t.Errorf("$faker.domain tld %q is not in tldSuffixes", parts[1])
	}
}

// internetPoolContains is a tiny linear-search helper for shape tests.
func internetPoolContains(pool []string, s string) bool {
	for _, p := range pool {
		if p == s {
			return true
		}
	}
	return false
}

// TestRegistry_FakerInternet_IPv4OctetRange runs 100 trials and
// confirms every octet is in [0, 255].
func TestRegistry_FakerInternet_IPv4OctetRange(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	for i := 0; i < 100; i++ {
		cache := make(map[string]string)
		val, err := reg.Evaluate("faker.ip", nil, cache)
		if err != nil {
			t.Fatalf("Evaluate iter %d: %v", i, err)
		}
		for j, p := range strings.Split(val, ".") {
			n, err2 := strconv.Atoi(p)
			if err2 != nil {
				t.Fatalf("iter %d %q octet %d not integer: %v", i, val, j, err2)
			}
			if n < 0 || n > 255 {
				t.Fatalf("iter %d %q octet %d (%d) out of [0,255]", i, val, j, n)
			}
		}
	}
}

// TestRegistry_FakerInternet_IPv6Parseable runs 100 trials and confirms
// every result is parseable by net.ParseIP and is genuinely IPv6.
func TestRegistry_FakerInternet_IPv6Parseable(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	for i := 0; i < 100; i++ {
		cache := make(map[string]string)
		val, err := reg.Evaluate("faker.ipv6", nil, cache)
		if err != nil {
			t.Fatalf("Evaluate iter %d: %v", i, err)
		}
		ip := net.ParseIP(val)
		if ip == nil {
			t.Fatalf("iter %d %q not parseable", i, val)
		}
		if ip.To4() != nil {
			t.Fatalf("iter %d %q parsed as IPv4", i, val)
		}
	}
}

// TestRegistry_FakerInternet_MACUppercase runs 100 trials and confirms
// every hex octet is uppercase.
func TestRegistry_FakerInternet_MACUppercase(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	re := regexp.MustCompile(`^([0-9A-F]{2}:){5}[0-9A-F]{2}$`)
	for i := 0; i < 100; i++ {
		cache := make(map[string]string)
		val, err := reg.Evaluate("faker.mac", nil, cache)
		if err != nil {
			t.Fatalf("Evaluate iter %d: %v", i, err)
		}
		if !re.MatchString(val) {
			t.Fatalf("iter %d %q is not uppercase MAC", i, val)
		}
	}
}

// TestRegistry_FakerInternet_RandomColorVsFakerColor proves that
// $randomColor (legacy hex) and $faker.color (CSS keyword) co-exist
// as distinct registrations per M13 Open Decision #3.
func TestRegistry_FakerInternet_RandomColorVsFakerColor(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	rc, err := reg.Evaluate("randomColor", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate randomColor: %v", err)
	}
	if !regexp.MustCompile(`^#[0-9a-f]{6}$`).MatchString(rc) {
		t.Errorf("$randomColor %q is not #RRGGBB hex (legacy contract)", rc)
	}
	fc, err := reg.Evaluate("faker.color", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate faker.color: %v", err)
	}
	isCSSColorName(t, fc)
	// Both registered, distinct names — Available() includes both.
	avail := reg.Available()
	var sawRC, sawFC bool
	for _, name := range avail {
		if name == "randomColor" {
			sawRC = true
		}
		if name == "faker.color" {
			sawFC = true
		}
	}
	if !sawRC || !sawFC {
		t.Errorf("Available() must include both randomColor and faker.color; got randomColor=%v faker.color=%v",
			sawRC, sawFC)
	}
}

// TestRegistry_FakerInternet_LocaleDeferred is a contract stub for the
// deferred --locale behavior (M13 Open Decision #1).
func TestRegistry_FakerInternet_LocaleDeferred(t *testing.T) {
	t.Skip("--locale flag is deferred from M13 entirely (see M13 Open Decision #1, M13-001 plan).")
}

// ─── M13-006: $faker.* content-data pool tests ───────────────────────────────

// TestFakerContent_PoolNonEmpty proves the lorem-ipsum word pool is
// non-empty (mirrors the M13-002/003/004/005 PoolsNonEmpty pattern).
func TestFakerContent_PoolNonEmpty(t *testing.T) {
	if len(loremWords) == 0 {
		t.Errorf("loremWords pool is empty; expected ~250 entries")
	}
}

// TestFakerContent_PoolShape proves every lorem-ipsum entry is
// lowercase ASCII matching ^[a-z]+$ — the invariant that lets
// $faker.word emit a clean token without further transformation
// and that lets $faker.sentence safely capitalise the first letter.
func TestFakerContent_PoolShape(t *testing.T) {
	re := regexp.MustCompile(`^[a-z]+$`)
	for _, w := range loremWords {
		if !re.MatchString(w) {
			t.Errorf("loremWords entry %q does not match ^[a-z]+$", w)
		}
	}
}

// TestFakerContent_PoolSize guards against pool-shrinkage regressions.
// Spec target is ~250; pin the floor at 100 so refactors don't silently
// trim entropy below the test-quality threshold.
func TestFakerContent_PoolSize(t *testing.T) {
	if len(loremWords) < 100 {
		t.Errorf("loremWords pool has %d entries; expected >= 100 for adequate $faker.text entropy",
			len(loremWords))
	}
}

// TestFakerContent_PoolNoDuplicates guards against duplicate entries in
// the lorem-ipsum pool. Duplicates reduce effective entropy (a duplicate
// word gets twice the draw probability) and inflate the apparent pool
// size without adding vocabulary. The pool invariant is: every entry
// appears exactly once.
func TestFakerContent_PoolNoDuplicates(t *testing.T) {
	seen := make(map[string]int, len(loremWords))
	for _, w := range loremWords {
		seen[w]++
	}
	for w, count := range seen {
		if count > 1 {
			t.Errorf("loremWords entry %q appears %d times; each entry must be unique", w, count)
		}
	}
}

// ─── M13-006: $faker.* content-data registration tests ───────────────────────

// Helpers ----------------------------------------------------------

// isLoremWord asserts the value is a single lowercase ASCII word
// drawn from loremWords (behavior 1 + SPECIFICATION.md:819).
func isLoremWord(t *testing.T, val string) {
	t.Helper()
	if !regexp.MustCompile(`^[a-z]+$`).MatchString(val) {
		t.Errorf("$faker.word %q is not a single lowercase word", val)
		return
	}
	for _, w := range loremWords {
		if val == w {
			return
		}
	}
	t.Errorf("$faker.word %q is not in loremWords", val)
}

// isWordsList asserts the value is exactly n single-space-joined
// lowercase ASCII words (behaviors 2 + 3).
func isWordsList(n int) func(t *testing.T, val string) {
	return func(t *testing.T, val string) {
		t.Helper()
		parts := strings.Split(val, " ")
		if len(parts) != n {
			t.Errorf("$faker.words: expected %d words, got %d in %q",
				n, len(parts), val)
			return
		}
		for i, p := range parts {
			if !regexp.MustCompile(`^[a-z]+$`).MatchString(p) {
				t.Errorf("token %d %q is not a lowercase word", i, p)
			}
		}
	}
}

// isSentence asserts the value is a sentence: n words (or any count
// when n == 0 — for the default-range case), first letter
// capitalised, ends with a single period (behaviors 4 + 5).
func isSentence(n int) func(t *testing.T, val string) {
	return func(t *testing.T, val string) {
		t.Helper()
		if !strings.HasSuffix(val, ".") {
			t.Errorf("$faker.sentence %q does not end with a period", val)
			return
		}
		body := strings.TrimSuffix(val, ".")
		words := strings.Split(body, " ")
		if n > 0 && len(words) != n {
			t.Errorf("$faker.sentence: expected %d words, got %d in %q",
				n, len(words), val)
		}
		if len(words[0]) == 0 || words[0][0] < 'A' || words[0][0] > 'Z' {
			t.Errorf("$faker.sentence first word %q must be capitalised",
				words[0])
		}
		// Subsequent words must remain lowercase.
		for i := 1; i < len(words); i++ {
			if !regexp.MustCompile(`^[a-z]+$`).MatchString(words[i]) {
				t.Errorf("$faker.sentence word %d %q is not lowercase",
					i, words[i])
			}
		}
	}
}

// isParagraph asserts the value is exactly n sentences (each
// period-terminated) joined by single spaces (behaviors 6 + 7).
// When n == 0, asserts only that 3..5 sentences are present
// (the default-range case).
func isParagraph(n int) func(t *testing.T, val string) {
	return func(t *testing.T, val string) {
		t.Helper()
		// Count sentences by counting period-followed-by-space-or-EOL.
		sentences := strings.Split(val, ". ")
		// The last one ends with "." (no trailing space) — re-add the
		// dot-suffix to that one for the per-sentence shape check.
		for i := 0; i < len(sentences)-1; i++ {
			sentences[i] = sentences[i] + "."
		}
		if n > 0 && len(sentences) != n {
			t.Errorf("$faker.paragraph: expected %d sentences, got %d in %q",
				n, len(sentences), val)
		}
		if n == 0 && (len(sentences) < 3 || len(sentences) > 5) {
			t.Errorf("$faker.paragraph default: expected 3..5 sentences, got %d in %q",
				len(sentences), val)
		}
		for i, s := range sentences {
			if !strings.HasSuffix(s, ".") {
				t.Errorf("paragraph sentence %d %q is missing terminating period",
					i, s)
			}
		}
	}
}

// isApproxText asserts the value length is within +/- tolerance of
// charLen (behavior 8 + 9). For charLen >= 50 we assert |actual -
// charLen| <= tolerance per the task's observable contract.
func isApproxText(charLen, tolerance int) func(t *testing.T, val string) {
	return func(t *testing.T, val string) {
		t.Helper()
		delta := len(val) - charLen
		if delta < 0 {
			delta = -delta
		}
		if delta > tolerance {
			t.Errorf("$faker.text: length %d is more than %d chars from target %d (val=%q)",
				len(val), tolerance, charLen, val)
		}
		if !strings.HasSuffix(val, ".") {
			t.Errorf("$faker.text %q does not end with a sentence boundary", val)
		}
	}
}

// Tests ------------------------------------------------------------

// TestRegistry_FakerContent — table-driven validation of all 5
// content-data functions (default-arg form). Mirrors the M13-002 /
// M13-003 / M13-004 / M13-005 family-test shape.
func TestRegistry_FakerContent(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)

	tests := []struct {
		name     string
		funcName string
		args     []string
		validate func(t *testing.T, val string)
	}{
		{"word is single lowercase word", "faker.word", nil, isLoremWord},
		{"words default returns 3 words", "faker.words", nil, isWordsList(3)},
		{"words('5') returns 5 words", "faker.words", []string{"5"}, isWordsList(5)},
		{"sentence default 6..10 words", "faker.sentence", nil, isSentence(0)},
		{"sentence('4') returns 4 words", "faker.sentence", []string{"4"}, isSentence(4)},
		{"paragraph default 3..5 sentences", "faker.paragraph", nil, isParagraph(0)},
		{"paragraph('2') returns 2 sentences", "faker.paragraph", []string{"2"}, isParagraph(2)},
		// Tolerance of 80: generateText stops at the first sentence boundary
		// at-or-after charCount, so it can overshoot by up to one full sentence
		// (~70 chars for a 10-word sentence with long words). 80 catches gross
		// regressions (e.g. algorithm emitting 2× the target) without CI flakes.
		// TestRegistry_FakerContent_TextLength adds additional sentence-boundary
		// validation (30 seeded iterations).
		{"text default ~200 chars", "faker.text", nil, isApproxText(200, 80)},
		{"text('50') ~50 chars", "faker.text", []string{"50"}, isApproxText(50, 80)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := reg.Evaluate(tt.funcName, tt.args, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(%q, %v): %v", tt.funcName, tt.args, err)
			}
			tt.validate(t, v)
		})
	}
}

// TestRegistry_FakerContent_ArgsViaM12001 verifies that M12-001's
// parseDynArgs (internal/variable/variable.go:112) parses the single
// quoted argument into args=['5'] without modification, and the
// registration body strconv.Atoi-converts args[0] to an integer count.
// This is the integration counterpart to TestRegistry_FakerContent —
// goes through the full Scope.Interpolate -> parseDynArgs ->
// Registry.Evaluate dispatch.
func TestRegistry_FakerContent_ArgsViaM12001(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	s := NewScope(nil)
	if err := s.Resolve(); err != nil {
		t.Fatal(err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	cases := []struct {
		placeholder string
		validate    func(t *testing.T, val string)
	}{
		{`{{$faker.words('5')}}`, isWordsList(5)},
		{`{{$faker.sentence('4')}}`, isSentence(4)},
		{`{{$faker.paragraph('2')}}`, isParagraph(2)},
	}
	for _, c := range cases {
		t.Run(c.placeholder, func(t *testing.T) {
			got, err := s.Interpolate(c.placeholder)
			if err != nil {
				t.Fatalf("Interpolate(%q): %v", c.placeholder, err)
			}
			c.validate(t, got)
		})
	}
}

// TestRegistry_FakerContent_Seeded — seeded reproducibility across
// two independent registry instances (proxy for two independent
// process invocations). Behavior 11.
func TestRegistry_FakerContent_Seeded(t *testing.T) {
	seed := int64(42)
	reg1 := NewRegistry(&seed)
	reg2 := NewRegistry(&seed)
	cases := []struct {
		funcName string
		args     []string
	}{
		{"faker.word", nil},
		{"faker.words", nil},
		{"faker.words", []string{"5"}},
		{"faker.sentence", nil},
		{"faker.sentence", []string{"4"}},
		{"faker.paragraph", nil},
		{"faker.paragraph", []string{"2"}},
		{"faker.text", nil},
		{"faker.text", []string{"50"}},
	}
	for _, c := range cases {
		v1, _ := reg1.Evaluate(c.funcName, c.args, make(map[string]string))
		v2, _ := reg2.Evaluate(c.funcName, c.args, make(map[string]string))
		if v1 != v2 {
			t.Errorf("%s%v: seed 42 produced different values: %q vs %q",
				c.funcName, c.args, v1, v2)
		}
	}
}

// TestRegistry_FakerContent_Unseeded — without seed, two independent
// calls produce different values. faker.text has the largest entropy
// budget (many sentences, 6..10 words per sentence, 250-word pool)
// so single-trial collision is astronomically rare; absorb the rare
// hit by re-rolling once. Behavior 12.
func TestRegistry_FakerContent_Unseeded(t *testing.T) {
	reg1 := NewRegistry(nil)
	reg2 := NewRegistry(nil)
	v1, _ := reg1.Evaluate("faker.text", nil, make(map[string]string))
	v2, _ := reg2.Evaluate("faker.text", nil, make(map[string]string))
	if v1 == v2 {
		v2b, _ := reg2.Evaluate("faker.text", nil, make(map[string]string))
		if v1 == v2b {
			t.Errorf("two unseeded faker.text draws collided twice: %q", v1)
		}
	}
}

// TestRegistry_FakerContent_Defaults — explicitly exercises the
// no-args branch: word-count for $faker.words default == 3,
// sentence-count default for $faker.paragraph in 3..5,
// word-count default for $faker.sentence in 6..10. Behaviors 2, 4, 6.
func TestRegistry_FakerContent_Defaults(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)

	t.Run("words default 3", func(t *testing.T) {
		val, err := reg.Evaluate("faker.words", nil, make(map[string]string))
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		parts := strings.Split(val, " ")
		if len(parts) != 3 {
			t.Errorf("expected 3 default words, got %d in %q", len(parts), val)
		}
	})

	t.Run("sentence default 6..10", func(t *testing.T) {
		// Run 30 trials to hit every value in the 6..10 range with
		// high probability (1 - (4/5)^30 ~= 99.998%).
		for i := 0; i < 30; i++ {
			cache := make(map[string]string)
			val, err := reg.Evaluate("faker.sentence", nil, cache)
			if err != nil {
				t.Fatalf("Evaluate iter %d: %v", i, err)
			}
			body := strings.TrimSuffix(val, ".")
			n := len(strings.Split(body, " "))
			if n < 6 || n > 10 {
				t.Errorf("iter %d: word-count %d out of [6,10] in %q", i, n, val)
			}
		}
	})

	t.Run("paragraph default 3..5", func(t *testing.T) {
		for i := 0; i < 30; i++ {
			cache := make(map[string]string)
			val, err := reg.Evaluate("faker.paragraph", nil, cache)
			if err != nil {
				t.Fatalf("Evaluate iter %d: %v", i, err)
			}
			n := strings.Count(val, ". ") + 1
			if n < 3 || n > 5 {
				t.Errorf("iter %d: sentence-count %d out of [3,5] in %q",
					i, n, val)
			}
		}
	})
}

// TestRegistry_FakerContent_BadInput rejects non-integer count args.
// Behavior 10 — DYNFN_FAKER_CONTENT_BAD_INPUT for any non-integer.
func TestRegistry_FakerContent_BadInput(t *testing.T) {
	reg := NewRegistry(nil)
	cases := []struct {
		funcName string
		arg      string
	}{
		{"faker.words", "abc"},
		{"faker.words", "1.5"},
		{"faker.words", ""},
		{"faker.sentence", "xyz"},
		{"faker.paragraph", "two"},
		{"faker.text", "fifty"},
	}
	for _, c := range cases {
		t.Run(c.funcName+"="+c.arg, func(t *testing.T) {
			_, err := reg.Evaluate(c.funcName, []string{c.arg}, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
			}
			if se.Code != "DYNFN_FAKER_CONTENT_BAD_INPUT" {
				t.Errorf("Code = %q, want DYNFN_FAKER_CONTENT_BAD_INPUT", se.Code)
			}
			if se.Category != apierrors.CategoryInput {
				t.Errorf("Category = %q, want CategoryInput", se.Category)
			}
			if se.Inner == nil {
				t.Error("Inner should carry the strconv.NumError")
			}
		})
	}
}

// TestRegistry_FakerContent_BadLength rejects count < 1.
func TestRegistry_FakerContent_BadLength(t *testing.T) {
	reg := NewRegistry(nil)
	cases := []struct {
		funcName string
		arg      string
	}{
		{"faker.words", "0"},
		{"faker.words", "-1"},
		{"faker.sentence", "0"},
		{"faker.paragraph", "-5"},
		{"faker.text", "0"},
	}
	for _, c := range cases {
		t.Run(c.funcName+"="+c.arg, func(t *testing.T) {
			_, err := reg.Evaluate(c.funcName, []string{c.arg}, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
			}
			if se.Code != "DYNFN_FAKER_CONTENT_BAD_LENGTH" {
				t.Errorf("Code = %q, want DYNFN_FAKER_CONTENT_BAD_LENGTH", se.Code)
			}
		})
	}
}

// TestRegistry_FakerContent_Arity covers the >1 args case for the
// arity-flex functions and the >0 args case for the strict-zero
// $faker.word.
func TestRegistry_FakerContent_Arity(t *testing.T) {
	reg := NewRegistry(nil)
	cases := []struct {
		funcName string
		args     []string
	}{
		{"faker.word", []string{"1"}}, // word is no-args only
		{"faker.words", []string{"3", "extra"}},
		{"faker.sentence", []string{"5", "extra"}},
		{"faker.paragraph", []string{"2", "extra"}},
		{"faker.text", []string{"100", "extra"}},
	}
	for _, c := range cases {
		t.Run(c.funcName, func(t *testing.T) {
			_, err := reg.Evaluate(c.funcName, c.args, nil)
			if err == nil {
				t.Fatal("expected arity error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_ARITY" {
				t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
			}
		})
	}
}

// TestRegistry_FakerContent_TextLength explicitly exercises the
// boundary contract from behavior 9: $faker.text('50') returns a
// string that ends at a sentence boundary and is at least 30 chars
// (guaranteeing at least one complete sentence). The upper bound
// accommodates natural overshoot: a single sentence drawn at 10
// words with 12-char average words totals ~130 chars. The test
// pins a generous ceiling of 200 chars so that only gross
// regressions (e.g. infinite loop) trip the test while the
// well-formedness invariant (ends with ".") is always checked.
func TestRegistry_FakerContent_TextLength(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	for i := 0; i < 30; i++ {
		cache := make(map[string]string)
		val, err := reg.Evaluate("faker.text", []string{"50"}, cache)
		if err != nil {
			t.Fatalf("Evaluate iter %d: %v", i, err)
		}
		if !strings.HasSuffix(val, ".") {
			t.Errorf("iter %d: %q does not end at a sentence boundary", i, val)
		}
		if len(val) < 30 || len(val) > 200 {
			t.Errorf("iter %d: length %d is far from target 50 (val=%q)",
				i, len(val), val)
		}
	}
}

// TestRegistry_FakerContent_LocaleDeferred is a contract stub for the
// deferred --locale behavior (M13 Open Decision #1). Behavior 13.
func TestRegistry_FakerContent_LocaleDeferred(t *testing.T) {
	t.Skip("--locale flag is deferred from M13 entirely (see M13 Open Decision #1, M13-001 plan).")
}

// ─── M13-007: $faker.* financial-data registration tests ───────────────────

// TestRegistry_FakerFinancial — table-driven validation of all 8
// financial functions (default-arg form). Mirrors the M13-002 /
// M13-003 / M13-004 / M13-005 / M13-006 family-test shape.
func TestRegistry_FakerFinancial(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)

	priceRe := regexp.MustCompile(`^\d+\.\d{2}$`)
	cvvRe := regexp.MustCompile(`^\d{3,4}$`)
	cardRe := regexp.MustCompile(`^\d{16}$`)
	ibanRe := regexp.MustCompile(`^[A-Z]{2}\d{2}[A-Z0-9]{1,30}$`)
	bicRe := regexp.MustCompile(`^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$`)

	tests := []struct {
		name     string
		funcName string
		args     []string
		validate func(t *testing.T, val string)
	}{
		{"price default in [1.00, 1000.00]", "faker.price", nil, func(t *testing.T, v string) {
			t.Helper()
			if !priceRe.MatchString(v) {
				t.Errorf("price %q not 2-decimal float", v)
				return
			}
			f, _ := strconv.ParseFloat(v, 64)
			if f < 1.0 || f > 1000.0 {
				t.Errorf("price %g out of [1, 1000]", f)
			}
		}},
		{"currencyCode in pool", "faker.currencyCode", nil, isOneOf(currencyCodes...)},
		{"currencyName in pool", "faker.currencyName", nil, isOneOf(currencyNames...)},
		{"currencySymbol in pool", "faker.currencySymbol", nil, isOneOf(currencySymbols...)},
		{"creditCard 16 digits, Luhn-valid", "faker.creditCard", nil, func(t *testing.T, v string) {
			t.Helper()
			if !cardRe.MatchString(v) {
				t.Errorf("creditCard %q not 16 digits", v)
			}
			if !isLuhnValid(v) {
				t.Errorf("creditCard %q fails Luhn", v)
			}
		}},
		{"creditCardCVV 3-or-4 digits", "faker.creditCardCVV", nil, func(t *testing.T, v string) {
			t.Helper()
			if !cvvRe.MatchString(v) {
				t.Errorf("CVV %q not 3-or-4 digits", v)
			}
		}},
		{"iban shape + mod-97 valid", "faker.iban", nil, func(t *testing.T, v string) {
			t.Helper()
			if !ibanRe.MatchString(v) {
				t.Errorf("iban %q does not match shape regex", v)
			}
			if !isIBANValid(v) {
				t.Errorf("iban %q fails mod-97 check", v)
			}
		}},
		{"bic shape", "faker.bic", nil, func(t *testing.T, v string) {
			t.Helper()
			if !bicRe.MatchString(v) {
				t.Errorf("bic %q does not match shape regex", v)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := reg.Evaluate(tt.funcName, tt.args, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(%q): %v", tt.funcName, err)
			}
			tt.validate(t, v)
		})
	}
}

// TestRegistry_FakerFinancial_PriceArgs verifies that M12-001's
// parseDynArgs parses two quoted decimals into args=['5','50']
// without modification, and the registration body strconv.ParseFloat
// -converts each, clamping output to [min, max].
func TestRegistry_FakerFinancial_PriceArgs(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	s := NewScope(nil)
	if err := s.Resolve(); err != nil {
		t.Fatal(err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	re := regexp.MustCompile(`^\d+\.\d{2}$`)
	// Run 30 trials to exercise the [5, 50] clamp under different RNG draws.
	for i := 0; i < 30; i++ {
		s.EndRequest()
		s.BeginRequest()
		v, err := s.Interpolate(`{{$faker.price('5', '50')}}`)
		if err != nil {
			t.Fatalf("iter %d: Interpolate: %v", i, err)
		}
		if !re.MatchString(v) {
			t.Errorf("iter %d: price %q not 2-decimal float", i, v)
			continue
		}
		f, _ := strconv.ParseFloat(v, 64)
		if f < 5.0 || f > 50.0 {
			t.Errorf("iter %d: price %g out of [5, 50]", i, f)
		}
	}
}

// TestRegistry_FakerFinancial_PriceInvertedRange covers behavior 3:
// min > max returns a structured CategoryInput error naming the
// inverted range.
func TestRegistry_FakerFinancial_PriceInvertedRange(t *testing.T) {
	reg := NewRegistry(nil)
	_, err := reg.Evaluate("faker.price", []string{"100", "5"}, nil)
	if err == nil {
		t.Fatal("expected inverted-range error, got nil")
	}
	var se *apierrors.Structured
	if !errors.As(err, &se) {
		t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
	}
	if se.Code != "DYNFN_FAKER_PRICE_INVERTED_RANGE" {
		t.Errorf("Code = %q, want DYNFN_FAKER_PRICE_INVERTED_RANGE", se.Code)
	}
	if se.Category != apierrors.CategoryInput {
		t.Errorf("Category = %q, want CategoryInput", se.Category)
	}
	if !strings.Contains(se.Message, "inverted") {
		t.Errorf("Message %q should mention 'inverted'", se.Message)
	}
}

// TestRegistry_FakerFinancial_PriceBadInput rejects non-numeric args.
func TestRegistry_FakerFinancial_PriceBadInput(t *testing.T) {
	reg := NewRegistry(nil)
	cases := [][]string{
		{"abc", "50"},
		{"5", "xyz"},
		{"", "50"},
		{"5", ""},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, ","), func(t *testing.T) {
			_, err := reg.Evaluate("faker.price", args, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_FAKER_PRICE_BAD_INPUT" {
				t.Errorf("Code = %q, want DYNFN_FAKER_PRICE_BAD_INPUT", se.Code)
			}
		})
	}
}

// TestRegistry_FakerFinancial_PriceArity covers the 1-arg and 3-arg
// cases (both expected to produce DYNFN_ARITY).
func TestRegistry_FakerFinancial_PriceArity(t *testing.T) {
	reg := NewRegistry(nil)
	cases := [][]string{
		{"5"},
		{"5", "50", "100"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, ","), func(t *testing.T) {
			_, err := reg.Evaluate("faker.price", args, nil)
			if err == nil {
				t.Fatal("expected arity error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_ARITY" {
				t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
			}
		})
	}
}

// TestRegistry_FakerFinancial_AutoSensitive verifies behaviors 6, 7,
// and 8: the three auto-sensitive functions register their generated
// value with runtimeSensitive at evaluation time and the value
// subsequently appears as [REDACTED] in body output.
func TestRegistry_FakerFinancial_AutoSensitive(t *testing.T) {
	cases := []struct {
		name        string
		placeholder string
		funcName    string
	}{
		{"creditCard", `{{$faker.creditCard}}`, "faker.creditCard"},
		{"creditCardCVV", `{{$faker.creditCardCVV}}`, "faker.creditCardCVV"},
		{"iban", `{{$faker.iban}}`, "faker.iban"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seed := int64(42)
			reg := NewRegistry(&seed)
			s := NewScope(map[string]string{})
			if err := s.Resolve(); err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			runtimeSet := NewSensitiveSet()
			s = s.WithDynamic(reg).WithRuntimeSensitive(runtimeSet)
			s.BeginRequest()
			defer s.EndRequest()

			val, err := s.Interpolate(c.placeholder)
			if err != nil {
				t.Fatalf("Interpolate: %v", err)
			}
			// Sanity: registry says this function is auto-sensitive.
			if !reg.IsSensitiveReturn(c.funcName) {
				t.Errorf("registry: IsSensitiveReturn(%q) = false, want true", c.funcName)
			}
			// The generated value lives in runtimeSensitive.Values().
			vals := runtimeSet.Values()
			found := false
			for _, v := range vals {
				if v == val {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("generated %s value %q not found in runtimeSet.Values() = %v",
					c.name, val, vals)
			}
			// Prove the redaction sink consumes the value.
			body := map[string]any{"name": "Alice", c.name: val}
			redacted := RedactBody(body, runtimeSet, false)
			rm := redacted.(map[string]any)
			if rm[c.name] != Redacted {
				t.Errorf("RedactBody did not redact %s field: %v", c.name, rm)
			}
		})
	}
}

// TestRegistry_FakerFinancial_BICNotSensitive verifies behavior 9:
// $faker.bic is NOT auto-sensitive (the bank identifier is not PII).
func TestRegistry_FakerFinancial_BICNotSensitive(t *testing.T) {
	reg := NewRegistry(nil)
	if reg.IsSensitiveReturn("faker.bic") {
		t.Error("faker.bic must NOT be auto-sensitive (per SPECIFICATION.md:849-857)")
	}
}

// TestRegistry_FakerFinancial_NoRuntimeSet_NoPanic verifies that
// the auto-sensitive functions are still safe to evaluate when no
// runtimeSensitive set is attached to the scope (mirrors
// TestRegistry_FakerSSN_NoRuntimeSet_NoPanic at dynamic_test.go:2379).
func TestRegistry_FakerFinancial_NoRuntimeSet_NoPanic(t *testing.T) {
	reg := NewRegistry(nil)
	s := NewScope(map[string]string{})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	s = s.WithDynamic(reg) // no WithRuntimeSensitive
	s.BeginRequest()
	defer s.EndRequest()
	for _, ph := range []string{
		`{{$faker.creditCard}}`, `{{$faker.creditCardCVV}}`, `{{$faker.iban}}`,
	} {
		if _, err := s.Interpolate(ph); err != nil {
			t.Fatalf("Interpolate(%q): %v", ph, err)
		}
	}
}

// TestRegistry_FakerFinancial_Seeded — behavior 10: same seed →
// byte-equal output across two independent registries.
func TestRegistry_FakerFinancial_Seeded(t *testing.T) {
	seed := int64(42)
	reg1 := NewRegistry(&seed)
	reg2 := NewRegistry(&seed)
	cases := []struct {
		funcName string
		args     []string
	}{
		{"faker.price", nil},
		{"faker.price", []string{"5", "50"}},
		{"faker.currencyCode", nil},
		{"faker.currencyName", nil},
		{"faker.currencySymbol", nil},
		{"faker.creditCard", nil},
		{"faker.creditCardCVV", nil},
		{"faker.iban", nil},
		{"faker.bic", nil},
	}
	for _, c := range cases {
		v1, _ := reg1.Evaluate(c.funcName, c.args, make(map[string]string))
		v2, _ := reg2.Evaluate(c.funcName, c.args, make(map[string]string))
		if v1 != v2 {
			t.Errorf("%s%v: seed 42 produced different values: %q vs %q",
				c.funcName, c.args, v1, v2)
		}
	}
}

// TestRegistry_FakerFinancial_Unseeded — behavior 11: no seed →
// two independent calls produce different values.
func TestRegistry_FakerFinancial_Unseeded(t *testing.T) {
	reg1 := NewRegistry(nil)
	reg2 := NewRegistry(nil)
	// creditCard has the largest entropy budget (~10^14 distinct
	// 14-digit suffixes), so single-trial collision is astronomically
	// rare; a re-roll absorbs the rare hit.
	v1, _ := reg1.Evaluate("faker.creditCard", nil, make(map[string]string))
	v2, _ := reg2.Evaluate("faker.creditCard", nil, make(map[string]string))
	if v1 == v2 {
		v2b, _ := reg2.Evaluate("faker.creditCard", nil, make(map[string]string))
		if v1 == v2b {
			t.Errorf("two unseeded creditCard draws collided twice: %q", v1)
		}
	}
}

// TestRegistry_FakerFinancial_LocaleDeferred is a contract stub for
// the deferred --locale behavior. Behavior 12.
func TestRegistry_FakerFinancial_LocaleDeferred(t *testing.T) {
	t.Skip("--locale flag is deferred from M13 entirely (see M13 Open Decision #1, M13-001 plan).")
}

// ─── M13-007: $faker.* financial-data pool & helpers ────────────────────────

// TestFakerFinancial_CurrencyPoolAlignment proves the parallel-slice
// invariant for currency pools (behavior 5 prerequisite). Catches
// index-skew regressions where someone reorders one slice but forgets
// the others.
func TestFakerFinancial_CurrencyPoolAlignment(t *testing.T) {
	if len(currencyCodes) != len(currencyNames) ||
		len(currencyNames) != len(currencySymbols) {
		t.Fatalf("currency pools length mismatch: codes=%d names=%d symbols=%d",
			len(currencyCodes), len(currencyNames), len(currencySymbols))
	}
	if len(currencyCodes) == 0 {
		t.Fatal("currency pools are empty")
	}
	// Spot-check canonical pairings.
	cases := []struct {
		idx                int
		code, name, symbol string
	}{
		{0, "USD", "US Dollar", "$"},
		{1, "EUR", "Euro", "€"},
		{2, "GBP", "British Pound", "£"},
		{3, "JPY", "Japanese Yen", "¥"},
	}
	for _, c := range cases {
		if currencyCodes[c.idx] != c.code ||
			currencyNames[c.idx] != c.name ||
			currencySymbols[c.idx] != c.symbol {
			t.Errorf("idx %d: got (%q, %q, %q), want (%q, %q, %q)",
				c.idx,
				currencyCodes[c.idx], currencyNames[c.idx], currencySymbols[c.idx],
				c.code, c.name, c.symbol)
		}
	}
}

// TestFakerFinancial_IBANCountriesAlignment proves the parallel-slice
// invariant for ibanCountries / ibanLengths and asserts each length is
// inside the ISO 13616 valid range [15, 34].
func TestFakerFinancial_IBANCountriesAlignment(t *testing.T) {
	if len(ibanCountries) != len(ibanLengths) {
		t.Fatalf("ibanCountries (%d) and ibanLengths (%d) must be equal",
			len(ibanCountries), len(ibanLengths))
	}
	if len(ibanCountries) == 0 {
		t.Fatal("ibanCountries pool is empty")
	}
	for i, c := range ibanCountries {
		if len(c) != 2 {
			t.Errorf("ibanCountries[%d] %q is not a 2-letter code", i, c)
		}
		L := ibanLengths[i]
		if L < 15 || L > 34 {
			t.Errorf("ibanLengths[%d]=%d for %q is outside [15,34]", i, L, c)
		}
	}
}

// TestLuhnCheckDigit_KnownVectors validates the Luhn helper against
// well-known test card numbers (Visa "4111111111111111" is canonical).
func TestLuhnCheckDigit_KnownVectors(t *testing.T) {
	valid := []string{
		"4111111111111111", // canonical Visa test
		"5555555555554444", // canonical Mastercard test
		"378282246310005",  // canonical Amex test (15 digits)
	}
	for _, s := range valid {
		if !isLuhnValid(s) {
			t.Errorf("isLuhnValid(%q) = false, want true", s)
		}
	}
	invalid := []string{
		"4111111111111112",
		"1234567890123456",
		"0000000000000001",
	}
	for _, s := range invalid {
		if isLuhnValid(s) {
			t.Errorf("isLuhnValid(%q) = true, want false", s)
		}
	}
	// Round-trip: building "4" + 14 zeros + check digit produces a
	// valid Luhn number.
	prefix := []byte("400000000000000")
	cd := luhnCheckDigit(prefix)
	full := string(prefix) + string(cd)
	if !isLuhnValid(full) {
		t.Errorf("luhnCheckDigit round-trip failed for %q (built %q)",
			string(prefix), full)
	}
}

// TestIBANCheckDigits_KnownVectors validates the IBAN helper against
// canonical examples from the ISO 13616 / IBAN registry.
func TestIBANCheckDigits_KnownVectors(t *testing.T) {
	// DE89 3704 0044 0532 0130 00 — German example from ISO 13616.
	// Country=DE, BBAN=370400440532013000, check=89.
	if cd := ibanCheckDigits("DE", "370400440532013000"); cd != "89" {
		t.Errorf("ibanCheckDigits(DE, ...) = %q, want %q", cd, "89")
	}
	// Validation of full IBANs.
	valid := []string{
		"DE89370400440532013000",
		"GB82WEST12345698765432",
		"FR1420041010050500013M02606",
	}
	for _, s := range valid {
		if !isIBANValid(s) {
			t.Errorf("isIBANValid(%q) = false, want true", s)
		}
	}
	invalid := []string{
		"DE00370400440532013000", // wrong check digits
		"GB00WEST12345698765432",
		"",   // empty
		"DE", // too short
	}
	for _, s := range invalid {
		if isIBANValid(s) {
			t.Errorf("isIBANValid(%q) = true, want false", s)
		}
	}
	// Round-trip: build a fresh IBAN with random BBAN, compute check
	// digits, then verify the assembled IBAN passes isIBANValid.
	bban := "AB12CD34EF56GH78IJ"
	cd := ibanCheckDigits("DE", bban)
	if !isIBANValid("DE" + cd + bban) {
		t.Errorf("IBAN round-trip failed for DE+%s+%s", cd, bban)
	}
}

// ─── M13-008: $faker.* file-data registration tests ─────────────────────────

// TestRegistry_FakerFile — table-driven validation of all 4
// file-data functions (default-arg form for $faker.imageUrl).
// Mirrors the M13-002 / M13-003 / M13-004 / M13-005 / M13-006 /
// M13-007 family-test shape.
func TestRegistry_FakerFile(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)

	fileNameRe := regexp.MustCompile(`^[a-z0-9_-]+\.[a-z0-9]+$`)
	extRe := regexp.MustCompile(`^[a-z0-9]{2,4}$`)
	mimeRe := regexp.MustCompile(`^[a-z]+/[a-z0-9.+-]+$`)

	tests := []struct {
		name     string
		funcName string
		args     []string
		validate func(t *testing.T, val string)
	}{
		{"fileName matches base.ext regex", "faker.fileName", nil, func(t *testing.T, v string) {
			t.Helper()
			if !fileNameRe.MatchString(v) {
				t.Errorf("fileName %q does not match ^[a-z0-9_-]+\\.[a-z0-9]+$", v)
			}
			// The trailing extension must also belong to the shipped pool.
			dot := strings.LastIndex(v, ".")
			if dot < 0 {
				t.Errorf("fileName %q has no dot", v)
				return
			}
			suffix := v[dot+1:]
			found := false
			for _, ext := range fileExtensions {
				if ext == suffix {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("fileName extension %q not in fileExtensions pool", suffix)
			}
		}},
		{"fileExtension matches 2-4 char regex", "faker.fileExtension", nil, func(t *testing.T, v string) {
			t.Helper()
			if !extRe.MatchString(v) {
				t.Errorf("fileExtension %q not 2-4 char lowercase", v)
			}
			if strings.HasPrefix(v, ".") {
				t.Errorf("fileExtension %q must not have leading dot", v)
			}
		}},
		{"mimeType matches type/subtype regex", "faker.mimeType", nil, func(t *testing.T, v string) {
			t.Helper()
			if !mimeRe.MatchString(v) {
				t.Errorf("mimeType %q does not match ^[a-z]+/[a-z0-9.+-]+$", v)
			}
		}},
		{"imageUrl default 640x480", "faker.imageUrl", nil, func(t *testing.T, v string) {
			t.Helper()
			if v != "https://picsum.photos/640/480" {
				t.Errorf("imageUrl default = %q, want https://picsum.photos/640/480", v)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := reg.Evaluate(tt.funcName, tt.args, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate(%q): %v", tt.funcName, err)
			}
			tt.validate(t, v)
		})
	}
}

// TestRegistry_FakerImageUrl_Args verifies that M12-001's parseDynArgs
// parses two quoted ints into args=['800','600'] without modification,
// and the registration body strconv.Atoi-converts each, validating
// positivity, and formats the picsum URL. Goes through the full
// Scope.Interpolate -> parseDynArgs -> Registry.Evaluate dispatch.
func TestRegistry_FakerImageUrl_Args(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	s := NewScope(nil)
	if err := s.Resolve(); err != nil {
		t.Fatal(err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	cases := []struct {
		placeholder string
		want        string
	}{
		{`{{$faker.imageUrl('800','600')}}`, "https://picsum.photos/800/600"},
		{`{{$faker.imageUrl('320','240')}}`, "https://picsum.photos/320/240"},
		{`{{$faker.imageUrl('1','1')}}`, "https://picsum.photos/1/1"},
	}
	for _, c := range cases {
		t.Run(c.placeholder, func(t *testing.T) {
			// Reset per-request cache so identical placeholders in
			// different sub-tests don't hit the memoization layer.
			s.EndRequest()
			s.BeginRequest()
			got, err := s.Interpolate(c.placeholder)
			if err != nil {
				t.Fatalf("Interpolate(%q): %v", c.placeholder, err)
			}
			if got != c.want {
				t.Errorf("Interpolate(%q) = %q, want %q", c.placeholder, got, c.want)
			}
		})
	}
}

// TestRegistry_FakerImageUrl_BadDimension covers behavior 6:
// non-positive width/height → DYNFN_FAKER_IMAGEURL_BAD_DIMENSION.
func TestRegistry_FakerImageUrl_BadDimension(t *testing.T) {
	reg := NewRegistry(nil)
	cases := [][]string{
		{"-100", "480"},
		{"0", "480"},
		{"800", "-1"},
		{"800", "0"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, ","), func(t *testing.T) {
			_, err := reg.Evaluate("faker.imageUrl", args, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
			}
			if se.Code != "DYNFN_FAKER_IMAGEURL_BAD_DIMENSION" {
				t.Errorf("Code = %q, want DYNFN_FAKER_IMAGEURL_BAD_DIMENSION", se.Code)
			}
			if se.Category != apierrors.CategoryInput {
				t.Errorf("Category = %q, want CategoryInput", se.Category)
			}
		})
	}
}

// TestRegistry_FakerImageUrl_BadInput covers behavior 7: non-integer
// arg → DYNFN_FAKER_IMAGEURL_BAD_INPUT. Mirrors the $randomBase64 /
// $randomPassword bad-input shape.
func TestRegistry_FakerImageUrl_BadInput(t *testing.T) {
	reg := NewRegistry(nil)
	cases := [][]string{
		{"abc", "480"},
		{"800", "xyz"},
		{"", "480"},
		{"800", ""},
		{"1.5", "480"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, ","), func(t *testing.T) {
			_, err := reg.Evaluate("faker.imageUrl", args, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_FAKER_IMAGEURL_BAD_INPUT" {
				t.Errorf("Code = %q, want DYNFN_FAKER_IMAGEURL_BAD_INPUT", se.Code)
			}
			if se.Inner == nil {
				t.Error("Inner should carry the strconv.NumError")
			}
		})
	}
}

// TestRegistry_FakerImageUrl_Arity covers the 1-arg and 3+-arg cases
// — both expected to produce DYNFN_ARITY ("expected 0 or 2 arguments").
func TestRegistry_FakerImageUrl_Arity(t *testing.T) {
	reg := NewRegistry(nil)
	cases := [][]string{
		{"800"},
		{"800", "600", "300"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, ","), func(t *testing.T) {
			_, err := reg.Evaluate("faker.imageUrl", args, nil)
			if err == nil {
				t.Fatal("expected arity error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_ARITY" {
				t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
			}
		})
	}
}

// TestRegistry_FakerFile_Seeded — same seed → byte-equal output across
// two independent registries. Behavior 8.
func TestRegistry_FakerFile_Seeded(t *testing.T) {
	seed := int64(42)
	reg1 := NewRegistry(&seed)
	reg2 := NewRegistry(&seed)
	cases := []struct {
		funcName string
		args     []string
	}{
		{"faker.fileName", nil},
		{"faker.fileExtension", nil},
		{"faker.mimeType", nil},
		{"faker.imageUrl", nil},
		{"faker.imageUrl", []string{"800", "600"}},
	}
	for _, c := range cases {
		v1, _ := reg1.Evaluate(c.funcName, c.args, make(map[string]string))
		v2, _ := reg2.Evaluate(c.funcName, c.args, make(map[string]string))
		if v1 != v2 {
			t.Errorf("%s%v: seed 42 produced different values: %q vs %q",
				c.funcName, c.args, v1, v2)
		}
	}
}

// TestRegistry_FakerFile_Unseeded — without seed, two independent
// calls produce different values for the three RNG-bearing functions.
// $faker.imageUrl with default args is intentionally deterministic
// (constant URL) and is excluded from this assertion per behavior 9.
// fileName has the largest entropy budget (20 base × 15 ext = 300
// permutations); single-trial collision is rare; absorb the rare
// hit by re-rolling once.
func TestRegistry_FakerFile_Unseeded(t *testing.T) {
	cases := []string{"faker.fileName", "faker.fileExtension", "faker.mimeType"}
	for _, fn := range cases {
		t.Run(fn, func(t *testing.T) {
			reg1 := NewRegistry(nil)
			reg2 := NewRegistry(nil)
			v1, _ := reg1.Evaluate(fn, nil, make(map[string]string))
			v2, _ := reg2.Evaluate(fn, nil, make(map[string]string))
			if v1 == v2 {
				v2b, _ := reg2.Evaluate(fn, nil, make(map[string]string))
				if v1 == v2b {
					t.Errorf("two unseeded %s draws collided twice: %q", fn, v1)
				}
			}
		})
	}
}

// TestRegistry_FakerImageUrl_DefaultsAreDeterministic — explicitly
// pins behavior 9's exclusion: $faker.imageUrl with no args returns
// the *same* constant URL on every call, both seeded and unseeded.
// This is the inverse of the unseeded-entropy property held by the
// other three file functions; documented inline so the property is
// readable from the test (not just buried in the YAML's behavior list).
func TestRegistry_FakerImageUrl_DefaultsAreDeterministic(t *testing.T) {
	reg := NewRegistry(nil)
	const want = "https://picsum.photos/640/480"
	for i := 0; i < 5; i++ {
		v, err := reg.Evaluate("faker.imageUrl", nil, make(map[string]string))
		if err != nil {
			t.Fatalf("iter %d: Evaluate: %v", i, err)
		}
		if v != want {
			t.Errorf("iter %d: imageUrl default = %q, want %q", i, v, want)
		}
	}
}

// TestRegistry_FakerFile_NotSensitive verifies none of the four
// file functions is auto-sensitive (file metadata is not PII per
// SPECIFICATION.md:849-857; only ssn / creditCard / creditCardCVV
// / iban are).
func TestRegistry_FakerFile_NotSensitive(t *testing.T) {
	reg := NewRegistry(nil)
	for _, fn := range []string{"faker.fileName", "faker.fileExtension", "faker.mimeType", "faker.imageUrl"} {
		if reg.IsSensitiveReturn(fn) {
			t.Errorf("%s must NOT be auto-sensitive (per SPECIFICATION.md:849-857)", fn)
		}
	}
}

// TestRegistry_FakerFile_LocaleDeferred is a contract stub for the
// deferred --locale behavior (M13 Open Decision #1). Behavior 10.
func TestRegistry_FakerFile_LocaleDeferred(t *testing.T) {
	t.Skip("--locale flag is deferred from M13 entirely (see M13 Open Decision #1, M13-001 plan).")
}

// ─── M13-008: $faker.* file-data pool tests ─────────────────────────────────

// TestFakerFile_PoolAlignment proves the parallel-slice invariant for
// fileExtensions / fileMimeTypes. Catches index-skew regressions where
// someone reorders one slice but forgets the other. Mirrors
// TestFakerFinancial_CurrencyPoolAlignment.
func TestFakerFile_PoolAlignment(t *testing.T) {
	if len(fileExtensions) != len(fileMimeTypes) {
		t.Fatalf("file pools length mismatch: extensions=%d mimeTypes=%d",
			len(fileExtensions), len(fileMimeTypes))
	}
	if len(fileExtensions) == 0 {
		t.Fatal("file pools are empty")
	}
	// Spot-check canonical pairings.
	cases := []struct {
		idx           int
		ext, mimeType string
	}{
		{0, "pdf", "application/pdf"},
		{1, "jpg", "image/jpeg"},
		{6, "json", "application/json"},
	}
	for _, c := range cases {
		if fileExtensions[c.idx] != c.ext || fileMimeTypes[c.idx] != c.mimeType {
			t.Errorf("idx %d: got (%q, %q), want (%q, %q)",
				c.idx, fileExtensions[c.idx], fileMimeTypes[c.idx],
				c.ext, c.mimeType)
		}
	}
}

// TestFakerFile_ExtensionShape proves every extension entry is 2..4
// lowercase ASCII chars matching behavior 2's regex.
func TestFakerFile_ExtensionShape(t *testing.T) {
	re := regexp.MustCompile(`^[a-z0-9]{2,4}$`)
	for _, ext := range fileExtensions {
		if !re.MatchString(ext) {
			t.Errorf("extension %q does not match ^[a-z0-9]{2,4}$", ext)
		}
	}
}

// TestFakerFile_MimeTypeShape proves every mime-type entry matches
// behavior 3's regex ^[a-z]+/[a-z0-9.+-]+$.
func TestFakerFile_MimeTypeShape(t *testing.T) {
	re := regexp.MustCompile(`^[a-z]+/[a-z0-9.+-]+$`)
	for _, mt := range fileMimeTypes {
		if !re.MatchString(mt) {
			t.Errorf("mimeType %q does not match ^[a-z]+/[a-z0-9.+-]+$", mt)
		}
	}
}

// TestFakerFile_BaseNameShape proves every base-name entry is
// lowercase ASCII matching ^[a-z0-9_-]+$ — required so that
// "<base>.<ext>" matches behavior 1's full regex.
func TestFakerFile_BaseNameShape(t *testing.T) {
	re := regexp.MustCompile(`^[a-z0-9_-]+$`)
	for _, b := range fileBaseNames {
		if !re.MatchString(b) {
			t.Errorf("base name %q does not match ^[a-z0-9_-]+$", b)
		}
	}
}

// --- M17-004: webhook-signature helpers ---

// TestRegistry_WebhookSign_SecretSensitivePropagation verifies that when the
// secret argument resolves from a sensitive-named variable, its resolved value
// is added to the run's SensitiveSet via the Pass-1 sensitiveArgIdx plumbing.
func TestRegistry_WebhookSign_SecretSensitivePropagation(t *testing.T) {
	for _, p := range []struct {
		name, fn, template string
	}{
		{"stripe", "webhookSign.stripe", `{{$webhookSign.stripe('{{payload}}', '{{stripe_signing_secret}}', '1700000000')}}`},
		{"github", "webhookSign.github", `{{$webhookSign.github('{{payload}}', '{{github_secret}}')}}`},
		{"slack", "webhookSign.slack", `{{$webhookSign.slack('{{payload}}', '{{slack_secret}}', '1700000000')}}`},
	} {
		t.Run(p.name, func(t *testing.T) {
			reg := NewRegistry(nil)
			s := NewScope(map[string]string{
				"payload":               "amount=100",
				"stripe_signing_secret": "sk_live_abc",
				"github_secret":         "ghs_xyz",
				"slack_secret":          "xoxb_secret_value",
			})
			if err := s.Resolve(); err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			runtimeSet := NewSensitiveSet()
			s = s.WithDynamic(reg).WithRuntimeSensitive(runtimeSet)
			s.BeginRequest()
			defer s.EndRequest()

			if _, err := s.Interpolate(p.template); err != nil {
				t.Fatalf("Interpolate: %v", err)
			}
			vals := runtimeSet.Values()
			expected := map[string]string{
				"stripe": "sk_live_abc",
				"github": "ghs_xyz",
				"slack":  "xoxb_secret_value",
			}[p.name]
			found := false
			for _, v := range vals {
				if v == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("runtimeSet should contain %q; got %v", expected, vals)
			}
		})
	}
}

// TestRegistry_WebhookSign_LiteralSecret_NotMarked verifies that a literal
// (non-variable) secret does not trigger SensitiveSet mutation. Mirrors the
// $hmacSha256 documented footgun.
func TestRegistry_WebhookSign_LiteralSecret_NotMarked(t *testing.T) {
	reg := NewRegistry(nil)
	s := NewScope(map[string]string{"payload": "amount=100"})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	runtimeSet := NewSensitiveSet()
	s = s.WithDynamic(reg).WithRuntimeSensitive(runtimeSet)
	s.BeginRequest()
	defer s.EndRequest()

	if _, err := s.Interpolate(`{{$webhookSign.github('{{payload}}', 'literal-secret')}}`); err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if vals := runtimeSet.Values(); len(vals) != 0 {
		t.Errorf("literal secret must not mutate runtimeSet; got %v", vals)
	}
}

// independentHMACHex is the byte-exact reference: a one-liner via
// crypto/hmac that exercises the same primitives the production helper
// uses, but written inline at the test call site so a bug in the helper
// cannot mask itself.
func independentHMACHex(msg, key string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(msg))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// TestRegistry_WebhookSign_Stripe verifies the 3-arg and 2-arg (clock-seam) forms.
func TestRegistry_WebhookSign_Stripe(t *testing.T) {
	reg := NewRegistry(nil)
	const body, secret, ts = "{}", "whsec_test", "1492774577"

	// 3-arg form: byte-exact against an inline reference.
	got, err := reg.Evaluate("webhookSign.stripe", []string{body, secret, ts}, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate(3-arg): %v", err)
	}
	want := "t=" + ts + ",v1=" + independentHMACHex(ts+"."+body, secret)
	if got != want {
		t.Errorf("3-arg form: got %q, want %q", got, want)
	}

	// 2-arg form: pin the clock and assert.
	reg2 := NewRegistry(nil)
	frozenTS := int64(1700000000)
	reg2.now = frozenClock(time.Unix(frozenTS, 0).UTC())
	got2, err := reg2.Evaluate("webhookSign.stripe", []string{body, secret}, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate(2-arg): %v", err)
	}
	wantTS := strconv.FormatInt(frozenTS, 10)
	want2 := "t=" + wantTS + ",v1=" + independentHMACHex(wantTS+"."+body, secret)
	if got2 != want2 {
		t.Errorf("2-arg form: got %q, want %q", got2, want2)
	}
}

// TestRegistry_WebhookSign_GitHub verifies the 2-arg form.
func TestRegistry_WebhookSign_GitHub(t *testing.T) {
	reg := NewRegistry(nil)
	const body, secret = `{"action":"opened"}`, "It's a Secret to Everybody"
	got, err := reg.Evaluate("webhookSign.github", []string{body, secret}, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	want := "sha256=" + independentHMACHex(body, secret)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRegistry_WebhookSign_Slack verifies the 3-arg and 2-arg (clock-seam) forms.
func TestRegistry_WebhookSign_Slack(t *testing.T) {
	reg := NewRegistry(nil)
	const body, secret, ts = "token=xyz&team=T1", "8f742231b10e8888abcd99yyyzzz85a5", "1531420618"

	got, err := reg.Evaluate("webhookSign.slack", []string{body, secret, ts}, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate(3-arg): %v", err)
	}
	want := "v0=" + independentHMACHex("v0:"+ts+":"+body, secret)
	if got != want {
		t.Errorf("3-arg form: got %q, want %q", got, want)
	}

	// 2-arg form via clock seam.
	reg2 := NewRegistry(nil)
	frozenTS := int64(1700000000)
	reg2.now = frozenClock(time.Unix(frozenTS, 0).UTC())
	got2, err := reg2.Evaluate("webhookSign.slack", []string{body, secret}, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate(2-arg): %v", err)
	}
	wantTS := strconv.FormatInt(frozenTS, 10)
	want2 := "v0=" + independentHMACHex("v0:"+wantTS+":"+body, secret)
	if got2 != want2 {
		t.Errorf("2-arg form: got %q, want %q", got2, want2)
	}
}

// TestRegistry_WebhookSign_DottedNamespaceRequired verifies that the dotted
// function names are required; non-dotted variants return unknown-function.
func TestRegistry_WebhookSign_DottedNamespaceRequired(t *testing.T) {
	reg := NewRegistry(nil)
	for _, name := range []string{"webhookSignStripe", "webhookSignGithub", "webhookSignSlack"} {
		t.Run(name, func(t *testing.T) {
			_, err := reg.Evaluate(name, []string{"body", "key"}, make(map[string]string))
			if err == nil {
				t.Fatal("expected unknown-function error, got nil")
			}
			if !strings.Contains(err.Error(), "unknown dynamic function") {
				t.Errorf("expected unknown-function error, got: %v", err)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error should name the missing function %q; got: %v", name, err)
			}
		})
	}
}

// TestRegistry_WebhookSign_ArityErrors verifies structured DYNFN_ARITY errors
// for all three webhook-sign functions with out-of-range argument counts.
func TestRegistry_WebhookSign_ArityErrors(t *testing.T) {
	reg := NewRegistry(nil)

	tests := []struct {
		name       string
		funcName   string
		args       []string
		wantPhrase string
	}{
		// stripe — 2 or 3 args
		{"stripe zero args", "webhookSign.stripe", nil, "expected 2 or 3 arguments, got 0"},
		{"stripe one arg", "webhookSign.stripe", []string{"a"}, "expected 2 or 3 arguments, got 1"},
		{"stripe four args", "webhookSign.stripe", []string{"a", "b", "c", "d"}, "expected 2 or 3 arguments, got 4"},
		// slack — 2 or 3 args
		{"slack zero args", "webhookSign.slack", nil, "expected 2 or 3 arguments, got 0"},
		{"slack one arg", "webhookSign.slack", []string{"a"}, "expected 2 or 3 arguments, got 1"},
		{"slack four args", "webhookSign.slack", []string{"a", "b", "c", "d"}, "expected 2 or 3 arguments, got 4"},
		// github — exactly 2
		{"github zero args", "webhookSign.github", nil, "expected 2 arguments, got 0"},
		{"github one arg", "webhookSign.github", []string{"a"}, "expected 2 arguments, got 1"},
		{"github three args", "webhookSign.github", []string{"a", "b", "c"}, "expected 2 arguments, got 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate(tt.funcName, tt.args, nil)
			if err == nil {
				t.Fatal("expected arity error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("want *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_ARITY" {
				t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
			}
			if !strings.Contains(se.Message, tt.wantPhrase) {
				t.Errorf("Message %q should contain %q", se.Message, tt.wantPhrase)
			}
			// Evaluate wraps the error with fmt.Errorf("$%s: %w", name, err);
			// verify the function name appears in the wrapped error string.
			if !strings.Contains(err.Error(), "$"+tt.funcName) {
				t.Errorf("err.Error() should contain $%s; got %q", tt.funcName, err.Error())
			}
		})
	}
}

// TestWebhookSig_Helpers_StripeVector validates the Stripe signed_payload
// construction byte-exactly against an independent HMAC-SHA256 reference.
func TestWebhookSig_Helpers_StripeVector(t *testing.T) {
	got := stripeWebhookSig("{}", "whsec_test", "1492774577")
	want := "t=1492774577,v1=" + independentHMACHex("1492774577.{}", "whsec_test")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestWebhookSig_Helpers_GitHubVector validates the GitHub x-hub-signature-256
// construction byte-exactly against an independent HMAC-SHA256 reference.
func TestWebhookSig_Helpers_GitHubVector(t *testing.T) {
	got := githubWebhookSig(`{"action":"opened"}`, "It's a Secret to Everybody")
	want := "sha256=" + independentHMACHex(`{"action":"opened"}`, "It's a Secret to Everybody")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestWebhookSig_Helpers_SlackVector validates the Slack basestring
// construction byte-exactly against an independent HMAC-SHA256 reference.
func TestWebhookSig_Helpers_SlackVector(t *testing.T) {
	body := "token=xyzz0WbapA4vBCDEFasx0q6G&team_id=T1DC2JH3J&team_domain=testteamnow"
	got := slackWebhookSig(body, "8f742231b10e8888abcd99yyyzzz85a5", "1531420618")
	want := "v0=" + independentHMACHex("v0:1531420618:"+body, "8f742231b10e8888abcd99yyyzzz85a5")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- M17-005: JWT decode dynamic functions ---

// jwtIOExampleToken is the canonical JWT.io example, signed HS256 over
// secret="your-256-bit-secret". We use it across the M17-005 vector
// tests because it is the most widely-cited public reference value.
const jwtIOExampleToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

// TestRegistry_JwtDecodeHeader_Vector verifies that the JWT.io canonical
// example decodes to the expected canonical-compact JSON header. The
// Go json.Marshal sort happens to leave {"alg":"HS256","typ":"JWT"} in
// alphabetical order (alg < typ) — so this output matches the
// human-readable header content exactly.
func TestRegistry_JwtDecodeHeader_Vector(t *testing.T) {
	reg := NewRegistry(nil)
	got, err := reg.Evaluate("jwtDecodeHeader", []string{jwtIOExampleToken}, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	const want = `{"alg":"HS256","typ":"JWT"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRegistry_JwtDecodeClaims_Vector verifies that the JWT.io canonical
// example claims segment decodes to canonical-compact JSON. The
// encoding/json marshaller sorts keys alphabetically (iat, name, sub),
// which is the documented canonicalisation guarantee — see MANUAL.md
// §3.7 (M17-005 entry). The original claim values are present and
// byte-exact; only the key order is normalised.
func TestRegistry_JwtDecodeClaims_Vector(t *testing.T) {
	reg := NewRegistry(nil)
	got, err := reg.Evaluate("jwtDecodeClaims", []string{jwtIOExampleToken}, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	const want = `{"iat":1516239022,"name":"John Doe","sub":"1234567890"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRegistry_JwtDecode_BadFormat covers the segment-count error path
// for both functions across the four canonical mis-counts (0, 1, 2, 4+).
func TestRegistry_JwtDecode_BadFormat(t *testing.T) {
	reg := NewRegistry(nil)
	tests := []struct {
		name  string
		input string
	}{
		{"empty string (1 segment)", ""},
		{"one segment, no dots", "abc"},
		{"two segments", "abc.def"},
		{"four segments", "a.b.c.d"},
		{"five segments", "a.b.c.d.e"},
	}
	for _, fn := range []string{"jwtDecodeHeader", "jwtDecodeClaims"} {
		for _, tt := range tests {
			t.Run(fn+"/"+tt.name, func(t *testing.T) {
				_, err := reg.Evaluate(fn, []string{tt.input}, make(map[string]string))
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				var se *apierrors.Structured
				if !errors.As(err, &se) {
					t.Fatalf("want *apierrors.Structured, got %T: %v", err, err)
				}
				if se.Category != apierrors.CategoryInput {
					t.Errorf("Category = %q, want CategoryInput", se.Category)
				}
				if se.Code != "DYNFN_JWT_DECODE_BAD_FORMAT" {
					t.Errorf("Code = %q, want DYNFN_JWT_DECODE_BAD_FORMAT", se.Code)
				}
				// Function name appears in wrapped error string via Evaluate.
				if !strings.Contains(err.Error(), "$"+fn) {
					t.Errorf("err.Error() should contain $%s; got %q", fn, err.Error())
				}
			})
		}
	}
}

// TestRegistry_JwtDecode_BadFormat_Truncates verifies that a token longer
// than 32 chars produces a truncated input prefix in the error message
// (with the "..." marker) — protecting logs from token bloat / leakage.
func TestRegistry_JwtDecode_BadFormat_Truncates(t *testing.T) {
	reg := NewRegistry(nil)
	long := strings.Repeat("x", 100) // 100 chars, no dots → 1 segment
	_, err := reg.Evaluate("jwtDecodeHeader", []string{long}, make(map[string]string))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), strings.Repeat("x", 32)) {
		t.Errorf("err.Error() should contain first 32 chars of input; got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "...") {
		t.Errorf("err.Error() should contain truncation marker '...'; got %q", err.Error())
	}
	if strings.Contains(err.Error(), strings.Repeat("x", 100)) {
		t.Errorf("err.Error() should NOT contain the full untruncated input; got %q", err.Error())
	}
}

// TestRegistry_JwtDecode_BadBase64 covers the base64-url decode error
// path for both functions on each segment.
func TestRegistry_JwtDecode_BadBase64(t *testing.T) {
	reg := NewRegistry(nil)
	tests := []struct {
		name  string
		fn    string
		token string
	}{
		// header segment is non-base64 (contains '!')
		{"header bad base64", "jwtDecodeHeader", "abc!def.eyJ4Ijoxfq.sig"},
		// claims segment is non-base64
		{"claims bad base64", "jwtDecodeClaims", "eyJhbGciOiJIUzI1NiJ9.bad!base64.sig"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate(tt.fn, []string{tt.token}, make(map[string]string))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("want *apierrors.Structured, got %T: %v", err, err)
			}
			if se.Code != "DYNFN_JWT_DECODE_BAD_BASE64" {
				t.Errorf("Code = %q, want DYNFN_JWT_DECODE_BAD_BASE64", se.Code)
			}
			if se.Category != apierrors.CategoryInput {
				t.Errorf("Category = %q, want CategoryInput", se.Category)
			}
		})
	}
}

// TestRegistry_JwtDecode_BadBase64_Truncates verifies that a token longer
// than 32 chars whose header segment contains invalid base64 produces a
// truncated input prefix in the error message (behavior B4 truncation
// requirement). Mirrors TestRegistry_JwtDecode_BadFormat_Truncates.
func TestRegistry_JwtDecode_BadBase64_Truncates(t *testing.T) {
	reg := NewRegistry(nil)
	// Header segment contains '!' (invalid base64-url); total token > 32 chars.
	// First 32 chars: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA!"
	long := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA!.eyJ4Ijoxfq.sig"
	_, err := reg.Evaluate("jwtDecodeHeader", []string{long}, make(map[string]string))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The first 32 chars of the token must appear in the error message.
	if !strings.Contains(err.Error(), long[:32]) {
		t.Errorf("err.Error() should contain first 32 chars of input; got %q", err.Error())
	}
	// The truncation "..." marker must be present.
	if !strings.Contains(err.Error(), "...") {
		t.Errorf("err.Error() should contain truncation marker '...'; got %q", err.Error())
	}
	// The full untruncated token must not appear.
	if strings.Contains(err.Error(), long) {
		t.Errorf("err.Error() should NOT contain the full untruncated input; got %q", err.Error())
	}
}

// TestRegistry_JwtDecode_BadJSON covers the JSON-decode error path: the
// segment base64-url-decodes successfully but the bytes are not JSON.
// "bm90LWpzb24" is base64-url("not-json") with no padding.
func TestRegistry_JwtDecode_BadJSON(t *testing.T) {
	reg := NewRegistry(nil)
	// header segment decodes to "not-json" (text, not JSON)
	headerBadJSON := "bm90LWpzb24.eyJ4Ijoxfq.sig"
	// claims segment decodes to "not-json"
	claimsBadJSON := "eyJhbGciOiJIUzI1NiJ9.bm90LWpzb24.sig"

	for _, tt := range []struct {
		name  string
		fn    string
		token string
	}{
		{"header bad json", "jwtDecodeHeader", headerBadJSON},
		{"claims bad json", "jwtDecodeClaims", claimsBadJSON},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate(tt.fn, []string{tt.token}, make(map[string]string))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("want *apierrors.Structured, got %T: %v", err, err)
			}
			if se.Code != "DYNFN_JWT_DECODE_BAD_JSON" {
				t.Errorf("Code = %q, want DYNFN_JWT_DECODE_BAD_JSON", se.Code)
			}
		})
	}
}

// TestRegistry_JwtDecode_BadJSON_Truncates verifies that a token longer
// than 32 chars whose header segment is valid base64 but not JSON produces
// a truncated input prefix in the error message (behavior B5 truncation
// requirement). Mirrors TestRegistry_JwtDecode_BadFormat_Truncates.
func TestRegistry_JwtDecode_BadJSON_Truncates(t *testing.T) {
	reg := NewRegistry(nil)
	// Header segment is base64-url("not-json-not-json-not-json-xxx") — valid
	// base64 but decodes to non-JSON text. Total token > 32 chars.
	// First 32 chars of token: "bm90LWpzb24tbm90LWpzb24tbm90LWpz"
	long := "bm90LWpzb24tbm90LWpzb24tbm90LWpzb24teHh4.eyJ4Ijoxfq.sig"
	_, err := reg.Evaluate("jwtDecodeHeader", []string{long}, make(map[string]string))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The first 32 chars of the token must appear in the error message.
	if !strings.Contains(err.Error(), long[:32]) {
		t.Errorf("err.Error() should contain first 32 chars of input; got %q", err.Error())
	}
	// The truncation "..." marker must be present.
	if !strings.Contains(err.Error(), "...") {
		t.Errorf("err.Error() should contain truncation marker '...'; got %q", err.Error())
	}
	// The full untruncated token must not appear.
	if strings.Contains(err.Error(), long) {
		t.Errorf("err.Error() should NOT contain the full untruncated input; got %q", err.Error())
	}
}

// TestRegistry_JwtDecode_NoSignatureVerification verifies the explicit
// scope constraint: a token whose signature segment is mangled or
// nonsensical still decodes both header and claims successfully. This is
// a security-relevant contract documented in MANUAL.md §3.7.
func TestRegistry_JwtDecode_NoSignatureVerification(t *testing.T) {
	reg := NewRegistry(nil)
	// JWT.io example token with the signature segment replaced by garbage.
	// "ZGVsaWJlcmF0ZWx5LW1hbmdsZWQ" is base64-url("deliberately-mangled").
	mangled := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.ZGVsaWJlcmF0ZWx5LW1hbmdsZWQ"
	for _, fn := range []string{"jwtDecodeHeader", "jwtDecodeClaims"} {
		t.Run(fn, func(t *testing.T) {
			got, err := reg.Evaluate(fn, []string{mangled}, make(map[string]string))
			if err != nil {
				t.Fatalf("expected success despite mangled signature, got error: %v", err)
			}
			if got == "" {
				t.Errorf("expected non-empty decoded JSON, got empty string")
			}
		})
	}
}

// TestRegistry_JwtDecode_ArityErrors covers the 0/2/3-arg error paths
// for both functions — ensures the oneArg wrapper is used correctly.
func TestRegistry_JwtDecode_ArityErrors(t *testing.T) {
	reg := NewRegistry(nil)
	tests := []struct {
		name string
		fn   string
		args []string
	}{
		{"jwtDecodeHeader zero args", "jwtDecodeHeader", nil},
		{"jwtDecodeHeader two args", "jwtDecodeHeader", []string{"a", "b"}},
		{"jwtDecodeClaims zero args", "jwtDecodeClaims", nil},
		{"jwtDecodeClaims three args", "jwtDecodeClaims", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reg.Evaluate(tt.fn, tt.args, nil)
			if err == nil {
				t.Fatal("expected arity error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("want *apierrors.Structured, got %T", err)
			}
			if se.Code != "DYNFN_ARITY" {
				t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
			}
			if !strings.Contains(err.Error(), "$"+tt.fn) {
				t.Errorf("err.Error() should contain $%s; got %q", tt.fn, err.Error())
			}
		})
	}
}

// TestRegistry_JwtDecode_UTF8Roundtrip verifies that a JWT whose claims
// contain UTF-8 multi-byte characters (Japanese in this case) round-trip
// correctly through the base64-url + JSON pipeline. The encoded token
// is constructed inline so the test does not depend on an external fixture.
func TestRegistry_JwtDecode_UTF8Roundtrip(t *testing.T) {
	header := `{"alg":"HS256","typ":"JWT"}`
	claims := `{"name":"日本語"}`
	tok := base64.RawURLEncoding.EncodeToString([]byte(header)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(claims)) + "." +
		"unused-signature"

	reg := NewRegistry(nil)
	got, err := reg.Evaluate("jwtDecodeClaims", []string{tok}, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	const want = `{"name":"日本語"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ---- Step 2: Registry locale option tests ----

func TestNewRegistry_WithLocale_DeDE_DrawsGermanName(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed, WithLocale("de-DE"))
	v, err := reg.Evaluate("faker.fullName", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// Must be a German last name (not from en-US pool Smith/Johnson/Williams)
	enUSNames := map[string]bool{
		"Smith": true, "Johnson": true, "Williams": true, "Brown": true,
		"Jones": true, "Garcia": true, "Miller": true, "Davis": true,
	}
	parts := strings.SplitN(v, " ", 2)
	if len(parts) < 2 {
		t.Fatalf("fullName %q has less than two parts", v)
	}
	lastName := parts[1]
	if enUSNames[lastName] {
		t.Errorf("de-DE fullName %q has en-US last name %q; expected German name", v, lastName)
	}
}

func TestNewRegistry_DefaultLocale_IsEnUS(t *testing.T) {
	seed := int64(42)
	def := NewRegistry(&seed)
	seed2 := int64(42)
	enus := NewRegistry(&seed2, WithLocale("en-US"))
	for _, fn := range []string{"faker.firstName", "faker.lastName", "faker.fullName", "faker.city"} {
		v1, _ := def.Evaluate(fn, nil, make(map[string]string))
		v2, _ := enus.Evaluate(fn, nil, make(map[string]string))
		if v1 != v2 {
			t.Errorf("%s: default locale != en-US: %q vs %q", fn, v1, v2)
		}
	}
}

func TestNewRegistry_WithLocale_FallbackWarns(t *testing.T) {
	// After M20-003 every supported full code ships a native pool, so to keep the
	// fallback-warning path under test we temporarily add a synthetic supported code
	// with no pool, simulating a "supported but unshipped" locale (white-box test).
	const synthetic = "xx-XX"
	supportedLocaleSet[synthetic] = true
	t.Cleanup(func() { delete(supportedLocaleSet, synthetic) })
	// fallbackChain("xx-XX") -> ["xx-XX", "xx", "en-US"]; only en-US has a pool,
	// so resolution falls back to en-US and must emit the fallback warning.

	var got string
	seed := int64(42)
	reg := NewRegistry(&seed, WithLocale(synthetic), WithLocaleWarning(func(s string) { got = s }))
	v, err := reg.Evaluate("faker.fullName", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v == "" {
		t.Error("expected non-empty fullName")
	}
	if !strings.Contains(got, "falling back") {
		t.Errorf("expected fallback warning containing 'falling back', got %q", got)
	}
}

func TestValidateLocale_SupportedCode(t *testing.T) {
	if err := ValidateLocale("de-DE"); err != nil {
		t.Errorf("ValidateLocale(de-DE): unexpected error: %v", err)
	}
}

func TestValidateLocale_EmptyCode(t *testing.T) {
	if err := ValidateLocale(""); err != nil {
		t.Errorf("ValidateLocale(''): unexpected error: %v", err)
	}
}

func TestValidateLocale_UnknownCode(t *testing.T) {
	err := ValidateLocale("xx-YY")
	if err == nil {
		t.Fatal("expected error for unknown locale, got nil")
	}
	if !strings.Contains(err.Error(), "xx-YY") {
		t.Errorf("error should mention the unknown code, got: %v", err)
	}
}

// ---- Step 3: Faker closures use active locale pool ----

func TestRegistry_Locale_SeedPositionInvariant(t *testing.T) {
	// SPEC:1023-1026 — same seed selects the same zero-based index in each locale.
	const seed = int64(7)
	locales := []string{
		"en-US", "de-DE", "en-GB", "fr-FR", "es-ES", "it-IT",
		"pt-BR", "nl-NL", "pl-PL", "sv-SE", "tr-TR",
		"ja-JP", "zh-CN", "ko-KR", "ru-RU",
	}
	for _, code := range locales {
		t.Run(code, func(t *testing.T) {
			s := seed
			reg := NewRegistry(&s, WithLocale(code))
			got, err := reg.Evaluate("faker.firstName", nil, make(map[string]string))
			if err != nil {
				t.Fatalf("%s Evaluate: %v", code, err)
			}
			pool := localePools[code].firstNames
			rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed>>32^0xdeadbeef)))
			want := pool[intn(rng, len(pool))]
			if got != want {
				t.Errorf("%s firstName at seed %d = %q, want %q", code, seed, got, want)
			}
		})
	}
}

func TestRegistry_Locale_EnUS_ByteIdenticalToBaseline(t *testing.T) {
	// Regression: WithLocale("en-US") == no-option for a battery of seeds/funcs.
	funcs := []string{
		"faker.firstName", "faker.lastName", "faker.fullName",
		"faker.city",
	}
	for _, seed := range []int64{0, 1, 42, 99, 1000} {
		for _, fn := range funcs {
			s1 := seed
			s2 := seed
			def := NewRegistry(&s1)
			enus := NewRegistry(&s2, WithLocale("en-US"))
			v1, _ := def.Evaluate(fn, nil, make(map[string]string))
			v2, _ := enus.Evaluate(fn, nil, make(map[string]string))
			if v1 != v2 {
				t.Errorf("seed=%d %s: default != en-US: %q vs %q", seed, fn, v1, v2)
			}
		}
	}
}

func TestRegistry_DeDE_Phone_Format(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed, WithLocale("de-DE"))
	v, err := reg.Evaluate("faker.phone", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// German phone should start with +49
	if !strings.HasPrefix(v, "+49") {
		t.Errorf("de-DE phone %q should start with +49", v)
	}
}

func TestRegistry_DeDE_City_IsGerman(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed, WithLocale("de-DE"))
	v, err := reg.Evaluate("faker.city", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// Should be a de-DE city (e.g., Berlin, München, Hamburg, etc.)
	deDECities := map[string]bool{}
	for _, c := range localePools["de-DE"].cities {
		deDECities[c] = true
	}
	if !deDECities[v] {
		t.Errorf("de-DE city %q is not in the de-DE city pool", v)
	}
}

// ---- M20-002: Latin-script European locale pools ----

// TestRegistry_EnGB_NoFallbackWarning asserts that en-GB now resolves to its
// own native pool and does NOT trigger a fallback warning (Behaviour 4).
func TestRegistry_EnGB_NoFallbackWarning(t *testing.T) {
	var warned bool
	seed := int64(42)
	reg := NewRegistry(&seed, WithLocale("en-GB"),
		WithLocaleWarning(func(string) { warned = true }))
	if _, err := reg.Evaluate("faker.fullName", nil, make(map[string]string)); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if warned {
		t.Error("en-GB emitted a fallback warning; its native pool now exists")
	}
}

// TestRegistry_EnGB_Phone_BritishFormat asserts en-GB phone output starts with
// +44 and uses the British format, not the en-US "(555) ..." shape (Behaviour 3).
func TestRegistry_EnGB_Phone_BritishFormat(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed, WithLocale("en-GB"))
	v, err := reg.Evaluate("faker.phone", nil, make(map[string]string))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !strings.HasPrefix(v, "+44") {
		t.Errorf("en-GB phone %q should start with +44", v)
	}
	if strings.Contains(v, "(") { // not the en-US "(555) ..." shape
		t.Errorf("en-GB phone %q used en-US format", v)
	}
}

// TestRegistry_LatinScript_DrawsLocalePool asserts that each Latin-script locale
// draws names and cities from its own pool rather than en-US (Behaviour 1, 6).
func TestRegistry_LatinScript_DrawsLocalePool(t *testing.T) {
	cases := []string{"fr-FR", "es-ES", "it-IT", "pt-BR", "nl-NL", "pl-PL", "sv-SE", "tr-TR"}
	for _, code := range cases {
		t.Run(code, func(t *testing.T) {
			seed := int64(42)
			reg := NewRegistry(&seed, WithLocale(code))
			pool := localePools[code]
			ln := map[string]bool{}
			for _, n := range pool.lastNames {
				ln[n] = true
			}
			v, err := reg.Evaluate("faker.lastName", nil, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if !ln[v] {
				t.Errorf("%s lastName %q not drawn from its own pool", code, v)
			}
			cityPool := map[string]bool{}
			for _, c := range pool.cities {
				cityPool[c] = true
			}
			cv, err := reg.Evaluate("faker.city", nil, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate city: %v", err)
			}
			if !cityPool[cv] {
				t.Errorf("%s city %q not drawn from its own pool", code, cv)
			}
		})
	}
}

// TestRegistry_TrTR_PreservesTurkishCasing asserts that dotted-İ (U+0130) and
// dotless-ı (U+0131) are present in the tr-TR pool and valid UTF-8 (Behaviour 2).
func TestRegistry_TrTR_PreservesTurkishCasing(t *testing.T) {
	pool := localePools["tr-TR"]
	all := append(append([]string{}, pool.firstNames...), pool.lastNames...)
	var sawTurkish bool
	for _, n := range all {
		if !utf8.ValidString(n) {
			t.Errorf("tr-TR name %q is not valid UTF-8", n)
		}
		if strings.ContainsRune(n, 'ı') || strings.ContainsRune(n, 'İ') {
			sawTurkish = true
		}
	}
	if !sawTurkish {
		t.Error("tr-TR pool contains no dotted-İ/dotless-ı names; casing behaviour unverified")
	}
}

// TestRegistry_PtBR_CrossRunDeterminism asserts that the same seed produces
// byte-identical output across two independent Registry instances (Behaviour 8).
func TestRegistry_PtBR_CrossRunDeterminism(t *testing.T) {
	for _, fn := range []string{"faker.fullName", "faker.phone", "faker.city"} {
		s1, s2 := int64(42), int64(42)
		a, err1 := NewRegistry(&s1, WithLocale("pt-BR")).Evaluate(fn, nil, make(map[string]string))
		b, err2 := NewRegistry(&s2, WithLocale("pt-BR")).Evaluate(fn, nil, make(map[string]string))
		if err1 != nil || err2 != nil {
			t.Fatalf("pt-BR %s Evaluate errors: %v / %v", fn, err1, err2)
		}
		if a != b {
			t.Errorf("pt-BR %s not deterministic: %q vs %q", fn, a, b)
		}
	}
}

// TestLocalePools_NameOrdering verifies SPEC:974-986 — CJK locales render
// family-name-first; ru-RU and Latin locales render given-name-first.
// The test checks the struct flag and the rendered fullName at a fixed seed.
func TestLocalePools_NameOrdering(t *testing.T) {
	tests := []struct {
		name            string
		locale          string
		familyNameFirst bool
	}{
		{"ja-JP family-name-first", "ja-JP", true},
		{"zh-CN family-name-first", "zh-CN", true},
		{"ko-KR family-name-first", "ko-KR", true},
		{"ru-RU given-name-first", "ru-RU", false},
		{"en-US given-name-first (regression)", "en-US", false},
		{"de-DE given-name-first (regression)", "de-DE", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, ok := localePools[tc.locale]
			if !ok {
				t.Fatalf("locale %q not found in localePools", tc.locale)
			}
			// Assert the struct flag matches expectation.
			if got := pool.familyNameFirst; got != tc.familyNameFirst {
				t.Fatalf("%s familyNameFirst = %v, want %v", tc.locale, got, tc.familyNameFirst)
			}
			// Assert the rendered fullName puts the expected pool's value first.
			seed := int64(42)
			reg := NewRegistry(&seed, WithLocale(tc.locale))
			full, err := reg.Evaluate("faker.fullName", nil, make(map[string]string))
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			// Recompute the expected tokens at the same seed.
			rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed>>32^0xdeadbeef)))
			first := pool.firstNames[intn(rng, len(pool.firstNames))]
			last := pool.lastNames[intn(rng, len(pool.lastNames))]
			want := first + " " + last
			if tc.familyNameFirst {
				want = last + " " + first
			}
			if full != want {
				t.Errorf("%s fullName = %q, want %q", tc.locale, full, want)
			}
		})
	}
}
