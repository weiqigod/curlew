package main

import (
	"fmt"
	"io"
)

// printCommandHelp handles only known commands; unknown names retain usage errors.
func printCommandHelp(command string, w io.Writer) bool {
	switch command {
	case "run", "watch":
		if command == "run" {
			_, _ = fmt.Fprintln(w, runUsageSynopsis)
		} else {
			_, _ = fmt.Fprintln(w, "Usage: curlew watch <file> [run options] [--clear]")
		}
		printRunOptionsTo(w)
		if command == "watch" {
			_, _ = fmt.Fprintln(w, "  --clear  Clear terminal between runs; Ctrl+C stops watching.")
		}
	case "exec":
		_, _ = fmt.Fprintln(w, "Usage: curlew exec <url> [options] | curlew exec --stdin [options]")
		printExecOptionsTo(w)
	case "validate":
		_, _ = fmt.Fprintln(w, "Usage: curlew validate <file|glob> [--format json] [--color auto|always|never] [--no-color]")
		_, _ = fmt.Fprintln(w, "Validate collections without sending requests. JSON diagnostics include file and location.")
	case "info":
		_, _ = fmt.Fprintln(w, "Usage: curlew info [--format json]")
		_, _ = fmt.Fprintln(w, "Discover the project root, collections, environments and version.")
	case "schema":
		_, _ = fmt.Fprintln(w, "Usage: curlew schema [--project] [--format json]")
		_, _ = fmt.Fprintln(w, "Print the collection JSON Schema, or the curlew.yaml schema with --project.")
	case "import":
		_, _ = fmt.Fprintln(w, usageSynopsis("import"))
		_, _ = fmt.Fprintln(w, "Usage: curlew import openapi <spec-path> [--output <file>]")
		_, _ = fmt.Fprintln(w, "Import a local OpenAPI 3.x file into a YAML collection.")
	case "init":
		printInitHelpTo(w)
	case "vault":
		printVaultHelpTo(w)
	case "plugins":
		printPluginsHelpTo(w)
	case "pr-check":
		printPrCheckHelpTo(w)
	case "perf":
		printPerfHelpTo(w)
	case "telemetry":
		printTelemetryHelpTo(w)
	case "ui":
		printUIHelpTo(w)
	default:
		return false
	}
	return true
}
