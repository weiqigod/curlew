package variable

import (
	"bytes"
	"encoding/json"
	"strings"
)

// RedactBody returns a copy of body with every occurrence of any registered
// sensitive value replaced by Redacted. It supports:
//
//   - nil                           -> returned unchanged
//   - string                        -> substring-replaced (or JSON-walked if valid JSON)
//   - []byte                        -> substring-replaced (or JSON-walked if valid JSON)
//   - map[string]any / []any / etc. -> walked recursively; string leaves replaced
//
// When allowSensitive is true or the sensitive set has no registered values,
// body is returned unchanged. The returned shape mirrors the input shape:
// string-in -> string-out, []byte-in -> []byte-out, structured-in -> structured-out.
func RedactBody(body any, s *SensitiveSet, allowSensitive bool) any {
	if body == nil || allowSensitive {
		return body
	}
	values := s.Values()
	if len(values) == 0 {
		return body
	}
	switch v := body.(type) {
	case string:
		if redacted, ok := redactJSONString(v, values); ok {
			return redacted
		}
		return redactString(v, values)
	case []byte:
		if redacted, ok := redactJSONBytes(v, values); ok {
			return redacted
		}
		return []byte(redactString(string(v), values))
	default:
		return redactStructured(body, values)
	}
}

// RedactText replaces every registered sensitive value inside s with
// [REDACTED]. Unlike RedactBody it never reinterprets the string as JSON, so it
// is the right tool for output that is already prose: an assertion's expected or
// actual value, a URL, a header line, an error message.
func RedactText(s string, set *SensitiveSet, allowSensitive bool) string {
	if s == "" || allowSensitive {
		return s
	}
	values := set.Values()
	if len(values) == 0 {
		return s
	}
	return redactString(s, values)
}

// redactString performs exact-substring replacement of each sensitive value.
func redactString(s string, values []string) string {
	for _, v := range values {
		if strings.Contains(s, v) {
			s = strings.ReplaceAll(s, v, Redacted)
		}
	}
	return s
}

// redactJSONString attempts to parse s as JSON. If it parses and yields an
// object, array, or string literal, walks it and returns the re-encoded JSON
// with sensitive string leaves replaced. Returns ok=false if s is not valid
// JSON or does not start with a JSON-initiating character.
func redactJSONString(s string, values []string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", false
	}
	first := trimmed[0]
	if first != '{' && first != '[' && first != '"' {
		return "", false
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return "", false
	}
	walked := redactStructured(parsed, values)
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	// enc.Encode cannot fail for values produced by json.Unmarshal
	// (map[string]any, []any, string, float64, bool, nil) — all are
	// JSON-encodable. The error return is retained for future safety should
	// the type set ever expand.
	if err := enc.Encode(walked); err != nil {
		return "", false
	}
	// json.Encoder appends a trailing newline; strip to preserve original shape.
	return strings.TrimRight(buf.String(), "\n"), true
}

// redactJSONBytes is the []byte twin of redactJSONString.
func redactJSONBytes(b []byte, values []string) ([]byte, bool) {
	s, ok := redactJSONString(string(b), values)
	if !ok {
		return nil, false
	}
	return []byte(s), true
}

// redactStructured walks maps, slices, and primitives, rewriting string leaves
// that contain a sensitive value.
func redactStructured(v any, values []string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			out[k] = redactStructured(child, values)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(t))
		for k, child := range t {
			out[k] = redactString(child, values)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = redactStructured(child, values)
		}
		return out
	case string:
		return redactString(t, values)
	default:
		return v
	}
}
