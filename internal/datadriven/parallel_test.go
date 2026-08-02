package datadriven

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/variable"
)

func TestExecuteParallel(t *testing.T) {
	tests := []struct {
		name           string
		rows           []Row
		maxWorkers     int
		wantIterations int
	}{
		{"three iterations with 2 workers", []Row{{"a": "1"}, {"a": "2"}, {"a": "3"}}, 2, 3},
		{"single iteration", []Row{{"a": "1"}}, 5, 1},
		{"more workers than rows", []Row{{"a": "1"}, {"a": "2"}}, 10, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := variable.NewScope(map[string]string{})
			if err := base.Resolve(); err != nil {
				t.Fatal(err)
			}

			ds := &DataSet{Rows: tt.rows, Columns: []string{"a"}}
			var callCount int32

			results, _, err := ExecuteParallel(context.Background(), ParallelConfig{
				DataSet:    ds,
				Scope:      base,
				MaxWorkers: tt.maxWorkers,
				ExecFn: func(_ context.Context, _ *variable.Scope, idx int) (*IterationResult, error) {
					atomic.AddInt32(&callCount, 1)
					return &IterationResult{Index: idx, Name: fmt.Sprintf("test [%d]", idx)}, nil
				},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantIterations {
				t.Errorf("results = %d, want %d", len(results), tt.wantIterations)
			}
			if int(atomic.LoadInt32(&callCount)) != tt.wantIterations {
				t.Errorf("callCount = %d, want %d", callCount, tt.wantIterations)
			}
		})
	}
}

func TestExecuteParallel_ConcurrencyBound(t *testing.T) {
	base := variable.NewScope(map[string]string{})
	if err := base.Resolve(); err != nil {
		t.Fatal(err)
	}

	const maxWorkers = 3
	const totalRows = 10
	rows := make([]Row, totalRows)
	for i := range rows {
		rows[i] = Row{"i": fmt.Sprintf("%d", i)}
	}
	ds := &DataSet{Rows: rows, Columns: []string{"i"}}

	var concurrent int32
	var maxConcurrent int32

	results, _, err := ExecuteParallel(context.Background(), ParallelConfig{
		DataSet:    ds,
		Scope:      base,
		MaxWorkers: maxWorkers,
		ExecFn: func(_ context.Context, _ *variable.Scope, idx int) (*IterationResult, error) {
			cur := atomic.AddInt32(&concurrent, 1)
			for {
				old := atomic.LoadInt32(&maxConcurrent)
				if cur <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&concurrent, -1)
			return &IterationResult{Index: idx, Name: fmt.Sprintf("test [%d]", idx)}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != totalRows {
		t.Errorf("results = %d, want %d", len(results), totalRows)
	}
	mc := atomic.LoadInt32(&maxConcurrent)
	if mc > maxWorkers {
		t.Errorf("max concurrent = %d, want <= %d", mc, maxWorkers)
	}
	if mc < 2 {
		t.Errorf("max concurrent = %d, expected at least 2 (parallel execution should use multiple workers)", mc)
	}
}

func TestExecuteParallel_RateLimit(t *testing.T) {
	base := variable.NewScope(map[string]string{})
	if err := base.Resolve(); err != nil {
		t.Fatal(err)
	}

	const rps = 20
	const totalRows = 10
	rows := make([]Row, totalRows)
	for i := range rows {
		rows[i] = Row{"i": fmt.Sprintf("%d", i)}
	}
	ds := &DataSet{Rows: rows, Columns: []string{"i"}}

	start := time.Now()
	results, _, err := ExecuteParallel(context.Background(), ParallelConfig{
		DataSet:      ds,
		Scope:        base,
		MaxWorkers:   5,
		RateLimitRPS: rps,
		ExecFn: func(_ context.Context, _ *variable.Scope, idx int) (*IterationResult, error) {
			return &IterationResult{Index: idx, Name: fmt.Sprintf("test [%d]", idx)}, nil
		},
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != totalRows {
		t.Errorf("results = %d, want %d", len(results), totalRows)
	}

	// With 10 requests at 20 RPS, minimum time should be around 450ms (9 intervals of 50ms).
	// Allow generous tolerance for CI.
	minExpected := time.Duration(totalRows-1) * time.Second / time.Duration(rps) * 80 / 100
	if elapsed < minExpected {
		t.Errorf("elapsed = %v, want >= %v (rate limiting should slow execution)", elapsed, minExpected)
	}
}

func TestExecuteParallel_FailFast(t *testing.T) {
	base := variable.NewScope(map[string]string{})
	if err := base.Resolve(); err != nil {
		t.Fatal(err)
	}

	const totalRows = 20
	rows := make([]Row, totalRows)
	for i := range rows {
		rows[i] = Row{"i": fmt.Sprintf("%d", i)}
	}
	ds := &DataSet{Rows: rows, Columns: []string{"i"}}

	var callCount int32

	results, _, err := ExecuteParallel(context.Background(), ParallelConfig{
		DataSet:    ds,
		Scope:      base,
		MaxWorkers: 2,
		FailFast:   true,
		ExecFn: func(_ context.Context, _ *variable.Scope, idx int) (*IterationResult, error) {
			atomic.AddInt32(&callCount, 1)
			time.Sleep(5 * time.Millisecond)
			if idx == 1 {
				return &IterationResult{Index: idx, Err: fmt.Errorf("intentional failure"), Name: fmt.Sprintf("test [%d]", idx)}, nil
			}
			return &IterationResult{Index: idx, Name: fmt.Sprintf("test [%d]", idx)}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cc := int(atomic.LoadInt32(&callCount))
	if cc >= totalRows {
		t.Errorf("callCount = %d, want < %d (fail-fast should stop early)", cc, totalRows)
	}
	if len(results) >= totalRows {
		t.Errorf("results = %d, want < %d", len(results), totalRows)
	}
}

func TestExecuteParallel_ContextCancellation(t *testing.T) {
	base := variable.NewScope(map[string]string{})
	if err := base.Resolve(); err != nil {
		t.Fatal(err)
	}

	const totalRows = 20
	rows := make([]Row, totalRows)
	for i := range rows {
		rows[i] = Row{"i": fmt.Sprintf("%d", i)}
	}
	ds := &DataSet{Rows: rows, Columns: []string{"i"}}

	ctx, cancel := context.WithCancel(context.Background())
	var callCount int32

	results, _, err := ExecuteParallel(ctx, ParallelConfig{
		DataSet:    ds,
		Scope:      base,
		MaxWorkers: 2,
		ExecFn: func(_ context.Context, _ *variable.Scope, idx int) (*IterationResult, error) {
			count := atomic.AddInt32(&callCount, 1)
			if count == 3 {
				cancel()
			}
			time.Sleep(5 * time.Millisecond)
			return &IterationResult{Index: idx, Name: fmt.Sprintf("test [%d]", idx)}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cc := int(atomic.LoadInt32(&callCount))
	if cc >= totalRows {
		t.Errorf("callCount = %d, want < %d (context cancel should stop early)", cc, totalRows)
	}
	if len(results) >= totalRows {
		t.Errorf("results = %d, want < %d", len(results), totalRows)
	}
}

func TestExecuteParallel_ExtractionAccumulates(t *testing.T) {
	base := variable.NewScope(map[string]string{})
	if err := base.Resolve(); err != nil {
		t.Fatal(err)
	}

	ds := &DataSet{
		Rows:    []Row{{"a": "1"}, {"a": "2"}, {"a": "3"}},
		Columns: []string{"a"},
	}

	results, accumulated, err := ExecuteParallel(context.Background(), ParallelConfig{
		DataSet:    ds,
		Scope:      base,
		MaxWorkers: 3,
		ExecFn: func(_ context.Context, _ *variable.Scope, idx int) (*IterationResult, error) {
			return &IterationResult{
				Index:     idx,
				Name:      fmt.Sprintf("test [%d]", idx),
				Extracted: map[string]string{"user_id": fmt.Sprintf("id_%d", idx)},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("results = %d, want 3", len(results))
	}
	ids, ok := accumulated["user_id"]
	if !ok {
		t.Fatal("expected accumulated user_id")
	}
	if len(ids) != 3 {
		t.Errorf("accumulated user_id length = %d, want 3", len(ids))
	}
	// Accumulated values should be in index order
	for i, want := range []string{"id_0", "id_1", "id_2"} {
		if ids[i] != want {
			t.Errorf("user_id[%d] = %q, want %q", i, ids[i], want)
		}
	}
}

func TestExecuteParallel_ScopeIsolation(t *testing.T) {
	base := variable.NewScope(map[string]string{"shared": "original"})
	if err := base.Resolve(); err != nil {
		t.Fatal(err)
	}

	ds := &DataSet{
		Rows:    []Row{{"x": "1"}, {"x": "2"}},
		Columns: []string{"x"},
	}

	type scopeCapture struct {
		idx   int
		value string
	}
	captures := make(chan scopeCapture, 2)

	_, _, err := ExecuteParallel(context.Background(), ParallelConfig{
		DataSet:    ds,
		Scope:      base,
		MaxWorkers: 2,
		ExecFn: func(_ context.Context, iterScope *variable.Scope, idx int) (*IterationResult, error) {
			resolved := iterScope.Resolved()
			captures <- scopeCapture{idx: idx, value: resolved["shared"]}
			iterScope.Set("shared", "mutated")
			return &IterationResult{Index: idx, Name: fmt.Sprintf("test [%d]", idx)}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(captures)

	for sc := range captures {
		if sc.value != "original" {
			t.Errorf("iteration %d saw shared=%q, want %q", sc.idx, sc.value, "original")
		}
	}
}

func TestExecuteParallel_ChunkOffset(t *testing.T) {
	// When ChunkOffset and TotalRows are provided, ExecuteParallel should
	// use global indices (chunkOffset + local) for InjectIterationVars and ExecFn.
	base := variable.NewScope(map[string]string{})
	if err := base.Resolve(); err != nil {
		t.Fatal(err)
	}

	// Simulate chunk 2 of a 5-row dataset (rows 2,3,4 with offset=2, totalRows=5)
	chunkRows := []Row{{"a": "c"}, {"a": "d"}, {"a": "e"}}
	ds := &DataSet{Rows: chunkRows, Columns: []string{"a"}}

	var capturedIndices []int
	var capturedTotals []int
	mu := &sync.Mutex{}

	results, _, err := ExecuteParallel(context.Background(), ParallelConfig{
		DataSet:     ds,
		Scope:       base,
		MaxWorkers:  3,
		ChunkOffset: 2,
		TotalRows:   5,
		ExecFn: func(_ context.Context, iterScope *variable.Scope, idx int) (*IterationResult, error) {
			resolved := iterScope.Resolved()
			// _index should be the global 0-based index
			mu.Lock()
			capturedIndices = append(capturedIndices, idx)
			mu.Unlock()
			// _total should be the total rows in the full dataset
			if resolved["_total"] != "5" {
				return nil, fmt.Errorf("_total = %q, want %q", resolved["_total"], "5")
			}
			return &IterationResult{
				Index: idx,
				Name:  fmt.Sprintf("test [%d/%d]", idx+1, 5),
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("results = %d, want 3", len(results))
	}

	// ExecFn should have received global indices 2, 3, 4
	mu.Lock()
	defer mu.Unlock()
	if len(capturedIndices) != 3 {
		t.Fatalf("capturedIndices = %d, want 3", len(capturedIndices))
	}
	// Sort since parallel execution order is non-deterministic
	seen := map[int]bool{}
	for _, idx := range capturedIndices {
		seen[idx] = true
	}
	for _, want := range []int{2, 3, 4} {
		if !seen[want] {
			t.Errorf("expected global index %d in captured indices %v", want, capturedIndices)
		}
	}

	_ = capturedTotals // used via scope inspection in ExecFn
}

// TestRateLimiter_* tests were removed: the private rateLimiter type was
// extracted to internal/ratelimit and tested there. See internal/ratelimit/limiter_test.go.
