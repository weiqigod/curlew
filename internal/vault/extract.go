package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// ErrFieldNotFound is returned when a field does not exist in the secret JSON.
var ErrFieldNotFound = errors.New("field not found in secret JSON")

// ErrNotJSONSecret is returned when the secret value is not valid JSON.
var ErrNotJSONSecret = errors.New("secret is not valid JSON for field extraction")

// ExtractField parses rawJSON as a JSON object and extracts fieldName.
// String values are returned directly; other types are JSON-encoded.
func ExtractField(rawJSON, fieldName string) (string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawJSON), &obj); err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotJSONSecret, err)
	}

	raw, ok := obj[fieldName]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrFieldNotFound, fieldName)
	}

	// Null value
	if string(raw) == "null" {
		return "", nil
	}

	// Try string first (most common for secrets)
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}

	// Try number
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String(), nil
	}

	// Try boolean
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return strconv.FormatBool(b), nil
	}

	// Nested object or array — return compact JSON
	compact, err := json.Marshal(json.RawMessage(raw))
	if err != nil {
		return "", fmt.Errorf("%w: cannot re-encode field %q", ErrNotJSONSecret, fieldName)
	}
	return string(compact), nil
}
