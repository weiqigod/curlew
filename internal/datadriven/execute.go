package datadriven

import (
	"context"
	"fmt"

	"github.com/peterlindqvist/apitest/internal/variable"
)

// iterationResult holds the outcome of a single data-driven iteration.
// Unexported: only used within this package and its tests.
type iterationResult struct {
	Index     int               // 0-based
	Row       Row               // data for this iteration
	Err       error             // execution error (nil = success)
	Extracted map[string]string // variables extracted from this iteration
	Name      string            // formatted name (e.g., "Create User [1/3]")
}

// executeConfig holds the configuration for executing data-driven iterations.
// Unexported: only used within this package and its tests.
type executeConfig struct {
	DataSet  *DataSet
	Scope    *variable.Scope
	ExecFn   func(ctx context.Context, iterScope *variable.Scope, index int) (*iterationResult, error)
	FailFast bool // stop on first iteration failure
}

// InjectIterationVars creates a scope snapshot and adds special iteration
// variables (_index, _iteration, _total, _row_number) and row column values.
// Row data is injected first, then special vars are set on top so that
// special vars always take precedence if a CSV column shares their name.
func InjectIterationVars(scope *variable.Scope, row Row, index, total int) *variable.Scope {
	snap := scope.Snapshot()
	// Row data first (lower precedence)
	for k, v := range row {
		snap.Set(k, v)
	}
	// Special iteration variables (higher precedence -- override row data)
	snap.Set("_index", fmt.Sprintf("%d", index))
	snap.Set("_iteration", fmt.Sprintf("%d", index+1))
	snap.Set("_total", fmt.Sprintf("%d", total))
	snap.Set("_row_number", fmt.Sprintf("%d", index+1))
	return snap
}

// execute runs the data-driven iterations sequentially. Each iteration gets
// its own scope snapshot with row data and special variables injected.
// Returns results for each iteration and accumulated extraction values
// (variable name -> slice of values across iterations).
// Unexported: only used within this package and its tests.
func execute(ctx context.Context, cfg executeConfig) ([]iterationResult, map[string][]string, error) {
	total := len(cfg.DataSet.Rows)
	results := make([]iterationResult, 0, total)
	accumulated := make(map[string][]string)

	for idx, row := range cfg.DataSet.Rows {
		if err := ctx.Err(); err != nil {
			break
		}

		iterScope := InjectIterationVars(cfg.Scope, row, idx, total)
		result, err := cfg.ExecFn(ctx, iterScope, idx)
		if err != nil {
			return results, accumulated, fmt.Errorf("iteration %d: %w", idx, err)
		}

		result.Row = row
		results = append(results, *result)

		// Fail-fast: stop on first failure if configured
		if cfg.FailFast && result.Err != nil {
			break
		}

		// Accumulate extracted values as arrays
		for k, v := range result.Extracted {
			accumulated[k] = append(accumulated[k], v)
		}
	}

	return results, accumulated, nil
}
