# Implementation Plan: M1-007

## Overview
Extend the assertion engine with the full operator set (matches, contains, contains_all, length, numeric comparisons, approximately, in_range) and add numeric string type coercion to `equals`.

## Task Details
- **ID:** M1-007
- **Title:** Full assertion operator set
- **Phase:** M1: Core CLI
- **Priority:** 7
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-005 | Assert on response body with JSONPath | done |

## Architecture Summary

**Current state:** `internal/assertion/assertion.go` handles assertion evaluation via a `switch a.Operator` in `evalBodyAssertion()`. Currently supports: `equals`, `exists`, `not_exists`, `type`. New operators are added by extending this switch.

**Key insight — no parser changes needed:** The YAML parser already handles arbitrary operator names and values generically. `BodyInput.Value` is typed `any`, so maps (`approximately: {value: 3.14, tolerance: 0.01}`) and slices (`contains_all: [1, 2, 3]`) flow through naturally.

**Scope of changes:** Entirely within `internal/assertion/` (plus smoke tests). No changes to parser, runner, output, or main.

## Implementation Steps

### Step 1: Enhance `valuesEqual` for numeric string coercion
**Rationale:** This is the smallest change and affects the existing `equals` operator — must be done first since later operators build on `toFloat64`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | Extend `toFloat64` to handle `string` via `strconv.ParseFloat` |
| `internal/assertion/assertion_test.go` | modify | Add coercion tests, update 1 existing test |

#### Current Code
```go
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
```

#### New Code
```go
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
```

#### Tests to Write FIRST (RED phase)

```go
// Add to TestCheckBody table-driven tests:
{"equals passes for numeric string 42 vs number 42", ...},   // actual: "42", expected: 42
{"equals passes for number 42 vs numeric string 42", ...},   // actual: 42, expected: "42"
{"equals passes for float string 3.14 vs number 3.14", ...}, // actual: "3.14", expected: 3.14
{"equals rejects non-numeric string vs number", ...},         // actual: "hello", expected: 42
```

#### Impact on Existing Tests
- **`"equals rejects string 1 vs number 1"`** — will break intentionally. Must update to `"equals passes for numeric string 1 vs number 1"` with `Passed: true`. This is the intended behavioral change per the task.

---

### Step 2: Add `matches` operator for body assertions
**Rationale:** Already implemented for headers, extending to body is a small isolated addition.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | Add `case "matches"` in `evalBodyAssertion` switch |
| `internal/assertion/assertion_test.go` | modify | Add 5 test cases |

#### New Code
```go
case "matches":
    if notFound {
        return notFoundResult(a, "matches")
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
    re, err := regexp.Compile(pattern)
    if err != nil {
        return Result{
            Type:     fmt.Sprintf("body %s matches", a.Path),
            Expected: fmt.Sprintf("matches %s", pattern),
            Actual:   fmt.Sprintf("invalid regex: %s", err),
            Passed:   false,
        }
    }
    actual := fmt.Sprintf("%v", val)
    passed := re.MatchString(actual)
    return Result{
        Type:     fmt.Sprintf("body %s matches", a.Path),
        Expected: fmt.Sprintf("matches %s", pattern),
        Actual:   actual,
        Passed:   passed,
    }
```

#### Tests to Write FIRST (RED phase)

```go
{"matches passes when string matches regex", ...},
{"matches fails when string does not match regex", ...},
{"matches fails with invalid regex", ...},
{"matches fails when path not found", ...},
{"matches converts number to string before matching", ...},
```

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 3: Add `contains` operator for body assertions
**Rationale:** Dual semantics (string substring, object partial, array element) — moderately complex but self-contained.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | Add `case "contains"` + `deepContains` helper |
| `internal/assertion/assertion_test.go` | modify | Add 7 test cases |

#### New Code (helper)
```go
func deepContains(actual any, expected any) bool {
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
```

#### Tests to Write FIRST (RED phase)

```go
{"contains passes for string containing substring", ...},
{"contains fails for string not containing substring", ...},
{"contains passes for object containing subset keys", ...},
{"contains fails for object missing expected keys", ...},
{"contains passes for array containing expected element", ...},
{"contains fails for array not containing expected element", ...},
{"contains fails when path not found", ...},
```

#### Impact on Existing Tests
- Header `contains` test (`"unsupported operator returns failure"`) remains unchanged — this adds body `contains` only.

---

### Step 4: Add `contains_all` operator
**Rationale:** Builds on `contains` helper infrastructure.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | Add `case "contains_all"` |
| `internal/assertion/assertion_test.go` | modify | Add 6 test cases |

#### Tests to Write FIRST (RED phase)

```go
{"contains_all passes when all items present", ...},
{"contains_all passes when array has extra items", ...},
{"contains_all fails when one item missing", ...},
{"contains_all fails when actual is not an array", ...},
{"contains_all fails when expected is not a list", ...},
{"contains_all fails when path not found", ...},
```

---

### Step 5: Add `length` operator
**Rationale:** Simple operator, self-contained.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | Add `case "length"` + `valueLength` helper |
| `internal/assertion/assertion_test.go` | modify | Add 8 test cases |

#### New Code (helper)
```go
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
```

#### Tests to Write FIRST (RED phase)

```go
{"length passes for array with matching length", ...},
{"length fails for array with different length", ...},
{"length passes for string with matching length", ...},
{"length fails for string with different length", ...},
{"length passes for object with matching key count", ...},
{"length fails for non-countable value", ...},
{"length fails when path not found", ...},
{"length handles zero-length array", ...},
```

---

### Step 6: Add numeric comparison operators
**Rationale:** Four related operators sharing a helper — batch them together.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | Add 4 cases + `numericCompare` helper |
| `internal/assertion/assertion_test.go` | modify | Add ~20 test cases |

#### New Code (helper)
```go
func numericCompare(a BodyInput, val any, notFound bool, cmp func(actual, expected float64) bool, opName string, opSymbol string) Result {
    if notFound {
        return notFoundResult(a, opName)
    }
    actualF, ok := toFloat64(val)
    if !ok {
        return Result{
            Type:     fmt.Sprintf("body %s %s", a.Path, opName),
            Expected: fmt.Sprintf("%s %v", opSymbol, a.Value),
            Actual:   fmt.Sprintf("%v (not numeric)", val),
            Passed:   false,
        }
    }
    expectedF, ok := toFloat64(a.Value)
    if !ok {
        return Result{
            Type:     fmt.Sprintf("body %s %s", a.Path, opName),
            Expected: fmt.Sprintf("%s %v", opSymbol, a.Value),
            Actual:   fmt.Sprintf("expected %v is not numeric", a.Value),
            Passed:   false,
        }
    }
    return Result{
        Type:     fmt.Sprintf("body %s %s", a.Path, opName),
        Expected: fmt.Sprintf("%s %v", opSymbol, a.Value),
        Actual:   fmt.Sprintf("%v", val),
        Passed:   cmp(actualF, expectedF),
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
// greater_than
{"greater_than passes when value exceeds threshold", ...},
{"greater_than fails when value equals threshold", ...},
{"greater_than fails when value below threshold", ...},
{"greater_than fails for non-numeric value", ...},
{"greater_than fails when path not found", ...},
// less_than
{"less_than passes when value below threshold", ...},
{"less_than fails when value equals threshold", ...},
{"less_than fails when value above threshold", ...},
{"less_than fails when path not found", ...},
// greater_than_or_equal
{"gte passes when value exceeds threshold", ...},
{"gte passes when value equals threshold", ...},
{"gte fails when value below threshold", ...},
{"gte fails when path not found", ...},
// less_than_or_equal
{"lte passes when value below threshold", ...},
{"lte passes when value equals threshold", ...},
{"lte fails when value above threshold", ...},
{"lte fails when path not found", ...},
// numeric string coercion
{"greater_than works with numeric string values", ...},
```

---

### Step 7: Add `approximately` operator
**Rationale:** Requires map parsing — more complex than simple value operators.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | Add `case "approximately"` + `parseMapValue` helper |
| `internal/assertion/assertion_test.go` | modify | Add 7 test cases |

#### New Code (helper)
```go
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
```

#### Tests to Write FIRST (RED phase)

```go
{"approximately passes when within tolerance", ...},
{"approximately fails when outside tolerance", ...},
{"approximately passes at exact boundary", ...},
{"approximately fails with missing value key", ...},
{"approximately fails with missing tolerance key", ...},
{"approximately fails with non-numeric actual", ...},
{"approximately fails when path not found", ...},
```

---

### Step 8: Add `in_range` operator
**Rationale:** Similar to `approximately` — map parsing with two keys.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | Add `case "in_range"` |
| `internal/assertion/assertion_test.go` | modify | Add 9 test cases |

#### Tests to Write FIRST (RED phase)

```go
{"in_range passes when value within range", ...},
{"in_range passes when value equals min", ...},
{"in_range passes when value equals max", ...},
{"in_range fails when value below min", ...},
{"in_range fails when value above max", ...},
{"in_range fails with missing min key", ...},
{"in_range fails with missing max key", ...},
{"in_range fails with non-numeric actual", ...},
{"in_range fails when path not found", ...},
```

---

### Step 9: Update `formatExpected` helper
**Rationale:** No new tests needed — covered by operator tests that check `Expected` field.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | Add cases to `formatExpected` switch |

#### New Code
```go
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
```

---

### Step 10: Update smoke tests
**Rationale:** Final step — validates the full stack with the real binary.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add test cases exercising new operators |

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/assertion/assertion_test.go` | `TestCheckBody` | 1 test case changes | Update `"equals rejects string 1 vs number 1"` to expect `true` |
| `internal/assertion/assertion_test.go` | `TestCheckHeaders` | none | — |
| `internal/runner/runner_test.go` | all | none | — |
| `internal/parser/*_test.go` | all | none | — |

## Risks and Edge Cases

- **Risk:** Regex compilation errors in `matches` → **Mitigation:** Return descriptive failure result (same pattern as header `matches`)
- **Risk:** Non-numeric values for comparison operators → **Mitigation:** `toFloat64` returns false, operator returns clear "not numeric" failure
- **Risk:** nil/null values from JSONPath → **Mitigation:** `toFloat64(nil)` returns false, all operators fail descriptively
- **Edge case:** Empty expected list for `contains_all` → **Handling:** Vacuous truth — passes (all zero items found)
- **Edge case:** Boolean coercion to number → **Handling:** `toFloat64` does NOT handle `bool`, so `true` won't coerce to 1.0 (correct per spec)
- **Edge case:** `in_range` with min > max → **Handling:** Assertion always fails (impossible range)
- **Edge case:** String `""` vs number → **Handling:** `strconv.ParseFloat("", 64)` fails, so empty string won't coerce to 0

## New imports needed
- `"strconv"` — for numeric string coercion in `toFloat64`
- `"math"` — for `math.Abs` in `approximately`
- `"strings"` — for `strings.Contains` in body `contains` (may already be imported)

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create test collection with multiple operators, run and see all assertions evaluated
cat > /tmp/test-operators.yaml << 'YAML'
requests:
  - name: "test operators"
    method: GET
    url: "https://httpbin.org/get?count=42"
    assert:
      status: 200
      body:
        $.url:
          contains: "httpbin.org"
        $.args.count:
          equals: "42"
YAML
go run ./cmd/curlew run /tmp/test-operators.yaml
```
