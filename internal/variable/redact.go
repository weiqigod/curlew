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

// redactJSONString redacts the string literals of s, which must be a JSON
// object, array or string. Every other byte is copied, so key order, spacing
// and number spelling survive. Literals are decoded before matching, so a
// secret written with JSON escapes is still found. Returns ok=false if s is
// not valid JSON or does not start with a JSON-initiating character.
func redactJSONString(s string, values []string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", false
	}
	first := trimmed[0]
	if first != '{' && first != '[' && first != '"' {
		return "", false
	}
	if !json.Valid([]byte(trimmed)) {
		return "", false
	}

	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != '"' {
			out.WriteByte(s[i])
			i++
			continue
		}
		end := i + 1
		for s[end] != '"' {
			if s[end] == '\\' {
				end++
			}
			end++
		}
		literal := s[i : end+1]
		out.WriteString(redactJSONLiteral(literal, values))
		i = end + 1
	}
	return out.String(), true
}

// redactJSONLiteral returns literal unchanged unless its decoded text holds a
// sensitive value, in which case it returns the redacted text re-encoded.
func redactJSONLiteral(literal string, values []string) string {
	var text string
	if err := json.Unmarshal([]byte(literal), &text); err != nil {
		return literal
	}
	redacted := redactString(text, values)
	if redacted == text {
		return literal
	}
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(redacted); err != nil {
		return literal
	}
	return strings.TrimRight(buf.String(), "\n")
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
