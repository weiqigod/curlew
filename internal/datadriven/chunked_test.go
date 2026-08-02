package datadriven

import (
	"testing"
)

func TestCheckLargeDataset_NilDataset(t *testing.T) {
	info := CheckLargeDataset(nil)
	if info != nil {
		t.Errorf("expected nil for nil dataset, got %+v", info)
	}
}

func TestCheckLargeDataset_BelowThreshold(t *testing.T) {
	rows := make([]Row, 1000)
	for i := range rows {
		rows[i] = Row{"i": "x"}
	}
	ds := &DataSet{Rows: rows}
	info := CheckLargeDataset(ds)
	if info != nil {
		t.Errorf("expected nil for %d rows (below threshold), got %+v", len(rows), info)
	}
}

func TestCheckLargeDataset_AboveThreshold(t *testing.T) {
	rows := make([]Row, 10001)
	for i := range rows {
		rows[i] = Row{"i": "x"}
	}
	ds := &DataSet{Rows: rows}
	info := CheckLargeDataset(ds)
	if info == nil {
		t.Fatal("expected non-nil info for 10001 rows")
	}
	if info.TotalRows != 10001 {
		t.Errorf("TotalRows = %d, want 10001", info.TotalRows)
	}
	if info.ChunkSize != DefaultChunkSize {
		t.Errorf("ChunkSize = %d, want %d", info.ChunkSize, DefaultChunkSize)
	}
	// 10001 / 1000 = 11 chunks (10 full + 1 remainder)
	if info.ChunkCount != 11 {
		t.Errorf("ChunkCount = %d, want 11", info.ChunkCount)
	}
	if info.EstimatedDuration == "" {
		t.Error("EstimatedDuration should not be empty")
	}
	if info.StorageEstimate == "" {
		t.Error("StorageEstimate should not be empty")
	}
}

func TestCheckLargeDataset_ExactThreshold(t *testing.T) {
	rows := make([]Row, LargeDatasetThreshold)
	for i := range rows {
		rows[i] = Row{"i": "x"}
	}
	ds := &DataSet{Rows: rows}
	info := CheckLargeDataset(ds)
	if info != nil {
		t.Errorf("expected nil for exactly %d rows (at threshold, not above), got %+v", LargeDatasetThreshold, info)
	}
}

func TestChunkRows(t *testing.T) {
	tests := []struct {
		name       string
		rowCount   int
		chunkSize  int
		wantChunks int
		wantLast   int // rows in last chunk
	}{
		{"exact multiple", 3000, 1000, 3, 1000},
		{"remainder", 2500, 1000, 3, 500},
		{"smaller than chunk", 500, 1000, 1, 500},
		{"single row", 1, 1000, 1, 1},
		{"zero chunk size defaults", 100, 0, 1, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := make([]Row, tt.rowCount)
			for i := range rows {
				rows[i] = Row{"i": "x"}
			}
			chunks := ChunkRows(rows, tt.chunkSize)
			if len(chunks) != tt.wantChunks {
				t.Errorf("chunks = %d, want %d", len(chunks), tt.wantChunks)
			}
			if len(chunks) > 0 {
				lastSize := len(chunks[len(chunks)-1])
				if lastSize != tt.wantLast {
					t.Errorf("last chunk size = %d, want %d", lastSize, tt.wantLast)
				}
			}
			// Verify total rows across all chunks equals input
			totalInChunks := 0
			for _, chunk := range chunks {
				totalInChunks += len(chunk)
			}
			if totalInChunks != tt.rowCount {
				t.Errorf("total rows in chunks = %d, want %d", totalInChunks, tt.rowCount)
			}
		})
	}
}
