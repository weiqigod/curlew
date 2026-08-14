package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/weiqigod/curlew/internal/docs"
)

// §3.2's phase table and §11.6's protocol table.
//
// Between them they make one claim over and over — this runs in parallel, that
// one never does — and a claim about concurrency cannot be checked by reading
// the output, because sequential and parallel produce the same lines in the
// same order. So the server counts: every handler holds for a moment and
// records how many requests were in flight at once. A phase that says "always
// sequential" must peak at one even under --parallel, and one that says "fully
// parallel" must peak above one, or the flag is decoration.

// concurrencyServer holds each request briefly and records the peak number in
// flight, per path prefix.
type concurrencyServer struct {
	mu       sync.Mutex
	inFlight map[string]int
	peak     map[string]int
	order    []string
	fail     map[string]bool // paths that answer 500
	hold     time.Duration
}

func newConcurrencyServer(hold time.Duration) *concurrencyServer {
	return &concurrencyServer{
		inFlight: map[string]int{},
		peak:     map[string]int{},
		fail:     map[string]bool{},
		hold:     hold,
	}
}

// group is the first path segment: "setup", "requests", "teardown", "http", …
func group(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	return parts[0]
}

func (c *concurrencyServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g := group(r.URL.Path)

		c.mu.Lock()
		c.inFlight[g]++
		if c.inFlight[g] > c.peak[g] {
			c.peak[g] = c.inFlight[g]
		}
		c.order = append(c.order, r.URL.Path)
		shouldFail := c.fail[r.URL.Path]
		c.mu.Unlock()

		time.Sleep(c.hold)

		c.mu.Lock()
		c.inFlight[g]--
		c.mu.Unlock()

		if shouldFail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":{"a":1}}`))
	})
}

func (c *concurrencyServer) peakFor(g string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.peak[g]
}

func (c *concurrencyServer) seen() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := append([]string{}, c.order...)
	sort.Strings(out)
	return out
}

func (c *concurrencyServer) failPath(p string) {
	c.mu.Lock()
	c.fail[p] = true
	c.mu.Unlock()
}

// runCollection writes and runs a collection, returning combined output and the
// exit code.
func runCollection(t *testing.T, bin, body string, args ...string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}
	cmd := exec.Command(bin, append([]string{"run", path}, args...)...)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	return string(out), code
}

// item renders one request item hitting the given path.
func item(name, base, path string) string {
	return fmt.Sprintf("  - name: %s\n    request:\n      method: GET\n      url: \"%s%s\"\n", name, base, path)
}

// ------------------------------------------------------------- §3.2 phases

func TestDocTables_phaseTableIsWhatHappens(t *testing.T) {
	bin := buildBinary(t)

	hdr, rows, err := docs.Table("CLI_SPECIFICATION.md", "Phase", "Ordering", "On failure")
	if err != nil {
		t.Fatalf("phase table: %v", err)
	}
	phaseCol := docs.Column(hdr, "Phase")
	orderCol := docs.Column(hdr, "Ordering")
	failCol := docs.Column(hdr, "On failure")
	if phaseCol < 0 || orderCol < 0 || failCol < 0 {
		t.Fatalf("phase table lost a column: %v", hdr)
	}

	known := map[string]bool{"setup": true, "requests": true, "teardown": true}
	seen := map[string]bool{}

	for _, row := range rows {
		if len(row) <= failCol {
			continue
		}
		phase := docs.FirstName(row[phaseCol])
		if !known[phase] {
			t.Errorf("§3.2 names phase %q, which is not one this test knows how to run", phase)
			continue
		}
		seen[phase] = true

		alwaysSequential := strings.Contains(strings.ToLower(row[orderCol]), "always sequential")
		wavesUnderParallel := strings.Contains(row[orderCol], "--parallel")
		if !alwaysSequential && !wavesUnderParallel {
			t.Errorf("§3.2's Ordering cell for %s says neither \"always sequential\" nor anything about "+
				"--parallel: %q", phase, row[orderCol])
			continue
		}

		t.Run(phase+"/ordering", func(t *testing.T) {
			srv2 := newConcurrencyServer(120 * time.Millisecond)
			srv := httptest.NewServer(srv2.handler())
			defer srv.Close()

			// Two independent items in the phase under test, plus one main
			// request so every phase has something to sit around.
			var body strings.Builder
			body.WriteString("name: phases\n")
			for _, p := range []string{"setup", "requests", "teardown"} {
				if p == "requests" {
					continue
				}
				if p != phase {
					continue
				}
				fmt.Fprintf(&body, "%s:\n%s%s", p,
					item("A", srv.URL, "/"+p+"/a"), item("B", srv.URL, "/"+p+"/b"))
			}
			body.WriteString("requests:\n")
			if phase == "requests" {
				body.WriteString(item("A", srv.URL, "/requests/a"))
				body.WriteString(item("B", srv.URL, "/requests/b"))
			} else {
				body.WriteString(item("Main", srv.URL, "/requests/main"))
			}

			out, code := runCollection(t, bin, body.String(), "--parallel")
			if code != 0 {
				t.Fatalf("run exited %d:\n%s", code, out)
			}

			// A peak of one proves sequencing only if two items actually ran.
			called := map[string]bool{}
			for _, p := range srv2.seen() {
				called[p] = true
			}
			for _, want := range []string{"/" + phase + "/a", "/" + phase + "/b"} {
				if !called[want] {
					t.Fatalf("the %s phase never called %s, so its peak concurrency says nothing\n%s",
						phase, want, out)
				}
			}

			peak := srv2.peakFor(phase)
			switch {
			case alwaysSequential && peak != 1:
				t.Errorf("§3.2 says the %s phase is %q; under --parallel its peak concurrency was %d",
					phase, row[orderCol], peak)
			case !alwaysSequential && peak < 2:
				t.Errorf("§3.2 says the %s phase runs in %q; under --parallel its peak concurrency was %d",
					phase, row[orderCol], peak)
			}

			// "Sequential by default" is the other half of the requests row,
			// and the half a --parallel-only check would never reach.
			if !alwaysSequential && strings.Contains(strings.ToLower(row[orderCol]), "sequential by default") {
				plain := newConcurrencyServer(120 * time.Millisecond)
				plainSrv := httptest.NewServer(plain.handler())
				defer plainSrv.Close()

				plainBody := "name: phases\nrequests:\n" +
					item("A", plainSrv.URL, "/requests/a") + item("B", plainSrv.URL, "/requests/b")
				if _, plainCode := runCollection(t, bin, plainBody); plainCode != 0 {
					t.Fatalf("the default run exited %d", plainCode)
				}
				if got := plain.peakFor("requests"); got != 1 {
					t.Errorf("§3.2 says the requests phase is %q; with no --parallel its peak "+
						"concurrency was %d", row[orderCol], got)
				}
			}
		})
	}

	for p := range known {
		if !seen[p] {
			t.Errorf("§3.2 has no row for the %s phase", p)
		}
	}
}

// "A failed item marked `required: true` aborts the run. Otherwise the run
// continues and the failure is recorded."
func TestDocTables_setupFailureRowIsWhatHappens(t *testing.T) {
	bin := buildBinary(t)

	hdr, rows, err := docs.Table("CLI_SPECIFICATION.md", "Phase", "Ordering", "On failure")
	if err != nil {
		t.Fatalf("phase table: %v", err)
	}
	cell := ""
	for _, row := range rows {
		if docs.FirstName(row[docs.Column(hdr, "Phase")]) == "setup" {
			cell = row[docs.Column(hdr, "On failure")]
		}
	}
	if !strings.Contains(cell, "required: true") || !strings.Contains(strings.ToLower(cell), "abort") {
		t.Fatalf("§3.2's setup row no longer says a required failure aborts the run: %q", cell)
	}

	for _, tc := range []struct {
		name         string
		required     string
		wantMainCall bool
	}{
		{"required aborts", "    required: true\n", false},
		{"not required continues", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv2 := newConcurrencyServer(0)
			srv := httptest.NewServer(srv2.handler())
			defer srv.Close()
			srv2.failPath("/setup/a")

			body := "name: phases\nsetup:\n" +
				fmt.Sprintf("  - name: A\n%s    request:\n      method: GET\n      url: \"%s/setup/a\"\n"+
					"    assertions:\n      status: 200\n", tc.required, srv.URL) +
				"requests:\n" + item("Main", srv.URL, "/requests/main")

			out, code := runCollection(t, bin, body)

			calledMain := false
			for _, p := range srv2.seen() {
				if p == "/requests/main" {
					calledMain = true
				}
			}
			if calledMain != tc.wantMainCall {
				t.Errorf("§3.2 says %q; with required=%q the requests phase %s run\n%s",
					cell, strings.TrimSpace(tc.required),
					map[bool]string{true: "did", false: "did not"}[calledMain], out)
			}
			if code == 0 {
				t.Errorf("a failed setup item exited 0; the failure was not recorded anywhere that shows\n%s", out)
			}
		})
	}
}

// "Governed by `options.stop_on_failure`" and "Executes even when the requests
// phase failed" — the two remaining halves of the table.
func TestDocTables_requestsObeyStopOnFailureAndTeardownStillRuns(t *testing.T) {
	bin := buildBinary(t)

	for _, tc := range []struct {
		name           string
		stopOnFailure  bool
		wantSecondCall bool
	}{
		{"stop_on_failure: true", true, false},
		{"stop_on_failure: false", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv2 := newConcurrencyServer(0)
			srv := httptest.NewServer(srv2.handler())
			defer srv.Close()
			srv2.failPath("/requests/first")

			body := fmt.Sprintf("name: phases\noptions:\n  stop_on_failure: %t\nrequests:\n", tc.stopOnFailure) +
				fmt.Sprintf("  - name: First\n    request:\n      method: GET\n      url: \"%s/requests/first\"\n"+
					"    assertions:\n      status: 200\n", srv.URL) +
				item("Second", srv.URL, "/requests/second") +
				"teardown:\n" + item("Cleanup", srv.URL, "/teardown/cleanup")

			out, _ := runCollection(t, bin, body)

			var second, teardown bool
			for _, p := range srv2.seen() {
				switch p {
				case "/requests/second":
					second = true
				case "/teardown/cleanup":
					teardown = true
				}
			}
			if second != tc.wantSecondCall {
				t.Errorf("§3.2 says the requests phase is governed by options.stop_on_failure; with "+
					"stop_on_failure: %t the second request %s run\n%s", tc.stopOnFailure,
					map[bool]string{true: "did", false: "did not"}[second], out)
			}
			if !teardown {
				t.Errorf("§3.2 says teardown executes even when the requests phase failed; it did not\n%s", out)
			}
		})
	}
}

// ---------------------------------------------------------- §11.6 protocols

func TestDocTables_protocolParallelismIsWhatHappens(t *testing.T) {
	bin := buildBinary(t)

	hdr, rows, err := docs.Table("CLI_SPECIFICATION.md", "Protocol", "Parallelism")
	if err != nil {
		t.Fatalf("protocol table: %v", err)
	}
	protoCol, claimCol := docs.Column(hdr, "Protocol"), docs.Column(hdr, "Parallelism")
	if protoCol < 0 || claimCol < 0 {
		t.Fatalf("protocol table lost a column: %v", hdr)
	}

	seen := map[string]bool{}
	for _, row := range rows {
		if len(row) <= claimCol {
			continue
		}
		proto := strings.ToLower(docs.FirstName(row[protoCol]))
		claim := row[claimCol]
		seen[proto] = true

		switch proto {
		case "http", "graphql":
			t.Run(proto, func(t *testing.T) {
				if !strings.Contains(strings.ToLower(claim), "fully parallel") {
					t.Fatalf("§11.6's %s row no longer claims full parallelism: %q", proto, claim)
				}
				peak := runTwoRequests(t, bin, proto)
				if peak < 2 {
					t.Errorf("§11.6 says %s is %q; under --parallel the peak concurrency was %d",
						proto, claim, peak)
				}
			})
		case "websocket":
			t.Run(proto, func(t *testing.T) {
				lower := strings.ToLower(claim)
				if !strings.Contains(lower, "parallel") || !strings.Contains(lower, "ordered") {
					t.Fatalf("§11.6's websocket row no longer makes both claims: %q", claim)
				}
				runWebSocketPair(t, bin, claim)
			})
		default:
			t.Errorf("§11.6 names protocol %q, which this test cannot run", proto)
		}
	}
	for _, want := range []string{"http", "graphql", "websocket"} {
		if !seen[want] {
			t.Errorf("§11.6 has no row for %s", want)
		}
	}
}

// runTwoRequests runs two independent items of the given protocol under
// --parallel and returns the peak concurrency the server saw.
func runTwoRequests(t *testing.T, bin, proto string) int {
	t.Helper()

	srv2 := newConcurrencyServer(150 * time.Millisecond)
	srv := httptest.NewServer(srv2.handler())
	defer srv.Close()

	one := func(name, path string) string {
		if proto == "graphql" {
			return fmt.Sprintf("  - name: %s\n    request:\n      method: POST\n      url: \"%s%s\"\n"+
				"      protocol: graphql\n      graphql:\n        query: \"query { a }\"\n", name, srv.URL, path)
		}
		return item(name, srv.URL, path)
	}

	body := "name: protocols\nrequests:\n" + one("A", "/p/a") + one("B", "/p/b")
	out, code := runCollection(t, bin, body, "--parallel")
	if code != 0 {
		t.Fatalf("run exited %d:\n%s", code, out)
	}
	return srv2.peakFor("p")
}

// runWebSocketPair proves both halves of the websocket row at once: two
// connections open together, and the steps inside one arrive in declared order.
func runWebSocketPair(t *testing.T, bin, claim string) {
	t.Helper()

	var (
		mu       sync.Mutex
		open     int
		peakOpen int
		received = map[string][]string{}
	)

	upgrader := gws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		which := strings.TrimPrefix(r.URL.Path, "/ws/")
		mu.Lock()
		open++
		if open > peakOpen {
			peakOpen = open
		}
		mu.Unlock()

		// Hold both connections open long enough that opening them one after
		// the other could not show a peak of two.
		time.Sleep(150 * time.Millisecond)

		for {
			_, data, readErr := conn.ReadMessage()
			if readErr != nil {
				break
			}
			mu.Lock()
			received[which] = append(received[which], string(data))
			mu.Unlock()
			if err := conn.WriteMessage(gws.TextMessage, []byte(`{"ack":true}`)); err != nil {
				break
			}
		}

		mu.Lock()
		open--
		mu.Unlock()
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	steps := func(name, path string) string {
		var b strings.Builder
		fmt.Fprintf(&b, "  - name: %s\n    request:\n      protocol: websocket\n      url: \"%s%s\"\n"+
			"      websocket:\n        steps:\n", name, wsURL, path)
		for _, msg := range []string{"one", "two", "three"} {
			fmt.Fprintf(&b, "          - action: send\n            message_raw: %q\n", msg)
			b.WriteString("          - action: expect\n            timeout_ms: 5000\n" +
				"            message:\n              $.ack:\n                equals: true\n")
		}
		b.WriteString("          - action: close\n")
		return b.String()
	}

	body := "name: ws\nrequests:\n" + steps("A", "/ws/a") + steps("B", "/ws/b")
	out, code := runCollection(t, bin, body, "--parallel")
	if code != 0 {
		t.Fatalf("run exited %d:\n%s", code, out)
	}

	mu.Lock()
	defer mu.Unlock()
	if peakOpen < 2 {
		t.Errorf("§11.6 says %q; the peak number of connections open at once was %d", claim, peakOpen)
	}
	for which, msgs := range received {
		want := []string{"one", "two", "three"}
		if strings.Join(msgs, ",") != strings.Join(want, ",") {
			t.Errorf("§11.6 says steps within one connection are strictly ordered; connection %s "+
				"received %v\n%s", which, msgs, out)
		}
	}
	if len(received) != 2 {
		t.Errorf("%d connections carried messages, want 2\n%s", len(received), out)
	}
}
