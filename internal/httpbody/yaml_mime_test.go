package httpbody

import (
	"mime"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A YAML body file must carry a YAML Content-Type.
//
// Detection delegates to the host's MIME database, and most hosts have no entry
// for .yaml at all — application/yaml was only registered in 2024 (RFC 9512),
// long after the system mime.types files most machines ship. So `body_file:
// payload.yaml` went out with no Content-Type header, while the manual's
// extension table promised `application/yaml` or `text/yaml`. A server that
// requires the header rejects the request, and the run's own error says nothing
// about a missing content type.
//
// The gap is filled without taking the mapping away from the host: the
// registration happens only where the host has nothing to say.
func TestLoadBody_yamlExtensionsCarryAYAMLContentType(t *testing.T) {
	dir := t.TempDir()

	for _, ext := range []string{".yaml", ".yml"} {
		path := filepath.Join(dir, "payload"+ext)
		if err := os.WriteFile(path, []byte("a: 1\n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		res, err := LoadBody(LoadBodyInput{BodyFile: path})
		if err != nil {
			t.Fatalf("LoadBody(%s): %v", ext, err)
		}
		if !strings.Contains(res.ContentType, "yaml") {
			t.Errorf("a %s body file detects Content-Type %q; the manual's extension table "+
				"promises a YAML type", ext, res.ContentType)
		}
	}
}

// The host keeps the last word. Registering a type the host already knows would
// make the specification's "the exact mapping is the host's" false for every
// extension, not just the one being filled in.
func TestMIME_registrationOnlyFillsGaps(t *testing.T) {
	// A type the Go standard library's own built-in table always answers.
	const known = ".html"
	if got := mime.TypeByExtension(known); !strings.Contains(got, "text/html") {
		t.Fatalf("%s resolves to %q; this test can no longer tell a filled gap from an override", known, got)
	}
	for _, ext := range yamlExtensions {
		if !strings.HasPrefix(ext, ".") {
			t.Errorf("%q is not an extension", ext)
		}
	}
}
