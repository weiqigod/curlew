package hooks

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("plugin/hooks",
		apierrors.RegisteredError{
			Name: "ErrHookAborted",
			Err:  ErrHookAborted,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "PLUGIN_HOOK_ABORTED",
				Hint:     "A plugin on_request hook aborted the request. Check the plugin's stderr for details.",
			},
		},
	)
}
