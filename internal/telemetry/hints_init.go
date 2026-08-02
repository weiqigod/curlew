package telemetry

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("telemetry",
		apierrors.RegisteredError{
			Name: "ErrNotEnabled",
			Err:  ErrNotEnabled,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "TELEMETRY_NOT_ENABLED",
				Hint:     "Run `apitest telemetry enable` to generate a persistent install_id and opt in to telemetry.",
			},
		},
	)
}
