package uiserver_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const validCollection = `name: Users API
setup:
  - name: Seed
    request: {method: POST, url: "{{base_url}}/seed"}
requests:
  - name: Create user
    request:
      method: POST
      url: "{{base_url}}/users"
    assertions:
      status: 201
teardown:
  - name: Cleanup
    request: {method: DELETE, url: "{{base_url}}/seed"}
`

func TestTree_ValidAndInvalidCollections(t *testing.T) {
	ts, root := newTestServer(t)
	writeFile(t, root, "collections/users.yaml", validCollection)
	writeFile(t, root, "collections/broken.yaml", "name: [oops\n")

	resp := apiGet(t, ts, "/api/v1/tree")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body := decodeJSON[struct {
		Etag        string `json:"etag"`
		Collections []struct {
			Path     string  `json:"path"`
			Name     *string `json:"name"`
			Valid    bool    `json:"valid"`
			Counts   *struct{ Setup, Main, Teardown int }
			Requests []struct {
				Name, Slug, Phase, Method, URL string
				SourceLine                     int `json:"source_line"`
			}
			Issues []struct{ Severity, Message string }
		} `json:"collections"`
	}](t, resp.Body)

	if body.Etag == "" {
		t.Error("etag empty")
	}
	if len(body.Collections) != 2 {
		t.Fatalf("collections = %d, want 2", len(body.Collections))
	}
	// Sorted: broken.yaml first.
	broken, users := body.Collections[0], body.Collections[1]
	if broken.Valid {
		t.Error("broken.yaml should be invalid")
	}
	if len(broken.Issues) == 0 {
		t.Error("broken.yaml has no issues")
	}
	if !users.Valid || users.Name == nil || *users.Name != "Users API" {
		t.Errorf("users.yaml entry wrong: %+v", users)
	}
	if users.Counts == nil || users.Counts.Setup != 1 || users.Counts.Main != 1 || users.Counts.Teardown != 1 {
		t.Errorf("counts = %+v", users.Counts)
	}
	if len(users.Requests) != 3 {
		t.Fatalf("requests = %d, want 3 (all phases)", len(users.Requests))
	}
	main := users.Requests[1]
	if main.Phase != "main" || main.Slug != "create-user" || main.URL != "{{base_url}}/users" {
		t.Errorf("main request = %+v (URL must stay a raw template)", main)
	}
}

func TestEnvironments_RedactsSensitiveValues(t *testing.T) {
	ts, root := newTestServer(t)
	writeFile(t, root, "environments/dev.yaml", "variables:\n  base_url: http://localhost:3000\n  api_token: super-secret\n")

	resp := apiGet(t, ts, "/api/v1/environments")
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	s := string(raw)
	if strings.Contains(s, "super-secret") {
		t.Errorf("sensitive value leaked: %s", s)
	}
	if !strings.Contains(s, "[REDACTED]") {
		t.Errorf("redaction marker missing: %s", s)
	}
	if !strings.Contains(s, "http://localhost:3000") {
		t.Errorf("non-sensitive value missing: %s", s)
	}
	if !strings.Contains(s, `"file":"environments/dev.yaml"`) {
		t.Errorf("env file path missing: %s", s)
	}
}

func TestValidate_SingleFileAndAll(t *testing.T) {
	ts, root := newTestServer(t)
	writeFile(t, root, "collections/good.yaml", validCollection)
	writeFile(t, root, "collections/bad.yaml", "requests: {nope: 1}\n")

	resp := apiGet(t, ts, "/api/v1/validate")
	defer func() { _ = resp.Body.Close() }()
	all := decodeJSON[struct {
		Valid bool `json:"valid"`
		Files []struct {
			File  string `json:"file"`
			Valid bool   `json:"valid"`
		} `json:"files"`
	}](t, resp.Body)
	if all.Valid {
		t.Error("valid = true with a broken collection present")
	}
	if len(all.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(all.Files))
	}

	resp2 := apiGet(t, ts, "/api/v1/validate?path=collections/good.yaml")
	defer func() { _ = resp2.Body.Close() }()
	one := decodeJSON[struct {
		Valid bool `json:"valid"`
	}](t, resp2.Body)
	if !one.Valid {
		t.Error("good.yaml should validate")
	}
}

func TestFiles_JailAndTypeRestrictions(t *testing.T) {
	ts, root := newTestServer(t)
	writeFile(t, root, "collections/c.yaml", validCollection)
	writeFile(t, root, "secrets.txt", "nope")

	resp := apiGet(t, ts, "/api/v1/files?path=collections/c.yaml")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	if string(raw) != validCollection {
		t.Error("file content mismatch")
	}

	for _, path := range []string{"../etc/passwd", "/etc/passwd", "secrets.txt"} {
		resp := apiGet(t, ts, "/api/v1/files?path="+path)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("path %q: status = %d, want 400", path, resp.StatusCode)
		}
	}
}
