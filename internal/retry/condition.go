package retry

import (
	"fmt"
	"strconv"
	"strings"
)

// ShouldRetryInput captures the state needed to evaluate retry conditions.
type ShouldRetryInput struct {
	Method       string
	StatusCode   int // 0 when no HTTP response (network error / timeout)
	IsNetworkErr bool
	IsTimeout    bool
}

// ShouldRetryResult holds the retry decision and optional warning.
type ShouldRetryResult struct {
	Retry   bool
	Warning string // non-empty when retrying a non-idempotent method
}

// nonIdempotentMethods are HTTP methods that are not idempotent.
var nonIdempotentMethods = map[string]bool{
	"POST": true, "PATCH": true,
}

// ShouldRetry evaluates the combined condition logic per spec:
//
//	(status_code IN retry_on.status_codes OR status_code IN retry_on.status_ranges
//	 OR network_error AND retry_on.network_errors
//	 OR timeout AND retry_on.timeouts)
//	AND HTTP_method IN retry_on.methods
//	AND NOT (status_code IN do_not_retry_on.status_codes OR status_code IN do_not_retry_on.status_ranges
//	         OR HTTP_method IN do_not_retry_on.methods)
//
// When retryOn is nil, uses DefaultRetriableStatusCodes and DefaultIdempotentMethods.
func ShouldRetry(input ShouldRetryInput, retryOn *RetryOnConfig, doNotRetryOn *DoNotRetryOnConfig) ShouldRetryResult {
	// Fall back to defaults when no retry_on config.
	if retryOn == nil {
		defaults := DefaultRetriableStatusCodes()
		methods := DefaultIdempotentMethods()
		triggered := defaults[input.StatusCode] || input.IsNetworkErr || input.IsTimeout
		retry := methods[input.Method] && triggered
		return ShouldRetryResult{Retry: retry}
	}

	// 1. Check trigger conditions (OR logic).
	triggered := false
	if input.StatusCode > 0 {
		triggered = statusInCodes(input.StatusCode, retryOn.StatusCodes) ||
			StatusInRanges(input.StatusCode, retryOn.StatusRanges)
	}
	if !triggered && input.IsNetworkErr && retryOn.NetworkErrors != nil && *retryOn.NetworkErrors {
		triggered = true
	}
	if !triggered && input.IsTimeout && retryOn.Timeouts != nil && *retryOn.Timeouts {
		triggered = true
	}
	if !triggered {
		return ShouldRetryResult{Retry: false}
	}

	// 2. Check method restriction (AND).
	if !methodAllowed(input.Method, retryOn.Methods) {
		return ShouldRetryResult{Retry: false}
	}

	// 3. Check exclusions (AND NOT).
	if doNotRetryOn != nil {
		if input.StatusCode > 0 &&
			(statusInCodes(input.StatusCode, doNotRetryOn.StatusCodes) ||
				StatusInRanges(input.StatusCode, doNotRetryOn.StatusRanges)) {
			return ShouldRetryResult{Retry: false}
		}
		if methodExcluded(input.Method, doNotRetryOn.Methods) {
			return ShouldRetryResult{Retry: false}
		}
	}

	// 4. Warning for non-idempotent methods.
	var warning string
	if nonIdempotentMethods[input.Method] {
		warning = fmt.Sprintf("retrying non-idempotent method %s", input.Method)
	}

	return ShouldRetryResult{Retry: true, Warning: warning}
}

// ParseStatusRange parses a status range into two ints. Two forms are
// accepted: explicit "min-max" (e.g. "500-599") and the class shorthand
// "Nxx" (e.g. "5xx" → 500-599), case-insensitive.
func ParseStatusRange(s string) (int, int, error) {
	if s == "" {
		return 0, 0, fmt.Errorf("invalid status range: empty string")
	}
	// Class shorthand: a single digit 1-9 followed by "xx".
	if len(s) == 3 && strings.EqualFold(s[1:], "xx") {
		if s[0] < '1' || s[0] > '9' {
			return 0, 0, fmt.Errorf("invalid status range %q: class digit must be 1-9", s)
		}
		min := int(s[0]-'0') * 100
		return min, min + 99, nil
	}
	// Guard against "500-599-600" by checking for additional dashes.
	if strings.Count(s, "-") != 1 {
		return 0, 0, fmt.Errorf("invalid status range %q: expected exactly one dash", s)
	}
	parts := strings.SplitN(s, "-", 2)
	min, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid status range %q: min: %w", s, err)
	}
	max, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid status range %q: max: %w", s, err)
	}
	if min < 0 || max < 0 {
		return 0, 0, fmt.Errorf("invalid status range %q: negative values", s)
	}
	if min > max {
		return 0, 0, fmt.Errorf("invalid status range %q: min > max", s)
	}
	return min, max, nil
}

// StatusInRanges checks whether code falls within any of the given ranges.
func StatusInRanges(code int, ranges []string) bool {
	for _, r := range ranges {
		min, max, err := ParseStatusRange(r)
		if err != nil {
			continue // skip invalid ranges
		}
		if code >= min && code <= max {
			return true
		}
	}
	return false
}

func statusInCodes(code int, codes []int) bool {
	for _, c := range codes {
		if c == code {
			return true
		}
	}
	return false
}

func methodAllowed(method string, methods []string) bool {
	if len(methods) == 0 {
		// No methods specified = default idempotent methods.
		return DefaultIdempotentMethods()[method]
	}
	for _, m := range methods {
		if strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}

func methodExcluded(method string, methods []string) bool {
	for _, m := range methods {
		if strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}
