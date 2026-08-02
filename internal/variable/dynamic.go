package variable

import (
	"crypto/hmac"
	"crypto/md5"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	apierrors "github.com/weiqigod/curlew/internal/errors"
)

// DynFunc generates a dynamic variable value. args is the resolved argument
// list parsed from {{$fn('a', 'b')}} (nil/empty for the no-args form).
// Returns a structured error on arity mismatch or per-function failure.
type DynFunc func(rng *rand.Rand, args []string) (string, error)

// Registry holds registered dynamic variable functions.
type Registry struct {
	funcs           map[string]DynFunc
	rng             *rand.Rand       // nil = use real randomness
	now             func() time.Time // clock seam — default time.Now; overridable in tests for frozen-clock date-arithmetic helpers
	sensitiveArgIdx map[string]int   // funcName → 0-indexed arg position whose resolved value should be auto-marked sensitive when the arg's source variable is sensitive (e.g. $hmacSha256 → key is index 1)
	// sensitiveReturn[funcName] = true marks the function's *return value*
	// as auto-sensitive: Scope.Interpolate calls runtimeSensitive.AddValue
	// on the result string after a successful Evaluate. Mirrors the
	// arg-side sensitiveArgIdx hook for value-generating helpers like
	// $faker.ssn. No-op when the scope has no runtimeSensitive set.
	sensitiveReturn map[string]bool
	locale          *localeData // active locale pool; never nil after NewRegistry (defaults to en-US)
}

// Option configures a Registry at construction. Variadic so existing
// NewRegistry(seed) call sites are unaffected.
type Option func(*registryConfig)

type registryConfig struct {
	locale string       // normalized "" means en-US default
	onWarn func(string) // verbose fallback-warning sink; nil = silent
}

// WithLocale selects the faker locale pool. Empty/"en-US" is the default.
// Unsupported codes are silently treated as en-US inside the registry;
// callers should call ValidateLocale before constructing the registry to
// surface ERR_LOCALE_UNKNOWN to the user.
func WithLocale(code string) Option { return func(c *registryConfig) { c.locale = code } }

// WithLocaleWarning installs a sink for the verbose fallback warning
// ("locale en-GB not available; falling back to en-US").
func WithLocaleWarning(fn func(string)) Option { return func(c *registryConfig) { c.onWarn = fn } }

// ValidateLocale reports ERR_LOCALE_UNKNOWN for an unsupported code, or nil.
// Exposed so the CLI layer can reject xx-YY before building the registry.
func ValidateLocale(code string) error {
	if code == "" {
		return nil
	}
	_, _, _, err := resolveLocaleData(code)
	return err
}

// NormalizeLocale returns the canonical form of a locale code
// (e.g. "de_DE" → "de-DE", "EN" → "en", "  fr-FR  " → "fr-FR").
// Exposed so the CLI layer can display a consistent value in diagnostics.
func NormalizeLocale(code string) string { return normalizeLocale(code) }

// NewRegistry creates a dynamic function registry.
// If seed is non-nil, all RNG-based functions use a deterministic source seeded with *seed.
// Optional Option values configure locale and warning behaviour; existing call sites
// that pass only seed remain valid.
func NewRegistry(seed *int64, opts ...Option) *Registry {
	cfg := registryConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	r := &Registry{
		funcs:           make(map[string]DynFunc),
		sensitiveArgIdx: make(map[string]int),
		sensitiveReturn: make(map[string]bool),
		now:             time.Now,
	}
	if seed != nil {
		r.rng = rand.New(rand.NewPCG(uint64(*seed), uint64(*seed>>32^0xdeadbeef)))
	}
	// Resolve locale; unknown codes surface via ValidateLocale before registry
	// construction. Here we walk the fallback chain (never errors for a valid code)
	// and default to en-US for "".
	data, used, fellBack, _ := resolveLocaleData(cfg.locale)
	r.locale = data
	if fellBack && cfg.onWarn != nil {
		normalized := normalizeLocale(cfg.locale)
		cfg.onWarn(fmt.Sprintf("locale %s not available; falling back to %s", normalized, used))
	}
	r.register()
	return r
}

// IsSensitiveReturn reports whether funcName's return value should be
// auto-marked sensitive at evaluation time. Returns true when the function
// is registered with sensitive-return policy (e.g. $faker.ssn). No-op safe
// when called on a nil registry.
func (r *Registry) IsSensitiveReturn(funcName string) bool {
	if r == nil || r.sensitiveReturn == nil {
		return false
	}
	return r.sensitiveReturn[funcName]
}

// arityError builds the canonical arity-mismatch error used by the noArgs wrapper.
// The Message field intentionally omits the function-name prefix; callers such as
// Evaluate wrap the returned error with fmt.Errorf("$%s: %w", name, err) to provide
// that context. Keeping the prefix out of Message avoids a doubled "$name: $name:"
// prefix when the error surfaces in user-facing terminal or JSON output.
func arityError(funcName string, want, got int) error {
	return &apierrors.Structured{
		Category: apierrors.CategoryInput,
		Code:     "DYNFN_ARITY",
		Message:  fmt.Sprintf("expected %d arguments, got %d", want, got),
		Hint:     fmt.Sprintf("Call $%s with the correct number of arguments.", funcName),
	}
}

// noArgs wraps a zero-arg generator so it conforms to DynFunc and returns a
// structured arity error when called with unexpected arguments.
func noArgs(funcName string, fn func(*rand.Rand) string) DynFunc {
	return func(rng *rand.Rand, args []string) (string, error) {
		if len(args) > 0 {
			return "", arityError(funcName, 0, len(args))
		}
		return fn(rng), nil
	}
}

// oneArg wraps a single-argument pure function so it conforms to DynFunc and
// returns a structured arity error when called with anything other than one
// argument. The wrapped function does not see the RNG — the helpers built on
// top of oneArg are deterministic transformations of their string input.
func oneArg(funcName string, fn func(string) (string, error)) DynFunc {
	return func(_ *rand.Rand, args []string) (string, error) {
		if len(args) != 1 {
			return "", arityError(funcName, 1, len(args))
		}
		return fn(args[0])
	}
}

// twoArgs wraps a two-argument pure function so it conforms to DynFunc
// and returns a structured arity error when called with anything other
// than two arguments. Like oneArg, the wrapped function does not see
// the RNG — helpers built on twoArgs are deterministic transformations
// of their two string inputs (e.g. $hmacSha256(payload, key)).
func twoArgs(funcName string, fn func(a, b string) (string, error)) DynFunc {
	return func(_ *rand.Rand, args []string) (string, error) {
		if len(args) != 2 {
			return "", arityError(funcName, 2, len(args))
		}
		return fn(args[0], args[1])
	}
}

// twoOrThreeArgs wraps a 2-or-3-argument pure function so it conforms to
// DynFunc and returns a structured arity error when called with anything
// other than 2 or 3 args. The wrapped function receives all three positional
// values: when only two args are supplied the third is the empty string and
// the wrapped function is responsible for supplying the default. Used by
// $webhookSign.stripe and $webhookSign.slack, where the third arg is an
// optional timestamp that defaults to Registry.now().Unix() when omitted.
func twoOrThreeArgs(funcName string, fn func(a, b, c string) (string, error)) DynFunc {
	return func(_ *rand.Rand, args []string) (string, error) {
		switch len(args) {
		case 2:
			return fn(args[0], args[1], "")
		case 3:
			return fn(args[0], args[1], args[2])
		default:
			return "", &apierrors.Structured{
				Category: apierrors.CategoryInput,
				Code:     "DYNFN_ARITY",
				Message:  fmt.Sprintf("expected 2 or 3 arguments, got %d", len(args)),
				Hint:     fmt.Sprintf("Call $%s with 2 args (body, secret) or 3 args (body, secret, timestamp).", funcName),
			}
		}
	}
}

// cacheKey produces a per-request memoization key.
// Format: funcName + NUL + args joined by NUL.
// NUL cannot appear in YAML scalars or single-quoted arg literals, preventing collisions.
// A no-args call produces funcName + NUL — distinct from any non-empty arg key.
func cacheKey(funcName string, args []string) string {
	if len(args) == 0 {
		return funcName + "\x00"
	}
	return funcName + "\x00" + strings.Join(args, "\x00")
}

// Evaluate returns the value for the named function with the given args.
// cache provides per-request memoization keyed by funcName plus canonicalised args.
// Returns a descriptive error if the function name is not recognised, or a
// structured error if the function rejects the given argument count.
func (r *Registry) Evaluate(name string, args []string, cache map[string]string) (string, error) {
	key := cacheKey(name, args)
	if cache != nil {
		if v, ok := cache[key]; ok {
			return v, nil
		}
	}
	fn, ok := r.funcs[name]
	if !ok {
		avail := strings.Join(r.Available(), ", ")
		return "", fmt.Errorf("unknown dynamic function %q; available: %s", name, avail)
	}
	val, err := fn(r.rng, args)
	if err != nil {
		return "", fmt.Errorf("$%s: %w", name, err)
	}
	if cache != nil {
		cache[key] = val
	}
	return val, nil
}

// Available returns a sorted list of all registered function names (without $ prefix).
func (r *Registry) Available() []string {
	names := make([]string, 0, len(r.funcs))
	for name := range r.funcs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SensitiveArgIndex returns the 0-indexed argument position for funcName
// whose resolved value should be added to the run's SensitiveSet when
// that argument's placeholder resolved from a sensitive source. The
// second return value is false when the function has no sensitive-arg
// policy. Callers that do not have access to the source-side sensitivity
// flag should treat ok=false as "no auto-redaction" rather than guessing.
func (r *Registry) SensitiveArgIndex(funcName string) (int, bool) {
	if r == nil || r.sensitiveArgIdx == nil {
		return 0, false
	}
	idx, ok := r.sensitiveArgIdx[funcName]
	return idx, ok
}

// register wires up all built-in dynamic functions.
// Zero-arg generators use noArgs; single-arg deterministic helpers use oneArg.
func (r *Registry) register() {
	// Timestamp functions — never seeded; always real time.
	r.funcs["timestamp"] = noArgs("timestamp", func(_ *rand.Rand) string {
		return strconv.FormatInt(time.Now().Unix(), 10)
	})
	r.funcs["isoTimestamp"] = noArgs("isoTimestamp", func(_ *rand.Rand) string {
		return time.Now().UTC().Format("2006-01-02T15:04:05Z")
	})
	r.funcs["timestampMs"] = noArgs("timestampMs", func(_ *rand.Rand) string {
		return strconv.FormatInt(time.Now().UnixMilli(), 10)
	})

	// UUID/GUID — seeded when rng is non-nil.
	r.funcs["uuid"] = noArgs("uuid", func(rng *rand.Rand) string { return makeUUID(rng) })
	r.funcs["guid"] = noArgs("guid", func(rng *rand.Rand) string { return makeUUID(rng) })

	// Random primitives.
	r.funcs["randomInt"] = noArgs("randomInt", func(rng *rand.Rand) string {
		return strconv.Itoa(intn(rng, 1001))
	})
	r.funcs["randomFloat"] = noArgs("randomFloat", func(rng *rand.Rand) string {
		f := float64InRange(rng) * 1000.0
		return strconv.FormatFloat(f, 'f', 2, 64)
	})
	r.funcs["randomBoolean"] = noArgs("randomBoolean", func(rng *rand.Rand) string {
		if intn(rng, 2) == 0 {
			return "true"
		}
		return "false"
	})
	r.funcs["randomString"] = noArgs("randomString", func(rng *rand.Rand) string {
		return randomString(rng, alphanumChars, 16)
	})
	r.funcs["randomHex"] = noArgs("randomHex", func(rng *rand.Rand) string {
		return randomString(rng, hexLowerChars, 32)
	})

	// Realistic values.
	r.funcs["randomEmail"] = noArgs("randomEmail", func(rng *rand.Rand) string {
		first := firstNames[intn(rng, len(firstNames))]
		n := intn(rng, 9000) + 1000
		return strings.ToLower(first) + "." + strconv.Itoa(n) + "@example.com"
	})
	r.funcs["randomName"] = noArgs("randomName", func(rng *rand.Rand) string {
		return firstNames[intn(rng, len(firstNames))] + " " + lastNames[intn(rng, len(lastNames))]
	})
	r.funcs["randomFirstName"] = noArgs("randomFirstName", func(rng *rand.Rand) string {
		return firstNames[intn(rng, len(firstNames))]
	})
	r.funcs["randomLastName"] = noArgs("randomLastName", func(rng *rand.Rand) string {
		return lastNames[intn(rng, len(lastNames))]
	})
	r.funcs["randomColor"] = noArgs("randomColor", func(rng *rand.Rand) string {
		rv := intn(rng, 256)
		g := intn(rng, 256)
		b := intn(rng, 256)
		return fmt.Sprintf("#%02x%02x%02x", rv, g, b)
	})

	// Encoding helpers — single string argument; deterministic.
	r.funcs["base64"] = oneArg("base64", func(s string) (string, error) {
		return base64.StdEncoding.EncodeToString([]byte(s)), nil
	})
	r.funcs["base64Decode"] = oneArg("base64Decode", func(s string) (string, error) {
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			// Truncate the offending input to 32 chars so long base64 blobs
			// don't dominate the error message (or leak in full).
			snippet := s
			if len(snippet) > 32 {
				snippet = snippet[:32] + "..."
			}
			return "", &apierrors.Structured{
				Category: apierrors.CategoryInput,
				Code:     "DYNFN_BASE64_DECODE",
				Message:  fmt.Sprintf("invalid base64 input %q: %v", snippet, err),
				Hint:     "Provide a valid standard-base64 (RFC 4648 §4) input — including '=' padding.",
				Inner:    err,
			}
		}
		return string(decoded), nil
	})

	// URL / JSON encoding helpers — single string argument; deterministic.

	// urlEncode applies query-component percent-encoding via net/url.QueryEscape.
	// Space becomes '+', '&' becomes '%26', '/' becomes '%2F'. This matches the
	// query-string convention; it is not suitable for path segments.
	r.funcs["urlEncode"] = oneArg("urlEncode", func(s string) (string, error) {
		return url.QueryEscape(s), nil
	})

	// jsonEncode returns the canonical RFC 8259 JSON-string literal for the
	// input string, including the surrounding double quotes. Characters such as
	// '"', '\', and control bytes are escaped per the spec. The result is valid
	// JSON and can be inlined directly into a JSON body without further quoting.
	// json.Marshal of a Go string never returns a non-nil error.
	r.funcs["jsonEncode"] = oneArg("jsonEncode", func(s string) (string, error) {
		b, _ := json.Marshal(s)
		return string(b), nil
	})

	// Hashing helpers — single string argument; deterministic.
	//
	// sha256 returns the lowercase hex (64 chars) of crypto/sha256.Sum256
	// over the UTF-8 bytes of the input. The %x verb on a [32]byte produces
	// the canonical hash representation used by sha256sum(1) and every
	// webhook-signature scheme we target.
	r.funcs["sha256"] = oneArg("sha256", func(s string) (string, error) {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(s))), nil
	})

	// md5 returns the lowercase hex (32 chars) of crypto/md5.Sum over
	// the UTF-8 bytes of the input. Provided for parity with legacy
	// webhook signature schemes only — see MANUAL.md §3.7. New integrations
	// should prefer $sha256 or HMAC-SHA-256.
	r.funcs["md5"] = oneArg("md5", func(s string) (string, error) {
		return fmt.Sprintf("%x", md5.Sum([]byte(s))), nil
	})

	// hmacSha256 returns the lowercase hex (64 chars) of an HMAC-SHA-256
	// computed over the payload (arg 0) using the key (arg 1). Both
	// inputs are taken as raw UTF-8 byte sequences. crypto/hmac handles
	// the inner/outer XOR and key-padding/keying per RFC 2104; we use
	// the canonical %x verb for the hex encoding (matches sha256sum(1)
	// and every webhook-signature spec we target).
	//
	// Sensitive-key auto-redaction lives in the registry's
	// sensitiveArgIdx table (consulted by Scope.Interpolate), not in
	// this closure — keeping the function signature symmetric with the
	// other oneArg / twoArgs helpers and decoupling the credential
	// policy from the cryptographic transformation.
	r.funcs["hmacSha256"] = twoArgs("hmacSha256", func(payload, key string) (string, error) {
		h := hmac.New(sha256.New, []byte(key))
		h.Write([]byte(payload))
		return fmt.Sprintf("%x", h.Sum(nil)), nil
	})
	// Auto-redaction of the key argument (index 1) when its placeholder
	// resolved from a sensitive source — see Scope.Interpolate dispatch.
	r.sensitiveArgIdx["hmacSha256"] = 1

	// Date arithmetic helpers — two args (amount, unit); the resolved
	// instant is taken from r.now() (default time.Now, overridable in
	// tests). Result is ISO-8601 UTC matching $isoTimestamp shape.
	//
	// $dateAdd(amount, unit)        — now + amount*unit
	// $dateSubtract(amount, unit)   — now - amount*unit
	//
	// Supported units: second, minute, hour, day, week, month, year.
	// Negative amounts are accepted (so $dateAdd('-1', 'day') is valid).
	//
	// Month and year offsets follow time.AddDate semantics — Feb 31 rolls
	// forward to March 3 (or 2). Use day-based arithmetic when end-of-month
	// stability matters. See MANUAL.md §3.7.
	r.funcs["dateAdd"] = twoArgs("dateAdd", func(amount, unit string) (string, error) {
		return r.applyDateOffset(amount, unit, +1)
	})
	r.funcs["dateSubtract"] = twoArgs("dateSubtract", func(amount, unit string) (string, error) {
		return r.applyDateOffset(amount, unit, -1)
	})

	// Layout-aware date helpers — two args (input, layout). Layout is
	// always a Go reference-time layout (e.g. "2006-01-02 15:04:05").
	// See MANUAL.md §3.7 "Go reference-time layouts" for the convention
	// and the layout-as-template footgun.
	//
	// $parseDate(s, layout)         — time.Parse(layout, s) → ISO-8601 UTC.
	// $formatDate(input, layout)    — input is digits-only Unix seconds or
	//                                  RFC3339 string; result is t.Format(layout).
	//
	// Both functions carry no credential material — no sensitiveArgIdx entry.
	r.funcs["parseDate"] = twoArgs("parseDate", func(s, layout string) (string, error) {
		return parseDateToUTC(layout, s)
	})
	r.funcs["formatDate"] = twoArgs("formatDate", func(input, layout string) (string, error) {
		return formatDate(input, layout)
	})

	// Random-secret helpers — single integer-string argument; seeded-aware.
	//
	// $randomBase64(byteLength) — sample byteLength uniformly random bytes
	// via intn (which routes through the registry's seeded PCG when --seed
	// is set, else crypto/rand). Encode via base64.StdEncoding (RFC 4648 §4).
	//
	// This function carries no credential material as input — the output is
	// the secret. No sensitiveArgIdx entry.
	r.funcs["randomBase64"] = func(rng *rand.Rand, args []string) (string, error) {
		if len(args) != 1 {
			return "", arityError("randomBase64", 1, len(args))
		}
		n, err := strconv.Atoi(args[0])
		if err != nil {
			return "", &apierrors.Structured{
				Category: apierrors.CategoryInput,
				Code:     "DYNFN_RANDOMBASE64_BAD_INPUT",
				Message:  fmt.Sprintf("invalid byteLength %q: must be an integer", args[0]),
				Hint:     "Pass an integer string, e.g. {{$randomBase64('32')}}.",
				Inner:    err,
			}
		}
		if n < 1 {
			return "", &apierrors.Structured{
				Category: apierrors.CategoryInput,
				Code:     "DYNFN_RANDOMBASE64_BAD_LENGTH",
				Message:  fmt.Sprintf("byteLength %d is invalid: must be >= 1", n),
				Hint:     "Pass an integer string >= 1, e.g. {{$randomBase64('32')}}.",
			}
		}
		b := make([]byte, n)
		for i := 0; i < n; i++ {
			b[i] = byte(intn(rng, 256))
		}
		return base64.StdEncoding.EncodeToString(b), nil
	}

	// $randomPassword(length) — reject length < 4; guarantee one character from
	// each of four classes (upper, lower, digit, symbol), fill the remainder from
	// the union, then Fisher-Yates shuffle. Uses makePassword for the algorithm.
	r.funcs["randomPassword"] = func(rng *rand.Rand, args []string) (string, error) {
		if len(args) != 1 {
			return "", arityError("randomPassword", 1, len(args))
		}
		n, err := strconv.Atoi(args[0])
		if err != nil {
			return "", &apierrors.Structured{
				Category: apierrors.CategoryInput,
				Code:     "DYNFN_RANDOMPASSWORD_BAD_INPUT",
				Message:  fmt.Sprintf("invalid length %q: must be an integer", args[0]),
				Hint:     "Pass an integer string >= 4, e.g. {{$randomPassword('16')}}.",
				Inner:    err,
			}
		}
		if n < 4 {
			return "", &apierrors.Structured{
				Category: apierrors.CategoryInput,
				Code:     "DYNFN_RANDOMPASSWORD_BAD_LENGTH",
				Message:  fmt.Sprintf("length %d is too short: must be >= 4 to satisfy upper/lower/digit/symbol classes", n),
				Hint:     "Pass an integer string >= 4, e.g. {{$randomPassword('16')}}.",
			}
		}
		return makePassword(rng, n), nil
	}

	// $faker.* personal-data family — M13-002; locale-aware from M20-001.
	// Closures read r.locale.firstNames / lastNames so the active locale's
	// pool is selected by the SAME intn index (seed/locale position-invariance,
	// SPEC:1023-1026). en-US localeData aliases the existing file-scope vars so
	// en-US output is byte-identical to the pre-M20 baseline.
	// namePrefixes and nameSuffixes are en-US only (no locale pools yet).
	// SSN is registered separately below with a sensitiveReturn hook.
	r.funcs["faker.firstName"] = noArgs("faker.firstName", func(rng *rand.Rand) string {
		p := r.locale.firstNames
		return p[intn(rng, len(p))]
	})
	r.funcs["faker.lastName"] = noArgs("faker.lastName", func(rng *rand.Rand) string {
		p := r.locale.lastNames
		return p[intn(rng, len(p))]
	})
	r.funcs["faker.fullName"] = noArgs("faker.fullName", func(rng *rand.Rand) string {
		fn, ln := r.locale.firstNames, r.locale.lastNames
		// Draw order is fixed (first index, then last index) so the seed selects
		// the same positions in every locale; only the join order varies.
		first := fn[intn(rng, len(fn))]
		last := ln[intn(rng, len(ln))]
		if r.locale.familyNameFirst {
			return last + " " + first
		}
		return first + " " + last
	})
	r.funcs["faker.username"] = noArgs("faker.username", func(rng *rand.Rand) string {
		fn, ln := r.locale.firstNames, r.locale.lastNames
		first := strings.ToLower(fn[intn(rng, len(fn))])
		last := strings.ToLower(ln[intn(rng, len(ln))])
		suffix := intn(rng, 100) // 0..99
		return fmt.Sprintf("%s.%s%d", first, last, suffix)
	})
	r.funcs["faker.email"] = noArgs("faker.email", func(rng *rand.Rand) string {
		fn, ln := r.locale.firstNames, r.locale.lastNames
		first := strings.ToLower(fn[intn(rng, len(fn))])
		last := strings.ToLower(ln[intn(rng, len(ln))])
		return fmt.Sprintf("%s.%s@example.com", first, last)
	})
	r.funcs["faker.phone"] = noArgs("faker.phone", func(rng *rand.Rand) string {
		// Use locale-specific phone format when available; fall back to US format.
		if r.locale.phoneFormat != nil {
			return r.locale.phoneFormat(rng)
		}
		area := intn(rng, 800) + 200 // 200..999
		exch := intn(rng, 800) + 200 // 200..999
		sub := intn(rng, 10000)      // 0000..9999
		return fmt.Sprintf("(%03d) %03d-%04d", area, exch, sub)
	})
	r.funcs["faker.phoneInternational"] = noArgs("faker.phoneInternational", func(rng *rand.Rand) string {
		// Use locale-specific phone format when available (same format for both
		// phone and phoneInternational — most locales use international format).
		if r.locale.phoneFormat != nil {
			return r.locale.phoneFormat(rng)
		}
		area := intn(rng, 800) + 200
		exch := intn(rng, 800) + 200
		sub := intn(rng, 10000)
		return fmt.Sprintf("+1-%03d-%03d-%04d", area, exch, sub)
	})
	r.funcs["faker.namePrefix"] = noArgs("faker.namePrefix", func(rng *rand.Rand) string {
		return namePrefixes[intn(rng, len(namePrefixes))]
	})
	r.funcs["faker.nameSuffix"] = noArgs("faker.nameSuffix", func(rng *rand.Rand) string {
		return nameSuffixes[intn(rng, len(nameSuffixes))]
	})

	// $faker.ssn — auto-sensitive: the generated SSN is registered in
	// runtimeSensitive via the IsSensitiveReturn hook in Scope.Interpolate.
	// SPECIFICATION.md:849-857 — only auto-sensitive function in the
	// personal-data category. Numbers use non-zero ranges to mirror real SSA
	// issuance constraints (zero-blocks are reserved/invalid).
	r.funcs["faker.ssn"] = noArgs("faker.ssn", func(rng *rand.Rand) string {
		area := intn(rng, 999) + 1    // 001..999
		group := intn(rng, 99) + 1    // 01..99
		serial := intn(rng, 9999) + 1 // 0001..9999
		return fmt.Sprintf("%03d-%02d-%04d", area, group, serial)
	})
	r.sensitiveReturn["faker.ssn"] = true

	// $faker.* location-data family — M13-003 (en-US only).
	// SPECIFICATION.md:774-789. All 12 functions are deterministic under
	// --seed via intn / float64InRange. Pool invariants (parallel
	// stateNames/stateAbbrs, countryNames/countryCodes) are asserted in
	// TestFakerLocation_PoolAlignment.
	r.funcs["faker.streetName"] = noArgs("faker.streetName", func(rng *rand.Rand) string {
		return streetNames[intn(rng, len(streetNames))] + " " +
			streetSuffixes[intn(rng, len(streetSuffixes))]
	})
	r.funcs["faker.street"] = noArgs("faker.street", func(rng *rand.Rand) string {
		num := intn(rng, 9999) + 1
		return fmt.Sprintf("%d %s %s", num,
			streetNames[intn(rng, len(streetNames))],
			streetSuffixes[intn(rng, len(streetSuffixes))])
	})
	r.funcs["faker.city"] = noArgs("faker.city", func(rng *rand.Rand) string {
		p := r.locale.cities
		return p[intn(rng, len(p))]
	})
	r.funcs["faker.state"] = noArgs("faker.state", func(rng *rand.Rand) string {
		return stateNames[intn(rng, len(stateNames))]
	})
	r.funcs["faker.stateAbbr"] = noArgs("faker.stateAbbr", func(rng *rand.Rand) string {
		return stateAbbrs[intn(rng, len(stateAbbrs))]
	})
	r.funcs["faker.zipCode"] = noArgs("faker.zipCode", func(rng *rand.Rand) string {
		return fmt.Sprintf("%05d", intn(rng, 100000))
	})
	// $faker.address composes a full US address: <num> <street>, <city>, <stateAbbr> <zip>.
	// The state-abbr lookup uses a single intn over the parallel pool so the
	// rendered address pairs consistently. The full-name $faker.state and the
	// abbr $faker.stateAbbr are independent draws when called separately.
	r.funcs["faker.address"] = noArgs("faker.address", func(rng *rand.Rand) string {
		num := intn(rng, 9999) + 1
		street := streetNames[intn(rng, len(streetNames))] + " " +
			streetSuffixes[intn(rng, len(streetSuffixes))]
		city := cities[intn(rng, len(cities))]
		stateIdx := intn(rng, len(stateAbbrs))
		zip := fmt.Sprintf("%05d", intn(rng, 100000))
		return fmt.Sprintf("%d %s, %s, %s %s", num, street, city, stateAbbrs[stateIdx], zip)
	})
	r.funcs["faker.country"] = noArgs("faker.country", func(rng *rand.Rand) string {
		return countryNames[intn(rng, len(countryNames))]
	})
	r.funcs["faker.countryCode"] = noArgs("faker.countryCode", func(rng *rand.Rand) string {
		return countryCodes[intn(rng, len(countryCodes))]
	})
	// Latitude/longitude: float64InRange returns [0, 1); map to centred range
	// via (2v - 1) * scale; format with 4 decimal places per SPEC:787-788.
	// Return type is string by DynFunc contract — JSON-numeric vs JSON-string
	// is decided by the placeholder's surrounding quotes at interpolation time,
	// matching $randomInt / $randomFloat behaviour.
	r.funcs["faker.latitude"] = noArgs("faker.latitude", func(rng *rand.Rand) string {
		return strconv.FormatFloat((2*float64InRange(rng)-1)*90, 'f', 4, 64)
	})
	r.funcs["faker.longitude"] = noArgs("faker.longitude", func(rng *rand.Rand) string {
		return strconv.FormatFloat((2*float64InRange(rng)-1)*180, 'f', 4, 64)
	})
	r.funcs["faker.timezone"] = noArgs("faker.timezone", func(rng *rand.Rand) string {
		return ianaTimezones[intn(rng, len(ianaTimezones))]
	})

	// $faker.* company-data family — M13-004 (en-US only).
	// SPECIFICATION.md:791-799. All 5 functions are deterministic under
	// --seed via intn. catchPhrase composes three independent draws
	// (adjective + noun + gerund) joined by single spaces; the fixed
	// call order makes the result byte-stable under --seed.
	r.funcs["faker.company"] = noArgs("faker.company", func(rng *rand.Rand) string {
		return companies[intn(rng, len(companies))]
	})
	r.funcs["faker.companySuffix"] = noArgs("faker.companySuffix", func(rng *rand.Rand) string {
		return companySuffixes[intn(rng, len(companySuffixes))]
	})
	r.funcs["faker.jobTitle"] = noArgs("faker.jobTitle", func(rng *rand.Rand) string {
		return jobTitles[intn(rng, len(jobTitles))]
	})
	r.funcs["faker.department"] = noArgs("faker.department", func(rng *rand.Rand) string {
		return departments[intn(rng, len(departments))]
	})
	r.funcs["faker.catchPhrase"] = noArgs("faker.catchPhrase", func(rng *rand.Rand) string {
		adj := catchPhraseAdjectives[intn(rng, len(catchPhraseAdjectives))]
		noun := catchPhraseNouns[intn(rng, len(catchPhraseNouns))]
		gerund := catchPhraseGerunds[intn(rng, len(catchPhraseGerunds))]
		return adj + " " + noun + " " + gerund
	})

	// $faker.* internet-data family — M13-005 (en-US only).
	// SPECIFICATION.md:801-813. All 9 functions are deterministic under
	// --seed via intn. $faker.color and the legacy $randomColor co-exist
	// (M13 Open Decision #3): the former returns a CSS color name from
	// cssColorNames, the latter returns a #RRGGBB hex string. No aliasing.
	r.funcs["faker.url"] = noArgs("faker.url", func(rng *rand.Rand) string {
		root := domainNameRoots[intn(rng, len(domainNameRoots))]
		tld := tldSuffixes[intn(rng, len(tldSuffixes))]
		seg := urlPathSegments[intn(rng, len(urlPathSegments))]
		return "https://" + root + "." + tld + "/" + seg
	})
	r.funcs["faker.domain"] = noArgs("faker.domain", func(rng *rand.Rand) string {
		root := domainNameRoots[intn(rng, len(domainNameRoots))]
		tld := tldSuffixes[intn(rng, len(tldSuffixes))]
		return root + "." + tld
	})
	r.funcs["faker.domainSuffix"] = noArgs("faker.domainSuffix", func(rng *rand.Rand) string {
		return tldSuffixes[intn(rng, len(tldSuffixes))]
	})
	// IPv4: four independent octet draws in [0, 255]; full address space.
	r.funcs["faker.ip"] = noArgs("faker.ip", func(rng *rand.Rand) string {
		return fmt.Sprintf("%d.%d.%d.%d",
			intn(rng, 256), intn(rng, 256), intn(rng, 256), intn(rng, 256))
	})
	// IPv6: 8 groups of 4 lowercase hex digits, full-form (no "::"
	// abbreviation). net.ParseIP accepts every shipped permutation.
	r.funcs["faker.ipv6"] = noArgs("faker.ipv6", func(rng *rand.Rand) string {
		return fmt.Sprintf("%04x:%04x:%04x:%04x:%04x:%04x:%04x:%04x",
			intn(rng, 65536), intn(rng, 65536), intn(rng, 65536), intn(rng, 65536),
			intn(rng, 65536), intn(rng, 65536), intn(rng, 65536), intn(rng, 65536))
	})
	// MAC-48: six octets of UPPERCASE hex digits joined by ":" per
	// behavior 6 (^([0-9A-F]{2}:){5}[0-9A-F]{2}$).
	r.funcs["faker.mac"] = noArgs("faker.mac", func(rng *rand.Rand) string {
		return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
			intn(rng, 256), intn(rng, 256), intn(rng, 256),
			intn(rng, 256), intn(rng, 256), intn(rng, 256))
	})
	r.funcs["faker.userAgent"] = noArgs("faker.userAgent", func(rng *rand.Rand) string {
		return userAgents[intn(rng, len(userAgents))]
	})
	// $faker.color: CSS color name (per SPECIFICATION.md:812).
	// Distinct from $randomColor (hex). See M13 Open Decision #3.
	r.funcs["faker.color"] = noArgs("faker.color", func(rng *rand.Rand) string {
		return cssColorNames[intn(rng, len(cssColorNames))]
	})
	// $faker.hexColor: #RRGGBB lowercase hex. Three intn(rng, 256) draws,
	// registered as a separate entry from $randomColor.
	r.funcs["faker.hexColor"] = noArgs("faker.hexColor", func(rng *rand.Rand) string {
		rv := intn(rng, 256)
		g := intn(rng, 256)
		b := intn(rng, 256)
		return fmt.Sprintf("#%02x%02x%02x", rv, g, b)
	})

	// $faker.* content-data family — M13-006 (en-US only; locale deferred
	// per M13 Open Decision #1). SPECIFICATION.md:815-823. All 5 functions
	// are deterministic under --seed via intn over loremWords. Four of the
	// five accept an optional integer-string argument parsed by M12-001's
	// parseDynArgs unchanged; the registration body strconv.Atoi-converts
	// args[0] internally, mirroring $randomBase64 and $randomPassword.

	// $faker.word — single lowercase word from the lorem-ipsum pool.
	r.funcs["faker.word"] = noArgs("faker.word", func(rng *rand.Rand) string {
		return loremWords[intn(rng, len(loremWords))]
	})

	// $faker.words(count) — count space-joined lowercase words; default 3.
	r.funcs["faker.words"] = func(rng *rand.Rand, args []string) (string, error) {
		n, err := parseFakerCountArg("faker.words", args, 3)
		if err != nil {
			return "", err
		}
		return generateWords(rng, n), nil
	}

	// $faker.sentence(wordCount) — wordCount words, first capitalised,
	// period-terminated; default range 6..10 words inclusive (5 values).
	r.funcs["faker.sentence"] = func(rng *rand.Rand, args []string) (string, error) {
		var n int
		if len(args) == 0 {
			n = intn(rng, 5) + 6 // 6..10
		} else {
			parsed, err := parseFakerCountArg("faker.sentence", args, 0)
			if err != nil {
				return "", err
			}
			n = parsed
		}
		return generateSentence(rng, n), nil
	}

	// $faker.paragraph(sentenceCount) — sentenceCount sentences (each
	// 6..10 words, default range), joined by single spaces; default
	// range 3..5 sentences inclusive (3 values).
	r.funcs["faker.paragraph"] = func(rng *rand.Rand, args []string) (string, error) {
		var n int
		if len(args) == 0 {
			n = intn(rng, 3) + 3 // 3..5
		} else {
			parsed, err := parseFakerCountArg("faker.paragraph", args, 0)
			if err != nil {
				return "", err
			}
			n = parsed
		}
		return generateParagraph(rng, n), nil
	}

	// $faker.text(charCount) — emit sentences until cumulative length
	// >= charCount, then stop at the sentence boundary (well-formed
	// prose; result length is within ~10 chars of charCount for
	// charCount >= 50). Default 200.
	r.funcs["faker.text"] = func(rng *rand.Rand, args []string) (string, error) {
		n, err := parseFakerCountArg("faker.text", args, 200)
		if err != nil {
			return "", err
		}
		return generateText(rng, n), nil
	}

	// $faker.* financial-data family — M13-007 (en-US only; locale
	// deferred per M13 Open Decision #1). SPECIFICATION.md:825-836.
	// All 8 functions are deterministic under --seed via intn /
	// float64InRange. Three (.creditCard, .creditCardCVV, .iban) are
	// auto-sensitive — their generated value is registered as a
	// redaction trigger via the IsSensitiveReturn hook in
	// Scope.Interpolate (mirrors the M13-002 $faker.ssn pattern).

	// $faker.price(min, max) — 2-decimal float in [min, max].
	// Default range is [1.00, 1000.00] per SPECIFICATION.md:829.
	// Args travel as quoted strings via M12-001's parseDynArgs
	// unchanged; the body strconv.ParseFloat-converts each.
	// 0-arg form uses defaults; 2-arg form is parsed; 1-arg or
	// >=3-arg forms produce a structured DYNFN_ARITY error.
	r.funcs["faker.price"] = func(rng *rand.Rand, args []string) (string, error) {
		minV, maxV := 1.0, 1000.0
		switch len(args) {
		case 0:
			// use defaults
		case 2:
			mv, err := strconv.ParseFloat(args[0], 64)
			if err != nil {
				return "", &apierrors.Structured{
					Category: apierrors.CategoryInput,
					Code:     "DYNFN_FAKER_PRICE_BAD_INPUT",
					Message:  fmt.Sprintf("invalid min %q: must be a decimal number", args[0]),
					Hint:     "Pass two single-quoted decimal strings, e.g. {{$faker.price('5', '50')}}.",
					Inner:    err,
				}
			}
			xv, err := strconv.ParseFloat(args[1], 64)
			if err != nil {
				return "", &apierrors.Structured{
					Category: apierrors.CategoryInput,
					Code:     "DYNFN_FAKER_PRICE_BAD_INPUT",
					Message:  fmt.Sprintf("invalid max %q: must be a decimal number", args[1]),
					Hint:     "Pass two single-quoted decimal strings, e.g. {{$faker.price('5', '50')}}.",
					Inner:    err,
				}
			}
			if mv > xv {
				return "", &apierrors.Structured{
					Category: apierrors.CategoryInput,
					Code:     "DYNFN_FAKER_PRICE_INVERTED_RANGE",
					Message:  fmt.Sprintf("inverted range: min %g > max %g", mv, xv),
					Hint:     "Pass min <= max, e.g. {{$faker.price('5', '50')}}.",
				}
			}
			minV, maxV = mv, xv
		default:
			return "", &apierrors.Structured{
				Category: apierrors.CategoryInput,
				Code:     "DYNFN_ARITY",
				Message:  fmt.Sprintf("expected 0 or 2 arguments, got %d", len(args)),
				Hint:     "Call $faker.price with no arguments (default range) or with two single-quoted decimals, e.g. {{$faker.price('5', '50')}}.",
			}
		}
		f := float64InRange(rng)*(maxV-minV) + minV
		return strconv.FormatFloat(f, 'f', 2, 64), nil
	}

	// $faker.currencyCode / .currencyName / .currencySymbol — three
	// independent draws over parallel pools aligned 1:1:1 by index.
	// Per behavior 5, the pools are aligned but the three functions
	// draw independently when called separately.
	r.funcs["faker.currencyCode"] = noArgs("faker.currencyCode", func(rng *rand.Rand) string {
		return currencyCodes[intn(rng, len(currencyCodes))]
	})
	r.funcs["faker.currencyName"] = noArgs("faker.currencyName", func(rng *rand.Rand) string {
		return currencyNames[intn(rng, len(currencyNames))]
	})
	r.funcs["faker.currencySymbol"] = noArgs("faker.currencySymbol", func(rng *rand.Rand) string {
		return currencySymbols[intn(rng, len(currencySymbols))]
	})

	// $faker.creditCard — Visa-like 16-digit number passing Luhn.
	// AUTO-SENSITIVE: the generated number is registered as a
	// redaction trigger via the IsSensitiveReturn hook.
	r.funcs["faker.creditCard"] = noArgs("faker.creditCard", func(rng *rand.Rand) string {
		prefix := append([]byte{'4'}, randomDigits(rng, 14)...)
		return string(append(prefix, luhnCheckDigit(prefix)))
	})
	r.sensitiveReturn["faker.creditCard"] = true

	// $faker.creditCardCVV — 3-or-4 digit CVV. Choice of 3 vs 4 is
	// a single random bit. AUTO-SENSITIVE.
	r.funcs["faker.creditCardCVV"] = noArgs("faker.creditCardCVV", func(rng *rand.Rand) string {
		n := 3
		if intn(rng, 2) == 1 {
			n = 4
		}
		return string(randomDigits(rng, n))
	})
	r.sensitiveReturn["faker.creditCardCVV"] = true

	// $faker.iban — randomly chosen country, random BBAN, mod-97
	// check digits. AUTO-SENSITIVE.
	r.funcs["faker.iban"] = noArgs("faker.iban", func(rng *rand.Rand) string {
		idx := intn(rng, len(ibanCountries))
		country := ibanCountries[idx]
		totalLen := ibanLengths[idx]
		bbanLen := totalLen - 4
		bban := make([]byte, bbanLen)
		for i := 0; i < bbanLen; i++ {
			bban[i] = ibanBBANChars[intn(rng, len(ibanBBANChars))]
		}
		bbanStr := string(bban)
		return country + ibanCheckDigits(country, bbanStr) + bbanStr
	})
	r.sensitiveReturn["faker.iban"] = true

	// $faker.bic — 6 uppercase letters (bank+country) + 2 alphanum
	// (location) + optional 3 alphanum (branch). NOT auto-sensitive
	// — BIC identifies a bank, not an account (per
	// SPECIFICATION.md:849-857).
	r.funcs["faker.bic"] = noArgs("faker.bic", func(rng *rand.Rand) string {
		const upper = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		var b strings.Builder
		b.Grow(11)
		for i := 0; i < 6; i++ {
			b.WriteByte(upper[intn(rng, len(upper))])
		}
		for i := 0; i < 2; i++ {
			b.WriteByte(ibanBBANChars[intn(rng, len(ibanBBANChars))])
		}
		// 50/50 between 8-char and 11-char BIC.
		if intn(rng, 2) == 1 {
			for i := 0; i < 3; i++ {
				b.WriteByte(ibanBBANChars[intn(rng, len(ibanBBANChars))])
			}
		}
		return b.String()
	})

	// $faker.* file-data family — M13-008 (en-US only; locale
	// deferred per M13 Open Decision #1). SPECIFICATION.md:838-845.
	// Three RNG-bearing functions are deterministic under --seed via
	// intn over the parallel fileExtensions / fileMimeTypes pool.
	// $faker.imageUrl has 0-or-2 arity that mirrors $faker.price
	// (M13-007); the no-arg form returns a constant default URL and
	// is intentionally deterministic across runs (per behavior 9).

	// $faker.fileName — base name + dotted extension, both lowercase.
	// Result matches ^[a-z0-9_-]+\.[a-z0-9]+$ per behavior 1.
	r.funcs["faker.fileName"] = noArgs("faker.fileName", func(rng *rand.Rand) string {
		base := fileBaseNames[intn(rng, len(fileBaseNames))]
		ext := fileExtensions[intn(rng, len(fileExtensions))]
		return base + "." + ext
	})

	// $faker.fileExtension — 2-to-4 char lowercase string with no
	// leading dot. Matches ^[a-z0-9]{2,4}$ per behavior 2.
	r.funcs["faker.fileExtension"] = noArgs("faker.fileExtension", func(rng *rand.Rand) string {
		return fileExtensions[intn(rng, len(fileExtensions))]
	})

	// $faker.mimeType — type/subtype matching ^[a-z]+/[a-z0-9.+-]+$
	// per behavior 3. Drawn from the parallel fileMimeTypes pool.
	r.funcs["faker.mimeType"] = noArgs("faker.mimeType", func(rng *rand.Rand) string {
		return fileMimeTypes[intn(rng, len(fileMimeTypes))]
	})

	// $faker.imageUrl — 0-or-2 arity. No-arg form returns the
	// constant default https://picsum.photos/640/480 (behavior 4).
	// Two-arg form parses positive-integer width/height via
	// strconv.Atoi and formats https://picsum.photos/<w>/<h>
	// (behavior 5). Bad input → DYNFN_FAKER_IMAGEURL_BAD_INPUT;
	// non-positive → DYNFN_FAKER_IMAGEURL_BAD_DIMENSION (behaviors
	// 6, 7). 1-arg or 3+-arg → DYNFN_ARITY ("expected 0 or 2
	// arguments"), mirroring $faker.price.
	r.funcs["faker.imageUrl"] = func(_ *rand.Rand, args []string) (string, error) {
		switch len(args) {
		case 0:
			return "https://picsum.photos/640/480", nil
		case 2:
			w, err := strconv.Atoi(args[0])
			if err != nil {
				return "", &apierrors.Structured{
					Category: apierrors.CategoryInput,
					Code:     "DYNFN_FAKER_IMAGEURL_BAD_INPUT",
					Message:  fmt.Sprintf("invalid width %q: must be an integer", args[0]),
					Hint:     "Pass two single-quoted positive-integer strings, e.g. {{$faker.imageUrl('800', '600')}}.",
					Inner:    err,
				}
			}
			h, err := strconv.Atoi(args[1])
			if err != nil {
				return "", &apierrors.Structured{
					Category: apierrors.CategoryInput,
					Code:     "DYNFN_FAKER_IMAGEURL_BAD_INPUT",
					Message:  fmt.Sprintf("invalid height %q: must be an integer", args[1]),
					Hint:     "Pass two single-quoted positive-integer strings, e.g. {{$faker.imageUrl('800', '600')}}.",
					Inner:    err,
				}
			}
			if w < 1 {
				return "", &apierrors.Structured{
					Category: apierrors.CategoryInput,
					Code:     "DYNFN_FAKER_IMAGEURL_BAD_DIMENSION",
					Message:  fmt.Sprintf("width %d is invalid: must be a positive integer", w),
					Hint:     "Pass positive-integer dimensions, e.g. {{$faker.imageUrl('800', '600')}}.",
				}
			}
			if h < 1 {
				return "", &apierrors.Structured{
					Category: apierrors.CategoryInput,
					Code:     "DYNFN_FAKER_IMAGEURL_BAD_DIMENSION",
					Message:  fmt.Sprintf("height %d is invalid: must be a positive integer", h),
					Hint:     "Pass positive-integer dimensions, e.g. {{$faker.imageUrl('800', '600')}}.",
				}
			}
			return fmt.Sprintf("https://picsum.photos/%d/%d", w, h), nil
		default:
			return "", &apierrors.Structured{
				Category: apierrors.CategoryInput,
				Code:     "DYNFN_ARITY",
				Message:  fmt.Sprintf("expected 0 or 2 arguments, got %d", len(args)),
				Hint:     "Call $faker.imageUrl with no arguments (default 640x480) or with two single-quoted positive integers, e.g. {{$faker.imageUrl('800', '600')}}.",
			}
		}
	}

	// $webhookSign.stripe / .github / .slack — M17-004.
	// Each function emits the provider-specific webhook-signature header
	// VALUE (not the header name) shaped per the published spec. All three
	// are HMAC-SHA256 under the secret (arg index 1); the secret is
	// registered in sensitiveArgIdx so a sensitive-named source variable
	// propagates into the run's SensitiveSet via the Pass-1 plumbing in
	// internal/variable/variable.go.
	//
	// stripe:  t=<ts>,v1=<hex>   hex = HMAC-SHA256("<ts>.<body>", secret)
	// github:  sha256=<hex>      hex = HMAC-SHA256(body, secret)
	// slack:   v0=<hex>          hex = HMAC-SHA256("v0:<ts>:<body>", secret)
	//
	// stripe and slack accept an optional 3rd arg (timestamp); when
	// omitted the registry's clock seam r.now().Unix() supplies it.
	r.funcs["webhookSign.stripe"] = twoOrThreeArgs("webhookSign.stripe", func(body, secret, ts string) (string, error) {
		if ts == "" {
			ts = strconv.FormatInt(r.now().Unix(), 10)
		}
		return stripeWebhookSig(body, secret, ts), nil
	})
	r.sensitiveArgIdx["webhookSign.stripe"] = 1

	r.funcs["webhookSign.github"] = twoArgs("webhookSign.github", func(body, secret string) (string, error) {
		return githubWebhookSig(body, secret), nil
	})
	r.sensitiveArgIdx["webhookSign.github"] = 1

	r.funcs["webhookSign.slack"] = twoOrThreeArgs("webhookSign.slack", func(body, secret, ts string) (string, error) {
		if ts == "" {
			ts = strconv.FormatInt(r.now().Unix(), 10)
		}
		return slackWebhookSig(body, secret, ts), nil
	})
	r.sensitiveArgIdx["webhookSign.slack"] = 1

	// $jwtDecodeHeader / $jwtDecodeClaims — M17-005.
	// Each function takes a single arg (the JWT token), splits on '.', and
	// returns the requested segment as canonical compact JSON via an
	// encoding/json roundtrip. Header is segment 0, claims is segment 1.
	// Signature (segment 2) is NOT examined — these helpers do NOT verify
	// the signature; that is explicitly out of scope. See MANUAL.md §3.7
	// for the security caveat. The shared helper decodeJWTSegment lives
	// in dynamic_helpers.go and shapes the three CategoryInput error codes
	// (DYNFN_JWT_DECODE_BAD_FORMAT / _BAD_BASE64 / _BAD_JSON).
	//
	// No sensitiveArgIdx registration: the single arg is the token (a
	// public bearer credential by convention), not a secret to be
	// auto-redacted as a side effect of this call. The token's own
	// sensitivity is preserved by the existing M12-005 plumbing wherever
	// it surfaces in serialised output.
	r.funcs["jwtDecodeHeader"] = oneArg("jwtDecodeHeader", func(token string) (string, error) {
		return decodeJWTSegment("jwtDecodeHeader", token, 0)
	})
	r.funcs["jwtDecodeClaims"] = oneArg("jwtDecodeClaims", func(token string) (string, error) {
		return decodeJWTSegment("jwtDecodeClaims", token, 1)
	})
}

// randomDigits returns n ASCII-digit bytes (each '0'..'9') drawn via
// intn. The caller must pass n >= 1; this function trusts its input.
// Used by the credit-card and CVV generators.
func randomDigits(rng *rand.Rand, n int) []byte {
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = byte(intn(rng, 10)) + '0'
	}
	return out
}

// parseFakerCountArg validates a 0-or-1-arg arity-flex argument list
// for the M13-006 content-data family. Returns defaultN when args is
// empty; otherwise strconv.Atoi-converts args[0] and rejects values
// < 1. The funcName is used for both the arity error and the
// structured BAD_INPUT / BAD_LENGTH error codes.
//
// Mirrors the body of $randomBase64 but generalised over the count default.
func parseFakerCountArg(funcName string, args []string, defaultN int) (int, error) {
	if len(args) > 1 {
		return 0, arityError(funcName, 1, len(args))
	}
	if len(args) == 0 {
		return defaultN, nil
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		return 0, &apierrors.Structured{
			Category: apierrors.CategoryInput,
			Code:     "DYNFN_FAKER_CONTENT_BAD_INPUT",
			Message:  fmt.Sprintf("invalid count %q: must be an integer", args[0]),
			Hint:     fmt.Sprintf("Pass an integer string, e.g. {{$%s('5')}}.", funcName),
			Inner:    err,
		}
	}
	if n < 1 {
		return 0, &apierrors.Structured{
			Category: apierrors.CategoryInput,
			Code:     "DYNFN_FAKER_CONTENT_BAD_LENGTH",
			Message:  fmt.Sprintf("count %d is invalid: must be >= 1", n),
			Hint:     fmt.Sprintf("Pass an integer string >= 1, e.g. {{$%s('5')}}.", funcName),
		}
	}
	return n, nil
}

// pickWords draws n words from loremWords via the seeded RNG.
// Each draw is independent; n must be >= 1 (caller's contract).
func pickWords(rng *rand.Rand, n int) []string {
	words := make([]string, n)
	for i := range words {
		words[i] = loremWords[intn(rng, len(loremWords))]
	}
	return words
}

// generateWords returns n space-joined lowercase words from loremWords.
// Caller must pass n >= 1 (parseFakerCountArg enforces this).
func generateWords(rng *rand.Rand, n int) string {
	return strings.Join(pickWords(rng, n), " ")
}

// generateSentence returns an n-word sentence from loremWords, first
// word capitalised (ASCII upper-case only — pool is lowercase ASCII)
// and period-terminated. Caller must pass n >= 1; if n == 0 the
// function returns a bare period rather than panicking (defensive guard
// per CLAUDE.md: return errors/safe values rather than panic).
func generateSentence(rng *rand.Rand, n int) string {
	words := pickWords(rng, n)
	if len(words) == 0 {
		return "."
	}
	// Capitalise first letter via ASCII bit-twiddle (pool is [a-z]+).
	if len(words[0]) > 0 {
		b := []byte(words[0])
		b[0] -= 'a' - 'A'
		words[0] = string(b)
	}
	return strings.Join(words, " ") + "."
}

// generateParagraph returns n sentences joined by single spaces;
// each sentence has a randomly-drawn 6..10 word count (range 5).
// Caller must pass n >= 1.
func generateParagraph(rng *rand.Rand, n int) string {
	sentences := make([]string, n)
	for i := range sentences {
		wordCount := intn(rng, 5) + 6 // 6..10
		sentences[i] = generateSentence(rng, wordCount)
	}
	return strings.Join(sentences, " ")
}

// generateText emits sentences until cumulative length >= charCount,
// then stops at the next sentence boundary. Each sentence has a
// randomly-drawn 6..10 word count (range 5). Result length is
// within ~10 chars of charCount for charCount >= 50; for very small
// charCount the smallest single-sentence overshoot is ~30 chars
// (six 4-letter words plus the period). Caller must pass charCount >= 1.
func generateText(rng *rand.Rand, charCount int) string {
	var out strings.Builder
	for out.Len() < charCount {
		wordCount := intn(rng, 5) + 6 // 6..10
		sentence := generateSentence(rng, wordCount)
		if out.Len() > 0 {
			out.WriteByte(' ')
		}
		out.WriteString(sentence)
	}
	return out.String()
}

// isAllDigits reports whether s is non-empty and every byte is an ASCII digit.
// Allocation-free; matches the digits-only branch of $formatDate's input parser.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// formatDate parses input as either a Unix-seconds digit string or an
// RFC3339 string, then formats the resulting time with layout.
//
// The two input shapes are deliberate — see MANUAL.md §3.7. layout is
// always a Go reference-time layout; if it contains no reference-time
// tokens, time.Format returns layout verbatim (this is intentional,
// documented stdlib behaviour — the "layout-as-template footgun").
func formatDate(input, layout string) (string, error) {
	if input == "" {
		return "", &apierrors.Structured{
			Category: apierrors.CategoryInput,
			Code:     "DYNFN_FORMATDATE_EMPTY_INPUT",
			Message:  "input string is empty",
			Hint:     "Pass either a Unix-seconds integer string (e.g. '1713701401') or an RFC3339 string (e.g. '2024-04-21T15:10:01Z') as the first argument.",
		}
	}
	if isAllDigits(input) {
		n, err := strconv.ParseInt(input, 10, 64)
		if err == nil {
			return time.Unix(n, 0).UTC().Format(layout), nil
		}
		// Overflow (input > MaxInt64) — fall through to bad-input.
	} else if t, err := time.Parse(time.RFC3339, input); err == nil {
		return t.Format(layout), nil
	}
	snippet := input
	if len(snippet) > 32 {
		snippet = snippet[:32] + "..."
	}
	return "", &apierrors.Structured{
		Category: apierrors.CategoryInput,
		Code:     "DYNFN_FORMATDATE_BAD_INPUT",
		Message:  fmt.Sprintf("cannot interpret %q as a Unix-seconds integer or an RFC3339 string", snippet),
		Hint:     "$formatDate accepts either a Unix-seconds integer string (e.g. '1713701401') or an RFC3339 string (e.g. '2024-04-21T15:10:01Z'). See MANUAL.md §3.7.",
	}
}

// parseDateToUTC parses s against layout (a Go reference-time layout),
// converts the result to UTC, and formats it as the canonical
// ISO-8601 UTC string (matching $isoTimestamp, $dateAdd, $dateSubtract).
//
// Empty s short-circuits with DYNFN_PARSEDATE_EMPTY_INPUT before
// calling time.Parse — the stdlib's empty-input error message is
// internal and does not name the user-facing function.
func parseDateToUTC(layout, s string) (string, error) {
	if s == "" {
		return "", &apierrors.Structured{
			Category: apierrors.CategoryInput,
			Code:     "DYNFN_PARSEDATE_EMPTY_INPUT",
			Message:  "input string is empty",
			Hint:     "Pass the date string to parse as the first argument, e.g. $parseDate('2024-04-21T15:10:01Z', '2006-01-02T15:04:05Z').",
		}
	}
	t, err := time.Parse(layout, s)
	if err != nil {
		snippet := s
		if len(snippet) > 32 {
			snippet = snippet[:32] + "..."
		}
		return "", &apierrors.Structured{
			Category: apierrors.CategoryInput,
			Code:     "DYNFN_PARSEDATE_BAD_INPUT",
			Message:  fmt.Sprintf("cannot parse %q with layout %q: %v", snippet, layout, err),
			Hint:     "layout must be a Go reference-time layout (e.g. '2006-01-02 15:04:05', not 'YYYY-MM-DD'). See MANUAL.md §3.7 'Go reference-time layouts'.",
			Inner:    err,
		}
	}
	return t.UTC().Format("2006-01-02T15:04:05Z"), nil
}

// applyDateOffset is the shared dispatch kernel for $dateAdd and $dateSubtract.
// sign is +1 for add, -1 for subtract; the kernel multiplies the parsed amount by sign.
// On parse failure, returns DYNFN_DATE_BAD_AMOUNT. On unknown unit, returns DYNFN_DATE_BAD_UNIT.
// The function-name prefix in error messages is added by Evaluate's fmt.Errorf wrapper.
func (r *Registry) applyDateOffset(amount, unit string, sign int) (string, error) {
	n, err := strconv.Atoi(amount)
	if err != nil {
		return "", &apierrors.Structured{
			Category: apierrors.CategoryInput,
			Code:     "DYNFN_DATE_BAD_AMOUNT",
			Message:  fmt.Sprintf("invalid amount %q: must be an integer", amount),
			Hint:     "Pass an integer string (e.g. '90'); decimal amounts are not supported — use a smaller unit (e.g. $dateAdd('90', 'minute') instead of '1.5' 'hour').",
			Inner:    err,
		}
	}
	n *= sign
	base := r.now()
	var result time.Time
	switch unit {
	case "second":
		result = base.Add(time.Duration(n) * time.Second)
	case "minute":
		result = base.Add(time.Duration(n) * time.Minute)
	case "hour":
		result = base.Add(time.Duration(n) * time.Hour)
	case "day":
		result = base.AddDate(0, 0, n)
	case "week":
		result = base.AddDate(0, 0, 7*n)
	case "month":
		result = base.AddDate(0, n, 0)
	case "year":
		result = base.AddDate(n, 0, 0)
	default:
		return "", &apierrors.Structured{
			Category: apierrors.CategoryInput,
			Code:     "DYNFN_DATE_BAD_UNIT",
			Message:  fmt.Sprintf("unknown unit %q", unit),
			Hint:     "Allowed units: second, minute, hour, day, week, month, year.",
		}
	}
	return result.UTC().Format("2006-01-02T15:04:05Z"), nil
}

// makeUUID generates a UUID v4 from the given RNG.
// If rng is nil, crypto/rand is used.
func makeUUID(rng *rand.Rand) string {
	var b [16]byte
	if rng == nil {
		_, _ = crand.Read(b[:])
	} else {
		binary.LittleEndian.PutUint64(b[0:8], rng.Uint64())
		binary.LittleEndian.PutUint64(b[8:16], rng.Uint64())
	}
	// Set version bits (4) at byte 6.
	b[6] = (b[6] & 0x0f) | 0x40
	// Set variant bits at byte 8.
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// intn returns a random integer in [0, n) using rng if non-nil, else crypto/rand.
func intn(rng *rand.Rand, n int) int {
	if rng != nil {
		return rng.IntN(n)
	}
	var b [8]byte
	_, _ = crand.Read(b[:])
	return int(binary.LittleEndian.Uint64(b[:]) % uint64(n))
}

// float64InRange returns a float64 in [0, 1) using rng if non-nil, else crypto/rand.
func float64InRange(rng *rand.Rand) float64 {
	if rng != nil {
		return rng.Float64()
	}
	var b [8]byte
	_, _ = crand.Read(b[:])
	return float64(binary.LittleEndian.Uint64(b[:])>>11) / (1 << 53)
}

// randomString returns a random string of length n drawn from charset.
func randomString(rng *rand.Rand, charset string, n int) string {
	sb := strings.Builder{}
	sb.Grow(n)
	for i := 0; i < n; i++ {
		sb.WriteByte(charset[intn(rng, len(charset))])
	}
	return sb.String()
}

// makePassword returns an n-character password (n >= 4) guaranteed to
// contain at least one ASCII upper, one lower, one digit, and one symbol
// from passwordSymbols. The remaining n-4 characters are sampled uniformly
// from the union of all four classes; the result is then Fisher-Yates
// shuffled so the four guaranteed characters are not always at positions 0..3.
// RNG dispatch via intn (seeded-aware).
//
// Caller must validate n >= 4; the function trusts its input.
func makePassword(rng *rand.Rand, n int) string {
	const allClasses = upperChars + lowerChars + digitChars + passwordSymbols
	buf := make([]byte, n)
	// Step 1: place one guaranteed character from each class.
	buf[0] = upperChars[intn(rng, len(upperChars))]
	buf[1] = lowerChars[intn(rng, len(lowerChars))]
	buf[2] = digitChars[intn(rng, len(digitChars))]
	buf[3] = passwordSymbols[intn(rng, len(passwordSymbols))]
	// Step 2: fill the rest from the union of all four classes.
	for i := 4; i < n; i++ {
		buf[i] = allClasses[intn(rng, len(allClasses))]
	}
	// Step 3: Fisher-Yates shuffle so the guaranteed positions are not always 0..3.
	for i := n - 1; i > 0; i-- {
		j := intn(rng, i+1)
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// Charsets used by the random-string family. Defined at file scope so
// they are constructed once per process, not per Registry.
const (
	alphanumChars   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	hexLowerChars   = "0123456789abcdef"
	upperChars      = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lowerChars      = "abcdefghijklmnopqrstuvwxyz"
	digitChars      = "0123456789"
	passwordSymbols = "!@#$%^&*()-_=+[]{}<>?,."
)

// Built-in name pools.
var firstNames = []string{
	"Alice", "Bob", "Carol", "Dave", "Eve", "Frank", "Grace", "Heidi",
	"Ivan", "Judy", "Kevin", "Laura", "Mike", "Nancy", "Oscar", "Peggy",
	"Quinn", "Rachel", "Steve", "Tina", "Ulysses", "Vera", "Walt", "Xena",
	"Yara", "Zoe", "Aaron", "Beth", "Chris", "Diana", "Evan", "Fiona",
	"George", "Hannah", "Ian", "Julia", "Karl", "Lisa", "Mark", "Nina",
	"Oliver", "Paula", "Rex", "Sara", "Tom", "Uma", "Victor", "Wendy",
	"Xavier", "Yvonne",
}

var lastNames = []string{
	"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller",
	"Davis", "Wilson", "Taylor", "Anderson", "Thomas", "Jackson", "White",
	"Harris", "Martin", "Thompson", "Young", "Allen", "King", "Wright",
	"Scott", "Green", "Baker", "Adams", "Nelson", "Carter", "Mitchell",
	"Perez", "Roberts", "Turner", "Phillips", "Campbell", "Parker", "Evans",
	"Edwards", "Collins", "Stewart", "Sanchez", "Morris", "Rogers", "Reed",
	"Cook", "Morgan", "Bell", "Murphy", "Bailey", "Rivera", "Cooper", "Cox",
}

// Built-in name-prefix and name-suffix vocabularies for the
// $faker.namePrefix / $faker.nameSuffix functions (en-US, per
// SPECIFICATION.md:771-772).
var namePrefixes = []string{
	"Mr.", "Mrs.", "Ms.", "Dr.", "Prof.",
}

var nameSuffixes = []string{
	"Jr.", "Sr.", "II", "III", "IV", "PhD", "MD", "Esq.",
}

// Built-in city pool for $faker.city. en-US only in M13.
var cities = []string{
	"Springfield", "Madison", "Franklin", "Greenville", "Bristol",
	"Clinton", "Salem", "Georgetown", "Fairview", "Arlington",
	"Burlington", "Centerville", "Manchester", "Marion", "Oxford",
	"Riverside", "Auburn", "Dover", "Hudson", "Newport",
	"Ashland", "Cleveland", "Lancaster", "Milton", "Winchester",
	"Chester", "Kingston", "Lexington", "Mount Vernon", "Troy",
}

// Built-in street-name and street-suffix pools for $faker.streetName,
// $faker.street, and $faker.address. Composed as
// "<streetNames[i]> <streetSuffixes[j]>" — two independent draws.
var streetNames = []string{
	"Main", "Oak", "Pine", "Maple", "Cedar",
	"Elm", "Washington", "Lake", "Hill", "Park",
	"Spring", "North", "South", "Church", "High",
	"River", "Sunset", "Walnut", "Lincoln", "Highland",
	"Forest", "Meadow", "Ridge", "Valley", "Birch",
	"Willow", "Chestnut", "Pleasant", "Cherry", "Madison",
}

var streetSuffixes = []string{
	"St", "Ave", "Blvd", "Rd", "Dr",
	"Ln", "Way", "Ct", "Pl", "Ter",
}

// Built-in US state name pool and matching 2-letter ISO 3166-2:US codes.
// stateNames[i] and stateAbbrs[i] MUST refer to the same state — this
// invariant is asserted in TestFakerLocation_PoolAlignment.
var stateNames = []string{
	"Alabama", "Alaska", "Arizona", "Arkansas", "California",
	"Colorado", "Connecticut", "Delaware", "Florida", "Georgia",
	"Hawaii", "Idaho", "Illinois", "Indiana", "Iowa",
	"Kansas", "Kentucky", "Louisiana", "Maine", "Maryland",
	"Massachusetts", "Michigan", "Minnesota", "Mississippi", "Missouri",
	"Montana", "Nebraska", "Nevada", "New Hampshire", "New Jersey",
	"New Mexico", "New York", "North Carolina", "North Dakota", "Ohio",
	"Oklahoma", "Oregon", "Pennsylvania", "Rhode Island", "South Carolina",
	"South Dakota", "Tennessee", "Texas", "Utah", "Vermont",
	"Virginia", "Washington", "West Virginia", "Wisconsin", "Wyoming",
}

var stateAbbrs = []string{
	"AL", "AK", "AZ", "AR", "CA",
	"CO", "CT", "DE", "FL", "GA",
	"HI", "ID", "IL", "IN", "IA",
	"KS", "KY", "LA", "ME", "MD",
	"MA", "MI", "MN", "MS", "MO",
	"MT", "NE", "NV", "NH", "NJ",
	"NM", "NY", "NC", "ND", "OH",
	"OK", "OR", "PA", "RI", "SC",
	"SD", "TN", "TX", "UT", "VT",
	"VA", "WA", "WV", "WI", "WY",
}

// Built-in country name pool and matching ISO 3166-1 alpha-2 codes.
// Same 1:1 parallel-slice contract as stateNames / stateAbbrs.
var countryNames = []string{
	"United States", "United Kingdom", "Canada", "Australia", "Germany",
	"France", "Italy", "Spain", "Netherlands", "Belgium",
	"Switzerland", "Austria", "Sweden", "Norway", "Denmark",
	"Finland", "Ireland", "Portugal", "Poland", "Greece",
	"Japan", "China", "South Korea", "India", "Singapore",
	"Brazil", "Mexico", "Argentina", "South Africa", "New Zealand",
}

var countryCodes = []string{
	"US", "GB", "CA", "AU", "DE",
	"FR", "IT", "ES", "NL", "BE",
	"CH", "AT", "SE", "NO", "DK",
	"FI", "IE", "PT", "PL", "GR",
	"JP", "CN", "KR", "IN", "SG",
	"BR", "MX", "AR", "ZA", "NZ",
}

// Built-in company-name pool for $faker.company (en-US only in M13).
// Curated to be plausibly real-sounding without using actual trademarks
// outside the established "Acme" naming convention from the spec example.
var companies = []string{
	"Acme Corporation", "Globex Industries", "Initech", "Umbrella Holdings",
	"Stark Solutions", "Wayne Enterprises", "Cyberdyne Systems", "Massive Dynamic",
	"Hooli", "Pied Piper", "Aperture Innovations", "Black Mesa Research",
	"Soylent Foods", "Nakatomi Trading", "Tyrell Robotics", "Weyland Logistics",
	"Sirius Cybernetics", "Oscorp Labs", "Stark-Wayne Capital", "Vandelay Imports",
	"Pendant Publishing", "InGen Bio", "Rekall Travel", "MomCorp",
	"Veridian Dynamics", "Buy n Large", "Wonka Industries", "Spacely Sprockets",
	"Cogswell Cogs", "Dunder Mifflin",
}

// Built-in canonical company-suffix set for $faker.companySuffix.
// Per SPECIFICATION.md:796, this is a closed set of exactly five entries.
// TestFakerCompany_SuffixSet asserts membership with the same five strings.
var companySuffixes = []string{
	"Inc.", "LLC", "Corp.", "Ltd.", "Co.",
}

// Built-in job-title pool for $faker.jobTitle.
var jobTitles = []string{
	"Senior Developer", "Software Engineer", "Product Manager",
	"Engineering Manager", "Designer", "UX Researcher",
	"Data Scientist", "Site Reliability Engineer", "DevOps Engineer",
	"Quality Engineer", "Technical Writer", "Solutions Architect",
	"Account Executive", "Customer Success Manager", "Marketing Manager",
	"Operations Analyst", "Financial Analyst", "Project Manager",
	"Director of Engineering", "VP of Product", "Chief Technology Officer",
	"Chief Executive Officer", "Chief Operating Officer", "Chief Financial Officer",
	"Junior Developer", "Staff Engineer", "Principal Engineer",
	"Engineering Director", "Frontend Developer", "Backend Developer",
}

// Built-in department-name pool for $faker.department.
var departments = []string{
	"Engineering", "Product", "Design", "Marketing", "Sales",
	"Operations", "Finance", "Legal", "Human Resources", "Customer Support",
	"Research and Development", "Quality Assurance", "Information Technology",
	"Business Development", "Data and Analytics", "Security", "Infrastructure",
	"Platform", "Mobile", "Web", "Backend", "Frontend",
	"Strategy", "Communications", "Procurement", "Logistics",
	"Compliance", "Internal Audit", "Risk Management", "Facilities",
}

// Built-in pools for $faker.catchPhrase, composed as:
// "<adjective> <noun> <gerund>" (one space separator, three independent draws).
// Per task YAML behavior 5 — this ordering is what behavior tests assert.
// Pool entries are chosen so the composed phrase reads as buzzword-shaped
// regardless of which random combination is drawn.
var catchPhraseAdjectives = []string{
	"Synergized", "Optimized", "Streamlined", "Scalable", "Robust",
	"Innovative", "Disruptive", "Strategic", "Holistic", "Dynamic",
	"Proactive", "Cutting-edge", "Cross-platform", "Best-of-breed", "Next-gen",
	"Mission-critical", "Turnkey", "Customer-centric", "Data-driven", "Cloud-native",
	"Enterprise-grade", "Frictionless", "Seamless", "End-to-end", "Real-time",
	"Bleeding-edge", "World-class", "Hyper-converged", "Visionary", "Transformative",
}

var catchPhraseNouns = []string{
	"leverage", "paradigm", "synergy", "bandwidth", "ecosystem",
	"architecture", "framework", "methodology", "infrastructure", "workflow",
	"platform", "interface", "pipeline", "integration", "deployment",
	"experience", "engagement", "alignment", "convergence", "scalability",
	"agility", "throughput", "velocity", "outcomes", "deliverables",
	"stakeholders", "value-add", "core-competencies", "best-practices", "key-drivers",
}

var catchPhraseGerunds = []string{
	"engaging", "scaling", "transforming", "empowering", "enabling",
	"accelerating", "optimizing", "streamlining", "innovating", "disrupting",
	"delivering", "iterating", "pivoting", "operationalizing", "monetizing",
	"amplifying", "harnessing", "unlocking", "orchestrating", "modernizing",
	"future-proofing", "evolving", "scaling-up", "rightsizing", "right-shoring",
	"onboarding", "synthesizing", "incentivizing", "ideating", "evangelizing",
}

// Built-in domain-name root pool for $faker.domain and $faker.url.
// Composed as "<root>.<tld>" — both pools are independent draws.
// Entries are lowercase ASCII matching ^[a-z0-9-]+$ so the composed
// domain matches behavior 2's regex ^[a-z0-9-]+\.[a-z]{2,}$.
var domainNameRoots = []string{
	"example", "acme", "widgets", "globex", "initech", "umbrella",
	"stark", "wayne", "hooli", "piedpiper", "aperture", "blackmesa",
	"soylent", "nakatomi", "tyrell", "weyland", "sirius", "oscorp",
	"vandelay", "pendant", "ingen", "rekall", "momcorp", "veridian",
	"wonka", "spacely", "cogswell", "dunder", "buynlarge", "rocketlabs",
}

// Built-in top-level domain suffix pool for $faker.domainSuffix,
// $faker.domain, and $faker.url. Per behavior 3, returned without a
// leading dot. Lowercase ASCII matching ^[a-z]{2,}$ so the composed
// domain regex (behavior 2) is satisfied for every shipped entry.
var tldSuffixes = []string{
	"com", "org", "net", "io", "dev", "app",
	"co", "uk", "de", "jp", "eu", "tech",
}

// Built-in URL-path-segment pool for $faker.url. Each entry is a
// single lowercase token (no slashes, no spaces) so the composed URL
// matches behavior 1's regex ^https?://[^/]+/[^\s]*$ for every
// permutation.
var urlPathSegments = []string{
	"home", "about", "contact", "products", "services", "blog",
	"support", "docs", "pricing", "login", "signup", "dashboard",
}

// Built-in user-agent string pool for $faker.userAgent. Six entries
// covering Chrome / Firefox / Safari / Edge on Windows, macOS, and
// Linux. Every entry begins with "Mozilla/5.0" per current browser
// convention (SPECIFICATION.md:811).
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
	"Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7; rv:121.0) Gecko/20100101 Firefox/121.0",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
}

// Built-in CSS color-name pool for $faker.color. Per
// SPECIFICATION.md:812, $faker.color returns a CSS color name —
// distinct from $randomColor, which returns a #RRGGBB hex string.
// Both registrations co-exist per M13 Open Decision #3. Entries
// are lowercase ASCII keywords from CSS Color Module Level 1
// (the canonical 17 named-color keywords) plus 7 widely-recognised
// CSS3 keywords.
var cssColorNames = []string{
	"aqua", "black", "blue", "fuchsia", "gray", "green",
	"lime", "maroon", "navy", "olive", "purple", "red",
	"silver", "teal", "white", "yellow", "orange",
	"crimson", "coral", "gold", "salmon", "tan", "khaki", "violet",
}

// Built-in lorem-ipsum word pool for the $faker content-data family
// ($faker.word, .words, .sentence, .paragraph, .text). Per
// SPECIFICATION.md:815-823 (en-US only; locale deferred per M13 Open
// Decision #1). Entries are lowercase ASCII matching ^[a-z]+$ — the
// invariant pinned by TestFakerContent_PoolShape and the contract
// that lets $faker.word emit a clean lowercase token without further
// transformation.
//
// Pool size is ~250 entries to give $faker.text and $faker.paragraph
// enough entropy that two seeded draws don't read like exact repeats,
// while keeping the binary footprint negligible. Curated lorem ipsum
// (the canonical Lorem Ipsum extended Latin word list).
var loremWords = []string{
	// Core lorem ipsum (classical passage — all entries unique, lowercase ASCII).
	"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing", "elit",
	"sed", "eiusmod", "tempor", "incididunt", "labore", "magna", "aliqua", "enim",
	"minim", "veniam", "quis", "nostrud", "exercitation", "ullamco", "laboris",
	"nisi", "aliquip", "commodo", "consequat", "duis", "aute", "irure", "reprehenderit",
	"voluptate", "velit", "esse", "cillum", "dolore", "eu", "fugiat", "nulla",
	"pariatur", "excepteur", "sint", "occaecat", "cupidatat", "non", "proident",
	"sunt", "culpa", "officia", "deserunt", "mollit", "anim", "est", "laborum",
	// Extended lorem ipsum vocabulary (de Finibus Bonorum et Malorum passage).
	"at", "vero", "eos", "accusamus", "iusto", "odio", "dignissimos", "ducimus",
	"blanditiis", "praesentium", "voluptatum", "deleniti", "corrupti", "quos",
	"dolores", "quas", "molestias", "excepturi", "occaecati", "cupiditate",
	"provident", "similique", "qui", "impedit", "minima", "perspiciatis",
	"unde", "omnis", "iste", "natus", "error", "voluptatem", "accusantium",
	"doloremque", "laudantium", "totam", "aperiam", "eaque", "ipsa", "quae",
	"inventore", "veritatis", "quasi", "architecto", "beatae", "vitae", "dicta",
	"explicabo", "aspernatur", "aut", "odit", "fugit", "consequuntur", "magni",
	"dolorum", "ratione", "sequi", "nesciunt", "neque", "porro", "quisquam",
	"dolorem", "adipisci", "numquam", "eius", "modi", "tempora", "incidunt",
	"magnam", "quaerat", "suscipit", "corporis", "facilis", "expedita",
	"distinctio", "libero", "tempore", "cumque", "soluta", "nobis", "optio",
	"nihil", "quo", "minus", "maxime", "placeat", "facere", "possimus",
	"repellendus", "temporibus", "autem", "officiis", "debitis", "rerum",
	"necessitatibus", "saepe", "eveniet", "voluptates", "repudiandae",
	"recusandae", "itaque", "earum", "sapiente", "delectus", "reiciendis",
	"molestiae", "maiores", "alias", "perferendis", "doloribus", "asperiores",
	"repellat", "harum", "mollitia", "animi", "id", "voluptatibus", "rem",
	"illo", "quibusdam", "eum", "iure",
	// Additional classical Latin vocabulary for ~250 total unique words.
	"fuga", "laudatur", "optimus", "virtus", "amicitia", "civitas", "natura",
	"animus", "sensus", "mens", "vita", "mors", "tempus", "locus", "causa",
	"forma", "genus", "species", "modus", "finis", "ordo", "pars", "totus",
	"unus", "duo", "tres", "primus", "secundus", "tertius", "novus", "antiquus",
	"parvus", "multus", "bonus", "malus", "verus", "falsus", "certus", "dubius",
	"clarus", "obscurus", "pater", "mater", "filius", "frater", "soror", "amicus",
	"hostis", "rex", "populus", "urbs", "terra", "caelum", "mare", "flumen",
	"mons", "silva", "campus", "via", "porta", "domus", "templum", "forum",
	"bellum", "pax", "victoria", "gloria", "honor", "fama", "nomen", "verbum",
}

// Built-in IANA timezone pool for $faker.timezone. Every entry must be
// loadable via time.LoadLocation; this is enforced by
// TestRegistry_FakerLocation_TimezonePool_AllValid.
var ianaTimezones = []string{
	"America/New_York", "America/Chicago", "America/Denver", "America/Los_Angeles",
	"America/Anchorage", "America/Phoenix", "America/Toronto", "America/Vancouver",
	"America/Mexico_City", "America/Sao_Paulo", "America/Buenos_Aires",
	"Europe/London", "Europe/Paris", "Europe/Berlin", "Europe/Madrid",
	"Europe/Rome", "Europe/Amsterdam", "Europe/Stockholm", "Europe/Moscow",
	"Asia/Tokyo", "Asia/Shanghai", "Asia/Hong_Kong", "Asia/Singapore",
	"Asia/Seoul", "Asia/Kolkata", "Asia/Dubai",
	"Australia/Sydney", "Australia/Melbourne", "Pacific/Auckland", "UTC",
}

// Built-in currency pools for the M13-007 financial-data family.
// Three parallel slices: currencyCodes[i], currencyNames[i], and
// currencySymbols[i] all refer to the SAME currency. Enforced by
// TestFakerFinancial_CurrencyPoolAlignment. Per SPECIFICATION.md:830-832
// and M13-007 task behavior 5. en-US-aligned defaults; locale-specific
// currency formatting is deferred per M13 Open Decision #1.
var currencyCodes = []string{
	"USD", "EUR", "GBP", "JPY", "CAD", "AUD", "CHF", "CNY",
	"SEK", "NZD", "MXN", "SGD", "HKD", "NOK", "KRW", "TRY",
	"INR", "BRL", "ZAR", "DKK", "PLN", "ILS", "THB", "RUB",
	"AED",
}

var currencyNames = []string{
	"US Dollar", "Euro", "British Pound", "Japanese Yen",
	"Canadian Dollar", "Australian Dollar", "Swiss Franc", "Chinese Yuan",
	"Swedish Krona", "New Zealand Dollar", "Mexican Peso", "Singapore Dollar",
	"Hong Kong Dollar", "Norwegian Krone", "South Korean Won", "Turkish Lira",
	"Indian Rupee", "Brazilian Real", "South African Rand", "Danish Krone",
	"Polish Zloty", "Israeli Shekel", "Thai Baht", "Russian Ruble",
	"UAE Dirham",
}

var currencySymbols = []string{
	"$", "€", "£", "¥", "C$", "A$", "Fr", "¥",
	"kr", "NZ$", "Mex$", "S$", "HK$", "kr", "₩", "₺",
	"₹", "R$", "R", "kr", "zł", "₪", "฿", "₽",
	"د.إ",
}

// Built-in IBAN country pool for $faker.iban. ibanCountries[i] is the
// 2-letter country code; ibanLengths[i] is the canonical IBAN length for
// that country (per SwiftRefData IBAN registry). Enforced by
// TestFakerFinancial_IBANCountriesAlignment.
var ibanCountries = []string{
	"DE", "GB", "FR", "ES", "IT", "NL", "BE", "CH",
}

var ibanLengths = []int{
	22, 22, 27, 24, 27, 18, 16, 21,
}

// ibanBBANChars is the IBAN BBAN alphabet (uppercase letters + digits).
const ibanBBANChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Built-in file base-name pool for $faker.fileName. Lowercase ASCII
// tokens matching ^[a-z0-9_-]+$ — composed with a random extension
// drawn from fileExtensions to produce ^[a-z0-9_-]+\.[a-z0-9]+$.
// en-US only in M13; locale-specific filename conventions are deferred
// per M13 Open Decision #1.
var fileBaseNames = []string{
	"document", "report", "invoice", "image", "photo",
	"screenshot", "presentation", "spreadsheet", "archive", "backup",
	"memo", "notes", "draft", "summary", "manifest",
	"config", "manual", "guide", "readme", "changelog",
}

// Built-in file-extension pool for $faker.fileExtension and the
// suffix of $faker.fileName. Each entry is a 2-to-4-character
// lowercase ASCII string with no leading dot, matching
// ^[a-z0-9]{2,4}$ per behavior 2. Parallel-aligned 1:1 with
// fileMimeTypes — fileExtensions[i] and fileMimeTypes[i] refer to
// the SAME format. Enforced by TestFakerFile_PoolAlignment.
var fileExtensions = []string{
	"pdf", "jpg", "png", "mp4", "txt",
	"csv", "json", "html", "xml", "zip",
	"doc", "xls", "mp3", "gif", "svg",
}

// Built-in MIME-type pool for $faker.mimeType. Each entry matches
// ^[a-z]+/[a-z0-9.+-]+$ per behavior 3. Parallel-aligned 1:1 with
// fileExtensions. Format choices follow the IANA Media Types registry
// for the matching extension (e.g. .pdf → application/pdf, .jpg →
// image/jpeg, .json → application/json).
var fileMimeTypes = []string{
	"application/pdf", "image/jpeg", "image/png", "video/mp4", "text/plain",
	"text/csv", "application/json", "text/html", "application/xml", "application/zip",
	"application/msword", "application/vnd.ms-excel", "audio/mpeg", "image/gif", "image/svg+xml",
}

// luhnCheckDigit returns the Luhn checksum digit (ASCII '0'..'9') for the
// digits-only prefix. The prefix must contain only ASCII digit bytes; this
// is the caller's contract (the only call site is the credit-card generator
// that builds the prefix from intn(rng, 10)+'0').
//
// Algorithm: walk right-to-left over the prefix, doubling every other digit
// starting from the rightmost prefix byte (which becomes position 2 from the
// right of the final 16-digit number once the check digit is appended). The
// check digit is chosen so the total sum (including the check digit at
// position 1) is divisible by 10.
func luhnCheckDigit(prefix []byte) byte {
	sum := 0
	n := len(prefix)
	for i := 0; i < n; i++ {
		d := int(prefix[i] - '0')
		// prefix[i] sits at position (n-i) from the right of the prefix,
		// which becomes position (n-i+1) from the right of the final number.
		// We double when that position is even, i.e. when (n-i) is odd.
		if (n-i)%2 == 1 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return byte((10-sum%10)%10) + '0'
}

// isLuhnValid reports whether s is a non-empty digit-only string that
// satisfies the Luhn checksum (total sum of doubled-and-folded digits
// divisible by 10). Used by TestRegistry_FakerFinancial to validate
// generated card numbers.
func isLuhnValid(s string) bool {
	if s == "" {
		return false
	}
	sum := 0
	n := len(s)
	for i := 0; i < n; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		// s[i] is at position (n-i) from the right. Double when that position
		// is even, i.e. when (n-i) is even.
		if (n-i)%2 == 0 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}

// ibanCheckDigits computes the two ISO 13616 check digits for an IBAN with
// the given country code and BBAN. The result is a zero-padded two-digit
// string in "00".."98".
//
// Algorithm (per ISO 13616-1):
//  1. Rearrange: bban + country + "00"
//  2. Convert each letter to its two-digit numeric form: A=10, B=11, ..., Z=35
//  3. Compute the rearranged numeral mod 97 via an iterative per-character
//     algorithm (no big.Int needed; each step rem = (rem*10 + digit) % 97
//     keeps rem < 97 and avoids int64 overflow).
//  4. Check digits = 98 − (numeral mod 97), formatted as %02d.
func ibanCheckDigits(country, bban string) string {
	var b strings.Builder
	b.Grow(len(bban) + len(country) + 2)
	b.WriteString(bban)
	b.WriteString(country)
	b.WriteString("00")
	rearranged := b.String()

	var rem int64
	for i := 0; i < len(rearranged); i++ {
		c := rearranged[i]
		if c >= '0' && c <= '9' {
			rem = (rem*10 + int64(c-'0')) % 97
		} else if c >= 'A' && c <= 'Z' {
			// Two-digit numeric form: A=10..Z=35. Process both decimal digits.
			v := int(c-'A') + 10
			rem = (rem*10 + int64(v/10)) % 97
			rem = (rem*10 + int64(v%10)) % 97
		}
		// Other characters never appear — caller's contract.
	}
	check := 98 - int(rem)
	return fmt.Sprintf("%02d", check)
}

// isIBANValid reports whether s is a structurally valid IBAN whose mod-97
// checksum equals 1 (the ISO 13616 validity criterion). Length is not
// asserted against any country-specific table — only the generic shape
// (ASCII letters + digits, length >= 5) and the mod-97 result.
func isIBANValid(s string) bool {
	if len(s) < 5 {
		return false
	}
	// Move the first 4 characters (country + check digits) to the end.
	rearranged := s[4:] + s[:4]
	var rem int64
	for i := 0; i < len(rearranged); i++ {
		c := rearranged[i]
		if c >= '0' && c <= '9' {
			rem = (rem*10 + int64(c-'0')) % 97
		} else if c >= 'A' && c <= 'Z' {
			v := int(c-'A') + 10
			rem = (rem*10 + int64(v/10)) % 97
			rem = (rem*10 + int64(v%10)) % 97
		} else {
			return false
		}
	}
	return rem == 1
}
