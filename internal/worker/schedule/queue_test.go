package schedule_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/worker/schedule"
)

func TestQueue(t *testing.T) {
	makePayload := func(pass, fail int) *schedule.ResultRequest {
		return &schedule.ResultRequest{
			ClaimToken: "tok-1",
			RunAt:      time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC),
			DurationMs: 500,
			PassCount:  pass,
			FailCount:  fail,
		}
	}

	tests := []struct {
		name string
		run  func(t *testing.T, q *schedule.Queue)
	}{
		{
			name: "enqueue then list returns one entry",
			run: func(t *testing.T, q *schedule.Queue) {
				payload := makePayload(3, 0)
				_, err := q.Enqueue("run_aaa", payload)
				if err != nil {
					t.Fatalf("Enqueue: %v", err)
				}
				items, err := q.List()
				if err != nil {
					t.Fatalf("List: %v", err)
				}
				if len(items) != 1 {
					t.Fatalf("len(items) = %d; want 1", len(items))
				}
				if items[0].RunID != "run_aaa" {
					t.Errorf("RunID = %q; want %q", items[0].RunID, "run_aaa")
				}
				if items[0].Payload.PassCount != 3 {
					t.Errorf("PassCount = %d; want 3", items[0].Payload.PassCount)
				}
			},
		},
		{
			name: "enqueue then remove leaves empty list",
			run: func(t *testing.T, q *schedule.Queue) {
				if _, err := q.Enqueue("run_bbb", makePayload(1, 0)); err != nil {
					t.Fatalf("Enqueue: %v", err)
				}
				if err := q.Remove("run_bbb"); err != nil {
					t.Fatalf("Remove: %v", err)
				}
				items, err := q.List()
				if err != nil {
					t.Fatalf("List: %v", err)
				}
				if len(items) != 0 {
					t.Errorf("len(items) = %d; want 0", len(items))
				}
			},
		},
		{
			name: "malformed JSON file is skipped, valid sibling is returned",
			run: func(t *testing.T, q *schedule.Queue) {
				// Enqueue a valid entry
				if _, err := q.Enqueue("run_valid", makePayload(2, 0)); err != nil {
					t.Fatalf("Enqueue: %v", err)
				}
				// Write a malformed JSON file manually
				dir := q.Dir
				malformed := filepath.Join(dir, "run_bad.json")
				if err := os.WriteFile(malformed, []byte("{not json"), 0o600); err != nil {
					t.Fatalf("write malformed: %v", err)
				}
				items, err := q.List()
				if err != nil {
					t.Fatalf("List: %v", err)
				}
				if len(items) != 1 {
					t.Fatalf("len(items) = %d; want 1 (malformed skipped)", len(items))
				}
				if items[0].RunID != "run_valid" {
					t.Errorf("RunID = %q; want run_valid", items[0].RunID)
				}
			},
		},
		{
			name: "directory created on first enqueue (0700 perms)",
			run: func(t *testing.T, q *schedule.Queue) {
				// Remove the dir if it exists (it shouldn't — it's a fresh tempdir subdir)
				subDir := filepath.Join(q.Dir, "newsubdir")
				q.Dir = subDir
				if _, err := q.Enqueue("run_ccc", makePayload(0, 1)); err != nil {
					t.Fatalf("Enqueue: %v", err)
				}
				info, err := os.Stat(subDir)
				if err != nil {
					t.Fatalf("Stat dir: %v", err)
				}
				if !info.IsDir() {
					t.Errorf("expected directory, got %v", info.Mode())
				}
				// Check perm mask (0700)
				mode := info.Mode().Perm()
				if mode&0o700 != 0o700 {
					t.Errorf("dir perms = %o; want at least 0700", mode)
				}
			},
		},
		{
			name: "remove of missing runID is a no-op (no error)",
			run: func(t *testing.T, q *schedule.Queue) {
				if err := q.Remove("run_not_exists"); err != nil {
					t.Errorf("Remove non-existent: got error %v; want nil", err)
				}
			},
		},
		{
			name: "list is sorted lexicographically by runID",
			run: func(t *testing.T, q *schedule.Queue) {
				for _, id := range []string{"run_zzz", "run_aaa", "run_mmm"} {
					if _, err := q.Enqueue(id, makePayload(1, 0)); err != nil {
						t.Fatalf("Enqueue %s: %v", id, err)
					}
				}
				items, err := q.List()
				if err != nil {
					t.Fatalf("List: %v", err)
				}
				if len(items) != 3 {
					t.Fatalf("len = %d; want 3", len(items))
				}
				ids := make([]string, len(items))
				for i, it := range items {
					ids[i] = it.RunID
				}
				if !sort.StringsAreSorted(ids) {
					t.Errorf("items not sorted: %v", ids)
				}
			},
		},
		{
			name: "enqueue returns the path written",
			run: func(t *testing.T, q *schedule.Queue) {
				path, err := q.Enqueue("run_path", makePayload(1, 0))
				if err != nil {
					t.Fatalf("Enqueue: %v", err)
				}
				if _, err := os.Stat(path); err != nil {
					t.Errorf("returned path %q not on disk: %v", path, err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := &schedule.Queue{Dir: t.TempDir()}
			tc.run(t, q)
		})
	}
}
