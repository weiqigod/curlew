package vault

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("vault",
		apierrors.RegisteredError{
			Name: "ErrFieldNotFound",
			Err:  ErrFieldNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAuth,
				Code:     "VAULT_FIELD_NOT_FOUND",
				Hint:     "The field name does not exist in the JSON secret. Verify the field against the secret's actual shape.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrNotJSONSecret",
			Err:  ErrNotJSONSecret,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAuth,
				Code:     "VAULT_NOT_JSON_SECRET",
				Hint:     "Field extraction requires a JSON secret. Remove the field: or store the secret as JSON.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrSecretNotFound",
			Err:  ErrSecretNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAuth,
				Code:     "VAULT_SECRET_NOT_FOUND",
				Hint:     "Verify the secret name exists in the vault provider and that credentials have read access.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrProviderAuth",
			Err:  ErrProviderAuth,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAuth,
				Code:     "VAULT_PROVIDER_AUTH",
				Hint:     "Vault provider authentication failed. Check credentials and that the token has not expired.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrUnknownProvider",
			Err:  ErrUnknownProvider,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "VAULT_UNKNOWN_PROVIDER",
				Hint:     "Set provider: to a supported value (aws, gcp, vault, 1password, env, file).",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrMissingRequiredField",
			Err:  ErrMissingRequiredField,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "VAULT_MISSING_REQUIRED_FIELD",
				Hint:     "Add the missing vault config field named in the message.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidKeyFormat",
			Err:  ErrInvalidKeyFormat,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "VAULT_INVALID_KEY_FORMAT",
				Hint:     "Format the vault key per the provider's schema (e.g. projects/<proj>/secrets/<name>/versions/latest for GCP).",
			},
		},
	)
}
