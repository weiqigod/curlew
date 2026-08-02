package signer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/variable"
)

func TestSignerRegistry_RegisterAndLookup(t *testing.T) {
	r := NewBuiltinRegistry()
	factory := func(params map[string]any) (Signer, error) { return nil, nil }
	if err := r.Register("test-noop", factory); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := r.Lookup("test-noop")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got == nil {
		t.Fatal("Lookup returned nil factory")
	}
}

func TestSignerRegistry_Lookup_UnknownType(t *testing.T) {
	r := NewBuiltinRegistry()
	_ = r.Register("alpha", func(map[string]any) (Signer, error) { return nil, nil })
	_ = r.Register("beta", func(map[string]any) (Signer, error) { return nil, nil })

	_, err := r.Lookup("missing")
	if err == nil {
		t.Fatal("expected ErrUnknownSignerType, got nil")
	}
	if !errors.Is(err, ErrUnknownSignerType) {
		t.Errorf("error %v does not wrap ErrUnknownSignerType", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, `"missing"`) {
		t.Errorf("error %q must mention requested type name", msg)
	}
	if !strings.Contains(msg, "alpha, beta") {
		t.Errorf("error %q must list available types sorted: alpha, beta", msg)
	}
}

func TestSignerRegistry_Lookup_EmptyRegistry(t *testing.T) {
	r := NewBuiltinRegistry()
	_, err := r.Lookup("anything")
	if !errors.Is(err, ErrUnknownSignerType) {
		t.Errorf("error %v does not wrap ErrUnknownSignerType", err)
	}
	if !strings.Contains(err.Error(), "no signer types registered") {
		t.Errorf("empty-registry error must say so, got %q", err.Error())
	}
}

func TestSignerRegistry_Register_Duplicate(t *testing.T) {
	r := NewBuiltinRegistry()
	f := func(map[string]any) (Signer, error) { return nil, nil }
	if err := r.Register("dup", f); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register("dup", f); err == nil {
		t.Fatal("second Register must reject duplicate name")
	}
}

func TestSignerRegistry_BuiltinTypesEmpty(t *testing.T) {
	r := NewBuiltinRegistry()
	// Lookup for any name returns ErrUnknownSignerType with the
	// "no signer types registered" branch.
	_, err := r.Lookup("aws-sigv4")
	if !errors.Is(err, ErrUnknownSignerType) {
		t.Errorf("expected ErrUnknownSignerType, got %v", err)
	}
}

func TestSigner_RequestSpec_ContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	if got := RequestSpecFromContext(ctx); got != nil {
		t.Errorf("empty ctx should yield nil spec, got %+v", got)
	}
	spec := &RequestSpec{Type: "noop", Params: map[string]any{"k": "v"}}
	ctx2 := WithRequestSpec(ctx, spec)
	got := RequestSpecFromContext(ctx2)
	if got == nil || got.Type != "noop" || got.Params["k"] != "v" {
		t.Errorf("round-trip failed: got %+v", got)
	}
	if RequestSpecFromContext(ctx) != nil {
		t.Error("derived ctx must not leak into parent")
	}
}

// fakeSigner implements Signer for the integration suite below.
type fakeSigner struct {
	header string
	value  string
}

func (f *fakeSigner) Sign(_ context.Context, req *httpexec.Request, _ *variable.SensitiveSet) error {
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	req.Headers[f.header] = f.value
	return nil
}

func TestSigner_FakeSigner_AddsHeader(t *testing.T) {
	fs := &fakeSigner{header: "X-Signed-By", value: "noop"}
	req := &httpexec.Request{Method: "GET", URL: "http://example.com"}
	if err := fs.Sign(context.Background(), req, variable.NewSensitiveSet()); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if req.Headers["X-Signed-By"] != "noop" {
		t.Errorf("header not injected, got %v", req.Headers)
	}
}
