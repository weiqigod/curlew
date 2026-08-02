package templates

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("websocket/templates",
		apierrors.RegisteredError{
			Name: "ErrTemplateNotFound",
			Err:  ErrTemplateNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "WS_TEMPLATE_NOT_FOUND",
				Hint:     "The referenced websocket message template is not defined. Declare it under templates: or correct the template name.",
			},
		},
	)
}
