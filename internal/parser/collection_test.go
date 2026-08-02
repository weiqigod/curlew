package parser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/peterlindqvist/apitest/internal/output"
)

// TestRequestItem_UnmarshalYAML_capturesLine verifies that RequestItem's
// custom UnmarshalYAML captures the YAML node's line number in SourceLine.
// SourceFile is left empty at this level — it is stamped by the parser
// once the file path is known (see TestParse_CarriesSourceLocation).
func TestRequestItem_UnmarshalYAML_capturesLine(t *testing.T) {
	doc := `
- name: alpha
  request:
    method: GET
    url: https://example.com/a
- name: beta
  request:
    method: GET
    url: https://example.com/b
`
	var items []RequestItem
	if err := yaml.Unmarshal([]byte(doc), &items); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].SourceLine != 2 { // 1-based; leading newline in backtick string puts alpha at line 2
		t.Errorf("items[0].SourceLine = %d, want 2", items[0].SourceLine)
	}
	if items[1].SourceLine != 6 {
		t.Errorf("items[1].SourceLine = %d, want 6", items[1].SourceLine)
	}
	if items[0].SourceFile != "" {
		t.Errorf("items[0].SourceFile = %q, want empty (stamped at higher level)", items[0].SourceFile)
	}
}

// TestParse_CarriesSourceLocation verifies that ParseFile populates SourceFile
// and SourceLine on every RequestItem from a direct collection file.
func TestParse_CarriesSourceLocation(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "src-demo.yaml")
	content := `name: demo
requests:
  - name: alpha
    request:
      method: GET
      url: https://example.com/a
  - name: beta
    request:
      method: GET
      url: https://example.com/b
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	col, err := ParseFile(file)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(col.Requests.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(col.Requests.Items))
	}

	// Use EvalSymlinks for platform portability (macOS /var → /private/var).
	absFile, _ := filepath.EvalSymlinks(file)
	for i, want := range []struct {
		name string
		line int
	}{
		{"alpha", 3},
		{"beta", 7},
	} {
		ri := col.Requests.Items[i]
		if ri.Name != want.name {
			t.Fatalf("Requests.Items[%d].Name = %q, want %q", i, ri.Name, want.name)
		}
		actualFile, _ := filepath.EvalSymlinks(ri.SourceFile)
		if actualFile != absFile {
			t.Errorf("Requests.Items[%d].SourceFile = %q, want %q", i, ri.SourceFile, absFile)
		}
		if ri.SourceLine != want.line {
			t.Errorf("Requests.Items[%d].SourceLine = %d, want %d", i, ri.SourceLine, want.line)
		}
	}
}

// TestParse_ExternalRequestCarriesExternalFilePath verifies that a request
// loaded via `path:` (request_file) carries the external file path as
// SourceFile and SourceLine == 1.
func TestParse_ExternalRequestCarriesExternalFilePath(t *testing.T) {
	dir := t.TempDir()
	extFile := filepath.Join(dir, "get-user.yaml")
	ext := `name: Get User
request:
  method: GET
  url: https://example.com/users/1
`
	if err := os.WriteFile(extFile, []byte(ext), 0o600); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(dir, "parent.yaml")
	pc := `name: parent
requests:
  - path: get-user.yaml
`
	if err := os.WriteFile(parent, []byte(pc), 0o600); err != nil {
		t.Fatal(err)
	}

	col, err := ParseFile(parent)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(col.Requests.Items))
	}
	// Use EvalSymlinks for platform portability (macOS /var → /private/var).
	absExt, _ := filepath.EvalSymlinks(extFile)
	actualSrc, _ := filepath.EvalSymlinks(col.Requests.Items[0].SourceFile)
	if actualSrc != absExt {
		t.Errorf("SourceFile = %q, want %q", col.Requests.Items[0].SourceFile, absExt)
	}
	if col.Requests.Items[0].SourceLine != 1 {
		t.Errorf("SourceLine = %d, want 1", col.Requests.Items[0].SourceLine)
	}
}

// TestParse_IncludesCarryIncludedFilePath verifies that requests contributed
// by an included file carry the included file's path in SourceFile (not the
// including parent's path).
func TestParse_IncludesCarryIncludedFilePath(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "child.yaml")
	cc := `name: child
requests:
  - name: child_req
    request:
      method: GET
      url: https://example.com/child
`
	if err := os.WriteFile(child, []byte(cc), 0o600); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(dir, "parent.yaml")
	pc := `name: parent
include:
  - ./child.yaml
requests:
  - name: parent_req
    request:
      method: GET
      url: https://example.com/parent
`
	if err := os.WriteFile(parent, []byte(pc), 0o600); err != nil {
		t.Fatal(err)
	}

	col, err := ParseFile(parent)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	absParent, _ := filepath.EvalSymlinks(parent)
	absChild, _ := filepath.EvalSymlinks(child)

	var parentItem, childItem *RequestItem
	for i := range col.Requests.Items {
		switch col.Requests.Items[i].Name {
		case "parent_req":
			parentItem = &col.Requests.Items[i]
		case "child_req":
			childItem = &col.Requests.Items[i]
		}
	}
	if parentItem == nil || childItem == nil {
		t.Fatalf("parent or child request missing: %+v", col.Requests.Items)
	}
	// Resolve symlinks on the actual results too for platform portability.
	actualParent, _ := filepath.EvalSymlinks(parentItem.SourceFile)
	actualChild, _ := filepath.EvalSymlinks(childItem.SourceFile)
	if actualParent != absParent {
		t.Errorf("parent SourceFile = %q, want %q", parentItem.SourceFile, absParent)
	}
	if actualChild != absChild {
		t.Errorf("child SourceFile = %q, want %q", childItem.SourceFile, absChild)
	}
	if childItem.SourceLine == 0 {
		t.Errorf("child SourceLine = 0, want > 0")
	}
}

// TestParser_SigningField_RequestLevel verifies that a request-level signing:
// field is parsed into RequestItem.Signing correctly.
func TestParser_SigningField_RequestLevel(t *testing.T) {
	col, err := ParseFile("testdata/with_signing_request.yaml")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("expected 1 request, got %d", len(col.Requests.Items))
	}
	sp := col.Requests.Items[0].Signing
	if sp == nil {
		t.Fatal("expected RequestItem.Signing to be non-nil")
	}
	if sp.Type != "noop" {
		t.Errorf("Signing.Type = %q, want noop", sp.Type)
	}
	if sp.Params["key"] != "value" {
		t.Errorf("Signing.Params[key] = %v, want value", sp.Params["key"])
	}
	if sp.IsExplicitNull() {
		t.Error("non-null YAML must not flag IsExplicitNull")
	}
}

// TestParser_SigningField_CollectionDefault verifies that collection-level
// signing: applies as a default, per-request signing: overrides, and
// explicit per-request signing: null disables signing.
func TestParser_SigningField_CollectionDefault(t *testing.T) {
	col, err := ParseFile("testdata/with_signing_collection_default.yaml")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if col.Signing == nil {
		t.Fatal("expected Collection.Signing to be non-nil")
	}
	if col.Signing.Type != "noop" {
		t.Errorf("Collection.Signing.Type = %q, want noop", col.Signing.Type)
	}

	// Three requests in fixture: inherits, overrides, explicit null.
	if len(col.Requests.Items) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(col.Requests.Items))
	}
	inherits := col.Requests.Items[0]
	overrides := col.Requests.Items[1]
	nulled := col.Requests.Items[2]

	if inherits.Signing != nil {
		t.Errorf("inheriting request must have Signing nil; got %+v", inherits.Signing)
	}
	if overrides.Signing == nil || overrides.Signing.Type != "other-noop" {
		t.Errorf("overriding request must use its own spec; got %+v", overrides.Signing)
	}
	if nulled.Signing == nil || !nulled.Signing.IsExplicitNull() {
		t.Errorf("nulled request must have non-nil Signing with IsExplicitNull=true; got %+v", nulled.Signing)
	}
}

// TestParser_SigningField_MissingType verifies that a signing: block without
// a type: field is rejected with a descriptive error.
func TestParser_SigningField_MissingType(t *testing.T) {
	_, err := ParseFile("testdata/signing_missing_type.yaml")
	if err == nil {
		t.Fatal("expected error for signing without type")
	}
	if !strings.Contains(err.Error(), "signing.type is required") {
		t.Errorf("error must mention required type, got %q", err.Error())
	}
}

// TestParser_SigningField_NonMapping verifies that a signing: field with a
// scalar string value is rejected with a descriptive error.
func TestParser_SigningField_NonMapping(t *testing.T) {
	_, err := ParseFile("testdata/signing_non_mapping.yaml")
	if err == nil {
		t.Fatal("expected error for signing as scalar string")
	}
}

// TestCollection_Output_Roundtrip verifies that the optional output: block is
// correctly unmarshalled into Collection.Output.
func TestCollection_Output_Roundtrip(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want *output.Config
	}{
		{
			"no_output_block",
			"name: x\nrequests:\n  - name: r\n    request:\n      method: GET\n      url: u\n",
			nil,
		},
		{
			"output_format_only",
			"name: x\noutput:\n  format: json\nrequests:\n  - name: r\n    request:\n      method: GET\n      url: u\n",
			&output.Config{Format: "json"},
		},
		{
			"output_all_fields",
			"name: x\noutput:\n  format: tap\n  report: out.txt\n  events: ev.jsonl\n  verbosity: verbose\nrequests:\n  - name: r\n    request:\n      method: GET\n      url: u\n",
			&output.Config{Format: "tap", Report: "out.txt", Events: "ev.jsonl", Verbosity: "verbose"},
		},
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

// TestParse_CelAssertion_ScalarForm verifies that cel: under assertions: accepts
// a list of plain strings as CEL expressions.
func TestParse_CelAssertion_ScalarForm(t *testing.T) {
	yamlDoc := `
name: c
requests:
  - name: r
    request:
      method: GET
      url: http://example.com
    assertions:
      cel:
        - "response.status == 200"
        - "response.body.total > 0"
`
	col, err := parseCollectionBytes("test.yaml", []byte(yamlDoc))
	if err != nil {
		t.Fatalf("parseCollectionBytes: %v", err)
	}
	items := col.Requests.Items[0].Assertions.CEL.Items
	if len(items) != 2 {
		t.Fatalf("want 2 cel assertions, got %d", len(items))
	}
	if items[0].Source != "response.status == 200" {
		t.Errorf("source[0] = %q, want %q", items[0].Source, "response.status == 200")
	}
	if items[1].Source != "response.body.total > 0" {
		t.Errorf("source[1] = %q, want %q", items[1].Source, "response.body.total > 0")
	}
}

// TestParse_CelAssertion_MappingForm verifies that cel: accepts the explicit
// mapping form with a single cel: key per entry.
func TestParse_CelAssertion_MappingForm(t *testing.T) {
	yamlDoc := `
name: c
requests:
  - name: r
    request:
      method: GET
      url: http://example.com
    assertions:
      cel:
        - cel: "response.status == 200"
`
	col, err := parseCollectionBytes("test.yaml", []byte(yamlDoc))
	if err != nil {
		t.Fatalf("parseCollectionBytes: %v", err)
	}
	items := col.Requests.Items[0].Assertions.CEL.Items
	if len(items) != 1 {
		t.Fatalf("want 1 cel assertion, got %d", len(items))
	}
	if got := items[0].Source; got != "response.status == 200" {
		t.Errorf("source = %q, want %q", got, "response.status == 200")
	}
}

// TestParse_CelAssertion_MutualExclusionWithOperator verifies that a cel:
// entry that also carries an operator key is rejected at parse time.
func TestParse_CelAssertion_MutualExclusionWithOperator(t *testing.T) {
	yamlDoc := `
name: c
requests:
  - name: r
    request:
      method: GET
      url: http://example.com
    assertions:
      cel:
        - cel: "response.status == 200"
          eq: 200
`
	_, err := parseCollectionBytes("test.yaml", []byte(yamlDoc))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, ErrCelAndOperatorMutuallyExclusive) {
		t.Errorf("want ErrCelAndOperatorMutuallyExclusive, got %v", err)
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want 'mutually exclusive'", err)
	}
}

// ---- Step 5: Collection config.locale ----

func TestParseCollection_Locale(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantLocale string
	}{
		{
			name: "config block with locale",
			yaml: `name: Test
config:
  locale: de-DE
requests:
  items: []
`,
			wantLocale: "de-DE",
		},
		{
			name: "no config block returns empty locale",
			yaml: `name: Test
requests:
  items: []
`,
			wantLocale: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			col, err := parseCollectionBytes("test.yaml", []byte(tc.yaml))
			if err != nil {
				t.Fatalf("parseCollectionBytes: %v", err)
			}
			if col.Config.Locale != tc.wantLocale {
				t.Errorf("Config.Locale = %q, want %q", col.Config.Locale, tc.wantLocale)
			}
		})
	}
}
