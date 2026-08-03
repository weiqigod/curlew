package teamtemplate_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/weiqigod/curlew/internal/vault/teamtemplate"
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

func writeLocalFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	return path
}

func TestLoad_LocalOnly(t *testing.T) {
	dir := t.TempDir()
	localPath := writeLocalFile(t, dir, "team.yaml", localYAML)

	result, err := teamtemplate.Load(teamtemplate.LoadOptions{LocalPath: localPath})
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if result == nil || result.Template == nil {
		t.Fatal("Load() returned nil result or nil template")
	}
	if !result.LocalOverlay {
		t.Error("LocalOverlay = false, want true")
	}
}

func TestLoad_NoLocalPath_ReturnsNilTemplate(t *testing.T) {
	result, err := teamtemplate.Load(teamtemplate.LoadOptions{})
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result when no template is configured, got %+v", result)
	}
}

func TestLoad_MissingLocalFile_IsAnError(t *testing.T) {
	_, err := teamtemplate.Load(teamtemplate.LoadOptions{
		LocalPath: filepath.Join(t.TempDir(), "absent.yaml"),
	})
	if err == nil {
		t.Fatal("Load() with a missing CURLEW_TEAM_CONFIG path = nil error; want ErrTemplateNotFound")
	}
	if !errors.Is(err, teamtemplate.ErrTemplateNotFound) {
		t.Errorf("err = %v; want ErrTemplateNotFound", err)
	}
}

func TestLoad_MalformedLocalFile_IsAnError(t *testing.T) {
	dir := t.TempDir()
	path := writeLocalFile(t, dir, "bad.yaml", "team_secrets: [not, a, mapping\n")

	if _, err := teamtemplate.Load(teamtemplate.LoadOptions{LocalPath: path}); err == nil {
		t.Fatal("Load() with a malformed template = nil error; want a parse error")
	}
}

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
