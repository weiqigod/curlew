package datadriven

import (
	"errors"
	"fmt"
	"testing"
)

func intPtr(n int) *int { return &n }

func makeRows(n int) []Row {
	rows := make([]Row, n)
	for i := range rows {
		rows[i] = Row{"idx": fmt.Sprintf("%d", i)}
	}
	return rows
}

func makeAgeRows() []Row {
	return []Row{
		{"name": "alice", "age": "25"},
		{"name": "bob", "age": "15"},
		{"name": "carol", "age": "30"},
		{"name": "dave", "age": "12"},
		{"name": "eve", "age": "18"},
	}
}

func TestApplyControls(t *testing.T) {
	tests := []struct {
		name     string
		rows     []Row
		cfg      Config
		wantRows int
		wantErr  error
	}{
		{"no controls", makeRows(10), Config{}, 10, nil},
		{"limit 5", makeRows(10), Config{Limit: intPtr(5)}, 5, nil},
		{"limit exceeds rows", makeRows(3), Config{Limit: intPtr(10)}, 3, nil},
		{"limit zero", makeRows(10), Config{Limit: intPtr(0)}, 0, nil},
		{"start_row 3", makeRows(10), Config{StartRow: intPtr(3)}, 7, nil},
		{"end_row 5", makeRows(10), Config{EndRow: intPtr(5)}, 6, nil},
		{"start_row and end_row", makeRows(10), Config{StartRow: intPtr(2), EndRow: intPtr(5)}, 4, nil},
		{"start_row beyond length", makeRows(5), Config{StartRow: intPtr(10)}, 0, nil},
		{"end_row before start_row", makeRows(10), Config{StartRow: intPtr(5), EndRow: intPtr(3)}, 0, nil},
		{"filter age >= 18", makeAgeRows(), Config{Filter: "{{age}} >= 18"}, 3, nil},
		{"filter then limit", makeAgeRows(), Config{Filter: "{{age}} >= 18", Limit: intPtr(2)}, 2, nil},
		{"filter with invalid expression", makeRows(5), Config{Filter: "{{x}} LIKE 'a'"}, 0, ErrInvalidFilter},
		{"range then limit", makeRows(20), Config{StartRow: intPtr(5), EndRow: intPtr(14), Limit: intPtr(3)}, 3, nil},
		{"negative start_row clamped to zero", makeRows(10), Config{StartRow: intPtr(-1)}, 10, nil},
		{"negative end_row returns empty", makeRows(10), Config{EndRow: intPtr(-1)}, 0, nil},
		{"negative start_row and end_row returns empty", makeRows(10), Config{StartRow: intPtr(-3), EndRow: intPtr(-1)}, 0, nil},
		{"negative start_row with valid end_row", makeRows(10), Config{StartRow: intPtr(-5), EndRow: intPtr(4)}, 5, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &DataSet{Rows: tt.rows, Columns: []string{"idx"}}
			result, err := ApplyControls(ds, tt.cfg)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.Rows) != tt.wantRows {
				t.Errorf("rows = %d, want %d", len(result.Rows), tt.wantRows)
			}
		})
	}
}

func TestApplyControls_FilterPreservesOrder(t *testing.T) {
	rows := makeAgeRows() // alice(25), bob(15), carol(30), dave(12), eve(18)
	ds := &DataSet{Rows: rows, Columns: []string{"name", "age"}}
	result, err := ApplyControls(ds, Config{Filter: "{{age}} >= 18"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result.Rows))
	}
	expected := []string{"alice", "carol", "eve"}
	for i, name := range expected {
		if result.Rows[i]["name"] != name {
			t.Errorf("row[%d].name = %q, want %q", i, result.Rows[i]["name"], name)
		}
	}
}

func TestApplyControls_DoesNotMutateOriginal(t *testing.T) {
	rows := makeRows(10)
	ds := &DataSet{Rows: rows, Columns: []string{"idx"}}
	result, err := ApplyControls(ds, Config{Limit: intPtr(3)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rows) != 3 {
		t.Errorf("result rows = %d, want 3", len(result.Rows))
	}
	if len(ds.Rows) != 10 {
		t.Errorf("original rows = %d, want 10 (should not be mutated)", len(ds.Rows))
	}
}
