//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRunCmd_Events_StdoutPath(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping /dev/stdout test in CI")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	binary := buildBinary(t)
	collection := writeCollection(t, t.TempDir(), "stdout-events.yaml", fmt.Sprintf(`
name: stdout-events
requests:
  - name: ping
    request:
      method: GET
      url: %s
`, server.URL))

	stdout, _, code := runBinary(t, binary, "run", collection, "--events", "/dev/stdout")
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout: %s", code, stdout)
	}

	for _, line := range strings.Split(stdout, "\n") {
		var event map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &event) == nil && event["kind"] == "run.start" {
			return
		}
	}
	t.Errorf("expected run.start NDJSON line in stdout, got:\n%s", stdout)
}
