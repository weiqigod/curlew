package teamtemplate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peterlindqvist/apitest/internal/vault/teamtemplate"
)

const backendYAML = `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: prod/api-key
        db_password: prod/db-credentials#password
`

const localYAML = `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: us-east-1
      keys:
        api_key: local/api-key
`

const localOverlayYAML = `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: us-east-1
      keys:
        api_key: local/api-key
    staging:
      provider: azure-key-vault
      vault_name: staging-vault
      keys:
        api_key: staging/api-key
`

// backendCache creates a Cache pre-seeded with a given YAML template.
func backendCache(t *testing.T, cfgDir, yaml string) *teamtemplate.Cache {
	t.Helper()
	env := &teamtemplate.CacheEnvelope{
		FetchedAt: time.Now().Unix(),
		Version:   3,
		Template:  yaml,
	}
	c := teamtemplate.NewCache(cfgDir)
	if err := c.Write(env); err != nil {
		t.Fatalf("write backend cache: %v", err)
	}
	return c
}

func writeLocalFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	return path
}

func TestLoad_BackendOnly(t *testing.T) {
	cfgDir := t.TempDir()
	cache := backendCache(t, cfgDir, backendYAML)

	result, err := teamtemplate.Load(context.Background(), teamtemplate.LoadOptions{
		Cache: cache,
	})
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if result == nil || result.Template == nil {
		t.Fatal("Load() returned nil result or nil template")
	}
	if result.LocalOverlay {
		t.Error("LocalOverlay = true, want false")
	}
	if result.BackendVersion != 3 {
		t.Errorf("BackendVersion = %d, want 3", result.BackendVersion)
	}

	env, ok := result.Template.Resolve("production")
	if !ok {
		t.Fatal("Resolve(production) = false, want true")
	}
	if env.Provider != "aws-secrets-manager" {
		t.Errorf("Provider = %q, want aws-secrets-manager", env.Provider)
	}
}

func TestLoad_LocalOnly(t *testing.T) {
	cfgDir := t.TempDir()
	localPath := writeLocalFile(t, cfgDir, "team.yaml", localYAML)

	result, err := teamtemplate.Load(context.Background(), teamtemplate.LoadOptions{
		LocalPath: localPath,
	})
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if result == nil || result.Template == nil {
		t.Fatal("Load() returned nil result or nil template")
	}
	if !result.LocalOverlay {
		t.Error("LocalOverlay = false, want true")
	}
	if result.BackendVersion != 0 {
		t.Errorf("BackendVersion = %d, want 0", result.BackendVersion)
	}
}

func TestLoad_NeitherSource_ReturnsNilTemplate(t *testing.T) {
	result, err := teamtemplate.Load(context.Background(), teamtemplate.LoadOptions{})
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if result != nil && result.Template != nil {
		t.Error("expected nil template when no sources configured")
	}
}

func TestLoad_BackendPlusLocalOverlay(t *testing.T) {
	cfgDir := t.TempDir()
	cache := backendCache(t, cfgDir, backendYAML)
	localPath := writeLocalFile(t, cfgDir, "team.yaml", localOverlayYAML)

	result, err := teamtemplate.Load(context.Background(), teamtemplate.LoadOptions{
		Cache:     cache,
		LocalPath: localPath,
	})
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if result == nil || result.Template == nil {
		t.Fatal("Load() returned nil result or nil template")
	}
	if !result.LocalOverlay {
		t.Error("LocalOverlay = false, want true")
	}

	// Local wins on production.api_key (local/api-key beats prod/api-key).
	prod, ok := result.Template.Resolve("production")
	if !ok {
		t.Fatal("Resolve(production) = false")
	}
	if prod.Keys["api_key"].Path != "local/api-key" {
		t.Errorf("production.api_key.Path = %q, want local/api-key", prod.Keys["api_key"].Path)
	}
	// Backend's db_password is preserved where local doesn't override it.
	if _, ok := prod.Keys["db_password"]; !ok {
		t.Error("production.db_password missing after merge")
	}

	// Staging added by local overlay.
	_, ok = result.Template.Resolve("staging")
	if !ok {
		t.Error("staging env missing after merge")
	}
}

// Merge tests.

func TestTeamTemplate_Merge_LocalKeyWins(t *testing.T) {
	base, err := teamtemplate.Parse([]byte(backendYAML))
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}

	local, err := teamtemplate.Parse([]byte(localYAML))
	if err != nil {
		t.Fatalf("parse local: %v", err)
	}

	merged := base.Merge(local)
	prod, ok := merged.Resolve("production")
	if !ok {
		t.Fatal("Resolve(production) = false after merge")
	}
	if prod.Keys["api_key"].Path != "local/api-key" {
		t.Errorf("api_key.Path = %q after merge, want local/api-key", prod.Keys["api_key"].Path)
	}
	// db_password from base still present.
	if _, ok := prod.Keys["db_password"]; !ok {
		t.Error("db_password missing after merge")
	}
}

func TestTeamTemplate_Merge_LocalAddsEnvironment(t *testing.T) {
	base, err := teamtemplate.Parse([]byte(backendYAML))
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}
	local, err := teamtemplate.Parse([]byte(localOverlayYAML))
	if err != nil {
		t.Fatalf("parse local: %v", err)
	}

	merged := base.Merge(local)
	if _, ok := merged.Resolve("staging"); !ok {
		t.Error("staging env missing after merge with local that adds it")
	}
	if len(merged.Environments) != 2 {
		t.Errorf("envs = %d, want 2", len(merged.Environments))
	}
}

func TestTeamTemplate_Merge_DisjointEnvironments(t *testing.T) {
	baseYAML := `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: prod/api-key
`
	localYAMLDisjoint := `team_secrets:
  vault_configs:
    staging:
      provider: aws-secrets-manager
      region: us-east-1
      keys:
        api_key: staging/api-key
`
	base, _ := teamtemplate.Parse([]byte(baseYAML))
	local, _ := teamtemplate.Parse([]byte(localYAMLDisjoint))

	merged := base.Merge(local)
	if len(merged.Environments) != 2 {
		t.Errorf("envs = %d, want 2 for disjoint merge", len(merged.Environments))
	}
}

func TestTeamTemplate_Merge_BaseAndLocalNilSafe(t *testing.T) {
	base, _ := teamtemplate.Parse([]byte(backendYAML))
	// Merging nil local should return base unchanged.
	merged := base.Merge(nil)
	if len(merged.Environments) != len(base.Environments) {
		t.Error("merge with nil local changed environment count")
	}
}
