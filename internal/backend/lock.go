package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// DefaultLockTimeout is the hard wall imposed on flock acquisition per
// SPECIFICATION.md:7942 — on timeout the waiter falls back to re-reading
// the cache and returning whatever token is present without triggering a
// second refresh.
const DefaultLockTimeout = 5 * time.Second

// Tokens is the trio returned by /api/v1/auth/refresh per
// SPECIFICATION.md:7907-7915.
type Tokens struct {
	LicenseJWT   string `json:"license_jwt"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// RefreshTokens acquires the flock at <configDir>/refresh.lock, calls
// POST /api/v1/auth/refresh, persists the new refresh token via storage, and
// releases the lock. Concurrent callers block on the lock; on timeout the
// caller re-reads storage and returns whatever token is present (no second
// refresh, per spec :7942).
//
// LicenseJWT is returned to the caller for the offline license cache. The
// rotated refresh token and short-lived API access token are persisted before
// return so sibling processes can reuse the complete backend session.
func RefreshTokens(
	ctx context.Context,
	client *Client,
	storage Storage,
	configDir, deviceID string,
) (Tokens, error) {
	lockPath := filepath.Join(configDir, refreshLockBase)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return Tokens{}, fmt.Errorf("mkdir config dir: %w", err)
	}
	lk := flock.New(lockPath)

	// Use the shorter of ctx deadline and DefaultLockTimeout for lock acquisition.
	lockTimeout := DefaultLockTimeout
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining < lockTimeout {
			lockTimeout = remaining
		}
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, lockTimeout)
	defer cancel()

	// Read the current token before acquiring the lock. We use this as a
	// baseline: inside the lock, if the token has changed, a sibling has
	// already refreshed and we reuse its result without a second network call.
	tokenBefore, _ := storage.GetRefreshToken()

	locked, err := lk.TryLockContext(timeoutCtx, 25*time.Millisecond)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		// Acquisition error (not just timeout) — bubble up.
		return Tokens{}, fmt.Errorf("flock: %w", err)
	}
	if !locked {
		// Timeout — fall back to cache per spec :7942.
		return tokensFromCache(storage)
	}
	defer func() { _ = lk.Unlock() }()

	// Inside the critical section: re-read cache. If the token has changed
	// since we read it before the lock (i.e., a sibling just refreshed),
	// return the updated token without a redundant network call.
	if t, err := tokensFromCache(storage); err == nil && t.RefreshToken != "" && t.RefreshToken != tokenBefore {
		return t, nil
	}

	rt, err := storage.GetRefreshToken()
	if err != nil {
		return Tokens{}, err
	}
	var out Tokens
	if err := client.PostJSON(ctx, "/api/v1/auth/refresh", "", map[string]string{
		"refresh_token": rt,
		"device_id":     deviceID,
	}, &out); err != nil {
		return Tokens{}, translateRefreshError(err)
	}
	if err := storage.SetRefreshToken(out.RefreshToken); err != nil {
		return Tokens{}, fmt.Errorf("persist refresh token: %w", err)
	}
	if out.AccessToken != "" {
		if err := storage.SetAccessToken(out.AccessToken); err != nil {
			return Tokens{}, fmt.Errorf("persist access token: %w", err)
		}
	}
	return out, nil
}

// tokensFromCache returns whatever the storage layer has on disk. Used
// after a flock timeout. LicenseJWT remains empty because it lives in the
// separate license cache; the access and refresh tokens come from secure
// storage written by the sibling process.
func tokensFromCache(s Storage) (Tokens, error) {
	rt, err := s.GetRefreshToken()
	if err != nil {
		return Tokens{}, err
	}
	at, err := s.GetAccessToken()
	if err != nil && !errors.Is(err, ErrTokenNotFound) {
		return Tokens{}, err
	}
	return Tokens{AccessToken: at, RefreshToken: rt}, nil
}
