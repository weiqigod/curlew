package variable

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	apierrors "github.com/weiqigod/curlew/internal/errors"
)

func TestScope_Interpolate(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[string]string
		input   string
		want    string
		wantErr error
	}{
		{"simple variable in URL", map[string]string{"base_url": "https://example.com"}, "{{base_url}}/get", "https://example.com/get", nil},
		{"multiple variables in same string", map[string]string{"host": "example.com", "path": "api"}, "https://{{host}}/{{path}}", "https://example.com/api", nil},
		{"variable in middle of string", map[string]string{"host": "example.com", "ver": "v1"}, "https://{{host}}/{{ver}}/resource", "https://example.com/v1/resource", nil},
		{"no variables passes through unchanged", map[string]string{"x": "y"}, "no vars here", "no vars here", nil},
		{"undefined variable", map[string]string{"a": "1"}, "{{unknown}}", "", ErrUndefinedVariable},
		{"special characters in value", map[string]string{"q": "a&b=c?d{e}f"}, "{{q}}", "a&b=c?d{e}f", nil},
		{"empty variable value", map[string]string{"empty": ""}, "pre{{empty}}post", "prepost", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScope(tt.vars)
			if err := s.Resolve(); err != nil {
				t.Fatalf("Resolve() error: %v", err)
			}
			got, err := s.Interpolate(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Interpolate() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Interpolate() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Interpolate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScope_Resolve(t *testing.T) {
	// Build a chain of N variables: v0 -> v1 -> ... -> vN-1 -> "done"
	makeChain := func(n int) map[string]string {
		m := make(map[string]string, n)
		for i := 0; i < n-1; i++ {
			m[fmt.Sprintf("v%d", i)] = fmt.Sprintf("{{v%d}}", i+1)
		}
		m[fmt.Sprintf("v%d", n-1)] = "done"
		return m
	}

	tests := []struct {
		name    string
		vars    map[string]string
		wantErr error
	}{
		{"no references - all literal values", map[string]string{"a": "1", "b": "2"}, nil},
		{"simple chain a->b", map[string]string{"a": "{{b}}", "b": "value"}, nil},
		{"two-level chain a->b->c", map[string]string{"a": "{{b}}", "b": "{{c}}", "c": "done"}, nil},
		{"circular reference a->b->a", map[string]string{"a": "{{b}}", "b": "{{a}}"}, ErrCircularReference},
		{"self-reference", map[string]string{"a": "{{a}}"}, ErrCircularReference},
		{"three-way cycle", map[string]string{"a": "{{b}}", "b": "{{c}}", "c": "{{a}}"}, ErrCircularReference},
		{"depth exactly 10 succeeds", makeChain(10), nil},
		{"depth 11 exceeds limit", makeChain(11), ErrDepthExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScope(tt.vars)
			err := s.Resolve()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Resolve() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() unexpected error: %v", err)
			}
		})
	}
}

func TestScope_Resolve_chain_value(t *testing.T) {
	// After resolving a->b->c, interpolating {{a}} should give "done"
	s := NewScope(map[string]string{"a": "{{b}}", "b": "{{c}}", "c": "done"})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	got, err := s.Interpolate("{{a}}")
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if got != "done" {
		t.Errorf("Interpolate({{a}}) = %q, want %q", got, "done")
	}
}

func TestScope_Resolve_error_messages(t *testing.T) {
	t.Run("circular path in error message includes cycle", func(t *testing.T) {
		s := NewScope(map[string]string{"a": "{{b}}", "b": "{{a}}"})
		err := s.Resolve()
		if err == nil {
			t.Fatal("expected error")
		}
		msg := err.Error()
		if !strings.Contains(msg, "a") || !strings.Contains(msg, "b") {
			t.Errorf("error message %q should mention cycle variables", msg)
		}
	})

	t.Run("depth path in error message includes resolution chain", func(t *testing.T) {
		vars := make(map[string]string, 11)
		for i := 0; i < 10; i++ {
			vars[fmt.Sprintf("v%d", i)] = fmt.Sprintf("{{v%d}}", i+1)
		}
		vars["v10"] = "done"
		s := NewScope(vars)
		err := s.Resolve()
		if err == nil {
			t.Fatal("expected error")
		}
		msg := err.Error()
		if !strings.Contains(msg, "v0") {
			t.Errorf("error message %q should include chain info", msg)
		}
	})

	t.Run("undefined variable error has actionable hint", func(t *testing.T) {
		// M6-007: the hint was changed from listing available variables to a
		// canonical action verb ("Define"), which is required by the agent
		// diagnosability contract (hint_contains_any: [Define, Set, ...]).
		s := NewScope(map[string]string{"alpha": "1", "beta": "2"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}
		_, err := s.Interpolate("{{unknown}}")
		if err == nil {
			t.Fatal("expected error")
		}
		var se *apierrors.Structured
		if !errors.As(err, &se) {
			t.Fatalf("expected *apierrors.Structured, got %T", err)
		}
		// Hint must contain at least one canonical action verb.
		hasActionVerb := strings.Contains(se.Hint, "Define") ||
			strings.Contains(se.Hint, "Set") ||
			strings.Contains(se.Hint, "Add") ||
			strings.Contains(se.Hint, "Pass")
		if !hasActionVerb {
			t.Errorf("hint %q must contain a canonical action verb (Define/Set/Add/Pass)", se.Hint)
		}
	})
}

func TestScope_InterpolateMap(t *testing.T) {
	t.Run("headers map with variables in values", func(t *testing.T) {
		s := NewScope(map[string]string{"token": "abc123"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}
		got, err := s.InterpolateMap(map[string]string{"Authorization": "Bearer {{token}}"})
		if err != nil {
			t.Fatalf("InterpolateMap() error: %v", err)
		}
		if got["Authorization"] != "Bearer abc123" {
			t.Errorf("Authorization = %q, want %q", got["Authorization"], "Bearer abc123")
		}
	})

	t.Run("query params with variables in values", func(t *testing.T) {
		s := NewScope(map[string]string{"ver": "v2"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}
		got, err := s.InterpolateMap(map[string]string{"version": "{{ver}}"})
		if err != nil {
			t.Fatalf("InterpolateMap() error: %v", err)
		}
		if got["version"] != "v2" {
			t.Errorf("version = %q, want %q", got["version"], "v2")
		}
	})

	t.Run("nil map returns nil", func(t *testing.T) {
		s := NewScope(map[string]string{"a": "1"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}
		got, err := s.InterpolateMap(nil)
		if err != nil {
			t.Fatalf("InterpolateMap() error: %v", err)
		}
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

func TestScope_InterpolateBody(t *testing.T) {
	t.Run("string body with variable", func(t *testing.T) {
		s := NewScope(map[string]string{"name": "test"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}
		got, err := s.InterpolateBody("hello {{name}}")
		if err != nil {
			t.Fatalf("InterpolateBody() error: %v", err)
		}
		if got != "hello test" {
			t.Errorf("got %q, want %q", got, "hello test")
		}
	})

	t.Run("map body with variable in values", func(t *testing.T) {
		s := NewScope(map[string]string{"email": "test@example.com"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}
		body := map[string]interface{}{"email": "{{email}}", "count": 42}
		got, err := s.InterpolateBody(body)
		if err != nil {
			t.Fatalf("InterpolateBody() error: %v", err)
		}
		m, ok := got.(map[string]interface{})
		if !ok {
			t.Fatalf("got %T, want map[string]interface{}", got)
		}
		if m["email"] != "test@example.com" {
			t.Errorf("email = %v, want %q", m["email"], "test@example.com")
		}
		if m["count"] != 42 {
			t.Errorf("count = %v, want 42", m["count"])
		}
	})

	t.Run("nested map body", func(t *testing.T) {
		s := NewScope(map[string]string{"val": "deep"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}
		body := map[string]interface{}{
			"outer": map[string]interface{}{
				"inner": "{{val}}",
			},
		}
		got, err := s.InterpolateBody(body)
		if err != nil {
			t.Fatalf("InterpolateBody() error: %v", err)
		}
		m := got.(map[string]interface{})
		inner := m["outer"].(map[string]interface{})
		if inner["inner"] != "deep" {
			t.Errorf("inner = %v, want %q", inner["inner"], "deep")
		}
	})

	t.Run("nil body returns nil", func(t *testing.T) {
		s := NewScope(map[string]string{"a": "1"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}
		got, err := s.InterpolateBody(nil)
		if err != nil {
			t.Fatalf("InterpolateBody() error: %v", err)
		}
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("array body with variable in string elements", func(t *testing.T) {
		s := NewScope(map[string]string{"item": "resolved"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}
		body := []interface{}{"{{item}}", 123}
		got, err := s.InterpolateBody(body)
		if err != nil {
			t.Fatalf("InterpolateBody() error: %v", err)
		}
		arr, ok := got.([]interface{})
		if !ok {
			t.Fatalf("got %T, want []interface{}", got)
		}
		if arr[0] != "resolved" {
			t.Errorf("arr[0] = %v, want %q", arr[0], "resolved")
		}
		if arr[1] != 123 {
			t.Errorf("arr[1] = %v, want 123", arr[1])
		}
	})
}

func TestScope_empty_scope(t *testing.T) {
	s := NewScope(map[string]string{})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	got, err := s.Interpolate("no vars here")
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if got != "no vars here" {
		t.Errorf("got %q, want %q", got, "no vars here")
	}
}

func TestScope_AvailableVars(t *testing.T) {
	s := NewScope(map[string]string{"beta": "2", "alpha": "1", "gamma": "3"})
	got := s.AvailableVars()
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestScope_Set(t *testing.T) {
	tests := []struct {
		name     string
		initial  map[string]string
		setName  string
		setValue string
		template string
		want     string
	}{
		{"adds new variable", nil, "token", "abc123", "Bearer {{token}}", "Bearer abc123"},
		{"overrides existing variable", map[string]string{"token": "old"}, "token", "new", "{{token}}", "new"},
		{"on empty scope", map[string]string{}, "x", "1", "{{x}}", "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScope(tt.initial)
			if err := s.Resolve(); err != nil {
				t.Fatalf("Resolve() error: %v", err)
			}
			s.Set(tt.setName, tt.setValue)
			got, err := s.Interpolate(tt.template)
			if err != nil {
				t.Fatalf("Interpolate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Interpolate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseVarFlag(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantKey   string
		wantValue string
		wantErr   bool
	}{
		{"simple_key_value", "base_url=http://localhost:8080", "base_url", "http://localhost:8080", false},
		{"value_with_equals", "name=value=with=equals", "name", "value=with=equals", false},
		{"empty_value", "key=", "key", "", false},
		{"url_value", "url=https://example.com/api?q=1&x=2", "url", "https://example.com/api?q=1&x=2", false},
		{"no_equals_sign", "justkey", "", "", true},
		{"empty_key", "=value", "", "", true},
		{"empty_string", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value, err := ParseVarFlag(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, ErrInvalidVarFlag) {
					t.Errorf("error = %v, want ErrInvalidVarFlag", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
			if value != tt.wantValue {
				t.Errorf("value = %q, want %q", value, tt.wantValue)
			}
		})
	}
}

func TestScope_WithOverrides(t *testing.T) {
	tests := []struct {
		name        string
		baseVars    map[string]string
		overrides   map[string]string
		interpolate string
		want        string
		wantErr     bool
	}{
		{"adds_new_variable", map[string]string{"a": "1"}, map[string]string{"b": "2"}, "{{b}}", "2", false},
		{"overrides_existing_variable", map[string]string{"a": "old"}, map[string]string{"a": "new"}, "{{a}}", "new", false},
		{"nil_overrides_returns_same_scope", map[string]string{"a": "1"}, nil, "{{a}}", "1", false},
		{"empty_overrides_returns_same_scope", map[string]string{"a": "1"}, map[string]string{}, "{{a}}", "1", false},
		{"override_references_base_variable", map[string]string{"host": "example.com"}, map[string]string{"url": "https://{{host}}/api"}, "{{url}}", "https://example.com/api", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := NewScope(tt.baseVars)
			if err := base.Resolve(); err != nil {
				t.Fatalf("Resolve() error: %v", err)
			}
			child, err := base.WithOverrides(tt.overrides)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("WithOverrides() error: %v", err)
			}
			got, interpErr := child.Interpolate(tt.interpolate)
			if interpErr != nil {
				t.Fatalf("Interpolate() error: %v", interpErr)
			}
			if got != tt.want {
				t.Errorf("Interpolate(%q) = %q, want %q", tt.interpolate, got, tt.want)
			}
		})
	}

	t.Run("does_not_mutate_original_scope", func(t *testing.T) {
		base := NewScope(map[string]string{"a": "original"})
		if err := base.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}
		_, err := base.WithOverrides(map[string]string{"a": "override"})
		if err != nil {
			t.Fatalf("WithOverrides() error: %v", err)
		}
		// Base scope should be unchanged
		got, interpErr := base.Interpolate("{{a}}")
		if interpErr != nil {
			t.Fatalf("Interpolate() error: %v", interpErr)
		}
		if got != "original" {
			t.Errorf("base scope Interpolate({{a}}) = %q, want %q", got, "original")
		}
	})
}

func TestParseEnvVarFlag(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		envState  map[string]string
		wantKey   string
		wantValue string
		wantErr   error
	}{
		{"bare_name_set", "API_KEY", map[string]string{"API_KEY": "secret"}, "API_KEY", "secret", nil},
		{"bare_name_unset", "API_KEY", map[string]string{}, "", "", ErrEnvVarNotSet},
		{"mapped_name_set", "API_KEY=$CI_KEY", map[string]string{"CI_KEY": "secret"}, "API_KEY", "secret", nil},
		{"mapped_name_unset", "API_KEY=$CI_KEY", map[string]string{}, "", "", ErrEnvVarNotSet},
		{"empty_string", "", nil, "", "", ErrInvalidEnvVarFlag},
		{"bare_name_empty_value", "API_KEY", map[string]string{"API_KEY": ""}, "API_KEY", "", nil},
		{"mapped_name_empty_value", "API_KEY=$CI_KEY", map[string]string{"CI_KEY": ""}, "API_KEY", "", nil},
		{"equals_in_env_var_name", "KEY=$VAR_WITH_EQUALS", map[string]string{"VAR_WITH_EQUALS": "val"}, "KEY", "val", nil},
		{"empty_varname_mapped", "=$OS_VAR", map[string]string{"OS_VAR": "val"}, "", "", ErrInvalidEnvVarFlag},
		{"empty_envname_equals_only", "KEY=", nil, "", "", ErrInvalidEnvVarFlag},
		{"empty_envname_dollar_only", "KEY=$", nil, "", "", ErrInvalidEnvVarFlag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(name string) (string, bool) {
				v, ok := tt.envState[name]
				return v, ok
			}
			key, value, err := ParseEnvVarFlag(tt.input, lookup)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
			if value != tt.wantValue {
				t.Errorf("value = %q, want %q", value, tt.wantValue)
			}
		})
	}
}

func TestScope_WithOverrides_propagates_registry(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)

	base := NewScope(map[string]string{"key": "value"})
	if err := base.Resolve(); err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	base = base.WithDynamic(reg)

	// WithOverrides should preserve the registry so {{$uuid}} resolves in the child scope.
	child, err := base.WithOverrides(map[string]string{"extra": "override"})
	if err != nil {
		t.Fatalf("WithOverrides() error: %v", err)
	}

	child.BeginRequest()
	defer child.EndRequest()

	got, err := child.Interpolate("{{$uuid}}")
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if !uuidPattern.MatchString(got) {
		t.Errorf("got %q, expected UUID v4 pattern — registry was not propagated by WithOverrides", got)
	}
}

func TestScope_StructuredErrors(t *testing.T) {
	t.Run("circular reference is structured error", func(t *testing.T) {
		s := NewScope(map[string]string{"a": "{{b}}", "b": "{{a}}"})
		err := s.Resolve()
		var se *apierrors.Structured
		if !errors.As(err, &se) {
			t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
		}
		if se.Category != apierrors.CategoryConfig {
			t.Errorf("Category = %q, want %q", se.Category, apierrors.CategoryConfig)
		}
	})

	t.Run("depth exceeded is structured error", func(t *testing.T) {
		vars := make(map[string]string, 12)
		for i := 0; i < 11; i++ {
			vars[fmt.Sprintf("v%d", i)] = fmt.Sprintf("{{v%d}}", i+1)
		}
		vars["v11"] = "done"
		s := NewScope(vars)
		err := s.Resolve()
		var se *apierrors.Structured
		if !errors.As(err, &se) {
			t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
		}
		if se.Category != apierrors.CategoryConfig {
			t.Errorf("Category = %q, want %q", se.Category, apierrors.CategoryConfig)
		}
	})

	t.Run("undefined variable is structured error", func(t *testing.T) {
		s := NewScope(map[string]string{"a": "1"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}
		_, err := s.Interpolate("{{unknown}}")
		var se *apierrors.Structured
		if !errors.As(err, &se) {
			t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
		}
		// M6-007: undefined variable is categorised as input (user omitted a value),
		// not config (a config file is wrong). Category was previously CategoryConfig.
		if se.Category != apierrors.CategoryInput {
			t.Errorf("Category = %q, want %q", se.Category, apierrors.CategoryInput)
		}
		if se.Code != "VAR_UNDEFINED" {
			t.Errorf("Code = %q, want VAR_UNDEFINED", se.Code)
		}
	})
}

func TestFindReferences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"no_refs", "hello world", nil},
		{"single_ref", "{{foo}}", []string{"foo"}},
		{"multiple_refs", "{{a}} and {{b}}", []string{"a", "b"}},
		{"dynamic_func_excluded", "{{$uuid}}", nil},
		{"mixed_refs_and_funcs", "{{a}} {{$uuid}} {{b}}", []string{"a", "b"}},
		{"empty_string", "", nil},
		{"ref_in_url", "https://example.com/{{id}}/items", []string{"id"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindReferences(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("FindReferences(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("FindReferences(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestScope_Interpolate_dynamic(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)

	t.Run("uuid in url", func(t *testing.T) {
		s := NewScope(nil)
		if err := s.Resolve(); err != nil {
			t.Fatal(err)
		}
		s = s.WithDynamic(reg)
		s.BeginRequest()
		defer s.EndRequest()
		got, err := s.Interpolate("https://api/{{$uuid}}")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(got, "https://api/") {
			t.Errorf("prefix missing: %q", got)
		}
		isValidUUIDv4(t, strings.TrimPrefix(got, "https://api/"))
	})

	t.Run("timestamp replaced", func(t *testing.T) {
		s := NewScope(nil)
		if err := s.Resolve(); err != nil {
			t.Fatal(err)
		}
		s = s.WithDynamic(reg)
		s.BeginRequest()
		defer s.EndRequest()
		got, err := s.Interpolate("ts={{$timestamp}}")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(got, "ts=") {
			t.Errorf("prefix missing: %q", got)
		}
		isValidUnixSeconds(t, strings.TrimPrefix(got, "ts="))
	})

	t.Run("two references same value", func(t *testing.T) {
		s := NewScope(nil)
		if err := s.Resolve(); err != nil {
			t.Fatal(err)
		}
		s = s.WithDynamic(reg)
		s.BeginRequest()
		defer s.EndRequest()
		got, err := s.Interpolate("{{$uuid}}-{{$uuid}}")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		parts := strings.SplitN(got, "-", 2)
		// UUID is 36 chars; split at index 36
		if len(got) < 73 { // 36 + 1 dash + 36
			t.Fatalf("result too short: %q", got)
		}
		first := got[:36]
		second := got[37:] // skip the separator dash
		if first != second {
			t.Errorf("same request: uuid should be memoized, got %q vs %q", first, second)
		}
		_ = parts
	})

	t.Run("explicit var overrides function", func(t *testing.T) {
		s := NewScope(map[string]string{"$uuid": "fixed-value"})
		if err := s.Resolve(); err != nil {
			t.Fatal(err)
		}
		s = s.WithDynamic(reg)
		s.BeginRequest()
		defer s.EndRequest()
		got, err := s.Interpolate("{{$uuid}}")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "fixed-value" {
			t.Errorf("explicit var should override dynamic function; got %q", got)
		}
	})

	t.Run("mixed normal and dynamic vars", func(t *testing.T) {
		s := NewScope(map[string]string{"base": "http://x"})
		if err := s.Resolve(); err != nil {
			t.Fatal(err)
		}
		s = s.WithDynamic(reg)
		s.BeginRequest()
		defer s.EndRequest()
		got, err := s.Interpolate("{{base}}/{{$uuid}}")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(got, "http://x/") {
			t.Errorf("expected http://x/ prefix, got %q", got)
		}
	})

	t.Run("unknown function returns error", func(t *testing.T) {
		s := NewScope(nil)
		if err := s.Resolve(); err != nil {
			t.Fatal(err)
		}
		s = s.WithDynamic(reg)
		s.BeginRequest()
		defer s.EndRequest()
		_, err := s.Interpolate("{{$unknownFunc}}")
		if err == nil {
			t.Fatal("expected error for unknown function, got nil")
		}
	})

	t.Run("no registry leaves pattern", func(t *testing.T) {
		s := NewScope(nil)
		if err := s.Resolve(); err != nil {
			t.Fatal(err)
		}
		// No WithDynamic call — registry is nil
		got, err := s.Interpolate("{{$uuid}}")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "{{$uuid}}" {
			t.Errorf("without registry, pattern should be left as-is; got %q", got)
		}
	})
}

func TestScope_Snapshot(t *testing.T) {
	t.Run("basic copy", func(t *testing.T) {
		s := NewScope(map[string]string{"host": "example.com", "port": "8080"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}

		snap := s.Snapshot()

		// Snapshot should have the same resolved values
		orig := s.Resolved()
		got := snap.Resolved()
		if len(got) != len(orig) {
			t.Fatalf("Snapshot Resolved() len = %d, want %d", len(got), len(orig))
		}
		for k, v := range orig {
			if got[k] != v {
				t.Errorf("Snapshot Resolved()[%q] = %q, want %q", k, got[k], v)
			}
		}
	})

	t.Run("independence from parent mutations", func(t *testing.T) {
		s := NewScope(map[string]string{"x": "1", "y": "2"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}

		snap := s.Snapshot()

		// Mutate the parent scope
		s.Set("x", "mutated")
		s.Set("z", "new")

		// Snapshot should still have original values
		snapResolved := snap.Resolved()
		if snapResolved["x"] != "1" {
			t.Errorf("snapshot x = %q, want %q (parent mutation leaked)", snapResolved["x"], "1")
		}
		if _, ok := snapResolved["z"]; ok {
			t.Error("snapshot has 'z' which was added to parent after snapshot")
		}

		// Mutate the snapshot
		snap.Set("y", "snap_mutated")

		// Parent should still have original value
		parentResolved := s.Resolved()
		if parentResolved["y"] != "2" {
			t.Errorf("parent y = %q, want %q (snapshot mutation leaked)", parentResolved["y"], "2")
		}
	})

	t.Run("preserves dynamic registry", func(t *testing.T) {
		s := NewScope(map[string]string{"key": "val"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}

		seed := int64(42)
		reg := NewRegistry(&seed)
		s = s.WithDynamic(reg)

		snap := s.Snapshot()

		// Snapshot should be able to interpolate dynamic functions
		snap.BeginRequest()
		result, err := snap.Interpolate("{{$uuid}}")
		snap.EndRequest()

		if err != nil {
			t.Fatalf("Interpolate error: %v", err)
		}
		if result == "" || result == "{{$uuid}}" {
			t.Errorf("dynamic function not resolved in snapshot: %q", result)
		}
	})

	t.Run("nil resolved map", func(t *testing.T) {
		s := NewScope(nil)
		// Don't call Resolve() — resolved map is nil

		snap := s.Snapshot()

		got := snap.Resolved()
		if got == nil {
			t.Error("Snapshot Resolved() returned nil, want empty map")
		}
		if len(got) != 0 {
			t.Errorf("Snapshot Resolved() len = %d, want 0", len(got))
		}
	})

	t.Run("nil vars map", func(t *testing.T) {
		s := &Scope{} // zero value — both vars and resolved are nil

		snap := s.Snapshot()

		if avail := snap.AvailableVars(); len(avail) != 0 {
			t.Errorf("Snapshot AvailableVars() = %v, want empty", avail)
		}
	})

	t.Run("funcCache is independent", func(t *testing.T) {
		s := NewScope(map[string]string{"a": "1"})
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}

		// Begin request on parent
		s.BeginRequest()

		snap := s.Snapshot()

		// Snapshot should have nil funcCache (independent)
		// BeginRequest on snapshot should not affect parent
		snap.BeginRequest()
		snap.EndRequest()

		// Parent should still have its own funcCache active
		s.EndRequest()
	})
}

func TestScope_Resolved(t *testing.T) {
	s := NewScope(map[string]string{"key1": "val1", "key2": "val2"})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	got := s.Resolved()
	want := map[string]string{"key1": "val1", "key2": "val2"}
	if len(got) != len(want) {
		t.Fatalf("Resolved() len = %d, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("Resolved()[%q] = %q, want %q", k, got[k], v)
		}
	}
	// Mutating the returned map should not affect the scope
	got["key1"] = "mutated"
	if s.Resolved()["key1"] != "val1" {
		t.Error("Resolved() returned a non-copy")
	}
}

func TestScope_Interpolate_Secrets(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[string]string
		secrets map[string]string
		input   string
		want    string
		wantErr error
	}{
		{
			name:    "resolves simple alias",
			secrets: map[string]string{"api_key": "abc"},
			input:   "Bearer {{secrets.api_key}}",
			want:    "Bearer abc",
		},
		{
			name:    "multiple aliases in one string",
			secrets: map[string]string{"api_key": "abc", "db_password": "pw"},
			input:   "{{secrets.api_key}}:{{secrets.db_password}}",
			want:    "abc:pw",
		},
		{
			name:    "unknown alias returns ErrUnknownSecret",
			secrets: map[string]string{"api_key": "abc"},
			input:   "{{secrets.db_password}}",
			wantErr: ErrUnknownSecret,
		},
		{
			name:  "no secrets map and no reference is a no-op",
			input: "plain",
			want:  "plain",
		},
		{
			name:    "no secrets map with reference returns ErrUnknownSecret",
			input:   "{{secrets.api_key}}",
			wantErr: ErrUnknownSecret,
		},
		{
			name:    "mixed with regular vars",
			vars:    map[string]string{"host": "x"},
			secrets: map[string]string{"api_key": "abc"},
			input:   "https://{{host}}/{{secrets.api_key}}",
			want:    "https://x/abc",
		},
		{
			name:    "secrets alias never shadowed by var of same name",
			vars:    map[string]string{"api_key": "plain-var"},
			secrets: map[string]string{"api_key": "from-secrets"},
			input:   "{{secrets.api_key}}/{{api_key}}",
			want:    "from-secrets/plain-var",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := tt.vars
			if vars == nil {
				vars = map[string]string{}
			}
			s := NewScope(vars)
			if err := s.Resolve(); err != nil {
				t.Fatalf("Resolve() error: %v", err)
			}
			if len(tt.secrets) > 0 {
				s = s.WithSecrets(tt.secrets)
			}
			got, err := s.Interpolate(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Interpolate() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Interpolate() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Interpolate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasSecretsNamespace(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"{{secrets.api_key}}", true},
		{"Bearer {{secrets.token}}", true},
		{"{{api_key}}", false},
		{"{{$uuid}}", false},
		{"plain text", false},
		{"{{secrets.}}", false}, // invalid alias — underscore/alpha required
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := HasSecretsNamespace(tt.input)
			if got != tt.want {
				t.Errorf("HasSecretsNamespace(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSecretReferences(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"{{secrets.api_key}}", []string{"api_key"}},
		{"{{secrets.api_key}}:{{secrets.db_password}}", []string{"api_key", "db_password"}},
		{"{{secrets.api_key}}:{{secrets.api_key}}", []string{"api_key"}}, // deduplicated
		{"{{api_key}}", nil},
		{"plain text", nil},
		{"", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SecretReferences(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("SecretReferences(%q) = %v (len %d), want %v (len %d)",
					tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("SecretReferences(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestScope_Snapshot_PreservesSecrets(t *testing.T) {
	secrets := map[string]string{"api_key": "secret-value"}
	s := NewScope(map[string]string{"host": "example.com"})
	if err := s.Resolve(); err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	s = s.WithSecrets(secrets)

	snap := s.Snapshot()
	got, err := snap.Interpolate("{{secrets.api_key}}")
	if err != nil {
		t.Fatalf("Snapshot.Interpolate() error: %v", err)
	}
	if got != "secret-value" {
		t.Errorf("Snapshot.Interpolate({{secrets.api_key}}) = %q, want %q", got, "secret-value")
	}
}

// slicesEqual reports whether two string slices are element-wise equal.
// Both nil and empty slices compare as equal.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDynPattern_ParensSyntax(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantMatch   bool
		wantFunc    string
		wantRawArgs string
	}{
		{"no parens, legacy", "{{$timestamp}}", true, "timestamp", ""},
		{"empty parens", "{{$base64()}}", true, "base64", ""},
		{"single literal arg", "{{$base64('hello')}}", true, "base64", "'hello'"},
		{"two literal args", "{{$hmacSha256('a', 'b')}}", true, "hmacSha256", "'a', 'b'"},
		{"arg with nested var ref", "{{$base64('{{user}}')}}", true, "base64", "'{{user}}'"},
		{"underscore in func name not allowed", "{{$_bad}}", false, "", ""},
		{"missing close brace", "{{$base64('a')", false, "", ""},
		{"unbalanced parens not captured by regex", "{{$base64('a'}}", false, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := dynPattern.FindStringSubmatch(tt.input)
			gotMatch := m != nil
			if gotMatch != tt.wantMatch {
				t.Fatalf("FindStringSubmatch match=%v, want %v (input=%q)", gotMatch, tt.wantMatch, tt.input)
			}
			if !tt.wantMatch {
				return
			}
			if m[1] != tt.wantFunc {
				t.Errorf("funcName = %q, want %q", m[1], tt.wantFunc)
			}
			if m[2] != tt.wantRawArgs {
				t.Errorf("rawArgs = %q, want %q", m[2], tt.wantRawArgs)
			}
		})
	}
}

// TestDynPattern_DottedNamespace verifies that dynPattern accepts
// dot-separated namespace segments in funcName and captures the literal
// dot in capture group 1.
func TestDynPattern_DottedNamespace(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantMatch   bool
		wantFunc    string
		wantRawArgs string
	}{
		{"single dot, no parens", "{{$faker.firstName}}", true, "faker.firstName", ""},
		{"single dot, empty parens", "{{$faker.firstName()}}", true, "faker.firstName", ""},
		{"two dots", "{{$pkg.sub.fn}}", true, "pkg.sub.fn", ""},
		{"consecutive dots rejected", "{{$faker..badName}}", false, "", ""},
		{"leading dot rejected", "{{$.faker.firstName}}", false, "", ""},
		{"trailing dot rejected", "{{$faker.}}", false, "", ""},
		{"dot then underscore-leading rejected", "{{$faker._bad}}", false, "", ""},
		{"dot then digit-leading rejected", "{{$faker.1bad}}", false, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := dynPattern.FindStringSubmatch(tt.input)
			gotMatch := m != nil
			if gotMatch != tt.wantMatch {
				t.Fatalf("FindStringSubmatch match=%v, want %v (input=%q)", gotMatch, tt.wantMatch, tt.input)
			}
			if !tt.wantMatch {
				return
			}
			if m[1] != tt.wantFunc {
				t.Errorf("funcName = %q, want %q", m[1], tt.wantFunc)
			}
			if m[2] != tt.wantRawArgs {
				t.Errorf("rawArgs = %q, want %q", m[2], tt.wantRawArgs)
			}
		})
	}
}

// TestDynPattern_DottedNamespace_WithArgs verifies that the M12-001 arg
// syntax interoperates with dotted funcNames: capture group 2 still
// contains the verbatim arg list.
func TestDynPattern_DottedNamespace_WithArgs(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantFunc    string
		wantRawArgs string
	}{
		{"two int-string args", "{{$faker.imageUrl(640, 480)}}", "faker.imageUrl", "640, 480"},
		{"single quoted arg", "{{$faker.sentence('5')}}", "faker.sentence", "'5'"},
		{"nested var ref in arg", "{{$faker.text('{{n}}')}}", "faker.text", "'{{n}}'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := dynPattern.FindStringSubmatch(tt.input)
			if m == nil {
				t.Fatalf("FindStringSubmatch returned no match for %q", tt.input)
			}
			if m[1] != tt.wantFunc {
				t.Errorf("funcName = %q, want %q", m[1], tt.wantFunc)
			}
			if m[2] != tt.wantRawArgs {
				t.Errorf("rawArgs = %q, want %q", m[2], tt.wantRawArgs)
			}
		})
	}
}

// TestDynPattern_BackwardCompatNoDot verifies that every legacy
// single-segment form continues to match unchanged after the dotted-
// namespace extension.
func TestDynPattern_BackwardCompatNoDot(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantFunc    string
		wantRawArgs string
	}{
		{"timestamp no parens", "{{$timestamp}}", "timestamp", ""},
		{"uuid no parens", "{{$uuid}}", "uuid", ""},
		{"base64 with single arg", "{{$base64('a')}}", "base64", "'a'"},
		{"hmacSha256 with two args", "{{$hmacSha256('p','k')}}", "hmacSha256", "'p','k'"},
		{"randomInt empty parens", "{{$randomInt()}}", "randomInt", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := dynPattern.FindStringSubmatch(tt.input)
			if m == nil {
				t.Fatalf("FindStringSubmatch returned no match for %q (regression)", tt.input)
			}
			if m[1] != tt.wantFunc {
				t.Errorf("funcName = %q, want %q", m[1], tt.wantFunc)
			}
			if m[2] != tt.wantRawArgs {
				t.Errorf("rawArgs = %q, want %q", m[2], tt.wantRawArgs)
			}
		})
	}
}

func TestParseDynArgs(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{"empty", "", nil, false},
		{"single literal", "'hello'", []string{"hello"}, false},
		{"two literals", "'a', 'b'", []string{"a", "b"}, false},
		{"whitespace tolerated around commas", "'a'  ,   'b'", []string{"a", "b"}, false},
		{"escaped quote", `'it\'s ok'`, []string{"it's ok"}, false},
		{"escaped backslash", `'a\\b'`, []string{`a\b`}, false},
		{"nested var placeholder preserved", "'{{user}}-{{$timestamp}}'", []string{"{{user}}-{{$timestamp}}"}, false},
		{"unterminated quote", "'oops", nil, true},
		{"trailing comma", "'a',", nil, true},
		{"missing comma between args", "'a' 'b'", nil, true},
		{"unquoted arg", "hello", nil, true},
		{"empty literal allowed", "''", []string{""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDynArgs("testFn", tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var se *apierrors.Structured
				if !errors.As(err, &se) {
					t.Fatalf("expected *apierrors.Structured, got %T", err)
				}
				if se.Category != apierrors.CategoryInput {
					t.Errorf("Category = %q, want %q", se.Category, apierrors.CategoryInput)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slicesEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestInterpolate_DynamicArgs_NestedVarSubstitution(t *testing.T) {
	seed := int64(42)
	reg := NewRegistry(&seed)
	// Test-only echo helper: returns the resolved first arg verbatim.
	reg.funcs["echo"] = func(_ *rand.Rand, args []string) (string, error) {
		if len(args) != 1 {
			return "", arityError("echo", 1, len(args))
		}
		return args[0], nil
	}

	s := NewScope(map[string]string{"user": "alice"})
	if err := s.Resolve(); err != nil {
		t.Fatal(err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	got, err := s.Interpolate(`{{$echo('{{user}}-static')}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "alice-static" {
		t.Errorf("got %q, want %q", got, "alice-static")
	}

	// Inner dynamic ref also resolves.
	got2, err := s.Interpolate(`{{$echo('{{user}}-{{$timestamp}}')}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got2, "alice-") {
		t.Errorf("got %q, expected alice-<digits>", got2)
	}
}

func TestInterpolate_BackwardCompatNoArgs(t *testing.T) {
	seed := int64(7)
	reg := NewRegistry(&seed)
	s := NewScope(nil)
	if err := s.Resolve(); err != nil {
		t.Fatal(err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	cases := []string{"{{$timestamp}}", "{{$uuid}}", "{{$randomInt}}", "{{$timestampMs}}"}
	for _, in := range cases {
		got, err := s.Interpolate(in)
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if got == "" || got == in {
			t.Errorf("%s: not resolved (%q)", in, got)
		}
	}
}

// TestInterpolate_EmptyParensEquivalence verifies that {{$fn()}} (empty parens) is
// fully equivalent to {{$fn}} (no parens) as documented in MANUAL.md §3.7.
// Both forms must resolve to the same value within a request (shared cache hit)
// and must not return an error.
func TestInterpolate_EmptyParensEquivalence(t *testing.T) {
	seed := int64(5)
	reg := NewRegistry(&seed)
	s := NewScope(nil)
	if err := s.Resolve(); err != nil {
		t.Fatal(err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	// {{$timestamp()}} must resolve to a non-empty, non-literal value.
	gotParens, err := s.Interpolate("{{$timestamp()}}")
	if err != nil {
		t.Fatalf("{{$timestamp()}} unexpected error: %v", err)
	}
	if gotParens == "" || gotParens == "{{$timestamp()}}" {
		t.Errorf("{{$timestamp()}} not resolved: got %q", gotParens)
	}

	// Within the same request, {{$timestamp}} must return the same memoized value.
	gotNoParens, err := s.Interpolate("{{$timestamp}}")
	if err != nil {
		t.Fatalf("{{$timestamp}} unexpected error: %v", err)
	}
	if gotNoParens != gotParens {
		t.Errorf("empty-parens and no-parens forms returned different values: %q vs %q — expected same memoized result within one request", gotParens, gotNoParens)
	}

	// {{$uuid()}} must also resolve to a non-empty value.
	gotUUID, err := s.Interpolate("{{$uuid()}}")
	if err != nil {
		t.Fatalf("{{$uuid()}} unexpected error: %v", err)
	}
	if gotUUID == "" || gotUUID == "{{$uuid()}}" {
		t.Errorf("{{$uuid()}} not resolved: got %q", gotUUID)
	}
}

func TestInterpolate_ArityMismatch(t *testing.T) {
	seed := int64(1)
	reg := NewRegistry(&seed)
	s := NewScope(nil)
	_ = s.Resolve()
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	_, err := s.Interpolate(`{{$timestamp('extra')}}`)
	if err == nil {
		t.Fatal("expected error for $timestamp('extra'), got nil")
	}
	if !strings.Contains(err.Error(), "expected 0 arguments, got 1") {
		t.Errorf("err = %v, want arity message", err)
	}
}

func TestInterpolate_ArityMismatch_NArgFunction(t *testing.T) {
	seed := int64(1)
	reg := NewRegistry(&seed)
	// echo2 expects exactly 2 arguments.
	reg.funcs["echo2"] = func(_ *rand.Rand, args []string) (string, error) {
		if len(args) != 2 {
			return "", arityError("echo2", 2, len(args))
		}
		return args[0] + args[1], nil
	}
	s := NewScope(nil)
	_ = s.Resolve()
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	_, err := s.Interpolate(`{{$echo2('a', 'b', 'c')}}`)
	if err == nil {
		t.Fatal("expected arity error for $echo2 called with 3 args, got nil")
	}
	if !strings.Contains(err.Error(), "expected 2 arguments, got 3") {
		t.Errorf("err = %v, want message containing \"expected 2 arguments, got 3\"", err)
	}
	var se *apierrors.Structured
	if !errors.As(err, &se) {
		t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
	}
	if se.Code != "DYNFN_ARITY" {
		t.Errorf("Code = %q, want DYNFN_ARITY", se.Code)
	}
}

func TestInterpolate_MalformedArgList(t *testing.T) {
	seed := int64(1)
	reg := NewRegistry(&seed)
	reg.funcs["echo"] = func(_ *rand.Rand, args []string) (string, error) {
		if len(args) != 1 {
			return "", arityError("echo", 1, len(args))
		}
		return args[0], nil
	}
	s := NewScope(nil)
	_ = s.Resolve()
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	_, err := s.Interpolate(`{{$echo('oops)}}`)
	if err == nil {
		t.Fatal("expected error for unterminated quote")
	}
	var se *apierrors.Structured
	if !errors.As(err, &se) {
		t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
	}
	if se.Category != apierrors.CategoryInput {
		t.Errorf("Category = %q, want input", se.Category)
	}
}

func TestInterpolate_PerRequestCache_DistinctArgs(t *testing.T) {
	seed := int64(1)
	reg := NewRegistry(&seed)
	calls := 0
	reg.funcs["echo"] = func(_ *rand.Rand, args []string) (string, error) {
		if len(args) != 1 {
			return "", arityError("echo", 1, len(args))
		}
		calls++
		return args[0] + "/" + strconv.Itoa(calls), nil
	}
	s := NewScope(nil)
	_ = s.Resolve()
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	got, err := s.Interpolate(`{{$echo('a')}}|{{$echo('b')}}|{{$echo('a')}}`)
	if err != nil {
		t.Fatal(err)
	}
	// a is cached → suffix /1; b is fresh → /2; second a hits cache → /1 again.
	want := "a/1|b/2|a/1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestInterpolate_DottedName_UnknownFunctionError proves that the regex
// resolves a dotted funcName (not as an unmatched literal) and that the
// registry receives the dotted name as the lookup key. Uses faker.unregistered
// as an example — a dotted name that is not registered in any M13 task.
// This is the integration counterpart to
// TestRegistry_DottedName_UnknownFunctionReturnsError.
func TestInterpolate_DottedName_UnknownFunctionError(t *testing.T) {
	seed := int64(1)
	reg := NewRegistry(&seed)
	s := NewScope(nil)
	if err := s.Resolve(); err != nil {
		t.Fatal(err)
	}
	s = s.WithDynamic(reg)
	s.BeginRequest()
	defer s.EndRequest()

	_, err := s.Interpolate("{{$faker.unregistered}}")
	if err == nil {
		t.Fatal("expected error for unregistered dotted function, got nil")
	}
	if !strings.Contains(err.Error(), `unknown dynamic function "faker.unregistered"`) {
		t.Errorf("err = %v, want message containing 'unknown dynamic function \"faker.unregistered\"'", err)
	}
}

// --- Step 2: WithRuntimeSensitive plumbing ---

func TestScope_WithRuntimeSensitive(t *testing.T) {
	base := NewScope(map[string]string{"x": "1"})
	if err := base.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	set := NewSensitiveSet()
	s := base.WithRuntimeSensitive(set)

	// Mutating through s registers on the same underlying set the caller
	// retains a handle to.
	s.runtimeSensitive.AddValue("super-secret")
	vals := set.Values()
	if len(vals) != 1 || vals[0] != "super-secret" {
		t.Errorf("set.Values() = %v, want [\"super-secret\"]", vals)
	}

	// Snapshot preserves the pointer (concurrent workers share the set).
	snap := s.Snapshot()
	if snap.runtimeSensitive != set {
		t.Errorf("Snapshot dropped runtimeSensitive pointer")
	}

	// WithOverrides propagates the pointer.
	child, err := s.WithOverrides(map[string]string{"y": "2"})
	if err != nil {
		t.Fatalf("WithOverrides: %v", err)
	}
	if child.runtimeSensitive != set {
		t.Errorf("WithOverrides dropped runtimeSensitive pointer")
	}

	// A scope with no runtime set is well-defined; runtimeSensitive is nil.
	raw := NewScope(map[string]string{"x": "1"})
	if raw.runtimeSensitive != nil {
		t.Errorf("unattached scope: runtimeSensitive should be nil")
	}
}
