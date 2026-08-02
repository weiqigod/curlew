package variable

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/jsonpath"
)

// Sentinel errors for variable extraction failures.
var (
	ErrExtractionFailed = errors.New("variable extraction failed")
	ErrNotJSON          = errors.New("response body is not JSON")
)

// ExtractionInput defines what to extract from a response.
type ExtractionInput struct {
	Extractions map[string]string // variable name -> JSONPath expression
	Body        []byte            // response body
}

// ExtractionResult holds the outcome of variable extraction.
type ExtractionResult struct {
	Variables map[string]string
}

// Extract evaluates JSONPath expressions against a response body and returns
// extracted variables as strings.
func Extract(input ExtractionInput) (*ExtractionResult, error) {
	if len(input.Extractions) == 0 {
		return &ExtractionResult{Variables: map[string]string{}}, nil
	}

	var doc any
	if err := json.Unmarshal(input.Body, &doc); err != nil {
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryConfig,
			Message:  "cannot extract variables: response body is not valid JSON",
			Hint:     "Extraction requires a JSON response body",
			Inner:    ErrNotJSON,
		}
	}

	result := &ExtractionResult{Variables: make(map[string]string, len(input.Extractions))}

	// Process in sorted order for deterministic error messages
	names := make([]string, 0, len(input.Extractions))
	for name := range input.Extractions {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		path := input.Extractions[name]
		val, err := jsonpath.Evaluate(path, doc)
		if err != nil {
			return nil, &apierrors.Structured{
				Category: apierrors.CategoryConfig,
				Message:  fmt.Sprintf("extraction failed for variable %q at path %s: no match", name, path),
				Hint:     "Check that the JSONPath expression matches the response structure",
				Inner:    ErrExtractionFailed,
			}
		}
		result.Variables[name] = stringify(val)
	}

	return result, nil
}

// stringify converts an extracted value to a string for variable storage.
func stringify(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case nil:
		return ""
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		data, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(data)
	}
}
