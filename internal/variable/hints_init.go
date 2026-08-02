package variable

import (
	"strings"

	apierrors "github.com/weiqigod/curlew/internal/errors"
)

func init() {
	apierrors.RegisterPackage("variable",
		apierrors.RegisteredError{
			Name: "ErrCircularReference",
			Err:  ErrCircularReference,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "VAR_CIRCULAR_REFERENCE",
				Hint:     "Break the cycle: a variable value must not transitively reference itself via {{...}}.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrDepthExceeded",
			Err:  ErrDepthExceeded,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "VAR_DEPTH_EXCEEDED",
				Hint:     "Reduce nested variable interpolation depth or refactor to fewer levels.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrUndefinedVariable",
			Err:  ErrUndefinedVariable,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "VAR_UNDEFINED",
				Hint:     "Define the variable in the environment file, pass it via --var NAME=VALUE, or add a default in the collection.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidVarFlag",
			Err:  ErrInvalidVarFlag,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "VAR_INVALID_FLAG",
				Hint:     "Use the format --var NAME=VALUE (no spaces around =).",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidEnvVarFlag",
			Err:  ErrInvalidEnvVarFlag,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "VAR_INVALID_ENV_VAR_FLAG",
				Hint:     "Use the format --env-var NAME (to pass through) or --env-var NAME=VALUE.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrEnvVarNotSet",
			Err:  ErrEnvVarNotSet,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "VAR_ENV_NOT_SET",
				Hint:     "Set the required environment variable in your shell before running, or pass --var NAME=VALUE.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrUnknownSecret",
			Err:  ErrUnknownSecret,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAuth,
				Code:     "VAR_UNKNOWN_SECRET",
				Hint:     "Declare the secret alias in vault config or the team template before referencing it.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrExtractionFailed",
			Err:  ErrExtractionFailed,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAssertion,
				Code:     "VAR_EXTRACTION_FAILED",
				Hint:     "Check that the extract path matches the response shape. Use --format jsonl to inspect the raw body.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrNotJSON",
			Err:  ErrNotJSON,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAssertion,
				Code:     "VAR_RESPONSE_NOT_JSON",
				Hint:     "JSON extraction requires a JSON response. Check the response Content-Type and body.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrCommandFailed",
			Err:  ErrCommandFailed,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "VAR_COMMAND_FAILED",
				Hint:     "Verify the from_command: command exists on PATH and runs successfully.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrLocaleUnknown",
			Err:  ErrLocaleUnknown,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "ERR_LOCALE_UNKNOWN",
				Hint:     "Supported locales: " + strings.Join(supportedLocales, ", "),
			},
		},
	)
}
