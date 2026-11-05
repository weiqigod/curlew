// Package errors provides structured error types with location context,
// network error classification, and consistent formatting for user-facing output.
package errors

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Category classifies error types for consistent formatting.
type Category string

const (
	CategoryParse     Category = "parse"
	CategoryNetwork   Category = "network"
	CategoryConfig    Category = "config"
	CategoryAssertion Category = "assertion"
	CategoryAuth      Category = "auth"
	CategoryInput     Category = "input"
	CategoryInternal  Category = "internal"
)

// Structured carries location context for user-facing error display.
type Structured struct {
	Category Category
	FilePath string
	Line     int // 0 = unknown
	Message  string
	Hint     string // optional fix suggestion
	Code     string // stable machine-readable identifier (e.g. "PARSE_INVALID_YAML"); empty if not classified
	Inner    error  // for errors.Is/As chain
}

func (e *Structured) Error() string {
	if e.FilePath != "" && e.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", e.FilePath, e.Line, e.Message)
	}
	if e.FilePath != "" {
		return fmt.Sprintf("%s: %s", e.FilePath, e.Message)
	}
	return e.Message
}

// Unwrap returns the inner error for errors.Is/As traversal.
func (e *Structured) Unwrap() error { return e.Inner }

// NetworkErrorKind classifies network-level failures.
type NetworkErrorKind int

const (
	NetworkDNS NetworkErrorKind = iota
	NetworkConnectionRefused
	NetworkTLS
	NetworkTimeout
	NetworkOther
)

// NetworkError is a classified network failure.
type NetworkError struct {
	Kind     NetworkErrorKind
	Host     string
	Duration time.Duration // meaningful for timeouts
	Message  string
	Hint     string
	Inner    error
}

func (e *NetworkError) Error() string { return e.Message }

// Unwrap returns the inner error for errors.Is/As traversal.
func (e *NetworkError) Unwrap() error { return e.Inner }

// ClassifyNetworkError inspects a Go net/http error chain and returns
// a classified NetworkError with user-facing message and hint.
func ClassifyNetworkError(err error) *NetworkError {
	// DNS resolution failure
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return &NetworkError{
			Kind:    NetworkDNS,
			Host:    dnsErr.Name,
			Message: fmt.Sprintf("DNS resolution failed for %s", dnsErr.Name),
			Hint:    "Check that the hostname is correct and DNS is configured",
			Inner:   err,
		}
	}

	// TLS certificate errors
	var unknownAuthErr *x509.UnknownAuthorityError
	var certInvalidErr *x509.CertificateInvalidError
	if errors.As(err, &unknownAuthErr) || errors.As(err, &certInvalidErr) {
		return &NetworkError{
			Kind:    NetworkTLS,
			Message: fmt.Sprintf("TLS certificate error: %s", err),
			Hint:    "Verify the server's certificate is valid and trusted",
			Inner:   err,
		}
	}

	// Timeout
	if errors.Is(err, context.DeadlineExceeded) {
		return &NetworkError{
			Kind:    NetworkTimeout,
			Message: "request timed out",
			Hint:    "Consider increasing the timeout duration",
			Inner:   err,
		}
	}

	// Connection refused — check error string since syscall errors vary by OS
	if isConnectionRefused(err) {
		host := extractHost(err)
		msg := "Connection refused"
		if host != "" {
			msg = fmt.Sprintf("Connection refused at %s", host)
		}
		return &NetworkError{
			Kind:    NetworkConnectionRefused,
			Host:    host,
			Message: msg,
			Hint:    "Check that the server is running and listening on this port",
			Inner:   err,
		}
	}

	// Fallback
	return &NetworkError{
		Kind:    NetworkOther,
		Message: err.Error(),
		Inner:   err,
	}
}

// Format returns a display string for any error.
// Structured: "[ERROR] file:line — message\n  Hint: ..."
// NetworkError: "[ERROR] message\n  Hint: ..."
// Plain: "[ERROR] message"
func Format(err error) string {
	var b strings.Builder

	var se *Structured
	var ne *NetworkError

	switch {
	case errors.As(err, &se):
		b.WriteString("[ERROR] ")
		if se.FilePath != "" && se.Line > 0 {
			fmt.Fprintf(&b, "%s:%d", se.FilePath, se.Line)
		} else if se.FilePath != "" {
			b.WriteString(se.FilePath)
		}
		if se.FilePath != "" {
			b.WriteString(" — ")
		}
		b.WriteString(se.Message)
		if se.Hint != "" {
			fmt.Fprintf(&b, "\n  Hint: %s", se.Hint)
		}

	case errors.As(err, &ne):
		fmt.Fprintf(&b, "[ERROR] %s", ne.Message)
		if ne.Hint != "" {
			fmt.Fprintf(&b, "\n  Hint: %s", ne.Hint)
		}

	default:
		fmt.Fprintf(&b, "[ERROR] %s", err.Error())
	}

	return b.String()
}

func isConnectionRefused(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "connection refused") || strings.Contains(message, "actively refused")
}

func extractHost(err error) string {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Addr != nil {
			return opErr.Addr.String()
		}
	}
	return ""
}
