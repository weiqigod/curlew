// Package main: cmd/apitest/telemetry.go
//
// Implements `apitest telemetry {enable,disable,status,reset-id,export,delete-request}`.
// Per v4-11, this uses a dedicated HTTP client from internal/telemetry and
// MUST NOT import internal/backend.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/peterlindqvist/apitest/internal/appdir"
	"github.com/peterlindqvist/apitest/internal/runner"
	"github.com/peterlindqvist/apitest/internal/telemetry"
)

// telemetryCmdOut handles `apitest telemetry <verb>`.
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
	case "delete-request":
		return telemetryDeleteRequest(store, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "Unknown telemetry subcommand: %s\n", args[0])
		printTelemetryHelpTo(stderr)
		return 1
	}
}

// telemetryStatus handles `apitest telemetry status`.
// Exit 1 when telemetry has never been enabled (no install_id).
// Exit 0 when enabled or explicitly disabled.
func telemetryStatus(store *telemetry.Store, stdout, stderr io.Writer) int {
	enabled, installID, _, err := store.Status()
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
		_, _ = fmt.Fprintf(stdout, "telemetry: enabled; install_id=%s\n", installID)
		return 0
	}
	_, _ = fmt.Fprintf(stdout, "telemetry: disabled; install_id=%s retained\n", installID)
	return 0
}

// telemetryEnable handles `apitest telemetry enable [--endpoint=<url>]`.
func telemetryEnable(store *telemetry.Store, stdout, stderr io.Writer) int {
	endpoint := store.ResolvedEndpoint() // honours APITEST_TELEMETRY_ENDPOINT
	id, err := store.Enable(endpoint)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "telemetry: enabled; install_id=%s; events will post to %s\n", id, endpoint)
	return 0
}

// telemetryDisable handles `apitest telemetry disable`.
func telemetryDisable(store *telemetry.Store, stdout, stderr io.Writer) int {
	if err := store.Disable(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "telemetry: disabled; install_id retained (use `reset-id` to regenerate or `delete-request` to remove)")
	return 0
}

// telemetryResetID handles `apitest telemetry reset-id`.
func telemetryResetID(store *telemetry.Store, stdout, stderr io.Writer) int {
	_, err := store.ResetID()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "telemetry: install_id regenerated; old id discarded")
	return 0
}

// telemetryExport handles `apitest telemetry export`.
// Prints JSON: { install_id, enabled, endpoint, recent_emissions }.
func telemetryExport(store *telemetry.Store, stdout, stderr io.Writer) int {
	enabled, installID, endpoint, err := store.Status()
	if err != nil {
		// Not enabled — still export what we have (empty).
		installID = ""
		endpoint = store.ResolvedEndpoint()
		enabled = false
	}
	ems, _ := store.RecentEmissions()
	out := map[string]any{
		"install_id":       installID,
		"enabled":          enabled,
		"endpoint":         endpoint,
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

// telemetryDeleteRequest handles `apitest telemetry delete-request`.
// Issues a synchronous POST with event_type=telemetry.delete_request (5s timeout),
// then removes local files regardless of POST outcome.
func telemetryDeleteRequest(store *telemetry.Store, stdout, stderr io.Writer) int {
	_, priorID, endpoint, statusErr := store.Status()

	// Only POST if there is a known install_id; no install_id means nothing to
	// delete on the backend (Status returns ErrNotEnabled with an empty id).
	var postErr error
	if priorID != "" && statusErr == nil {
		client := telemetry.NewClient(telemetry.ClientOptions{
			Endpoint: endpoint,
			Timeout:  5 * time.Second,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		postErr = client.Emit(ctx, priorID, "telemetry.delete_request", map[string]any{
			"install_id": priorID,
		})
	}

	// Remove local files regardless of POST outcome.
	actualPriorID, delErr := store.DeleteLocal()
	if delErr != nil {
		_, _ = fmt.Fprintln(stderr, delErr)
		return 1
	}
	if actualPriorID != "" {
		priorID = actualPriorID
	}

	if actualPriorID == "" && priorID == "" {
		// Telemetry was never enabled — nothing to delete locally or remotely.
		_, _ = fmt.Fprintln(stdout, "telemetry: nothing to delete (telemetry was never enabled)")
		return 0
	}
	if postErr != nil {
		_, _ = fmt.Fprintf(stdout,
			"telemetry: local files removed; backend delete POST failed (offline). Re-run when online to send the delete request.\n")
	} else {
		_, _ = fmt.Fprintf(stdout,
			"telemetry: install_id %s deleted locally; delete-request event posted\n", priorID)
	}
	return 0
}

// emitTelemetryRunCompleted fires a fire-and-forget run.completed event.
// Called from the deferred end-of-run block in runCmdWithWriters.
// All errors are silently swallowed (v4-9). Only contacts the network when
// telemetry is enabled (telemetry.json present and enabled=true).
func emitTelemetryRunCompleted(sessionID string, started time.Time, summary *runner.Summary, exitCode int) {
	configDir, err := appdir.ResolveConfigDir()
	if err != nil {
		return
	}
	store, err := telemetry.NewStore(configDir)
	if err != nil {
		return
	}
	enabled, _, endpoint, statusErr := store.Status()
	if statusErr != nil || !enabled {
		return
	}
	client := telemetry.NewClient(telemetry.ClientOptions{
		Endpoint: endpoint,
		Timeout:  2 * time.Second,
	})
	emitter := telemetry.NewEmitter(store, client)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
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
	emitter.Emit(ctx, "run.completed", payload)
}

// printTelemetryHelpTo writes usage for `apitest telemetry` to w.
func printTelemetryHelpTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: apitest telemetry <subcommand>")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Manage anonymous usage telemetry (opt-in).")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Subcommands:")
	_, _ = fmt.Fprintln(w, "  enable         Generate a persistent install_id (UUIDv4) and enable telemetry.")
	_, _ = fmt.Fprintln(w, "                 Stores ~/.config/apitesttool/install_id (mode 0600) and")
	_, _ = fmt.Fprintln(w, "                 ~/.config/apitesttool/telemetry.json.")
	_, _ = fmt.Fprintln(w, "  disable        Disable telemetry; install_id is retained for re-enabling.")
	_, _ = fmt.Fprintln(w, "  status         Show current telemetry state. Exit 1 if never enabled.")
	_, _ = fmt.Fprintln(w, "  reset-id       Regenerate the install_id (new UUIDv4; old id unrecoverable).")
	_, _ = fmt.Fprintln(w, "  export         Print install_id and recent emission history as JSON.")
	_, _ = fmt.Fprintln(w, "                 (CLI-side GDPR export; backend export covered by M18-004.)")
	_, _ = fmt.Fprintln(w, "  delete-request Remove install_id and telemetry.json locally, and POST a")
	_, _ = fmt.Fprintln(w, "                 telemetry.delete_request event so the backend can purge rows.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Env vars:")
	_, _ = fmt.Fprintln(w, "  APITEST_TELEMETRY_ENDPOINT=<url>  Override the default ingest endpoint.")
	_, _ = fmt.Fprintln(w, "                                    Default: https://api.apitest.org/telemetry/events")
	_, _ = fmt.Fprintln(w, "  APITEST_CONFIG_DIR=<path>         Override ~/.config/apitesttool")
}
