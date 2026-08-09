// Package requtil provides shared request interpolation and assertion conversion
// helpers used by both the sequential runner and the parallel executor.
package requtil

import (
	"context"
	"fmt"
	"strings"

	"github.com/weiqigod/curlew/internal/assertion"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/variable"
)

// ExecuteFunc is the function signature for executing a single HTTP request.
type ExecuteFunc func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error)

// InterpolateRequest applies variable interpolation to all string fields of a request.
// BeginRequest/EndRequest bracket the operation to enable per-request function memoization.
func InterpolateRequest(scope *variable.Scope, req *parser.Request) (*parser.Request, error) {
	scope.BeginRequest()
	defer scope.EndRequest()

	out := *req // shallow copy

	var err error
	if out.URL, err = scope.Interpolate(out.URL); err != nil {
		return nil, fmt.Errorf("URL: %w", err)
	}
	if out.Headers, err = scope.InterpolateMap(out.Headers); err != nil {
		return nil, fmt.Errorf("headers: %w", err)
	}
	if out.QueryParams, err = scope.InterpolateMap(out.QueryParams); err != nil {
		return nil, fmt.Errorf("query params: %w", err)
	}

	// Resolve body_file / body_binary_file before Body interpolation so the rest
	// of the pipeline sees a single uniform Body (string or []byte). Parse time
	// has already loaded the file contents into BodyFileContent /
	// BodyBinaryFileContent; here we interpolate placeholders (text variant
	// only) and inject the auto-detected Content-Type header iff no explicit
	// Content-Type is already set.
	switch {
	case out.BodyBinaryFileContent != nil:
		// Binary: copy bytes so downstream mutations cannot reach the parser's
		// source of truth.
		buf := make([]byte, len(out.BodyBinaryFileContent))
		copy(buf, out.BodyBinaryFileContent)
		out.Body = buf
		out.Headers = injectContentType(out.Headers, out.BodyFileContentType)
	case out.BodyFileContent != "":
		interpolated, intErr := scope.Interpolate(out.BodyFileContent)
		if intErr != nil {
			return nil, fmt.Errorf("body_file: %w", intErr)
		}
		out.Body = interpolated
		out.Headers = injectContentType(out.Headers, out.BodyFileContentType)
	default:
		if out.Body, err = scope.InterpolateBody(out.Body); err != nil {
			return nil, fmt.Errorf("body: %w", err)
		}
	}

	// Interpolate WebSocket step fields (URL/headers already handled above).
	// Each step is deep-copied so the parsed collection is never mutated.
	if out.WebSocket != nil {
		ws := *out.WebSocket // shallow copy
		if len(ws.Steps) > 0 {
			steps := make([]parser.WebSocketStep, len(ws.Steps))
			for i, step := range ws.Steps {
				interpStep, stepErr := interpolateWebSocketStep(scope, step)
				if stepErr != nil {
					return nil, fmt.Errorf("websocket step %d: %w", i+1, stepErr)
				}
				steps[i] = interpStep
			}
			ws.Steps = steps
		}
		out.WebSocket = &ws
	}

	// Interpolate GraphQL fields (query string and variable values)
	if out.GraphQL != nil {
		gql := *out.GraphQL // shallow copy
		if gql.Query, err = scope.Interpolate(gql.Query); err != nil {
			return nil, fmt.Errorf("graphql query: %w", err)
		}
		if gql.Variables != nil {
			interpolatedVars := make(map[string]any, len(gql.Variables))
			for k, v := range gql.Variables {
				if sv, ok := v.(string); ok {
					iv, intErr := scope.Interpolate(sv)
					if intErr != nil {
						return nil, fmt.Errorf("graphql variable %q: %w", k, intErr)
					}
					interpolatedVars[k] = iv
				} else {
					interpolatedVars[k] = v
				}
			}
			gql.Variables = interpolatedVars
		}
		out.GraphQL = &gql
	}

	return &out, nil
}

// interpolateWebSocketStep returns a deep-copied WebSocketStep with all string
// fields (MessageRaw, Reason, Extract values) and every nested value inside the
// Message map interpolated. The input step is never mutated.
func interpolateWebSocketStep(scope *variable.Scope, step parser.WebSocketStep) (parser.WebSocketStep, error) {
	out := step // shallow copy of scalars

	if step.MessageRaw != "" {
		v, err := scope.Interpolate(step.MessageRaw)
		if err != nil {
			return out, fmt.Errorf("message_raw: %w", err)
		}
		out.MessageRaw = v
	}

	if step.Message != nil {
		v, err := scope.InterpolateBody(step.Message)
		if err != nil {
			return out, fmt.Errorf("message: %w", err)
		}
		// InterpolateBody returns a deep copy; cast back to map[string]any
		// (parser always decodes Message as a map, so the type is preserved).
		if m, ok := v.(map[string]any); ok {
			out.Message = m
		}
	}

	if step.Reason != "" {
		v, err := scope.Interpolate(step.Reason)
		if err != nil {
			return out, fmt.Errorf("reason: %w", err)
		}
		out.Reason = v
	}

	if len(step.Extract) > 0 {
		extracted, err := scope.InterpolateMap(step.Extract)
		if err != nil {
			return out, fmt.Errorf("extract: %w", err)
		}
		out.Extract = extracted
	}

	return out, nil
}

// ToHeaderInputs converts parser.HeaderAssertion to assertion.HeaderInput.
func ToHeaderInputs(items []parser.HeaderAssertion) []assertion.HeaderInput {
	if len(items) == 0 {
		return nil
	}
	inputs := make([]assertion.HeaderInput, len(items))
	for i, item := range items {
		inputs[i] = assertion.HeaderInput{
			Name:     item.Name,
			Operator: item.Operator,
			Value:    fmt.Sprint(item.Value),
			Line:     item.Line,
		}
	}
	return inputs
}

// ToBodyInputs converts parser.BodyAssertion to assertion.BodyInput.
func ToBodyInputs(items []parser.BodyAssertion) []assertion.BodyInput {
	if len(items) == 0 {
		return nil
	}
	inputs := make([]assertion.BodyInput, len(items))
	for i, item := range items {
		inputs[i] = assertion.BodyInput{
			Path:     item.Path,
			Operator: item.Operator,
			Value:    item.Value,
			Line:     item.Line,
		}
	}
	return inputs
}

// injectContentType returns a headers map with an auto-detected Content-Type
// added if contentType is non-empty and no Content-Type header (case-insensitive)
// is already present. The input map is never mutated.
func injectContentType(headers map[string]string, contentType string) map[string]string {
	if contentType == "" {
		return headers
	}
	for name := range headers {
		if strings.EqualFold(name, "Content-Type") {
			return headers
		}
	}
	out := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		out[k] = v
	}
	out["Content-Type"] = contentType
	return out
}

// ToHTTPRequest converts a parser.Request to an httpexec.Request.
func ToHTTPRequest(req *parser.Request) *httpexec.Request {
	return &httpexec.Request{
		Method:      req.Method,
		URL:         req.URL,
		Headers:     req.Headers,
		Body:        req.Body,
		QueryParams: req.QueryParams,
	}
}
