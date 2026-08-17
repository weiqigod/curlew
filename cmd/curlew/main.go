package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/weiqigod/curlew/internal/config"
	"github.com/weiqigod/curlew/internal/discovery"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/openapi"
	"github.com/weiqigod/curlew/internal/output"
	"github.com/weiqigod/curlew/internal/output/events"
	"github.com/weiqigod/curlew/internal/output/ids"
	mdformat "github.com/weiqigod/curlew/internal/output/markdown"
	"github.com/weiqigod/curlew/internal/parallel"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/prcheck"
	"github.com/weiqigod/curlew/internal/runner"
	"github.com/weiqigod/curlew/internal/runservice"
	"github.com/weiqigod/curlew/internal/scaffold"
	"github.com/weiqigod/curlew/internal/schema"
	_ "github.com/weiqigod/curlew/internal/signer/awssigv4" // registers "aws-sigv4" signer at init time
	_ "github.com/weiqigod/curlew/internal/signer/oauth1"   // registers "oauth1" signer at init time
	"github.com/weiqigod/curlew/internal/telemetry"
	"github.com/weiqigod/curlew/internal/validator"
	"github.com/weiqigod/curlew/internal/variable"
	teamtmpl "github.com/weiqigod/curlew/internal/vault/teamtemplate"
	"github.com/weiqigod/curlew/internal/watch"
	"github.com/weiqigod/curlew/templates"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	return runWithWriters(args, os.Stdout, os.Stderr)
}

// runWithWriters is the writer-injectable version of run, used by tests and
// called from run() with os.Stdout/os.Stderr for production entry. All
// subcommands thread the provided stdout and stderr through their writer-aware
// variants (*CmdOut functions), so no subcommand writes directly to the
// process-global os.Stdout or os.Stderr.
func runWithWriters(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelpTo(stdout)
		return 0
	}

	switch args[0] {
	case "--version":
		_, _ = fmt.Fprintf(stdout, "curlew %s\n", resolvedVersion)
		return 0
	case "--help", "-h":
		printHelpTo(stdout)
		return 0
	case "run":
		return runCmdWithWriters(args[1:], stdout, stderr)
	case "exec":
		return execCmdOut(args[1:], os.Stdin, stdout, stderr)
	case "validate":
		return validateCmdOut(args[1:], stdout, stderr)
	case "init":
		return initCmdOut(args[1:], stdout, stderr)
	case "info":
		return infoCmdOut(args[1:], stdout, stderr)
	case "schema":
		return schemaCmdOut(args[1:], stdout, stderr)
	case "watch":
		return watchCmdOut(args[1:], stdout, stderr)
	case "ui":
		return uiCmdOut(args[1:], stdout, stderr)
	case "vault":
		return vaultCmdOut(args[1:], stdout, stderr)
	case "pr-check":
		return prCheckCmdOut(args[1:], stdout, stderr)
	case "import":
		return importCmdOut(args[1:], stdout, stderr)
	case "telemetry":
		return telemetryCmdOut(args[1:], stdout, stderr)
	case "perf":
		return perfCmdOut(args[1:], stdout, stderr)
	case "plugins":
		return pluginsCmdOut(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "Unknown command: %s\n", args[0])
		_, _ = fmt.Fprintln(stderr, usageSynopsis(""))
		return 1
	}
}

// runFlags holds all parsed flags for the run subcommand.
type runFlags struct {
	file, envName, format, report                                   string
	vars, envVarVars                                                map[string]string
	seed                                                            *int64
	color                                                           colorMode
	verbosity                                                       output.Verbosity
	allowSensitive, showDeps, dryRun, parallel, confirmLargeDataset bool

	// M6-005: events NDJSON stream
	events string // --events <path>; empty = disabled

	// M8-003: explicit-set tracking so zero values of --format/--report/--events/-q/-v/-vv
	// are distinguishable from "flag not passed". See resolveOutputPrecedence.
	formatSet, reportSet, eventsSet, verbositySet bool

	// M8-004: --only flag for single/union request selection.
	onlyNames []string

	// M20-001: --locale flag for faker locale selection.
	locale    string
	localeSet bool
}

// parseRunArgs extracts all flags for the run subcommand into a runFlags struct.
// Supports: --env, --format, --report, --var, --env-var, --seed, --no-color,
// --allow-sensitive, --show-dependencies, --dry-run, --parallel,
// --confirm-large-dataset, --events, --only, --locale.
func parseRunArgs(args []string) (runFlags, error) {
	f := runFlags{
		vars:       make(map[string]string),
		envVarVars: make(map[string]string),
	}
	errorf := func(format string, a ...any) (runFlags, error) {
		return runFlags{}, fmt.Errorf(format, a...)
	}
	var positional []string
	for i := 0; i < len(args); i++ {
		// --color=<value>; the space-separated form is a case below.
		if v, ok := strings.CutPrefix(args[i], "--color="); ok {
			m, cErr := parseColorMode(v)
			if cErr != nil {
				return f, cErr
			}
			f.color = m
			continue
		}
		switch args[i] {
		case "--no-color":
			// Retained as the spelling most scripts already use; it means
			// exactly --color=never.
			f.color = colorNever
		case "--color":
			i++
			if i >= len(args) {
				return f, fmt.Errorf("--color requires a value (%s)", strings.Join(colorModeValues, "|"))
			}
			m, cErr := parseColorMode(args[i])
			if cErr != nil {
				return f, cErr
			}
			f.color = m
		case "--allow-sensitive":
			f.allowSensitive = true
		case "--show-dependencies":
			f.showDeps = true
		case "--dry-run":
			f.dryRun = true
		case "--parallel":
			f.parallel = true
		case "--confirm-large-dataset":
			f.confirmLargeDataset = true
		case "-vv":
			f.verbosity = output.VerbosityDebug
			f.verbositySet = true
		case "-v":
			f.verbosity = output.VerbosityVerbose
			f.verbositySet = true
		case "-q", "--quiet":
			f.verbosity = output.VerbosityQuiet
			f.verbositySet = true
		case "--var":
			i++
			if i >= len(args) {
				return errorf("--var requires a value (e.g. --var key=value)")
			}
			k, v, parseErr := variable.ParseVarFlag(args[i])
			if parseErr != nil {
				return runFlags{}, parseErr
			}
			f.vars[k] = v
		case "--env-var":
			i++
			if i >= len(args) {
				return errorf("--env-var requires a value (e.g. --env-var API_KEY)")
			}
			k, v, parseErr := variable.ParseEnvVarFlag(args[i], os.LookupEnv)
			if parseErr != nil {
				return runFlags{}, parseErr
			}
			f.envVarVars[k] = v
		case "--env":
			i++
			if i >= len(args) {
				return errorf("--env requires a value (e.g. --env dev)")
			}
			f.envName = args[i]
		case "--seed":
			i++
			if i >= len(args) {
				return errorf("--seed requires a value")
			}
			n, parseErr := strconv.ParseInt(args[i], 10, 64)
			if parseErr != nil {
				return errorf("--seed value must be an integer: %w", parseErr)
			}
			f.seed = &n
		case "--format":
			i++
			if i >= len(args) {
				return errorf("--format requires a value (e.g. --format json)")
			}
			f.format = args[i]
			f.formatSet = true
		case "--report":
			i++
			if i >= len(args) {
				return errorf("--report requires a file path (e.g. --report results.xml)")
			}
			f.report = args[i]
			f.reportSet = true
		case "--events":
			i++
			if i >= len(args) {
				return errorf("--events requires a file path (e.g. --events /tmp/events.jsonl)")
			}
			f.events = args[i]
			f.eventsSet = true
		case "--only":
			// M8-004: repeatable flag to run only the named main request(s).
			// Setup and teardown still run in full regardless of --only.
			i++
			if i >= len(args) {
				return errorf("--only requires a name (e.g. --only \"Get user\")")
			}
			f.onlyNames = append(f.onlyNames, strings.TrimSpace(args[i]))
		case "--locale":
			// M20-001: faker locale selection
			i++
			if i >= len(args) {
				return errorf("--locale requires a value (e.g. --locale de-DE)")
			}
			f.locale = args[i]
			f.localeSet = true
		default:
			// A dash-prefixed token is never a collection path. Rejecting it
			// here means a removed or misspelled flag fails loudly instead of
			// being silently swallowed as an extra positional argument.
			if strings.HasPrefix(args[i], "-") && args[i] != "-" {
				return errorf("unknown flag: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) == 0 {
		return errorf("missing collection file path")
	}
	f.file = positional[0]

	return f, nil
}

// colorMode is the resolved value of --color. The zero value is auto, so a
// caller that never sets it gets the historical behaviour.
type colorMode int

const (
	// colorAuto emits colour when the writer is a terminal and NO_COLOR is
	// unset or empty.
	colorAuto colorMode = iota
	// colorAlways emits colour regardless of TTY state or NO_COLOR — for
	// piping to a pager that renders escapes, or capturing coloured output in
	// CI.
	colorAlways
	// colorNever suppresses colour. `--no-color` is exactly this.
	colorNever
)

func (c colorMode) String() string {
	switch c {
	case colorAlways:
		return "always"
	case colorNever:
		return "never"
	default:
		return "auto"
	}
}

// colorModeValues is the accepted set, in the order the help text lists them.
var colorModeValues = []string{"auto", "always", "never"}

// parseColorMode converts a --color value into a colorMode.
func parseColorMode(v string) (colorMode, error) {
	switch v {
	case "auto":
		return colorAuto, nil
	case "always":
		return colorAlways, nil
	case "never":
		return colorNever, nil
	default:
		return colorAuto, fmt.Errorf("invalid --color value %q: expected one of %s",
			v, strings.Join(colorModeValues, ", "))
	}
}

// shouldUseColor returns true when color output is appropriate for w.
//
// Under auto, colour follows the writer's own TTY state and the NO_COLOR
// environment variable. An explicit --color wins over NO_COLOR: the variable is
// a standing preference, the flag is a decision made for this invocation, and
// the more specific one takes precedence — as it does in git, grep and ripgrep.
//
// NO_COLOR takes effect when it is set to a non-empty value, whatever that
// value is (no-color.org). An empty NO_COLOR is not a request for plain output:
// it is how a caller clears an inherited preference for one command, and it
// leaves the TTY check to decide.
//
// This is the only place the decision is made. Machine formats never reach a
// terminal printer at all, so `always` cannot put escape codes into a JSON, TAP
// or JUnit payload.
func shouldUseColor(w io.Writer, mode colorMode) bool {
	switch mode {
	case colorNever:
		return false
	case colorAlways:
		return true
	default:
		if os.Getenv("NO_COLOR") != "" {
			return false
		}
		return output.IsTerminal(w)
	}
}

// runErrorExitCode maps an error that ended a run to its documented exit code.
//
// Every such error used to become a 5 — "variable resolution error" — whatever
// it was. The large-dataset guard is not that: it is a safety rail the caller
// clears with a flag, which all three exit-code tables document as a 2. The
// distinction is the point of having separate codes, since a pipeline branching
// on 5 goes looking for a missing variable.
func runErrorExitCode(err error) int {
	if errors.Is(err, runner.ErrLargeDataset) {
		return 2
	}
	if errors.Is(err, runner.ErrParallelAnalysis) {
		return 3
	}
	return 5
}

// newStderrPrinter constructs an output.Printer bound to os.Stderr whose
// color flag is derived from os.Stderr's own TTY state, never from stdout.
// This prevents ANSI escape sequences from leaking into piped stderr when
// stdout happens to be a TTY (regression guard for M7-001).
//
// Prefer this helper over output.NewPrinter(os.Stderr, ...) at every new
// call site so the "use stdout-derived color on stderr" anti-pattern cannot
// be reintroduced silently.
func newStderrPrinter(mode colorMode) *output.Printer {
	return output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, mode))
}

// newStderrPrinterTo constructs an output.Printer bound to the given writer w,
// deriving color from w's own TTY state. Use this inside runCmdInner (and
// similar functions that accept an explicit stderr writer) so that injected
// buffers do not emit ANSI escapes (they are never TTYs).
func newStderrPrinterTo(w io.Writer, mode colorMode) *output.Printer {
	return output.NewPrinter(w, shouldUseColor(w, mode))
}

// buildPreExecVarSet collects all variable names that are available before execution
// (collection vars, env vars, dotenv, env-var imports, CLI args, project config).
// These variables should NOT create inter-request dependencies.
func buildPreExecVarSet(sources ...map[string]string) map[string]bool {
	result := make(map[string]bool)
	for _, src := range sources {
		for k := range src {
			result[k] = true
		}
	}
	return result
}

// localeSource returns (sourceDescription, localeValue) for the highest-priority
// locale source in the precedence chain. Used only for the verbose diagnostic line.
// Returns ("", "") when no locale is set from any source.
func localeSource(
	flags runFlags,
	projCfg *config.ProjectConfig,
	col *parser.Collection,
	environmentLocale string,
) (string, string) {
	if flags.localeSet && flags.locale != "" {
		// Normalize so "de_DE" displays as "de-DE" in the diagnostic (finding #4).
		return "--locale flag", variable.NormalizeLocale(flags.locale)
	}
	if col.Config.Locale != "" {
		return "collection config", col.Config.Locale
	}
	if environmentLocale != "" {
		return "environment config", environmentLocale
	}
	if projCfg != nil && projCfg.Config.Locale != "" {
		return "project config", projCfg.Config.Locale
	}
	return "", ""
}

// redactedCLIArgs returns a shallow copy of args with values of sensitive flags
// (--var, --env-var) replaced by "<redacted>" so they are safe to embed in events.
// resolveOutputPrecedence coalesces output configuration from three sources.
// Precedence (high to low): CLI flag > collection output > project output > built-in default.
// Each of the four fields (format, report, events, verbosity) is resolved independently:
// the highest-priority non-empty value wins for each field. CLI "set" is tracked by
// the parallel bool fields on runFlags (formatSet, reportSet, eventsSet, verbositySet).
func resolveOutputPrecedence(flags *runFlags, colOut, projOut *output.Config) error {
	// Start from project-level defaults (lowest priority among YAML sources).
	var effectiveFormat, effectiveReport, effectiveEvents, effectiveVerbosityStr string
	if projOut != nil {
		effectiveFormat = projOut.Format
		effectiveReport = projOut.Report
		effectiveEvents = projOut.Events
		effectiveVerbosityStr = projOut.Verbosity
	}
	// Collection overrides project (higher priority).
	if colOut != nil {
		if colOut.Format != "" {
			effectiveFormat = colOut.Format
		}
		if colOut.Report != "" {
			effectiveReport = colOut.Report
		}
		if colOut.Events != "" {
			effectiveEvents = colOut.Events
		}
		if colOut.Verbosity != "" {
			effectiveVerbosityStr = colOut.Verbosity
		}
	}
	// CLI wins (highest priority): only apply YAML-derived value if CLI flag was not set.
	if !flags.formatSet && effectiveFormat != "" {
		flags.format = effectiveFormat
	}
	if !flags.reportSet && effectiveReport != "" {
		flags.report = effectiveReport
	}
	if !flags.eventsSet && effectiveEvents != "" {
		flags.events = effectiveEvents
	}
	if !flags.verbositySet && effectiveVerbosityStr != "" {
		v, _, err := output.ParseVerbosity(effectiveVerbosityStr)
		if err != nil {
			return err
		}
		flags.verbosity = v
	}
	// Defensive: validate final resolved format (schema should have caught invalid values).
	if flags.format != "" && !output.IsSupportedFormat(flags.format) {
		return fmt.Errorf("unknown output format %q (supported: %s)", flags.format, output.FormatList())
	}
	return nil
}

func redactedCLIArgs(args []string) []string {
	sensitiveFlags := map[string]bool{"--var": true, "--env-var": true}
	out := make([]string, len(args))
	copy(out, args)
	for i := 0; i < len(out)-1; i++ {
		if sensitiveFlags[out[i]] {
			out[i+1] = "<redacted>"
			i++ // skip the value
		}
	}
	return out
}

// runCmdInner is the shared implementation for runCmd and watch mode.
// stdout and stderr are the writers used for all output produced by this
// invocation. Passing os.Stdout / os.Stderr preserves CLI behaviour; passing
// bytes.Buffer is safe for concurrent calls (used by glob-discovery and tests).
//
// NOTE: the --events file path is not safe for concurrent invocation with the
// same file — two goroutines writing to the same NDJSON file will corrupt it.
// Concurrent usage (e.g. TestConcurrentDiscovery) must avoid --events.
//
// Returns exit code and the runner summary (nil if execution never reached runner.Run).
func runCmdInner(args []string, stdout, stderr io.Writer) (int, *runner.Summary) {
	flags, parseErr := parseRunArgs(args)
	if parseErr != nil {
		_, _ = fmt.Fprintln(stderr, "Usage: curlew run <collection-file> [--env <name>] [--env-var VAR ...] [--var key=value ...] [--seed <number>] [--format <type>] [--report <file>] [--only \"<name>\"] [--show-dependencies] [--dry-run] [--parallel] [--confirm-large-dataset] [--color <when>] [--no-color] [-v] [-vv] [-q]")
		errOut := newStderrPrinterTo(stderr, flags.color)
		errOut.StructuredError(parseErr)
		return 1, nil
	}

	file := flags.file
	envName := flags.envName
	format := flags.format
	report := flags.report
	cliVars := flags.vars
	envVarVars := flags.envVarVars
	seed := flags.seed
	color := flags.color
	verbosity := flags.verbosity
	allowSensitive := flags.allowSensitive
	showDeps := flags.showDeps
	dryRun := flags.dryRun
	runParallel := flags.parallel
	confirmLargeDS := flags.confirmLargeDataset

	if format != "" && format != "json" && format != "terminal" && format != "tap" && format != "junit" && format != "html" && format != "markdown" {
		errOut := newStderrPrinterTo(stderr, color)
		errOut.StructuredError(fmt.Errorf("unknown output format %q (supported: terminal, json, tap, junit, html, markdown)", format))
		return 1, nil
	}

	// M9-002: Mint a single run_id for this invocation. Both the events emitter
	// (when --events is active) and the markdown formatter (when --format markdown
	// is active) must carry the same run_id so event stream and markdown sentinels
	// are correlated. The runner honours a non-empty VarSources.RunID, which is
	// set from this value at the runner.Run call site below.
	runID := runner.NewRunID()

	// M18-008: Mint a per-execution session UUID and record the run start time.
	// The session UUID is nested inside the telemetry event payload (per v4-8).
	runStart := time.Now()
	sessionUUID, _ := telemetry.NewSessionUUID()

	// M6-005: Open events file early so an unwritable path fails before any requests,
	// and so run.start / run.error / run.end are emitted even for pre-parse failures
	// when --events is supplied via CLI. A project- or collection-level output.events
	// path is handled in the late-open block after precedence resolution (see below).
	evExitCode := 0
	var (
		eventsEmitter *events.Emitter
		eventsCloser  io.Closer
		eventsSink    runner.EventSink
	)
	if flags.eventsSet && flags.events != "" {
		evF, evErr := os.OpenFile(flags.events, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if evErr != nil {
			errEvOut := newStderrPrinterTo(stderr, color)
			errEvOut.StructuredError(fmt.Errorf("cannot open --events file %q: %w", flags.events, evErr))
			return 1, nil
		}
		eventsCloser = evF
		em, emErr := events.NewEmitter(evF, events.Options{CurlewVersion: resolvedVersion, RunID: runID})
		if emErr != nil {
			_ = evF.Close()
			errEvOut := newStderrPrinterTo(stderr, color)
			errEvOut.StructuredError(fmt.Errorf("cannot initialise events emitter: %w", emErr))
			return 1, nil
		}
		eventsEmitter = em
		// eventsSink is set to a placeholder here so nil-checks below know events
		// are enabled. The sensitive-aware adapter is constructed just before
		// runner.Run once all variable sources are known (see pre-run block below).
		eventsSink = runservice.NewEmitterSink(em, stderr, nil, false)
	}

	// Emit run.start as soon as the emitter is ready. CLIArgs are passed
	// through redactedCLIArgs to strip sensitive --var/--env-var values.
	if eventsEmitter != nil {
		_ = eventsEmitter.EmitRunStartWithInput(events.RunStartInput{
			CLIArgs:        redactedCLIArgs(args),
			CollectionFile: flags.file,
			EnvName:        flags.envName,
			Selection:      flags.onlyNames,
		})
	}

	// Defer run.end to fire on every exit path. evExitCode is updated before
	// each return so the event carries the correct CLI exit code.
	var evSummary *runner.Summary
	defer func() {
		if eventsEmitter != nil {
			total, passed, failed, skipped := 0, 0, 0, 0
			if evSummary != nil {
				total = evSummary.Total
				passed = evSummary.Passed
				failed = evSummary.Failed
				skipped = evSummary.Skipped
			}
			_ = eventsEmitter.EmitRunEnd(total, passed, failed, skipped, evExitCode)
			if eventsCloser != nil {
				_ = eventsCloser.Close()
			}
		}
		// M18-008: fire-and-forget telemetry. Silent; bounded to 2s.
		emitTelemetryRunCompleted(sessionUUID, runStart, evSummary, evExitCode)
	}()

	if flags.formatSet && format == "html" {
		// --format html requires --report flag
		if report == "" {
			htmlReportErr := fmt.Errorf("--format html requires --report <file> (e.g. --format html --report report.html)")
			errOut := newStderrPrinterTo(stderr, color)
			errOut.StructuredError(htmlReportErr)
			if eventsEmitter != nil {
				_ = eventsEmitter.EmitRunError(htmlReportErr)
				evExitCode = 1
			}
			return 1, nil
		}
	}

	// M9-002: --format markdown requires --report <dir> (CLI-set case).
	if flags.formatSet && format == "markdown" {
		if report == "" {
			mdReportErr := fmt.Errorf("format: markdown requires --report <dir>")
			errOut2 := newStderrPrinterTo(stderr, color)
			errOut2.StructuredError(mdReportErr)
			if eventsEmitter != nil {
				_ = eventsEmitter.EmitRunError(mdReportErr)
				evExitCode = 3
			}
			return 3, nil
		}
	}

	stdoutUseColor := shouldUseColor(stdout, color)
	errOut := newStderrPrinterTo(stderr, color)

	// Glob-pattern discovery: if the positional argument contains glob
	// metacharacters, run all matched collections as a batch.
	if discovery.IsGlob(file) {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			cwdWrappedErr := fmt.Errorf("cannot resolve working directory: %w", cwdErr)
			errOut.StructuredError(cwdWrappedErr)
			if eventsEmitter != nil {
				_ = eventsEmitter.EmitRunError(cwdWrappedErr)
				evExitCode = 3
			}
			return 3, nil
		}
		matches, expandErr := discovery.Expand(cwd, file)
		if expandErr != nil {
			errOut.StructuredError(expandErr)
			if eventsEmitter != nil {
				_ = eventsEmitter.EmitRunError(expandErr)
			}
			switch {
			case errors.Is(expandErr, discovery.ErrNoMatches):
				if eventsEmitter != nil {
					evExitCode = 2
				}
				return 2, nil
			case errors.Is(expandErr, discovery.ErrTraversalOutsideRoot),
				errors.Is(expandErr, discovery.ErrAbsolutePattern):
				if eventsEmitter != nil {
					evExitCode = 1
				}
				return 1, nil
			default:
				if eventsEmitter != nil {
					evExitCode = 3
				}
				return 3, nil
			}
		}
		// For glob runs, the events emitter is not threaded into sub-runs.
		// evExitCode is set from the returned exit code so deferred run.end carries it.
		code, discoverySummary := runDiscoveredCollections(matches, envName, format, report, cliVars, envVarVars, seed,
			color, verbosity, allowSensitive, showDeps, dryRun, runParallel, confirmLargeDS, stdout, stderr)
		evExitCode = code
		evSummary = discoverySummary
		return code, discoverySummary
	}

	col, err := parser.ParseFile(file)
	if err != nil {
		if eventsEmitter != nil {
			_ = eventsEmitter.EmitRunError(err)
			evExitCode = 3
		}
		if format == "json" {
			jsonOut := buildJSONOutput("", nil, &runner.Summary{}, err, output.VerbosityDefault)
			_ = output.WriteJSON(stdout, jsonOut)
			return 3, nil
		}
		if format == "tap" {
			_ = writeTAPBailout(stdout, err)
			return 3, nil
		}
		if format == "junit" {
			_ = writeJUnitError(stdout, err)
			return 3, nil
		}
		if format == "html" {
			_ = writeHTMLError(report, err)
			return 3, nil
		}
		errOut.StructuredError(err)
		return 3, nil
	}

	if len(col.Setup.Items) == 0 && len(col.Requests.Items) == 0 && len(col.Teardown.Items) == 0 {
		if format == "json" {
			jsonOut := buildJSONOutput(col.Name, nil, &runner.Summary{}, nil, output.VerbosityDefault)
			jsonOut.Status = "passed"
			_ = output.WriteJSON(stdout, jsonOut)
			return 0, nil
		}
		if format == "tap" {
			if writeErr := output.WriteTAP(stdout, nil, 0, 0, nil); writeErr != nil {
				_, _ = fmt.Fprintf(stderr, "tap write error: %v\n", writeErr)
			}
			return 0, nil
		}
		if format == "junit" {
			emptyOut := buildJUnitOutput(col.Name, nil, &runner.Summary{})
			if writeErr := output.WriteJUnitXML(stdout, emptyOut); writeErr != nil {
				_, _ = fmt.Fprintf(stderr, "junit xml encode error: %v\n", writeErr)
			}
			return 0, nil
		}
		if format == "html" {
			emptyReport := buildHTMLReport(col.Name, nil, &runner.Summary{})
			if writeErr := writeHTMLFile(report, emptyReport); writeErr != nil {
				errOut.StructuredError(fmt.Errorf("cannot write html report: %w", writeErr))
				evExitCode = 1
				return 1, nil
			}
			return 0, nil
		}
		errOut.Warning("collection has no requests")
		return 0, nil
	}

	collectionDir := filepath.Dir(file)

	// Read CURLEW_TEAM_CONFIG early so env-file loading can tolerate its presence.
	teamCfgPath := os.Getenv("CURLEW_TEAM_CONFIG")

	// Load environment variables if --env specified.
	// When CURLEW_TEAM_CONFIG is set, ErrEnvironmentNotFound is tolerated:
	// --env selects the team template environment even if no env file exists.
	var envVars map[string]string
	var environmentLocale string
	if envName != "" {
		var environment *config.EnvironmentConfig
		environment, err = config.LoadEnvironmentConfig(envName, collectionDir)
		environmentProjectRoot, hasProjectRoot := config.FindProjectRoot(collectionDir)
		if errors.Is(err, config.ErrEnvironmentNotFound) && hasProjectRoot && environmentProjectRoot != collectionDir {
			environment, err = config.LoadEnvironmentConfig(envName, environmentProjectRoot)
		}
		if err != nil {
			if errors.Is(err, config.ErrEnvironmentNotFound) && teamCfgPath != "" {
				// Env file absent but team template is configured — treat as empty env vars.
				envVars = nil
			} else {
				if eventsEmitter != nil {
					_ = eventsEmitter.EmitRunError(err)
					evExitCode = 3
				}
				if format == "json" {
					jsonOut := buildJSONOutput(col.Name, nil, &runner.Summary{}, err, output.VerbosityDefault)
					_ = output.WriteJSON(stdout, jsonOut)
					return 3, nil
				}
				if format == "tap" {
					_ = writeTAPBailout(stdout, err)
					return 3, nil
				}
				if format == "junit" {
					_ = writeJUnitError(stdout, err)
					return 3, nil
				}
				if format == "html" {
					_ = writeHTMLError(report, err)
					return 3, nil
				}
				errOut.StructuredError(err)
				return 3, nil
			}
		} else {
			envVars = environment.Variables
			environmentLocale = environment.Config.Locale
		}
	}

	// Load project config by walking up from collection directory (optional)
	projectCfg, projectRoot, err := config.LoadProjectConfig(collectionDir)
	if err != nil {
		if eventsEmitter != nil {
			_ = eventsEmitter.EmitRunError(err)
			evExitCode = 3
		}
		if format == "json" {
			jsonOut := buildJSONOutput(col.Name, nil, &runner.Summary{}, err, output.VerbosityDefault)
			_ = output.WriteJSON(stdout, jsonOut)
			return 3, nil
		}
		if format == "tap" {
			_ = writeTAPBailout(stdout, err)
			return 3, nil
		}
		if format == "junit" {
			_ = writeJUnitError(stdout, err)
			return 3, nil
		}
		if format == "html" {
			_ = writeHTMLError(report, err)
			return 3, nil
		}
		errOut.StructuredError(err)
		return 3, nil
	}

	// Load shared vault template: backend cache (base) + CURLEW_TEAM_CONFIG (overlay).
	// M16-018: use teamtemplate.Load which handles backend fetch, TTL, and merge.
	teamLoadResult, teamErr := loadTeamTemplate(teamCfgPath, stderr)
	if teamErr != nil {
		if eventsEmitter != nil {
			_ = eventsEmitter.EmitRunError(teamErr)
			evExitCode = 3
		}
		if format == "json" {
			jsonOut := buildJSONOutput(col.Name, nil, &runner.Summary{}, teamErr, output.VerbosityDefault)
			_ = output.WriteJSON(stdout, jsonOut)
			return 3, nil
		}
		if format == "tap" {
			_ = writeTAPBailout(stdout, teamErr)
			return 3, nil
		}
		if format == "junit" {
			_ = writeJUnitError(stdout, teamErr)
			return 3, nil
		}
		if format == "html" {
			_ = writeHTMLError(report, teamErr)
			return 3, nil
		}
		errOut.StructuredError(teamErr)
		return 3, nil
	}
	var teamTemplate *teamtmpl.TeamTemplate
	if teamLoadResult != nil {
		teamTemplate = teamLoadResult.Template
	}
	teamStub := os.Getenv("CURLEW_VAULT_STUB") == "1"

	// Load .env from project root if found, otherwise from collection directory
	dotenvDir := collectionDir
	if projectRoot != "" {
		dotenvDir = projectRoot
	}
	dotenvVars, dotenvSensitive, dotenvErr := config.LoadDotenv(dotenvDir)
	if dotenvErr != nil {
		if eventsEmitter != nil {
			_ = eventsEmitter.EmitRunError(dotenvErr)
			evExitCode = 3
		}
		if format == "json" {
			jsonOut := buildJSONOutput(col.Name, nil, &runner.Summary{}, dotenvErr, output.VerbosityDefault)
			_ = output.WriteJSON(stdout, jsonOut)
			return 3, nil
		}
		if format == "tap" {
			_ = writeTAPBailout(stdout, dotenvErr)
			return 3, nil
		}
		if format == "junit" {
			_ = writeJUnitError(stdout, dotenvErr)
			return 3, nil
		}
		if format == "html" {
			_ = writeHTMLError(report, dotenvErr)
			return 3, nil
		}
		errOut.StructuredError(dotenvErr)
		return 3, nil
	}

	// M8-003: Resolve output configuration from three sources.
	// Precedence: CLI flag > collection.output > project.output > built-in default.
	// Each field (format, report, events, verbosity) is resolved independently.
	if resolveErr := resolveOutputPrecedence(&flags, col.Output, projectCfg.Output); resolveErr != nil {
		errOut.StructuredError(resolveErr)
		return 3, nil
	}
	// Propagate resolved values to local variables used by the rest of runCmdInner.
	format = flags.format
	report = flags.report
	verbosity = flags.verbosity
	// flags.events has now been updated with the resolved path (CLI > collection > project).

	// M8-003: Late-open events file for YAML-declared output.events paths.
	// The early open above already handled CLI-provided --events paths; this block
	// handles the case where the events path came from collection or project YAML.
	// The emitter is still opened before any HTTP request, preserving the fast-fail guarantee.
	if eventsEmitter == nil && flags.events != "" {
		evF, evErr := os.OpenFile(flags.events, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if evErr != nil {
			errEvOut := newStderrPrinterTo(stderr, color)
			errEvOut.StructuredError(fmt.Errorf("cannot open events file %q: %w", flags.events, evErr))
			return 1, nil
		}
		eventsCloser = evF
		em, emErr := events.NewEmitter(evF, events.Options{CurlewVersion: resolvedVersion, RunID: runID})
		if emErr != nil {
			_ = evF.Close()
			errEvOut := newStderrPrinterTo(stderr, color)
			errEvOut.StructuredError(fmt.Errorf("cannot initialise events emitter: %w", emErr))
			return 1, nil
		}
		eventsEmitter = em
		eventsSink = runservice.NewEmitterSink(em, stderr, nil, false)
		// Emit run.start now that the emitter is ready.
		_ = eventsEmitter.EmitRunStartWithInput(events.RunStartInput{
			CLIArgs:        redactedCLIArgs(args),
			CollectionFile: flags.file,
			EnvName:        flags.envName,
			Selection:      flags.onlyNames,
		})
	}

	if !flags.formatSet {
		if format == "html" {
			// format html requires report (either via flag or resolved output.report)
			if report == "" {
				htmlReportErr := fmt.Errorf("--format html requires --report <file> (e.g. --format html --report report.html)")
				errOut.StructuredError(htmlReportErr)
				if eventsEmitter != nil {
					_ = eventsEmitter.EmitRunError(htmlReportErr)
					evExitCode = 1
				}
				return 1, nil
			}
		}
		// M9-002: format markdown requires report (YAML-resolved case).
		if format == "markdown" && report == "" {
			mdReportErr := fmt.Errorf("format: markdown requires --report <dir>")
			errOut.StructuredError(mdReportErr)
			if eventsEmitter != nil {
				_ = eventsEmitter.EmitRunError(mdReportErr)
				evExitCode = 3
			}
			return 3, nil
		}
	}

	// Handle --show-dependencies: analyze and display dependency graph, then exit.
	if showDeps {
		preExecVars := buildPreExecVarSet(col.Variables.Values, envVars, dotenvVars, envVarVars, cliVars, projectCfg.Variables)
		// M8-004: apply --only filter before parallel.Analyze so the dependency
		// graph reflects the same filtered subset that runner.Run will execute.
		// This matches the edge-case in the task plan: "filter before parallel.Analyze
		// in the --show-dependencies path too, so the graph visualisation matches
		// the run-time semantics."
		depsItems := col.Requests.Items
		if len(flags.onlyNames) > 0 {
			filtered, filterErr := filterShowDepsItems(col.Requests.Items, flags.onlyNames)
			if filterErr != nil {
				errOut.StructuredError(filterErr)
				if eventsEmitter != nil {
					_ = eventsEmitter.EmitRunError(filterErr)
					evExitCode = 3
				}
				return 3, nil
			}
			depsItems = filtered
		}
		graph := parallel.Analyze(depsItems, preExecVars, parallel.AnalyzeOptions{
			OtherPhaseNames: showDepsOtherPhaseNames(col),
		})
		if !graph.IsValid {
			for _, e := range graph.Errors {
				_, _ = fmt.Fprintln(stderr, e)
			}
			if eventsEmitter != nil {
				graphErr := fmt.Errorf("dependency graph is invalid: %s", strings.Join(graph.Errors, "; "))
				_ = eventsEmitter.EmitRunError(graphErr)
				evExitCode = 3
			}
			return 3, nil
		}
		if dryRun {
			_, _ = fmt.Fprint(stdout, parallel.FormatWaves(graph))
		} else {
			if writeErr := parallel.WriteDOT(stdout, graph); writeErr != nil {
				_, _ = fmt.Fprintf(stderr, "Error writing DOT graph: %v\n", writeErr)
				if eventsEmitter != nil {
					_ = eventsEmitter.EmitRunError(fmt.Errorf("writing DOT graph: %w", writeErr))
					evExitCode = 1
				}
				return 1, nil
			}
		}
		return 0, nil
	}
	_ = dryRun // --dry-run without --show-dependencies is a no-op for now

	ctx := context.Background()

	// Build plugin hook dispatcher (nil when CURLEW_PLUGINS is empty).
	hookDispatcher, closeHooks, hookErr := buildHookDispatcher(ctx, stderr)
	if hookErr != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", hookErr)
		if eventsEmitter != nil {
			_ = eventsEmitter.EmitRunError(hookErr)
			evExitCode = 2
		}
		return 2, nil
	}
	defer closeHooks()

	// Build a pre-run sensitive set so the events adapter can redact request and
	// response bodies that contain sensitive values. This set is built from all
	// variable sources known before runner.Run; it mirrors the post-run sensitive
	// set (lines ~1070+) minus summary.AuthSensitive (only available after run).
	// When eventsEmitter is nil, preSensitive is still built so updateEventsAdapter
	// can be called uniformly, but it never fires.
	//
	// The set is also handed to the runner as its runtime set, so a value
	// discovered mid-run — an extracted token, a dynamic-function credential —
	// is redacted from the very event that carries it. A set collected only at
	// the end would arrive after the stream had already been written.
	var runtimeSensitive *variable.SensitiveSet
	if eventsSink != nil {
		runtimeSensitive = runservice.BuildPreRunSensitive(runservice.SensitiveInputs{
			Collection:      col,
			ProjectCfg:      projectCfg,
			EnvVars:         envVars,
			DotenvVars:      dotenvVars,
			DotenvSensitive: dotenvSensitive,
			EnvVarVars:      envVarVars,
			CLIVars:         cliVars,
		})
		eventsSink = runservice.NewEmitterSink(eventsEmitter, stderr, runtimeSensitive, allowSensitive)
	}

	// Print the collection header before requests start (terminal mode only).
	var out *output.Printer
	if format != "json" && format != "tap" && format != "junit" && format != "html" {
		out = output.NewPrinter(stdout, stdoutUseColor, verbosity)
		out.CollectionHeader(col.Name)
	}

	// M20-001: emit verbose resolved-locale line when -v is active.
	if verbosity >= output.VerbosityVerbose {
		if src, val := localeSource(flags, projectCfg, col, environmentLocale); val != "" {
			_, _ = fmt.Fprintf(stderr, "curlew: resolved locale: %s (source: %s)\n", val, src)
		}
	}

	results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{
		Project:             projectCfg.Variables,
		EnvFile:             envVars,
		DotEnv:              dotenvVars,
		EnvVar:              envVarVars,
		CLI:                 cliVars,
		Seed:                seed,
		Locale:              flags.locale,
		ProjectLocale:       projectCfg.Config.Locale,
		EnvironmentLocale:   environmentLocale,
		CollectionLocale:    col.Config.Locale,
		Secrets:             projectCfg.Secrets,
		AuthProfiles:        projectCfg.AuthProfiles,
		ProjectRoot:         projectRoot,
		GlobalRetry:         projectCfg.Defaults.Retry,
		GlobalGraphQL:       projectCfg.Defaults.GraphQL,
		Parallel:            runParallel,
		CollectionDir:       collectionDir,
		ConfirmLargeDataset: confirmLargeDS,
		TeamTemplate:        teamTemplate,
		TeamEnv:             envName,
		TeamStub:            teamStub,
		Hooks:               hookDispatcher,
		OnEvent:             eventsSink,
		RuntimeSensitive:    runtimeSensitive,
		Selection:           flags.onlyNames,
		Diagnostics:         stderr,
		LocaleVerbose:       verbosity >= output.VerbosityVerbose,
		RunID:               runID, // M9-002: shared run_id; matches events stream when --events is also active
	})
	evSummary = summary // update for deferred run.end

	// M8-004: Handle --only no-match error with exit code 3 (config error,
	// no HTTP requests were sent). Placed before all other error checks so
	// it is never misclassified as a gate or template error.
	if varErr != nil && errors.Is(varErr, runner.ErrNoMatchingRequests) {
		if eventsEmitter != nil {
			_ = eventsEmitter.EmitRunError(varErr)
			evExitCode = 3
		}
		errOut := newStderrPrinterTo(stderr, color)
		errOut.StructuredError(varErr)
		return 3, summary
	}

	// Handle team-template config errors with exit code 3 (config error, not HTTP error).
	if varErr != nil && (errors.Is(varErr, teamtmpl.ErrEnvFlagRequired) ||
		errors.Is(varErr, teamtmpl.ErrUnknownAlias) ||
		errors.Is(varErr, teamtmpl.ErrUnknownEnvironment)) {
		if eventsEmitter != nil {
			_ = eventsEmitter.EmitRunError(varErr)
			evExitCode = 3
		}
		switch format {
		case "json":
			jsonOut := buildJSONOutput(col.Name, nil, summary, varErr, output.VerbosityDefault)
			_ = output.WriteJSON(stdout, jsonOut)
		case "tap":
			_ = writeTAPBailout(stdout, varErr)
		case "junit":
			_ = writeJUnitError(stdout, varErr)
		case "html":
			_ = writeHTMLError(report, varErr)
		default:
			errOut.StructuredError(varErr)
		}
		return 3, summary
	}

	// Emit resolved-secrets log line to stderr (never touches stdout formatters).
	if summary != nil && summary.SharedSecretsResolved > 0 && teamTemplate != nil {
		_, _ = fmt.Fprintf(stderr, "Resolved %d secrets from shared template (%s)\n",
			summary.SharedSecretsResolved, envName)
	}

	// Build sensitivity set from all variable sources and apply redaction.
	sensitive := runservice.BuildPostRunSensitive(runservice.SensitiveInputs{
		Collection:      col,
		ProjectCfg:      projectCfg,
		EnvVars:         envVars,
		DotenvVars:      dotenvVars,
		DotenvSensitive: dotenvSensitive,
		EnvVarVars:      envVarVars,
		CLIVars:         cliVars,
	}, summary)
	// Redact sensitive values from all results before formatting — request and
	// response alike, including each assertion's expected and actual strings.
	runservice.RedactResults(results, sensitive, allowSensitive)

	if format == "json" {
		jsonOut := buildJSONOutput(col.Name, results, summary, varErr, verbosity)
		if summary != nil && summary.LimitExceeded {
			jsonOut.Status = "guard_rail"
			jsonOut.GuardRail = &output.GuardRailJSON{
				LimitExceeded:    true,
				RequestsExecuted: summary.RequestsExecuted,
				Limit:            runner.MaxRequests,
				Message:          "Request limit exceeded. Split this collection into multiple smaller collections.",
			}
		}
		if writeErr := output.WriteJSON(stdout, jsonOut); writeErr != nil {
			_, _ = fmt.Fprintf(stderr, "json encode error: %v\n", writeErr)
			evExitCode = 1
			return 1, summary
		}
		if summary != nil && summary.LimitExceeded {
			evExitCode = 2
			return 2, summary
		}
		if varErr != nil {
			if eventsEmitter != nil {
				_ = eventsEmitter.EmitRunError(varErr)
			}
			code := runErrorExitCode(varErr)
			evExitCode = code
			return code, summary
		}
		if summary != nil {
			mainAssertionFailed := summary.AssertionFailures - summary.TeardownAssertionErrors
			mainFailed := summary.Failed - summary.TeardownErrors
			if mainAssertionFailed > 0 {
				evExitCode = 1
				return 1, summary
			}
			if mainFailed > 0 {
				evExitCode = 4
				return 4, summary
			}
		}
		return 0, summary
	}

	if format == "tap" {
		isParallel := summary != nil && summary.IsParallel
		tapResults := buildTAPOutput(results, isParallel)
		var passed, failed int
		if summary != nil {
			passed = summary.Passed
			failed = summary.Failed
		}
		var parallelInfo *output.ParallelTAP
		if isParallel && summary.WaveCount > 1 {
			parallelInfo = newParallelTAP(summary)
		}
		if writeErr := output.WriteTAP(stdout, tapResults, passed, failed, parallelInfo); writeErr != nil {
			_, _ = fmt.Fprintf(stderr, "tap encode error: %v\n", writeErr)
			evExitCode = 1
			return 1, summary
		}
		if summary != nil && summary.LimitExceeded {
			_, _ = fmt.Fprintf(stdout, "# Guard rail: executed %d requests (limit: %d)\n",
				summary.RequestsExecuted, runner.MaxRequests)
			evExitCode = 2
			return 2, summary
		}
		if varErr != nil {
			if eventsEmitter != nil {
				_ = eventsEmitter.EmitRunError(varErr)
			}
			code := runErrorExitCode(varErr)
			evExitCode = code
			return code, summary
		}
		if summary != nil {
			mainAssertionFailed := summary.AssertionFailures - summary.TeardownAssertionErrors
			mainFailed := summary.Failed - summary.TeardownErrors
			if mainAssertionFailed > 0 {
				evExitCode = 1
				return 1, summary
			}
			if mainFailed > 0 {
				evExitCode = 4
				return 4, summary
			}
		}
		return 0, summary
	}

	if format == "junit" {
		junitOut := buildJUnitOutput(col.Name, results, summary)
		junitWriter := stdout
		if report != "" {
			f, createErr := os.Create(report)
			if createErr != nil {
				errOut.StructuredError(fmt.Errorf("cannot create report file: %w", createErr))
				evExitCode = 1
				return 1, nil
			}
			defer func() { _ = f.Close() }()
			junitWriter = f
		}
		if writeErr := output.WriteJUnitXML(junitWriter, junitOut); writeErr != nil {
			_, _ = fmt.Fprintf(stderr, "junit xml encode error: %v\n", writeErr)
			evExitCode = 1
			return 1, summary
		}
		if summary != nil && summary.LimitExceeded {
			evExitCode = 2
			return 2, summary
		}
		if varErr != nil {
			if eventsEmitter != nil {
				_ = eventsEmitter.EmitRunError(varErr)
			}
			code := runErrorExitCode(varErr)
			evExitCode = code
			return code, summary
		}
		if summary != nil {
			mainAssertionFailed := summary.AssertionFailures - summary.TeardownAssertionErrors
			mainFailed := summary.Failed - summary.TeardownErrors
			if mainAssertionFailed > 0 {
				evExitCode = 1
				return 1, summary
			}
			if mainFailed > 0 {
				evExitCode = 4
				return 4, summary
			}
		}
		return 0, summary
	}

	if format == "html" {
		htmlReport := buildHTMLReport(col.Name, results, summary)
		if writeErr := writeHTMLFile(report, htmlReport); writeErr != nil {
			errOut.StructuredError(fmt.Errorf("cannot write html report: %w", writeErr))
			evExitCode = 1
			return 1, summary
		}
		if summary != nil && summary.LimitExceeded {
			evExitCode = 2
			return 2, summary
		}
		if varErr != nil {
			if eventsEmitter != nil {
				_ = eventsEmitter.EmitRunError(varErr)
			}
			code := runErrorExitCode(varErr)
			evExitCode = code
			return code, summary
		}
		if summary != nil {
			mainAssertionFailed := summary.AssertionFailures - summary.TeardownAssertionErrors
			mainFailed := summary.Failed - summary.TeardownErrors
			if mainAssertionFailed > 0 {
				evExitCode = 1
				return 1, summary
			}
			if mainFailed > 0 {
				evExitCode = 4
				return 4, summary
			}
		}
		return 0, summary
	}

	// M9-002: markdown format dispatch.
	if format == "markdown" {
		// M9-003: markdown always redacts regardless of --allow-sensitive.
		// Apply a second redaction pass with allowSensitive=false so that the
		// formatter never sees raw secret values even when the events stream
		// was permitted to emit them. This preserves the dispatcher contract:
		// --allow-sensitive only affects the NDJSON events stream.
		runservice.RedactResults(results, sensitive, false)
		mdReport := buildMarkdownReport(col, envName, results, summary)
		if err := mdformat.EnsureReportDir(report); err != nil {
			errOut.StructuredError(fmt.Errorf("cannot create report directory: %w", err))
			evExitCode = 1
			return 1, summary
		}
		if err := mdformat.WriteReport(mdReport, report, mdformat.WriteOptions{Stderr: stderr}); err != nil {
			errOut.StructuredError(fmt.Errorf("cannot write markdown report: %w", err))
			evExitCode = 1
			return 1, summary
		}
		if summary != nil && summary.LimitExceeded {
			evExitCode = 2
			return 2, summary
		}
		if varErr != nil {
			if eventsEmitter != nil {
				_ = eventsEmitter.EmitRunError(varErr)
			}
			code := runErrorExitCode(varErr)
			evExitCode = code
			return code, summary
		}
		if summary != nil {
			mainAssertionFailed := summary.AssertionFailures - summary.TeardownAssertionErrors
			mainFailed := summary.Failed - summary.TeardownErrors
			if mainAssertionFailed > 0 {
				evExitCode = 1
				return 1, summary
			}
			if mainFailed > 0 {
				evExitCode = 4
				return 4, summary
			}
		}
		return 0, summary
	}

	if varErr != nil {
		code := runErrorExitCode(varErr)
		if eventsEmitter != nil {
			_ = eventsEmitter.EmitRunError(varErr)
			evExitCode = code
		}
		errOut.StructuredError(varErr)
		return code, summary
	}

	var currentPhase runner.Phase
	currentWaveIndex := -1
	for i := 0; i < len(results); i++ {
		r := results[i]
		if r.Phase != currentPhase {
			currentPhase = r.Phase
			currentWaveIndex = -1 // reset wave tracking on phase change
			switch currentPhase {
			case runner.PhaseSetup:
				out.SectionHeader("Setup")
			case runner.PhaseTeardown:
				out.SectionHeader("Teardown")
			}
		}
		// Wave header for parallel execution
		if summary.IsParallel && r.Phase == runner.PhaseMain && r.WaveIndex != currentWaveIndex {
			currentWaveIndex = r.WaveIndex
			waveSize := countWaveResults(results, currentWaveIndex)
			out.WaveHeader(currentWaveIndex+1, waveSize)
		}

		// Data-driven group rendering
		if r.IsDataDriven && r.IterationIndex == 0 {
			group, nextIdx := groupDataDrivenResults(results, i)
			renderDataDrivenGroup(out, errOut, group)
			i = nextIdx - 1 // -1 because the for loop will increment
			continue
		}

		switch {
		case r.Skipped:
			if r.SkipReason != "" {
				out.SkippedWithReason(r.Name, r.SkipReason)
			} else {
				out.Skipped(r.Name)
			}
		case r.Err != nil:
			errOut.RequestError(r.Name, r.Err)
		default:
			passed := r.AssertionResults == nil || r.AssertionResults.Passed
			out.RequestDetail(r.Method, r.URL, r.RequestHeaders)
			out.RequestBodyDump(r.RequestBody)
			out.Result(r.Name, r.Result, passed, r.RetryCount)
			out.RetryAttemptDetails(r.AttemptDetails)
			if r.Result != nil {
				out.ResponseDetail(r.Result.StatusCode, r.Result.Headers)
				out.ResponseBodyDump(r.Result.Body)
			}
			if r.AssertionResults != nil {
				for _, ar := range r.AssertionResults.Items {
					if !ar.Passed {
						out.AssertionDetail(ar.Label(), ar.Expected, ar.Actual)
					}
				}
			}
			for _, w := range r.Warnings {
				out.Warning(w)
			}
		}
	}

	out.SummaryWithDuration(summary.Total, summary.Passed, summary.Failed, summary.Skipped, summary.Duration)

	// Render parallel execution summary
	if summary.IsParallel {
		out.ParallelSummary(summary.WaveCount, summary.MaxParallelism, summary.Duration, summary.WaveDurations)
	}

	// Render impact analysis from parallel execution
	if len(summary.Impact) > 0 {
		impactLines := make([]output.ImpactLine, len(summary.Impact))
		for i, e := range summary.Impact {
			impactLines[i] = output.ImpactLine{
				FailedName:   e.FailedName,
				SkippedCount: e.SkippedCount,
			}
		}
		out.ImpactSummary(impactLines)
	}

	// Guard rail takes precedence — execution was incomplete
	if summary.LimitExceeded {
		out.GuardRail(summary.RequestsExecuted, runner.MaxRequests)
		evExitCode = 2
		return 2, summary
	}

	// Teardown failures are informational and do not affect exit code
	mainAssertionFailed := summary.AssertionFailures - summary.TeardownAssertionErrors
	mainFailed := summary.Failed - summary.TeardownErrors
	runExitCode := 0
	if mainAssertionFailed > 0 {
		runExitCode = 1
	} else if mainFailed > 0 {
		runExitCode = 4
	}

	evExitCode = runExitCode
	return runExitCode, summary
}

// runCmdWithWriters is a thin wrapper around runCmdInner used by both the CLI
// entry point and tests. It returns only the exit code, discarding the summary.
func runCmdWithWriters(args []string, stdout, stderr io.Writer) int {
	code, _ := runCmdInner(args, stdout, stderr)
	return code
}

func runCmd(args []string) int {
	return runCmdWithWriters(args, os.Stdout, os.Stderr)
}

// watchCmdOut implements the watch subcommand.
func watchCmdOut(args []string, stdout, stderr io.Writer) int {
	// Extract watch-specific flags before parseRunArgs.
	clearScreen := false
	var filteredArgs []string
	for _, a := range args {
		if a == "--clear" {
			clearScreen = true
		} else {
			filteredArgs = append(filteredArgs, a)
		}
	}

	watchFlags, err := parseRunArgs(filteredArgs)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Usage: curlew watch <collection-file> [--env <name>] [--var key=value ...] [--format <type>] [--clear] [--color <when>] [--no-color] [-v] [-vv] [-q]")
		errOut := newStderrPrinterTo(stderr, watchFlags.color)
		errOut.StructuredError(err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	stdoutUseColor := shouldUseColor(stdout, watchFlags.color)

	return watch.Run(ctx, watch.Config{
		CollectionPath: watchFlags.file,
		Args:           filteredArgs,
		EnvName:        watchFlags.envName,
		Debounce:       500 * time.Millisecond,
		Stdout:         stdout,
		Stderr:         stderr,
		UseColor:       stdoutUseColor,
		Format:         watchFlags.format,
		ClearScreen:    clearScreen,
		RunFunc: func(a []string, wOut, wErr io.Writer) watch.RunResult {
			exitCode, summary := runCmdInner(a, wOut, wErr)
			r := watch.RunResult{ExitCode: exitCode}
			if summary != nil {
				r.Total = summary.Total
				r.Passed = summary.Passed
				r.Failed = summary.Failed
				r.Skipped = summary.Skipped
			}
			return r
		},
	})
}

// buildJSONOutput constructs a JSONOutput from runner results and summary.
// preExecErr is non-nil for errors that occurred before or during runner.Run.
// verbosity controls whether request/response headers and body are included.
func buildJSONOutput(name string, results []runner.RequestResult, summary *runner.Summary, preExecErr error, verbosity output.Verbosity) *output.JSONOutput {
	out := &output.JSONOutput{
		Name:     name,
		Requests: make([]output.JSONRequest, 0),
		Summary:  buildSummaryJSON(summary), // M11-002
	}

	if summary != nil {
		out.DurationMs = summary.Duration.Milliseconds()
	}

	if preExecErr != nil {
		out.Status = "error"
		out.Errors = []output.JSONError{{Message: preExecErr.Error()}}
		return out
	}

	for _, r := range results {
		jr := output.JSONRequest{
			Name:       r.Name,
			Method:     r.Method,
			URL:        r.URL,
			RetryCount: r.RetryCount,
			Assertions: make([]output.JSONAssertion, 0),
			Warnings:   r.Warnings,
		}

		// Populate attempt details when retries occurred.
		if r.RetryCount > 0 && len(r.AttemptDetails) > 0 {
			jr.AttemptDetails = make([]output.JSONAttemptDetail, len(r.AttemptDetails))
			for i, d := range r.AttemptDetails {
				jr.AttemptDetails[i] = output.JSONAttemptDetail{
					Attempt:    d.Number,
					StatusCode: d.StatusCode,
					DurationMs: d.Duration.Milliseconds(),
					DelayMs:    d.Delay.Milliseconds(),
				}
				if d.Err != nil {
					jr.AttemptDetails[i].Error = d.Err.Error()
				}
			}
		}

		switch {
		case r.Skipped:
			jr.Status = "skipped"
			jr.SkipReason = r.SkipReason
		case r.Err != nil:
			jr.Status = "error"
			jr.Error = &output.JSONError{Message: r.Err.Error()}
		default:
			if r.Result != nil {
				jr.StatusCode = r.Result.StatusCode
				jr.DurationMs = r.Result.Duration.Milliseconds()
			}
			if r.AssertionResults != nil {
				for _, ar := range r.AssertionResults.Items {
					jr.Assertions = append(jr.Assertions, output.JSONAssertion{
						Type:     ar.Type,
						Target:   ar.Target,
						Operator: ar.Operator,
						Label:    ar.Label(),
						Expected: ar.Expected,
						Actual:   ar.Actual,
						Passed:   ar.Passed,
					})
				}
				if r.AssertionResults.Passed {
					jr.Status = "passed"
				} else {
					jr.Status = "failed"
				}
			} else {
				jr.Status = "passed"
			}
		}

		if verbosity >= output.VerbosityVerbose && r.RequestHeaders != nil {
			jr.RequestHeaders = r.RequestHeaders
		}
		if verbosity >= output.VerbosityVerbose && r.Result != nil {
			jr.ResponseHeaders = map[string][]string(r.Result.Headers)
		}
		if verbosity >= output.VerbosityDebug && r.Result != nil {
			jr.ResponseBody = string(r.Result.Body)
		}

		// Set wave index for parallel execution main-phase results
		if summary != nil && summary.IsParallel && r.Phase == runner.PhaseMain {
			idx := r.WaveIndex
			jr.WaveIndex = &idx
		}

		out.Requests = append(out.Requests, jr)
	}

	if summary != nil {
		mainFailed := summary.Failed - summary.TeardownErrors
		if mainFailed > 0 {
			out.Status = "failed"
		} else {
			out.Status = "passed"
		}
		// Add impact analysis from parallel execution
		if len(summary.Impact) > 0 {
			out.Impact = make([]output.JSONImpact, len(summary.Impact))
			for i, e := range summary.Impact {
				out.Impact[i] = output.JSONImpact{
					FailedRequest: e.FailedName,
					SkippedCount:  e.SkippedCount,
				}
			}
		}
		// Add parallel execution metadata
		if summary.IsParallel {
			out.ParallelExecution = newParallelExecutionJSON(summary)
		}
	}

	// Build data-driven aggregates
	out.DataDriven = buildDataDrivenJSON(results)

	return out
}

// dataDrivenStats holds aggregated statistics for a data-driven group.
type dataDrivenStats struct {
	passed        int
	failed        int
	totalDuration int64 // total duration in milliseconds
	avgDuration   int64 // average duration in milliseconds
	failedIndices []int // 1-based iteration indices that failed
}

// computeDataDrivenStats computes aggregate statistics for a data-driven group.
func computeDataDrivenStats(group []runner.RequestResult) dataDrivenStats {
	var s dataDrivenStats
	var durationCount int64
	for _, r := range group {
		if r.Skipped {
			continue
		}
		iterPassed := r.Err == nil && (r.AssertionResults == nil || r.AssertionResults.Passed)
		if iterPassed {
			s.passed++
		} else {
			s.failed++
			s.failedIndices = append(s.failedIndices, r.IterationIndex+1)
		}
		if r.Result != nil {
			s.totalDuration += r.Result.Duration.Milliseconds()
			durationCount++
		}
	}
	if durationCount > 0 {
		s.avgDuration = s.totalDuration / durationCount
	}
	return s
}

// buildSummaryJSON returns a SummaryJSON populated from runner.Summary.
// Returns a zeroed SummaryJSON when summary is nil so the JSON contract
// `jq '.summary.passed'` never produces null. (M11-002)
func buildSummaryJSON(summary *runner.Summary) *output.SummaryJSON {
	s := &output.SummaryJSON{}
	if summary != nil {
		s.Total = summary.Total
		s.Passed = summary.Passed
		s.Failed = summary.Failed
		s.Skipped = summary.Skipped
	}
	return s
}

// buildParallelMetadata extracts wave count, max parallelism, and speedup factor
// from a runner.Summary. SpeedupFactor mirrors the JSON formatter's computation:
// sum(WaveDurations) / Duration rounded to one decimal, zero when WaveCount <= 1
// or Duration <= 0. Used by both JSON and TAP paths to keep the values in
// lockstep. (M11-003)
func buildParallelMetadata(summary *runner.Summary) (waveCount, maxParallelism int, speedup float64) {
	if summary == nil {
		return 0, 0, 0
	}
	waveCount = summary.WaveCount
	maxParallelism = summary.MaxParallelism
	if summary.WaveCount > 1 && summary.Duration > 0 {
		var seqEstimate time.Duration
		for _, d := range summary.WaveDurations {
			seqEstimate += d
		}
		speedup = float64(seqEstimate) / float64(summary.Duration)
		speedup = math.Round(speedup*10) / 10
	}
	return waveCount, maxParallelism, speedup
}

// newParallelTAP builds the TAP parallel-execution diagnostic for a summary.
//
// This and newParallelExecutionJSON exist so the two output paths cannot drift:
// each is one call rather than a repeated three-field copy, and both are covered
// by TestParallelMetadata_formatters_agree. (M21-003)
func newParallelTAP(summary *runner.Summary) *output.ParallelTAP {
	waveCount, maxParallelism, speedup := buildParallelMetadata(summary)
	return &output.ParallelTAP{
		WaveCount:      waveCount,
		MaxParallelism: maxParallelism,
		SpeedupFactor:  speedup,
	}
}

// newParallelExecutionJSON builds the JSON parallel_execution block for a summary.
// See newParallelTAP.
func newParallelExecutionJSON(summary *runner.Summary) *output.ParallelExecutionJSON {
	waveCount, maxParallelism, speedup := buildParallelMetadata(summary)
	return &output.ParallelExecutionJSON{
		WaveCount:      waveCount,
		MaxParallelism: maxParallelism,
		SpeedupFactor:  speedup,
	}
}

// buildDataDrivenJSON computes data-driven aggregate entries from runner results.
// Returns nil when no data-driven results are present.
func buildDataDrivenJSON(results []runner.RequestResult) []output.DataDrivenJSON {
	var ddEntries []output.DataDrivenJSON
	for i := 0; i < len(results); i++ {
		r := results[i]
		if !r.IsDataDriven || r.IterationIndex != 0 {
			continue
		}
		group, nextIdx := groupDataDrivenResults(results, i)
		s := computeDataDrivenStats(group)
		ddEntries = append(ddEntries, output.DataDrivenJSON{
			Type:             "data_driven",
			Name:             r.DataDrivenName,
			TotalIterations:  len(group),
			PassedIterations: s.passed,
			FailedIterations: s.failed,
			TotalDurationMs:  s.totalDuration,
			AvgDurationMs:    s.avgDuration,
			Iterations:       buildDataDrivenIterations(group), // M11-002
		})
		i = nextIdx - 1
	}
	return ddEntries
}

// buildDataDrivenIterations converts a contiguous data-driven result group
// into per-iteration entries for JSON output. (M11-002)
func buildDataDrivenIterations(group []runner.RequestResult) []output.DataDrivenIterationJSON {
	out := make([]output.DataDrivenIterationJSON, 0, len(group))
	for _, r := range group {
		entry := output.DataDrivenIterationJSON{Name: r.Name}
		switch {
		case r.Skipped:
			entry.Status = "skipped"
		case r.Err != nil:
			entry.Status = "failed"
		case r.AssertionResults != nil && !r.AssertionResults.Passed:
			entry.Status = "failed"
		default:
			entry.Status = "passed"
		}
		if r.Result != nil {
			entry.DurationMs = r.Result.Duration.Milliseconds()
		}
		if len(r.IterationData) > 0 {
			entry.DataColumns = r.IterationData
		}
		out = append(out, entry)
	}
	return out
}

// buildTAPOutput converts runner results to TAP result structs.
// When parallel is true, wave index annotations are set on main-phase results.
func buildTAPOutput(results []runner.RequestResult, parallel bool) []output.TAPResult {
	out := make([]output.TAPResult, 0, len(results))
	for _, r := range results {
		tr := output.TAPResult{Name: r.Name, RetryCount: r.RetryCount}
		switch {
		case r.Skipped:
			tr.Skipped = true
			tr.SkipReason = r.SkipReason
			tr.Passed = true
		case r.Err != nil:
			tr.Passed = false
			tr.Error = r.Err.Error()
		default:
			if r.Result != nil {
				tr.DurationMs = r.Result.Duration.Milliseconds()
			}
			if r.AssertionResults != nil {
				tr.Passed = r.AssertionResults.Passed
				if !r.AssertionResults.Passed {
					for _, ar := range r.AssertionResults.Items {
						if !ar.Passed {
							tr.Failures = append(tr.Failures, output.TAPFailure{
								Type:     ar.Label(),
								Expected: ar.Expected,
								Actual:   ar.Actual,
							})
						}
					}
				}
			} else {
				tr.Passed = true
			}
		}
		// Populate retry details for TAP diagnostics.
		if r.RetryCount > 0 && len(r.AttemptDetails) > 0 {
			tr.RetryDetails = make([]output.TAPRetryDetail, len(r.AttemptDetails))
			for i, d := range r.AttemptDetails {
				tr.RetryDetails[i] = output.TAPRetryDetail{
					Attempt:    d.Number,
					StatusCode: d.StatusCode,
					DurationMs: d.Duration.Milliseconds(),
					DelayMs:    d.Delay.Milliseconds(),
				}
				if d.Err != nil {
					tr.RetryDetails[i].Error = d.Err.Error()
				}
			}
		}
		// Set wave index for parallel execution main-phase results
		if parallel && r.Phase == runner.PhaseMain {
			idx := r.WaveIndex
			tr.WaveIndex = &idx
		}
		// Set data-driven group annotation on first iteration
		if r.IsDataDriven && r.IterationIndex == 0 {
			label := fmt.Sprintf("%s (%d iterations)", r.DataDrivenName, r.IterationTotal)
			tr.DataDrivenGroup = &label
		}
		out = append(out, tr)
	}
	return out
}

// countWaveResults counts results belonging to the given wave index in the main phase.
func countWaveResults(results []runner.RequestResult, waveIndex int) int {
	count := 0
	for _, r := range results {
		if r.Phase == runner.PhaseMain && r.WaveIndex == waveIndex {
			count++
		}
	}
	return count
}

// groupDataDrivenResults extracts contiguous data-driven results for the same
// base name starting at index i. Returns the group and the next index to process.
func groupDataDrivenResults(results []runner.RequestResult, startIdx int) ([]runner.RequestResult, int) {
	baseName := results[startIdx].DataDrivenName
	group := []runner.RequestResult{results[startIdx]}
	next := startIdx + 1
	for next < len(results) && results[next].IsDataDriven && results[next].DataDrivenName == baseName {
		group = append(group, results[next])
		next++
	}
	return group, next
}

// dataDrivenCompactThreshold is the iteration count threshold for compact mode.
const dataDrivenCompactThreshold = 10

// renderDataDrivenGroup renders a data-driven group of results.
// For >= dataDrivenCompactThreshold iterations: compact mode with summary line.
// For < dataDrivenCompactThreshold iterations: verbose mode with per-iteration lines.
// Failed iteration details are always shown regardless of mode.
func renderDataDrivenGroup(out, errOut *output.Printer, group []runner.RequestResult) {
	if len(group) == 0 {
		return
	}

	baseName := group[0].DataDrivenName
	total := group[0].IterationTotal
	s := computeDataDrivenStats(group)

	out.DataDrivenHeader(baseName, total)

	if len(group) >= dataDrivenCompactThreshold {
		// Compact mode
		out.DataDrivenCompactSummary(s.passed, s.failed, len(group), s.avgDuration, s.failedIndices)
		// Show failed iteration details
		for _, r := range group {
			iterPassed := r.Err == nil && (r.AssertionResults == nil || r.AssertionResults.Passed)
			if !iterPassed {
				if r.Err != nil {
					errOut.RequestError(r.Name, r.Err)
				}
				if r.AssertionResults != nil {
					for _, ar := range r.AssertionResults.Items {
						if !ar.Passed {
							out.AssertionDetail(ar.Label(), ar.Expected, ar.Actual)
						}
					}
				}
			}
		}
	} else {
		// Verbose mode
		for _, r := range group {
			iterPassed := r.Err == nil && (r.AssertionResults == nil || r.AssertionResults.Passed)
			var dms int64
			if r.Result != nil {
				dms = r.Result.Duration.Milliseconds()
			}
			out.DataDrivenVerboseResult(r.IterationIndex+1, r.Name, dms, iterPassed)
			// Show failed details for each iteration
			if !iterPassed {
				if r.Err != nil {
					errOut.RequestError(r.Name, r.Err)
				}
				if r.AssertionResults != nil {
					for _, ar := range r.AssertionResults.Items {
						if !ar.Passed {
							out.AssertionDetail(ar.Label(), ar.Expected, ar.Actual)
						}
					}
				}
			}
		}
		out.DataDrivenSummary(s.passed, s.failed, len(group), s.avgDuration, s.failedIndices)
	}
}

// buildJUnitOutput constructs JUnit XML output from runner results and summary.
func buildJUnitOutput(name string, results []runner.RequestResult, summary *runner.Summary) *output.JUnitTestSuites {
	suite := output.JUnitTestSuite{
		Name: name,
	}
	var failures, errCount, skipped int
	for _, r := range results {
		tc := output.JUnitTestCase{
			Name:      r.Name,
			ClassName: name,
		}
		switch {
		case r.Skipped:
			tc.Skipped = &output.JUnitSkipped{Message: r.SkipReason}
			skipped++
		case r.Err != nil:
			tc.Error = &output.JUnitError{
				Message: r.Err.Error(),
				Type:    "ExecutionError",
			}
			errCount++
		default:
			if r.Result != nil {
				tc.Time = fmt.Sprintf("%.3f", r.Result.Duration.Seconds())
			}
			if r.AssertionResults != nil && !r.AssertionResults.Passed {
				var msgs []string
				for _, ar := range r.AssertionResults.Items {
					if !ar.Passed {
						msgs = append(msgs, fmt.Sprintf("Expected %s %s, got %s", ar.Label(), ar.Expected, ar.Actual))
					}
				}
				if len(msgs) > 0 {
					tc.Failure = &output.JUnitFailure{
						Message: msgs[0],
						Type:    "AssertionFailure",
						Body:    strings.Join(msgs, "\n"),
					}
				}
				failures++
			}
		}
		suite.TestCases = append(suite.TestCases, tc)
	}
	suite.Tests = len(results)
	suite.Failures = failures
	suite.Errors = errCount
	suite.Skipped = skipped
	if summary != nil {
		suite.Time = fmt.Sprintf("%.3f", summary.Duration.Seconds())
	}
	return &output.JUnitTestSuites{TestSuites: []output.JUnitTestSuite{suite}}
}

// writeJUnitError writes a minimal JUnit XML with a single error testcase for pre-execution errors.
func writeJUnitError(w io.Writer, err error) error {
	suites := &output.JUnitTestSuites{
		TestSuites: []output.JUnitTestSuite{{
			Name:   "curlew",
			Tests:  1,
			Errors: 1,
			TestCases: []output.JUnitTestCase{{
				Name:      "initialization",
				ClassName: "curlew",
				Error: &output.JUnitError{
					Message: err.Error(),
					Type:    "InitializationError",
				},
			}},
		}},
	}
	return output.WriteJUnitXML(w, suites)
}

// writeTAPBailout writes a TAP bail-out message for pre-execution errors.
func writeTAPBailout(w io.Writer, err error) error {
	if _, wErr := fmt.Fprintln(w, "TAP version 13"); wErr != nil {
		return wErr
	}
	_, wErr := fmt.Fprintf(w, "Bail out! %s\n", err.Error())
	return wErr
}

// buildHTMLReport constructs an HTMLReport from runner results and summary.
func buildHTMLReport(name string, results []runner.RequestResult, summary *runner.Summary) *output.HTMLReport {
	htmlReport := &output.HTMLReport{
		Name:        name,
		GeneratedAt: time.Now().Format(time.RFC3339),
	}
	if summary != nil {
		htmlReport.Total = summary.Total
		htmlReport.Passed = summary.Passed
		htmlReport.Failed = summary.Failed
		htmlReport.Skipped = summary.Skipped
		htmlReport.DurationMs = summary.Duration.Milliseconds()
		if summary.Failed > 0 {
			htmlReport.Status = "failed"
		} else {
			htmlReport.Status = "passed"
		}
	}
	var iterInputs []output.IterationInput
	var waveInputs []output.WaveInput
	var totalRequestMs int64

	for _, r := range results {
		hr := output.HTMLRequest{
			Name:   r.Name,
			Method: r.Method,
			URL:    r.URL,
		}
		status := htmlResultStatus(r)
		statusCode := htmlResultStatusCode(r)
		durationMs := htmlResultDurationMs(r)
		errStr := htmlResultError(r)

		switch {
		case r.Skipped:
			hr.Status = "skipped"
			hr.SkipReason = r.SkipReason
		case r.Err != nil:
			hr.Status = "error"
			hr.Error = r.Err.Error()
		default:
			if r.Result != nil {
				hr.StatusCode = r.Result.StatusCode
				hr.DurationMs = r.Result.Duration.Milliseconds()
			}
			if r.AssertionResults != nil && !r.AssertionResults.Passed {
				hr.Status = "failed"
				for _, ar := range r.AssertionResults.Items {
					hr.Assertions = append(hr.Assertions, output.HTMLAssertion{
						Type:     ar.Label(),
						Expected: ar.Expected,
						Actual:   ar.Actual,
						Passed:   ar.Passed,
					})
				}
			} else {
				hr.Status = "passed"
			}
		}
		htmlReport.Requests = append(htmlReport.Requests, hr)

		// Collect data-driven iteration inputs.
		if r.IsDataDriven {
			iterInputs = append(iterInputs, output.IterationInput{
				GroupName:  r.DataDrivenName,
				Index:      r.IterationIndex,
				Total:      r.IterationTotal,
				Status:     status,
				StatusCode: statusCode,
				DurationMs: durationMs,
				Data:       r.IterationData,
				Error:      errStr,
			})
		}

		// Collect parallel wave inputs.
		if summary != nil && summary.IsParallel && r.WaveIndex >= 0 {
			waveInputs = append(waveInputs, output.WaveInput{
				WaveIndex:  r.WaveIndex,
				Name:       r.Name,
				Status:     status,
				DurationMs: durationMs,
				StatusCode: statusCode,
			})
		}

		if r.Result != nil {
			totalRequestMs += r.Result.Duration.Milliseconds()
		}
	}

	// Aggregate data-driven groups.
	htmlReport.DataDrivenGroups = output.BuildDataDrivenGroups(iterInputs)

	// Aggregate parallel wave data.
	if summary != nil && summary.IsParallel {
		htmlReport.IsParallel = true
		htmlReport.WaveCount = summary.WaveCount
		htmlReport.MaxParallelism = summary.MaxParallelism
		waveDurs := make([]int64, len(summary.WaveDurations))
		var totalWaveMs int64
		for i, d := range summary.WaveDurations {
			waveDurs[i] = d.Milliseconds()
			totalWaveMs += waveDurs[i]
		}
		htmlReport.Waves = output.BuildWaves(waveInputs, waveDurs)
		htmlReport.TotalWaveDurationMs = totalWaveMs
		htmlReport.SpeedupFactor = output.ComputeSpeedup(totalRequestMs, totalWaveMs)
	}

	return htmlReport
}

// buildMarkdownReport converts runner results into a markdown.Report.
// Mirrors buildHTMLReport in shape. For M9-004: groups consecutive data-driven
// iterations sharing DataDrivenName into a single RequestEntry with Iterations
// populated; sets IsParallel from summary.
func buildMarkdownReport(col *parser.Collection, envName string, results []runner.RequestResult, summary *runner.Summary) *mdformat.Report {
	runID := ""
	isParallel := false
	if summary != nil {
		runID = summary.RunID
		isParallel = summary.IsParallel
	}
	rep := &mdformat.Report{
		CollectionName: col.Name,
		EnvName:        envName,
		RunID:          runID,
		StartedAt:      time.Now().UTC(),
		IsParallel:     isParallel,
	}
	if summary != nil {
		rep.Summary = mdformat.SummaryCounts{
			Total:   summary.Total,
			Passed:  summary.Passed,
			Failed:  summary.Failed,
			Skipped: summary.Skipped,
		}
	}
	for i := 0; i < len(results); i++ {
		r := results[i]
		if r.Phase != runner.PhaseMain {
			continue
		}
		if !r.IsDataDriven {
			// M9-002/M9-003 path: single-request entry.
			// RequestSlug may be empty for parallel results (parallel runner does not
			// populate RequestID/RequestSlug); derive slug from name as fallback.
			slug := r.RequestSlug
			if slug == "" {
				slug, _ = parser.Slug(r.Name)
			}
			entry := mdformat.RequestEntry{
				RequestID:   r.RequestID,
				Slug:        slug,
				Name:        r.Name,
				Method:      r.Method,
				URL:         r.URL,
				WaveIndex:   r.WaveIndex,
				StartedAt:   rep.StartedAt,
				RequestHdr:  r.RequestHeaders,
				RequestBody: r.RequestBody,
				Assertions:  r.AssertionResults,
				Err:         r.Err,
				Skipped:     r.Skipped,
				SkipReason:  r.SkipReason,
			}
			if r.Result != nil {
				entry.StatusCode = r.Result.StatusCode
				entry.DurationMs = r.Result.Duration.Milliseconds()
				entry.RespHeaders = r.Result.Headers
				entry.RespBody = r.Result.Body
			}
			rep.Requests = append(rep.Requests, entry)
			continue
		}
		// M9-004: data-driven — collect all consecutive results sharing DataDrivenName.
		groupName := r.DataDrivenName
		baseSlug, _ := parser.Slug(groupName)
		var iters []mdformat.IterationEntry
		var iterTotal int
		var firstReqID string
		for ; i < len(results); i++ {
			rr := results[i]
			if !rr.IsDataDriven || rr.DataDrivenName != groupName || rr.Phase != runner.PhaseMain {
				break
			}
			if firstReqID == "" {
				firstReqID = rr.RequestID
				iterTotal = rr.IterationTotal
			}
			it := mdformat.IterationEntry{
				RequestID:   rr.RequestID,
				Index:       rr.IterationIndex,
				Method:      rr.Method,
				URL:         rr.URL,
				RequestHdr:  rr.RequestHeaders,
				RequestBody: rr.RequestBody,
				Assertions:  rr.AssertionResults,
				Err:         rr.Err,
				Skipped:     rr.Skipped,
				SkipReason:  rr.SkipReason,
				StartedAt:   rep.StartedAt,
				Data:        rr.IterationData,
			}
			if rr.Result != nil {
				it.StatusCode = rr.Result.StatusCode
				it.DurationMs = rr.Result.Duration.Milliseconds()
				it.RespHeaders = rr.Result.Headers
				it.RespBody = rr.Result.Body
			}
			iters = append(iters, it)
		}
		i-- // outer loop will increment; we already advanced i to one past the last DD iteration
		rep.Requests = append(rep.Requests, mdformat.RequestEntry{
			RequestID:      firstReqID,
			Slug:           baseSlug,
			Name:           groupName,
			DataDrivenName: groupName,
			IterationTotal: iterTotal,
			Iterations:     iters,
			WaveIndex:      -1, // DD aggregates always sequential in run.md
			StartedAt:      rep.StartedAt,
		})
	}
	return rep
}

// htmlResultStatus returns the HTML status string for a request result.
func htmlResultStatus(r runner.RequestResult) string {
	switch {
	case r.Skipped:
		return "skipped"
	case r.Err != nil:
		return "error"
	case r.AssertionResults != nil && !r.AssertionResults.Passed:
		return "failed"
	default:
		return "passed"
	}
}

// htmlResultStatusCode returns the HTTP status code from a result (0 if unavailable).
func htmlResultStatusCode(r runner.RequestResult) int {
	if r.Result != nil {
		return r.Result.StatusCode
	}
	return 0
}

// htmlResultDurationMs returns the request duration in milliseconds (0 if unavailable).
func htmlResultDurationMs(r runner.RequestResult) int64 {
	if r.Result != nil {
		return r.Result.Duration.Milliseconds()
	}
	return 0
}

// htmlResultError returns the error string for a result (empty if no error).
func htmlResultError(r runner.RequestResult) string {
	if r.Err != nil {
		return r.Err.Error()
	}
	return ""
}

// writeHTMLFile writes an HTML report to the specified file path.
func writeHTMLFile(path string, htmlReport *output.HTMLReport) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create html report file: %w", err)
	}
	defer func() { _ = f.Close() }()
	return output.WriteHTML(f, htmlReport)
}

// writeHTMLError writes an HTML error report for pre-execution errors.
func writeHTMLError(path string, err error) error {
	htmlReport := &output.HTMLReport{
		Name:        "curlew",
		Status:      "error",
		GeneratedAt: time.Now().Format(time.RFC3339),
		Requests: []output.HTMLRequest{{
			Name:   "initialization",
			Status: "error",
			Error:  err.Error(),
		}},
		Total:  1,
		Failed: 1,
	}
	return writeHTMLFile(path, htmlReport)
}

// validateCmdOut validates one or more collection files without executing HTTP requests.
// Exit codes: 0 = valid (or warnings only), 1 = usage error, 2 = team template validation error, 3 = collection validation errors.
func validateCmdOut(args []string, stdout, stderr io.Writer) int {
	patterns, format, color, err := parseValidateArgs(args)
	if err != nil || len(patterns) == 0 {
		_, _ = fmt.Fprintln(stderr, "Usage: curlew validate <file|glob> [...] [--format json] [--color <when>] [--no-color]")
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		}
		return 1
	}

	if format != "" && format != "json" && format != "terminal" {
		_, _ = fmt.Fprintf(stderr, "unknown output format %q (supported: terminal, json)\n", format)
		return 1
	}

	files, err := expandGlobs(patterns)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 3
	}

	useColor := shouldUseColor(stdout, color)
	results := make([]*validator.Result, 0, len(files))
	for _, f := range files {
		results = append(results, validator.ValidateAuto(f, nil))
	}

	allValid := true
	for _, r := range results {
		if !r.Valid {
			allValid = false
			break
		}
	}

	if format == "json" {
		jsonOut := buildValidationJSONOutput(results)
		if writeErr := output.WriteValidationJSON(stdout, jsonOut); writeErr != nil {
			_, _ = fmt.Fprintf(stderr, "json encode error: %v\n", writeErr)
			return 1
		}
		if !allValid {
			return teamTemplateExitCode(results)
		}
		return 0
	}

	// Terminal output
	for _, r := range results {
		printValidationResult(stdout, r, useColor)
	}

	if !allValid {
		return teamTemplateExitCode(results)
	}
	return 0
}

// teamTemplateExitCode returns exit 2 if any failed result is a team template,
// otherwise exit 3 (collection validation failure).
func teamTemplateExitCode(results []*validator.Result) int {
	for _, r := range results {
		if !r.Valid && r.Kind == validator.KindTeamTemplate {
			return 2
		}
	}
	return 3
}

// parseValidateArgs parses arguments for the validate command.
func parseValidateArgs(args []string) (files []string, format string, color colorMode, err error) {
	for i := 0; i < len(args); i++ {
		if v, ok := strings.CutPrefix(args[i], "--color="); ok {
			m, cErr := parseColorMode(v)
			if cErr != nil {
				return nil, "", colorAuto, cErr
			}
			color = m
			continue
		}
		switch args[i] {
		case "--no-color":
			color = colorNever
		case "--color":
			i++
			if i >= len(args) {
				return nil, "", colorAuto, fmt.Errorf("--color requires a value (%s)", strings.Join(colorModeValues, "|"))
			}
			m, cErr := parseColorMode(args[i])
			if cErr != nil {
				return nil, "", colorAuto, cErr
			}
			color = m
		case "--format":
			i++
			if i >= len(args) {
				return nil, "", colorAuto, fmt.Errorf("--format requires a value (e.g. --format json)")
			}
			format = args[i]
		default:
			files = append(files, args[i])
		}
	}
	return files, format, color, nil
}

// expandGlobs expands any glob patterns in patterns to concrete file paths.
// Returns an error if a glob matches no files.
func expandGlobs(patterns []string) ([]string, error) {
	var files []string
	for _, p := range patterns {
		if discovery.IsGlob(p) {
			matches, err := filepath.Glob(p)
			if err != nil {
				return nil, fmt.Errorf("invalid glob pattern %q: %w", p, err)
			}
			if len(matches) == 0 {
				return nil, fmt.Errorf("no files matched pattern %q", p)
			}
			files = append(files, matches...)
		} else {
			files = append(files, p)
		}
	}
	return files, nil
}

// buildValidationJSONOutput converts validator results to JSON output.
func buildValidationJSONOutput(results []*validator.Result) *output.ValidationJSONOutput {
	allValid := true
	files := make([]output.ValidationFileJSON, 0, len(results))
	for _, r := range results {
		if !r.Valid {
			allValid = false
		}
		issues := make([]output.ValidationIssueJSON, 0, len(r.Issues))
		for _, iss := range r.Issues {
			issues = append(issues, output.ValidationIssueJSON{
				Severity: iss.Severity.String(),
				Line:     iss.Line,
				Message:  iss.Message,
				Hint:     iss.Hint,
			})
		}
		files = append(files, output.ValidationFileJSON{
			File:   r.FilePath,
			Valid:  r.Valid,
			Issues: issues,
		})
	}
	return &output.ValidationJSONOutput{Files: files, Valid: allValid}
}

// printValidationResult prints a single file's validation result to w in terminal format.
func printValidationResult(w io.Writer, r *validator.Result, useColor bool) {
	// Team template: print a one-line summary on success.
	if r.Valid && r.Kind == validator.KindTeamTemplate {
		if useColor {
			_, _ = fmt.Fprintf(w, "\033[32m✓\033[0m OK: shared vault template valid (%s)\n", r.Summary)
		} else {
			_, _ = fmt.Fprintf(w, "OK: shared vault template valid (%s)\n", r.Summary)
		}
		return
	}

	if r.Valid && len(r.Issues) == 0 {
		if useColor {
			_, _ = fmt.Fprintf(w, "\033[32m✓\033[0m %s is valid\n", r.FilePath)
		} else {
			_, _ = fmt.Fprintf(w, "OK %s is valid\n", r.FilePath)
		}
		return
	}

	if r.Valid {
		// Warnings only
		if useColor {
			_, _ = fmt.Fprintf(w, "\033[33m!\033[0m %s is valid (with warnings)\n", r.FilePath)
		} else {
			_, _ = fmt.Fprintf(w, "WARN %s is valid (with warnings)\n", r.FilePath)
		}
	} else {
		if useColor {
			_, _ = fmt.Fprintf(w, "\033[31m✗\033[0m %s is invalid\n", r.FilePath)
		} else {
			_, _ = fmt.Fprintf(w, "FAIL %s is invalid\n", r.FilePath)
		}
	}

	for _, iss := range r.Issues {
		prefix := "  [WARNING]"
		if iss.Severity == validator.SeverityError {
			prefix = "  [ERROR]  "
		}
		if iss.Line > 0 {
			_, _ = fmt.Fprintf(w, "%s line %d: %s\n", prefix, iss.Line, iss.Message)
		} else {
			_, _ = fmt.Fprintf(w, "%s %s\n", prefix, iss.Message)
		}
		if iss.Hint != "" {
			_, _ = fmt.Fprintf(w, "           Hint: %s\n", iss.Hint)
		}
	}
}

// initCmdOut implements the init subcommand.
// Exit codes: 0 = success, 1 = scaffolding error, 3 = invalid --output or --skill value.
func initCmdOut(args []string, stdout, stderr io.Writer) int {
	dir := "."
	projectNameFlag := ""
	outputFormat := ""
	skillName := ""
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			printInitHelpTo(stdout)
			return 0
		case "--project-name":
			i++
			if i >= len(args) {
				_, _ = fmt.Fprintln(stderr, "Error: --project-name requires a value")
				return 1
			}
			projectNameFlag = args[i]
		case "--output":
			i++
			if i >= len(args) {
				_, _ = fmt.Fprintln(stderr, "Error: --output requires a value")
				return 1
			}
			outputFormat = args[i]
		case "--skill":
			i++
			if i >= len(args) {
				_, _ = fmt.Fprintln(stderr, "Error: --skill requires a value")
				return 1
			}
			skillName = args[i]
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) > 0 {
		dir = positional[0]
	}

	if outputFormat != "" && !output.IsSupportedFormat(outputFormat) {
		_, _ = fmt.Fprintf(stderr, "Error: unknown --output value %q (supported: %s)\n",
			outputFormat, output.FormatList())
		return 3
	}
	if skillName != "" && !templates.IsSupportedSkill(skillName) {
		_, _ = fmt.Fprintf(stderr, "Error: unknown --skill value %q (supported: %s)\n",
			skillName, strings.Join(templates.SupportedSkills, ", "))
		return 3
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	projectName := projectNameFlag
	if projectName == "" {
		projectName = filepath.Base(absDir)
	}

	if err := scaffold.Init(scaffold.Options{
		Dir:           dir,
		ProjectName:   projectName,
		OutputFormat:  outputFormat,
		SkillName:     skillName,
		CurlewVersion: resolvedVersion,
	}); err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintln(stdout, "Project initialized successfully!")
	_, _ = fmt.Fprintln(stdout)
	_, _ = fmt.Fprintln(stdout, "Created:")
	_, _ = fmt.Fprintln(stdout, "  curlew.yaml")
	_, _ = fmt.Fprintln(stdout, "  .gitignore")
	_, _ = fmt.Fprintln(stdout, "  .env.example")
	_, _ = fmt.Fprintln(stdout, "  environments/dev.yaml")
	_, _ = fmt.Fprintln(stdout, "  collections/sample.yaml")
	if skillName != "" {
		_, _ = fmt.Fprintf(stdout, "  %s\n", templates.SkillRelativePath(skillName))
	}
	_, _ = fmt.Fprintln(stdout)
	_, _ = fmt.Fprintln(stdout, "Next steps:")
	_, _ = fmt.Fprintln(stdout, "  curlew run collections/sample.yaml")
	return 0
}

// printInitHelpTo writes the init subcommand help text to w.
func printInitHelpTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: curlew init [dir] [options]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Initialize a new curlew project in [dir] (default: current directory).")
	_, _ = fmt.Fprintln(w, "Creates curlew.yaml, .gitignore, .env.example, environments/dev.yaml,")
	_, _ = fmt.Fprintln(w, "and collections/sample.yaml.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --project-name <name>   Override project name (default: directory basename)")
	_, _ = fmt.Fprintln(w, "  --output <format>       Scaffold an output: block for the named format.")
	_, _ = fmt.Fprintln(w, "                          One of: terminal, json, tap, junit, html, markdown.")
	_, _ = fmt.Fprintln(w, "                          markdown is recommended for VS Code + AI agent workflows.")
	_, _ = fmt.Fprintln(w, "  --skill <name>          Scaffold an agent skill at .claude/skills/curlew/SKILL.md.")
	_, _ = fmt.Fprintln(w, "                          One of: agent, claude (aliases for the same skill).")
	_, _ = fmt.Fprintln(w, "                          .claude/skills/ is read by Claude Code and by GitHub")
	_, _ = fmt.Fprintln(w, "                          Copilot, so one file serves both.")
	_, _ = fmt.Fprintln(w, "                          --skill agent defaults --output to markdown, appends")
	_, _ = fmt.Fprintln(w, "                          events: .curlew/run.ndjson to the output: block, and")
	_, _ = fmt.Fprintln(w, "                          adds .curlew/ to .gitignore.")
	_, _ = fmt.Fprintln(w, "  --help, -h              Show this help message")
}

// infoCmdOut displays project metadata (collections, environments, root path).
// Exit codes: 0 = success, 1 = usage error, 5 = no project found.
func infoCmdOut(args []string, stdout, stderr io.Writer) int {
	format, err := parseInfoArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	if format != "" && format != "json" {
		_, _ = fmt.Fprintf(stderr, "unknown output format %q (supported: json)\n", format)
		return 1
	}

	wd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	root, found := config.FindProjectRoot(wd)
	if !found {
		_, _ = fmt.Fprintln(stderr, "no curlew project found (no curlew.yaml in current or parent directories)")
		return 5
	}

	cfg, _, err := config.LoadProjectConfig(root)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 5
	}

	collections := config.ListCollections(root)
	if collections == nil {
		collections = []string{}
	}
	environments := config.ListAvailableEnvironments(root)
	if environments == nil {
		environments = []string{}
	}

	if format == "json" {
		out := &output.InfoJSONOutput{
			ProjectRoot:  root,
			ProjectName:  cfg.ProjectName,
			Collections:  collections,
			Environments: environments,
			Version:      resolvedVersion,
		}
		if writeErr := output.WriteInfoJSON(stdout, out); writeErr != nil {
			_, _ = fmt.Fprintf(stderr, "json encode error: %v\n", writeErr)
			return 1
		}
		return 0
	}

	// Human-readable output
	_, _ = fmt.Fprintf(stdout, "Project: %s\n", cfg.ProjectName)
	_, _ = fmt.Fprintf(stdout, "Root:    %s\n", root)
	_, _ = fmt.Fprintln(stdout)
	_, _ = fmt.Fprintln(stdout, "Collections:")
	if len(collections) == 0 {
		_, _ = fmt.Fprintln(stdout, "  (none)")
	} else {
		for _, c := range collections {
			_, _ = fmt.Fprintf(stdout, "  %s\n", c)
		}
	}
	_, _ = fmt.Fprintln(stdout)
	_, _ = fmt.Fprintln(stdout, "Environments:")
	if len(environments) == 0 {
		_, _ = fmt.Fprintln(stdout, "  (none)")
	} else {
		for _, e := range environments {
			_, _ = fmt.Fprintf(stdout, "  %s\n", e)
		}
	}

	return 0
}

// parseInfoArgs extracts --format from args.
func parseInfoArgs(args []string) (format string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			i++
			if i >= len(args) {
				return "", fmt.Errorf("--format requires a value (e.g. --format json)")
			}
			format = args[i]
		default:
			return "", fmt.Errorf("unknown argument: %s", args[i])
		}
	}
	return format, nil
}

// schemaCmdOut outputs the JSON Schema for either collection or project files.
// Exit codes: 0 = success, 1 = usage error.
func schemaCmdOut(args []string, stdout, stderr io.Writer) int {
	format, project, err := parseSchemaArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	if format != "" && format != "json" {
		_, _ = fmt.Fprintf(stderr, "unknown output format %q (supported: json)\n", format)
		return 1
	}

	data := schema.CollectionSchema
	if project {
		data = schema.ProjectSchema
	}
	_, _ = stdout.Write(data)
	// Ensure trailing newline
	if len(data) > 0 && data[len(data)-1] != '\n' {
		_, _ = fmt.Fprintln(stdout)
	}
	return 0
}

// parseSchemaArgs extracts --format and --project from args.
func parseSchemaArgs(args []string) (format string, project bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			i++
			if i >= len(args) {
				return "", false, fmt.Errorf("--format requires a value (e.g. --format json)")
			}
			format = args[i]
		case "--project":
			project = true
		default:
			return "", false, fmt.Errorf("unknown argument: %s", args[i])
		}
	}
	return format, project, nil
}

// ExecOptions holds parsed arguments for the exec command.
type ExecOptions struct {
	URL            string
	Method         string
	Stdin          bool
	DryRun         bool
	LogFile        string
	Format         string
	NonInteractive bool
	Color          colorMode
	Verbosity      output.Verbosity
	Vars           map[string]string
	EnvVars        map[string]string
	// M20-001: locale-aware faker support
	Seed   *int64
	Locale string
}

// parseExecArgs extracts flags and positional arguments for the exec command.
func parseExecArgs(args []string) (ExecOptions, error) {
	opts := ExecOptions{
		Vars:    make(map[string]string),
		EnvVars: make(map[string]string),
	}
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--stdin":
			opts.Stdin = true
		case "--dry-run":
			opts.DryRun = true
		case "--non-interactive":
			opts.NonInteractive = true
		case "--no-color":
			opts.Color = colorNever
		case "-v":
			opts.Verbosity = output.VerbosityVerbose
		case "-vv":
			opts.Verbosity = output.VerbosityDebug
		case "-q", "--quiet":
			opts.Verbosity = output.VerbosityQuiet
		case "--log":
			i++
			if i >= len(args) {
				return ExecOptions{}, fmt.Errorf("--log requires a value (e.g. --log output.jsonl)")
			}
			opts.LogFile = args[i]
		case "--format":
			i++
			if i >= len(args) {
				return ExecOptions{}, fmt.Errorf("--format requires a value (e.g. --format json)")
			}
			opts.Format = args[i]
		case "-X", "--method":
			i++
			if i >= len(args) {
				return ExecOptions{}, fmt.Errorf("-X/--method requires a value (e.g. -X POST)")
			}
			opts.Method = args[i]
		case "--var":
			i++
			if i >= len(args) {
				return ExecOptions{}, fmt.Errorf("--var requires a value (e.g. --var key=value)")
			}
			k, v, err := variable.ParseVarFlag(args[i])
			if err != nil {
				return ExecOptions{}, err
			}
			opts.Vars[k] = v
		case "--env-var":
			i++
			if i >= len(args) {
				return ExecOptions{}, fmt.Errorf("--env-var requires a value (e.g. --env-var API_KEY)")
			}
			k, v, err := variable.ParseEnvVarFlag(args[i], os.LookupEnv)
			if err != nil {
				return ExecOptions{}, err
			}
			opts.EnvVars[k] = v
		case "--seed":
			// M20-001: deterministic seed for faker functions in exec
			i++
			if i >= len(args) {
				return ExecOptions{}, fmt.Errorf("--seed requires a value (e.g. --seed 42)")
			}
			n, err := strconv.ParseInt(args[i], 10, 64)
			if err != nil {
				return ExecOptions{}, fmt.Errorf("--seed value must be an integer: %w", err)
			}
			opts.Seed = &n
		case "--locale":
			// M20-001: faker locale selection for exec
			i++
			if i >= len(args) {
				return ExecOptions{}, fmt.Errorf("--locale requires a value (e.g. --locale de-DE)")
			}
			opts.Locale = args[i]
		default:
			positional = append(positional, args[i])
		}
	}
	if opts.Stdin && len(positional) > 0 {
		return ExecOptions{}, fmt.Errorf("cannot use --stdin with a URL argument")
	}
	if !opts.Stdin && len(positional) == 0 {
		return ExecOptions{}, fmt.Errorf("no URL provided and --stdin not set")
	}
	if len(positional) > 0 {
		opts.URL = positional[0]
	}
	return opts, nil
}

// stdinRequest is the JSON structure for --stdin input.
type stdinRequest struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body"`
	Query   map[string]string `json:"query"`
}

// parseStdinRequest reads JSON from r and returns a parser.Request.
func parseStdinRequest(r io.Reader) (*parser.Request, error) {
	data, err := io.ReadAll(io.LimitReader(r, 10<<20)) // 10MB limit
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("no input received on stdin")
	}

	var req stdinRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid JSON input: %w", err)
	}

	if req.URL == "" {
		return nil, fmt.Errorf("missing required field \"url\" in stdin JSON")
	}

	method := req.Method
	if method == "" {
		method = "GET"
	}

	return &parser.Request{
		Method:      method,
		URL:         req.URL,
		Headers:     req.Headers,
		Body:        req.Body,
		QueryParams: req.Query,
	}, nil
}

// execCmdOut handles the "exec" subcommand for single-request execution.
// stdin is used when --stdin is passed; stdout receives request results; stderr receives errors.
// Exit codes: 0 = success, 1 = assertion/usage error, 3 = parse error, 4 = execution error.
func execCmdOut(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, parseErr := parseExecArgs(args)
	if parseErr != nil {
		_, _ = fmt.Fprintln(stderr, "Usage: curlew exec <url> [--stdin] [-X method] [--dry-run] [--log <file>] [--format <type>] [--non-interactive] [--var key=value ...] [--env-var VAR ...] [--color <when>] [--no-color] [-v] [-vv] [-q]")
		errOut := newStderrPrinterTo(stderr, colorAuto)
		errOut.StructuredError(parseErr)
		return 1
	}

	if opts.Format != "" && opts.Format != "json" && opts.Format != "terminal" {
		errOut := newStderrPrinterTo(stderr, opts.Color)
		errOut.StructuredError(fmt.Errorf("unknown output format %q (supported: terminal, json)", opts.Format))
		return 1
	}

	stdoutUseColor := shouldUseColor(stdout, opts.Color)

	// M11-004: Mint a single run_id per exec invocation. The same id flows into
	// every JSONL log entry written by this call (dry-run, exec error, success),
	// matching the run command's events-stream correlation contract.
	runID := ids.NewRunID()
	const execRequestID = "req-1" // exec is single-request; matches runner.nextRequestID's req-N convention.

	// Determine request source
	var req *parser.Request
	if opts.Stdin {
		var err error
		req, err = parseStdinRequest(stdin)
		if err != nil {
			if opts.Format == "json" {
				jsonOut := buildJSONOutput("", nil, nil, err, output.VerbosityDefault)
				_ = output.WriteJSON(stdout, jsonOut)
			} else {
				errOut := newStderrPrinterTo(stderr, opts.Color)
				errOut.StructuredError(err)
			}
			return 3
		}
	} else {
		method := opts.Method
		if method == "" {
			method = "GET"
		}
		req = &parser.Request{
			Method: method,
			URL:    opts.URL,
		}
	}

	// Apply method override from -X/--method if provided (overrides stdin method too)
	if opts.Method != "" {
		req.Method = opts.Method
	}

	// M20-001: Validate locale before building the registry so ERR_LOCALE_UNKNOWN
	// surfaces cleanly before any request is sent.
	if err := variable.ValidateLocale(opts.Locale); err != nil {
		errOut := newStderrPrinterTo(stderr, opts.Color)
		errOut.StructuredError(err)
		return 1
	}

	// Apply variable interpolation (always wired so $faker.* resolves in exec).
	{
		merged := make(map[string]string)
		for k, v := range opts.EnvVars {
			merged[k] = v
		}
		for k, v := range opts.Vars {
			merged[k] = v
		}
		scope := variable.NewScope(merged)
		if resolveErr := scope.Resolve(); resolveErr != nil {
			errOut := newStderrPrinterTo(stderr, opts.Color)
			errOut.StructuredError(resolveErr)
			return 3
		}
		// Build the dynamic registry with locale and optional warning sink.
		var lopts []variable.Option
		if opts.Locale != "" {
			lopts = append(lopts, variable.WithLocale(opts.Locale))
			if opts.Verbosity >= output.VerbosityVerbose {
				lopts = append(lopts, variable.WithLocaleWarning(func(m string) {
					_, _ = fmt.Fprintln(stderr, "curlew: "+m)
				}))
			}
		}
		reg := variable.NewRegistry(opts.Seed, lopts...)
		scope = scope.WithDynamic(reg)

		var interpErr error
		req.URL, interpErr = scope.Interpolate(req.URL)
		if interpErr != nil {
			errOut := newStderrPrinterTo(stderr, opts.Color)
			errOut.StructuredError(interpErr)
			return 3
		}
		if req.Headers != nil {
			for k, v := range req.Headers {
				req.Headers[k], interpErr = scope.Interpolate(v)
				if interpErr != nil {
					errOut := newStderrPrinterTo(stderr, opts.Color)
					errOut.StructuredError(interpErr)
					return 3
				}
			}
		}
		if bodyStr, ok := req.Body.(string); ok && bodyStr != "" {
			req.Body, interpErr = scope.Interpolate(bodyStr)
			if interpErr != nil {
				errOut := newStderrPrinterTo(stderr, opts.Color)
				errOut.StructuredError(interpErr)
				return 3
			}
		}
	}

	// Dry-run mode
	if opts.DryRun {
		if opts.Format == "json" {
			jsonOut := &output.JSONOutput{
				Status:  "passed",
				Summary: buildSummaryJSON(nil), // M11-002: summary must never be null
				Requests: []output.JSONRequest{
					{
						Name:       "exec",
						Status:     "skipped",
						Method:     req.Method,
						URL:        req.URL,
						Assertions: make([]output.JSONAssertion, 0),
					},
				},
			}
			_ = output.WriteJSON(stdout, jsonOut)
		} else {
			_, _ = fmt.Fprintln(stdout, "DRY RUN")
			// Force VerbosityVerbose so RequestDetail renders regardless of -q/-v.
			// --dry-run is itself a verbose intent: the user asked to see the request.
			dryPrinter := output.NewPrinter(stdout, stdoutUseColor, output.VerbosityVerbose)
			dryPrinter.RequestDetail(req.Method, req.URL, req.Headers)
		}
		if opts.LogFile != "" {
			entry := &output.JSONLEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Method:    req.Method,
				URL:       req.URL,
				DryRun:    true,
				RunID:     runID,
				RequestID: execRequestID,
			}
			if logErr := output.AppendJSONL(opts.LogFile, entry); logErr != nil {
				_, _ = fmt.Fprintf(stderr, "log error: %v\n", logErr)
			}
		}
		return 0
	}

	// Execute the request
	ctx := context.Background()
	result, execErr := httpexec.Execute(ctx, &httpexec.Request{
		Method:      req.Method,
		URL:         req.URL,
		Headers:     req.Headers,
		Body:        req.Body,
		QueryParams: req.QueryParams,
	})
	if execErr != nil {
		if opts.Format == "json" {
			jsonOut := buildJSONOutput("", nil, nil, execErr, output.VerbosityDefault)
			_ = output.WriteJSON(stdout, jsonOut)
		} else {
			errOut := newStderrPrinterTo(stderr, opts.Color)
			errOut.StructuredError(execErr)
		}
		if opts.LogFile != "" {
			entry := &output.JSONLEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Method:    req.Method,
				URL:       req.URL,
				Error:     execErr.Error(),
				RunID:     runID,
				RequestID: execRequestID,
			}
			if logErr := output.AppendJSONL(opts.LogFile, entry); logErr != nil {
				_, _ = fmt.Fprintf(stderr, "log error: %v\n", logErr)
			}
		}
		return 4
	}

	// Build and output results
	rr := runner.RequestResult{
		Name:   "exec",
		Method: req.Method,
		URL:    req.URL,
		Result: result,
	}

	if opts.Format == "json" {
		jsonOut := buildJSONOutput("", []runner.RequestResult{rr}, &runner.Summary{
			Total:    1,
			Passed:   1,
			Duration: result.Duration,
		}, nil, opts.Verbosity)
		if writeErr := output.WriteJSON(stdout, jsonOut); writeErr != nil {
			_, _ = fmt.Fprintf(stderr, "json encode error: %v\n", writeErr)
			return 1
		}
	} else {
		out := output.NewPrinter(stdout, stdoutUseColor, opts.Verbosity)
		out.RequestDetail(req.Method, req.URL, req.Headers)
		out.Result(rr.Name, result, true, 0)
		if opts.Verbosity >= output.VerbosityVerbose && result != nil {
			out.ResponseDetail(result.StatusCode, result.Headers)
		}
		if opts.Verbosity >= output.VerbosityDebug && result != nil {
			out.ResponseBodyDump(result.Body)
		}
	}

	if opts.LogFile != "" {
		entry := &output.JSONLEntry{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Method:     req.Method,
			URL:        req.URL,
			StatusCode: result.StatusCode,
			DurationMs: result.Duration.Milliseconds(),
			RunID:      runID,
			RequestID:  execRequestID,
		}
		if logErr := output.AppendJSONL(opts.LogFile, entry); logErr != nil {
			_, _ = fmt.Fprintf(stderr, "log error: %v\n", logErr)
		}
	}

	return 0
}

// vaultCmdOut handles the vault subcommand tree.
func vaultCmdOut(args []string, stdout, stderr io.Writer) int {
	subcommand, format, _, err := parseVaultArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	switch subcommand {
	case "list":
		return vaultListCmdOut(format, stdout, stderr)
	default:
		printVaultHelpTo(stdout)
		return 0
	}
}

// vaultListCmdOut lists configured vault provider profiles.
func vaultListCmdOut(format string, stdout, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	cfg, _, err := config.LoadProjectConfig(wd)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error loading project config: %v\n", err)
		return 1
	}

	if cfg.Secrets == nil {
		if format == "json" {
			out := &output.VaultListJSONOutput{
				Provider: "",
				Keys:     []output.VaultKeyJSON{},
			}
			if writeErr := output.WriteVaultListJSON(stdout, out); writeErr != nil {
				_, _ = fmt.Fprintf(stderr, "json encode error: %v\n", writeErr)
				return 1
			}
		} else {
			_, _ = fmt.Fprintln(stdout, "No vault profiles configured.")
			_, _ = fmt.Fprintln(stdout)
			_, _ = fmt.Fprintln(stdout, "Add a secrets block to curlew.yaml to configure vault providers.")
		}
		return 0
	}

	// Sort keys for deterministic output
	keyNames := make([]string, 0, len(cfg.Secrets.Keys))
	for name := range cfg.Secrets.Keys {
		keyNames = append(keyNames, name)
	}
	sort.Strings(keyNames)

	if format == "json" {
		keys := make([]output.VaultKeyJSON, 0, len(keyNames))
		for _, name := range keyNames {
			keys = append(keys, output.VaultKeyJSON{
				Name: name,
				Path: cfg.Secrets.Keys[name],
			})
		}
		out := &output.VaultListJSONOutput{
			Provider: cfg.Secrets.Provider,
			Keys:     keys,
		}
		if writeErr := output.WriteVaultListJSON(stdout, out); writeErr != nil {
			_, _ = fmt.Fprintf(stderr, "json encode error: %v\n", writeErr)
			return 1
		}
	} else {
		_, _ = fmt.Fprintf(stdout, "Provider: %s (%d key(s) configured)\n", cfg.Secrets.Provider, len(cfg.Secrets.Keys))
		_, _ = fmt.Fprintln(stdout)
		for _, name := range keyNames {
			_, _ = fmt.Fprintf(stdout, "  %s → %s\n", name, cfg.Secrets.Keys[name])
		}
	}

	return 0
}

// printVaultHelpTo writes vault help to w.
func printVaultHelpTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: curlew vault <subcommand>")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Subcommands:")
	_, _ = fmt.Fprintln(w, "  vault list      List configured vault provider profiles")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --format json   Output in JSON format")
	_, _ = fmt.Fprintln(w, "  --color <when>  auto (default) | always | never")
	_, _ = fmt.Fprintln(w, "  --no-color      Disable colored output (same as --color=never)")
}

// parseVaultArgs extracts the subcommand, --format, and --no-color from vault args.
func parseVaultArgs(args []string) (subcommand, format string, noColor bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			i++
			if i >= len(args) {
				return "", "", false, fmt.Errorf("--format requires a value (e.g. --format json)")
			}
			format = args[i]
		case "--no-color":
			noColor = true
		case "list":
			if subcommand != "" {
				return "", "", false, fmt.Errorf("unexpected argument: %s", args[i])
			}
			subcommand = "list"
		default:
			return "", "", false, fmt.Errorf("unknown argument: %s", args[i])
		}
	}
	return subcommand, format, noColor, nil
}

// filterShowDepsItems applies the --only selection to col.Requests.Items before
// passing them to parallel.Analyze in the --show-dependencies path. This ensures
// the dependency graph visualisation reflects the same filtered subset that
// runner.Run will execute at runtime (M8-004 edge-case requirement).
// Delegates to runner.FilterMainItems so filtering logic and error formatting
// stay in sync with the runner (finding #3/#4 from review iteration 2).
func filterShowDepsItems(items []parser.RequestItem, selection []string) ([]parser.RequestItem, error) {
	return runner.FilterMainItems(items, selection)
}

// usageSynopses holds the one-line "Usage: ..." strings for each subcommand
// keyed by the subcommand name ("" = top-level curlew). Keep each value in
// sync with the first Usage line of the corresponding print*HelpTo function;
// TestUsageSynopsis_MatchesPrintHelpFirstLine asserts the two stay in sync.
var usageSynopses = map[string]string{
	"":          "Usage: curlew <command> [arguments]",
	"perf":      "Usage: curlew perf <request-file> [options]",
	"ui":        "Usage: curlew ui [--port <n>] [--env <name>] [--collection <file>] [--no-open] [--no-color]",
	"plugins":   "Usage: curlew plugins <subcommand>",
	"import":    "Usage: curlew import <format> <spec-path>",
	"telemetry": "Usage: curlew telemetry <subcommand>",
}

// usageSynopsis returns the one-line "Usage: ..." synopsis for the named
// subcommand ("" for the top-level curlew command). Returns the top-level
// synopsis as a fallback for unknown keys so callers never emit an empty
// string on an error path.
func usageSynopsis(cmd string) string {
	if s, ok := usageSynopses[cmd]; ok {
		return s
	}
	return usageSynopses[""]
}

// printHelpTo writes the top-level usage text to w.
func printHelpTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "curlew — a file-based API testing tool")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Version: %s\n", resolvedVersion)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  curlew <command> [arguments]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  run <file>      Execute requests in a collection file")
	_, _ = fmt.Fprintln(w, "  run <pattern>   Run all collections matching a glob")
	_, _ = fmt.Fprintln(w, "  exec <url>      Execute a single request (for AI agents and scripts)")
	_, _ = fmt.Fprintln(w, "  validate <file> Validate collection files without executing requests")
	_, _ = fmt.Fprintln(w, "                  Also validates shared vault configuration templates (team_secrets.vault_configs)")
	_, _ = fmt.Fprintln(w, "  init [dir]      Initialize a new curlew project (use --output <fmt> to scaffold an output: block; use --skill agent for an agent skill)")
	_, _ = fmt.Fprintln(w, "  info            Show project metadata (collections, environments, root)")
	_, _ = fmt.Fprintln(w, "  schema          Output JSON Schema for the collection format")
	_, _ = fmt.Fprintln(w, "                  Use --project for the curlew.yaml project-config schema")
	_, _ = fmt.Fprintln(w, "  watch <file>    Watch collection and re-run on file changes")
	_, _ = fmt.Fprintln(w, "  vault           Manage vault provider profiles")
	_, _ = fmt.Fprintln(w, "  vault list      List configured vault provider profiles")
	_, _ = fmt.Fprintln(w, "  import openapi  Import OpenAPI 3.x spec into a collection with headers, request bodies, and status assertions")
	_, _ = fmt.Fprintln(w, "  pr-check        Gate CI on a run's results file (exit 1 on failures)")
	_, _ = fmt.Fprintln(w, "  ui              Start the local web UI (runner & inspector)")
	_, _ = fmt.Fprintln(w, "  perf <file>     Run a load test against a single request")
	_, _ = fmt.Fprintln(w, "  plugins         Manage external-process plugins")
	_, _ = fmt.Fprintln(w, "  plugins list    Discover plugins and print registered capabilities")
	_, _ = fmt.Fprintln(w, "  telemetry       Record anonymous usage events to a local file (opt-in)")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Exec Options:")
	_, _ = fmt.Fprintln(w, "  --stdin             Read request JSON from stdin")
	_, _ = fmt.Fprintln(w, "  -X, --method <M>    HTTP method (default: GET)")
	_, _ = fmt.Fprintln(w, "  --dry-run           Show request details without executing")
	_, _ = fmt.Fprintln(w, "  --log <file>        Append structured JSONL log entry to file")
	_, _ = fmt.Fprintln(w, "  --non-interactive   Suppress interactive prompts on errors")
	_, _ = fmt.Fprintln(w, "  --format <type>     Output format: terminal (default), json")
	_, _ = fmt.Fprintln(w, "  --var key=value     Set a variable (repeatable)")
	_, _ = fmt.Fprintln(w, "  --env-var VAR       Import OS environment variable (repeatable)")
	_, _ = fmt.Fprintln(w, "  --seed <number>     Seed for deterministic random variable functions (e.g. $faker.*)")
	_, _ = fmt.Fprintln(w, "  --locale <code>     Faker locale for $faker.* functions (default en-US; e.g. de-DE)")
	_, _ = fmt.Fprintln(w, "  --color <when>      auto (default) | always | never")
	_, _ = fmt.Fprintln(w, "  --no-color          Disable colored output (same as --color=never)")
	_, _ = fmt.Fprintln(w, "  -v / -vv / -q       Verbosity control")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Run Options:")
	_, _ = fmt.Fprintln(w, "  --env <name>        Load environment file (from environments/<name>.yaml)")
	_, _ = fmt.Fprintln(w, "                      Also selects shared vault template environment")
	_, _ = fmt.Fprintln(w, "  --env-var VAR_NAME  Import OS environment variable (repeatable)")
	_, _ = fmt.Fprintln(w, "  --env-var VAR=$OS   Import and rename OS environment variable")
	_, _ = fmt.Fprintln(w, "  --var key=value     Set a variable (overrides all other sources, repeatable)")
	_, _ = fmt.Fprintln(w, "  --seed <number>     Seed for deterministic random variable functions")
	_, _ = fmt.Fprintln(w, "  --locale <code>     Faker locale for $faker.* functions (default en-US; e.g. de-DE)")
	_, _ = fmt.Fprintln(w, "                      Supported: en-US, en-GB, de-DE, fr-FR, es-ES, it-IT, pt-BR,")
	_, _ = fmt.Fprintln(w, "                                 ja-JP, zh-CN, ko-KR, nl-NL, pl-PL, ru-RU, sv-SE, tr-TR")
	_, _ = fmt.Fprintln(w, "  --format <type>     Output format: terminal (default), json, tap, junit, html, markdown")
	_, _ = fmt.Fprintln(w, "  --report <file>     Write report to file (required for --format html, optional for --format junit)")
	_, _ = fmt.Fprintln(w, "                      --format markdown requires --report <dir>")
	_, _ = fmt.Fprintln(w, "  --events <file>     Write an NDJSON event stream for the run (schema v1.6)")
	_, _ = fmt.Fprintln(w, "                      One JSON object per line; see docs/EVENTS_SCHEMA_v1.6.md")
	_, _ = fmt.Fprintln(w, "  --color <when>      auto (default) | always | never. always forces colour on a pipe")
	_, _ = fmt.Fprintln(w, "  --no-color          Disable colored output (same as --color=never; auto also respects NO_COLOR)")
	_, _ = fmt.Fprintln(w, "  -v                  Verbose: show request/response headers")
	_, _ = fmt.Fprintln(w, "  -vv                 Very verbose: full HTTP request/response dump")
	_, _ = fmt.Fprintln(w, "  -q, --quiet         Quiet: summary line only")
	_, _ = fmt.Fprintln(w, "  --allow-sensitive   Show sensitive values in plain text (default: redact to [REDACTED])")
	_, _ = fmt.Fprintln(w, "  --parallel          Execute independent requests concurrently")
	_, _ = fmt.Fprintln(w, "  --confirm-large-dataset  Confirm execution of data files with >10,000 rows")
	_, _ = fmt.Fprintln(w, "  --show-dependencies Show dependency graph (DOT format) without executing")
	_, _ = fmt.Fprintln(w, "  --dry-run           With --show-dependencies: show execution waves")
	_, _ = fmt.Fprintln(w, "  --only \"<name>\"     Run only the named main request; repeatable for a union (e.g. --only \"Get user\" --only \"Update user\")")
	_, _ = fmt.Fprintln(w, "                      Setup and teardown still run in full. Fails with exit 3 when no match is found.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Request Item Fields (in collection YAML):")
	_, _ = fmt.Fprintln(w, "  if: <CEL bool>      Skip this request unless the CEL expression evaluates to true.")
	_, _ = fmt.Fprintln(w, "                      Evaluated before templating; references vars.<name>, env.<name>,")
	_, _ = fmt.Fprintln(w, "                      and previous.{status,headers,body} (last non-skipped response).")
	_, _ = fmt.Fprintln(w, "                      Example: if: 'previous.body.status == \"pending\"'")
	_, _ = fmt.Fprintln(w, "  depends_on: [<name>]  Skip this request if any listed item was skipped in the same")
	_, _ = fmt.Fprintln(w, "                      phase. Names must match an existing request item (case-sensitive).")
	_, _ = fmt.Fprintln(w, "                      Example: depends_on: [confirm-order]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Perf Options:")
	_, _ = fmt.Fprintln(w, "  --vus <n>           Number of virtual users (concurrent workers)")
	_, _ = fmt.Fprintln(w, "  --duration <d>      Total run duration (e.g. 30s, 2m)")
	_, _ = fmt.Fprintln(w, "  --ramp-up <d>       Linearly ramp VU count from 1 to --vus over this window")
	_, _ = fmt.Fprintln(w, "  --rps <n>           Target throughput in requests/sec (0 = unbounded)")
	_, _ = fmt.Fprintln(w, "  --output <dest>     Output destination (stdout|json|html). Only 'stdout' supported currently.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Glob Discovery:")
	_, _ = fmt.Fprintln(w, "  Patterns support *, ?, [abc], and ** (multi-segment wildcard)")
	_, _ = fmt.Fprintln(w, "  Honors .curlewignore in the working directory")
	_, _ = fmt.Fprintln(w, "  Example: curlew run \"tests/**/*_test.yaml\"")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Watch Options:")
	_, _ = fmt.Fprintln(w, "  Accepts all Run Options, plus:")
	_, _ = fmt.Fprintln(w, "  --clear             Clear terminal between re-runs")
	_, _ = fmt.Fprintln(w, "  --format json       Suppress terminal decorations, output raw JSON per run")
	_, _ = fmt.Fprintln(w, "  Ctrl+C              Stop watching and exit")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Shared Vault Templates:")
	_, _ = fmt.Fprintln(w, "  CURLEW_TEAM_CONFIG=path   Load a shared vault configuration template")
	_, _ = fmt.Fprintln(w, "                             {{secrets.ALIAS}} references in collections resolve")
	_, _ = fmt.Fprintln(w, "                             through the template for the environment named by --env")
	_, _ = fmt.Fprintln(w, "  CURLEW_VAULT_STUB=1       Use an in-memory stub provider (for local/CI testing)")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Plugins:")
	_, _ = fmt.Fprintln(w, "  curlew plugins list    Discover plugins from CURLEW_PLUGINS and show their")
	_, _ = fmt.Fprintln(w, "                          name, version, and registered hooks.")
	_, _ = fmt.Fprintln(w, "  Env vars:  CURLEW_PLUGINS   Colon-separated list of plugin executables or")
	_, _ = fmt.Fprintln(w, "                               directories (see docs/plugins.md)")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Plugin hooks:")
	_, _ = fmt.Fprintln(w, "  Plugins may register three lifecycle hooks, called in declared order:")
	_, _ = fmt.Fprintln(w, "    on_request   — called before each HTTP request is sent; may mutate")
	_, _ = fmt.Fprintln(w, "                   the method/url/headers/body/query_params.")
	_, _ = fmt.Fprintln(w, "    on_response  — called after each response; may attach annotations")
	_, _ = fmt.Fprintln(w, "                   (status/headers/body are not replaceable).")
	_, _ = fmt.Fprintln(w, "    on_result    — called once at run completion with pass/fail counts")
	_, _ = fmt.Fprintln(w, "                   and per-test rows.")
	_, _ = fmt.Fprintln(w, "  Per-hook timeout: 10 seconds. A timed-out plugin is terminated and")
	_, _ = fmt.Fprintln(w, "  the run continues as if the hook were not registered.")
	_, _ = fmt.Fprintln(w, "  Set CURLEW_PLUGINS=/path/to/plugin[:...] to enable.")
	_, _ = fmt.Fprintln(w, "  Retry attempts and data-driven iterations fire hooks per attempt/iteration.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Auto-loaded:")
	_, _ = fmt.Fprintln(w, "  curlew.yaml        Project config with global variables (optional, walks up from collection dir)")
	_, _ = fmt.Fprintln(w, "  .env                Local secrets (KEY=VALUE format, optional)")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --version    Show version information")
	_, _ = fmt.Fprintln(w, "  --help       Show this help message")
}

// importCmdOut dispatches import subcommands.
func importCmdOut(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, usageSynopsis("import"))
		_, _ = fmt.Fprintln(stderr, "Formats:")
		_, _ = fmt.Fprintln(stderr, "  openapi    Import an OpenAPI 3.0/3.1 spec")
		return 1
	}
	switch args[0] {
	case "openapi":
		return importOpenAPICmdOut(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "Unknown import format: %s\n", args[0])
		_, _ = fmt.Fprintln(stderr, usageSynopsis("import"))
		return 1
	}
}

// importOpenAPICmdOut imports an OpenAPI spec and writes a collection with headers,
// request bodies, and status-code assertions derived from the spec.
func importOpenAPICmdOut(args []string, stdout, stderr io.Writer) int {
	specPath, outputPath, err := parseImportOpenAPIArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Usage: curlew import openapi <spec-path> [--output <file>]")
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	col, importErr := openapi.Import(specPath)
	if importErr != nil {
		errOut := newStderrPrinterTo(stderr, colorAuto)
		errOut.StructuredError(importErr)
		return 3
	}

	w := stdout
	if outputPath != "" {
		f, openErr := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if openErr != nil {
			_, _ = fmt.Fprintf(stderr, "Error: %v\n", openErr)
			return 3
		}
		defer f.Close() //nolint:errcheck
		w = f
	}
	if emitErr := openapi.Emit(w, col); emitErr != nil {
		_, _ = fmt.Fprintf(stderr, "Error writing collection: %v\n", emitErr)
		return 1
	}
	return 0
}

// prCheckCmdOut implements the pr-check subcommand. It reads a local results
// file and reports the CI verdict; exit 1 when the run contained failures.
func prCheckCmdOut(args []string, stdout, stderr io.Writer) int {
	cfg, showHelp, err := parsePrCheckArgs(args)
	if showHelp {
		printPrCheckHelpTo(stdout)
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		printPrCheckHelpTo(stdout)
		return 2
	}

	result, runErr := prcheck.Run(cfg, stdout)
	if runErr != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", runErr)
		return 2
	}

	if cfg.DryRun {
		return 0
	}

	if cfg.SummaryFile != "" {
		_, _ = fmt.Fprintf(stdout, "%s: pass=%d fail=%d; summary written to %s\n",
			result.State, result.Pass, result.Fail, cfg.SummaryFile)
	} else {
		_, _ = fmt.Fprintf(stdout, "%s: pass=%d fail=%d\n", result.State, result.Pass, result.Fail)
	}

	if result.State == "failure" {
		return 1
	}
	return 0
}

// parsePrCheckArgs parses flags for the pr-check subcommand.
// Returns the config, whether --help was requested, and any parse error.
func parsePrCheckArgs(args []string) (cfg prcheck.Config, showHelp bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return cfg, true, nil
		case "--results":
			i++
			if i >= len(args) {
				return cfg, false, fmt.Errorf("--results requires a file path")
			}
			cfg.ResultsFile = args[i]
		case "--summary":
			i++
			if i >= len(args) {
				return cfg, false, fmt.Errorf("--summary requires a file path")
			}
			cfg.SummaryFile = args[i]
		case "--dry-run":
			cfg.DryRun = true
		case "--events":
			return cfg, false, fmt.Errorf("--events is supported only on run; use --format jsonl for streaming samples")
		default:
			return cfg, false, fmt.Errorf("unknown flag: %s", args[i])
		}
	}
	return cfg, false, nil
}

// printPrCheckHelpTo writes pr-check help to w.
func printPrCheckHelpTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: curlew pr-check [options]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Turn a run's results file into a CI gate: report the pass/fail verdict")
	_, _ = fmt.Fprintln(w, "and exit 1 when the run contained failures. Nothing is transmitted.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --results <file>    Path to test results JSON file (required)")
	_, _ = fmt.Fprintln(w, "  --summary <file>    Write the verdict as JSON to this path")
	_, _ = fmt.Fprintln(w, "  --dry-run           Print the summary instead of writing it")
	_, _ = fmt.Fprintln(w, "  --help              Show this help message")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Exit codes:")
	_, _ = fmt.Fprintln(w, "  0  all tests passed")
	_, _ = fmt.Fprintln(w, "  1  the results file contains failures")
	_, _ = fmt.Fprintln(w, "  2  usage error or unreadable results file")
}

// loadTeamTemplate loads the shared vault template using the backend-cache+overlay loader.
// Returns (nil, nil) when CURLEW_TEAM_CONFIG is not set. The template is read
// from the local filesystem — curlew has no vault backend to fetch from.
func loadTeamTemplate(localPath string, warnW io.Writer) (*teamtmpl.LoadResult, error) {
	return teamtmpl.Load(teamtmpl.LoadOptions{
		LocalPath: localPath,
		Warn:      warnW,
	})
}

// parseImportOpenAPIArgs extracts the spec path and optional --output flag.
func parseImportOpenAPIArgs(args []string) (specPath, outputPath string, err error) {
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output", "-o":
			i++
			if i >= len(args) {
				return "", "", fmt.Errorf("--output requires a file path")
			}
			outputPath = args[i]
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) == 0 {
		return "", "", fmt.Errorf("missing openapi spec path")
	}
	if len(positional) > 1 {
		return "", "", fmt.Errorf("unexpected extra arguments: %v", positional[1:])
	}
	return positional[0], outputPath, nil
}

// showDepsOtherPhaseNames mirrors the runner's view of which names belong to
// another phase, so --show-dependencies rejects what a run would reject rather
// than drawing a graph for a collection that will not start.
func showDepsOtherPhaseNames(col *parser.Collection) map[string]bool {
	out := make(map[string]bool, len(col.Setup.Items)+len(col.Teardown.Items))
	for _, section := range [][]parser.RequestItem{col.Setup.Items, col.Teardown.Items} {
		for _, item := range section {
			if item.Name != "" {
				out[item.Name] = true
			}
		}
	}
	return out
}
