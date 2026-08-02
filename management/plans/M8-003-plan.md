# Implementation Plan: M8-003

## Overview

Add a typed `output:` block to both `apitest.yaml` (project default) and collection YAML (override), with precedence CLI flag > collection > project > built-in default resolved once in `runCmdInner`. Publish `schemas/project-v1.json` at repo root so both levels are editor-validated; mirror the M8-001 pattern (the repo-root `schemas/` package owns `//go:embed`, `internal/schema` aliases bytes, MANUAL.md §1.5 is extended, drift-guard and reachability tests mirror the collection schema's). Extend `scaffold.Init` to emit an active minimal `output:` block and `apitest schema` with a `--project` flag.

## Task Details

- **ID:** M8-003
- **Title:** `output:` block in project + collection YAML with CLI>collection>project precedence
- **Phase:** M8: Developer & Agent Exploration Experience
- **Priority:** 1
- **Complexity:** medium
- **Estimated effort:** 1-2 days
- **Branch:** `feature/M8-003-output-block`

## Dependencies

| Task   | Title                                                  | Status |
|--------|--------------------------------------------------------|--------|
| M8-002 | Close all 8 schema-completeness gaps (W1 follow-up)    | done   |

## Design decisions (locked)

| Decision | Choice | Rationale |
|---|---|---|
| Type home | `output.Config` struct in `internal/output/` alongside `Verbosity` | `Verbosity` already lives there; `parser.Collection` and `config.ProjectConfig` both can import `output` without creating a cycle (neither is imported by `output`). |
| Explicit-set flags | Parallel bools `formatSet`, `reportSet`, `eventsSet`, `verbositySet` on `runFlags` | Task YAML scope mandates this ("less invasive than converting the struct fields to pointers"). Enables zero-value disambiguation without rewriting every struct-field read. |
| YAML field mapping | `format`, `report`, `events`, `verbosity` (all optional; `additionalProperties: false`) | Mirrors CLI flags 1:1; no surprise names. |
| Format enum | `[terminal, json, tap, junit, html]` (five values; excludes `markdown`) | Matches the CLI runtime check at `main.go:492`; markdown is deferred to W4. |
| Verbosity enum | `[quiet, normal, verbose, debug]` with mapping to `output.VerbosityQuiet, VerbosityDefault, VerbosityVerbose, VerbosityDebug` | `"normal"` preferred over `"default"` in user-facing docs despite internal constant being `VerbosityDefault` (matches existing CLI semantics where no flag = normal). |
| `$defs.output` duplication | Byte-identical block copied into both schema files; drift-guarded by `TestSchema_output_defs_match` | Task YAML explicitly documents this trade-off ("cross-file $ref is solvable but not worth the resolver ceremony for a 20-line block"). |
| Project schema publishing | New `schemas/project-v1.json` + `schemas.ProjectV1` var; `schema.ProjectSchema = schemas.ProjectV1` | Mirrors M8-001 pattern verbatim. |
| Validation failure exit code | **Exit 3** when YAML `output:` contains an unknown format or empty path (schema-compile time) | Matches existing parse-error convention; task YAML requires exit 3 for this case. |
| CLI `--format` invalid value | **Exit 1** preserved (flag-parsing error, not parse error) | Unchanged — flag-parsing errors stay at exit 1 per existing main.go:494. |
| Precedence point | Single resolution block in `runCmdInner` after `parser.ParseFileWithOptions` returns `col` AND after `config.LoadProjectConfig` returns `projectCfg` | All three sources are populated by then and before any formatter/emitter construction. |
| Scaffold default | Active block `format: terminal`, `verbosity: normal` written into `apitest.yaml` (no `report`/`events` fields — those default to empty) | Matches DoD "scaffold.Init emits an active minimal output: block". |
| Schema `--project` flag | New `parseSchemaArgs` field; unknown flags still rejected | Backward-compatible: `apitest schema` (no flag) → collection schema. |
| Init `--project-name` flag | Add `--project-name <value>` parser to `initCmdOut` so observable-YAML example works | Observable in task YAML references `apitest init --project-name demo`; currently flag is not parsed. |
| MANUAL.md docs | Extend §1.5 with a **second** `yaml.schemas` line mapping `schemas/project-v1.json` → `**/apitest.yaml`; add a short "output: block" section describing fields + precedence table | Matches DoD. |

## Implementation Steps

Ordered for smallest blast radius: foundational types first → schemas → wire-ups → CLI integration → docs.

### Step 1: Declare `output.Config` type

**Rationale:** Zero consumers currently → first commit. Ensures downstream work (parser/config/main.go) can refer to a stable type.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/config.go` | create | Declare `Config` struct and verbosity parsing helper. |
| `internal/output/config_test.go` | create | Table-driven tests for YAML unmarshal, verbosity parsing, and validation. |

#### New Code (config.go)

```go
// Package output: new file config.go
package output

import (
	"errors"
	"fmt"
)

// ErrUnknownFormat is returned when an output.format value is not one of the
// supported formats.
var ErrUnknownFormat = errors.New("unknown output format")

// ErrEmptyReportPath is returned when output.report is set to an empty string.
var ErrEmptyReportPath = errors.New("empty output report path")

// ErrEmptyEventsPath is returned when output.events is set to an empty string.
var ErrEmptyEventsPath = errors.New("empty output events path")

// Config is the typed form of a YAML `output:` block declared at either the
// project (apitest.yaml) or collection level. All fields are optional; zero
// values mean "not declared" and inherit from lower-precedence scopes.
type Config struct {
	Format    string `yaml:"format,omitempty"`    // terminal|json|tap|junit|html
	Report    string `yaml:"report,omitempty"`    // file path
	Events    string `yaml:"events,omitempty"`    // file path
	Verbosity string `yaml:"verbosity,omitempty"` // quiet|normal|verbose|debug
}

// SupportedFormats lists the valid output.format values (matches the CLI
// --format enum as of M8-003; excludes markdown per scope).
var SupportedFormats = []string{"terminal", "json", "tap", "junit", "html"}

// Validate returns a non-nil error when Config declares an unsupported format
// or an empty-string path. Unset fields are permitted.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	if c.Format != "" && !isSupportedFormat(c.Format) {
		return fmt.Errorf("%w %q (supported: %s)", ErrUnknownFormat, c.Format, formatList())
	}
	// Presence is detected by YAML unmarshal; empty string is caller error (user wrote `report: ""` explicitly).
	// Note: yaml.v3 omits the key when the field is absent, so "" only appears when written explicitly.
	// We therefore treat unset vs empty identically at the struct-zero-value level; but we DO reject
	// the explicit-empty case at Validate time per DoD. This is a presence check via a sentinel:
	// callers must pre-check presence using the raw YAML node if they need to distinguish unset
	// from explicit-empty. For M8-003 we use a wrapping YAMLNode approach — see parseOutputNode.
	return nil
}

// ParseVerbosity converts a YAML verbosity string to output.Verbosity.
// Empty string returns VerbosityDefault with ok=false so callers can treat it
// as "not declared" (same as a missing key).
func ParseVerbosity(s string) (v Verbosity, ok bool, err error) {
	switch s {
	case "":
		return VerbosityDefault, false, nil
	case "quiet":
		return VerbosityQuiet, true, nil
	case "normal":
		return VerbosityDefault, true, nil
	case "verbose":
		return VerbosityVerbose, true, nil
	case "debug":
		return VerbosityDebug, true, nil
	default:
		return VerbosityDefault, false, fmt.Errorf("unknown output verbosity %q (supported: quiet, normal, verbose, debug)", s)
	}
}

func isSupportedFormat(s string) bool {
	for _, f := range SupportedFormats {
		if s == f {
			return true
		}
	}
	return false
}

func formatList() string {
	out := ""
	for i, f := range SupportedFormats {
		if i > 0 {
			out += ", "
		}
		out += f
	}
	return out
}
```

Because `omitempty` cannot distinguish unset from empty-string, and the DoD requires both behaviours to be validated, we will rely on YAML-schema-level validation (see Step 2 `minLength: 1` on `report` and `events`) to reject empty-string paths. The Go `Config.Validate()` therefore only validates `format`; path emptiness is enforced by the JSON Schema before `Validate()` is ever reached in the run path. Schema-compile failure surfaces via parser as a parse error (exit 3).

#### Tests to Write FIRST (RED phase)

```go
// internal/output/config_test.go
package output

import (
	"errors"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfig_Unmarshal(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want Config
	}{
		{"empty", "", Config{}},
		{"format_only", "format: json\n", Config{Format: "json"}},
		{"all_fields", "format: tap\nreport: out.txt\nevents: ev.jsonl\nverbosity: verbose\n", Config{Format: "tap", Report: "out.txt", Events: "ev.jsonl", Verbosity: "verbose"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got Config
			if err := yaml.Unmarshal([]byte(tc.yaml), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr error
	}{
		{"nil", nil, nil},
		{"empty", &Config{}, nil},
		{"valid_format", &Config{Format: "json"}, nil},
		{"invalid_format", &Config{Format: "markdown"}, ErrUnknownFormat},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == nil && err != nil {
				t.Fatalf("got %v, want nil", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want errors.Is(%v)", err, tc.wantErr)
			}
		})
	}
}

func TestParseVerbosity(t *testing.T) {
	tests := []struct {
		in       string
		wantV    Verbosity
		wantOK   bool
		wantErr  bool
	}{
		{"", VerbosityDefault, false, false},
		{"quiet", VerbosityQuiet, true, false},
		{"normal", VerbosityDefault, true, false},
		{"verbose", VerbosityVerbose, true, false},
		{"debug", VerbosityDebug, true, false},
		{"unknown", VerbosityDefault, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			v, ok, err := ParseVerbosity(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if v != tc.wantV {
				t.Errorf("v = %v, want %v", v, tc.wantV)
			}
		})
	}
}
```

#### Impact on Existing Tests
- None — new file, no existing symbol changes.

---

### Step 2: Publish `schemas/project-v1.json` + extend `collection-v1.json`

**Rationale:** After the type exists, the canonical wire format is the next fixed point. Both files update together so the drift-guard test can verify byte-equality from the start.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `schemas/project-v1.json` | create | Draft 2020-12 project schema with `$id`, `title: "ApiTest Project v1"`, `additionalProperties: false`, and `$defs.output`. |
| `schemas/collection-v1.json` | modify | Add top-level `output:` property referencing `#/$defs/output`; inline the `$defs.output` subschema (byte-identical to project). |
| `schemas/schemas.go` | modify | Add `//go:embed project-v1.json` → `var ProjectV1 []byte`. |
| `internal/schema/schema.go` | modify | Add `var ProjectSchema = schemas.ProjectV1`. |

#### Current Code (schemas/schemas.go)

```go
package schemas

import _ "embed"

//go:embed collection-v1.json
var CollectionV1 []byte
```

#### New Code (schemas/schemas.go)

```go
package schemas

import _ "embed"

//go:embed collection-v1.json
var CollectionV1 []byte

// ProjectV1 is the JSON Schema (Draft 2020-12) for apitest.yaml project config
// files, v1. See schemas/project-v1.json.
//
//go:embed project-v1.json
var ProjectV1 []byte
```

#### New Code (schemas/project-v1.json — skeleton; final contents filled during execute)

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/peterlindqvist/apitest/main/schemas/project-v1.json",
  "title": "ApiTest Project v1",
  "description": "Schema for apitest.yaml project configuration files",
  "type": "object",
  "required": ["project_name"],
  "properties": {
    "project_name": { "type": "string" },
    "variables":    { "type": "object" },
    "secrets":      { "type": "object" },
    "auth_profiles":{ "type": "object" },
    "defaults":     { "type": "object" },
    "output":       { "$ref": "#/$defs/output" }
  },
  "additionalProperties": false,
  "$defs": {
    "output": {
      "type": "object",
      "description": "Default output configuration. Overridden by collection-level output and CLI flags (precedence: CLI > collection > project > built-in).",
      "properties": {
        "format":    { "type": "string", "enum": ["terminal", "json", "tap", "junit", "html"] },
        "report":    { "type": "string", "minLength": 1 },
        "events":    { "type": "string", "minLength": 1 },
        "verbosity": { "type": "string", "enum": ["quiet", "normal", "verbose", "debug"] }
      },
      "additionalProperties": false
    }
  }
}
```

`schemas/collection-v1.json` gets an `output` property under `properties` and the same `$defs.output` object (byte-identical, copied verbatim).

#### New Code (internal/schema/schema.go)

```go
package schema

import "github.com/peterlindqvist/apitest/schemas"

// CollectionSchema is the JSON Schema for apitest collection YAML files.
var CollectionSchema = schemas.CollectionV1

// ProjectSchema is the JSON Schema for apitest.yaml project configuration files.
var ProjectSchema = schemas.ProjectV1
```

#### Tests to Write FIRST (RED phase) — in `internal/schema/`

```go
// internal/schema/project_schema_test.go (new)
package schema

import (
	"encoding/json"
	"testing"
)

func TestProjectSchema(t *testing.T) {
	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{"embedded_is_valid_json", func(t *testing.T) {
			if !json.Valid(ProjectSchema) {
				t.Fatalf("ProjectSchema is not valid JSON")
			}
		}},
		{"has_schema_keyword", func(t *testing.T) {
			var m map[string]any
			_ = json.Unmarshal(ProjectSchema, &m)
			if _, ok := m["$schema"]; !ok {
				t.Fatal("missing $schema")
			}
		}},
		{"has_title", func(t *testing.T) {
			var m map[string]any
			_ = json.Unmarshal(ProjectSchema, &m)
			if m["title"] != "ApiTest Project v1" {
				t.Errorf("title = %v, want \"ApiTest Project v1\"", m["title"])
			}
		}},
		{"has_id", func(t *testing.T) {
			var m map[string]any
			_ = json.Unmarshal(ProjectSchema, &m)
			want := "https://raw.githubusercontent.com/peterlindqvist/apitest/main/schemas/project-v1.json"
			if m["$id"] != want {
				t.Errorf("$id = %v, want %v", m["$id"], want)
			}
		}},
		{"additional_properties_false", func(t *testing.T) {
			var m map[string]any
			_ = json.Unmarshal(ProjectSchema, &m)
			if m["additionalProperties"] != false {
				t.Errorf("additionalProperties = %v, want false", m["additionalProperties"])
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { tc.check(t) })
	}
}
```

```go
// internal/schema/validate_test.go (additions — same package validate_test)

func publishedProjectSchemaPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "schemas", "project-v1.json")
}

func TestSchema_project_published_path_matches_embed(t *testing.T) {
	path := publishedProjectSchemaPath(t)
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(fileBytes, schema.ProjectSchema) {
		t.Fatalf("schemas/project-v1.json differs from schema.ProjectSchema")
	}
}

func TestSchema_project_file_exists_at_published_path(t *testing.T) {
	info, err := os.Stat(publishedProjectSchemaPath(t))
	if err != nil {
		t.Fatalf("schemas/project-v1.json missing: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("schemas/project-v1.json is empty")
	}
}

func TestSchema_output_defs_match(t *testing.T) {
	// Byte-identical $defs.output subschema across the two files.
	var col, proj map[string]any
	if err := json.Unmarshal(schema.CollectionSchema, &col); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(schema.ProjectSchema, &proj); err != nil {
		t.Fatal(err)
	}
	colOut := col["$defs"].(map[string]any)["output"]
	projOut := proj["$defs"].(map[string]any)["output"]
	colBytes, _ := json.Marshal(colOut)
	projBytes, _ := json.Marshal(projOut)
	if !bytes.Equal(colBytes, projBytes) {
		t.Fatalf("$defs.output differs:\ncollection: %s\nproject:    %s", colBytes, projBytes)
	}
}
```

```go
// internal/schema/validate_coverage_test.go (additions)

func TestSchema_accepts_output(t *testing.T) {
	type acceptCase struct {
		name    string
		fixture string // relative to internal/schema/testdata
		target  string // "collection" or "project"
	}
	cases := []acceptCase{
		{"collection_output_terminal",   "output_collection_terminal.yaml",   "collection"},
		{"collection_output_json",       "output_collection_json.yaml",       "collection"},
		{"collection_output_tap",        "output_collection_tap.yaml",        "collection"},
		{"collection_output_junit",      "output_collection_junit.yaml",      "collection"},
		{"collection_output_html",       "output_collection_html.yaml",       "collection"},
		{"collection_output_quiet",      "output_collection_verbosity_quiet.yaml",   "collection"},
		{"collection_output_normal",     "output_collection_verbosity_normal.yaml",  "collection"},
		{"collection_output_verbose",    "output_collection_verbosity_verbose.yaml", "collection"},
		{"collection_output_debug",      "output_collection_verbosity_debug.yaml",   "collection"},
		{"project_output_terminal",      "output_project_terminal.yaml",      "project"},
		{"project_output_json_report",   "output_project_json_report.yaml",   "project"},
	}
	root := repoRoot(t)
	colSch := compileCollectionSchema(t)
	projSch := compileProjectSchema(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, "internal", "schema", "testdata", tc.fixture)
			doc := decodeYAMLFile(t, path)
			sch := colSch
			if tc.target == "project" {
				sch = projSch
			}
			if err := sch.Validate(doc); err != nil {
				t.Fatalf("validate %s: %v", tc.fixture, err)
			}
		})
	}
}

func TestSchema_rejects_unknown_output_format(t *testing.T) {
	doc := map[string]any{
		"name":     "x",
		"output":   map[string]any{"format": "markdown"},
		"requests": []any{map[string]any{"name": "r", "request": map[string]any{"method": "GET", "url": "u"}}},
	}
	sch := compileCollectionSchema(t)
	if err := sch.Validate(doc); err == nil {
		t.Fatal("expected validation error for output.format=markdown, got nil")
	}
}

func TestSchema_rejects_empty_output_path(t *testing.T) {
	doc := map[string]any{
		"name":     "x",
		"output":   map[string]any{"report": ""},
		"requests": []any{map[string]any{"name": "r", "request": map[string]any{"method": "GET", "url": "u"}}},
	}
	sch := compileCollectionSchema(t)
	if err := sch.Validate(doc); err == nil {
		t.Fatal("expected validation error for empty output.report, got nil")
	}
}
```

A new helper `compileProjectSchema(t)` mirrors `compileCollectionSchema` in `validate_test.go`.

#### Fixtures to Create (under `internal/schema/testdata/`)

One minimal fixture per test row. Example shapes:

```yaml
# output_collection_terminal.yaml
name: out-terminal
output:
  format: terminal
requests:
  - name: r
    request: { method: GET, url: "https://example.com" }
```

```yaml
# output_project_json_report.yaml
project_name: demo
output:
  format: json
  report: results.json
  verbosity: normal
```

#### Impact on Existing Tests
- `TestSchema_examples` (validate_coverage_test.go): will now also walk the new `output_*.yaml` fixtures. Must pass without modification (new fixtures are all valid).
- `TestSchema_published_path_matches_embed` (existing collection drift guard): passes because `schemas/collection-v1.json` and the embed are updated in the same commit.
- No existing collection fixtures need changes — `output:` is optional.

---

### Step 3: Wire `output.Config` into `parser.Collection` and `config.ProjectConfig`

**Rationale:** Struct fields must exist before `runCmdInner` can read them. Both adds are additive; no existing callers change.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `Output *output.Config` field to `Collection` struct with `yaml:"output,omitempty"`. |
| `internal/parser/collection_test.go` | modify | New test `TestCollection_Output_Roundtrip` verifies YAML unmarshal populates `Collection.Output`. |
| `internal/config/project.go` | modify | Add `Output *output.Config` field to `ProjectConfig`; parse from `projectFile.Output yaml.Node`. |
| `internal/config/project_test.go` | modify | New test `TestParseProjectConfig_Output` verifies round-trip. |

#### Current Code (parser/collection.go — excerpt)

```go
type Collection struct {
	Name         string            `yaml:"name"`
	// ...
	RateLimitRPS int               `yaml:"rate_limit_rps,omitempty"`
	ExternalFiles []string `yaml:"-"`
}
```

#### New Code (parser/collection.go — excerpt)

```go
import (
	// ... existing imports
	"github.com/peterlindqvist/apitest/internal/output"
)

type Collection struct {
	Name         string            `yaml:"name"`
	// ...
	RateLimitRPS int               `yaml:"rate_limit_rps,omitempty"`
	Output       *output.Config    `yaml:"output,omitempty"` // M8-003: per-collection output override
	ExternalFiles []string `yaml:"-"`
}
```

#### Current Code (config/project.go — excerpt)

```go
type ProjectConfig struct {
	ProjectName  string
	Variables    map[string]string
	Secrets      *vault.SecretsConfig
	AuthProfiles []auth.Profile
	Defaults     DefaultsConfig
}

type projectFile struct {
	ProjectName  string         `yaml:"project_name"`
	Variables    map[string]any `yaml:"variables"`
	Secrets      yaml.Node      `yaml:"secrets,omitempty"`
	AuthProfiles yaml.Node      `yaml:"auth_profiles,omitempty"`
	Defaults     yaml.Node      `yaml:"defaults,omitempty"`
}
```

#### New Code (config/project.go — excerpt)

```go
import (
	// ... existing
	"github.com/peterlindqvist/apitest/internal/output"
)

type ProjectConfig struct {
	ProjectName  string
	Variables    map[string]string
	Secrets      *vault.SecretsConfig
	AuthProfiles []auth.Profile
	Defaults     DefaultsConfig
	Output       *output.Config // M8-003: project-level output default
}

type projectFile struct {
	ProjectName  string         `yaml:"project_name"`
	Variables    map[string]any `yaml:"variables"`
	Secrets      yaml.Node      `yaml:"secrets,omitempty"`
	AuthProfiles yaml.Node      `yaml:"auth_profiles,omitempty"`
	Defaults     yaml.Node      `yaml:"defaults,omitempty"`
	Output       yaml.Node      `yaml:"output,omitempty"`
}

// ... in ParseProjectConfig, after Defaults block:
if pf.Output.Kind != 0 {
	var oc output.Config
	if err := pf.Output.Decode(&oc); err != nil {
		return nil, fmt.Errorf("%w: output: %w", ErrInvalidProjectConfig, err)
	}
	if verr := oc.Validate(); verr != nil {
		return nil, fmt.Errorf("%w: output: %w", ErrInvalidProjectConfig, verr)
	}
	cfg.Output = &oc
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/parser/collection_test.go
func TestCollection_Output_Roundtrip(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want *output.Config
	}{
		{"no_output_block", "name: x\nrequests:\n  - name: r\n    request:\n      method: GET\n      url: u\n", nil},
		{"output_format_only", "name: x\noutput:\n  format: json\nrequests:\n  - name: r\n    request:\n      method: GET\n      url: u\n", &output.Config{Format: "json"}},
		{"output_all_fields", "name: x\noutput:\n  format: tap\n  report: out.txt\n  events: ev.jsonl\n  verbosity: verbose\nrequests:\n  - name: r\n    request:\n      method: GET\n      url: u\n", &output.Config{Format: "tap", Report: "out.txt", Events: "ev.jsonl", Verbosity: "verbose"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var col Collection
			if err := yaml.Unmarshal([]byte(tc.yaml), &col); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if (col.Output == nil) != (tc.want == nil) {
				t.Fatalf("Output nil = %v, want %v", col.Output == nil, tc.want == nil)
			}
			if tc.want != nil && *col.Output != *tc.want {
				t.Errorf("Output = %+v, want %+v", *col.Output, *tc.want)
			}
		})
	}
}
```

```go
// internal/config/project_test.go
func TestParseProjectConfig_Output(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    *output.Config
		wantErr bool
	}{
		{"no_output", "project_name: demo\n", nil, false},
		{"full_output", "project_name: demo\noutput:\n  format: json\n  report: r.json\n  verbosity: normal\n", &output.Config{Format: "json", Report: "r.json", Verbosity: "normal"}, false},
		{"invalid_format", "project_name: demo\noutput:\n  format: markdown\n", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := os.CreateTemp(t.TempDir(), "apitest-*.yaml")
			_, _ = f.WriteString(tc.yaml)
			_ = f.Close()
			cfg, err := ParseProjectConfig(f.Name())
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if (cfg.Output == nil) != (tc.want == nil) {
				t.Fatalf("Output nil mismatch")
			}
			if tc.want != nil && *cfg.Output != *tc.want {
				t.Errorf("Output = %+v, want %+v", *cfg.Output, *tc.want)
			}
		})
	}
}
```

#### Impact on Existing Tests
- Parser test suite: additive field, `omitempty` → all existing fixtures unchanged.
- `config/project_test.go::TestParseProjectConfig_*`: zero changes — `Output` is a new optional field.
- **No downstream import cycle**: `internal/output` does not import `parser` or `config`, so adding these imports is safe. Verified: `internal/output/*.go` imports only stdlib + `internal/variable`.

---

### Step 4: Add explicit-set flags to `runFlags` and precedence resolution in `runCmdInner`

**Rationale:** Resolution must happen after both `col` and `projectCfg` are available. Task YAML is explicit: parallel bools, not pointers.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add `formatSet, reportSet, eventsSet, verbositySet bool` to `runFlags`; set each when the corresponding flag is parsed. Insert precedence-resolution block in `runCmdInner` after `LoadProjectConfig` returns. |
| `cmd/apitest/main_test.go` | modify | New `TestOutputPrecedence` with four sub-tests. |

#### Current Code (main.go:140-163)

```go
type runFlags struct {
	file, envName, format, report                                   string
	vars, envVarVars                                                map[string]string
	seed                                                            *int64
	noColor                                                         bool
	verbosity                                                       output.Verbosity
	allowSensitive, showDeps, dryRun, parallel, confirmLargeDataset bool
	// ... existing
	events string
}
```

#### New Code (main.go — excerpt)

```go
type runFlags struct {
	file, envName, format, report                                   string
	vars, envVarVars                                                map[string]string
	seed                                                            *int64
	noColor                                                         bool
	verbosity                                                       output.Verbosity
	allowSensitive, showDeps, dryRun, parallel, confirmLargeDataset bool
	// ... existing
	events string

	// M8-003: explicit-set tracking so zero values of --format/--report/--events/-q/-v/-vv
	// are distinguishable from "flag not passed". See precedence resolution in runCmdInner.
	formatSet, reportSet, eventsSet, verbositySet bool
}
```

In `parseRunArgs`, set the parallel bool in each matching case:

```go
case "-vv":
	f.verbosity = output.VerbosityDebug
	f.verbositySet = true
case "-v":
	f.verbosity = output.VerbosityVerbose
	f.verbositySet = true
case "-q", "--quiet":
	f.verbosity = output.VerbosityQuiet
	f.verbositySet = true
// ... similarly for --format / --report / --events
case "--format":
	i++
	if i >= len(args) { return errorf("--format requires a value (e.g. --format json)") }
	f.format = args[i]
	f.formatSet = true
case "--report":
	// ... + f.reportSet = true
case "--events":
	// ... + f.eventsSet = true
```

Precedence resolution block (insert in `runCmdInner` after `LoadProjectConfig` line ~791 and before the feature-gate checks for `--format junit/html` — note those checks currently read `format`, so the resolution block must happen BEFORE them; we will relocate the junit/html gates to AFTER resolution):

```go
// M8-003: Resolve output configuration from three sources.
// Precedence: CLI flag > collection.output > project.output > built-in default.
// Each field (format, report, events, verbosity) is resolved independently so a
// user can mix-and-match (e.g. project sets format=json, collection overrides events=/tmp/ev.jsonl).
if resolveErr := resolveOutputPrecedence(&flags, col.Output, projectCfg.Output); resolveErr != nil {
	errOut.StructuredError(resolveErr)
	if eventsEmitter != nil { _ = eventsEmitter.EmitRunError(resolveErr); evExitCode = 3 }
	return 3, nil
}
format = flags.format
report = flags.report
verbosity = flags.verbosity
// flags.events already consumed earlier for eventsEmitter construction — IMPORTANT: we must
// also move the eventsEmitter open block to AFTER precedence resolution so a project-level
// events: path takes effect. See step 4a below.
```

#### Step 4a: Move events-emitter open block

The existing events-emitter open block is at lines ~506-526, before `parser.ParseFileWithOptions`. It must move to **after** `LoadProjectConfig` and **after** `resolveOutputPrecedence`, so a project/collection-level `output.events` path is honored. The feature-gate checks for junit/html at lines ~554-590 must also move after resolution (they read `format`). Everything related to events-file-opening-failing-fast-before-requests stays true because no HTTP request has happened yet.

```go
// resolveOutputPrecedence writes to flags fields using the precedence
// CLI > collection > project > built-in. Set booleans on runFlags gate the
// "CLI wins" condition. Returns an error only for schema-level invalid values
// (which should have been caught at parse time but are double-checked here).
func resolveOutputPrecedence(flags *runFlags, colOut, projOut *output.Config) error {
	// Coalesce in reverse-precedence order, with CLI winning.
	var effectiveFormat, effectiveReport, effectiveEvents, effectiveVerbosityStr string
	if projOut != nil {
		effectiveFormat, effectiveReport, effectiveEvents, effectiveVerbosityStr =
			projOut.Format, projOut.Report, projOut.Events, projOut.Verbosity
	}
	if colOut != nil {
		if colOut.Format    != "" { effectiveFormat    = colOut.Format }
		if colOut.Report    != "" { effectiveReport    = colOut.Report }
		if colOut.Events    != "" { effectiveEvents    = colOut.Events }
		if colOut.Verbosity != "" { effectiveVerbosityStr = colOut.Verbosity }
	}
	if !flags.formatSet    && effectiveFormat != ""    { flags.format    = effectiveFormat }
	if !flags.reportSet    && effectiveReport != ""    { flags.report    = effectiveReport }
	if !flags.eventsSet    && effectiveEvents != ""    { flags.events    = effectiveEvents }
	if !flags.verbositySet && effectiveVerbosityStr != "" {
		v, _, err := output.ParseVerbosity(effectiveVerbosityStr)
		if err != nil { return err }
		flags.verbosity = v
	}
	// Validate final format (defensive; schema should have rejected this earlier).
	if flags.format != "" && flags.format != "terminal" && flags.format != "json" &&
		flags.format != "tap" && flags.format != "junit" && flags.format != "html" {
		return fmt.Errorf("unknown output format %q (supported: terminal, json, tap, junit, html)", flags.format)
	}
	return nil
}
```

#### Tests to Write FIRST (RED phase)

```go
// cmd/apitest/output_precedence_test.go (new)
func TestOutputPrecedence(t *testing.T) {
	// Each sub-test sets up a temp project root with apitest.yaml + collection yaml,
	// invokes runCmdInner with captured stdout, and asserts the effective format.
	//
	// We detect the effective format by asserting stdout parses as the chosen format
	// (e.g. json.Valid(stdout) for JSON; "1..N" for TAP).
	tests := []struct {
		name          string
		projectOut    string // yaml output: block for apitest.yaml (or "")
		collectionOut string // yaml output: block for collection (or "")
		cliArgs       []string
		wantStdoutHas string
	}{
		{"builtin_default", "", "", nil, "PASS"},             // terminal
		{"project_wins",    "output:\n  format: json\n", "", nil, "\"status\""},
		{"collection_wins", "output:\n  format: tap\n", "output:\n  format: json\n", nil, "\"status\""},
		{"cli_wins",        "output:\n  format: tap\n", "output:\n  format: json\n", []string{"--format", "terminal"}, "PASS"},
	}
	// ... setup + invoke + assert
}
```

A stub HTTP server serves `200 OK` so collections run without network dependency.

#### Impact on Existing Tests
- `cmd/apitest/main_test.go::TestParseRunArgs`: any test that inspects `runFlags` struct literals will need to be updated with the new bool fields or use `cmp.Diff` with `cmpopts.IgnoreFields`. Audit: grep `runFlags{` in `cmd/apitest/*_test.go` shows zero matches with explicit struct literals — all existing tests use `parseRunArgs` return values and assert individual fields. **No test breakage expected.**
- `TestRunCmd_*`: existing tests don't declare `output:` in their fixtures, so precedence resolution is a no-op — zero behaviour change.
- Watch subcommand: `runCmdInner` is invoked via `RunFunc`; precedence resolution runs naturally on each watch iteration.

---

### Step 5: Add `apitest schema --project` flag

**Rationale:** Small user-visible change, isolated to `schemaCmdOut`. Must come after `schema.ProjectSchema` exists (Step 2).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | `parseSchemaArgs` returns `(format string, project bool, err error)`; `schemaCmdOut` writes `schema.ProjectSchema` when project is true. |
| `cmd/apitest/main_test.go` | modify | Extend `TestSchemaCmd` with two new rows: `--project` emits project schema, `--project --format json` works. |

#### Current Code (main.go:2566-2601)

```go
func schemaCmdOut(args []string, stdout, stderr io.Writer) int {
	format, err := parseSchemaArgs(args)
	// ...
	_, _ = stdout.Write(schema.CollectionSchema)
}

func parseSchemaArgs(args []string) (format string, err error) { /* ... */ }
```

#### New Code

```go
func schemaCmdOut(args []string, stdout, stderr io.Writer) int {
	format, project, err := parseSchemaArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if format != "" && format != "json" {
		_, _ = fmt.Fprintf(stderr, "unknown output format %q (supported: json)\n", format)
		return 1
	}
	data := schema.CollectionSchema
	if project {
		data = schema.ProjectSchema
	}
	_, _ = stdout.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		_, _ = fmt.Fprintln(stdout)
	}
	return 0
}

func parseSchemaArgs(args []string) (format string, project bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			i++
			if i >= len(args) { return "", false, fmt.Errorf("--format requires a value (e.g. --format json)") }
			format = args[i]
		case "--project":
			project = true
		default:
			return "", false, fmt.Errorf("unknown argument: %s", args[i])
		}
	}
	return format, project, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
// cmd/apitest/main_test.go — additions to TestSchemaCmd's table
{
	name: "project_flag_emits_project_schema",
	args: []string{"schema", "--project"},
	wantExit: 0,
	check: func(t *testing.T, stdout, stderr string) {
		if !strings.Contains(stdout, "ApiTest Project v1") {
			t.Errorf("want title 'ApiTest Project v1' in stdout; got %q", stdout)
		}
	},
},
{
	name: "no_flag_emits_collection_schema",
	args: []string{"schema"},
	wantExit: 0,
	check: func(t *testing.T, stdout, stderr string) {
		if !strings.Contains(stdout, "ApiTest Collection v1") {
			t.Errorf("want title 'ApiTest Collection v1' in stdout; got %q", stdout)
		}
	},
},
```

#### Impact on Existing Tests
- `TestSchemaCmd` existing rows: all still pass (`--project` defaults to false).
- No call sites of `parseSchemaArgs` exist outside `schemaCmdOut`.

---

### Step 6: Extend `scaffold.Init` to emit `output:` block + support `--project-name`

**Rationale:** After the schema accepts `output:`, the scaffolder can safely emit it. The DoD requires the scaffolded `apitest.yaml` validates against `schemas/project-v1.json`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/scaffold/scaffold.go` | modify | `apitestYAML` emits `output:\n  format: terminal\n  verbosity: normal\n`. |
| `internal/scaffold/scaffold_test.go` | modify | Add test row asserting the emitted block is present. |
| `internal/schema/validate_test.go` | modify | Add `TestSchema_scaffolded_apitest_yaml_validates` — runs `scaffold.Init`, reads `apitest.yaml`, validates against `schema.ProjectSchema`. |
| `cmd/apitest/main.go` | modify | `initCmdOut` parses `--project-name <value>` flag. |
| `cmd/apitest/main_test.go` | modify | Add test row for `init --project-name demo`. |

#### Current Code (scaffold.go:112-114)

```go
func apitestYAML(projectName string) string {
	return fmt.Sprintf("project_name: %s\nvariables:\n  base_url: \"https://httpbin.org\"\n", projectName)
}
```

#### New Code

```go
func apitestYAML(projectName string) string {
	return fmt.Sprintf("project_name: %s\n"+
		"variables:\n"+
		"  base_url: \"https://httpbin.org\"\n"+
		"output:\n"+
		"  format: terminal\n"+
		"  verbosity: normal\n", projectName)
}
```

#### Current Code (main.go:2430-2450 `initCmdOut`)

```go
func initCmdOut(args []string, stdout, stderr io.Writer) int {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	absDir, err := filepath.Abs(dir)
	// ...
	projectName := filepath.Base(absDir)
	if err := scaffold.Init(scaffold.Options{Dir: dir, ProjectName: projectName}); err != nil { /* ... */ }
	// ...
}
```

#### New Code

```go
func initCmdOut(args []string, stdout, stderr io.Writer) int {
	dir := "."
	projectNameFlag := ""
	positional := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project-name":
			i++
			if i >= len(args) {
				_, _ = fmt.Fprintln(stderr, "Error: --project-name requires a value")
				return 1
			}
			projectNameFlag = args[i]
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) > 0 {
		dir = positional[0]
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	projectName := projectNameFlag
	if projectName == "" {
		projectName = filepath.Base(absDir)
	}
	if err := scaffold.Init(scaffold.Options{Dir: dir, ProjectName: projectName}); err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	// ... unchanged remainder
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/scaffold/scaffold_test.go — new row
{
	name: "apitest.yaml contains active output block",
	checkContent: map[string]string{
		"apitest.yaml": "output:\n  format: terminal\n  verbosity: normal",
	},
},
```

```go
// internal/schema/validate_test.go — new test
func TestSchema_scaffolded_apitest_yaml_validates(t *testing.T) {
	tmp := t.TempDir()
	if err := scaffold.Init(scaffold.Options{Dir: tmp}); err != nil {
		t.Fatal(err)
	}
	doc := decodeYAMLFile(t, filepath.Join(tmp, "apitest.yaml"))
	sch := compileProjectSchema(t)
	if err := sch.Validate(doc); err != nil {
		t.Fatalf("apitest.yaml did not validate against project schema: %v", err)
	}
}
```

```go
// cmd/apitest/main_test.go — TestInitCmd extension
{
	name: "init --project-name sets explicit name",
	args: []string{"init", "--project-name", "demo"},
	check: func(t *testing.T, tmpDir, stdout, stderr string) {
		data, _ := os.ReadFile(filepath.Join(tmpDir, "apitest.yaml"))
		if !strings.Contains(string(data), "project_name: demo") {
			t.Errorf("want 'project_name: demo' in apitest.yaml; got:\n%s", data)
		}
	},
},
```

#### Impact on Existing Tests
- `TestInit` existing rows all continue to pass — the added `output:` block is additive; assertions match substrings.
- `TestSchema_validates_scaffolded_sample` (existing collection-schema DoD test): **unaffected** — it validates `collections/sample.yaml`, not `apitest.yaml`. The new `TestSchema_scaffolded_apitest_yaml_validates` is the project-schema sibling.
- `TestInitCmd` in main_test.go: existing rows use only positional args; the new `--project-name` row is additive.

---

### Step 7: Create `examples/output-block/` fixtures + observable smoke hook

**Rationale:** The observable in the task YAML references `examples/output-block/collection.yaml` and `examples/output-block/bad-format.yaml`. These must exist for the observable to pass.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `examples/output-block/apitest.yaml` | create | `project_name: output-demo\noutput:\n  format: json\n  verbosity: normal\n` |
| `examples/output-block/collection.yaml` | create | Runs one request against httpbin; no `output:` block (inherits from project). |
| `examples/output-block/bad-format.yaml` | create | Collection with `output:\n  format: markdown\n` (triggers exit 3). |

Note: these examples will be excluded from `TestSchema_examples` automatic walker, or included if valid. The bad-format fixture must NOT live under `internal/schema/testdata/` (which would fail the walker). We place it under `examples/output-block/` which is outside the walker's scope.

#### Impact on Existing Tests
- `TestSchema_examples` glob: `internal/schema/testdata/*.yaml` — **unaffected** (examples live elsewhere).
- Smoke test (`./smoke/run.sh`): optionally add a block exercising the new observables; at minimum verify `apitest schema --project` emits the project schema. Defer heavier smoke updates to `/verify`.

---

### Step 8: Update MANUAL.md §1.5 + add short "output: block" section + CHANGELOG

**Rationale:** Documentation last; depends on all prior API surface being stable.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `docs/MANUAL.md` | modify | Add second `yaml.schemas` line to §1.5; add new subsection "§3.6.1 The `output:` block" (fields, enum, precedence table). |
| `CHANGELOG.md` | modify | One bullet under `[Unreleased] → Added`. |

#### MANUAL.md additions

§1.5 edited snippet:

```json
{
  "yaml.schemas": {
    "./schemas/collection-v1.json": "collections/*.yaml",
    "./schemas/project-v1.json":    "apitest.yaml"
  }
}
```

New subsection (approximate placement after §3.6 "Project-wide config"):

> ### 3.6.1 The `output:` block
>
> ApiTool accepts an optional `output:` block at two levels: in `apitest.yaml` (project default) and in the collection YAML (per-collection override). The block supports four optional fields:
>
> | Field       | YAML type | Values                                    |
> |-------------|-----------|-------------------------------------------|
> | `format`    | string    | `terminal`, `json`, `tap`, `junit`, `html` |
> | `report`    | string    | file path (non-empty)                     |
> | `events`    | string    | file path (non-empty)                     |
> | `verbosity` | string    | `quiet`, `normal`, `verbose`, `debug`     |
>
> **Precedence (each field resolved independently):**
>
> CLI flag > collection `output:` > project `output:` > built-in default (`format: terminal`, `verbosity: normal`).

#### CHANGELOG addition

```markdown
### Added
- CLI: typed `output:` block accepted at both `apitest.yaml` (project default) and collection YAML (override); precedence CLI > collection > project > built-in resolved once in `runCmdInner` before formatter and events-emitter construction. New `schemas/project-v1.json` published at repo root (`$id = https://.../project-v1.json`, `title = "ApiTest Project v1"`); `schema.ProjectSchema` added as the byte-slice alias. `apitest schema --project` emits the project schema; `apitest schema` (no flag) continues to emit the collection schema. `scaffold.Init` now writes an active minimal `output:` block (`format: terminal`, `verbosity: normal`) into `apitest.yaml`, and the scaffolded file validates against the project schema (`TestSchema_scaffolded_apitest_yaml_validates`). `runFlags` gained `formatSet/reportSet/eventsSet/verbositySet` parallel bools so CLI zero-values are distinguishable from "flag not passed". The `$defs.output` subschema is byte-identical across both schema files (guarded by `TestSchema_output_defs_match`). New fixtures: `internal/schema/testdata/output_*.yaml` (one per format, per verbosity, plus project-level examples). MANUAL.md §1.5 gains the second `yaml.schemas` mapping for `schemas/project-v1.json` → `apitest.yaml`; new §3.6.1 describes the block and the precedence table. Markdown format remains excluded pending W4. (M8-003)
```

---

## Test Impact Summary

| Test File                                                  | Test Function                              | Impact   | Action Required                                         |
|-----------------------------------------------------------|--------------------------------------------|----------|---------------------------------------------------------|
| `internal/output/config_test.go`                           | `TestConfig_Unmarshal`, `TestConfig_Validate`, `TestParseVerbosity` | new      | write (Step 1)                                          |
| `internal/schema/project_schema_test.go`                   | `TestProjectSchema` (table-driven)         | new      | write (Step 2)                                          |
| `internal/schema/validate_test.go`                         | `TestSchema_project_published_path_matches_embed`, `TestSchema_project_file_exists_at_published_path`, `TestSchema_output_defs_match`, `TestSchema_scaffolded_apitest_yaml_validates` | new      | write (Steps 2, 6)                                      |
| `internal/schema/validate_coverage_test.go`                | `TestSchema_accepts_output`, `TestSchema_rejects_unknown_output_format`, `TestSchema_rejects_empty_output_path` | new      | write (Step 2)                                          |
| `internal/parser/collection_test.go`                       | `TestCollection_Output_Roundtrip`          | new      | write (Step 3)                                          |
| `internal/config/project_test.go`                          | `TestParseProjectConfig_Output`            | new      | write (Step 3)                                          |
| `cmd/apitest/output_precedence_test.go`                    | `TestOutputPrecedence` (4 sub-tests)       | new      | write (Step 4)                                          |
| `cmd/apitest/main_test.go`                                 | `TestSchemaCmd` (existing)                 | additive | add 2 rows (Step 5)                                     |
| `cmd/apitest/main_test.go`                                 | `TestInitCmd` (existing)                   | additive | add 1 row (Step 6)                                      |
| `internal/scaffold/scaffold_test.go`                       | `TestInit` (existing)                      | additive | add 1 row (Step 6)                                      |

Net new tests: ~22. No deletions. No existing test assertions changed.

---

## Risks and Edge Cases

- **Risk: YAML unmarshal cannot distinguish "unset" from explicit empty string.**
  **Mitigation:** JSON Schema's `minLength: 1` on `output.report` and `output.events` rejects explicit empties at schema-compile time (exit 3 via parser). In Go structs, zero values are treated as "not declared" consistently across all three precedence layers — so the practical semantics match the user's mental model.

- **Risk: Relocating the events-emitter-open block breaks early-fail guarantees.**
  **Mitigation:** Events-emitter open must still happen before any HTTP request runs. `parser.ParseFileWithOptions`, `LoadProjectConfig`, `LoadEnvironment`, `LoadTeamTemplate`, `LoadDotenv` all happen earlier — none of them perform network I/O. Moving `OpenFile(flags.events, ...)` to after `resolveOutputPrecedence` still keeps it strictly before any outbound HTTP. New ordering: `ParseFileWithOptions → LoadEnvironment → LoadProjectConfig → resolveOutputPrecedence → OpenFile(events) → feature-gate checks → runner.Run`. **Verify by auditing all `return` paths between current and new events-open location — none perform HTTP.**

- **Risk: `col.Output` is set but `projectCfg` is the zero-value stub (no apitest.yaml found).**
  **Mitigation:** `LoadProjectConfig` returns `&ProjectConfig{Variables: map[string]string{}}` with `Output == nil` when no root is found (line 163). `resolveOutputPrecedence` nil-checks projOut. Collection-declared output still takes effect.

- **Risk: Feature-gate checks at `main.go:554-590` read `format` from the pre-resolution struct.**
  **Mitigation:** Relocate both junit/html gate blocks to **after** `resolveOutputPrecedence`. Their exit codes (6 for gate failure) are preserved.

- **Edge case: `output.format: html` set by project but `output.report` unset.**
  **Handling:** The existing check "html requires --report" remains in place and now operates on resolved values. It returns exit 1 with a human-readable error — correct behaviour.

- **Edge case: Project has `output.events: ev.jsonl`, CLI passes `--events /dev/null`.**
  **Handling:** `flags.eventsSet` is true → CLI path wins. Exactly the spec precedence.

- **Edge case: Watch mode.**
  **Handling:** Watch's `RunFunc` calls `runCmdInner` on each file-change. Each invocation re-runs precedence resolution against the re-parsed collection — correct by construction.

- **Edge case: `TestSchema_examples` walker picks up new `output_*.yaml` fixtures.**
  **Handling:** All new fixtures are well-formed and valid; walker passes. If any fixture happens to be project-shaped (no `requests:` block), the walker's current code will fail because it uses only `compileCollectionSchema`. **Mitigation: keep all walker-scoped fixtures collection-shaped; project-shaped fixtures live in a separate file pattern (e.g., `output_project_*.yaml`) and are excluded from the walker by updating `TestSchema_examples` to skip `output_project_*` or to validate project fixtures against `compileProjectSchema`.** Simplest path: include project-flavor fixtures, exclude them from the collection walker glob by using a filename prefix convention (`project_*.yaml`) and enhance the walker to route by prefix.

- **Risk: `examples/output-block/*.yaml` fixtures are not walked today, but a future M8 task may extend the walker.**
  **Mitigation:** Document the convention in a comment at top of `examples/output-block/bad-format.yaml` ("Intentionally invalid per M8-003 observable — do not add to TestSchema_examples walker").

---

## Proposed Go Function Signatures

```go
// internal/output/config.go
type Config struct {
	Format    string `yaml:"format,omitempty"`
	Report    string `yaml:"report,omitempty"`
	Events    string `yaml:"events,omitempty"`
	Verbosity string `yaml:"verbosity,omitempty"`
}
func (c *Config) Validate() error
func ParseVerbosity(s string) (Verbosity, bool, error)
var SupportedFormats []string
var ErrUnknownFormat error
var ErrEmptyReportPath error
var ErrEmptyEventsPath error

// internal/schema/schema.go
var ProjectSchema = schemas.ProjectV1

// cmd/apitest/main.go
func resolveOutputPrecedence(flags *runFlags, colOut, projOut *output.Config) error
func parseSchemaArgs(args []string) (format string, project bool, err error) // signature change

// internal/parser/collection.go
type Collection struct {
	// ... existing
	Output *output.Config `yaml:"output,omitempty"` // M8-003
}

// internal/config/project.go
type ProjectConfig struct {
	// ... existing
	Output *output.Config // M8-003
}
```

---

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):

```bash
# 1. Schemas accept output: at both levels.
go test -run TestSchema_accepts_output ./internal/schema/...

# 2. New project schema exists at the stable versioned path.
ls -la schemas/project-v1.json

# 3. apitest schema --project emits the project schema.
./apitest schema --project | jq -r .title                           # "ApiTest Project v1"
./apitest schema --project | jq -r '.["$id"]'                       # https://.../project-v1.json

# 4. apitest schema (no flag) still emits the collection schema.
./apitest schema | jq -r .title                                     # "ApiTest Collection v1"

# 5. Precedence: CLI > collection > project > built-in.
go test -run TestOutputPrecedence ./cmd/apitest/...

# 6. End-to-end: project-level output.format=json → valid JSON on stdout.
./apitest run examples/output-block/collection.yaml | jq -r '.summary.total'

# 7. Unknown format in YAML fails before any request runs.
./apitest run examples/output-block/bad-format.yaml                 # exit 3

# 8. apitest init scaffolds an active output block in apitest.yaml.
cd "$(mktemp -d)" && /path/to/apitest init --project-name demo && grep -A3 '^output:' apitest.yaml
```

Coverage gate:

```bash
go test -cover \
  ./internal/schema/... \
  ./cmd/apitest/... \
  ./internal/config/... \
  ./internal/parser/... \
  ./internal/output/...
# Expect >= 80% in each listed package.
```

---

## TDD sequence (commit shape)

1. `test(output): failing Config unmarshal and Validate tests` (RED) — Step 1 tests.
2. `feat(output): introduce output.Config with format/verbosity parsing` (GREEN) — Step 1 impl.
3. `test(schemas): failing project schema drift + $defs.output match tests` (RED) — Step 2 tests.
4. `feat(schemas): publish project-v1.json + extend collection-v1.json with output` (GREEN) — Step 2 impl (schema files + embeds + alias).
5. `test(schema): failing accept/reject tests for output block` (RED) — Step 2 schema-accept/reject tests.
6. (GREEN fold-in above — the schema edits already satisfy these.)
7. `test(parser,config): failing round-trip tests for Collection.Output and ProjectConfig.Output` (RED) — Step 3 tests.
8. `feat(parser,config): thread Output field through YAML unmarshal` (GREEN) — Step 3 impl.
9. `test(cmd): failing TestOutputPrecedence four-case table` (RED) — Step 4 tests.
10. `feat(cmd): resolve output precedence once before formatter/events construction` (GREEN) — Step 4 impl + 4a relocation.
11. `test(cmd): failing schema --project flag tests` (RED) — Step 5 tests.
12. `feat(cmd): apitest schema --project emits project schema` (GREEN) — Step 5 impl.
13. `test(scaffold,cmd,schema): failing init --project-name + scaffolded apitest.yaml validates` (RED) — Step 6 tests.
14. `feat(scaffold,cmd): emit active output block and accept --project-name` (GREEN) — Step 6 impl.
15. `chore(examples): add examples/output-block fixtures` — Step 7.
16. `docs(manual,changelog): document output block + project schema snippet` — Step 8.

Each pair must keep `go build ./cmd/apitest && go test ./...` green before the next RED begins.

