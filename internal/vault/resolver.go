package vault

import (
	"context"
	"fmt"
	"time"

	apierrors "github.com/weiqigod/curlew/internal/errors"
)

// ResolveResult holds resolved secret values keyed by variable name.
type ResolveResult struct {
	Variables map[string]string
}

// NewProvider creates the appropriate Provider for the given config.
func NewProvider(cfg *SecretsConfig, exec CommandExecutor) (Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: nil secrets config", ErrMissingRequiredField)
	}
	switch cfg.Provider {
	case ProviderAWS:
		return NewAWSProvider(cfg.Region, exec), nil
	case ProviderAzure:
		return NewAzureProvider(cfg.VaultName, exec), nil
	case ProviderHashiCorp:
		return NewHashiCorpProvider(cfg.Address, cfg.Auth, exec), nil
	case ProviderGCP:
		return NewGCPProvider(cfg.Project, exec), nil
	case Provider1Password:
		return NewOnePasswordProvider(exec), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, cfg.Provider)
	}
}

// wrapVaultFetchError wraps a provider fetch error in a Structured error so
// callers receive a user-facing message and hint alongside the original error chain.
func wrapVaultFetchError(p Provider, err error) error {
	return &apierrors.Structured{
		Category: apierrors.CategoryConfig,
		Message:  fmt.Sprintf("vault secret fetch failed (provider: %s): %v", p.Name(), err),
		Hint:     fmt.Sprintf("Check that the secret exists and credentials are configured for %s", p.Name()),
		Inner:    err,
	}
}

// resolveSecrets performs a single BulkFetch via provider and applies field extraction
// for each ref. Separated from Resolve so tests can inject a mock Provider directly.
func resolveSecrets(ctx context.Context, provider Provider, refs []KeyRef) (*ResolveResult, error) {
	return resolveSecretsCached(ctx, provider, refs, nil)
}

// pathCache serves raw provider values for a Resolve call from a SecretStore.
type pathCache struct {
	store SecretStore
	ttl   time.Duration
	key   func(path string) string
}

func resolveSecretsCached(ctx context.Context, provider Provider, refs []KeyRef, cache *pathCache) (*ResolveResult, error) {
	result := &ResolveResult{Variables: make(map[string]string)}

	// Collect unique paths not already cached; BulkFetch lets providers
	// optimise the misses into a single call.
	fetched := make(map[string]string, len(refs))
	misses := make([]string, 0, len(refs))
	seen := make(map[string]bool)
	for _, ref := range refs {
		if seen[ref.Path] {
			continue
		}
		seen[ref.Path] = true
		if cache != nil {
			if v, ok := cache.store.Load(cache.key(ref.Path)); ok {
				fetched[ref.Path] = v
				continue
			}
		}
		misses = append(misses, ref.Path)
	}

	if len(misses) > 0 {
		values, err := provider.BulkFetch(ctx, misses)
		if err != nil {
			return nil, wrapVaultFetchError(provider, err)
		}
		for _, path := range misses {
			fetched[path] = values[path]
			if cache != nil {
				cache.store.Save(cache.key(path), values[path], cache.ttl)
			}
		}
	}

	// Resolve each key ref — apply field extraction where needed.
	for _, ref := range refs {
		raw := fetched[ref.Path]
		if ref.Field == "" {
			result.Variables[ref.VarName] = raw
			continue
		}
		val, err := ExtractField(raw, ref.Field)
		if err != nil {
			return nil, fmt.Errorf("secret %q field %q: %w", ref.Path, ref.Field, err)
		}
		result.Variables[ref.VarName] = val
	}

	return result, nil
}

// ResolveOptions narrows and caches a ResolveWith call.
type ResolveOptions struct {
	// Only lists the variable names to resolve. Nil resolves every key.
	Only map[string]bool
	// Store keeps fetched values across runs. Used only when cfg.CacheTTL > 0.
	Store SecretStore
}

// Resolve fetches all secrets defined in cfg, applies field extraction,
// and returns a map of variable names to resolved values.
func Resolve(ctx context.Context, cfg *SecretsConfig, exec CommandExecutor) (*ResolveResult, error) {
	return ResolveWith(ctx, cfg, exec, ResolveOptions{})
}

// ResolveWith is Resolve limited to opts.Only and backed by opts.Store when
// cfg.CacheTTL is positive.
func ResolveWith(ctx context.Context, cfg *SecretsConfig, exec CommandExecutor, opts ResolveOptions) (*ResolveResult, error) {
	if cfg == nil {
		return &ResolveResult{Variables: make(map[string]string)}, nil
	}

	refs, err := cfg.ParsedKeys()
	if err != nil {
		return nil, err
	}
	if opts.Only != nil {
		selected := refs[:0]
		for _, ref := range refs {
			if opts.Only[ref.VarName] {
				selected = append(selected, ref)
			}
		}
		refs = selected
	}
	if len(refs) == 0 {
		return &ResolveResult{Variables: make(map[string]string)}, nil
	}

	provider, err := NewProvider(cfg, exec)
	if err != nil {
		return nil, err
	}

	var cache *pathCache
	if opts.Store != nil && cfg.CacheTTL > 0 {
		cache = &pathCache{
			store: opts.Store,
			ttl:   time.Duration(cfg.CacheTTL) * time.Second,
			key:   func(path string) string { return SecretCacheKey(cfg, path) },
		}
	}
	return resolveSecretsCached(ctx, provider, refs, cache)
}
