package datadriven

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// loadYAML reads a YAML file containing a list of maps and returns a DataSet.
// Each list item becomes a Row. Non-string values (numbers, booleans, nested
// objects, arrays) are converted to their string representation.
// Returns ErrEmptyDataFile if the list is empty or the file is empty.
// Returns ErrMalformedData if the file does not contain a YAML list of maps.
func loadYAML(path string) (*DataSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read YAML: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty YAML file", ErrEmptyDataFile)
	}

	var raw interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedData, err)
	}

	if raw == nil {
		return nil, fmt.Errorf("%w: empty YAML file", ErrEmptyDataFile)
	}

	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%w: expected YAML list, got %T", ErrMalformedData, raw)
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("%w: empty YAML list", ErrEmptyDataFile)
	}

	columnSet := make(map[string]struct{})
	rows := make([]Row, 0, len(items))

	for i, item := range items {
		obj, isMap := item.(map[string]interface{})
		if !isMap {
			return nil, fmt.Errorf("%w: element %d is not a map", ErrMalformedData, i)
		}

		row := make(Row, len(obj))
		for k, v := range obj {
			columnSet[k] = struct{}{}
			row[k] = yamlValueToString(v)
		}
		rows = append(rows, row)
	}

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

// yamlValueToString converts a YAML value to its string representation.
// Strings are returned as-is. Numbers and booleans are formatted.
// Nil becomes empty string. Nested structures are JSON-encoded.
func yamlValueToString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		// Use compact representation: avoid trailing zeros
		s := fmt.Sprintf("%g", val)
		return s
	case bool:
		if val {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		// Nested maps, slices, etc. -> JSON encode
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}
