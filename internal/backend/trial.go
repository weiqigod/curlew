package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for trial activation.
var (
	// ErrTrialAlreadyConsumed is returned when the backend's TRIAL_ALREADY_CONSUMED
	// problem code surfaces (HTTP 409). CLI exit code 5; the user has previously
	// used a trial of the requested feature.
	ErrTrialAlreadyConsumed = errors.New("backend: trial already consumed")
	// ErrTrialFeatureUnknown is returned for TRIAL_FEATURE_UNKNOWN (HTTP 404).
	// The feature slug is not registered.
	ErrTrialFeatureUnknown = errors.New("backend: trial feature unknown")
)

// trialCodeToSentinel routes problem codes returned by /api/v1/trials/{feature}.
var trialCodeToSentinel = map[string]error{
	"TRIAL_ALREADY_CONSUMED": ErrTrialAlreadyConsumed,
	"TRIAL_FEATURE_UNKNOWN":  ErrTrialFeatureUnknown,
}

// TrialActivation is the success-shape from POST /api/v1/trials/{feature}.
type TrialActivation struct {
	Feature   string    `json:"feature"`
	Kind      string    `json:"kind"`
	GrantedAt time.Time `json:"granted_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Tokens    Tokens    `json:"tokens"`
}

// TrialPreviousGrant holds the previous-grant details from a TRIAL_ALREADY_CONSUMED
// 409 response body. Callers can retrieve it via errors.As(err, &TrialPreviousGrant{}).
type TrialPreviousGrant struct {
	Feature   string    `json:"feature"`
	Kind      string    `json:"kind"`
	GrantedAt time.Time `json:"granted_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// trialSentinelError mirrors refreshSentinelError — sentinel + ProblemDetails
// preserved via errors.As for docs navigation. It additionally surfaces the
// previous_grant extension from 409 responses so the CLI can print the date.
type trialSentinelError struct {
	sentinel      error
	pd            *ProblemDetails
	previousGrant *TrialPreviousGrant // nil when not a 409 ALREADY_CONSUMED
}

func (e *trialSentinelError) Error() string {
	return fmt.Sprintf("%s: %s (%s)", e.sentinel, e.pd.Title, e.pd.Type)
}

// Unwrap returns both the sentinel and the original *ProblemDetails so that
// errors.Is(err, ErrTrialAlreadyConsumed) and errors.As(err, &*ProblemDetails)
// both work.
func (e *trialSentinelError) Unwrap() []error {
	return []error{e.sentinel, e.pd}
}

// PreviousGrant returns the previous_grant detail from a TRIAL_ALREADY_CONSUMED
// error, or nil if this error is not of that type.
func (e *trialSentinelError) PreviousGrant() *TrialPreviousGrant {
	return e.previousGrant
}

// parsePreviousGrant attempts to decode the "previous_grant" extension field
// from a TRIAL_ALREADY_CONSUMED ProblemDetails. Returns nil on any error.
func parsePreviousGrant(pd *ProblemDetails) *TrialPreviousGrant {
	if pd.Extensions == nil {
		return nil
	}
	raw, ok := pd.Extensions["previous_grant"]
	if !ok {
		return nil
	}
	var pg TrialPreviousGrant
	if err := json.Unmarshal(raw, &pg); err != nil {
		return nil
	}
	return &pg
}

// StartTrial calls POST /api/v1/trials/{feature}.
//
// On 200, returns the activation result with re-minted tokens.
// On 404 with TRIAL_FEATURE_UNKNOWN → ErrTrialFeatureUnknown.
// On 409 with TRIAL_ALREADY_CONSUMED → wrapped sentinel that also implements
// errors.As for *ProblemDetails so callers can inspect the previous_grant extension.
func (c *Client) StartTrial(
	ctx context.Context,
	feature, refreshToken, deviceID, accessToken string,
) (TrialActivation, error) {
	body := map[string]string{
		"refresh_token": refreshToken,
		"device_id":     deviceID,
	}
	var out TrialActivation
	err := c.PostJSON(ctx, "/api/v1/trials/"+feature, accessToken, body, &out)
	if err == nil {
		return out, nil
	}

	var pd *ProblemDetails
	if !errors.As(err, &pd) {
		return TrialActivation{}, err
	}
	if sentinel, ok := trialCodeToSentinel[pd.Code]; ok {
		se := &trialSentinelError{sentinel: sentinel, pd: pd}
		if pd.Code == "TRIAL_ALREADY_CONSUMED" {
			se.previousGrant = parsePreviousGrant(pd)
		}
		return TrialActivation{}, se
	}
	// Also route AUTH_* codes through the refresh sentinel table for
	// symmetry with the --refresh flow (device-mismatch, expired, etc.).
	if translated := translateRefreshError(err); translated != err {
		return TrialActivation{}, translated
	}
	return TrialActivation{}, err
}
