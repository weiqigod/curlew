// Package jsonpath provides minimal JSONPath evaluation for parsed JSON documents.
//
// Supported syntax:
//   - $ (root reference)
//   - .field (dot notation)
//   - [N] (array index)
//   - Chaining: $.data.items[0].name
package jsonpath

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrNotFound indicates the path matched nothing in the document.
var ErrNotFound = errors.New("no match at path")

// ErrInvalidPath indicates the path expression could not be parsed.
var ErrInvalidPath = errors.New("invalid JSONPath expression")

// segment represents one step in a JSONPath: either a field name or an array index.
type segment struct {
	field string // non-empty for field access
	index int    // >= 0 for array access; -1 means "not an index"
}

// Evaluate resolves a JSONPath expression against a parsed JSON document.
// The document should be the result of json.Unmarshal into any.
// Returns (value, nil) on success, (nil, nil) when the path points to JSON null,
// or (nil, ErrNotFound) when the path matches nothing.
func Evaluate(path string, doc any) (any, error) {
	segs, err := parse(path)
	if err != nil {
		return nil, err
	}

	cur := doc
	for _, seg := range segs {
		if seg.index >= 0 {
			arr, ok := cur.([]any)
			if !ok {
				return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
			}
			if seg.index >= len(arr) {
				return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
			}
			cur = arr[seg.index]
		} else {
			obj, ok := cur.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
			}
			val, exists := obj[seg.field]
			if !exists {
				return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
			}
			cur = val
		}
	}
	return cur, nil
}

// parse splits a JSONPath expression into segments.
func parse(path string) ([]segment, error) {
	if path == "" || path[0] != '$' {
		return nil, fmt.Errorf("%w: must start with $", ErrInvalidPath)
	}

	// Just the root
	if path == "$" {
		return nil, nil
	}

	rest := path[1:] // skip '$'
	var segs []segment

	for len(rest) > 0 {
		switch rest[0] {
		case '.':
			rest = rest[1:] // skip '.'
			if len(rest) == 0 {
				return nil, fmt.Errorf("%w: trailing dot", ErrInvalidPath)
			}
			// Read field name until '.', '[', or end
			end := strings.IndexAny(rest, ".[")
			if end == -1 {
				end = len(rest)
			}
			if end == 0 {
				return nil, fmt.Errorf("%w: empty field name", ErrInvalidPath)
			}
			segs = append(segs, segment{field: rest[:end], index: -1})
			rest = rest[end:]

		case '[':
			rest = rest[1:] // skip '['
			closeBracket := strings.IndexByte(rest, ']')
			if closeBracket == -1 {
				return nil, fmt.Errorf("%w: unclosed bracket", ErrInvalidPath)
			}
			idxStr := rest[:closeBracket]
			idx, err := strconv.Atoi(idxStr)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid array index %q", ErrInvalidPath, idxStr)
			}
			if idx < 0 {
				return nil, fmt.Errorf("%w: negative array index in %s", ErrInvalidPath, path)
			}
			segs = append(segs, segment{index: idx})
			rest = rest[closeBracket+1:]

		default:
			return nil, fmt.Errorf("%w: unexpected character %q", ErrInvalidPath, rest[0])
		}
	}
	return segs, nil
}
