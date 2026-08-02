package signer

import (
	"context"

	"github.com/weiqigod/curlew/internal/variable"
)

// scopeKey is the unexported context key for the per-request
// *variable.Scope, attached by the runner before invoking the signer
// wrap. Concrete signers retrieve it via ScopeFromContext when they
// need to interpolate {{var}} references in their params.
type scopeKey struct{}

// WithScope returns a derived context carrying scope. Pass nil to
// indicate "no scope available" (downstream signers must treat params
// as already-literal in that case).
func WithScope(ctx context.Context, scope *variable.Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// ScopeFromContext returns the scope previously attached via WithScope,
// or nil when none is present.
func ScopeFromContext(ctx context.Context) *variable.Scope {
	v, _ := ctx.Value(scopeKey{}).(*variable.Scope)
	return v
}
