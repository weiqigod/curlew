# Implementation Plan: M2-003

## Overview
Replace the gated vault stub with a proper `vault` subcommand supporting `vault list` that displays configured vault provider profiles at Solo tier, with both terminal and JSON output formats.

## Task Details
- **ID:** M2-003
- **Title:** Vault CLI subcommand (gated placeholder)
- **Phase:** M2: Vault Commands
- **Priority:** 3
- **Complexity:** low

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-002 | Vault provider profile configuration and parsing | done |
| M1-028 | Feature gate framework | done |

## Implementation Steps

### Step 1: Add JSON output types for vault list
**Rationale:** Smallest blast radius — adds new types to the output package with no changes to existing code. Establishes the data contract used by later steps.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/json.go` | modify | Add `VaultListJSONOutput`, `VaultKeyJSON`, `WriteVaultListJSON` |
| `internal/output/json_test.go` | modify | Add tests for vault list JSON serialisation |

#### New Code
```go
// VaultListJSONOutput is the JSON structure for vault list --format json.
type VaultListJSONOutput struct {
	Provider string         `json:"provider"`
	Keys     []VaultKeyJSON `json:"keys"`
}

// VaultKeyJSON represents a single vault key mapping.
type VaultKeyJSON struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// WriteVaultListJSON serialises vault list output as indented JSON.
func WriteVaultListJSON(w io.Writer, out *VaultListJSONOutput) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
```

#### Tests to Write FIRST (RED phase)
```go
func TestWriteVaultListJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    *VaultListJSONOutput
		wantKey  string
	}{
		{"valid JSON output", &VaultListJSONOutput{Provider: "aws", Keys: []VaultKeyJSON{{Name: "k", Path: "p"}}}, `"provider"`},
		{"all fields present", &VaultListJSONOutput{Provider: "azure", Keys: []VaultKeyJSON{{Name: "a", Path: "b"}}}, `"keys"`},
		{"empty keys is empty array", &VaultListJSONOutput{Provider: "aws", Keys: []VaultKeyJSON{}}, `"keys": []`},
		{"keys include name and path", &VaultListJSONOutput{Provider: "aws", Keys: []VaultKeyJSON{{Name: "api_key", Path: "prod/key"}}}, `"name": "api_key"`},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected — new types only

### Step 2: Implement `parseVaultArgs` argument parser
**Rationale:** Second-smallest blast radius — a pure function with no side effects, fully unit-testable in isolation. Needed by `vaultCmd` in Step 3.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add `parseVaultArgs` function |
| `cmd/apitest/main_test.go` | modify | Add table-driven tests for arg parsing |

#### New Code
```go
func parseVaultArgs(args []string) (subcommand, format string, noColor bool, remaining []string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			i++
			if i >= len(args) {
				return "", "", false, nil, fmt.Errorf("--format requires a value (e.g. --format json)")
			}
			format = args[i]
		case "--no-color":
			noColor = true
		case "list":
			if subcommand != "" {
				return "", "", false, nil, fmt.Errorf("unexpected argument: %s", args[i])
			}
			subcommand = "list"
		default:
			return "", "", false, nil, fmt.Errorf("unknown argument: %s", args[i])
		}
	}
	return subcommand, format, noColor, remaining, nil
}
```

#### Tests to Write FIRST (RED phase)
```go
func TestParseVaultArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantSub    string
		wantFmt    string
		wantNoClr  bool
		wantErr    bool
	}{
		{"empty args", nil, "", "", false, false},
		{"list subcommand", []string{"list"}, "list", "", false, false},
		{"format json", []string{"--format", "json"}, "", "json", false, false},
		{"format missing value", []string{"--format"}, "", "", false, true},
		{"no color flag", []string{"--no-color"}, "", "", true, false},
		{"list with format json", []string{"list", "--format", "json"}, "list", "json", false, false},
		{"unknown arg", []string{"--bogus"}, "", "", false, true},
		{"format before list", []string{"--format", "json", "list"}, "list", "json", false, false},
		{"format after list", []string{"list", "--format", "json"}, "list", "json", false, false},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected — new function

### Step 3: Add `currentTier` variable for testability
**Rationale:** Required before implementing `vaultCmd` so that Solo-tier tests can override the tier. Small change, enables all subsequent testing.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add package-level `currentTier` variable |

#### New Code
```go
// currentTier returns the current user's product tier.
// Hardcoded to TierFree until auth backend is implemented.
// Package-level variable to allow test overrides.
var currentTier = func() auth.Tier { return auth.TierFree }
```

#### Impact on Existing Tests
- No existing tests affected — new variable, unused until Step 4

### Step 4: Implement `vaultCmd`, `vaultListCmd`, `printVaultHelp`
**Rationale:** Core implementation step. Depends on Steps 1-3. Replaces the gated stub with full subcommand routing.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add `vaultCmd`, `vaultListCmd`, `printVaultHelp`; update `run()` switch |
| `cmd/apitest/main_test.go` | modify | Add Free-tier and Solo-tier tests |

#### Current Code
```go
case "vault":
    return gatedCmd("vault_provider_profiles", args[1:])
```

#### New Code
```go
case "vault":
    return vaultCmd(args[1:])
```

`vaultCmd` checks the feature gate via `currentTier()`, returns exit 6 if gated (preserving existing behavior), otherwise dispatches to `vaultListCmd`.

`vaultListCmd` loads `config.LoadProjectConfig(wd)`, reads `cfg.Secrets`, and outputs:
- Terminal: provider name, key count, key-name-to-path mapping (sorted)
- JSON: `VaultListJSONOutput` via `WriteVaultListJSON`
- No profiles: helpful message or empty JSON structure

`printVaultHelp` shows available vault subcommands.

#### Tests to Write FIRST (RED phase)
```go
// Free tier tests (validate existing behavior preserved)
func TestVaultCmd_free_tier(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantStr  string
	}{
		{"vault returns exit code 6", []string{"vault"}, 6, ""},
		{"vault includes gate message", []string{"vault"}, 6, "solo"},
		{"vault json exit code 6", []string{"vault", "--format", "json"}, 6, "feature_gated"},
		{"vault json includes upgrade_url", []string{"vault", "--format", "json"}, 6, "upgrade_url"},
		{"vault json includes workaround", []string{"vault", "--format", "json"}, 6, "workaround"},
		{"vault format missing value exits 1", []string{"vault", "--format"}, 1, ""},
		{"vault unknown arg exits 1", []string{"vault", "--bogus"}, 1, ""},
		{"vault list gated at free tier", []string{"vault", "list"}, 6, "solo"},
		{"vault list json gated at free tier", []string{"vault", "list", "--format", "json"}, 6, "feature_gated"},
	}
}

// Solo tier tests
func TestVaultCmd_solo_tier(t *testing.T) {
	// Override currentTier, create temp dir with apitest.yaml containing secrets block
	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantStr  string
	}{
		{"list shows provider", []string{"vault", "list"}, 0, "aws-secrets-manager"},
		{"list shows key count", []string{"vault", "list"}, 0, "2 configured"},
		{"list shows key names", []string{"vault", "list"}, 0, "api_key"},
		{"list json is valid json", []string{"vault", "list", "--format", "json"}, 0, "{"},
		{"list json includes provider", []string{"vault", "list", "--format", "json"}, 0, "aws-secrets-manager"},
		{"list json includes keys array", []string{"vault", "list", "--format", "json"}, 0, "keys"},
		{"no subcommand shows help", []string{"vault"}, 0, "vault list"},
	}
}

func TestVaultCmd_solo_tier_no_profiles(t *testing.T) {
	// Override currentTier, create temp dir with apitest.yaml WITHOUT secrets block
	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantStr  string
	}{
		{"list no profiles shows message", []string{"vault", "list"}, 0, "No vault profiles configured"},
		{"list json no profiles empty output", []string{"vault", "list", "--format", "json"}, 0, "keys"},
	}
}
```

#### Impact on Existing Tests
- `TestGatedCmd_vault` — will continue to pass (same Free-tier behavior via `vaultCmd`)
- `TestGatedCmd_vault_json` — will continue to pass
- `TestGatedCmd_vault_format_missing_value` — will continue to pass
- `TestGatedCmd_vault_unknown_arg` — will continue to pass
- `TestHelpText_vault` — will continue to pass (help still mentions "vault")
- Existing tests may be renamed for clarity (from `TestGatedCmd_vault*` to `TestVaultCmd_*`) if they become redundant with new tests

### Step 5: Update help text
**Rationale:** Last step — user-facing text update after implementation is complete and tested.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Update `printHelp()` vault line to show subcommands |

#### Current Code
```
  vault           Manage vault provider profiles (requires Solo tier)
```

#### New Code
```
  vault           Manage vault provider profiles (requires Solo tier)
  vault list      List configured vault provider profiles
```

#### Impact on Existing Tests
- `TestHelpText_vault` may need assertion update if checking exact text

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `cmd/apitest/main_test.go` | `TestGatedCmd_vault` | preserved | may rename to `TestVaultCmd_free_tier` |
| `cmd/apitest/main_test.go` | `TestGatedCmd_vault_json` | preserved | may rename |
| `cmd/apitest/main_test.go` | `TestGatedCmd_vault_format_missing_value` | preserved | may rename |
| `cmd/apitest/main_test.go` | `TestGatedCmd_vault_unknown_arg` | preserved | may rename |
| `cmd/apitest/main_test.go` | `TestHelpText_vault` | preserved | verify assertion still matches |
| `internal/output/json_test.go` | — | new tests | add `TestWriteVaultListJSON` |

## Risks and Edge Cases

- **Risk: `os.Getwd()` / `os.Chdir()` in tests** -> **Mitigation:** Run Solo-tier tests sequentially (no `t.Parallel()`), restore working directory in `t.Cleanup`
- **Risk: Map iteration order for `Secrets.Keys`** -> **Mitigation:** Sort keys before display in both terminal and JSON output
- **Edge case: No `apitest.yaml` found** -> **Handling:** `LoadProjectConfig` returns empty config with nil `Secrets`; `vaultListCmd` shows "No vault profiles configured"
- **Edge case: Invalid `apitest.yaml`** -> **Handling:** `LoadProjectConfig` returns error; `vaultListCmd` shows error and returns exit 1
- **Edge case: `vault` with no subcommand at Solo tier** -> **Handling:** Show vault help with available subcommands, exit 0
- **Edge case: `vault list --format json` at Free tier** -> **Handling:** Gate fires before subcommand dispatch; returns JSON gate error, exit 6

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Free tier (default, always active)
./apitest vault
echo "Exit: $?"  # expect 6

./apitest vault --format json
echo "Exit: $?"  # expect 6, JSON gate output

# Solo tier — requires test override or future auth backend
go test -run TestVaultCmd_solo_tier ./cmd/apitest/
```
