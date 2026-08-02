// Package schema exposes the curlew JSON Schemas to internal callers. The
// canonical files live at schemas/*.json so that editor tooling (yaml.schemas
// via redhat.vscode-yaml) can point at stable, versioned in-repo paths. This
// package aliases the bytes owned by the schemas/ package at repo root.
package schema

import "github.com/weiqigod/curlew/schemas"

// CollectionSchema is the JSON Schema for curlew collection YAML files.
var CollectionSchema = schemas.CollectionV1

// ProjectSchema is the JSON Schema for curlew.yaml project configuration files.
var ProjectSchema = schemas.ProjectV1
