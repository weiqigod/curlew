# Implementation Plan: M1-028

## Overview
Create a feature gate framework in `internal/auth/` that checks features against product tiers, outputs structured gate messages (terminal and JSON), and exits with code 6 when a gated feature is accessed without the required tier.

## Task Details
- **ID:** M1-028
- **Title:** Feature gate framework
- **Phase:** M1: Core CLI
- **Priority:** 28
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-020 | JSON output format | done |

## Implementation Steps

### Step 1: Define tier types and ordering
**Rationale:** Pure types with zero dependencies on the rest of the codebase. Smallest possible blast radius — everything else builds on this.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/tier.go` | create | Tier type, constants, ordering logic |
| `internal/auth/tier_test.go` | create | Table-driven tier comparison tests |

#### New Code
```go
// tier.go
package auth

// Tier represents a product tier level.
type Tier string

const (
    TierFree         Tier = "free"
    TierSolo         Tier = "solo"
    TierProfessional Tier = "professional"
    TierTeam         Tier = "team"
    TierEnterprise   Tier = "enterprise"
)

// tierRank maps each tier to its numeric rank for comparison.
var tierRank = map[Tier]int{
    TierFree:         0,
    TierSolo:         1,
    TierProfessional: 2,
    TierTeam:         3,
    TierEnterprise:   4,
}

// Includes reports whether t has access to features requiring the given tier.
func (t Tier) Includes(required Tier) bool {
    return tierRank[t] >= tierRank[required]
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestTier_Includes(t *testing.T) {
    tests := []struct {
        name     string
        current  Tier
        required Tier
        want     bool
    }{
        {"free includes free", TierFree, TierFree, true},
        {"free excludes solo", TierFree, TierSolo, false},
        {"solo includes solo", TierSolo, TierSolo, true},
        {"solo includes free", TierSolo, TierFree, true},
        {"solo excludes professional", TierSolo, TierProfessional, false},
        {"professional includes solo", TierProfessional, TierSolo, true},
        {"enterprise includes all", TierEnterprise, TierProfessional, true},
        {"enterprise includes enterprise", TierEnterprise, TierEnterprise, true},
        {"free excludes enterprise", TierFree, TierEnterprise, false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := tt.current.Includes(tt.required)
            if got != tt.want {
                t.Errorf("Tier(%q).Includes(%q) = %v, want %v", tt.current, tt.required, got, tt.want)
            }
        })
    }
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 2: Feature registry
**Rationale:** Builds on Step 1 tier types. Still fully isolated from the rest of the codebase. The registry satisfies the "configurable, not hardcoded" behavior — feature-to-tier mappings are data, not if-statements.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | create | Feature registry with register/lookup |
| `internal/auth/registry_test.go` | create | Registry tests |

#### New Code
```go
// registry.go
package auth

// FeatureDefinition describes a gated feature.
type FeatureDefinition struct {
    Name         string
    RequiredTier Tier
    Description  string // human-readable, e.g. "Vault profiles require Solo tier"
    Workaround   string // optional free-tier alternative
}

// Registry holds registered feature definitions.
type Registry struct {
    features map[string]FeatureDefinition
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
    return &Registry{features: make(map[string]FeatureDefinition)}
}

// Register adds or replaces a feature definition.
func (r *Registry) Register(def FeatureDefinition) {
    r.features[def.Name] = def
}

// Lookup returns the definition for a feature, or false if unregistered.
func (r *Registry) Lookup(feature string) (FeatureDefinition, bool) {
    def, ok := r.features[feature]
    return def, ok
}

// DefaultRegistry returns the built-in feature registry.
func DefaultRegistry() *Registry {
    r := NewRegistry()
    r.Register(FeatureDefinition{
        Name:         "vault_provider_profiles",
        RequiredTier: TierSolo,
        Description:  "Vault provider profiles require Solo tier",
        Workaround:   "Use --env-var to inject secrets from environment variables",
    })
    // additional features registered here as they are implemented
    return r
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRegistry(t *testing.T) {
    tests := []struct {
        name       string
        setup      func(*Registry)
        lookup     string
        wantFound  bool
        wantTier   Tier
    }{
        {"register and lookup", func(r *Registry) {
            r.Register(FeatureDefinition{Name: "feat", RequiredTier: TierSolo})
        }, "feat", true, TierSolo},
        {"lookup unknown returns false", func(r *Registry) {}, "unknown", false, ""},
        {"override existing feature", func(r *Registry) {
            r.Register(FeatureDefinition{Name: "feat", RequiredTier: TierSolo})
            r.Register(FeatureDefinition{Name: "feat", RequiredTier: TierProfessional})
        }, "feat", true, TierProfessional},
    }
    // ...
}

func TestDefaultRegistry(t *testing.T) {
    tests := []struct {
        name    string
        feature string
        wantOK  bool
    }{
        {"contains vault_provider_profiles", "vault_provider_profiles", true},
        {"unknown feature not registered", "nonexistent", false},
    }
    // ...
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 3: Gate checking and GateError
**Rationale:** Builds on Steps 1-2. Contains the core gate-checking logic and the error type used by output formatting and CLI wiring.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/gate.go` | create | GateResult, GateError, CheckFeature |
| `internal/auth/gate_test.go` | create | Gate checking tests |

#### New Code
```go
// gate.go
package auth

const (
    UpgradeURL  = "https://apitesttool.com/upgrade"
    RegisterURL = "https://apitesttool.com/register"
)

// GateResult holds the details of a triggered feature gate.
type GateResult struct {
    Feature        string `json:"feature"`
    RequiredTier   Tier   `json:"required_tier"`
    CurrentTier    Tier   `json:"current_tier"`
    Message        string `json:"message"`
    UpgradeURL     string `json:"upgrade_url"`
    TrialAvailable bool   `json:"trial_available"`
    RegisterURL    string `json:"register_for_trial,omitempty"`
    Workaround     string `json:"workaround,omitempty"`
}

// GateError wraps GateResult as an error.
type GateError struct {
    Result GateResult
}

func (e *GateError) Error() string {
    return e.Result.Message
}

// CheckFeature checks if the given feature is available at the current tier.
// Returns nil if allowed or if the feature is not registered (unknown = ungated).
// Returns *GateError if the current tier does not include the required tier.
func CheckFeature(registry *Registry, feature string, currentTier Tier) error {
    def, ok := registry.Lookup(feature)
    if !ok {
        return nil // unregistered features are ungated
    }
    if currentTier.Includes(def.RequiredTier) {
        return nil
    }
    return &GateError{
        Result: GateResult{
            Feature:        def.Name,
            RequiredTier:   def.RequiredTier,
            CurrentTier:    currentTier,
            Message:        def.Description,
            UpgradeURL:     UpgradeURL,
            TrialAvailable: true,
            RegisterURL:    RegisterURL,
            Workaround:     def.Workaround,
        },
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestCheckFeature(t *testing.T) {
    tests := []struct {
        name        string
        feature     string
        currentTier Tier
        wantErr     bool
    }{
        {"gated feature returns GateError", "vault_provider_profiles", TierFree, true},
        {"allowed feature returns nil", "vault_provider_profiles", TierSolo, false},
        {"free tier cannot access solo feature", "vault_provider_profiles", TierFree, true},
        {"higher tier can access lower feature", "vault_provider_profiles", TierEnterprise, false},
        {"unknown feature returns nil", "nonexistent", TierFree, false},
    }
    // ...
}

func TestGateError_fields(t *testing.T) {
    // Verify GateResult contains feature name, required tier, trial_available, register URL
}

func TestGateError_implements_error(t *testing.T) {
    // var _ error = (*GateError)(nil)
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 4: Terminal gate output
**Rationale:** Now that the auth types exist, add rendering. Terminal output is the primary user-facing format. Adding a method to the existing `Printer` is a narrow change.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify | Add FeatureGate method to Printer |
| `internal/output/terminal_test.go` | modify | Add terminal gate output tests |

#### Current Code
```go
// terminal.go — end of file (line 185)
// ResponseBodyDump writes full response body (shown at -vv only).
func (p *Printer) ResponseBodyDump(body []byte) {
    // ...
}
```

#### New Code (append to terminal.go)
```go
// FeatureGate writes a user-friendly feature gate message.
func (p *Printer) FeatureGate(result *auth.GateResult) {
    _, _ = fmt.Fprintf(p.w, "\n%s\n", colorize("✗ Feature requires upgrade", ansiRed, p.color))
    _, _ = fmt.Fprintf(p.w, "\n  %s\n", result.Message)
    _, _ = fmt.Fprintf(p.w, "\n  Your current tier: %s\n", result.CurrentTier)
    _, _ = fmt.Fprintf(p.w, "  Required tier:     %s\n", result.RequiredTier)
    if result.TrialAvailable {
        _, _ = fmt.Fprintf(p.w, "\n  Register for a free trial:\n")
        _, _ = fmt.Fprintf(p.w, "  %s\n", colorize(result.RegisterURL, ansiCyan, p.color))
    }
    _, _ = fmt.Fprintf(p.w, "\n  Upgrade:\n")
    _, _ = fmt.Fprintf(p.w, "  %s\n", colorize(result.UpgradeURL, ansiCyan, p.color))
    if result.Workaround != "" {
        _, _ = fmt.Fprintf(p.w, "\n  %s %s\n", colorize("Workaround:", ansiBold, p.color), result.Workaround)
    }
    _, _ = fmt.Fprintln(p.w)
}
```

Note: This adds an import of `github.com/weiqigod/curlew/internal/auth` to the `output` package. Since `auth` has no dependencies on `output`, there is no cycle.

#### Tests to Write FIRST (RED phase)

```go
func TestPrinter_FeatureGate(t *testing.T) {
    tests := []struct {
        name       string
        result     *auth.GateResult
        color      bool
        wantSubstr []string
        wantAbsent []string
    }{
        {"includes feature message", result, false, []string{"Vault provider profiles require Solo tier"}, nil},
        {"includes current tier", result, false, []string{"free"}, nil},
        {"includes required tier", result, false, []string{"solo"}, nil},
        {"includes register URL when trial available", resultWithTrial, false, []string{"apitesttool.com/register"}, nil},
        {"includes upgrade URL", result, false, []string{"apitesttool.com/upgrade"}, nil},
        {"includes workaround when present", resultWithWorkaround, false, []string{"Workaround:", "Use --env-var"}, nil},
        {"omits workaround when empty", resultNoWorkaround, false, nil, []string{"Workaround:"}},
        {"no color mode has no ANSI codes", result, false, nil, []string{"\033["}},
        {"color mode has ANSI codes", result, true, []string{"\033["}, nil},
    }
    // ...
}
```

#### Impact on Existing Tests
- No existing tests break — this is a new method on `Printer`, no existing signatures change

---

### Step 5: JSON gate output
**Rationale:** Parallel to Step 4 for the JSON format path. Required for behavior 3 (`--format json` with gated feature).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/json.go` | modify | Add GateJSONOutput struct and WriteGateJSON |
| `internal/output/json_test.go` | modify | Add JSON gate output tests |

#### Current Code
```go
// json.go — end of file (line 96)
func WriteJSON(w io.Writer, out *JSONOutput) error {
    // ...
}
```

#### New Code (append to json.go)
```go
// GateJSONOutput is the JSON structure for feature gate responses.
type GateJSONOutput struct {
    Status           string `json:"status"`
    ExitCode         int    `json:"exit_code"`
    Feature          string `json:"feature"`
    RequiredTier     string `json:"required_tier"`
    CurrentTier      string `json:"current_tier"`
    Message          string `json:"message"`
    UpgradeURL       string `json:"upgrade_url"`
    TrialAvailable   bool   `json:"trial_available"`
    RegisterForTrial string `json:"register_for_trial,omitempty"`
    Workaround       string `json:"workaround,omitempty"`
}

// WriteGateJSON serializes a feature gate output as indented JSON.
func WriteGateJSON(w io.Writer, out *GateJSONOutput) error {
    enc := json.NewEncoder(w)
    enc.SetIndent("", "  ")
    return enc.Encode(out)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestWriteGateJSON(t *testing.T) {
    tests := []struct {
        name       string
        output     *GateJSONOutput
        wantSubstr []string
        wantAbsent []string
    }{
        {"serializes all fields", fullOutput, []string{"feature_gated", "vault_provider_profiles", "solo", "free"}, nil},
        {"status is feature_gated", fullOutput, []string{`"status": "feature_gated"`}, nil},
        {"exit code is 6", fullOutput, []string{`"exit_code": 6`}, nil},
        {"omits empty workaround", outputNoWorkaround, nil, []string{"workaround"}},
        {"includes workaround when present", fullOutput, []string{`"workaround"`}, nil},
        {"valid JSON", fullOutput, nil, nil}, // parse with json.Unmarshal
    }
    // ...
}
```

#### Impact on Existing Tests
- No existing tests break — new struct and function, no existing signatures change

---

### Step 6: Wire into CLI — vault command, exit code 6, help text
**Rationale:** This is the integration step that makes the gate observable. The `vault` command is a thin shim that calls `CheckFeature` and routes to the appropriate output formatter. Exit code 6 is returned. Help text is updated to list the vault command.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add vault case, gatedCmd helper, help text |
| `cmd/curlew/main_test.go` | modify | Add gate integration tests |

#### Current Code
```go
// main.go line 54-59
case "schema":
    return schemaCmd(args[1:])
default:
    _, _ = fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
    printHelp()
    return 1
```

#### New Code
```go
case "schema":
    return schemaCmd(args[1:])
case "vault":
    return gatedCmd("vault_provider_profiles", args[1:])
default:
    _, _ = fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
    printHelp()
    return 1
```

New helper function:
```go
// gatedCmd handles commands that are behind a feature gate.
// It checks the feature against the default registry at the current tier (free),
// and outputs the gate message in the requested format.
func gatedCmd(feature string, args []string) int {
    format, noColor := parseGatedArgs(args)
    registry := auth.DefaultRegistry()
    err := auth.CheckFeature(registry, feature, auth.TierFree)
    if err == nil {
        return 0 // feature is available (shouldn't happen for gated commands at free tier)
    }
    var gateErr *auth.GateError
    if !errors.As(err, &gateErr) {
        _, _ = fmt.Fprintf(os.Stderr, "unexpected error: %v\n", err)
        return 1
    }
    if format == "json" {
        out := &output.GateJSONOutput{
            Status:           "feature_gated",
            ExitCode:         6,
            Feature:          gateErr.Result.Feature,
            RequiredTier:     string(gateErr.Result.RequiredTier),
            CurrentTier:      string(gateErr.Result.CurrentTier),
            Message:          gateErr.Result.Message,
            UpgradeURL:       gateErr.Result.UpgradeURL,
            TrialAvailable:   gateErr.Result.TrialAvailable,
            RegisterForTrial: gateErr.Result.RegisterURL,
            Workaround:       gateErr.Result.Workaround,
        }
        _ = output.WriteGateJSON(os.Stdout, out)
    } else {
        printer := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, noColor))
        printer.FeatureGate(&gateErr.Result)
    }
    return 6
}

// parseGatedArgs extracts --format and --no-color from args for gated commands.
func parseGatedArgs(args []string) (format string, noColor bool) {
    for i := 0; i < len(args); i++ {
        switch args[i] {
        case "--format":
            i++
            if i < len(args) {
                format = args[i]
            }
        case "--no-color":
            noColor = true
        }
    }
    return format, noColor
}
```

Help text addition (in `printHelp()`):
```go
fmt.Println("  vault         Manage vault provider profiles (requires Solo tier)")
```

#### Tests to Write FIRST (RED phase)

```go
func TestGatedCmd_vault(t *testing.T) {
    tests := []struct {
        name       string
        args       []string
        wantExit   int
        wantSubstr []string
    }{
        {"vault returns exit code 6", []string{"vault"}, 6, nil},
        {"vault terminal message includes feature name", []string{"vault"}, 6, []string{"vault_provider_profiles"}},
        {"vault terminal message includes tier", []string{"vault"}, 6, []string{"solo"}},
        {"vault terminal message includes register URL", []string{"vault"}, 6, []string{"apitesttool.com/register"}},
        {"vault terminal message includes upgrade URL", []string{"vault"}, 6, []string{"apitesttool.com/upgrade"}},
    }
    // ...
}

func TestGatedCmd_vault_json(t *testing.T) {
    tests := []struct {
        name       string
        args       []string
        wantExit   int
        wantSubstr []string
    }{
        {"vault json exit code 6", []string{"vault", "--format", "json"}, 6, nil},
        {"vault json status is feature_gated", []string{"vault", "--format", "json"}, 6, []string{`"status": "feature_gated"`}},
        {"vault json includes exit_code field", []string{"vault", "--format", "json"}, 6, []string{`"exit_code": 6`}},
        {"vault json is valid JSON", []string{"vault", "--format", "json"}, 6, nil},
    }
    // ...
}

func TestHelpText_vault(t *testing.T) {
    // Verify help output contains "vault"
}
```

#### Impact on Existing Tests
- No existing tests break — new case in switch, existing cases unchanged
- `TestUnknownCommand` still works (vault is no longer unknown)

---

### Step 7: Smoke test update
**Rationale:** The smoke test exercises the binary end-to-end. Must verify the gate fires and returns exit code 6 with structured output.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add vault gate smoke tests |

#### New Code (append before final line)
```bash
echo "=== Feature Gate ==="

echo "--- Vault command returns exit code 6 ---"
./curlew vault > /dev/null 2>&1 && EXITCODE=0 || EXITCODE=$?
[ "$EXITCODE" = "6" ] && echo "PASS: vault exit code 6" || { echo "FAIL: expected exit 6, got $EXITCODE"; exit 1; }
echo

echo "--- Vault gate message contains feature name ---"
VAULT_OUTPUT=$(./curlew vault --no-color 2>&1 || true)
echo "$VAULT_OUTPUT" | grep -q "vault_provider_profiles" && echo "PASS: feature name in output" || { echo "FAIL: Missing feature name in: $VAULT_OUTPUT"; exit 1; }
echo

echo "--- Vault --format json produces valid JSON ---"
VAULT_JSON=$(./curlew vault --format json 2>&1 || true)
echo "$VAULT_JSON" | python3 -m json.tool > /dev/null && echo "PASS: vault --format json is valid JSON" || { echo "FAIL: Invalid JSON: $VAULT_JSON"; exit 1; }
echo "$VAULT_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['status']=='feature_gated', f'Wrong status: {d[\"status\"]}'" && echo "PASS: status is feature_gated" || { echo "FAIL: Wrong status"; exit 1; }
echo

echo "--- Help text shows vault ---"
HELP_OUT=$(./curlew --help 2>&1)
echo "$HELP_OUT" | grep -q "vault" && echo "PASS: vault in help" || { echo "FAIL: Missing vault in help output"; exit 1; }
echo
```

#### Impact on Existing Tests
- No existing smoke tests affected

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/auth/tier_test.go` | new | new file | create |
| `internal/auth/registry_test.go` | new | new file | create |
| `internal/auth/gate_test.go` | new | new file | create |
| `internal/output/terminal_test.go` | `TestPrinter_FeatureGate` | new tests | add |
| `internal/output/json_test.go` | `TestWriteGateJSON` | new tests | add |
| `cmd/curlew/main_test.go` | `TestGatedCmd_*` | new tests | add |
| All existing tests | — | none | — |

## Risks and Edge Cases

- **Risk:** Multiple gates firing simultaneously → **Mitigation:** Each gated command maps to a single feature. `CheckFeature` is called once per command entry. Future parse-time gates check features one at a time; first error short-circuits.
- **Risk:** Import cycle output → auth → output → **Mitigation:** `auth` has no imports from other internal packages. `output` imports `auth` (one-directional). No cycle.
- **Edge case:** Unknown/unregistered feature in `CheckFeature` → **Handling:** Returns nil (ungated). Unknown features default to accessible, not blocked.
- **Edge case:** Gate interaction with `--dry-run` → **Handling:** Not applicable — gated commands fire before any execution. The vault command is entirely a gate check.
- **Edge case:** Gate interaction with `validate` command → **Handling:** Validate checks YAML syntax, not feature access. Gates do not fire during validation.
- **Risk:** Thread safety for parallel execution → **Mitigation:** `Registry` is read-only after initialization. `CheckFeature` is a pure function (lookup + compare). No shared mutable state.
- **Edge case:** Tier string casing → **Handling:** Tier constants are lowercase strings. All comparisons use the Tier type, not raw strings.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Terminal output
./curlew vault
# Expected: structured gate message with feature name, tier, URLs, exit code 6

# JSON output
./curlew vault --format json
# Expected: JSON with status "feature_gated", exit_code 6, all gate fields

# Exit code
./curlew vault; echo "Exit: $?"
# Expected: Exit: 6
```
