package parser

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Slug returns a URL-safe identifier derived from name. Algorithm:
//
//  1. Unicode NFKD normalization (decomposes accents into base + combining marks).
//  2. Strip combining marks (Unicode category Mn).
//  3. Lowercase ASCII letters; preserve [a-z0-9]. Everything else is treated as
//     a separator.
//  4. Collapse runs of separator runes to a single '-'.
//  5. Trim leading and trailing '-'.
//
// If the result is empty (name was whitespace-only or punctuation-only after
// normalization), Slug returns an empty string and ErrSlugEmpty wrapped in a
// fmt.Errorf. Callers in the parser surface this as a load-time error matching
// the early-reject posture of duplicate-name detection.
func Slug(name string) (string, error) {
	// Step 1+2: NFKD then strip combining marks (Mn category).
	t := transform.Chain(
		norm.NFKD,
		runes.Remove(runes.In(unicode.Mn)),
	)
	decomposed, _, err := transform.String(t, name)
	if err != nil {
		// transform.String only errors on writer failures; in-memory
		// transformation cannot fail. Treat as an internal bug.
		return "", fmt.Errorf("parser: slug normalization failed: %w", err)
	}

	// Steps 3+4: lowercase + collapse non-[a-z0-9] runs to a single '-'.
	var b strings.Builder
	b.Grow(len(decomposed))
	prevSep := true // suppress leading separators
	for _, r := range decomposed {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			prevSep = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevSep = false
		default:
			if !prevSep {
				b.WriteRune('-')
				prevSep = true
			}
		}
	}

	// Step 5: trim trailing '-' (leading was suppressed above).
	out := strings.TrimRight(b.String(), "-")
	if out == "" {
		return "", fmt.Errorf("parser: %w: %q", ErrSlugEmpty, name)
	}
	return out, nil
}
