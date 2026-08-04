package schema_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/datadriven"
	"github.com/weiqigod/curlew/internal/output"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/retry"
	"github.com/weiqigod/curlew/internal/schema"
)

// The collection schema is maintained by hand, and both $defs.requestItem and
// $defs.request declare additionalProperties: false. That combination makes an
// undescribed parser field a functional defect rather than a documentation gap:
// an editor wired up per MANUAL §1.5 reports an error on valid YAML. The tests
// in this file walk the parser structs and fail in both directions — a parser
// field the schema does not describe, and a schema property the parser would
// silently ignore.

// parityCase binds a location in the collection schema to the parser type that
// decodes it. Types that decode with a hand-rolled key switch carry no yaml
// tags, so those rows supply keys explicitly; astsource_test.go pins those
// lists to the switch statements themselves.
type parityCase struct {
	name string
	path string       // "" is the document root
	typ  reflect.Type // reflection source; nil when keys is set
	keys []string     // explicit key set for tagless hand-rolled decoders
}

var collectionParity = []parityCase{
	{name: "collection_root", path: "", typ: reflect.TypeOf(parser.Collection{})},
	{name: "options", path: "properties/options", typ: reflect.TypeOf(parser.Options{})},
	{name: "section_object_form", path: "$defs/section/oneOf/1", typ: reflect.TypeOf(parser.Section{})},
	{name: "request_item", path: "$defs/requestItem", typ: reflect.TypeOf(parser.RequestItem{})},
	{name: "request", path: "$defs/request", typ: reflect.TypeOf(parser.Request{})},
	{name: "assertions", path: "$defs/assertions", typ: reflect.TypeOf(parser.Assertions{})},
	{name: "assertions_timing", path: "$defs/assertions/properties/timing", typ: reflect.TypeOf(parser.TimingAssertion{})},
	{name: "signing", path: "$defs/signing", typ: reflect.TypeOf(parser.SigningSpec{})},
	{name: "collection_config", path: "$defs/config", typ: reflect.TypeOf(parser.CollectionConfigBlock{})},
	{name: "graphql", path: "$defs/graphql", typ: reflect.TypeOf(parser.GraphQLConfig{})},
	{name: "websocket", path: "$defs/websocket", typ: reflect.TypeOf(parser.WebSocketConfig{})},
	{name: "websocket_reconnect", path: "$defs/websocketReconnect", typ: reflect.TypeOf(parser.ReconnectConfig{})},
	{name: "websocket_heartbeat", path: "$defs/websocketHeartbeat", typ: reflect.TypeOf(parser.HeartbeatConfig{})},
	{name: "websocket_step", path: "$defs/websocketStep", keys: webSocketStepKeys},
	{name: "variable_entry", path: "$defs/variableEntry", keys: variableEntryKeys},
	{name: "retry", path: "$defs/retry", typ: reflect.TypeOf(retry.FullConfig{})},
	{name: "retry_on", path: "$defs/retryOn", typ: reflect.TypeOf(retry.RetryOnConfig{})},
	{name: "do_not_retry_on", path: "$defs/doNotRetryOn", typ: reflect.TypeOf(retry.DoNotRetryOnConfig{})},
	{name: "data_driven", path: "$defs/dataDriven", typ: reflect.TypeOf(datadriven.Config{})},
	{name: "output", path: "$defs/output", typ: reflect.TypeOf(output.Config{})},
}

// webSocketStepKeys is the key set accepted by (*parser.WebSocketStep).UnmarshalYAML,
// which decodes with a key switch rather than yaml tags.
var webSocketStepKeys = []string{
	"action", "any_of", "code", "count", "duration_ms", "extract", "message",
	"message_raw", "message_template", "reason", "timeout_ms", "variables",
}

// variableEntryKeys is the key set accepted by (*parser.SensitiveVars).parseObjectVar.
var variableEntryKeys = []string{"cache", "from_command", "sensitive", "value"}

// opaqueTypes decode from a free-form map or from a scalar/sequence union, so
// the schema describes them without a one-to-one property list. The reflection
// walk stops at these rather than demanding a parity row.
var opaqueTypes = map[reflect.Type]string{
	reflect.TypeOf(parser.SensitiveVars{}):    "$defs/variables — free-form map with a custom UnmarshalYAML",
	reflect.TypeOf(parser.StatusCodes{}):      "assertions.status — integer or array of integers",
	reflect.TypeOf(parser.HeaderAssertions{}): "assertions.headers — header name to operator map",
	reflect.TypeOf(parser.BodyAssertions{}):   "assertions.body — JSONPath to operator map",
	reflect.TypeOf(parser.CELAssertions{}):    "$defs/celAssertions — string or {cel: string} per entry",
}

// yamlKeys returns the set of YAML mapping keys gopkg.in/yaml.v3 binds for typ.
// Pointers, slices and arrays are dereferenced to their element type.
func yamlKeys(t *testing.T, typ reflect.Type) map[string]bool {
	t.Helper()
	typ = deref(typ)
	if typ.Kind() != reflect.Struct {
		t.Fatalf("yamlKeys: %s is not a struct", typ)
	}
	keys := map[string]bool{}
	for i := range typ.NumField() {
		f := typ.Field(i)
		if f.PkgPath != "" { // unexported: invisible to yaml.v3
			continue
		}
		name, opts := splitYAMLTagValue(f.Tag.Get("yaml"))
		if name == "-" {
			continue
		}
		if opts["inline"] {
			if deref(f.Type).Kind() != reflect.Struct {
				t.Fatalf("yamlKeys: inline non-struct on %s.%s is not modelled", typ, f.Name)
			}
			for k := range yamlKeys(t, f.Type) {
				keys[k] = true
			}
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		keys[name] = true
	}
	return keys
}

// splitYAMLTagValue splits a raw yaml struct tag into its name and its options
// (omitempty, flow, inline).
func splitYAMLTagValue(tag string) (string, map[string]bool) {
	parts := strings.Split(tag, ",")
	opts := make(map[string]bool, len(parts))
	for _, o := range parts[1:] {
		opts[o] = true
	}
	return parts[0], opts
}

// deref unwraps pointer, slice and array types to their element type.
func deref(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}
	return typ
}

// parityKeys returns the YAML keys for a parity row, from reflection or from
// the row's explicit key list.
func parityKeys(t *testing.T, tc parityCase) []string {
	t.Helper()
	if tc.keys != nil {
		return tc.keys
	}
	return sortedKeys(yamlKeys(t, tc.typ))
}

// schemaObjectAt resolves a slash-separated path inside a decoded schema. ""
// is the document root; a numeric segment indexes a JSON array, so oneOf
// branches are addressable.
func schemaObjectAt(t *testing.T, doc map[string]any, path string) map[string]any {
	t.Helper()
	cur := any(doc)
	if path != "" {
		for _, seg := range strings.Split(path, "/") {
			switch node := cur.(type) {
			case map[string]any:
				next, ok := node[seg]
				if !ok {
					t.Fatalf("schema path %q: no key %q", path, seg)
				}
				cur = next
			case []any:
				i, err := strconv.Atoi(seg)
				if err != nil || i >= len(node) {
					t.Fatalf("schema path %q: bad array index %q", path, seg)
				}
				cur = node[i]
			default:
				t.Fatalf("schema path %q: %q is not traversable", path, seg)
			}
		}
	}
	m, ok := cur.(map[string]any)
	if !ok {
		t.Fatalf("schema path %q is not an object", path)
	}
	return m
}

// schemaPropertyNames returns the property names declared on a schema object.
func schemaPropertyNames(t *testing.T, obj map[string]any) map[string]bool {
	t.Helper()
	props, ok := obj["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema object has no properties: %v", sortedKeys(anyKeys(obj)))
	}
	names := make(map[string]bool, len(props))
	for k := range props {
		names[k] = true
	}
	return names
}

// decodeSchemaDoc unmarshals schema bytes into a generic map.
func decodeSchemaDoc(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	return m
}

func collectionSchemaBytes() []byte { return schema.CollectionSchema }
func projectSchemaBytes() []byte    { return schema.ProjectSchema }

// parserCollectionPath is the source file holding the collection parser types.
func parserCollectionPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "internal", "parser", "collection.go")
}

// projectConfigPath is the source file holding the curlew.yaml projectFile struct.
func projectConfigPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "internal", "config", "project.go")
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func anyKeys(m map[string]any) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

// symmetricDiff describes every element present in exactly one of the two sets.
func symmetricDiff(got, want []string) []string {
	inGot, inWant := map[string]bool{}, map[string]bool{}
	for _, s := range got {
		inGot[s] = true
	}
	for _, s := range want {
		inWant[s] = true
	}
	var out []string
	for _, s := range want {
		if !inGot[s] {
			out = append(out, fmt.Sprintf("%q is expected but absent", s))
		}
	}
	for _, s := range got {
		if !inWant[s] {
			out = append(out, fmt.Sprintf("%q is present but unexpected", s))
		}
	}
	sort.Strings(out)
	return out
}

// TestSchema_parser_fields_are_described is the forward guard: every YAML key
// the parser binds must be described at its schema location. Because both
// requestItem and request declare additionalProperties: false, an undescribed
// key turns a valid collection into an editor error.
func TestSchema_parser_fields_are_described(t *testing.T) {
	doc := decodeSchemaDoc(t, collectionSchemaBytes())
	for _, tc := range collectionParity {
		t.Run(tc.name, func(t *testing.T) {
			props := schemaPropertyNames(t, schemaObjectAt(t, doc, tc.path))
			for _, k := range parityKeys(t, tc) {
				if !props[k] {
					t.Errorf("parser accepts %q but schema %q does not describe it "+
						"— additionalProperties:false makes this a false error in editors", k, tc.path)
				}
			}
		})
	}
}

// TestSchema_properties_have_parser_fields is the converse guard: a property
// the parser would silently ignore is a documentation lie, because it
// autocompletes a key that does nothing.
func TestSchema_properties_have_parser_fields(t *testing.T) {
	doc := decodeSchemaDoc(t, collectionSchemaBytes())
	for _, tc := range collectionParity {
		t.Run(tc.name, func(t *testing.T) {
			keys := map[string]bool{}
			for _, k := range parityKeys(t, tc) {
				keys[k] = true
			}
			for _, p := range sortedKeys(schemaPropertyNames(t, schemaObjectAt(t, doc, tc.path))) {
				if !keys[p] {
					t.Errorf("schema %q describes %q but the parser binds no such key", tc.path, p)
				}
			}
		})
	}
}

// TestSchema_parity_table_covers_every_parser_struct closes the loop on the
// table itself: a newly added nested struct must land in collectionParity or in
// opaqueTypes, never in neither.
func TestSchema_parity_table_covers_every_parser_struct(t *testing.T) {
	covered := map[reflect.Type]bool{}
	for _, tc := range collectionParity {
		if tc.typ != nil {
			covered[tc.typ] = true
		}
	}
	covered[reflect.TypeOf(parser.WebSocketStep{})] = true // covered by webSocketStepKeys
	for _, typ := range reachableStructs(reflect.TypeOf(parser.Collection{})) {
		if covered[typ] || opaqueTypes[typ] != "" {
			continue
		}
		t.Errorf("%s is reachable from parser.Collection through a yaml-visible field "+
			"but is in neither collectionParity nor opaqueTypes", typ)
	}
}

// reachableStructs returns every struct type reachable from root through
// yaml-visible fields, in deterministic order. Opaque types are returned but
// not descended into.
func reachableStructs(root reflect.Type) []reflect.Type {
	seen := map[reflect.Type]bool{}
	var out []reflect.Type
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		typ = deref(typ)
		if typ.Kind() == reflect.Map {
			walk(typ.Elem())
			return
		}
		if typ.Kind() != reflect.Struct || seen[typ] {
			return
		}
		seen[typ] = true
		out = append(out, typ)
		if opaqueTypes[typ] != "" {
			return
		}
		for i := range typ.NumField() {
			f := typ.Field(i)
			if f.PkgPath != "" {
				continue
			}
			if name, _ := splitYAMLTagValue(f.Tag.Get("yaml")); name == "-" {
				continue
			}
			walk(f.Type)
		}
	}
	walk(root)
	return out
}

// TestSchema_enums_match_parser pins every closed-set enum in the schema to the
// parser's own list, so adding a protocol or a websocket action without
// touching the schema fails here instead of in a user's editor.
func TestSchema_enums_match_parser(t *testing.T) {
	doc := decodeSchemaDoc(t, collectionSchemaBytes())
	tests := []struct {
		name string
		path string
		want []string
	}{
		{"protocol", "$defs/request/properties/protocol", parser.SupportedProtocols},
		{"websocket_action", "$defs/websocketStep/properties/action", parser.WebSocketActions},
		{"reconnect_backoff", "$defs/websocketReconnect/properties/backoff", parser.WebSocketBackoffStrategies},
		{"graphql_error_handling", "$defs/graphql/properties/error_handling", parser.GraphQLErrorHandlingValues},
		{"output_format", "$defs/output/properties/format", output.SupportedFormats},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := schemaEnum(t, schemaObjectAt(t, doc, tc.path))
			for _, d := range symmetricDiff(got, tc.want) {
				t.Errorf("enum at %q: %s", tc.path, d)
			}
		})
	}
}

// schemaEnum returns the enum values declared on a schema object.
func schemaEnum(t *testing.T, obj map[string]any) []string {
	t.Helper()
	raw, ok := obj["enum"].([]any)
	if !ok {
		t.Fatalf("schema object declares no enum: %v", sortedKeys(anyKeys(obj)))
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("enum value %v is not a string", v)
		}
		out = append(out, s)
	}
	return out
}

// TestSchema_dod_fixture_parses closes the loop the parity test opens. Parity
// compares names; this proves the two agree on one real document: the fixture
// exercising all nine previously-undescribed fields is accepted by the schema
// (via TestSchema_accepts) and by the parser that schema claims to describe.
func TestSchema_dod_fixture_parses(t *testing.T) {
	path := filepath.Join(repoRoot(t), "internal", "schema", "testdata", "gap_15_all_nine_fields.yaml")
	col, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{"collection_config_locale", func(t *testing.T) {
			if col.Config.Locale != "de-DE" {
				t.Errorf("config.locale = %q, want de-DE", col.Config.Locale)
			}
		}},
		{"collection_signing", func(t *testing.T) {
			if col.Signing == nil || col.Signing.Type != "aws-sigv4" {
				t.Errorf("collection signing = %+v, want type aws-sigv4", col.Signing)
			}
		}},
		{"item_if_and_signing_override", func(t *testing.T) {
			a := col.Requests.Items[0]
			if a.If == "" {
				t.Error("item A: if: was dropped")
			}
			if a.Signing == nil || a.Signing.Type != "oauth1" {
				t.Errorf("item A signing = %+v, want type oauth1", a.Signing)
			}
		}},
		{"item_graphql_and_cel", func(t *testing.T) {
			a := col.Requests.Items[0]
			if a.Request.Protocol != "graphql" || a.Request.GraphQL == nil {
				t.Errorf("item A protocol = %q, graphql = %+v", a.Request.Protocol, a.Request.GraphQL)
			}
			if len(a.Assertions.CEL.Items) != 2 {
				t.Errorf("item A cel entries = %d, want 2", len(a.Assertions.CEL.Items))
			}
		}},
		{"item_depends_on_and_explicit_null_signing", func(t *testing.T) {
			b := col.Requests.Items[1]
			if len(b.DependsOn) != 1 || b.DependsOn[0] != "A" {
				t.Errorf("item B depends_on = %v, want [A]", b.DependsOn)
			}
			if !b.Signing.IsExplicitNull() {
				t.Error("item B: signing: ~ did not survive as an explicit null")
			}
		}},
		{"item_websocket_defaults_method", func(t *testing.T) {
			b := col.Requests.Items[1]
			if b.Request.Protocol != "websocket" || b.Request.WebSocket == nil {
				t.Fatalf("item B protocol = %q, websocket = %+v", b.Request.Protocol, b.Request.WebSocket)
			}
			if b.Request.Method != "WS" {
				t.Errorf("item B method = %q, want WS — the fixture declares none, which is why $defs.request no longer requires it", b.Request.Method)
			}
			if len(b.Request.WebSocket.Steps) != 4 {
				t.Errorf("item B websocket steps = %d, want 4", len(b.Request.WebSocket.Steps))
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { tc.check(t) })
	}
}

// TestSchema_observable_no_missing_fields is the M21-001 observable as a Go
// test: the nine keys the task names must be present at their schema locations.
func TestSchema_observable_no_missing_fields(t *testing.T) {
	doc := decodeSchemaDoc(t, collectionSchemaBytes())
	tests := []struct {
		name string
		path string
		want []string
	}{
		{"requestItem", "$defs/requestItem", []string{"if", "depends_on", "signing"}},
		{"request", "$defs/request", []string{"protocol", "graphql", "websocket"}},
		{"assertions", "$defs/assertions", []string{"cel"}},
		{"top_level", "", []string{"config", "signing"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			props := schemaPropertyNames(t, schemaObjectAt(t, doc, tc.path))
			for _, k := range tc.want {
				if !props[k] {
					t.Errorf("MISSING: %s at schema path %q", k, tc.path)
				}
			}
		})
	}
}
