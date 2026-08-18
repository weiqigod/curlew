package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// Redaction on the way back.
//
// CLI_SPECIFICATION §6.5 says a sensitive value is replaced with [REDACTED] in
// terminal output, JSON, TAP, JUnit, HTML, Markdown, event streams and JSONL
// logs. It does not restrict that to values curlew sent. Dogfooding against
// mudflat's /leak/* family (docs/TESTAPI_SPECIFICATION.md §11B.2) found the
// same value redacted where curlew sent it —
//
//	> Authorization: [REDACTED]
//
// — and printed verbatim where the server returned it:
//
//	✗ body $.authorization equals: … got Bearer SENTINELVALUE123
//
// An API that echoes a token, a Set-Cookie carrying a session, or a redirect
// with a token in its query then puts the secret straight into a CI log.
//
// The three routes to sensitivity that must survive the round trip:
//
//	declared    a collection variable whose name matches the §6.5 heuristic
//	heuristic   an extracted value whose name matches it
//	explicit    an extracted value declaring `sensitive: true` (§8 object form),
//	            the only route that reaches a name the heuristic cannot
const (
	leakDeclared  = "declared-secret-AAA"
	leakHeuristic = "heuristic-secret-BBB"
	leakExplicit  = "explicit-secret-CCC"
	leakCookie    = "cookie-secret-DDD"
)

// leakServer returns every secret in a place a report will print it: a body to
// extract from, a Set-Cookie header, a URL query, and an echo of the caller's
// own Authorization header.
func leakServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Set-Cookie", "session="+leakCookie+"; Path=/; HttpOnly")
			_, _ = w.Write([]byte(`{"access_token":"` + leakHeuristic +
				`","account_ref":"` + leakExplicit + `"}`))
		default:
			// Echo the credential back, plus the request target so a secret
			// that only ever lived in a URL is visible too.
			_, _ = w.Write([]byte(`{"authorization":"` + r.Header.Get("Authorization") +
				`","target":"` + r.URL.RequestURI() + `"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// leakCollection extracts by both routes, then sends every secret back out and
// asserts wrongly on the echo so the actual value is printed.
func leakCollection(url string) string {
	return `name: redaction round trip
variables:
  my_api_token: "` + leakDeclared + `"
requests:
  - name: collect secrets
    request:
      method: GET
      url: "` + url + `/token"
    extract:
      leaked_token: "$.access_token"
      account_ref:
        path: "$.account_ref"
        sensitive: true
  - name: send them back
    request:
      method: GET
      url: "` + url + `/echo?ref={{account_ref}}"
      headers:
        Authorization: "Bearer {{my_api_token}} {{leaked_token}}"
    assertions:
      status: 200
      body:
        $.authorization: { equals: "forced failure so actual is printed" }
        $.target: { equals: "forced failure so actual is printed" }
`
}

// scanArtefacts fails for every secret found under root.
func scanArtefacts(t *testing.T, label, root string) {
	t.Helper()
	secrets := map[string]string{
		"declared collection variable":     leakDeclared,
		"extracted, sensitive by name":     leakHeuristic,
		"extracted, sensitive by decl":     leakExplicit,
		"Set-Cookie value from the server": leakCookie,
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		raw, readErr := os.ReadFile(path) //nolint:gosec // test-controlled path
		if readErr != nil {
			return readErr
		}
		for why, secret := range secrets {
			if strings.Contains(string(raw), secret) {
				t.Errorf("%s: %s (%s) leaked into %s",
					label, secret, why, filepath.Base(path))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

// TestRedaction_SecretsFromTheServerAreRedactedInEveryFormat is the eight-surface
// sweep of §6.5. Each format is its own code path, and a secret redacted in the
// terminal but present in the HTML report has still leaked.
func TestRedaction_SecretsFromTheServerAreRedactedInEveryFormat(t *testing.T) {
	if _, err := docs.Prose("MANUAL.md", "replaced with `[REDACTED]` wherever it appears"); err != nil {
		t.Fatalf("documented claim: %v", err)
	}

	srv := leakServer(t)
	dir := t.TempDir()
	collPath := writeCollection(t, dir, "c.yaml", leakCollection(srv.URL))

	// Each entry gets its own directory: writing the event stream into every
	// format's output made one leak in the stream look like a leak in all of
	// them, which misattributes the defect.
	formats := []struct {
		label string
		args  func(out string) []string
	}{
		{"terminal", func(string) []string { return []string{"--format", "terminal", "--no-color"} }},
		{"terminal -vv", func(string) []string { return []string{"--format", "terminal", "--no-color", "-vv"} }},
		{"json", func(string) []string { return []string{"--format", "json"} }},
		{"tap", func(string) []string { return []string{"--format", "tap"} }},
		{"junit", func(out string) []string {
			return []string{"--format", "junit", "--report", filepath.Join(out, "report.xml")}
		}},
		{"html", func(out string) []string {
			return []string{"--format", "html", "--report", filepath.Join(out, "report.html")}
		}},
		{"markdown", func(out string) []string {
			return []string{"--format", "markdown", "--report", out}
		}},
		{"jsonl", func(out string) []string {
			return []string{"--format", "terminal", "--no-color", "--log", filepath.Join(out, "run.jsonl")}
		}},
		{"events", func(out string) []string {
			return []string{"--format", "terminal", "--no-color", "--events", filepath.Join(out, "events.ndjson")}
		}},
	}

	for _, f := range formats {
		t.Run(f.label, func(t *testing.T) {
			out := filepath.Join(dir, strings.ReplaceAll(f.label, " ", "_"))
			if err := os.MkdirAll(out, 0o750); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"run", collPath}, f.args(out)...)
			stdout, stderr, code := captureRun(t, args...)
			if code >= 2 {
				t.Fatalf("exit %d — the collection did not run; stderr=%q", code, stderr)
			}
			// stdout is an artefact too.
			if err := os.WriteFile(filepath.Join(out, "stdout.txt"), []byte(stdout+stderr), 0o600); err != nil {
				t.Fatal(err)
			}
			scanArtefacts(t, f.label, out)
		})
	}
}

// TestRedaction_AssertionActualIsRedacted pins the specific surface the
// dogfooding run found: the failure message, whose whole job is to print the
// value that did not match.
func TestRedaction_AssertionActualIsRedacted(t *testing.T) {
	srv := leakServer(t)
	dir := t.TempDir()
	collPath := writeCollection(t, dir, "c.yaml", leakCollection(srv.URL))

	stdout, _, _ := captureRun(t, "run", collPath, "--format", "terminal", "--no-color")

	if !strings.Contains(stdout, "[REDACTED]") {
		t.Errorf("no [REDACTED] in the failure output; got:\n%s", stdout)
	}
	for _, secret := range []string{leakDeclared, leakHeuristic, leakExplicit} {
		if strings.Contains(stdout, secret) {
			t.Errorf("assertion output printed %q verbatim:\n%s", secret, stdout)
		}
	}
}

// TestRedaction_ResponseHeadersAreRedacted covers -v, which prints the response
// headers the server sent.
func TestRedaction_ResponseHeadersAreRedacted(t *testing.T) {
	srv := leakServer(t)
	dir := t.TempDir()
	collPath := writeCollection(t, dir, "c.yaml", leakCollection(srv.URL))

	stdout, _, _ := captureRun(t, "run", collPath, "--format", "terminal", "--no-color", "-v")

	if !strings.Contains(strings.ToLower(stdout), "set-cookie") {
		t.Skip("-v does not print response headers in this build; nothing to assert")
	}
	if strings.Contains(stdout, leakCookie) {
		t.Errorf("Set-Cookie value printed verbatim:\n%s", stdout)
	}
}
