package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/datadriven"
	"github.com/weiqigod/curlew/internal/docs"
	"github.com/weiqigod/curlew/internal/output"
	"github.com/weiqigod/curlew/internal/plugin/hooks"
	"github.com/weiqigod/curlew/internal/variable"
)

// Tables that name things the binary either has or does not.
//
// Each of these is a list a reader will trust — the formats they can ask for,
// the telemetry subcommands they can run — and each was checked by nothing. The
// set on the binary's side is read from the binary, never restated here, so a
// name added or removed in the source moves both sides at once.

// backticked pulls `code`-spanned names out of a cell, which is how every one
// of these tables writes them.
var backticked = regexp.MustCompile("`([^`]+)`")

// firstBackticked returns the first `code` span in a cell, or "".
func firstBackticked(cell string) string {
	// SplitRow strips backticks from both ends of the whole cell, so a cell
	// like "`terminal` (default)" arrives with its opening backtick already
	// gone and its closing one still there. Trim what survives either way.
	if m := backticked.FindStringSubmatch(cell); m != nil {
		return strings.Trim(m[1], "`")
	}
	name := cell
	if i := strings.IndexAny(cell, " |"); i > 0 {
		name = cell[:i]
	}
	return strings.Trim(name, "`")
}

// columnNames reads one column of a table and returns the leading name in each
// row. It fails rather than returning nothing, so a moved table cannot make a
// check pass by finding no rows.
func columnNames(t *testing.T, doc, where, column string, header ...string) []string {
	t.Helper()

	hdr, rows, err := docs.TableUnder(doc, where, header...)
	if err != nil {
		t.Fatalf("%s under %q: %v", doc, where, err)
	}
	col := docs.Column(hdr, column)
	if col < 0 {
		t.Fatalf("%s under %q: no %q column in %v", doc, where, column, hdr)
	}
	var names []string
	for _, row := range rows {
		if len(row) <= col {
			continue
		}
		if name := firstBackticked(row[col]); name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		t.Fatalf("%s under %q: no names read from column %q", doc, where, column)
	}
	return names
}

// Both documents list the output formats. The list the binary accepts is
// output.SupportedFormats, and the two must be the same set in both
// directions: a format documented and unsupported is a user's run failing on
// the manual's advice, and one supported and undocumented is a feature nobody
// can find.
func TestDocTables_formatTablesMatchTheSupportedSet(t *testing.T) {
	supported := map[string]bool{}
	for _, f := range output.SupportedFormats {
		supported[f] = true
	}

	for _, tbl := range []struct {
		doc    string
		where  string
		header []string
	}{
		{"CLI_SPECIFICATION.md", "16.1 Formats", []string{"Format", "Destination", "Purpose"}},
		{"MANUAL.md", "4.1 Output formats", []string{"Format", "Intended for", "Flag", "Notes"}},
	} {
		documented := map[string]bool{}
		for _, name := range columnNames(t, tbl.doc, tbl.where, "Format", tbl.header...) {
			documented[name] = true
			if !supported[name] {
				t.Errorf("%s under %q documents format %q, which output.SupportedFormats does not list",
					tbl.doc, tbl.where, name)
			}
		}
		for _, f := range output.SupportedFormats {
			if !documented[f] {
				t.Errorf("%s under %q omits format %q, which the binary accepts",
					tbl.doc, tbl.where, f)
			}
		}
	}
}

// The telemetry table lists six subcommands. Each must be one the binary runs
// rather than one it rejects as unknown.
func TestDocTables_everyTelemetrySubcommandIsAccepted(t *testing.T) {
	bin := buildBinary(t)
	names := columnNames(t, "CLI_SPECIFICATION.md", "22. Telemetry", "Subcommand",
		"Subcommand", "Effect")

	// A private config directory: `enable` writes an install_id, and no test
	// should touch the developer's own.
	dir := t.TempDir()

	for _, name := range names {
		cmd := exec.Command(bin, "telemetry", name)
		cmd.Env = append(os.Environ(), "CURLEW_CONFIG_DIR="+dir, "NO_COLOR=1")
		out, _ := cmd.CombinedOutput()
		// Exit codes vary by subcommand — `status` exits 1 when telemetry was
		// never enabled, which is itself documented. What must never happen is
		// the binary not recognising the name at all.
		if strings.Contains(string(out), "unknown") && strings.Contains(string(out), name) {
			t.Errorf("CLI_SPECIFICATION.md documents `curlew telemetry %s`, which the binary rejects:\n%s",
				name, out)
		}
	}
}

// The limits table states numbers the binary enforces. Only the ones with a
// single unambiguous constant behind them are compared here; the rest are
// covered by the guard-rail tests that exercise the behaviour on breach.
func TestDocTables_limitsTableStatesTheEnforcedNumbers(t *testing.T) {
	hdr, rows, err := docs.TableUnder("CLI_SPECIFICATION.md", "24. Limits and Guard Rails",
		"Limit", "Value", "Behaviour on breach")
	if err != nil {
		t.Fatalf("limits table: %v", err)
	}
	limitCol, valueCol := docs.Column(hdr, "Limit"), docs.Column(hdr, "Value")
	if limitCol < 0 || valueCol < 0 {
		t.Fatalf("limits table lost a column: %v", hdr)
	}

	// Each value is rendered from the constant the binary actually enforces,
	// never typed out here — a second copy of the number would drift from the
	// first exactly the way the table did. Rows whose limit has no single
	// constant behind it are not checked here; they remain in the baseline.
	want := map[string]string{
		"Data-driven rows without confirmation": humanInt(datadriven.LargeDatasetThreshold),
		"Data-driven parallel workers":          strconv.Itoa(datadriven.DefaultMaxWorkers),
		"Plugin hook invocation":                fmt.Sprintf("%d s", int(hooks.HookTimeout.Seconds())),
		"Nested variable resolution depth":      strconv.Itoa(variable.MaxDepth),
	}

	seen := 0
	for _, row := range rows {
		if len(row) <= valueCol {
			continue
		}
		for fragment, value := range want {
			if !strings.Contains(row[limitCol], fragment) {
				continue
			}
			seen++
			if row[valueCol] != value {
				t.Errorf("limits table says %q is %q; the binary enforces %q",
					fragment, row[valueCol], value)
			}
		}
	}
	if seen != len(want) {
		t.Errorf("matched %d of %d limit rows; a row was renamed and its check silently stopped running",
			seen, len(want))
	}
}

// humanInt renders a threshold the way the documentation writes one: 10000
// becomes "10,000". The separator is the document's convention, so the value
// still comes from the constant rather than from a second copy of the number.
func humanInt(n int) string {
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}
