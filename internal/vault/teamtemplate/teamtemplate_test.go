package teamtemplate_test

import (
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/vault/teamtemplate"
)

// validTemplateYAML has two environments (production=AWS, staging=Azure)
// with a total of 4 key aliases.
const validTemplateYAML = `team_secrets:
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

func TestTeamTemplate(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantErr    bool
		wantIssues []string // substrings expected in issue messages
		wantEnvs   int
		wantKeys   int
	}{
		{
			name:     "valid_aws_plus_azure",
			yaml:     validTemplateYAML,
			wantEnvs: 2,
			wantKeys: 4,
		},
		{
			name: "unknown_provider_error",
			yaml: `team_secrets:
  vault_configs:
    production:
      provider: foo
      keys:
        api_key: prod/api-key`,
			wantIssues: []string{
				"team_secrets.vault_configs.production.provider",
				"unknown provider 'foo'",
			},
		},
		{
			name: "aws_missing_region",
			yaml: `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      keys:
        api_key: prod/api-key`,
			wantIssues: []string{
				"team_secrets.vault_configs.production.region",
			},
		},
		{
			name: "azure_missing_vault_name",
			yaml: `team_secrets:
  vault_configs:
    staging:
      provider: azure-key-vault
      keys:
        api_key: staging-api-key`,
			wantIssues: []string{
				"team_secrets.vault_configs.staging.vault_name",
			},
		},
		{
			name: "compound_secret_with_field",
			yaml: `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        db_password: prod/db-credentials#password`,
			wantEnvs: 1,
			wantKeys: 1,
		},
		{
			name: "duplicate_alias_in_same_env",
			yaml: `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: prod/api-key
        api_key: prod/other-key`,
			wantIssues: []string{"duplicate key alias 'api_key'"},
		},
		{
			name: "env_missing_keys_block",
			yaml: `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1`,
			wantIssues: []string{
				"team_secrets.vault_configs.production.keys",
			},
		},
		{
			name: "invalid_key_ref_empty_path_before_hash",
			yaml: `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: "#password"`,
			wantIssues: []string{
				"team_secrets.vault_configs.production.keys.api_key",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tpl, err := teamtemplate.Parse([]byte(tc.yaml))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected parse error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			issues := tpl.Validate()

			if tc.wantEnvs > 0 && len(tpl.Environments) != tc.wantEnvs {
				t.Errorf("environments = %d, want %d", len(tpl.Environments), tc.wantEnvs)
			}

			if tc.wantKeys > 0 {
				total := 0
				for _, env := range tpl.Environments {
					total += len(env.Keys)
				}
				if total != tc.wantKeys {
					t.Errorf("total keys = %d, want %d", total, tc.wantKeys)
				}
			}

			if len(tc.wantIssues) > 0 {
				if len(issues) == 0 {
					t.Fatal("expected validation issues, got none")
				}
				// Collect all issue text for checking.
				allIssueText := make([]string, 0, len(issues))
				for _, iss := range issues {
					allIssueText = append(allIssueText, iss.Path+" "+iss.Message)
				}
				combined := strings.Join(allIssueText, "\n")
				for _, want := range tc.wantIssues {
					if !strings.Contains(combined, want) {
						t.Errorf("issues do not contain %q; issues:\n%s", want, combined)
					}
				}
			} else if tc.wantEnvs > 0 || tc.wantKeys > 0 {
				// Expected a valid template.
				if len(issues) != 0 {
					t.Errorf("expected no issues, got %d: %v", len(issues), issues)
				}
			}
		})
	}
}

func TestTeamTemplate_Resolve(t *testing.T) {
	tpl, err := teamtemplate.Parse([]byte(validTemplateYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	env, ok := tpl.Resolve("production")
	if !ok {
		t.Fatal("Resolve(production) returned false")
	}
	if env.Provider != "aws-secrets-manager" {
		t.Errorf("Provider = %q, want aws-secrets-manager", env.Provider)
	}
	if env.Region != "eu-west-1" {
		t.Errorf("Region = %q, want eu-west-1", env.Region)
	}

	apiKey, ok := env.Keys["api_key"]
	if !ok {
		t.Fatal("Keys[api_key] not found")
	}
	if apiKey.Path != "prod/api-key" {
		t.Errorf("api_key.Path = %q, want prod/api-key", apiKey.Path)
	}

	dbPwd, ok := env.Keys["db_password"]
	if !ok {
		t.Fatal("Keys[db_password] not found")
	}
	if dbPwd.Path != "prod/db-credentials" {
		t.Errorf("db_password.Path = %q, want prod/db-credentials", dbPwd.Path)
	}
	if dbPwd.Field != "password" {
		t.Errorf("db_password.Field = %q, want password", dbPwd.Field)
	}

	staging, ok := tpl.Resolve("staging")
	if !ok {
		t.Fatal("Resolve(staging) returned false")
	}
	if staging.VaultName != "staging-vault" {
		t.Errorf("VaultName = %q, want staging-vault", staging.VaultName)
	}

	_, ok = tpl.Resolve("nonexistent")
	if ok {
		t.Error("Resolve(nonexistent) should return false")
	}
}

func TestTeamTemplate_Summary(t *testing.T) {
	tpl, err := teamtemplate.Parse([]byte(validTemplateYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := "2 environments, 4 secrets"
	got := tpl.Summary()
	if got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}
}
