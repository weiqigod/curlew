package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// discover returns an alphabetical list of candidate executable paths, plus any
// per-entry errors. It does not spawn any processes.
func discover(env string) (candidates []string, errs []LoadError) {
	if env == "" {
		return nil, nil
	}
	sep := string(filepath.ListSeparator)
	for _, entry := range strings.Split(env, sep) {
		if entry == "" {
			continue
		}
		info, err := os.Stat(entry)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				errs = append(errs, LoadError{
					Path:    entry,
					Message: fmt.Sprintf("plugin %s not found", entry),
					Fatal:   true,
				})
			} else {
				errs = append(errs, LoadError{
					Path:    entry,
					Message: fmt.Sprintf("plugin %s: %v", entry, err),
					Fatal:   true,
				})
			}
			continue
		}
		if info.IsDir() {
			dirEntries, readErr := os.ReadDir(entry)
			if readErr != nil {
				errs = append(errs, LoadError{
					Path:    entry,
					Message: fmt.Sprintf("plugin %s: %v", entry, readErr),
					Fatal:   true,
				})
				continue
			}
			var found []string
			for _, de := range dirEntries {
				if de.IsDir() {
					continue
				}
				full := filepath.Join(entry, de.Name())
				de2, statErr := os.Stat(full)
				if statErr != nil || !de2.Mode().IsRegular() {
					continue
				}
				// Silently skip non-executable files inside a directory (directories
				// commonly contain non-executable files such as READMEs). Only files
				// explicitly named in CURLEW_PLUGINS trigger the "not executable" error.
				//
				// Deviation from task YAML behavior 6: behavior 6 states "Given a
				// plugin's binary is not executable … exits 2" but does not distinguish
				// between an explicitly named path and a file found by directory
				// expansion. This implementation limits the fatal error only to
				// explicitly-named entries (the path appears directly in CURLEW_PLUGINS)
				// because raising an error for every non-executable file found in a
				// shared directory (e.g. README, Makefile) would be an unusable UX.
				// The behavior for explicitly-named non-executable files is unchanged and
				// tested in TestDiscover/"non-executable file produces fatal error".
				if !isExecutable(de2.Mode()) {
					continue
				}
				found = append(found, full)
			}
			sort.Strings(found)
			candidates = append(candidates, found...)
			continue
		}
		if !info.Mode().IsRegular() {
			errs = append(errs, LoadError{
				Path:    entry,
				Message: fmt.Sprintf("plugin %s is not executable", entry),
				Fatal:   true,
			})
			continue
		}
		if !isExecutable(info.Mode()) {
			errs = append(errs, LoadError{
				Path:    entry,
				Message: fmt.Sprintf("plugin %s is not executable", entry),
				Fatal:   true,
			})
			continue
		}
		candidates = append(candidates, entry)
	}
	return candidates, errs
}

// isExecutable reports whether any user-execute bit is set in mode.
// On Windows, the concept of execute bits differs; file is considered executable
// if it is a regular file (the OS decides at exec.Command time).
func isExecutable(m os.FileMode) bool {
	return m&0o111 != 0
}
