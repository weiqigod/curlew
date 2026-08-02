package auth

import (
	apierrors "github.com/peterlindqvist/apitest/internal/errors"
)

func init() {
	apierrors.RegisterPackage("auth",
		apierrors.RegisteredError{
			Name: "ErrProfileFailed",
			Err:  ErrProfileFailed,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAuth,
				Code:     "AUTH_PROFILE_FAILED",
				Hint:     "The auth profile's login request returned a non-2xx response. Check credentials and the profile's login endpoint.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrProfileNoExtract",
			Err:  ErrProfileNoExtract,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAuth,
				Code:     "AUTH_PROFILE_NO_EXTRACT",
				Hint:     "The auth profile's extract path did not match the login response body. Verify the path against the actual response shape.",
			},
		},
		// Cache errors are transient, auto-handled internals; classified but without hints.
		apierrors.RegisteredError{
			Name: "ErrCacheExpired",
			Err:  ErrCacheExpired,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInternal, Code: "AUTH_CACHE_EXPIRED"},
		},
		apierrors.RegisteredError{
			Name: "ErrCacheMiss",
			Err:  ErrCacheMiss,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInternal, Code: "AUTH_CACHE_MISS"},
		},
		apierrors.RegisteredError{
			Name: "ErrCacheCorrupted",
			Err:  ErrCacheCorrupted,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAuth,
				Code:     "AUTH_CACHE_CORRUPTED",
				Hint:     "Delete the auth cache directory and re-run to force re-authentication.",
			},
		},
	)
}
