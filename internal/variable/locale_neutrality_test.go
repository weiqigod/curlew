package variable

import (
	"strings"
	"testing"
)

// localeAwareFuncs names the $faker.* functions docs/MANUAL.md:1311-1314
// documents as locale-aware: the personal-data family plus $faker.city.
// Everything else under the faker. prefix is locale-neutral by design --
// docs/MANUAL.md says so five separate times, once per family (location
// pools other than city: 1149; company: 1163; internet: 1182; content:
// 1212; financial: 1254; file: 1289) -- and until this file, nothing
// checked it.
//
// A newly added faker.* function is auto-classified as locale-neutral by
// TestFaker_localeNeutralFunctionsIgnoreLocale and fails loudly the moment
// it consults r.locale, forcing an explicit decision to add it here instead.
var localeAwareFuncs = map[string]bool{
	"faker.firstName":          true,
	"faker.lastName":           true,
	"faker.fullName":           true,
	"faker.username":           true,
	"faker.email":              true,
	"faker.phone":              true,
	"faker.phoneInternational": true,
	"faker.city":               true,
}

// fakerFuncNames returns every registered $faker.* function name.
func fakerFuncNames(t *testing.T) []string {
	t.Helper()
	reg := NewRegistry(nil)
	var names []string
	for _, name := range reg.Available() {
		if strings.HasPrefix(name, "faker.") {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		t.Fatal("no faker.* functions registered — the registry is broken, not the locale contract")
	}
	return names
}

// evalAt evaluates funcName with no arguments against a freshly built
// registry at locale and seed. No-arg is valid for every faker.* function:
// every arg-taking one (words, sentence, paragraph, text, price) defaults on
// len(args) == 0.
func evalAt(t *testing.T, funcName, locale string, seed int64) string {
	t.Helper()
	reg := NewRegistry(&seed, WithLocale(locale))
	got, err := reg.Evaluate(funcName, nil, make(map[string]string))
	if err != nil {
		t.Fatalf("%s at locale %s seed %d: Evaluate error: %v", funcName, locale, seed, err)
	}
	return got
}

// TestFaker_localeNeutralFunctionsIgnoreLocale proves the per-family
// locale-*neutrality* docs/MANUAL.md promises: every $faker.* function
// outside localeAwareFuncs produces byte-identical output regardless of
// --locale.
//
// NewRegistry performs no RNG draws during construction -- it seeds the PCG,
// resolves the locale, then only assigns closures (see dynamic.go's
// register) -- so two registries built from the same seed under different
// locales sit at an identical RNG position. Any divergence in a neutral
// function's output can therefore only come from the function itself
// reading locale-specific data, which is exactly the bug this guards
// against.
//
// This test (with its converse below) replaces seven t.Skip stubs that used
// to sit in dynamic_test.go, one per $faker.* family, all skipping
// unconditionally on "--locale flag is deferred from M13". M20-001 shipped
// --locale; the reason expired four milestones before this test was
// written. A permanently-skipped test proves nothing while still appearing
// green in the suite -- this replaces that with a test that actually
// exercises the promise the stubs gestured at.
func TestFaker_localeNeutralFunctionsIgnoreLocale(t *testing.T) {
	const seed = int64(98765)

	checked := 0
	for _, fn := range fakerFuncNames(t) {
		if localeAwareFuncs[fn] {
			continue
		}
		fn := fn
		checked++
		t.Run(fn, func(t *testing.T) {
			want := evalAt(t, fn, "en-US", seed)
			for _, code := range supportedLocales {
				if code == "en-US" {
					continue
				}
				if got := evalAt(t, fn, code, seed); got != want {
					t.Errorf("locale %s: got %q, want %q (en-US baseline) — a locale-neutral family must ignore --locale", code, got, want)
				}
			}
		})
	}
	if checked == 0 {
		t.Fatal("no locale-neutral faker.* functions found — the classification or the registry is broken")
	}
}

// TestFaker_localeAwareFunctionsVaryByLocale is the converse of
// TestFaker_localeNeutralFunctionsIgnoreLocale: without it, localeAwareFuncs
// could rot into a lie -- a function left in the set (or never removed after
// its locale pools were dropped) that no longer actually varies by locale --
// and silently hollow out the neutral test's coverage, since anything listed
// here is exempted from it.
func TestFaker_localeAwareFunctionsVaryByLocale(t *testing.T) {
	if len(localeAwareFuncs) == 0 {
		t.Fatal("localeAwareFuncs is empty — this test would pass vacuously")
	}

	candidates := []string{"de-DE", "ja-JP", "ru-RU"}
	seeds := []int64{1, 2, 3, 4, 5}

	for fn := range localeAwareFuncs {
		fn := fn
		t.Run(fn, func(t *testing.T) {
			varied := false
			for _, seed := range seeds {
				enUS := evalAt(t, fn, "en-US", seed)
				for _, code := range candidates {
					if evalAt(t, fn, code, seed) != enUS {
						varied = true
					}
				}
			}
			if !varied {
				t.Errorf("%s produced output identical to en-US across every seed and every locale in %v — is it still locale-aware?", fn, candidates)
			}
		})
	}
}
