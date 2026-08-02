package harness_test

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Expectation describes the required shape of the event stream produced by a
// single harness scenario. It is loaded from testdata/agent-harness/<name>/expect.yaml.
type Expectation struct {
	// Description is a human-readable summary of what the scenario tests.
	Description string `yaml:"description"`
	// ExitCode is the expected process exit code. 0 means any exit code is acceptable
	// only when no events are expected; otherwise use the actual expected value.
	ExitCode int `yaml:"exit_code"`
	// Events is the ordered list of required events. Each entry must match at
	// least one event in the stream (order-independent matching).
	Events []ExpectedEvent `yaml:"events"`
}

// ExpectedEvent describes the required shape of a single event in the stream.
type ExpectedEvent struct {
	// Kind is the required event kind (e.g. "run.error", "request.end").
	Kind string `yaml:"kind"`
	// MustHave is a flat map of dotted JSON paths to expected values.
	// Special suffix semantics:
	//   key ending in "_nonempty: true"  → field must be non-empty string
	//   key ending in "_nonzero: true"   → field must be non-zero integer
	//   key ending in "_pattern: <re>"   → field value must match the regex
	//   bare value                       → exact match
	//
	// Network-category exemption: when the matched event has error.category="network",
	// error.line_nonzero constraints allow line=0.
	MustHave map[string]any `yaml:"must_have"`
	// HintContainsAny requires that the event's error.hint contains at least one
	// of the listed substrings.
	HintContainsAny []string `yaml:"hint_contains_any"`
}

// ParseExpectation decodes an expect.yaml file into an Expectation.
func ParseExpectation(data []byte) (Expectation, error) {
	var e Expectation
	if err := yaml.Unmarshal(data, &e); err != nil {
		return Expectation{}, fmt.Errorf("parse expectation: %w", err)
	}
	return e, nil
}

// Verify checks that the event stream satisfies every constraint in e.
// exitCode is the process exit code from the apitest invocation.
// Returns a human-readable diff-style error if any constraint is unmet, nil on success.
func (e Expectation) Verify(stream []map[string]any, exitCode int) error {
	var errs []string

	// Check exit code.
	if e.ExitCode != exitCode {
		errs = append(errs, fmt.Sprintf("exit_code = %d, want %d", exitCode, e.ExitCode))
	}

	// For each required event, find a matching event in the stream.
	for _, ev := range e.Events {
		if err := matchEvent(ev, stream); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

// matchEvent finds an event with ev.Kind in stream and verifies ev.MustHave and
// ev.HintContainsAny constraints against it.
func matchEvent(ev ExpectedEvent, stream []map[string]any) error {
	// Collect all events of the required kind.
	var candidates []map[string]any
	for _, e := range stream {
		if k, _ := e["kind"].(string); k == ev.Kind {
			candidates = append(candidates, e)
		}
	}
	if len(candidates) == 0 {
		return fmt.Errorf("no event with kind=%s found in stream", ev.Kind)
	}

	// Try each candidate; return nil if any satisfies all constraints.
	var lastErr error
	for _, candidate := range candidates {
		if err := verifyEventConstraints(ev, candidate); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

// verifyEventConstraints checks all MustHave and HintContainsAny constraints
// against a single event map. Returns nil if all constraints are satisfied.
func verifyEventConstraints(ev ExpectedEvent, event map[string]any) error {
	// Determine if this is a network-category event (exempts line_nonzero).
	isNetwork := isNetworkCategory(event)

	for key, val := range ev.MustHave {
		// Determine the base path and constraint type.
		basePath, constraint := parseConstraintKey(key)

		switch constraint {
		case "nonempty":
			fieldVal := getNestedString(event, basePath)
			if fieldVal == "" {
				return fmt.Errorf("%s is empty, want non-empty", basePath)
			}
		case "nonzero":
			// Network exemption: allow line=0 when category=network.
			if basePath == "error.line" && isNetwork {
				continue
			}
			fieldNum := getNestedNumber(event, basePath)
			if fieldNum == 0 {
				return fmt.Errorf("%s is 0, want non-zero", basePath)
			}
		case "pattern":
			pattern, ok := val.(string)
			if !ok {
				return fmt.Errorf("%s: pattern value must be a string", key)
			}
			fieldVal := getNestedString(event, basePath)
			matched, err := regexp.MatchString(pattern, fieldVal)
			if err != nil {
				return fmt.Errorf("%s: invalid regex %q: %v", basePath, pattern, err)
			}
			if !matched {
				return fmt.Errorf("%s = %q does not match pattern %q", basePath, fieldVal, pattern)
			}
		default: // exact match
			fieldVal := getNestedValue(event, basePath)
			if !valueEqual(fieldVal, val) {
				return fmt.Errorf("%s = %v, want %v", basePath, fieldVal, val)
			}
		}
	}

	// Check hint constraint.
	if len(ev.HintContainsAny) > 0 {
		hint := getNestedString(event, "error.hint")
		found := false
		for _, verb := range ev.HintContainsAny {
			if strings.Contains(hint, verb) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("error.hint %q contains none of [%s]", hint, strings.Join(ev.HintContainsAny, " "))
		}
	}

	return nil
}

// parseConstraintKey splits a MustHave key into a dotted path and a constraint
// type. The constraint is identified by the last component suffix:
//   - "_nonempty" → ("error.file", "nonempty")
//   - "_nonzero"  → ("error.line", "nonzero")
//   - "_pattern"  → ("error.code", "pattern")
//   - otherwise   → (key, "exact")
func parseConstraintKey(key string) (path, constraint string) {
	for _, suffix := range []string{"_nonempty", "_nonzero", "_pattern"} {
		if strings.HasSuffix(key, suffix) {
			return key[:len(key)-len(suffix)], suffix[1:] // trim leading "_"
		}
	}
	return key, "exact"
}

// isNetworkCategory reports whether the event has error.category="network".
func isNetworkCategory(event map[string]any) bool {
	return getNestedString(event, "error.category") == "network"
}

// getNestedValue resolves a dotted path like "error.category" against an event map.
// Returns nil if the path does not exist.
func getNestedValue(event map[string]any, path string) any {
	parts := strings.SplitN(path, ".", 2)
	val, ok := event[parts[0]]
	if !ok {
		return nil
	}
	if len(parts) == 1 {
		return val
	}
	nested, ok := val.(map[string]any)
	if !ok {
		return nil
	}
	return getNestedValue(nested, parts[1])
}

// getNestedString returns the string value at path, or "" if not found or not a string.
func getNestedString(event map[string]any, path string) string {
	v := getNestedValue(event, path)
	s, _ := v.(string)
	return s
}

// getNestedNumber returns the numeric value at path, or 0 if not found or not a number.
func getNestedNumber(event map[string]any, path string) float64 {
	v := getNestedValue(event, path)
	n, _ := v.(float64)
	return n
}

// valueEqual compares a JSON-decoded value (where numbers are float64) to a
// YAML-decoded expected value.
func valueEqual(jsonVal, yamlVal any) bool {
	if jsonVal == nil && yamlVal == nil {
		return true
	}
	// Normalise both to string for simple string comparison.
	if s, ok := yamlVal.(string); ok {
		js, ok := jsonVal.(string)
		return ok && js == s
	}
	// Numeric: YAML may produce int, JSON always float64.
	if n, ok := toFloat64(yamlVal); ok {
		jn, ok := jsonVal.(float64)
		return ok && jn == n
	}
	if b, ok := yamlVal.(bool); ok {
		jb, ok := jsonVal.(bool)
		return ok && jb == b
	}
	return fmt.Sprintf("%v", jsonVal) == fmt.Sprintf("%v", yamlVal)
}

// toFloat64 converts an integer or float to float64.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}
