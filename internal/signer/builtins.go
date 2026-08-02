package signer

import (
	"fmt"
	"sync"
)

// builtinFactories holds first-party signer factories registered at
// init time by subpackages (e.g. internal/signer/awssigv4). It is
// distinct from the per-Run *Registry returned by NewBuiltinRegistry —
// NewWithBuiltins copies each entry into a fresh *Registry so callers
// get an isolated, mutable instance.
var (
	builtinMu        sync.Mutex
	builtinFactories = make(map[string]Factory)
)

// Register installs factory under name in the package-level builtins
// map consulted by NewWithBuiltins. Returns an error when name is
// already registered. Subpackages call this from init().
func Register(name string, factory Factory) error {
	builtinMu.Lock()
	defer builtinMu.Unlock()
	if _, dup := builtinFactories[name]; dup {
		return fmt.Errorf("signer type %q already registered", name)
	}
	builtinFactories[name] = factory
	return nil
}

// MustRegister panics on duplicate registration. Use from init().
func MustRegister(name string, factory Factory) {
	if err := Register(name, factory); err != nil {
		panic(err)
	}
}

// Unregister removes name from the package-level builtins map. It is a
// no-op when name is not registered. Tests use this to clean up factories
// registered via Register so that parallel test runs do not see stale
// registrations. Production callers may use it to deregister a factory
// before replacing it with a newer version.
func Unregister(name string) {
	builtinMu.Lock()
	defer builtinMu.Unlock()
	delete(builtinFactories, name)
}

// NewWithBuiltins returns a registry preloaded with every signer that
// registered itself via Register at package init time. Each call
// returns a fresh, independent *Registry — mutations to the returned
// registry do not affect the package-level builtins map.
func NewWithBuiltins() *Registry {
	r := NewBuiltinRegistry()
	builtinMu.Lock()
	defer builtinMu.Unlock()
	for name, factory := range builtinFactories {
		_ = r.Register(name, factory) // safe: r is fresh
	}
	return r
}
