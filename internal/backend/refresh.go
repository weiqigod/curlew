package backend

import (
	"errors"
	"fmt"
)

var (
	// ErrRefreshExpired is returned when the backend's AUTH_REFRESH_EXPIRED
	// problem code surfaces. CLI exit code 4. Re-auth via `curlew login`.
	ErrRefreshExpired = errors.New("backend: refresh token expired")
	// ErrRefreshReused is returned for AUTH_REFRESH_REUSED. CLI exit code 5;
	// the entire refresh-token family has been revoked for security.
	ErrRefreshReused = errors.New("backend: refresh token reused — family revoked")
	// ErrDeviceMismatch is returned for AUTH_DEVICE_MISMATCH or when the
	// device_id sent does not match the family on the server. CLI exit code 7.
	ErrDeviceMismatch = errors.New("backend: device mismatch — re-register via curlew login")
)

// refreshCodeToSentinel maps AUTH_* problem codes returned by /auth/refresh
// to sentinel errors. Caller uses errors.Is to branch.
var refreshCodeToSentinel = map[string]error{
	"AUTH_REFRESH_EXPIRED": ErrRefreshExpired,
	"AUTH_REFRESH_REUSED":  ErrRefreshReused,
	"AUTH_DEVICE_MISMATCH": ErrDeviceMismatch,
}

// refreshSentinelError is a sentinel-wrapped error that also carries the
// original *ProblemDetails so callers can errors.As it for docs navigation.
type refreshSentinelError struct {
	sentinel error
	pd       *ProblemDetails
}

func (e *refreshSentinelError) Error() string {
	return fmt.Sprintf("%s: %s (%s)", e.sentinel, e.pd.Title, e.pd.Type)
}

// Unwrap returns both the sentinel and the original *ProblemDetails so that
// errors.Is(err, ErrRefreshExpired) and errors.As(err, &*ProblemDetails)
// both work.
func (e *refreshSentinelError) Unwrap() []error {
	return []error{e.sentinel, e.pd}
}

// translateRefreshError checks whether err carries one of the AUTH_REFRESH_*
// problem codes and, if so, returns a wrapped sentinel error. The original
// *ProblemDetails is preserved via errors.As-able wrapping so the caller can
// surface ProblemDetails.Type for docs lookup.
func translateRefreshError(err error) error {
	if err == nil {
		return nil
	}
	var pd *ProblemDetails
	if !errors.As(err, &pd) {
		return err
	}
	if sentinel, ok := refreshCodeToSentinel[pd.Code]; ok {
		return &refreshSentinelError{sentinel: sentinel, pd: pd}
	}
	return err
}
