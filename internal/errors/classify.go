package errors

import (
	"errors"
	"sync"
)

// ClassifiedHint describes the metadata associated with a registered sentinel
// error. It is the source of truth that ClassifyError uses to populate the
// Category, Code, and Hint fields of a Structured error when the given error
// chain matches a known sentinel.
type ClassifiedHint struct {
	Category Category
	Code     string // stable machine-readable identifier (e.g. "PARSE_INVALID_YAML")
	Hint     string // concrete next action; empty string means no actionable hint applies
}

// RegisteredError pairs a sentinel error with its classification and the
// source-level var name it was declared under. The Name is used by coverage
// tests to confirm that every Err* var in the tree has been registered.
type RegisteredError struct {
	Name string // matches the var name in source, e.g. "ErrInvalidYAML"
	Err  error
	Hint ClassifiedHint
}

// typeClassifier pairs a matcher function with a ClassifiedHint. It is used for
// typed errors that carry struct payloads and therefore
// cannot be matched by errors.Is in the sentinel registry.
type typeClassifier struct {
	Match func(error) bool
	Hint  ClassifiedHint
}

var (
	registryMu      sync.RWMutex
	registry        = map[error]ClassifiedHint{}
	packageEntries  = map[string][]RegisteredError{} // package short name -> entries
	typeClassifiers []typeClassifier                 // consulted after sentinel registry
)

// RegisterTypeClassifier adds a classifier that matches errors by Go type via
// the supplied matcher function (typically using errors.As). Sentinel matches
// take priority; type classifiers are consulted in registration order when no
// sentinel matches. Safe to call concurrently; intended for use from init().
func RegisterTypeClassifier(match func(error) bool, hint ClassifiedHint) {
	registryMu.Lock()
	defer registryMu.Unlock()
	typeClassifiers = append(typeClassifiers, typeClassifier{Match: match, Hint: hint})
}

// RegisterPackage registers all sentinel errors from a package at init() time.
// The short package name (e.g. "parser") is recorded for coverage introspection.
// Safe to call concurrently; intended for use from init() functions.
func RegisterPackage(pkg string, entries ...RegisteredError) {
	registryMu.Lock()
	defer registryMu.Unlock()
	packageEntries[pkg] = append(packageEntries[pkg], entries...)
	for _, e := range entries {
		if e.Err == nil {
			continue
		}
		registry[e.Err] = e.Hint
	}
}

// LookupHint returns the registered hint for err if the error chain contains
// a registered sentinel or matches a registered type classifier, and whether a
// match was found. Sentinel matches (via errors.Is) take priority over type
// classifiers. Type classifiers are consulted in registration order.
func LookupHint(err error) (ClassifiedHint, bool) {
	if err == nil {
		return ClassifiedHint{}, false
	}
	registryMu.RLock()
	defer registryMu.RUnlock()
	// Sentinel registry: errors.Is walks the error chain.
	for sentinel, hint := range registry {
		if errors.Is(err, sentinel) {
			return hint, true
		}
	}
	// Type classifiers: consulted in registration order; first match wins.
	for _, tc := range typeClassifiers {
		if tc.Match(err) {
			return tc.Hint, true
		}
	}
	return ClassifiedHint{}, false
}

// IsRegistered reports whether the given error (exact value) is in the registry.
// Used by coverage tests to verify every sentinel declared in the tree has been
// classified.
func IsRegistered(err error) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok := registry[err]
	return ok
}

// RegisteredPackages returns the set of package names that have called
// RegisterPackage, for test introspection.
func RegisteredPackages() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(packageEntries))
	for k := range packageEntries {
		out = append(out, k)
	}
	return out
}

// PackageRegistrations returns the sentinels registered for the given short
// package name. Order is registration order.
func PackageRegistrations(pkg string) []RegisteredError {
	registryMu.RLock()
	defer registryMu.RUnlock()
	entries := packageEntries[pkg]
	out := make([]RegisteredError, len(entries))
	copy(out, entries)
	return out
}

// RegisteredNames returns the set of (package, var-name) pairs currently in
// the registry, for coverage introspection. Each inner slice element is a
// sentinel name (e.g. "ErrInvalidYAML") registered under the given package.
func RegisteredNames() map[string][]string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make(map[string][]string, len(packageEntries))
	for pkg, entries := range packageEntries {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.Name != "" {
				names = append(names, e.Name)
			}
		}
		out[pkg] = names
	}
	return out
}

// ClassifyError produces a *Structured describing err. Precedence:
//
//  1. If err (or any wrapped error in its chain) already is a *Structured,
//     it is returned as-is so caller-provided context (FilePath, Line) is not
//     overwritten. If that *Structured has empty Code/Hint but a registered
//     sentinel matches, Code and Hint are filled in.
//  2. If the chain contains a *NetworkError, it is wrapped into a Structured
//     with Category=CategoryNetwork.
//  3. If the chain contains a registered sentinel, the hint's Category/Code/Hint
//     are used.
//  4. Otherwise, Category=CategoryInternal and the error message is used.
//
// ClassifyError never returns nil for a non-nil err.
func ClassifyError(err error) *Structured {
	if err == nil {
		return nil
	}

	// Case 1: chain already carries a *Structured.
	var existing *Structured
	if errors.As(err, &existing) {
		if existing.Code == "" || existing.Hint == "" {
			if hint, ok := LookupHint(err); ok {
				enriched := *existing
				if enriched.Code == "" {
					enriched.Code = hint.Code
				}
				if enriched.Hint == "" {
					enriched.Hint = hint.Hint
				}
				if enriched.Category == "" {
					enriched.Category = hint.Category
				}
				return &enriched
			}
		}
		return existing
	}

	// Case 2: network error in chain.
	var net *NetworkError
	if errors.As(err, &net) {
		return &Structured{
			Category: CategoryNetwork,
			Message:  net.Message,
			Hint:     net.Hint,
			Code:     networkCodeFor(net.Kind),
			Inner:    err,
		}
	}

	// Case 3: registered sentinel.
	if hint, ok := LookupHint(err); ok {
		return &Structured{
			Category: hint.Category,
			Message:  err.Error(),
			Hint:     hint.Hint,
			Code:     hint.Code,
			Inner:    err,
		}
	}

	// Case 4: unclassified.
	return &Structured{
		Category: CategoryInternal,
		Message:  err.Error(),
		Inner:    err,
	}
}

func networkCodeFor(kind NetworkErrorKind) string {
	switch kind {
	case NetworkDNS:
		return "NETWORK_DNS"
	case NetworkConnectionRefused:
		return "NETWORK_CONNECTION_REFUSED"
	case NetworkTLS:
		return "NETWORK_TLS"
	case NetworkTimeout:
		return "NETWORK_TIMEOUT"
	default:
		return "NETWORK_OTHER"
	}
}
