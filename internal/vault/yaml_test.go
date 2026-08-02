package vault

import (
	"errors"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseSecretsYAML_NilNode(t *testing.T) {
	_, err := ParseSecretsYAML(nil)
	if err == nil {
		t.Fatal("expected error for nil node, got nil")
	}
}

func TestParseSecretsYAML(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *SecretsConfig
		wantErr error
	}{
		{
			"aws_full_config",
			"provider: aws-secrets-manager\nregion: us-east-1\nkeys:\n  api_key: prod/api-key\ncache_ttl: 300",
			&SecretsConfig{
				Provider: ProviderAWS,
				Region:   "us-east-1",
				Keys:     map[string]string{"api_key": "prod/api-key"},
				CacheTTL: 300,
			},
			nil,
		},
		{
			"azure_with_keys",
			"provider: azure-key-vault\nvault_name: my-vault\nkeys:\n  db: prod-db",
			&SecretsConfig{
				Provider:  ProviderAzure,
				VaultName: "my-vault",
				Keys:      map[string]string{"db": "prod-db"},
			},
			nil,
		},
		{
			"hashicorp_approle",
			"provider: hashicorp-vault\naddress: https://vault:8200\nauth:\n  method: approle\n  role_id: r\n  secret_id: s\nkeys:\n  k: v",
			&SecretsConfig{
				Provider: ProviderHashiCorp,
				Address:  "https://vault:8200",
				Auth:     AuthConfig{Method: "approle", RoleID: "r", SecretID: "s"},
				Keys:     map[string]string{"k": "v"},
			},
			nil,
		},
		{
			"hashicorp_token",
			"provider: hashicorp-vault\naddress: https://vault:8200\nauth:\n  method: token\n  token: my-token\nkeys:\n  k: v",
			&SecretsConfig{
				Provider: ProviderHashiCorp,
				Address:  "https://vault:8200",
				Auth:     AuthConfig{Method: "token", Token: "my-token"},
				Keys:     map[string]string{"k": "v"},
			},
			nil,
		},
		{
			"gcp_with_project",
			"provider: gcp-secret-manager\nproject: my-proj\nkeys:\n  k: v",
			&SecretsConfig{
				Provider: ProviderGCP,
				Project:  "my-proj",
				Keys:     map[string]string{"k": "v"},
			},
			nil,
		},
		{
			"1password_minimal",
			"provider: 1password\nkeys:\n  k: v",
			&SecretsConfig{
				Provider: Provider1Password,
				Keys:     map[string]string{"k": "v"},
			},
			nil,
		},
		{
			"refresh_on_failure_true",
			"provider: 1password\nkeys:\n  k: v\nrefresh_on_failure: true",
			&SecretsConfig{
				Provider:         Provider1Password,
				Keys:             map[string]string{"k": "v"},
				RefreshOnFailure: true,
			},
			nil,
		},
		{
			"unknown_provider_error",
			"provider: unknown\nkeys:\n  k: v",
			nil,
			ErrUnknownProvider,
		},
		{
			"missing_provider_error",
			"keys:\n  k: v",
			nil,
			ErrUnknownProvider,
		},
		{
			"empty_keys_error",
			"provider: aws-secrets-manager\nregion: us-east-1\nkeys: {}",
			nil,
			ErrMissingRequiredField,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the raw YAML string into a yaml.Node
			var doc yaml.Node
			if err := yaml.Unmarshal([]byte(tc.input), &doc); err != nil {
				t.Fatalf("test setup: invalid YAML: %v", err)
			}
			// doc is a document node; the actual mapping is its first child
			if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
				t.Fatal("test setup: expected document node")
			}
			node := doc.Content[0]

			got, err := ParseSecretsYAML(node)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Provider != tc.want.Provider {
				t.Errorf("Provider = %q, want %q", got.Provider, tc.want.Provider)
			}
			if got.Region != tc.want.Region {
				t.Errorf("Region = %q, want %q", got.Region, tc.want.Region)
			}
			if got.VaultName != tc.want.VaultName {
				t.Errorf("VaultName = %q, want %q", got.VaultName, tc.want.VaultName)
			}
			if got.Address != tc.want.Address {
				t.Errorf("Address = %q, want %q", got.Address, tc.want.Address)
			}
			if got.Project != tc.want.Project {
				t.Errorf("Project = %q, want %q", got.Project, tc.want.Project)
			}
			if got.CacheTTL != tc.want.CacheTTL {
				t.Errorf("CacheTTL = %d, want %d", got.CacheTTL, tc.want.CacheTTL)
			}
			if got.RefreshOnFailure != tc.want.RefreshOnFailure {
				t.Errorf("RefreshOnFailure = %v, want %v", got.RefreshOnFailure, tc.want.RefreshOnFailure)
			}
			if got.Auth != tc.want.Auth {
				t.Errorf("Auth = %+v, want %+v", got.Auth, tc.want.Auth)
			}
			if len(got.Keys) != len(tc.want.Keys) {
				t.Fatalf("Keys len = %d, want %d", len(got.Keys), len(tc.want.Keys))
			}
			for k, v := range tc.want.Keys {
				if got.Keys[k] != v {
					t.Errorf("Keys[%q] = %q, want %q", k, got.Keys[k], v)
				}
			}
		})
	}
}
