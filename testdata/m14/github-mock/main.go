// github-mock is a minimal HTTP stub that simulates the GitHub API endpoints
// used by the ApiTool backend for GitHub App integration (M14-018).
//
// Routes implemented:
//
//	POST /app/installations/{id}/access_tokens  — returns a synthetic installation token
//	POST /repos/{owner}/{repo}/check-runs       — returns a synthetic check-run id
//	GET  /repos/{owner}/{repo}/check-runs       — returns empty list (idempotency GET)
//	GET  /installation/repositories             — returns a synthetic repo list
//	GET  /healthz                               — liveness probe
//
// Environment:
//
//	PORT    Listen port (default: 8443)
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

var checkRunIDSeq atomic.Int64

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8443"
	}
	log.Printf("github-mock: listening on :%s", port)

	mux := http.NewServeMux()

	// GET /healthz
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ok")
	})

	// POST /app/installations/{id}/access_tokens
	mux.HandleFunc("POST /app/installations/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":      "ghs_mockinstalltoken12345678901234567890",
			"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			"permissions": map[string]string{
				"checks": "write",
			},
		})
	})

	// POST /repos/{owner}/{repo}/check-runs
	// GET  /repos/{owner}/{repo}/check-runs
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodPost:
			id := checkRunIDSeq.Add(1)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":  id,
				"url": fmt.Sprintf("https://api.github.com%s/%d", r.URL.Path, id),
				"name": "ApiTool",
				"status": "completed",
			})
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total_count":  0,
				"check_runs":   []any{},
			})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// GET /installation/repositories
	mux.HandleFunc("GET /installation/repositories", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count":        1,
			"repository_selection": "selected",
			"repositories": []map[string]any{
				{
					"id":        1,
					"full_name": "acme/api",
					"private":   false,
				},
			},
		})
	})

	log.Fatal(http.ListenAndServe(":"+port, mux))
}
