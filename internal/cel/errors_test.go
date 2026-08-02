package cel

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func Test_truncateSource(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"under_limit", "abc", "abc"},
		{"exact_limit", strings.Repeat("a", 200), strings.Repeat("a", 200)},
		{"over_limit", strings.Repeat("a", 201), strings.Repeat("a", 200) + "…"},
		{"multibyte_under_limit", "héllo", "héllo"},
		{"multibyte_over_limit", strings.Repeat("é", 250), strings.Repeat("é", 200) + "…"},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateSource(tc.in)
			if got != tc.want {
				t.Fatalf("truncateSource: want %d runes, got %d runes",
					utf8.RuneCountInString(tc.want),
					utf8.RuneCountInString(got))
			}
		})
	}
}
