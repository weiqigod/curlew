// hooklog-plugin is a test fixture plugin that responds to the curlew hello
// handshake and all three lifecycle hooks, logging each invocation to stderr.
//
// Use this plugin in tests and smoke tests to verify that on_request,
// on_response, and on_result are called by curlew run.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	r := bufio.NewReader(os.Stdin)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params,omitempty"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			return
		}

		var result any
		switch req.Method {
		case "curlew/hello":
			result = map[string]any{
				"name":             "hooklog",
				"version":          "0.1.0",
				"hooks":            []string{"on_request", "on_response", "on_result"},
				"protocol_version": 1,
			}
		case "curlew/on_request":
			var p struct {
				Method string `json:"method"`
				URL    string `json:"url"`
			}
			_ = json.Unmarshal(req.Params, &p)
			_, _ = fmt.Fprintf(os.Stderr, "[plugin:hooklog] on_request %s %s\n", p.Method, p.URL)
			result = map[string]any{} // no mutation — return empty to keep original
		case "curlew/on_response":
			var p struct {
				StatusCode int   `json:"status_code"`
				DurationMs int64 `json:"duration_ms"`
			}
			_ = json.Unmarshal(req.Params, &p)
			_, _ = fmt.Fprintf(os.Stderr, "[plugin:hooklog] on_response %d %dms\n", p.StatusCode, p.DurationMs)
			result = map[string]any{}
		case "curlew/on_result":
			var p struct {
				PassCount int `json:"pass_count"`
				FailCount int `json:"fail_count"`
			}
			_ = json.Unmarshal(req.Params, &p)
			_, _ = fmt.Fprintf(os.Stderr, "[plugin:hooklog] on_result pass_count=%d fail_count=%d\n", p.PassCount, p.FailCount)
			result = map[string]any{}
		default:
			result = map[string]any{}
		}

		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		}
		out, _ := json.Marshal(resp)
		_, _ = os.Stdout.Write(append(out, '\n'))
	}
}
