package vault

import (
	"context"
	"fmt"

	apierrors "github.com/peterlindqvist/apitest/internal/errors"
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
	result := &ResolveResult{Variables: make(map[string]string)}

	// Collect unique paths; BulkFetch lets providers optimise into a single API call.
	uniquePaths := make([]string, 0, len(refs))
	seen := make(map[string]bool)
	for _, ref := range refs {
		if !seen[ref.Path] {
			uniquePaths = append(uniquePaths, ref.Path)
			seen[ref.Path] = true
		}
	}

	fetched, err := provider.BulkFetch(ctx, uniquePaths)
	if err != nil {
		return nil, wrapVaultFetchError(provider, err)
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

// Resolve fetches all secrets defined in cfg, applies caching and field extraction,
// and returns a map of variable names to resolved values.
func Resolve(ctx context.Context, cfg *SecretsConfig, exec CommandExecutor) (*ResolveResult, error) {
	if cfg == nil {
		return &ResolveResult{Variables: make(map[string]string)}, nil
	}

	refs, err := cfg.ParsedKeys()
	if err != nil {
		return nil, err
	}

	provider, err := NewProvider(cfg, exec)
	if err != nil {
		return nil, err
	}

	return resolveSecrets(ctx, provider, refs)
}
