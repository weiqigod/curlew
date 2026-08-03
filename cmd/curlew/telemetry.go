// Package main: cmd/curlew/telemetry.go
//
// Implements `curlew telemetry {enable,disable,status,reset-id,export,delete}`.
// Telemetry is local-only: events are appended to an NDJSON file on this
// machine. Nothing is transmitted, and this file MUST NOT gain a network
// client — there is no telemetry backend.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/weiqigod/curlew/internal/appdir"
	"github.com/weiqigod/curlew/internal/runner"
	"github.com/weiqigod/curlew/internal/telemetry"
)

// telemetryCmdOut handles `curlew telemetry <verb>`.
func telemetryCmdOut(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printTelemetryHelpTo(stdout)
		return 0
	}
	configDir, err := appdir.ResolveConfigDir()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	store, err := telemetry.NewStore(configDir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	switch args[0] {
	case "status":
		return telemetryStatus(store, stdout, stderr)
	case "enable":
		return telemetryEnable(store, stdout, stderr)
	case "disable":
		return telemetryDisable(store, stdout, stderr)
	case "reset-id":
		return telemetryResetID(store, stdout, stderr)
	case "export":
		return telemetryExport(store, stdout, stderr)
	case "delete":
		return telemetryDelete(store, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "Unknown telemetry subcommand: %s\n", args[0])
		printTelemetryHelpTo(stderr)
		return 1
	}
}

// telemetryStatus handles `curlew telemetry status`.
// Exit 1 when telemetry has never been enabled (no install_id).
// Exit 0 when enabled or explicitly disabled.
func telemetryStatus(store *telemetry.Store, stdout, stderr io.Writer) int {
	enabled, installID, file, err := store.Status()
	if err != nil {
		if errors.Is(err, telemetry.ErrNotEnabled) {
			// No install_id file: user has never opted in.
			_, _ = fmt.Fprintln(stdout, "telemetry: disabled (no install_id; opt-in required)")
			return 1
		}
		// Genuine I/O error (permission denied, corrupt JSON, etc.).
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if enabled {
		_, _ = fmt.Fprintf(stdout, "telemetry: enabled; install_id=%s; events file %s\n", installID, file)
		return 0
	}
	_, _ = fmt.Fprintf(stdout, "telemetry: disabled; install_id=%s retained\n", installID)
	return 0
}

// telemetryEnable handles `curlew telemetry enable`.
func telemetryEnable(store *telemetry.Store, stdout, stderr io.Writer) int {
	file := store.ResolvedFile() // honours CURLEW_TELEMETRY_FILE
	id, err := store.Enable(file)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "telemetry: enabled; install_id=%s; events will be appended to %s\n", id, file)
	return 0
}

// telemetryDisable handles `curlew telemetry disable`.
func telemetryDisable(store *telemetry.Store, stdout, stderr io.Writer) int {
	if err := store.Disable(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "telemetry: disabled; install_id retained (use `reset-id` to regenerate or `delete` to remove)")
	return 0
}

// telemetryResetID handles `curlew telemetry reset-id`.
func telemetryResetID(store *telemetry.Store, stdout, stderr io.Writer) int {
	_, err := store.ResetID()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "telemetry: install_id regenerated; old id discarded")
	return 0
}

// telemetryExport handles `curlew telemetry export`.
// Prints JSON: { install_id, enabled, file, recent_emissions }.
func telemetryExport(store *telemetry.Store, stdout, stderr io.Writer) int {
	enabled, installID, file, err := store.Status()
	if err != nil {
		// Not enabled — still export what we have (empty).
		installID = ""
		file = store.ResolvedFile()
		enabled = false
	}
	ems, _ := store.RecentEmissions()
	out := map[string]any{
		"install_id":       installID,
		"enabled":          enabled,
		"file":             file,
		"recent_emissions": ems,
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// telemetryDelete handles `curlew telemetry delete`: removes install_id,
// telemetry.json, and the collected events file. Everything telemetry has ever
// recorded lives on this machine, so local removal is the whole deletion.
func telemetryDelete(store *telemetry.Store, stdout, stderr io.Writer) int {
	_, _, file, statusErr := store.Status()
	if statusErr != nil {
		file = store.ResolvedFile()
	}

	priorID, delErr := store.DeleteLocal()
	if delErr != nil {
		_, _ = fmt.Fprintln(stderr, delErr)
		return 1
	}
	if rmErr := os.Remove(file); rmErr != nil && !os.IsNotExist(rmErr) {
		_, _ = fmt.Fprintln(stderr, rmErr)
		return 1
	}

	if priorID == "" {
		_, _ = fmt.Fprintln(stdout, "telemetry: nothing to delete (telemetry was never enabled)")
		return 0
	}
	_, _ = fmt.Fprintf(stdout, "telemetry: install_id %s and collected events deleted\n", priorID)
	return 0
}

// emitTelemetryRunCompleted appends a fire-and-forget run.completed event.
// Called from the deferred end-of-run block in runCmdWithWriters.
// All errors are silently swallowed (v4-9). Only writes when telemetry is
// enabled (telemetry.json present and enabled=true).
func emitTelemetryRunCompleted(sessionID string, started time.Time, summary *runner.Summary, exitCode int) {
	configDir, err := appdir.ResolveConfigDir()
	if err != nil {
		return
	}
	store, err := telemetry.NewStore(configDir)
	if err != nil {
		return
	}
	enabled, _, file, statusErr := store.Status()
	if statusErr != nil || !enabled {
		return
	}
	emitter := telemetry.NewEmitter(store, telemetry.NewFileSink(file))
	payload := map[string]any{
		"session_id":  sessionID,
		"duration_ms": time.Since(started).Milliseconds(),
		"exit_code":   exitCode,
	}
	if summary != nil {
		payload["collection_size"] = summary.Total
		payload["success_count"] = summary.Passed
		payload["failure_count"] = summary.Failed
		payload["skip_count"] = summary.Skipped
	}
	emitter.Emit("run.completed", payload)
}

// printTelemetryHelpTo writes usage for `curlew telemetry` to w.
func printTelemetryHelpTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: curlew telemetry <subcommand>")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Record anonymous usage events to a local file (opt-in).")
	_, _ = fmt.Fprintln(w, "Nothing is transmitted: curlew has no telemetry backend.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Subcommands:")
	_, _ = fmt.Fprintln(w, "  enable         Generate a persistent install_id (UUIDv4) and enable telemetry.")
	_, _ = fmt.Fprintln(w, "                 Stores ~/.config/curlew/install_id (mode 0600) and")
	_, _ = fmt.Fprintln(w, "                 ~/.config/curlew/telemetry.json.")
	_, _ = fmt.Fprintln(w, "  disable        Disable telemetry; install_id is retained for re-enabling.")
	_, _ = fmt.Fprintln(w, "  status         Show current telemetry state. Exit 1 if never enabled.")
	_, _ = fmt.Fprintln(w, "  reset-id       Regenerate the install_id (new UUIDv4; old id unrecoverable).")
	_, _ = fmt.Fprintln(w, "  export         Print install_id and recent emission history as JSON.")
	_, _ = fmt.Fprintln(w, "  delete         Remove install_id, telemetry.json, and the events file.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Events are appended as NDJSON, one object per line.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Env vars:")
	_, _ = fmt.Fprintln(w, "  CURLEW_TELEMETRY_FILE=<path>  Override the events file.")
	_, _ = fmt.Fprintln(w, "                                Default: ~/.config/curlew/telemetry.ndjson")
	_, _ = fmt.Fprintln(w, "  CURLEW_CONFIG_DIR=<path>      Override ~/.config/curlew")
}
