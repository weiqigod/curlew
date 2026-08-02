package plugin

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestWriteRequest_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := writeRequest(&buf, rpcRequest{ID: 1, Method: "curlew/hello"}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("missing trailing newline: %q", got)
	}
	var back rpcRequest
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &back); err != nil {
		t.Fatal(err)
	}
	if back.JSONRPC != "2.0" || back.Method != "curlew/hello" {
		t.Errorf("round-trip mismatch: %+v", back)
	}
}

func TestReadResponse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
		check   func(*testing.T, rpcResponse)
	}{
		{
			name:    "valid hello result",
			input:   `{"jsonrpc":"2.0","id":1,"result":{"name":"x","version":"1","hooks":[],"protocol_version":1}}` + "\n",
			wantErr: nil,
			check: func(t *testing.T, r rpcResponse) {
				t.Helper()
				if r.ID != 1 {
					t.Error("id mismatch")
				}
			},
		},
		{
			name:    "missing jsonrpc field",
			input:   `{"id":1,"result":{}}` + "\n",
			wantErr: ErrHandshakeProtocol,
		},
		{
			name:    "malformed json",
			input:   "not-json\n",
			wantErr: ErrHandshakeProtocol,
		},
		{
			name:    "eof before newline",
			input:   "",
			wantErr: io.EOF,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := bufio.NewReader(strings.NewReader(tc.input))
			resp, err := readResponse(r)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.check != nil {
				tc.check(t, resp)
			}
		})
	}
}
