package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/parser"
)

// parseCollectionYAML writes body to a temporary collection file and parses it.
func parseCollectionYAML(t *testing.T, body string) *parser.Collection {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	col, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return col
}

// TestRun_DependentOfPassedErrorStatusRuns: a producer that got a 4xx it
// asserted for passed, so what depends on it runs. A 4xx with no assertions,
// and a failed assertion, still count as a failed producer. The rule is the
// same with and without --parallel (§11.5).
func TestRun_DependentOfPassedErrorStatusRuns(t *testing.T) {
	if _, err := docs.Prose("MANUAL.md", "This rule is the same with and without `--parallel`"); err != nil {
		t.Fatalf("documented claim: %v", err)
	}
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		status := 200
		if strings.HasSuffix(req.URL, "/status/404") {
			status = 404
		}
		return &httpexec.Result{StatusCode: status, Body: []byte(`{}`)}, nil
	}
	const body = `name: dependency outcome
requests:
  - name: expected 404
    request: { method: GET, url: "https://example.com/status/404" }
    assertions: { status: 404 }
  - name: after expected 404
    depends_on: [expected 404]
    request: { method: GET, url: "https://example.com/a" }
  - name: unasserted 404
    request: { method: GET, url: "https://example.com/status/404" }
  - name: after unasserted 404
    depends_on: [unasserted 404]
    request: { method: GET, url: "https://example.com/b" }
  - name: failed assertion
    request: { method: GET, url: "https://example.com/c" }
    assertions: { status: 201 }
  - name: after failed assertion
    depends_on: [failed assertion]
    request: { method: GET, url: "https://example.com/d" }
`
	wantSkipped := map[string]bool{
		"after expected 404":     false,
		"after unasserted 404":   true,
		"after failed assertion": true,
	}
	for _, par := range []bool{false, true} {
		results, _, err := Run(context.Background(), parseCollectionYAML(t, body), exec, VarSources{Parallel: par})
		if err != nil {
			t.Fatalf("parallel=%v: Run: %v", par, err)
		}
		for _, r := range results {
			want, ok := wantSkipped[r.Name]
			if !ok {
				continue
			}
			if r.Skipped != want {
				t.Errorf("parallel=%v: %q skipped=%v (reason %q), want skipped=%v", par, r.Name, r.Skipped, r.SkipReason, want)
			}
		}
	}
}
