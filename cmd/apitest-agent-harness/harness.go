//go:build never

// Package harness is a test-only package containing the agent validation harness
// for M6-007. It builds and executes the real apitest binary against a curated
// set of deliberately-broken fixture collections, asserts each failure event
// satisfies the agent-diagnosability contract, and gates the v0.1 → v1.0
// schema promotion.
//
// The package is excluded from release builds via the "never" build tag.
// Run the harness with: go test ./cmd/apitest-agent-harness/...
package harness
