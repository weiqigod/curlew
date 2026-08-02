// Package httpbody loads HTTP request bodies from external files.
//
// Two variants are supported:
//
//   - Text: bytes are loaded verbatim; {{variable}} placeholders are preserved
//     for later interpolation by the runner at request time.
//   - Binary: bytes are loaded verbatim and sent raw; no interpolation.
//
// Callers are responsible for deciding which variant to treat the returned
// content as. This package is oblivious to that distinction — it only reads
// bytes, resolves paths, and enforces a size cap.
package httpbody

import (
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
)

// DefaultMaxBytes is the default size cap applied when LoadBodyInput.MaxBytes
// is zero. Collections that legitimately need larger bodies should not use
// body_file and instead stream the payload via an application-level tool.
const DefaultMaxBytes int64 = 50 * 1024 * 1024 // 50 MiB

// LoadBodyInput parameterizes a call to LoadBody.
type LoadBodyInput struct {
	// BaseDir is the directory that relative BodyFile paths are resolved against.
	// Ignored when BodyFile is absolute.
	BaseDir string

	// BodyFile is the YAML-supplied path. Relative paths are resolved against
	// BaseDir; absolute paths are honored as-is.
	BodyFile string

	// MaxBytes is the maximum allowed file size. If 0, DefaultMaxBytes is used.
	// A negative value disables the size check (intended for tests only).
	MaxBytes int64
}

// LoadBodyResult holds the loaded body along with metadata the caller needs
// to feed into downstream pipelines (parser ExternalFiles, Content-Type
// auto-detection).
type LoadBodyResult struct {
	// Body is the raw file contents. Not copied — do not mutate.
	Body []byte

	// FilePath is the absolute resolved path of the loaded file.
	FilePath string

	// ContentType is the MIME type derived from the file extension via
	// mime.TypeByExtension. Empty when the extension is unknown; callers
	// decide whether to fall back to application/octet-stream (binary variant)
	// or leave unset (text variant).
	ContentType string
}

// LoadBody reads a body file and returns its raw contents, the absolute path,
// and the auto-detected Content-Type. Relative BodyFile values are resolved
// against BaseDir; absolute paths are honored as-is. Returns a wrapped
// ErrBodyFileNotFound if the file is missing, ErrBodyFileTooLarge if it
// exceeds the size cap.
func LoadBody(input LoadBodyInput) (*LoadBodyResult, error) {
	if input.BodyFile == "" {
		return nil, fmt.Errorf("body file path is empty")
	}

	abs := input.BodyFile
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(input.BaseDir, input.BodyFile)
	}

	limit := input.MaxBytes
	if limit == 0 {
		limit = DefaultMaxBytes
	}

	if limit > 0 {
		info, statErr := os.Stat(abs)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				return nil, fmt.Errorf("%w: %s", ErrBodyFileNotFound, input.BodyFile)
			}
			return nil, fmt.Errorf("stat body file %q: %w", input.BodyFile, statErr)
		}
		if info.Size() > limit {
			return nil, fmt.Errorf("%w: %q is %d bytes (limit %d)", ErrBodyFileTooLarge, input.BodyFile, info.Size(), limit)
		}
	}

	data, readErr := os.ReadFile(abs)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrBodyFileNotFound, input.BodyFile)
		}
		return nil, fmt.Errorf("reading body file %q: %w", input.BodyFile, readErr)
	}

	return &LoadBodyResult{
		Body:        data,
		FilePath:    abs,
		ContentType: mime.TypeByExtension(filepath.Ext(abs)),
	}, nil
}
