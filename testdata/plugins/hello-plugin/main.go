// Package main is the sample plugin fixture for M5-017.
// It reads one JSON-RPC 2.0 request on stdin and writes one JSON-RPC 2.0
// response on stdout, declaring the on_request and on_response hooks.
// This serves as the canonical reference implementation for the plugin contract.
// See docs/plugins.md for the wire format documentation.
package main

import (
	"bufio"
	"encoding/json"
	"os"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
}

type hello struct {
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Hooks           []string `json:"hooks"`
	ProtocolVersion int      `json:"protocol_version"`
}

func main() {
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadBytes('\n')
	if err != nil {
		os.Exit(1)
	}
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		os.Exit(1)
	}
	resp := response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: hello{
			Name:            "hello-plugin",
			Version:         "0.1.0",
			Hooks:           []string{"on_request", "on_response"},
			ProtocolVersion: 1,
		},
	}
	out, _ := json.Marshal(resp)
	_, _ = os.Stdout.Write(append(out, '\n'))
}
