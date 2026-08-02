package templates_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/peterlindqvist/apitest/internal/websocket/templates"
)

func TestLoadTemplate_Relative(t *testing.T) {
	dir := t.TempDir()
	content := `{"channel":"{{channel}}","action":"subscribe"}`
	path := filepath.Join(dir, "msg.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	body, abs, err := templates.LoadTemplate(dir, "msg.json")
	if err != nil {
		t.Fatalf("LoadTemplate error: %v", err)
	}
	if body != content {
		t.Errorf("body = %q, want %q", body, content)
	}
	if abs != path {
		t.Errorf("abs = %q, want %q", abs, path)
	}
}

func TestLoadTemplate_Absolute(t *testing.T) {
	dir := t.TempDir()
	content := `{"hello":"world"}`
	absPath := filepath.Join(dir, "template.json")
	if err := os.WriteFile(absPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	body, abs, err := templates.LoadTemplate("/any/base/dir", absPath)
	if err != nil {
		t.Fatalf("LoadTemplate error: %v", err)
	}
	if body != content {
		t.Errorf("body = %q, want %q", body, content)
	}
	if abs != absPath {
		t.Errorf("abs = %q, want %q", abs, absPath)
	}
}

func TestLoadTemplate_Missing(t *testing.T) {
	_, _, err := templates.LoadTemplate("/nonexistent", "missing.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !errors.Is(err, templates.ErrTemplateNotFound) {
		t.Errorf("err = %v, want ErrTemplateNotFound", err)
	}
}

func TestLoadTemplate_PreservesPlaceholders(t *testing.T) {
	dir := t.TempDir()
	content := `{"channel":"{{channel}}","user":"{{user}}"}`
	path := filepath.Join(dir, "tpl.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	body, _, err := templates.LoadTemplate(dir, "tpl.json")
	if err != nil {
		t.Fatalf("LoadTemplate error: %v", err)
	}
	if body != content {
		t.Errorf("placeholders not preserved: body = %q", body)
	}
}
