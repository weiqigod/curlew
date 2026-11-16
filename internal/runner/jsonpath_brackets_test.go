package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/httpexec"
)

// TestRun_BracketNameSelectorInAssertionAndExtract: a body assertion and an
// extraction can address a key with a hyphen through a quoted name selector.
func TestRun_BracketNameSelectorInAssertionAndExtract(t *testing.T) {
	var secondURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		if strings.HasSuffix(req.URL, "/second") || strings.Contains(req.URL, "/second?") {
			secondURL = req.URL
		}
		return &httpexec.Result{StatusCode: 200, Body: []byte(`{"headers":{"X-Mock-Run":"abc"}}`)}, nil
	}
	col := parseCollectionYAML(t, `name: brackets
requests:
  - name: first
    request: { method: GET, url: "https://example.com/first" }
    assertions:
      body:
        "$.headers['X-Mock-Run']": { equals: abc }
    extract:
      run: "$.headers[\"X-Mock-Run\"]"
  - name: second
    request: { method: GET, url: "https://example.com/second?run={{run}}" }
`)
	results, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Failed != 0 {
		for _, r := range results {
			if r.AssertionResults != nil {
				for _, a := range r.AssertionResults.Items {
					t.Logf("%s: %s expected %s, got %s", r.Name, a.Target, a.Expected, a.Actual)
				}
			}
			if r.Err != nil {
				t.Logf("%s: %v", r.Name, r.Err)
			}
		}
		t.Fatalf("failed = %d, want 0", summary.Failed)
	}
	if secondURL != "https://example.com/second?run=abc" {
		t.Errorf("second URL = %q, want the extracted value abc", secondURL)
	}
}
