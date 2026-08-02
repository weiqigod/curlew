package datadriven

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
)

// loadCSV reads a CSV file and returns a DataSet. The first row is treated as
// the header (column names). All subsequent rows become data rows.
// Returns ErrEmptyDataFile if the file has no header or no data rows.
// Returns ErrMalformedData if any data row has a different number of fields
// than the header.
func loadCSV(path string) (*DataSet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open CSV: %w", err)
	}
	defer func() { _ = f.Close() }()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1 // allow variable fields so we can validate ourselves

	header, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("%w: no header row", ErrEmptyDataFile)
		}
		return nil, fmt.Errorf("%w: reading header: %w", ErrMalformedData, err)
	}

	columns := make([]string, len(header))
	copy(columns, header)

	var rows []Row
	for {
		record, readErr := reader.Read()
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, fmt.Errorf("%w: %w", ErrMalformedData, readErr)
		}

		if len(record) != len(columns) {
			return nil, fmt.Errorf("%w: row has %d fields, header has %d columns",
				ErrMalformedData, len(record), len(columns))
		}

		row := make(Row, len(columns))
		for i, col := range columns {
			row[col] = record[i]
		}
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: header present but no data rows", ErrEmptyDataFile)
	}

	return &DataSet{
		Rows:    rows,
		Columns: columns,
	}, nil
}
