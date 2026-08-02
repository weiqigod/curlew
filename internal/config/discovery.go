package config

import (
	"os"
	"path/filepath"
	"sort"
)

// ListCollections returns sorted relative paths of collection files
// found in the collections/ subdirectory of baseDir.
// Only top-level .yaml and .yml files are included.
// Returns nil if the directory doesn't exist or contains no YAML files.
func ListCollections(baseDir string) []string {
	colDir := filepath.Join(baseDir, "collections")
	entries, err := os.ReadDir(colDir)
	if err != nil {
		return nil
	}

	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext == ".yaml" || ext == ".yml" {
			paths = append(paths, filepath.Join("collections", e.Name()))
		}
	}
	sort.Strings(paths)
	return paths
}
