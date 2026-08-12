package mudflat

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Redaction bait (§9.O).
//
// CLI_SPECIFICATION §6.5 enumerates where a sensitive value must be replaced
// with [REDACTED]: terminal output, JSON, TAP, JUnit, HTML, Markdown, event
// streams and JSONL logs. Eight surfaces, each a separate code path and each a
// separate opportunity to leak.
//
// These endpoints hand curlew realistic secrets to extract. The assertion that
// they stay out of the output is not here — it is on curlew's own artefacts, so
// it lives in testapi/harness/redaction.sh. What these tests pin is that the
// needles the harness greps for are the values the server actually sends.

func TestLeak_ValuesMatchAppendixB(t *testing.T) {
	base, client := startServer(t)

	tests := []struct {
		path   string
		needle string
	}{
		{"/leak/token", MudflatBearerToken},
		{"/leak/pan", MudflatLeakCardNumber},
		{"/leak/nested", MudflatAPIKey},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			_, body := get(t, client, base+tc.path)
			if !strings.Contains(string(body), tc.needle) {
				t.Errorf("body does not contain the documented value %q; "+
					"harness/redaction.sh greps for it and would pass vacuously\n%s",
					tc.needle, body)
			}
		})
	}
}

func TestLeak_SetCookieCarriesTheSecretValue(t *testing.T) {
	base, client := startServer(t)

	resp, _ := get(t, client, base+"/leak/set-cookie")
	cookie := resp.Header.Get("Set-Cookie")
	if !strings.Contains(cookie, MudflatLeakCookieValue) {
		t.Errorf("Set-Cookie = %q, want it to carry %q", cookie, MudflatLeakCookieValue)
	}
	if !strings.Contains(cookie, "HttpOnly") {
		t.Errorf("Set-Cookie = %q, want realistic attributes", cookie)
	}
}

func TestLeak_NestedSecretIsDeep(t *testing.T) {
	// A secret at the top level is easy. One six levels down tests whether a
	// redactor walks the document or only skims it.
	base, client := startServer(t)

	_, body := get(t, client, base+"/leak/nested")

	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("not JSON: %v", err)
	}

	depth := depthOfValue(doc, MudflatAPIKey)
	if depth < 5 {
		t.Errorf("secret sits at depth %d, want at least 5", depth)
	}
}

func TestLeak_InURLRedirectsWithTheTokenInTheQuery(t *testing.T) {
	// A token in a query string survives into logs, Referer headers and
	// browser history. It is the leak vector redaction most often misses,
	// because the value never appears in a body.
	base, client := startServer(t)

	resp, _ := get(t, client, base+"/leak/in-url")
	if resp.StatusCode != http.StatusFound {
		t.Errorf("status = %d, want 302", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if !strings.Contains(location, MudflatBearerToken) {
		t.Errorf("Location = %q, want the token in the query string", location)
	}
}

func TestLeak_HeaderEchoReturnsWhatWasSent(t *testing.T) {
	base, client := startServer(t)

	req, _ := http.NewRequest(http.MethodGet, base+"/leak/header-echo", nil)
	req.Header.Set("Authorization", "Bearer "+MudflatBearerToken)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got, _ := out["authorization"].(string); !strings.Contains(got, MudflatBearerToken) {
		t.Errorf("echo = %q, want the Authorization header returned verbatim", got)
	}
}

// depthOfValue returns how deep needle sits inside a decoded JSON document.
func depthOfValue(v any, needle string) int {
	switch t := v.(type) {
	case string:
		if strings.Contains(t, needle) {
			return 0
		}
		return -1
	case map[string]any:
		for _, child := range t {
			if d := depthOfValue(child, needle); d >= 0 {
				return d + 1
			}
		}
	case []any:
		for _, child := range t {
			if d := depthOfValue(child, needle); d >= 0 {
				return d + 1
			}
		}
	}
	return -1
}
