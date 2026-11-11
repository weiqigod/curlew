package runner

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/assertion"
	"github.com/weiqigod/curlew/internal/docs"
	"github.com/weiqigod/curlew/internal/httpexec"
)

func jsonNoteExecutor(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
	return &httpexec.Result{StatusCode: 200, Body: []byte(`{"note":"x"}`)}, nil
}

func celItems(r RequestResult) []assertion.Result {
	if r.AssertionResults == nil {
		return nil
	}
	var out []assertion.Result
	for _, a := range r.AssertionResults.Items {
		if a.Type == assertion.TypeCEL {
			out = append(out, a)
		}
	}
	return out
}

// TestRun_ParallelRunsCelAndIfInWaves: under --parallel, cel: assertions and
// if: guards that do not use `previous` keep the main phase in waves, and are
// evaluated there just as they are sequentially.
func TestRun_ParallelRunsCelAndIfInWaves(t *testing.T) {
	col := parseCollectionYAML(t, `name: parallel cel
variables:
  flag: "yes"
requests:
  - name: passes
    request: { method: GET, url: "https://example.com/a" }
    assertions:
      cel: ['response.status == 200 && vars.flag == "yes"']
  - name: fails
    request: { method: GET, url: "https://example.com/b" }
    assertions:
      cel: ['response.status == 201']
  - name: gated off
    if: 'vars.flag == "no"'
    request: { method: GET, url: "https://example.com/c" }
  - name: after gated off
    depends_on: [gated off]
    request: { method: GET, url: "https://example.com/d" }
  - name: gated on
    if: 'vars.flag == "yes"'
    request: { method: GET, url: "https://example.com/e" }
  - name: names previous only in text
    request: { method: GET, url: "https://example.com/f" }
    assertions:
      cel: ['response.body.note != "previous"']
`)
	var diag bytes.Buffer
	results, summary, err := Run(context.Background(), col, jsonNoteExecutor, VarSources{Parallel: true, Diagnostics: &diag})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !summary.IsParallel || strings.Contains(diag.String(), "falling back") {
		t.Fatalf("main phase did not run in waves: IsParallel=%v diagnostics=%q", summary.IsParallel, diag.String())
	}
	byName := map[string]RequestResult{}
	for _, r := range results {
		byName[r.Name] = r
		if r.WaveIndex < 0 {
			t.Errorf("%q has no wave (WaveIndex %d)", r.Name, r.WaveIndex)
		}
	}

	if c := celItems(byName["passes"]); len(c) != 1 || !c[0].Passed {
		t.Errorf("passes: cel results %+v, want one passed", c)
	}
	if c := celItems(byName["fails"]); len(c) != 1 || c[0].Passed || !strings.HasPrefix(c[0].Actual, "false") {
		t.Errorf("fails: cel results %+v, want one failed with got false", c)
	}
	if c := celItems(byName["names previous only in text"]); len(c) != 1 || !c[0].Passed {
		t.Errorf("names previous only in text: cel results %+v, want one passed", c)
	}
	if r := byName["gated off"]; !r.Skipped || r.SkipReason != "if: false" {
		t.Errorf("gated off: skipped=%v reason=%q, want skipped by if: false", r.Skipped, r.SkipReason)
	}
	if r := byName["after gated off"]; !r.Skipped || !strings.Contains(r.SkipReason, "gated off") {
		t.Errorf("after gated off: skipped=%v reason=%q, want skipped because its parent was", r.Skipped, r.SkipReason)
	}
	if r := byName["gated on"]; r.Skipped || r.Result == nil {
		t.Errorf("gated on: skipped=%v, want it to run", r.Skipped)
	}
	if summary.Passed != 3 || summary.Failed != 1 || summary.Skipped != 2 {
		t.Errorf("summary passed=%d failed=%d skipped=%d, want 3/1/2", summary.Passed, summary.Failed, summary.Skipped)
	}
}

// TestRun_ParallelOrdersExpressionsAfterTheirVariables: an if: or cel:
// expression that reads vars.<name> extracted by another request waits for
// that request's wave, as a {{name}} reference does.
func TestRun_ParallelOrdersExpressionsAfterTheirVariables(t *testing.T) {
	if _, err := docs.Prose("CLI_SPECIFICATION.md", "fields an `if:` guard or a `cel:` assertion reads"); err != nil {
		t.Fatalf("specified claim: %v", err)
	}
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Body: []byte(`{"token":"t1"}`)}, nil
	}
	col := parseCollectionYAML(t, `name: expression dependencies
requests:
  - name: login
    request: { method: GET, url: "https://example.com/login" }
    extract:
      token: "$.token"
  - name: guarded
    if: 'vars.token == "t1"'
    request: { method: GET, url: "https://example.com/a" }
  - name: asserted
    request: { method: GET, url: "https://example.com/b" }
    assertions:
      cel: ['vars.token == response.body.token']
`)
	results, summary, err := Run(context.Background(), col, exec, VarSources{Parallel: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !summary.IsParallel {
		t.Fatal("main phase did not run in waves")
	}
	for _, r := range results {
		if r.Name == "login" {
			continue
		}
		if r.Skipped || r.WaveIndex < 1 {
			t.Errorf("%q: skipped=%v wave=%d, want it to run in a wave after login", r.Name, r.Skipped, r.WaveIndex)
		}
	}
	if summary.Failed != 0 {
		t.Errorf("failed = %d, want 0", summary.Failed)
	}
}

// TestRun_ParallelFallsBackWhenPreviousIsUsed: `previous` only has a meaning in
// file order, so an if: or cel: expression that reads it runs the main phase
// sequentially, and the diagnostic says why.
func TestRun_ParallelFallsBackWhenPreviousIsUsed(t *testing.T) {
	if _, err := docs.Prose("MANUAL.md", "Curlew runs the requests phase sequentially and says so on stderr"); err != nil {
		t.Fatalf("documented claim: %v", err)
	}
	for name, item := range map[string]string{
		"if":  "    if: 'previous.status == 200'\n",
		"cel": "    assertions:\n      cel: ['previous.status == 200']\n",
	} {
		col := parseCollectionYAML(t, `name: previous
requests:
  - name: first
    request: { method: GET, url: "https://example.com/a" }
  - name: second
    request: { method: GET, url: "https://example.com/b" }
`+item)
		var diag bytes.Buffer
		_, summary, err := Run(context.Background(), col, jsonNoteExecutor, VarSources{Parallel: true, Diagnostics: &diag})
		if err != nil {
			t.Fatalf("%s: Run: %v", name, err)
		}
		if summary.IsParallel {
			t.Errorf("%s: main phase ran in waves, want sequential", name)
		}
		if d := diag.String(); !strings.Contains(d, "previous") || !strings.Contains(d, "falling back to sequential") {
			t.Errorf("%s: diagnostics %q, want a fallback line naming previous", name, d)
		}
		if summary.Failed != 0 {
			t.Errorf("%s: failed = %d, want 0", name, summary.Failed)
		}
	}
}
