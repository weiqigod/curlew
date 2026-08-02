package variable

import (
	"encoding/json"
	"math/rand/v2"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// phoneShapePatterns maps each Latin-script locale to a regex its phoneFormat
// output must match, derived from the SPEC:973-987 phone-format column.
var phoneShapePatterns = map[string]*regexp.Regexp{
	"en-GB": regexp.MustCompile(`^\+44 \d{2} \d{4} \d{4}$`),
	"fr-FR": regexp.MustCompile(`^\+33 \d \d{2} \d{2} \d{2} \d{2}$`),
	"es-ES": regexp.MustCompile(`^\+34 \d{2} \d{3} \d{2} \d{2}$`),
	"it-IT": regexp.MustCompile(`^\+39 \d{2} \d{4} \d{4}$`),
	"pt-BR": regexp.MustCompile(`^\+55 \d{2} \d{5}-\d{4}$`),
	"nl-NL": regexp.MustCompile(`^\+31 \d{2} \d{3} \d{4}$`),
	"pl-PL": regexp.MustCompile(`^\+48 \d{2} \d{3} \d{2} \d{2}$`),
	"sv-SE": regexp.MustCompile(`^\+46 \d \d{3} \d{2} \d{2}$`),
	"tr-TR": regexp.MustCompile(`^\+90 \d{3} \d{3} \d{2} \d{2}$`),
}

// TestLocalePools_Behaviour6_EagerRegistrationEquivalent verifies Behaviour 6 of
// task M20-002: "Given any of the nine Latin-script locale codes, when its data
// table is first accessed, then it is initialised … per SPEC:1110."
//
// SPEC:1110 mentions lazy init behind sync.Once. The implementation uses an
// eager package-level map literal instead, which is a conscious architectural
// decision (plan §KAD-1): the pools are tiny in-source Go slices (~few hundred
// bytes each), so the startup-cost concern that motivates lazy loading does not
// apply. The observable contract — pools are accessible on any request — is
// identical for both strategies; the difference is only in *when* initialisation
// occurs (package init vs first call).
//
// This test verifies that all nine pools are registered and fully populated
// before any request is handled, which satisfies the behavioural intent of
// SPEC:1110 (no missing pools, no panics on first access).
func TestLocalePools_Behaviour6_EagerRegistrationEquivalent(t *testing.T) {
	locales := []string{"en-GB", "fr-FR", "es-ES", "it-IT", "pt-BR", "nl-NL", "pl-PL", "sv-SE", "tr-TR"}
	for _, code := range locales {
		t.Run(code, func(t *testing.T) {
			// Pool must be present at package init time (eager registration).
			pool, ok := localePools[code]
			if !ok {
				t.Fatalf("locale %q not registered in localePools at startup", code)
			}
			// Pool must be fully populated — not a nil or empty placeholder.
			if pool == nil {
				t.Fatalf("%s: pool is nil", code)
			}
			if len(pool.firstNames) == 0 {
				t.Errorf("%s: firstNames pool empty at startup", code)
			}
			if len(pool.lastNames) == 0 {
				t.Errorf("%s: lastNames pool empty at startup", code)
			}
			if len(pool.cities) == 0 {
				t.Errorf("%s: cities pool empty at startup", code)
			}
			if pool.phoneFormat == nil {
				t.Errorf("%s: phoneFormat is nil at startup", code)
			}
		})
	}
}

// nonLatinPhonePatterns maps each non-Latin locale to a regex its phoneFormat
// output must match, derived from the SPEC:974-986 phone-format column (KAD-4).
var nonLatinPhonePatterns = map[string]*regexp.Regexp{
	"ja-JP": regexp.MustCompile(`^\+81 \d-\d{4}-\d{4}$`),
	"zh-CN": regexp.MustCompile(`^\+86 \d{2} \d{4} \d{4}$`),
	"ko-KR": regexp.MustCompile(`^\+82 \d-\d{4}-\d{4}$`),
	"ru-RU": regexp.MustCompile(`^\+7 \d{3} \d{3}-\d{2}-\d{2}$`),
}

// TestLocalePools_NonLatinScript_Shape asserts the universal pool-shape contract
// for all four non-Latin-script locales: non-empty pools, no empty/whitespace
// entries, valid UTF-8, familyNameFirst flag set correctly, and phone output
// matching the spec-documented format across 200 seeds.
func TestLocalePools_NonLatinScript_Shape(t *testing.T) {
	locales := []string{"ja-JP", "zh-CN", "ko-KR", "ru-RU"}
	familyNameFirstLocales := map[string]bool{"ja-JP": true, "zh-CN": true, "ko-KR": true, "ru-RU": false}
	for _, code := range locales {
		t.Run(code, func(t *testing.T) {
			pool, ok := localePools[code]
			if !ok {
				t.Fatalf("locale %q not registered in localePools", code)
			}
			// familyNameFirst flag
			wantFNF := familyNameFirstLocales[code]
			if pool.familyNameFirst != wantFNF {
				t.Errorf("%s familyNameFirst = %v, want %v", code, pool.familyNameFirst, wantFNF)
			}
			// Non-empty pools
			if len(pool.firstNames) == 0 || len(pool.lastNames) == 0 || len(pool.cities) == 0 {
				t.Fatalf("%s: empty firstNames/lastNames/cities pool", code)
			}
			// No empty/whitespace entries, valid UTF-8
			for _, group := range [][]string{pool.firstNames, pool.lastNames, pool.cities} {
				for _, e := range group {
					if strings.TrimSpace(e) == "" {
						t.Errorf("%s: empty/whitespace entry %q", code, e)
					}
					if !utf8.ValidString(e) {
						t.Errorf("%s: invalid UTF-8 entry %q", code, e)
					}
				}
			}
			// Phone format present and shape-correct across many draws
			if pool.phoneFormat == nil {
				t.Fatalf("%s: phoneFormat is nil", code)
			}
			pat := nonLatinPhonePatterns[code]
			for seed := int64(0); seed < 200; seed++ {
				rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed>>32^0xdeadbeef)))
				got := pool.phoneFormat(rng)
				if !pat.MatchString(got) {
					t.Errorf("%s seed %d: phone %q does not match %s", code, seed, got, pat)
				}
			}
		})
	}
}

// TestLocalePools_NonLatinScript_Class verifies each name and city entry in the
// four non-Latin pools contains at least one rune in the locale's expected
// Unicode script block, proving the data is native-script (not romanised/en-US).
func TestLocalePools_NonLatinScript_Class(t *testing.T) {
	inRange := func(s string, lo, hi rune) bool {
		for _, r := range s {
			if r >= lo && r <= hi {
				return true
			}
		}
		return false
	}
	cases := []struct {
		locale string
		check  func(string) bool
		descr  string
	}{
		// CJK Unified Ideographs U+4E00–U+9FFF (plus Hiragana/Katakana for ja-JP).
		{"ja-JP", func(s string) bool {
			return inRange(s, 0x4E00, 0x9FFF) || inRange(s, 0x3040, 0x30FF)
		}, "kanji/kana"},
		{"zh-CN", func(s string) bool { return inRange(s, 0x4E00, 0x9FFF) }, "Han"},
		// Hangul Syllables U+AC00–U+D7A3.
		{"ko-KR", func(s string) bool { return inRange(s, 0xAC00, 0xD7A3) }, "Hangul"},
		// Cyrillic U+0400–U+04FF.
		{"ru-RU", func(s string) bool { return inRange(s, 0x0400, 0x04FF) }, "Cyrillic"},
	}
	for _, c := range cases {
		t.Run(c.locale, func(t *testing.T) {
			pool := localePools[c.locale]
			if pool == nil {
				t.Fatalf("locale %q not found in localePools", c.locale)
			}
			for _, group := range [][]string{pool.firstNames, pool.lastNames, pool.cities} {
				for _, e := range group {
					if !c.check(e) {
						t.Errorf("%s: %q has no %s rune", c.locale, e, c.descr)
					}
				}
			}
		})
	}
}

// TestLocalePools_NonLatinScript_JSONRoundTrip verifies every pool entry is
// valid UTF-8 and survives JSON marshal+unmarshal unchanged (DoD line 5).
func TestLocalePools_NonLatinScript_JSONRoundTrip(t *testing.T) {
	for _, code := range []string{"ja-JP", "zh-CN", "ko-KR", "ru-RU"} {
		t.Run(code, func(t *testing.T) {
			pool := localePools[code]
			if pool == nil {
				t.Fatalf("locale %q not found in localePools", code)
			}
			for _, group := range [][]string{pool.firstNames, pool.lastNames, pool.cities} {
				for _, e := range group {
					if !utf8.ValidString(e) {
						t.Errorf("%s: invalid UTF-8 %q", code, e)
					}
					b, err := json.Marshal(e)
					if err != nil {
						t.Fatalf("%s: marshal %q: %v", code, e, err)
					}
					var back string
					if err := json.Unmarshal(b, &back); err != nil {
						t.Fatalf("%s: unmarshal %q: %v", code, e, err)
					}
					if back != e {
						t.Errorf("%s: round-trip changed %q -> %q", code, e, back)
					}
				}
			}
		})
	}
}

// TestLocalePools_LatinScript_Shape asserts the universal pool-shape contract for
// all nine Latin-script European locales: non-empty pools, no empty/whitespace
// entries, valid UTF-8 throughout, and phone output matching the spec-documented
// format across 200 seeds.
func TestLocalePools_LatinScript_Shape(t *testing.T) {
	locales := []string{"en-GB", "fr-FR", "es-ES", "it-IT", "pt-BR", "nl-NL", "pl-PL", "sv-SE", "tr-TR"}
	for _, code := range locales {
		t.Run(code, func(t *testing.T) {
			pool, ok := localePools[code]
			if !ok {
				t.Fatalf("locale %q not registered in localePools", code)
			}
			// Non-empty pools
			if len(pool.firstNames) == 0 || len(pool.lastNames) == 0 || len(pool.cities) == 0 {
				t.Fatalf("%s: empty firstNames/lastNames/cities pool", code)
			}
			// No empty/whitespace entries, valid UTF-8
			for _, group := range [][]string{pool.firstNames, pool.lastNames, pool.cities} {
				for _, e := range group {
					if strings.TrimSpace(e) == "" {
						t.Errorf("%s: empty/whitespace entry %q", code, e)
					}
					if !utf8.ValidString(e) {
						t.Errorf("%s: invalid UTF-8 entry %q", code, e)
					}
				}
			}
			// Phone format present and shape-correct across many draws
			if pool.phoneFormat == nil {
				t.Fatalf("%s: phoneFormat is nil", code)
			}
			pat := phoneShapePatterns[code]
			for seed := int64(0); seed < 200; seed++ {
				rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed>>32^0xdeadbeef)))
				got := pool.phoneFormat(rng)
				if !pat.MatchString(got) {
					t.Errorf("%s: phone %q does not match %s", code, got, pat)
				}
			}
		})
	}
}
