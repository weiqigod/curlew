package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Execute the README recipes with the shipped fixture and actual CLI/plugin
// processes. A successful CLI run alone would not prove metric submission.
func TestLocalExampleRecipes(t *testing.T) {
	root := readmeRepoRoot(t)
	binary := buildBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var startCommand string
	for _, path := range []string{"examples/output-block/README.md", "examples/plugins/datadog-metrics/README.md"} {
		doc := readmeReadFileOrFatal(t, filepath.Join(root, path))
		blocks := readmeSectionBlocks(doc, "Start the fixture")
		if len(blocks) != 1 || blocks[0].lang != "bash" {
			t.Fatal("expected one fixture startup command")
		}
		command := strings.TrimSpace(blocks[0].body)
		if startCommand != "" && startCommand != command {
			t.Fatal("example fixture startup instructions disagree")
		}
		startCommand = command
	}
	// The documented --port 0 option avoids fixed-port collisions in tests.
	server := exec.CommandContext(ctx, "bash", "-euo", "pipefail", "-c", "exec "+startCommand+" --port 0")
	server.Dir = root
	pipe, err := server.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Process.Kill(); _ = server.Wait() })
	scanner := bufio.NewScanner(pipe)
	if !scanner.Scan() {
		t.Fatal("example fixture did not print its address")
	}
	base := scanner.Text()
	if !strings.HasPrefix(base, "http://127.0.0.1:") {
		t.Fatalf("nonlocal fixture: %q", base)
	}
	dir := t.TempDir()
	if err := os.CopyFS(filepath.Join(dir, "examples"), os.DirFS(filepath.Join(root, "examples"))); err != nil {
		t.Fatal(err)
	}
	for _, recipe := range []struct{ doc, heading string }{
		{"examples/output-block/README.md", "Run it"},
		{"examples/output-block/README.md", "Expected failure"},
		{"examples/plugins/datadog-metrics/README.md", "Run locally"},
	} {
		t.Run(recipe.heading, func(t *testing.T) {
			doc := readmeReadFileOrFatal(t, filepath.Join(root, recipe.doc))
			blocks := readmeSectionBlocks(doc, recipe.heading)
			if len(blocks) != 1 || blocks[0].lang != "bash" {
				t.Fatal("expected one executable bash recipe")
			}
			command := exec.CommandContext(ctx, "bash", "-euo", "pipefail", "-c", blocks[0].body)
			command.Dir = dir
			command.Env = append(os.Environ(), "PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"), "CURLEW_CONFIG_DIR="+t.TempDir(), "EXAMPLE_URL="+base, "CURLEW_PLUGINS=", "NO_COLOR=1")
			if out, err := command.CombinedOutput(); err != nil {
				t.Fatalf("recipe failed: %v\n%s", err, out)
			}
		})
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(base + "/observations")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var observed struct {
		Gets        int `json:"gets"`
		Submissions []struct {
			Series []struct {
				Metric string   `json:"metric"`
				Tags   []string `json:"tags"`
			} `json:"series"`
		} `json:"submissions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&observed); err != nil {
		t.Fatal(err)
	}
	if observed.Gets != 2 || len(observed.Submissions) != 1 {
		t.Fatalf("expected two local requests, one metric, and no request from invalid example: %+v", observed)
	}
	metrics := observed.Submissions[0].Series
	if len(metrics) != 1 || metrics[0].Metric != "curlew.request.duration" || len(metrics[0].Tags) != 1 || metrics[0].Tags[0] != "status:200" {
		t.Fatalf("wrong metric: %+v", metrics)
	}
}
