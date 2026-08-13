package websocket

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("websocket",
		apierrors.RegisteredError{
			Name: "ErrDialFailed",
			Err:  ErrDialFailed,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryNetwork,
				Code:     "WS_DIAL_FAILED",
				Hint:     "The WebSocket handshake failed. Check the URL scheme (ws/wss), TLS settings, and that the server is reachable.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrUnknownAction",
			Err:  ErrUnknownAction,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "WS_UNKNOWN_ACTION",
				Hint:     "Use a supported step action: send, expect, close, wait.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrExpectTimeout",
			Err:  ErrExpectTimeout,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAssertion,
				Code:     "WS_EXPECT_TIMEOUT",
				Hint:     "No matching message arrived before timeout: raise the timeout, or verify the server sends the expected payload.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrSendFailed",
			Err:  ErrSendFailed,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryNetwork,
				Code:     "WS_SEND_FAILED",
				Hint:     "Write to the WebSocket failed. The connection may have closed unexpectedly.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrCloseFailed",
			Err:  ErrCloseFailed,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryNetwork,
				Code:     "WS_CLOSE_FAILED",
				Hint:     "Sending the close frame failed. The server may have terminated the connection first.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrWaitFailed",
			Err:  ErrWaitFailed,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryNetwork,
				Code:     "WS_WAIT_FAILED",
				Hint:     "The connection broke while a wait step was holding it open. A wait reads the connection rather than sleeping, so a peer that disconnects mid-wait is reported here instead of surfacing on the next step.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrExtractFailed",
			Err:  ErrExtractFailed,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAssertion,
				Code:     "WS_EXTRACT_FAILED",
				Hint:     "The extract path did not match the received message. Verify the path against the actual payload shape.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrExpectAssertionVars",
			Err:  ErrExpectAssertionVars,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAssertion,
				Code:     "WS_EXPECT_ASSERTION_VARS",
				Hint:     "An expect assertion referenced a variable that is not defined. Check the name, or extract it in an earlier step.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrContextCanceled",
			Err:  ErrContextCanceled,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInternal, Code: "WS_CONTEXT_CANCELED"},
		},
		apierrors.RegisteredError{
			Name: "ErrNoSteps",
			Err:  ErrNoSteps,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "WS_NO_STEPS",
				Hint:     "A websocket request must declare at least one step under steps:.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrReconnectExhausted",
			Err:  ErrReconnectExhausted,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryNetwork,
				Code:     "WS_RECONNECT_EXHAUSTED",
				Hint:     "Reconnection attempts were exhausted. Increase max_reconnects or investigate why the server keeps dropping the connection.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrHeartbeatTimeout",
			Err:  ErrHeartbeatTimeout,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryNetwork,
				Code:     "WS_HEARTBEAT_TIMEOUT",
				Hint:     "The server did not respond to a heartbeat in time. Raise the heartbeat timeout or verify the server is alive.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrHeartbeatFailed",
			Err:  ErrHeartbeatFailed,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryNetwork,
				Code:     "WS_HEARTBEAT_FAILED",
				Hint:     "Sending the heartbeat failed. The connection may have dropped.",
			},
		},
	)
}
