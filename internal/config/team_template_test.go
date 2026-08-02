package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/peterlindqvist/apitest/internal/vault/teamtemplate"
)

const validTeamTemplateYAML = `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: prod/api-key
        db_password: prod/db-credentials#password
`

func TestLoadTeamTemplate(t *testing.T) {
	tmp := t.TempDir()

	validPath := filepath.Join(tmp, "valid-team.yaml")
	if err := os.WriteFile(validPath, []byte(validTeamTemplateYAML), 0o644); err != nil {
		t.Fatalf("write valid template: %v", err)
	}

	invalidYAMLPath := filepath.Join(tmp, "invalid.yaml")
	if err := os.WriteFile(invalidYAMLPath, []byte("not: valid: yaml: :"), 0o644); err != nil {
		t.Fatalf("write invalid yaml: %v", err)
	}

	invalidProviderPath := filepath.Join(tmp, "bad-provider.yaml")
	badProviderYAML := `team_secrets:
  vault_configs:
    production:
      provider: foo
      keys:
        api_key: prod/api-key
`
	if err := os.WriteFile(invalidProviderPath, []byte(badProviderYAML), 0o644); err != nil {
		t.Fatalf("write bad provider: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantNil bool
		wantErr error
	}{
		{
			name:    "valid template",
			path:    validPath,
			wantNil: false,
		},
		{
			name:    "missing file",
			path:    "/nonexistent/apitest-team.yaml",
			wantErr: teamtemplate.ErrTemplateNotFound,
		},
		{
			name:    "invalid yaml",
			path:    invalidYAMLPath,
			wantErr: teamtemplate.ErrInvalidTemplate,
		},
		{
			name:    "validation fails (unknown provider)",
			path:    invalidProviderPath,
			wantErr: teamtemplate.ErrInvalidTemplate,
		},
		{
			name:    "empty path returns nil nil",
			path:    "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadTeamTemplate(tt.path)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("LoadTeamTemplate() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadTeamTemplate() unexpected error: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("LoadTeamTemplate(%q) = non-nil, want nil", tt.path)
				}
			} else {
				if got == nil {
					t.Errorf("LoadTeamTemplate(%q) = nil, want non-nil", tt.path)
				}
			}
		})
	}
}
