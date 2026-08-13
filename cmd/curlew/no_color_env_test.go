package main

import (
	"os"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
	"github.com/weiqigod/curlew/internal/output"
)

// NO_COLOR disables colour when it is set to a *non-empty* value.
//
// no-color.org: the variable takes effect "when present and not an empty
// string (regardless of its value)". curlew disabled on presence alone, so
// `NO_COLOR= curlew run ...` — the conventional way to clear an inherited
// preference for one command — turned colour off instead of leaving the TTY
// check to decide. Every other tool honouring the convention treats that as
// "no preference".
//
// Both documents' environment-variable tables had said "any non-empty value"
// since the variable shipped. The row was right and the binary was wrong, and
// neither noticed because nothing read the row; the second test here does.

// terminalStandIn returns a writer that output.IsTerminal reports as a
// terminal. /dev/null is a character device and so satisfies the same check a
// real terminal does, with no pty involved.
//
// The stand-in is what makes the decision observable at all: on an ordinary
// buffer, auto collapses to "no colour" whatever NO_COLOR says, and the
// variable cannot be seen to do anything either way.
func terminalStandIn(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("%s unavailable: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if !output.IsTerminal(f) {
		t.Skipf("%s is not a character device here; no hermetic terminal stand-in", os.DevNull)
	}
	return f
}

// setNoColor sets NO_COLOR to value, or removes it entirely when set is false.
func setNoColor(t *testing.T, value string, set bool) {
	t.Helper()
	// t.Setenv first either way, so the original value is restored on cleanup.
	t.Setenv("NO_COLOR", value)
	if !set {
		if err := os.Unsetenv("NO_COLOR"); err != nil {
			t.Fatalf("unset NO_COLOR: %v", err)
		}
	}
}

func TestNoColor_disablesOnlyWhenNonEmpty(t *testing.T) {
	tty := terminalStandIn(t)

	tests := []struct {
		name      string
		value     string
		set       bool
		wantColor bool
	}{
		{"unset", "", false, true},
		{"empty", "", true, true}, // the conventional "no preference"
		{"1", "1", true, false},   // the usual spelling
		{"0", "0", true, false},   // regardless of its value — 0 still disables
		{"false", "false", true, false},
		{"space", " ", true, false}, // a space is not the empty string
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setNoColor(t, tt.value, tt.set)
			if got := shouldUseColor(tty, colorAuto); got != tt.wantColor {
				t.Errorf("NO_COLOR=%q (set=%v): colour = %v, want %v",
					tt.value, tt.set, got, tt.wantColor)
			}
		})
	}
}

// An explicit --color still outranks the variable in both directions, which is
// the precedence the flag was built with and must survive this change.
func TestNoColor_explicitFlagStillWins(t *testing.T) {
	tty := terminalStandIn(t)

	setNoColor(t, "1", true)
	if !shouldUseColor(tty, colorAlways) {
		t.Error("--color=always was overridden by a non-empty NO_COLOR")
	}
	setNoColor(t, "", true)
	if shouldUseColor(tty, colorNever) {
		t.Error("--color=never emitted colour under an empty NO_COLOR")
	}
}

// The documents' environment-variable tables state the rule. Read the row and
// run it, rather than trusting that a sentence and a switch statement agree.
func TestNoColor_docTableRowIsWhatTheBinaryDoes(t *testing.T) {
	tty := terminalStandIn(t)

	docTables := []struct {
		doc    string
		header []string
	}{
		{"MANUAL.md", []string{"Variable", "Used by", "Meaning"}},
		{"CLI_SPECIFICATION.md", []string{"Variable", "Consumed by", "Meaning"}},
	}

	for _, dt := range docTables {
		header, rows, err := docs.Table(dt.doc, dt.header...)
		if err != nil {
			t.Fatalf("%s: %v", dt.doc, err)
		}
		varCol, meaningCol := docs.Column(header, "Variable"), docs.Column(header, "Meaning")
		if varCol < 0 || meaningCol < 0 {
			t.Fatalf("%s: environment-variable table lost its Variable/Meaning columns: %v", dt.doc, header)
		}

		meaning := ""
		for _, row := range rows {
			if len(row) > varCol && len(row) > meaningCol && row[varCol] == "NO_COLOR" {
				meaning = row[meaningCol]
			}
		}
		if meaning == "" {
			t.Fatalf("%s: no NO_COLOR row in the environment-variable table", dt.doc)
		}

		lower := strings.ToLower(meaning)
		if !strings.Contains(lower, "non-empty") || !strings.Contains(lower, "disable") {
			t.Fatalf("%s: NO_COLOR row no longer says a non-empty value disables colour: %q",
				dt.doc, meaning)
		}

		// Now execute what it says.
		setNoColor(t, "", true)
		if !shouldUseColor(tty, colorAuto) {
			t.Errorf("%s claims %q, but an empty NO_COLOR disabled colour", dt.doc, meaning)
		}
		setNoColor(t, "1", true)
		if shouldUseColor(tty, colorAuto) {
			t.Errorf("%s claims %q, but a non-empty NO_COLOR left colour on", dt.doc, meaning)
		}
	}
}
