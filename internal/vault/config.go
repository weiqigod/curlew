package vault

import (
	"errors"
	"fmt"
	"strings"

	"github.com/weiqigod/curlew/internal/variable"
)

const (
	// ProviderAWS is the AWS Secrets Manager provider.
	ProviderAWS = "aws-secrets-manager"
	// ProviderAzure is the Azure Key Vault provider.
	ProviderAzure = "azure-key-vault"
	// ProviderHashiCorp is the HashiCorp Vault provider.
	ProviderHashiCorp = "hashicorp-vault"
	// ProviderGCP is the GCP Secret Manager provider.
	ProviderGCP = "gcp-secret-manager"
	// Provider1Password is the 1Password provider.
	Provider1Password = "1password"
)

// SupportedProviders lists all recognized vault provider names.
var SupportedProviders = []string{
	ProviderAWS, ProviderAzure, ProviderHashiCorp, ProviderGCP, Provider1Password,
}

var (
	// ErrUnknownProvider is returned when a provider name is not recognized.
	ErrUnknownProvider = errors.New("unknown vault provider")
	// ErrMissingRequiredField is returned when a required config field is empty.
	ErrMissingRequiredField = errors.New("missing required vault config field")
	// ErrInvalidKeyFormat is returned when a vault key reference cannot be parsed.
	ErrInvalidKeyFormat = errors.New("invalid vault key format")
)

// SecretsConfig holds the parsed configuration for a vault provider profile.
type SecretsConfig struct {
	Provider         string
	Region           string
	VaultName        string
	Address          string
	Auth             AuthConfig
	Project          string
	Keys             map[string]string // variable_name -> vault_path (may contain #field)
	CacheTTL         int
	RefreshOnFailure bool
}

// AuthConfig holds authentication details for vault providers.
type AuthConfig struct {
	Method   string
	Token    string
	RoleID   string
	SecretID string
	Role     string
}

// KeyRef is a parsed vault key reference.
type KeyRef struct {
	VarName string
	Path    string
	Field   string // empty if no # separator
}

// ParseKeyRef parses a raw vault path (e.g. "prod/db#password") into a KeyRef.
// The path is split on the first '#'; everything before is the path, after is the field.
func ParseKeyRef(varName, raw string) (KeyRef, error) {
	if varName == "" {
		return KeyRef{}, fmt.Errorf("%w: empty variable name", ErrInvalidKeyFormat)
	}
	if raw == "" {
		return KeyRef{}, fmt.Errorf("%w: empty key path for %q", ErrInvalidKeyFormat, varName)
	}

	idx := strings.Index(raw, "#")
	if idx < 0 {
		return KeyRef{VarName: varName, Path: raw}, nil
	}

	path := raw[:idx]
	field := raw[idx+1:]

	if path == "" {
		return KeyRef{}, fmt.Errorf("%w: empty path before '#' in %q", ErrInvalidKeyFormat, raw)
	}
	if field == "" {
		return KeyRef{}, fmt.Errorf("%w: empty field after '#' in %q", ErrInvalidKeyFormat, raw)
	}

	return KeyRef{VarName: varName, Path: path, Field: field}, nil
}

// Validate checks that the config has all required fields for its provider.
func (c *SecretsConfig) Validate() error {
	if len(c.Keys) == 0 {
		return fmt.Errorf("%w: keys", ErrMissingRequiredField)
	}

	switch c.Provider {
	case ProviderAWS:
		if c.Region == "" {
			return fmt.Errorf("%w: region (required for %s)", ErrMissingRequiredField, c.Provider)
		}
	case ProviderAzure:
		if c.VaultName == "" {
			return fmt.Errorf("%w: vault_name (required for %s)", ErrMissingRequiredField, c.Provider)
		}
	case ProviderHashiCorp:
		if c.Address == "" {
			return fmt.Errorf("%w: address (required for %s)", ErrMissingRequiredField, c.Provider)
		}
		switch c.Auth.Method {
		case "approle":
			if c.Auth.RoleID == "" {
				return fmt.Errorf("%w: auth.role_id (required for approle auth)", ErrMissingRequiredField)
			}
			if c.Auth.SecretID == "" {
				return fmt.Errorf("%w: auth.secret_id (required for approle auth)", ErrMissingRequiredField)
			}
		case "token":
			if c.Auth.Token == "" {
				return fmt.Errorf("%w: auth.token (required for token auth)", ErrMissingRequiredField)
			}
		case "":
			return fmt.Errorf("%w: auth.method (required for %s)", ErrMissingRequiredField, c.Provider)
		default:
			return fmt.Errorf("%w: auth.method %q (supported: token, approle)", ErrMissingRequiredField, c.Auth.Method)
		}
	case ProviderGCP:
		if c.Project == "" {
			return fmt.Errorf("%w: project (required for %s)", ErrMissingRequiredField, c.Provider)
		}
	case Provider1Password:
		// No additional required fields beyond keys.
	default:
		return fmt.Errorf("%w: %q (supported: %s)", ErrUnknownProvider, c.Provider, strings.Join(SupportedProviders, ", "))
	}

	return nil
}

// ParsedKeys returns all keys parsed into KeyRef structs.
func (c *SecretsConfig) ParsedKeys() ([]KeyRef, error) {
	refs := make([]KeyRef, 0, len(c.Keys))
	for name, raw := range c.Keys {
		ref, err := ParseKeyRef(name, raw)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// SensitiveNames returns a SensitiveSet containing all key variable names.
// All vault-sourced variables are automatically treated as sensitive.
func (c *SecretsConfig) SensitiveNames() *variable.SensitiveSet {
	ss := variable.NewSensitiveSet()
	for name := range c.Keys {
		ss.Add(name)
	}
	return ss
}
