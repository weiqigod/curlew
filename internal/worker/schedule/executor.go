package schedule

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/runner"
	"github.com/weiqigod/curlew/internal/vault"
	teamtemplate "github.com/weiqigod/curlew/internal/vault/teamtemplate"
)

// NewRunnerExecutor returns a CollectionExecutor that parses the YAML at the
// given path and executes it via runner.Run.
func NewRunnerExecutor() CollectionExecutor {
	return NewRunnerExecutorWithOptions(RunnerExecutorOptions{})
}

// RunnerExecutorOptions configures the real collection executor used by a
// schedule-pull worker. LoadTeamTemplate is called for every execution so its
// implementation can apply the shared-vault cache TTL and refresh stale data.
type RunnerExecutorOptions struct {
	TeamEnv          string
	LoadTeamTemplate func(context.Context) (*teamtemplate.TeamTemplate, error)
	VaultExecutor    vault.CommandExecutor
}

// NewRunnerExecutorWithOptions returns a CollectionExecutor with optional
// shared-vault propagation. NewRunnerExecutor remains the compatibility
// constructor for callers that do not use a team template.
func NewRunnerExecutorWithOptions(opts RunnerExecutorOptions) CollectionExecutor {
	return &runnerExecutor{opts: opts}
}

type runnerExecutor struct {
	opts RunnerExecutorOptions
}

// Execute parses the collection at collectionPath, merges envVars into the
// variable scope, runs all phases via runner.Run, and maps the results to
// ExecutionOutcome.
func (e *runnerExecutor) Execute(ctx context.Context, collectionPath string, envVars map[string]string) (*ExecutionOutcome, error) {
	started := time.Now().UTC()

	col, err := parser.ParseFile(collectionPath)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", collectionPath, err)
	}

	var teamTemplate *teamtemplate.TeamTemplate
	if e.opts.LoadTeamTemplate != nil {
		teamTemplate, err = e.opts.LoadTeamTemplate(ctx)
		if err != nil {
			return nil, fmt.Errorf("load shared vault template: %w", err)
		}
	}
	teamEnv := e.opts.TeamEnv
	if teamEnv == "" && teamTemplate != nil && len(teamTemplate.Environments) == 1 {
		teamEnv = teamTemplate.Environments[0].Name
	}

	colDir := filepath.Dir(collectionPath)
	results, summary, runErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{
		EnvVar:        envVars,
		ProjectRoot:   colDir,
		CollectionDir: colDir,
		TeamTemplate:  teamTemplate,
		TeamEnv:       teamEnv,
		VaultExecutor: e.opts.VaultExecutor,
	})
	if runErr != nil {
		return nil, runErr
	}

	return mapOutcome(col.Name, started, results, summary), nil
}

// mapOutcome converts runner results into an ExecutionOutcome.
func mapOutcome(collectionName string, started time.Time, results []runner.RequestResult, sum *runner.Summary) *ExecutionOutcome {
	items := make([]ResultItem, 0, len(results))
	for _, r := range results {
		item := ResultItem{Name: r.Name}
		switch {
		case r.Skipped:
			item.Status = "skipped"
			item.Message = r.SkipReason
		case r.Err != nil:
			item.Status = "error"
			item.Message = r.Err.Error()
		case r.AssertionResults != nil && !r.AssertionResults.Passed:
			item.Status = "failed"
			// Surface the first failing assertion description if available.
			for _, a := range r.AssertionResults.Items {
				if !a.Passed {
					// Format: "expected <Expected>; got <Actual>"
					item.Message = fmt.Sprintf("%s expected %s; got %s", a.Type, a.Expected, a.Actual)
					break
				}
			}
		default:
			item.Status = "passed"
		}
		if r.Result != nil {
			item.DurationMs = r.Result.Duration.Milliseconds()
		}
		items = append(items, item)
	}
	return &ExecutionOutcome{
		CollectionName: collectionName,
		StartedAt:      started,
		DurationMs:     sum.Duration.Milliseconds(),
		PassCount:      sum.Passed,
		FailCount:      sum.Failed,
		SkippedCount:   sum.Skipped,
		Items:          items,
	}
}
