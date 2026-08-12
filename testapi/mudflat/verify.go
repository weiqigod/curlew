package mudflat

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 5849 specifies HMAC-SHA1 for OAuth 1.0a.
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Signature verification (§9.G).
//
// Written from the AWS Signature Version 4 documentation and RFC 5849, not from
// internal/signer. That distinction is the entire value of this file: a verifier
// derived from the implementation it verifies agrees with it perfectly and
// proves nothing. Two independent readings of the same specification agreeing is
// evidence; disagreeing localises the bug.
//
// TestSigV4_SignatureMatchesTheAWSVector pins this side against AWS's own
// published answer, so a mismatch with curlew is a statement about curlew.

// Fixed test credentials (Appendix B). Published, non-secret by design: they
// exist so a signature can be verified, and so redaction can be tested against
// values whose leakage is detectable.
const (
	MudflatSigV4AccessKey = "AKIAMUDFLATTEST0000"
	MudflatSigV4SecretKey = "wJalrMudflatEXAMPLEKEY/K7MDENG/bPxRfi"

	MudflatOAuthConsumerKey    = "mud_consumer_0001"
	MudflatOAuthConsumerSecret = "mud_consumer_secret_0001"
	MudflatOAuthTokenSecret    = "mud_token_secret_0001"
)

// sigV4Keys maps an access key id to its secret. The AWS documentation's own
// example credentials are here too, so the published test vector can be
// exercised through the endpoint rather than only through a unit test.
var sigV4Keys = map[string]string{
	MudflatSigV4AccessKey: MudflatSigV4SecretKey,
	"AKIDEXAMPLE":         "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
}

// SigV4Request is the subset of a request that SigV4 signs.
type SigV4Request struct {
	Method        string
	Path          string
	Query         string
	Headers       map[string][]string
	SignedHeaders []string
	Body          []byte
}

// SigV4Credentials identify the signer and scope the derived key.
type SigV4Credentials struct {
	AccessKey string
	SecretKey string
	Region    string
	Service   string
	// Date is the full ISO8601 basic timestamp, e.g. 20150830T123600Z.
	Date string
}

// SigV4Encode percent-encodes per RFC 3986, which is what SigV4 requires and
// what url.QueryEscape does not do: QueryEscape renders a space as "+" and
// leaves "~" encoded, and either difference produces a signature that can never
// match.
func SigV4Encode(s string) string {
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		switch {
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// encodePath encodes each path segment, leaving the separators alone.
func encodePath(path string) string {
	if path == "" {
		return "/"
	}
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		segments[i] = SigV4Encode(seg)
	}
	return strings.Join(segments, "/")
}

// canonicalQuery sorts parameters by encoded name and then by encoded value,
// and re-encodes both.
func canonicalQuery(raw string) string {
	if raw == "" {
		return ""
	}

	type pair struct{ name, value string }
	var pairs []pair
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		// The incoming query is already percent-encoded; decode before
		// re-encoding so the result is canonical rather than double-encoded.
		decodedName, err := url.QueryUnescape(name)
		if err != nil {
			decodedName = name
		}
		decodedValue, err := url.QueryUnescape(value)
		if err != nil {
			decodedValue = value
		}
		pairs = append(pairs, pair{SigV4Encode(decodedName), SigV4Encode(decodedValue)})
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].name != pairs[j].name {
			return pairs[i].name < pairs[j].name
		}
		return pairs[i].value < pairs[j].value
	})

	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, p.name+"="+p.value)
	}
	return strings.Join(out, "&")
}

// CanonicalRequest builds the SigV4 canonical request.
//
//	METHOD \n
//	CanonicalURI \n
//	CanonicalQueryString \n
//	CanonicalHeaders \n      (each header line already ends in \n)
//	SignedHeaders \n
//	HexEncode(SHA256(payload))
func CanonicalRequest(req SigV4Request) string {
	signed := make([]string, len(req.SignedHeaders))
	for i, name := range req.SignedHeaders {
		signed[i] = strings.ToLower(name)
	}
	sort.Strings(signed)

	lookup := map[string]string{}
	for name, values := range req.Headers {
		lookup[strings.ToLower(name)] = canonicalHeaderValue(values)
	}

	var headerBlock strings.Builder
	for _, name := range signed {
		fmt.Fprintf(&headerBlock, "%s:%s\n", name, lookup[name])
	}

	sum := sha256.Sum256(req.Body)

	return strings.Join([]string{
		strings.ToUpper(req.Method),
		encodePath(req.Path),
		canonicalQuery(req.Query),
		headerBlock.String() + "\n" + strings.Join(signed, ";"),
		hex.EncodeToString(sum[:]),
	}, "\n")
}

// canonicalHeaderValue trims each value and collapses internal runs of
// whitespace to a single space, per the SigV4 rules.
func canonicalHeaderValue(values []string) string {
	trimmed := make([]string, len(values))
	for i, v := range values {
		trimmed[i] = strings.Join(strings.Fields(v), " ")
	}
	return strings.Join(trimmed, ",")
}

// StringToSign builds the SigV4 string to sign from a canonical request.
func StringToSign(canonical string, creds SigV4Credentials) string {
	sum := sha256.Sum256([]byte(canonical))
	return strings.Join([]string{
		"AWS4-HMAC-SHA256",
		creds.Date,
		credentialScope(creds),
		hex.EncodeToString(sum[:]),
	}, "\n")
}

func credentialScope(creds SigV4Credentials) string {
	return strings.Join([]string{datestamp(creds.Date), creds.Region, creds.Service, "aws4_request"}, "/")
}

// datestamp takes the YYYYMMDD prefix of an ISO8601 basic timestamp.
func datestamp(amzDate string) string {
	if len(amzDate) >= 8 {
		return amzDate[:8]
	}
	return amzDate
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// SigningKey derives the scoped signing key: the chained HMAC over date,
// region, service and the terminator.
func SigningKey(creds SigV4Credentials) []byte {
	kDate := hmacSHA256([]byte("AWS4"+creds.SecretKey), datestamp(creds.Date))
	kRegion := hmacSHA256(kDate, creds.Region)
	kService := hmacSHA256(kRegion, creds.Service)
	return hmacSHA256(kService, "aws4_request")
}

// SigV4Signature runs the whole chain and returns the hex signature.
func SigV4Signature(req SigV4Request, creds SigV4Credentials) string {
	canonical := CanonicalRequest(req)
	toSign := StringToSign(canonical, creds)
	return hex.EncodeToString(hmacSHA256(SigningKey(creds), toSign))
}

// --- OAuth 1.0a (RFC 5849) --------------------------------------------------

// OAuth1BaseString builds the signature base string of RFC 5849 §3.4.1:
// uppercase method, the normalised base URI, and the normalised parameters,
// each percent-encoded and joined with "&".
func OAuth1BaseString(method, rawURL string, params map[string]string) string {
	names := make([]string, 0, len(params))
	for name := range params {
		if name == "oauth_signature" {
			// §3.4.1.3.1: the signature itself is never part of what it signs.
			continue
		}
		names = append(names, name)
	}

	encoded := make([]string, 0, len(names))
	for _, name := range names {
		encoded = append(encoded, SigV4Encode(name)+"="+SigV4Encode(params[name]))
	}
	sort.Strings(encoded)

	return strings.Join([]string{
		strings.ToUpper(method),
		SigV4Encode(oauthBaseURI(rawURL)),
		SigV4Encode(strings.Join(encoded, "&")),
	}, "&")
}

// oauthBaseURI normalises per §3.4.1.2: lowercase scheme and host, no query or
// fragment, and no port when it is the scheme's default.
func oauthBaseURI(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()

	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host = host + ":" + port
	}

	path := u.Path
	if path == "" {
		path = "/"
	}
	return scheme + "://" + host + path
}

// OAuth1Signature computes HMAC-SHA1 over the base string with the two-part
// key, base64-encoded. RFC 5849 §3.4.2 specifies SHA-1; it is not a choice.
func OAuth1Signature(baseString, consumerSecret, tokenSecret string) string {
	key := SigV4Encode(consumerSecret) + "&" + SigV4Encode(tokenSecret)
	h := hmac.New(sha1.New, []byte(key)) //nolint:gosec // required by RFC 5849
	h.Write([]byte(baseString))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// --- Endpoints --------------------------------------------------------------

// verifyCheck is one stage of the verification, reported whether it passed or
// not. A SigV4 mismatch is close to undebuggable without knowing which stage
// diverged, so the whole ladder is returned every time.
type verifyCheck struct {
	Stage    string `json:"stage"`
	OK       bool   `json:"ok"`
	Expected string `json:"expected,omitempty"`
	Received string `json:"received,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type verifyResult struct {
	OK        bool          `json:"ok"`
	Algorithm string        `json:"algorithm"`
	Stage     string        `json:"stage,omitempty"`
	Checks    []verifyCheck `json:"checks"`

	CanonicalRequest string `json:"canonical_request,omitempty"`
	StringToSign     string `json:"string_to_sign,omitempty"`
	BaseString       string `json:"base_string,omitempty"`

	Hint string `json:"hint,omitempty"`
}

func (s *Server) registerVerify() {
	s.register(Endpoint{
		Pattern:   "/verify/sigv4",
		Family:    "G",
		Summary:   "Recomputes the AWS SigV4 signature and returns a staged diff.",
		Exercises: "internal/signer's aws-sigv4 scheme, whose output no verifier had ever checked. The canonical request and string to sign are returned on failure because a bare mismatch is not actionable.",
		Handler:   s.handleVerifySigV4,
	})

	s.register(Endpoint{
		Pattern:   "/verify/oauth1",
		Family:    "G",
		Summary:   "Recomputes the OAuth 1.0a signature and returns the base string.",
		Exercises: "internal/signer's oauth1 scheme. The base string is returned on failure: parameter normalisation is where OAuth signatures go wrong, and it is invisible from the outside.",
		Handler:   s.handleVerifyOAuth1,
	})
}

func (s *Server) handleVerifySigV4(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		writeJSON(w, http.StatusUnauthorized, verifyResult{
			Algorithm: "AWS4-HMAC-SHA256",
			Stage:     "authorization_header",
			Checks:    []verifyCheck{{Stage: "authorization_header", OK: false, Detail: "no Authorization header"}},
			Hint:      "Sign the request with signing: { type: aws-sigv4 } and send the Authorization header.",
		})
		return
	}

	result := verifySigV4(r, auth, bodyFromContext(r.Context()))
	status := http.StatusOK
	if !result.OK {
		status = http.StatusUnauthorized
	}
	writeJSON(w, status, result)
}

// parseSigV4Authorization splits the Authorization header into its three parts.
func parseSigV4Authorization(auth string) (credential, signedHeaders, signature string, err error) {
	rest, ok := strings.CutPrefix(auth, "AWS4-HMAC-SHA256")
	if !ok {
		return "", "", "", fmt.Errorf("scheme is not AWS4-HMAC-SHA256")
	}

	for _, part := range strings.Split(rest, ",") {
		name, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
		}
		switch name {
		case "Credential":
			credential = value
		case "SignedHeaders":
			signedHeaders = value
		case "Signature":
			signature = value
		}
	}
	if credential == "" || signedHeaders == "" || signature == "" {
		return "", "", "", fmt.Errorf("missing Credential, SignedHeaders or Signature")
	}
	return credential, signedHeaders, signature, nil
}

func verifySigV4(r *http.Request, auth string, body []byte) verifyResult {
	out := verifyResult{Algorithm: "AWS4-HMAC-SHA256"}

	credential, signedHeaders, signature, err := parseSigV4Authorization(auth)
	if err != nil {
		out.Stage = "authorization_header"
		out.Checks = append(out.Checks, verifyCheck{Stage: "authorization_header", OK: false, Detail: err.Error()})
		out.Hint = "Expected: AWS4-HMAC-SHA256 Credential=<key>/<date>/<region>/<service>/aws4_request, SignedHeaders=..., Signature=..."
		return out
	}
	out.Checks = append(out.Checks, verifyCheck{Stage: "authorization_header", OK: true})

	parts := strings.Split(credential, "/")
	if len(parts) != 5 {
		out.Stage = "credential_scope"
		out.Checks = append(out.Checks, verifyCheck{
			Stage: "credential_scope", OK: false, Received: credential,
			Detail: "expected <access-key>/<yyyymmdd>/<region>/<service>/aws4_request",
		})
		return out
	}
	accessKey, scopeDate, region, service := parts[0], parts[1], parts[2], parts[3]
	out.Checks = append(out.Checks, verifyCheck{Stage: "credential_scope", OK: true, Received: credential})

	secret, known := sigV4Keys[accessKey]
	if !known {
		out.Stage = "credentials"
		out.Checks = append(out.Checks, verifyCheck{Stage: "credentials", OK: false, Received: accessKey})
		out.Hint = fmt.Sprintf("unknown access key %q — mudflat knows %s (see Appendix B of the specification)",
			accessKey, MudflatSigV4AccessKey)
		return out
	}
	out.Checks = append(out.Checks, verifyCheck{Stage: "credentials", OK: true, Received: accessKey})

	amzDate := r.Header.Get("X-Amz-Date")
	if amzDate == "" {
		out.Stage = "timestamp"
		out.Checks = append(out.Checks, verifyCheck{Stage: "timestamp", OK: false, Detail: "no X-Amz-Date header"})
		out.Hint = "SigV4 signs a timestamp; without X-Amz-Date the string to sign cannot be rebuilt."
		return out
	}
	if datestamp(amzDate) != scopeDate {
		out.Stage = "timestamp"
		out.Checks = append(out.Checks, verifyCheck{
			Stage: "timestamp", OK: false, Expected: scopeDate, Received: datestamp(amzDate),
			Detail: "X-Amz-Date disagrees with the date in the credential scope",
		})
		return out
	}
	out.Checks = append(out.Checks, verifyCheck{Stage: "timestamp", OK: true, Received: amzDate})

	names := strings.Split(signedHeaders, ";")
	headers := collectSignedHeaders(r, names)

	var missing []string
	for _, name := range names {
		if _, ok := headers[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		out.Stage = "signed_headers"
		out.Checks = append(out.Checks, verifyCheck{
			Stage: "signed_headers", OK: false, Received: signedHeaders,
			Detail: "listed in SignedHeaders but absent from the request: " + strings.Join(missing, ", "),
		})
		out.Hint = "Every name in SignedHeaders must be a header actually sent, or the canonical request cannot be rebuilt."
		return out
	}
	out.Checks = append(out.Checks, verifyCheck{Stage: "signed_headers", OK: true, Received: signedHeaders})

	sigReq := SigV4Request{
		Method:        r.Method,
		Path:          r.URL.Path,
		Query:         r.URL.RawQuery,
		Headers:       headers,
		SignedHeaders: names,
		Body:          body,
	}
	creds := SigV4Credentials{
		AccessKey: accessKey, SecretKey: secret,
		Region: region, Service: service, Date: amzDate,
	}

	out.CanonicalRequest = CanonicalRequest(sigReq)
	out.StringToSign = StringToSign(out.CanonicalRequest, creds)
	expected := hex.EncodeToString(hmacSHA256(SigningKey(creds), out.StringToSign))

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		out.Stage = "signature"
		out.Checks = append(out.Checks, verifyCheck{
			Stage: "signature", OK: false, Expected: expected, Received: signature,
		})
		out.Hint = "The earlier stages agree, so the divergence is inside the canonical request or the string to sign — both are returned above. Compare them line by line against what the signer built."
		return out
	}

	out.Checks = append(out.Checks, verifyCheck{Stage: "signature", OK: true})
	out.OK = true
	return out
}

// collectSignedHeaders gathers the named headers from the request. Host is
// special: Go moves it off the header map onto Request.Host.
func collectSignedHeaders(r *http.Request, names []string) map[string][]string {
	out := map[string][]string{}
	for _, name := range names {
		lower := strings.ToLower(name)
		if lower == "host" {
			host := r.Host
			if host == "" {
				host = r.URL.Host
			}
			if host != "" {
				out["host"] = []string{host}
			}
			continue
		}
		if values, ok := r.Header[http.CanonicalHeaderKey(name)]; ok {
			out[lower] = values
		}
	}
	return out
}

func (s *Server) handleVerifyOAuth1(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		writeJSON(w, http.StatusUnauthorized, verifyResult{
			Algorithm: "OAuth 1.0a HMAC-SHA1",
			Stage:     "authorization_header",
			Checks:    []verifyCheck{{Stage: "authorization_header", OK: false, Detail: "no Authorization header"}},
			Hint:      "Sign the request with signing: { type: oauth1 } and send the Authorization header.",
		})
		return
	}

	result := verifyOAuth1(r, auth)
	status := http.StatusOK
	if !result.OK {
		status = http.StatusUnauthorized
	}
	writeJSON(w, status, result)
}

func verifyOAuth1(r *http.Request, auth string) verifyResult {
	out := verifyResult{Algorithm: "OAuth 1.0a HMAC-SHA1"}

	rest, ok := strings.CutPrefix(auth, "OAuth ")
	if !ok {
		out.Stage = "authorization_header"
		out.Checks = append(out.Checks, verifyCheck{
			Stage: "authorization_header", OK: false, Detail: "scheme is not OAuth",
		})
		return out
	}

	params := parseOAuthHeader(rest)
	provided := params["oauth_signature"]
	if provided == "" {
		out.Stage = "authorization_header"
		out.Checks = append(out.Checks, verifyCheck{
			Stage: "authorization_header", OK: false, Detail: "no oauth_signature parameter",
		})
		return out
	}
	out.Checks = append(out.Checks, verifyCheck{Stage: "authorization_header", OK: true})

	if key := params["oauth_consumer_key"]; key != MudflatOAuthConsumerKey {
		out.Stage = "credentials"
		out.Checks = append(out.Checks, verifyCheck{
			Stage: "credentials", OK: false, Expected: MudflatOAuthConsumerKey, Received: key,
		})
		out.Hint = "See Appendix B of the specification for the fixed consumer key."
		return out
	}
	out.Checks = append(out.Checks, verifyCheck{Stage: "credentials", OK: true})

	// Query parameters join the oauth_* parameters in the signature base
	// string (RFC 5849 §3.4.1.3.1).
	for name, values := range r.URL.Query() {
		if len(values) > 0 {
			params[name] = values[0]
		}
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	fullURL := scheme + "://" + r.Host + r.URL.Path

	out.BaseString = OAuth1BaseString(r.Method, fullURL, params)
	expected := OAuth1Signature(out.BaseString, MudflatOAuthConsumerSecret, MudflatOAuthTokenSecret)

	if !hmac.Equal([]byte(expected), []byte(provided)) {
		out.Stage = "signature"
		out.Checks = append(out.Checks, verifyCheck{
			Stage: "signature", OK: false, Expected: expected, Received: provided,
		})
		out.Hint = "Compare the base string above against the one the signer built. Parameter normalisation — sorting, encoding, and which parameters are included — is where OAuth signatures diverge."
		return out
	}

	out.Checks = append(out.Checks, verifyCheck{Stage: "signature", OK: true})
	out.OK = true
	return out
}

// parseOAuthHeader splits the comma-separated, quoted parameters of an OAuth
// Authorization header, percent-decoding each value.
func parseOAuthHeader(rest string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(rest, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		value = strings.Trim(value, `"`)
		decoded, err := url.QueryUnescape(value)
		if err != nil {
			decoded = value
		}
		if name == "realm" {
			// §3.4.1.3.1: realm is excluded from the signature base string.
			continue
		}
		out[name] = decoded
	}
	return out
}
