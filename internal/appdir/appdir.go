// Package appdir resolves the per-user configuration directory holding local
// CLI state. Its only consumer today is telemetry, which keeps three files
// there: install_id, telemetry.json (the opt-in record) and telemetry.ndjson
// (the local events file). Nothing here is transmitted anywhere — the CLI
// makes no backend calls.
package appdir

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigEnv overrides the default config directory. Its name is published in
// `curlew telemetry --help` and in docs/MANUAL.md, and several cmd/curlew
// tests set it as a literal, so it is part of the CLI's contract.
const ConfigEnv = "CURLEW_CONFIG_DIR"

// ResolveConfigDir returns the directory to use for CLI state files.
// Precedence:
//  1. CURLEW_CONFIG_DIR, when set and non-empty, used verbatim
//  2. os.UserConfigDir()/curlew — ~/.config/curlew on Linux,
//     ~/Library/Application Support/curlew on macOS
func ResolveConfigDir() (string, error) {
	if c := os.Getenv(ConfigEnv); c != "" {
		return c, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(base, "curlew"), nil
}
