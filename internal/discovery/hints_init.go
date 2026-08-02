package discovery

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("discovery",
		apierrors.RegisteredError{
			Name: "ErrNoMatches",
			Err:  ErrNoMatches,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "DISCOVERY_NO_MATCHES",
				Hint:     "Check the glob pattern. Quote it in the shell to prevent expansion: curlew run 'tests/**/*.yaml'.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrTraversalOutsideRoot",
			Err:  ErrTraversalOutsideRoot,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "DISCOVERY_TRAVERSAL_OUTSIDE_ROOT",
				Hint:     "Remove ../ segments. Patterns must resolve within the working directory.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrAbsolutePattern",
			Err:  ErrAbsolutePattern,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "DISCOVERY_ABSOLUTE_PATTERN",
				Hint:     "Use a relative pattern. Absolute glob patterns are not supported.",
			},
		},
	)
}
