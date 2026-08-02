package httpexec

import "errors"

// ErrNetwork indicates a network-level failure (DNS, connectivity, timeout).
var ErrNetwork = errors.New("network error")
