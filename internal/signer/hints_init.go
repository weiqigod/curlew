package signer

import (
	apierrors "github.com/weiqigod/curlew/internal/errors"
)

func init() {
	apierrors.RegisterPackage("signer",
		apierrors.RegisteredError{
			Name: "ErrUnknownSignerType",
			Err:  ErrUnknownSignerType,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "UNKNOWN_SIGNER_TYPE",
				Hint:     "Check the signer type name in your signing: field. Available types are listed in the error message. Built-in signers (aws-sigv4, oauth1) ship in M17-002 and M17-003.",
			},
		},
	)
}
