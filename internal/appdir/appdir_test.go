package appdir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ResolveConfigDir has one rule with two branches, and both matter: the
// override is how every test in cmd/curlew keeps its state out of the
// developer's real ~/.config, and the fallback is what real users get.

func TestResolveConfigDir_env_override_wins(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom-config")
	t.Setenv(ConfigEnv, want)

	got, err := ResolveConfigDir()
	if err != nil {
		t.Fatalf("ResolveConfigDir: %v", err)
	}
	if got != want {
		t.Errorf("ResolveConfigDir() = %q, want %q", got, want)
	}
}

// TestResolveConfigDir_env_override_is_used_verbatim pins that the override
// is not joined with anything. Appending "curlew" to it would silently move
// every test fixture's state one directory deeper.
func TestResolveConfigDir_env_override_is_used_verbatim(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ConfigEnv, dir)

	got, err := ResolveConfigDir()
	if err != nil {
		t.Fatalf("ResolveConfigDir: %v", err)
	}
	if got != dir {
		t.Errorf("ResolveConfigDir() = %q, want exactly %q with no suffix appended", got, dir)
	}
}

// TestResolveConfigDir_empty_env_falls_through covers the "non-empty" half of
// the precedence rule. An exported but empty CURLEW_CONFIG_DIR must behave as
// if it were unset, not resolve state to the process working directory.
func TestResolveConfigDir_empty_env_falls_through(t *testing.T) {
	t.Setenv(ConfigEnv, "")

	got, err := ResolveConfigDir()
	if err != nil {
		t.Fatalf("ResolveConfigDir: %v", err)
	}
	if got == "" {
		t.Fatal("ResolveConfigDir() = \"\" — an empty override must fall through to the user config dir")
	}
	if filepath.Base(got) != "curlew" {
		t.Errorf("ResolveConfigDir() = %q, want a path ending in %q", got, "curlew")
	}
}

// TestResolveConfigDir_default_is_user_config_dir_curlew pins the fallback
// against the standard library rather than against a hardcoded platform path,
// so the test holds on darwin (~/Library/Application Support) and linux
// (~/.config) alike.
func TestResolveConfigDir_default_is_user_config_dir_curlew(t *testing.T) {
	t.Setenv(ConfigEnv, "")

	base, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("os.UserConfigDir unavailable on this host: %v", err)
	}
	want := filepath.Join(base, "curlew")

	got, err := ResolveConfigDir()
	if err != nil {
		t.Fatalf("ResolveConfigDir: %v", err)
	}
	if got != want {
		t.Errorf("ResolveConfigDir() = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, base) {
		t.Errorf("ResolveConfigDir() = %q, want it under the user config dir %q", got, base)
	}
}

// TestConfigEnv_name is not decoration. The literal "CURLEW_CONFIG_DIR"
// appears in `curlew telemetry --help`, in docs/MANUAL.md, and hardcoded in
// several cmd/curlew tests that call t.Setenv directly. Renaming the constant
// alone would leave all of those pointing at a variable nothing reads.
func TestConfigEnv_name(t *testing.T) {
	if ConfigEnv != "CURLEW_CONFIG_DIR" {
		t.Errorf("ConfigEnv = %q, want %q — this name is published in help text, MANUAL.md and cmd/curlew tests",
			ConfigEnv, "CURLEW_CONFIG_DIR")
	}
}
