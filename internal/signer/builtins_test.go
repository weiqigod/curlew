package signer

import (
	"testing"
)

// noopFactory is a test-only factory used by builtins tests.
var noopFactory Factory = func(params map[string]any) (Signer, error) { return nil, nil }

// resetBuiltins clears the package-level builtinFactories map between tests.
// Must only be called from t.Cleanup.
func resetBuiltins() {
	builtinMu.Lock()
	defer builtinMu.Unlock()
	builtinFactories = make(map[string]Factory)
}

func TestRegister_AddsToBuiltins(t *testing.T) {
	t.Cleanup(resetBuiltins)
	if err := Register("test-only", noopFactory); err != nil {
		t.Fatalf("Register: %v", err)
	}
	r := NewWithBuiltins()
	if _, err := r.Lookup("test-only"); err != nil {
		t.Errorf("expected test-only to be registered in NewWithBuiltins, got %v", err)
	}
}

func TestRegister_RejectsDuplicates(t *testing.T) {
	t.Cleanup(resetBuiltins)
	if err := Register("dup", noopFactory); err != nil {
		t.Fatal(err)
	}
	if err := Register("dup", noopFactory); err == nil {
		t.Fatal("second Register must reject duplicate name")
	}
}

func TestMustRegister_PanicsOnDuplicate(t *testing.T) {
	t.Cleanup(resetBuiltins)
	MustRegister("once", noopFactory)
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustRegister must panic on duplicate")
		}
	}()
	MustRegister("once", noopFactory)
}

func TestNewWithBuiltins_IsolatedRegistry(t *testing.T) {
	t.Cleanup(resetBuiltins)
	_ = Register("alpha", noopFactory)
	r1 := NewWithBuiltins()
	r2 := NewWithBuiltins()
	// Mutating r1 must not leak to r2.
	_ = r1.Register("beta", noopFactory)
	if _, err := r2.Lookup("beta"); err == nil {
		t.Error("NewWithBuiltins must return isolated *Registry instances")
	}
}

func TestNewWithBuiltins_EmptyWhenNoneRegistered(t *testing.T) {
	t.Cleanup(resetBuiltins)
	r := NewWithBuiltins()
	_, err := r.Lookup("anything")
	if err == nil {
		t.Fatal("expected ErrUnknownSignerType, got nil")
	}
}
