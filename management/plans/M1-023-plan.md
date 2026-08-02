# Implementation Plan: M1-023

## Overview
Add sensitive variable detection (by name heuristics and `!sensitive` YAML tag), a redaction layer applied before all output formatters, and a `--allow-sensitive` CLI flag to reveal real values.

## Task Details
- **ID:** M1-023
- **Title:** Sensitive variable detection and redaction
- **Phase:** M1: Core CLI
- **Priority:** 23
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-022 | Verbosity levels (-v, -vv, -q) | done |

---

## Implementation Steps

### Step 1: `internal/variable/sensitive.go` — Core Sensitivity Types and Redaction

**Rationale:** Smallest blast radius. New file, no existing code changes. Establishes the types and pure functions that all subsequent steps depend on.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/sensitive.go` | create | SensitiveSet, IsSensitiveName, RedactValue, RedactHeaders |
| `internal/variable/sensitive_test.go` | create | Full table-driven tests for all new functions |

#### Current Code
```go
// (file does not exist yet)
```

#### New Code
```go
package variable

import (
    "net/http"
    "strings"
)

// Redacted is the replacement text for sensitive variable values in output.
const Redacted = "[REDACTED]"

// sensitiveKeywords are lowercase substrings that trigger auto-sensitivity detection.
var sensitiveKeywords = []string{
    "password", "passwd", "token", "secret",
    "api_key", "apikey", "credential", "authorization",
}

// sensitiveHeaderNames are lowercase header names always treated as sensitive.
var sensitiveHeaderNames = []string{
    "authorization", "x-api-key", "x-auth-token",
    "proxy-authorization", "cookie",
}

// IsSensitiveName returns true if name contains a known sensitive keyword
// (case-insensitive substring match).
func IsSensitiveName(name string) bool {
    lower := strings.ToLower(name)
    for _, kw := range sensitiveKeywords {
        if strings.Contains(lower, kw) {
            return true
        }
    }
    return false
}

// IsSensitiveHeaderName returns true if the HTTP header name is inherently sensitive.
func IsSensitiveHeaderName(name string) bool {
    lower := strings.ToLower(name)
    for _, h := range sensitiveHeaderNames {
        if lower == h {
            return true
        }
    }
    return false
}

// SensitiveSet tracks which variable names are marked sensitive.
type SensitiveSet struct {
    names map[string]bool
}

// NewSensitiveSet returns an initialised empty SensitiveSet.
func NewSensitiveSet() *SensitiveSet {
    return &SensitiveSet{names: make(map[string]bool)}
}

// Add marks name as sensitive.
func (s *SensitiveSet) Add(name string) {
    s.names[name] = true
}

// IsSensitive returns true if name was explicitly added to the set.
func (s *SensitiveSet) IsSensitive(name string) bool {
    if s == nil {
        return false
    }
    return s.names[name]
}

// Merge adds all names from other into s. Nil other is a no-op.
func (s *SensitiveSet) Merge(other *SensitiveSet) {
    if other == nil {
        return
    }
    for k := range other.names {
        s.names[k] = true
    }
}

// Names returns the sorted list of sensitive variable names (for debugging).
func (s *SensitiveSet) Names() []string {
    if s == nil {
        return nil
    }
    out := make([]string, 0, len(s.names))
    for k := range s.names {
        out = append(out, k)
    }
    return out
}

// RedactValue returns Redacted if name is in the sensitive set and allow is false;
// otherwise it returns value unchanged.
func RedactValue(name, value string, s *SensitiveSet, allow bool) string {
    if allow {
        return value
    }
    if s.IsSensitive(name) {
        return Redacted
    }
    return value
}

// RedactHeaders returns a shallow copy of headers with sensitive values replaced.
// A header is redacted when its name is inherently sensitive (Authorization, etc.)
// or when it is listed in s. Returns nil if headers is nil.
func RedactHeaders(headers map[string]string, s *SensitiveSet, allow bool) map[string]string {
    if headers == nil {
        return nil
    }
    if allow {
        out := make(map[string]string, len(headers))
        for k, v := range headers {
            out[k] = v
        }
        return out
    }
    out := make(map[string]string, len(headers))
    for k, v := range headers {
        if IsSensitiveHeaderName(k) || s.IsSensitive(k) {
            out[k] = Redacted
        } else {
            out[k] = v
        }
    }
    return out
}

// RedactResponseHeaders returns a copy of the response header map with sensitive
// headers redacted. Returns nil if headers is nil.
func RedactResponseHeaders(headers http.Header, s *SensitiveSet, allow bool) http.Header {
    if headers == nil {
        return nil
    }
    if allow {
        return headers.Clone()
    }
    out := headers.Clone()
    for k := range out {
        if IsSensitiveHeaderName(k) || s.IsSensitive(k) {
            out[k] = []string{Redacted}
        }
    }
    return out
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestIsSensitiveName(t *testing.T) {
    tests := []struct {
        name      string
        varName   string
        sensitive bool
    }{
        {"password_exact_match", "password", true},
        {"PASSWORD_case_insensitive", "PASSWORD", true},
        {"db_password_substring", "db_password", true},
        {"myPassword_camelCase", "myPassword", true},
        {"token_exact", "token", true},
        {"access_token_substring", "access_token", true},
        {"TOKEN_uppercase", "TOKEN", true},
        {"secret_exact", "secret", true},
        {"my_secret_key_substring", "my_secret_key", true},
        {"api_key_exact", "api_key", true},
        {"API_KEY_uppercase", "API_KEY", true},
        {"apikey_no_underscore", "apikey", true},
        {"authorization_exact", "authorization", true},
        {"AUTHORIZATION_uppercase", "AUTHORIZATION", true},
        {"credential_exact", "credential", true},
        {"base_url_not_sensitive", "base_url", false},
        {"count_not_sensitive", "count", false},
        {"name_not_sensitive", "name", false},
        {"empty_string_not_sensitive", "", false},
    }
    // ...
}

func TestIsSensitiveHeaderName(t *testing.T) {
    tests := []struct{ name, header string; want bool }{
        {"authorization_header", "authorization", true},
        {"Authorization_mixed_case", "Authorization", true},
        {"x_api_key_header", "x-api-key", true},
        {"cookie_header", "cookie", true},
        {"content_type_not_sensitive", "content-type", false},
        {"accept_not_sensitive", "accept", false},
    }
    // ...
}

func TestSensitiveSet(t *testing.T) { /* new_set_empty, add_and_check, ... */ }
func TestRedactValue(t *testing.T) { /* sensitive_name_redacted, allow shows value, ... */ }
func TestRedactHeaders(t *testing.T) { /* authorization redacted, allow_sensitive shows all, ... */ }
```

Complete named test cases:
- `TestIsSensitiveName`: 19 cases as listed above
- `TestIsSensitiveHeaderName`: 6 cases
- `TestSensitiveSet/new_set_empty`, `/add_and_check`, `/not_added_returns_false`, `/nil_set_returns_false`, `/merge_combines_sets`, `/merge_nil_is_noop`
- `TestRedactValue/sensitive_name_redacted`, `/sensitive_name_with_allow_shows_value`, `/non_sensitive_name_passthrough`, `/nil_set_passthrough`
- `TestRedactHeaders/authorization_redacted`, `/content_type_not_redacted`, `/custom_sensitive_var_in_set_redacted`, `/allow_sensitive_shows_all`, `/nil_headers_returns_nil`, `/empty_headers_returns_empty`

#### Impact on Existing Tests
- No existing tests affected (new file).

---

### Step 2: `internal/parser/collection.go` — `SensitiveVars` with `!sensitive` Tag Parsing

**Rationale:** Isolated parser change. Introduces the `SensitiveVars` type and custom YAML unmarshaling before runner/main touches it. Breaking change to `Collection.Variables` type is cleanest when done second, so Step 3+ can be adapted at compile time.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Introduce SensitiveVars, change Variables field type |
| `internal/parser/collection_test.go` or `parser_test.go` | modify | Update existing .Variables refs; add !sensitive tests |
| `internal/parser/testdata/with_sensitive_vars.yaml` | create | Test fixture with !sensitive tag |

#### Current Code
```go
// In Collection struct:
Variables map[string]string `yaml:"variables,omitempty"`

// In RequestItem struct:
Variables map[string]string `yaml:"variables,omitempty"`
```

#### New Code
```go
// SensitiveVars holds a variable map and tracks which names were tagged !sensitive.
type SensitiveVars struct {
    Values    map[string]string
    Sensitive *variable.SensitiveSet
}

// UnmarshalYAML detects the !sensitive tag on individual variable values.
func (sv *SensitiveVars) UnmarshalYAML(value *yaml.Node) error {
    if value.Kind != yaml.MappingNode {
        return fmt.Errorf("variables: expected mapping, got %v", value.Tag)
    }
    sv.Values = make(map[string]string)
    sv.Sensitive = variable.NewSensitiveSet()
    for i := 0; i+1 < len(value.Content); i += 2 {
        keyNode := value.Content[i]
        valNode := value.Content[i+1]
        key := keyNode.Value
        sv.Values[key] = valNode.Value
        if valNode.Tag == "!sensitive" {
            sv.Sensitive.Add(key)
        }
    }
    return nil
}

// In Collection struct:
Variables SensitiveVars `yaml:"variables,omitempty"`

// In RequestItem struct:
Variables SensitiveVars `yaml:"variables,omitempty"`
```

Test fixture `internal/parser/testdata/with_sensitive_vars.yaml`:
```yaml
name: Sensitive Vars Test
variables:
  base_url: "https://example.com"
  api_key: !sensitive "sk_live_abc123"
  password: "secret123"
requests:
  - name: Test request
    request:
      method: GET
      url: "{{base_url}}/get"
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseSensitiveVars(t *testing.T) {
    tests := []struct {
        name             string
        fixture          string
        wantValues       map[string]string
        wantSensitive    []string
    }{
        {
            "sensitive_yaml_tag_detected",
            "with_sensitive_vars.yaml",
            map[string]string{"base_url": "https://example.com", "api_key": "sk_live_abc123", "password": "secret123"},
            []string{"api_key"},
        },
        {
            "no_sensitive_tag_backward_compat",
            "full_example.yaml", // existing fixture
            // existing vars, empty sensitive set
        },
    }
}
```

Named test cases:
- `TestParseSensitiveVars/sensitive_yaml_tag_detected`
- `TestParseSensitiveVars/sensitive_tag_preserves_value`
- `TestParseSensitiveVars/mixed_sensitive_and_regular_vars`
- `TestParseSensitiveVars/no_sensitive_tag_backward_compat`

#### Impact on Existing Tests
- All tests referencing `col.Variables["key"]` must change to `col.Variables.Values["key"]`.
- All tests referencing `item.Variables["key"]` must change to `item.Variables.Values["key"]`.
- Estimate: ~10–15 test lines need the `.Values` access update.

---

### Step 3: `internal/config/dotenv.go` — `!sensitive` Prefix in `.env` Files

**Rationale:** Isolated to the config package. Signature change to return a `SensitiveSet` alongside the map, which `main.go` will consume in Step 5.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/config/dotenv.go` | modify | ParseDotenv/LoadDotenv return *variable.SensitiveSet |
| `internal/config/dotenv_test.go` | modify | Update callers; add !sensitive tests |

#### Current Code
```go
func ParseDotenv(data []byte) (map[string]string, error) { ... }
func LoadDotenv(dir string) (map[string]string, error) { ... }
```

#### New Code
```go
// ParseDotenv parses KEY=VALUE lines. Lines prefixed with "!sensitive " mark the
// variable as sensitive (e.g. "!sensitive API_KEY=value").
func ParseDotenv(data []byte) (map[string]string, *variable.SensitiveSet, error)

// LoadDotenv loads .env from dir, returning variables and their sensitivity metadata.
func LoadDotenv(dir string) (map[string]string, *variable.SensitiveSet, error)
```

Parsing logic for `!sensitive` lines:
```go
if strings.HasPrefix(line, "!sensitive ") {
    line = strings.TrimPrefix(line, "!sensitive ")
    // parse KEY=VALUE as normal, then:
    sensitive.Add(key)
}
```

#### Tests to Write FIRST (RED phase)

Named test cases:
- `TestParseDotenv/sensitive_prefix_marks_variable`
- `TestParseDotenv/sensitive_prefix_preserves_value`
- `TestParseDotenv/no_sensitive_returns_empty_set`
- `TestParseDotenv/multiple_sensitive_lines`
- `TestParseDotenv/regular_lines_not_in_sensitive_set`

#### Impact on Existing Tests
- `TestParseDotenv` and `TestLoadDotenv` test cases need to accept the extra `*variable.SensitiveSet` return value and ignore it (or assert it is empty).
- `main.go` call to `LoadDotenv` must capture the new return value.

---

### Step 4: `internal/runner/runner.go` — Adapt to `SensitiveVars` Type

**Rationale:** Mechanical adaptation so the runner compiles after the parser change. Sensitivity is a display concern, so the runner does not need to understand it — it just reads `col.Variables.Values` instead of `col.Variables`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | col.Variables → col.Variables.Values; item.Variables → item.Variables.Values |
| `internal/runner/runner_test.go` | modify | Fixture collection structs need Variables.Values |

#### Current Code
```go
// In Run or mergeVars:
for k, v := range col.Variables {
    merged[k] = v
}
// ...
for k, v := range item.Variables {
    merged[k] = v
}
```

#### New Code
```go
for k, v := range col.Variables.Values {
    merged[k] = v
}
// ...
for k, v := range item.Variables.Values {
    merged[k] = v
}
```

#### Impact on Existing Tests
- Runner tests that construct `parser.Collection{Variables: map[string]string{...}}` must change to `Variables: parser.SensitiveVars{Values: map[string]string{...}}`.
- Estimate: 5–10 test lines.

---

### Step 5: `cmd/curlew/main.go` — CLI Flag, Sensitivity Building, Redaction

**Rationale:** Wires everything together. Comes last because it depends on all prior steps.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | --allow-sensitive flag, SensitiveSet construction, header redaction before formatters, help text |
| `cmd/curlew/main_test.go` | modify | Add tests for flag parsing and redaction behavior |

#### Current Code
```go
func parseRunArgs(args []string) (string, string, string, map[string]string, Verbosity, error) { ... }
```

#### New Code
```go
func parseRunArgs(args []string) (string, string, string, map[string]string, Verbosity, bool, error) {
    // ... existing parsing ...
    // New flag:
    //   --allow-sensitive    Show sensitive values in plain text (default: redact)
    var allowSensitive bool
    // parse "--allow-sensitive" from args
    return file, format, envFile, cliVars, verbosity, allowSensitive, nil
}
```

Sensitivity construction (after parsing all variable sources):
```go
sensitive := variable.NewSensitiveSet()
// From collection YAML !sensitive tags
sensitive.Merge(col.Variables.Sensitive)
for _, req := range col.Requests {
    sensitive.Merge(req.Variables.Sensitive)
}
// From .env !sensitive prefix
sensitive.Merge(dotenvSensitive)
// Heuristic: check all merged variable names
for name := range allVars {
    if variable.IsSensitiveName(name) {
        sensitive.Add(name)
    }
}
```

Post-run redaction (before any formatter):
```go
for i := range results {
    results[i].RequestHeaders = variable.RedactHeaders(
        results[i].RequestHeaders, sensitive, allowSensitive)
}
```

Help text addition:
```
  --allow-sensitive       Show sensitive values in plain text (default: redact to [REDACTED])
```

#### Tests to Write FIRST (RED phase)

Named test cases:
- `TestParseRunArgs/allow_sensitive_flag_present`
- `TestParseRunArgs/allow_sensitive_default_false`
- `TestRunCmd/sensitive_var_redacted_in_verbose_output`
- `TestRunCmd/allow_sensitive_shows_plaintext_value`
- `TestRunCmd/authorization_header_redacted_in_verbose`
- `TestRunCmd/sensitive_in_json_output_format`
- `TestRunCmd/heuristic_password_var_redacted`
- `TestRunCmd/heuristic_token_var_redacted`

#### Impact on Existing Tests
- Tests that call `parseRunArgs` need to capture the new `allowSensitive bool` return value.
- Integration tests that check verbose output for header values may need updates if they use Authorization headers.

---

### Step 6: Smoke Test Update

**Rationale:** Validates the observable output described in the task.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add sensitive redaction smoke tests |
| `smoke/fixtures/sensitive.yaml` | create | Collection with password and !sensitive vars |

New smoke fixture `smoke/fixtures/sensitive.yaml`:
```yaml
name: Sensitive Test
variables:
  base_url: "https://httpbin.org"
  password: "secret123"
  api_key: !sensitive "sk_live_abc"
requests:
  - name: Get with sensitive vars
    request:
      method: GET
      url: "{{base_url}}/get"
      headers:
        Authorization: "Bearer {{api_key}}"
```

New smoke checks:
```bash
# Sensitive values should be redacted at -vv
output=$(./curlew run -vv smoke/fixtures/sensitive.yaml 2>&1)
echo "$output" | grep -v "secret123" || fail "password value leaked in verbose output"
echo "$output" | grep -v "sk_live_abc" || fail "api_key value leaked in verbose output"
echo "$output" | grep "\[REDACTED\]" || fail "[REDACTED] not shown for sensitive vars"

# --allow-sensitive reveals values
output=$(./curlew run -vv --allow-sensitive smoke/fixtures/sensitive.yaml 2>&1)
echo "$output" | grep "secret123" || fail "--allow-sensitive did not show password value"

# --allow-sensitive in help text
./curlew run --help 2>&1 | grep "allow-sensitive" || fail "--allow-sensitive not in help"
```

---

## Test Impact Summary

| Test File | Impact | Action Required |
|-----------|--------|----------------|
| `internal/parser/*_test.go` | breaks | Change `col.Variables["k"]` to `col.Variables.Values["k"]` (all occurrences) |
| `internal/runner/runner_test.go` | breaks | Change `Variables: map[string]string{...}` to `Variables: parser.SensitiveVars{Values: ...}` |
| `internal/config/dotenv_test.go` | breaks | Accept extra `*variable.SensitiveSet` return value from `ParseDotenv` |
| `cmd/curlew/main_test.go` | breaks | Accept extra `bool` return from `parseRunArgs` |
| `internal/variable/*_test.go` | none | New files only |

---

## Risks and Edge Cases

- **`!sensitive` on non-scalar YAML** → Mitigation: `UnmarshalYAML` checks `valNode.Kind == yaml.ScalarNode`; return error otherwise.
- **Variable `PASSWORD` (all caps)** → `IsSensitiveName` lowercases before matching. Covered.
- **Variable `db_password_hash` (substring)** → Substring match catches "password". Correct and expected.
- **`--allow-sensitive` with `--format json`** → The `allowSensitive` flag is threaded through all formatters uniformly. Works.
- **Sensitive value in URL** → URLs are not redacted (functional requirement). This is a known limitation; the spec does not require URL redaction.
- **Sensitive value in response body** → Out of scope. Response bodies are user data, not variable display contexts.
- **Parser type change blast radius** → Mechanical change at ~15 call sites. All identified. Compiler enforces exhaustiveness.
- **`ParseDotenv` signature change** → Two callers (identified in exploration). Both will fail at compile time — easy to fix.
- **Error message leakage** → Current code never includes variable VALUES in error messages (only names). Add assertion tests to guard this invariant.
- **Sensitivity propagation through interpolation** → Not implemented in this task. `auth_header: "Bearer {{token}}"` makes `auth_header` non-sensitive by name but its value contains a sensitive value. Future work. For now, heuristic name matching and explicit tags cover primary cases.

---

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Setup:
cat > /tmp/sensitive-test.yaml << 'EOF'
name: Sensitive Test
variables:
  password: "secret123"
  api_key: !sensitive abc
requests:
  - name: Test
    request:
      method: GET
      url: "https://httpbin.org/get"
EOF

# Redacted by default at -vv:
./curlew run -vv /tmp/sensitive-test.yaml
# Expect: password and api_key values show [REDACTED]

# Plain text with --allow-sensitive:
./curlew run -vv --allow-sensitive /tmp/sensitive-test.yaml
# Expect: actual values shown
```
