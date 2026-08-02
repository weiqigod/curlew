package runner

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/signer"
	"github.com/weiqigod/curlew/internal/variable"
)

// TestRunner_Signing_EndToEnd_Noop exercises the full registry → signing-field
// → runner-step pipeline using a test-only noop signer. Proves that:
//  1. The parser surfaces the signing: field correctly.
//  2. The runner wires the signer between variable templating and exec.
//  3. The signer receives the post-templating URL (not the raw template string).
//  4. The exec function receives the signer-injected header.
func TestRunner_Signing_EndToEnd_Noop(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `name: signing demo
variables:
  base: "https://{{host}}"
  host: example.com
requests:
  - name: signed-call
    request:
      method: GET
      url: "{{base}}/path"
    signing:
      type: noop
      params:
        marker: "X-Signed-By"
`
	p := filepath.Join(dir, "demo.yaml")
	if err := os.WriteFile(p, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}
	col, err := parser.ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if col.Requests.Items[0].Signing == nil {
		t.Fatal("parser did not surface signing field")
	}

	reg := signer.NewBuiltinRegistry()
	if regErr := reg.Register("noop", func(params map[string]any) (signer.Signer, error) {
		marker, _ := params["marker"].(string)
		return &noopFixtureSigner{header: marker, value: "noop"}, nil
	}); regErr != nil {
		t.Fatalf("Register: %v", regErr)
	}

	var seenURL, seenHeader string
	execFn := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		seenURL = req.URL
		seenHeader = req.Headers["X-Signed-By"]
		return &httpexec.Result{StatusCode: 200, Duration: time.Millisecond}, nil
	}
	_, _, runErr := Run(context.Background(), col, execFn, VarSources{Signer: reg})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if seenURL != "https://example.com/path" {
		t.Errorf("URL templating order regression: signer saw %q, want https://example.com/path", seenURL)
	}
	if seenHeader != "noop" {
		t.Errorf("signer-injected header missing in exec; got %q, want noop", seenHeader)
	}
}

// noopFixtureSigner is a test-only signer that injects a single header.
type noopFixtureSigner struct {
	header string
	value  string
}

func (s *noopFixtureSigner) Sign(_ context.Context, req *httpexec.Request, _ *variable.SensitiveSet) error {
	if s.header == "" {
		return nil
	}
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	req.Headers[s.header] = s.value
	return nil
}

// TestRunner_Signing_PassesScopeToSigner verifies that the runner attaches
// the per-request *variable.Scope to the context before invoking the signer
// exec-wrap, so that signers that call signer.ScopeFromContext can access
// interpolation-ready variables.
func TestRunner_Signing_PassesScopeToSigner(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `name: scope-propagation demo
variables:
  host: example.com
requests:
  - name: scoped-call
    request:
      method: GET
      url: "https://{{host}}/path"
    signing:
      type: scope-capture
      params: {}
`
	p := filepath.Join(dir, "demo.yaml")
	if err := os.WriteFile(p, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}
	col, err := parser.ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}

	var scopeSeen atomic.Pointer[variable.Scope]

	reg := signer.NewBuiltinRegistry()
	if regErr := reg.Register("scope-capture", func(_ map[string]any) (signer.Signer, error) {
		return &scopeCaptureSigner{dst: &scopeSeen}, nil
	}); regErr != nil {
		t.Fatalf("Register: %v", regErr)
	}

	execFn := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Duration: time.Millisecond}, nil
	}
	if _, _, runErr := Run(context.Background(), col, execFn, VarSources{Signer: reg}); runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	got := scopeSeen.Load()
	if got == nil {
		t.Fatal("signer did not receive a non-nil scope via context")
	}
	v, interpErr := got.Interpolate("{{host}}")
	if interpErr != nil {
		t.Fatalf("Interpolate: %v", interpErr)
	}
	if v != "example.com" {
		t.Errorf("scope.Interpolate({{host}}) = %q; want example.com", v)
	}
}

// scopeCaptureSigner captures the *variable.Scope from the context for assertion.
type scopeCaptureSigner struct {
	dst *atomic.Pointer[variable.Scope]
}

func (s *scopeCaptureSigner) Sign(ctx context.Context, req *httpexec.Request, _ *variable.SensitiveSet) error {
	if scope := signer.ScopeFromContext(ctx); scope != nil {
		s.dst.Store(scope)
	}
	return nil
}

// TestRunner_Signing_HeadersPropagate_NilOriginal verifies that signed headers
// injected by the signer are visible in RequestResult.RequestHeaders even when
// the original request has no headers defined (req.Headers == nil in the YAML).
// This covers the nil-map aliasing issue: ToHTTPRequest creates an httpexec.Request
// with Headers: nil; the signer allocates a new map on that copy; without the
// sync-back, the parser.Request.Headers remains nil and JSON output omits
// request_headers (causing the smoke test to produce null for Authorization).
func TestRunner_Signing_HeadersPropagate_NilOriginal(t *testing.T) {
	dir := t.TempDir()
	// Deliberately no headers: field in YAML so parser.Request.Headers is nil.
	yamlContent := `name: nil-headers signing test
requests:
  - name: signed-no-headers
    request:
      method: GET
      url: "https://example.com/"
    signing:
      type: inject-auth
      params:
        header: X-Test-Auth
        value: test-token-123
`
	p := filepath.Join(dir, "demo.yaml")
	if err := os.WriteFile(p, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}
	col, err := parser.ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}

	reg := signer.NewBuiltinRegistry()
	if regErr := reg.Register("inject-auth", func(params map[string]any) (signer.Signer, error) {
		header, _ := params["header"].(string)
		value, _ := params["value"].(string)
		return &noopFixtureSigner{header: header, value: value}, nil
	}); regErr != nil {
		t.Fatalf("Register: %v", regErr)
	}

	execFn := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Duration: time.Millisecond}, nil
	}
	results, _, runErr := Run(context.Background(), col, execFn, VarSources{Signer: reg})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	rr := results[0]
	if rr.RequestHeaders == nil {
		t.Fatal("RequestHeaders is nil — signed headers were not propagated back to parser.Request")
	}
	if got := rr.RequestHeaders["X-Test-Auth"]; got != "test-token-123" {
		t.Errorf("RequestHeaders[X-Test-Auth] = %q; want test-token-123", got)
	}
}

// TestRunner_NewWithBuiltins_PicksUpInitRegistered verifies that when Signer is
// nil in VarSources, the runner uses NewWithBuiltins() which includes any
// factory registered at init time via signer.Register.
func TestRunner_NewWithBuiltins_PicksUpInitRegistered(t *testing.T) {
	t.Cleanup(func() {
		// Clean up test-registered factory to avoid polluting other tests.
		signer.Unregister("builtin-test")
	})

	dir := t.TempDir()
	yamlContent := `name: builtins demo
requests:
  - name: builtin-call
    request:
      method: GET
      url: "https://example.com/"
    signing:
      type: builtin-test
      params: {}
`
	p := filepath.Join(dir, "demo.yaml")
	if err := os.WriteFile(p, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}
	col, err := parser.ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}

	// Register a factory via the package-level Register (init-time mechanism).
	if regErr := signer.Register("builtin-test", func(_ map[string]any) (signer.Signer, error) {
		return &noopFixtureSigner{}, nil
	}); regErr != nil {
		t.Fatalf("signer.Register: %v", regErr)
	}

	execFn := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Duration: time.Millisecond}, nil
	}
	// Pass Signer: nil so the runner falls back to NewWithBuiltins().
	results, _, runErr := Run(context.Background(), col, execFn, VarSources{Signer: nil})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	// Verify the signed request succeeded (not an ErrUnknownSignerType failure).
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Err != nil {
		t.Fatalf("builtin-test signer not found via NewWithBuiltins: %v", results[0].Err)
	}
}
