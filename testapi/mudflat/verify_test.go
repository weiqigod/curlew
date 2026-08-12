package mudflat

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Signature verification (§9.G).
//
// internal/signer has emitted AWS SigV4 and OAuth 1.0a signatures since it was
// written and no verifier has ever checked one. This is the highest-information
// endpoint family in the specification for exactly that reason.
//
// The implementation under testapi/ is written from the AWS SigV4 documentation
// and RFC 5849, NOT from reading internal/signer. Deriving it from curlew's own
// code would rebuild the closed loop this directory exists to break: two copies
// of the same misreading agree perfectly.
//
// The tests below pin it against published test vectors, so "curlew and mudflat
// disagree" is an unambiguous statement about curlew rather than a coin toss
// between two unverified implementations.

// --- SigV4 against the published AWS test vector ----------------------------

// The get-vanilla case from the AWS Signature Version 4 test suite. These exact
// values appear in AWS's own documentation, which is what makes them worth
// pinning: they verify mudflat's implementation against AWS rather than against
// anything in this repository.
const (
	awsVectorAccessKey = "AKIDEXAMPLE"
	awsVectorSecret    = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
	awsVectorRegion    = "us-east-1"
	awsVectorService   = "service"
	awsVectorDate      = "20150830T123600Z"
	awsVectorHost      = "example.amazonaws.com"

	awsVectorCanonicalRequest = "GET\n" +
		"/\n" +
		"\n" +
		"host:example.amazonaws.com\n" +
		"x-amz-date:20150830T123600Z\n" +
		"\n" +
		"host;x-amz-date\n" +
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	awsVectorSignature = "5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"
)

func TestSigV4_CanonicalRequestMatchesTheAWSVector(t *testing.T) {
	req := SigV4Request{
		Method: "GET",
		Path:   "/",
		Query:  "",
		Headers: map[string][]string{
			"Host":       {awsVectorHost},
			"X-Amz-Date": {awsVectorDate},
		},
		SignedHeaders: []string{"host", "x-amz-date"},
		Body:          nil,
	}

	got := CanonicalRequest(req)
	if got != awsVectorCanonicalRequest {
		t.Errorf("canonical request does not match the AWS vector.\n got: %q\nwant: %q",
			got, awsVectorCanonicalRequest)
	}
}

func TestSigV4_SignatureMatchesTheAWSVector(t *testing.T) {
	// The whole chain: canonical request, string to sign, derived key, HMAC.
	// If this passes, mudflat's SigV4 is correct against AWS's own published
	// answer, and a disagreement with curlew is curlew's.
	req := SigV4Request{
		Method: "GET",
		Path:   "/",
		Headers: map[string][]string{
			"Host":       {awsVectorHost},
			"X-Amz-Date": {awsVectorDate},
		},
		SignedHeaders: []string{"host", "x-amz-date"},
	}

	got := SigV4Signature(req, SigV4Credentials{
		AccessKey: awsVectorAccessKey,
		SecretKey: awsVectorSecret,
		Region:    awsVectorRegion,
		Service:   awsVectorService,
		Date:      awsVectorDate,
	})

	if got != awsVectorSignature {
		t.Errorf("signature = %s\nwant       %s", got, awsVectorSignature)
	}
}

func TestSigV4_CanonicalQueryStringIsSortedAndEncoded(t *testing.T) {
	// AWS sorts by encoded parameter name, then by encoded value, and uses
	// %20 for a space rather than +.
	req := SigV4Request{
		Method:        "GET",
		Path:          "/",
		Query:         "b=2&a=1&a=0&c=hello%20world",
		Headers:       map[string][]string{"Host": {"example.com"}},
		SignedHeaders: []string{"host"},
	}

	canonical := CanonicalRequest(req)
	line := strings.Split(canonical, "\n")[2]
	want := "a=0&a=1&b=2&c=hello%20world"
	if line != want {
		t.Errorf("canonical query = %q, want %q", line, want)
	}
}

func TestSigV4_UnreservedCharactersAreNotEncoded(t *testing.T) {
	// RFC 3986 unreserved: ALPHA / DIGIT / "-" / "." / "_" / "~". Encoding any
	// of these produces a signature that will never match.
	got := SigV4Encode("Aa0-._~")
	if got != "Aa0-._~" {
		t.Errorf("SigV4Encode(unreserved) = %q, want it unchanged", got)
	}
	if enc := SigV4Encode("a b"); enc != "a%20b" {
		t.Errorf("space encoded as %q, want %%20 (not +)", enc)
	}
	if enc := SigV4Encode("/"); enc != "%2F" {
		t.Errorf("slash encoded as %q, want %%2F", enc)
	}
}

// --- OAuth 1.0a against RFC 5849 --------------------------------------------

func TestOAuth1_BaseStringFollowsRFC5849(t *testing.T) {
	// Hand-computed from RFC 5849 §3.4.1: the base string is
	// METHOD & pctenc(base URI) & pctenc(normalised parameters), with the
	// parameters sorted by encoded name and joined with unencoded "=" and "&"
	// before the whole string is encoded once more.
	params := map[string]string{
		"oauth_consumer_key":     "key",
		"oauth_nonce":            "n",
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        "1",
		"oauth_token":            "tok",
		"oauth_version":          "1.0",
	}

	got := OAuth1BaseString("post", "http://example.com/request", params)
	want := "POST&http%3A%2F%2Fexample.com%2Frequest&" +
		"oauth_consumer_key%3Dkey%26" +
		"oauth_nonce%3Dn%26" +
		"oauth_signature_method%3DHMAC-SHA1%26" +
		"oauth_timestamp%3D1%26" +
		"oauth_token%3Dtok%26" +
		"oauth_version%3D1.0"

	if got != want {
		t.Errorf("base string mismatch.\n got: %s\nwant: %s", got, want)
	}
}

func TestOAuth1_MethodIsUppercasedAndDefaultPortDropped(t *testing.T) {
	// RFC 5849 §3.4.1.2: the base string URI excludes a default port and
	// lowercases scheme and host.
	got := OAuth1BaseString("get", "HTTP://Example.COM:80/Path", map[string]string{"a": "1"})
	if !strings.HasPrefix(got, "GET&") {
		t.Errorf("method not uppercased: %s", got)
	}
	if !strings.Contains(got, "http%3A%2F%2Fexample.com%2FPath&") {
		t.Errorf("base URI not normalised (default port and case): %s", got)
	}
}

func TestOAuth1_SignatureIsStableAndKeyedByBothSecrets(t *testing.T) {
	params := map[string]string{"oauth_consumer_key": "key", "oauth_nonce": "n"}
	base := OAuth1BaseString("POST", "http://example.com/r", params)

	a := OAuth1Signature(base, "consumer-secret", "token-secret")
	b := OAuth1Signature(base, "consumer-secret", "token-secret")
	if a != b {
		t.Error("signature is not deterministic")
	}
	if c := OAuth1Signature(base, "consumer-secret", "different"); c == a {
		t.Error("token secret does not affect the signature; the signing key is wrong")
	}
	if c := OAuth1Signature(base, "different", "token-secret"); c == a {
		t.Error("consumer secret does not affect the signature")
	}
}

// --- The endpoints ----------------------------------------------------------

func TestVerifyEndpoint_AcceptsACorrectSigV4Signature(t *testing.T) {
	base, client := startServer(t)
	url := base + "/verify/sigv4"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	signRequestForTest(t, req, nil)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out verifyResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.OK {
		t.Errorf("a correctly signed request was rejected at stage %q: %s\ncanonical request:\n%s",
			out.Stage, out.Hint, out.CanonicalRequest)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestVerifyEndpoint_RejectsATamperedSignature(t *testing.T) {
	base, client := startServer(t)

	req, err := http.NewRequest(http.MethodGet, base+"/verify/sigv4", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	signRequestForTest(t, req, nil)

	// Flip the last character of the signature.
	auth := req.Header.Get("Authorization")
	req.Header.Set("Authorization", auth[:len(auth)-1]+flip(auth[len(auth)-1]))

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}

	var out verifyResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.OK {
		t.Error("a tampered signature was accepted")
	}
	if out.Stage != "signature" {
		t.Errorf("stage = %q, want signature", out.Stage)
	}
	// The staged output is the point: a SigV4 mismatch is close to undebuggable
	// without knowing what the server computed at each step.
	if out.CanonicalRequest == "" {
		t.Error("no canonical request returned; the diff is what makes a mismatch actionable")
	}
	if out.StringToSign == "" {
		t.Error("no string to sign returned")
	}
	if len(out.Checks) == 0 {
		t.Error("no staged checks returned")
	}
}

func TestVerifyEndpoint_ReportsAnUnknownAccessKey(t *testing.T) {
	base, client := startServer(t)

	req, _ := http.NewRequest(http.MethodGet, base+"/verify/sigv4", nil)
	req.Header.Set("Authorization",
		"AWS4-HMAC-SHA256 Credential=NOSUCHKEY/20260811/us-east-1/execute-api/aws4_request, "+
			"SignedHeaders=host, Signature=abc")

	resp, _ := client.Do(req)
	defer func() { _ = resp.Body.Close() }()

	var out verifyResult
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Stage != "credentials" {
		t.Errorf("stage = %q, want credentials", out.Stage)
	}
	if !strings.Contains(out.Hint, "NOSUCHKEY") {
		t.Errorf("hint does not name the unknown key: %q", out.Hint)
	}
}

func TestVerifyEndpoint_ReportsAMissingAuthorizationHeader(t *testing.T) {
	base, client := startServer(t)

	resp, body := get(t, client, base+"/verify/sigv4")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Authorization") {
		t.Errorf("error does not mention the missing header: %s", body)
	}
}

func TestVerifyEndpoint_OAuth1RejectsATamperedSignature(t *testing.T) {
	base, client := startServer(t)

	req, _ := http.NewRequest(http.MethodPost, base+"/verify/oauth1", nil)
	req.Header.Set("Authorization",
		`OAuth oauth_consumer_key="mud_consumer_0001", oauth_nonce="abc", `+
			`oauth_signature_method="HMAC-SHA1", oauth_timestamp="1700000000", `+
			`oauth_version="1.0", oauth_signature="ZGVmaW5pdGVseXdyb25n"`)

	resp, _ := client.Do(req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}

	var out verifyResult
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.OK {
		t.Error("a wrong OAuth signature was accepted")
	}
	if out.BaseString == "" {
		t.Error("no base string returned; it is the only way to debug an OAuth mismatch")
	}
}

// signRequestForTest signs req with mudflat's own SigV4 implementation.
//
// Using mudflat to produce the signature it then verifies proves only that the
// endpoint is wired up — the AWS-vector tests above are what prove the algorithm
// is right, and curlew signing it in the dogfood suite is what proves curlew is.
func signRequestForTest(t *testing.T, req *http.Request, body []byte) {
	t.Helper()

	const date = "20260811T120000Z"
	req.Header.Set("X-Amz-Date", date)
	if req.Host == "" {
		req.Host = req.URL.Host
	}

	sigReq := SigV4Request{
		Method:        req.Method,
		Path:          req.URL.Path,
		Query:         req.URL.RawQuery,
		Headers:       map[string][]string{"Host": {req.URL.Host}, "X-Amz-Date": {date}},
		SignedHeaders: []string{"host", "x-amz-date"},
		Body:          body,
	}
	creds := SigV4Credentials{
		AccessKey: MudflatSigV4AccessKey,
		SecretKey: MudflatSigV4SecretKey,
		Region:    "us-east-1",
		Service:   "execute-api",
		Date:      date,
	}
	sig := SigV4Signature(sigReq, creds)

	req.Header.Set("Authorization",
		"AWS4-HMAC-SHA256 Credential="+creds.AccessKey+"/"+date[:8]+"/"+creds.Region+"/"+
			creds.Service+"/aws4_request, SignedHeaders=host;x-amz-date, Signature="+sig)
}

func flip(c byte) string {
	if c == 'a' {
		return "b"
	}
	return "a"
}
