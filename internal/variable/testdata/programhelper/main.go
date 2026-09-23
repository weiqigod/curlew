package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func main() {
	switch os.Args[1] {
	case "hello":
		fmt.Print("hello\n")
	case "argv":
		_ = json.NewEncoder(os.Stdout).Encode(os.Args[2:])
	case "env":
		_ = json.NewEncoder(os.Stdout).Encode(os.Environ())
	case "output":
		fmt.Print(os.Args[2])
	case "exit42":
		fmt.Print("private-stdout")
		fmt.Fprint(os.Stderr, "private-stderr\n")
		os.Exit(42)
	case "invalid-stdout":
		_, _ = os.Stdout.Write([]byte{0xff})
	case "invalid-stderr":
		_, _ = os.Stderr.Write([]byte{0xff})
	case "wait":
		if len(os.Args) > 2 {
			if err := os.WriteFile(os.Args[2], []byte("ready"), 0o600); err != nil {
				os.Exit(2)
			}
		}
		time.Sleep(time.Minute)
	case "touch":
		if err := os.WriteFile(os.Args[2], []byte("launched"), 0o600); err != nil {
			os.Exit(2)
		}
	default:
		os.Exit(2)
	}
}
