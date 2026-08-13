package openapi

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// OpenAPI 3.1 support (§11C.9).
//
// Both docs/CLI_SPECIFICATION.md §18.8 and docs/MANUAL.md promise "OpenAPI
// 3.x", and the importer accepted a document declaring `openapi: 3.1.0` and
// then validated it against 3.0 rules. Ordinary 3.1 constructs were refused:
//
//	info.summary             added in 3.1          invalid info: extra sibling fields: [summary]
//	webhooks                 added in 3.1          extra sibling fields: [webhooks]
//	type: ["string","null"]  JSON Schema 2020-12   unsupported 'type' value "null"
//
// The validator is kin-openapi's, which implements 3.0. Swapping it for a
// 3.1-native library is a large dependency for a small gap — the import only
// needs paths, parameters, request bodies and response codes, and 3.1 changed
// none of those in ways that matter here. So a 3.1 document is translated into
// the 3.0 spelling of the same meaning before validation, and only when the
// document declares 3.1; a 3.0 document is passed through untouched.
//
// The translation is deliberately small and additive: each entry below is one
// construct, and the ones that cannot be represented are reported rather than
// silently dropped.

// dropped31RootKeys are 3.1 root fields with no 3.0 equivalent that the import
// does not read. jsonSchemaDialect names the schema vocabulary; `webhooks` is
// handled separately because dropping it is worth saying out loud.
var dropped31RootKeys = []string{"jsonSchemaDialect"}

// dropped31InfoKeys are 3.1 additions to the Info object. The import reads only
// info.title.
var dropped31InfoKeys = []string{"summary"}

// isOpenAPI31 reports whether the document declares 3.1.
func isOpenAPI31(root map[string]any) bool {
	v, _ := root["openapi"].(string)
	return strings.HasPrefix(v, "3.1")
}

// relax31 translates the 3.1-only constructs in raw into their 3.0 spelling and
// returns the rewritten document. A document that does not declare 3.1 is
// returned unchanged, so the 3.0 path is exactly as it was.
//
// warn receives one message per construct that could not be carried across.
func relax31(raw []byte, warn func(string)) ([]byte, error) {
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		// Not a mapping, or not parseable. Hand the original bytes back and let
		// the loader produce the error, which will be about the real problem.
		return raw, nil //nolint:nilerr // deliberate: the loader reports it better
	}
	if !isOpenAPI31(root) {
		return raw, nil
	}

	for _, k := range dropped31RootKeys {
		delete(root, k)
	}
	if info, ok := root["info"].(map[string]any); ok {
		for _, k := range dropped31InfoKeys {
			delete(info, k)
		}
		// license.identifier is a 3.1 alternative to license.url.
		if lic, licOK := info["license"].(map[string]any); licOK {
			delete(lic, "identifier")
		}
	}
	if hooks, ok := root["webhooks"].(map[string]any); ok {
		delete(root, "webhooks")
		warn(fmt.Sprintf("openapi 3.1 webhooks are not imported: %s. "+
			"A webhook describes a callback the server sends to you, so there is no request to make.",
			strings.Join(sortedAnyKeys(hooks), ", ")))
	}

	normalizeTypeArrays(root, warn)

	out, err := yaml.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("re-encoding openapi 3.1 document: %w", err)
	}
	return out, nil
}

// normalizeTypeArrays rewrites JSON Schema 2020-12 type arrays into 3.0's
// single type plus `nullable`. It walks the whole document because schemas
// appear under components, parameters, request bodies and responses alike.
//
// It acts only when `type` holds a LIST, which is what makes it safe to run
// everywhere: a security scheme's `type` is always a string, so it is never
// touched.
func normalizeTypeArrays(node any, warn func(string)) {
	switch n := node.(type) {
	case map[string]any:
		if raw, ok := n["type"]; ok {
			if list, isList := raw.([]any); isList {
				applyTypeArray(n, list, warn)
			}
		}
		for _, v := range n {
			normalizeTypeArrays(v, warn)
		}
	case []any:
		for _, v := range n {
			normalizeTypeArrays(v, warn)
		}
	}
}

// applyTypeArray replaces schema["type"] with the 3.0 equivalent of the given
// type list.
func applyTypeArray(schema map[string]any, list []any, warn func(string)) {
	var concrete []string
	nullable := false
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			continue
		}
		if s == "null" {
			nullable = true
			continue
		}
		concrete = append(concrete, s)
	}

	if nullable {
		schema["nullable"] = true
	}

	switch len(concrete) {
	case 1:
		// The common case by far: ["string","null"] is 3.0's
		// `type: string, nullable: true`.
		schema["type"] = concrete[0]
	case 0:
		// A schema typed only `null`. 3.0 has no spelling for it; leaving the
		// type off makes the schema unconstrained, which is the closest honest
		// answer.
		delete(schema, "type")
	default:
		// A genuine union — ["string","integer"]. 3.0 cannot express it, and
		// picking one member would quietly change the document's meaning.
		delete(schema, "type")
		warn(fmt.Sprintf("openapi 3.1 union type [%s] has no 3.0 equivalent; "+
			"the schema is treated as unconstrained and any generated body value for it is a guess",
			strings.Join(concrete, ", ")))
	}
}

// sortedAnyKeys is the map[string]any counterpart of schema.go's sortedKeys,
// which is typed to openapi3.Schemas.
func sortedAnyKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
