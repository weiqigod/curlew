package variable

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"

	apierrors "github.com/weiqigod/curlew/internal/errors"
)

// localeData holds the per-locale generation pools. Pools that a given locale
// has not yet shipped are nil; the resolver falls back so nil is never read.
type localeData struct {
	code        string
	firstNames  []string
	lastNames   []string
	cities      []string
	phoneFormat func(rng *rand.Rand) string // nil = use en-US default
	// familyNameFirst renders $faker.fullName as "last first" (family name
	// first). Default false = given-name-first ("first last"), correct for
	// en-US and every Latin/Cyrillic locale. Set true for ja-JP/zh-CN/ko-KR.
	// Only the concatenation order changes; both pool draws still occur in
	// firstName-then-lastName order, preserving seed-position-invariance.
	familyNameFirst bool
}

// supportedLocales is the canonical 15-locale table from SPECIFICATION.md:973-987.
// Membership here gates ERR_LOCALE_UNKNOWN; a code can be supported (valid) yet
// have no pool yet (falls back). Order is the spec table order for stable hints.
var supportedLocales = []string{
	"en-US", "en-GB", "de-DE", "fr-FR", "es-ES", "it-IT", "pt-BR",
	"ja-JP", "zh-CN", "ko-KR", "nl-NL", "pl-PL", "ru-RU", "sv-SE", "tr-TR",
}

// supportedLocaleSet provides fast membership lookup.
var supportedLocaleSet = func() map[string]bool {
	m := make(map[string]bool, len(supportedLocales))
	for _, code := range supportedLocales {
		m[code] = true
	}
	return m
}()

// localePools maps a locale code to its shipped data. en-US and de-DE ship in
// M20-001; the nine Latin-script European locales ship in M20-002;
// the four CJK and Cyrillic locales ship in M20-003.
// Keyed by canonical code; package-level, immutable.
var localePools = map[string]*localeData{
	"en-US": enUSLocale(),
	"de-DE": deDELocale(),
	"en-GB": enGBLocale(),
	"fr-FR": frFRLocale(),
	"es-ES": esESLocale(),
	"it-IT": itITLocale(),
	"pt-BR": ptBRLocale(),
	"nl-NL": nlNLLocale(),
	"pl-PL": plPLLocale(),
	"sv-SE": svSELocale(),
	"tr-TR": trTRLocale(),
	"ja-JP": jaJPLocale(),
	"zh-CN": zhCNLocale(),
	"ko-KR": koKRLocale(),
	"ru-RU": ruRULocale(),
}

// ErrLocaleUnknown is the sentinel for an unsupported --locale value.
var ErrLocaleUnknown = errors.New("locale unknown")

// enUSLocale returns the en-US localeData, reusing the existing firstNames/lastNames/cities
// file-scope vars so en-US output is byte-identical to the pre-locale baseline.
func enUSLocale() *localeData {
	return &localeData{
		code:       "en-US",
		firstNames: firstNames,
		lastNames:  lastNames,
		cities:     cities,
	}
}

// deDELocale returns the de-DE localeData with German name/city/phone pools.
// This is the proof locale shipped in M20-001.
func deDELocale() *localeData {
	return &localeData{
		code: "de-DE",
		firstNames: []string{
			"Johann", "Sophie", "Lukas", "Anna", "Felix", "Marie", "Paul",
			"Emma", "Maximilian", "Lena", "Jonas", "Mia", "Elias", "Laura",
			"Noah", "Hannah", "Leon", "Lea", "Finn", "Clara", "Ben", "Johanna",
			"David", "Katharina", "Moritz", "Sarah", "Julian", "Julia", "Tim", "Sabrina",
		},
		lastNames: []string{
			"Schmidt", "Müller", "Schneider", "Fischer", "Weber", "Meyer", "Wagner",
			"Becker", "Schulz", "Hoffmann", "Schäfer", "Koch", "Bauer", "Richter",
			"Klein", "Wolf", "Schröder", "Neumann", "Schwarz", "Zimmermann",
			"Braun", "Krüger", "Hofmann", "Hartmann", "Lange", "Schmitt", "Werner",
			"Schmitz", "Krause", "Meier",
		},
		cities: []string{
			"Berlin", "München", "Hamburg", "Köln", "Frankfurt", "Stuttgart",
			"Düsseldorf", "Dortmund", "Essen", "Leipzig", "Bremen", "Dresden",
			"Hannover", "Nürnberg", "Duisburg",
		},
		phoneFormat: func(rng *rand.Rand) string {
			return fmt.Sprintf("+49 %d %08d", intn(rng, 90)+10, intn(rng, 100000000))
		},
	}
}

// normalizeLocale canonicalises a locale code: trims whitespace, splits on '-'
// or '_', lowercases language part, uppercases region part.
// Examples: "de_de" -> "de-DE", "EN" -> "en", "  fr-FR  " -> "fr-FR".
func normalizeLocale(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	// Split on '-' or '_'
	sep := ""
	if strings.Contains(code, "-") {
		sep = "-"
	} else if strings.Contains(code, "_") {
		sep = "_"
	}
	if sep != "" {
		parts := strings.SplitN(code, sep, 2)
		lang := strings.ToLower(parts[0])
		region := strings.ToUpper(parts[1])
		return lang + "-" + region
	}
	// Language only
	return strings.ToLower(code)
}

// fallbackChain returns the resolution order for a normalized code, e.g.
// "en-GB" -> ["en-GB", "en", "en-US"], "de-DE" -> ["de-DE", "de", "en-US"].
// Always terminates at "en-US" (the default pool).
func fallbackChain(code string) []string {
	chain := []string{code}
	// Add language-only form if code has a region
	if idx := strings.Index(code, "-"); idx != -1 {
		chain = append(chain, code[:idx])
	}
	// Always end at en-US
	chain = append(chain, "en-US")
	return chain
}

// resolveLocaleData validates code against supportedLocales, then walks the
// fallback chain to the first code with a shipped pool.
// Returns the resolved *localeData, the code actually used (for the warning),
// and a bool true when a fallback occurred.
// Returns ErrLocaleUnknown (structured) for unsupported codes.
// Empty code resolves to en-US without error.
func resolveLocaleData(code string) (data *localeData, usedCode string, fellBack bool, err error) {
	normalized := normalizeLocale(code)
	if normalized == "" {
		return localePools["en-US"], "en-US", false, nil
	}

	// Validate: must be in the supported table
	if !supportedLocaleSet[normalized] {
		return nil, "", false, localeUnknownError(normalized)
	}

	// Walk the fallback chain to find a shipped pool
	chain := fallbackChain(normalized)
	for _, candidate := range chain {
		if pool, ok := localePools[candidate]; ok {
			fell := candidate != normalized
			return pool, candidate, fell, nil
		}
	}

	// Should not reach here — en-US is always in localePools
	return localePools["en-US"], "en-US", true, nil
}

// localeUnknownError builds the structured ERR_LOCALE_UNKNOWN error listing
// the supported locales.
func localeUnknownError(code string) error {
	return &apierrors.Structured{
		Category: apierrors.CategoryInput,
		Code:     "ERR_LOCALE_UNKNOWN",
		Message:  fmt.Sprintf("unknown locale %q", code),
		Hint:     "Supported locales: " + strings.Join(supportedLocales, ", "),
		Inner:    ErrLocaleUnknown,
	}
}
