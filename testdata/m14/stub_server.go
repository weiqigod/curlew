//go:build ignore

// stub_server.go is a tiny HTTP stub that implements the apitool device-code
// authorization endpoints for local testing and smoke tests.
//
// Run via: go run testdata/m14/stub_server.go
//
// Environment:
//
//	PORT          Listen port (default: 18080)
//	STUB_BEHAVIOR Comma-separated poll transition script (default: "pending,success").
//	              Supported values: pending, slow_down, expired, success.
package main

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

var pollIndex atomic.Int32

// injectStatus holds a one-shot injected HTTP status for the next
// /api/v1/auth/refresh request. Zero means "use normal behavior".
var injectStatus atomic.Int32

// injectCode holds a one-shot RFC 7807 problem code to accompany injectStatus.
var injectCode atomic.Value

// stubBehavior returns the ordered list of poll responses from $STUB_BEHAVIOR.
// Defaults to ["pending", "success"].
func stubBehavior() []string {
	b := os.Getenv("STUB_BEHAVIOR")
	if b == "" {
		b = "pending,success"
	}
	parts := strings.Split(b, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// loadPrivateKey loads the ES256 P-256 private key from the test fixture.
func loadPrivateKey() (*ecdsa.PrivateKey, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	keyPath := filepath.Join(root, "internal", "license", "keys", "testdata", "private.pem")
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read private.pem: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in private.pem")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return key, nil
}

// mintLicenseJWT mints a self-signed ES256 License JWT from the testdata private key.
// The resulting token has email=smoke@example.com, tier=enterprise, typ=license+jwt.
func mintLicenseJWT(key *ecdsa.PrivateKey) (string, error) {
	header := map[string]string{"alg": "ES256", "kid": "dev-es256-202605-a3f4d2", "typ": "license+jwt"}
	claims := map[string]any{
		"iss":   "apitest-license-server",
		"aud":   "apitest-cli",
		"sub":   "stub-user",
		"email": "smoke@example.com",
		"tier":  "enterprise",
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
		"nbf":   time.Now().Add(-time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
	hJSON, _ := json.Marshal(header)
	cJSON, _ := json.Marshal(claims)
	hB64 := base64.RawURLEncoding.EncodeToString(hJSON)
	cB64 := base64.RawURLEncoding.EncodeToString(cJSON)
	signing := hB64 + "." + cB64
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	// JWS ES256 signature: 64-byte R||S, big-endian, zero-padded to 32 bytes each.
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// loadCombinedJWKS reads and merges the embedded JWKS (primary kid) and the
// extra JWKS (secondary kid) from the test fixtures. The stub serves both keys
// so that the M14-007 smoke test can verify a token signed with either kid via
// the online fetch path.
func loadCombinedJWKS() ([]byte, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")

	type jwksDoc struct {
		Keys []map[string]string `json:"keys"`
	}

	readJWKS := func(path string) (jwksDoc, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return jwksDoc{}, fmt.Errorf("read %s: %w", path, err)
		}
		var doc jwksDoc
		if err := json.Unmarshal(data, &doc); err != nil {
			return jwksDoc{}, fmt.Errorf("parse %s: %w", path, err)
		}
		return doc, nil
	}

	primary, err := readJWKS(filepath.Join(root, "internal", "license", "keys", "jwks.json"))
	if err != nil {
		return nil, err
	}
	extra, err := readJWKS(filepath.Join(root, "internal", "license", "keys", "testdata", "jwks_extra.json"))
	if err != nil {
		return nil, err
	}

	combined := jwksDoc{Keys: append(primary.Keys, extra.Keys...)}
	return json.Marshal(combined)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "18080"
	}
	behaviors := stubBehavior()
	log.Printf("stub-backend: listening on :%s, STUB_BEHAVIOR=%v", port, behaviors)

	key, err := loadPrivateKey()
	if err != nil {
		log.Fatalf("load private key: %v", err)
	}

	licenseJWT, err := mintLicenseJWT(key)
	if err != nil {
		log.Fatalf("mint license JWT: %v", err)
	}

	http.HandleFunc("/api/v1/auth/device/start", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		log.Printf("%s %s body=%s", r.Method, r.URL.Path, string(body))

		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":               "stub-d",
			"user_code":                 "ABCD-EFGH",
			"verification_uri":          "https://app.apitool.dev/device",
			"verification_uri_complete": "https://app.apitool.dev/device?user_code=ABCD-EFGH",
			"expires_in":                900,
			"interval":                  1,
		})
	})

	http.HandleFunc("/api/v1/auth/device/poll", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		log.Printf("%s %s body=%s", r.Method, r.URL.Path, string(body))

		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}

		idx := int(pollIndex.Add(1)) - 1
		behavior := "success"
		if idx < len(behaviors) {
			behavior = behaviors[idx]
		}

		switch behavior {
		case "pending":
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":   "AUTH_DEVICE_AUTHORIZATION_PENDING",
				"status": 400,
				"title":  "Authorization pending",
			})
		case "slow_down":
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":   "AUTH_DEVICE_SLOW_DOWN",
				"status": 400,
				"title":  "Slow down",
			})
		case "expired":
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":   "AUTH_DEVICE_EXPIRED_TOKEN",
				"status": 400,
				"title":  "Device code expired",
			})
		default: // "success"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"license_jwt":   licenseJWT,
				"access_token":  "at_stub_" + fmt.Sprint(time.Now().UnixNano()),
				"refresh_token": "rt_stub_" + fmt.Sprint(time.Now().UnixNano()),
				"device_id":     "dev_stub_001",
			})
		}
	})

	http.HandleFunc("/api/v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		log.Printf("%s %s body=%s", r.Method, r.URL.Path, string(body))

		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}

		// Check for a one-shot injected response.
		if status := int(injectStatus.Swap(0)); status != 0 {
			code, _ := injectCode.Swap("").(string)
			if code != "" {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"type":   "https://api.apitool.dev/errors/" + strings.ToLower(code),
					"title":  "Injected: " + code,
					"status": status,
					"code":   code,
				})
			} else {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "injected error")
			}
			return
		}

		// Normal success: return fresh tokens.
		freshJWT, err := mintLicenseJWT(key)
		if err != nil {
			log.Printf("mint license JWT: %v", err)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"license_jwt":   freshJWT,
			"access_token":  "at_stub_" + fmt.Sprint(time.Now().UnixNano()),
			"refresh_token": "rt_stub_" + fmt.Sprint(time.Now().UnixNano()),
		})
	})

	// GET /api/v1/.well-known/jwks.json — serves combined ES256 JWKS (primary + secondary kid).
	// Refs docs/SPECIFICATION.md:8237 + M14-003 backend endpoint.
	http.HandleFunc("/api/v1/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		jwksData, err := loadCombinedJWKS()
		if err != nil {
			log.Printf("load combined jwks: %v", err)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/jwk-set+json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(jwksData)
	})

	// /__inject — control endpoint: POST /__inject?status=500&code=AUTH_REFRESH_REUSED
	// Sets a one-shot injected response for the next /api/v1/auth/refresh call.
	http.HandleFunc("/__inject", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		status := 500
		if s := r.URL.Query().Get("status"); s != "" {
			fmt.Sscanf(s, "%d", &status) //nolint:errcheck
		}
		code := r.URL.Query().Get("code")
		injectStatus.Store(int32(status))
		injectCode.Store(code)
		log.Printf("/__inject: status=%d code=%q will apply to next /auth/refresh", status, code)
		w.WriteHeader(200)
		_, _ = io.WriteString(w, "ok")
	})

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
