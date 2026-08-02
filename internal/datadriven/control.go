package datadriven

import "fmt"

// ApplyControls filters and slices a DataSet according to the Config settings.
// Order of operations: filter -> start_row/end_row range -> limit.
// Returns a new DataSet; the original is not modified.
func ApplyControls(ds *DataSet, cfg Config) (*DataSet, error) {
	rows := ds.Rows

	// Step 1: Apply filter
	if cfg.Filter != "" {
		filtered, err := applyFilter(rows, cfg.Filter)
		if err != nil {
			return nil, err
		}
		rows = filtered
	}

	// Step 2: Apply range (start_row / end_row)
	rows = applyRange(rows, cfg.StartRow, cfg.EndRow)

	// Step 3: Apply limit
	rows = applyLimit(rows, cfg.Limit)

	return &DataSet{
		Rows:    rows,
		Columns: ds.Columns,
	}, nil
}

// applyFilter evaluates the filter expression against each row and returns
// only matching rows. Preserves original order.
func applyFilter(rows []Row, expr string) ([]Row, error) {
	result := make([]Row, 0, len(rows))
	for i, row := range rows {
		match, err := EvalFilter(expr, row)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i, err)
		}
		if match {
			result = append(result, row)
		}
	}
	return result, nil
}

// applyRange slices rows by start_row and end_row (0-based, inclusive).
// If start_row > len(rows), returns empty. If end_row < start_row, returns empty.
func applyRange(rows []Row, startRow, endRow *int) []Row {
	start := 0
	end := len(rows) - 1

	if startRow != nil {
		start = *startRow
	}
	if endRow != nil {
		end = *endRow
	}

	// Clamp negative values to zero.
	if start < 0 {
		start = 0
	}
	if end < 0 {
		return nil
	}

	if start >= len(rows) || start > end {
		return nil
	}

	if end >= len(rows) {
		end = len(rows) - 1
	}

	return rows[start : end+1]
}

// applyLimit caps the number of rows to the given limit.
// A nil limit means no cap. A zero limit returns empty.
func applyLimit(rows []Row, limit *int) []Row {
	if limit == nil {
		return rows
	}
	n := *limit
	if n <= 0 {
		return nil
	}
	if n >= len(rows) {
		return rows
	}
	return rows[:n]
}
