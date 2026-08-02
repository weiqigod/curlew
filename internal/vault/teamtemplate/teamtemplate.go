// Package teamtemplate parses and validates shared vault configuration
// templates (the team_secrets.vault_configs section from apitest.yaml).
// It defines only the file format; runtime resolution is handled by
// internal/runner via M4-002.
package teamtemplate

import (
	"errors"
	"fmt"

	"github.com/peterlindqvist/apitest/internal/vault"
)

// Sentinel errors — callers may match with errors.Is.
var (
	// ErrInvalidTemplate is returned when the YAML structure is malformed.
	ErrInvalidTemplate = errors.New("invalid team template")
	// ErrUnknownProvider is returned when a vault_configs entry has an unrecognised provider.
	ErrUnknownProvider = errors.New("unknown provider")
	// ErrMissingField is returned when a required field is absent for its provider.
	ErrMissingField = errors.New("missing required field")
	// ErrDuplicateAlias is returned when two key entries share the same alias in one environment.
	ErrDuplicateAlias = errors.New("duplicate key alias")
	// ErrInvalidKeyRef is returned when a vault key reference cannot be parsed.
	ErrInvalidKeyRef = errors.New("invalid key reference")

	// M4-002 runtime sentinels.

	// ErrTemplateNotFound is returned when APITEST_TEAM_CONFIG points to a file
	// that does not exist or cannot be read.
	ErrTemplateNotFound = errors.New("shared vault template not found")
	// ErrUnknownEnvironment is returned when --env names an environment that is
	// not declared in the template.
	ErrUnknownEnvironment = errors.New("unknown environment in shared vault template")
	// ErrUnknownAlias is returned when a collection references an alias that is
	// not declared in the active environment.
	ErrUnknownAlias = errors.New("unknown secret alias")
	// ErrEnvFlagRequired is returned when a collection uses {{secrets.X}} but
	// --env was not provided.
	ErrEnvFlagRequired = errors.New("shared template requires --env")
)

// supportedProviders lists the providers accepted in this slice (M4-001).
// M4-002 may extend this whitelist.
var supportedProviders = map[string]bool{
	vault.ProviderAWS:   true,
	vault.ProviderAzure: true,
}

// TeamTemplate is a parsed shared vault configuration template.
// Environments preserves YAML declaration order for deterministic output.
type TeamTemplate struct {
	Environments []EnvConfig
}

// EnvConfig is a single environment's vault wiring.
type EnvConfig struct {
	Name        string            // map key, e.g. "production"
	Provider    string            // vault.ProviderAWS / vault.ProviderAzure
	Region      string            // aws-secrets-manager only
	VaultName   string            // azure-key-vault only
	Keys        map[string]string // alias -> vault path (may contain "#field")
	Line        int               // YAML line of the env node, for diagnostics
	parseIssues []Issue           // issues found during parsing (e.g. duplicate aliases)
}

// Issue is a single validation finding. Mirrors validator.Issue shape
// so cmd/apitest can render team-template results through the same
// printer without an extra struct.
type Issue struct {
	Path    string // dotted key path, e.g. "team_secrets.vault_configs.production.provider"
	Message string
	Line    int
}

// ResolvedEnv is the structured view M4-002 will consume at runtime.
type ResolvedEnv struct {
	Name      string
	Provider  string
	Region    string
	VaultName string
	Keys      map[string]vault.KeyRef
}

// Parse decodes a YAML byte slice into a TeamTemplate. It does NOT
// validate provider-level rules; callers should call Validate afterwards.
func Parse(data []byte) (*TeamTemplate, error) {
	return parseBytes(data)
}

// Validate walks the template and returns all validation issues
// (empty slice if the template is valid). Validation never short-circuits.
func (t *TeamTemplate) Validate() []Issue {
	return validate(t)
}

// Resolve returns the resolved environment config for the given name,
// or (nil, false) if the environment is not declared. Intended for
// M4-002 runtime consumers.
func (t *TeamTemplate) Resolve(name string) (*ResolvedEnv, bool) {
	for i := range t.Environments {
		env := &t.Environments[i]
		if env.Name != name {
			continue
		}
		keys := make(map[string]vault.KeyRef, len(env.Keys))
		for alias, raw := range env.Keys {
			ref, err := vault.ParseKeyRef(alias, raw)
			if err != nil {
				continue // bad refs are caught by Validate; skip here
			}
			keys[alias] = ref
		}
		return &ResolvedEnv{
			Name:      env.Name,
			Provider:  env.Provider,
			Region:    env.Region,
			VaultName: env.VaultName,
			Keys:      keys,
		}, true
	}
	return nil, false
}

// Merge overlays local onto t per SPECIFICATION.md:5702 ("per-key merge —
// local file's keys replace backend's at the same path"). Local environments
// not present in the base are appended. Local keys missing from the base env
// are added; conflicting keys are overwritten by local. Returns the merged
// template; the receiver and local are not mutated.
//
// Passing nil for local returns a shallow copy of t.
func (t *TeamTemplate) Merge(local *TeamTemplate) *TeamTemplate {
	if local == nil {
		out := &TeamTemplate{
			Environments: make([]EnvConfig, len(t.Environments)),
		}
		copy(out.Environments, t.Environments)
		return out
	}

	// Index base environments for O(1) lookup.
	baseIdx := make(map[string]int, len(t.Environments))
	merged := make([]EnvConfig, len(t.Environments))
	for i, env := range t.Environments {
		// Deep-copy keys map so we don't mutate the base.
		copiedKeys := make(map[string]string, len(env.Keys))
		for k, v := range env.Keys {
			copiedKeys[k] = v
		}
		env.Keys = copiedKeys
		merged[i] = env
		baseIdx[env.Name] = i
	}

	for _, localEnv := range local.Environments {
		if idx, exists := baseIdx[localEnv.Name]; exists {
			// Existing env: merge keys (local wins on collision).
			for k, v := range localEnv.Keys {
				merged[idx].Keys[k] = v
			}
		} else {
			// New environment from local: append a deep copy.
			copiedKeys := make(map[string]string, len(localEnv.Keys))
			for k, v := range localEnv.Keys {
				copiedKeys[k] = v
			}
			env := localEnv
			env.Keys = copiedKeys
			merged = append(merged, env)
		}
	}

	return &TeamTemplate{Environments: merged}
}

// Summary returns the one-line "N environments, M secrets" summary
// used by apitest validate.
func (t *TeamTemplate) Summary() string {
	total := 0
	for _, env := range t.Environments {
		total += len(env.Keys)
	}
	return fmt.Sprintf("%d environments, %d secrets", len(t.Environments), total)
}
