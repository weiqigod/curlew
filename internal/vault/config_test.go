package vault

import (
	"errors"
	"sort"
	"testing"
)

func TestParseKeyRef(t *testing.T) {
	tests := []struct {
		name    string
		varName string
		raw     string
		want    KeyRef
		wantErr bool
	}{
		{"simple_key_no_field", "db_pass", "prod/api-key", KeyRef{"db_pass", "prod/api-key", ""}, false},
		{"key_with_field_separator", "db_pass", "prod/db#password", KeyRef{"db_pass", "prod/db", "password"}, false},
		{"nested_path_with_field", "db_pass", "secret/data/api-testing/db#password", KeyRef{"db_pass", "secret/data/api-testing/db", "password"}, false},
		{"no_hash_returns_empty_field", "key", "simple/path", KeyRef{"key", "simple/path", ""}, false},
		{"multiple_hashes_splits_on_first", "key", "path/with#hash#double", KeyRef{"key", "path/with", "hash#double"}, false},
		{"empty_path_before_hash", "key", "#field", KeyRef{}, true},
		{"empty_field_after_hash", "key", "path#", KeyRef{}, true},
		{"empty_var_name", "", "some/path", KeyRef{}, true},
		{"empty_raw_string", "key", "", KeyRef{}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseKeyRef(tc.varName, tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, ErrInvalidKeyFormat) {
					t.Fatalf("expected ErrInvalidKeyFormat, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSecretsConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  SecretsConfig
		wantErr error
	}{
		{"valid_aws_config", SecretsConfig{Provider: ProviderAWS, Region: "us-east-1", Keys: map[string]string{"k": "v"}}, nil},
		{"valid_azure_config", SecretsConfig{Provider: ProviderAzure, VaultName: "my-vault", Keys: map[string]string{"k": "v"}}, nil},
		{"valid_hashicorp_approle", SecretsConfig{Provider: ProviderHashiCorp, Address: "https://vault:8200", Auth: AuthConfig{Method: "approle", RoleID: "r", SecretID: "s"}, Keys: map[string]string{"k": "v"}}, nil},
		{"valid_hashicorp_token", SecretsConfig{Provider: ProviderHashiCorp, Address: "https://vault:8200", Auth: AuthConfig{Method: "token", Token: "t"}, Keys: map[string]string{"k": "v"}}, nil},
		{"valid_gcp_config", SecretsConfig{Provider: ProviderGCP, Project: "my-project", Keys: map[string]string{"k": "v"}}, nil},
		{"valid_1password_config", SecretsConfig{Provider: Provider1Password, Keys: map[string]string{"k": "v"}}, nil},
		{"unknown_provider_returns_error", SecretsConfig{Provider: "unknown", Keys: map[string]string{"k": "v"}}, ErrUnknownProvider},
		{"aws_missing_region", SecretsConfig{Provider: ProviderAWS, Keys: map[string]string{"k": "v"}}, ErrMissingRequiredField},
		{"azure_missing_vault_name", SecretsConfig{Provider: ProviderAzure, Keys: map[string]string{"k": "v"}}, ErrMissingRequiredField},
		{"hashicorp_missing_address", SecretsConfig{Provider: ProviderHashiCorp, Auth: AuthConfig{Method: "token", Token: "t"}, Keys: map[string]string{"k": "v"}}, ErrMissingRequiredField},
		{"hashicorp_approle_missing_role_id", SecretsConfig{Provider: ProviderHashiCorp, Address: "https://vault:8200", Auth: AuthConfig{Method: "approle", SecretID: "s"}, Keys: map[string]string{"k": "v"}}, ErrMissingRequiredField},
		{"gcp_missing_project", SecretsConfig{Provider: ProviderGCP, Keys: map[string]string{"k": "v"}}, ErrMissingRequiredField},
		{"hashicorp_empty_auth_method", SecretsConfig{Provider: ProviderHashiCorp, Address: "https://vault:8200", Auth: AuthConfig{Method: ""}, Keys: map[string]string{"k": "v"}}, ErrMissingRequiredField},
		{"hashicorp_unsupported_auth_method", SecretsConfig{Provider: ProviderHashiCorp, Address: "https://vault:8200", Auth: AuthConfig{Method: "ldap"}, Keys: map[string]string{"k": "v"}}, ErrMissingRequiredField},
		{"empty_keys", SecretsConfig{Provider: ProviderAWS, Region: "us-east-1", Keys: map[string]string{}}, ErrMissingRequiredField},
		{"nil_keys", SecretsConfig{Provider: ProviderAWS, Region: "us-east-1"}, ErrMissingRequiredField},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
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
		})
	}
}

func TestSecretsConfig_ParsedKeys(t *testing.T) {
	tests := []struct {
		name    string
		keys    map[string]string
		want    []KeyRef
		wantErr bool
	}{
		{"all_keys_parsed", map[string]string{"api_key": "prod/api-key"}, []KeyRef{{"api_key", "prod/api-key", ""}}, false},
		{"key_with_field", map[string]string{"db_pass": "prod/db#password"}, []KeyRef{{"db_pass", "prod/db", "password"}}, false},
		{"invalid_key_returns_error", map[string]string{"bad": "#field"}, nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &SecretsConfig{
				Provider: ProviderAWS,
				Region:   "us-east-1",
				Keys:     tc.keys,
			}
			got, err := cfg.ParsedKeys()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d keys, want %d", len(got), len(tc.want))
			}
			// Sort for deterministic comparison
			sort.Slice(got, func(i, j int) bool { return got[i].VarName < got[j].VarName })
			sort.Slice(tc.want, func(i, j int) bool { return tc.want[i].VarName < tc.want[j].VarName })
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("key[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestSecretsConfig_SensitiveNames(t *testing.T) {
	tests := []struct {
		name string
		keys map[string]string
		want []string
	}{
		{"all_key_names_are_sensitive", map[string]string{"api_key": "v", "db_pass": "v2"}, []string{"api_key", "db_pass"}},
		{"empty_keys_returns_empty_set", map[string]string{}, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &SecretsConfig{Keys: tc.keys}
			ss := cfg.SensitiveNames()
			if tc.want == nil {
				if ss == nil {
					return
				}
				names := ss.Names()
				if len(names) != 0 {
					t.Errorf("expected nil/empty set, got %v", names)
				}
				return
			}
			if ss == nil {
				t.Fatal("expected non-nil SensitiveSet")
			}
			for _, name := range tc.want {
				if !ss.IsSensitive(name) {
					t.Errorf("expected %q to be sensitive", name)
				}
			}
			names := ss.Names()
			if len(names) != len(tc.want) {
				t.Errorf("got %d names, want %d", len(names), len(tc.want))
			}
		})
	}
}
