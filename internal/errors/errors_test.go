package errors

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestStructuredError(t *testing.T) {
	tests := []struct {
		name string
		err  *Structured
		want string
	}{
		{"with file and line", &Structured{FilePath: "test.yaml", Line: 5, Message: "bad syntax"}, "test.yaml:5: bad syntax"},
		{"with file no line", &Structured{FilePath: "test.yaml", Line: 0, Message: "empty"}, "test.yaml: empty"},
		{"no file", &Structured{Message: "something failed"}, "something failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStructuredUnwrap(t *testing.T) {
	inner := errors.New("inner error")
	err := &Structured{Message: "outer", Inner: inner}
	if !errors.Is(err, inner) {
		t.Error("errors.Is should find inner error through Unwrap")
	}
}

func TestNetworkErrorError(t *testing.T) {
	err := &NetworkError{Message: "DNS resolution failed for bad.invalid"}
	got := err.Error()
	if got != "DNS resolution failed for bad.invalid" {
		t.Errorf("Error() = %q, want %q", got, "DNS resolution failed for bad.invalid")
	}
}

func TestClassifyNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantKind NetworkErrorKind
	}{
		{
			"dns failure",
			&net.DNSError{Err: "no such host", Name: "bad.invalid"},
			NetworkDNS,
		},
		{
			"connection refused",
			&net.OpError{Op: "dial", Err: &net.OpError{Op: "dial", Err: errors.New("connect: connection refused")}},
			NetworkConnectionRefused,
		},
		{
			"Windows connection actively refused",
			errors.New("connectex: No connection could be made because the target machine actively refused it"),
			NetworkConnectionRefused,
		},
		{
			"tls certificate error",
			&x509.UnknownAuthorityError{},
			NetworkTLS,
		},
		{
			"timeout - context deadline",
			context.DeadlineExceeded,
			NetworkTimeout,
		},
		{
			"other network error",
			errors.New("unknown network failure"),
			NetworkOther,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyNetworkError(tt.err)
			if result.Kind != tt.wantKind {
				t.Errorf("Kind = %v, want %v", result.Kind, tt.wantKind)
			}
		})
	}
}

func TestNetworkErrorHints(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantHint string
	}{
		{"dns hint", &net.DNSError{Err: "no such host", Name: "bad.invalid"}, "Check that the hostname is correct"},
		{"connection refused hint", &net.OpError{Op: "dial", Err: errors.New("connect: connection refused")}, "server is running"},
		{"tls hint", &x509.UnknownAuthorityError{}, "certificate"},
		{"timeout hint", context.DeadlineExceeded, "increasing the timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyNetworkError(tt.err)
			if !strings.Contains(result.Hint, tt.wantHint) {
				t.Errorf("Hint = %q, want to contain %q", result.Hint, tt.wantHint)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			"structured with file and line",
			&Structured{FilePath: "t.yaml", Line: 5, Message: "bad"},
			"[ERROR] t.yaml:5 — bad",
		},
		{
			"structured with hint",
			&Structured{FilePath: "t.yaml", Line: 5, Message: "bad", Hint: "fix it"},
			"[ERROR] t.yaml:5 — bad\n  Hint: fix it",
		},
		{
			"structured with file no line",
			&Structured{FilePath: "t.yaml", Message: "bad"},
			"[ERROR] t.yaml — bad",
		},
		{
			"plain error",
			errors.New("boom"),
			"[ERROR] boom",
		},
		{
			"network error with hint",
			&NetworkError{Message: "DNS failed", Hint: "check host"},
			"[ERROR] DNS failed\n  Hint: check host",
		},
		{
			"network error no hint",
			&NetworkError{Message: "something"},
			"[ERROR] something",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(tt.err)
			if got != tt.want {
				t.Errorf("Format() = %q, want %q", got, tt.want)
			}
		})
	}
}
