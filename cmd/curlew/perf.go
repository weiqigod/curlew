package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/weiqigod/curlew/internal/loadgen"
	"github.com/weiqigod/curlew/internal/loadgen/report"
)

// perfFlags holds the parsed flags for the perf subcommand.
type perfFlags struct {
	file     string
	vus      int
	duration time.Duration
	rampUp   time.Duration
	rps      int
	output   string // "stdout" (default), "path.json", or "path.html"
}

// perfCmd implements the perf subcommand using os.Stdout and os.Stderr.
func perfCmd(args []string) int {
	return perfCmdOut(args, os.Stdout, os.Stderr)
}

// perfCmdOut is the writer-injectable core of perfCmd: run a load test against
// a single HTTP request file. stdout carries only the declared result payload
// (the final "Results: requests=..." summary line); stderr carries progress
// lines, warnings, and error messages.
//
// Exit codes:
//
//	0   = success (no failures)
//	1   = at least one request failure
//	2   = usage / validation error
//	3   = request file error
//	130 = interrupted by SIGINT
func perfCmdOut(args []string, stdout, stderr io.Writer) int {
	flags, showHelp, err := parsePerfArgs(args)
	if showHelp {
		printPerfHelpTo(stdout)
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		_, _ = fmt.Fprintln(stderr, usageSynopsis("perf"))
		return 2
	}

	// Output destination validation — fail fast before the run starts.
	format, outPath, fmtErr := report.DetectFormat(flags.output)
	if fmtErr != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", fmtErr)
		return 2
	}

	req, loadErr := loadgen.LoadRequestFile(flags.file)
	if loadErr != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", loadErr)
		return 3
	}

	cfg := loadgen.Config{
		VUs:      flags.vus,
		Duration: flags.duration,
		RampUp:   flags.rampUp,
		RPS:      flags.rps,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Header lines before run — progress goes to stderr.
	_, _ = fmt.Fprintf(stderr, "Load test: %d virtual users, %s duration, %s ramp-up\n",
		flags.vus, flags.duration, flags.rampUp)
	if flags.rps > 0 {
		_, _ = fmt.Fprintf(stderr, "Target rate: %d req/s\n", flags.rps)
	}
	_, _ = fmt.Fprintf(stderr, "VUs: 1 ... %d (ramped in %s)\n", flags.vus, flags.rampUp)
	_, _ = fmt.Fprintln(stderr, "Running...")

	// Wire the aggregator to collect per-request samples.
	agg := report.New()
	runOpts := loadgen.RunOptions{
		OnSample: func(s loadgen.Sample) {
			agg.Observe(report.Sample{
				StartOffsetNs: s.StartOffsetNs,
				LatencyNs:     s.LatencyNs,
				Success:       s.Success,
			})
		},
	}

	sum, runErr := loadgen.Run(ctx, cfg, req, runOpts)
	if runErr != nil {
		if errors.Is(runErr, loadgen.ErrInvalidVUs) ||
			errors.Is(runErr, loadgen.ErrInvalidDuration) ||
			errors.Is(runErr, loadgen.ErrInvalidRampUp) ||
			errors.Is(runErr, loadgen.ErrInvalidRPS) {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", runErr)
			return 2
		}
		_, _ = fmt.Fprintf(stderr, "error: %v\n", runErr)
		return 1
	}

	_, _ = fmt.Fprintf(stderr, "Requests sent: %d; successes: %d; failures: %d\n",
		sum.Requests, sum.Successes, sum.Failures)

	// Compute metrics from the aggregated samples.
	metrics := agg.Metrics(sum.Elapsed)
	_, _ = fmt.Fprintln(stdout, report.SummaryLine(metrics))

	// Write report file if requested.
	switch format {
	case report.FormatJSON:
		if err := writeReportFile(outPath, func(w io.Writer) error {
			return report.WriteJSON(w, metrics, "curlew perf report")
		}); err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stderr, "Wrote %s\n", outPath)
	case report.FormatHTML:
		if err := writeReportFile(outPath, func(w io.Writer) error {
			return report.WriteHTML(w, metrics, "curlew perf report")
		}); err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stderr, "Wrote %s\n", outPath)
	}

	switch {
	case sum.Aborted:
		return 130 // SIGINT
	case sum.Failures > 0:
		return 1
	default:
		return 0
	}
}

// writeReportFile opens path for writing (truncating any existing content),
// calls write to populate it, and closes it. Any error is wrapped with context.
func writeReportFile(path string, write func(io.Writer) error) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := write(f); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", path, err)
	}
	return nil
}

// parsePerfArgs parses flags for the perf subcommand.
func parsePerfArgs(args []string) (perfFlags, bool, error) {
	f := perfFlags{}
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return f, true, nil
		case "--vus":
			i++
			if i >= len(args) {
				return f, false, fmt.Errorf("--vus requires a value")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return f, false, fmt.Errorf("--vus must be an integer: %w", err)
			}
			f.vus = n
		case "--duration":
			i++
			if i >= len(args) {
				return f, false, fmt.Errorf("--duration requires a value (e.g. 30s)")
			}
			d, err := time.ParseDuration(args[i])
			if err != nil {
				return f, false, fmt.Errorf("--duration invalid: %w", err)
			}
			f.duration = d
		case "--ramp-up":
			i++
			if i >= len(args) {
				return f, false, fmt.Errorf("--ramp-up requires a value (e.g. 5s)")
			}
			d, err := time.ParseDuration(args[i])
			if err != nil {
				return f, false, fmt.Errorf("--ramp-up invalid: %w", err)
			}
			f.rampUp = d
		case "--rps":
			i++
			if i >= len(args) {
				return f, false, fmt.Errorf("--rps requires a value")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return f, false, fmt.Errorf("--rps must be an integer: %w", err)
			}
			f.rps = n
		case "--output":
			i++
			if i >= len(args) {
				return f, false, fmt.Errorf("--output requires a value")
			}
			f.output = args[i]
		case "--events":
			return f, false, fmt.Errorf("--events is supported only on run; use curlew run --events <file> to record run events")
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return f, false, fmt.Errorf("unknown flag: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return f, false, fmt.Errorf("perf requires exactly one positional argument: <request-file>")
	}
	f.file = positional[0]
	// Validate early for friendly exit-code-2 messages before entering loadgen.
	if f.vus < 1 {
		return f, false, fmt.Errorf("--vus must be >= 1")
	}
	if f.duration <= 0 {
		return f, false, fmt.Errorf("--duration must be > 0")
	}
	return f, false, nil
}

// printPerfHelpTo writes the perf subcommand usage to w.
func printPerfHelpTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: curlew perf <request-file> [options]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Run a load test against a single HTTP request.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Required:")
	_, _ = fmt.Fprintln(w, "  <request-file>     Path to a YAML file defining { name, request }")
	_, _ = fmt.Fprintln(w, "  --vus <n>          Virtual users (>= 1)")
	_, _ = fmt.Fprintln(w, "  --duration <d>     Run duration (> 0, e.g. 30s, 2m)")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --ramp-up <d>      Linear ramp from 1 -> <vus> over this window (default 0)")
	_, _ = fmt.Fprintln(w, "  --rps <n>          Target throughput in requests/sec (default 0 = unbounded)")
	_, _ = fmt.Fprintln(w, "  --output <dest>    Output destination — extension selects format:")
	_, _ = fmt.Fprintln(w, "                       .json    write JSON time-series + metrics")
	_, _ = fmt.Fprintln(w, "                       .html    write self-contained HTML (Chart.js CDN)")
	_, _ = fmt.Fprintln(w, "                       stdout   print summary only (default)")
	_, _ = fmt.Fprintln(w, "  --help, -h         Show this help message")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Exit codes: 0=ok, 1=failures, 2=usage, 3=request-file error, 130=SIGINT")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  curlew perf request.yaml --vus 10 --duration 60s")
	_, _ = fmt.Fprintln(w, "  curlew perf request.yaml --vus 10 --duration 60s --output report.json")
	_, _ = fmt.Fprintln(w, "  curlew perf request.yaml --vus 20 --duration 2m --ramp-up 30s --output report.html")
}
