package scaffold

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("scaffold",
		apierrors.RegisteredError{
			Name: "ErrProjectExists",
			Err:  ErrProjectExists,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "SCAFFOLD_PROJECT_EXISTS",
				Hint:     "Run init in an empty directory, or pass --force to overwrite.",
			},
		},
	)
}
