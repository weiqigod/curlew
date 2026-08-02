package loadgen

import (
	"errors"
	"fmt"
	"os"

	"github.com/weiqigod/curlew/internal/httpexec"
	"gopkg.in/yaml.v3"
)

// ErrRequestFileNotFound is returned when the request file path does not exist.
var ErrRequestFileNotFound = errors.New("request file not found")

// requestFile mirrors the external-request YAML format but is intentionally
// scoped down (no assertions, no extract, no retry).
type requestFile struct {
	Name    string `yaml:"name"`
	Request struct {
		Method  string            `yaml:"method"`
		URL     string            `yaml:"url"`
		Headers map[string]string `yaml:"headers,omitempty"`
		Query   map[string]string `yaml:"query,omitempty"`
		Body    any               `yaml:"body,omitempty"`
	} `yaml:"request"`
}

// LoadRequestFile reads a YAML request file and returns an httpexec.Request.
// Only the { name, request: { method, url, headers, query, body } } fields
// are decoded; all other YAML fields are silently ignored.
func LoadRequestFile(path string) (*httpexec.Request, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrRequestFileNotFound, path)
		}
		return nil, fmt.Errorf("reading request file %s: %w", path, err)
	}

	var rf requestFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parsing request file %s: %w", path, err)
	}

	if rf.Request.URL == "" {
		return nil, fmt.Errorf("request file %s: missing required field 'request.url'", path)
	}

	method := rf.Request.Method
	if method == "" {
		method = "GET"
	}

	return &httpexec.Request{
		Method:      method,
		URL:         rf.Request.URL,
		Headers:     rf.Request.Headers,
		QueryParams: rf.Request.Query,
		Body:        rf.Request.Body,
	}, nil
}
