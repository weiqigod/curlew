// Package templates provides loading of external WebSocket message template files.
// Template files may contain {{variable}} placeholders that are preserved for
// later interpolation by the executor at send time.
package templates

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrTemplateNotFound is returned when a message template file does not exist.
var ErrTemplateNotFound = errors.New("websocket message template not found")

// LoadTemplate reads a message template file and returns its contents plus
// the absolute resolved path. Relative paths are resolved against baseDir.
// Variable placeholders ({{name}}) are preserved verbatim for later interpolation.
func LoadTemplate(baseDir, relPath string) (body, abs string, err error) {
	abs = relPath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(baseDir, relPath)
	}
	data, readErr := os.ReadFile(abs)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return "", "", fmt.Errorf("%w: %s", ErrTemplateNotFound, relPath)
		}
		return "", "", fmt.Errorf("reading message_template %q: %w", relPath, readErr)
	}
	return string(data), abs, nil
}
