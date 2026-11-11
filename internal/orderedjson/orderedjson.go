// Package orderedjson holds a JSON object that keeps its keys in the order they
// were written. Request bodies written as YAML maps decode into it, so the body
// sent on the wire lists keys as the collection does rather than sorted.
package orderedjson

import (
	"bytes"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Object is a JSON object with ordered keys. Keys lists every key of Values
// once, in order. Values hold the same kinds as a decoded JSON document,
// with nested objects as Object.
type Object struct {
	Keys   []string
	Values map[string]any
}

// MarshalJSON encodes the object with its keys in order.
func (o Object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.Keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := json.Marshal(o.Values[k])
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", k, err)
		}
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// String renders the object as compact JSON, so printing a body shows what is
// sent.
func (o Object) String() string {
	b, err := o.MarshalJSON()
	if err != nil {
		return fmt.Sprintf("%v", o.Values)
	}
	return string(b)
}

// FromYAML decodes a YAML node into the value it would decode to as `any`,
// except that mappings become Object with their keys in document order, at
// every nesting level. Aliases are followed. A mapping that uses a merge key
// (<<) decodes as a plain map, since its combined key order is not written
// anywhere.
func FromYAML(node *yaml.Node) (any, error) {
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil, nil
		}
		return FromYAML(node.Content[0])
	case yaml.AliasNode:
		return FromYAML(node.Alias)
	case yaml.MappingNode:
		if hasMergeKey(node) {
			var v any
			if err := node.Decode(&v); err != nil {
				return nil, err
			}
			return v, nil
		}
		obj := Object{Keys: make([]string, 0, len(node.Content)/2), Values: make(map[string]any, len(node.Content)/2)}
		for i := 0; i+1 < len(node.Content); i += 2 {
			var key string
			if err := node.Content[i].Decode(&key); err != nil {
				return nil, fmt.Errorf("line %d: object keys must be text: %w", node.Content[i].Line, err)
			}
			val, err := FromYAML(node.Content[i+1])
			if err != nil {
				return nil, err
			}
			if _, dup := obj.Values[key]; !dup {
				obj.Keys = append(obj.Keys, key)
			}
			obj.Values[key] = val
		}
		return obj, nil
	case yaml.SequenceNode:
		out := make([]any, 0, len(node.Content))
		for _, n := range node.Content {
			val, err := FromYAML(n)
			if err != nil {
				return nil, err
			}
			out = append(out, val)
		}
		return out, nil
	default:
		var v any
		if err := node.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	}
}

func hasMergeKey(node *yaml.Node) bool {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Tag == "!!merge" {
			return true
		}
	}
	return false
}
