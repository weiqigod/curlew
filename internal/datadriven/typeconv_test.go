package datadriven

import (
	"testing"
)

func TestConvertValue(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		filter  string
		want    interface{}
		wantErr bool
	}{
		{"int valid", "42", "int", int64(42), false},
		{"int negative", "-5", "int", int64(-5), false},
		{"int invalid", "abc", "int", nil, true},
		{"int float string", "3.14", "int", nil, true},
		{"float valid", "3.14", "float", 3.14, false},
		{"float integer", "42", "float", 42.0, false},
		{"float negative", "-1.5", "float", -1.5, false},
		{"float invalid", "abc", "float", nil, true},
		{"bool true", "true", "bool", true, false},
		{"bool false", "false", "bool", false, false},
		{"bool yes", "yes", "bool", true, false},
		{"bool no", "no", "bool", false, false},
		{"bool 1", "1", "bool", true, false},
		{"bool 0", "0", "bool", false, false},
		{"bool True", "True", "bool", true, false},
		{"bool FALSE", "FALSE", "bool", false, false},
		{"bool invalid", "maybe", "bool", nil, true},
		{"unknown filter", "42", "date", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConvertValue(tt.value, tt.filter)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ConvertValue(%q, %q) = %v (%T), want %v (%T)", tt.value, tt.filter, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestTryNumeric(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want float64
		ok   bool
	}{
		{"integer", "42", 42.0, true},
		{"negative", "-5", -5.0, true},
		{"float", "3.14", 3.14, true},
		{"not numeric", "abc", 0, false},
		{"empty", "", 0, false},
		{"whitespace", "  42  ", 42.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := TryNumeric(tt.s)
			if ok != tt.ok {
				t.Errorf("TryNumeric(%q) ok = %v, want %v", tt.s, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("TryNumeric(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
