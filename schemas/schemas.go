// Package schemas owns the canonical JSON Schema files published at repo
// root. Files in this package are the source of truth; internal packages
// expose thin aliases to their byte slices so callers get a stable API
// while editors (via yaml.schemas in .vscode/settings.json) can point at
// the versioned files directly.
package schemas

import _ "embed"

// CollectionV1 is the JSON Schema (Draft 2020-12) for curlew collection
// YAML files, v1. See schemas/collection-v1.json.
//
//go:embed collection-v1.json
var CollectionV1 []byte

// ProjectV1 is the JSON Schema (Draft 2020-12) for curlew.yaml project config
// files, v1. See schemas/project-v1.json.
//
//go:embed project-v1.json
var ProjectV1 []byte
