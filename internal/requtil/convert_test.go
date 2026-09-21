package requtil

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/assertion"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/variable"
)

// The three converters below are called from the sequential runner, the
// parallel executor and the WebSocket executor, so they were exercised
// indirectly and worked -- but nothing pinned their behaviour directly, and
// the two assertion converters differ in a way that is easy to "fix" wrongly.

// mustHeaderInputs and mustBodyInputs run the converters with an empty scope,
// for the tests that pin field mapping rather than interpolation. Interpolation
// against a populated scope has its own tests further down.
func mustHeaderInputs(t *testing.T, items []parser.HeaderAssertion) []assertion.HeaderInput {
	t.Helper()
	got, err := ToHeaderInputs(resolvedScope(t, nil), items)
	if err != nil {
		t.Fatalf("ToHeaderInputs: %v", err)
	}
	return got
}

// resolvedScope builds a scope and resolves it. Resolve must run before
// Interpolate, and a scope that skipped it reports every variable as undefined
// — which looks exactly like the defect these tests cover.
func resolvedScope(t *testing.T, vars map[string]string) *variable.Scope {
	t.Helper()
	scope := variable.NewScope(vars)
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return scope
}

func mustBodyInputs(t *testing.T, items []parser.BodyAssertion) []assertion.BodyInput {
	t.Helper()
	got, err := ToBodyInputs(resolvedScope(t, nil), items)
	if err != nil {
		t.Fatalf("ToBodyInputs: %v", err)
	}
	return got
}

// --- ToHeaderInputs ---------------------------------------------------------

func TestToHeaderInputs_empty_returns_nil(t *testing.T) {
	if got := mustHeaderInputs(t, nil); got != nil {
		t.Errorf("mustHeaderInputs(t, nil) = %#v, want nil", got)
	}
	if got := mustHeaderInputs(t, []parser.HeaderAssertion{}); got != nil {
		t.Errorf("mustHeaderInputs(t, empty) = %#v, want nil", got)
	}
}

func TestToHeaderInputs_preserves_order_and_fields(t *testing.T) {
	in := []parser.HeaderAssertion{
		{Name: "Content-Type", Operator: "equals", Value: "application/json"},
		{Name: "X-Request-Id", Operator: "exists"},
		{Name: "Server", Operator: "matches", Value: "nginx/.*"},
	}
	got := mustHeaderInputs(t, in)
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
			got := mustHeaderInputs(t, []parser.HeaderAssertion{
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
	if got := mustBodyInputs(t, nil); got != nil {
		t.Errorf("mustBodyInputs(t, nil) = %#v, want nil", got)
	}
	if got := mustBodyInputs(t, []parser.BodyAssertion{}); got != nil {
		t.Errorf("mustBodyInputs(t, empty) = %#v, want nil", got)
	}
}

func TestToBodyInputs_preserves_order_and_fields(t *testing.T) {
	in := []parser.BodyAssertion{
		{Path: "$.data.id", Operator: "equals", Value: 42},
		{Path: "$.data.name", Operator: "exists"},
		{Path: "$.items", Operator: "type", Value: "array"},
	}
	got := mustBodyInputs(t, in)
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
			got := mustBodyInputs(t, []parser.BodyAssertion{
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

// --- Assertion expected values must interpolate (dogfood defect 1) ---
//
// Found by running curlew against mudflat: `equals: "{{var}}"` compared the
// response against the literal seven-character template. The same variable
// interpolated correctly in the URL of the very same request, so a request
// could fetch exactly the right resource and then fail to assert anything
// about it.

func TestToHeaderInputs_InterpolatesExpectedValues(t *testing.T) {
	scope := resolvedScope(t, map[string]string{
		"expected_ct": "application/json",
		"version":     "v2",
	})

	got, err := ToHeaderInputs(scope, []parser.HeaderAssertion{
		{Name: "Content-Type", Operator: "equals", Value: "{{expected_ct}}", Line: 7},
		{Name: "X-API-Version", Operator: "matches", Value: "^{{version}}$", Line: 8},
	})
	if err != nil {
		t.Fatalf("ToHeaderInputs: %v", err)
	}

	if got[0].Value != "application/json" {
		t.Errorf("equals value = %q, want %q", got[0].Value, "application/json")
	}
	if got[1].Value != "^v2$" {
		t.Errorf("matches value = %q, want %q", got[1].Value, "^v2$")
	}
	if got[0].Line != 7 {
		t.Errorf("Line = %d, want 7 — interpolation must not lose the source pointer", got[0].Line)
	}
}

func TestToBodyInputs_InterpolatesExpectedValues(t *testing.T) {
	scope := resolvedScope(t, map[string]string{
		"expected_id":   "res_abc_1",
		"expected_name": "widget",
	})

	got, err := ToBodyInputs(scope, []parser.BodyAssertion{
		{Path: "$.id", Operator: "equals", Value: "{{expected_id}}", Line: 3},
		{Path: "$.name", Operator: "contains", Value: "{{expected_name}}", Line: 4},
	})
	if err != nil {
		t.Fatalf("ToBodyInputs: %v", err)
	}

	if got[0].Value != "res_abc_1" {
		t.Errorf("equals value = %v, want %q", got[0].Value, "res_abc_1")
	}
	if got[1].Value != "widget" {
		t.Errorf("contains value = %v, want %q", got[1].Value, "widget")
	}
	if got[0].Line != 3 {
		t.Errorf("Line = %d, want 3", got[0].Line)
	}
}

func TestInterpolateRequest_DynamicCacheCoversAssertionsAndResets(t *testing.T) {
	scope := resolvedScope(t, map[string]string{"request_id": "{{$uuid}}"}).WithDynamic(variable.NewRegistry(nil))
	template := &parser.Request{
		Method:  "POST",
		URL:     "http://localhost/{{request_id}}",
		Headers: map[string]string{"X-Request-ID": "{{request_id}}"},
		Body:    "{{request_id}}",
	}
	previousID := ""
	for range 2 {
		request, err := InterpolateRequest(scope, template)
		if err != nil {
			t.Fatal(err)
		}
		requestID := request.Headers["X-Request-ID"]
		if requestID == "" || requestID == previousID || strings.Contains(requestID, "{{") {
			t.Fatalf("request ID = %q, previous = %q; want a fresh resolved value", requestID, previousID)
		}
		if request.URL != "http://localhost/"+requestID || request.Body != requestID {
			t.Fatalf("request fields disagree: %+v", request)
		}
		headers, err := ToHeaderInputs(scope, []parser.HeaderAssertion{
			{Name: "X-Request-ID", Operator: "equals", Value: "{{request_id}}"},
		})
		if err != nil {
			t.Fatal(err)
		}
		body, err := ToBodyInputs(scope, []parser.BodyAssertion{
			{Path: "$.id", Operator: "equals", Value: "{{request_id}}"},
			{Path: "$.direct", Operator: "equals", Value: "{{$uuid}}"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if headers[0].Value != requestID || body[0].Value != requestID || body[1].Value != requestID {
			t.Fatalf("assertions lost request ID %q: headers=%+v body=%+v", requestID, headers, body)
		}
		independent, err := InterpolateRequest(scope.Snapshot(), template)
		if err != nil {
			t.Fatal(err)
		}
		if independent.Headers["X-Request-ID"] == requestID {
			t.Fatal("independent request reused another request's cache")
		}
		retained, err := scope.Interpolate("{{request_id}}")
		if err != nil || retained != requestID {
			t.Fatalf("independent request changed the original cache: %q, %v", retained, err)
		}
		previousID = requestID
	}
}

func TestToBodyInputs_LeavesNonStringValuesAlone(t *testing.T) {
	// Numbers, booleans and nested structures must survive untouched. The
	// bignum case matters: routing an integer through a string round trip is
	// how 9007199254740993 becomes a different number.
	scope := resolvedScope(t, map[string]string{"unused": "x"})

	got, err := ToBodyInputs(scope, []parser.BodyAssertion{
		{Path: "$.n", Operator: "equals", Value: 9007199254740993},
		{Path: "$.ok", Operator: "equals", Value: true},
		{Path: "$.range", Operator: "in_range", Value: map[string]any{"min": 1, "max": 10}},
		{Path: "$.nil", Operator: "equals", Value: nil},
	})
	if err != nil {
		t.Fatalf("ToBodyInputs: %v", err)
	}

	if got[0].Value != 9007199254740993 {
		t.Errorf("integer value = %v (%T), want 9007199254740993 unchanged", got[0].Value, got[0].Value)
	}
	if got[1].Value != true {
		t.Errorf("bool value = %v, want true", got[1].Value)
	}
	if m, ok := got[2].Value.(map[string]any); !ok || m["max"] != 10 {
		t.Errorf("map value = %v, want the map preserved", got[2].Value)
	}
	if got[3].Value != nil {
		t.Errorf("nil value = %v, want nil", got[3].Value)
	}
}

func TestToBodyInputs_InterpolatesInsideNestedValues(t *testing.T) {
	scope := resolvedScope(t, map[string]string{"lo": "5", "hi": "10"})

	got, err := ToBodyInputs(scope, []parser.BodyAssertion{
		{Path: "$.n", Operator: "in_range", Value: map[string]any{"min": "{{lo}}", "max": "{{hi}}"}},
	})
	if err != nil {
		t.Fatalf("ToBodyInputs: %v", err)
	}

	m, ok := got[0].Value.(map[string]any)
	if !ok {
		t.Fatalf("value = %T, want a map", got[0].Value)
	}
	if fmt.Sprint(m["min"]) != "5" || fmt.Sprint(m["max"]) != "10" {
		t.Errorf("nested values = %v, want min 5 and max 10", m)
	}
}

func TestToInputs_UndefinedVariableIsAnError(t *testing.T) {
	// Consistent with every other interpolation site: an unresolvable reference
	// is an error, not a silent literal. Silently comparing against "{{nope}}"
	// is the defect this whole change removes.
	scope := resolvedScope(t, map[string]string{"defined": "x"})

	if _, err := ToBodyInputs(scope, []parser.BodyAssertion{
		{Path: "$.id", Operator: "equals", Value: "{{nope}}"},
	}); err == nil {
		t.Error("ToBodyInputs with an undefined variable = nil error, want failure")
	}

	if _, err := ToHeaderInputs(scope, []parser.HeaderAssertion{
		{Name: "X", Operator: "equals", Value: "{{nope}}"},
	}); err == nil {
		t.Error("ToHeaderInputs with an undefined variable = nil error, want failure")
	}
}

func TestToInputs_EmptyReturnsNilWithoutError(t *testing.T) {
	scope := resolvedScope(t, nil)

	h, err := ToHeaderInputs(scope, nil)
	if err != nil || h != nil {
		t.Errorf("mustHeaderInputs(t, nil) = %v, %v; want nil, nil", h, err)
	}
	b, err := ToBodyInputs(scope, nil)
	if err != nil || b != nil {
		t.Errorf("mustBodyInputs(t, nil) = %v, %v; want nil, nil", b, err)
	}
}
