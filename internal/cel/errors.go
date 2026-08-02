package cel

const ellipsisMarker = "…" // U+2026 HORIZONTAL ELLIPSIS

// truncateSource returns src verbatim when it is at most 200 runes long, or
// the first 200 runes followed by ellipsisMarker otherwise. Operates on
// runes (not bytes) so multi-byte input is handled correctly.
func truncateSource(src string) string {
	runes := []rune(src)
	if len(runes) <= 200 {
		return src
	}
	return string(runes[:200]) + ellipsisMarker
}

// newParseError constructs a *CelError with Sentinel = ErrCelParse.
func newParseError(src string, inner error) error {
	return &CelError{
		Sentinel: ErrCelParse,
		Source:   truncateSource(src),
		Inner:    inner.Error(),
	}
}

// newTypeError constructs a *CelError with Sentinel = ErrCelType.
func newTypeError(src, actual, expected, innerMsg string) error {
	return &CelError{
		Sentinel: ErrCelType,
		Source:   truncateSource(src),
		Actual:   actual,
		Expected: expected,
		Inner:    innerMsg,
	}
}
