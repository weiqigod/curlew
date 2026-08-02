package schedule

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Queue persists ResultRequest payloads keyed by runID under a directory.
// It exists so a worker that loses backend connectivity does not lose work.
// Each entry is stored as <Dir>/<runID>.json.
//
// TODO(M17): cap the queue size if it becomes an operational concern.
type Queue struct {
	// Dir is the absolute path to the pending-uploads directory. When empty,
	// callers must set it before use. The production default is resolved by
	// RunSchedulePull to <license.ResolveConfigDir()>/pending-uploads.
	Dir string
}

// Pending is one queued item: the runID (filename without .json) plus the
// decoded payload.
type Pending struct {
	RunID   string         // derived from the filename (without .json extension)
	Payload *ResultRequest // decoded JSON payload
	Path    string         // absolute path on disk; for logging only
}

// Enqueue writes payload as JSON to <Dir>/<runID>.json, creating the directory
// with mode 0o700 if it does not yet exist. Returns the absolute path written.
func (q *Queue) Enqueue(runID string, payload *ResultRequest) (path string, err error) {
	if err := os.MkdirAll(q.Dir, 0o700); err != nil {
		return "", fmt.Errorf("queue: mkdir %s: %w", q.Dir, err)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("queue: marshal %s: %w", runID, err)
	}
	dest := filepath.Join(q.Dir, runID+".json")
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return "", fmt.Errorf("queue: write %s: %w", dest, err)
	}
	return dest, nil
}

// List returns all queued pending items sorted lexicographically by runID.
// Malformed JSON files are silently skipped (not returned as errors) so a
// corrupted entry does not block draining of valid entries.
func (q *Queue) List() ([]Pending, error) {
	entries, err := os.ReadDir(q.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("queue: readdir %s: %w", q.Dir, err)
	}

	var items []Pending
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		runID := strings.TrimSuffix(name, ".json")
		absPath := filepath.Join(q.Dir, name)
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue // skip unreadable entries
		}
		var payload ResultRequest
		if err := json.Unmarshal(data, &payload); err != nil {
			continue // skip malformed entries
		}
		items = append(items, Pending{
			RunID:   runID,
			Payload: &payload,
			Path:    absPath,
		})
	}

	// Sort lexicographically by RunID for deterministic drain order.
	sort.Slice(items, func(i, j int) bool {
		return items[i].RunID < items[j].RunID
	})

	return items, nil
}

// Remove deletes the queued payload for runID. If the file does not exist,
// Remove returns nil (idempotent).
func (q *Queue) Remove(runID string) error {
	path := filepath.Join(q.Dir, runID+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("queue: remove %s: %w", path, err)
	}
	return nil
}
