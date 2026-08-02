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
type Result struct {
	Type     string // e.g. "status", "body"
	Expected string // human-readable expected value
	Actual   string // human-readable actual value
	Passed   bool
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
}

// HeaderInput describes a single header assertion to evaluate.
type HeaderInput struct {
	Name     string // Header name (matched case-insensitively)
	Operator string // "equals", "exists", "matches"
	Value    string // expected value (for equals/matches)
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
	typeName := fmt.Sprintf("header %s %s", a.Name, a.Operator)

	switch a.Operator {
	case "equals":
		actual := headers.Get(a.Name)
		if headers == nil || len(headers.Values(a.Name)) == 0 {
			return Result{
				Type:     typeName,
				Expected: a.Value,
				Actual:   "header not present",
				Passed:   false,
			}
		}
		return Result{
			Type:     typeName,
			Expected: a.Value,
			Actual:   actual,
			Passed:   actual == a.Value,
		}

	case "exists":
		if headers == nil || len(headers.Values(a.Name)) == 0 {
			return Result{
				Type:     typeName,
				Expected: "exists",
				Actual:   "header not present",
				Passed:   false,
			}
		}
		return Result{
			Type:     typeName,
			Expected: "exists",
			Actual:   "exists",
			Passed:   true,
		}

	case "matches":
		re, err := regexp.Compile(a.Value)
		if err != nil {
			return Result{
				Type:     typeName,
				Expected: fmt.Sprintf("matches %s", a.Value),
				Actual:   fmt.Sprintf("invalid regex: %s", err),
				Passed:   false,
			}
		}
		actual := headers.Get(a.Name)
		if headers == nil || len(headers.Values(a.Name)) == 0 {
			return Result{
				Type:     typeName,
				Expected: fmt.Sprintf("matches %s", a.Value),
				Actual:   "header not present",
				Passed:   false,
			}
		}
		passed := re.MatchString(actual)
		return Result{
			Type:     typeName,
			Expected: fmt.Sprintf("matches %s", a.Value),
			Actual:   actual,
			Passed:   passed,
		}

	default:
		return Result{
			Type:     typeName,
			Expected: a.Value,
			Actual:   "unsupported operator",
			Passed:   false,
		}
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
		Type:     "timing",
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
	for _, code := range expected {
		if code == actual {
			return &Result{
				Type:     "status",
				Expected: formatCodes(expected),
				Actual:   fmt.Sprintf("%d", actual),
				Passed:   true,
			}
		}
	}
	return &Result{
		Type:     "status",
		Expected: formatCodes(expected),
		Actual:   fmt.Sprintf("%d", actual),
		Passed:   false,
	}
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
			results = append(results, Result{
				Type:     fmt.Sprintf("body %s %s", a.Path, a.Operator),
				Expected: formatExpected(a.Operator, a.Value),
				Actual:   "response body is not valid JSON",
				Passed:   false,
			})
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
		items = append(items, *r)
	}

	if headerResults := CheckHeaders(in.HeaderAssertions, in.Headers); headerResults != nil {
		items = append(items, headerResults...)
	}

	if bodyResults := CheckBody(in.BodyAssertions, in.Body); bodyResults != nil {
		items = append(items, bodyResults...)
	}

	if schemaResults := CheckSchema(in.Schema, in.Body); schemaResults != nil {
		items = append(items, schemaResults...)
	}

	if celResults := CheckCEL(in.CELInputs, in.CELCtx); celResults != nil {
		items = append(items, celResults...)
	}

	if r := CheckTiming(in.MaxDurationMs, in.ActualDuration); r != nil {
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

func evalBodyAssertion(a BodyInput, doc any) Result {
	val, err := jsonpath.Evaluate(a.Path, doc)
	notFound := errors.Is(err, jsonpath.ErrNotFound)

	if err != nil && !notFound {
		return Result{
			Type:     fmt.Sprintf("body %s %s", a.Path, a.Operator),
			Expected: formatExpected(a.Operator, a.Value),
			Actual:   fmt.Sprintf("invalid path: %s", err),
			Passed:   false,
		}
	}

	switch a.Operator {
	case "exists":
		if notFound {
			return Result{
				Type:     fmt.Sprintf("body %s exists", a.Path),
				Expected: "exists",
				Actual:   "no match at path",
				Passed:   false,
			}
		}
		return Result{
			Type:     fmt.Sprintf("body %s exists", a.Path),
			Expected: "exists",
			Actual:   "exists",
			Passed:   true,
		}

	case "not_exists":
		if notFound {
			return Result{
				Type:     fmt.Sprintf("body %s not_exists", a.Path),
				Expected: "not_exists",
				Actual:   "not_exists",
				Passed:   true,
			}
		}
		return Result{
			Type:     fmt.Sprintf("body %s not_exists", a.Path),
			Expected: "not_exists",
			Actual:   fmt.Sprintf("found: %v", val),
			Passed:   false,
		}

	case "type":
		if notFound {
			return Result{
				Type:     fmt.Sprintf("body %s type", a.Path),
				Expected: fmt.Sprintf("type %v", a.Value),
				Actual:   "no match at path",
				Passed:   false,
			}
		}
		actualType := jsonType(val)
		expectedType := fmt.Sprintf("%v", a.Value)
		return Result{
			Type:     fmt.Sprintf("body %s type", a.Path),
			Expected: fmt.Sprintf("type %s", expectedType),
			Actual:   fmt.Sprintf("type %s", actualType),
			Passed:   actualType == expectedType,
		}

	case "equals":
		if notFound {
			return Result{
				Type:     fmt.Sprintf("body %s equals", a.Path),
				Expected: fmt.Sprintf("%v", a.Value),
				Actual:   "no match at path",
				Passed:   false,
			}
		}
		passed := valuesEqual(a.Value, val)
		return Result{
			Type:     fmt.Sprintf("body %s equals", a.Path),
			Expected: fmt.Sprintf("%v", a.Value),
			Actual:   fmt.Sprintf("%v", val),
			Passed:   passed,
		}

	case "matches":
		if notFound {
			return notFoundResult(a)
		}
		pattern, ok := a.Value.(string)
		if !ok {
			return Result{
				Type:     fmt.Sprintf("body %s matches", a.Path),
				Expected: fmt.Sprintf("matches %v", a.Value),
				Actual:   "pattern must be a string",
				Passed:   false,
			}
		}
		re, err2 := regexp.Compile(pattern)
		if err2 != nil {
			return Result{
				Type:     fmt.Sprintf("body %s matches", a.Path),
				Expected: fmt.Sprintf("matches %s", pattern),
				Actual:   fmt.Sprintf("invalid regex: %s", err2),
				Passed:   false,
			}
		}
		actual := fmt.Sprintf("%v", val)
		return Result{
			Type:     fmt.Sprintf("body %s matches", a.Path),
			Expected: fmt.Sprintf("matches %s", pattern),
			Actual:   actual,
			Passed:   re.MatchString(actual),
		}

	case "contains":
		if notFound {
			return notFoundResult(a)
		}
		return Result{
			Type:     fmt.Sprintf("body %s contains", a.Path),
			Expected: fmt.Sprintf("contains %v", a.Value),
			Actual:   fmt.Sprintf("%v", val),
			Passed:   deepContains(val, a.Value),
		}

	case "contains_all":
		if notFound {
			return notFoundResult(a)
		}
		expectedItems, ok := a.Value.([]any)
		if !ok {
			return Result{
				Type:     fmt.Sprintf("body %s contains_all", a.Path),
				Expected: fmt.Sprintf("contains all of %v", a.Value),
				Actual:   "expected value must be a list",
				Passed:   false,
			}
		}
		actualArr, ok := val.([]any)
		if !ok {
			return Result{
				Type:     fmt.Sprintf("body %s contains_all", a.Path),
				Expected: fmt.Sprintf("contains all of %v", a.Value),
				Actual:   fmt.Sprintf("%v (not an array)", val),
				Passed:   false,
			}
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
		return Result{
			Type:     fmt.Sprintf("body %s contains_all", a.Path),
			Expected: fmt.Sprintf("contains all of %v", a.Value),
			Actual:   fmt.Sprintf("%v", val),
			Passed:   allFound,
		}

	case "length":
		if notFound {
			return notFoundResult(a)
		}
		actualLen, ok := valueLength(val)
		if !ok {
			return Result{
				Type:     fmt.Sprintf("body %s length", a.Path),
				Expected: fmt.Sprintf("length %v", a.Value),
				Actual:   fmt.Sprintf("%v (not countable)", val),
				Passed:   false,
			}
		}
		expectedF, ok := toFloat64(a.Value)
		if !ok {
			return Result{
				Type:     fmt.Sprintf("body %s length", a.Path),
				Expected: fmt.Sprintf("length %v", a.Value),
				Actual:   fmt.Sprintf("expected %v is not numeric", a.Value),
				Passed:   false,
			}
		}
		return Result{
			Type:     fmt.Sprintf("body %s length", a.Path),
			Expected: fmt.Sprintf("length %v", a.Value),
			Actual:   fmt.Sprintf("length %d", actualLen),
			Passed:   float64(actualLen) == expectedF,
		}

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
			return Result{
				Type:     fmt.Sprintf("body %s approximately", a.Path),
				Expected: fmt.Sprintf("≈ %v", a.Value),
				Actual:   "expected must be {value, tolerance}",
				Passed:   false,
			}
		}
		expVal, err2 := parseMapValue(m, "value")
		if err2 != nil {
			return Result{
				Type:     fmt.Sprintf("body %s approximately", a.Path),
				Expected: fmt.Sprintf("≈ %v", a.Value),
				Actual:   err2.Error(),
				Passed:   false,
			}
		}
		tolerance, err2 := parseMapValue(m, "tolerance")
		if err2 != nil {
			return Result{
				Type:     fmt.Sprintf("body %s approximately", a.Path),
				Expected: fmt.Sprintf("≈ %v", a.Value),
				Actual:   err2.Error(),
				Passed:   false,
			}
		}
		actualF, ok := toFloat64(val)
		if !ok {
			return Result{
				Type:     fmt.Sprintf("body %s approximately", a.Path),
				Expected: fmt.Sprintf("≈ %v", a.Value),
				Actual:   fmt.Sprintf("%v (not numeric)", val),
				Passed:   false,
			}
		}
		return Result{
			Type:     fmt.Sprintf("body %s approximately", a.Path),
			Expected: fmt.Sprintf("≈ %v", a.Value),
			Actual:   fmt.Sprintf("%v", val),
			Passed:   math.Abs(actualF-expVal) <= tolerance,
		}

	case "in_range":
		if notFound {
			return notFoundResult(a)
		}
		m, ok := a.Value.(map[string]any)
		if !ok {
			return Result{
				Type:     fmt.Sprintf("body %s in_range", a.Path),
				Expected: fmt.Sprintf("in range %v", a.Value),
				Actual:   "expected must be {min, max}",
				Passed:   false,
			}
		}
		minVal, err2 := parseMapValue(m, "min")
		if err2 != nil {
			return Result{
				Type:     fmt.Sprintf("body %s in_range", a.Path),
				Expected: fmt.Sprintf("in range %v", a.Value),
				Actual:   err2.Error(),
				Passed:   false,
			}
		}
		maxVal, err2 := parseMapValue(m, "max")
		if err2 != nil {
			return Result{
				Type:     fmt.Sprintf("body %s in_range", a.Path),
				Expected: fmt.Sprintf("in range %v", a.Value),
				Actual:   err2.Error(),
				Passed:   false,
			}
		}
		actualF, ok := toFloat64(val)
		if !ok {
			return Result{
				Type:     fmt.Sprintf("body %s in_range", a.Path),
				Expected: fmt.Sprintf("in range %v", a.Value),
				Actual:   fmt.Sprintf("%v (not numeric)", val),
				Passed:   false,
			}
		}
		return Result{
			Type:     fmt.Sprintf("body %s in_range", a.Path),
			Expected: fmt.Sprintf("in range %v", a.Value),
			Actual:   fmt.Sprintf("%v", val),
			Passed:   actualF >= minVal && actualF <= maxVal,
		}

	default:
		return Result{
			Type:     fmt.Sprintf("body %s %s", a.Path, a.Operator),
			Expected: fmt.Sprintf("%v", a.Value),
			Actual:   "unsupported operator",
			Passed:   false,
		}
	}
}

func notFoundResult(a BodyInput) Result {
	return Result{
		Type:     fmt.Sprintf("body %s %s", a.Path, a.Operator),
		Expected: formatExpected(a.Operator, a.Value),
		Actual:   "no match at path",
		Passed:   false,
	}
}

func numericCompare(a BodyInput, val any, notFound bool, cmp func(actual, expected float64) bool, opSymbol string) Result {
	if notFound {
		return notFoundResult(a)
	}
	actualF, ok := toFloat64(val)
	if !ok {
		return Result{
			Type:     fmt.Sprintf("body %s %s", a.Path, a.Operator),
			Expected: fmt.Sprintf("%s %v", opSymbol, a.Value),
			Actual:   fmt.Sprintf("%v (not numeric)", val),
			Passed:   false,
		}
	}
	expectedF, ok := toFloat64(a.Value)
	if !ok {
		return Result{
			Type:     fmt.Sprintf("body %s %s", a.Path, a.Operator),
			Expected: fmt.Sprintf("%s %v", opSymbol, a.Value),
			Actual:   fmt.Sprintf("expected %v is not numeric", a.Value),
			Passed:   false,
		}
	}
	return Result{
		Type:     fmt.Sprintf("body %s %s", a.Path, a.Operator),
		Expected: fmt.Sprintf("%s %v", opSymbol, a.Value),
		Actual:   fmt.Sprintf("%v", val),
		Passed:   cmp(actualF, expectedF),
	}
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
