package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
)

type request struct {
	ID int `json:"id"`
}

type response struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Result  map[string]any `json:"result"`
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "probe" {
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "child" {
		signal.Ignore(os.Interrupt)
		select {}
	}

	child := exec.Command(os.Args[0], "child")
	if err := child.Start(); err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(os.Getenv("CURLEW_PLUGIN_CHILD_PID_FILE"), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
		_ = child.Process.Kill()
		os.Exit(3)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req request
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			continue
		}
		body, _ := json.Marshal(response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"name":             "lifecycle-plugin",
				"version":          "1.0.0",
				"hooks":            []string{},
				"protocol_version": 1,
			},
		})
		_, _ = os.Stdout.Write(append(body, '\n'))
	}
	_ = os.WriteFile(os.Getenv("CURLEW_PLUGIN_EOF_FILE"), []byte("closed\n"), 0o600)
	if os.Getenv("CURLEW_PLUGIN_STALL_AFTER_EOF") == "1" {
		select {}
	}
}
