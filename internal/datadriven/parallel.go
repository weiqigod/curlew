package datadriven

import (
	"context"
	"fmt"
	"sync"

	"github.com/peterlindqvist/apitest/internal/assertion"
	"github.com/peterlindqvist/apitest/internal/httpexec"
	"github.com/peterlindqvist/apitest/internal/ratelimit"
	"github.com/peterlindqvist/apitest/internal/retry"
	"github.com/peterlindqvist/apitest/internal/variable"
)

// IterationResult holds the outcome of a single parallel data-driven iteration.
type IterationResult struct {
	Index            int               // 0-based (global index when chunked)
	Row              Row               // data for this iteration
	Err              error             // execution error (nil = success)
	Extracted        map[string]string // variables extracted from this iteration
	Name             string            // formatted name (e.g., "Create User [1/3]")
	HTTPResult       *httpexec.Result
	AssertionResults *assertion.Results
	Method           string                // HTTP method (after interpolation)
	URL              string                // full URL (after interpolation)
	RequestHeaders   map[string]string     // interpolated request headers
	RequestBody      any                   // interpolated request body
	RetryCount       int                   // number of retries (Attempts-1)
	RetryWarnings    []string              // warnings from retry conditions
	AttemptDetails   []retry.AttemptDetail // per-attempt details (from retry)
	// RequestID is the per-run request identifier minted in the runner (e.g. "req-3").
	// Populated by the runner's execFn so the conversion loop can carry it onto
	// RequestResult without re-minting. M9-002.
	RequestID string
	// RequestSlug is the URL-safe slug for the iteration name. M9-002.
	RequestSlug string
}

// ParallelConfig holds the configuration for parallel data-driven execution.
type ParallelConfig struct {
	DataSet      *DataSet
	Scope        *variable.Scope
	ExecFn       func(ctx context.Context, iterScope *variable.Scope, index int) (*IterationResult, error)
	FailFast     bool
	MaxWorkers   int // max concurrent workers (default DefaultMaxWorkers)
	RateLimitRPS int // max requests per second (0 = unlimited)
	ChunkOffset  int // offset added to local indices to compute global indices (for chunked processing)
	TotalRows    int // total rows in the full dataset (0 = use DataSet length)
}

// ExecuteParallel runs data-driven iterations concurrently using a worker pool.
// Results are returned in original index order. Accumulated extractions are
// collected after all workers complete to avoid races.
func ExecuteParallel(ctx context.Context, cfg ParallelConfig) ([]IterationResult, map[string][]string, error) {
	chunkSize := len(cfg.DataSet.Rows)
	// Use TotalRows for the full dataset total if provided; otherwise use chunk size.
	total := cfg.TotalRows
	if total <= 0 {
		total = chunkSize
	}
	results := make([]IterationResult, chunkSize)
	var resultErrors []error
	var mu sync.Mutex

	workers := cfg.MaxWorkers
	if workers <= 0 {
		workers = DefaultMaxWorkers
	}
	if workers > chunkSize {
		workers = chunkSize
	}

	rl := ratelimit.New(cfg.RateLimitRPS)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var failFastTriggered bool

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for localIdx, row := range cfg.DataSet.Rows {
		if runCtx.Err() != nil {
			break
		}

		mu.Lock()
		if failFastTriggered {
			mu.Unlock()
			break
		}
		mu.Unlock()

		if err := rl.Wait(runCtx); err != nil {
			break
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(localIdx, globalIdx int, row Row) {
			defer wg.Done()
			defer func() { <-sem }()

			iterScope := InjectIterationVars(cfg.Scope, row, globalIdx, total)
			result, err := cfg.ExecFn(runCtx, iterScope, globalIdx)
			if err != nil {
				mu.Lock()
				resultErrors = append(resultErrors, fmt.Errorf("iteration %d: %w", globalIdx, err))
				mu.Unlock()
				return
			}

			result.Row = row
			mu.Lock()
			results[localIdx] = *result

			if cfg.FailFast && result.Err != nil {
				failFastTriggered = true
				cancel()
			}
			mu.Unlock()
		}(localIdx, cfg.ChunkOffset+localIdx, row)
	}

	wg.Wait()

	if len(resultErrors) > 0 {
		return nil, nil, resultErrors[0]
	}

	var filtered []IterationResult
	accumulated := make(map[string][]string)
	for i := 0; i < chunkSize; i++ {
		r := results[i]
		if r.Row == nil && r.Err == nil && r.Name == "" {
			continue
		}
		filtered = append(filtered, r)
		for k, v := range r.Extracted {
			accumulated[k] = append(accumulated[k], v)
		}
	}

	return filtered, accumulated, nil
}
