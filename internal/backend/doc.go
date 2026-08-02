// Package backend implements the CLI side of the apitest network boundary.
//
// This package provides the HTTP client for communicating with the apitool
// backend service, including RFC 7807 problem-details error decoding, a
// single-flight flock guard for concurrent token refresh, and a hybrid
// OS-keychain + AES-256-GCM encrypted-file fallback for refresh/access-token storage.
//
// Spec references:
//   - docs/SPECIFICATION.md:7938-7944  flock single-flight policy
//   - docs/SPECIFICATION.md:7946-7951  hybrid keychain + encrypted-file storage
//   - docs/SPECIFICATION.md:8195-8202  Bearer-only Authorization header policy
//   - docs/SPECIFICATION.md:8243-8267  RFC 7807 problem-details error model
//
// # Error model
//
// All non-2xx responses from the backend are decoded as *ProblemDetails when
// the Content-Type is application/problem+json. Callers MUST branch on
// ProblemDetails.Code, NEVER on Status or Title — Code is the stable
// machine-readable identifier; Status and Title may shift across releases
// without breaking compatibility.
package backend
