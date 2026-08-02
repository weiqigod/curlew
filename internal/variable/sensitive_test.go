package variable

import (
	"sort"
	"testing"
)

func TestIsSensitiveName(t *testing.T) {
	tests := []struct {
		name      string
		varName   string
		sensitive bool
	}{
		{"password_exact_match", "password", true},
		{"PASSWORD_case_insensitive", "PASSWORD", true},
		{"db_password_substring", "db_password", true},
		{"myPassword_camelCase", "myPassword", true},
		{"token_exact", "token", true},
		{"access_token_substring", "access_token", true},
		{"TOKEN_uppercase", "TOKEN", true},
		{"secret_exact", "secret", true},
		{"my_secret_key_substring", "my_secret_key", true},
		{"api_key_exact", "api_key", true},
		{"API_KEY_uppercase", "API_KEY", true},
		{"apikey_no_underscore", "apikey", true},
		{"authorization_exact", "authorization", true},
		{"AUTHORIZATION_uppercase", "AUTHORIZATION", true},
		{"credential_exact", "credential", true},
		{"base_url_not_sensitive", "base_url", false},
		{"count_not_sensitive", "count", false},
		{"name_not_sensitive", "name", false},
		{"empty_string_not_sensitive", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSensitiveName(tt.varName)
			if got != tt.sensitive {
				t.Errorf("IsSensitiveName(%q) = %v, want %v", tt.varName, got, tt.sensitive)
			}
		})
	}
}

func TestIsSensitiveHeaderName(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{"authorization_header", "authorization", true},
		{"Authorization_mixed_case", "Authorization", true},
		{"x_api_key_header", "x-api-key", true},
		{"cookie_header", "cookie", true},
		{"content_type_not_sensitive", "content-type", false},
		{"accept_not_sensitive", "accept", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSensitiveHeaderName(tt.header)
			if got != tt.want {
				t.Errorf("IsSensitiveHeaderName(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestSensitiveSet(t *testing.T) {
	t.Run("new_set_empty", func(t *testing.T) {
		s := NewSensitiveSet()
		if s.IsSensitive("anything") {
			t.Error("new set should be empty")
		}
	})

	t.Run("add_and_check", func(t *testing.T) {
		s := NewSensitiveSet()
		s.Add("api_key")
		if !s.IsSensitive("api_key") {
			t.Error("api_key should be sensitive after Add")
		}
	})

	t.Run("not_added_returns_false", func(t *testing.T) {
		s := NewSensitiveSet()
		s.Add("api_key")
		if s.IsSensitive("other") {
			t.Error("other should not be sensitive")
		}
	})

	t.Run("nil_set_returns_false", func(t *testing.T) {
		var s *SensitiveSet
		if s.IsSensitive("anything") {
			t.Error("nil set should return false")
		}
	})

	t.Run("merge_combines_sets", func(t *testing.T) {
		a := NewSensitiveSet()
		a.Add("key1")
		b := NewSensitiveSet()
		b.Add("key2")
		a.Merge(b)
		if !a.IsSensitive("key1") || !a.IsSensitive("key2") {
			t.Error("merged set should contain both keys")
		}
	})

	t.Run("merge_nil_is_noop", func(t *testing.T) {
		s := NewSensitiveSet()
		s.Add("key1")
		s.Merge(nil)
		if !s.IsSensitive("key1") {
			t.Error("merge nil should not affect existing keys")
		}
	})
}

func TestRedactValue(t *testing.T) {
	t.Run("sensitive_name_redacted", func(t *testing.T) {
		s := NewSensitiveSet()
		s.Add("api_key")
		got := RedactValue("api_key", "secret", s, false)
		if got != Redacted {
			t.Errorf("got %q, want %q", got, Redacted)
		}
	})

	t.Run("sensitive_name_with_allow_shows_value", func(t *testing.T) {
		s := NewSensitiveSet()
		s.Add("api_key")
		got := RedactValue("api_key", "secret", s, true)
		if got != "secret" {
			t.Errorf("got %q, want %q", got, "secret")
		}
	})

	t.Run("non_sensitive_name_passthrough", func(t *testing.T) {
		s := NewSensitiveSet()
		got := RedactValue("base_url", "https://example.com", s, false)
		if got != "https://example.com" {
			t.Errorf("got %q, want %q", got, "https://example.com")
		}
	})

	t.Run("nil_set_passthrough", func(t *testing.T) {
		got := RedactValue("api_key", "secret", nil, false)
		if got != "secret" {
			t.Errorf("got %q, want %q", got, "secret")
		}
	})
}

func TestRedactHeaders(t *testing.T) {
	t.Run("authorization_redacted", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer token123", "Content-Type": "application/json"}
		s := NewSensitiveSet()
		got := RedactHeaders(headers, s, false)
		if got["Authorization"] != Redacted {
			t.Errorf("Authorization = %q, want %q", got["Authorization"], Redacted)
		}
		if got["Content-Type"] != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got["Content-Type"], "application/json")
		}
	})

	t.Run("content_type_not_redacted", func(t *testing.T) {
		headers := map[string]string{"Content-Type": "application/json"}
		s := NewSensitiveSet()
		got := RedactHeaders(headers, s, false)
		if got["Content-Type"] != "application/json" {
			t.Errorf("Content-Type should not be redacted, got %q", got["Content-Type"])
		}
	})

	t.Run("custom_sensitive_var_in_set_redacted", func(t *testing.T) {
		headers := map[string]string{"X-Custom-Key": "myvalue"}
		s := NewSensitiveSet()
		s.Add("X-Custom-Key")
		got := RedactHeaders(headers, s, false)
		if got["X-Custom-Key"] != Redacted {
			t.Errorf("X-Custom-Key = %q, want %q", got["X-Custom-Key"], Redacted)
		}
	})

	t.Run("allow_sensitive_shows_all", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer token123"}
		s := NewSensitiveSet()
		got := RedactHeaders(headers, s, true)
		if got["Authorization"] != "Bearer token123" {
			t.Errorf("Authorization = %q, want %q", got["Authorization"], "Bearer token123")
		}
	})

	t.Run("nil_headers_returns_nil", func(t *testing.T) {
		s := NewSensitiveSet()
		got := RedactHeaders(nil, s, false)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("empty_headers_returns_empty", func(t *testing.T) {
		headers := map[string]string{}
		s := NewSensitiveSet()
		got := RedactHeaders(headers, s, false)
		if got == nil || len(got) != 0 {
			t.Errorf("expected empty map, got %v", got)
		}
	})
}

func TestSensitiveSet_AddHeuristicNames(t *testing.T) {
	t.Run("sensitive_names_added", func(t *testing.T) {
		s := NewSensitiveSet()
		s.AddHeuristicNames(map[string]string{"password": "secret", "base_url": "https://example.com"})
		if !s.IsSensitive("password") {
			t.Error("password should be sensitive after AddHeuristicNames")
		}
		if s.IsSensitive("base_url") {
			t.Error("base_url should not be sensitive")
		}
	})

	t.Run("nil_vars_is_noop", func(t *testing.T) {
		s := NewSensitiveSet()
		s.AddHeuristicNames(nil)
		if s.IsSensitive("anything") {
			t.Error("nil vars should not add any names")
		}
	})

	t.Run("empty_vars_is_noop", func(t *testing.T) {
		s := NewSensitiveSet()
		s.AddHeuristicNames(map[string]string{})
		if s.IsSensitive("anything") {
			t.Error("empty vars should not add any names")
		}
	})

	t.Run("multiple_sensitive_names", func(t *testing.T) {
		s := NewSensitiveSet()
		s.AddHeuristicNames(map[string]string{"api_key": "k", "token": "t", "name": "n"})
		if !s.IsSensitive("api_key") || !s.IsSensitive("token") {
			t.Error("api_key and token should both be sensitive")
		}
		if s.IsSensitive("name") {
			t.Error("name should not be sensitive")
		}
	})
}

func TestSensitiveSet_Names(t *testing.T) {
	s := NewSensitiveSet()
	s.Add("b")
	s.Add("a")
	names := s.Names()
	sort.Strings(names)
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Errorf("Names() = %v, want [a b]", names)
	}
}

func TestSensitiveSet_ValueTracking(t *testing.T) {
	tests := []struct {
		name string
		add  []string
		want []string // expected Values() — longest first, then lex
	}{
		{"empty_set_returns_nil", nil, nil},
		{"single_value", []string{"hunter2"}, []string{"hunter2"}},
		{"deduplicates", []string{"a", "a", "b"}, []string{"a", "b"}},
		{"longest_first", []string{"ab", "abcd", "a"}, []string{"abcd", "ab", "a"}},
		{"empty_value_ignored", []string{""}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSensitiveSet()
			for _, v := range tt.add {
				s.AddValue(v)
			}
			got := s.Values()
			if len(got) != len(tt.want) {
				t.Errorf("Values() len = %d, want %d; got %v, want %v", len(got), len(tt.want), got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Values()[%d] = %q, want %q; full: %v", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

func TestSensitiveSet_MergeValues(t *testing.T) {
	a := NewSensitiveSet()
	a.AddValue("v1")
	b := NewSensitiveSet()
	b.AddValue("v2")
	a.Merge(b)
	vals := a.Values()
	found := map[string]bool{}
	for _, v := range vals {
		found[v] = true
	}
	if !found["v1"] || !found["v2"] {
		t.Errorf("Merge should combine values; got %v", vals)
	}
}

func TestSensitiveSet_NilSafe(t *testing.T) {
	var s *SensitiveSet
	s.AddValue("x") // must not panic
	if got := s.Values(); got != nil {
		t.Errorf("nil Values() = %v, want nil", got)
	}
	if got := s.Names(); got != nil {
		t.Errorf("nil Names() = %v, want nil", got)
	}
}

func TestSensitiveSet_ZeroValue(t *testing.T) {
	// A zero-value SensitiveSet (not via NewSensitiveSet) must not panic on
	// AddValue or Merge — the defensive nil guards in those methods protect
	// against this usage pattern.
	t.Run("AddValue_on_zero_value", func(t *testing.T) {
		var s SensitiveSet
		s.AddValue("hunter2") // must not panic
		// Values() should return the added value via the nil guard initialisation.
		vals := s.Values()
		if len(vals) != 1 || vals[0] != "hunter2" {
			t.Errorf("zero-value AddValue: Values() = %v, want [hunter2]", vals)
		}
	})

	t.Run("Merge_on_zero_value", func(t *testing.T) {
		var s SensitiveSet
		other := NewSensitiveSet()
		other.AddValue("v1")
		s.Merge(other) // must not panic
		vals := s.Values()
		if len(vals) != 1 || vals[0] != "v1" {
			t.Errorf("zero-value Merge: Values() = %v, want [v1]", vals)
		}
	})
}
