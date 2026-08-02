# Implementation Plan: M2-001

## Overview
Add `from_command` variable source that resolves variables by executing shell commands, with caching, sensitivity support, and Solo-tier feature gating.

## Task Details
- **ID:** M2-001
- **Title:** from_command variable source
- **Phase:** M2: Vault Commands
- **Priority:** 1
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-009 | Collection variables | done |
| M1-014 | Request vars & env var precedence | done |
| M1-028 | Feature gate framework | done |

## Implementation Steps

### Step 1: Register `from_command` as Solo-tier feature gate

**Rationale:** Pure addition to `DefaultRegistry()`. Zero risk to existing behavior. Smallest blast radius.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | Add `from_command` registration |
| `internal/auth/registry_test.go` | modify | Add test case for new feature |

#### Current Code
```go
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(FeatureDefinition{
		Name:         "vault_provider_profiles",
		RequiredTier: TierSolo,
		Description:  "Vault provider profiles require Solo tier",
		Workaround:   "Use --env-var to inject secrets from environment variables",
	})
	return r
}
```

#### New Code
```go
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(FeatureDefinition{
		Name:         "vault_provider_profiles",
		RequiredTier: TierSolo,
		Description:  "Vault provider profiles require Solo tier",
		Workaround:   "Use --env-var to inject secrets from environment variables",
	})
	r.Register(FeatureDefinition{
		Name:         "from_command",
		RequiredTier: TierSolo,
		Description:  "from_command requires Solo tier ($9/month)",
		Workaround:   "Use inline credentials or --env-var for basic secret injection",
	})
	return r
}
```

#### Tests to Write FIRST (RED phase)

```go
// Add to existing TestDefaultRegistry table cases:
{"contains from_command", "from_command", true},
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 2: Extend parser to support object-form variables

**Rationale:** Must come before runner integration. Parser needs to produce structured data distinguishing simple values from `from_command` sources.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `CommandVar` type, extend `SensitiveVars` with `Commands` map, update `UnmarshalYAML` to handle mapping nodes |
| `internal/parser/parser_test.go` | modify | Add test cases for object-form parsing |

#### New Types
```go
// CommandVar represents a variable sourced from a shell command.
type CommandVar struct {
	Command   string // shell command to execute
	Sensitive bool   // whether value should be redacted
	Cache     int    // TTL in seconds; 0 = no caching
}
```

#### Extended SensitiveVars
```go
type SensitiveVars struct {
	Values    map[string]string
	Sensitive *variable.SensitiveSet
	Commands  map[string]CommandVar // from_command entries
}
```

#### UnmarshalYAML Changes

Handle three node kinds for value nodes:
- `yaml.ScalarNode` — current behavior (string value, check `!sensitive` tag)
- `yaml.MappingNode` — object form: look for `value`, `from_command`, `sensitive`, `cache` keys
  - If `from_command` key present: store in `Commands` map, mark sensitive if flagged
  - If `value` key present (without `from_command`): store in `Values` map, mark sensitive if flagged
  - Error if both `value` and `from_command` present
  - Error if `from_command` is empty string

#### Tests to Write FIRST (RED phase)

```go
func TestSensitiveVarsObjectForm(t *testing.T) {
	tests := []struct {
		name           string
		yaml           string
		wantValues     map[string]string
		wantCommands   map[string]CommandVar
		wantSensitive  []string
		wantErr        bool
	}{
		{"from_command_simple", ...},
		{"from_command_with_sensitive_true", ...},
		{"from_command_with_cache", ...},
		{"from_command_all_fields", ...},
		{"object_form_value_only", ...},
		{"mixed_scalar_and_from_command", ...},
		{"error_value_and_from_command_mutually_exclusive", ...},
		{"error_empty_from_command", ...},
		{"backward_compat_scalar_values", ...},
		{"backward_compat_sensitive_tag", ...},
	}
}
```

#### Impact on Existing Tests
- Existing scalar-only variable parsing tests must continue to pass unchanged
- The `SensitiveVars` struct gains a new `Commands` field; existing tests that construct `SensitiveVars` manually need `Commands` initialized (but only if they access it)

---

### Step 3: Implement command executor

**Rationale:** Isolated new file, fully testable in isolation before wiring into runner.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/command.go` | create | Shell command execution with stdout capture, error handling |
| `internal/variable/command_test.go` | create | Table-driven tests for execution and caching |

#### New Code

```go
package variable

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	ErrCommandFailed   = errors.New("from_command execution failed")
	ErrCommandTimedOut = errors.New("from_command execution timed out")
)

type CommandCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	value   string
	expires time.Time
}

func NewCommandCache() *CommandCache

func (c *CommandCache) Get(key string) (string, bool)
func (c *CommandCache) Set(key, value string, ttlSeconds int)

// ExecuteCommand runs a shell command via /bin/sh -c and returns stdout
// (trailing newline trimmed). Returns ErrCommandFailed on non-zero exit
// with command, exit code, and stderr in the error message.
func ExecuteCommand(ctx context.Context, command string) (string, error)
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecuteCommand(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		wantOut   string
		wantErr   error
	}{
		{"simple_echo_returns_stdout", "echo hello", "hello", nil},
		{"trailing_newline_stripped", "echo hello", "hello", nil},
		{"pipe_syntax_works", "echo hello | tr a-z A-Z", "HELLO", nil},
		{"non_zero_exit_returns_error", "exit 1", "", ErrCommandFailed},
		{"error_includes_command_and_exit_code", "exit 42", "", ErrCommandFailed},
		{"error_includes_stderr", "echo err >&2; exit 1", "", ErrCommandFailed},
		{"empty_stdout_returns_empty_string", "true", "", nil},
		{"multiline_stdout_preserved", "printf 'a\nb'", "a\nb", nil},
	}
}

func TestCommandCache(t *testing.T) {
	tests := []struct {
		name string
		// ...
	}{
		{"cache_hit_within_ttl", ...},
		{"cache_miss_empty", ...},
		{"cache_expired_after_ttl", ...},
		{"cache_zero_ttl_no_caching", ...},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected (new file)

---

### Step 4: Integrate from_command into runner variable resolution

**Rationale:** Depends on Steps 2 and 3. Wires from_command into the precedence chain at level 5.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `Tier`/`Registry` to `VarSources`, resolve from_command at precedence 5, feature gate check |
| `internal/runner/runner_test.go` | modify | Add test cases for from_command integration |

#### VarSources Changes
```go
type VarSources struct {
	Project  map[string]string
	EnvFile  map[string]string
	DotEnv   map[string]string
	EnvVar   map[string]string
	CLI      map[string]string
	Seed     *int64
	Tier     auth.Tier      // current user tier; "" defaults to TierFree
	Registry *auth.Registry // nil = use DefaultRegistry()
}
```

#### Run() Changes

After merging project(2)+env(3)+dotenv(4) and BEFORE adding collection vars(7), insert from_command resolution at precedence 5:

```go
// Precedence 5: from_command variables (Solo tier)
if len(col.Variables.Commands) > 0 {
	reg := vars.Registry
	if reg == nil {
		reg = auth.DefaultRegistry()
	}
	tier := vars.Tier
	if tier == "" {
		tier = auth.TierFree
	}
	if err := auth.CheckFeature(reg, "from_command", tier); err != nil {
		return nil, summary, err
	}

	cache := variable.NewCommandCache()
	for name, cmd := range col.Variables.Commands {
		// Higher precedence sources skip command execution
		if _, ok := vars.CLI[name]; ok {
			continue
		}
		if _, ok := vars.EnvVar[name]; ok {
			continue
		}

		if cmd.Cache > 0 {
			if val, ok := cache.Get(name); ok {
				merged[name] = val
				continue
			}
		}

		val, err := variable.ExecuteCommand(ctx, cmd.Command)
		if err != nil {
			return nil, summary, err
		}

		if cmd.Cache > 0 {
			cache.Set(name, val, cmd.Cache)
		}
		merged[name] = val
	}
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRunFromCommand(t *testing.T) {
	tests := []struct {
		name    string
		// ...
	}{
		{"from_command_resolves_at_solo_tier", ...},
		{"from_command_at_free_tier_returns_gate_error", ...},
		{"from_command_cli_override_skips_execution", ...},
		{"from_command_env_var_override_skips_execution", ...},
		{"from_command_collection_var_overrides_command", ...},
		{"from_command_error_propagates_to_caller", ...},
		{"from_command_no_commands_at_free_tier_ok", ...},
		{"from_command_used_in_request_url", ...},
	}
}
```

#### Impact on Existing Tests
- `VarSources{}` defaults `Tier` to `""` and `Registry` to `nil` — `Run()` must treat these as `TierFree` and `DefaultRegistry()` respectively
- **Critical:** Since `DefaultRegistry()` now includes `from_command`, existing tests with no from_command vars must NOT trigger gate checks — the `len(col.Variables.Commands) > 0` guard ensures this

---

### Step 5: Wire feature gate and sensitivity into CLI

**Rationale:** Last step — connects everything to exit codes and output formatting.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Pass `Tier` to `VarSources`, handle `*GateError` for exit 6, build sensitive set for from_command vars |

#### Changes

1. Pass tier to `VarSources`:
```go
results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{
	// ... existing fields ...
	Tier: auth.TierFree, // hardcoded until auth backend
})
```

2. Handle `*GateError` from `runner.Run()` — return exit code 6

3. Build sensitive set for from_command vars:
```go
for name, cmd := range col.Variables.Commands {
	if cmd.Sensitive {
		sensitive.Add(name)
	}
}
```

#### Tests to Write FIRST (RED phase)

Integration tests in `cmd/curlew/`:
```go
{"from_command_gate_returns_exit_6", ...},
{"from_command_sensitive_redacted_in_terminal", ...},
```

#### Impact on Existing Tests
- Existing CLI tests unaffected — `Tier: auth.TierFree` with no from_command vars triggers no gate check

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/auth/registry_test.go` | `TestDefaultRegistry` | extend | add from_command test case |
| `internal/parser/parser_test.go` | existing tests | none | verify backward compat |
| `internal/parser/parser_test.go` | new tests | add | object-form variable parsing |
| `internal/variable/command_test.go` | all | new file | create from scratch |
| `internal/runner/runner_test.go` | existing tests | minor | may need Tier field on VarSources |
| `internal/runner/runner_test.go` | new tests | add | from_command integration |
| `cmd/curlew/` | existing tests | none | no change expected |

## Risks and Edge Cases

- **Risk:** Shell portability — `/bin/sh` varies across systems → **Mitigation:** Use only POSIX shell features. Document that from_command uses `/bin/sh -c`.

- **Risk:** Command output with embedded newlines → **Mitigation:** Trim only trailing newline. Entire stdout becomes value. Matches `$(command)` shell substitution behavior.

- **Risk:** `SensitiveVars.UnmarshalYAML` backward compatibility → **Mitigation:** Only interpret mapping nodes with recognized keys (`value`, `from_command`, `sensitive`, `cache`). Current behavior for accidental mappings already produces empty strings.

- **Risk:** `VarSources` zero-value safety → **Mitigation:** `Run()` treats empty `Tier` as `TierFree` and nil `Registry` as `DefaultRegistry()`. All existing tests pass without modification.

- **Risk:** Precedence correctness → **Mitigation:** Insert from_command (level 5) between dotenv (4) and collection (7) in the merge sequence. Higher-precedence `for k, v := range` assignments overwrite lower ones.

- **Edge case:** Empty `from_command` string → **Handling:** Validate during parsing, return error.
- **Edge case:** CLI --var overrides from_command var → **Handling:** Skip command execution entirely (precedence 10 > 5).
- **Edge case:** Cache key collisions → **Handling:** Variable names are unique per collection by definition.
- **Edge case:** Long-running commands → **Handling:** `exec.CommandContext` sends SIGKILL on context cancellation.
- **Edge case:** Both `value` and `from_command` specified → **Handling:** Parse error — mutually exclusive.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create test collection with from_command variable
cat > /tmp/m2001-test.yaml << 'EOF'
name: from_command test
variables:
  secret:
    from_command: "echo secret123"
requests:
  - name: test request
    url: "https://httpbin.org/get?key={{secret}}"
    method: GET
    assert:
      - type: status
        value: 200
EOF

# Run at Solo tier — variable resolves to "secret123"
curlew run /tmp/m2001-test.yaml

# Run at Free tier — exit code 6 with feature gate message
curlew run /tmp/m2001-test.yaml
echo $?  # should be 6

# Run variable tests
go test ./internal/variable/... -v
```
