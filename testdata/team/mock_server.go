//go:build ignore

// mock_server.go is a tiny HTTP stub that records calls to the backend API endpoints.
// Run via: go run testdata/team/mock_server.go
// Listens on :18080 and logs each request to stdout.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		log.Printf("%s %s body=%s", r.Method, r.URL.Path, string(body))

		if r.Method == "POST" && strings.Contains(r.URL.Path, "/results") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(202)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"result_id": "res_mock123",
				"status":    "accepted",
			})
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/pr-checks") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		w.WriteHeader(404)
		fmt.Fprintln(w, "not found")
	})
	log.Println("mock-backend listening on :18080")
	log.Fatal(http.ListenAndServe(":18080", nil))
}
