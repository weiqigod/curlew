package main

import (
	"bufio"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// rawHeaderServer answers one request with 200 and hands back its header
// lines exactly as they arrived on the wire.
func rawHeaderServer(t *testing.T) (string, <-chan []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	out := make(chan []string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		if _, err := r.ReadString('\n'); err != nil {
			return
		}
		var lines []string
		length := 0
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			lines = append(lines, line)
			if name, value, ok := strings.Cut(line, ":"); ok && strings.EqualFold(name, "Content-Length") {
				length, _ = strconv.Atoi(strings.TrimSpace(value))
			}
		}
		_, _ = io.CopyN(io.Discard, r, int64(length))
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
		out <- lines
	}()
	return "http://" + ln.Addr().String(), out
}

// TestRun_SendsHeaderNamesAsWritten: request header names reach the server
// spelled as the collection wrote them, and a header the environment adds is
// not sent a second time when the request sets it in another case.
func TestRun_SendsHeaderNamesAsWritten(t *testing.T) {
	if _, err := docs.Prose("MANUAL.md", "Curlew sends header names exactly as you write them"); err != nil {
		t.Fatalf("documented claim: %v", err)
	}
	if _, err := docs.Prose("MANUAL.md", "is never sent a second time"); err != nil {
		t.Fatalf("documented claim: %v", err)
	}
	if _, err := docs.Prose("CLI_SPECIFICATION.md", "a signer that must replace a value replaces it under the request's"); err != nil {
		t.Fatalf("specified claim: %v", err)
	}
	url, lines := rawHeaderServer(t)

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "environments"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeCollection(t, filepath.Join(dir, "environments"), "mock.yaml", "headers:\n  X-Mock-Run: from-env\n")
	collPath := writeCollection(t, dir, "c.yaml", `name: header case
requests:
  - name: echo
    request:
      method: POST
      url: "`+url+`/echo"
      headers:
        x-tenant: a
        X-API-KEY: b
        x-mock-run: own
      body:
        zeta: 1
`)

	_, stderr, code := captureRun(t, "run", collPath, "--env", "mock")
	if code != 0 {
		t.Fatalf("exit %d; stderr=%q", code, stderr)
	}
	got := <-lines
	count := func(name string) (n int, spelled []string) {
		for _, l := range got {
			k, v, _ := strings.Cut(l, ":")
			if strings.EqualFold(k, name) {
				n++
				spelled = append(spelled, k+":"+strings.TrimSpace(v))
			}
		}
		return n, spelled
	}
	for _, want := range []string{"x-tenant:a", "X-API-KEY:b", "x-mock-run:own"} {
		name, _, _ := strings.Cut(want, ":")
		if n, spelled := count(name); n != 1 || spelled[0] != want {
			t.Errorf("%s: sent as %v, want once as %s (all: %v)", name, spelled, want, got)
		}
	}
	if n, spelled := count("Content-Type"); n != 1 {
		t.Errorf("Content-Type sent %d times: %v", n, spelled)
	}
}
