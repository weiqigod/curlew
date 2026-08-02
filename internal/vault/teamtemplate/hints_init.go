package teamtemplate

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("vault/teamtemplate",
		apierrors.RegisteredError{
			Name: "ErrInvalidTemplate",
			Err:  ErrInvalidTemplate,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "VAULT_TEMPLATE_INVALID",
				Hint:     "Validate the team template YAML against the documented schema.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrUnknownProvider",
			Err:  ErrUnknownProvider,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "VAULT_TEMPLATE_UNKNOWN_PROVIDER",
				Hint:     "Use a supported provider name (aws, gcp, vault, 1password, env, file).",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrMissingField",
			Err:  ErrMissingField,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "VAULT_TEMPLATE_MISSING_FIELD",
				Hint:     "Add the missing field named in the message to the team template.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrDuplicateAlias",
			Err:  ErrDuplicateAlias,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "VAULT_TEMPLATE_DUPLICATE_ALIAS",
				Hint:     "Remove or rename one of the duplicate alias entries.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidKeyRef",
			Err:  ErrInvalidKeyRef,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "VAULT_TEMPLATE_INVALID_KEY_REF",
				Hint:     "Check the key: reference against the provider's required format.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrTemplateNotFound",
			Err:  ErrTemplateNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "VAULT_TEMPLATE_NOT_FOUND",
				Hint:     "The shared vault template file was not found. Confirm its path relative to the project root.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrUnknownEnvironment",
			Err:  ErrUnknownEnvironment,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "VAULT_TEMPLATE_UNKNOWN_ENV",
				Hint:     "Pass --env to a value defined under environments: in the team template.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrUnknownAlias",
			Err:  ErrUnknownAlias,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "VAULT_TEMPLATE_UNKNOWN_ALIAS",
				Hint:     "The alias is referenced but not declared in the team template. Add it under aliases: or rename the reference.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrEnvFlagRequired",
			Err:  ErrEnvFlagRequired,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "VAULT_TEMPLATE_ENV_FLAG_REQUIRED",
				Hint:     "Pass --env <name> to select an environment from the shared vault template.",
			},
		},
	)
}
