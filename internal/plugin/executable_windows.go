//go:build windows

package plugin

import (
	"os"
	"path/filepath"
	"strings"
)

func isExecutable(path string, mode os.FileMode) bool {
	return mode.IsRegular() && strings.EqualFold(filepath.Ext(path), ".exe")
}