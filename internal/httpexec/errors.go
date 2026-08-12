package httpexec

import "errors"

// ErrNetwork indicates a network-level failure (DNS, connectivity, timeout).
var ErrNetwork = errors.New("network error")

// ErrDecode indicates the response arrived intact but could not be decoded —
// a Content-Encoding the body does not actually use, or a corrupt compressed
// stream.
//
// It is deliberately distinct from ErrNetwork. Everything that went wrong while
// reading a body used to be reported as a network error, which was wrong in
// both directions: a lying Content-Encoding was labelled a network failure even
// though no retry can fix it, and a genuine transport failure mid-body was
// never classified as a *errors.NetworkError, so retry_on.network_errors
// silently covered only the failures that happened before the body started.
var ErrDecode = errors.New("response decoding error")
