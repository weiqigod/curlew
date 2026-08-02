# Implementation Plan: M4-001

## Overview

Introduce a new `internal/vault/teamtemplate` package that parses and validates shared vault configuration template files (the `team_secrets.vault_configs` YAML shape from `docs/SPECIFICATION.md` Layer 4), and teach `apitest validate` to dispatch to it whenever a file's top-level key is `team_secrets`. This slice defines the file format only — no network resolution (that is M4-002).

## Task Details

- **ID:** M4-001
- **Title:** Shared vault configuration template format
- **Phase:** M4: Team Tier
- **Priority:** 1
- **Complexity:** medium

## Dependencies

None (M4-001 is the root of the M4 DAG).

| Task  | Title | Status |
|-------|-------|--------|
| — | — | — |

## Exploration Notes

### Existing building blocks (reused, not rewritten)

- `internal/vault/config.go` already defines provider constants (`ProviderAWS`, `ProviderAzure`, …), sentinel errors (`ErrUnknownProvider`, `ErrMissingRequiredField`, `ErrInvalidKeyFormat`), `ParseKeyRef`, and `SecretsConfig.Validate` (per-environment provider rules). We will import these; we will **not** duplicate the provider constant strings or the `#field` parsing logic.
- `internal/vault/yaml.go#ParseSecretsYAML` already decodes a single-provider `yaml.Node` into a validated `SecretsConfig`. The team template is just a named map of these, so we can layer on top.
- `internal/validator/validator.go#Validate` is the existing `apitest validate` entry point. It returns a `*Result` holding `Issues`. Team template validation does not fit the collection-oriented `joinSections`/variable-reference flow, so it will live alongside it as a parallel `ValidateTeamTemplate` function that returns the same `*validator.Result` shape (same `Issue` struct, same `Valid` flag).
- `cmd/apitest/main.go#validateCmd` iterates files, calls `validator.Validate`, and prints results. It is the single dispatch point we need to tweak: sniff the top-level YAML key of each file and route to the team-template path when it is `team_secrets`.
- Providers limited to `aws-secrets-manager` (requires `region`) and `azure-key-vault` (requires `vault_name`) per task scope.

### Key design decisions

1. **New package location.** `internal/vault/teamtemplate/` (sub-package of the existing `vault` tree). This keeps the provider constants close, avoids a circular import with `internal/validator`, and aligns with the task observable `go test ./internal/vault/teamtemplate/...`.
2. **Exit code for team-template validation failures.** The task observable specifies exit code **2** for invalid templates, whereas the existing collection validator uses exit 3. This is deliberate — team templates are a distinct artifact class, and exit 2 gives CI systems a way to distinguish "bad team template" from "bad collection". Implementation: `validateCmd` will check whether any failed result originated from a team template and return 2 instead of 3 in that case. We will track this via a new `Result.Kind` field (values `collection`, `team_template`) rather than re-parsing errors.
3. **Detection of team templates.** We read a few bytes of YAML and sniff for a top-level `team_secrets:` key using `yaml.Unmarshal` into a small discriminator struct. This avoids guessing based on file name and keeps `testdata/team/*.yaml` self-describing.
4. **One error per problem.** Each behavior expects *one* error per missing field / bad provider. We will accumulate issues in a slice, but the unit tests will pin the count so a future refactor cannot silently add duplicates.
5. **Deterministic ordering.** Environments are declared in a YAML map (Go map iteration is unordered). To keep error messages deterministic, we walk the decoded YAML via `yaml.Node` (not `map[string]…`) so we preserve source order, and we sort key aliases alphabetically when emitting duplicate-alias errors.
6. **Success summary format.** The observable requires:
   `OK: shared vault template valid (2 environments, 4 secrets)`
   The count of "secrets" equals the total number of key aliases across all environments.
7. **Error key path format.** `team_secrets.vault_configs.<env>.provider` as the task specifies. All error messages from the team-template validator start with that path.
8. **No runtime coupling.** The new package does **not** import `internal/runner`, `internal/variable`, or `internal/config/project.go`. M4-002 will wire the template into the runtime; this slice stays purely file-format.
9. **Compound secret parsing.** We reuse `vault.ParseKeyRef` verbatim so the `prod/db-credentials#password` syntax is handled identically to the existing single-provider flow.

## Implementation Steps

Steps are ordered smallest-blast-radius first: build the leaf package with no external coupling, then integrate the dispatcher in the validator, then wire up the CLI command, then add fixtures and smoke coverage.

### Step 1: Create `internal/vault/teamtemplate` package with types and parser

**Rationale:** Leaf package with zero callers — safe to land first. Every subsequent step depends on the exported types and `Parse`/`Validate` functions.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/teamtemplate/teamtemplate.go` | create | Public types (`TeamTemplate`, `EnvConfig`, `ResolvedEnv`), sentinel errors, and doc comment |
| `internal/vault/teamtemplate/parse.go` | create | YAML decoding from bytes or `io.Reader`, preserving source ordering via `yaml.Node` |
| `internal/vault/teamtemplate/validate.go` | create | Accumulating validator that returns `[]Issue` |
| `internal/vault/teamtemplate/teamtemplate_test.go` | create | Table-driven behavior tests (the 8 tests named in the task DoD) |

#### Proposed types (new code)

```go
// Package teamtemplate parses and validates shared vault configuration
// templates (the team_secrets.vault_configs section from apitest.yaml).
// It defines only the file format; runtime resolution is handled by
// internal/runner via M4-002.
package teamtemplate

import (
    "errors"
    "fmt"
    "io"

    "github.com/peterlindqvist/apitest/internal/vault"
    "gopkg.in/yaml.v3"
)

// Sentinel errors — callers may match with errors.Is.
var (
    ErrInvalidTemplate    = errors.New("invalid team template")
    ErrUnknownProvider    = errors.New("unknown provider")
    ErrMissingField       = errors.New("missing required field")
    ErrDuplicateAlias     = errors.New("duplicate key alias")
    ErrInvalidKeyRef      = errors.New("invalid key reference")
)

// TeamTemplate is a parsed shared vault configuration template.
// Environments preserves YAML declaration order for deterministic output.
type TeamTemplate struct {
    Environments []EnvConfig
}

// EnvConfig is a single environment's vault wiring.
type EnvConfig struct {
    Name      string            // map key, e.g. "production"
    Provider  string            // vault.ProviderAWS / vault.ProviderAzure
    Region    string            // aws-secrets-manager only
    VaultName string            // azure-key-vault only
    Keys      map[string]string // alias -> vault path (may contain "#field")
    Line      int               // YAML line of the env node, for diagnostics
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
    Name     string
    Provider string
    Keys     map[string]vault.KeyRef
}

// Parse decodes a YAML byte slice into a TeamTemplate. It does NOT
// validate; callers should call Validate afterwards.
func Parse(data []byte) (*TeamTemplate, error) { /* ... */ }

// ParseReader is a convenience wrapper around Parse.
func ParseReader(r io.Reader) (*TeamTemplate, error) { /* ... */ }

// Validate walks the template and returns all validation issues
// (empty slice if the template is valid). Validation never short-circuits.
func (t *TeamTemplate) Validate() []Issue { /* ... */ }

// Resolve returns the resolved environment config for the given name,
// or (nil, false) if the environment is not declared. Intended for
// M4-002 runtime consumers.
func (t *TeamTemplate) Resolve(name string) (*ResolvedEnv, bool) { /* ... */ }

// Summary returns the one-line "N environments, M secrets" summary
// used by apitest validate.
func (t *TeamTemplate) Summary() string { /* ... */ }
```

#### Parser implementation strategy

Decode the root document as `yaml.Node`, walk to `team_secrets → vault_configs`, then iterate the mapping node's `Content` in pairs to preserve source order:

```go
type rootNode struct {
    TeamSecrets struct {
        VaultConfigs yaml.Node `yaml:"vault_configs"`
    } `yaml:"team_secrets"`
}
```

For each `(keyNode, valueNode)` pair under `VaultConfigs`, decode the value into:

```go
type envFile struct {
    Provider  string            `yaml:"provider"`
    Region    string            `yaml:"region,omitempty"`
    VaultName string            `yaml:"vault_name,omitempty"`
    Keys      map[string]string `yaml:"keys"`
}
```

...and append an `EnvConfig` to `TeamTemplate.Environments` with `Name = keyNode.Value` and `Line = keyNode.Line`.

If the YAML is structurally invalid, return `fmt.Errorf("%w: %v", ErrInvalidTemplate, err)`.

#### Validator implementation strategy

`Validate()` accumulates issues into a slice and returns it. Checks, per environment, in order:

1. Empty provider → `ErrMissingField` at `team_secrets.vault_configs.<env>.provider`.
2. Provider not in `{vault.ProviderAWS, vault.ProviderAzure}` → `ErrUnknownProvider` at `team_secrets.vault_configs.<env>.provider`, message `unknown provider '<value>'`. (Other providers are out of scope for this slice per the task — even if `internal/vault` knows about them, teamtemplate only accepts AWS + Azure.)
3. `aws-secrets-manager` + empty `region` → `ErrMissingField` at `…<env>.region`.
4. `azure-key-vault` + empty `vault_name` → `ErrMissingField` at `…<env>.vault_name`.
5. Empty or missing `keys` → `ErrMissingField` at `…<env>.keys`.
6. For each alias in `keys`: parse with `vault.ParseKeyRef(alias, raw)` — on error, append `ErrInvalidKeyRef` at `…<env>.keys.<alias>`.
7. Duplicate alias detection happens during parse, not validate, because Go map decoding silently collapses duplicates. Solution: walk the keys node's `Content` pairs directly in the parser (the same way we iterate environments), track seen aliases, and emit `Issue{Path: "team_secrets.vault_configs.<env>.keys.<alias>", Message: "duplicate key alias '<alias>'"}` — stored on the `EnvConfig` struct via a private `parseIssues []Issue` field so `Validate()` can fold them into its output.

#### Tests to Write FIRST (RED phase)

```go
// internal/vault/teamtemplate/teamtemplate_test.go
func TestTeamTemplate(t *testing.T) {
    tests := []struct {
        name       string
        yaml       string
        wantErr    error       // parse-time error (nil for valid templates)
        wantIssues []string    // substrings expected in issue messages
        wantEnvs   int
        wantKeys   int
    }{
        {
            name: "valid_aws_plus_azure",
            yaml: validTemplateYAML, // two envs, four keys total
            wantEnvs: 2, wantKeys: 4,
        },
        {
            name: "unknown_provider_error",
            yaml: `team_secrets:
  vault_configs:
    production:
      provider: foo
      keys: { api_key: prod/api-key }`,
            wantIssues: []string{"team_secrets.vault_configs.production.provider", "unknown provider 'foo'"},
        },
        {
            name: "aws_missing_region",
            yaml: `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      keys: { api_key: prod/api-key }`,
            wantIssues: []string{"team_secrets.vault_configs.production.region"},
        },
        {
            name: "azure_missing_vault_name",
            yaml: `team_secrets:
  vault_configs:
    staging:
      provider: azure-key-vault
      keys: { api_key: staging-api-key }`,
            wantIssues: []string{"team_secrets.vault_configs.staging.vault_name"},
        },
        {
            name: "compound_secret_with_field",
            yaml: `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        db_password: prod/db-credentials#password`,
            wantEnvs: 1, wantKeys: 1,
        },
        {
            name: "duplicate_alias_in_same_env",
            yaml: `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: prod/api-key
        api_key: prod/other-key`,
            wantIssues: []string{"duplicate key alias 'api_key'"},
        },
        {
            name: "env_missing_keys_block",
            yaml: `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1`,
            wantIssues: []string{"team_secrets.vault_configs.production.keys"},
        },
        {
            name: "invalid_key_ref_empty_path_before_hash",
            yaml: `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: "#password"`,
            wantIssues: []string{"team_secrets.vault_configs.production.keys.api_key"},
        },
    }
    // table body: Parse, optionally assert parse error, Validate, assert
    // issue substrings and counts.
}

func TestTeamTemplate_Resolve(t *testing.T) {
    // Load validTemplateYAML, call Resolve("production"), check Provider,
    // Keys["api_key"].Path == "prod/api-key",
    // Keys["db_password"].Path == "prod/db-credentials" && .Field == "password".
}

func TestTeamTemplate_Summary(t *testing.T) {
    // Assert Summary() == "2 environments, 4 secrets" for validTemplateYAML.
}
```

Eight behavior tests (matches task DoD) + two structural tests (`Resolve`, `Summary`).

#### Impact on Existing Tests

None — this is a brand-new package with no callers yet.

---

### Step 2: Extend `internal/validator` with team-template dispatch

**Rationale:** Adds a single new exported function and one new `Result.Kind` field. Does not touch existing collection-validation paths, so existing tests stay green.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/validator/validator.go` | modify | Add `ResultKind` enum, `Result.Kind` field, `ValidateAuto(path)` dispatcher, and `validateTeamTemplate(path)` helper |
| `internal/validator/teamtemplate_test.go` | create | Tests for the dispatcher: detects team templates, returns correct Kind, maps issues to `validator.Issue` |

#### Current Code

```go
// internal/validator/validator.go
type Result struct {
    FilePath string
    Valid    bool
    Issues   []Issue
}
```

#### New Code

```go
// ResultKind identifies what flavor of file was validated.
type ResultKind int

const (
    KindCollection    ResultKind = iota // regular collection file
    KindTeamTemplate                    // shared vault configuration template
)

type Result struct {
    FilePath string
    Valid    bool
    Kind     ResultKind
    Issues   []Issue
    Summary  string // e.g. "2 environments, 4 secrets" for team templates; empty for collections
}

// ValidateAuto inspects the top-level YAML keys of path and dispatches
// to either collection or team-template validation. Files with a top-level
// "team_secrets:" key are treated as team templates.
func ValidateAuto(path string, knownVars map[string]string) *Result {
    if isTeamTemplate, err := sniffTeamTemplate(path); err == nil && isTeamTemplate {
        return validateTeamTemplate(path)
    }
    return Validate(path, knownVars)
}
```

`sniffTeamTemplate` does a minimal `yaml.Unmarshal` into `struct{ TeamSecrets yaml.Node \`yaml:"team_secrets"\` }` and returns true if the node's `Kind` is non-zero. This avoids a full collection parse on team templates (which would fail spuriously because they lack `requests:`).

`validateTeamTemplate` reads the file, calls `teamtemplate.Parse` + `Validate`, translates `teamtemplate.Issue` → `validator.Issue` with `Message = fmt.Sprintf("%s: %s", tplIssue.Path, tplIssue.Message)`, and sets `Result.Kind = KindTeamTemplate` and `Result.Summary = tpl.Summary()` on success.

#### Tests to Write FIRST

```go
// internal/validator/teamtemplate_test.go
func TestValidateAuto_DispatchesByTopLevelKey(t *testing.T) {
    tests := []struct {
        name     string
        content  string
        wantKind validator.ResultKind
        wantValid bool
    }{
        {"collection_file_routes_to_collection", "name: x\nrequests: []\n", validator.KindCollection, true},
        {"team_template_routes_to_team", validTeamTemplateYAML, validator.KindTeamTemplate, true},
        {"invalid_team_template_reports_key_path", invalidProviderYAML, validator.KindTeamTemplate, false},
    }
    // ...
}

func TestValidateTeamTemplate_SummaryPopulated(t *testing.T) {
    // Assert Result.Summary == "2 environments, 4 secrets".
}

func TestValidateTeamTemplate_ErrorMessageIncludesKeyPath(t *testing.T) {
    // Assert issue.Message starts with "team_secrets.vault_configs.production.provider".
}
```

#### Impact on Existing Tests

- No changes required to existing collection validation tests — they continue to call `validator.Validate` directly. Dispatcher is additive.
- `cmd/apitest` currently calls `validator.Validate(f, nil)` (main.go:1542). That call site will switch to `validator.ValidateAuto(f, nil)` in Step 3.

---

### Step 3: Wire the dispatcher into `cmd/apitest validate`

**Rationale:** Smallest possible CLI change — one call-site swap, one exit-code branch, one help-text line. Depends on Steps 1 and 2.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | `validateCmd` uses `ValidateAuto`, returns exit 2 when team templates fail, prints one-line success summary |
| `cmd/apitest/main.go` | modify | `printHelp` adds a line under `validate` mentioning shared vault templates |
| `cmd/apitest/run_test.go` or new `cmd/apitest/validate_team_test.go` | create | Integration test that runs `validateCmd` against the valid + invalid fixtures and asserts exit code, stdout summary, stderr error path |

#### Current Code

```go
// cmd/apitest/main.go:1540-1573
results := make([]*validator.Result, 0, len(files))
for _, f := range files {
    results = append(results, validator.Validate(f, nil))
}
// ...
if !allValid {
    return 3
}
return 0
```

```go
// cmd/apitest/main.go:2353
fmt.Println("  validate <file> Validate collection files without executing requests")
```

#### New Code

```go
results := make([]*validator.Result, 0, len(files))
for _, f := range files {
    results = append(results, validator.ValidateAuto(f, nil))
}
// ...
if !allValid {
    // exit 2 if any failure came from a team template, else 3
    for _, r := range results {
        if !r.Valid && r.Kind == validator.KindTeamTemplate {
            return 2
        }
    }
    return 3
}
return 0
```

Printing path (`printValidationResult`):

```go
if r.Valid && r.Kind == validator.KindTeamTemplate {
    fmt.Fprintf(w, "OK: shared vault template valid (%s)\n", r.Summary)
    return
}
```

Error path for team templates prints each issue's `Message` verbatim (already includes the full key path), prefixed with `error: `.

Help-text:

```go
fmt.Println("  validate <file> Validate collection files without executing requests")
fmt.Println("                  Also validates shared vault configuration templates (team_secrets.vault_configs)")
```

#### Tests to Write FIRST

```go
// cmd/apitest/validate_team_test.go
func TestValidateCmd_TeamTemplate(t *testing.T) {
    t.Run("valid_template_prints_summary_exit_0", func(t *testing.T) {
        // Invoke validateCmd against testdata/team/shared-vault-template.yaml
        // via the same helper the existing tests use (captureStdout or similar).
        // Assert exit 0 and stdout contains "OK: shared vault template valid (2 environments, 4 secrets)".
    })
    t.Run("invalid_template_prints_key_path_exit_2", func(t *testing.T) {
        // Assert exit 2 and stderr contains
        // "team_secrets.vault_configs.production.provider: unknown provider 'foo'".
    })
    t.Run("help_mentions_shared_vault_template", func(t *testing.T) {
        // Capture printHelp output, grep for "shared vault configuration templates".
    })
}
```

#### Impact on Existing Tests

- `TestValidateCmd_*` in `cmd/apitest/main_test.go` (if any): none. They use collection fixtures, which now go through `ValidateAuto → Validate` without behavior changes.
- `smoke/run.sh` existing validate tests: none, same reason.

---

### Step 4: Add fixtures under `testdata/team/`

**Rationale:** Fixtures only — zero code risk. Must land with or before Step 3's integration test.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `testdata/team/shared-vault-template.yaml` | create | Valid template with AWS production (2 keys) and Azure staging (2 keys) — matches spec example |
| `testdata/team/shared-vault-template.invalid.yaml` | create | Production env with `provider: foo` to trigger the expected error |

#### Fixture: `testdata/team/shared-vault-template.yaml`

```yaml
# Shared vault configuration template for the apitest team.
# Validates via: apitest validate testdata/team/shared-vault-template.yaml
team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: prod/api-key
        db_password: prod/db-credentials#password
    staging:
      provider: azure-key-vault
      vault_name: staging-vault
      keys:
        api_key: staging-api-key
        db_password: staging-db-password
```

Expected summary: `2 environments, 4 secrets`.

#### Fixture: `testdata/team/shared-vault-template.invalid.yaml`

```yaml
team_secrets:
  vault_configs:
    production:
      provider: foo
      region: eu-west-1
      keys:
        api_key: prod/api-key
```

Expected error: `error: team_secrets.vault_configs.production.provider: unknown provider 'foo'`, exit 2.

#### Impact on Existing Tests

None.

---

### Step 5: Smoke test coverage

**Rationale:** Required by DoD (`smoke/run.sh validates the new fixture in its CI path`). Smallest possible addition: two new assertions in the existing `=== Validate command ===` section.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add two validate assertions for the valid + invalid team fixtures |

#### New Code (appended after the existing validate block, around line 936)

```bash
echo "--- Validate: shared vault template (valid) ---"
./apitest validate testdata/team/shared-vault-template.yaml \
  && echo "PASS: valid team template exits 0" \
  || { echo "FAIL: expected exit 0"; exit 1; }
echo

echo "--- Validate: shared vault template (invalid provider, expect exit 2) ---"
set +e
./apitest validate testdata/team/shared-vault-template.invalid.yaml
TEAM_RC=$?
set -e
if [ "$TEAM_RC" -eq 2 ]; then
  echo "PASS: invalid team template exits 2"
else
  echo "FAIL: expected exit 2, got $TEAM_RC"
  exit 1
fi
echo
```

#### Impact on Existing Tests

None.

---

### Step 6: CHANGELOG

**Rationale:** Quality gate item in CLAUDE.md — every task-completion commit updates CHANGELOG.md.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add entry under Unreleased/Added describing shared vault template validation |

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/vault/teamtemplate/teamtemplate_test.go` | `TestTeamTemplate` | new | Write 8 table-driven behavior cases |
| `internal/vault/teamtemplate/teamtemplate_test.go` | `TestTeamTemplate_Resolve` | new | Write |
| `internal/vault/teamtemplate/teamtemplate_test.go` | `TestTeamTemplate_Summary` | new | Write |
| `internal/validator/teamtemplate_test.go` | `TestValidateAuto_*` | new | Write 3 cases (dispatch, summary, error path) |
| `cmd/apitest/validate_team_test.go` | `TestValidateCmd_TeamTemplate` | new | Write 3 cases (valid, invalid, help) |
| `internal/vault/*_test.go` | existing | none | Not affected — we only *import* sentinel constants |
| `internal/validator/validator_test.go` | existing | none | Not affected — dispatcher is additive |
| `cmd/apitest/main_test.go` | existing validate tests | none | Collection files still go through `Validate` unchanged |

**Coverage target:** ≥ 80 % for the new `internal/vault/teamtemplate` package — achievable because every branch is exercised by the 8 behavior tests plus the two structural tests.

## Risks and Edge Cases

- **Risk:** YAML duplicate keys silently collapse into one entry when decoded into a Go map, so our duplicate-alias test would never fail.
  → **Mitigation:** Walk the `yaml.Node.Content` slice of the `keys` mapping directly and track seen aliases before decoding into `map[string]string`. Verified by the `duplicate_alias_in_same_env` behavior test.

- **Risk:** `sniffTeamTemplate` false-positives on a collection file that happens to include a `team_secrets` variable name.
  → **Mitigation:** Sniff only at the *top level* of the document, not recursively. Collections have `name:` + `requests:` at the top level, never `team_secrets:` (enforced by our schema).

- **Risk:** `sniffTeamTemplate` on a non-YAML file could produce a misleading error.
  → **Mitigation:** If sniff fails, fall through to the existing collection validator — its error messages already handle malformed YAML gracefully.

- **Risk:** Exit code 2 conflict with existing uses (discovery `ErrNoMatches` also uses 2 per main.go:285).
  → **Mitigation:** This is fine — 2 is documented semantically as "user-correctable input error" (no matches, bad template). Task observable explicitly mandates exit 2.

- **Edge case:** Template with zero environments (`vault_configs: {}`).
  → **Handling:** Valid parse, but `Validate()` emits one issue: `team_secrets.vault_configs: no environments declared`. Covered by the 9th-10th (structural) tests if needed; if not covered by behavior tests exactly, add a dedicated case to the table.

- **Edge case:** Provider known to `internal/vault` (e.g. `hashicorp-vault`) but out of scope for this slice.
  → **Handling:** Reject with `unknown provider '<value>'` — the teamtemplate package maintains its own whitelist ({AWS, Azure}) rather than reusing `vault.SupportedProviders`. Documented in the package doc-comment so M4-002 can expand the whitelist without confusion.

- **Edge case:** A `keys:` value that is not a string (e.g. a nested map).
  → **Handling:** YAML decode into `map[string]string` will fail; parser wraps the error as `ErrInvalidTemplate`. Covered by an additional edge-case row if needed.

- **Edge case:** Empty file / file without `team_secrets` → sniff returns false → dispatcher routes to collection validator, which correctly reports "missing name/requests" or similar.

## Proposed Go Function Signatures

```go
// internal/vault/teamtemplate/teamtemplate.go
func Parse(data []byte) (*TeamTemplate, error)
func ParseReader(r io.Reader) (*TeamTemplate, error)
func (t *TeamTemplate) Validate() []Issue
func (t *TeamTemplate) Resolve(name string) (*ResolvedEnv, bool)
func (t *TeamTemplate) Summary() string

// internal/validator/validator.go
func ValidateAuto(path string, knownVars map[string]string) *Result
// unexported:
func sniffTeamTemplate(path string) (bool, error)
func validateTeamTemplate(path string) *Result
```

Errors wrap with `fmt.Errorf("parse team template %s: %w", path, err)` and use `%w` throughout so callers can `errors.Is(err, teamtemplate.ErrUnknownProvider)`.

No `context.Context` parameters in this slice — all work is synchronous file I/O with no network.

## Verification

```bash
go build ./cmd/apitest
go test ./...
go test ./internal/vault/teamtemplate/... -run TestTeamTemplate -count=1
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (exact commands from task YAML):

```bash
go build ./cmd/apitest && \
  ./apitest validate testdata/team/shared-vault-template.yaml
# Expected stdout: "OK: shared vault template valid (2 environments, 4 secrets)"
# Expected exit: 0

./apitest validate testdata/team/shared-vault-template.invalid.yaml
# Expected stderr: "error: team_secrets.vault_configs.production.provider: unknown provider 'foo'"
# Expected exit: 2

go test ./internal/vault/teamtemplate/... -run TestTeamTemplate -count=1
# Expected: ok with >= 8 tests passing
```
