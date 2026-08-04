package schema_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/weiqigod/curlew/internal/schema"
)

// Two parser types decode from YAML with a hand-rolled key switch instead of
// struct tags, so reflection cannot discover the keys they accept. The parity
// table hardcodes those key sets; the helpers here read the same keys back out
// of the source with go/ast so the hardcoded lists cannot go stale silently.

// caseStrings returns every string literal used as a switch case value inside
// the named method of the named receiver type. Used to recover the accepted
// YAML keys of a hand-rolled UnmarshalYAML.
func caseStrings(t *testing.T, file, recvType, method string) []string {
	t.Helper()
	fn := findMethod(t, file, recvType, method)
	var keys []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		cc, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, expr := range cc.List {
			lit, ok := expr.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("unquote %s in %s.%s: %v", lit.Value, recvType, method, err)
			}
			keys = append(keys, s)
		}
		return true
	})
	if len(keys) == 0 {
		t.Fatalf("%s.%s: no string case clauses found — has the decoder been rewritten?", recvType, method)
	}
	sort.Strings(keys)
	return keys
}

// findMethod locates a method declaration by receiver type name and method name.
func findMethod(t *testing.T, file, recvType, method string) *ast.FuncDecl {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != method || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		if receiverTypeName(fn.Recv.List[0].Type) == recvType {
			return fn
		}
	}
	t.Fatalf("%s: method %s.%s not found", file, recvType, method)
	return nil
}

// receiverTypeName unwraps a pointer receiver to its bare type name.
func receiverTypeName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// structYAMLTags returns the yaml tag names declared on the named struct type,
// skipping `yaml:"-"`. Used for unexported structs that reflection in another
// package cannot reach.
func structYAMLTags(t *testing.T, file, typeName string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var tags []string
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != typeName {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			t.Fatalf("%s: %s is not a struct", file, typeName)
		}
		for _, field := range st.Fields.List {
			if field.Tag == nil {
				continue
			}
			raw, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				t.Fatalf("unquote tag on %s: %v", typeName, err)
			}
			name, _ := splitYAMLTagValue(reflect.StructTag(raw).Get("yaml"))
			if name == "" || name == "-" {
				continue
			}
			tags = append(tags, name)
		}
		return false
	})
	if len(tags) == 0 {
		t.Fatalf("%s: struct %s has no yaml tags — has it been renamed?", file, typeName)
	}
	sort.Strings(tags)
	return tags
}

// TestSchema_hand_rolled_decoder_keys_match_source pins the parity table's
// hardcoded key lists to the switch statements they describe. Without this, a
// key added to WebSocketStep.UnmarshalYAML would be accepted by the parser,
// missing from the schema, and invisible to the parity test.
func TestSchema_hand_rolled_decoder_keys_match_source(t *testing.T) {
	collectionGo := parserCollectionPath(t)
	tests := []struct {
		name     string
		recvType string
		method   string
		want     []string
	}{
		{"websocket_step_switch", "WebSocketStep", "UnmarshalYAML", webSocketStepKeys},
		{"variable_entry_switch", "SensitiveVars", "parseObjectVar", variableEntryKeys},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := caseStrings(t, collectionGo, tc.recvType, tc.method)
			for _, d := range symmetricDiff(got, tc.want) {
				t.Errorf("%s.%s: %s", tc.recvType, tc.method, d)
			}
		})
	}
}

// TestSchema_project_root_properties_match_project_file asserts that the root
// properties of project-v1.json equal the yaml tags on internal/config's
// projectFile struct. projectFile is unexported, so its tags are read from
// source rather than by reflection.
func TestSchema_project_root_properties_match_project_file(t *testing.T) {
	want := structYAMLTags(t, projectConfigPath(t), "projectFile")
	got := sortedKeys(schemaPropertyNames(t, decodeSchemaDoc(t, schema.ProjectSchema)))
	for _, d := range symmetricDiff(got, want) {
		t.Errorf("project-v1.json root vs projectFile: %s", d)
	}
}
