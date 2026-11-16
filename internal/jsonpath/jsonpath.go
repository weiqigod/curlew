// Package jsonpath provides minimal JSONPath evaluation for parsed JSON documents.
//
// Supported syntax:
//   - $ (root reference)
//   - .field (dot notation)
//   - ['field'] and ["field"] (name selectors, for keys with hyphens, dots or spaces)
//   - [N] (array index)
//   - Chaining: $.data.items[0].name
package jsonpath

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
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
			if len(rest) > 0 && (rest[0] == '\'' || rest[0] == '"') {
				name, n, err := parseQuotedName(rest)
				if err != nil {
					return nil, err
				}
				rest = rest[n:]
				if len(rest) == 0 || rest[0] != ']' {
					return nil, fmt.Errorf("%w: name selector must be closed by ] in %s", ErrInvalidPath, path)
				}
				segs = append(segs, segment{field: name, index: -1})
				rest = rest[1:]
				continue
			}
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

// parseQuotedName reads a quoted name selector (RFC 9535 §2.3.1) at the start
// of s: 'name' or "name", with the escapes \' \" \\ \/ \b \f \n \r \t and
// \uXXXX. It returns the decoded name and the number of bytes consumed,
// including both quotes.
func parseQuotedName(s string) (string, int, error) {
	quote := s[0]
	var b strings.Builder
	i := 1
	for i < len(s) {
		c := s[i]
		switch {
		case c == quote:
			return b.String(), i + 1, nil
		case c == '\\':
			if i+1 >= len(s) {
				return "", 0, fmt.Errorf("%w: unterminated name selector", ErrInvalidPath)
			}
			esc := s[i+1]
			i += 2
			switch esc {
			case '\'', '"', '\\', '/':
				b.WriteByte(esc)
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'u':
				r, n, err := parseUnicodeEscape(s[i:])
				if err != nil {
					return "", 0, err
				}
				b.WriteRune(r)
				i += n
			default:
				return "", 0, fmt.Errorf("%w: invalid escape \\%c in name selector", ErrInvalidPath, esc)
			}
		default:
			b.WriteByte(c)
			i++
		}
	}
	return "", 0, fmt.Errorf("%w: unterminated name selector", ErrInvalidPath)
}

// parseUnicodeEscape decodes the four hex digits after \u, and a following
// \uXXXX low surrogate when the first is a high surrogate. It returns the
// rune and the number of bytes consumed after the first "\u".
func parseUnicodeEscape(s string) (rune, int, error) {
	hex4 := func(h string) (rune, error) {
		if len(h) < 4 {
			return 0, fmt.Errorf("%w: short \\u escape in name selector", ErrInvalidPath)
		}
		v, err := strconv.ParseUint(h[:4], 16, 32)
		if err != nil {
			return 0, fmt.Errorf("%w: invalid \\u escape in name selector", ErrInvalidPath)
		}
		return rune(v), nil
	}
	r, err := hex4(s)
	if err != nil {
		return 0, 0, err
	}
	if utf16.IsSurrogate(r) && len(s) >= 10 && s[4] == '\\' && s[5] == 'u' {
		low, err := hex4(s[6:])
		if err == nil {
			if pair := utf16.DecodeRune(r, low); pair != unicode.ReplacementChar {
				return pair, 10, nil
			}
		}
	}
	return r, 4, nil
}
