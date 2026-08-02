package parallel

import (
	"testing"

	"github.com/peterlindqvist/apitest/internal/parser"
)

// set is a test helper to create a map[string]bool from names.
func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

func TestScanVariables(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		preExecVars map[string]bool
		want        map[string]bool
	}{
		{"simple variable", "{{user_id}}", nil, set("user_id")},
		{"multiple variables", "{{a}} and {{b}}", nil, set("a", "b")},
		{"dynamic function excluded", "{{$timestamp}}", nil, set()},
		{"pre-exec var excluded", "{{base_url}}", set("base_url"), set()},
		{"default syntax strips pipe", "{{user_id|default:123}}", nil, set("user_id")},
		{"nested dot access", "{{user.id}}", nil, set("user.id")},
		{"mixed dynamic and regular", "{{$uuid}} {{token}}", nil, set("token")},
		{"no variables", "plain text", nil, set()},
		{"single braces ignored", "{not_a_var}", nil, set()},
		{"empty input", "", nil, set()},
		{"underscore prefix", "{{_private}}", nil, set("_private")},
		{"multiple dynamic excluded", "{{$timestamp}} {{$uuid}} {{$randomInt}}", nil, set()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScanVariables(tt.input, tt.preExecVars)
			if len(got) != len(tt.want) {
				t.Errorf("ScanVariables() returned %d vars, want %d\ngot:  %v\nwant: %v",
					len(got), len(tt.want), got, tt.want)
				return
			}
			for k := range tt.want {
				if !got[k] {
					t.Errorf("ScanVariables() missing variable %q", k)
				}
			}
		})
	}
}

func TestScanRequestFields(t *testing.T) {
	tests := []struct {
		name        string
		item        *parser.RequestItem
		preExecVars map[string]bool
		want        map[string]bool
	}{
		{"url variable", &parser.RequestItem{
			Name:    "test",
			Request: parser.Request{URL: "{{base_url}}/users/{{user_id}}"},
		}, set("base_url"), set("user_id")},
		{"header variables", &parser.RequestItem{
			Name:    "test",
			Request: parser.Request{Headers: map[string]string{"Auth": "Bearer {{token}}"}},
		}, nil, set("token")},
		{"body string variable", &parser.RequestItem{
			Name:    "test",
			Request: parser.Request{Body: `{"id": "{{id}}"}`},
		}, nil, set("id")},
		{"query param variable", &parser.RequestItem{
			Name:    "test",
			Request: parser.Request{QueryParams: map[string]string{"page": "{{page}}"}},
		}, nil, set("page")},
		{"extract path variable", &parser.RequestItem{
			Name:    "test",
			Extract: map[string]string{"id": "$.users[{{idx}}].id"},
		}, nil, set("idx")},
		{"assertion body path variable", &parser.RequestItem{
			Name: "test",
			Assertions: parser.Assertions{
				Body: parser.BodyAssertions{
					Items: []parser.BodyAssertion{
						{Path: "$.users.{{field}}", Operator: "exists", Value: true},
					},
				},
			},
		}, nil, set("field")},
		{"no variables anywhere", &parser.RequestItem{
			Name:    "plain",
			Request: parser.Request{Method: "GET", URL: "http://example.com"},
		}, nil, set()},
		{"body map variable", &parser.RequestItem{
			Name: "test",
			Request: parser.Request{Body: map[string]any{
				"name": "{{username}}",
				"age":  42,
			}},
		}, nil, set("username")},
		{"method variable", &parser.RequestItem{
			Name:    "test",
			Request: parser.Request{Method: "{{method}}"},
		}, nil, set("method")},
		{"nil item returns empty", nil, nil, set()},
		{"body array of objects", &parser.RequestItem{
			Name: "test",
			Request: parser.Request{Body: []any{
				map[string]any{"id": "{{item_id}}"},
				"plain",
			}},
		}, nil, set("item_id")},
		{"body map string string", &parser.RequestItem{
			Name: "test",
			Request: parser.Request{Body: map[string]string{
				"key": "{{map_var}}",
			}},
		}, nil, set("map_var")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScanRequestFields(tt.item, tt.preExecVars)
			if len(got) != len(tt.want) {
				t.Errorf("ScanRequestFields() returned %d vars, want %d\ngot:  %v\nwant: %v",
					len(got), len(tt.want), got, tt.want)
				return
			}
			for k := range tt.want {
				if !got[k] {
					t.Errorf("ScanRequestFields() missing variable %q", k)
				}
			}
		})
	}
}

func TestExtractProducedVars(t *testing.T) {
	tests := []struct {
		name      string
		extract   map[string]string
		wantVars  map[string]bool
		wantError bool
	}{
		{"single extraction", map[string]string{"user_id": "$.id"}, set("user_id"), false},
		{"multiple extractions", map[string]string{"a": "$.a", "b": "$.b"}, set("a", "b"), false},
		{"dynamic key rejected", map[string]string{"{{prefix}}_id": "$.id"}, nil, true},
		{"nil extract", nil, set(), false},
		{"empty extract", map[string]string{}, set(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVars, gotErr := ExtractProducedVars(tt.extract)
			if tt.wantError {
				if gotErr == "" {
					t.Error("ExtractProducedVars() expected error, got none")
				}
				return
			}
			if gotErr != "" {
				t.Errorf("ExtractProducedVars() unexpected error: %s", gotErr)
				return
			}
			if len(gotVars) != len(tt.wantVars) {
				t.Errorf("ExtractProducedVars() returned %d vars, want %d\ngot:  %v\nwant: %v",
					len(gotVars), len(tt.wantVars), gotVars, tt.wantVars)
				return
			}
			for k := range tt.wantVars {
				if !gotVars[k] {
					t.Errorf("ExtractProducedVars() missing variable %q", k)
				}
			}
		})
	}
}

func TestScanVariables_SecretsNamespace(t *testing.T) {
	// {{secrets.X}} tokens use a dot which means varScanPattern would pick
	// them up as "secrets.X" — they should NOT be treated as run-time variable
	// dependencies because secrets are resolved before the first wave.
	tests := []struct {
		name        string
		input       string
		preExecVars map[string]bool
		want        map[string]bool
	}{
		{
			name:  "secrets namespace is not a dependency",
			input: "Bearer {{secrets.api_key}}",
			want:  set(), // secrets resolved before execution — not a per-request dep
		},
		{
			name:  "secrets and regular var together",
			input: "{{secrets.api_key}}/{{user_id}}",
			want:  set("user_id"),
		},
		{
			name:        "secrets with pre-exec regular var",
			input:       "{{secrets.api_key}}/{{base_url}}",
			preExecVars: set("base_url"),
			want:        set(), // base_url is pre-exec, secrets not a dep
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScanVariables(tt.input, tt.preExecVars)
			// Remove secrets.* keys — the parallel scanner should not see them
			// as execution-time dependencies.
			for k := range got {
				if len(k) > 8 && k[:8] == "secrets." {
					// secrets.X appeared in result — that's the bug we're testing against
					t.Errorf("ScanVariables() should not include secrets namespace %q as dependency", k)
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("ScanVariables() returned %d vars, want %d\ngot:  %v\nwant: %v",
					len(got), len(tt.want), got, tt.want)
				return
			}
			for k := range tt.want {
				if !got[k] {
					t.Errorf("ScanVariables() missing variable %q", k)
				}
			}
		})
	}
}
