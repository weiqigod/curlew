package plugin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// rpcRequest is a JSON-RPC 2.0 request object (newline-framed).
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcError is the JSON-RPC 2.0 error object embedded in a response.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// rpcResponse is a JSON-RPC 2.0 response object.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// writeRequest encodes req as a single newline-framed JSON-RPC 2.0 line and
// writes it to w. It always sets req.JSONRPC = "2.0".
func writeRequest(w io.Writer, req rpcRequest) error {
	req.JSONRPC = "2.0"
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	if _, err := w.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write request: %w", err)
	}
	return nil
}

// readResponse reads one newline-framed JSON-RPC 2.0 response from r.
// Returns a wrapped ErrHandshakeProtocol for malformed responses.
func readResponse(r *bufio.Reader) (rpcResponse, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return rpcResponse{}, fmt.Errorf("read response: %w", err)
	}
	var resp rpcResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return rpcResponse{}, fmt.Errorf("%w: %v", ErrHandshakeProtocol, err)
	}
	if resp.JSONRPC != "2.0" {
		return rpcResponse{}, fmt.Errorf("%w: missing jsonrpc field", ErrHandshakeProtocol)
	}
	return resp, nil
}
