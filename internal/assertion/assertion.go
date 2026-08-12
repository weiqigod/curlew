package assertion

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/weiqigod/curlew/internal/jsonpath"
)

// Result represents the outcome of a single assertion.
//
// Type, Target and Operator together identify what was asserted. They are the
// machine-readable form: Type is a fixed discriminator, and the other two carry
// the parts that vary per assertion. Human-facing output uses Label instead,
// which reassembles them into a readable phrase.
//
// Keeping these separate is load-bearing. Type was previously built as a
// composite ("body $.user.name equals"), which meant no consumer could
// discriminate on it and the published JSON Schema's enum was violated by every
// assertion except status.
type Result struct {
	// Type is the assertion kind and is always one of the constants below.
	Type string
	// Target names what the assertion addressed: a JSONPath for body, a header
	// name for header, a schema file or instance path for schema, and
	// "assertions[N]" for cel. Empty for status and timing, which address the
	// response as a whole.
	Target string
	// Operator is the comparison applied — "equals", "matches", "exists" and so
	// on. Empty when the type implies the comparison (status, timing, schema,
	// cel).
	Operator string
	Expected string // human-readable expected value
	Actual   string // human-readable actual value
	Passed   bool
	// SourceLine is the 1-based line in the request's source file where this
	// assertion is written — the operator key for body and header assertions,
	// the status:/max_duration_ms:/schema: key otherwise. Zero when the
	// assertion has no YAML origin (a runner-synthesised graphql_error, or an
	// input built by a caller that did not supply one).
	SourceLine int
}

// Assertion kinds. These are the complete vocabulary of Result.Type and must
// stay in step with the assertion.result type enum published in
// docs/events-schema/ — internal/output/events holds them to it.
const (
	TypeStatus = "status"
	TypeBody   = "body"
	TypeHeader = "header"
	TypeSchema = "schema"
	TypeTiming = "timing"
	TypeCEL    = "cel"
	// TypeGraphQLError is produced by the runner, not by this package's
	// evaluators, when a GraphQL response carries errors under a failing
	// error_handling mode. It addresses the response as a whole, so it has
	// neither a target nor an operator.
	TypeGraphQLError = "graphql_error"
)

// Label returns the human-readable description of the assertion, e.g.
// "body $.user.name equals". Every human-facing surface (terminal output, HTML
// reports, pr-check messages, the local UI) renders this, and the strings it
// produces are the ones those surfaces have always shown.
func (r Result) Label() string { return Label(r.Type, r.Target, r.Operator) }

// Label builds the human-readable description from the identity triple. It is
// exported so that packages carrying the triple in their own event types can
// render the same string without restating the rules.
func Label(typ, target, operator string) string {
	switch {
	case typ == TypeCEL:
		// CEL is the one kind whose label puts the type last.
		return target + ".cel"
	case target == "" && operator == "":
		return typ
	case operator == "":
		return typ + " " + target
	default:
		return typ + " " + target + " " + operator
	}
}

// with returns a copy of r carrying the given outcome, preserving the identity
// fields. Evaluators build the identity once and use this at each return, so a
// branch cannot accidentally ship a result that has lost its target.
func (r Result) with(expected, actual string, passed bool) Result {
	r.Expected, r.Actual, r.Passed = expected, actual, passed
	return r
}

// Results is a collection of assertion outcomes for one request.
type Results struct {
	Items  []Result
	Passed bool // true if all items passed (or empty)
}

// BodyInput describes a single body assertion to evaluate.
type BodyInput struct {
	Path     string // JSONPath, e.g. "$.data.id"
	Operator string // e.g. "equals", "exists", "matches", "contains", "greater_than"
	Value    any    // expected value (operator-dependent)
	Line     int    // 1-based source line of the operator key; 0 when unknown
}

// HeaderInput describes a single header assertion to evaluate.
type HeaderInput struct {
	Name     string // Header name (matched case-insensitively)
	Operator string // "equals", "exists", "matches"
	Value    string // expected value (for equals/matches)
	Line     int    // 1-based source line of the operator key; 0 when unknown
}

// CheckHeaders evaluates header assertions against response headers.
// Header names are matched case-insensitively per HTTP spec.
// Returns nil if no assertions are defined.
func CheckHeaders(assertions []HeaderInput, headers http.Header) []Result {
	if len(assertions) == 0 {
		return nil
	}

	results := make([]Result, 0, len(assertions))
	for _, a := range assertions {
		results = append(results, evalHeaderAssertion(a, headers))
	}
	return results
}

func evalHeaderAssertion(a HeaderInput, headers http.Header) Result {
	id := Result{Type: TypeHeader, Target: a.Name, Operator: a.Operator, SourceLine: a.Line}

	switch a.Operator {
	case "equals":
		actual := headers.Get(a.Name)
		if headers == nil || len(headers.Values(a.Name)) == 0 {
			return id.with(a.Value, "header not present", false)
		}
		return id.with(a.Value, actual, actual == a.Value)

	case "exists":
		// `exists` takes a boolean: true asserts presence, false asserts
		// absence (CLI_SPECIFICATION §7.2). The false branch was missing, so
		// there was no way to say "this response must not carry Set-Cookie" —
		// an absent header reported "expected exists, got header not present",
		// which is the condition the assertion had asked for.
		present := headers != nil && len(headers.Values(a.Name)) > 0

		if !wantPresent(a.Value) {
			if present {
				return id.with("absent", fmt.Sprintf("present: %s", headers.Get(a.Name)), false)
			}
			return id.with("absent", "absent", true)
		}
		if !present {
			return id.with("exists", "header not present", false)
		}
		return id.with("exists", "exists", true)

	case "matches":
		expected := fmt.Sprintf("matches %s", a.Value)
		re, err := regexp.Compile(a.Value)
		if err != nil {
			return id.with(expected, fmt.Sprintf("invalid regex: %s", err), false)
		}
		actual := headers.Get(a.Name)
		if headers == nil || len(headers.Values(a.Name)) == 0 {
			return id.with(expected, "header not present", false)
		}
		return id.with(expected, actual, re.MatchString(actual))

	default:
		return id.with(a.Value, "unsupported operator", false)
	}
}

// CheckTiming evaluates whether the response duration is within the allowed threshold.
// Returns nil if maxDurationMs is 0 or negative (no assertion).
func CheckTiming(maxDurationMs int, actual time.Duration) *Result {
	if maxDurationMs <= 0 {
		return nil
	}
	passed := actual.Milliseconds() <= int64(maxDurationMs)
	return &Result{
		Type:     TypeTiming,
		Expected: fmt.Sprintf("<= %dms", maxDurationMs),
		Actual:   fmt.Sprintf("%dms", actual.Milliseconds()),
		Passed:   passed,
	}
}

// CheckStatus evaluates whether actual matches any of the expected status codes.
// Returns nil if expected is empty (no assertion to evaluate).
func CheckStatus(expected []int, actual int) *Result {
	if len(expected) == 0 {
		return nil
	}
	id := Result{Type: TypeStatus}
	passed := false
	for _, code := range expected {
		if code == actual {
			passed = true
			break
		}
	}
	r := id.with(formatCodes(expected), fmt.Sprintf("%d", actual), passed)
	return &r
}

// CheckBody evaluates body assertions against the response body bytes.
// Returns nil if no assertions are defined.
func CheckBody(assertions []BodyInput, body []byte) []Result {
	if len(assertions) == 0 {
		return nil
	}

	var doc any
	parseErr := json.Unmarshal(body, &doc)

	results := make([]Result, 0, len(assertions))
	for _, a := range assertions {
		if parseErr != nil {
			results = append(results, bodyID(a).with(
				formatExpected(a.Operator, a.Value),
				"response body is not valid JSON",
				false,
			))
			continue
		}
		results = append(results, evalBodyAssertion(a, doc))
	}
	return results
}

// EvalInput bundles all inputs for assertion evaluation.
type EvalInput struct {
	StatusCodes      []int
	ActualStatus     int
	BodyAssertions   []BodyInput
	Body             []byte
	HeaderAssertions []HeaderInput
	Headers          http.Header
	MaxDurationMs    int
	ActualDuration   time.Duration
	Schema           *CompiledSchema // optional JSON Schema for body validation

	// StatusLine, TimingLine and SchemaLine carry the 1-based source line of
	// the status:, max_duration_ms: and schema: keys. Body, header and CEL
	// assertions carry their own line on each input item instead, because
	// there can be many of each per request.
	StatusLine int
	TimingLine int
	SchemaLine int

	// CELInputs carries zero or more CEL boolean assertion expressions to
	// evaluate. Each entry produces one Result. When empty, no CEL evaluation
	// is performed and CELCtx is ignored.
	CELInputs []CELInput
	// CELCtx provides the activation, evaluator, cache, and sensitive-value
	// snapshot for CEL assertion evaluation. Ignored when CELInputs is empty.
	CELCtx CELContext
}

// Evaluate runs all assertions for a request and returns aggregated results.
// Returns nil if no assertions are defined.
func Evaluate(in EvalInput) *Results {
	var items []Result

	if r := CheckStatus(in.StatusCodes, in.ActualStatus); r != nil {
		r.SourceLine = in.StatusLine
		items = append(items, *r)
	}

	if headerResults := CheckHeaders(in.HeaderAssertions, in.Headers); headerResults != nil {
		items = append(items, headerResults...)
	}

	if bodyResults := CheckBody(in.BodyAssertions, in.Body); bodyResults != nil {
		items = append(items, bodyResults...)
	}

	if schemaResults := CheckSchema(in.Schema, in.Body); schemaResults != nil {
		for i := range schemaResults {
			schemaResults[i].SourceLine = in.SchemaLine
		}
		items = append(items, schemaResults...)
	}

	if celResults := CheckCEL(in.CELInputs, in.CELCtx); celResults != nil {
		items = append(items, celResults...)
	}

	if r := CheckTiming(in.MaxDurationMs, in.ActualDuration); r != nil {
		r.SourceLine = in.TimingLine
		items = append(items, *r)
	}

	if len(items) == 0 {
		return nil
	}

	passed := true
	for _, r := range items {
		if !r.Passed {
			passed = false
			break
		}
	}
	return &Results{Items: items, Passed: passed}
}

// bodyID returns the identity fields shared by every result a body assertion
// can produce. The operator is always the one the caller declared, so a branch
// cannot silently relabel itself.
func bodyID(a BodyInput) Result {
	return Result{Type: TypeBody, Target: a.Path, Operator: a.Operator, SourceLine: a.Line}
}

func evalBodyAssertion(a BodyInput, doc any) Result {
	id := bodyID(a)
	val, err := jsonpath.Evaluate(a.Path, doc)
	notFound := errors.Is(err, jsonpath.ErrNotFound)

	if err != nil && !notFound {
		return id.with(formatExpected(a.Operator, a.Value), fmt.Sprintf("invalid path: %s", err), false)
	}

	switch a.Operator {
	case "exists":
		// Same boolean handling as the header path. not_exists already covered
		// absence, but exists: false silently meant exists: true, so the two
		// spellings of the same intent disagreed.
		if !wantPresent(a.Value) {
			if notFound {
				return id.with("absent", "absent", true)
			}
			return id.with("absent", fmt.Sprintf("found: %v", val), false)
		}
		if notFound {
			return id.with("exists", "no match at path", false)
		}
		return id.with("exists", "exists", true)

	case "not_exists":
		if notFound {
			return id.with("not_exists", "not_exists", true)
		}
		return id.with("not_exists", fmt.Sprintf("found: %v", val), false)

	case "type":
		if notFound {
			return id.with(fmt.Sprintf("type %v", a.Value), "no match at path", false)
		}
		actualType := jsonType(val)
		expectedType := fmt.Sprintf("%v", a.Value)
		return id.with(fmt.Sprintf("type %s", expectedType), fmt.Sprintf("type %s", actualType), actualType == expectedType)

	case "equals":
		if notFound {
			return id.with(fmt.Sprintf("%v", a.Value), "no match at path", false)
		}
		passed := valuesEqual(a.Value, val)
		return id.with(fmt.Sprintf("%v", a.Value), fmt.Sprintf("%v", val), passed)

	case "matches":
		if notFound {
			return notFoundResult(a)
		}
		pattern, ok := a.Value.(string)
		if !ok {
			return id.with(fmt.Sprintf("matches %v", a.Value), "pattern must be a string", false)
		}
		re, err2 := regexp.Compile(pattern)
		if err2 != nil {
			return id.with(fmt.Sprintf("matches %s", pattern), fmt.Sprintf("invalid regex: %s", err2), false)
		}
		actual := fmt.Sprintf("%v", val)
		return id.with(fmt.Sprintf("matches %s", pattern), actual, re.MatchString(actual))

	case "contains":
		if notFound {
			return notFoundResult(a)
		}
		return id.with(fmt.Sprintf("contains %v", a.Value), fmt.Sprintf("%v", val), deepContains(val, a.Value))

	case "contains_all":
		if notFound {
			return notFoundResult(a)
		}
		expectedItems, ok := a.Value.([]any)
		if !ok {
			return id.with(fmt.Sprintf("contains all of %v", a.Value), "expected value must be a list", false)
		}
		actualArr, ok := val.([]any)
		if !ok {
			return id.with(fmt.Sprintf("contains all of %v", a.Value), fmt.Sprintf("%v (not an array)", val), false)
		}
		allFound := true
		for _, exp := range expectedItems {
			found := false
			for _, act := range actualArr {
				if valuesEqual(exp, act) {
					found = true
					break
				}
			}
			if !found {
				allFound = false
				break
			}
		}
		return id.with(fmt.Sprintf("contains all of %v", a.Value), fmt.Sprintf("%v", val), allFound)

	case "length":
		if notFound {
			return notFoundResult(a)
		}
		actualLen, ok := valueLength(val)
		if !ok {
			return id.with(fmt.Sprintf("length %v", a.Value), fmt.Sprintf("%v (not countable)", val), false)
		}
		expectedF, ok := toFloat64(a.Value)
		if !ok {
			return id.with(fmt.Sprintf("length %v", a.Value), fmt.Sprintf("expected %v is not numeric", a.Value), false)
		}
		return id.with(fmt.Sprintf("length %v", a.Value), fmt.Sprintf("length %d", actualLen), float64(actualLen) == expectedF)

	case "greater_than":
		return numericCompare(a, val, notFound, func(act, exp float64) bool { return act > exp }, ">")

	case "less_than":
		return numericCompare(a, val, notFound, func(act, exp float64) bool { return act < exp }, "<")

	case "greater_than_or_equal":
		return numericCompare(a, val, notFound, func(act, exp float64) bool { return act >= exp }, ">=")

	case "less_than_or_equal":
		return numericCompare(a, val, notFound, func(act, exp float64) bool { return act <= exp }, "<=")

	case "approximately":
		if notFound {
			return notFoundResult(a)
		}
		m, ok := a.Value.(map[string]any)
		if !ok {
			return id.with(fmt.Sprintf("≈ %v", a.Value), "expected must be {value, tolerance}", false)
		}
		expVal, err2 := parseMapValue(m, "value")
		if err2 != nil {
			return id.with(fmt.Sprintf("≈ %v", a.Value), err2.Error(), false)
		}
		tolerance, err2 := parseMapValue(m, "tolerance")
		if err2 != nil {
			return id.with(fmt.Sprintf("≈ %v", a.Value), err2.Error(), false)
		}
		actualF, ok := toFloat64(val)
		if !ok {
			return id.with(fmt.Sprintf("≈ %v", a.Value), fmt.Sprintf("%v (not numeric)", val), false)
		}
		return id.with(fmt.Sprintf("≈ %v", a.Value), fmt.Sprintf("%v", val), math.Abs(actualF-expVal) <= tolerance)

	case "in_range":
		if notFound {
			return notFoundResult(a)
		}
		m, ok := a.Value.(map[string]any)
		if !ok {
			return id.with(fmt.Sprintf("in range %v", a.Value), "expected must be {min, max}", false)
		}
		minVal, err2 := parseMapValue(m, "min")
		if err2 != nil {
			return id.with(fmt.Sprintf("in range %v", a.Value), err2.Error(), false)
		}
		maxVal, err2 := parseMapValue(m, "max")
		if err2 != nil {
			return id.with(fmt.Sprintf("in range %v", a.Value), err2.Error(), false)
		}
		actualF, ok := toFloat64(val)
		if !ok {
			return id.with(fmt.Sprintf("in range %v", a.Value), fmt.Sprintf("%v (not numeric)", val), false)
		}
		return id.with(fmt.Sprintf("in range %v", a.Value), fmt.Sprintf("%v", val), actualF >= minVal && actualF <= maxVal)

	default:
		return id.with(fmt.Sprintf("%v", a.Value), "unsupported operator", false)
	}
}

func notFoundResult(a BodyInput) Result {
	return bodyID(a).with(formatExpected(a.Operator, a.Value), "no match at path", false)
}

func numericCompare(a BodyInput, val any, notFound bool, cmp func(actual, expected float64) bool, opSymbol string) Result {
	id := bodyID(a)
	if notFound {
		return notFoundResult(a)
	}
	actualF, ok := toFloat64(val)
	if !ok {
		return id.with(fmt.Sprintf("%s %v", opSymbol, a.Value), fmt.Sprintf("%v (not numeric)", val), false)
	}
	expectedF, ok := toFloat64(a.Value)
	if !ok {
		return id.with(fmt.Sprintf("%s %v", opSymbol, a.Value), fmt.Sprintf("expected %v is not numeric", a.Value), false)
	}
	return id.with(fmt.Sprintf("%s %v", opSymbol, a.Value), fmt.Sprintf("%v", val), cmp(actualF, expectedF))
}

func deepContains(actual, expected any) bool {
	switch act := actual.(type) {
	case string:
		exp, ok := expected.(string)
		if !ok {
			return false
		}
		return strings.Contains(act, exp)
	case map[string]any:
		exp, ok := expected.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range exp {
			av, exists := act[k]
			if !exists || !valuesEqual(av, v) {
				return false
			}
		}
		return true
	case []any:
		for _, item := range act {
			if valuesEqual(item, expected) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func valueLength(val any) (int, bool) {
	switch v := val.(type) {
	case string:
		return len(v), true
	case []any:
		return len(v), true
	case map[string]any:
		return len(v), true
	default:
		return 0, false
	}
}

func parseMapValue(m map[string]any, key string) (float64, error) {
	v, exists := m[key]
	if !exists {
		return 0, fmt.Errorf("missing '%s' key", key)
	}
	f, ok := toFloat64(v)
	if !ok {
		return 0, fmt.Errorf("'%s' value %v is not numeric", key, v)
	}
	return f, nil
}

// valuesEqual compares an expected value (from YAML) with an actual value (from JSON).
// Handles cross-type numeric comparison including numeric strings (e.g., "42" == 42).
// Non-numeric type mismatches never compare equal (e.g., string "true" != boolean true).
func valuesEqual(expected, actual any) bool {
	ef, eOk := toFloat64(expected)
	af, aOk := toFloat64(actual)
	if eOk && aOk {
		return ef == af
	}
	// Reject cross-type comparison when types differ
	if fmt.Sprintf("%T", expected) != fmt.Sprintf("%T", actual) {
		return false
	}
	return fmt.Sprintf("%v", expected) == fmt.Sprintf("%v", actual)
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// jsonType returns the user-facing type name for a JSON-unmarshalled value.
func jsonType(v any) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func formatExpected(operator string, value any) string {
	switch operator {
	case "exists":
		return "exists"
	case "not_exists":
		return "not_exists"
	case "type":
		return fmt.Sprintf("type %v", value)
	case "matches":
		return fmt.Sprintf("matches %v", value)
	case "contains":
		return fmt.Sprintf("contains %v", value)
	case "contains_all":
		return fmt.Sprintf("contains all of %v", value)
	case "length":
		return fmt.Sprintf("length %v", value)
	case "greater_than":
		return fmt.Sprintf("> %v", value)
	case "less_than":
		return fmt.Sprintf("< %v", value)
	case "greater_than_or_equal":
		return fmt.Sprintf(">= %v", value)
	case "less_than_or_equal":
		return fmt.Sprintf("<= %v", value)
	case "approximately":
		return fmt.Sprintf("≈ %v", value)
	case "in_range":
		return fmt.Sprintf("in range %v", value)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func formatCodes(codes []int) string {
	if len(codes) == 1 {
		return fmt.Sprintf("%d", codes[0])
	}
	parts := make([]string, len(codes))
	for i, c := range codes {
		parts[i] = fmt.Sprintf("%d", c)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// wantPresent interprets the value of an `exists` assertion.
//
// true asserts presence, false asserts absence (CLI_SPECIFICATION §7.2). The
// value arrives as a bool from the body path and as a string from the header
// path, which stringifies it on the way through requtil, so both are accepted.
//
// Anything unrecognisable keeps the historical meaning — assert presence —
// rather than becoming a new error class, so a collection that was passing
// before cannot start failing on a value nobody intended as a boolean.
func wantPresent(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case bool:
		return v
	case string:
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
		}
	}
	return true
}
