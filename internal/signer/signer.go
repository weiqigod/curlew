// Package signer defines the contract and registry for first-party request
// signers. A Signer mutates an httpexec.Request in place to add signing
// material (typically headers, sometimes query params) just before the
// request is dispatched.
//
// # Clock seam
//
// The Signer interface deliberately does NOT carry a clock argument.
// Concrete signers that need a clock for testability (e.g. AWS SigV4 in
// M17-002, OAuth1 in M17-003) own it inside their own package — the
// convention is a Factory option WithClock(now func() time.Time)
// defaulting to time.Now. This matches the clock seam in
// Registry.now in internal/variable/dynamic.go. Most signers will never
// need a clock; threading it through the interface signature would force
// every implementer to carry an argument they ignore.
//
// # Sensitive values
//
// Each concrete signer is responsible for calling
// sensitives.AddValue(resolved) for any credential it resolves from a
// sensitive variable, mirroring the M12-005 $hmacSha256 pattern.
// AddValue is no-op safe on a nil set.
package signer

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/peterlindqvist/apitest/internal/httpexec"
	"github.com/peterlindqvist/apitest/internal/variable"
)

// ErrUnknownSignerType is returned by Registry.Lookup when the requested
// type name is not registered. Callers wrap with fmt.Errorf to add the
// requested name and the sorted list of available types.
var ErrUnknownSignerType = errors.New("unknown signer type")

// Signer mutates an httpexec.Request to add signing material. Implementations
// must be safe to call on any *httpexec.Request and may add headers, mutate
// the body, or re-encode the URL. Implementations that resolve credentials
// from sensitive variables should register the resolved string value via
// sensitives.AddValue for redaction.
type Signer interface {
	Sign(ctx context.Context, req *httpexec.Request, sensitives *variable.SensitiveSet) error
}

// Factory builds a Signer from the YAML params: map. Returning a non-nil
// error fails the run before the request is dispatched; the factory must
// surface a structured, actionable error message.
type Factory func(params map[string]any) (Signer, error)

// Registry is a name-keyed lookup of signer factories.
type Registry struct {
	factories map[string]Factory
}

// NewBuiltinRegistry returns a registry preloaded with all first-party
// signers. In M17-001 the registry is empty; aws-sigv4 (M17-002) and
// oauth1 (M17-003) register themselves into NewBuiltinRegistry as they
// land.
func NewBuiltinRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

// Register associates name with factory. Returns an error when name is
// already registered (callers should treat this as a programming error).
func (r *Registry) Register(name string, factory Factory) error {
	if _, dup := r.factories[name]; dup {
		return fmt.Errorf("signer type %q already registered", name)
	}
	r.factories[name] = factory
	return nil
}

// Lookup returns the factory for name, or ErrUnknownSignerType wrapped
// with the requested name and a sorted, comma-joined list of registered
// types.
func (r *Registry) Lookup(name string) (Factory, error) {
	if f, ok := r.factories[name]; ok {
		return f, nil
	}
	available := make([]string, 0, len(r.factories))
	for n := range r.factories {
		available = append(available, n)
	}
	sort.Strings(available)
	if len(available) == 0 {
		return nil, fmt.Errorf("%w: %q; no signer types registered", ErrUnknownSignerType, name)
	}
	return nil, fmt.Errorf("%w: %q; available types: %s",
		ErrUnknownSignerType, name, strings.Join(available, ", "))
}

// requestSpecKey is the context key used to thread the resolved per-request
// signing spec through the runner's exec-wrap. Use WithRequestSpec /
// RequestSpecFromContext from the runner; the type is unexported to prevent
// collisions.
type requestSpecKey struct{}

// RequestSpec is the data the exec-wrap needs to invoke the right signer
// for the current request. The runner builds this once per request after
// resolving precedence (per-request > collection > none).
type RequestSpec struct {
	Type   string
	Params map[string]any
}

// WithRequestSpec returns a derived context carrying spec. Pass nil to
// indicate "no signing for this request".
func WithRequestSpec(ctx context.Context, spec *RequestSpec) context.Context {
	return context.WithValue(ctx, requestSpecKey{}, spec)
}

// RequestSpecFromContext returns the spec previously attached via
// WithRequestSpec, or nil when none is present.
func RequestSpecFromContext(ctx context.Context) *RequestSpec {
	v, _ := ctx.Value(requestSpecKey{}).(*RequestSpec)
	return v
}
