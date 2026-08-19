package main

// README.md's "## Quickstart" section is the first thing anyone runs, and
// until this file existed nothing had ever executed it. This file extracts
// the section's one command block and one expected-output block, normalises
// the only thing a real run cannot hold constant (elapsed milliseconds), and
// -- in TestReadme_quickstart_actually_works below -- actually runs the
// commands against a local mudflat server and asserts the output matches.
//
// Reuses readmeSectionBlocks (readme_install_test.go) rather than inventing
// new fence-parsing machinery: a "### " subheading does not end the
// section, only another "## " does, and that behaviour is already proven
// there.
//
// Untagged, unlike the install exec test: it costs about as much as
// buildBinary plus starting an in-process mudflat server, with no network,
// no `gh`, and no credentials, so there is no reason to keep it out of the
// three routine `go test` passes in scripts/ci-local.sh.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/weiqigod/curlew/testapi/mudflat"
)

// quickstart is the README's executable quickstart: the commands and the
// bytes they print.
type quickstart struct {
	commands []string // non-blank, non-comment lines of the bash block
	script   string   // the block verbatim, as it is written to disk and run
	want     string   // the expected-output block, verbatim
	line     int      // 1-based line of the bash block's opening fence
}

var (
	errNoQuickstartSection  = errors.New("no quickstart section")
	errNoQuickstartCommands = errors.New("no quickstart commands")
	errNoQuickstartOutput   = errors.New("no quickstart output")
)

// quickstartExtract pulls the command block and the expected-output block
// out of doc's "## Quickstart" section.
//
// Every empty result is an error rather than an empty struct: a quickstart
// that extracts to nothing must fail this test, not pass it having checked
// nothing (M22-001's failure shape, one level down).
func quickstartExtract(doc string) (quickstart, error) {
	blocks := readmeSectionBlocks(doc, "Quickstart")
	if len(blocks) == 0 {
		return quickstart{}, errNoQuickstartSection
	}

	var cmdBlocks, outputBlocks []readmeBlock
	for _, b := range blocks {
		if b.lang == "bash" || b.lang == "sh" {
			cmdBlocks = append(cmdBlocks, b)
			continue
		}
		outputBlocks = append(outputBlocks, b)
	}

	if len(cmdBlocks) == 0 {
		return quickstart{}, errNoQuickstartCommands
	}
	if len(cmdBlocks) > 1 {
		return quickstart{}, fmt.Errorf("quickstart: %d bash/sh blocks under \"## Quickstart\" -- exactly one is required, unambiguously", len(cmdBlocks))
	}
	if len(outputBlocks) == 0 {
		return quickstart{}, errNoQuickstartOutput
	}
	if len(outputBlocks) > 1 {
		return quickstart{}, fmt.Errorf("quickstart: %d non-command blocks under \"## Quickstart\" -- exactly one expected-output block is required, unambiguously", len(outputBlocks))
	}

	cmdBlock := cmdBlocks[0]
	outputBlock := outputBlocks[0]

	var commands []string
	for _, line := range strings.Split(cmdBlock.body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		commands = append(commands, trimmed)
	}
	if len(commands) == 0 {
		return quickstart{}, errNoQuickstartCommands
	}

	if strings.TrimSpace(outputBlock.body) == "" {
		return quickstart{}, errNoQuickstartOutput
	}

	return quickstart{
		commands: commands,
		script:   cmdBlock.body,
		want:     outputBlock.body,
		line:     cmdBlock.line,
	}, nil
}

// quickstartDurationRE matches an elapsed-millisecond figure: a per-request
// "  200  12ms" or a summary "(12ms)". A status code like "200" never
// matches -- it is never immediately followed by "ms".
var quickstartDurationRE = regexp.MustCompile(`\d+ms`)

// quickstartDurationPlaceholder replaces every matched duration.
const quickstartDurationPlaceholder = "<ms>"

// quickstartNormalizeDurations replaces every elapsed-millisecond figure
// with a placeholder, so the assertion covers every byte a run controls and
// no byte it does not. Measured: a real run printed "0ms" per request and
// "(1ms)" in the summary on one run, and different values on the next, so
// both spellings must normalise regardless of the digit(s) they carry.
func quickstartNormalizeDurations(s string) string {
	return quickstartDurationRE.ReplaceAllString(s, quickstartDurationPlaceholder)
}

func TestReadme_quickstart_extraction(t *testing.T) {
	tests := []struct {
		name         string
		doc          string
		wantCommands []string
		wantScript   string
		wantWant     string
		err          error
	}{
		{
			name: "happy path: one bash block and one output block",
			doc: "# Curlew\n\n## Quickstart\n\n```bash\n" +
				"curlew init\n" +
				"curlew run collections/sample.yaml\n" +
				"```\n\nThat prints:\n\n```\nProject initialized successfully!\n```\n",
			wantCommands: []string{"curlew init", "curlew run collections/sample.yaml"},
			wantScript:   "curlew init\ncurlew run collections/sample.yaml",
			wantWant:     "Project initialized successfully!",
		},
		{
			name:         "MUTATION no Quickstart heading yields errNoQuickstartSection",
			doc:          "# Curlew\n\n## Something Else\n\n```bash\ncurlew init\n```\n",
			wantCommands: nil,
			err:          errNoQuickstartSection,
		},
		{
			name:         "MUTATION heading present, prose only, yields errNoQuickstartSection",
			doc:          "# Curlew\n\n## Quickstart\n\nJust words, no fenced blocks at all.\n",
			wantCommands: nil,
			err:          errNoQuickstartSection,
		},
		{
			name:         "MUTATION a bash block with no output block yields errNoQuickstartOutput",
			doc:          "# Curlew\n\n## Quickstart\n\n```bash\ncurlew init\n```\n",
			wantCommands: nil,
			err:          errNoQuickstartOutput,
		},
		{
			name:         "MUTATION an output block with no bash block yields errNoQuickstartCommands",
			doc:          "# Curlew\n\n## Quickstart\n\n```\nProject initialized successfully!\n```\n",
			wantCommands: nil,
			err:          errNoQuickstartCommands,
		},
		{
			name:         "MUTATION a bash block whose lines are all comments yields errNoQuickstartCommands",
			doc:          "# Curlew\n\n## Quickstart\n\n```bash\n# just a comment\n```\n\n```\noutput\n```\n",
			wantCommands: nil,
			err:          errNoQuickstartCommands,
		},
		{
			name:         "MUTATION a bash block whose lines are all blank yields errNoQuickstartCommands",
			doc:          "# Curlew\n\n## Quickstart\n\n```bash\n\n\n```\n\n```\noutput\n```\n",
			wantCommands: nil,
			err:          errNoQuickstartCommands,
		},
		{
			name:         "MUTATION a blank output block yields errNoQuickstartOutput",
			doc:          "# Curlew\n\n## Quickstart\n\n```bash\ncurlew init\n```\n\n```\n\n\n```\n",
			wantCommands: nil,
			err:          errNoQuickstartOutput,
		},
		{
			name:         "MUTATION two bash blocks are ambiguous and error",
			doc:          "# Curlew\n\n## Quickstart\n\n```bash\ncurlew init\n```\n\n```bash\ncurlew run x.yaml\n```\n\n```\noutput\n```\n",
			wantCommands: nil,
			// Not one of the three named sentinels: ambiguity is a distinct
			// failure shape from "found nothing".
		},
		{
			name: "a trailing comment on a command line is still a command",
			doc: "# Curlew\n\n## Quickstart\n\n```bash\n" +
				"curlew run x.yaml   # note\n" +
				"```\n\n```\noutput\n```\n",
			wantCommands: []string{"curlew run x.yaml   # note"},
			wantScript:   "curlew run x.yaml   # note",
			wantWant:     "output",
		},
		{
			name: "a ### subheading does not end the section",
			doc: "# Curlew\n\n## Quickstart\n\n### Step one\n\n```bash\n" +
				"curlew init\n" +
				"```\n\n```\noutput\n```\n",
			wantCommands: []string{"curlew init"},
			wantScript:   "curlew init",
			wantWant:     "output",
		},
		{
			name: "the section stops at the next ## heading",
			doc: "# Curlew\n\n## Quickstart\n\n```bash\n" +
				"curlew init\n" +
				"```\n\n```\noutput\n```\n\n## Documentation\n\n```bash\nnot part of quickstart\n```\n",
			wantCommands: []string{"curlew init"},
			wantScript:   "curlew init",
			wantWant:     "output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := quickstartExtract(tt.doc)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("quickstartExtract() err = %v, want %v", err, tt.err)
				}
				return
			}
			if tt.name == "MUTATION two bash blocks are ambiguous and error" {
				if err == nil {
					t.Fatal("quickstartExtract() err = nil, want a non-nil ambiguity error")
				}
				return
			}
			if err != nil {
				t.Fatalf("quickstartExtract() unexpected error: %v", err)
			}
			if !reflectEqualStrings(got.commands, tt.wantCommands) {
				t.Errorf("commands = %#v, want %#v", got.commands, tt.wantCommands)
			}
			if got.script != tt.wantScript {
				t.Errorf("script = %q, want %q", got.script, tt.wantScript)
			}
			if got.want != tt.wantWant {
				t.Errorf("want = %q, want %q", got.want, tt.wantWant)
			}
		})
	}
}

// reflectEqualStrings compares two string slices without pulling in
// reflect.DeepEqual's nil-vs-empty distinction: a table case that never sets
// wantCommands (an error case) compares nil against nil either way, and this
// keeps that comparison boring rather than reaching for a generic helper.
func reflectEqualStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReadme_quickstart_normalization(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "a per-request duration is normalised",
			in:   "  ✓ Hello World  200  12ms",
			want: "  ✓ Hello World  200  <ms>",
		},
		{
			name: "a summary duration in parentheses is normalised",
			in:   "  1 request(s): 1 passed, 0 failed (12ms)",
			want: "  1 request(s): 1 passed, 0 failed (<ms>)",
		},
		{
			name: "a status code is not a duration and survives",
			in:   "  ✓ Hello World  200  12ms",
			want: "  ✓ Hello World  200  <ms>",
		},
		{
			name: "0ms is normalised",
			in:   "  ✓ Hello World  200  0ms",
			want: "  ✓ Hello World  200  <ms>",
		},
		{
			name: "text with no durations is returned unchanged",
			in:   "Project initialized successfully!",
			want: "Project initialized successfully!",
		},
		{
			name: "a retry suffix is untouched",
			in:   "  ✓ Hello World  200  0ms (retry: 2)",
			want: "  ✓ Hello World  200  <ms> (retry: 2)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quickstartNormalizeDurations(tt.in)
			if got != tt.want {
				t.Errorf("quickstartNormalizeDurations(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// quickstartServer starts mudflat in-process on an ephemeral loopback port
// and returns the base URL the README's $BASE_URL is set to. The path
// suffix is mudflat's httpbin-compatible catch-all
// (testapi/mudflat/endpoints_echo.go's "/anything/{rest...}" -- mudflat has
// no "/get"), so the scaffolded {{base_url}}/get resolves to
// GET /anything/get.
//
// A zero mudflat.Options gets the specification's defaults, allocates no TLS
// material, and starts no goroutine of its own -- Serve is what starts
// serving, so t.Cleanup below is what stops it.
func quickstartServer(ctx context.Context, t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on an ephemeral loopback port: %v", err)
	}

	srv := mudflat.New(mudflat.Options{})
	go func() { _ = srv.Serve(ln) }() // Serve swallows http.ErrServerClosed itself.

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			t.Logf("mudflat shutdown: %v", err)
		}
	})

	return "http://" + ln.Addr().String() + "/anything"
}

// TestReadme_quickstart_actually_works is the observable: README.md's
// quickstart is run, not read. It extracts the section's command block and
// expected-output block, runs the commands in a temp directory against a
// local mudflat server, and asserts the (duration-normalised) output
// matches what the README shows byte for byte.
func TestReadme_quickstart_actually_works(t *testing.T) {
	repoRoot := readmeRepoRoot(t)
	doc := readmeReadFileOrFatal(t, filepath.Join(repoRoot, "README.md"))

	qs, err := quickstartExtract(doc)
	if err != nil {
		t.Fatalf("extract quickstart: %v", err)
	}

	// Vacuity guard, independent of the sentinels above: a truncated block
	// must fail here rather than be "successfully" run and matched against a
	// truncated expectation. Measured on this tree: 3 command lines.
	if len(qs.commands) < 3 {
		t.Fatalf("found %d quickstart command(s); measured 3 on this tree -- a command was silently dropped, or the extraction is broken", len(qs.commands))
	}
	// Floor on the expected-output side too, for the same reason: measured
	// 14 non-blank lines in the real output block.
	if nonBlankLines(qs.want) < 10 {
		t.Fatalf("quickstart expected-output block has only %d non-blank line(s); measured 14 on this tree", nonBlankLines(qs.want))
	}
	if !strings.Contains(qs.want, "passed") {
		t.Fatalf("quickstart expected-output block never mentions \"passed\" -- it does not look like a real curlew run's output")
	}

	// D3: the harness supplies environment and never rewrites the README's
	// text. A README that hardcoded a URL would silently stop being
	// redirectable at a local server, so that is checked here rather than
	// merely relied upon.
	if !strings.Contains(qs.script, "$BASE_URL") {
		t.Fatal("the quickstart block names no $BASE_URL -- the harness can no longer redirect it at a local server")
	}
	for _, host := range []string{"httpbin.org", "example.com", "https://", "http://"} {
		if strings.Contains(qs.script, host) {
			t.Fatalf("the quickstart block names %q directly -- it must reach only the local server the harness supplies via $BASE_URL", host)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	base := quickstartServer(ctx, t)
	binary := buildBinary(t)

	dir := t.TempDir()
	script := filepath.Join(dir, "quickstart.sh")
	if err := os.WriteFile(script, []byte(qs.script), 0o600); err != nil {
		t.Fatalf("write quickstart script: %v", err)
	}

	cmd := exec.CommandContext(ctx, "bash", "-euo", "pipefail", script)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"BASE_URL="+base,
		"NO_COLOR=1",
		"CURLEW_CONFIG_DIR="+t.TempDir(),
		"PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("README.md:%d: quickstart block failed: %v\n--- block ---\n%s\n--- output ---\n%s", qs.line, err, qs.script, out)
	}

	got := quickstartNormalizeDurations(strings.TrimSpace(string(out)))
	want := quickstartNormalizeDurations(strings.TrimSpace(qs.want))
	if got != want {
		t.Errorf("README.md:%d: quickstart output does not match what the README shows.\n--- got ---\n%s\n--- want ---\n%s", qs.line, got, want)
	}
}

// nonBlankLines counts the non-blank lines of s, for the expected-output
// floor guard above.
func nonBlankLines(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}
