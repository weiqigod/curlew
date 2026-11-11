package httpexec

import (
	"bufio"
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
)

// wireHeaders starts a raw TCP server that answers one request with 200 and
// returns the header lines exactly as they arrived, before Go's server would
// canonicalise them.
func wireHeaders(t *testing.T) (url string, lines <-chan []string) {
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
		if _, err := r.ReadString('\n'); err != nil { // request line
			return
		}
		var got []string
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
			got = append(got, line)
			if name, value, ok := strings.Cut(line, ":"); ok && strings.EqualFold(name, "Content-Length") {
				length, _ = strconv.Atoi(strings.TrimSpace(value))
			}
		}
		_, _ = io.CopyN(io.Discard, r, int64(length))
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
		out <- got
	}()
	return "http://" + ln.Addr().String(), out
}

// headerNames returns the names of the header lines, in order.
func headerNames(lines []string) []string {
	names := make([]string, 0, len(lines))
	for _, l := range lines {
		name, _, _ := strings.Cut(l, ":")
		names = append(names, name)
	}
	return names
}

func countFold(names []string, want string) (n int, spelled string) {
	for _, name := range names {
		if strings.EqualFold(name, want) {
			n++
			spelled = name
		}
	}
	return n, spelled
}

// TestExecute_SendsHeaderNamesAsWritten: header names reach the wire spelled
// as the collection wrote them, and a header Curlew or Go would add itself is
// not sent a second time when the request already set it in another case.
func TestExecute_SendsHeaderNamesAsWritten(t *testing.T) {
	url, lines := wireHeaders(t)
	_, err := Execute(context.Background(), &Request{
		Method: "POST",
		URL:    url + "/echo",
		Headers: map[string]string{
			"x-tenant":     "a",
			"X-API-KEY":    "b",
			"content-type": "application/json; charset=utf-8",
			"user-agent":   "mine",
		},
		Body: map[string]any{"a": 1},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	names := headerNames(<-lines)
	for _, want := range []string{"x-tenant", "X-API-KEY"} {
		if n, spelled := countFold(names, want); n != 1 || spelled != want {
			t.Errorf("%s: sent %d time(s) as %q, want once as %q (all: %v)", want, n, spelled, want, names)
		}
	}
	for _, want := range []string{"content-type", "user-agent"} {
		if n, spelled := countFold(names, want); n != 1 || spelled != want {
			t.Errorf("%s: sent %d time(s) as %q, want once, spelled as the request wrote it (all: %v)", want, n, spelled, names)
		}
	}
}
