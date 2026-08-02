package datadriven

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrUnsupportedFilter is returned when a type conversion filter is unknown.
var ErrUnsupportedFilter = errors.New("unsupported type conversion filter")

// ConvertValue applies a type conversion filter to a string value.
// Supported filters: "int", "float", "bool".
// Returns the converted value and any error.
func ConvertValue(value, filter string) (interface{}, error) {
	switch strings.ToLower(filter) {
	case "int":
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("convert %q to int: %w", value, err)
		}
		return n, nil

	case "float":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("convert %q to float: %w", value, err)
		}
		return f, nil

	case "bool":
		return parseBool(value)

	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFilter, filter)
	}
}

// TryNumeric attempts to parse a string as a float64.
// Returns the numeric value and true, or 0 and false.
func TryNumeric(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// parseBool converts common boolean string representations to bool.
// Accepts: true/false, yes/no, 1/0 (case-insensitive).
func parseBool(s string) (interface{}, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "1":
		return true, nil
	case "false", "no", "0":
		return false, nil
	default:
		return nil, fmt.Errorf("convert %q to bool: unrecognized boolean value", s)
	}
}
