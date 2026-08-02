package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/peterlindqvist/apitest/internal/backend"
	"github.com/peterlindqvist/apitest/internal/backend/device"
)

func TestBackendProbe_SelfTest_ProducesDocumentedStdout(t *testing.T) {
	t.Setenv("APITEST_INTERNAL", "1")
	var stdout, stderr bytes.Buffer
	exit := runWithWriters([]string{"internal", "backend-probe", "--base", "http://127.0.0.1:0", "--self-test"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", exit, stderr.String(), stdout.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"OK:", "client built", "lock OK", "keychain available=", "rfc7807 mapping OK",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q\n%s", want, out)
		}
	}
}

func TestBackendProbe_HiddenWithoutEnvVar(t *testing.T) {
	t.Setenv("APITEST_INTERNAL", "")
	var stdout, stderr bytes.Buffer
	exit := runWithWriters([]string{"internal", "backend-probe", "--self-test"}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("expected exit 1 without env var, got %d", exit)
	}
	if !strings.Contains(stderr.String(), "Unknown command") {
		t.Fatalf("expected 'Unknown command' in stderr=%s", stderr.String())
	}
}

func TestPrintHelp_DoesNotMentionInternal(t *testing.T) {
	var b bytes.Buffer
	printHelpTo(&b)
	if strings.Contains(b.String(), "internal") {
		t.Fatalf("help text exposes hidden command:\n%s", b.String())
	}
}

func TestSeedLogin_WritesIsolatedFileState(t *testing.T) {
	t.Setenv("APITEST_INTERNAL", "1")
	t.Setenv("APITEST_CONFIG_DIR", t.TempDir())
	t.Setenv("APITEST_INTERNAL_DEVICE_ID", "dev-e2e")
	t.Setenv("APITEST_INTERNAL_REFRESH_TOKEN", "rt-e2e")
	t.Setenv("APITEST_INTERNAL_ACCESS_TOKEN", "at-e2e")

	var stdout, stderr bytes.Buffer
	exit := runWithWriters([]string{"internal", "seed-login"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}

	cfgDir := os.Getenv("APITEST_CONFIG_DIR")
	dev, err := device.Read(cfgDir)
	if err != nil || dev.DeviceID != "dev-e2e" {
		t.Fatalf("device = %+v, err=%v", dev, err)
	}
	storage, err := backend.NewStorage(backend.StorageOptions{
		ConfigDir: cfgDir,
		DeviceID:  dev.DeviceID,
		ForceFile: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	refresh, err := storage.GetRefreshToken()
	if err != nil || refresh != "rt-e2e" {
		t.Fatalf("refresh token = %q, err=%v", refresh, err)
	}
	access, err := storage.GetAccessToken()
	if err != nil || access != "at-e2e" {
		t.Fatalf("access token = %q, err=%v", access, err)
	}
}

func TestBackendProbe_BinaryEndToEnd(t *testing.T) {
	binary := buildBinary(t)
	env := append(os.Environ(), "APITEST_INTERNAL=1")
	stdout, stderr, exit := runBinaryWithEnv(t, binary, env,
		"internal", "backend-probe", "--base", "http://127.0.0.1:0", "--self-test")
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	if !strings.Contains(stdout, "OK:") {
		t.Fatalf("stdout missing OK: %s", stdout)
	}
	if !strings.Contains(stdout, "keychain available=") {
		t.Fatalf("stdout missing keychain available=: %s", stdout)
	}
}
