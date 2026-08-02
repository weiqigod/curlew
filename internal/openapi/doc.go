// Package openapi parses OpenAPI 3.0/3.1 specifications and emits minimal
// apitest collection skeletons. Skeletons include method, URL (with path
// parameters interpolated as {{name}}), request name, and a base_url variable
// derived from the spec's servers[0]. Request bodies, headers, and assertions
// are handled in M3-006.
package openapi
