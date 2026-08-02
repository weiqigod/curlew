// Package discovery expands glob patterns into collection file paths,
// honoring .apitestignore and rejecting unsafe patterns.
//
// Supported glob syntax (relative to the walk root):
//
//	`*`      matches any sequence of non-separator characters
//	`?`      matches any single non-separator character
//	`[abc]`  character class (as in filepath.Match)
//	`**`     matches zero or more path segments (including separators)
//
// Patterns are always interpreted with forward slashes as separators,
// regardless of the operating system. Windows absolute patterns (C:\...)
// are rejected. Symlinks to directories are not followed during the walk.
package discovery

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Sentinel errors returned by Expand.
var (
	// ErrNoMatches is returned when a glob pattern matches zero files.
	ErrNoMatches = errors.New("no collections matched pattern")
	// ErrTraversalOutsideRoot is returned when a pattern contains "..".
	ErrTraversalOutsideRoot = errors.New("pattern escapes working directory")
	// ErrAbsolutePattern is returned when an absolute path is used as a glob.
	ErrAbsolutePattern = errors.New("absolute glob patterns are not supported")
)

// IsGlob reports whether pattern contains any glob metacharacters (* ? [).
func IsGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

// Expand walks root and returns absolute paths of all files matching pattern,
// sorted deterministically (lexicographic on the relative path). Ignore rules
// from .apitestignore at root are applied before returning.
//
// Returns ErrNoMatches when nothing matches, ErrTraversalOutsideRoot when
// the pattern contains "..", and ErrAbsolutePattern when the pattern is
// absolute.
func Expand(root, pattern string) ([]string, error) {
	// Safety: reject absolute patterns.
	if filepath.IsAbs(pattern) {
		return nil, ErrAbsolutePattern
	}
	// Safety: reject traversal. Check both forward-slash and OS separator.
	normalized := filepath.ToSlash(pattern)
	if strings.HasPrefix(normalized, "../") ||
		strings.Contains(normalized, "/../") ||
		strings.HasSuffix(normalized, "/..") ||
		normalized == ".." {
		return nil, ErrTraversalOutsideRoot
	}

	// Load ignore rules.
	ignorePatterns, err := LoadIgnore(root)
	if err != nil {
		return nil, fmt.Errorf("loading .apitestignore at %s: %w", root, err)
	}

	// Normalize pattern to forward slashes for matching.
	patternFwd := filepath.ToSlash(pattern)

	var matches []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("accessing %s: %w", path, err)
		}

		// Compute path relative to root, always with forward slashes.
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("computing relative path for %s: %w", path, relErr)
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			// Skip hidden directories (starting with '.'), except root itself.
			if rel != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Match against primary pattern.
		if !matchPattern(patternFwd, rel) {
			return nil
		}

		// Check ignore patterns.
		for _, ig := range ignorePatterns {
			if matchPattern(ig, rel) {
				return nil
			}
		}

		matches = append(matches, path)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walking %s: %w", root, walkErr)
	}

	if len(matches) == 0 {
		return nil, ErrNoMatches
	}

	// Sort deterministically by relative path (forward slashes).
	sort.Slice(matches, func(i, j int) bool {
		ri, _ := filepath.Rel(root, matches[i])
		rj, _ := filepath.Rel(root, matches[j])
		return filepath.ToSlash(ri) < filepath.ToSlash(rj)
	})

	return matches, nil
}

// LoadIgnore reads .apitestignore at root (if present) and returns
// its non-comment, non-blank patterns. Returns nil, nil when the file
// does not exist.
func LoadIgnore(root string) ([]string, error) {
	ignoreFile := filepath.Join(root, ".apitestignore")
	f, err := os.Open(ignoreFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening .apitestignore: %w", err)
	}
	defer func() { _ = f.Close() }()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading .apitestignore: %w", err)
	}
	return patterns, nil
}
