package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSmokeConditionalFixtureUsesLoopbackOverride(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "smoke", "fixtures", "if_conditional.yaml"))
	if err != nil {
		t.Fatalf("read conditional fixture: %v", err)
	}
	if bytes.Contains(fixture, []byte("https://httpbin.org")) {
		t.Fatal("conditional smoke fixture contains a public HTTPBin URL")
	}
	if !bytes.Contains(fixture, []byte("{{base_url}}")) {
		t.Fatal("conditional smoke fixture does not use base_url")
	}

	script, err := os.ReadFile(filepath.Join("..", "..", "smoke", "run.sh"))
	if err != nil {
		t.Fatalf("read smoke script: %v", err)
	}
	if !bytes.Contains(script, []byte(`if_conditional.yaml --var base_url="$SMOKE_HTTPBIN_URL"`)) {
		t.Fatal("smoke script does not override the conditional fixture with its loopback server")
	}
}
