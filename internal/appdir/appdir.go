// Package appdir resolves the per-user configuration directory used for
// device registration, backend session tokens, and telemetry state.
package appdir

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigEnv overrides the default ~/.config/apitesttool directory.
const ConfigEnv = "APITEST_CONFIG_DIR"

// ResolveConfigDir returns the directory to use for CLI state files.
// Precedence:
//  1. APITEST_CONFIG_DIR
//  2. os.UserConfigDir()/apitesttool
func ResolveConfigDir() (string, error) {
	if c := os.Getenv(ConfigEnv); c != "" {
		return c, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(base, "apitesttool"), nil
}
