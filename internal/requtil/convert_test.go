package requtil

import (
	"reflect"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/variable"
)

// The three converters below are called from the sequential runner, the
// parallel executor and the WebSocket executor, so they were exercised
// indirectly and worked -- but nothing pinned their behaviour directly, and
// the two assertion converters differ in a way that is easy to "fix" wrongly.

// --- ToHeaderInputs ---------------------------------------------------------

func TestToHeaderInputs_empty_returns_nil(t *testing.T) {
	if got := ToHeaderInputs(nil); got != nil {
		t.Errorf("ToHeaderInputs(nil) = %#v, want nil", got)
	}
	if got := ToHeaderInputs([]parser.HeaderAssertion{}); got != nil {
		t.Errorf("ToHeaderInputs(empty) = %#v, want nil", got)
	}
}

func TestToHeaderInputs_preserves_order_and_fields(t *testing.T) {
	in := []parser.HeaderAssertion{
		{Name: "Content-Type", Operator: "equals", Value: "application/json"},
		{Name: "X-Request-Id", Operator: "exists"},
		{Name: "Server", Operator: "matches", Value: "nginx/.*"},
	}
	got := ToHeaderInputs(in)
	if len(got) != len(in) {
		t.Fatalf("len = %d, want %d", len(got), len(in))
	}
	for i := range in {
		if got[i].Name != in[i].Name || got[i].Operator != in[i].Operator {
			t.Errorf("[%d] = %+v, want name/operator from %+v", i, got[i], in[i])
		}
	}
	if got[0].Value != "application/json" {
		t.Errorf("[0].Value = %q", got[0].Value)
	}
}

// TestToHeaderInputs_stringifies_values pins the fmt.Sprint conversion.
// assertion.HeaderInput.Value is a string while parser.HeaderAssertion.Value
// is any, so every non-string value is rendered here rather than at
// evaluation time. A YAML author writing `status: 200` under a header
// assertion gets "200", and an operator with no value gets "<nil>" -- worth
// pinning because it is the kind of conversion someone may later "simplify"
// into a type switch that changes these results.
func TestToHeaderInputs_stringifies_values(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"string", "text/html", "text/html"},
		{"int", 200, "200"},
		{"float", 1.5, "1.5"},
		{"bool", true, "true"},
		{"nil", nil, "<nil>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToHeaderInputs([]parser.HeaderAssertion{
				{Name: "X-Test", Operator: "equals", Value: tc.value},
			})
			if len(got) != 1 {
				t.Fatalf("len = %d, want 1", len(got))
			}
			if got[0].Value != tc.want {
				t.Errorf("Value = %q, want %q", got[0].Value, tc.want)
			}
		})
	}
}

// --- ToBodyInputs -----------------------------------------------------------

func TestToBodyInputs_empty_returns_nil(t *testing.T) {
	if got := ToBodyInputs(nil); got != nil {
		t.Errorf("ToBodyInputs(nil) = %#v, want nil", got)
	}
	if got := ToBodyInputs([]parser.BodyAssertion{}); got != nil {
		t.Errorf("ToBodyInputs(empty) = %#v, want nil", got)
	}
}

func TestToBodyInputs_preserves_order_and_fields(t *testing.T) {
	in := []parser.BodyAssertion{
		{Path: "$.data.id", Operator: "equals", Value: 42},
		{Path: "$.data.name", Operator: "exists"},
		{Path: "$.items", Operator: "type", Value: "array"},
	}
	got := ToBodyInputs(in)
	if len(got) != len(in) {
		t.Fatalf("len = %d, want %d", len(got), len(in))
	}
	for i := range in {
		if got[i].Path != in[i].Path || got[i].Operator != in[i].Operator {
			t.Errorf("[%d] = %+v, want path/operator from %+v", i, got[i], in[i])
		}
	}
}

// TestToBodyInputs_does_not_stringify_values guards the asymmetry with
// ToHeaderInputs. assertion.BodyInput.Value is any, so a numeric assertion
// must stay numeric -- operators like greater_than compare on the typed
// value, and stringifying here (mirroring the header converter, which is the
// obvious "consistency" edit) would silently break them.
func TestToBodyInputs_does_not_stringify_values(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"int", 42},
		{"float", 1.5},
		{"bool", true},
		{"string", "hello"},
		{"slice", []any{1, 2}},
		{"map", map[string]any{"k": "v"}},
		{"nil", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToBodyInputs([]parser.BodyAssertion{
				{Path: "$.x", Operator: "equals", Value: tc.value},
			})
			if len(got) != 1 {
				t.Fatalf("len = %d, want 1", len(got))
			}
			if !reflect.DeepEqual(got[0].Value, tc.value) {
				t.Errorf("Value = %#v (%T), want %#v (%T) — body values must keep their type",
					got[0].Value, got[0].Value, tc.value, tc.value)
			}
		})
	}
}

// --- ToHTTPRequest ----------------------------------------------------------

func TestToHTTPRequest_copies_every_field(t *testing.T) {
	src := &parser.Request{
		Method:      "POST",
		URL:         "https://example.com/api",
		Headers:     map[string]string{"Authorization": "Bearer t"},
		Body:        map[string]any{"name": "test"},
		QueryParams: map[string]string{"page": "2"},
	}
	got := ToHTTPRequest(src)

	if got.Method != src.Method {
		t.Errorf("Method = %q, want %q", got.Method, src.Method)
	}
	if got.URL != src.URL {
		t.Errorf("URL = %q, want %q", got.URL, src.URL)
	}
	if !reflect.DeepEqual(got.Headers, src.Headers) {
		t.Errorf("Headers = %#v, want %#v", got.Headers, src.Headers)
	}
	if !reflect.DeepEqual(got.Body, src.Body) {
		t.Errorf("Body = %#v, want %#v", got.Body, src.Body)
	}
	if !reflect.DeepEqual(got.QueryParams, src.QueryParams) {
		t.Errorf("QueryParams = %#v, want %#v", got.QueryParams, src.QueryParams)
	}
}

// TestToHTTPRequest_populates_every_destination_field is the guard that
// matters. ToHTTPRequest is a hand-maintained field copy, so a field added to
// httpexec.Request would arrive at the executor silently zero -- the same
// divergence class as the TAP/JSON speedup metadata fixed in M21-003. Driving
// this by reflection means the new field fails this test on the day it is
// added, rather than the day someone notices requests losing it.
func TestToHTTPRequest_populates_every_destination_field(t *testing.T) {
	src := &parser.Request{
		Method:      "POST",
		URL:         "https://example.com/api",
		Headers:     map[string]string{"Authorization": "Bearer t"},
		Body:        "payload",
		QueryParams: map[string]string{"page": "2"},
	}
	got := reflect.ValueOf(*ToHTTPRequest(src))

	for i := 0; i < got.NumField(); i++ {
		field := got.Type().Field(i)
		if got.Field(i).IsZero() {
			t.Errorf("httpexec.Request.%s was left zero by ToHTTPRequest — a field added to the destination must be mapped in the converter (or explicitly excluded here with a reason)",
				field.Name)
		}
	}
}

func TestToHTTPRequest_nil_maps_survive(t *testing.T) {
	got := ToHTTPRequest(&parser.Request{Method: "GET", URL: "https://example.com"})
	if got.Headers != nil {
		t.Errorf("Headers = %#v, want nil", got.Headers)
	}
	if got.QueryParams != nil {
		t.Errorf("QueryParams = %#v, want nil", got.QueryParams)
	}
	if got.Body != nil {
		t.Errorf("Body = %#v, want nil", got.Body)
	}
}

// --- injectContentType via InterpolateRequest -------------------------------

// TestInterpolateRequest_body_file_without_content_type covers the branch
// where a body file resolved to no detected content type: the headers map
// must be returned untouched rather than gaining an empty Content-Type.
func TestInterpolateRequest_body_file_without_content_type(t *testing.T) {
	scope := variable.NewScope(map[string]string{"name": "world"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		Method:          "POST",
		URL:             "https://example.com",
		Headers:         map[string]string{"X-Custom": "v"},
		BodyFileContent: "hello {{name}}",
		// BodyFileContentType deliberately empty.
	}
	got, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest: %v", err)
	}
	if got.Body != "hello world" {
		t.Errorf("Body = %#v, want %q", got.Body, "hello world")
	}
	for name := range got.Headers {
		if strings.EqualFold(name, "Content-Type") {
			t.Errorf("Content-Type was injected with no detected type: %#v", got.Headers)
		}
	}
	if got.Headers["X-Custom"] != "v" {
		t.Errorf("existing header lost: %#v", got.Headers)
	}
}

// --- interpolation error paths ----------------------------------------------

// Each field interpolated by InterpolateRequest wraps its failure with a
// distinct prefix. Asserting the prefix keeps the error attributable: an
// undefined variable in the query string should not report itself as a URL
// problem.
func TestInterpolateRequest_undefined_variable_errors_name_their_field(t *testing.T) {
	cases := []struct {
		name       string
		req        parser.Request
		wantPrefix string
	}{
		{
			name:       "url",
			req:        parser.Request{Method: "GET", URL: "https://{{missing}}/api"},
			wantPrefix: "URL:",
		},
		{
			name: "headers",
			req: parser.Request{
				Method: "GET", URL: "https://example.com",
				Headers: map[string]string{"X-Token": "{{missing}}"},
			},
			wantPrefix: "headers:",
		},
		{
			name: "query params",
			req: parser.Request{
				Method: "GET", URL: "https://example.com",
				QueryParams: map[string]string{"q": "{{missing}}"},
			},
			wantPrefix: "query params:",
		},
		{
			name: "body_file",
			req: parser.Request{
				Method: "POST", URL: "https://example.com",
				BodyFileContent: "hello {{missing}}",
			},
			wantPrefix: "body_file:",
		},
		{
			name: "body",
			req: parser.Request{
				Method: "POST", URL: "https://example.com",
				Body: "hello {{missing}}",
			},
			wantPrefix: "body:",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scope := variable.NewScope(map[string]string{"defined": "x"})
			if err := scope.Resolve(); err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			got, err := InterpolateRequest(scope, &tc.req)
			if err == nil {
				t.Fatalf("expected an error, got request %+v", got)
			}
			if !strings.HasPrefix(err.Error(), tc.wantPrefix) {
				t.Errorf("error %q does not start with %q — the failing field must be named",
					err, tc.wantPrefix)
			}
			if got != nil {
				t.Errorf("expected a nil request alongside the error, got %+v", got)
			}
		})
	}
}
