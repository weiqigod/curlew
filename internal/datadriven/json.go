package datadriven

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// loadJSON reads a JSON file containing an array of objects and returns a DataSet.
// Each object in the array becomes a Row. Non-string values (numbers, booleans,
// nested objects, arrays) are JSON-encoded to their string representation.
// Returns ErrEmptyDataFile if the array is empty.
// Returns ErrMalformedData if the file does not contain a JSON array or is invalid JSON.
func loadJSON(path string) (*DataSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read JSON: %w", err)
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		// Check if it's a non-array value
		var obj map[string]interface{}
		if json.Unmarshal(data, &obj) == nil {
			return nil, fmt.Errorf("%w: expected JSON array, got object", ErrMalformedData)
		}
		return nil, fmt.Errorf("%w: %w", ErrMalformedData, err)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: empty JSON array", ErrEmptyDataFile)
	}

	// Collect all unique column names across all objects for consistent ordering.
	columnSet := make(map[string]struct{})
	rows := make([]Row, 0, len(raw))

	for i, item := range raw {
		var obj map[string]interface{}
		if err := json.Unmarshal(item, &obj); err != nil {
			return nil, fmt.Errorf("%w: element %d is not an object: %w", ErrMalformedData, i, err)
		}

		row := make(Row, len(obj))
		for k, v := range obj {
			columnSet[k] = struct{}{}
			row[k] = jsonValueToString(v)
		}
		rows = append(rows, row)
	}

	// Sort columns for deterministic ordering
	columns := make([]string, 0, len(columnSet))
	for c := range columnSet {
		columns = append(columns, c)
	}
	sort.Strings(columns)

	return &DataSet{
		Rows:    rows,
		Columns: columns,
	}, nil
}

// jsonValueToString converts a JSON value to its string representation.
// Strings are returned as-is. All other types are JSON-encoded.
func jsonValueToString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		// Use compact representation
		b, _ := json.Marshal(val)
		return string(b)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		// Nested objects, arrays, etc.
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}
