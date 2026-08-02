package httpbody_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/httpbody"
)

func TestLoadBody_Relative(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`{"user":"{{user}}"}`)
	path := filepath.Join(dir, "body.json")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := httpbody.LoadBody(httpbody.LoadBodyInput{BaseDir: dir, BodyFile: "body.json"})
	if err != nil {
		t.Fatalf("LoadBody error: %v", err)
	}
	if !bytes.Equal(res.Body, content) {
		t.Errorf("Body = %q, want %q", res.Body, content)
	}
	if res.FilePath != path {
		t.Errorf("FilePath = %q, want %q", res.FilePath, path)
	}
	if res.ContentType != "application/json" {
		t.Errorf("ContentType = %q, want application/json", res.ContentType)
	}
}

func TestLoadBody_Absolute(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`<root><id>1</id></root>`)
	abs := filepath.Join(dir, "body.xml")
	if err := os.WriteFile(abs, content, 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := httpbody.LoadBody(httpbody.LoadBodyInput{BaseDir: "/ignored", BodyFile: abs})
	if err != nil {
		t.Fatalf("LoadBody error: %v", err)
	}
	if !bytes.Equal(res.Body, content) {
		t.Errorf("Body mismatch for absolute path")
	}
	if res.FilePath != abs {
		t.Errorf("FilePath = %q, want %q", res.FilePath, abs)
	}
	// XML MIME is registered on all platforms we support; accept any variant
	// so we don't couple the test to a specific mime database ordering.
	if !strings.Contains(res.ContentType, "xml") {
		t.Errorf("ContentType = %q, want a variant containing \"xml\"", res.ContentType)
	}
}

func TestLoadBody_Missing(t *testing.T) {
	_, err := httpbody.LoadBody(httpbody.LoadBodyInput{BaseDir: t.TempDir(), BodyFile: "missing.json"})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !errors.Is(err, httpbody.ErrBodyFileNotFound) {
		t.Errorf("err = %v, want ErrBodyFileNotFound", err)
	}
	if !strings.Contains(err.Error(), "missing.json") {
		t.Errorf("error %q should contain the missing file name", err.Error())
	}
}

func TestLoadBody_TooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.bin")
	if err := os.WriteFile(path, make([]byte, 1024), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := httpbody.LoadBody(httpbody.LoadBodyInput{BaseDir: dir, BodyFile: "big.bin", MaxBytes: 100})
	if err == nil {
		t.Fatal("expected error for oversized file, got nil")
	}
	if !errors.Is(err, httpbody.ErrBodyFileTooLarge) {
		t.Errorf("err = %v, want ErrBodyFileTooLarge", err)
	}
}

func TestLoadBody_BinarySafe(t *testing.T) {
	dir := t.TempDir()
	content := []byte{0x00, 0xFF, 0x01, 0xFE, 0x7F, 0x80, 0xDE, 0xAD, 0xBE, 0xEF}
	path := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := httpbody.LoadBody(httpbody.LoadBodyInput{BaseDir: dir, BodyFile: "blob.bin"})
	if err != nil {
		t.Fatalf("LoadBody error: %v", err)
	}
	if !bytes.Equal(res.Body, content) {
		t.Errorf("binary content mangled: got %x want %x", res.Body, content)
	}
}

func TestLoadBody_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := httpbody.LoadBody(httpbody.LoadBodyInput{BaseDir: dir, BodyFile: "empty.txt"})
	if err != nil {
		t.Fatalf("LoadBody error: %v", err)
	}
	if len(res.Body) != 0 {
		t.Errorf("Body len = %d, want 0", len(res.Body))
	}
}

func TestLoadBody_UnknownExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.tmpl")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := httpbody.LoadBody(httpbody.LoadBodyInput{BaseDir: dir, BodyFile: "payload.tmpl"})
	if err != nil {
		t.Fatalf("LoadBody error: %v", err)
	}
	if res.ContentType != "" {
		t.Errorf("ContentType = %q, want empty for unknown extension", res.ContentType)
	}
}

func TestLoadBody_EmptyPath(t *testing.T) {
	_, err := httpbody.LoadBody(httpbody.LoadBodyInput{BaseDir: t.TempDir(), BodyFile: ""})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}
