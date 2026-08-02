package teamtemplate

import (
	"fmt"
	"sort"

	"github.com/peterlindqvist/apitest/internal/vault"
)

// validate walks all environments and accumulates validation issues.
// All checks run — it never short-circuits on first error.
func validate(t *TeamTemplate) []Issue {
	var issues []Issue

	for i := range t.Environments {
		env := &t.Environments[i]
		base := "team_secrets.vault_configs." + env.Name

		// Fold in any issues found during parsing (e.g. duplicate aliases).
		issues = append(issues, env.parseIssues...)

		// 1. Empty provider.
		if env.Provider == "" {
			issues = append(issues, Issue{
				Path:    base + ".provider",
				Message: "missing required field 'provider'",
				Line:    env.Line,
			})
			continue // Can't validate provider-specific fields without a provider.
		}

		// 2. Unknown provider (teamtemplate only accepts AWS + Azure in this slice).
		if !supportedProviders[env.Provider] {
			issues = append(issues, Issue{
				Path:    base + ".provider",
				Message: fmt.Sprintf("unknown provider '%s'", env.Provider),
				Line:    env.Line,
			})
		}

		// 3. Provider-specific required fields.
		switch env.Provider {
		case vault.ProviderAWS:
			if env.Region == "" {
				issues = append(issues, Issue{
					Path:    base + ".region",
					Message: fmt.Sprintf("missing required field 'region' for provider %s", env.Provider),
					Line:    env.Line,
				})
			}
		case vault.ProviderAzure:
			if env.VaultName == "" {
				issues = append(issues, Issue{
					Path:    base + ".vault_name",
					Message: fmt.Sprintf("missing required field 'vault_name' for provider %s", env.Provider),
					Line:    env.Line,
				})
			}
		}

		// 4. Keys block must be present and non-empty.
		if len(env.Keys) == 0 {
			issues = append(issues, Issue{
				Path:    base + ".keys",
				Message: "missing required field 'keys'",
				Line:    env.Line,
			})
			continue
		}

		// 5. Validate each key reference (sorted for deterministic output).
		keysBase := base + ".keys"
		aliases := make([]string, 0, len(env.Keys))
		for alias := range env.Keys {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		for _, alias := range aliases {
			raw := env.Keys[alias]
			if _, err := vault.ParseKeyRef(alias, raw); err != nil {
				issues = append(issues, Issue{
					Path:    keysBase + "." + alias,
					Message: fmt.Sprintf("invalid key reference for '%s': %v", alias, err),
				})
			}
		}
	}

	return issues
}
