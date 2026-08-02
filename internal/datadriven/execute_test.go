package datadriven

import (
	"context"
	"fmt"
	"testing"

	"github.com/peterlindqvist/apitest/internal/variable"
)

func TestInjectIterationVars(t *testing.T) {
	tests := []struct {
		name       string
		row        Row
		index      int
		total      int
		wantIndex  string
		wantIter   string
		wantTotal  string
		wantRowNum string
		wantData   map[string]string
	}{
		{
			"first of three",
			Row{"email": "a@b.com"},
			0, 3,
			"0", "1", "3", "1",
			map[string]string{"email": "a@b.com"},
		},
		{
			"last of three",
			Row{"email": "c@d.com"},
			2, 3,
			"2", "3", "3", "3",
			map[string]string{"email": "c@d.com"},
		},
		{
			"single iteration",
			Row{"x": "1"},
			0, 1,
			"0", "1", "1", "1",
			map[string]string{"x": "1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := variable.NewScope(map[string]string{"existing": "val"})
			if err := base.Resolve(); err != nil {
				t.Fatal(err)
			}

			snap := InjectIterationVars(base, tt.row, tt.index, tt.total)
			resolved := snap.Resolved()

			if got := resolved["_index"]; got != tt.wantIndex {
				t.Errorf("_index = %q, want %q", got, tt.wantIndex)
			}
			if got := resolved["_iteration"]; got != tt.wantIter {
				t.Errorf("_iteration = %q, want %q", got, tt.wantIter)
			}
			if got := resolved["_total"]; got != tt.wantTotal {
				t.Errorf("_total = %q, want %q", got, tt.wantTotal)
			}
			if got := resolved["_row_number"]; got != tt.wantRowNum {
				t.Errorf("_row_number = %q, want %q", got, tt.wantRowNum)
			}
			for k, v := range tt.wantData {
				if got := resolved[k]; got != v {
					t.Errorf("%s = %q, want %q", k, got, v)
				}
			}
			// Base scope variable should still be accessible
			if got := resolved["existing"]; got != "val" {
				t.Errorf("existing = %q, want %q", got, "val")
			}
		})
	}
}

func TestInjectIterationVars_SpecialVarsOverrideRowData(t *testing.T) {
	// Per spec: special vars (_index, etc.) take precedence over row data
	// with the same name.
	base := variable.NewScope(map[string]string{})
	if err := base.Resolve(); err != nil {
		t.Fatal(err)
	}

	row := Row{"_index": "should_be_overridden", "name": "john"}
	snap := InjectIterationVars(base, row, 5, 10)
	resolved := snap.Resolved()

	// Special var should win over row data
	if got := resolved["_index"]; got != "5" {
		t.Errorf("_index = %q, want %q (special var should override row data)", got, "5")
	}
	if got := resolved["name"]; got != "john" {
		t.Errorf("name = %q, want %q", got, "john")
	}
}

func TestExecute(t *testing.T) {
	tests := []struct {
		name           string
		rows           []Row
		wantIterations int
	}{
		{"three iterations", []Row{{"a": "1"}, {"a": "2"}, {"a": "3"}}, 3},
		{"single iteration", []Row{{"a": "1"}}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := variable.NewScope(map[string]string{})
			if err := base.Resolve(); err != nil {
				t.Fatal(err)
			}

			ds := &DataSet{Rows: tt.rows, Columns: []string{"a"}}
			var callCount int

			results, _, err := execute(context.Background(), executeConfig{
				DataSet: ds,
				Scope:   base,
				ExecFn: func(_ context.Context, _ *variable.Scope, idx int) (*iterationResult, error) {
					callCount++
					return &iterationResult{Index: idx}, nil
				},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantIterations {
				t.Errorf("results = %d, want %d", len(results), tt.wantIterations)
			}
			if callCount != tt.wantIterations {
				t.Errorf("callCount = %d, want %d", callCount, tt.wantIterations)
			}
		})
	}
}

func TestExecute_ExtractionAccumulates(t *testing.T) {
	base := variable.NewScope(map[string]string{})
	if err := base.Resolve(); err != nil {
		t.Fatal(err)
	}

	ds := &DataSet{
		Rows:    []Row{{"a": "1"}, {"a": "2"}, {"a": "3"}},
		Columns: []string{"a"},
	}

	results, accumulated, err := execute(context.Background(), executeConfig{
		DataSet: ds,
		Scope:   base,
		ExecFn: func(_ context.Context, _ *variable.Scope, idx int) (*iterationResult, error) {
			return &iterationResult{
				Index:     idx,
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
	for i, want := range []string{"id_0", "id_1", "id_2"} {
		if ids[i] != want {
			t.Errorf("user_id[%d] = %q, want %q", i, ids[i], want)
		}
	}
}

func TestExecute_ContextCancellation(t *testing.T) {
	base := variable.NewScope(map[string]string{})
	if err := base.Resolve(); err != nil {
		t.Fatal(err)
	}

	ds := &DataSet{
		Rows:    []Row{{"a": "1"}, {"a": "2"}, {"a": "3"}, {"a": "4"}, {"a": "5"}},
		Columns: []string{"a"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	var callCount int

	results, _, err := execute(ctx, executeConfig{
		DataSet: ds,
		Scope:   base,
		ExecFn: func(_ context.Context, _ *variable.Scope, idx int) (*iterationResult, error) {
			callCount++
			if callCount == 2 {
				cancel()
			}
			return &iterationResult{Index: idx}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have executed 2 iterations before cancellation was detected
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2", callCount)
	}
	// The results should contain the 2 executed iterations
	if len(results) != 2 {
		t.Errorf("results = %d, want 2", len(results))
	}
}

func TestExecute_ScopeIsolation(t *testing.T) {
	// Each iteration should get its own scope snapshot, not share state.
	base := variable.NewScope(map[string]string{"shared": "original"})
	if err := base.Resolve(); err != nil {
		t.Fatal(err)
	}

	ds := &DataSet{
		Rows:    []Row{{"x": "1"}, {"x": "2"}},
		Columns: []string{"x"},
	}

	var scopes []*variable.Scope
	_, _, err := execute(context.Background(), executeConfig{
		DataSet: ds,
		Scope:   base,
		ExecFn: func(_ context.Context, iterScope *variable.Scope, idx int) (*iterationResult, error) {
			scopes = append(scopes, iterScope)
			// Mutate scope in first iteration
			if idx == 0 {
				iterScope.Set("shared", "mutated")
			}
			return &iterationResult{Index: idx}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %d", len(scopes))
	}
	// Second iteration should have gotten the original value, not the mutated one
	resolved := scopes[1].Resolved()
	if got := resolved["shared"]; got != "original" {
		t.Errorf("second iteration shared = %q, want %q (should not see first iteration's mutation)", got, "original")
	}
}

func TestExecute_FailFast(t *testing.T) {
	base := variable.NewScope(map[string]string{})
	if err := base.Resolve(); err != nil {
		t.Fatal(err)
	}

	ds := &DataSet{
		Rows:    []Row{{"a": "1"}, {"a": "2"}, {"a": "3"}, {"a": "4"}, {"a": "5"}},
		Columns: []string{"a"},
	}

	var callCount int
	results, _, err := execute(context.Background(), executeConfig{
		DataSet:  ds,
		Scope:    base,
		FailFast: true,
		ExecFn: func(_ context.Context, _ *variable.Scope, idx int) (*iterationResult, error) {
			callCount++
			if idx == 2 {
				return &iterationResult{Index: idx, Err: fmt.Errorf("intentional failure")}, nil
			}
			return &iterationResult{Index: idx}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should stop after the 3rd iteration (index 2) which failed
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3 (fail_fast should stop after failure)", callCount)
	}
	if len(results) != 3 {
		t.Errorf("results = %d, want 3", len(results))
	}
}

func TestExecute_ContinueOnError(t *testing.T) {
	base := variable.NewScope(map[string]string{})
	if err := base.Resolve(); err != nil {
		t.Fatal(err)
	}

	ds := &DataSet{
		Rows:    []Row{{"a": "1"}, {"a": "2"}, {"a": "3"}, {"a": "4"}, {"a": "5"}},
		Columns: []string{"a"},
	}

	var callCount int
	results, _, err := execute(context.Background(), executeConfig{
		DataSet:  ds,
		Scope:    base,
		FailFast: false, // default
		ExecFn: func(_ context.Context, _ *variable.Scope, idx int) (*iterationResult, error) {
			callCount++
			if idx == 2 {
				return &iterationResult{Index: idx, Err: fmt.Errorf("intentional failure")}, nil
			}
			return &iterationResult{Index: idx}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should continue despite failure at index 2
	if callCount != 5 {
		t.Errorf("callCount = %d, want 5 (continue-on-error should run all)", callCount)
	}
	if len(results) != 5 {
		t.Errorf("results = %d, want 5", len(results))
	}
}
