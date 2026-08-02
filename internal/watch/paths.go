package watch

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/peterlindqvist/apitest/internal/config"
	"github.com/peterlindqvist/apitest/internal/parser"
)

// Paths holds all file paths that should be monitored for a collection run.
type Paths struct {
	Collection    string   // absolute path to the main collection file
	ExternalFiles []string // absolute paths to external request files
	EnvFile       string   // absolute path to environment file (empty if not used)
	DotEnv        string   // absolute path to .env file (empty if not found)
	ProjectConfig string   // absolute path to apitest.yaml (empty if not found)
}

// All returns a deduplicated, sorted list of all non-empty file paths to watch.
func (wp *Paths) All() []string {
	seen := make(map[string]struct{})
	var paths []string

	add := func(p string) {
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}

	add(wp.Collection)
	for _, f := range wp.ExternalFiles {
		add(f)
	}
	add(wp.EnvFile)
	add(wp.DotEnv)
	add(wp.ProjectConfig)

	sort.Strings(paths)
	return paths
}

// Dirs returns a deduplicated, sorted list of directories containing watched files.
func (wp *Paths) Dirs() []string {
	all := wp.All()
	seen := make(map[string]struct{})
	var dirs []string
	for _, p := range all {
		d := filepath.Dir(p)
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs
}

// CollectPaths determines all files related to a collection run.
// It parses the collection to discover external file references and
// uses the config package to locate environment, project, and dotenv files.
func CollectPaths(collectionPath, envName string) (*Paths, error) {
	absCol, err := filepath.Abs(collectionPath)
	if err != nil {
		return nil, fmt.Errorf("resolving collection path: %w", err)
	}

	col, err := parser.ParseFile(collectionPath)
	if err != nil {
		return nil, fmt.Errorf("parsing collection: %w", err)
	}

	wp := &Paths{
		Collection:    absCol,
		ExternalFiles: col.ExternalFiles,
	}

	collectionDir := filepath.Dir(absCol)

	// Locate environment file if specified.
	if envName != "" {
		envPath, envErr := config.FindEnvironmentFile(envName, collectionDir)
		if envErr != nil {
			return nil, fmt.Errorf("finding environment file: %w", envErr)
		}
		absEnv, envErr := filepath.Abs(envPath)
		if envErr != nil {
			return nil, fmt.Errorf("resolving environment path: %w", envErr)
		}
		wp.EnvFile = absEnv
	}

	// Locate project config.
	projectRoot, found := config.FindProjectRoot(collectionDir)
	if found {
		for _, name := range []string{"apitest.yaml", "apitest.yml"} {
			cfgPath := filepath.Join(projectRoot, name)
			if _, statErr := os.Stat(cfgPath); statErr == nil {
				wp.ProjectConfig = cfgPath
				break
			}
		}
	}

	// Locate .env file: prefer project root, fall back to collection dir.
	dotenvDir := collectionDir
	if projectRoot != "" {
		dotenvDir = projectRoot
	}
	dotenvPath := filepath.Join(dotenvDir, ".env")
	if _, statErr := os.Stat(dotenvPath); statErr == nil {
		wp.DotEnv = dotenvPath
	}

	return wp, nil
}
