package awssigv4

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/signer"
	"github.com/weiqigod/curlew/internal/variable"
)

// loadFixture reads a testdata fixture file and trims trailing whitespace.
func loadFixture(t *testing.T, vector, file string) string {
	t.Helper()
	path := filepath.Join("testdata", vector, file)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("loadFixture(%s/%s): %v", vector, file, err)
	}
	return strings.TrimRight(string(data), "\r\n ")
}

// frozenClock returns the AWS SigV4 test-suite reference timestamp.
func frozenClock() func() time.Time {
	return func() time.Time {
		return time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)
	}
}

// defaultParams returns minimal params for test-suite vectors.
func defaultParams() map[string]any {
	return map[string]any{
		"region":     "us-east-1",
		"service":    "service",
		"access_key": "AKIDEXAMPLE",
		"secret_key": "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
	}
}

// dropField returns defaultParams minus the named field.
func dropField(name string) map[string]any {
	p := defaultParams()
	delete(p, name)
	return p
}

// TestSigV4_GetVanilla tests the AWS SigV4 published test-suite get-vanilla vector.
// Frozen clock: 2015-08-30T12:36:00Z.
// Authorization, canonical request, and string-to-sign are verified byte-exactly
// against the testdata fixtures (DoD: three test vectors byte-exact at each stage).
func TestSigV4_GetVanilla(t *testing.T) {
	s, err := NewSigner(defaultParams(), WithClock(frozenClock()))
	if err != nil {
		t.Fatal(err)
	}
	req := &httpexec.Request{
		Method:  "GET",
		URL:     "http://example.amazonaws.com/",
		Headers: map[string]string{},
	}
	if err := s.Sign(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	// Byte-exact Authorization check against testdata fixture.
	wantAuth := loadFixture(t, "get-vanilla", "authorization.txt")
	if got := req.Headers["Authorization"]; got != wantAuth {
		t.Errorf("Authorization mismatch\n got: %s\nwant: %s", got, wantAuth)
	}
	// Verify x-amz-date was injected.
	if req.Headers["x-amz-date"] != "20150830T123600Z" {
		t.Errorf("x-amz-date = %q; want 20150830T123600Z", req.Headers["x-amz-date"])
	}
	// Empty-body requests: x-amz-content-sha256 is injected but NOT in SignedHeaders.
	if req.Headers["x-amz-content-sha256"] != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("x-amz-content-sha256 = %q; want empty-body hash", req.Headers["x-amz-content-sha256"])
	}
	if strings.Contains(wantAuth, "x-amz-content-sha256") {
		t.Errorf("get-vanilla SignedHeaders must NOT include x-amz-content-sha256 for empty body; got %s", wantAuth)
	}
	// Byte-exact canonical request check against testdata fixture (DoD requirement).
	// Use a fresh request (pre-Sign) to avoid the Authorization header polluting
	// the canonical request computation.
	wantCanonReq := loadFixture(t, "get-vanilla", "canonical-request.txt")
	const emptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	preSignReq := &httpexec.Request{
		Method:  "GET",
		URL:     "http://example.amazonaws.com/",
		Headers: map[string]string{"x-amz-date": "20150830T123600Z"},
	}
	gotCanonReq, _ := buildCanonicalRequest(preSignReq, emptyHash, false)
	if gotCanonReq != wantCanonReq {
		t.Errorf("canonical request mismatch\n got:\n%s\nwant:\n%s", gotCanonReq, wantCanonReq)
	}
	// Byte-exact string-to-sign check against testdata fixture.
	wantSTS := loadFixture(t, "get-vanilla", "string-to-sign.txt")
	credScope := "20150830/us-east-1/service/aws4_request"
	gotSTS := "AWS4-HMAC-SHA256\n20150830T123600Z\n" + credScope + "\n" + sha256Hex([]byte(gotCanonReq))
	if gotSTS != wantSTS {
		t.Errorf("string-to-sign mismatch\n got:\n%s\nwant:\n%s", gotSTS, wantSTS)
	}
}

// TestSigV4_PostHeaderKeyCase tests mixed-case header name canonicalisation.
// All header names must be lowercased in the SignedHeaders list and canonical request.
// Authorization is verified byte-exactly against the testdata fixture.
func TestSigV4_PostHeaderKeyCase(t *testing.T) {
	s, err := NewSigner(defaultParams(), WithClock(frozenClock()))
	if err != nil {
		t.Fatal(err)
	}
	req := &httpexec.Request{
		Method: "GET",
		URL:    "http://example.amazonaws.com/",
		Headers: map[string]string{
			"My-Header1": "value1", // mixed case — must be lowercased
		},
	}
	if err := s.Sign(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	auth := req.Headers["Authorization"]
	// SignedHeaders must contain lowercase header names, sorted.
	if !strings.Contains(auth, "SignedHeaders=host;my-header1;x-amz-date") {
		t.Errorf("SignedHeaders not fully lowercase or not sorted:\n  auth: %s", auth)
	}
	// Byte-exact check against testdata fixture.
	want := loadFixture(t, "get-header-key-case", "authorization.txt")
	if auth != want {
		t.Errorf("Authorization mismatch (byte-exact)\n got: %s\nwant: %s", auth, want)
	}
}

// TestSigV4_PostXWWWFormURLEncoded tests the AWS SigV4 published test-suite
// post-x-www-form-urlencoded vector. Verifies that x-amz-content-sha256 is
// included in SignedHeaders for non-empty bodies. Authorization and canonical
// request are verified byte-exactly against the testdata fixtures.
func TestSigV4_PostXWWWFormURLEncoded(t *testing.T) {
	s, err := NewSigner(defaultParams(), WithClock(frozenClock()))
	if err != nil {
		t.Fatal(err)
	}
	req := &httpexec.Request{
		Method: "POST",
		URL:    "http://example.amazonaws.com/",
		Headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		},
		Body: "Param1=value1",
	}
	if err := s.Sign(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	auth := req.Headers["Authorization"]
	// SignedHeaders must include x-amz-content-sha256 for non-empty body.
	if !strings.Contains(auth, "x-amz-content-sha256") {
		t.Errorf("SignedHeaders must include x-amz-content-sha256 for non-empty body; got: %s", auth)
	}
	// Byte-exact Authorization check against testdata fixture.
	wantAuth := loadFixture(t, "post-x-www-form-urlencoded", "authorization.txt")
	if auth != wantAuth {
		t.Errorf("Authorization mismatch (byte-exact)\n got: %s\nwant: %s", auth, wantAuth)
	}
	// Byte-exact canonical request check against testdata fixture (DoD requirement).
	// Use a fresh request (pre-Sign state) to avoid the Authorization header
	// polluting the canonical request computation.
	wantCanonReq := loadFixture(t, "post-x-www-form-urlencoded", "canonical-request.txt")
	bodyHash := req.Headers["x-amz-content-sha256"]
	preSignReq := &httpexec.Request{
		Method: "POST",
		URL:    "http://example.amazonaws.com/",
		Headers: map[string]string{
			"Content-Type":         "application/x-www-form-urlencoded",
			"x-amz-date":           "20150830T123600Z",
			"x-amz-content-sha256": bodyHash,
		},
	}
	gotCanonReq, _ := buildCanonicalRequest(preSignReq, bodyHash, true)
	if gotCanonReq != wantCanonReq {
		t.Errorf("canonical request mismatch\n got:\n%s\nwant:\n%s", gotCanonReq, wantCanonReq)
	}
}

// TestSigV4_BodyHashInjection verifies x-amz-content-sha256 injection.
// Empty body → well-known empty hash injected (but NOT in SignedHeaders).
// Non-empty body → correct SHA-256 injected AND included in SignedHeaders,
// verified via the final Authorization signature.
func TestSigV4_BodyHashInjection(t *testing.T) {
	t.Run("empty body", func(t *testing.T) {
		s, err := NewSigner(defaultParams(), WithClock(frozenClock()))
		if err != nil {
			t.Fatal(err)
		}
		req := &httpexec.Request{Method: "GET", URL: "http://example.amazonaws.com/"}
		if err := s.Sign(context.Background(), req, nil); err != nil {
			t.Fatal(err)
		}
		const emptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
		if got := req.Headers["x-amz-content-sha256"]; got != emptyHash {
			t.Errorf("empty-body hash = %q; want %q", got, emptyHash)
		}
		// Empty body: x-amz-content-sha256 must NOT appear in SignedHeaders.
		auth := req.Headers["Authorization"]
		if strings.Contains(auth, "x-amz-content-sha256") {
			t.Errorf("empty-body SignedHeaders must NOT include x-amz-content-sha256; got: %s", auth)
		}
	})

	t.Run("string body abc", func(t *testing.T) {
		s, err := NewSigner(defaultParams(), WithClock(frozenClock()))
		if err != nil {
			t.Fatal(err)
		}
		req := &httpexec.Request{
			Method: "POST",
			URL:    "http://example.amazonaws.com/",
			Body:   "abc",
		}
		if err := s.Sign(context.Background(), req, nil); err != nil {
			t.Fatal(err)
		}
		// SHA-256("abc") — verified via `echo -n "abc" | shasum -a 256`
		const abcHash = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
		got := req.Headers["x-amz-content-sha256"]
		if got != abcHash {
			t.Errorf("abc-body hash = %q; want %q", got, abcHash)
		}
		// Non-empty body: x-amz-content-sha256 MUST appear in SignedHeaders,
		// proving it was included in the canonical request BEFORE hashing.
		auth := req.Headers["Authorization"]
		if !strings.Contains(auth, "x-amz-content-sha256") {
			t.Errorf("non-empty body SignedHeaders must include x-amz-content-sha256; got: %s", auth)
		}
		// Verify the Authorization signature is deterministic (the hash feeds into
		// the canonical request — a different hash would produce a different signature).
		// This pre-computed value was verified independently.
		const wantSig = "23d9f767c2ba83120dadb120d165678fd16708427ac62dcaa596fd0ef0f2f29a"
		if !strings.Contains(auth, wantSig) {
			t.Errorf("abc-body Authorization signature mismatch; got: %s", auth)
		}
	})
}

// TestSigV4_WithSessionToken verifies session_token header injection.
func TestSigV4_WithSessionToken(t *testing.T) {
	p := defaultParams()
	p["session_token"] = "FQoGZXIvYXdz"
	s, err := NewSigner(p, WithClock(frozenClock()))
	if err != nil {
		t.Fatal(err)
	}
	req := &httpexec.Request{
		Method:  "GET",
		URL:     "http://example.amazonaws.com/",
		Headers: map[string]string{},
	}
	if err := s.Sign(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	// Header must be injected.
	if got := req.Headers["x-amz-security-token"]; got != "FQoGZXIvYXdz" {
		t.Errorf("x-amz-security-token = %q; want FQoGZXIvYXdz", got)
	}
	// SignedHeaders must include x-amz-security-token in sorted position.
	auth := req.Headers["Authorization"]
	if !strings.Contains(auth, "x-amz-security-token") {
		t.Errorf("SignedHeaders must include x-amz-security-token; got auth: %s", auth)
	}
}

// TestSigV4_SecretKeySensitivePropagation verifies that when secret_key resolves
// from a sensitive variable, sensitives.AddValue is called with the resolved secret.
func TestSigV4_SecretKeySensitivePropagation(t *testing.T) {
	sensSet := variable.NewSensitiveSet()
	sensSet.Add("aws_secret")
	scope := variable.NewScope(map[string]string{
		"aws_key":    "AKIDEXAMPLE",
		"aws_secret": "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
	})
	if err := scope.Resolve(); err != nil {
		t.Fatal(err)
	}
	scope = scope.WithRuntimeSensitive(sensSet)

	s, err := NewSigner(map[string]any{
		"region":     "us-east-1",
		"service":    "service",
		"access_key": "{{aws_key}}",
		"secret_key": "{{aws_secret}}",
	}, WithClock(frozenClock()))
	if err != nil {
		t.Fatal(err)
	}

	ctx := signer.WithScope(context.Background(), scope)
	req := &httpexec.Request{Method: "GET", URL: "http://example.amazonaws.com/"}
	if err := s.Sign(ctx, req, sensSet); err != nil {
		t.Fatal(err)
	}

	const wantSecret = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
	found := false
	for _, v := range sensSet.Values() {
		if v == wantSecret {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("secret_key not registered as sensitive value; got %v", sensSet.Values())
	}
}

// TestSigV4_SessionTokenSensitivePropagation verifies that when session_token
// resolves from a sensitive variable, sensitives.AddValue is called with the
// resolved token value.
func TestSigV4_SessionTokenSensitivePropagation(t *testing.T) {
	sensSet := variable.NewSensitiveSet()
	sensSet.Add("aws_token")
	scope := variable.NewScope(map[string]string{
		"aws_key":    "AKIDEXAMPLE",
		"aws_secret": "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
		"aws_token":  "FQoGZXIvYXdzTokenValue",
	})
	if err := scope.Resolve(); err != nil {
		t.Fatal(err)
	}
	scope = scope.WithRuntimeSensitive(sensSet)

	s, err := NewSigner(map[string]any{
		"region":        "us-east-1",
		"service":       "service",
		"access_key":    "{{aws_key}}",
		"secret_key":    "{{aws_secret}}",
		"session_token": "{{aws_token}}",
	}, WithClock(frozenClock()))
	if err != nil {
		t.Fatal(err)
	}

	ctx := signer.WithScope(context.Background(), scope)
	req := &httpexec.Request{Method: "GET", URL: "http://example.amazonaws.com/"}
	if err := s.Sign(ctx, req, sensSet); err != nil {
		t.Fatal(err)
	}

	const wantToken = "FQoGZXIvYXdzTokenValue"
	found := false
	for _, v := range sensSet.Values() {
		if v == wantToken {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("session_token not registered as sensitive value; got %v", sensSet.Values())
	}
	// Also confirm x-amz-security-token was injected with the resolved value.
	if got := req.Headers["x-amz-security-token"]; got != wantToken {
		t.Errorf("x-amz-security-token = %q; want %q", got, wantToken)
	}
}

// TestSigV4_LiteralSecretKey_NotMarked verifies that a literal (non-variable)
// secret_key is NOT added to the sensitive set.
func TestSigV4_LiteralSecretKey_NotMarked(t *testing.T) {
	sensSet := variable.NewSensitiveSet()
	s, err := NewSigner(map[string]any{
		"region":     "us-east-1",
		"service":    "service",
		"access_key": "AKIDEXAMPLE",
		"secret_key": "literal-key",
	}, WithClock(frozenClock()))
	if err != nil {
		t.Fatal(err)
	}

	req := &httpexec.Request{Method: "GET", URL: "http://example.amazonaws.com/"}
	if err := s.Sign(context.Background(), req, sensSet); err != nil {
		t.Fatal(err)
	}

	for _, v := range sensSet.Values() {
		if v == "literal-key" {
			t.Errorf("literal secret_key was incorrectly marked as sensitive; values: %v", sensSet.Values())
		}
	}
}

// TestSigV4_MissingField_StructuredError verifies that missing required params
// return a structured error with code SIGNER_AWSSIGV4_MISSING_FIELD.
func TestSigV4_MissingField_StructuredError(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{"missing region", dropField("region"), "region"},
		{"missing service", dropField("service"), "service"},
		{"missing access_key", dropField("access_key"), "access_key"},
		{"missing secret_key", dropField("secret_key"), "secret_key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewSigner(tc.params)
			if err == nil {
				t.Fatal("expected error for missing field, got nil")
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("want *errors.Structured, got %T: %v", err, err)
			}
			if se.Code != "SIGNER_AWSSIGV4_MISSING_FIELD" {
				t.Errorf("Code = %q; want SIGNER_AWSSIGV4_MISSING_FIELD", se.Code)
			}
			if se.Category != apierrors.CategoryInput {
				t.Errorf("Category = %q; want %q", se.Category, apierrors.CategoryInput)
			}
			if !strings.Contains(se.Message, tc.want) {
				t.Errorf("Message %q does not name missing field %q", se.Message, tc.want)
			}
		})
	}
}

// TestFactory_DelegatesToNewSigner verifies Factory behaves identically to
// NewSigner with the same params.
func TestFactory_DelegatesToNewSigner(t *testing.T) {
	s, err := Factory(defaultParams())
	if err != nil {
		t.Fatalf("Factory: %v", err)
	}
	if s == nil {
		t.Fatal("Factory returned nil signer")
	}
}

// TestSigV4_MultiValueQueryParams verifies multi-value query param handling.
// URL-embedded ?Key=2&Key=1 + QueryParams{"Other":"x"} →
// canonical query == "Key=1&Key=2&Other=x" (sorted by key then value).
func TestSigV4_MultiValueQueryParams(t *testing.T) {
	s, err := NewSigner(defaultParams(), WithClock(frozenClock()))
	if err != nil {
		t.Fatal(err)
	}
	req := &httpexec.Request{
		Method:      "GET",
		URL:         "http://example.amazonaws.com/?Key=2&Key=1",
		QueryParams: map[string]string{"Other": "x"},
		Headers:     map[string]string{},
	}
	if err := s.Sign(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	auth := req.Headers["Authorization"]

	// Verify the canonical query string order via the package-internal helper.
	// canonicalQuery merges URL params + map params, sorts by key then value.
	// ?Key=2&Key=1 → [Key=1, Key=2]; + Other=x → [Key=1, Key=2, Other=x].
	u, _ := url.Parse(req.URL)
	gotCanonQuery := canonicalQuery(u, req.QueryParams)
	wantCanonQuery := "Key=1&Key=2&Other=x"
	if gotCanonQuery != wantCanonQuery {
		t.Errorf("canonical query:\n got: %s\nwant: %s", gotCanonQuery, wantCanonQuery)
	}

	// Byte-exact Authorization check — the signature encodes the canonical query,
	// so any sort-order deviation produces a different signature.
	const wantAuth = "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, " +
		"SignedHeaders=host;x-amz-date, " +
		"Signature=d85599bf1ea1ea2933cae92a7eaf7237d3a447b0f8f7479be63a4c8456e44098"
	if auth != wantAuth {
		t.Errorf("Authorization mismatch\n got: %s\nwant: %s", auth, wantAuth)
	}
}

// TestUriEncode_PercentEncoding verifies that uriEncode correctly percent-encodes
// non-unreserved characters (e.g. space → %20, slash → %2F) and passes through
// RFC 3986 unreserved characters unchanged.
func TestUriEncode_PercentEncoding(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"hello world", "hello%20world"},
		{"path/value", "path%2Fvalue"},
		{"a-z_0.9~", "a-z_0.9~"},     // all unreserved chars
		{"a+b=c&d", "a%2Bb%3Dc%26d"}, // common URL special chars
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := uriEncode(tc.input)
			if got != tc.want {
				t.Errorf("uriEncode(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestSigV4_InterpolationError verifies that when a {{var}} param references an
// undefined variable, Sign returns an error rather than silently using the literal
// template string as the value.
func TestSigV4_InterpolationError(t *testing.T) {
	// Create a scope without the variable referenced by region.
	scope := variable.NewScope(map[string]string{
		// "aws_region" intentionally absent — undefined variable.
		"aws_key":    "AKIDEXAMPLE",
		"aws_secret": "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
	})
	if err := scope.Resolve(); err != nil {
		t.Fatal(err)
	}

	s, err := NewSigner(map[string]any{
		"region":     "{{aws_region}}", // references undefined variable
		"service":    "service",
		"access_key": "{{aws_key}}",
		"secret_key": "{{aws_secret}}",
	}, WithClock(frozenClock()))
	if err != nil {
		t.Fatal(err)
	}

	ctx := signer.WithScope(context.Background(), scope)
	req := &httpexec.Request{Method: "GET", URL: "http://example.amazonaws.com/"}
	err = s.Sign(ctx, req, nil)
	if err == nil {
		t.Fatal("Sign must return an error when a {{var}} param references an undefined variable; got nil")
	}
	// The error must not be nil and must contain the problematic param name.
	if !strings.Contains(err.Error(), "aws_region") && !strings.Contains(err.Error(), "region") {
		t.Errorf("error message should reference the unresolved variable or param name, got: %v", err)
	}
}

// TestSigV4_SerializeBodyError verifies that Sign returns an error when the
// request body cannot be JSON-marshalled (e.g. a channel field), rather than
// silently using the empty-body hash which would produce a wrong Authorization.
func TestSigV4_SerializeBodyError(t *testing.T) {
	s, err := NewSigner(defaultParams(), WithClock(frozenClock()))
	if err != nil {
		t.Fatal(err)
	}
	// Channels cannot be marshalled to JSON — json.Marshal returns an error.
	req := &httpexec.Request{
		Method: "POST",
		URL:    "http://example.amazonaws.com/",
		Body:   make(chan int), // un-marshallable type
	}
	signErr := s.Sign(context.Background(), req, nil)
	if signErr == nil {
		t.Fatal("Sign must return an error when body cannot be JSON-marshalled; got nil")
	}
	if !strings.Contains(signErr.Error(), "serializing body") {
		t.Errorf("error must mention 'serializing body'; got: %v", signErr)
	}
}
