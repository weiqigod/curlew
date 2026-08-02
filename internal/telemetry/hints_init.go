package telemetry

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("telemetry",
		apierrors.RegisteredError{
			Name: "ErrNotEnabled",
			Err:  ErrNotEnabled,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "TELEMETRY_NOT_ENABLED",
				Hint:     "Run `curlew telemetry enable` to generate a persistent install_id and opt in to telemetry.",
			},
		},
	)
}
