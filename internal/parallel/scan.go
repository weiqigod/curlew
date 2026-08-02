package parallel

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/weiqigod/curlew/internal/parser"
)

// varScanPattern matches {{varName}} and {{varName|default}} patterns.
// Broader than variable.varPattern: includes dots for nested access and pipe for defaults.
var varScanPattern = regexp.MustCompile(`\{\{([a-zA-Z_$][a-zA-Z0-9_.]*(?:\|[^}]+)?)\}\}`)

// ScanVariables extracts all variable references from a string.
// Returns variable names (before the pipe if default syntax is used).
// Dynamic functions (starting with $) are excluded.
// Pre-execution variables are excluded.
// The {{secrets.X}} namespace is excluded — secrets are resolved before the
// first execution wave and are therefore never per-request dependencies.
func ScanVariables(input string, preExecVars map[string]bool) map[string]bool {
	result := make(map[string]bool)
	matches := varScanPattern.FindAllStringSubmatch(input, -1)
	for _, m := range matches {
		name := m[1]
		// Strip default value after pipe
		if idx := strings.IndexByte(name, '|'); idx >= 0 {
			name = name[:idx]
		}
		// Skip dynamic functions (starting with $)
		if strings.HasPrefix(name, "$") {
			continue
		}
		// Skip secrets namespace — resolved pre-execution, not a wave dependency
		if strings.HasPrefix(name, "secrets.") {
			continue
		}
		// Skip pre-execution variables
		if preExecVars != nil && preExecVars[name] {
			continue
		}
		result[name] = true
	}
	return result
}

// ScanRequestFields scans all interpolatable fields of a request item
// and returns the set of referenced variable names.
func ScanRequestFields(item *parser.RequestItem, preExecVars map[string]bool) map[string]bool {
	result := make(map[string]bool)
	if item == nil {
		return result
	}

	merge := func(vars map[string]bool) {
		for k := range vars {
			result[k] = true
		}
	}

	// Scan URL
	merge(ScanVariables(item.Request.URL, preExecVars))

	// Scan method
	merge(ScanVariables(item.Request.Method, preExecVars))

	// Scan headers
	for _, v := range item.Request.Headers {
		merge(ScanVariables(v, preExecVars))
	}

	// Scan query params
	for _, v := range item.Request.QueryParams {
		merge(ScanVariables(v, preExecVars))
	}

	// Scan body
	scanBody(item.Request.Body, preExecVars, result)

	// Scan extract paths (the values, not the keys)
	for _, v := range item.Extract {
		merge(ScanVariables(v, preExecVars))
	}

	// Scan assertion body paths
	for _, ba := range item.Assertions.Body.Items {
		merge(ScanVariables(ba.Path, preExecVars))
		// Also scan string values in assertions
		if s, ok := ba.Value.(string); ok {
			merge(ScanVariables(s, preExecVars))
		}
	}

	// Scan assertion header keys/values
	for _, ha := range item.Assertions.Headers.Items {
		if s, ok := ha.Value.(string); ok {
			merge(ScanVariables(s, preExecVars))
		}
	}

	return result
}

// scanBody recursively scans body content for variable references.
func scanBody(body any, preExecVars, result map[string]bool) {
	switch v := body.(type) {
	case string:
		for k := range ScanVariables(v, preExecVars) {
			result[k] = true
		}
	case map[string]any:
		for _, val := range v {
			scanBody(val, preExecVars, result)
		}
	case map[string]string:
		for _, val := range v {
			for k := range ScanVariables(val, preExecVars) {
				result[k] = true
			}
		}
	case []any:
		for _, val := range v {
			scanBody(val, preExecVars, result)
		}
	}
}

// ExtractProducedVars returns the set of variable names from extract blocks.
// Returns an error string if any extract key contains {{.
func ExtractProducedVars(extract map[string]string) (map[string]bool, string) {
	result := make(map[string]bool)
	if extract == nil {
		return result, ""
	}
	for key := range extract {
		if strings.Contains(key, "{{") {
			return nil, fmt.Sprintf("Dynamic variable names in extract keys are not supported: %q", key)
		}
		result[key] = true
	}
	return result, ""
}
