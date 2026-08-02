package openapi

import (
	"fmt"
	"io"

	"github.com/weiqigod/curlew/internal/parser"
	"gopkg.in/yaml.v3"
)

// writerCollection is a YAML-friendly projection of parser.Collection used
// strictly for emission. parser.Collection has custom UnmarshalYAML on
// SensitiveVars and Section that would not round-trip via yaml.Marshal, so we
// define our own outbound shape here.
type writerCollection struct {
	Name      string              `yaml:"name"`
	Variables map[string]string   `yaml:"variables,omitempty"`
	Requests  []writerRequestItem `yaml:"requests"`
}

type writerAssertions struct {
	Status []int `yaml:"status,omitempty"`
}

type writerRequestItem struct {
	Name       string           `yaml:"name"`
	Request    writerRequest    `yaml:"request"`
	Assertions writerAssertions `yaml:"assertions,omitempty"`
}

type writerRequest struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Body    any               `yaml:"body,omitempty"`
}

// nilIfEmpty returns nil when m is empty, otherwise m. This prevents emitting
// an empty map as `headers: {}` in YAML (the omitempty tag handles nil but not
// empty maps for the yaml.v3 encoder).
func nilIfEmpty[K comparable, V any](m map[K]V) map[K]V {
	if len(m) == 0 {
		return nil
	}
	return m
}

// Emit writes col to w as a YAML collection suitable for curlew run/validate.
func Emit(w io.Writer, col *parser.Collection) (err error) {
	out := writerCollection{
		Name:      col.Name,
		Variables: col.Variables.Values,
		Requests:  make([]writerRequestItem, 0, len(col.Requests.Items)),
	}
	for _, it := range col.Requests.Items {
		wi := writerRequestItem{
			Name: it.Name,
			Request: writerRequest{
				Method:  it.Request.Method,
				URL:     it.Request.URL,
				Headers: nilIfEmpty(it.Request.Headers),
				Body:    it.Request.Body,
			},
		}
		if len(it.Assertions.Status.Codes) > 0 {
			wi.Assertions = writerAssertions{Status: it.Assertions.Status.Codes}
		}
		out.Requests = append(out.Requests, wi)
	}
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	// Close flushes the stream-end event. For the plain structs encoded here,
	// yaml.v3's Close() performs no additional writes and cannot return an
	// error in practice; nonetheless we capture the error to satisfy the
	// resource-leak guard. The cerr branch is an accepted coverage gap.
	defer func() {
		if cerr := enc.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	if err := enc.Encode(&out); err != nil {
		return fmt.Errorf("encoding collection: %w", err)
	}
	return nil
}
