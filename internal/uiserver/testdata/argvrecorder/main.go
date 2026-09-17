package main

import (
	"encoding/json"
	"os"
)

func main() {
	body, err := json.Marshal(os.Args[1:])
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(os.Getenv("CURLEW_EDITOR_ARGV_FILE"), body, 0o600); err != nil {
		os.Exit(3)
	}
}
