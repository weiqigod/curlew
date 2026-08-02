package plugin

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("plugin",
		apierrors.RegisteredError{
			Name: "ErrHandshakeTimeout",
			Err:  ErrHandshakeTimeout,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInternal, Code: "PLUGIN_HANDSHAKE_TIMEOUT", Hint: "The plugin did not complete its handshake in time. Check the plugin's startup logs."},
		},
		apierrors.RegisteredError{
			Name: "ErrDuplicateName",
			Err:  ErrDuplicateName,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryConfig, Code: "PLUGIN_DUPLICATE_NAME", Hint: "Two plugins share the same name. Rename one in the plugin manifest."},
		},
		apierrors.RegisteredError{
			Name: "ErrNotExecutable",
			Err:  ErrNotExecutable,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryConfig, Code: "PLUGIN_NOT_EXECUTABLE", Hint: "Ensure the plugin binary exists and has the executable bit set (chmod +x)."},
		},
		apierrors.RegisteredError{
			Name: "ErrHandshakeProtocol",
			Err:  ErrHandshakeProtocol,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInternal, Code: "PLUGIN_HANDSHAKE_PROTOCOL", Hint: "The plugin responded with an unexpected handshake. Verify the plugin targets a compatible curlew version."},
		},
		apierrors.RegisteredError{
			Name: "ErrCallTimeout",
			Err:  ErrCallTimeout,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInternal, Code: "PLUGIN_CALL_TIMEOUT", Hint: "A plugin RPC did not return in time. Check the plugin's logs for hangs."},
		},
		apierrors.RegisteredError{
			Name: "ErrCallPluginError",
			Err:  ErrCallPluginError,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInternal, Code: "PLUGIN_CALL_ERROR", Hint: "The plugin returned an error for the call. Check the plugin's stderr."},
		},
		apierrors.RegisteredError{
			Name: "ErrChannelClosed",
			Err:  ErrChannelClosed,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInternal, Code: "PLUGIN_CHANNEL_CLOSED", Hint: "The plugin channel closed unexpectedly. The plugin may have crashed."},
		},
	)
}
