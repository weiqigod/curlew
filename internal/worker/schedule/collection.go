package schedule

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ResolveCollection resolves a schedule collection_ref to an absolute file
// path on the local filesystem. Only the "file:" scheme is supported; all
// other schemes return ErrUnsupportedCollectionRef.
//
// For "file:" refs:
//   - "file:./relative" and "file:relative" are joined with workingDir.
//   - "file:/absolute" is returned as-is after stripping the "file:" prefix.
//   - An empty path after stripping the prefix returns ErrUnsupportedCollectionRef.
//
// Future extension point for "git:" refs: see TODO(M-future) in this function.
func ResolveCollection(ref, workingDir string) (string, error) {
	// TODO(M-future): add git: scheme resolution here. When implementing,
	// the worker host must have git available. Per Open Decision 4 this is
	// deferred past M16; the function is isolated so only this file changes.
	const fileScheme = "file:"

	if !strings.HasPrefix(ref, fileScheme) {
		if strings.HasPrefix(ref, "git:") {
			return "", fmt.Errorf("%w: git: collection refs are not yet supported in M16; "+
				"please clone the repo on the worker host and use file: instead", ErrUnsupportedCollectionRef)
		}
		return "", fmt.Errorf("%w: %q", ErrUnsupportedCollectionRef, ref)
	}

	rest := strings.TrimPrefix(ref, fileScheme)
	if rest == "" {
		return "", fmt.Errorf("%w: file: ref has empty path", ErrUnsupportedCollectionRef)
	}

	// Absolute path: return as-is.
	if filepath.IsAbs(rest) {
		return rest, nil
	}

	// Relative path: join with workingDir.
	return filepath.Join(workingDir, rest), nil
}
