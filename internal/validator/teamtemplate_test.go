package validator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/validator"
)

const validTeamTemplateYAML = `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: prod/api-key
        db_password: prod/db-credentials#password
    staging:
      provider: azure-key-vault
      vault_name: staging-vault
      keys:
        api_key: staging-api-key
        db_password: staging-db-password
`

const invalidProviderYAML = `team_secrets:
  vault_configs:
    production:
      provider: foo
      region: eu-west-1
      keys:
        api_key: prod/api-key
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "tpl*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_ = f.Close()
	return f.Name()
}

func TestValidateAuto_DispatchesByTopLevelKey(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantKind  validator.ResultKind
		wantValid bool
	}{
		{
			name:      "collection_file_routes_to_collection",
			content:   "name: x\nrequests: []\n",
			wantKind:  validator.KindCollection,
			wantValid: true,
		},
		{
			name:      "team_template_routes_to_team",
			content:   validTeamTemplateYAML,
			wantKind:  validator.KindTeamTemplate,
			wantValid: true,
		},
		{
			name:      "invalid_team_template_reports_key_path",
			content:   invalidProviderYAML,
			wantKind:  validator.KindTeamTemplate,
			wantValid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTemp(t, tc.content)
			result := validator.ValidateAuto(path, nil)
			if result.Kind != tc.wantKind {
				t.Errorf("Kind = %v, want %v", result.Kind, tc.wantKind)
			}
			if result.Valid != tc.wantValid {
				t.Errorf("Valid = %v, want %v; issues: %v", result.Valid, tc.wantValid, result.Issues)
			}
		})
	}
}

func TestValidateAuto_SummaryPopulated(t *testing.T) {
	path := writeTemp(t, validTeamTemplateYAML)
	result := validator.ValidateAuto(path, nil)
	if result.Summary != "2 environments, 4 secrets" {
		t.Errorf("Summary = %q, want %q", result.Summary, "2 environments, 4 secrets")
	}
}

func TestValidateAuto_ErrorMessageIncludesKeyPath(t *testing.T) {
	path := writeTemp(t, invalidProviderYAML)
	result := validator.ValidateAuto(path, nil)
	if result.Valid {
		t.Fatal("expected invalid result")
	}
	if len(result.Issues) == 0 {
		t.Fatal("expected issues, got none")
	}
	for _, iss := range result.Issues {
		if strings.Contains(iss.Message, "team_secrets.vault_configs.production.provider") {
			return
		}
	}
	t.Errorf("no issue contains key path 'team_secrets.vault_configs.production.provider'; issues: %v", result.Issues)
}

func TestValidateAuto_ValidCollectionFile(t *testing.T) {
	// Create a real collection file on disk using tempdir.
	dir := t.TempDir()
	path := filepath.Join(dir, "col.yaml")
	if err := os.WriteFile(path, []byte("name: Test\nrequests: []\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	result := validator.ValidateAuto(path, nil)
	if result.Kind != validator.KindCollection {
		t.Errorf("Kind = %v, want KindCollection", result.Kind)
	}
	if !result.Valid {
		t.Errorf("expected valid; issues: %v", result.Issues)
	}
}
