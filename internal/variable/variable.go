// Package variable provides variable interpolation with cycle detection and depth limiting.
package variable

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	apierrors "github.com/weiqigod/curlew/internal/errors"
)

// ErrUnknownSecret is returned when {{secrets.NAME}} references an alias that
// has not been registered on the scope via WithSecrets.
var ErrUnknownSecret = errors.New("unknown secret alias")

// MaxDepth is the maximum variable reference chain depth allowed.
const MaxDepth = 10

// Sentinel errors for variable resolution failures.
var (
	ErrCircularReference = errors.New("circular variable reference")
	ErrDepthExceeded     = errors.New("variable interpolation depth limit exceeded")
	ErrUndefinedVariable = errors.New("undefined variable")
	ErrInvalidVarFlag    = errors.New("invalid --var flag format")
	ErrInvalidEnvVarFlag = errors.New("invalid --env-var flag format")
	ErrEnvVarNotSet      = errors.New("environment variable not set")
)

// ParseVarFlag parses a "--var" argument value of the form "key=value".
// Splits on first "="; key must be non-empty. Value may be empty.
func ParseVarFlag(s string) (key, value string, err error) {
	idx := strings.Index(s, "=")
	if idx < 0 {
		return "", "", fmt.Errorf("%w: expected key=value, got %q", ErrInvalidVarFlag, s)
	}
	key = s[:idx]
	if key == "" {
		return "", "", fmt.Errorf("%w: empty key in %q", ErrInvalidVarFlag, s)
	}
	return key, s[idx+1:], nil
}

// ParseEnvVarFlag parses a "--env-var" argument value.
// Supports two forms:
//
//	"VAR_NAME"         -> imports lookupEnv("VAR_NAME") as "VAR_NAME"
//	"VAR_NAME=$OS_VAR" -> imports lookupEnv("OS_VAR") as "VAR_NAME"
//
// Returns the variable name and its resolved value.
// Returns ErrEnvVarNotSet if the environment variable is not set.
func ParseEnvVarFlag(s string, lookupEnv func(string) (string, bool)) (key, value string, err error) {
	if s == "" {
		return "", "", fmt.Errorf("%w: expected VAR_NAME or VAR_NAME=$OS_VAR, got empty string", ErrInvalidEnvVarFlag)
	}

	envName := s
	varName := s
	if idx := strings.Index(s, "="); idx >= 0 {
		varName = s[:idx]
		envName = s[idx+1:]
		// Strip leading $ from OS var name
		envName = strings.TrimPrefix(envName, "$")
		if varName == "" {
			return "", "", fmt.Errorf("%w: empty variable name in %q", ErrInvalidEnvVarFlag, s)
		}
		if envName == "" {
			return "", "", fmt.Errorf("%w: empty environment variable name in %q", ErrInvalidEnvVarFlag, s)
		}
	}

	val, ok := lookupEnv(envName)
	if !ok {
		return "", "", fmt.Errorf("%w: %q is not set in the environment", ErrEnvVarNotSet, envName)
	}
	return varName, val, nil
}

var (
	varPattern = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)
	// dynPattern matches {{$funcName}} (legacy, no parens) or
	// {{$funcName(<rawArgList>)}}. Capture groups:
	//
	//	[1] funcName — may contain dotted namespaces (e.g. "faker.firstName"),
	//	    with each dot-separated segment satisfying
	//	    [a-zA-Z][a-zA-Z0-9_]*. Consecutive dots, leading dots, and
	//	    trailing dots are rejected (the entire match fails).
	//	[2] rawArgList (empty string when there are no parens at all, or the
	//	    literal characters between the outer parens).
	//
	// The raw arg list uses a non-greedy .*? so that:
	//   - args may contain {{var}} references (nested braces are fine)
	//   - the match stops at the first `)}}` sequence
	//
	// NOTE: a literal `)}}` sequence inside a single-quoted arg would
	// terminate the match early; such sequences are not expected in practice.
	// Actual arg parsing (quotes, escapes, commas) is handled by parseDynArgs.
	// The captured funcName (including any dots) is used verbatim as the
	// Registry.Evaluate lookup key — see internal/variable/dynamic.go.
	dynPattern     = regexp.MustCompile(`\{\{\$([a-zA-Z][a-zA-Z0-9_]*(?:\.[a-zA-Z][a-zA-Z0-9_]*)*)(?:\((.*?)\))?\}\}`)
	secretsPattern = regexp.MustCompile(`\{\{secrets\.([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)
)

// parseDynArgs scans a comma-separated list of single-quoted string literals
// from raw, honouring \' and \\ escapes inside quotes. Whitespace around
// commas is ignored. Returns the parsed arg slice or a structured
// CategoryInput error pointing at the offending position.
//
// raw is the substring between ( and ) of {{$fn(raw)}}. Empty raw (including
// all-whitespace) means "no args"; the function returns nil, nil.
func parseDynArgs(funcName, raw string) ([]string, error) {
	dynArgsInputErr := func(msg string) error {
		return &apierrors.Structured{
			Category: apierrors.CategoryInput,
			Code:     "DYNFN_ARGS",
			Message:  fmt.Sprintf("$%s: %s", funcName, msg),
			Hint:     fmt.Sprintf("Arguments must be single-quoted string literals, e.g. $%s('value').", funcName),
		}
	}

	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, nil
	}

	var args []string
	i := 0
	for i < len(s) {
		// Skip whitespace.
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}

		if s[i] != '\'' {
			return nil, dynArgsInputErr(fmt.Sprintf("expected single-quoted argument at position %d, got %q", i, string(s[i])))
		}
		i++ // consume opening quote

		// Scan quoted literal, honouring \' and \\.
		var buf strings.Builder
		for {
			if i >= len(s) {
				return nil, dynArgsInputErr("unterminated single-quoted string")
			}
			ch := s[i]
			if ch == '\\' {
				if i+1 >= len(s) {
					return nil, dynArgsInputErr("unterminated escape sequence")
				}
				next := s[i+1]
				switch next {
				case '\'':
					buf.WriteByte('\'')
				case '\\':
					buf.WriteByte('\\')
				default:
					// Pass through other escapes unchanged.
					buf.WriteByte('\\')
					buf.WriteByte(next)
				}
				i += 2
				continue
			}
			if ch == '\'' {
				i++ // consume closing quote
				break
			}
			buf.WriteByte(ch)
			i++
		}
		args = append(args, buf.String())

		// Skip whitespace after closing quote.
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}

		if i >= len(s) {
			break
		}

		// Expect comma separator.
		if s[i] != ',' {
			return nil, dynArgsInputErr(fmt.Sprintf("expected ',' after argument at position %d, got %q", i, string(s[i])))
		}
		i++ // consume comma

		// After consuming a comma there must be another argument.
		// Peek ahead (skip whitespace) to detect trailing comma.
		j := i
		for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
			j++
		}
		if j >= len(s) {
			return nil, dynArgsInputErr("trailing comma in argument list")
		}
	}

	return args, nil
}

// Scope holds a set of named variables that can be resolved and interpolated.
type Scope struct {
	vars             map[string]string
	resolved         map[string]string
	registry         *Registry         // nil = no dynamic functions
	funcCache        map[string]string // per-request memoization; nil outside a request
	secrets          map[string]string // nil when no team template is active
	runtimeSensitive *SensitiveSet     // nil = no runtime tracking; mutated by dynamic-fn dispatch when a credential-bearing arg resolves from a sensitive source
}

// WithSecrets returns a shallow copy of the scope with the given secrets map
// attached. Passing nil or an empty map returns the receiver unchanged.
// Secrets are resolved by the pre-pass in Interpolate via {{secrets.ALIAS}}.
func (s *Scope) WithSecrets(secrets map[string]string) *Scope {
	if len(secrets) == 0 {
		return s
	}
	cp := *s
	cp.secrets = secrets
	return &cp
}

// HasSecretsNamespace reports whether input contains any {{secrets.X}} tokens.
// Used by the runner to decide whether --env is required.
func HasSecretsNamespace(input string) bool {
	return secretsPattern.MatchString(input)
}

// SecretReferences returns the sorted unique alias names referenced via
// {{secrets.ALIAS}} in input. Returns nil when there are no references.
func SecretReferences(input string) []string {
	matches := secretsPattern.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		seen[m[1]] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for alias := range seen {
		out = append(out, alias)
	}
	sort.Strings(out)
	return out
}

// WithDynamic returns a shallow copy of the scope with the given registry attached.
func (s *Scope) WithDynamic(r *Registry) *Scope {
	cp := *s
	cp.registry = r
	return &cp
}

// WithRuntimeSensitive returns a shallow copy of the scope with the given
// SensitiveSet attached as the *runtime* set — the set that the dynamic-
// function evaluation path (Interpolate) mutates via AddValue when a
// credential-bearing argument (e.g. the key of $hmacSha256) resolves from
// a sensitive source. The runner allocates one such set per run and
// retrieves it from RunSummary.RuntimeSensitive. Passing nil normalises the
// field to nil — callers that want no runtime tracking should simply not
// call this method.
//
// The set is shared across Snapshots (concurrent workers register against
// the same set). AddValue is safe for concurrent calls; reads should happen
// only after the run completes.
func (s *Scope) WithRuntimeSensitive(set *SensitiveSet) *Scope {
	cp := *s
	cp.runtimeSensitive = set
	return &cp
}

// RuntimeSensitiveSet returns the runtime sensitive set attached to this scope,
// or nil when no runtime tracking is configured. Used by the CEL if: gate to
// route sensitive values discovered during expression evaluation to the same set
// that dynamic function dispatch populates.
func (s *Scope) RuntimeSensitiveSet() *SensitiveSet {
	return s.runtimeSensitive
}

// BeginRequest initialises a fresh per-request function value cache.
func (s *Scope) BeginRequest() {
	s.funcCache = make(map[string]string)
}

// EndRequest clears the per-request function cache.
func (s *Scope) EndRequest() {
	s.funcCache = nil
}

// NewScope creates a Scope from a variables map.
func NewScope(vars map[string]string) *Scope {
	return &Scope{vars: vars}
}

// Resolve resolves all internal variable references, detecting circular
// references and enforcing depth limits. Must be called before Interpolate.
func (s *Scope) Resolve() error {
	s.resolved = make(map[string]string, len(s.vars))
	for name := range s.vars {
		if _, ok := s.resolved[name]; ok {
			continue
		}
		if _, err := s.resolveVar(name, nil, 0); err != nil {
			return err
		}
	}
	return nil
}

// Interpolate replaces all {{var}} and {{$func}} placeholders in the input string
// with their resolved values. Returns error if any variable is undefined or an
// unknown dynamic function is referenced.
func (s *Scope) Interpolate(input string) (string, error) {
	// Pre-scan: if there are dynamic function calls and a runtimeSensitive
	// set is attached, build a map of per-call sensitive-arg flags from the
	// original (pre-secrets-substitution) input. This must happen before Pass 0
	// replaces {{secrets.X}} tokens, which are intrinsically sensitive.
	//
	// sensFlagsByCallIdx maps call-index → []bool (one bool per arg).
	// This handles the case where {{secrets.ALIAS}} appears inside a dyn-fn arg
	// and is replaced by Pass 0 before we can inspect it in Pass 1.
	var sensFlagsByCallIdx map[int][]bool
	if s.runtimeSensitive != nil && dynPattern.MatchString(input) {
		allMatches := dynPattern.FindAllStringSubmatch(input, -1)
		sensFlagsByCallIdx = make(map[int][]bool, len(allMatches))
		for callIdx, sub := range allMatches {
			funcName := sub[1]
			rawArgs := sub[2]
			argLits, _ := parseDynArgs(funcName, rawArgs)
			flags := make([]bool, len(argLits))
			for i, lit := range argLits {
				if secretsPattern.MatchString(lit) {
					flags[i] = true
					continue
				}
				for _, ref := range findVarRefs(lit) {
					if IsSensitiveName(ref) {
						flags[i] = true
						break
					}
				}
			}
			sensFlagsByCallIdx[callIdx] = flags
		}
	}

	// Pass 0: resolve {{secrets.ALIAS}} via the dedicated secrets namespace.
	// This runs before the dynamic and variable passes so that secrets are never
	// confused with regular variables.
	if secretsPattern.MatchString(input) {
		var retErr error
		input = secretsPattern.ReplaceAllStringFunc(input, func(match string) string {
			if retErr != nil {
				return match
			}
			// alias is the part between "{{secrets." and "}}"
			alias := match[len("{{secrets.") : len(match)-2]
			val, ok := s.secrets[alias]
			if !ok {
				retErr = &apierrors.Structured{
					Category: apierrors.CategoryConfig,
					Message:  fmt.Sprintf("unknown secret alias %q", alias),
					Hint:     "Check team_secrets.vault_configs.<env>.keys in your shared vault template",
					Inner:    ErrUnknownSecret,
				}
				return match
			}
			return val
		})
		if retErr != nil {
			return "", retErr
		}
	}

	// Pass 1: resolve {{$funcName}} or {{$funcName(args...)}} dynamic patterns.
	if dynPattern.MatchString(input) {
		var retErr error
		callIdx := 0
		result := dynPattern.ReplaceAllStringFunc(input, func(match string) string {
			if retErr != nil {
				return match
			}
			thisCallIdx := callIdx
			callIdx++

			sub := dynPattern.FindStringSubmatch(match)
			funcName := sub[1]
			rawArgs := sub[2] // empty string when there are no parens at all

			// Explicit variable named "$funcName" wins for the legacy no-args
			// form only. A parenthesized call {{$fn(...)}} is identity-distinct
			// from any $fn variable binding because the args are part of the call.
			if rawArgs == "" && !strings.Contains(match, "(") {
				if val, ok := s.resolved["$"+funcName]; ok {
					return val
				}
			}
			if s.registry == nil {
				// No registry: leave the pattern as-is.
				return match
			}

			// Parse the literal arg list, then recursively interpolate each arg
			// so that {{$base64('{{secret}}')}} resolves the inner var first.
			argLits, err := parseDynArgs(funcName, rawArgs)
			if err != nil {
				retErr = err
				return match
			}
			resolvedArgs := make([]string, len(argLits))
			for i, lit := range argLits {
				v, ierr := s.Interpolate(lit)
				if ierr != nil {
					retErr = ierr
					return match
				}
				resolvedArgs[i] = v
			}

			cache := s.funcCache
			if cache == nil {
				cache = make(map[string]string)
			}
			val, err := s.registry.Evaluate(funcName, resolvedArgs, cache)
			if err != nil {
				retErr = err
				return match
			}

			// Auto-mark the configured sensitive-arg position when it
			// originated from a sensitive source. No-op when the registry
			// has no sensitiveArgIdx entry for this function, when this
			// scope has no runtimeSensitive set attached, or when the arg
			// resolved from a non-sensitive (or literal) source.
			if s.runtimeSensitive != nil {
				if idx, ok := s.registry.SensitiveArgIndex(funcName); ok {
					flags := sensFlagsByCallIdx[thisCallIdx]
					if idx < len(flags) && flags[idx] && idx < len(resolvedArgs) {
						s.runtimeSensitive.AddValue(resolvedArgs[idx])
					}
				}
				// Value-side hook: mark the produced value as sensitive when the
				// function is registered as auto-sensitive (e.g. $faker.ssn).
				if s.registry.IsSensitiveReturn(funcName) {
					s.runtimeSensitive.AddValue(val)
				}
			}
			return val
		})
		if retErr != nil {
			return "", retErr
		}
		input = result
	}

	// Pass 2: resolve {{varName}} regular variables.
	var retErr error
	result := varPattern.ReplaceAllStringFunc(input, func(match string) string {
		if retErr != nil {
			return match
		}
		name := match[2 : len(match)-2]
		val, ok := s.resolved[name]
		if !ok {
			retErr = &apierrors.Structured{
				Category: apierrors.CategoryInput,
				Code:     "VAR_UNDEFINED",
				Message:  fmt.Sprintf("undefined variable %q", name),
				Hint:     "Define the variable in the environment file, pass it via --var NAME=VALUE, or add a default in the collection.",
				Inner:    ErrUndefinedVariable,
			}
			return match
		}
		return val
	})
	if retErr != nil {
		return "", retErr
	}
	return result, nil
}

// InterpolateMap replaces all {{var}} placeholders in map values.
// Returns a new map with interpolated values.
func (s *Scope) InterpolateMap(m map[string]string) (map[string]string, error) {
	if m == nil {
		return nil, nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		val, err := s.Interpolate(v)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", k, err)
		}
		out[k] = val
	}
	return out, nil
}

// InterpolateBody handles body interpolation for string, map[string]interface{}, and []interface{} bodies.
func (s *Scope) InterpolateBody(body any) (any, error) {
	if body == nil {
		return nil, nil
	}
	return s.interpolateValue(body)
}

// Resolved returns a copy of all currently resolved variable values.
func (s *Scope) Resolved() map[string]string {
	out := make(map[string]string, len(s.resolved))
	for k, v := range s.resolved {
		out[k] = v
	}
	return out
}

// Set adds or overrides a variable in the resolved scope.
// Used for extracted variables injected after initial resolution.
func (s *Scope) Set(name, value string) {
	if s.resolved == nil {
		s.resolved = make(map[string]string)
	}
	s.resolved[name] = value
	if s.vars == nil {
		s.vars = make(map[string]string)
	}
	s.vars[name] = value
}

// WithOverrides returns a new Scope that contains all resolved variables
// from the receiver plus the given overrides applied on top. Overrides
// may reference variables from the base scope. The receiver is not mutated.
// Returns the receiver unchanged if overrides is nil or empty.
func (s *Scope) WithOverrides(overrides map[string]string) (*Scope, error) {
	if len(overrides) == 0 {
		return s, nil
	}
	merged := make(map[string]string, len(s.resolved)+len(overrides))
	for k, v := range s.resolved {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	child := NewScope(merged)
	child.registry = s.registry
	child.runtimeSensitive = s.runtimeSensitive
	if err := child.Resolve(); err != nil {
		return nil, err
	}
	return child, nil
}

// Snapshot creates an independent copy of this scope that can be safely
// used from a separate goroutine. The returned scope has its own funcCache
// and resolved map, preventing data races on BeginRequest/EndRequest.
func (s *Scope) Snapshot() *Scope {
	resolved := make(map[string]string, len(s.resolved))
	for k, v := range s.resolved {
		resolved[k] = v
	}
	vars := make(map[string]string, len(s.vars))
	for k, v := range s.vars {
		vars[k] = v
	}
	var secrets map[string]string
	if len(s.secrets) > 0 {
		secrets = make(map[string]string, len(s.secrets))
		for k, v := range s.secrets {
			secrets[k] = v
		}
	}
	return &Scope{
		vars:             vars,
		resolved:         resolved,
		registry:         s.registry,
		secrets:          secrets,
		runtimeSensitive: s.runtimeSensitive,
		// funcCache left nil — will be initialised by BeginRequest()
	}
}

// AvailableVars returns a sorted list of variable names in this scope.
func (s *Scope) AvailableVars() []string {
	names := make([]string, 0, len(s.vars))
	for name := range s.vars {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// resolveVar resolves a single variable, tracking the resolution stack for
// cycle detection and depth limiting.
func (s *Scope) resolveVar(name string, stack []string, depth int) (string, error) {
	if depth >= MaxDepth {
		return "", &apierrors.Structured{
			Category: apierrors.CategoryConfig,
			Message:  fmt.Sprintf("variable interpolation depth limit exceeded (max %d): %s", MaxDepth, strings.Join(append(stack, name), " -> ")),
			Hint:     "Simplify the variable chain to fewer than 10 levels",
			Inner:    ErrDepthExceeded,
		}
	}

	// Cycle detection
	for _, v := range stack {
		if v == name {
			cycle := append(stack, name)
			return "", &apierrors.Structured{
				Category: apierrors.CategoryConfig,
				Message:  fmt.Sprintf("circular variable reference: %s", strings.Join(cycle, " -> ")),
				Hint:     "Remove the circular dependency between these variables",
				Inner:    ErrCircularReference,
			}
		}
	}

	raw, ok := s.vars[name]
	if !ok {
		return "", &apierrors.Structured{
			Category: apierrors.CategoryConfig,
			Message:  fmt.Sprintf("undefined variable %q", name),
			Hint:     fmt.Sprintf("Available variables: %s", strings.Join(s.AvailableVars(), ", ")),
			Inner:    ErrUndefinedVariable,
		}
	}

	// Find all references in the raw value and resolve them
	refs := findVarRefs(raw)
	if len(refs) == 0 {
		s.resolved[name] = raw
		return raw, nil
	}

	newStack := append(stack, name)
	resolved := raw
	for _, ref := range refs {
		val, err := s.resolveVar(ref, newStack, depth+1)
		if err != nil {
			return "", err
		}
		resolved = strings.ReplaceAll(resolved, "{{"+ref+"}}", val)
	}

	s.resolved[name] = resolved
	return resolved, nil
}

// FindReferences returns all static variable names referenced via {{varName}} in s.
// Dynamic function references ({{$funcName}}) are not included.
func FindReferences(s string) []string {
	return findVarRefs(s)
}

// findVarRefs extracts all variable names referenced in a string value.
func findVarRefs(s string) []string {
	matches := varPattern.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	refs := make([]string, len(matches))
	for i, m := range matches {
		refs[i] = m[1]
	}
	return refs
}

func (s *Scope) interpolateValue(v any) (any, error) {
	switch val := v.(type) {
	case string:
		return s.Interpolate(val)
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, elem := range val {
			resolved, err := s.interpolateValue(elem)
			if err != nil {
				return nil, err
			}
			out[k] = resolved
		}
		return out, nil
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, elem := range val {
			resolved, err := s.interpolateValue(elem)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	default:
		return v, nil
	}
}
