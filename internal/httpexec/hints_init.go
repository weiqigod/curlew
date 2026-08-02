package httpexec

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("httpexec",
		apierrors.RegisteredError{
			Name: "ErrNetwork",
			Err:  ErrNetwork,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryNetwork,
				Code:     "NETWORK_OTHER",
				Hint:     "Check network connectivity to the target host and that the server is reachable.",
			},
		},
	)
}
