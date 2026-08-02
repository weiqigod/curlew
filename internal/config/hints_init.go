package config

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("config",
		apierrors.RegisteredError{
			Name: "ErrInvalidProjectConfig",
			Err:  ErrInvalidProjectConfig,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "CONFIG_INVALID_PROJECT",
				Hint:     "Validate curlew.yaml against the documented project config schema.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidDotenv",
			Err:  ErrInvalidDotenv,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "CONFIG_INVALID_DOTENV",
				Hint:     "Fix the .env file: each line must be blank, a comment, or KEY=VALUE.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrEnvironmentNotFound",
			Err:  ErrEnvironmentNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "CONFIG_ENV_NOT_FOUND",
				Hint:     "Create environments/<name>.yaml or pass --env to an existing environment.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidEnvironment",
			Err:  ErrInvalidEnvironment,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "CONFIG_INVALID_ENV",
				Hint:     "The environment file is malformed. Ensure variables: is a map of string values.",
			},
		},
	)
}
