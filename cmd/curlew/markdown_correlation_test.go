package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The markdown sentinel's request_id survives every retention policy.
//
// §4.1a promises three correlation IDs linking each markdown file back to its
// events-stream and JSONL counterparts, and the manual's cross-format section
// says `request_id` and `request_slug` "link a specific event line to a
// specific markdown file … so an agent can fan out from any fragment back to
// the whole run".
//
// Under store_results: summary or failed_only the sentinel came out as
// `id=-iter-0`. The events stream still emitted req-1 and req-2, so the break
// was in the worst direction available: the event names a file, and the file
// cannot say which event it belongs to.

var sentinelID = regexp.MustCompile(`BEGIN curlew:response id=(\S*) slug=(\S*) run=(\S+)`)

func TestMarkdown_correlationIDsSurviveEveryRetentionPolicy(t *testing.T) {
	bin := buildBinary(t)

	for _, policy := range []string{"all", "summary", "failed_only"} {
		t.Run(policy, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer srv.Close()

			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "rows.csv"), []byte("n\n1\n2\n"), 0o600); err != nil {
				t.Fatalf("write data: %v", err)
			}
			collection := fmt.Sprintf(
				"name: correlation\nrequests:\n  - name: Each\n"+
					"    data_driven:\n      source: \"./rows.csv\"\n      store_results: %s\n"+
					"    request:\n      method: GET\n      url: \"%s/r/{{n}}\"\n"+
					"    assertions:\n      status: 200\n", policy, srv.URL)
			path := filepath.Join(dir, "c.yaml")
			if err := os.WriteFile(path, []byte(collection), 0o600); err != nil {
				t.Fatalf("write collection: %v", err)
			}

			report := filepath.Join(dir, "report")
			events := filepath.Join(dir, "events.ndjson")
			cmd := exec.Command(bin, "run", path, "--format", "markdown", "--report", report,
				"--events", events)
			cmd.Env = append(os.Environ(), "NO_COLOR=1")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("run: %v\n%s", err, out)
			}

			eventIDs := requestIDsFromEvents(t, events)
			if len(eventIDs) == 0 {
				t.Fatal("the events stream carried no request ids; there is no correlation to check")
			}

			for _, name := range []string{"iter-0.md", "iter-1.md"} {
				data, err := os.ReadFile(filepath.Join(report, "each", name))
				if err != nil {
					t.Fatalf("reading %s: %v", name, err)
				}
				m := sentinelID.FindStringSubmatch(string(data))
				if m == nil {
					t.Fatalf("%s has no sentinel:\n%s", name, data)
				}
				id, slug, runID := m[1], m[2], m[3]

				if id == "" || strings.HasPrefix(id, "-") {
					t.Errorf("store_results: %s left %s with sentinel id %q; the events stream for the "+
						"same run carries %v, and §4.1a says the two correlate",
						policy, name, id, eventIDs)
				}
				if slug == "" {
					t.Errorf("store_results: %s left %s with an empty sentinel slug", policy, name)
				}
				if runID == "" {
					t.Errorf("store_results: %s left %s with an empty run id", policy, name)
				}
			}
		})
	}
}

// requestIDsFromEvents reads the request ids the events stream emitted, which
// are the values a markdown sentinel is supposed to be joinable against.
func requestIDsFromEvents(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading events: %v", err)
	}
	idField := regexp.MustCompile(`"request_id":"([^"]+)"`)
	seen := map[string]bool{}
	var out []string
	for _, m := range idField.FindAllStringSubmatch(string(data), -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}
