package variable

import (
	"net/http"
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

// RedactText is for output that is already prose — an assertion's expected or
// actual value, a URL, a header line. RedactBody would try to reinterpret a
// JSON-shaped string and re-encode it; here the string must come back as
// written apart from the secrets.
func TestRedactText(t *testing.T) {
	s := NewSensitiveSet()
	s.AddValue("sk_live_abc123")

	got := RedactText("got Bearer sk_live_abc123 from the server", s, false)
	want := "got Bearer " + Redacted + " from the server"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	if got := RedactText(`{"a":1}`, s, false); got != `{"a":1}` {
		t.Errorf("JSON-shaped text should be returned verbatim, got %q", got)
	}
	if got := RedactText("sk_live_abc123", s, true); got != "sk_live_abc123" {
		t.Errorf("--allow-sensitive should not redact, got %q", got)
	}
	if got := RedactText("sk_live_abc123", nil, false); got != "sk_live_abc123" {
		t.Errorf("nil set should not redact, got %q", got)
	}
}

func TestRedactHTTPHeaders(t *testing.T) {
	s := NewSensitiveSet()
	s.AddValue("tok-abc")

	in := http.Header{
		"Set-Cookie":   []string{"session=deadbeef; HttpOnly"},
		"Content-Type": []string{"application/json"},
		"X-Trace":      []string{"prefix-tok-abc-suffix"},
	}
	out := RedactHTTPHeaders(in, s, false)

	if got := out.Get("Set-Cookie"); got != Redacted {
		t.Errorf("Set-Cookie = %q, want %q — an inherently sensitive header is redacted whole", got, Redacted)
	}
	if got := out.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want it untouched", got)
	}
	if got := out.Get("X-Trace"); got != "prefix-"+Redacted+"-suffix" {
		t.Errorf("X-Trace = %q, want the registered value replaced in place", got)
	}
	if got := in.Get("Set-Cookie"); got != "session=deadbeef; HttpOnly" {
		t.Errorf("input header was mutated: %q", got)
	}

	if out := RedactHTTPHeaders(in, s, true); out.Get("Set-Cookie") != "session=deadbeef; HttpOnly" {
		t.Error("--allow-sensitive should not redact")
	}
	if RedactHTTPHeaders(nil, s, false) != nil {
		t.Error("nil headers should return nil")
	}
}
