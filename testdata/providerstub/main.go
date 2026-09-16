package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func main() {
	if capture := os.Getenv("CURLEW_STUB_CAPTURE"); capture != "" {
		file, err := os.OpenFile(capture, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(90)
		}
		err = json.NewEncoder(file).Encode(struct {
			Args []string          `json:"args"`
			Env  map[string]string `json:"env"`
		}{os.Args[1:], map[string]string{
			"VAULT_ADDR":            os.Getenv("VAULT_ADDR"),
			"VAULT_TOKEN":           os.Getenv("VAULT_TOKEN"),
			"CURLEW_STUB_INHERITED": os.Getenv("CURLEW_STUB_INHERITED"),
		}})
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			os.Exit(91)
		}
	}
	if os.Getenv("CURLEW_STUB_WAIT") != "" {
		time.Sleep(10 * time.Second)
	}
	if diagnostic := os.Getenv("CURLEW_STUB_ERROR"); diagnostic != "" {
		fmt.Fprintln(os.Stderr, diagnostic, os.Args[1:], os.Getenv("VAULT_TOKEN"))
		fmt.Fprintln(os.Stdout, os.Getenv("CURLEW_STUB_SECRET"))
		os.Exit(42)
	}
	if output := os.Getenv("CURLEW_STUB_OUTPUT"); output != "" {
		fmt.Print(output)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "write" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"auth": map[string]string{"client_token": os.Getenv("CURLEW_STUB_TOKEN")},
		})
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "kv" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"data": map[string]any{"data": map[string]string{"secret": os.Getenv("CURLEW_STUB_SECRET")}},
		})
		return
	}
	fmt.Print(os.Getenv("CURLEW_STUB_SECRET") + "\r\n")
}
