package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/appdir"
)

// The telemetry help text used to hardcode "~/.config/curlew". That is the
// Linux answer: os.UserConfigDir resolves to ~/Library/Application Support on
// macOS and %AppData% on Windows, so the paths printed to most developers on
// this project were simply wrong. Printing the resolved directory is both
// correct everywhere and more useful, because it also reflects an active
// CURLEW_CONFIG_DIR override.

func TestTelemetryHelp_reports_the_active_config_dir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(appdir.ConfigEnv, dir)

	var buf bytes.Buffer
	printTelemetryHelpTo(&buf)
	out := buf.String()

	if !strings.Contains(out, dir) {
		t.Errorf("help text does not mention the active config dir %q\n---\n%s", dir, out)
	}
	// The CURLEW_CONFIG_DIR entry must show the resolved value specifically.
	// Without this the directory appearing anywhere in the output satisfies
	// the check above, which leaves the one line a user consults to confirm
	// an override took effect entirely unpinned.
	if !strings.Contains(out, "Currently: "+dir) {
		t.Errorf("the CURLEW_CONFIG_DIR entry does not report the resolved directory %q\n---\n%s", dir, out)
	}
}

func TestTelemetryHelp_does_not_hardcode_a_platform_path(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(appdir.ConfigEnv, dir)

	var buf bytes.Buffer
	printTelemetryHelpTo(&buf)
	out := buf.String()

	if strings.Contains(out, "~/.config/curlew") {
		t.Errorf("help text hardcodes ~/.config/curlew, which is wrong on macOS and Windows\n---\n%s", out)
	}
}

// With no override, the help must name the same directory the commands
// actually write to -- resolved through appdir, not restated as a literal.
func TestTelemetryHelp_default_matches_appdir(t *testing.T) {
	t.Setenv(appdir.ConfigEnv, "")

	want, err := appdir.ResolveConfigDir()
	if err != nil {
		t.Skipf("os.UserConfigDir unavailable on this host: %v", err)
	}

	var buf bytes.Buffer
	printTelemetryHelpTo(&buf)
	out := buf.String()

	if !strings.Contains(out, want) {
		t.Errorf("help text does not mention the resolved default %q\n---\n%s", want, out)
	}
}

// Making the directory dynamic must not cost the reader the filenames. This
// asserts each name appears somewhere in the help, not in any particular
// line -- moving "telemetry.ndjson" between the delete and env-var entries is
// a presentation choice, while dropping it entirely leaves a user unable to
// tell what curlew stores.
func TestTelemetryHelp_still_names_the_state_files(t *testing.T) {
	t.Setenv(appdir.ConfigEnv, t.TempDir())

	var buf bytes.Buffer
	printTelemetryHelpTo(&buf)
	out := buf.String()

	for _, name := range []string{"install_id", "telemetry.json", "telemetry.ndjson"} {
		if !strings.Contains(out, name) {
			t.Errorf("help text no longer names %q\n---\n%s", name, out)
		}
	}
}
