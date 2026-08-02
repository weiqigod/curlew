// Package graphql provides GraphQL protocol support for the API testing tool.
// It transforms GraphQL-specific request configuration into standard HTTP POST
// requests and checks responses for GraphQL-level errors.
package graphql

import (
	"encoding/json"
	"fmt"

	"github.com/weiqigod/curlew/internal/parser"
)

// ErrorHandlingMode controls how GraphQL errors in responses are treated.
type ErrorHandlingMode string

const (
	// ErrorHandlingFail fails the test when GraphQL errors are present (default).
	ErrorHandlingFail ErrorHandlingMode = "fail"
	// ErrorHandlingWarn logs a warning but does not fail the test.
	ErrorHandlingWarn ErrorHandlingMode = "warn"
	// ErrorHandlingIgnore silently ignores partial-success GraphQL errors.
	ErrorHandlingIgnore ErrorHandlingMode = "ignore"
)

// RequestBody is the JSON structure sent as the GraphQL HTTP POST body.
type RequestBody struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// BuildRequest transforms a GraphQL request config into HTTP-compatible request fields.
// It sets Method to POST, serializes query+variables as JSON body, and sets Content-Type.
// Returns a new request; the input is not mutated.
func BuildRequest(req *parser.Request) (*parser.Request, error) {
	if req.GraphQL == nil {
		return nil, ErrMissingConfig
	}
	if req.GraphQL.Query == "" {
		return nil, ErrEmptyQuery
	}

	body := RequestBody{
		Query:     req.GraphQL.Query,
		Variables: req.GraphQL.Variables,
	}
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("serializing graphql body: %w", err)
	}

	out := *req // shallow copy
	out.Method = "POST"
	out.Body = string(jsonBytes)
	// Deep-copy the headers map to avoid mutating the caller's map.
	newHeaders := make(map[string]string, len(out.Headers)+1)
	for k, v := range out.Headers {
		newHeaders[k] = v
	}
	if _, ok := newHeaders["Content-Type"]; !ok {
		newHeaders["Content-Type"] = "application/json"
	}
	out.Headers = newHeaders
	return &out, nil
}

// ParseErrorHandling returns the error handling mode from the config string.
// Empty defaults to fail. Accepts "fail", "warn", "ignore".
func ParseErrorHandling(mode string) (ErrorHandlingMode, error) {
	switch mode {
	case "", "fail":
		return ErrorHandlingFail, nil
	case "warn":
		return ErrorHandlingWarn, nil
	case "ignore":
		return ErrorHandlingIgnore, nil
	default:
		return "", fmt.Errorf("%w: %q (allowed: fail, warn, ignore)", ErrInvalidErrorHandling, mode)
	}
}

// Outcome classifies a GraphQL response against the three spec scenarios.
type Outcome int

const (
	// OutcomeEmpty means no body or no recognisable data/errors fields.
	OutcomeEmpty Outcome = iota
	// OutcomeSuccess means data is present and errors are absent.
	OutcomeSuccess
	// OutcomePartialSuccess means both data and errors are present.
	OutcomePartialSuccess
	// OutcomeFullFailure means errors are present and data is absent or null.
	OutcomeFullFailure
)

// ClassifyOutcome maps a parsed ResponseCheck into a spec scenario.
// ClassifyOutcome(nil) returns OutcomeEmpty.
func ClassifyOutcome(c *ResponseCheck) Outcome {
	if c == nil || (!c.HasData && !c.HasErrors) {
		return OutcomeEmpty
	}
	switch {
	case c.HasErrors && c.HasData:
		return OutcomePartialSuccess
	case c.HasErrors && !c.HasData:
		return OutcomeFullFailure
	default:
		return OutcomeSuccess
	}
}

// GraphQLError represents a single error from a GraphQL response's errors array.
type GraphQLError struct {
	Message string `json:"message"`
}

// ResponseCheck holds the result of checking a GraphQL response for errors.
type ResponseCheck struct {
	HasErrors bool           // true if $.errors array is non-empty
	HasData   bool           // true if $.data is non-null
	Errors    []GraphQLError // parsed error objects (if any)
}

// CheckResponse examines a response body for GraphQL-level errors.
// It parses the JSON and checks for the presence of "errors" and "data" fields.
func CheckResponse(body []byte) (*ResponseCheck, error) {
	if len(body) == 0 {
		return &ResponseCheck{}, nil
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing graphql response: %w", err)
	}

	check := &ResponseCheck{}

	// Check for errors array
	if errRaw, ok := parsed["errors"]; ok {
		var gqlErrors []GraphQLError
		if err := json.Unmarshal(errRaw, &gqlErrors); err == nil && len(gqlErrors) > 0 {
			check.HasErrors = true
			check.Errors = gqlErrors
		}
	}

	// Check for data field (data: null is not "has data")
	if dataRaw, ok := parsed["data"]; ok {
		if string(dataRaw) != "null" {
			check.HasData = true
		}
	}

	return check, nil
}
