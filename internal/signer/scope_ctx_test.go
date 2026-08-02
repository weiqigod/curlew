package signer

import (
	"context"
	"testing"

	"github.com/weiqigod/curlew/internal/variable"
)

func TestWithScope_RoundTrip(t *testing.T) {
	scope := variable.NewScope(map[string]string{"k": "v"})
	if err := scope.Resolve(); err != nil {
		t.Fatal(err)
	}
	ctx := WithScope(context.Background(), scope)
	got := ScopeFromContext(ctx)
	if got == nil {
		t.Fatal("ScopeFromContext returned nil")
	}
	if v, err := got.Interpolate("{{k}}"); err != nil || v != "v" {
		t.Errorf("scope round-trip lost data: %q (err: %v)", v, err)
	}
}

func TestScopeFromContext_AbsentReturnsNil(t *testing.T) {
	if got := ScopeFromContext(context.Background()); got != nil {
		t.Errorf("expected nil scope on bare ctx, got %v", got)
	}
}

func TestWithScope_DerivedContextIsolated(t *testing.T) {
	scope := variable.NewScope(map[string]string{})
	ctx1 := WithScope(context.Background(), scope)
	ctx2 := WithScope(ctx1, nil)
	if ScopeFromContext(ctx1) == nil {
		t.Error("parent ctx must retain scope")
	}
	if ScopeFromContext(ctx2) != nil {
		t.Error("derived ctx with nil scope must override parent")
	}
}
