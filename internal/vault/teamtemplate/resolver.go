package teamtemplate

import (
	"context"
	"fmt"
	"sync"

	"github.com/peterlindqvist/apitest/internal/vault"
)

// SecretsResolver resolves {{secrets.X}} aliases against a chosen environment
// of a shared vault template. One instance lives for the duration of a single
// runner.Run invocation and caches the first successful resolve in memory.
type SecretsResolver struct {
	env      *ResolvedEnv   // active environment (never nil once constructed)
	provider vault.Provider // AWS/Azure or Stub
	mu       sync.Mutex
	cached   map[string]string // nil until first Resolve completes
	cachedEr error             // set if first Resolve returned an error
}

// NewSecretsResolver binds a template + environment name + provider factory
// into a single resolver. Returns ErrUnknownEnvironment when envName is not
// declared in the template.
func NewSecretsResolver(t *TeamTemplate, envName string, makeProvider func(*ResolvedEnv) (vault.Provider, error)) (*SecretsResolver, error) {
	env, ok := t.Resolve(envName)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownEnvironment, envName)
	}
	p, err := makeProvider(env)
	if err != nil {
		return nil, err
	}
	return &SecretsResolver{env: env, provider: p}, nil
}

// Resolve returns the resolved secrets map for the active environment.
// Subsequent calls are served from the in-memory cache; the provider is
// queried at most once per SecretsResolver instance.
func (r *SecretsResolver) Resolve(ctx context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cached != nil || r.cachedEr != nil {
		return r.cached, r.cachedEr
	}

	// Collect paths to fetch.
	refs := r.env.Keys
	paths := make([]string, 0, len(refs))
	for _, ref := range refs {
		paths = append(paths, ref.Path)
	}

	fetched, err := r.provider.BulkFetch(ctx, paths)
	if err != nil {
		r.cachedEr = fmt.Errorf("vault bulk fetch (provider=%s): %w", r.provider.Name(), err)
		return nil, r.cachedEr
	}

	out := make(map[string]string, len(refs))
	for alias, ref := range refs {
		raw, ok := fetched[ref.Path]
		if !ok {
			// Path was not returned by the provider — treat as missing.
			raw = ""
		}
		if ref.Field == "" {
			out[alias] = raw
			continue
		}
		val, extractErr := vault.ExtractField(raw, ref.Field)
		if extractErr != nil {
			r.cachedEr = fmt.Errorf("secret %q field %q: %w", ref.Path, ref.Field, extractErr)
			return nil, r.cachedEr
		}
		out[alias] = val
	}
	r.cached = out
	return out, nil
}

// EnvName returns the active environment name (for log messages).
func (r *SecretsResolver) EnvName() string { return r.env.Name }

// Count returns the number of aliases declared in the active environment.
func (r *SecretsResolver) Count() int { return len(r.env.Keys) }

// HasAlias reports whether the active environment declares the given alias.
func (r *SecretsResolver) HasAlias(alias string) bool {
	_, ok := r.env.Keys[alias]
	return ok
}
