package schema_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/weiqigod/curlew/internal/scaffold"
	"github.com/weiqigod/curlew/internal/schema"
)

// publishedSchemaPath returns the absolute path to schemas/collection-v1.json
// by walking up from this test file to the repo root.
func publishedSchemaPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "schemas", "collection-v1.json")
}

// compileCollectionSchema compiles the embedded collection JSON Schema.
func compileCollectionSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.CollectionSchema))
	if err != nil {
		t.Fatalf("unmarshal embedded schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	const resourceID = "memory://collection-v1.json"
	if err := c.AddResource(resourceID, doc); err != nil {
		t.Fatalf("AddResource: %v", err)
	}
	sch, err := c.Compile(resourceID)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return sch
}

// decodeYAMLFile unmarshals a YAML file into a generic Go value and normalizes
// it for JSON Schema validation (map[any]any → map[string]any).
func decodeYAMLFile(t *testing.T, path string) any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("yaml unmarshal %s: %v", path, err)
	}
	return normalizeForJSONSchema(doc)
}

// normalizeForJSONSchema converts any map[any]any keys (yaml-compat) to
// map[string]any so santhosh-tekuri/jsonschema can walk them. yaml.v3 usually
// returns map[string]any directly, but nested mappings can still surprise us.
func normalizeForJSONSchema(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, vv := range x {
			x[k] = normalizeForJSONSchema(vv)
		}
		return x
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[fmt.Sprint(k)] = normalizeForJSONSchema(vv)
		}
		return out
	case []any:
		for i, vv := range x {
			x[i] = normalizeForJSONSchema(vv)
		}
		return x
	default:
		return v
	}
}

// TestSchema_validates_scaffolded_sample is the DoD test: the JSON Schema
// validates the collections/sample.yaml file produced by `curlew init`.
func TestSchema_validates_scaffolded_sample(t *testing.T) {
	tmp := t.TempDir()
	if err := scaffold.Init(scaffold.Options{Dir: tmp}); err != nil {
		t.Fatalf("scaffold.Init: %v", err)
	}
	samplePath := filepath.Join(tmp, "collections", "sample.yaml")
	doc := decodeYAMLFile(t, samplePath)
	sch := compileCollectionSchema(t)
	if err := sch.Validate(doc); err != nil {
		t.Fatalf("validate sample.yaml: %v", err)
	}
}

// TestSchema_rejects_sample_missing_required_name is the negative control:
// dropping the required top-level `name` field must cause validation to fail.
func TestSchema_rejects_sample_missing_required_name(t *testing.T) {
	tmp := t.TempDir()
	if err := scaffold.Init(scaffold.Options{Dir: tmp}); err != nil {
		t.Fatalf("scaffold.Init: %v", err)
	}
	samplePath := filepath.Join(tmp, "collections", "sample.yaml")
	doc := decodeYAMLFile(t, samplePath)

	m, ok := doc.(map[string]any)
	if !ok {
		t.Fatalf("sample.yaml decoded to %T, want map[string]any", doc)
	}
	delete(m, "name")

	sch := compileCollectionSchema(t)
	err := sch.Validate(m)
	if err == nil {
		t.Fatal("validate: want error for missing required 'name', got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("validate error should mention 'name': %v", err)
	}
}

// TestSchema_published_path_matches_embed is the drift guard: the file on disk
// at schemas/collection-v1.json must be byte-identical to the embedded bytes
// exposed by schema.CollectionSchema.
func TestSchema_published_path_matches_embed(t *testing.T) {
	path := publishedSchemaPath(t)
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !bytes.Equal(fileBytes, schema.CollectionSchema) {
		t.Fatalf("schemas/collection-v1.json (%d bytes) differs from schema.CollectionSchema (%d bytes) — published file and embed are out of sync", len(fileBytes), len(schema.CollectionSchema))
	}
}

// TestSchema_file_exists_at_published_path is the cheap reachability guard:
// the published file must exist and be non-empty.
func TestSchema_file_exists_at_published_path(t *testing.T) {
	path := publishedSchemaPath(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("schemas/collection-v1.json missing at %s: %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("schemas/collection-v1.json is empty at %s", path)
	}
}

// publishedProjectSchemaPath returns the absolute path to schemas/project-v1.json.
func publishedProjectSchemaPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "schemas", "project-v1.json")
}

// compileProjectSchema compiles the embedded project JSON Schema.
func compileProjectSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.ProjectSchema))
	if err != nil {
		t.Fatalf("unmarshal embedded project schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	const resourceID = "memory://project-v1.json"
	if err := c.AddResource(resourceID, doc); err != nil {
		t.Fatalf("AddResource: %v", err)
	}
	sch, err := c.Compile(resourceID)
	if err != nil {
		t.Fatalf("Compile project schema: %v", err)
	}
	return sch
}

// TestSchema_project_published_path_matches_embed is the drift guard for the
// project schema: the file on disk at schemas/project-v1.json must be
// byte-identical to the embedded bytes exposed by schema.ProjectSchema.
func TestSchema_project_published_path_matches_embed(t *testing.T) {
	path := publishedProjectSchemaPath(t)
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !bytes.Equal(fileBytes, schema.ProjectSchema) {
		t.Fatalf("schemas/project-v1.json (%d bytes) differs from schema.ProjectSchema (%d bytes) — published file and embed are out of sync", len(fileBytes), len(schema.ProjectSchema))
	}
}

// TestSchema_project_file_exists_at_published_path is the cheap reachability guard:
// the published project file must exist and be non-empty.
func TestSchema_project_file_exists_at_published_path(t *testing.T) {
	info, err := os.Stat(publishedProjectSchemaPath(t))
	if err != nil {
		t.Fatalf("schemas/project-v1.json missing: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("schemas/project-v1.json is empty")
	}
}

// TestSchema_output_defs_match verifies that the $defs.output subschema is
// byte-identical across both schema files (collection and project).
func TestSchema_output_defs_match(t *testing.T) {
	var col, proj map[string]any
	if err := json.Unmarshal(schema.CollectionSchema, &col); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(schema.ProjectSchema, &proj); err != nil {
		t.Fatal(err)
	}
	colDefs, ok := col["$defs"].(map[string]any)
	if !ok {
		t.Fatal("collection schema missing $defs")
	}
	projDefs, ok := proj["$defs"].(map[string]any)
	if !ok {
		t.Fatal("project schema missing $defs")
	}
	colOut, ok := colDefs["output"]
	if !ok {
		t.Fatal("collection schema $defs missing output")
	}
	projOut, ok := projDefs["output"]
	if !ok {
		t.Fatal("project schema $defs missing output")
	}
	colBytes, _ := json.Marshal(colOut)
	projBytes, _ := json.Marshal(projOut)
	if !bytes.Equal(colBytes, projBytes) {
		t.Fatalf("$defs.output differs:\ncollection: %s\nproject:    %s", colBytes, projBytes)
	}
}

// TestSchema_config_defs_match mirrors TestSchema_output_defs_match: the
// config: block has the same shape in a collection and in curlew.yaml, and JSON
// Schema offers no cross-file include an editor can resolve offline, so the
// definition is duplicated and pinned here.
func TestSchema_config_defs_match(t *testing.T) {
	var col, proj map[string]any
	if err := json.Unmarshal(schema.CollectionSchema, &col); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(schema.ProjectSchema, &proj); err != nil {
		t.Fatal(err)
	}
	colCfg, ok := col["$defs"].(map[string]any)["config"]
	if !ok {
		t.Fatal("collection schema $defs missing config")
	}
	projCfg, ok := proj["$defs"].(map[string]any)["config"]
	if !ok {
		t.Fatal("project schema $defs missing config")
	}
	colBytes, _ := json.Marshal(colCfg)
	projBytes, _ := json.Marshal(projCfg)
	if !bytes.Equal(colBytes, projBytes) {
		t.Fatalf("$defs.config differs:\ncollection: %s\nproject:    %s", colBytes, projBytes)
	}
}

// TestSchema_ids_match_module_path pins the $id of both published schemas to
// the Go module path's org. MANUAL §1.5 hands out-of-tree users these URLs to
// wire into yaml.schemas, so an org segment that does not match the remote
// gives them a 404 instead of a schema.
func TestSchema_ids_match_module_path(t *testing.T) {
	const org = "weiqigod"
	tests := []struct {
		name string
		raw  []byte
		file string
	}{
		{"collection", schema.CollectionSchema, "collection-v1.json"},
		{"project", schema.ProjectSchema, "project-v1.json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal(tc.raw, &m); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			want := "https://raw.githubusercontent.com/" + org + "/curlew/main/schemas/" + tc.file
			if m["$id"] != want {
				t.Errorf("$id = %v, want %v", m["$id"], want)
			}
		})
	}
}

// TestSchema_scaffolded_curlew_yaml_validates verifies that the file produced
// by scaffold.Init validates against schemas/project-v1.json.
func TestSchema_scaffolded_curlew_yaml_validates(t *testing.T) {
	tmp := t.TempDir()
	if err := scaffold.Init(scaffold.Options{Dir: tmp}); err != nil {
		t.Fatal(err)
	}
	doc := decodeYAMLFile(t, filepath.Join(tmp, "curlew.yaml"))
	sch := compileProjectSchema(t)
	if err := sch.Validate(doc); err != nil {
		t.Fatalf("curlew.yaml did not validate against project schema: %v", err)
	}
}

// TestSchema_scaffolded_all_output_formats_validate verifies that each supported
// --output format produces an curlew.yaml that validates against the project schema
// end-to-end. This satisfies the DoD item: "each of terminal/json/tap/junit/html/markdown
// produces a scaffold that the config validator accepts end-to-end".
func TestSchema_scaffolded_all_output_formats_validate(t *testing.T) {
	cases := []struct {
		name string
		opts scaffold.Options
	}{
		{"terminal", scaffold.Options{OutputFormat: "terminal"}},
		{"json", scaffold.Options{OutputFormat: "json"}},
		{"tap", scaffold.Options{OutputFormat: "tap"}},
		{"junit", scaffold.Options{OutputFormat: "junit"}},
		{"html", scaffold.Options{OutputFormat: "html"}},
		{"markdown", scaffold.Options{OutputFormat: "markdown"}},
		{"skill_claude", scaffold.Options{SkillName: "claude", CurlewVersion: "test"}},
	}
	sch := compileProjectSchema(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			opts := tc.opts
			opts.Dir = tmp
			if err := scaffold.Init(opts); err != nil {
				t.Fatalf("scaffold.Init(%s): %v", tc.name, err)
			}
			doc := decodeYAMLFile(t, filepath.Join(tmp, "curlew.yaml"))
			if err := sch.Validate(doc); err != nil {
				t.Fatalf("curlew.yaml for %s did not validate against project schema: %v", tc.name, err)
			}
		})
	}
}

// TestSchema_skill_claude_scaffold_matches_fixture asserts that the scaffolded
// curlew.yaml for --skill agent contains the output: block from the checked-in
// fixture file.
func TestSchema_skill_claude_scaffold_matches_fixture(t *testing.T) {
	tmp := t.TempDir()
	if err := scaffold.Init(scaffold.Options{
		Dir:           tmp,
		ProjectName:   "demo",
		SkillName:     "claude",
		CurlewVersion: "test",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(tmp, "curlew.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "output_project_skill_claude.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// The fixture covers only the output: block. We assert the fixture's
	// output: block is a substring of the scaffold output, which is the
	// invariant that matters for schema parity.
	outputIdx := bytes.Index(want, []byte("output:"))
	if outputIdx < 0 {
		t.Fatalf("fixture file missing output: block")
	}
	if !bytes.Contains(got, want[outputIdx:]) {
		t.Errorf("scaffold output: block diverged from fixture\nwant suffix:\n%s\ngot:\n%s", want[outputIdx:], got)
	}
}
