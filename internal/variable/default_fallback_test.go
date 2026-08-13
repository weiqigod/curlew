package variable

import "testing"

// `{{name|default:value}}` substitutes the default when name is undefined.
//
// The syntax was documented in three places — §6.1 as an interpolation form,
// §9.x and §11.5 as the thing that lets a dependent run when its producer
// succeeded but the JSONPath missed — and half-built: internal/parallel's
// scanner strips the pipe to find the dependency name, and the runner uses it
// to decide skip-versus-run. Nothing ever substituted the value.
//
// So the dependent *ran*, as §11.5's second row promises, and sent
//
//	http://host/{{user_id|default:FALLBACK}}
//
// to the server, placeholder and all. That is worse than the exit 5 an
// unresolvable reference would have produced: a failure that looks like a
// success and reaches the network.
//
// Found by executing §11.5's failure-and-skip table.

func TestInterpolate_defaultFallback(t *testing.T) {
	scope := NewScope(map[string]string{"defined": "real"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"undefined takes the default", "{{missing|default:123}}", "123"},
		{"defined ignores the default", "{{defined|default:123}}", "real"},
		{"default in a URL path", "http://h/{{missing|default:FALLBACK}}/x", "http://h/FALLBACK/x"},
		{"empty default is allowed", "{{missing|default:}}", ""},
		{"default may contain spaces", "{{missing|default:two words}}", "two words"},
		{"two references in one string", "{{defined|default:a}}-{{missing|default:b}}", "real-b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scope.Interpolate(tt.input)
			if err != nil {
				t.Fatalf("Interpolate(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Interpolate(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// A reference with no default is unchanged in its behaviour: undefined is still
// an error, because a silent empty string is how a bad URL reaches a server.
func TestInterpolate_undefinedWithoutDefaultStillErrors(t *testing.T) {
	scope := NewScope(map[string]string{})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := scope.Interpolate("{{missing}}"); err == nil {
		t.Error("an undefined reference with no default no longer errors")
	}
}
