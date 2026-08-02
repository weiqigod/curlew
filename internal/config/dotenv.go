package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/peterlindqvist/apitest/internal/variable"
)

// ErrInvalidDotenv indicates the .env file contains malformed lines.
var ErrInvalidDotenv = errors.New("invalid .env file")

// ParseDotenv parses .env format content from a byte slice.
// Lines starting with '#' are comments. Empty lines are ignored.
// Values may be optionally quoted (single or double quotes stripped).
// Lines prefixed with "!sensitive " mark the variable as sensitive.
// Returns ErrInvalidDotenv for malformed lines (no '=' or empty key).
func ParseDotenv(data []byte) (map[string]string, *variable.SensitiveSet, error) {
	result := make(map[string]string)
	sensitive := variable.NewSensitiveSet()
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		isSensitive := false
		if strings.HasPrefix(trimmed, "!sensitive ") {
			isSensitive = true
			trimmed = strings.TrimPrefix(trimmed, "!sensitive ")
		}
		eqIdx := strings.Index(trimmed, "=")
		if eqIdx < 0 {
			return nil, nil, fmt.Errorf("%w: line %d: %q", ErrInvalidDotenv, i+1, trimmed)
		}
		key := strings.TrimSpace(trimmed[:eqIdx])
		if key == "" {
			return nil, nil, fmt.Errorf("%w: line %d: empty key", ErrInvalidDotenv, i+1)
		}
		value := strings.TrimSpace(trimmed[eqIdx+1:])
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		result[key] = value
		if isSensitive {
			sensitive.Add(key)
		}
	}
	return result, sensitive, nil
}

// LoadDotenv loads a .env file from dir. Returns empty map and nil error
// if the file does not exist. Returns error if file exists but cannot be parsed.
func LoadDotenv(dir string) (map[string]string, *variable.SensitiveSet, error) {
	path := filepath.Join(dir, ".env")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]string), variable.NewSensitiveSet(), nil
		}
		return nil, nil, fmt.Errorf("reading .env file: %w", err)
	}
	return ParseDotenv(data)
}
