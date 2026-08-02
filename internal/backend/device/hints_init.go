package device

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("backend/device",
		apierrors.RegisteredError{
			Name: "ErrNotFound",
			Err:  ErrNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAuth,
				Code:     "BACKEND_DEVICE_NOT_FOUND",
				Hint:     "No device identity found. Authenticate via curlew login.",
			},
		},
	)
}
