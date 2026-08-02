package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/peterlindqvist/apitest/internal/plugin"
	"github.com/peterlindqvist/apitest/internal/plugin/hooks"
	"github.com/peterlindqvist/apitest/internal/runner"
)

// pluginsHostFactory is a test seam: replace in tests to inject an in-process
// spawner-backed host.
var pluginsHostFactory = func() *plugin.Host { return plugin.NewHost(os.Stderr) }

// pluginsCtxFactory is a test seam: replace in tests to inject a context with
// a shorter deadline (e.g. pre-cancelled for timeout scenario tests).
var pluginsCtxFactory = func() context.Context { return context.Background() }

// hookTimeoutOverride is a test seam: when non-zero, overrides the hook
// timeout on each Dispatcher built by buildHookDispatcher. Tests set this to
// a short duration to avoid waiting the full 10-second hook timeout.
var hookTimeoutOverride time.Duration

// buildHookDispatcher loads plugins from APITEST_PLUGINS, keeps their channels
// alive, and returns a Dispatcher ready for hook invocations plus a cleanup
// func that terminates all plugin processes. Returns (nil, noop, nil) when
// APITEST_PLUGINS is empty (no error — just no-op).
func buildHookDispatcher(ctx context.Context, stderr io.Writer) (runner.HooksDispatcher, func(), error) {
	env := os.Getenv("APITEST_PLUGINS")
	if env == "" {
		return nil, func() {}, nil
	}

	host := pluginsHostFactory()
	loaded, channels, loadErrs, err := host.LoadForRun(ctx, env)
	if err != nil {
		return nil, func() {}, fmt.Errorf("plugin: %w", err)
	}

	// Print load warnings / errors.
	for _, le := range loadErrs {
		if le.Fatal {
			_, _ = fmt.Fprintf(stderr, "error: %s\n", le.Message)
		} else {
			_, _ = fmt.Fprintf(stderr, "warning: %s\n", le.Message)
		}
	}

	// Build registrations in the same order as loaded.
	regs := make([]hooks.Registration, len(loaded))
	for i, p := range loaded {
		regs[i] = hooks.Registration{Plugin: p, Channel: channels[i]}
	}

	var opts []hooks.DispatcherOption
	if hookTimeoutOverride > 0 {
		opts = append(opts, hooks.WithHookTimeout(hookTimeoutOverride))
	}
	d := hooks.NewDispatcher(stderr, regs, opts...)
	// host.Close() terminates all plugin processes registered in h.running
	// (populated by LoadForRun). Calling d.Close() here would be redundant
	// because it closes the same channels via the same Channel.Close/sync.Once.
	// Rely on host.Close() as the single cleanup owner.
	cleanup := func() {
		_ = host.Close()
	}
	return d, cleanup, nil
}

// pluginsCmdOut dispatches `apitest plugins <subcommand>`.
func pluginsCmdOut(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printPluginsHelpTo(stdout)
		return 0
	}
	switch args[0] {
	case "list":
		return pluginsListCmdOut(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "Unknown plugins subcommand: %s\n", args[0])
		_, _ = fmt.Fprintln(stderr, usageSynopsis("plugins"))
		return 1
	}
}

// pluginsListCmdOut loads all plugins from APITEST_PLUGINS and prints a table.
// Exit codes: 0 ok (warnings still produce 0); 2 any fatal load error.
func pluginsListCmdOut(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		switch a {
		case "--help", "-h":
			printPluginsHelpTo(stdout)
			return 0
		default:
			_, _ = fmt.Fprintf(stderr, "Unknown flag: %s\n", a)
			return 2
		}
	}
	env := os.Getenv("APITEST_PLUGINS")

	host := pluginsHostFactory()
	defer func() { _ = host.Close() }()

	loaded, loadErrs, err := host.Load(pluginsCtxFactory(), env)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	// Emit stderr messages in order and track the worst exit code.
	exit := 0
	for _, le := range loadErrs {
		if le.Fatal {
			_, _ = fmt.Fprintln(stderr, "error: "+le.Message)
			if exit < 2 {
				exit = 2
			}
		} else {
			_, _ = fmt.Fprintln(stderr, "warning: "+le.Message)
		}
	}

	renderPluginsTable(stdout, loaded)
	return exit
}

// renderPluginsTable prints the fixed-width table for the plugins list command.
// Columns: NAME VERSION HOOKS. Header is always printed even with zero rows.
func renderPluginsTable(w io.Writer, plugins []plugin.Plugin) {
	const minGap = "  "
	nameW := len("NAME")
	verW := len("VERSION")
	for _, p := range plugins {
		if len(p.Name) > nameW {
			nameW = len(p.Name)
		}
		if len(p.Version) > verW {
			verW = len(p.Version)
		}
	}
	_, _ = fmt.Fprintf(w, "%-*s%s%-*s%s%s\n",
		nameW, "NAME", minGap, verW, "VERSION", minGap, "HOOKS")
	for _, p := range plugins {
		hooks := strings.Join(p.Hooks, ",")
		_, _ = fmt.Fprintf(w, "%-*s%s%-*s%s%s\n",
			nameW, p.Name, minGap, verW, p.Version, minGap, hooks)
	}
}

// printPluginsHelpTo writes the plugins subcommand usage to w.
func printPluginsHelpTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: apitest plugins <subcommand>")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Subcommands:")
	_, _ = fmt.Fprintln(w, "  list    Discover plugins, handshake, and print registered capabilities")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Environment:")
	_, _ = fmt.Fprintln(w, "  APITEST_PLUGINS   Colon-separated (semicolon-separated on Windows) list of plugin")
	_, _ = fmt.Fprintln(w, "                    executables or directories. Directory entries load every")
	_, _ = fmt.Fprintln(w, "                    executable file inside (non-recursive, alphabetical).")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Exit codes:")
	_, _ = fmt.Fprintln(w, "  0    ok (warnings printed to stderr are non-fatal)")
	_, _ = fmt.Fprintln(w, "  2    fatal load error (missing file, not executable, duplicate name)")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "See docs/plugins.md for the handshake protocol.")
}
