package datadriven

import "fmt"

// LargeDatasetInfo provides performance estimates for large datasets.
type LargeDatasetInfo struct {
	TotalRows         int
	ChunkSize         int
	ChunkCount        int
	EstimatedDuration string // human-readable estimate
	StorageEstimate   string // human-readable storage estimate
}

// CheckLargeDataset returns info if the dataset exceeds LargeDatasetThreshold, or nil if not.
// Returns nil for nil datasets.
func CheckLargeDataset(ds *DataSet) *LargeDatasetInfo {
	if ds == nil {
		return nil
	}
	if len(ds.Rows) <= LargeDatasetThreshold {
		return nil
	}
	chunks := (len(ds.Rows) + DefaultChunkSize - 1) / DefaultChunkSize
	return &LargeDatasetInfo{
		TotalRows:         len(ds.Rows),
		ChunkSize:         DefaultChunkSize,
		ChunkCount:        chunks,
		EstimatedDuration: estimateDuration(len(ds.Rows)),
		StorageEstimate:   estimateStorage(len(ds.Rows)),
	}
}

// ChunkRows splits rows into chunks of the given size.
// If chunkSize <= 0, DefaultChunkSize is used.
func ChunkRows(rows []Row, chunkSize int) [][]Row {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	chunks := make([][]Row, 0, (len(rows)+chunkSize-1)/chunkSize)
	for i := 0; i < len(rows); i += chunkSize {
		end := i + chunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunks = append(chunks, rows[i:end])
	}
	return chunks
}

// estimateDuration returns a human-readable duration estimate assuming ~100ms per request.
func estimateDuration(rows int) string {
	seconds := rows / 10 // ~100ms per request
	if seconds < 60 {
		return fmt.Sprintf("~%ds", seconds)
	}
	minutes := seconds / 60
	remainSec := seconds % 60
	if minutes < 60 {
		return fmt.Sprintf("~%dm%ds", minutes, remainSec)
	}
	hours := minutes / 60
	remainMin := minutes % 60
	return fmt.Sprintf("~%dh%dm", hours, remainMin)
}

// estimateStorage returns a human-readable storage estimate assuming ~2KB per result.
func estimateStorage(rows int) string {
	bytes := rows * 2048 // ~2KB per result
	if bytes < 1024*1024 {
		return fmt.Sprintf("~%dKB", bytes/1024)
	}
	return fmt.Sprintf("~%dMB", bytes/(1024*1024))
}
