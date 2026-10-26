package variable

import (
	"errors"
	"sync/atomic"
	"testing"
)

func deferredScope(t *testing.T, vars map[string]string, values map[string]string, calls *int32) *Scope {
	t.Helper()
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	d := NewDeferred(names, func(name string) (string, error) {
		atomic.AddInt32(calls, 1)
		v, ok := values[name]
		if !ok {
			return "", errors.New("no such secret")
		}
		return v, nil
	})
	s := NewScope(vars).WithDeferred(d)
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return s
}

func TestDeferred_ResolveDoesNotFetch(t *testing.T) {
	var calls int32
	deferredScope(t, map[string]string{"token": "{{prod_secret}}", "plain": "x"}, map[string]string{"prod_secret": "p"}, &calls)
	if calls != 0 {
		t.Fatalf("Resolve fetched %d deferred values; it must fetch none", calls)
	}
}

func TestDeferred_FetchedOnFirstUseThroughAnotherVariable(t *testing.T) {
	var calls int32
	s := deferredScope(t, map[string]string{"token": "Bearer {{prod_secret}}"}, map[string]string{"prod_secret": "p"}, &calls)
	for i := 0; i < 2; i++ {
		got, err := s.Interpolate("auth: {{token}}")
		if err != nil {
			t.Fatalf("Interpolate: %v", err)
		}
		if got != "auth: Bearer p" {
			t.Fatalf("got %q", got)
		}
	}
	if calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", calls)
	}
}

func TestDeferred_DirectAndDefaultReferences(t *testing.T) {
	var calls int32
	s := deferredScope(t, map[string]string{}, map[string]string{"prod_secret": "p"}, &calls)
	got, err := s.Interpolate("{{prod_secret}} {{prod_secret|default:none}}")
	if err != nil || got != "p p" {
		t.Fatalf("got %q, %v", got, err)
	}
	if calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", calls)
	}
}

func TestDeferred_FetchErrorFailsTheInterpolation(t *testing.T) {
	var calls int32
	d := NewDeferred([]string{"broken"}, func(string) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "", errors.New("provider failed")
	})
	s := NewScope(map[string]string{"token": "{{broken}}"}).WithDeferred(d)
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := s.Interpolate("{{token}}"); err == nil {
		t.Fatal("expected the fetch error")
	}
}

func TestDeferred_SharedBySnapshotAndOverrides(t *testing.T) {
	var calls int32
	s := deferredScope(t, map[string]string{"token": "{{prod_secret}}"}, map[string]string{"prod_secret": "p"}, &calls)
	snap := s.Snapshot()
	child, err := s.WithOverrides(map[string]string{"extra": "{{token}}-x"})
	if err != nil {
		t.Fatalf("WithOverrides: %v", err)
	}
	for _, sc := range []*Scope{snap, child, s} {
		if got, err := sc.Interpolate("{{token}}"); err != nil || got != "p" {
			t.Fatalf("got %q, %v", got, err)
		}
	}
	if got, err := child.Interpolate("{{extra}}"); err != nil || got != "p-x" {
		t.Fatalf("override got %q, %v", got, err)
	}
	if calls != 1 {
		t.Fatalf("fetch calls = %d, want 1 across snapshot, overrides and parent", calls)
	}
}

func TestDeferred_UnknownNameIsStillUndefined(t *testing.T) {
	d := NewDeferred([]string{"prod_secret"}, func(string) (string, error) { return "p", nil })
	s := NewScope(map[string]string{"token": "{{nope}}"}).WithDeferred(d)
	if err := s.Resolve(); !errors.Is(err, ErrUndefinedVariable) {
		t.Fatalf("Resolve error = %v, want undefined variable", err)
	}
}
