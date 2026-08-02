package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestParseUIArgs(t *testing.T) {
	f, help, err := parseUIArgs([]string{"--port", "9000", "--env", "dev", "--no-open"})
	if err != nil || help {
		t.Fatalf("err=%v help=%v", err, help)
	}
	if f.port != 9000 || !f.portSet || f.env != "dev" || !f.noOpen {
		t.Errorf("flags = %+v", f)
	}

	_, help, err = parseUIArgs([]string{"--help"})
	if err != nil || !help {
		t.Errorf("--help: err=%v help=%v", err, help)
	}

	if _, _, err := parseUIArgs([]string{"--allow-sensitive"}); err == nil ||
		!strings.Contains(err.Error(), "the UI always redacts sensitive values") {
		t.Errorf("--allow-sensitive must be rejected with the spec message, got %v", err)
	}

	if _, _, err := parseUIArgs([]string{"--port", "notaport"}); err == nil {
		t.Error("invalid port accepted")
	}
	if _, _, err := parseUIArgs([]string{"--bogus"}); err == nil {
		t.Error("unknown flag accepted")
	}
}

func TestUICmd_NoProject_Exit5(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	var stdout, stderr bytes.Buffer
	code := uiCmdOut([]string{"--no-open"}, &stdout, &stderr)
	if code != 5 {
		t.Fatalf("exit = %d, want 5; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no curlew project found") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestUICmd_UnknownEnv_Exit3(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/curlew.yaml", []byte("project_name: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	var stdout, stderr bytes.Buffer
	code := uiCmdOut([]string{"--env", "missing", "--no-open"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit = %d, want 3; stderr: %s", code, stderr.String())
	}
}

func TestUICmd_UsageError_Exit1(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := uiCmdOut([]string{"--allow-sensitive"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "Usage: curlew ui") {
		t.Errorf("usage synopsis missing from stderr: %q", stderr.String())
	}
}
