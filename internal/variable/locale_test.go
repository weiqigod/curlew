package variable

import (
	"errors"
	"testing"
)

func TestNormalizeLocale(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"already canonical", "de-DE", "de-DE"},
		{"lowercase region", "de-de", "de-DE"},
		{"underscore separator", "de_DE", "de-DE"},
		{"language only lowercase", "en", "en"},
		{"language only uppercase", "EN", "en"},
		{"whitespace trimmed", "  fr-FR  ", "fr-FR"},
		{"lowercase language uppercase region", "fr-fr", "fr-FR"},
		{"empty string", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeLocale(tc.in)
			if got != tc.want {
				t.Errorf("normalizeLocale(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFallbackChain(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"en-GB falls through en to en-US", "en-GB", []string{"en-GB", "en", "en-US"}},
		{"de-DE falls through de to en-US", "de-DE", []string{"de-DE", "de", "en-US"}},
		{"en-US always terminates at en-US", "en-US", []string{"en-US", "en", "en-US"}},
		{"fr-FR falls through fr to en-US", "fr-FR", []string{"fr-FR", "fr", "en-US"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fallbackChain(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("fallbackChain(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i, v := range got {
				if v != tc.want[i] {
					t.Errorf("fallbackChain(%q)[%d] = %q, want %q", tc.in, i, v, tc.want[i])
				}
			}
		})
	}
}

func TestResolveLocaleData(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		wantUsed     string
		wantFellBack bool
		wantErr      bool
	}{
		{"en-US default no fallback", "en-US", "en-US", false, false},
		{"de-DE shipped no fallback", "de-DE", "de-DE", false, false},
		{"en-GB now ships its own pool", "en-GB", "en-GB", false, false},
		{"fr-FR now ships its own pool", "fr-FR", "fr-FR", false, false},
		{"sv-SE now ships its own pool", "sv-SE", "sv-SE", false, false},
		{"tr-TR now ships its own pool", "tr-TR", "tr-TR", false, false},
		{"ja-JP now ships its own pool", "ja-JP", "ja-JP", false, false},
		{"zh-CN now ships its own pool", "zh-CN", "zh-CN", false, false},
		{"ko-KR now ships its own pool", "ko-KR", "ko-KR", false, false},
		{"ru-RU now ships its own pool", "ru-RU", "ru-RU", false, false},
		{"unknown code errors", "xx-YY", "", false, true},
		{"empty resolves to en-US default", "", "en-US", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, used, fellBack, err := resolveLocaleData(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, ErrLocaleUnknown) {
					t.Errorf("expected ErrLocaleUnknown in error chain, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if used != tc.wantUsed {
				t.Errorf("used = %q, want %q", used, tc.wantUsed)
			}
			if fellBack != tc.wantFellBack {
				t.Errorf("fellBack = %v, want %v", fellBack, tc.wantFellBack)
			}
			if data == nil {
				t.Error("expected non-nil localeData")
			}
		})
	}
}
