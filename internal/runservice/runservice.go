// Package runservice is the extracted load-and-run pipeline shared by the CLI
// run command and the apitest ui orchestrator. It owns the happy path —
// parse → load environment → load project config → load .env → build the
// pre-run sensitive set → runner.Run — plus the sensitive-set builders and the
// events emitter sink that were previously inline in cmd/apitest.
//
// It deliberately excludes the CLI-only concerns that surround the pipeline in
// runCmdInner: per-format error rendering, glob discovery, dry-run, team
// vault, plugin hooks, telemetry, and events-file management. Converging
// runCmdInner onto Execute is a deferred follow-up (UI_SPECIFICATION.md §15).
package runservice

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/peterlindqvist/apitest/internal/config"
	"github.com/peterlindqvist/apitest/internal/parser"
	"github.com/peterlindqvist/apitest/internal/runner"
	"github.com/peterlindqvist/apitest/internal/variable"
)

// ErrCollectionInvalid wraps parse/validation failures on the collection file
// so HTTP handlers can map them to 422 collection_invalid without string
// matching. Unwrap exposes the underlying parser error.
var ErrCollectionInvalid = errors.New("collection invalid")

// Request bundles the inputs to Execute.
type Request struct {
	CollectionPath  string   // absolute (caller resolves against project root)
	EnvName         string   // empty = no environment file
	Selection       []string // nil = all (VarSources.Selection semantics)
	Parallel        bool
	Seed            *int64           // nil in UI v1
	RunID           string           // required; caller mints via runner.NewRunID()
	RequestIDPrefix string           // "" for single runs; "c<i>-" per collection in batch runs
	Sink            runner.EventSink // may be nil
	Diagnostics     io.Writer        // may be nil
}

// Result bundles the outputs of Execute.
type Result struct {
	Collection   *parser.Collection
	ProjectCfg   *config.ProjectConfig
	Results      []runner.RequestResult
	Summary      *runner.Summary
	PreSensitive *variable.SensitiveSet // what the sink used mid-run
	Sensitive    *variable.SensitiveSet // post-run full set (incl. Auth/RuntimeSensitive)
}

// Execute runs the load-and-run pipeline. exec is injectable
// (httpexec.Execute in production) so orchestrator tests run against fakes,
// exactly as runner tests do.
//
// Error contract: parse errors are wrapped in ErrCollectionInvalid
// (422 collection_invalid); config.ErrEnvironmentNotFound passes through
// (422); runner errors (including runner.ErrNoMatchingRequests) pass through.
func Execute(ctx context.Context, req Request, exec runner.ExecuteFunc) (*Result, error) {
	col, err := parser.ParseFile(req.CollectionPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCollectionInvalid, err)
	}

	collectionDir := filepath.Dir(req.CollectionPath)

	projectCfg, projectRoot, err := config.LoadProjectConfig(collectionDir)
	if err != nil {
		return nil, err
	}

	var envVars map[string]string
	var environmentLocale string
	if req.EnvName != "" {
		var environment *config.EnvironmentConfig
		environment, err = config.LoadEnvironmentConfig(req.EnvName, collectionDir)
		if errors.Is(err, config.ErrEnvironmentNotFound) && projectRoot != "" && projectRoot != collectionDir {
			// CLI parity looks next to the collection file; the scaffolded
			// layout keeps environments/ at the project root — fall back.
			environment, err = config.LoadEnvironmentConfig(req.EnvName, projectRoot)
		}
		if err != nil {
			return nil, err
		}
		envVars = environment.Variables
		environmentLocale = environment.Config.Locale
	}

	dotenvDir := collectionDir
	if projectRoot != "" {
		dotenvDir = projectRoot
	}
	dotenvVars, dotenvSensitive, err := config.LoadDotenv(dotenvDir)
	if err != nil {
		return nil, err
	}

	in := SensitiveInputs{
		Collection:      col,
		ProjectCfg:      projectCfg,
		EnvVars:         envVars,
		DotenvVars:      dotenvVars,
		DotenvSensitive: dotenvSensitive,
	}
	preSensitive := BuildPreRunSensitive(in)

	sink := req.Sink
	if ss, ok := sink.(interface {
		SetSensitive(*variable.SensitiveSet)
	}); ok {
		// Sinks that redact (EmitterSink, the UI's detail collector and its
		// fan-out) receive the pre-run set built here, mirroring the CLI's
		// adapter rebuild after variable sources are known.
		ss.SetSensitive(preSensitive)
	}

	results, summary, runErr := runner.Run(ctx, col, exec, runner.VarSources{
		Project:           projectCfg.Variables,
		EnvFile:           envVars,
		DotEnv:            dotenvVars,
		Seed:              req.Seed,
		ProjectLocale:     projectCfg.Config.Locale,
		EnvironmentLocale: environmentLocale,
		CollectionLocale:  col.Config.Locale,
		Secrets:           projectCfg.Secrets,
		AuthProfiles:      projectCfg.AuthProfiles,
		ProjectRoot:       projectRoot,
		GlobalRetry:       projectCfg.Defaults.Retry,
		GlobalGraphQL:     projectCfg.Defaults.GraphQL,
		Parallel:          req.Parallel,
		CollectionDir:     collectionDir,
		OnEvent:           sink,
		Selection:         req.Selection,
		RunID:             req.RunID,
		RequestIDPrefix:   req.RequestIDPrefix,
		Diagnostics:       req.Diagnostics,
		// ConfirmLargeDataset stays false: oversized data-driven sets fail
		// with the runner's guard error (no interactive prompt in services).
	})

	res := &Result{
		Collection:   col,
		ProjectCfg:   projectCfg,
		Results:      results,
		Summary:      summary,
		PreSensitive: preSensitive,
		Sensitive:    BuildPostRunSensitive(in, summary),
	}
	if runErr != nil {
		return res, runErr
	}
	return res, nil
}
