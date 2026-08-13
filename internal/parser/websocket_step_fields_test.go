package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// §11C.11 and its class. A `wait` step reads `duration_ms`; `timeout_ms` sets a
// field runWait never looks at. The parser accepted both for every action and
// ignored unknown keys entirely, so a step could be written wrong in a way that
// produced no error and no behaviour — the documented example in
// CLI_SPECIFICATION §12.3 paused for zero milliseconds, and survived being
// written down twice.
//
// A field an action ignores is a mistake, not a no-op, and the parser now says
// so. That is also what lets a documentation test catch this class at all: an
// example that parses cleanly can still be wrong, but one the parser rejects
// cannot hide.

func parseStepYAML(t *testing.T, step string) error {
	t.Helper()
	body := "name: t\nrequests:\n  - name: r\n    request:\n" +
		"      protocol: websocket\n      url: \"ws://example.test/s\"\n" +
		"      websocket:\n        steps:\n" + step
	file := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ParseFile(file)
	return err
}

func TestWebSocketStep_rejectsFieldTheActionIgnores(t *testing.T) {
	tests := []struct {
		name     string
		step     string
		wantMent []string // substrings the message must contain
	}{
		{
			name:     "wait with timeout_ms",
			step:     "          - action: wait\n            timeout_ms: 1000\n",
			wantMent: []string{"timeout_ms", "wait", "duration_ms"},
		},
		{
			name:     "send with duration_ms",
			step:     "          - action: send\n            message: { a: 1 }\n            duration_ms: 10\n",
			wantMent: []string{"duration_ms", "send"},
		},
		{
			name:     "close with count",
			step:     "          - action: close\n            count: 3\n",
			wantMent: []string{"count", "close"},
		},
		{
			name:     "expect with reason",
			step:     "          - action: expect\n            timeout_ms: 10\n            reason: nope\n",
			wantMent: []string{"reason", "expect"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parseStepYAML(t, tt.step)
			if err == nil {
				t.Fatal("accepted a field the action ignores; it would silently do nothing")
			}
			msg := err.Error()
			for _, want := range tt.wantMent {
				if !strings.Contains(msg, want) {
					t.Errorf("message does not mention %q: %s", want, msg)
				}
			}
		})
	}
}

func TestWebSocketStep_rejectsUnknownField(t *testing.T) {
	err := parseStepYAML(t, "          - action: wait\n            duration_ms: 10\n            durationMs: 10\n")
	if err == nil {
		t.Fatal("an unknown step field was ignored silently")
	}
	if !strings.Contains(err.Error(), "durationMs") {
		t.Errorf("message does not name the unknown field: %v", err)
	}
}

func TestWebSocketStep_acceptsEveryDocumentedCombination(t *testing.T) {
	// The other half: the check must not reject what the documents describe.
	steps := []string{
		"          - action: send\n            message: { type: subscribe }\n",
		"          - action: send\n            message_raw: \"PING\"\n",
		"          - action: expect\n            timeout_ms: 5000\n            count: 2\n            message:\n              $.type: { equals: ok }\n            extract:\n              ids: \"$.id\"\n",
		"          - action: expect\n            any_of:\n              - message:\n                  $.type: { equals: a }\n",
		"          - action: wait\n            duration_ms: 500\n",
		"          - action: close\n            code: 1000\n            reason: done\n",
	}
	for _, s := range steps {
		if err := parseStepYAML(t, s); err != nil {
			t.Errorf("rejected a documented step:\n%s  error: %v", s, err)
		}
	}
}
