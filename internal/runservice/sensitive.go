package runservice

import (
	"github.com/weiqigod/curlew/internal/config"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/runner"
	"github.com/weiqigod/curlew/internal/variable"
)

// SensitiveInputs carries the variable sources the sensitive-set builders
// consider. EnvVarVars and CLIVars exist for the CLI run path; service callers
// (curlew ui) leave them nil — the UI accepts no ad-hoc variables.
type SensitiveInputs struct {
	Collection      *parser.Collection
	ProjectCfg      *config.ProjectConfig
	EnvVars         map[string]string // --env file variables
	DotenvVars      map[string]string
	DotenvSensitive *variable.SensitiveSet
	EnvVarVars      map[string]string // --env-var OS imports (CLI only)
	CLIVars         map[string]string // --var values (CLI only)
}

// BuildPreRunSensitive builds the sensitive set available before runner.Run:
// collection/request sensitive markers, .env sensitives, configured secret
// names, plus name-heuristics and concrete values over every known variable
// source. The events sink redacts mid-run with this set. Extracted verbatim
// from the pre-run block in cmd/curlew runCmdInner.
func BuildPreRunSensitive(in SensitiveInputs) *variable.SensitiveSet {
	return buildSensitive(in, nil)
}

// BuildPostRunSensitive builds the full post-run sensitive set: everything in
// BuildPreRunSensitive plus summary.AuthSensitive (auth-profile values) and
// summary.RuntimeSensitive (dynamic-fn registrations), which only exist after
// the run. Extracted verbatim from the post-run block in cmd/curlew
// runCmdInner. summary may be nil.
func BuildPostRunSensitive(in SensitiveInputs, summary *runner.Summary) *variable.SensitiveSet {
	return buildSensitive(in, summary)
}

func buildSensitive(in SensitiveInputs, summary *runner.Summary) *variable.SensitiveSet {
	s := variable.NewSensitiveSet()
	if col := in.Collection; col != nil {
		s.Merge(col.Variables.Sensitive)
		for _, req := range col.Requests.Items {
			s.Merge(req.Variables.Sensitive)
		}
		for _, req := range col.Setup.Items {
			s.Merge(req.Variables.Sensitive)
		}
		for _, req := range col.Teardown.Items {
			s.Merge(req.Variables.Sensitive)
		}
	}
	s.Merge(in.DotenvSensitive)
	var projectVars map[string]string
	if in.ProjectCfg != nil {
		projectVars = in.ProjectCfg.Variables
		if in.ProjectCfg.Secrets != nil {
			s.Merge(in.ProjectCfg.Secrets.SensitiveNames())
		}
	}
	if summary != nil {
		if summary.AuthSensitive != nil {
			s.Merge(summary.AuthSensitive)
		}
		if summary.RuntimeSensitive != nil {
			s.Merge(summary.RuntimeSensitive)
		}
	}
	// Heuristic: check all variable names from every source.
	var colVars map[string]string
	if in.Collection != nil {
		colVars = in.Collection.Variables.Values
	}
	sources := []map[string]string{colVars, in.DotenvVars, in.EnvVars, in.EnvVarVars, in.CLIVars, projectVars}
	for _, src := range sources {
		s.AddHeuristicNames(src)
	}
	// Populate sensitive *values* so body redaction can replace them wherever
	// they appear inside request and response body content.
	for _, src := range sources {
		addSensitiveValues(s, src)
	}
	return s
}

// addSensitiveValues registers the concrete values of sensitive-named
// variables so substring redaction works on bodies.
func addSensitiveValues(s *variable.SensitiveSet, vars map[string]string) {
	for name, value := range vars {
		if s.IsSensitive(name) {
			s.AddValue(value)
		}
	}
}
