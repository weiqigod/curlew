package plugin

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestParseHello(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  error
		wantName string
		wantPV   int
	}{
		{
			name:     "full response",
			input:    `{"name":"hello","version":"0.1","hooks":["on_request"],"protocol_version":1}`,
			wantErr:  nil,
			wantName: "hello",
			wantPV:   1,
		},
		{
			name:     "missing protocol_version defaults to 1",
			input:    `{"name":"hello","version":"0.1","hooks":[]}`,
			wantErr:  nil,
			wantName: "hello",
			wantPV:   1,
		},
		{
			name:    "empty name",
			input:   `{"name":"","version":"0.1","hooks":[]}`,
			wantErr: ErrHandshakeProtocol,
		},
		{
			name:    "empty version",
			input:   `{"name":"x","version":"","hooks":[]}`,
			wantErr: ErrHandshakeProtocol,
		},
		{
			name:    "malformed json",
			input:   `{"name":`,
			wantErr: ErrHandshakeProtocol,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, err := parseHello(json.RawMessage(tc.input))
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if h.Name != tc.wantName || h.ProtocolVersion != tc.wantPV {
				t.Errorf("got %+v, want name=%s pv=%d", h, tc.wantName, tc.wantPV)
			}
		})
	}
}
