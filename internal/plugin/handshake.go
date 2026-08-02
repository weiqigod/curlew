package plugin

import (
	"encoding/json"
	"fmt"
)

// helloResponse is the result field from an apitest/hello JSON-RPC response.
type helloResponse struct {
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Hooks           []string `json:"hooks"`
	ProtocolVersion int      `json:"protocol_version"`
}

// parseHello decodes a rpcResponse.Result into a helloResponse and validates
// the required fields (name, version). If protocol_version is absent it
// defaults to 1.
func parseHello(raw json.RawMessage) (helloResponse, error) {
	var h helloResponse
	if err := json.Unmarshal(raw, &h); err != nil {
		return h, fmt.Errorf("%w: decode hello: %v", ErrHandshakeProtocol, err)
	}
	if h.Name == "" {
		return h, fmt.Errorf("%w: missing name", ErrHandshakeProtocol)
	}
	if h.Version == "" {
		return h, fmt.Errorf("%w: missing version", ErrHandshakeProtocol)
	}
	if h.ProtocolVersion == 0 {
		h.ProtocolVersion = 1
	}
	return h, nil
}
