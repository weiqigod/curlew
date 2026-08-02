package backend

import (
	"context"
	"errors"
	"fmt"
)

// DeviceStart is the response shape from POST /api/v1/auth/device/start.
// Maps RFC 8628 §3.2 device authorization response fields.
type DeviceStart struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// DevicePollResult is the success-shape from POST /api/v1/auth/device/poll.
// All four fields are persisted by the CLI after a successful poll.
type DevicePollResult struct {
	LicenseJWT   string `json:"license_jwt"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	DeviceID     string `json:"device_id"`
}

// RFC 8628 §3.5 sentinel errors. Mapped from ProblemDetails.Code.
// Callers should use errors.Is to branch on these rather than inspecting
// ProblemDetails fields directly.
var (
	// ErrAuthorizationPending is returned by PollDevice while the user has not
	// yet approved the authorization request.
	ErrAuthorizationPending = errors.New("backend: authorization_pending")
	// ErrSlowDown is returned by PollDevice when the polling rate is too high.
	// The caller MUST add 5 seconds to its polling interval per RFC 8628 §3.5.
	ErrSlowDown = errors.New("backend: slow_down")
	// ErrExpiredToken is returned by PollDevice when the device code has expired.
	// The user must restart the login flow.
	ErrExpiredToken = errors.New("backend: expired_token")
	// ErrAccessDenied is returned by PollDevice when the user explicitly denied
	// the authorization request.
	ErrAccessDenied = errors.New("backend: access_denied")
)

// pollCodeToSentinel maps AUTH_DEVICE_* problem codes to sentinel errors.
var pollCodeToSentinel = map[string]error{
	"AUTH_DEVICE_AUTHORIZATION_PENDING": ErrAuthorizationPending,
	"AUTH_DEVICE_SLOW_DOWN":             ErrSlowDown,
	"AUTH_DEVICE_EXPIRED_TOKEN":         ErrExpiredToken,
	"AUTH_DEVICE_ACCESS_DENIED":         ErrAccessDenied,
}

// StartDevice initiates the RFC 8628 device authorization flow by calling
// POST /api/v1/auth/device/start and returning the device start response.
func (c *Client) StartDevice(ctx context.Context) (DeviceStart, error) {
	var result DeviceStart
	if err := c.PostJSON(ctx, "/api/v1/auth/device/start", "", nil, &result); err != nil {
		return DeviceStart{}, err
	}
	return result, nil
}

// PollDevice polls POST /api/v1/auth/device/poll once with the given device
// code. It translates RFC 8628 §3.5 ProblemDetails.Code values into sentinel
// errors so the caller can use errors.Is rather than re-decoding the RFC 7807
// envelope. Unrecognised ProblemDetails codes are bubbled as *ProblemDetails.
//
// The wrapped form is fmt.Errorf("%w: %s", ErrXxx, pd.Detail) so callers get
// the sentinel via errors.Is and the human detail via Error().
func (c *Client) PollDevice(ctx context.Context, deviceCode string) (DevicePollResult, error) {
	body := map[string]string{"device_code": deviceCode}
	var result DevicePollResult
	err := c.PostJSON(ctx, "/api/v1/auth/device/poll", "", body, &result)
	if err == nil {
		return result, nil
	}

	// Check if it's a ProblemDetails with a device-code sentinel code.
	var pd *ProblemDetails
	if !errors.As(err, &pd) {
		// Network or server error — bubble as-is.
		return DevicePollResult{}, err
	}

	if sentinel, ok := pollCodeToSentinel[pd.Code]; ok {
		if pd.Detail != "" {
			return DevicePollResult{}, fmt.Errorf("%w: %s", sentinel, pd.Detail)
		}
		return DevicePollResult{}, sentinel
	}

	// Unrecognised problem code — return the *ProblemDetails directly.
	return DevicePollResult{}, pd
}
