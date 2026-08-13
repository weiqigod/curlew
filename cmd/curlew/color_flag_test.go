package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --color={auto|always|never}.
//
// The manual documented this flag, with three worked examples, against a binary
// that answered `unknown flag: --color=never`. The documentation was corrected
// first — a flag that does not exist must not be advertised — and this builds
// the flag the documentation had been describing.
//
// `always` is the one that could not be expressed before. `--no-color` already
// covered "off", and auto-detection covered "on when it is safe", but there was
// no way to say "on anyway": piping a run to `less -R`, or capturing coloured
// output in CI, both need colour on a writer that is not a TTY.

func TestColorMode_parse(t *testing.T) {
	tests := []struct {
		arg     string
		want    colorMode
		wantErr bool
	}{
		{"auto", colorAuto, false},
		{"always", colorAlways, false},
		{"never", colorNever, false},
		{"", colorAuto, true},
		{"yes", colorAuto, true},
		{"Always", colorAuto, true}, // case-sensitive, like every other value
	}
	for _, tt := range tests {
		got, err := parseColorMode(tt.arg)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseColorMode(%q) accepted an invalid value", tt.arg)
				continue
			}
			// The error must name the alternatives, or the user has to guess.
			for _, want := range []string{"auto", "always", "never"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("parseColorMode(%q) error does not name %q: %v", tt.arg, want, err)
				}
			}
			continue
		}
		if err != nil {
			t.Errorf("parseColorMode(%q): %v", tt.arg, err)
		}
		if got != tt.want {
			t.Errorf("parseColorMode(%q) = %v, want %v", tt.arg, got, tt.want)
		}
	}
}

func TestShouldUseColor_modes(t *testing.T) {
	// A bytes.Buffer is never a TTY, which is exactly the case `always` exists
	// to override.
	var notATTY bytes.Buffer

	t.Setenv("NO_COLOR", "")
	if shouldUseColor(&notATTY, colorAlways) != true {
		t.Error("--color=always did not force colour on a non-TTY")
	}
	if shouldUseColor(&notATTY, colorNever) != false {
		t.Error("--color=never emitted colour")
	}
	if shouldUseColor(&notATTY, colorAuto) != false {
		t.Error("auto emitted colour on a non-TTY")
	}
}

// NO_COLOR is the user's standing preference; --color is a decision they made
// for this invocation. The more specific one wins, which is how ripgrep, git
// and grep all behave.
func TestShouldUseColor_explicitFlagBeatsNoColorEnv(t *testing.T) {
	var w bytes.Buffer
	t.Setenv("NO_COLOR", "1")

	if !shouldUseColor(&w, colorAlways) {
		t.Error("--color=always was overridden by NO_COLOR; an explicit flag is the more specific intent")
	}
	if shouldUseColor(&w, colorAuto) {
		t.Error("NO_COLOR was ignored under auto, where it is meant to decide")
	}
	if shouldUseColor(&w, colorNever) {
		t.Error("--color=never emitted colour")
	}
}

// --no-color keeps working and means exactly --color=never.
func TestParseRunArgs_noColorIsNever(t *testing.T) {
	f, err := parseRunArgs([]string{"c.yaml", "--no-color"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if f.color != colorNever {
		t.Errorf("--no-color gave %v, want colorNever", f.color)
	}
}

func TestParseRunArgs_colorForms(t *testing.T) {
	tests := []struct {
		args []string
		want colorMode
	}{
		{[]string{"c.yaml"}, colorAuto},
		{[]string{"c.yaml", "--color=always"}, colorAlways},
		{[]string{"c.yaml", "--color", "always"}, colorAlways},
		{[]string{"c.yaml", "--color=never"}, colorNever},
		{[]string{"c.yaml", "--color=auto"}, colorAuto},
	}
	for _, tt := range tests {
		f, err := parseRunArgs(tt.args)
		if err != nil {
			t.Errorf("parseRunArgs(%v): %v", tt.args, err)
			continue
		}
		if f.color != tt.want {
			t.Errorf("parseRunArgs(%v) color = %v, want %v", tt.args, f.color, tt.want)
		}
	}
}

func TestParseRunArgs_colorRejectsBadValue(t *testing.T) {
	if _, err := parseRunArgs([]string{"c.yaml", "--color=purple"}); err == nil {
		t.Fatal("--color=purple was accepted")
	}
}

// The invariant --color=always must not break: a machine format's payload never
// carries escape codes, however the flag is set. A consumer piping JSON must be
// able to parse it.
func TestColorAlways_neverLeaksIntoMachineFormats(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	collection := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(collection, []byte(
		"name: colour\nrequests:\n  - name: r\n    request:\n      method: GET\n"+
			"      url: \"http://127.0.0.1:1/\"\n"), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}

	for _, format := range []string{"json", "tap", "junit"} {
		cmd := exec.Command(bin, "run", collection, "--format", format, "--color=always")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &bytes.Buffer{}
		_ = cmd.Run() // the request fails; the payload is what matters

		if ansiRE.MatchString(stdout.String()) {
			t.Errorf("--format %s --color=always put ANSI escapes on stdout:\n%q",
				format, stdout.String())
		}
	}
}

// And the flag does what it says on a terminal format, where stdout is a pipe.
func TestColorAlways_forcesColourOnAPipedTerminalFormat(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	collection := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(collection, []byte(
		"name: colour\nrequests:\n  - name: r\n    request:\n      method: GET\n"+
			"      url: \"http://127.0.0.1:1/\"\n"), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}

	run := func(extra ...string) string {
		args := append([]string{"run", collection}, extra...)
		cmd := exec.Command(bin, args...)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &bytes.Buffer{}
		// Cleared so the environment cannot decide the outcome either way.
		cmd.Env = append(os.Environ(), "NO_COLOR=")
		cmd.Env = removeEnv(cmd.Env, "NO_COLOR")
		_ = cmd.Run()
		return stdout.String()
	}

	if plain := run(); ansiRE.MatchString(plain) {
		t.Errorf("default emitted colour on a pipe:\n%q", plain)
	}
	if forced := run("--color=always"); !ansiRE.MatchString(forced) {
		t.Errorf("--color=always did not emit colour on a pipe:\n%q", forced)
	}
}

func removeEnv(env []string, key string) []string {
	out := env[:0]
	for _, e := range env {
		if !strings.HasPrefix(e, key+"=") {
			out = append(out, e)
		}
	}
	return out
}
