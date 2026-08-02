package teamtemplate

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/weiqigod/curlew/internal/vault"
)

// validTwoEnvTemplate is a fixture with production (AWS) and staging (Azure),
// each having two keys. Paths deliberately avoid #field references so that
// the StubProvider — which returns plain strings, not JSON — can resolve them
// without field extraction errors.
const validTwoEnvTemplate = `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: prod/api-key
        db_password: prod/db-password
    staging:
      provider: azure-key-vault
      vault_name: staging-vault
      keys:
        api_key: staging-api-key
        db_password: staging-db-password
`

func mustParse(t *testing.T, yaml string) *TeamTemplate {
	t.Helper()
	tpl, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if issues := tpl.Validate(); len(issues) > 0 {
		t.Fatalf("Validate() issues: %v", issues)
	}
	return tpl
}

// stubFactory returns a provider factory that always creates a StubProvider
// for the given env name, regardless of what env.Provider says.
func stubFactory(envName string) func(*ResolvedEnv) (vault.Provider, error) {
	return func(env *ResolvedEnv) (vault.Provider, error) {
		return NewStubProvider(envName, env.Provider), nil
	}
}

// countingProvider wraps an inner Provider and counts BulkFetch calls.
type countingProvider struct {
	inner vault.Provider
	calls *int
}

func (c *countingProvider) Name() string { return c.inner.Name() }
func (c *countingProvider) Fetch(ctx context.Context, path string) (string, error) {
	return c.inner.Fetch(ctx, path)
}

func (c *countingProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error) {
	*c.calls++
	return c.inner.BulkFetch(ctx, paths)
}
func (c *countingProvider) ValidateConfig() error { return c.inner.ValidateConfig() }

func TestSecretsResolver_Resolve(t *testing.T) {
	tpl := mustParse(t, validTwoEnvTemplate)
	r, err := NewSecretsResolver(tpl, "production", stubFactory("production"))
	if err != nil {
		t.Fatalf("NewSecretsResolver() error: %v", err)
	}

	got, err := r.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	wantAPIKey := "stub::production::prod/api-key"
	if got["api_key"] != wantAPIKey {
		t.Errorf("api_key = %q, want %q", got["api_key"], wantAPIKey)
	}
	// db_password has a #field suffix in the path but stub ignores field extraction
	if got["db_password"] == "" {
		t.Error("db_password should be present")
	}
}

func TestSecretsResolver_CachedOnSecondCall(t *testing.T) {
	tpl := mustParse(t, validTwoEnvTemplate)
	calls := 0
	factory := func(env *ResolvedEnv) (vault.Provider, error) {
		inner := NewStubProvider(env.Name, env.Provider)
		return &countingProvider{inner: inner, calls: &calls}, nil
	}
	r, err := NewSecretsResolver(tpl, "production", factory)
	if err != nil {
		t.Fatalf("NewSecretsResolver() error: %v", err)
	}

	ctx := context.Background()
	_, err = r.Resolve(ctx)
	if err != nil {
		t.Fatalf("first Resolve() error: %v", err)
	}
	_, err = r.Resolve(ctx)
	if err != nil {
		t.Fatalf("second Resolve() error: %v", err)
	}

	if calls != 1 {
		t.Errorf("BulkFetch called %d times, want 1 (second call should be cached)", calls)
	}
}

func TestSecretsResolver_UnknownEnvironment(t *testing.T) {
	tpl := mustParse(t, validTwoEnvTemplate)
	_, err := NewSecretsResolver(tpl, "qa", stubFactory("qa"))
	if !errors.Is(err, ErrUnknownEnvironment) {
		t.Errorf("NewSecretsResolver() with unknown env = %v, want ErrUnknownEnvironment", err)
	}
}

func TestSecretsResolver_EnvName(t *testing.T) {
	tpl := mustParse(t, validTwoEnvTemplate)
	r, err := NewSecretsResolver(tpl, "staging", stubFactory("staging"))
	if err != nil {
		t.Fatalf("NewSecretsResolver() error: %v", err)
	}
	if r.EnvName() != "staging" {
		t.Errorf("EnvName() = %q, want %q", r.EnvName(), "staging")
	}
}

func TestSecretsResolver_Count(t *testing.T) {
	tpl := mustParse(t, validTwoEnvTemplate)
	r, err := NewSecretsResolver(tpl, "production", stubFactory("production"))
	if err != nil {
		t.Fatalf("NewSecretsResolver() error: %v", err)
	}
	if r.Count() != 2 {
		t.Errorf("Count() = %d, want 2", r.Count())
	}
}

func TestSecretsResolver_HasAlias(t *testing.T) {
	tpl := mustParse(t, validTwoEnvTemplate)
	r, err := NewSecretsResolver(tpl, "production", stubFactory("production"))
	if err != nil {
		t.Fatalf("NewSecretsResolver() error: %v", err)
	}
	if !r.HasAlias("api_key") {
		t.Error("HasAlias(api_key) = false, want true")
	}
	if r.HasAlias("missing") {
		t.Error("HasAlias(missing) = true, want false")
	}
}

// errProvider is a vault.Provider that always returns an error from BulkFetch.
type errProvider struct {
	name string
	err  error
}

func (e *errProvider) Name() string { return e.name }
func (e *errProvider) Fetch(_ context.Context, _ string) (string, error) {
	return "", e.err
}

func (e *errProvider) BulkFetch(_ context.Context, _ []string) (map[string]string, error) {
	return nil, e.err
}
func (e *errProvider) ValidateConfig() error { return nil }

// omitPathProvider returns BulkFetch results that omit one of the requested paths.
type omitPathProvider struct {
	inner    vault.Provider
	omitPath string
}

func (o *omitPathProvider) Name() string { return o.inner.Name() }
func (o *omitPathProvider) Fetch(ctx context.Context, path string) (string, error) {
	return o.inner.Fetch(ctx, path)
}

func (o *omitPathProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error) {
	m, err := o.inner.BulkFetch(ctx, paths)
	if err != nil {
		return nil, err
	}
	delete(m, o.omitPath)
	return m, nil
}
func (o *omitPathProvider) ValidateConfig() error { return nil }

// fieldErrProvider returns BulkFetch results where one path value is not valid JSON,
// triggering a vault.ExtractField error when ref.Field is set.
type fieldErrProvider struct {
	inner    vault.Provider
	badPath  string
	badValue string
}

func (f *fieldErrProvider) Name() string { return f.inner.Name() }
func (f *fieldErrProvider) Fetch(ctx context.Context, path string) (string, error) {
	return f.inner.Fetch(ctx, path)
}

func (f *fieldErrProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error) {
	m, err := f.inner.BulkFetch(ctx, paths)
	if err != nil {
		return nil, err
	}
	m[f.badPath] = f.badValue
	return m, nil
}
func (f *fieldErrProvider) ValidateConfig() error { return nil }

// fieldEnvTemplate is a fixture whose production environment uses a #field
// reference so that ExtractField is exercised.
const fieldEnvTemplate = `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: prod/api-key#token
`

// TestNewSecretsResolver_FactoryError verifies that when makeProvider returns
// an error, NewSecretsResolver propagates it (finding #4).
func TestNewSecretsResolver_FactoryError(t *testing.T) {
	tpl := mustParse(t, validTwoEnvTemplate)
	factoryErr := errors.New("credentials not configured")
	_, err := NewSecretsResolver(tpl, "production", func(_ *ResolvedEnv) (vault.Provider, error) {
		return nil, factoryErr
	})
	if !errors.Is(err, factoryErr) {
		t.Errorf("NewSecretsResolver() error = %v, want %v", err, factoryErr)
	}
}

// TestSecretsResolver_BulkFetchError verifies that a BulkFetch error is
// cached and returned on subsequent calls (finding #3a).
func TestSecretsResolver_BulkFetchError(t *testing.T) {
	tpl := mustParse(t, validTwoEnvTemplate)
	fetchErr := errors.New("vault unavailable")
	r, err := NewSecretsResolver(tpl, "production", func(env *ResolvedEnv) (vault.Provider, error) {
		return &errProvider{name: "stub", err: fetchErr}, nil
	})
	if err != nil {
		t.Fatalf("NewSecretsResolver() error: %v", err)
	}

	ctx := context.Background()
	_, resolveErr := r.Resolve(ctx)
	if resolveErr == nil {
		t.Fatal("Resolve() expected error, got nil")
	}
	if !errors.Is(resolveErr, fetchErr) {
		t.Errorf("Resolve() error = %v, want to wrap %v", resolveErr, fetchErr)
	}

	// Second call should return the cached error.
	_, cachedErr := r.Resolve(ctx)
	if cachedErr == nil {
		t.Fatal("second Resolve() expected cached error, got nil")
	}
}

// TestSecretsResolver_MissingPath verifies that when BulkFetch omits a
// requested path the resolver treats it as an empty string and still
// returns a result map (finding #3b).
func TestSecretsResolver_MissingPath(t *testing.T) {
	tpl := mustParse(t, validTwoEnvTemplate)
	r, err := NewSecretsResolver(tpl, "production", func(env *ResolvedEnv) (vault.Provider, error) {
		inner := NewStubProvider(env.Name, env.Provider)
		return &omitPathProvider{inner: inner, omitPath: "prod/api-key"}, nil
	})
	if err != nil {
		t.Fatalf("NewSecretsResolver() error: %v", err)
	}

	secrets, resolveErr := r.Resolve(context.Background())
	if resolveErr != nil {
		t.Fatalf("Resolve() unexpected error: %v", resolveErr)
	}
	// The omitted path should produce an empty string for its alias.
	if secrets["api_key"] != "" {
		t.Errorf("api_key = %q, want empty string (path omitted by provider)", secrets["api_key"])
	}
	// The non-omitted path should still be resolved.
	if secrets["db_password"] == "" {
		t.Error("db_password should be non-empty (path was returned by provider)")
	}
}

// TestSecretsResolver_FieldExtractionError verifies that when vault.ExtractField
// fails (because the secret value is not valid JSON), the error is cached and
// returned (finding #3c).
func TestSecretsResolver_FieldExtractionError(t *testing.T) {
	tpl := mustParse(t, fieldEnvTemplate)
	r, err := NewSecretsResolver(tpl, "production", func(env *ResolvedEnv) (vault.Provider, error) {
		inner := NewStubProvider(env.Name, env.Provider)
		// Return a non-JSON value for the path that has a #field reference.
		return &fieldErrProvider{
			inner:    inner,
			badPath:  "prod/api-key",
			badValue: fmt.Sprintf("not-json-%s", env.Name),
		}, nil
	})
	if err != nil {
		t.Fatalf("NewSecretsResolver() error: %v", err)
	}

	_, resolveErr := r.Resolve(context.Background())
	if resolveErr == nil {
		t.Fatal("Resolve() expected error from ExtractField, got nil")
	}
	if !errors.Is(resolveErr, vault.ErrNotJSONSecret) {
		t.Errorf("Resolve() error = %v, want to wrap vault.ErrNotJSONSecret", resolveErr)
	}
}
