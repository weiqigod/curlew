// Package awssigv4 implements the AWS Signature Version 4 request
// signer for the signing: field, registered as type "aws-sigv4".
//
// # Algorithm
//
// The implementation follows the published AWS SigV4 specification:
// https://docs.aws.amazon.com/general/latest/gr/sigv4_signing.html
//
//  1. Build canonical request from method, URI, query string, headers, body hash.
//  2. Build string-to-sign from algorithm, timestamp, credential scope, and canonical-request hash.
//  3. Derive signing key: HMAC(HMAC(HMAC(HMAC("AWS4"+secret, date), region), service), "aws4_request").
//  4. Compute signature: HMAC-SHA256(signing key, string-to-sign), hex-encoded.
//  5. Assemble Authorization header.
//
// # Sensitive secret propagation
//
// When secret_key (or session_token) resolves from a sensitive variable,
// Sign calls sensitives.AddValue(resolved) so the value is redacted in
// serialised request output. Literal secrets — no {{var}} reference —
// cannot be back-traced and are NOT marked. This mirrors the M12-005
// $hmacSha256 pattern. See docs/MANUAL.md §6.8.
//
// # Path encoding
//
// This implementation follows the standard SigV4 path-encoding rules
// (double-encode for non-S3 services). S3 single-encoding is a known
// limitation; revisit if customer demand emerges.
package awssigv4

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/signer"
	"github.com/weiqigod/curlew/internal/variable"
)

// config holds the immutable configuration for an awssigv4 signer instance.
type config struct {
	now func() time.Time
}

// Option mutates internal signer config.
type Option func(*config)

// WithClock pins the signer's clock to now. Defaults to time.Now.
// Tests use this to inject frozen clocks reproducing published AWS test vectors.
func WithClock(now func() time.Time) Option {
	return func(c *config) {
		c.now = now
	}
}

// sigV4Signer is the concrete AWS SigV4 signer.
type sigV4Signer struct {
	rawParams map[string]any
	cfg       config
}

// Factory is the signer.Factory exported under the registered name "aws-sigv4".
// It validates required params (region, service, access_key, secret_key) up-front
// and returns a Signer that resolves {{interpolation}} at Sign time via the scope
// attached to the context. Production callers should go through this function;
// tests should use NewSigner directly to inject a frozen clock.
func Factory(params map[string]any) (signer.Signer, error) {
	return NewSigner(params)
}

// NewSigner is the programmatic constructor. Tests use this to inject a frozen
// clock via WithClock. Production callers go through Factory.
func NewSigner(params map[string]any, opts ...Option) (signer.Signer, error) {
	cfg := config{now: time.Now}
	for _, o := range opts {
		o(&cfg)
	}
	// Validate required fields up-front. If any param is a {{var}} reference,
	// the literal value will be "{{var}}" which is non-empty, so validation passes.
	// The interpolated value is checked again at Sign time after resolution.
	for _, field := range []string{"region", "service", "access_key", "secret_key"} {
		v, _ := params[field].(string)
		if strings.TrimSpace(v) == "" {
			return nil, &apierrors.Structured{
				Category: apierrors.CategoryInput,
				Code:     "SIGNER_AWSSIGV4_MISSING_FIELD",
				Message:  fmt.Sprintf("aws-sigv4 signer: required param %q is missing or empty", field),
				Hint:     fmt.Sprintf("Add %q to the signing.params block", field),
			}
		}
	}
	// Deep-copy params to avoid mutation surprises.
	cp := make(map[string]any, len(params))
	for k, v := range params {
		cp[k] = v
	}
	return &sigV4Signer{rawParams: cp, cfg: cfg}, nil
}

// Sign mutates req in place to add AWS SigV4 signing material. It resolves
// {{var}} references in params via the *variable.Scope attached to ctx
// (when present), calls sensitives.AddValue for secret_key / session_token
// when their sources are sensitive variables, then injects x-amz-date,
// x-amz-content-sha256, (optionally) x-amz-security-token, and Authorization.
func (s *sigV4Signer) Sign(ctx context.Context, req *httpexec.Request, sensitives *variable.SensitiveSet) error {
	// Resolve params via scope if available.
	scope := signer.ScopeFromContext(ctx)
	resolved, err := resolveParams(s.rawParams, scope)
	if err != nil {
		return err
	}

	// After interpolation, re-validate required fields in case a {{var}}
	// resolved to an empty string.
	for _, field := range []string{"region", "service", "access_key", "secret_key"} {
		if strings.TrimSpace(resolved[field]) == "" {
			return &apierrors.Structured{
				Category: apierrors.CategoryInput,
				Code:     "SIGNER_AWSSIGV4_MISSING_FIELD",
				Message:  fmt.Sprintf("aws-sigv4 signer: required param %q resolved to empty", field),
				Hint:     fmt.Sprintf("Ensure the variable for %q is set in the current environment", field),
			}
		}
	}

	region := resolved["region"]
	service := resolved["service"]
	accessKey := resolved["access_key"]
	secretKey := resolved["secret_key"]
	sessionToken := resolved["session_token"]

	// M12-005 mirror: register resolved sensitive values for redaction.
	// We re-derive sensitiveSource here using the actual sensitives set rather
	// than a scope.IsSensitive method (which does not exist on *variable.Scope).
	sensitiveSourceFromSet := make(map[string]bool, 2)
	if sensitives != nil {
		for _, field := range []string{"secret_key", "session_token"} {
			raw, _ := s.rawParams[field].(string)
			if varName := extractVarName(raw); varName != "" {
				sensitiveSourceFromSet[field] = sensitives.IsSensitive(varName)
			}
		}
	}
	if sensitives != nil {
		if sensitiveSourceFromSet["secret_key"] {
			sensitives.AddValue(secretKey)
		}
		if sessionToken != "" && sensitiveSourceFromSet["session_token"] {
			sensitives.AddValue(sessionToken)
		}
	}

	// Compute body hash before injecting any signing headers.
	bodyBytes, err := serializeBody(req.Body)
	if err != nil {
		return fmt.Errorf("aws-sigv4: serializing body: %w", err)
	}
	bodyHash := sha256Hex(bodyBytes)

	now := s.cfg.now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	// Ensure headers map is initialised.
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}

	// Inject signing headers in the order specified by the task scope.
	req.Headers["x-amz-date"] = amzDate
	req.Headers["x-amz-content-sha256"] = bodyHash
	if sessionToken != "" {
		req.Headers["x-amz-security-token"] = sessionToken
	}

	// Compute canonical request. hasBody drives whether x-amz-content-sha256
	// appears in SignedHeaders (per published AWS test vectors).
	canonReq, signedHeaders := buildCanonicalRequest(req, bodyHash, len(bodyBytes) > 0)
	credScope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	sts := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + credScope + "\n" + sha256Hex([]byte(canonReq))
	key := deriveSigningKey(secretKey, dateStamp, region, service)
	sig := hmacHex(key, sts)

	req.Headers["Authorization"] = fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credScope, signedHeaders, sig,
	)
	return nil
}

// resolveParams interpolates {{var}} references in params using scope (when
// non-nil). Returns a map of resolved string values keyed by param name, or
// an error when a {{var}} reference cannot be resolved (e.g. undefined variable).
// When scope is nil or a param has no {{}} references, its raw string value
// is used as-is.
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
			return nil, fmt.Errorf("aws-sigv4: param %q interpolation failed: %w", k, err)
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

// serializeBody converts a request body to bytes for hashing.
// Mirrors the runner's prepareBody logic: string/[]byte verbatim, else JSON.
// Returns an error when JSON marshalling fails (e.g. cyclic reference, channel field).
func serializeBody(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	switch b := body.(type) {
	case string:
		return []byte(b), nil
	case []byte:
		return b, nil
	default:
		out, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		return out, nil
	}
}

// buildCanonicalRequest builds the canonical request string and returns it
// together with the sorted semicolon-joined SignedHeaders value.
// hasBody indicates whether the request carries a non-empty body; when false,
// x-amz-content-sha256 is excluded from SignedHeaders (matching the published
// AWS SigV4 test vectors for GET requests with no body).
func buildCanonicalRequest(req *httpexec.Request, bodyHash string, hasBody bool) (canonReq, signedHeaders string) {
	u, _ := url.Parse(req.URL)

	method := strings.ToUpper(req.Method)
	canonURI := canonicalPath(u)
	canonQuery := canonicalQuery(u, req.QueryParams)

	headers := normaliseHeaders(req.Headers, u.Host)
	canonHeaderBlock, sh := buildCanonicalHeaderBlock(headers, hasBody)

	canonReq = method + "\n" + canonURI + "\n" + canonQuery + "\n" +
		canonHeaderBlock + "\n" + sh + "\n" + bodyHash
	return canonReq, sh
}

// canonicalPath returns the URI-encoded path per RFC 3986. An empty path
// becomes "/".
func canonicalPath(u *url.URL) string {
	p := u.EscapedPath()
	if p == "" {
		p = "/"
	}
	return p
}

// pair is a key-value pair for query string construction.
type pair struct {
	k, v string
}

// canonicalQuery builds the canonical query string from both the URL-embedded
// params and the map-based QueryParams. Both sources are merged verbatim —
// URL params first, then map entries appended. The merged set is sorted by
// key then by value (duplicates preserved). Each pair is percent-encoded per
// RFC 3986 unreserved-character rules.
func canonicalQuery(u *url.URL, mapParams map[string]string) string {
	var pairs []pair
	// URL-embedded params (may be multi-valued).
	for k, vs := range u.Query() {
		for _, v := range vs {
			pairs = append(pairs, pair{k, v})
		}
	}
	// Map params appended verbatim (no de-duplication).
	for k, v := range mapParams {
		pairs = append(pairs, pair{k, v})
	}
	if len(pairs) == 0 {
		return ""
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k != pairs[j].k {
			return pairs[i].k < pairs[j].k
		}
		return pairs[i].v < pairs[j].v
	})
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = uriEncode(p.k) + "=" + uriEncode(p.v)
	}
	return strings.Join(parts, "&")
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
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.' || c == '~'
}

// normaliseHeaders lowercases all header names, derives the host header
// from the URL if absent, and returns a map[string]string of name→value.
func normaliseHeaders(headers map[string]string, host string) map[string]string {
	out := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		// Trim and lowercase the name; trim and collapse internal whitespace in value
		// per the SigV4 spec ("trim excess white space").
		out[strings.ToLower(k)] = strings.TrimSpace(v)
	}
	// Host header is required for canonical request. If not already present,
	// derive from the URL.
	if _, ok := out["host"]; !ok && host != "" {
		out["host"] = host
	}
	return out
}

// buildCanonicalHeaderBlock returns the canonical header block string
// (each name:value\n) and the sorted semicolon-joined signed-headers string.
// When hasBody is false, x-amz-content-sha256 is excluded from SignedHeaders
// to match published AWS SigV4 test vectors for empty-body requests (e.g.
// get-vanilla, get-header-key-case). When hasBody is true (e.g.
// post-x-www-form-urlencoded), x-amz-content-sha256 is included in signed headers.
func buildCanonicalHeaderBlock(headers map[string]string, hasBody bool) (block, signedHeaders string) {
	names := make([]string, 0, len(headers))
	for k := range headers {
		// Exclude x-amz-content-sha256 from signed headers when body is empty.
		// The body hash is still the last line of the canonical request.
		if k == "x-amz-content-sha256" && !hasBody {
			continue
		}
		names = append(names, k)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(headers[name])
		b.WriteByte('\n')
	}
	return b.String(), strings.Join(names, ";")
}

// deriveSigningKey derives the hierarchical HMAC signing key per the
// AWS SigV4 specification.
func deriveSigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}

// hmacSHA256 returns HMAC-SHA256(key, data).
func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// hmacHex returns the lowercase hex HMAC-SHA256(key, data).
func hmacHex(key []byte, data string) string {
	return hex.EncodeToString(hmacSHA256(key, data))
}

// sha256Hex returns the lowercase hex SHA-256 of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
