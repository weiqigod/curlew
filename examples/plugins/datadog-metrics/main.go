// Package main is the datadog-metrics example plugin for ApiTool.
//
// It listens on stdin for JSON-RPC 2.0 requests from apitest, responds to the
// apitest/hello handshake declaring the on_response and on_result hooks, and
// submits an apitest.request.duration gauge metric to Datadog on every
// apitest/on_response call.
//
// Configuration is done via environment variables:
//
//	DATADOG_API_KEY  Datadog API key. When absent, the plugin starts in
//	                 disabled mode: it still responds to the handshake but
//	                 never submits metrics.
//	DD_API_URL       Override the full Datadog API base URL (no trailing
//	                 slash). Useful for testing against a local mock server.
//	DD_SITE          Datadog site (e.g. "datadoghq.eu"). Defaults to
//	                 "datadoghq.com". Ignored when DD_API_URL is set.
//
// Invoke with --help or -h to print plugin metadata and exit 0.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	pluginName    = "datadog-metrics"
	pluginVersion = "0.1.0"
)

// config holds the runtime configuration derived from environment variables.
type config struct {
	enabled bool
	apiKey  string
	apiURL  string // never ends with a trailing slash
}

// loadConfig derives configuration from the provided environment (os.Environ()
// shape). DATADOG_API_KEY enables submission; absent/blank disables. DD_API_URL
// overrides the endpoint (full URL, no trailing slash); otherwise DD_SITE
// (e.g. "datadoghq.com") builds https://api.<site>; default "datadoghq.com".
func loadConfig(env []string) config {
	m := make(map[string]string, len(env))
	for _, e := range env {
		if i := strings.IndexByte(e, '='); i > 0 {
			m[e[:i]] = e[i+1:]
		}
	}
	cfg := config{apiKey: strings.TrimSpace(m["DATADOG_API_KEY"])}
	cfg.enabled = cfg.apiKey != ""
	switch {
	case m["DD_API_URL"] != "":
		cfg.apiURL = strings.TrimRight(m["DD_API_URL"], "/")
	case m["DD_SITE"] != "":
		cfg.apiURL = "https://api." + m["DD_SITE"]
	default:
		cfg.apiURL = "https://api.datadoghq.com"
	}
	return cfg
}

// printMetadata writes a human-readable summary of this plugin's identity and
// supported hooks to w. Used when the binary is invoked with --help or -h.
func printMetadata(w io.Writer) {
	_, _ = fmt.Fprintf(w, "Plugin:   %s\n", pluginName)
	_, _ = fmt.Fprintf(w, "Version:  %s\n", pluginVersion)
	_, _ = fmt.Fprintf(w, "Hooks:    on_response, on_result\n")
	_, _ = fmt.Fprintf(w, "Protocol: 1\n")
	_, _ = fmt.Fprintf(w, "\nSubmits apitest.request.duration (gauge) to Datadog v2 /api/v2/series\n")
	_, _ = fmt.Fprintf(w, "on every on_response hook. Disabled when DATADOG_API_KEY is unset.\n")
}

// handle dispatches a single JSON-RPC method call and returns the result value.
// It is exposed for testing so callers can drive the plugin logic without
// spawning a process or wiring up a full stdin/stdout loop.
func handle(ctx context.Context, cfg config, client *http.Client, stderr io.Writer, method string, params json.RawMessage) (any, error) {
	switch method {
	case "apitest/hello":
		return map[string]any{
			"name":             pluginName,
			"version":          pluginVersion,
			"hooks":            []string{"on_response", "on_result"},
			"protocol_version": 1,
		}, nil
	case "apitest/on_response":
		if !cfg.enabled {
			return map[string]any{}, nil
		}
		var p struct {
			StatusCode int   `json:"status_code"`
			DurationMs int64 `json:"duration_ms"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			_, _ = fmt.Fprintf(stderr, "[plugin:%s] on_response: bad params: %v\n", pluginName, err)
			return map[string]any{}, nil
		}
		metric := ddMetric{
			Metric: "apitest.request.duration",
			Type:   3, // gauge
			Points: []ddPoint{{Timestamp: time.Now().Unix(), Value: float64(p.DurationMs)}},
			Tags:   []string{fmt.Sprintf("status:%d", p.StatusCode)},
		}
		if err := submitMetric(ctx, client, cfg.apiURL, cfg.apiKey, metric); err != nil {
			_, _ = fmt.Fprintf(stderr, "[plugin:%s] submit failed: %v\n", pluginName, err)
			return map[string]any{}, nil
		}
		_, _ = fmt.Fprintf(stderr, "[plugin:%s] submitted 1 metric\n", pluginName)
		return map[string]any{}, nil
	case "apitest/on_result":
		return map[string]any{}, nil
	default:
		return map[string]any{}, nil
	}
}

// run is the main JSON-RPC dispatch loop. It reads newline-delimited JSON-RPC
// 2.0 requests from stdin, calls handle(), and writes responses to stdout.
// It returns nil on clean EOF; other read/write errors are propagated.
func run(ctx context.Context, cfg config, stdin io.Reader, stdout, stderr io.Writer) error {
	if !cfg.enabled {
		_, _ = fmt.Fprintf(stderr, "[plugin:%s] DATADOG_API_KEY not set, disabled\n", pluginName)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	r := bufio.NewReader(stdin)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			continue // malformed — ignore, keep reading
		}
		result, hErr := handle(ctx, cfg, client, stderr, req.Method, req.Params)
		resp := response{JSONRPC: "2.0", ID: req.ID}
		if hErr != nil {
			resp.Error = &rpcError{Code: -32000, Message: hErr.Error()}
		} else {
			resp.Result = result
		}
		out, mErr := json.Marshal(resp)
		if mErr != nil {
			_, _ = fmt.Fprintf(stderr, "[plugin:%s] marshal response: %v\n", pluginName, mErr)
			continue
		}
		if _, err := stdout.Write(append(out, '\n')); err != nil {
			return err
		}
	}
}

func main() {
	for _, a := range os.Args[1:] {
		if a == "--help" || a == "-h" {
			printMetadata(os.Stdout)
			return
		}
	}
	cfg := loadConfig(os.Environ())
	if err := run(context.Background(), cfg, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "[plugin:%s] fatal: %v\n", pluginName, err)
		os.Exit(1)
	}
}
