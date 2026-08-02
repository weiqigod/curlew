// Package oauth1 implements the OAuth 1.0a HMAC-SHA1 (default) and
// HMAC-SHA256 (opt-in) request signer for the signing: field,
// registered as type "oauth1".
//
// # Algorithm
//
// The implementation follows RFC 5849:
// https://datatracker.ietf.org/doc/html/rfc5849
//
//  1. Build the parameter string (oauth_* + URL query + form body).
//  2. Build the signature base string (METHOD&URI&PARAMS).
//  3. Build the signing key (consumer_secret&token_secret).
//  4. Compute HMAC-SHA1 (default) or HMAC-SHA256 (opt-in) signature.
//  5. Assemble the Authorization header.
//
// # Test seams
//
// The spec map may carry optional "nonce" and "timestamp" string keys to
// override the auto-generated values. These are exclusively test seams —
// production users must not set them.
//
// # Sensitive secret propagation
//
// When consumer_secret or token_secret resolves from a sensitive
// variable, Sign calls sensitives.AddValue(resolved) so the value is
// redacted in serialised request output. Literal secrets — no {{var}}
// reference — cannot be back-traced and are NOT marked. This mirrors
// the M12-005 $hmacSha256 pattern. See docs/MANUAL.md §6.8.
//
// # Method support
//
// Only HMAC-SHA1 and HMAC-SHA256 are supported. RSA-SHA1 (requires
// PEM-key handling) and PLAINTEXT (security footgun) are explicitly
// rejected per Open Decision #3.
package oauth1

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"hash"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	apierrors "github.com/peterlindqvist/apitest/internal/errors"
	"github.com/peterlindqvist/apitest/internal/httpexec"
	"github.com/peterlindqvist/apitest/internal/signer"
	"github.com/peterlindqvist/apitest/internal/variable"
)

// MethodHMACSHA1 and MethodHMACSHA256 are the only accepted signature methods.
const (
	MethodHMACSHA1   = "HMAC-SHA1"
	MethodHMACSHA256 = "HMAC-SHA256"
)

// config holds clock and nonce seams for the signer instance.
type config struct {
	now         func() time.Time
	nonceSource io.Reader
}

// Option mutates internal signer config.
type Option func(*config)

// WithClock pins the signer's clock to now. Defaults to time.Now.
// Tests use this to inject frozen clocks reproducing published RFC test vectors.
func WithClock(now func() time.Time) Option {
	return func(c *config) { c.now = now }
}

// WithNonceSource pins the signer's nonce entropy source. Defaults to
// crypto/rand.Reader. Tests use this to inject deterministic bytes.
func WithNonceSource(r io.Reader) Option {
	return func(c *config) { c.nonceSource = r }
}

// oauth1Signer is the concrete OAuth 1.0a signer.
type oauth1Signer struct {
	rawParams map[string]any
	method    string // MethodHMACSHA1 or MethodHMACSHA256
	cfg       config
}

// Factory is the signer.Factory exported under the registered name "oauth1".
// It validates required params (consumer_key, consumer_secret) and the
// optional method up-front and returns a Signer that resolves
// {{interpolation}} at Sign time via the scope attached to the context.
// Production callers should go through this function; tests should use
// NewSigner directly to inject a frozen clock or nonce source.
func Factory(params map[string]any) (signer.Signer, error) {
	return NewSigner(params)
}

// NewSigner is the programmatic constructor. Tests use this to inject a frozen
// clock via WithClock or a deterministic nonce via WithNonceSource.
// Production callers go through Factory.
func NewSigner(params map[string]any, opts ...Option) (signer.Signer, error) {
	cfg := config{now: time.Now, nonceSource: rand.Reader}
	for _, o := range opts {
		o(&cfg)
	}

	method, err := validateMethod(params)
	if err != nil {
		return nil, err
	}

	for _, field := range []string{"consumer_key", "consumer_secret"} {
		v, _ := params[field].(string)
		if strings.TrimSpace(v) == "" {
			return nil, &apierrors.Structured{
				Category: apierrors.CategoryInput,
				Code:     "SIGNER_OAUTH1_MISSING_FIELD",
				Message:  fmt.Sprintf("oauth1 signer: required param %q is missing or empty", field),
				Hint:     fmt.Sprintf("Add %q to the signing.params block", field),
			}
		}
	}

	// Deep-copy params to avoid mutation surprises.
	cp := make(map[string]any, len(params))
	for k, v := range params {
		cp[k] = v
	}
	return &oauth1Signer{rawParams: cp, method: method, cfg: cfg}, nil
}

// validateMethod returns the canonical method string ("HMAC-SHA1" or
// "HMAC-SHA256") or a structured error for unsupported methods.
func validateMethod(params map[string]any) (string, error) {
	v, _ := params["method"].(string)
	v = strings.TrimSpace(v)
	if v == "" {
		return MethodHMACSHA1, nil
	}
	switch v {
	case MethodHMACSHA1, MethodHMACSHA256:
		return v, nil
	}
	return "", &apierrors.Structured{
		Category: apierrors.CategoryInput,
		Code:     "SIGNER_OAUTH1_UNSUPPORTED_METHOD",
		Message: fmt.Sprintf(
			"oauth1 signer: unsupported method %q; only HMAC-SHA1 and HMAC-SHA256 are supported", v),
		Hint: "Set method to HMAC-SHA1 (default) or HMAC-SHA256. RSA-SHA1 and PLAINTEXT are not supported.",
	}
}

// Sign mutates req in place to add OAuth 1.0a signing material. It resolves
// {{var}} references in params via the *variable.Scope attached to ctx
// (when present), calls sensitives.AddValue for consumer_secret / token_secret
// when their sources are sensitive variables, then injects the Authorization
// header.
func (s *oauth1Signer) Sign(ctx context.Context, req *httpexec.Request, sensitives *variable.SensitiveSet) error {
	scope := signer.ScopeFromContext(ctx)
	resolved, err := resolveParams(s.rawParams, scope)
	if err != nil {
		return err
	}

	// Re-validate required fields post-interpolation.
	for _, field := range []string{"consumer_key", "consumer_secret"} {
		if strings.TrimSpace(resolved[field]) == "" {
			return &apierrors.Structured{
				Category: apierrors.CategoryInput,
				Code:     "SIGNER_OAUTH1_MISSING_FIELD",
				Message:  fmt.Sprintf("oauth1 signer: required param %q resolved to empty", field),
				Hint:     fmt.Sprintf("Ensure the variable for %q is set in the current environment", field),
			}
		}
	}

	consumerKey := resolved["consumer_key"]
	consumerSecret := resolved["consumer_secret"]
	token := resolved["token"]
	tokenSecret := resolved["token_secret"]
	realm := resolved["realm"]

	// M12-005 mirror: register resolved sensitive values for redaction.
	if sensitives != nil {
		for _, field := range []string{"consumer_secret", "token_secret"} {
			raw, _ := s.rawParams[field].(string)
			if varName := extractVarName(raw); varName != "" && sensitives.IsSensitive(varName) {
				switch field {
				case "consumer_secret":
					sensitives.AddValue(consumerSecret)
				case "token_secret":
					sensitives.AddValue(tokenSecret)
				}
			}
		}
	}

	// Resolve nonce + timestamp (test-seam overrides take precedence).
	nonce, err := resolveNonce(resolved["nonce"], s.cfg.nonceSource)
	if err != nil {
		return fmt.Errorf("oauth1: generating nonce: %w", err)
	}
	timestamp := resolved["timestamp"]
	if timestamp == "" {
		timestamp = fmt.Sprintf("%d", s.cfg.now().Unix())
	}

	// Assemble protocol params for base string + header.
	protoParams := map[string]string{
		"oauth_consumer_key":     consumerKey,
		"oauth_nonce":            nonce,
		"oauth_signature_method": s.method,
		"oauth_timestamp":        timestamp,
		"oauth_version":          "1.0",
	}
	if token != "" {
		protoParams["oauth_token"] = token
	}

	base, err := buildBaseString(req, protoParams)
	if err != nil {
		return fmt.Errorf("oauth1: building base string: %w", err)
	}
	key := buildSigningKey(consumerSecret, tokenSecret)
	sig := computeSignature(s.method, key, base)

	// Inject signature and serialise the header.
	protoParams["oauth_signature"] = sig

	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	req.Headers["Authorization"] = buildAuthHeader(realm, protoParams)
	return nil
}

// buildBaseStringForTest exposes buildBaseString to tests in this package
// WITHOUT calling Sign (so it does not mutate req or generate a fresh nonce).
// It reads the pinned nonce/timestamp directly from rawParams.
func (s *oauth1Signer) buildBaseStringForTest(req *httpexec.Request) (string, error) {
	nonce, _ := s.rawParams["nonce"].(string)
	timestamp, _ := s.rawParams["timestamp"].(string)
	consumerKey, _ := s.rawParams["consumer_key"].(string)
	token, _ := s.rawParams["token"].(string)
	p := map[string]string{
		"oauth_consumer_key":     consumerKey,
		"oauth_nonce":            nonce,
		"oauth_signature_method": s.method,
		"oauth_timestamp":        timestamp,
		"oauth_version":          "1.0",
	}
	if token != "" {
		p["oauth_token"] = token
	}
	return buildBaseString(req, p)
}

// resolveNonce returns override when non-empty, else generates 16 base32
// chars from src (10 random bytes, base32-encoded without padding).
func resolveNonce(override string, src io.Reader) (string, error) {
	if override != "" {
		return override, nil
	}
	buf := make([]byte, 10) // 10 bytes → 16 base32 chars (no padding)
	if _, err := io.ReadFull(src, buf); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf), nil
}

// resolveParams interpolates {{var}} references in params using scope (when
// non-nil). Returns a map of resolved string values keyed by param name, or
// an error when a {{var}} reference cannot be resolved.
func resolveParams(rawParams map[string]any, scope *variable.Scope) (map[string]string, error) {
	resolved := make(map[string]string, len(rawParams))
	for k, v := range rawParams {
		raw, _ := v.(string)
		if scope == nil || !strings.Contains(raw, "{{") {
			resolved[k] = raw
			continue
		}
		interp, err := scope.Interpolate(raw)
		if err != nil {
			return nil, fmt.Errorf("oauth1: param %q interpolation failed: %w", k, err)
		}
		resolved[k] = interp
	}
	return resolved, nil
}

// extractVarName returns the variable name from a simple "{{name}}" expression,
// or "" for anything more complex (dynamic functions, multiple references, etc.).
func extractVarName(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{{") || !strings.HasSuffix(s, "}}") {
		return ""
	}
	inner := s[2 : len(s)-2]
	// Reject dynamic functions and complex expressions.
	if strings.Contains(inner, "$") || strings.Contains(inner, " ") || strings.Contains(inner, "{{") {
		return ""
	}
	return inner
}

// uriEncode percent-encodes s per RFC 3986 unreserved characters.
// Unreserved: A-Z a-z 0-9 - _ . ~
func uriEncode(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if isUnreserved(c) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~'
}

// kv is a key-value pair for parameter sorting.
type kv struct{ k, v string }

// buildBaseString constructs the RFC 5849 §3.4.1 signature base string.
func buildBaseString(req *httpexec.Request, protoParams map[string]string) (string, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}

	method := strings.ToUpper(req.Method)
	basePath := u.EscapedPath()
	if basePath == "" {
		basePath = "/"
	}
	baseURI := u.Scheme + "://" + u.Host + basePath

	// Collect parameters: protocol + URL query + QueryParams map + form body.
	var params []kv
	for k, v := range protoParams {
		params = append(params, kv{k, v})
	}
	for k, vs := range u.Query() {
		for _, v := range vs {
			params = append(params, kv{k, v})
		}
	}
	for k, v := range req.QueryParams {
		params = append(params, kv{k, v})
	}
	if isFormBody(req) {
		formParams, err := parseFormBody(req.Body)
		if err != nil {
			return "", err
		}
		for k, vs := range formParams {
			for _, v := range vs {
				params = append(params, kv{k, v})
			}
		}
	}

	// Each name and value percent-encoded; sort by key then by value.
	encoded := make([]kv, 0, len(params))
	for _, p := range params {
		encoded = append(encoded, kv{uriEncode(p.k), uriEncode(p.v)})
	}
	sort.Slice(encoded, func(i, j int) bool {
		if encoded[i].k != encoded[j].k {
			return encoded[i].k < encoded[j].k
		}
		return encoded[i].v < encoded[j].v
	})

	parts := make([]string, len(encoded))
	for i, p := range encoded {
		parts[i] = p.k + "=" + p.v
	}
	paramString := strings.Join(parts, "&")

	return method + "&" + uriEncode(baseURI) + "&" + uriEncode(paramString), nil
}

// buildSigningKey constructs the RFC 5849 §3.4.2 signing key:
// percent_encode(consumer_secret) + "&" + percent_encode(token_secret).
// token_secret may be empty; the trailing "&" is preserved.
func buildSigningKey(consumerSecret, tokenSecret string) []byte {
	return []byte(uriEncode(consumerSecret) + "&" + uriEncode(tokenSecret))
}

// computeSignature computes HMAC-SHA1 (default) or HMAC-SHA256 (opt-in) over
// base with key, returning the base64-encoded digest.
func computeSignature(method string, key []byte, base string) string {
	var h hash.Hash
	switch method {
	case MethodHMACSHA256:
		h = hmac.New(sha256.New, key)
	default: // MethodHMACSHA1
		h = hmac.New(sha1.New, key)
	}
	h.Write([]byte(base))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// buildAuthHeader serialises the Authorization header value per RFC 5849 §3.5.1.
// realm (when non-empty) is placed first. All other keys from protoParams are
// sorted alphabetically. Values are RFC 3986 percent-encoded and double-quoted.
func buildAuthHeader(realm string, protoParams map[string]string) string {
	keys := make([]string, 0, len(protoParams))
	for k := range protoParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys)+1)
	if realm != "" {
		parts = append(parts, `realm="`+uriEncode(realm)+`"`)
	}
	for _, k := range keys {
		parts = append(parts, k+`="`+uriEncode(protoParams[k])+`"`)
	}
	return "OAuth " + strings.Join(parts, ", ")
}

// isFormBody returns true when the request's Content-Type is
// application/x-www-form-urlencoded (case-insensitive; charset params tolerated).
func isFormBody(req *httpexec.Request) bool {
	ct := strings.ToLower(req.Headers["Content-Type"])
	return strings.HasPrefix(ct, "application/x-www-form-urlencoded")
}

// parseFormBody parses a form-urlencoded body. Only string and []byte body
// types are recognised; other types return (nil, nil).
func parseFormBody(body any) (url.Values, error) {
	var raw string
	switch b := body.(type) {
	case string:
		raw = b
	case []byte:
		raw = string(b)
	default:
		return nil, nil
	}
	return url.ParseQuery(raw)
}
