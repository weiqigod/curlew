package teamtemplate

import (
	"context"
	"fmt"
	"sort"

	"github.com/peterlindqvist/apitest/internal/vault"
)

// stubValueFormat is the deterministic value pattern returned by StubProvider.
// Pinned as a constant so tests can reference it via fmt.Sprintf.
const stubValueFormat = "stub::%s::%s"

// StubProvider is a deterministic in-memory vault provider used when
// APITEST_VAULT_STUB=1. It returns values of the form
//
//	stub::<envName>::<path>[#<field>]
//
// so tests can assert on the exact resolved value without needing real
// cloud credentials.
type StubProvider struct {
	envName  string
	provider string // "aws-secrets-manager" or "azure-key-vault"
}

// Ensure StubProvider satisfies vault.Provider at compile time.
var _ vault.Provider = (*StubProvider)(nil)

// NewStubProvider constructs a stub scoped to a single environment.
func NewStubProvider(envName, provider string) *StubProvider {
	return &StubProvider{envName: envName, provider: provider}
}

// Name returns the provider identifier for this stub.
func (s *StubProvider) Name() string { return "stub::" + s.provider }

// Fetch returns a deterministic value derived from the environment name and path.
func (s *StubProvider) Fetch(_ context.Context, path string) (string, error) {
	return fmt.Sprintf(stubValueFormat, s.envName, path), nil
}

// BulkFetch retrieves multiple secrets in a single call.
// Paths are processed in sorted order for deterministic output.
func (s *StubProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error) {
	// Deduplicate and sort for determinism.
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			unique = append(unique, p)
		}
	}
	sort.Strings(unique)

	out := make(map[string]string, len(unique))
	for _, p := range unique {
		v, _ := s.Fetch(ctx, p)
		out[p] = v
	}
	return out, nil
}

// ValidateConfig always returns nil — the stub has no prerequisites.
func (s *StubProvider) ValidateConfig() error { return nil }
