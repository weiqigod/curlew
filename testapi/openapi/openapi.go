// Package openapi holds the hand-written OpenAPI document mudflat serves at
// /openapi.json.
//
// It is a package of its own because go:embed cannot reach outside its own
// directory, and because the file being separately addressable is the point:
// §9.P requires the document to be hand-written and version-controlled rather
// than generated from the handlers. A generated document would make the round
// trip — import it, run what comes out — a tautology, since both sides would
// derive from the same source.
package openapi

import _ "embed"

// Document is the OpenAPI 3.1 description of a slice of the structured layer.
//
//go:embed mudflat-openapi.json
var Document []byte
