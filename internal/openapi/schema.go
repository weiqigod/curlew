package openapi

import (
	"fmt"
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
)

// schemaWalker generates placeholder values from resolved OpenAPI 3 schemas.
// It detects $ref cycles via a visited-set passed through recursion and emits
// a warning (via warn) on first occurrence of each cycle.
type schemaWalker struct {
	warn       func(string)
	warnedRefs map[string]bool
}

// placeholder returns a Go value suitable for YAML serialisation that represents
// a plausible placeholder for the given schema. It prefers example values when
// present, otherwise synthesises type-appropriate defaults.
//
// visited is the set of $ref names seen on the current recursion path; pass nil
// for the root call. It is cloned before descent so sibling branches do not
// pollute each other's cycle sets.
func (w *schemaWalker) placeholder(ref *openapi3.SchemaRef, visited map[string]bool) any {
	if ref == nil || ref.Value == nil {
		return "string"
	}

	// Cycle detection: if we've already descended through this named ref on the
	// current path, stop and return a placeholder.
	refName := ref.Ref
	if refName != "" {
		if visited[refName] {
			msg := fmt.Sprintf("recursive $ref detected: %s — using placeholder", refName)
			if !w.warnedRefs[refName] {
				w.warnedRefs[refName] = true
				w.warn(msg)
			}
			return "string"
		}
		// Clone visited before descending so sibling branches are independent.
		next := make(map[string]bool, len(visited)+1)
		for k, v := range visited {
			next[k] = v
		}
		next[refName] = true
		visited = next
	}

	s := ref.Value

	// Prefer explicit example on the schema.
	if s.Example != nil {
		return s.Example
	}
	// Prefer first enum value.
	if len(s.Enum) > 0 {
		return s.Enum[0]
	}

	// allOf: merge all object sub-schemas (non-objects are ignored).
	if len(s.AllOf) > 0 {
		merged := map[string]any{}
		for _, sub := range s.AllOf {
			if v, ok := w.placeholder(sub, visited).(map[string]any); ok {
				for k, val := range v {
					merged[k] = val
				}
			}
		}
		if len(merged) > 0 {
			return merged
		}
	}

	// oneOf/anyOf: use first branch.
	if len(s.OneOf) > 0 {
		return w.placeholder(s.OneOf[0], visited)
	}
	if len(s.AnyOf) > 0 {
		return w.placeholder(s.AnyOf[0], visited)
	}

	// Type-based dispatch.
	switch {
	case s.Type.Is("string"):
		return defaultForStringFormat(s.Format)
	case s.Type.Is("integer") || s.Type.Is("number"):
		return 0
	case s.Type.Is("boolean"):
		return false
	case s.Type.Is("array"):
		if s.Items != nil {
			return []any{w.placeholder(s.Items, visited)}
		}
		return []any{}
	default: // object (or untyped)
		if len(s.Properties) == 0 {
			return map[string]any{}
		}
		obj := make(map[string]any, len(s.Properties))
		for _, k := range sortedKeys(s.Properties) {
			obj[k] = w.placeholder(s.Properties[k], visited)
		}
		return obj
	}
}

// defaultForStringFormat returns a format-appropriate placeholder string.
func defaultForStringFormat(format string) string {
	switch format {
	case "date-time":
		return "2006-01-02T15:04:05Z"
	case "date":
		return "2006-01-02"
	case "uuid":
		return "00000000-0000-0000-0000-000000000000"
	case "email":
		return "user@example.com"
	case "uri":
		return "https://example.com"
	default:
		return "string"
	}
}

// sortedKeys returns the keys of a Properties map in lexicographic order.
func sortedKeys(m openapi3.Schemas) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
