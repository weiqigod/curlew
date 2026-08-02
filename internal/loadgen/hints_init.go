package loadgen

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("loadgen",
		apierrors.RegisteredError{
			Name: "ErrInvalidVUs",
			Err:  ErrInvalidVUs,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInput, Code: "LOADGEN_INVALID_VUS", Hint: "Pass --vus with a positive integer (>= 1)."},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidDuration",
			Err:  ErrInvalidDuration,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInput, Code: "LOADGEN_INVALID_DURATION", Hint: "Pass --duration with a positive Go duration (e.g. 30s, 2m)."},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidRampUp",
			Err:  ErrInvalidRampUp,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInput, Code: "LOADGEN_INVALID_RAMP_UP", Hint: "--ramp-up must be non-negative and not exceed --duration."},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidRPS",
			Err:  ErrInvalidRPS,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInput, Code: "LOADGEN_INVALID_RPS", Hint: "--rps must be non-negative. Omit the flag to run open-loop."},
		},
		apierrors.RegisteredError{
			Name: "ErrRequestFileNotFound",
			Err:  ErrRequestFileNotFound,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInput, Code: "LOADGEN_REQUEST_FILE_NOT_FOUND", Hint: "Verify the --request path. File must exist and be readable."},
		},
	)
}
