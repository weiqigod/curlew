# Implementation Plan: M2-002

## Overview
Create the `internal/vault/` package with configuration types for vault provider profiles, parse the `secrets:` block from `curlew.yaml`, validate all five supported providers, support structured extraction syntax (`key#field`), and auto-mark all vault variables as sensitive. Feature-gate vault at Solo tier.

## Task Details
- **ID:** M2-002
- **Title:** Vault provider profile configuration and parsing
- **Phase:** M2: Vault Commands
- **Priority:** 2
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-001 | from_command variable source | done |
| M1-017 | global project config | done |

## Implementation Steps

### Step 1: Create `internal/vault/config.go` — Configuration Types and Validation

**Rationale:** Smallest blast radius — introduces a new package with no effect on existing code. Establishes the domain types that all subsequent steps depend on.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/config.go` | create | Provider constants, config types, KeyRef, validation |
| `internal/vault/config_test.go` | create | Table-driven tests for ParseKeyRef, Validate, ParsedKeys, SensitiveNames |

#### New Code

```go
package vault

import (
    "errors"
    "fmt"
    "strings"

    "github.com/weiqigod/curlew/internal/variable"
)

const (
    ProviderAWS       = "aws-secrets-manager"
    ProviderAzure     = "azure-key-vault"
    ProviderHashiCorp = "hashicorp-vault"
    ProviderGCP       = "gcp-secret-manager"
    Provider1Password = "1password"
)

var SupportedProviders = []string{
    ProviderAWS, ProviderAzure, ProviderHashiCorp, ProviderGCP, Provider1Password,
}

var (
    ErrUnknownProvider      = errors.New("unknown vault provider")
    ErrMissingRequiredField = errors.New("missing required vault config field")
    ErrInvalidKeyFormat     = errors.New("invalid vault key format")
)

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

type AuthConfig struct {
    Method   string
    Token    string
    RoleID   string
    SecretID string
    Role     string
}

type KeyRef struct {
    VarName string
    Path    string
    Field   string // empty if no # separator
}

func ParseKeyRef(varName, raw string) (KeyRef, error) { ... }
func (c *SecretsConfig) Validate() error { ... }
func (c *SecretsConfig) ParsedKeys() ([]KeyRef, error) { ... }
func (c *SecretsConfig) SensitiveNames() *variable.SensitiveSet { ... }
```

#### Tests to Write FIRST (RED phase)

```go
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
        {"empty_raw_string", "key", "", KeyRef{}, true},
    }
    // ...
}

func TestSecretsConfig_Validate(t *testing.T) {
    tests := []struct {
        name    string
        config  SecretsConfig
        wantErr error
    }{
        {"valid_aws_config", SecretsConfig{Provider: "aws-secrets-manager", Region: "us-east-1", Keys: map[string]string{"k": "v"}}, nil},
        {"valid_azure_config", SecretsConfig{Provider: "azure-key-vault", VaultName: "my-vault", Keys: map[string]string{"k": "v"}}, nil},
        {"valid_hashicorp_approle", SecretsConfig{Provider: "hashicorp-vault", Address: "https://vault:8200", Auth: AuthConfig{Method: "approle", RoleID: "r", SecretID: "s"}, Keys: map[string]string{"k": "v"}}, nil},
        {"valid_hashicorp_token", SecretsConfig{Provider: "hashicorp-vault", Address: "https://vault:8200", Auth: AuthConfig{Method: "token", Token: "t"}, Keys: map[string]string{"k": "v"}}, nil},
        {"valid_gcp_config", SecretsConfig{Provider: "gcp-secret-manager", Project: "my-project", Keys: map[string]string{"k": "v"}}, nil},
        {"valid_1password_config", SecretsConfig{Provider: "1password", Keys: map[string]string{"k": "v"}}, nil},
        {"unknown_provider_returns_error", SecretsConfig{Provider: "unknown", Keys: map[string]string{"k": "v"}}, ErrUnknownProvider},
        {"aws_missing_region", SecretsConfig{Provider: "aws-secrets-manager", Keys: map[string]string{"k": "v"}}, ErrMissingRequiredField},
        {"azure_missing_vault_name", SecretsConfig{Provider: "azure-key-vault", Keys: map[string]string{"k": "v"}}, ErrMissingRequiredField},
        {"hashicorp_missing_address", SecretsConfig{Provider: "hashicorp-vault", Auth: AuthConfig{Method: "token", Token: "t"}, Keys: map[string]string{"k": "v"}}, ErrMissingRequiredField},
        {"hashicorp_approle_missing_role_id", SecretsConfig{Provider: "hashicorp-vault", Address: "https://vault:8200", Auth: AuthConfig{Method: "approle", SecretID: "s"}, Keys: map[string]string{"k": "v"}}, ErrMissingRequiredField},
        {"empty_keys", SecretsConfig{Provider: "aws-secrets-manager", Region: "us-east-1", Keys: map[string]string{}}, ErrMissingRequiredField},
        {"nil_keys", SecretsConfig{Provider: "aws-secrets-manager", Region: "us-east-1"}, ErrMissingRequiredField},
    }
    // ...
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
    // ...
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
    // ...
}
```

#### Impact on Existing Tests
- No existing tests affected (new package)

---

### Step 2: Create `internal/vault/yaml.go` — YAML Unmarshaling

**Rationale:** Depends on Step 1 types. Still isolated to the new package.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/yaml.go` | create | YAML deserialization from `*yaml.Node` to `SecretsConfig` |
| `internal/vault/yaml_test.go` | create | Round-trip tests with YAML strings |

#### New Code

```go
package vault

import "gopkg.in/yaml.v3"

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

// ParseSecretsYAML parses a raw YAML node into a validated SecretsConfig.
func ParseSecretsYAML(node *yaml.Node) (*SecretsConfig, error) { ... }
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseSecretsYAML(t *testing.T) {
    tests := []struct {
        name    string
        yaml    string
        want    *SecretsConfig
        wantErr bool
    }{
        {"aws_full_config", `provider: aws-secrets-manager\nregion: us-east-1\nkeys:\n  api_key: prod/api-key\ncache_ttl: 300`, ...},
        {"azure_with_keys", `provider: azure-key-vault\nvault_name: my-vault\nkeys:\n  db: prod-db`, ...},
        {"hashicorp_approle", `provider: hashicorp-vault\naddress: https://vault:8200\nauth:\n  method: approle\n  role_id: r\n  secret_id: s\nkeys:\n  k: v`, ...},
        {"gcp_with_project", `provider: gcp-secret-manager\nproject: my-proj\nkeys:\n  k: v`, ...},
        {"1password_minimal", `provider: 1password\nkeys:\n  k: v`, ...},
        {"unknown_provider_error", `provider: unknown\nkeys:\n  k: v`, nil, true},
        {"missing_provider_error", `keys:\n  k: v`, nil, true},
        {"empty_keys_error", `provider: aws-secrets-manager\nregion: us-east-1\nkeys: {}`, nil, true},
    }
    // ...
}
```

#### Impact on Existing Tests
- No existing tests affected (new package)

---

### Step 3: Extend `internal/config/project.go` — Add `secrets:` to Project Config

**Rationale:** Depends on Steps 1-2. Touches existing code but only adds a new field — zero risk to existing behavior.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/config/project.go` | modify | Add `Secrets` field to `projectFile` and `ProjectConfig`, parse via vault package |
| `internal/config/project_test.go` | modify | Add test cases for secrets parsing |

#### Current Code

```go
type projectFile struct {
	ProjectName string         `yaml:"project_name"`
	Variables   map[string]any `yaml:"variables"`
}

type ProjectConfig struct {
	ProjectName string
	Variables   map[string]string
}
```

#### New Code

```go
type projectFile struct {
	ProjectName string         `yaml:"project_name"`
	Variables   map[string]any `yaml:"variables"`
	Secrets     *yaml.Node     `yaml:"secrets,omitempty"`
}

type ProjectConfig struct {
	ProjectName string
	Variables   map[string]string
	Secrets     *vault.SecretsConfig
}
```

In `ParseProjectConfig`, after unmarshaling, if `pf.Secrets != nil`:
```go
secrets, err := vault.ParseSecretsYAML(pf.Secrets)
if err != nil {
    return nil, fmt.Errorf("parsing secrets config: %w", err)
}
cfg.Secrets = secrets
```

#### Tests to Write FIRST (RED phase)

```go
// Add to existing TestParseProjectConfig:
{"valid_with_secrets_aws", "project_name: test\nsecrets:\n  provider: aws-secrets-manager\n  region: us-east-1\n  keys:\n    api_key: prod/key", ...},
{"valid_with_secrets_and_variables", "project_name: test\nvariables:\n  base_url: http://localhost\nsecrets:\n  provider: 1password\n  keys:\n    k: v", ...},
{"unknown_secrets_provider_returns_error", "project_name: test\nsecrets:\n  provider: unknown\n  keys:\n    k: v", nil, true},
{"no_secrets_block_returns_nil", "project_name: test", ...}, // cfg.Secrets == nil
```

#### Impact on Existing Tests
- No existing tests affected — existing test YAML doesn't contain `secrets:`, so `cfg.Secrets` will be nil

---

### Step 4: Add Feature Gate Check in Runner

**Rationale:** Depends on Step 3. Follows the established `from_command` gate check pattern exactly.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `Secrets` to `VarSources`, add gate check |
| `internal/runner/runner_test.go` | modify | Add gate check test cases |

#### Current Code (runner.go VarSources)

```go
type VarSources struct {
    Project  map[string]string
    EnvFile  map[string]string
    DotEnv   map[string]string
    EnvVar   map[string]string
    CLI      map[string]string
    Seed     int64
    Tier     string
    Registry *auth.Registry
}
```

#### New Code

```go
type VarSources struct {
    Project  map[string]string
    EnvFile  map[string]string
    DotEnv   map[string]string
    EnvVar   map[string]string
    CLI      map[string]string
    Seed     int64
    Tier     string
    Registry *auth.Registry
    Secrets  *vault.SecretsConfig
}
```

In `Run`, after the `from_command` gate check block (~line 106), add:

```go
// Check vault feature gate
if vars.Secrets != nil {
    reg := vars.Registry
    if reg == nil {
        reg = auth.DefaultRegistry()
    }
    tier := vars.Tier
    if tier == "" {
        tier = auth.TierFree
    }
    if err := auth.CheckFeature(reg, "vault_provider_profiles", tier); err != nil {
        return nil, summary, err
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_VaultGate(t *testing.T) {
    tests := []struct {
        name      string
        secrets   *vault.SecretsConfig
        tier      string
        wantGate  bool
    }{
        {"vault_at_free_tier_returns_gate_error", &vault.SecretsConfig{...}, auth.TierFree, true},
        {"vault_at_solo_tier_no_error", &vault.SecretsConfig{...}, auth.TierSolo, false},
        {"no_vault_no_gate_check", nil, auth.TierFree, false},
    }
    // ...
}
```

#### Impact on Existing Tests
- No existing tests affected — `Secrets` field defaults to nil, existing `VarSources` literals don't set it

---

### Step 5: Wire Into `cmd/curlew/main.go`

**Rationale:** Final step — connects all pieces. Depends on all prior steps.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Pass Secrets to runner, merge sensitive names |
| `smoke/run.sh` | modify | Add vault config smoke test |

#### Current Code (main.go runCmd, VarSources construction)

```go
results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{
    Project: projectCfg.Variables,
    EnvFile: envVars,
    DotEnv:  dotenvVars,
    EnvVar:  envVarVars,
    CLI:     cliVars,
    Seed:    seed,
    Tier:    auth.TierFree,
})
```

#### New Code

```go
results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{
    Project: projectCfg.Variables,
    EnvFile: envVars,
    DotEnv:  dotenvVars,
    EnvVar:  envVarVars,
    CLI:     cliVars,
    Seed:    seed,
    Tier:    auth.TierFree,
    Secrets: projectCfg.Secrets,
})
```

And merge sensitive names:

```go
if projectCfg.Secrets != nil {
    sensitive.Merge(projectCfg.Secrets.SensitiveNames())
}
```

#### Impact on Existing Tests
- No existing tests affected

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/config/project_test.go` | `TestParseProjectConfig` | extended | add new test cases for secrets parsing |
| `internal/runner/runner_test.go` | existing tests | none | VarSources gains nil Secrets field |
| all other test files | — | none | — |

## Risks and Edge Cases

- **Risk:** YAML `*yaml.Node` capture for deferred parsing → **Mitigation:** This is the same pattern used by `SensitiveVars.UnmarshalYAML` in the parser package. Test with real YAML strings.

- **Edge case:** Variable interpolation in vault config fields (e.g., `"{{VAULT_ROLE_ID}}"` in `auth.role_id`) → **Handling:** Store raw strings. Interpolation is deferred to secret retrieval (future task M2-004+). Document this in code comments.

- **Edge case:** Multiple `#` in key paths (e.g., `path/with#hash#double`) → **Handling:** Split on first `#` only. `path/with` → path, `hash#double` → field. Tested explicitly.

- **Edge case:** Empty `keys` map → **Handling:** Validation error — a secrets config without keys is meaningless.

- **Risk:** Circular imports (`vault` → `variable`, `config` → `vault`, `runner` → `vault`) → **Mitigation:** No cycles exist: `variable` ← `vault` ← `config` and `variable` ← `vault` ← `runner`. Verified by import tracing.

- **Edge case:** `validate` command and vault config → **Handling:** The validate command validates collection files, not project config. A `secrets:` block in `curlew.yaml` doesn't affect validate's collection validation. The observable "validate without errors" means the vault config doesn't interfere.

- **Risk:** Provider-specific required fields missed → **Mitigation:** Table-driven validation per provider with explicit test cases for each missing required field.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create curlew.yaml with secrets block
cat > /tmp/vault-test/curlew.yaml << 'EOF'
project_name: vault-test
secrets:
  provider: aws-secrets-manager
  region: us-east-1
  keys:
    api_key: prod/api-key
  cache_ttl: 300
EOF

# Validate collection (should parse without errors)
curlew validate /tmp/vault-test/collection.yaml

# Run at Free tier — expect exit code 6
curlew run /tmp/vault-test/collection.yaml; echo "Exit code: $?"

# Unit tests
go test ./internal/vault/...
```
