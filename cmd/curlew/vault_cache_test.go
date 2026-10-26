package main

import (
	"strings"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/docs"
	"github.com/weiqigod/curlew/internal/vault"
	"github.com/zalando/go-keyring"
)

func TestVaultCmd_clearCache(t *testing.T) {
	if _, err := docs.Prose("CLI_SPECIFICATION.md", "`curlew vault clear-cache` removes every cached value"); err != nil {
		t.Fatal(err)
	}
	keyring.MockInit()
	store := &vault.KeyringStore{}
	store.Save("cached-entry", "value", time.Hour)

	helpOut, _, helpExit := captureRun(t, "vault", "clear-cache", "--help")
	if helpExit != 0 || !strings.Contains(helpOut, "vault clear-cache") {
		t.Fatalf("clear-cache --help: exit %d, stdout %q", helpExit, helpOut)
	}
	if _, ok := store.Load("cached-entry"); !ok {
		t.Fatal("clear-cache --help must not clear the cache")
	}

	stdout, stderr, exitCode := captureRun(t, "vault", "clear-cache")
	if exitCode != 0 {
		t.Fatalf("exit code = %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "Cleared") {
		t.Errorf("stdout %q does not confirm the clear", stdout)
	}
	if _, ok := store.Load("cached-entry"); ok {
		t.Error("cached secret survived vault clear-cache")
	}

	help, _, _ := captureRun(t, "vault")
	if !strings.Contains(help, "vault clear-cache") {
		t.Errorf("vault help does not list clear-cache: %q", help)
	}
}
