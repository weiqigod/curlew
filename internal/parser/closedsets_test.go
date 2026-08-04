package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apierrors "github.com/weiqigod/curlew/internal/errors"
)

// The parser validates four fields against closed sets of values, and the same
// four sets are duplicated in schemas/collection-v1.json as enums and again in
// the error hints users see when they get one wrong. These tests pin the
// duplicates to the parser's own lists so a value added in one place cannot be
// silently missing from another.

// parseCollection writes src to a temp collection file and parses it.
func parseCollection(t *testing.T, src string) (*Collection, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "col.yaml")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write collection: %v", err)
	}
	return ParseFile(path)
}

// TestParser_supported_protocols_are_accepted proves SupportedProtocols is the
// real accepted set rather than a list that merely looks right: every value in
// it parses, and a value outside it is rejected.
func TestParser_supported_protocols_are_accepted(t *testing.T) {
	bodyFor := map[string]string{
		"http":      "",
		"graphql":   "      graphql:\n        query: \"{ me { id } }\"\n",
		"websocket": "      websocket:\n        steps:\n          - action: close\n",
	}
	for _, proto := range SupportedProtocols {
		t.Run(proto, func(t *testing.T) {
			extra, ok := bodyFor[proto]
			if !ok {
				t.Fatalf("no fixture body for protocol %q — SupportedProtocols gained a value this test does not exercise", proto)
			}
			src := fmt.Sprintf("name: T\nrequests:\n  - name: R\n    request:\n      url: \"https://example.com/\"\n      protocol: %s\n%s", proto, extra)
			if _, err := parseCollection(t, src); err != nil {
				t.Errorf("protocol %q is in SupportedProtocols but was rejected: %v", proto, err)
			}
		})
	}

	t.Run("error case - protocol outside the set", func(t *testing.T) {
		src := "name: T\nrequests:\n  - name: R\n    request:\n      url: \"https://example.com/\"\n      protocol: grpc\n"
		if _, err := parseCollection(t, src); err == nil {
			t.Fatal("expected an error for protocol: grpc, got nil")
		}
	})
}

// plausibleButRejectedProtocols are values a reader could reasonably expect to
// find in a protocol list and which the parser rejects. `https` was in the hint
// for exactly that reason: it reads naturally and is wrong.
var plausibleButRejectedProtocols = []string{"https", "http2", "ws", "wss", "grpc", "tcp", "rest", "soap"}

// TestParser_protocol_hint_lists_only_accepted_values guards the failure mode
// where a user follows the hint and hits the same error again: the registered
// hint for ErrUnsupportedProtocol must name every accepted protocol and none of
// the plausible-looking values the parser turns down.
func TestParser_protocol_hint_lists_only_accepted_values(t *testing.T) {
	hint := registeredHint(t, ErrUnsupportedProtocol)
	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(hint, func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == ':' || r == '"'
	}) {
		words[w] = true
	}

	for _, proto := range SupportedProtocols {
		if !words[proto] {
			t.Errorf("hint %q omits accepted protocol %q", hint, proto)
		}
	}

	accepted := map[string]bool{}
	for _, p := range SupportedProtocols {
		accepted[p] = true
	}
	for _, bad := range plausibleButRejectedProtocols {
		if accepted[bad] {
			continue // it became supported; nothing to guard
		}
		if words[bad] {
			t.Errorf("hint %q offers %q, which the parser rejects — a user who follows it hits the same error", hint, bad)
		}
	}
}

// TestParser_graphql_error_handling_values pins GraphQLErrorHandlingValues: all
// three parse, and a fourth does not. The struct comment on GraphQLConfig and
// the CLI specification both claimed only two.
func TestParser_graphql_error_handling_values(t *testing.T) {
	for _, eh := range GraphQLErrorHandlingValues {
		t.Run(eh, func(t *testing.T) {
			src := fmt.Sprintf("name: T\nrequests:\n  - name: R\n    request:\n      url: \"https://example.com/graphql\"\n      protocol: graphql\n      graphql:\n        query: \"{ me { id } }\"\n        error_handling: %s\n", eh)
			if _, err := parseCollection(t, src); err != nil {
				t.Errorf("error_handling %q is in GraphQLErrorHandlingValues but was rejected: %v", eh, err)
			}
		})
	}

	t.Run("error case - value outside the set", func(t *testing.T) {
		src := "name: T\nrequests:\n  - name: R\n    request:\n      url: \"https://example.com/graphql\"\n      protocol: graphql\n      graphql:\n        query: \"{ me { id } }\"\n        error_handling: explode\n"
		if _, err := parseCollection(t, src); err == nil {
			t.Fatal("expected an error for error_handling: explode, got nil")
		}
	})
}

// TestParser_websocket_closed_sets pins WebSocketActions and
// WebSocketBackoffStrategies the same way.
func TestParser_websocket_closed_sets(t *testing.T) {
	for _, action := range WebSocketActions {
		t.Run("action_"+action, func(t *testing.T) {
			src := fmt.Sprintf("name: T\nrequests:\n  - name: R\n    request:\n      url: \"wss://example.com/s\"\n      protocol: websocket\n      websocket:\n        steps:\n          - action: %s\n", action)
			if _, err := parseCollection(t, src); err != nil {
				t.Errorf("action %q is in WebSocketActions but was rejected: %v", action, err)
			}
		})
	}

	for _, backoff := range WebSocketBackoffStrategies {
		t.Run("backoff_"+backoff, func(t *testing.T) {
			src := fmt.Sprintf("name: T\nrequests:\n  - name: R\n    request:\n      url: \"wss://example.com/s\"\n      protocol: websocket\n      websocket:\n        steps:\n          - action: close\n        reconnect:\n          enabled: true\n          backoff: %s\n", backoff)
			if _, err := parseCollection(t, src); err != nil {
				t.Errorf("backoff %q is in WebSocketBackoffStrategies but was rejected: %v", backoff, err)
			}
		})
	}

	t.Run("error case - action outside the set", func(t *testing.T) {
		src := "name: T\nrequests:\n  - name: R\n    request:\n      url: \"wss://example.com/s\"\n      protocol: websocket\n      websocket:\n        steps:\n          - action: yell\n"
		if _, err := parseCollection(t, src); err == nil {
			t.Fatal("expected an error for action: yell, got nil")
		}
	})

	t.Run("error case - backoff outside the set", func(t *testing.T) {
		src := "name: T\nrequests:\n  - name: R\n    request:\n      url: \"wss://example.com/s\"\n      protocol: websocket\n      websocket:\n        steps:\n          - action: close\n        reconnect:\n          enabled: true\n          backoff: linear\n"
		if _, err := parseCollection(t, src); err == nil {
			t.Fatal("expected an error for backoff: linear, got nil")
		}
	})
}

// registeredHint returns the hint text the error registry serves for a sentinel.
func registeredHint(t *testing.T, err error) string {
	t.Helper()
	hint, ok := apierrors.LookupHint(err)
	if !ok {
		t.Fatalf("no hint registered for %v", err)
	}
	return hint.Hint
}
