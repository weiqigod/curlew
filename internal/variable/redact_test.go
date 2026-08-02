package variable

import (
	"reflect"
	"testing"
)

func TestRedactBody(t *testing.T) {
	s := NewSensitiveSet()
	s.AddValue("hunter2")
	s.AddValue("sk_live_abc")

	tests := []struct {
		name  string
		body  any
		allow bool
		want  any
	}{
		{"nil_body_returned", nil, false, nil},
		{"allow_sensitive_passthrough", "token=hunter2", true, "token=hunter2"},
		{"plain_text_substring", "token=hunter2", false, "token=" + Redacted},
		{"plain_text_multiple_values", "a=hunter2&b=sk_live_abc", false, "a=" + Redacted + "&b=" + Redacted},
		{"plain_text_partial_not_replaced", "hunter", false, "hunter"},
		{
			"json_string_object",
			`{"token":"hunter2","user":"alice"}`,
			false,
			`{"token":"` + Redacted + `","user":"alice"}`,
		},
		{
			"json_string_array",
			`["hunter2","alice"]`,
			false,
			`["` + Redacted + `","alice"]`,
		},
		{
			"json_string_nested",
			`{"outer":{"inner":"hunter2"}}`,
			false,
			`{"outer":{"inner":"` + Redacted + `"}}`,
		},
		{
			"json_bytes_object",
			[]byte(`{"token":"hunter2"}`),
			false,
			[]byte(`{"token":"` + Redacted + `"}`),
		},
		{
			"map_string_string",
			map[string]string{"k": "hunter2"},
			false,
			map[string]string{"k": Redacted},
		},
		{
			"map_string_any",
			map[string]any{"k": "hunter2", "n": 3.0},
			false,
			map[string]any{"k": Redacted, "n": 3.0},
		},
		{
			"slice_any",
			[]any{"hunter2", "alice"},
			false,
			[]any{Redacted, "alice"},
		},
		{
			"non_json_bytes_substring",
			[]byte("token=hunter2"),
			false,
			[]byte("token=" + Redacted),
		},
		{"empty_string", "", false, ""},
		{"empty_bytes", []byte{}, false, []byte{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactBody(tt.body, s, tt.allow)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RedactBody() = %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestRedactBody_EmptySet(t *testing.T) {
	s := NewSensitiveSet()
	got := RedactBody("hunter2", s, false)
	if got != "hunter2" {
		t.Errorf("empty set should not redact; got %v", got)
	}
}

func TestRedactBody_NilSet(t *testing.T) {
	got := RedactBody("hunter2", nil, false)
	if got != "hunter2" {
		t.Errorf("nil set should not redact; got %v", got)
	}
}

func TestRedactBody_LongestValueFirst(t *testing.T) {
	// Registering "ab" and "abcd" — "abcd" must be redacted as a whole,
	// not "ab" first leaving "[REDACTED]cd".
	s := NewSensitiveSet()
	s.AddValue("ab")
	s.AddValue("abcd")
	got := RedactBody("xabcdy", s, false)
	want := "x" + Redacted + "y"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
