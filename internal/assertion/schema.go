package assertion

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// ErrAssertionFailed is the sentinel used when a request.end event is emitted
// for a request that failed due to assertion mismatches (not a network error).
// Registering it allows the error classification system to produce a structured
// error block with category=assertion, code=ASSERTION_FAILED, and a concrete hint.
var ErrAssertionFailed = errors.New("assertion failed")

// ErrSchemaFileNotFound is returned when a schema path cannot be resolved.
var ErrSchemaFileNotFound = errors.New("schema file not found")

// ErrSchemaInvalid is returned when a schema file is not a valid JSON Schema.
var ErrSchemaInvalid = errors.New("invalid JSON Schema")

// CompiledSchema is an opaque wrapper around a compiled JSON Schema.
// It is produced once per unique schema file and reused across every
// request (and every parallel/data-driven iteration) that references it.
// CompiledSchema is safe for concurrent use by multiple goroutines.
type CompiledSchema struct {
	path string             // absolute path, for error messages
	sch  *jsonschema.Schema // compiled schema
}

// Path returns the absolute path of the source schema file.
func (c *CompiledSchema) Path() string { return c.path }

// CompileSchemaFile reads absPath and compiles it as a JSON Schema.
// Returns ErrSchemaFileNotFound when the file is missing or
// ErrSchemaInvalid (with a wrapped cause) when the file is not a valid
// JSON Schema document. The returned *CompiledSchema is safe for
// concurrent use by multiple goroutines.
func CompileSchemaFile(absPath string) (*CompiledSchema, error) {
	f, err := os.Open(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrSchemaFileNotFound, absPath)
		}
		return nil, fmt.Errorf("reading schema %s: %w", absPath, err)
	}

	doc, err := jsonschema.UnmarshalJSON(f)
	if closeErr := f.Close(); closeErr != nil {
		return nil, fmt.Errorf("closing schema %s: %w", absPath, closeErr)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrSchemaInvalid, absPath, err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource(absPath, doc); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrSchemaInvalid, absPath, err)
	}

	sch, err := c.Compile(absPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrSchemaInvalid, absPath, err)
	}

	return &CompiledSchema{path: absPath, sch: sch}, nil
}

// CheckSchema validates body against compiled. Returns nil when compiled
// is nil (no assertion configured). On success returns a single passing
// Result; on failure returns one Result per leaf validation error. When
// body is not valid JSON a single failing Result is returned with
// Actual = "response body is not JSON".
func CheckSchema(compiled *CompiledSchema, body []byte) []Result {
	if compiled == nil {
		return nil
	}

	typeTag := fmt.Sprintf("schema %s", compiled.path)

	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return []Result{{
			Type:     typeTag,
			Expected: "valid JSON",
			Actual:   "response body is not JSON",
			Passed:   false,
		}}
	}

	err := compiled.sch.Validate(doc)
	if err == nil {
		return []Result{{
			Type:     typeTag,
			Expected: "valid",
			Actual:   "valid",
			Passed:   true,
		}}
	}

	var verr *jsonschema.ValidationError
	if !errors.As(err, &verr) {
		// Defensive fallback: the jsonschema/v6 library always returns a
		// *ValidationError from Schema.Validate, so this branch is
		// unreachable in practice. It is retained to guard against future
		// library changes that might introduce other error types.
		return []Result{{
			Type:     typeTag,
			Expected: "valid",
			Actual:   err.Error(),
			Passed:   false,
		}}
	}

	var results []Result
	for _, leaf := range flattenLeafErrors(verr) {
		results = append(results, leafToResult(leaf, doc))
	}
	if len(results) == 0 {
		results = append(results, Result{
			Type:     typeTag,
			Expected: "valid",
			Actual:   "validation failed",
			Passed:   false,
		})
	}
	return results
}

// flattenLeafErrors walks ValidationError.Causes depth-first and yields
// nodes with no further Causes. Each returned node corresponds to a
// single failing schema keyword at a single instance location.
func flattenLeafErrors(e *jsonschema.ValidationError) []*jsonschema.ValidationError {
	if len(e.Causes) == 0 {
		return []*jsonschema.ValidationError{e}
	}
	var out []*jsonschema.ValidationError
	for _, c := range e.Causes {
		out = append(out, flattenLeafErrors(c)...)
	}
	return out
}

// leafToResult converts a single ValidationError leaf to an assertion.Result.
// For "type" errors it names the expected + actual type and value.
// For "required" errors it names the missing property.
// For all other kinds it falls back to the library's LocalizedString.
func leafToResult(leaf *jsonschema.ValidationError, root any) Result {
	path := toJSONPath(leaf.InstanceLocation)
	typeTag := fmt.Sprintf("schema %s", path)

	switch k := leaf.ErrorKind.(type) {
	case *kind.Type:
		val, _ := lookupAt(root, leaf.InstanceLocation)
		return Result{
			Type:     typeTag,
			Expected: fmt.Sprintf("type %s", strings.Join(k.Want, " or ")),
			Actual:   fmt.Sprintf("%s (%v)", k.Got, val),
			Passed:   false,
		}
	case *kind.Required:
		missing := strings.Join(k.Missing, ", ")
		return Result{
			Type:     typeTag,
			Expected: fmt.Sprintf("required: %s", missing),
			Actual:   "missing",
			Passed:   false,
		}
	default:
		val, found := lookupAt(root, leaf.InstanceLocation)
		actual := "missing"
		if found {
			actual = fmt.Sprintf("%v", val)
		}
		// Use leaf.Error() (which uses the library's default English printer)
		// rather than calling ErrorKind.LocalizedString(nil), which panics for
		// kinds (e.g. Minimum, Maximum, Enum) that use x/text/message.Printer.
		return Result{
			Type:     typeTag,
			Expected: leaf.Error(),
			Actual:   actual,
			Passed:   false,
		}
	}
}

// toJSONPath converts a ValidationError.InstanceLocation slice into a
// JSONPath-style string. Numeric tokens become "[N]" (array index),
// all other tokens become ".key". Empty input yields "$".
func toJSONPath(loc []string) string {
	var b strings.Builder
	b.WriteByte('$')
	for _, tok := range loc {
		if _, err := strconv.Atoi(tok); err == nil {
			b.WriteByte('[')
			b.WriteString(tok)
			b.WriteByte(']')
			continue
		}
		b.WriteByte('.')
		b.WriteString(tok)
	}
	return b.String()
}

// lookupAt walks root by the given InstanceLocation tokens and returns
// the value at that position, or (nil, false) when the path does not
// resolve (e.g. the error is about a missing required field).
func lookupAt(root any, loc []string) (any, bool) {
	cur := root
	for _, tok := range loc {
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[tok]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(tok)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			cur = v[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}
