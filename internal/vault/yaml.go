package vault

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type secretsFile struct {
	Provider         string            `yaml:"provider"`
	Region           string            `yaml:"region,omitempty"`
	VaultName        string            `yaml:"vault_name,omitempty"`
	Address          string            `yaml:"address,omitempty"`
	Auth             *authFile         `yaml:"auth,omitempty"`
	Project          string            `yaml:"project,omitempty"`
	Keys             map[string]string `yaml:"keys"`
	CacheTTL         int               `yaml:"cache_ttl,omitempty"`
	RefreshOnFailure bool              `yaml:"refresh_on_failure,omitempty"`
}

type authFile struct {
	Method   string `yaml:"method"`
	Token    string `yaml:"token,omitempty"`
	RoleID   string `yaml:"role_id,omitempty"`
	SecretID string `yaml:"secret_id,omitempty"`
	Role     string `yaml:"role,omitempty"`
}

// ParseSecretsYAML parses a raw YAML mapping node into a validated SecretsConfig.
func ParseSecretsYAML(node *yaml.Node) (*SecretsConfig, error) {
	if node == nil {
		return nil, fmt.Errorf("decoding secrets config: nil YAML node")
	}
	var sf secretsFile
	if err := node.Decode(&sf); err != nil {
		return nil, fmt.Errorf("decoding secrets config: %w", err)
	}

	cfg := &SecretsConfig{
		Provider:         sf.Provider,
		Region:           sf.Region,
		VaultName:        sf.VaultName,
		Address:          sf.Address,
		Project:          sf.Project,
		Keys:             sf.Keys,
		CacheTTL:         sf.CacheTTL,
		RefreshOnFailure: sf.RefreshOnFailure,
	}

	if sf.Auth != nil {
		cfg.Auth = AuthConfig{
			Method:   sf.Auth.Method,
			Token:    sf.Auth.Token,
			RoleID:   sf.Auth.RoleID,
			SecretID: sf.Auth.SecretID,
			Role:     sf.Auth.Role,
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
