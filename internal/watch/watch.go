package watch

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// RunResult holds structured feedback from a single test run.
type RunResult struct {
	ExitCode int
	Passed   int
	Failed   int
	Skipped  int
	Total    int
}

// RunningTotals accumulates pass/fail counts across multiple watch runs.
type RunningTotals struct {
	Runs    int
	Passed  int
	Failed  int
	Skipped int
	Total   int
}

// Add incorporates a single run's results into the running totals.
func (rt *RunningTotals) Add(r RunResult) {
	rt.Runs++
	rt.Passed += r.Passed
	rt.Failed += r.Failed
	rt.Skipped += r.Skipped
	rt.Total += r.Total
}

// Config holds the configuration for a watch session.
type Config struct {
	CollectionPath string                                         // path to collection file
	Args           []string                                       // full CLI args to pass to RunFunc on each re-run
	EnvName        string                                         // --env flag value (for path collection)
	Debounce       time.Duration                                  // debounce interval (default 500ms if zero)
	Stdout         io.Writer                                      // watch status messages
	Stderr         io.Writer                                      // error output
	UseColor       bool                                           // ANSI color codes
	Format         string                                         // "json" or "" (terminal)
	ClearScreen    bool                                           // --clear flag
	RunFunc        func([]string, io.Writer, io.Writer) RunResult // function called on each run; receives Args and stdout/stderr writers, returns structured result
}

// Run starts the file watch loop. Blocks until ctx is cancelled.
// Performs an initial run, then re-runs whenever watched files change.
// Returns 0 on clean shutdown.
func Run(ctx context.Context, cfg Config) int {
	if cfg.Debounce == 0 {
		cfg.Debounce = 500 * time.Millisecond
	}

	var totals RunningTotals

	// Initial run.
	result := cfg.RunFunc(cfg.Args, cfg.Stdout, cfg.Stderr)
	totals.Add(result)
	if cfg.Format != "json" {
		printRunningTotals(cfg.Stdout, cfg.UseColor, &totals)
	}

	// Collect paths to watch.
	wp, err := CollectPaths(cfg.CollectionPath, cfg.EnvName)
	if err != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "watch: failed to collect paths: %v\n", err)
		return 1
	}

	// Show watch status (terminal mode only).
	if cfg.Format != "json" {
		printWatchStatus(cfg.Stdout, cfg.UseColor, wp)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "watch: failed to create watcher: %v\n", err)
		return 1
	}
	defer func() { _ = watcher.Close() }()

	if err := addWatchDirs(watcher, wp); err != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "watch: failed to add directories: %v\n", err)
		return 1
	}

	watchedFiles := buildFileSet(wp)
	watchedDirs := wp.Dirs()
	db := newDebouncer(cfg.Debounce)
	defer db.stop()

	var lastChanged string

	for {
		select {
		case <-ctx.Done():
			return 0

		case event, ok := <-watcher.Events:
			if !ok {
				return 0
			}
			if !isWriteEvent(event) {
				continue
			}
			absName, absErr := filepath.Abs(event.Name)
			if absErr != nil {
				continue
			}
			if _, ok := watchedFiles[absName]; !ok {
				continue
			}
			lastChanged = filepath.Base(absName)
			db.trigger()

		case err, ok := <-watcher.Errors:
			if !ok {
				return 0
			}
			_, _ = fmt.Fprintf(cfg.Stderr, "watch error: %v\n", err)

		case <-db.C:
			if cfg.ClearScreen && cfg.Format != "json" {
				clearScreen(cfg.Stdout)
			}
			if cfg.Format != "json" {
				printSeparator(cfg.Stdout, cfg.UseColor, lastChanged)
			}
			rerunResult := cfg.RunFunc(cfg.Args, cfg.Stdout, cfg.Stderr)
			totals.Add(rerunResult)
			if cfg.Format != "json" {
				printRunningTotals(cfg.Stdout, cfg.UseColor, &totals)
			}

			// Refresh watch list — files may have changed.
			newWP, wpErr := CollectPaths(cfg.CollectionPath, cfg.EnvName)
			if wpErr != nil {
				_, _ = fmt.Fprintf(cfg.Stderr, "watch: failed to refresh paths: %v\n", wpErr)
				continue
			}
			newFiles := buildFileSet(newWP)
			newDirs := newWP.Dirs()

			// Sync watched directories: add new, remove stale.
			syncWatchDirs(watcher, watchedDirs, newDirs, cfg.Stderr)
			watchedDirs = newDirs
			watchedFiles = newFiles
		}
	}
}

// syncWatchDirs adds new directories and removes stale ones from the watcher.
func syncWatchDirs(w *fsnotify.Watcher, oldDirs, newDirs []string, stderr io.Writer) {
	newSet := make(map[string]struct{}, len(newDirs))
	for _, d := range newDirs {
		newSet[d] = struct{}{}
	}

	oldSet := make(map[string]struct{}, len(oldDirs))
	for _, d := range oldDirs {
		oldSet[d] = struct{}{}
	}

	for _, d := range newDirs {
		if _, exists := oldSet[d]; !exists {
			if err := w.Add(d); err != nil {
				_, _ = fmt.Fprintf(stderr, "watch: failed to add directory %s: %v\n", d, err)
			}
		}
	}

	for _, d := range oldDirs {
		if _, exists := newSet[d]; !exists {
			if err := w.Remove(d); err != nil {
				_, _ = fmt.Fprintf(stderr, "watch: failed to remove directory %s: %v\n", d, err)
			}
		}
	}
}

func addWatchDirs(w *fsnotify.Watcher, wp *Paths) error {
	for _, d := range wp.Dirs() {
		if err := w.Add(d); err != nil {
			return fmt.Errorf("watching %s: %w", d, err)
		}
	}
	return nil
}

func buildFileSet(wp *Paths) map[string]struct{} {
	all := wp.All()
	set := make(map[string]struct{}, len(all))
	for _, p := range all {
		set[p] = struct{}{}
	}
	return set
}

func isWriteEvent(e fsnotify.Event) bool {
	return e.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0
}

func printWatchStatus(w io.Writer, useColor bool, wp *Paths) {
	msg := "Watching for changes..."
	if useColor {
		msg = "\033[36m" + msg + "\033[0m"
	}
	_, _ = fmt.Fprintln(w, msg)
	for _, f := range wp.All() {
		_, _ = fmt.Fprintf(w, "  %s\n", filepath.Base(f))
	}
}

func clearScreen(w io.Writer) {
	_, _ = fmt.Fprint(w, "\033[2J\033[H")
}

func printRunningTotals(w io.Writer, useColor bool, rt *RunningTotals) {
	noun := "runs"
	if rt.Runs == 1 {
		noun = "run"
	}
	msg := fmt.Sprintf("Totals (%d %s): %d passed, %d failed, %d skipped",
		rt.Runs, noun, rt.Passed, rt.Failed, rt.Skipped)
	if useColor {
		msg = "\033[90m" + msg + "\033[0m"
	}
	_, _ = fmt.Fprintln(w, msg)
}

func printSeparator(w io.Writer, useColor bool, changed string) {
	ts := time.Now().Format("15:04:05")
	msg := fmt.Sprintf("--- Re-running (changed: %s) at %s ---", changed, ts)
	if useColor {
		// Cyan: \033[36m, reset: \033[0m
		msg = "\033[36m" + msg + "\033[0m"
	}
	_, _ = fmt.Fprintln(w, msg)
}

// debouncer coalesces rapid file change events.
type debouncer struct {
	interval time.Duration
	timer    *time.Timer
	C        chan struct{}
}

func newDebouncer(interval time.Duration) *debouncer {
	return &debouncer{
		interval: interval,
		C:        make(chan struct{}, 1),
	}
}

func (d *debouncer) trigger() {
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.interval, func() {
		select {
		case d.C <- struct{}{}:
		default:
		}
	})
}

func (d *debouncer) stop() {
	if d.timer != nil {
		d.timer.Stop()
	}
}
