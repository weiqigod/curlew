package teamtemplate

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// envFile holds the provider-level fields for one environment entry.
type envFile struct {
	Provider  string `yaml:"provider"`
	Region    string `yaml:"region,omitempty"`
	VaultName string `yaml:"vault_name,omitempty"`
	// Keys are NOT decoded here; we walk the node directly to detect duplicates.
}

// parseBytes decodes YAML data into a TeamTemplate, preserving source order.
func parseBytes(data []byte) (*TeamTemplate, error) {
	// Decode as a raw document node so we can walk nodes manually.
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidTemplate, err)
	}

	// An empty document returns Kind==0.
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return &TeamTemplate{}, nil
	}

	// The document root is a DocumentNode wrapping the actual mapping.
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w: expected a YAML mapping at document root", ErrInvalidTemplate)
	}

	// Find the "team_secrets" key.
	teamSecretsNode := findMappingValue(root, "team_secrets")
	if teamSecretsNode == nil {
		return &TeamTemplate{}, nil
	}
	if teamSecretsNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w: team_secrets must be a mapping", ErrInvalidTemplate)
	}

	// Find vault_configs inside team_secrets.
	vcNode := findMappingValue(teamSecretsNode, "vault_configs")
	if vcNode == nil || vcNode.Kind == 0 {
		return &TeamTemplate{}, nil
	}
	if vcNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w: team_secrets.vault_configs must be a mapping", ErrInvalidTemplate)
	}

	tpl := &TeamTemplate{
		Environments: make([]EnvConfig, 0, len(vcNode.Content)/2),
	}

	// vcNode.Content is a flat [key, value, key, value, ...] slice.
	for i := 0; i+1 < len(vcNode.Content); i += 2 {
		keyNode := vcNode.Content[i]
		valueNode := vcNode.Content[i+1]

		var ef envFile
		if err := valueNode.Decode(&ef); err != nil {
			return nil, fmt.Errorf("%w: environment %q: %w", ErrInvalidTemplate, keyNode.Value, err)
		}

		// Walk the keys mapping node directly to detect duplicate aliases and
		// build the keys map without relying on Go map decoding (which silently
		// collapses duplicates).
		keys, parseIssues := parseKeysNode(keyNode.Value, valueNode)

		env := EnvConfig{
			Name:        keyNode.Value,
			Provider:    ef.Provider,
			Region:      ef.Region,
			VaultName:   ef.VaultName,
			Keys:        keys,
			Line:        keyNode.Line,
			parseIssues: parseIssues,
		}
		tpl.Environments = append(tpl.Environments, env)
	}

	return tpl, nil
}

// findMappingValue returns the value node for the given key in a mapping node,
// or nil if the key is not found.
func findMappingValue(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// parseKeysNode walks the YAML node for the environment value and extracts its
// keys mapping, detecting duplicate aliases. It returns the decoded map and any
// Issues found during parsing.
func parseKeysNode(envName string, envNode *yaml.Node) (map[string]string, []Issue) {
	if envNode.Kind != yaml.MappingNode {
		return nil, nil
	}

	// Find the "keys" mapping child node.
	keysNode := findMappingValue(envNode, "keys")
	if keysNode == nil || keysNode.Kind != yaml.MappingNode {
		return nil, nil
	}

	// Walk the keys mapping to detect duplicates and build the map.
	seen := make(map[string]bool, len(keysNode.Content)/2)
	result := make(map[string]string, len(keysNode.Content)/2)
	var issues []Issue

	base := "team_secrets.vault_configs." + envName + ".keys"
	for i := 0; i+1 < len(keysNode.Content); i += 2 {
		aliasNode := keysNode.Content[i]
		alias := aliasNode.Value
		value := keysNode.Content[i+1].Value
		if seen[alias] {
			issues = append(issues, Issue{
				Path:    base + "." + alias,
				Message: fmt.Sprintf("duplicate key alias '%s'", alias),
				Line:    aliasNode.Line,
			})
			continue
		}
		seen[alias] = true
		result[alias] = value
	}

	return result, issues
}
