package mudflat

import (
	"fmt"
	"net/http"
)

// Redaction bait (§9.O).
//
// CLI_SPECIFICATION §6.5 says a sensitive value is replaced with [REDACTED] in
// terminal output, JSON, TAP, JUnit, HTML, Markdown, event streams and JSONL
// logs. Eight surfaces, each its own code path, each its own chance to leak —
// and a secret that is redacted in the terminal but present in the HTML report
// has still leaked.
//
// These endpoints hand curlew realistic secrets to extract. The assertion that
// they stay out of the output is on curlew's own artefacts, so it lives in
// testapi/harness/redaction.sh rather than in a collection.
//
// Every value here is published in Appendix B and non-secret by design. They
// exist to be leaked in a test, which is also why mudflat refuses to bind a
// non-loopback interface without --bind-unsafe (§14.5).
const (
	MudflatBearerToken     = "mud_tok_7f3a9c2e5b1d4680"
	MudflatAPIKey          = "mud_key_a1b2c3d4e5f60718"
	MudflatLeakCardNumber  = "4242424242424242"
	MudflatLeakCookieValue = "mud_sess_c9e4f1a7b2d80356"
)

func (s *Server) registerLeak() {
	s.register(Endpoint{
		Pattern:   "/leak/token",
		Methods:   []string{http.MethodGet, http.MethodPost},
		Family:    "O",
		Summary:   "Returns a realistic bearer token and refresh token.",
		Exercises: "Redaction of an extracted value across all eight output surfaces. The name heuristic (§6.5) should mark access_token sensitive without an explicit declaration.",
		Handler:   s.handleLeakToken,
	})

	s.register(Endpoint{
		Pattern:   "/leak/set-cookie",
		Methods:   []string{http.MethodGet},
		Family:    "O",
		Summary:   "Sets a session cookie with a secret value and realistic attributes.",
		Exercises: "Whether a secret in a response header is redacted. Header output is a separate path from body output, and -v prints it.",
		Handler:   s.handleLeakSetCookie,
	})

	s.register(Endpoint{
		Pattern:   "/leak/pan",
		Methods:   []string{http.MethodGet},
		Family:    "O",
		Summary:   "Returns a Luhn-valid test card number.",
		Exercises: "Redaction of a value that matches no name heuristic. $faker.creditCard values are redacted unconditionally per §6.7; a card number arriving in a response is the harder case.",
		Handler:   s.handleLeakPAN,
	})

	s.register(Endpoint{
		Pattern:   "/leak/in-url",
		Methods:   []string{http.MethodGet},
		Family:    "O",
		Summary:   "Redirects to a URL carrying a token in the query string.",
		Exercises: "The leak vector redaction most often misses: the secret never appears in a body, only in a URL that ends up in logs, reports and Referer headers.",
		Handler:   s.handleLeakInURL,
	})

	s.register(Endpoint{
		Pattern:   "/leak/nested",
		Methods:   []string{http.MethodGet},
		Family:    "O",
		Summary:   "Buries an API key six levels deep in a JSON document.",
		Exercises: "Whether redaction walks a document or only skims its top level.",
		Handler:   s.handleLeakNested,
	})

	s.register(Endpoint{
		Pattern:   "/leak/header-echo",
		Methods:   []string{http.MethodGet, http.MethodPost},
		Family:    "O",
		Summary:   "Echoes the request's Authorization header back in the body.",
		Exercises: "A credential the client itself sent, returned where it will be printed. Redaction must cover values on the way back as well as on the way out.",
		Handler:   s.handleLeakHeaderEcho,
	})
}

func (s *Server) handleLeakToken(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  MudflatBearerToken,
		"refresh_token": "mud_refresh_3b8c1f9e7a2d5460",
		"token_type":    "Bearer",
		"expires_in":    3600,
		"note":          "published test values (Appendix B) — they exist to be leaked in a test",
	})
}

func (s *Server) handleLeakSetCookie(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Set-Cookie",
		fmt.Sprintf("session=%s; Path=/; Max-Age=3600; HttpOnly; Secure; SameSite=Strict",
			MudflatLeakCookieValue))
	writeJSON(w, http.StatusOK, map[string]any{
		"note": "the secret is in the Set-Cookie header, not the body",
	})
}

func (s *Server) handleLeakPAN(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"card": map[string]any{
			"number":      MudflatLeakCardNumber,
			"exp_month":   12,
			"exp_year":    2030,
			"cvv":         "123",
			"cardholder":  "TEST CARD",
			"brand":       "visa",
			"description": "the canonical test PAN; Luhn-valid and issued by nobody",
		},
	})
}

func (s *Server) handleLeakInURL(w http.ResponseWriter, _ *http.Request) {
	// An absolute path, not one relative to this endpoint. curlew follows
	// redirects and cannot be told not to (§11.2), so the client lands on /echo
	// and the token arrives as a query parameter — which is the point: it is
	// then in the request URL that every output surface records.
	target := fmt.Sprintf("/echo?access_token=%s&api_key=%s",
		MudflatBearerToken, MudflatAPIKey)
	w.Header().Set("Location", target)
	writeJSON(w, http.StatusFound, map[string]any{
		"location": target,
		"note":     "the secret is only in the URL — no body carries it",
	})
}

func (s *Server) handleLeakNested(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"account": map[string]any{
			"profile": map[string]any{
				"integrations": map[string]any{
					"provider": map[string]any{
						"credentials": map[string]any{
							"api_key": MudflatAPIKey,
							"label":   "buried six levels down",
						},
					},
				},
			},
		},
	})
}

func (s *Server) handleLeakHeaderEcho(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"authorization": r.Header.Get("Authorization"),
		"x_api_key":     r.Header.Get("X-Api-Key"),
		"note":          "whatever credential you sent, returned where it will be printed",
	})
}
