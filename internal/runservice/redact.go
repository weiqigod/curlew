package runservice

import (
	"github.com/weiqigod/curlew/internal/assertion"
	"github.com/weiqigod/curlew/internal/runner"
	"github.com/weiqigod/curlew/internal/variable"
)

// RedactResults scrubs sensitive values out of everything a formatter will
// print: the request curlew sent, and the response the server sent back.
//
// CLI_SPECIFICATION §6.5 lists eight output surfaces and does not restrict
// redaction to values curlew originated. Dogfooding found the response side
// unguarded — a token echoed by the API, a Set-Cookie, or an assertion's actual
// value all reached the artefacts verbatim (docs/TESTAPI_SPECIFICATION.md
// §11B.2). Formatters read these structs and nothing else, so scrubbing here
// covers every format at once rather than once per formatter.
//
// Called after the run, immediately before formatting: assertions have already
// been evaluated against the real values.
func RedactResults(results []runner.RequestResult, s *variable.SensitiveSet, allow bool) {
	for i := range results {
		// A secret in a query string is the leak vector redaction most often
		// misses: it never appears in a body, only in a URL that every surface
		// records. Only the value is replaced, so the URL stays readable.
		results[i].URL = variable.RedactText(results[i].URL, s, allow)
		results[i].RequestHeaders = variable.RedactHeaders(results[i].RequestHeaders, s, allow)
		results[i].RequestBody = variable.RedactBody(results[i].RequestBody, s, allow)
		if r := results[i].Result; r != nil {
			if body, ok := variable.RedactBody(r.Body, s, allow).([]byte); ok {
				r.Body = body
			}
			r.Headers = variable.RedactHTTPHeaders(r.Headers, s, allow)
		}
		RedactAssertionResults(results[i].AssertionResults, s, allow)
	}
}

// RedactAssertionResults scrubs each assertion's expected and actual strings.
func RedactAssertionResults(res *assertion.Results, s *variable.SensitiveSet, allow bool) {
	if res == nil {
		return
	}
	for i := range res.Items {
		res.Items[i].Expected, res.Items[i].Actual = RedactAssertionText(
			res.Items[i].Type, res.Items[i].Target,
			res.Items[i].Expected, res.Items[i].Actual, s, allow)
	}
}

// RedactAssertionText returns the expected and actual strings of one assertion
// with sensitive values removed.
//
// A failure message exists to print the value that did not match, which makes
// it the single most likely place for a secret to surface. Registered values
// are replaced wherever they appear. On top of that, the actual value of an
// assertion against an inherently sensitive header — Authorization, Cookie,
// Set-Cookie — is replaced whole: the value came from the server, so nothing
// registered it, and matching a substring of a credential does not make the
// rest of it safe to print. Expected is not treated that way; it is the
// author's own string ("matches HttpOnly") and hiding it would leave a failure
// no one can read.
func RedactAssertionText(typ, target, expected, actual string, s *variable.SensitiveSet, allow bool) (string, string) {
	if allow {
		return expected, actual
	}
	expected = variable.RedactText(expected, s, false)
	if typ == assertion.TypeHeader && variable.IsSensitiveHeaderName(target) && actual != "" {
		return expected, variable.Redacted
	}
	return expected, variable.RedactText(actual, s, false)
}
