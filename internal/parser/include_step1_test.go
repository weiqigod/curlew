package parser

import (
	"errors"
	"testing"
)

// Step 1 tests: sentinel errors and Include field on Collection.

func TestSentinelErrors_include(t *testing.T) {
	t.Run("ErrCircularInclude exists", func(t *testing.T) {
		if ErrCircularInclude == nil {
			t.Fatal("ErrCircularInclude is nil")
		}
	})

	t.Run("ErrIncludeNotFound exists", func(t *testing.T) {
		if ErrIncludeNotFound == nil {
			t.Fatal("ErrIncludeNotFound is nil")
		}
	})

	t.Run("ErrCircularInclude is distinct from ErrCircularFileReference", func(t *testing.T) {
		if errors.Is(ErrCircularInclude, ErrCircularFileReference) {
			t.Fatal("ErrCircularInclude should be distinct from ErrCircularFileReference")
		}
	})

	t.Run("ErrIncludeNotFound is distinct from ErrExternalFileNotFound", func(t *testing.T) {
		if errors.Is(ErrIncludeNotFound, ErrExternalFileNotFound) {
			t.Fatal("ErrIncludeNotFound should be distinct from ErrExternalFileNotFound")
		}
	})
}
