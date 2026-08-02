package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// JSONLEntry is a single structured log entry for --log output.
//
// RunID and RequestID are correlation fields populated by the exec command
// (M11-004). RunID is a 32-char lowercase hex string minted per invocation;
// RequestID is the per-request identifier (always "req-1" for exec, since
// exec runs a single request). Both are omitempty so older consumers parsing
// log files written before M11-004 see no schema change.
type JSONLEntry struct {
	Timestamp  string `json:"timestamp"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	StatusCode int    `json:"status_code,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Error      string `json:"error,omitempty"`
	DryRun     bool   `json:"dry_run,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
}

// AppendJSONL appends a single JSON-encoded line to the file at path.
// Creates the file if it does not exist.
func AppendJSONL(path string, entry *JSONLEntry) (err error) {
	f, openErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if openErr != nil {
		return fmt.Errorf("opening log file: %w", openErr)
	}
	defer func() {
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	}()

	data, marshalErr := json.Marshal(entry)
	if marshalErr != nil {
		return fmt.Errorf("encoding log entry: %w", marshalErr)
	}
	data = append(data, '\n')
	if _, writeErr := f.Write(data); writeErr != nil {
		return fmt.Errorf("writing log entry: %w", writeErr)
	}
	return nil
}
