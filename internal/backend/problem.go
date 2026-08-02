package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ErrNotProblem is returned by DecodeProblem when the response body is not
// application/problem+json. Callers should treat this as a non-fatal signal
// and fall through to plain text or another decoder.
var ErrNotProblem = errors.New("backend: response is not application/problem+json")

// ProblemDetails models RFC 7807 application/problem+json with the
// project-specific Code and RequestID extensions documented at
// SPECIFICATION.md:8243-8267. Callers MUST branch on Code, NEVER on Status
// or Title — Code is the stable machine-readable identifier; Status and
// Title may shift across releases without breaking compatibility.
//
// Extensions captures any additional JSON fields present in the problem body
// (e.g. "previous_grant" in TRIAL_ALREADY_CONSUMED responses). Values are
// stored as raw JSON; callers can unmarshal into concrete types as needed.
type ProblemDetails struct {
	Type       string                     `json:"type"`
	Title      string                     `json:"title"`
	Status     int                        `json:"status"`
	Detail     string                     `json:"detail,omitempty"`
	Code       string                     `json:"code"`
	RequestID  string                     `json:"request_id,omitempty"`
	Extensions map[string]json.RawMessage `json:"-"`
}

// Error implements the error interface, formatting the RFC 7807 fields in
// a stable order so logs are diff-friendly.
func (p *ProblemDetails) Error() string {
	if p.RequestID != "" {
		return fmt.Sprintf("backend: %s (status=%d, code=%s, request_id=%s)",
			p.Title, p.Status, p.Code, p.RequestID)
	}
	return fmt.Sprintf("backend: %s (status=%d, code=%s)", p.Title, p.Status, p.Code)
}

// knownProblemFields are the standard RFC 7807 + project fields decoded
// explicitly by ProblemDetails. All other fields land in Extensions.
var knownProblemFields = map[string]struct{}{
	"type":       {},
	"title":      {},
	"status":     {},
	"detail":     {},
	"code":       {},
	"request_id": {},
}

// DecodeProblem parses an application/problem+json response body into a
// typed *ProblemDetails. Returns ErrNotProblem if the Content-Type is
// missing or not application/problem+json (caller should branch elsewhere).
// The response body is fully consumed and closed regardless of outcome.
//
// Any JSON fields beyond the standard set (type, title, status, detail, code,
// request_id) are captured in ProblemDetails.Extensions as raw JSON values.
// This allows callers such as trial-error handling to access fields like
// "previous_grant" without requiring knowledge of every possible extension.
func DecodeProblem(resp *http.Response) (*ProblemDetails, error) {
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read body: %w", readErr)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/problem+json") {
		return nil, ErrNotProblem
	}
	var pd ProblemDetails
	if err := json.Unmarshal(body, &pd); err != nil {
		return nil, fmt.Errorf("decode problem+json: %w", err)
	}
	if pd.Status == 0 {
		pd.Status = resp.StatusCode
	}
	if pd.RequestID == "" {
		pd.RequestID = resp.Header.Get("X-Request-Id")
	}

	// Capture extension fields (anything not in the standard set).
	var all map[string]json.RawMessage
	if err := json.Unmarshal(body, &all); err == nil {
		ext := make(map[string]json.RawMessage, len(all))
		for k, v := range all {
			if _, known := knownProblemFields[k]; !known {
				ext[k] = v
			}
		}
		if len(ext) > 0 {
			pd.Extensions = ext
		}
	}

	return &pd, nil
}
