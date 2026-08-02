// Package websocket provides WebSocket protocol support for the API testing
// tool. It owns the connection lifecycle (dial -> send/expect/wait/close) and
// integrates with variable interpolation, JSONPath assertions, and variable
// extraction. The runner delegates WebSocket execution to this package via
// the Dialer interface, which makes the executor fully unit-testable without
// a real network.
package websocket

import "errors"

// Sentinel errors for WebSocket execution failures.
var (
	// ErrDialFailed indicates the underlying WebSocket handshake failed.
	ErrDialFailed = errors.New("websocket dial failed")
	// ErrUnknownAction indicates a WebSocketStep specified an unsupported action.
	ErrUnknownAction = errors.New("unknown websocket action")
	// ErrExpectTimeout indicates an expect step timed out waiting for a matching message.
	ErrExpectTimeout = errors.New("expect timed out")
	// ErrSendFailed indicates a send step failed to write a frame to the peer.
	ErrSendFailed = errors.New("websocket send failed")
	// ErrCloseFailed indicates a close step failed to write the close control frame.
	ErrCloseFailed = errors.New("websocket close failed")
	// ErrExtractFailed indicates variable extraction from an expect match failed.
	ErrExtractFailed = errors.New("websocket expect extract failed")
	// ErrContextCanceled indicates the caller canceled the context mid-step.
	ErrContextCanceled = errors.New("websocket context canceled")
	// ErrNoSteps indicates a WebSocket request was constructed with no steps.
	// Parser validation also rejects this case; this is the defensive guard
	// for callers that construct a parser.Request directly.
	ErrNoSteps = errors.New("websocket request has no steps")
	// ErrReconnectExhausted indicates that reconnection was attempted the
	// maximum number of times without success.
	ErrReconnectExhausted = errors.New("websocket reconnect exhausted")
	// ErrHeartbeatTimeout indicates that no pong was received within the
	// heartbeat interval after a ping was sent.
	ErrHeartbeatTimeout = errors.New("websocket heartbeat timeout")
	// ErrHeartbeatFailed indicates a failure writing the heartbeat ping.
	ErrHeartbeatFailed = errors.New("websocket heartbeat failed")
)
