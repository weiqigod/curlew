package assertion

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCheckStatus(t *testing.T) {
	tests := []struct {
		name     string
		expected []int
		actual   int
		wantNil  bool
		wantPass bool
	}{
		{"single match 200", []int{200}, 200, false, true},
		{"single mismatch 200 vs 404", []int{200}, 404, false, false},
		{"list match first", []int{200, 201}, 200, false, true},
		{"list match second", []int{200, 201}, 201, false, true},
		{"list mismatch", []int{200, 201}, 404, false, false},
		{"exact 404 match", []int{404}, 404, false, true},
		{"empty expected returns nil", []int{}, 200, true, false},
		{"nil expected returns nil", nil, 200, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckStatus(tt.expected, tt.actual)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil, want non-nil result")
			}
			if got.Passed != tt.wantPass {
				t.Errorf("Passed = %v, want %v", got.Passed, tt.wantPass)
			}
			if got.Type != "status" {
				t.Errorf("Type = %q, want %q", got.Type, "status")
			}
		})
	}
}

func TestCheckStatus_expected_formatting(t *testing.T) {
	tests := []struct {
		name         string
		expected     []int
		actual       int
		wantExpected string
		wantActual   string
	}{
		{"single code formats as plain int", []int{200}, 404, "200", "404"},
		{"list formats as bracketed", []int{200, 201}, 404, "[200, 201]", "404"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckStatus(tt.expected, tt.actual)
			if got.Expected != tt.wantExpected {
				t.Errorf("Expected = %q, want %q", got.Expected, tt.wantExpected)
			}
			if got.Actual != tt.wantActual {
				t.Errorf("Actual = %q, want %q", got.Actual, tt.wantActual)
			}
		})
	}
}

func TestCheckHeaders(t *testing.T) {
	tests := []struct {
		name       string
		assertions []HeaderInput
		headers    http.Header
		wantCount  int
		wantPassed []bool
	}{
		{
			"equals passes when header value matches",
			[]HeaderInput{{Name: "Content-Type", Operator: "equals", Value: "application/json"}},
			http.Header{"Content-Type": {"application/json"}},
			1,
			[]bool{true},
		},
		{
			"equals fails when header value differs",
			[]HeaderInput{{Name: "Content-Type", Operator: "equals", Value: "text/html"}},
			http.Header{"Content-Type": {"application/json"}},
			1,
			[]bool{false},
		},
		{
			"equals case-insensitive header name",
			[]HeaderInput{{Name: "content-type", Operator: "equals", Value: "application/json"}},
			http.Header{"Content-Type": {"application/json"}},
			1,
			[]bool{true},
		},
		{
			"equals empty header value",
			[]HeaderInput{{Name: "X-Empty", Operator: "equals", Value: ""}},
			http.Header{"X-Empty": {""}},
			1,
			[]bool{true},
		},
		{
			"exists passes when header present",
			[]HeaderInput{{Name: "X-Request-Id", Operator: "exists"}},
			http.Header{"X-Request-Id": {"abc123"}},
			1,
			[]bool{true},
		},
		{
			"exists fails when header absent",
			[]HeaderInput{{Name: "X-Missing", Operator: "exists"}},
			http.Header{"Content-Type": {"text/html"}},
			1,
			[]bool{false},
		},
		{
			"exists case-insensitive header name",
			[]HeaderInput{{Name: "x-request-id", Operator: "exists"}},
			http.Header{"X-Request-Id": {"abc123"}},
			1,
			[]bool{true},
		},
		{
			"matches passes when regex matches",
			[]HeaderInput{{Name: "Content-Type", Operator: "matches", Value: "^application/.*"}},
			http.Header{"Content-Type": {"application/json"}},
			1,
			[]bool{true},
		},
		{
			"matches fails when regex does not match",
			[]HeaderInput{{Name: "Content-Type", Operator: "matches", Value: "^text/.*"}},
			http.Header{"Content-Type": {"application/json"}},
			1,
			[]bool{false},
		},
		{
			"matches case-insensitive header name",
			[]HeaderInput{{Name: "content-type", Operator: "matches", Value: "^application/.*"}},
			http.Header{"Content-Type": {"application/json"}},
			1,
			[]bool{true},
		},
		{
			"matches invalid regex returns failure",
			[]HeaderInput{{Name: "Content-Type", Operator: "matches", Value: "[invalid"}},
			http.Header{"Content-Type": {"application/json"}},
			1,
			[]bool{false},
		},
		{
			"nil assertions returns nil",
			nil,
			http.Header{"Content-Type": {"text/html"}},
			0, nil,
		},
		{
			"empty assertions returns nil",
			[]HeaderInput{},
			http.Header{"Content-Type": {"text/html"}},
			0, nil,
		},
		{
			"nil headers fails all assertions",
			[]HeaderInput{{Name: "Content-Type", Operator: "exists"}},
			nil,
			1,
			[]bool{false},
		},
		{
			"multiple assertions all evaluated",
			[]HeaderInput{
				{Name: "Content-Type", Operator: "equals", Value: "application/json"},
				{Name: "X-Request-Id", Operator: "exists"},
			},
			http.Header{"Content-Type": {"application/json"}, "X-Request-Id": {"abc"}},
			2,
			[]bool{true, true},
		},
		{
			"unsupported operator returns failure",
			[]HeaderInput{{Name: "Content-Type", Operator: "contains", Value: "json"}},
			http.Header{"Content-Type": {"application/json"}},
			1,
			[]bool{false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckHeaders(tt.assertions, tt.headers)
			if tt.wantPassed == nil {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil, want non-nil results")
			}
			if len(got) != tt.wantCount {
				t.Fatalf("result count = %d, want %d", len(got), tt.wantCount)
			}
			for i, want := range tt.wantPassed {
				if got[i].Passed != want {
					t.Errorf("result[%d].Passed = %v, want %v (Expected=%q, Actual=%q)",
						i, got[i].Passed, want, got[i].Expected, got[i].Actual)
				}
			}
		})
	}
}

func TestCheckTiming(t *testing.T) {
	tests := []struct {
		name          string
		maxDurationMs int
		actual        time.Duration
		wantNil       bool
		wantPassed    bool
	}{
		{"passes when faster than limit", 500, 200 * time.Millisecond, false, true},
		{"fails when slower than limit", 100, 200 * time.Millisecond, false, false},
		{"passes when exactly at limit", 200, 200 * time.Millisecond, false, true},
		{"zero max returns nil", 0, 200 * time.Millisecond, true, false},
		{"negative max returns nil", -1, 200 * time.Millisecond, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckTiming(tt.maxDurationMs, tt.actual)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil, want non-nil result")
			}
			if got.Passed != tt.wantPassed {
				t.Errorf("Passed = %v, want %v (Expected=%q, Actual=%q)", got.Passed, tt.wantPassed, got.Expected, got.Actual)
			}
			if got.Type != "timing" {
				t.Errorf("Type = %q, want %q", got.Type, "timing")
			}
		})
	}
}

func TestCheckBody(t *testing.T) {
	tests := []struct {
		name       string
		assertions []BodyInput
		body       []byte
		wantCount  int
		wantPassed []bool
	}{
		{
			"equals passes when values match",
			[]BodyInput{{Path: "$.id", Operator: "equals", Value: 1}},
			[]byte(`{"id":1}`),
			1,
			[]bool{true},
		},
		{
			"equals fails when values differ",
			[]BodyInput{{Path: "$.id", Operator: "equals", Value: 1}},
			[]byte(`{"id":2}`),
			1,
			[]bool{false},
		},
		{
			"equals with string value",
			[]BodyInput{{Path: "$.name", Operator: "equals", Value: "Alice"}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{true},
		},
		{
			"equals with boolean value",
			[]BodyInput{{Path: "$.active", Operator: "equals", Value: true}},
			[]byte(`{"active":true}`),
			1,
			[]bool{true},
		},
		{
			"exists passes when path has value",
			[]BodyInput{{Path: "$.id", Operator: "exists"}},
			[]byte(`{"id":1}`),
			1,
			[]bool{true},
		},
		{
			"exists passes for null value",
			[]BodyInput{{Path: "$.value", Operator: "exists"}},
			[]byte(`{"value":null}`),
			1,
			[]bool{true},
		},
		{
			"exists fails when path missing",
			[]BodyInput{{Path: "$.missing", Operator: "exists"}},
			[]byte(`{"id":1}`),
			1,
			[]bool{false},
		},
		{
			"not_exists passes when path missing",
			[]BodyInput{{Path: "$.missing", Operator: "not_exists"}},
			[]byte(`{"id":1}`),
			1,
			[]bool{true},
		},
		{
			"not_exists fails when path has value",
			[]BodyInput{{Path: "$.id", Operator: "not_exists"}},
			[]byte(`{"id":1}`),
			1,
			[]bool{false},
		},
		{
			"type string passes for string",
			[]BodyInput{{Path: "$.name", Operator: "type", Value: "string"}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{true},
		},
		{
			"type number passes for number",
			[]BodyInput{{Path: "$.id", Operator: "type", Value: "number"}},
			[]byte(`{"id":42}`),
			1,
			[]bool{true},
		},
		{
			"type boolean passes for bool",
			[]BodyInput{{Path: "$.active", Operator: "type", Value: "boolean"}},
			[]byte(`{"active":true}`),
			1,
			[]bool{true},
		},
		{
			"type array passes for array",
			[]BodyInput{{Path: "$.items", Operator: "type", Value: "array"}},
			[]byte(`{"items":[]}`),
			1,
			[]bool{true},
		},
		{
			"type object passes for object",
			[]BodyInput{{Path: "$.data", Operator: "type", Value: "object"}},
			[]byte(`{"data":{}}`),
			1,
			[]bool{true},
		},
		{
			"type null passes for null",
			[]BodyInput{{Path: "$.value", Operator: "type", Value: "null"}},
			[]byte(`{"value":null}`),
			1,
			[]bool{true},
		},
		{
			"type fails when type mismatches",
			[]BodyInput{{Path: "$.id", Operator: "type", Value: "string"}},
			[]byte(`{"id":42}`),
			1,
			[]bool{false},
		},
		{
			"no match at path gives clear message",
			[]BodyInput{{Path: "$.missing", Operator: "equals", Value: 1}},
			[]byte(`{"id":1}`),
			1,
			[]bool{false},
		},
		{
			"non-JSON body returns error result",
			[]BodyInput{{Path: "$.id", Operator: "equals", Value: 1}},
			[]byte(`<html>not json</html>`),
			1,
			[]bool{false},
		},
		{
			"empty body with assertions returns error result",
			[]BodyInput{{Path: "$.id", Operator: "equals", Value: 1}},
			[]byte{},
			1,
			[]bool{false},
		},
		{
			"nil assertions returns nil",
			nil,
			[]byte(`{"id":1}`),
			0, nil,
		},
		{
			"multiple assertions all evaluated",
			[]BodyInput{
				{Path: "$.id", Operator: "equals", Value: 1},
				{Path: "$.name", Operator: "type", Value: "string"},
				{Path: "$.active", Operator: "exists"},
			},
			[]byte(`{"id":1,"name":"Alice","active":true}`),
			3,
			[]bool{true, true, true},
		},
		{
			"multiple assertions with one failure",
			[]BodyInput{
				{Path: "$.id", Operator: "equals", Value: 1},
				{Path: "$.id", Operator: "type", Value: "string"},
			},
			[]byte(`{"id":1}`),
			2,
			[]bool{true, false},
		},
		{
			"invalid path fails exists",
			[]BodyInput{{Path: "invalid_path", Operator: "exists"}},
			[]byte(`{"id":1}`),
			1,
			[]bool{false},
		},
		{
			"invalid path fails not_exists",
			[]BodyInput{{Path: "invalid_path", Operator: "not_exists"}},
			[]byte(`{"id":1}`),
			1,
			[]bool{false},
		},
		{
			"invalid path fails type",
			[]BodyInput{{Path: "invalid_path", Operator: "type", Value: "null"}},
			[]byte(`{"id":1}`),
			1,
			[]bool{false},
		},
		{
			"invalid path fails equals",
			[]BodyInput{{Path: "invalid_path", Operator: "equals", Value: 1}},
			[]byte(`{"id":1}`),
			1,
			[]bool{false},
		},
		{
			"equals passes for numeric string 1 vs number 1",
			[]BodyInput{{Path: "$.val", Operator: "equals", Value: "1"}},
			[]byte(`{"val":1}`),
			1,
			[]bool{true},
		},
		{
			"equals passes for numeric string 42 vs number 42",
			[]BodyInput{{Path: "$.val", Operator: "equals", Value: 42}},
			[]byte(`{"val":"42"}`),
			1,
			[]bool{true},
		},
		{
			"equals passes for number 42 vs numeric string 42",
			[]BodyInput{{Path: "$.val", Operator: "equals", Value: "42"}},
			[]byte(`{"val":42}`),
			1,
			[]bool{true},
		},
		{
			"equals passes for float string 3.14 vs number 3.14",
			[]BodyInput{{Path: "$.val", Operator: "equals", Value: 3.14}},
			[]byte(`{"val":"3.14"}`),
			1,
			[]bool{true},
		},
		{
			"equals rejects non-numeric string vs number",
			[]BodyInput{{Path: "$.val", Operator: "equals", Value: 42}},
			[]byte(`{"val":"hello"}`),
			1,
			[]bool{false},
		},
		{
			"equals rejects string true vs boolean true",
			[]BodyInput{{Path: "$.val", Operator: "equals", Value: "true"}},
			[]byte(`{"val":true}`),
			1,
			[]bool{false},
		},
		{
			"equals rejects number 0 vs boolean false",
			[]BodyInput{{Path: "$.val", Operator: "equals", Value: false}},
			[]byte(`{"val":0}`),
			1,
			[]bool{false},
		},

		// matches operator
		{
			"matches passes when string matches regex",
			[]BodyInput{{Path: "$.name", Operator: "matches", Value: "^Al.*"}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{true},
		},
		{
			"matches fails when string does not match regex",
			[]BodyInput{{Path: "$.name", Operator: "matches", Value: "^Bob.*"}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{false},
		},
		{
			"matches fails with invalid regex",
			[]BodyInput{{Path: "$.name", Operator: "matches", Value: "[invalid"}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{false},
		},
		{
			"matches fails when path not found",
			[]BodyInput{{Path: "$.missing", Operator: "matches", Value: ".*"}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{false},
		},
		{
			"matches converts number to string before matching",
			[]BodyInput{{Path: "$.id", Operator: "matches", Value: "^42$"}},
			[]byte(`{"id":42}`),
			1,
			[]bool{true},
		},

		// contains operator
		{
			"contains passes for string containing substring",
			[]BodyInput{{Path: "$.name", Operator: "contains", Value: "lic"}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{true},
		},
		{
			"contains fails for string not containing substring",
			[]BodyInput{{Path: "$.name", Operator: "contains", Value: "xyz"}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{false},
		},
		{
			"contains passes for array containing expected element",
			[]BodyInput{{Path: "$.items", Operator: "contains", Value: 2}},
			[]byte(`{"items":[1,2,3]}`),
			1,
			[]bool{true},
		},
		{
			"contains fails for array not containing expected element",
			[]BodyInput{{Path: "$.items", Operator: "contains", Value: 9}},
			[]byte(`{"items":[1,2,3]}`),
			1,
			[]bool{false},
		},
		{
			"contains passes for object containing subset keys",
			[]BodyInput{{Path: "$.data", Operator: "contains", Value: map[string]any{"a": float64(1)}}},
			[]byte(`{"data":{"a":1,"b":2}}`),
			1,
			[]bool{true},
		},
		{
			"contains fails for object missing expected keys",
			[]BodyInput{{Path: "$.data", Operator: "contains", Value: map[string]any{"c": float64(3)}}},
			[]byte(`{"data":{"a":1,"b":2}}`),
			1,
			[]bool{false},
		},
		{
			"contains fails when path not found",
			[]BodyInput{{Path: "$.missing", Operator: "contains", Value: "x"}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{false},
		},

		// contains_all operator
		{
			"contains_all passes when all items present",
			[]BodyInput{{Path: "$.items", Operator: "contains_all", Value: []any{float64(1), float64(3)}}},
			[]byte(`{"items":[1,2,3]}`),
			1,
			[]bool{true},
		},
		{
			"contains_all passes when array has extra items",
			[]BodyInput{{Path: "$.items", Operator: "contains_all", Value: []any{float64(2)}}},
			[]byte(`{"items":[1,2,3]}`),
			1,
			[]bool{true},
		},
		{
			"contains_all fails when one item missing",
			[]BodyInput{{Path: "$.items", Operator: "contains_all", Value: []any{float64(1), float64(9)}}},
			[]byte(`{"items":[1,2,3]}`),
			1,
			[]bool{false},
		},
		{
			"contains_all fails when actual is not an array",
			[]BodyInput{{Path: "$.name", Operator: "contains_all", Value: []any{"a"}}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{false},
		},
		{
			"contains_all fails when expected is not a list",
			[]BodyInput{{Path: "$.items", Operator: "contains_all", Value: "not a list"}},
			[]byte(`{"items":[1,2,3]}`),
			1,
			[]bool{false},
		},
		{
			"contains_all fails when path not found",
			[]BodyInput{{Path: "$.missing", Operator: "contains_all", Value: []any{float64(1)}}},
			[]byte(`{"items":[1,2,3]}`),
			1,
			[]bool{false},
		},

		// length operator
		{
			"length passes for array with matching length",
			[]BodyInput{{Path: "$.items", Operator: "length", Value: 3}},
			[]byte(`{"items":[1,2,3]}`),
			1,
			[]bool{true},
		},
		{
			"length fails for array with different length",
			[]BodyInput{{Path: "$.items", Operator: "length", Value: 5}},
			[]byte(`{"items":[1,2,3]}`),
			1,
			[]bool{false},
		},
		{
			"length passes for string with matching length",
			[]BodyInput{{Path: "$.name", Operator: "length", Value: 5}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{true},
		},
		{
			"length fails for string with different length",
			[]BodyInput{{Path: "$.name", Operator: "length", Value: 3}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{false},
		},
		{
			"length passes for object with matching key count",
			[]BodyInput{{Path: "$.data", Operator: "length", Value: 2}},
			[]byte(`{"data":{"a":1,"b":2}}`),
			1,
			[]bool{true},
		},
		{
			"length fails for non-countable value",
			[]BodyInput{{Path: "$.id", Operator: "length", Value: 1}},
			[]byte(`{"id":42}`),
			1,
			[]bool{false},
		},
		{
			"length fails when path not found",
			[]BodyInput{{Path: "$.missing", Operator: "length", Value: 0}},
			[]byte(`{"id":1}`),
			1,
			[]bool{false},
		},
		{
			"length handles zero-length array",
			[]BodyInput{{Path: "$.items", Operator: "length", Value: 0}},
			[]byte(`{"items":[]}`),
			1,
			[]bool{true},
		},

		// greater_than operator
		{
			"greater_than passes when value exceeds threshold",
			[]BodyInput{{Path: "$.val", Operator: "greater_than", Value: 10}},
			[]byte(`{"val":15}`),
			1,
			[]bool{true},
		},
		{
			"greater_than fails when value equals threshold",
			[]BodyInput{{Path: "$.val", Operator: "greater_than", Value: 10}},
			[]byte(`{"val":10}`),
			1,
			[]bool{false},
		},
		{
			"greater_than fails when value below threshold",
			[]BodyInput{{Path: "$.val", Operator: "greater_than", Value: 10}},
			[]byte(`{"val":5}`),
			1,
			[]bool{false},
		},
		{
			"greater_than fails for non-numeric value",
			[]BodyInput{{Path: "$.name", Operator: "greater_than", Value: 10}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{false},
		},
		{
			"greater_than fails when path not found",
			[]BodyInput{{Path: "$.missing", Operator: "greater_than", Value: 10}},
			[]byte(`{"val":15}`),
			1,
			[]bool{false},
		},

		// less_than operator
		{
			"less_than passes when value below threshold",
			[]BodyInput{{Path: "$.val", Operator: "less_than", Value: 10}},
			[]byte(`{"val":5}`),
			1,
			[]bool{true},
		},
		{
			"less_than fails when value equals threshold",
			[]BodyInput{{Path: "$.val", Operator: "less_than", Value: 10}},
			[]byte(`{"val":10}`),
			1,
			[]bool{false},
		},
		{
			"less_than fails when value above threshold",
			[]BodyInput{{Path: "$.val", Operator: "less_than", Value: 10}},
			[]byte(`{"val":15}`),
			1,
			[]bool{false},
		},
		{
			"less_than fails when path not found",
			[]BodyInput{{Path: "$.missing", Operator: "less_than", Value: 10}},
			[]byte(`{"val":5}`),
			1,
			[]bool{false},
		},

		// greater_than_or_equal operator
		{
			"gte passes when value exceeds threshold",
			[]BodyInput{{Path: "$.val", Operator: "greater_than_or_equal", Value: 10}},
			[]byte(`{"val":15}`),
			1,
			[]bool{true},
		},
		{
			"gte passes when value equals threshold",
			[]BodyInput{{Path: "$.val", Operator: "greater_than_or_equal", Value: 10}},
			[]byte(`{"val":10}`),
			1,
			[]bool{true},
		},
		{
			"gte fails when value below threshold",
			[]BodyInput{{Path: "$.val", Operator: "greater_than_or_equal", Value: 10}},
			[]byte(`{"val":5}`),
			1,
			[]bool{false},
		},
		{
			"gte fails when path not found",
			[]BodyInput{{Path: "$.missing", Operator: "greater_than_or_equal", Value: 10}},
			[]byte(`{"val":15}`),
			1,
			[]bool{false},
		},

		// less_than_or_equal operator
		{
			"lte passes when value below threshold",
			[]BodyInput{{Path: "$.val", Operator: "less_than_or_equal", Value: 10}},
			[]byte(`{"val":5}`),
			1,
			[]bool{true},
		},
		{
			"lte passes when value equals threshold",
			[]BodyInput{{Path: "$.val", Operator: "less_than_or_equal", Value: 10}},
			[]byte(`{"val":10}`),
			1,
			[]bool{true},
		},
		{
			"lte fails when value above threshold",
			[]BodyInput{{Path: "$.val", Operator: "less_than_or_equal", Value: 10}},
			[]byte(`{"val":15}`),
			1,
			[]bool{false},
		},
		{
			"lte fails when path not found",
			[]BodyInput{{Path: "$.missing", Operator: "less_than_or_equal", Value: 10}},
			[]byte(`{"val":5}`),
			1,
			[]bool{false},
		},

		// numeric comparison with string coercion
		{
			"greater_than works with numeric string values",
			[]BodyInput{{Path: "$.val", Operator: "greater_than", Value: 10}},
			[]byte(`{"val":"15"}`),
			1,
			[]bool{true},
		},

		// approximately operator
		{
			"approximately passes when within tolerance",
			[]BodyInput{{Path: "$.price", Operator: "approximately", Value: map[string]any{"value": 3.14, "tolerance": 0.01}}},
			[]byte(`{"price":3.141}`),
			1,
			[]bool{true},
		},
		{
			"approximately fails when outside tolerance",
			[]BodyInput{{Path: "$.price", Operator: "approximately", Value: map[string]any{"value": 3.14, "tolerance": 0.001}}},
			[]byte(`{"price":3.2}`),
			1,
			[]bool{false},
		},
		{
			"approximately passes at exact boundary",
			[]BodyInput{{Path: "$.price", Operator: "approximately", Value: map[string]any{"value": 10.0, "tolerance": 0.5}}},
			[]byte(`{"price":10.5}`),
			1,
			[]bool{true},
		},
		{
			"approximately fails with missing value key",
			[]BodyInput{{Path: "$.price", Operator: "approximately", Value: map[string]any{"tolerance": 0.01}}},
			[]byte(`{"price":3.14}`),
			1,
			[]bool{false},
		},
		{
			"approximately fails with missing tolerance key",
			[]BodyInput{{Path: "$.price", Operator: "approximately", Value: map[string]any{"value": 3.14}}},
			[]byte(`{"price":3.14}`),
			1,
			[]bool{false},
		},
		{
			"approximately fails with non-numeric actual",
			[]BodyInput{{Path: "$.name", Operator: "approximately", Value: map[string]any{"value": 3.14, "tolerance": 0.01}}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{false},
		},
		{
			"approximately fails when path not found",
			[]BodyInput{{Path: "$.missing", Operator: "approximately", Value: map[string]any{"value": 3.14, "tolerance": 0.01}}},
			[]byte(`{"price":3.14}`),
			1,
			[]bool{false},
		},

		// in_range operator
		{
			"in_range passes when value within range",
			[]BodyInput{{Path: "$.val", Operator: "in_range", Value: map[string]any{"min": float64(10), "max": float64(20)}}},
			[]byte(`{"val":15}`),
			1,
			[]bool{true},
		},
		{
			"in_range passes when value equals min",
			[]BodyInput{{Path: "$.val", Operator: "in_range", Value: map[string]any{"min": float64(10), "max": float64(20)}}},
			[]byte(`{"val":10}`),
			1,
			[]bool{true},
		},
		{
			"in_range passes when value equals max",
			[]BodyInput{{Path: "$.val", Operator: "in_range", Value: map[string]any{"min": float64(10), "max": float64(20)}}},
			[]byte(`{"val":20}`),
			1,
			[]bool{true},
		},
		{
			"in_range fails when value below min",
			[]BodyInput{{Path: "$.val", Operator: "in_range", Value: map[string]any{"min": float64(10), "max": float64(20)}}},
			[]byte(`{"val":5}`),
			1,
			[]bool{false},
		},
		{
			"in_range fails when value above max",
			[]BodyInput{{Path: "$.val", Operator: "in_range", Value: map[string]any{"min": float64(10), "max": float64(20)}}},
			[]byte(`{"val":25}`),
			1,
			[]bool{false},
		},
		{
			"in_range fails with missing min key",
			[]BodyInput{{Path: "$.val", Operator: "in_range", Value: map[string]any{"max": float64(20)}}},
			[]byte(`{"val":15}`),
			1,
			[]bool{false},
		},
		{
			"in_range fails with missing max key",
			[]BodyInput{{Path: "$.val", Operator: "in_range", Value: map[string]any{"min": float64(10)}}},
			[]byte(`{"val":15}`),
			1,
			[]bool{false},
		},
		{
			"in_range fails with non-numeric actual",
			[]BodyInput{{Path: "$.name", Operator: "in_range", Value: map[string]any{"min": float64(10), "max": float64(20)}}},
			[]byte(`{"name":"Alice"}`),
			1,
			[]bool{false},
		},
		{
			"in_range fails when path not found",
			[]BodyInput{{Path: "$.missing", Operator: "in_range", Value: map[string]any{"min": float64(10), "max": float64(20)}}},
			[]byte(`{"val":15}`),
			1,
			[]bool{false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckBody(tt.assertions, tt.body)
			if tt.wantPassed == nil {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil, want non-nil results")
			}
			if len(got) != tt.wantCount {
				t.Fatalf("result count = %d, want %d", len(got), tt.wantCount)
			}
			for i, want := range tt.wantPassed {
				if got[i].Passed != want {
					t.Errorf("result[%d].Passed = %v, want %v (Expected=%q, Actual=%q)",
						i, got[i].Passed, want, got[i].Expected, got[i].Actual)
				}
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name       string
		input      EvalInput
		wantNil    bool
		wantPassed bool
		wantItems  int
	}{
		{
			"no assertions returns nil",
			EvalInput{ActualStatus: 200},
			true, false, 0,
		},
		{
			"empty codes returns nil",
			EvalInput{StatusCodes: []int{}, ActualStatus: 200},
			true, false, 0,
		},
		{
			"passing status assertion",
			EvalInput{StatusCodes: []int{200}, ActualStatus: 200},
			false, true, 1,
		},
		{
			"failing status assertion",
			EvalInput{StatusCodes: []int{200}, ActualStatus: 404},
			false, false, 1,
		},
		{
			"body assertion only",
			EvalInput{
				ActualStatus:   200,
				BodyAssertions: []BodyInput{{Path: "$.id", Operator: "equals", Value: 1}},
				Body:           []byte(`{"id":1}`),
			},
			false, true, 1,
		},
		{
			"status and body both pass",
			EvalInput{
				StatusCodes:    []int{200},
				ActualStatus:   200,
				BodyAssertions: []BodyInput{{Path: "$.id", Operator: "equals", Value: 1}},
				Body:           []byte(`{"id":1}`),
			},
			false, true, 2,
		},
		{
			"status passes body fails",
			EvalInput{
				StatusCodes:    []int{200},
				ActualStatus:   200,
				BodyAssertions: []BodyInput{{Path: "$.id", Operator: "equals", Value: 99}},
				Body:           []byte(`{"id":1}`),
			},
			false, false, 2,
		},
		{
			"status fails body passes",
			EvalInput{
				StatusCodes:    []int{201},
				ActualStatus:   200,
				BodyAssertions: []BodyInput{{Path: "$.id", Operator: "equals", Value: 1}},
				Body:           []byte(`{"id":1}`),
			},
			false, false, 2,
		},
		{
			"header assertion only",
			EvalInput{
				ActualStatus:     200,
				HeaderAssertions: []HeaderInput{{Name: "Content-Type", Operator: "equals", Value: "application/json"}},
				Headers:          http.Header{"Content-Type": {"application/json"}},
			},
			false, true, 1,
		},
		{
			"timing assertion only",
			EvalInput{
				ActualStatus:   200,
				MaxDurationMs:  500,
				ActualDuration: 200 * time.Millisecond,
			},
			false, true, 1,
		},
		{
			"all assertion types pass",
			EvalInput{
				StatusCodes:      []int{200},
				ActualStatus:     200,
				HeaderAssertions: []HeaderInput{{Name: "Content-Type", Operator: "exists"}},
				Headers:          http.Header{"Content-Type": {"application/json"}},
				BodyAssertions:   []BodyInput{{Path: "$.id", Operator: "equals", Value: 1}},
				Body:             []byte(`{"id":1}`),
				MaxDurationMs:    500,
				ActualDuration:   200 * time.Millisecond,
			},
			false, true, 4,
		},
		{
			"header fails others pass",
			EvalInput{
				StatusCodes:      []int{200},
				ActualStatus:     200,
				HeaderAssertions: []HeaderInput{{Name: "X-Missing", Operator: "exists"}},
				Headers:          http.Header{"Content-Type": {"text/html"}},
			},
			false, false, 2,
		},
		{
			"timing fails others pass",
			EvalInput{
				StatusCodes:    []int{200},
				ActualStatus:   200,
				MaxDurationMs:  100,
				ActualDuration: 200 * time.Millisecond,
			},
			false, false, 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil, want non-nil result")
			}
			if got.Passed != tt.wantPassed {
				t.Errorf("Passed = %v, want %v", got.Passed, tt.wantPassed)
			}
			if len(got.Items) != tt.wantItems {
				t.Fatalf("Items count = %d, want %d", len(got.Items), tt.wantItems)
			}
		})
	}
}

func TestEvaluate_with_schema(t *testing.T) {
	compiled := mustCompile(t, `{"type":"object","required":["id"]}`)
	in := EvalInput{
		StatusCodes:  []int{200},
		ActualStatus: 200,
		Body:         []byte(`{}`),
		Schema:       compiled,
	}
	got := Evaluate(in)
	if got == nil {
		t.Fatal("expected non-nil results")
	}
	if got.Passed {
		t.Error("expected Passed=false due to missing required field")
	}
	sawStatus, sawSchema := false, false
	for _, r := range got.Items {
		if r.Type == TypeStatus && r.Passed {
			sawStatus = true
		}
		// Exact discriminator match: before the identity split this had to
		// prefix-test "schema " because Type carried the instance path too.
		if r.Type == TypeSchema && !r.Passed {
			sawSchema = true
		}
	}
	if !sawStatus || !sawSchema {
		t.Errorf("missing expected result rows: %+v", got.Items)
	}
}

func TestEvaluate_schema_nil_no_assertions_returns_nil(t *testing.T) {
	// Evaluate with no assertions at all must still return nil.
	got := Evaluate(EvalInput{ActualStatus: 200})
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

// --- Header exists: false must assert absence (dogfood defect 2) ---
//
// CLI_SPECIFICATION §7.2 documents the header `exists` operator as "Presence
// (`true`) or absence (`false`)". Passing false behaved identically to passing
// true, so an absent header reported "expected exists, got header not present"
// — the very condition the assertion asked for. There was no way to express
// "this response must not carry Set-Cookie".

func TestCheckHeaders_ExistsFalsePassesWhenHeaderAbsent(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"application/json"}}

	results := CheckHeaders([]HeaderInput{
		{Name: "X-Not-Sent", Operator: "exists", Value: "false"},
	}, headers)

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if !results[0].Passed {
		t.Errorf("exists:false against an absent header failed: expected %q, actual %q",
			results[0].Expected, results[0].Actual)
	}
}

func TestCheckHeaders_ExistsFalseFailsWhenHeaderPresent(t *testing.T) {
	headers := http.Header{"Set-Cookie": []string{"session=abc"}}

	results := CheckHeaders([]HeaderInput{
		{Name: "Set-Cookie", Operator: "exists", Value: "false"},
	}, headers)

	if results[0].Passed {
		t.Error("exists:false against a present header passed; it must fail")
	}
	if !strings.Contains(fmt.Sprint(results[0].Expected), "absent") {
		t.Errorf("Expected = %q, want it to say the header should be absent", results[0].Expected)
	}
	if !strings.Contains(fmt.Sprint(results[0].Actual), "session=abc") {
		t.Errorf("Actual = %q, want the offending value quoted", results[0].Actual)
	}
}

func TestCheckHeaders_ExistsTrueIsUnchanged(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"application/json"}}

	present := CheckHeaders([]HeaderInput{
		{Name: "Content-Type", Operator: "exists", Value: "true"},
	}, headers)
	if !present[0].Passed {
		t.Error("exists:true against a present header must pass")
	}

	absent := CheckHeaders([]HeaderInput{
		{Name: "X-Not-Sent", Operator: "exists", Value: "true"},
	}, headers)
	if absent[0].Passed {
		t.Error("exists:true against an absent header must fail")
	}
}

func TestCheckHeaders_ExistsWithUnparseableValueExpectsPresence(t *testing.T) {
	// Backwards compatible: anything that is not a recognisable boolean keeps
	// the historical meaning rather than becoming a new error class.
	headers := http.Header{"Content-Type": []string{"application/json"}}

	results := CheckHeaders([]HeaderInput{
		{Name: "Content-Type", Operator: "exists", Value: "yes please"},
	}, headers)
	if !results[0].Passed {
		t.Error("a non-boolean exists value should still assert presence")
	}
}

func TestCheckBody_ExistsFalseAssertsAbsence(t *testing.T) {
	// The body path has the same shape as the header path and had the same
	// hole. not_exists already worked; exists:false silently meant exists:true.
	body := []byte(`{"present":1}`)

	absent := CheckBody([]BodyInput{
		{Path: "$.missing", Operator: "exists", Value: false},
	}, body)
	if !absent[0].Passed {
		t.Errorf("exists:false on a missing path failed: expected %q, actual %q",
			absent[0].Expected, absent[0].Actual)
	}

	present := CheckBody([]BodyInput{
		{Path: "$.present", Operator: "exists", Value: false},
	}, body)
	if present[0].Passed {
		t.Error("exists:false on a present path passed; it must fail")
	}
}

func TestCheckBody_ExistsTrueIsUnchanged(t *testing.T) {
	body := []byte(`{"present":1}`)

	if r := CheckBody([]BodyInput{
		{Path: "$.present", Operator: "exists", Value: true},
	}, body); !r[0].Passed {
		t.Error("exists:true on a present path must pass")
	}
	if r := CheckBody([]BodyInput{
		{Path: "$.missing", Operator: "exists", Value: true},
	}, body); r[0].Passed {
		t.Error("exists:true on a missing path must fail")
	}
}
