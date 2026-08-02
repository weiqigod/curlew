package oauth1

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apierrors "github.com/peterlindqvist/apitest/internal/errors"
	"github.com/peterlindqvist/apitest/internal/httpexec"
	"github.com/peterlindqvist/apitest/internal/signer"
	"github.com/peterlindqvist/apitest/internal/variable"
)

// defaultParams returns minimal valid params.
func defaultParams() map[string]any {
	return map[string]any{
		"consumer_key":    "dpf43f3p2l4k3l03",
		"consumer_secret": "kd94hf93k423kf44",
	}
}

// rfc5849ExampleParams returns the params for the §3.4.1.1 example.
// The nonce/timestamp are pinned as spec entries because the published
// vector's nonce "wIjqoS" is not derivable from random bytes.
func rfc5849ExampleParams() map[string]any {
	return map[string]any{
		"consumer_key":    "dpf43f3p2l4k3l03",
		"consumer_secret": "kd94hf93k423kf44",
		"nonce":           "wIjqoS",    // test-seam override
		"timestamp":       "137131200", // test-seam override
	}
}

// rfc5849ExampleRequest returns the POST request with the oauth_callback param attached.
func rfc5849ExampleRequest() *httpexec.Request {
	return &httpexec.Request{
		Method:      "POST",
		URL:         "https://photos.example.net/initiate",
		Headers:     map[string]string{},
		QueryParams: map[string]string{"oauth_callback": "http://printer.example.com/ready"},
	}
}

func loadFixture(t *testing.T, vector, file string) string {
	t.Helper()
	path := filepath.Join("testdata", vector, file)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("loadFixture(%s/%s): %v", vector, file, err)
	}
	return strings.TrimRight(string(data), "\r\n ")
}

// extractParam extracts the value of key="..." in authorization header string.
func extractParam(auth, key string) string {
	prefix := key + `="`
	idx := strings.Index(auth, prefix)
	if idx < 0 {
		return ""
	}
	rest := auth[idx+len(prefix):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// ============= Step 1 tests: factory + method validation =============

// TestOAuth1_DefaultMethodIsHMACSHA1 — when method is omitted, the
// signer constructs successfully and records HMAC-SHA1.
func TestOAuth1_DefaultMethodIsHMACSHA1(t *testing.T) {
	s, err := NewSigner(defaultParams())
	if err != nil {
		t.Fatal(err)
	}
	inner, ok := s.(*oauth1Signer)
	if !ok {
		t.Fatalf("want *oauth1Signer, got %T", s)
	}
	if inner.method != MethodHMACSHA1 {
		t.Errorf("method = %q; want %q", inner.method, MethodHMACSHA1)
	}
}

// TestOAuth1_HMACSHA256_OptIn — explicit method=HMAC-SHA256 is accepted.
func TestOAuth1_HMACSHA256_OptIn(t *testing.T) {
	p := defaultParams()
	p["method"] = "HMAC-SHA256"
	s, err := NewSigner(p)
	if err != nil {
		t.Fatal(err)
	}
	inner := s.(*oauth1Signer)
	if inner.method != MethodHMACSHA256 {
		t.Errorf("method = %q; want %q", inner.method, MethodHMACSHA256)
	}
}

// TestOAuth1_RejectsUnsupportedMethods — every non-HMAC method is rejected
// with structured error code SIGNER_OAUTH1_UNSUPPORTED_METHOD.
func TestOAuth1_RejectsUnsupportedMethods(t *testing.T) {
	cases := []string{"RSA-SHA1", "PLAINTEXT", "hmac-sha1", "MD5", "garbage"}
	for _, m := range cases {
		t.Run(m, func(t *testing.T) {
			p := defaultParams()
			p["method"] = m
			_, err := NewSigner(p)
			if err == nil {
				t.Fatalf("expected error for method=%q, got nil", m)
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("want *errors.Structured, got %T: %v", err, err)
			}
			if se.Code != "SIGNER_OAUTH1_UNSUPPORTED_METHOD" {
				t.Errorf("Code = %q; want SIGNER_OAUTH1_UNSUPPORTED_METHOD", se.Code)
			}
			if se.Category != apierrors.CategoryInput {
				t.Errorf("Category = %q; want %q", se.Category, apierrors.CategoryInput)
			}
			if !strings.Contains(se.Message, m) {
				t.Errorf("Message %q must name the offending method", se.Message)
			}
			if !strings.Contains(se.Hint, "HMAC-SHA1") || !strings.Contains(se.Hint, "HMAC-SHA256") {
				t.Errorf("Hint %q must list both supported values", se.Hint)
			}
		})
	}
}

// TestOAuth1_MissingField_StructuredError — consumer_key / consumer_secret
// must be present at factory time.
func TestOAuth1_MissingField_StructuredError(t *testing.T) {
	cases := []struct{ name, drop string }{
		{"missing consumer_key", "consumer_key"},
		{"missing consumer_secret", "consumer_secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := defaultParams()
			delete(p, tc.drop)
			_, err := NewSigner(p)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("want *Structured, got %T", err)
			}
			if se.Code != "SIGNER_OAUTH1_MISSING_FIELD" {
				t.Errorf("Code = %q; want SIGNER_OAUTH1_MISSING_FIELD", se.Code)
			}
			if !strings.Contains(se.Message, tc.drop) {
				t.Errorf("Message %q must name the missing field", se.Message)
			}
		})
	}
}

// TestFactory_DelegatesToNewSigner — Factory builds a signer with default options.
func TestFactory_DelegatesToNewSigner(t *testing.T) {
	s, err := Factory(defaultParams())
	if err != nil {
		t.Fatalf("Factory: %v", err)
	}
	if s == nil {
		t.Fatal("Factory returned nil signer")
	}
}

// ============= Step 2 tests: Sign + base string + signatures =============

// TestOAuth1_HMACSHA1_RFC5849Vector — byte-exact base string AND signature
// against the published RFC 5849 §3.4.1.1 example.
func TestOAuth1_HMACSHA1_RFC5849Vector(t *testing.T) {
	s, err := NewSigner(rfc5849ExampleParams())
	if err != nil {
		t.Fatal(err)
	}
	req := rfc5849ExampleRequest()
	if err := s.Sign(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}

	// Direct base-string check via package-internal helper.
	inner := s.(*oauth1Signer)
	gotBase, err := inner.buildBaseStringForTest(req)
	if err != nil {
		t.Fatal(err)
	}
	wantBase := loadFixture(t, "rfc5849-example", "base-string.txt")
	if gotBase != wantBase {
		t.Errorf("base string mismatch\n got: %s\nwant: %s", gotBase, wantBase)
	}

	// Signature via Authorization header.
	auth := req.Headers["Authorization"]
	wantSig := loadFixture(t, "rfc5849-example", "signature.txt")
	wantSubstr := `oauth_signature="` + uriEncode(wantSig) + `"`
	if !strings.Contains(auth, wantSubstr) {
		t.Errorf("Authorization missing expected signature\n got: %s\nwant substr: %s",
			auth, wantSubstr)
	}
}

// TestOAuth1_HMACSHA256_OptIn_Vector — same inputs with method=HMAC-SHA256
// produce the same base string structure and a fixture-anchored signature.
func TestOAuth1_HMACSHA256_OptIn_Vector(t *testing.T) {
	p := rfc5849ExampleParams()
	p["method"] = "HMAC-SHA256"
	s, err := NewSigner(p)
	if err != nil {
		t.Fatal(err)
	}
	req := rfc5849ExampleRequest()
	if err := s.Sign(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}

	// Base string has oauth_signature_method=HMAC-SHA256.
	inner := s.(*oauth1Signer)
	gotBase, err := inner.buildBaseStringForTest(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotBase, "oauth_signature_method%3DHMAC-SHA256") {
		t.Errorf("HMAC-SHA256 base string must carry oauth_signature_method=HMAC-SHA256; got %s", gotBase)
	}

	// Signature anchored to fixture file.
	wantSig := loadFixture(t, "rfc5849-example-sha256", "signature.txt")
	auth := req.Headers["Authorization"]
	wantSubstr := `oauth_signature="` + uriEncode(wantSig) + `"`
	if !strings.Contains(auth, wantSubstr) {
		t.Errorf("HMAC-SHA256 Authorization missing expected signature\n got: %s\nwant substr: %s",
			auth, wantSubstr)
	}
	// Confirm the method header param.
	if !strings.Contains(auth, `oauth_signature_method="HMAC-SHA256"`) {
		t.Errorf("Authorization must declare oauth_signature_method=HMAC-SHA256; got %s", auth)
	}
}

// TestOAuth1_AuthorizationHeaderShape — alphabetical key order, OAuth prefix,
// quoted percent-encoded values.
func TestOAuth1_AuthorizationHeaderShape(t *testing.T) {
	p := rfc5849ExampleParams()
	p["token"] = "tokenABC"
	p["token_secret"] = "tokenSecretXYZ"
	s, err := NewSigner(p)
	if err != nil {
		t.Fatal(err)
	}
	req := rfc5849ExampleRequest()
	if err := s.Sign(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}

	auth := req.Headers["Authorization"]
	if !strings.HasPrefix(auth, "OAuth ") {
		t.Errorf("Authorization must start with %q; got %q", "OAuth ", auth)
	}

	// Order: oauth_consumer_key < oauth_nonce < oauth_signature <
	//        oauth_signature_method < oauth_timestamp < oauth_token < oauth_version.
	wantOrder := []string{
		"oauth_consumer_key=",
		"oauth_nonce=",
		"oauth_signature=",
		"oauth_signature_method=",
		"oauth_timestamp=",
		"oauth_token=",
		`oauth_version="1.0"`,
	}
	pos := -1
	for _, k := range wantOrder {
		i := strings.Index(auth, k)
		if i < 0 {
			t.Errorf("Authorization missing %q\n  got: %s", k, auth)
			continue
		}
		if i < pos {
			t.Errorf("Authorization out of order at %q (pos %d < %d)\n  got: %s",
				k, i, pos, auth)
		}
		pos = i
	}

	// Values are double-quoted.
	if !strings.Contains(auth, `oauth_consumer_key="dpf43f3p2l4k3l03"`) {
		t.Errorf("consumer_key must be double-quoted with raw value (alphanumeric, no encoding needed)\n  got: %s", auth)
	}
}

// TestOAuth1_RealmFirst — when realm is set, it appears first in the
// Authorization header and is absent from the signature base string
// (RFC 5849 §3.4.1.3 excludes realm).
func TestOAuth1_RealmFirst(t *testing.T) {
	p := rfc5849ExampleParams()
	p["realm"] = "Photos"
	s, err := NewSigner(p)
	if err != nil {
		t.Fatal(err)
	}
	req := rfc5849ExampleRequest()
	if err := s.Sign(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	auth := req.Headers["Authorization"]
	if !strings.HasPrefix(auth, `OAuth realm="Photos", `) {
		t.Errorf("realm must be the first param after OAuth prefix; got %s", auth)
	}

	// RFC 5849 §3.4.1.3: realm MUST NOT appear in the signature base string.
	inner := s.(*oauth1Signer)
	base, err := inner.buildBaseStringForTest(req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(base, "realm") {
		t.Errorf("realm must be absent from the signature base string; got %s", base)
	}
}

// TestOAuth1_QueryAndFormParams_Merged — when both URL query and form-body
// params are present, both contribute to the base string. A struct body with
// a form content-type header contributes nothing (parseFormBody edge case).
func TestOAuth1_QueryAndFormParams_Merged(t *testing.T) {
	p := rfc5849ExampleParams()
	s, err := NewSigner(p)
	if err != nil {
		t.Fatal(err)
	}
	req := &httpexec.Request{
		Method:      "POST",
		URL:         "https://example.com/path?queryA=v1",
		Headers:     map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		Body:        "formA=fv1&formB=fv2",
		QueryParams: map[string]string{"queryB": "v2"},
	}
	if err := s.Sign(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}

	inner := s.(*oauth1Signer)
	base, err := inner.buildBaseStringForTest(req)
	if err != nil {
		t.Fatal(err)
	}
	// Each contributing key must appear in the params section.
	for _, k := range []string{"queryA", "queryB", "formA", "formB", "oauth_consumer_key"} {
		if !strings.Contains(base, "%26"+k+"%3D") && !strings.HasPrefix(strings.SplitN(base, "&", 3)[2], k+"%3D") {
			t.Errorf("base string missing %q: %s", k, base)
		}
	}

	// A non-string/non-[]byte body (e.g. a struct) with a form content-type
	// header must silently contribute no form params (parseFormBody default branch).
	t.Run("struct_body_ignored", func(t *testing.T) {
		s2, err := NewSigner(p)
		if err != nil {
			t.Fatal(err)
		}
		structReq := &httpexec.Request{
			Method:  "POST",
			URL:     "https://example.com/path",
			Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
			Body:    struct{ Field string }{Field: "value"}, // non-string/non-[]byte
		}
		if err := s2.Sign(context.Background(), structReq, nil); err != nil {
			t.Fatalf("Sign with struct body failed: %v", err)
		}
		inner2 := s2.(*oauth1Signer)
		base2, err := inner2.buildBaseStringForTest(structReq)
		if err != nil {
			t.Fatal(err)
		}
		// Only oauth_* params should appear — no form-body keys from the struct.
		if strings.Contains(base2, "Field") {
			t.Errorf("struct body fields must not appear in base string; got %s", base2)
		}
	})
}

// TestOAuth1_PercentEncoding_Spec — RFC 3986 unreserved char passthrough +
// percent-encode all others.
func TestOAuth1_PercentEncoding_Spec(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello", "hello"},
		{"hello world", "hello%20world"},
		{"a-z_0.9~", "a-z_0.9~"},
		{"a+b=c&d", "a%2Bb%3Dc%26d"},
		{"http://printer.example.com/ready", "http%3A%2F%2Fprinter.example.com%2Fready"},
	}
	for _, tc := range cases {
		if got := uriEncode(tc.in); got != tc.want {
			t.Errorf("uriEncode(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// TestOAuth1_InterpolationError — undefined variable reference fails Sign.
func TestOAuth1_InterpolationError(t *testing.T) {
	scope := variable.NewScope(map[string]string{
		"consumer_key": "dpf43f3p2l4k3l03",
		// consumer_secret intentionally undefined
	})
	_ = scope.Resolve()
	s, err := NewSigner(map[string]any{
		"consumer_key":    "{{consumer_key}}",
		"consumer_secret": "{{consumer_secret_undefined}}",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := signer.WithScope(context.Background(), scope)
	req := rfc5849ExampleRequest()
	if err := s.Sign(ctx, req, nil); err == nil {
		t.Fatal("expected error for undefined variable; got nil")
	}
}

// TestOAuth1_GeneratedNonceAndTimestamp — when spec omits nonce/timestamp,
// the signer generates them from cfg.nonceSource and cfg.now.
func TestOAuth1_GeneratedNonceAndTimestamp(t *testing.T) {
	fixedRand := bytes.NewReader(bytes.Repeat([]byte{0x42}, 64))
	fixedNow := func() time.Time { return time.Unix(1700000000, 0) }
	s, err := NewSigner(defaultParams(),
		WithClock(fixedNow), WithNonceSource(fixedRand))
	if err != nil {
		t.Fatal(err)
	}
	req := rfc5849ExampleRequest()
	if err := s.Sign(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	auth := req.Headers["Authorization"]
	if !strings.Contains(auth, `oauth_timestamp="1700000000"`) {
		t.Errorf("oauth_timestamp from clock; got %s", auth)
	}
	// 0x42 base32 = "II..." — ensure nonce is 16+ chars in [A-Z2-7].
	nonceParam := extractParam(auth, "oauth_nonce")
	if len(nonceParam) < 16 {
		t.Errorf("nonce must be ≥16 chars; got %q (len %d)", nonceParam, len(nonceParam))
	}
	for _, c := range nonceParam {
		if (c < 'A' || c > 'Z') && (c < '2' || c > '7') {
			t.Errorf("nonce char %q outside base32 alphabet; got nonce %q", c, nonceParam)
		}
	}
}

// ============= Step 3 tests: sensitive-secret propagation =============

// TestOAuth1_SecretsSensitivePropagation — both consumer_secret and
// token_secret, when resolved from sensitive variables, are added to
// the SensitiveSet via AddValue.
func TestOAuth1_SecretsSensitivePropagation(t *testing.T) {
	sensSet := variable.NewSensitiveSet()
	sensSet.Add("consumer_secret_var")
	sensSet.Add("token_secret_var")
	scope := variable.NewScope(map[string]string{
		"consumer_key_var":    "dpf43f3p2l4k3l03",
		"consumer_secret_var": "kd94hf93k423kf44",
		"token_var":           "tokenABC",
		"token_secret_var":    "tokenSecretXYZ",
	})
	if err := scope.Resolve(); err != nil {
		t.Fatal(err)
	}
	scope = scope.WithRuntimeSensitive(sensSet)

	s, err := NewSigner(map[string]any{
		"consumer_key":    "{{consumer_key_var}}",
		"consumer_secret": "{{consumer_secret_var}}",
		"token":           "{{token_var}}",
		"token_secret":    "{{token_secret_var}}",
		"nonce":           "wIjqoS",
		"timestamp":       "137131200",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := signer.WithScope(context.Background(), scope)
	req := rfc5849ExampleRequest()
	if err := s.Sign(ctx, req, sensSet); err != nil {
		t.Fatal(err)
	}

	wants := []string{"kd94hf93k423kf44", "tokenSecretXYZ"}
	values := sensSet.Values()
	for _, w := range wants {
		found := false
		for _, v := range values {
			if v == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("sensitive value %q not registered; values: %v", w, values)
		}
	}
}

// TestOAuth1_LiteralSecret_NotMarked — literal (non-{{var}}) secrets are
// NOT added to the SensitiveSet, since there is no variable to back-trace.
func TestOAuth1_LiteralSecret_NotMarked(t *testing.T) {
	sensSet := variable.NewSensitiveSet()
	s, err := NewSigner(map[string]any{
		"consumer_key":    "dpf43f3p2l4k3l03",
		"consumer_secret": "literal-consumer-secret",
		"token_secret":    "literal-token-secret",
		"token":           "tokenABC",
		"nonce":           "wIjqoS",
		"timestamp":       "137131200",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := rfc5849ExampleRequest()
	if err := s.Sign(context.Background(), req, sensSet); err != nil {
		t.Fatal(err)
	}
	for _, v := range sensSet.Values() {
		if v == "literal-consumer-secret" || v == "literal-token-secret" {
			t.Errorf("literal secret was incorrectly marked as sensitive; values: %v", sensSet.Values())
		}
	}
}

// ============= Step 4 tests: init-time registration =============

// TestOAuth1_RegisteredAtInit — confirms init() registered the type.
func TestOAuth1_RegisteredAtInit(t *testing.T) {
	r := signer.NewWithBuiltins()
	f, err := r.Lookup("oauth1")
	if err != nil {
		t.Fatalf("Lookup(oauth1): %v", err)
	}
	s, err := f(defaultParams())
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if s == nil {
		t.Fatal("factory returned nil")
	}
}
